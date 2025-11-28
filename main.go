package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/qtgolang/SunnyNet/SunnyNet"
	"github.com/qtgolang/SunnyNet/public"

	"wx_channel/internal/config"
	"wx_channel/internal/handlers"
	"wx_channel/internal/storage"
	"wx_channel/internal/utils"
	"wx_channel/pkg/argv"
	"wx_channel/pkg/certificate"
	"wx_channel/pkg/proxy"
)

//go:embed certs/SunnyRoot.cer
var cert_data []byte

//go:embed lib/FileSaver.min.js
var file_saver_js []byte

//go:embed lib/jszip.min.js
var zip_js []byte

//go:embed inject/main.js
var main_js []byte

var Sunny = SunnyNet.NewSunny()
var cfg *config.Config
var v string
var port int
var currentPageURL = "" // 存储当前页面的完整URL
var logInitMsg string

// 全局管理器
var (
	csvManager     *storage.CSVManager
	fileManager    *storage.FileManager
	apiHandler     *handlers.APIHandler
	uploadHandler  *handlers.UploadHandler
	recordHandler  *handlers.RecordHandler
	scriptHandler  *handlers.ScriptHandler
	batchHandler   *handlers.BatchHandler
	commentHandler *handlers.CommentHandler
)

// downloadRecordsHeader CSV 文件的表头
var downloadRecordsHeader = []string{"ID", "标题", "视频号名称", "视频号分类", "公众号名称", "视频链接", "页面链接", "文件大小", "时长", "阅读量", "点赞量", "评论量", "收藏数", "转发数", "创建时间", "IP所在地", "下载时间", "页面来源", "搜索关键词"}

// initDownloadRecords 初始化下载记录系统
func initDownloadRecords() error {
	// 获取基础目录
	baseDir, err := utils.GetBaseDir()
	if err != nil {
		return fmt.Errorf("获取基础目录失败: %v", err)
	}

	// 创建文件管理器
	downloadsDir := filepath.Join(baseDir, cfg.DownloadsDir)
	fileManager, err = storage.NewFileManager(downloadsDir)
	if err != nil {
		return fmt.Errorf("创建文件管理器失败: %v", err)
	}

	// 创建CSV管理器
	csvPath := filepath.Join(downloadsDir, cfg.RecordsFile)
	csvManager, err = storage.NewCSVManager(csvPath, downloadRecordsHeader)
	if err != nil {
		return fmt.Errorf("创建CSV管理器失败: %v", err)
	}

	return nil
}

// 已废弃的辅助函数：addDownloadRecord 已移除，避免未使用告警

// saveDynamicHTML 保存动态页面的完整HTML内容，按日期和域名归档
func saveDynamicHTML(htmlContent string, parsedURL *url.URL, fullURL string, timestamp int64) {
	if fileManager == nil {
		utils.Warn("文件管理器未初始化，无法保存页面内容: %s", fullURL)
		return
	}
	if cfg == nil {
		utils.Warn("配置尚未初始化，无法保存页面内容: %s", fullURL)
		return
	}
	// 检查是否启用页面快照保存
	if !cfg.SavePageSnapshot {
		return
	}
	if htmlContent == "" {
		utils.Warn("收到空的HTML内容，跳过保存: %s", fullURL)
		return
	}
	if parsedURL == nil {
		utils.Warn("解析页面URL失败，跳过保存: %s", fullURL)
		return
	}

	if cfg.SaveDelay > 0 {
		time.Sleep(cfg.SaveDelay)
	}

	saveTime := time.Now()
	if timestamp > 0 {
		saveTime = time.Unix(0, timestamp*int64(time.Millisecond))
	}

	baseDir, err := utils.GetBaseDir()
	if err != nil {
		utils.HandleError(err, "获取基础目录用于保存页面内容")
		return
	}

	downloadsDir := filepath.Join(baseDir, cfg.DownloadsDir)
	if err := utils.EnsureDir(downloadsDir); err != nil {
		utils.HandleError(err, "创建下载目录用于保存页面内容")
		return
	}

	pagesRoot := filepath.Join(downloadsDir, "page_snapshots")
	if err := utils.EnsureDir(pagesRoot); err != nil {
		utils.HandleError(err, "创建页面保存根目录")
		return
	}

	// 去掉域名文件夹，直接使用日期目录
	dateDir := filepath.Join(pagesRoot, saveTime.Format("2006-01-02"))
	if err := utils.EnsureDir(dateDir); err != nil {
		utils.HandleError(err, "创建页面保存日期目录")
		return
	}

	var filenameParts []string
	if parsedURL.Path != "" && parsedURL.Path != "/" {
		segments := strings.Split(parsedURL.Path, "/")
		for _, segment := range segments {
			segment = strings.TrimSpace(segment)
			if segment == "" || segment == "." {
				continue
			}
			filenameParts = append(filenameParts, utils.CleanFilename(segment))
		}
	}

	if parsedURL.RawQuery != "" {
		querySegment := strings.ReplaceAll(parsedURL.RawQuery, "&", "_")
		querySegment = strings.ReplaceAll(querySegment, "=", "-")
		querySegment = utils.CleanFilename(querySegment)
		if querySegment != "" {
			filenameParts = append(filenameParts, querySegment)
		}
	}

	if len(filenameParts) == 0 {
		filenameParts = append(filenameParts, "page")
	}

	baseName := strings.Join(filenameParts, "_")
	// CleanFilename 已经处理了长度限制，这里不需要再次限制
	
	fileName := fmt.Sprintf("%s_%s.html", saveTime.Format("150405"), baseName)
	targetPath := utils.GenerateUniqueFilename(dateDir, fileName, 100)

	if err := os.WriteFile(targetPath, []byte(htmlContent), 0644); err != nil {
		utils.HandleError(err, "保存页面HTML内容")
		return
	}

	metaData := map[string]interface{}{
		"url":       fullURL,
		"host":      parsedURL.Host,
		"path":      parsedURL.Path,
		"query":     parsedURL.RawQuery,
		"saved_at":  saveTime.Format(time.RFC3339),
		"timestamp": timestamp,
	}

	metaBytes, err := json.MarshalIndent(metaData, "", "  ")
	if err == nil {
		metaPath := strings.TrimSuffix(targetPath, filepath.Ext(targetPath)) + ".meta.json"
		if err := os.WriteFile(metaPath, metaBytes, 0644); err != nil {
			utils.HandleError(err, "保存页面HTML元数据")
		}
	} else {
		utils.HandleError(err, "序列化页面HTML元数据")
	}

	relativePath, err := filepath.Rel(downloadsDir, targetPath)
	if err != nil {
		relativePath = targetPath
	}
	utils.Info("页面HTML已保存: %s -> %s", fullURL, relativePath)
	utils.LogInfo("[页面快照] URL=%s | 路径=%s", fullURL, relativePath)
}

// saveSearchData 保存搜索页面的结构化数据（账号信息、直播数据、动态数据）
func saveSearchData(fullURL string, parsedURL *url.URL, keyword string, profiles, liveResults, feedResults []map[string]interface{}, timestamp int64) {
	if fileManager == nil {
		utils.Warn("文件管理器未初始化，无法保存搜索数据: %s", fullURL)
		return
	}
	if cfg == nil {
		utils.Warn("配置尚未初始化，无法保存搜索数据: %s", fullURL)
		return
	}
	// 检查是否启用搜索数据保存
	if !cfg.SaveSearchData {
		return
	}
	if parsedURL == nil {
		utils.Warn("解析搜索页面URL失败，跳过保存: %s", fullURL)
		return
	}

	if cfg.SaveDelay > 0 {
		time.Sleep(cfg.SaveDelay)
	}

	saveTime := time.Now()
	if timestamp > 0 {
		saveTime = time.Unix(0, timestamp*int64(time.Millisecond))
	}

	baseDir, err := utils.GetBaseDir()
	if err != nil {
		utils.HandleError(err, "获取基础目录用于保存搜索数据")
		return
	}

	downloadsDir := filepath.Join(baseDir, cfg.DownloadsDir)
	if err := utils.EnsureDir(downloadsDir); err != nil {
		utils.HandleError(err, "创建下载目录用于保存搜索数据")
		return
	}

	searchDataRoot := filepath.Join(downloadsDir, "search_data")
	if err := utils.EnsureDir(searchDataRoot); err != nil {
		utils.HandleError(err, "创建搜索数据根目录")
		return
	}

	// 去掉域名文件夹，直接使用日期目录
	dateDir := filepath.Join(searchDataRoot, saveTime.Format("2006-01-02"))
	if err := utils.EnsureDir(dateDir); err != nil {
		utils.HandleError(err, "创建搜索数据日期目录")
		return
	}

	// 构建文件名
	sanitizedKeyword := utils.CleanFilename(keyword)
	if sanitizedKeyword == "" {
		sanitizedKeyword = "search"
	}
	// CleanFilename 已经处理了长度限制（100字符），这里不需要再次限制

	fileName := fmt.Sprintf("%s_%s.json", saveTime.Format("150405"), sanitizedKeyword)
	targetPath := utils.GenerateUniqueFilename(dateDir, fileName, 100)

	// 构建数据结构
	searchData := map[string]interface{}{
		"url":          fullURL,
		"host":         parsedURL.Host,
		"path":         parsedURL.Path,
		"query":        parsedURL.RawQuery,
		"keyword":      keyword,
		"profiles":     profiles,
		"liveResults":  liveResults,
		"feedResults":  feedResults,
		"profileCount": len(profiles),
		"liveCount":    len(liveResults),
		"feedCount":    len(feedResults),
		"saved_at":     saveTime.Format(time.RFC3339),
		"timestamp":    timestamp,
	}

	// 保存JSON数据
	dataBytes, err := json.MarshalIndent(searchData, "", "  ")
	if err != nil {
		utils.HandleError(err, "序列化搜索数据")
		return
	}

	if err := os.WriteFile(targetPath, dataBytes, 0644); err != nil {
		utils.HandleError(err, "保存搜索数据")
		return
	}

	relativePath, err := filepath.Rel(downloadsDir, targetPath)
	if err != nil {
		relativePath = targetPath
	}
	utils.Info("搜索数据已保存: 关键词=%s, 账号=%d, 直播=%d, 动态=%d -> %s",
		keyword, len(profiles), len(liveResults), len(feedResults), relativePath)
	utils.LogInfo("[搜索数据] 关键词=%s | 账号=%d | 直播=%d | 动态=%d | 路径=%s",
		keyword, len(profiles), len(liveResults), len(feedResults), relativePath)
}

// printDownloadRecordInfo 打印下载记录信息
func printDownloadRecordInfo() {
	utils.PrintSeparator()
	color.Blue("📋 下载记录信息")
	utils.PrintSeparator()

	baseDir, err := utils.GetBaseDir()
	if err != nil {
		utils.HandleError(err, "获取基础目录")
		return
	}

	recordsPath := filepath.Join(baseDir, cfg.DownloadsDir, cfg.RecordsFile)
	utils.PrintLabelValue("📁", "记录文件", recordsPath)
	utils.PrintLabelValue("✏️", "记录格式", "CSV表格格式")
	utils.PrintLabelValue("📊", "记录字段", strings.Join(downloadRecordsHeader, ", "))
	utils.PrintSeparator()
}

// 打印帮助信息
func print_usage() {
	fmt.Printf("Usage: wx_video_download [OPTION...]\n")
	fmt.Printf("Download WeChat video.\n\n")
	fmt.Printf("      --help                 display this help and exit\n")
	fmt.Printf("  -v, --version              output version information and exit\n")
	fmt.Printf("  -p, --port                 set proxy server network port\n")
	fmt.Printf("  -d, --dev                  set proxy server network device\n")
	fmt.Printf("      --uninstall            uninstall root certificate and exit\n")
	os.Exit(0)
}

// 卸载证书
func uninstall_certificate() {
	color.Yellow("正在卸载根证书...\n")

	// 检查证书是否存在
	existing, err := certificate.CheckCertificate("SunnyNet")
	if err != nil {
		color.Red("检查证书时发生错误: %v\n", err.Error())
		color.Yellow("请手动检查证书是否已安装。\n")
		os.Exit(1)
	}

	if !existing {
		color.Green("✓ 证书未安装，无需卸载。\n")
		os.Exit(0)
	}

	// 尝试卸载证书
	err = certificate.RemoveCertificate("SunnyNet")
	if err != nil {
		color.Red("卸载证书失败: %v\n", err.Error())
		color.Yellow("请尝试以管理员身份运行此命令。\n")
		os.Exit(1)
	}

	color.Green("✓ 证书卸载成功！\n")
	color.Yellow("注意：如果程序仍在运行，请重启浏览器以确保更改生效。\n")
	os.Exit(0)
}

// printTitle 打印标题
func printTitle() {
	color.Set(color.FgCyan)
	fmt.Println("")
	fmt.Println(" ██╗    ██╗██╗  ██╗     ██████╗██╗  ██╗ █████╗ ███╗   ██╗███╗   ██╗███████╗██╗     ")
	fmt.Println(" ██║    ██║╚██╗██╔╝    ██╔════╝██║  ██║██╔══██╗████╗  ██║████╗  ██║██╔════╝██║     ")
	fmt.Println(" ██║ █╗ ██║ ╚███╔╝     ██║     ███████║███████║██╔██╗ ██║██╔██╗ ██║█████╗  ██║     ")
	fmt.Println(" ██║███╗██║ ██╔██╗     ██║     ██╔══██║██╔══██║██║╚██╗██║██║╚██╗██║██╔══╝  ██║     ")
	fmt.Println(" ╚███╔███╔╝██╔╝ ██╗    ╚██████╗██║  ██║██║  ██║██║ ╚████║██║ ╚████║███████╗███████╗")
	fmt.Println("  ╚══╝╚══╝ ╚═╝  ╚═╝     ╚═════╝╚═╝  ╚═╝╚═╝  ╚═╝╚═╝  ╚═══╝╚═╝  ╚═══╝╚══════╝╚══════╝")
	color.Unset()

	color.Yellow("    微信视频号下载助手 v%s", cfg.Version)
	color.Yellow("    项目地址：https://github.com/nobiyou/wx_channel")
	color.Green("    v5.0.0 更新要点：")
	color.Green("    • 代码重构，完善文档")
	color.Green("    • 搜索批量下载，主页批量下载")
	color.Green("    • 下载保持评论")
	color.Green("    • web控制台批量下载")
	color.Green("    • 修复已知bug")
	fmt.Println()
}

// 格式化视频时长为时分秒
// formatDuration 和 formatNumber 已移至 internal/utils/output.go
func main() {
	// 初始化配置
	cfg = config.Load()
	// 记录配置加载
	utils.LogConfigLoad("config.yaml", true)
	
	// 初始化日志（可选滚动）
	if cfg.LogFile != "" {
		_ = utils.InitLoggerWithRotation(utils.INFO, cfg.LogFile, cfg.MaxLogSizeMB)
		logInitMsg = fmt.Sprintf("日志已初始化: %s (最大 %dMB)", cfg.LogFile, cfg.MaxLogSizeMB)
	}
	port = cfg.Port
	v = "?t=" + cfg.Version

	os_env := runtime.GOOS
	args := argv.ArgsToMap(os.Args) // 分解参数列表为Map
	if _, ok := args["help"]; ok {
		print_usage()
	} // 存在help则输出帮助信息并退出主程序
	if v, ok := args["v"]; ok { // 存在v则输出版本信息并退出主程序
		fmt.Printf("v%s %.0s\n", cfg.Version, v)
		os.Exit(0)
	}
	if v, ok := args["version"]; ok { // 存在version则输出版本信息并退出主程序
		fmt.Printf("v%s %.0s\n", cfg.Version, v)
		os.Exit(0)
	}
	if _, ok := args["uninstall"]; ok { // 存在uninstall则卸载证书并退出主程序
		uninstall_certificate()
	}
	// 设置参数默认值
	args["dev"] = argv.ArgsValue(args, "", "d", "dev")
	args["port"] = argv.ArgsValue(args, "", "p", "port")

	iport, errstr := strconv.Atoi(args["port"])
	if errstr != nil {
		args["port"] = strconv.Itoa(cfg.DefaultPort) // 用户自定义值解析失败则使用默认端口
	} else {
		port = iport
		cfg.SetPort(port)
	}

	delete(args, "p") // 删除冗余的参数p
	delete(args, "d") // 删除冗余的参数d

	signalChan := make(chan os.Signal, 1)
	// Notify the signal channel on SIGINT (Ctrl+C) and SIGTERM
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-signalChan
		color.Red("\n正在关闭服务...%v\n\n", sig)
		// 记录系统关闭
		utils.LogSystemShutdown(fmt.Sprintf("收到信号: %v", sig))
		if os_env == "darwin" {
			proxy.DisableProxyInMacOS(proxy.ProxySettings{
				Device:   args["dev"],
				Hostname: "127.0.0.1",
				Port:     args["port"],
			})
		}
		os.Exit(0)
	}()

	// 打印标题和程序信息
	printTitle()

	// 初始化下载记录系统
	if err := initDownloadRecords(); err != nil {
		utils.HandleError(err, "初始化下载记录系统")
	} else {
		printDownloadRecordInfo()
		if logInitMsg != "" {
			utils.Info(logInitMsg)
			logInitMsg = ""
		}
	}

	// 初始化API处理器
	apiHandler = handlers.NewAPIHandler(cfg)

	// 初始化上传处理器（需要在csvManager初始化之后）
	if csvManager != nil {
		uploadHandler = handlers.NewUploadHandler(cfg, csvManager)
		// 初始化记录处理器
		recordHandler = handlers.NewRecordHandler(cfg, csvManager)
	}

	// 初始化脚本处理器
	scriptHandler = handlers.NewScriptHandler(cfg, main_js, zip_js, file_saver_js, v)

	// 初始化批量下载处理器
	if csvManager != nil {
		batchHandler = handlers.NewBatchHandler(cfg, csvManager)
	}

	// 初始化评论处理器
	commentHandler = handlers.NewCommentHandler(cfg)

	existing, err1 := certificate.CheckCertificate("SunnyNet")
	if err1 != nil {
		utils.HandleError(err1, "检查证书")
		utils.Warn("程序将继续运行，但HTTPS功能可能受限...")
		existing = false // 假设证书未安装
	} else if !existing {
		utils.Info("正在安装证书...")
		err := certificate.InstallCertificate(cert_data)
		time.Sleep(cfg.CertInstallDelay)
		if err != nil {
			utils.HandleError(err, "证书安装")
			utils.Warn("程序将继续运行，但HTTPS功能可能受限。")
			utils.Warn("如需完整功能，请手动安装证书或以管理员身份运行程序。")

			// 保存证书文件到 downloads 目录，方便用户手动安装
			if fileManager != nil {
				baseDir, err := utils.GetBaseDir()
				if err == nil {
					downloadsDir := filepath.Join(baseDir, cfg.DownloadsDir)
					certPath := filepath.Join(downloadsDir, cfg.CertFile)
					if err := utils.EnsureDir(downloadsDir); err == nil {
						if err := os.WriteFile(certPath, cert_data, 0644); err == nil {
							utils.Info("证书文件已保存到: %s", certPath)
							utils.Info("您可以双击此文件手动安装证书。")
						} else {
							utils.HandleError(err, "保存证书文件")
						}
					}
				}
			}
		} else {
			utils.Info("✓ 证书安装成功！")
		}
	} else {
		utils.Info("✓ 证书已存在，无需重新安装。")
	}
	Sunny.SetPort(port)
	Sunny.SetGoCallback(HttpCallback, nil, nil, nil)
	err := Sunny.Start().Error
	if err != nil {
		utils.HandleError(err, "启动代理服务")
		utils.Warn("按 Ctrl+C 退出...")
		select {}
	}
	proxy_server := fmt.Sprintf("127.0.0.1:%v", port)
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(&url.URL{
				Scheme: "http",
				Host:   proxy_server,
			}),
		},
	}
	_, err3 := client.Get("https://sunny.io/")
	if err3 == nil {
		if os_env == "windows" {
			ok := Sunny.StartProcess()
			if !ok {
				color.Red("\nERROR 启动进程代理失败，检查是否以管理员身份运行\n")
				color.Yellow("按 Ctrl+C 退出...\n")
				select {}
			}
			Sunny.ProcessAddName("WeChatAppEx.exe")
		}

		// 打印服务状态信息
		utils.PrintSeparator()
		color.Blue("📡 服务状态信息")
		utils.PrintSeparator()

		utils.PrintLabelValue("⏳", "服务状态", "已启动")
		utils.PrintLabelValue("🔌", "代理端口", port)
		utils.PrintLabelValue("📱", "支持平台", "微信视频号")

		// 记录系统启动
		proxyMode := "进程代理"
		if os_env != "windows" {
			proxyMode = "系统代理"
		}
		utils.LogSystemStart(port, proxyMode)
		
		utils.Info("🔍 请打开需要下载的视频号页面进行下载")
	} else {
		utils.PrintSeparator()
		utils.Warn("⚠️ 您还未安装证书，请在浏览器打开 http://%v 并根据说明安装证书", proxy_server)
		utils.Warn("⚠️ 在安装完成后重新启动此程序即可")
		utils.PrintSeparator()
	}
	utils.Info("💡 服务正在运行，按 Ctrl+C 退出...")
	select {}
}

type ChannelProfile struct {
	Title string `json:"title"`
}
type FrontendTip struct {
	Msg string `json:"msg"`
}

func HttpCallback(Conn *SunnyNet.HttpConn) {
	host := Conn.Request.URL.Hostname()
	path := Conn.Request.URL.Path
	if Conn.Type == public.HttpSendRequest {
		// Conn.Request.Header.Set("Cache-Control", "no-cache")
		Conn.Request.Header.Del("Accept-Encoding")

		// 处理静态文件请求
		if handlers.HandleStaticFiles(Conn, zip_js, file_saver_js) {
			return
		}

		// 处理API请求
		if apiHandler != nil {
			// 处理profile请求
			if apiHandler.HandleProfile(Conn) {
				return
			}
			// 处理tip请求
			if apiHandler.HandleTip(Conn) {
				return
			}
			// 处理page_url请求
			if apiHandler.HandlePageURL(Conn) {
				currentPageURL = apiHandler.GetCurrentURL() // 同步URL
				// 同步URL到recordHandler
				if recordHandler != nil {
					recordHandler.SetCurrentURL(currentPageURL)
				}
				return
			}
		}

		// 处理上传相关API请求
		if uploadHandler != nil {
			// 处理分片上传初始化
			if uploadHandler.HandleInitUpload(Conn) {
				return
			}
			// 处理分片上传
			if uploadHandler.HandleUploadChunk(Conn) {
				return
			}
			// 处理分片上传完成
			if uploadHandler.HandleCompleteUpload(Conn) {
				return
			}
			// 查询已上传分片
			if uploadHandler.HandleUploadStatus(Conn) {
				return
			}
			// 处理直接保存视频
			if uploadHandler.HandleSaveVideo(Conn) {
				return
			}
		}

		// 处理记录相关API请求
		if recordHandler != nil {
			// 处理记录下载信息
			if recordHandler.HandleRecordDownload(Conn) {
				return
			}
			// 处理导出视频列表
			if recordHandler.HandleExportVideoList(Conn) {
				return
			}
			// 处理导出视频列表(JSON)
			if recordHandler.HandleExportVideoListJSON(Conn) {
				return
			}
			// 处理导出视频列表(Markdown)
			if recordHandler.HandleExportVideoListMarkdown(Conn) {
				return
			}
			// 处理批量下载状态
			if recordHandler.HandleBatchDownloadStatus(Conn) {
				return
			}
		}

		// 处理批量下载相关API请求
		if batchHandler != nil {
			if batchHandler.HandleBatchStart(Conn) {
				return
			}
			if batchHandler.HandleBatchProgress(Conn) {
				return
			}
			if batchHandler.HandleBatchCancel(Conn) {
				return
			}
			if batchHandler.HandleBatchFailed(Conn) {
				return
			}
		}

		// 处理评论数据保存请求
		if commentHandler != nil {
			if commentHandler.HandleSaveCommentData(Conn) {
				return
			}
		}

		// 提供 Web 控制台
		if path == "/console" || path == "/console/" {
			consoleHTML, err := os.ReadFile("web/console.html")
			if err != nil {
				utils.Warn("无法读取 web/console.html: %v", err)
				Conn.StopRequest(404, "Console not found", http.Header{})
				return
			}
			headers := http.Header{}
			headers.Set("Content-Type", "text/html; charset=utf-8")
			Conn.StopRequest(200, string(consoleHTML), headers)
			return
		}

		// 处理预检请求（CORS）
		if strings.HasPrefix(path, "/__wx_channels_api/") && Conn.Request.Method == "OPTIONS" {
			headers := http.Header{}
			headers.Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			headers.Set("Access-Control-Allow-Headers", "Content-Type, X-Local-Auth")
			// 若配置了允许的 Origin 且来路匹配，回显 origin
			if cfg != nil && len(cfg.AllowedOrigins) > 0 {
				origin := Conn.Request.Header.Get("Origin")
				for _, o := range cfg.AllowedOrigins {
					if o == origin {
						headers.Set("Access-Control-Allow-Origin", origin)
						headers.Set("Vary", "Origin")
						break
					}
				}
			}
			Conn.StopRequest(204, "", headers)
			return
		}

		// 保存页面完整内容的API端点（用于测试，保留在main.go中）
		if path == "/__wx_channels_api/save_page_content" {
			var contentData struct {
				URL       string `json:"url"`
				HTML      string `json:"html"`
				Timestamp int64  `json:"timestamp"`
			}
			body, err := io.ReadAll(Conn.Request.Body)
			if err != nil {
				utils.HandleError(err, "读取save_page_content请求体")
				return
			}
			if err := Conn.Request.Body.Close(); err != nil {
				utils.HandleError(err, "关闭请求体")
			}
			err = json.Unmarshal(body, &contentData)
			if err != nil {
				utils.HandleError(err, "解析页面内容数据")
			} else {
				parsedURL, err := url.Parse(contentData.URL)
				if err != nil {
					utils.HandleError(err, "解析页面内容URL")
				} else {
					saveDynamicHTML(contentData.HTML, parsedURL, contentData.URL, contentData.Timestamp)
				}
			}
			headers := http.Header{}
			headers.Set("Content-Type", "application/json")
			headers.Set("__debug", "fake_resp")
			Conn.StopRequest(200, "{}", headers)
			return
		}

		// 保存搜索页面结构化数据的API端点
		if path == "/__wx_channels_api/save_search_data" {
			var searchData struct {
				URL         string                   `json:"url"`
				Keyword     string                   `json:"keyword"`
				Profiles    []map[string]interface{} `json:"profiles"`    // 账号信息
				LiveResults []map[string]interface{} `json:"liveResults"` // 直播数据
				FeedResults []map[string]interface{} `json:"feedResults"` // 动态数据
				Timestamp   int64                    `json:"timestamp"`
			}
			body, err := io.ReadAll(Conn.Request.Body)
			if err != nil {
				utils.HandleError(err, "读取save_search_data请求体")
				return
			}
			if err := Conn.Request.Body.Close(); err != nil {
				utils.HandleError(err, "关闭请求体")
			}
			err = json.Unmarshal(body, &searchData)
			if err != nil {
				utils.HandleError(err, "解析搜索数据")
			} else {
				parsedURL, err := url.Parse(searchData.URL)
				if err != nil {
					utils.HandleError(err, "解析搜索页面URL")
				} else {
					saveSearchData(searchData.URL, parsedURL, searchData.Keyword, searchData.Profiles, searchData.LiveResults, searchData.FeedResults, searchData.Timestamp)
				}
			}
			headers := http.Header{}
			headers.Set("Content-Type", "application/json")
			headers.Set("__debug", "fake_resp")
			Conn.StopRequest(200, "{}", headers)
			return
		}
	}
	if Conn.Type == public.HttpResponseOK {
		if Conn.Response.Body != nil {
			Body, _ := io.ReadAll(Conn.Response.Body)
			_ = Conn.Response.Body.Close()

			// 记录JS文件请求（调试用）
			if strings.Contains(path, ".js") {
				contentType := strings.ToLower(Conn.Response.Header.Get("content-type"))
				utils.LogInfo("[响应] Path=%s | ContentType=%s", path, contentType)
			}

			// 使用ScriptHandler处理HTML响应
			if scriptHandler != nil {
				if scriptHandler.HandleHTMLResponse(Conn, host, path, Body) {
					return
				}
			}

			// 使用ScriptHandler处理JavaScript响应
			if scriptHandler != nil {
				if scriptHandler.HandleJavaScriptResponse(Conn, host, path, Body) {
					return
				}
			}

			// 如果没有被ScriptHandler处理，使用原始响应
			Conn.Response.Body = io.NopCloser(bytes.NewBuffer(Body))
		}

	}
	if Conn.Type == public.HttpRequestFail {
		// 请求错误处理
	}
}

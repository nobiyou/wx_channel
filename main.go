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
	"wx_channel/internal/models"
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

// 全局管理器
var (
	csvManager    *storage.CSVManager
	fileManager   *storage.FileManager
	apiHandler    *handlers.APIHandler
	uploadHandler *handlers.UploadHandler
	recordHandler *handlers.RecordHandler
	scriptHandler *handlers.ScriptHandler
)

// downloadRecordsHeader CSV 文件的表头
var downloadRecordsHeader = []string{"ID", "标题", "视频号名称", "视频号分类", "公众号名称", "视频链接", "页面链接", "文件大小", "时长", "阅读量", "点赞量", "评论量", "收藏数", "转发数", "创建时间", "IP所在地", "下载时间"}

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

// addDownloadRecord 添加下载记录
func addDownloadRecord(record *models.VideoDownloadRecord) error {
	if csvManager == nil {
		return fmt.Errorf("CSV管理器未初始化")
	}
	return csvManager.AddRecord(record)
}

// checkExistingRecord 已移至storage模块，不再需要

// saveDynamicHTML 保存动态HTML内容（已禁用，保留函数声明）
func saveDynamicHTML(html, host, path, fullURL string, timestamp int64) {
	// 只保存微信视频号相关的HTML页面
	if host != "channels.weixin.qq.com" {
		return
	}

	// 创建HTML保存目录
	currentDir, err := os.Getwd()
	if err != nil {
		fmt.Printf("获取当前目录失败: %v\n", err)
		return
	}

	htmlDir := filepath.Join(currentDir, "downloads", "dynamic_html_pages")
	if err := os.MkdirAll(htmlDir, 0755); err != nil {
		fmt.Printf("创建动态HTML保存目录失败: %v\n", err)
		return
	}

	// 生成文件名：使用时间戳和URL信息
	timestampStr := time.Unix(timestamp/1000, 0).Format("20060102_150405")
	pathSafe := strings.ReplaceAll(strings.Trim(path, "/"), "/", "_")
	if pathSafe == "" {
		pathSafe = "root"
	}

	// 如果URL包含视频ID或其他标识符，尝试提取
	videoID := ""
	if parsedURL, err := url.Parse(fullURL); err == nil {
		if fragment := parsedURL.Fragment; fragment != "" {
			// 提取fragment中的信息作为视频ID
			if len(fragment) > 50 {
				videoID = "_" + fragment[:20] + "..." // 截取前20个字符
			} else {
				videoID = "_" + fragment
			}
			// 清理文件名中的特殊字符
			videoID = strings.ReplaceAll(videoID, "=", "_")
			videoID = strings.ReplaceAll(videoID, "&", "_")
			videoID = strings.ReplaceAll(videoID, "?", "_")
			videoID = strings.ReplaceAll(videoID, "/", "_")
		}
	}

	filename := fmt.Sprintf("%s_%s_%s%s_dynamic.html", host, pathSafe, timestampStr, videoID)
	filePath := filepath.Join(htmlDir, filename)

	// 保存HTML文件
	file, err := os.Create(filePath)
	if err != nil {
		fmt.Printf("创建动态HTML文件失败: %v\n", err)
		return
	}
	defer file.Close()

	// 写入UTF-8 BOM以确保中文正确显示
	_, err = file.Write([]byte{0xEF, 0xBB, 0xBF})
	if err != nil {
		fmt.Printf("写入UTF-8 BOM失败: %v\n", err)
		return
	}

	// 写入HTML内容
	_, err = file.WriteString(html)
	if err != nil {
		fmt.Printf("写入动态HTML内容失败: %v\n", err)
		return
	}

	// 打印保存信息
	utils.PrintSeparator()
	color.Green("🎯 已保存动态加载后的完整HTML页面")
	utils.PrintLabelValue("📄", "文件名", filename)
	utils.PrintLabelValue("📁", "路径", htmlDir)
	utils.PrintLabelValue("🌐", "完整URL", fullURL)
	utils.PrintLabelValue("📊", "内容大小", fmt.Sprintf("%.2f KB", float64(len(html))/1024))
	utils.PrintSeparator()
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

	color.Yellow("    视频号下载助手 v%s", cfg.Version)
	color.Yellow("    项目地址：https://github.com/nobiyou/wx_channel")
	color.Green("    更新内容：")
	color.Green("    🎯 主页视频批量下载功能")
	color.Green("       - 📦 新增主页批量采集，自动采集所有视频")
	color.Green("       - 🎬 手动下载模式，可自定义保存位置")
	color.Green("       - 🚀 自动下载模式，静默批量下载到软件目录")
	color.Green("       - 📊 实时进度显示，成功/失败统计")
	color.Green("       - 🔗 一键导出视频链接列表")
	color.Green("    ⚡ 分片上传优化")
	color.Green("       - 📦 全量分片上传，所有文件更稳定")
	color.Green("       - 🔄 自动重试机制，每片重试3次")
	color.Green("       - 📈 智能进度报告，实时显示百分比")
	color.Green("       - ✅ 文件名优化，自动添加时间前缀")
	color.Green("    🛠️ 技术改进")
	color.Green("       - 🔐 修复JSON转义，正确处理Windows路径")
	color.Green("       - 📁 文件名清理，移除非法字符和标签")
	color.Green("       - 🔢 冲突避免，同名文件自动编号")
	color.Green("       - 🎨 UI优化，日志更清晰简洁")
	color.Green("    💡 发现问题后给我留言，我会尽快修复")
	fmt.Println()
}

// 格式化视频时长为时分秒
// formatDuration 和 formatNumber 已移至 internal/utils/output.go
func main() {
	// 初始化配置
	cfg = config.Load()
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
			// 处理批量下载状态
			if recordHandler.HandleBatchDownloadStatus(Conn) {
				return
			}
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
				// 动态HTML保存功能已被禁用
				// 解析URL获取更详细的文件名信息
				// parsedURL, err := url.Parse(contentData.URL)
				// if err != nil {
				// 	fmt.Printf("解析URL失败: %v\n", err)
				// } else {
				// 	// 保存动态加载后的完整HTML内容
				// 	saveDynamicHTML(contentData.HTML, parsedURL.Host, parsedURL.Path, contentData.URL, contentData.Timestamp)
				// }
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

package main

import (
	"bytes"
	_ "embed"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/qtgolang/SunnyNet/SunnyNet"
	"github.com/qtgolang/SunnyNet/public"

	"wx_channel/pkg/argv"
	"wx_channel/pkg/certificate"
	"wx_channel/pkg/proxy"
	"wx_channel/pkg/util"
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
var version = "20250804"
var v = "?t=" + version
var port = 2025
var currentPageURL = "" // 存储当前页面的完整URL

// VideoDownloadRecord 存储视频下载记录
type VideoDownloadRecord struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Author       string    `json:"nickname"`      // 视频号名称
	AuthorType   string    `json:"author_type"`   // 视频号分类
	OfficialName string    `json:"official_name"` // 公众号名称
	URL          string    `json:"url"`
	PageURL      string    `json:"page_url"`
	FileSize     string    `json:"file_size"`
	Duration     string    `json:"duration"`
	PlayCount    string    `json:"play_count"`    // 播放量/阅读数
	LikeCount    string    `json:"like_count"`    // 点赞量
	CommentCount string    `json:"comment_count"` // 评论量
	FavCount     string    `json:"fav_count"`     // 收藏数
	ForwardCount string    `json:"forward_count"` // 转发数
	CreateTime   string    `json:"create_time"`   // 视频创建时间
	IPRegion     string    `json:"ip_region"`     // 视频发布IP所在地
	DownloadAt   time.Time `json:"download_at"`
}

var (
	// downloadRecordsLock 用于保护下载记录文件的并发访问
	downloadRecordsLock sync.Mutex
	// downloadRecordsFile 下载记录文件路径
	downloadRecordsFile string
	// downloadRecordsHeader CSV 文件的表头
	downloadRecordsHeader = []string{"ID", "标题", "视频号名称", "视频号分类", "公众号名称", "视频链接", "页面链接", "文件大小", "时长", "阅读量", "点赞量", "评论量", "收藏数", "转发数", "创建时间", "IP所在地", "下载时间"}
)

// initDownloadRecords 初始化下载记录系统
func initDownloadRecords() error {
	// 创建记录目录 - 使用当前程序目录
	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("获取当前目录失败: %v", err)
	}

	recordsDir := filepath.Join(currentDir, "downloads")
	if err := os.MkdirAll(recordsDir, 0755); err != nil {
		return fmt.Errorf("创建下载记录目录失败: %v", err)
	}

	// 设置记录文件路径
	downloadRecordsFile = filepath.Join(recordsDir, "download_records.csv")

	// 如果文件不存在，创建并写入表头
	if _, err := os.Stat(downloadRecordsFile); os.IsNotExist(err) {
		file, err := os.Create(downloadRecordsFile)
		if err != nil {
			return fmt.Errorf("创建下载记录文件失败: %v", err)
		}
		defer file.Close()

		// 写入UTF-8 BOM
		_, err = file.Write([]byte{0xEF, 0xBB, 0xBF})
		if err != nil {
			return fmt.Errorf("写入UTF-8 BOM失败: %v", err)
		}

		writer := csv.NewWriter(file)
		if err := writer.Write(downloadRecordsHeader); err != nil {
			return fmt.Errorf("写入表头失败: %v", err)
		}
		writer.Flush()

		if err := writer.Error(); err != nil {
			return fmt.Errorf("写入表头时出错: %v", err)
		}
	}

	return nil
}

// addDownloadRecord 添加下载记录
func addDownloadRecord(record VideoDownloadRecord) error {
	downloadRecordsLock.Lock()
	defer downloadRecordsLock.Unlock()

	// 检查是否已经存在相同的记录（防止重复记录）
	existing, err := checkExistingRecord(record)
	if err != nil {
		return fmt.Errorf("检查现有记录失败: %v", err)
	}

	if existing {
		// 记录已存在，不需要再次添加
		return nil
	}

	// 记录不存在，添加新记录
	file, err := os.OpenFile(downloadRecordsFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("打开下载记录文件失败: %v", err)
	}
	defer file.Close()

	// 格式化ID为文本格式，确保长数字ID不会被Excel等应用程序截断或显示为科学计数法
	formattedID := "ID_" + record.ID

	writer := csv.NewWriter(file)
	err = writer.Write([]string{
		formattedID,
		record.Title,
		record.Author,
		record.AuthorType,
		record.OfficialName,
		record.URL,
		record.PageURL,
		record.FileSize,
		record.Duration,
		record.PlayCount,
		record.LikeCount,
		record.CommentCount,
		record.FavCount,
		record.ForwardCount,
		record.CreateTime,
		record.IPRegion,
		record.DownloadAt.Format("2006-01-02 15:04:05"),
	})
	if err != nil {
		return fmt.Errorf("写入记录失败: %v", err)
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("写入记录时出错: %v", err)
	}

	return nil
}

// checkExistingRecord 检查记录是否已存在
func checkExistingRecord(record VideoDownloadRecord) (bool, error) {
	// 如果文件不存在，则直接返回不存在
	if _, err := os.Stat(downloadRecordsFile); os.IsNotExist(err) {
		return false, nil
	}

	// 打开文件
	file, err := os.Open(downloadRecordsFile)
	if err != nil {
		return false, fmt.Errorf("打开下载记录文件失败: %v", err)
	}
	defer file.Close()

	// 创建CSV读取器
	reader := csv.NewReader(file)
	// 跳过标题行
	_, err = reader.Read()
	if err != nil {
		if err == io.EOF {
			return false, nil
		}
		return false, fmt.Errorf("读取CSV标题失败: %v", err)
	}

	// 格式化当前记录ID，用于比较
	formattedID := "ID_" + record.ID

	// 读取所有记录
	for {
		row, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return false, fmt.Errorf("读取CSV行失败: %v", err)
		}

		// 检查是否是同一个视频的记录（只比较ID，不再比较质量）
		if len(row) >= 8 && row[0] == formattedID {
			return true, nil
		}
	}

	return false, nil
}

// saveDynamicHTML 保存动态加载后的完整HTML内容
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
	printSeparator()
	color.Green("🎯 已保存动态加载后的完整HTML页面")
	printLabelValue("📄", "文件名", filename, color.New(color.FgGreen))
	printLabelValue("📁", "路径", htmlDir, color.New(color.FgGreen))
	printLabelValue("🌐", "完整URL", fullURL, color.New(color.FgGreen))
	printLabelValue("📊", "内容大小", fmt.Sprintf("%.2f KB", float64(len(html))/1024), color.New(color.FgGreen))
	printSeparator()
}

// printDownloadRecordInfo 打印下载记录信息
func printDownloadRecordInfo() {
	printSeparator()
	color.Blue("📋 下载记录信息")
	printSeparator()

	currentDir, err := os.Getwd()
	if err != nil {
		color.Red("获取当前目录失败: %v", err)
		return
	}

	recordsPath := filepath.Join(currentDir, "downloads", "download_records.csv")
	printLabelValue("📁", "记录文件", recordsPath, color.New(color.FgGreen))
	printLabelValue("✏️", "记录格式", "CSV表格格式", color.New(color.FgGreen))
	printLabelValue("📊", "记录字段", strings.Join(downloadRecordsHeader, ", "), color.New(color.FgGreen))
	printSeparator()
}

// 打印帮助信息
func print_usage() {
	fmt.Printf("Usage: wx_video_download [OPTION...]\n")
	fmt.Printf("Download WeChat video.\n\n")
	fmt.Printf("      --help                 display this help and exit\n")
	fmt.Printf("  -v, --version              output version information and exit\n")
	fmt.Printf("  -p, --port                 set proxy server network port\n")
	fmt.Printf("  -d, --dev                  set proxy server network device\n")
	os.Exit(0)
}

// 打印分隔线
func printSeparator() {
	color.Cyan("─────────────────────────────────────────────────────────────────")
}

// 打印标题
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

	color.Yellow("    视频号下载助手 v%s", version)
	color.Green("    更新内容：")
	color.Green("    1. 添加videojs播放器的优化和视频缓存监控")
	color.Green("    2. 修复添加下载按钮失败问题，页面更新后导致")
	color.Green("    3. 发现问题后给我私信，我会尽快修复")
	fmt.Println()
}

// 打印带颜色的标签和值
func printLabelValue(icon string, label string, value interface{}, textColor *color.Color) {
	if textColor == nil {
		// 默认使用绿色
		textColor = color.New(color.FgGreen)
	}
	textColor.Printf("%-2s %-6s", icon, label+":")
	fmt.Println(value)
}

// 格式化视频时长为时分秒
func formatDuration(seconds float64) string {
	// 将毫秒转换为秒
	totalSeconds := int(seconds / 1000)

	// 计算时分秒
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	secs := totalSeconds % 60

	// 根据时长返回不同格式
	if hours > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, secs)
	}
	return fmt.Sprintf("%02d:%02d", minutes, secs)
}

func main() {
	os_env := runtime.GOOS
	args := argv.ArgsToMap(os.Args) // 分解参数列表为Map
	if _, ok := args["help"]; ok {
		print_usage()
	} // 存在help则输出帮助信息并退出主程序
	if v, ok := args["v"]; ok { // 存在v则输出版本信息并退出主程序
		fmt.Printf("v%s %.0s\n", version, v)
		os.Exit(0)
	}
	if v, ok := args["version"]; ok { // 存在version则输出版本信息并退出主程序
		fmt.Printf("v%s %.0s\n", version, v)
		os.Exit(0)
	}
	// 设置参数默认值
	args["dev"] = argv.ArgsValue(args, "", "d", "dev")
	args["port"] = argv.ArgsValue(args, "", "p", "port")

	iport, errstr := strconv.Atoi(args["port"])
	if errstr != nil {
		args["port"] = strconv.Itoa(port) // 用户自定义值解析失败则使用默认端口
	} else {
		port = iport
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
		color.Red("\n初始化下载记录系统失败: %v\n", err)
	} else {
		printDownloadRecordInfo()
	}

	existing, err1 := certificate.CheckCertificate("SunnyNet")
	if err1 != nil {
		color.Red("\nERROR %v\n", err1.Error())
		color.Yellow("按 Ctrl+C 退出...\n")
		select {}
	}
	if !existing {
		color.Yellow("\n\n正在安装证书...\n")
		err := certificate.InstallCertificate(cert_data)
		time.Sleep(3 * time.Second)
		if err != nil {
			color.Red("\nERROR %v\n", err.Error())
			color.Yellow("按 Ctrl+C 退出...\n")
			select {}
		}
	}
	Sunny.SetPort(port)
	Sunny.SetGoCallback(HttpCallback, nil, nil, nil)
	err := Sunny.Start().Error
	if err != nil {
		color.Red("\nERROR %v\n", err.Error())
		color.Yellow("按 Ctrl+C 退出...\n")
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
		printSeparator()
		color.Blue("📡 服务状态信息")
		printSeparator()

		printLabelValue("⏳", "服务状态", "已启动", color.New(color.FgGreen))
		printLabelValue("🔌", "代理端口", port, color.New(color.FgGreen))
		printLabelValue("📱", "支持平台", "微信视频号", color.New(color.FgGreen))

		color.Yellow("\n🔍 请打开需要下载的视频号页面进行下载")
	} else {
		printSeparator()
		color.Yellow("\n⚠️ 您还未安装证书，请在浏览器打开 http://%v 并根据说明安装证书", proxy_server)
		color.Yellow("⚠️ 在安装完成后重新启动此程序即可\n")
		printSeparator()
	}
	color.Cyan("\n💡 服务正在运行，按 Ctrl+C 退出...")
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
		if util.Includes(path, "jszip") {
			headers := http.Header{}
			headers.Set("Content-Type", "application/javascript")
			headers.Set("__debug", "local_file")
			Conn.StopRequest(200, zip_js, headers)
			return
		}
		if util.Includes(path, "FileSaver.min") {
			headers := http.Header{}
			headers.Set("Content-Type", "application/javascript")
			headers.Set("__debug", "local_file")
			Conn.StopRequest(200, file_saver_js, headers)
			return
		}
		if path == "/__wx_channels_api/profile" {
			var data map[string]interface{}
			body, _ := io.ReadAll(Conn.Request.Body)
			_ = Conn.Request.Body.Close()

			// 保存原始JSON数据到本地用于调试（调试用）
			// debugDir := filepath.Join("downloads", "profile_debug")
			// os.MkdirAll(debugDir, 0755)
			// timestamp := time.Now().Format("20060102_150405")
			// debugFile := filepath.Join(debugDir, fmt.Sprintf("profile_%s.json", timestamp))
			//
			// // 格式化JSON
			// var prettyJSON bytes.Buffer
			// err := json.Indent(&prettyJSON, body, "", "  ")
			// if err == nil {
			// 	os.WriteFile(debugFile, prettyJSON.Bytes(), 0644)
			// 	color.Cyan("💾 已保存原始数据到: %s\n", debugFile)
			// } else {
			// 	os.WriteFile(debugFile, body, 0644)
			// }

			err := json.Unmarshal(body, &data)
			if err != nil {
				fmt.Println(err.Error())
			} else {
				// 打印标题，保持原有功能
				printLabelValue("💡", "[提醒]", "视频已成功播放", color.New(color.FgYellow))
				printLabelValue("💡", "[提醒]", "可以在「更多」菜单中下载视频啦！", color.New(color.FgYellow))
				color.Yellow("\n")

				// 打印视频详细信息
				printSeparator()
				color.Blue("📊 视频详细信息")
				printSeparator()

				if nickname, ok := data["nickname"].(string); ok {
					printLabelValue("👤", "视频号名称", nickname, color.New(color.FgGreen))
				}
				if title, ok := data["title"].(string); ok {
					printLabelValue("📝", "视频标题", title, color.New(color.FgGreen))
				}

				if duration, ok := data["duration"].(float64); ok {
					printLabelValue("⏱️", "视频时长", formatDuration(duration), color.New(color.FgGreen))
				}
				if size, ok := data["size"].(float64); ok {
					sizeMB := size / (1024 * 1024)
					printLabelValue("📦", "视频大小", fmt.Sprintf("%.2f MB", sizeMB), color.New(color.FgGreen))
				}

				// 添加互动数据显示（显示所有数据，包括0）
				if likeCount, ok := data["likeCount"].(float64); ok {
					printLabelValue("👍", "点赞量", formatNumber(likeCount), color.New(color.FgGreen))
				}
				if commentCount, ok := data["commentCount"].(float64); ok {
					printLabelValue("💬", "评论量", formatNumber(commentCount), color.New(color.FgGreen))
				}
				if favCount, ok := data["favCount"].(float64); ok {
					printLabelValue("🔖", "收藏数", formatNumber(favCount), color.New(color.FgGreen))
				}
				if forwardCount, ok := data["forwardCount"].(float64); ok {
					printLabelValue("🔄", "转发数", formatNumber(forwardCount), color.New(color.FgGreen))
				}
				// readCount 在Home页通常为0，先不显示
				// if readCount, ok := data["readCount"].(float64); ok && readCount > 0 {
				// 	printLabelValue("👁️", "阅读量", formatNumber(readCount), color.New(color.FgGreen))
				// }

				// 添加创建时间
				if createtime, ok := data["createtime"].(float64); ok {
					t := time.Unix(int64(createtime), 0)
					printLabelValue("📅", "创建时间", t.Format("2006-01-02 15:04:05"), color.New(color.FgGreen))
				}

				// 添加IP所在地（从多个来源获取）
				locationFound := false

				// 方法1：从 ipRegionInfo 获取
				if ipRegionInfo, ok := data["ipRegionInfo"].(map[string]interface{}); ok {
					if regionText, ok := ipRegionInfo["regionText"].(string); ok && regionText != "" {
						printLabelValue("🌍", "IP所在地", regionText, color.New(color.FgGreen))
						locationFound = true
					}
				}

				// 方法2：从 contact.extInfo 获取
				if !locationFound {
					if contact, ok := data["contact"].(map[string]interface{}); ok {
						if extInfo, ok := contact["extInfo"].(map[string]interface{}); ok {
							var location string
							if province, ok := extInfo["province"].(string); ok && province != "" {
								location = province
								if city, ok := extInfo["city"].(string); ok && city != "" {
									location += " " + city
								}
								printLabelValue("🌍", "地理位置", location, color.New(color.FgGreen))
								locationFound = true
							}
						}
					}
				}

				// 注释掉调试信息
				// color.Blue("\n🔍 所有可能的数字字段:")
				// for key, value := range data {
				// 	if num, ok := value.(float64); ok {
				// 		fmt.Printf("  %s: %v\n", key, num)
				// 	}
				// }

				if fileFormat, ok := data["fileFormat"].([]interface{}); ok && len(fileFormat) > 0 {
					printLabelValue("🎞️", "视频格式", fileFormat, color.New(color.FgGreen))
				}
				if coverUrl, ok := data["coverUrl"].(string); ok {
					printLabelValue("🖼️", "视频封面", coverUrl, color.New(color.FgGreen))
				}
				if url, ok := data["url"].(string); ok {
					printLabelValue("🔗", "原始链接", url, color.New(color.FgGreen))
				}
				printSeparator()
				color.Yellow("\n\n")
			}
			headers := http.Header{}
			headers.Set("Content-Type", "application/json")
			headers.Set("__debug", "fake_resp")
			Conn.StopRequest(200, "{}", headers)
			return
		}
		if path == "/__wx_channels_api/tip" {
			var data FrontendTip
			body, _ := io.ReadAll(Conn.Request.Body)
			_ = Conn.Request.Body.Close()
			err := json.Unmarshal(body, &data)
			if err != nil {
				fmt.Println(err.Error())
			}
			printLabelValue("💡", "[提醒]", data.Msg, color.New(color.FgYellow))
			headers := http.Header{}
			headers.Set("Content-Type", "application/json")
			headers.Set("__debug", "fake_resp")
			Conn.StopRequest(200, "{}", headers)
			return
		}
	}
	if Conn.Type == public.HttpResponseOK {
		content_type := strings.ToLower(Conn.Response.Header.Get("content-type"))
		if Conn.Response.Body != nil {
			Body, _ := io.ReadAll(Conn.Response.Body)
			_ = Conn.Response.Body.Close()
			if content_type == "text/html; charset=utf-8" {
				html := string(Body)
				script_reg1 := regexp.MustCompile(`src="([^"]{1,})\.js"`)
				html = script_reg1.ReplaceAllString(html, `src="$1.js`+v+`"`)
				script_reg2 := regexp.MustCompile(`href="([^"]{1,})\.js"`)
				html = script_reg2.ReplaceAllString(html, `href="$1.js`+v+`"`)
				Conn.Response.Header.Set("__debug", "append_script")
				script2 := ""

				if host == "channels.weixin.qq.com" && (path == "/web/pages/feed" || path == "/web/pages/home") {
					// 添加我们的脚本
					script := fmt.Sprintf(`<script>%s</script>`, main_js)

					// 预先加载FileSaver.js库
					preloadScript := `<script>
					// 预加载FileSaver.js库
					(function() {
						const script = document.createElement('script');
						script.src = '/FileSaver.min.js';
						document.head.appendChild(script);
					})();
					</script>`

					// 添加下载记录功能到JavaScript代码
					downloadTrackerScript := `<script>
					// 确保FileSaver.js库已加载
					if (typeof saveAs === 'undefined') {
						console.log('加载FileSaver.js库');
						const script = document.createElement('script');
						script.src = '/FileSaver.min.js';
						script.onload = function() {
							console.log('FileSaver.js库加载成功');
						};
						document.head.appendChild(script);
					}

					// 跟踪已记录的下载，防止重复记录
					window.__wx_channels_recorded_downloads = {};

					// 添加下载记录功能
					window.__wx_channels_record_download = function(data) {
						// 检查是否已经记录过这个下载
						const recordKey = data.id;
						if (window.__wx_channels_recorded_downloads[recordKey]) {
							console.log("已经记录过此下载，跳过记录");
							return;
						}
						
						// 标记为已记录
						window.__wx_channels_recorded_downloads[recordKey] = true;
						
						// 发送到记录API
						fetch("/__wx_channels_api/record_download", {
							method: "POST",
							headers: {
								"Content-Type": "application/json"
							},
							body: JSON.stringify(data)
						});
					};
					
					// 覆盖原有的下载处理函数
					const originalHandleClick = window.__wx_channels_handle_click_download__;
					if (originalHandleClick) {
						window.__wx_channels_handle_click_download__ = function(sp) {
							// 调用原始函数进行下载
							originalHandleClick(sp);
							
							// 记录下载
							if (window.__wx_channels_store__ && window.__wx_channels_store__.profile) {
								const profile = {...window.__wx_channels_store__.profile};
								window.__wx_channels_record_download(profile);
							}
						};
					}
					
					// 覆盖当前视频下载函数
					const originalDownloadCur = window.__wx_channels_download_cur__;
					if (originalDownloadCur) {
						window.__wx_channels_download_cur__ = function() {
							// 调用原始函数进行下载
							originalDownloadCur();
							
							// 记录下载
							if (window.__wx_channels_store__ && window.__wx_channels_store__.profile) {
								const profile = {...window.__wx_channels_store__.profile};
								window.__wx_channels_record_download(profile);
							}
						};
					}
					
					// 修复封面下载函数
					window.__wx_channels_handle_download_cover = function() {
						if (window.__wx_channels_store__ && window.__wx_channels_store__.profile) {
							const profile = window.__wx_channels_store__.profile;
							// 优先使用thumbUrl，然后是fullThumbUrl，最后才是coverUrl
							const coverUrl = profile.thumbUrl || profile.fullThumbUrl || profile.coverUrl;
							
							if (!coverUrl) {
								alert("未找到封面图片");
								return;
							}
							
							// 记录下载
							const recordProfile = {...profile};
							window.__wx_channels_record_download(recordProfile);
							
							// 创建一个隐藏的a标签来下载图片，避免使用saveAs可能导致的确认框问题
							const downloadLink = document.createElement('a');
							downloadLink.href = coverUrl;
							downloadLink.download = "cover_" + profile.id + ".jpg";
							downloadLink.target = "_blank";
							
							// 添加到文档中并模拟点击
							document.body.appendChild(downloadLink);
							downloadLink.click();
							
							// 清理DOM
							setTimeout(() => {
								document.body.removeChild(downloadLink);
							}, 100);
							
							// 备用方法：如果直接下载失败，尝试使用fetch和saveAs
							setTimeout(() => {
								if (typeof saveAs !== 'undefined') {
									fetch(coverUrl)
										.then(response => response.blob())
										.then(blob => {
											saveAs(blob, "cover_" + profile.id + ".jpg");
										})
										.catch(error => {
											console.error("下载封面失败:", error);
											alert("下载封面失败，请重试");
										});
								}
							}, 1000); // 延迟1秒执行备用方法
						} else {
							alert("未找到视频信息");
						}
					};
					</script>`

					// 添加捕获完整URL的JavaScript代码
					captureUrlScript := `<script>
					setTimeout(function() {
						// 获取完整的URL
						var fullUrl = window.location.href;
						// 发送到我们的API端点
						fetch("/__wx_channels_api/page_url", {
							method: "POST",
							headers: {
								"Content-Type": "application/json"
							},
							body: JSON.stringify({
								url: fullUrl
							})
						});
					}, 2000); // 延迟2秒执行，确保页面完全加载
					</script>`

					// 添加保存动态内容的JavaScript代码
					savePageContentScript := `<script>
					// 保存当前页面完整内容的函数
					window.__wx_channels_save_page_content = function() {
						try {
							// 获取当前完整的HTML内容
							var fullHtml = document.documentElement.outerHTML;
							var currentUrl = window.location.href;
							
							// 发送到保存API
							fetch("/__wx_channels_api/save_page_content", {
								method: "POST",
								headers: {
									"Content-Type": "application/json"
								},
								body: JSON.stringify({
									url: currentUrl,
									html: fullHtml,
									timestamp: new Date().getTime()
								})
							}).then(response => {
								if (response.ok) {
									console.log("页面内容已保存");
								}
							}).catch(error => {
								console.error("保存页面内容失败:", error);
							});
						} catch (error) {
							console.error("获取页面内容失败:", error);
						}
					};
					
					// 监听URL变化，自动保存页面内容
					let currentPageUrl = window.location.href;
					const checkUrlChange = () => {
						if (window.location.href !== currentPageUrl) {
							currentPageUrl = window.location.href;
							// URL变化后延迟保存，等待内容加载
							setTimeout(() => {
								window.__wx_channels_save_page_content();
							}, 3000);
						}
					};
					
					// 定期检查URL变化（适用于SPA）
					setInterval(checkUrlChange, 1000);
					
					// 监听历史记录变化
					window.addEventListener('popstate', () => {
						setTimeout(() => {
							window.__wx_channels_save_page_content();
						}, 3000);
					});
					
					// 在页面加载完成后也保存一次
					setTimeout(() => {
						window.__wx_channels_save_page_content();
					}, 5000);
					</script>`

					// 添加视频缓存完成通知脚本
					videoCacheNotificationScript := `<script>
					// 初始化视频缓存监控
					window.__wx_channels_video_cache_monitor = {
						isBuffering: false,
						lastBufferTime: 0,
						totalBufferSize: 0,
						videoSize: 0,
						completeThreshold: 0.98, // 认为98%缓冲完成时视频已缓存完成
						checkInterval: null,
						notificationShown: false, // 防止重复显示通知
						
						// 开始监控缓存
						startMonitoring: function(expectedSize) {
							console.log('=== 开始启动视频缓存监控 ===');
							
							// 检查播放器状态
							const vjsPlayer = document.querySelector('.video-js');
							const video = vjsPlayer ? vjsPlayer.querySelector('video') : document.querySelector('video');
							
							if (!video) {
								console.error('未找到视频元素，无法启动监控');
								return;
							}
							
							console.log('视频元素状态:');
							console.log('- readyState:', video.readyState);
							console.log('- duration:', video.duration);
							console.log('- buffered.length:', video.buffered ? video.buffered.length : 0);
							
							if (this.checkInterval) {
								clearInterval(this.checkInterval);
							}
							
							this.isBuffering = true;
							this.lastBufferTime = Date.now();
							this.totalBufferSize = 0;
							this.videoSize = expectedSize || 0;
							this.notificationShown = false; // 重置通知状态
							
							console.log('视频缓存监控已启动');
							console.log('- 视频大小:', (this.videoSize / (1024 * 1024)).toFixed(2) + 'MB');
							console.log('- 监控间隔: 2秒');
							
							// 定期检查缓冲状态 - 增加检查频率
							this.checkInterval = setInterval(() => this.checkBufferStatus(), 2000);
							
							// 添加可见的缓存状态指示器
							this.addStatusIndicator();
							
							// 监听视频播放完成事件
							this.setupVideoEndedListener();
							
							// 延迟开始监控，让播放器有时间初始化
							setTimeout(() =>{
								this.monitorNativeBuffering();
							}, 1000);
						},
						
						// 监控Video.js播放器和原生视频元素的缓冲状态
						monitorNativeBuffering: function() {
							const checkBufferedProgress = () => {
								// 优先检查Video.js播放器
								const vjsPlayer = document.querySelector('.video-js');
								let video = null;
								
								if (vjsPlayer) {
									// 从Video.js播放器中获取video元素
									video = vjsPlayer.querySelector('video');
									console.log('找到Video.js播放器，开始监控');
								} else {
									// 回退到查找普通video元素
									const videoElements = document.querySelectorAll('video');
									if (videoElements.length > 0) {
										video = videoElements[0];
										console.log('使用普通video元素监控');
									}
								}
								
								if (video) {
									// 获取预加载进度条数据
									if (video.buffered && video.buffered.length > 0 && video.duration) {
										// 获取最后缓冲时间范围的结束位置
										const bufferedEnd = video.buffered.end(video.buffered.length - 1);
										// 计算缓冲百分比
										const bufferedPercent = (bufferedEnd / video.duration) * 100;
										
										// 更新页面指示器
										const indicator = document.getElementById('video-cache-indicator');
										if (indicator) {
											indicator.innerHTML = '<div>视频缓存中: ' + bufferedPercent.toFixed(1) + '% (Video.js播放器)</div>';
											
											// 高亮显示接近完成的状态
											if (bufferedPercent >= 95) {
												indicator.style.backgroundColor = 'rgba(0,128,0,0.8)';
											}
										}
										
										// 检查Video.js播放器的就绪状态
										if (vjsPlayer && typeof vjsPlayer.readyState !== 'undefined') {
											console.log('Video.js播放器就绪状态:', vjsPlayer.readyState);
										}
										
										// 检查是否缓冲完成
										if (bufferedPercent >= 98) {
											console.log('根据Video.js播放器数据，视频已缓存完成 (' + bufferedPercent.toFixed(1) + '%)');
											this.showNotification();
											this.stopMonitoring();
											return true; // 缓存完成，停止监控
										}
										
										// 查找Video.js播放器的缓冲进度条元素
										const vjs_loadProgressElements = document.querySelectorAll('.vjs-load-progress');
										for (let i = 0; i < vjs_loadProgressElements.length; i++) {
											const loadProgress = vjs_loadProgressElements[i];
											if (loadProgress && loadProgress.style && loadProgress.style.width) {
												// 如果找到了缓冲进度条，记录其宽度值
												console.log('找到Video.js缓冲进度条，当前宽度:', loadProgress.style.width);
											}
										}
										
										// 查找Video.js播放器的播放进度条元素
										const vjs_playProgressElements = document.querySelectorAll('.vjs-play-progress');
										for (let i = 0; i < vjs_playProgressElements.length; i++) {
											const playProgress = vjs_playProgressElements[i];
											if (playProgress && playProgress.style && playProgress.style.width) {
												// 如果找到了播放进度条，记录其宽度值
												console.log('找到Video.js播放进度条，当前宽度:', playProgress.style.width);
											}
										}
										
										// 查找进度条持有者，获取更详细的进度信息
										const progressHolders = document.querySelectorAll('.vjs-progress-holder');
										for (let i = 0; i < progressHolders.length; i++) {
											const holder = progressHolders[i];
											if (holder && holder.getAttribute) {
												const valueNow = holder.getAttribute('aria-valuenow');
												const valueMax = holder.getAttribute('aria-valuemax');
												if (valueNow && valueMax) {
													const progressPercent = (parseFloat(valueNow) / parseFloat(valueMax)) * 100;
													console.log('Video.js进度条ARIA数据:', progressPercent.toFixed(1) + '%');
													
													// 如果进度接近完成，也更新指示器
													if (progressPercent >= 95) {
														const indicator = document.getElementById('video-cache-indicator');
														if (indicator) {
															indicator.innerHTML = '<div>视频缓存中: ' + progressPercent.toFixed(1) + '% (进度条ARIA)</div>';
															indicator.style.backgroundColor = 'rgba(0,128,0,0.8)';
														}
													}
												}
											}
										}
									}
								}
								return false; // 继续监控
							};
							
							// 立即检查一次
							if (!checkBufferedProgress()) {
								// 每秒检查一次预加载进度
								const bufferCheckInterval = setInterval(() => {
									if (checkBufferedProgress() || !this.isBuffering) {
										clearInterval(bufferCheckInterval);
									}
								}, 1000);
							}
						},
						
						// 设置Video.js播放器和视频播放结束监听
						setupVideoEndedListener: function() {
							// 尝试查找Video.js播放器和视频元素
							setTimeout(() => {
								const vjsPlayer = document.querySelector('.video-js');
								let video = null;
								
								if (vjsPlayer) {
									// 从Video.js播放器中获取video元素
									video = vjsPlayer.querySelector('video');
									console.log('为Video.js播放器设置事件监听');
									
									// 尝试监听Video.js特有的事件
									if (vjsPlayer.addEventListener) {
										vjsPlayer.addEventListener('ended', () => {
											console.log('Video.js播放器播放结束，标记为缓存完成');
											this.showNotification();
											this.stopMonitoring();
										});
										
										vjsPlayer.addEventListener('loadeddata', () => {
											console.log('Video.js播放器数据加载完成');
										});
										
										// 移除canplaythrough事件，因为它会过早触发
										// vjsPlayer.addEventListener('canplaythrough', () => {
										// 	console.log('Video.js播放器可以流畅播放，标记为缓存完成');
										// 	this.showNotification();
										// 	this.stopMonitoring();
										// });
									}
								} else {
									// 回退到查找普通video元素
									const videoElements = document.querySelectorAll('video');
									if (videoElements.length > 0) {
										video = videoElements[0];
										console.log('为普通video元素设置事件监听');
									}
								}
								
								if (video) {
									// 监听视频播放结束事件
									video.addEventListener('ended', () => {
										console.log('视频播放已结束，标记为缓存完成');
										this.showNotification();
										this.stopMonitoring();
									});
									
									// 移除canplaythrough事件，因为它会过早触发
									// video.addEventListener('canplaythrough', () => {
									// 	console.log('视频数据足够播放完整视频，标记为缓存完成');
									// 	this.showNotification();
									// 	this.stopMonitoring();
									// });
									
									// 如果视频已在播放中，添加定期检查播放状态
									if (!video.paused) {
										const playStateInterval = setInterval(() => {
											// 如果视频已经播放完或接近结束（剩余小于2秒）
											if (video.ended || (video.duration && video.currentTime > 0 && video.duration - video.currentTime < 2)) {
												console.log('视频接近或已播放完成，标记为缓存完成');
												this.showNotification();
												this.stopMonitoring();
												clearInterval(playStateInterval);
											}
										}, 1000);
									}
								}
							}, 3000); // 延迟3秒再查找视频元素，确保Video.js播放器完全初始化
						},
						
						// 添加缓冲状态指示器
						addStatusIndicator: function() {
							console.log('正在创建缓存状态指示器...');
							
							// 移除现有指示器
							const existingIndicator = document.getElementById('video-cache-indicator');
							if (existingIndicator) {
								console.log('移除现有指示器');
								existingIndicator.remove();
							}
							
							// 创建新指示器
							const indicator = document.createElement('div');
							indicator.id = 'video-cache-indicator';
							indicator.style.cssText = "position:fixed;bottom:20px;left:20px;background-color:rgba(0,0,0,0.8);color:white;padding:10px 15px;border-radius:6px;z-index:99999;font-size:14px;font-family:Arial,sans-serif;border:2px solid rgba(255,255,255,0.3);";
							indicator.innerHTML = '<div>🔄 视频缓存中: 0%</div>';
							document.body.appendChild(indicator);
							
							console.log('缓存状态指示器已创建并添加到页面');
							
							// 每秒更新进度
							const updateInterval = setInterval(() => {
								if (!this.isBuffering) {
									clearInterval(updateInterval);
									indicator.remove();
									return;
								}
								
								let progress = 0;
								if (this.videoSize > 0) {
									progress = (this.totalBufferSize / this.videoSize) * 100;
								} else {
									// 优先使用Video.js播放器
									const vjsPlayer = document.querySelector('.video-js');
									let video = null;
									
									if (vjsPlayer) {
										video = vjsPlayer.querySelector('video');
									} else {
										const videoElements = document.querySelectorAll('video');
										if (videoElements.length > 0) {
											video = videoElements[0];
										}
									}
									
									if (video && video.duration && video.buffered.length > 0) {
										const bufferedEnd = video.buffered.end(video.buffered.length - 1);
										progress = (bufferedEnd / video.duration) * 100;
									}
								}
								
								// 更新指示器
								if (progress > 0) {
									indicator.innerHTML = '<div>🔄 视频缓存中: ' + progress.toFixed(1) + '%</div>';
								} else {
									indicator.innerHTML = '<div>⏳ 等待视频数据...</div>';
								}
								
								// 根据进度改变样式
								if (progress >= 95) {
									indicator.style.backgroundColor = 'rgba(0,128,0,0.8)';
									indicator.innerHTML = '<div>✅ 视频缓存接近完成: ' + progress.toFixed(1) + '%</div>';
								} else if (progress >= 50) {
									indicator.style.backgroundColor = 'rgba(255,165,0,0.8)';
								} else {
									indicator.style.backgroundColor = 'rgba(0,0,0,0.8)';
								}
								
								// 输出调试信息
								if (progress > 0) {
									console.log('缓存进度更新:', progress.toFixed(1) + '%');
								}
							}, 1000);
						},
						
						// 添加缓冲块
						addBuffer: function(buffer) {
							if (!this.isBuffering) return;
							
							// 更新最后缓冲时间
							this.lastBufferTime = Date.now();
							
							// 累计缓冲大小
							if (buffer && buffer.byteLength) {
								this.totalBufferSize += buffer.byteLength;
								
								// 输出调试信息到控制台
								if (this.videoSize > 0) {
									const percent = ((this.totalBufferSize / this.videoSize) * 100).toFixed(1);
									console.log('视频缓存进度: ' + percent + '% (' + (this.totalBufferSize / (1024 * 1024)).toFixed(2) + 'MB/' + (this.videoSize / (1024 * 1024)).toFixed(2) + 'MB)');
								}
							}
							
							// 检查是否接近完成
							this.checkCompletion();
						},
						
						// 检查Video.js播放器和原生视频的缓冲状态
						checkBufferStatus: function() {
							if (!this.isBuffering) return;
							
							// 优先检查Video.js播放器
							const vjsPlayer = document.querySelector('.video-js');
							let video = null;
							
							if (vjsPlayer) {
								// 从Video.js播放器中获取video元素
								video = vjsPlayer.querySelector('video');
								
								// 检查Video.js播放器特有的状态
								if (vjsPlayer.classList.contains('vjs-has-started')) {
									console.log('Video.js播放器已开始播放');
								}
								
								if (vjsPlayer.classList.contains('vjs-waiting')) {
									console.log('Video.js播放器正在等待数据');
								}
								
								if (vjsPlayer.classList.contains('vjs-ended')) {
									console.log('Video.js播放器播放结束，标记为缓存完成');
									this.checkCompletion(true);
									return;
								}
							} else {
								// 回退到查找普通video元素
								const videoElements = document.querySelectorAll('video');
								if (videoElements.length > 0) {
									video = videoElements[0];
								}
							}
							
							if (video) {
								if (video.buffered && video.buffered.length > 0 && video.duration) {
									// 获取最后缓冲时间范围的结束位置
									const bufferedEnd = video.buffered.end(video.buffered.length - 1);
									// 计算缓冲百分比
									const bufferedPercent = (bufferedEnd / video.duration) * 100;
									
									// 如果预加载接近完成，触发完成检测
									if (bufferedPercent >= 95) {
										console.log('检测到视频预加载接近完成 (' + bufferedPercent.toFixed(1) + '%)');
										this.checkCompletion(true);
									}
								}
								
								// 只在readyState为4且缓冲百分比较高时才认为完成
								if (video.readyState >= 4 && video.buffered && video.buffered.length > 0 && video.duration) {
									const bufferedEnd = video.buffered.end(video.buffered.length - 1);
									const bufferedPercent = (bufferedEnd / video.duration) * 100;
									if (bufferedPercent >= 98) {
										console.log('视频readyState为4且缓冲98%以上，标记为缓存完成');
										this.checkCompletion(true);
									}
								}
							}
							
							// 如果超过10秒没有新的缓冲数据且已经缓冲了部分数据，可能表示视频已暂停或缓冲完成
							const timeSinceLastBuffer = Date.now() - this.lastBufferTime;
							if (timeSinceLastBuffer > 10000 && this.totalBufferSize > 0) {
								this.checkCompletion(true);
							}
						},
						
						// 检查是否完成
						checkCompletion: function(forcedCheck) {
							if (!this.isBuffering) return;
							
							let isComplete = false;
							
							// 优先检查Video.js播放器是否已播放完成
							const vjsPlayer = document.querySelector('.video-js');
							let video = null;
							
							if (vjsPlayer) {
								video = vjsPlayer.querySelector('video');
								
								// 检查Video.js播放器的完成状态
								if (vjsPlayer.classList.contains('vjs-ended')) {
									console.log('Video.js播放器已播放完毕，认为缓存完成');
									isComplete = true;
								}
							} else {
								// 回退到查找普通video元素
								const videoElements = document.querySelectorAll('video');
								if (videoElements.length > 0) {
									video = videoElements[0];
								}
							}
							
							if (video && !isComplete) {
								// 如果视频已经播放完毕或接近结束，直接认为完成
								if (video.ended || (video.duration && video.currentTime > 0 && video.duration - video.currentTime < 2)) {
									console.log('视频已播放完毕或接近结束，认为缓存完成');
									isComplete = true;
								}
								
								// 只在readyState为4且缓冲百分比较高时才认为完成
								if (video.readyState >= 4 && video.buffered && video.buffered.length > 0 && video.duration) {
									const bufferedEnd = video.buffered.end(video.buffered.length - 1);
									const bufferedPercent = (bufferedEnd / video.duration) * 100;
									if (bufferedPercent >= 98) {
										console.log('视频readyState为4且缓冲98%以上，认为缓存完成');
										isComplete = true;
									}
								}
							}
							
							// 如果未通过播放状态判断完成，再检查缓冲大小
							if (!isComplete) {
								// 如果知道视频大小，则根据百分比判断
								if (this.videoSize > 0) {
									const ratio = this.totalBufferSize / this.videoSize;
									// 对短视频降低阈值要求
									const threshold = this.videoSize < 5 * 1024 * 1024 ? 0.9 : this.completeThreshold; // 5MB以下视频降低阈值到90%
									isComplete = ratio >= threshold;
								} 
								// 强制检查：如果长时间没有新数据且视频元素可以播放到最后，也认为已完成
								else if (forcedCheck && video) {
									if (video.readyState >= 3 && video.buffered.length > 0) {
										const bufferedEnd = video.buffered.end(video.buffered.length - 1);
										const duration = video.duration;
										isComplete = duration > 0 && (bufferedEnd / duration) >= 0.95; // 降低阈值到95%
										
										if (isComplete) {
											console.log('强制检查：根据缓冲数据判断视频缓存完成');
										}
									}
								}
							}
							
							// 如果完成，显示通知
							if (isComplete) {
								this.showNotification();
								this.stopMonitoring();
							}
						},
						
						// 显示通知
						showNotification: function() {
							// 防止重复显示通知
							if (this.notificationShown) {
								console.log('通知已经显示过，跳过重复显示');
								return;
							}
							
							console.log('显示缓存完成通知');
							this.notificationShown = true;
							
							// 移除进度指示器
							const indicator = document.getElementById('video-cache-indicator');
							if (indicator) {
								indicator.remove();
							}
							
							// 创建桌面通知
							if ("Notification" in window && Notification.permission === "granted") {
								new Notification("视频缓存完成", {
									body: "视频已缓存完成，可以进行下载操作",
									icon: window.__wx_channels_store__?.profile?.coverUrl
								});
							}
							
							// 在页面上显示通知
							const notification = document.createElement('div');
							notification.style.cssText = "position:fixed;bottom:20px;right:20px;background-color:rgba(0,128,0,0.9);color:white;padding:15px 25px;border-radius:8px;z-index:99999;animation:fadeInOut 12s forwards;box-shadow:0 4px 12px rgba(0,0,0,0.3);font-size:16px;font-weight:bold;";
							notification.innerHTML = '<div style="display:flex;align-items:center;"><span style="font-size:24px;margin-right:12px;">🎉</span> <span>视频缓存完成，可以下载了！</span></div>';
							
							// 添加动画样式 - 延长显示时间到12秒
							const style = document.createElement('style');
							style.textContent = '@keyframes fadeInOut {0% {opacity:0;transform:translateY(20px);} 8% {opacity:1;transform:translateY(0);} 85% {opacity:1;} 100% {opacity:0;}}';
							document.head.appendChild(style);
							
							document.body.appendChild(notification);
							
							// 12秒后移除通知
							setTimeout(() => {
								notification.remove();
							}, 12000);
							
							// 发送通知事件
							fetch("/__wx_channels_api/tip", {
								method: "POST",
								headers: {
									"Content-Type": "application/json"
								},
								body: JSON.stringify({
									msg: "视频缓存完成，可以下载了！"
								})
							});
							
							console.log("视频缓存完成通知已显示");
						},
						
						// 停止监控
						stopMonitoring: function() {
							console.log('停止视频缓存监控');
							if (this.checkInterval) {
								clearInterval(this.checkInterval);
								this.checkInterval = null;
							}
							this.isBuffering = false;
							// 注意：不重置notificationShown，保持通知状态直到下次startMonitoring
						}
					};
					
					// 请求通知权限
					if ("Notification" in window && Notification.permission !== "granted" && Notification.permission !== "denied") {
						// 用户操作后再请求权限
						document.addEventListener('click', function requestPermission() {
							Notification.requestPermission();
							document.removeEventListener('click', requestPermission);
						}, {once: true});
					}
					</script>`

					html = strings.Replace(html, "<head>", "<head>\n"+script+preloadScript+downloadTrackerScript+captureUrlScript+savePageContentScript+videoCacheNotificationScript+script2, 1)
					fmt.Println("\n页面已成功加载！")
					fmt.Println("已添加视频缓存监控和提醒功能")
					Conn.Response.Body = io.NopCloser(bytes.NewBuffer([]byte(html)))
					return
				}
				Conn.Response.Body = io.NopCloser(bytes.NewBuffer([]byte(html)))
				return
			}
			if content_type == "application/javascript" {
				content := string(Body)

				// 调试：输出所有JS文件路径（调试用）
				// if strings.Contains(path, "vuexStores") {
				// 	fmt.Println("\n📦 拦截到JS文件:", path)
				// 	fmt.Println("📏 文件大小:", len(content), "bytes")
				// }

				dep_reg := regexp.MustCompile(`"js/([^"]{1,})\.js"`)
				from_reg := regexp.MustCompile(`from {0,1}"([^"]{1,})\.js"`)
				lazy_import_reg := regexp.MustCompile(`import\("([^"]{1,})\.js"\)`)
				import_reg := regexp.MustCompile(`import {0,1}"([^"]{1,})\.js"`)
				content = from_reg.ReplaceAllString(content, `from"$1.js`+v+`"`)
				content = dep_reg.ReplaceAllString(content, `"js/$1.js`+v+`"`)
				content = lazy_import_reg.ReplaceAllString(content, `import("$1.js`+v+`")`)
				content = import_reg.ReplaceAllString(content, `import"$1.js`+v+`"`)
				Conn.Response.Header.Set("__debug", "replace_script")

				if util.Includes(path, "/t/wx_fed/finder/web/web-finder/res/js/index.publish") {
					regexp1 := regexp.MustCompile(`this.sourceBuffer.appendBuffer\(h\),`)
					replaceStr1 := `(() => {
if (window.__wx_channels_store__) {
window.__wx_channels_store__.buffers.push(h);
// 添加缓存监控
if (window.__wx_channels_video_cache_monitor) {
    window.__wx_channels_video_cache_monitor.addBuffer(h);
}
}
})(),this.sourceBuffer.appendBuffer(h),`
					if regexp1.MatchString(content) {
						fmt.Println("\n视频播放已成功加载！")
						fmt.Println("视频缓冲将被监控，完成时会有提醒")
					}
					content = regexp1.ReplaceAllString(content, replaceStr1)
					regexp2 := regexp.MustCompile(`if\(f.cmd===re.MAIN_THREAD_CMD.AUTO_CUT`)
					replaceStr2 := `if(f.cmd==="CUT"){
	if (window.__wx_channels_store__) {
	console.log("CUT", f, __wx_channels_store__.profile.key);
	window.__wx_channels_store__.keys[__wx_channels_store__.profile.key]=f.decryptor_array;
	}
}
if(f.cmd===re.MAIN_THREAD_CMD.AUTO_CUT`
					content = regexp2.ReplaceAllString(content, replaceStr2)
					Conn.Response.Body = io.NopCloser(bytes.NewBuffer([]byte(content)))
					return
				}
				if util.Includes(path, "/t/wx_fed/finder/web/web-finder/res/js/virtual_svg-icons-register") {
					regexp1 := regexp.MustCompile(`async finderGetCommentDetail\((\w+)\)\{return(.*?)\}async`)
					replaceStr1 := `async finderGetCommentDetail($1) {
					var feedResult = await$2;
					var data_object = feedResult.data.object;
					if (!data_object.objectDesc) {
						return feedResult;
					}
					
					// 不再输出调试信息
					// console.log("原始视频数据对象:", data_object);
					
					var media = data_object.objectDesc.media[0];
					var profile = media.mediaType !== 4 ? {
						type: "picture",
						id: data_object.id,
						title: data_object.objectDesc.description,
						files: data_object.objectDesc.media,
						spec: [],
						contact: data_object.contact
					} : {
						type: "media",
						duration: media.spec[0].durationMs,
						spec: media.spec.map(s => ({
							...s,
							width: s.width || s.videoWidth,
							height: s.height || s.videoHeight
						})),
						title: data_object.objectDesc.description,
						coverUrl: media.thumbUrl || media.coverUrl, // 使用thumbUrl作为主要封面，如果不存在则使用coverUrl
						thumbUrl: media.thumbUrl, // 添加thumbUrl字段
						fullThumbUrl: media.fullThumbUrl, // 添加fullThumbUrl字段
						url: media.url+media.urlToken,
						size: media.fileSize,
						key: media.decodeKey,
						id: data_object.id,
						nonce_id: data_object.objectNonceId,
						nickname: data_object.nickname,
						createtime: data_object.createtime,
						fileFormat: media.spec.map(o => o.fileFormat),
						contact: data_object.contact,
						// 互动数据
						readCount: data_object.readCount || 0,
						likeCount: data_object.likeCount || 0,
						commentCount: data_object.commentCount || 0,
						favCount: data_object.favCount || 0,
						forwardCount: data_object.forwardCount || 0,
						// IP区域信息
						ipRegionInfo: data_object.ipRegionInfo || {}
					};
					
					// 如果存在对象扩展信息，添加到profile
					if (data_object.objectExtend && data_object.objectExtend.monotonicData) {
						const monotonicData = data_object.objectExtend.monotonicData;
						if (monotonicData.countInfo) {
							profile.readCount = monotonicData.countInfo.readCount || profile.readCount;
							profile.likeCount = monotonicData.countInfo.likeCount || profile.likeCount;
							profile.commentCount = monotonicData.countInfo.commentCount || profile.commentCount;
							profile.favCount = monotonicData.countInfo.favCount || profile.favCount;
							profile.forwardCount = monotonicData.countInfo.forwardCount || profile.forwardCount;
						}
					}
					
					fetch("/__wx_channels_api/profile", {
						method: "POST",
						headers: {
							"Content-Type": "application/json"
						},
						body: JSON.stringify(profile)
					});
					if (window.__wx_channels_store__) {
					__wx_channels_store__.profile = profile;
					window.__wx_channels_store__.profiles.push(profile);
					
					// 启动视频缓存监控
					if (window.__wx_channels_video_cache_monitor && profile.type === "media" && profile.size) {
						console.log("正在初始化视频缓存监控系统...");
						console.log("视频大小:", (profile.size / (1024 * 1024)).toFixed(2) + 'MB');
						console.log("视频标题:", profile.title);
						setTimeout(() => {
							// 确保Video.js播放器已经加载
							const vjsPlayer = document.querySelector('.video-js');
							const video = vjsPlayer ? vjsPlayer.querySelector('video') : document.querySelector('video');
							
							if (video) {
								console.log("找到视频元素，启动缓存监控");
								console.log("视频readyState:", video.readyState);
								console.log("视频duration:", video.duration);
								window.__wx_channels_video_cache_monitor.startMonitoring(profile.size);
							} else {
								console.log("未找到视频元素，延迟重试");
								setTimeout(() => {
									window.__wx_channels_video_cache_monitor.startMonitoring(profile.size);
								}, 2000); // 再延迟2秒重试
							}
						}, 3000); // 延迟3秒启动，确保Video.js播放器完全初始化
					}
					}
					return feedResult;
				}async`
					if regexp1.MatchString(content) {
						fmt.Println("\n视频详情数据已获取成功！")
					}
					content = regexp1.ReplaceAllString(content, replaceStr1)
					regex2 := regexp.MustCompile(`i.default={dialog`)
					replaceStr2 := `i.default=window.window.__wx_channels_tip__={dialog`
					content = regex2.ReplaceAllString(content, replaceStr2)
					regex5 := regexp.MustCompile(`this.updateDetail\(o\)`)
					replaceStr5 := `(() => {
					if (Object.keys(o).length===0){
					return;
					}
					
					// 不再输出调试信息
					// console.log("updateDetail原始数据:", o);
					
					var data_object = o;
					var media = data_object.objectDesc.media[0];
					var profile = media.mediaType !== 4 ? {
						type: "picture",
						id: data_object.id,
						title: data_object.objectDesc.description,
						files: data_object.objectDesc.media,
						spec: [],
						contact: data_object.contact
					} : {
						type: "media",
						duration: media.spec[0].durationMs,
						spec: media.spec.map(s => ({
							...s,
							width: s.width || s.videoWidth,
							height: s.height || s.videoHeight
						})),
						title: data_object.objectDesc.description,
						coverUrl: media.thumbUrl || media.coverUrl, // 使用thumbUrl作为主要封面，如果不存在则使用coverUrl
						thumbUrl: media.thumbUrl, // 添加thumbUrl字段
						fullThumbUrl: media.fullThumbUrl, // 添加fullThumbUrl字段
						url: media.url+media.urlToken,
						size: media.fileSize,
						key: media.decodeKey,
						id: data_object.id,
						nonce_id: data_object.objectNonceId,
						nickname: data_object.nickname,
						createtime: data_object.createtime,
						fileFormat: media.spec.map(o => o.fileFormat),
						contact: data_object.contact,
						// 互动数据
						readCount: data_object.readCount || 0,
						likeCount: data_object.likeCount || 0,
						commentCount: data_object.commentCount || 0,
						favCount: data_object.favCount || 0,
						forwardCount: data_object.forwardCount || 0,
						// IP区域信息
						ipRegionInfo: data_object.ipRegionInfo || {}
					};
					
					// 如果存在对象扩展信息，添加到profile
					if (data_object.objectExtend && data_object.objectExtend.monotonicData) {
						const monotonicData = data_object.objectExtend.monotonicData;
						if (monotonicData.countInfo) {
							profile.readCount = monotonicData.countInfo.readCount || profile.readCount;
							profile.likeCount = monotonicData.countInfo.likeCount || profile.likeCount;
							profile.commentCount = monotonicData.countInfo.commentCount || profile.commentCount;
							profile.favCount = monotonicData.countInfo.favCount || profile.favCount;
							profile.forwardCount = monotonicData.countInfo.forwardCount || profile.forwardCount;
						}
					}
					
					if (window.__wx_channels_store__) {
window.__wx_channels_store__.profiles.push(profile);
					}
					})(),this.updateDetail(o)`
					content = regex5.ReplaceAllString(content, replaceStr5)
					Conn.Response.Body = io.NopCloser(bytes.NewBuffer([]byte(content)))
					return
				}
				if util.Includes(path, "/t/wx_fed/finder/web/web-finder/res/js/FeedDetail.publish") {
					regex := regexp.MustCompile(`,"投诉"\)]`)
					replaceStr := `,"投诉"),...(() => {
					if (window.__wx_channels_store__ && window.__wx_channels_store__.profile) {
						return window.__wx_channels_store__.profile.spec.map((sp) => {
							return f("div",{class:"context-item",role:"button",onClick:() => __wx_channels_handle_click_download__(sp)},__wx_format_quality_option(sp));
						});
					}
					})(),f("div",{class:"context-item",role:"button",onClick:()=>__wx_channels_handle_click_download__()},"原始视频"),f("div",{class:"context-item",role:"button",onClick:__wx_channels_download_cur__},"当前视频"),f("div",{class:"context-item",role:"button",onClick:()=>__wx_channels_handle_download_cover()},"下载封面")]`
					content = regex.ReplaceAllString(content, replaceStr)
					Conn.Response.Body = io.NopCloser(bytes.NewBuffer([]byte(content)))
					return
				}
				if util.Includes(path, "worker_release") {
					regex := regexp.MustCompile(`fmp4Index:p.fmp4Index`)
					replaceStr := `decryptor_array:p.decryptor_array,fmp4Index:p.fmp4Index`
					content = regex.ReplaceAllString(content, replaceStr)
					Conn.Response.Body = io.NopCloser(bytes.NewBuffer([]byte(content)))
					return
				}
				// Home页面视频数据采集逻辑 - 拦截vuexStores.publish中的视频信息流
				if util.Includes(path, "vuexStores.publish") {
					// 保存原始JS文件到本地进行分析（调试用）
					// debugDir := filepath.Join("downloads", "js_debug")
					// os.MkdirAll(debugDir, 0755)
					// timestamp := time.Now().Format("20060102_150405")
					// debugFile := filepath.Join(debugDir, fmt.Sprintf("vuexStores_%s.js", timestamp))
					// err := os.WriteFile(debugFile, []byte(content), 0644)
					// if err == nil {
					// 	fmt.Println("💾 已保存原始JS文件到:", debugFile)
					// }

					// 策略1：拦截 goToNextFlowFeed (下一个视频)
					callNextRegex := regexp.MustCompile(`(\w)\.goToNextFlowFeed\(\{goBackWhenEnd:[^,]+,eleInfo:\{[^}]+\}[^)]*\}\)`)
					// 策略2：拦截 goToPrevFlowFeed (上一个视频)
					callPrevRegex := regexp.MustCompile(`(\w)\.goToPrevFlowFeed\(\{eleInfo:\{[^}]+\}\}\)`)

					// 数据采集代码（通用，包含互动数据）
					captureCode := `setTimeout(function(){try{var __tab=Ue.value;if(__tab&&__tab.currentFeed){var __feed=__tab.currentFeed;if(__feed.objectDesc){var __media=__feed.objectDesc.media[0];var __profile={type:"media",duration:__media.spec[0].durationMs,spec:__media.spec.map(function(s){return{width:s.width||s.videoWidth,height:s.height||s.videoHeight,bitrate:s.bitrate,fileFormat:s.fileFormat}}),title:__feed.objectDesc.description,coverUrl:__media.thumbUrl,url:__media.url+__media.urlToken,size:__media.fileSize,key:__media.decodeKey,id:__feed.id,nonce_id:__feed.objectNonceId,nickname:__feed.nickname,createtime:__feed.createtime,fileFormat:__media.spec.map(function(o){return o.fileFormat}),contact:__feed.contact,readCount:__feed.readCount,likeCount:__feed.likeCount,commentCount:__feed.commentCount,favCount:__feed.favCount,forwardCount:__feed.forwardCount,ipRegionInfo:__feed.ipRegionInfo};fetch("/__wx_channels_api/profile",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(__profile)});window.__wx_channels_store__=window.__wx_channels_store__||{profile:null,buffers:[],keys:{}};window.__wx_channels_store__.profile=__profile;console.log("[Home页面] 视频数据采集成功:",__profile.title)}}}catch(__e){console.error("[Home] 采集失败:",__e)}},500)`

					// 替换 goToNextFlowFeed
					if callNextRegex.MatchString(content) {
						replaceNext := `$1.goToNextFlowFeed({goBackWhenEnd:f.goBackWhenEnd,eleInfo:{type:f.source,tagId:Ct.value},ignoreCoolDown:f.ignoreCoolDown});` + captureCode
						content = callNextRegex.ReplaceAllString(content, replaceNext)
					}

					// 替换 goToPrevFlowFeed
					if callPrevRegex.MatchString(content) {
						replacePrev := `$1.goToPrevFlowFeed({eleInfo:{type:f.source,tagId:Ct.value}});` + captureCode
						content = callPrevRegex.ReplaceAllString(content, replacePrev)
					}

					// 保存修改后的JS文件（调试用）
					// modifiedFile := filepath.Join(debugDir, fmt.Sprintf("vuexStores_modified_%s.js", timestamp))
					// err = os.WriteFile(modifiedFile, []byte(content), 0644)
					// if err == nil {
					// 	fmt.Println("💾 已保存修改后的JS文件到:", modifiedFile)
					// }

					// 禁用浏览器缓存，确保每次都能拦截到最新的代码
					Conn.Response.Header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
					Conn.Response.Header.Set("Pragma", "no-cache")
					Conn.Response.Header.Set("Expires", "0")

					Conn.Response.Body = io.NopCloser(bytes.NewBuffer([]byte(content)))
					return
				}
				Conn.Response.Body = io.NopCloser(bytes.NewBuffer([]byte(content)))
				return
			}
			Conn.Response.Body = io.NopCloser(bytes.NewBuffer(Body))
		}

	}
	if Conn.Type == public.HttpRequestFail {
		//请求错误
		// Body := []byte("Hello Sunny Response")
		// Conn.Response = &http.Response{
		// 	Body: io.NopCloser(bytes.NewBuffer(Body)),
		// }
	}

	// 在HttpCallback函数中添加处理URL的端点
	if path == "/__wx_channels_api/page_url" {
		var urlData struct {
			URL string `json:"url"`
		}
		body, _ := io.ReadAll(Conn.Request.Body)
		_ = Conn.Request.Body.Close()
		err := json.Unmarshal(body, &urlData)
		if err != nil {
			fmt.Println(err.Error())
		} else {
			// 保存URL而不是立即显示
			currentPageURL = urlData.URL

			// 显示在原始链接后的新形式
			printSeparator()
			color.Blue("📋 页面完整链接")
			printSeparator()
			printLabelValue("🔗", "分享链接", currentPageURL, color.New(color.FgGreen))
			printSeparator()
			fmt.Println("\n\n")
		}
		headers := http.Header{}
		headers.Set("Content-Type", "application/json")
		headers.Set("__debug", "fake_resp")
		Conn.StopRequest(200, "{}", headers)
		return
	}

	// 新增：保存页面完整内容的API端点
	if path == "/__wx_channels_api/save_page_content" {
		var contentData struct {
			URL       string `json:"url"`
			HTML      string `json:"html"`
			Timestamp int64  `json:"timestamp"`
		}
		body, _ := io.ReadAll(Conn.Request.Body)
		_ = Conn.Request.Body.Close()
		err := json.Unmarshal(body, &contentData)
		if err != nil {
			fmt.Printf("解析页面内容数据失败: %v\n", err)
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

	// 新增：记录下载信息的API端点
	if path == "/__wx_channels_api/record_download" {
		var data map[string]interface{}
		body, _ := io.ReadAll(Conn.Request.Body)
		_ = Conn.Request.Body.Close()

		var err error
		err = json.Unmarshal(body, &data)
		if err != nil {
			fmt.Println("记录下载信息错误:", err.Error())
		} else {
			// 创建下载记录
			record := VideoDownloadRecord{
				ID:         fmt.Sprintf("%v", data["id"]),
				Title:      fmt.Sprintf("%v", data["title"]),
				Author:     "", // 将在后面从contact中获取
				URL:        fmt.Sprintf("%v", data["url"]),
				PageURL:    currentPageURL,
				DownloadAt: time.Now(),
			}

			// 从正确的位置获取作者昵称
			// 优先从顶层获取（Feed页）
			if nickname, ok := data["nickname"].(string); ok && nickname != "" {
				record.Author = nickname
			} else {
				// 从 contact.nickname 获取（Home页）
				if contact, ok := data["contact"].(map[string]interface{}); ok {
					if nickname, ok := contact["nickname"].(string); ok {
						record.Author = nickname
					}
				}
			}

			// 添加可选字段
			if size, ok := data["size"].(float64); ok {
				record.FileSize = fmt.Sprintf("%.2f MB", size/(1024*1024))
			}
			if duration, ok := data["duration"].(float64); ok {
				record.Duration = formatDuration(duration)
			}

			// 添加互动数据
			if readCount, ok := data["readCount"].(float64); ok {
				record.PlayCount = formatNumber(readCount)
			}
			if likeCount, ok := data["likeCount"].(float64); ok {
				record.LikeCount = formatNumber(likeCount)
			}
			if commentCount, ok := data["commentCount"].(float64); ok {
				record.CommentCount = formatNumber(commentCount)
			}
			if favCount, ok := data["favCount"].(float64); ok {
				record.FavCount = formatNumber(favCount)
			}
			if forwardCount, ok := data["forwardCount"].(float64); ok {
				record.ForwardCount = formatNumber(forwardCount)
			}

			// 添加创建时间
			if createtime, ok := data["createtime"].(float64); ok {
				// 转换Unix时间戳为可读格式
				t := time.Unix(int64(createtime), 0)
				record.CreateTime = t.Format("2006-01-02 15:04:05")
			}

			// 添加视频号分类和公众号名称
			if contact, ok := data["contact"].(map[string]interface{}); ok {
				if authInfo, ok := contact["authInfo"].(map[string]interface{}); ok {
					if authProfession, ok := authInfo["authProfession"].(string); ok {
						record.AuthorType = authProfession
					}
				}

				// 尝试获取公众号名称
				if bindInfo, ok := contact["bindInfo"].([]interface{}); ok && len(bindInfo) > 0 {
					for _, bind := range bindInfo {
						if bindMap, ok := bind.(map[string]interface{}); ok {
							if bizInfo, ok := bindMap["bizInfo"].(map[string]interface{}); ok {
								if info, ok := bizInfo["info"].([]interface{}); ok && len(info) > 0 {
									if infoMap, ok := info[0].(map[string]interface{}); ok {
										if bizNickname, ok := infoMap["bizNickname"].(string); ok {
											record.OfficialName = bizNickname
											break
										}
									}
								}
							}
						}
					}
				}
			}

			// 添加IP所在地
			if ipRegionInfo, ok := data["ipRegionInfo"].(map[string]interface{}); ok {
				if regionText, ok := ipRegionInfo["regionText"].(string); ok {
					record.IPRegion = regionText
				}
			}

			// 保存记录
			if err := addDownloadRecord(record); err != nil {
				fmt.Println("保存下载记录失败:", err.Error())
			} else {
				printSeparator()
				color.Green("✅ 下载记录已保存")
				printSeparator()
			}
		}

		headers := http.Header{}
		headers.Set("Content-Type", "application/json")
		headers.Set("__debug", "fake_resp")
		Conn.StopRequest(200, "{}", headers)
		return
	}
}

// formatNumber 格式化数字，将大数字格式化为更易读的形式
func formatNumber(num float64) string {
	if num >= 100000000 {
		return fmt.Sprintf("%.1f亿", num/100000000)
	} else if num >= 10000 {
		return fmt.Sprintf("%.1f万", num/10000)
	}
	return fmt.Sprintf("%.0f", num)
}

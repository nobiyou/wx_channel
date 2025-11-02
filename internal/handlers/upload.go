package handlers

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"wx_channel/internal/config"
	"wx_channel/internal/storage"
	"wx_channel/internal/utils"
	"github.com/qtgolang/SunnyNet/SunnyNet"
)

// UploadHandler 文件上传处理器
type UploadHandler struct {
	config     *config.Config
	csvManager *storage.CSVManager
}

// NewUploadHandler 创建上传处理器
func NewUploadHandler(cfg *config.Config, csvManager *storage.CSVManager) *UploadHandler {
	return &UploadHandler{
		config:     cfg,
		csvManager: csvManager,
	}
}

// HandleInitUpload 处理分片上传初始化请求
func (h *UploadHandler) HandleInitUpload(Conn *SunnyNet.HttpConn) bool {
	path := Conn.Request.URL.Path
	if path != "/__wx_channels_api/init_upload" {
		return false
	}

	// 计算基路径
	baseDir, err := utils.GetBaseDir()
	if err != nil {
		utils.HandleError(err, "获取基础目录")
		h.sendErrorResponse(Conn, err)
		return true
	}

	uploadsRoot := filepath.Join(baseDir, h.config.DownloadsDir, ".uploads")
	if err := utils.EnsureDir(uploadsRoot); err != nil {
		utils.HandleError(err, "创建上传目录")
	}

	// 生成 uploadId
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		utils.HandleError(err, "生成随机数")
		h.sendErrorResponse(Conn, err)
		return true
	}
	uploadId := fmt.Sprintf("%x", b)
	utils.Info("🔄 init_upload: 生成 uploadId = %s", uploadId)

	// 创建临时目录
	upDir := filepath.Join(uploadsRoot, uploadId)
	if err := os.MkdirAll(upDir, 0755); err != nil {
		utils.HandleError(err, "创建上传目录")
		h.sendErrorResponse(Conn, err)
		return true
	}

	// 使用 JSON 编码确保正确转义
	responseData := map[string]interface{}{
		"success":  true,
		"uploadId": uploadId,
	}
	responseBytes, err := json.Marshal(responseData)
	if err != nil {
		utils.HandleError(err, "生成响应JSON")
		h.sendErrorResponse(Conn, err)
		return true
	}

	utils.Info("✅ init_upload: 返回响应: %s", string(responseBytes))
	h.sendJSONResponse(Conn, 200, responseBytes)
	return true
}

// HandleUploadChunk 处理分片上传请求
func (h *UploadHandler) HandleUploadChunk(Conn *SunnyNet.HttpConn) bool {
	path := Conn.Request.URL.Path
	if path != "/__wx_channels_api/upload_chunk" {
		return false
	}

	// 解析multipart表单
	err := Conn.Request.ParseMultipartForm(h.config.MaxUploadSize)
	if err != nil {
		utils.HandleError(err, "解析multipart表单")
		h.sendErrorResponse(Conn, err)
		return true
	}

	uploadId := Conn.Request.FormValue("uploadId")
	indexStr := Conn.Request.FormValue("index")
	totalStr := Conn.Request.FormValue("total")

	if uploadId == "" || indexStr == "" || totalStr == "" {
		h.sendErrorResponse(Conn, fmt.Errorf("missing fields"))
		return true
	}

	index, err := strconv.Atoi(indexStr)
	if err != nil {
		utils.HandleError(err, "解析索引")
		h.sendErrorResponse(Conn, err)
		return true
	}

	total, err := strconv.Atoi(totalStr)
	if err != nil {
		utils.HandleError(err, "解析总数")
		h.sendErrorResponse(Conn, err)
		return true
	}

	utils.Info("📦 upload_chunk: 接收分片 %d/%d (uploadId: %s)", index+1, total, uploadId[:8])

	file, _, err := Conn.Request.FormFile("chunk")
	if err != nil {
		utils.HandleError(err, "获取分片文件")
		h.sendErrorResponse(Conn, err)
		return true
	}
	defer file.Close()

	baseDir, err := utils.GetBaseDir()
	if err != nil {
		utils.HandleError(err, "获取基础目录")
		h.sendErrorResponse(Conn, err)
		return true
	}

	uploadsRoot := filepath.Join(baseDir, h.config.DownloadsDir, ".uploads")
	upDir := filepath.Join(uploadsRoot, uploadId)

	if _, err := os.Stat(upDir); os.IsNotExist(err) {
		h.sendErrorResponse(Conn, fmt.Errorf("uploadId not found"))
		return true
	}

	partPath := filepath.Join(upDir, fmt.Sprintf("%06d.part", index))
	out, err := os.Create(partPath)
	if err != nil {
		utils.HandleError(err, "创建分片文件")
		h.sendErrorResponse(Conn, err)
		return true
	}
	defer out.Close()

	written, err := io.Copy(out, file)
	if err != nil {
		utils.HandleError(err, "写入分片数据")
		h.sendErrorResponse(Conn, err)
		return true
	}

	utils.Info("✅ upload_chunk: 分片 %d/%d 已保存 (%.2f MB)", index+1, total, float64(written)/(1024*1024))
	h.sendSuccessResponse(Conn)
	return true
}

// HandleCompleteUpload 处理分片上传完成请求
func (h *UploadHandler) HandleCompleteUpload(Conn *SunnyNet.HttpConn) bool {
	path := Conn.Request.URL.Path
	if path != "/__wx_channels_api/complete_upload" {
		return false
	}

	body, err := io.ReadAll(Conn.Request.Body)
	if err != nil {
		utils.HandleError(err, "读取complete_upload请求体")
		h.sendErrorResponse(Conn, err)
		return true
	}

	if err := Conn.Request.Body.Close(); err != nil {
		utils.HandleError(err, "关闭请求体")
	}

	var req struct {
		UploadId   string `json:"uploadId"`
		Total      int    `json:"total"`
		Filename   string `json:"filename"`
		AuthorName string `json:"authorName"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		utils.HandleError(err, "解析complete_upload JSON")
		h.sendErrorResponse(Conn, err)
		return true
	}

	if req.UploadId == "" || req.Total <= 0 || req.Filename == "" {
		h.sendErrorResponse(Conn, fmt.Errorf("missing fields"))
		return true
	}

	baseDir, err := utils.GetBaseDir()
	if err != nil {
		utils.HandleError(err, "获取基础目录")
		h.sendErrorResponse(Conn, err)
		return true
	}

	uploadsRoot := filepath.Join(baseDir, h.config.DownloadsDir, ".uploads")
	upDir := filepath.Join(uploadsRoot, req.UploadId)

	// 目标作者目录
	authorFolder := utils.CleanFolderName(req.AuthorName)
	downloadsDir := filepath.Join(baseDir, h.config.DownloadsDir)
	savePath := filepath.Join(downloadsDir, authorFolder)

	if err := utils.EnsureDir(savePath); err != nil {
		utils.HandleError(err, "创建作者目录")
		h.sendErrorResponse(Conn, err)
		return true
	}

	// 清理文件名
	cleanFilename := utils.CleanFilename(req.Filename)
	cleanFilename = utils.EnsureExtension(cleanFilename, ".mp4")

	// 冲突处理
	base := filepath.Base(cleanFilename)
	ext := filepath.Ext(cleanFilename)
	baseName := strings.TrimSuffix(base, ext)
	finalPath := filepath.Join(savePath, cleanFilename)
	if _, err := os.Stat(finalPath); err == nil {
		// 文件已存在，生成唯一文件名
		for i := 1; i < 1000; i++ {
			candidate := filepath.Join(savePath, fmt.Sprintf("%s(%d)%s", baseName, i, ext))
			if _, existsErr := os.Stat(candidate); os.IsNotExist(existsErr) {
				finalPath = candidate
				break
			}
		}
	}

	// 合并分片
	out, err := os.Create(finalPath)
	if err != nil {
		utils.HandleError(err, "创建目标文件")
		h.sendErrorResponse(Conn, err)
		return true
	}
	defer out.Close()

	var totalWritten int64
	for i := 0; i < req.Total; i++ {
		partPath := filepath.Join(upDir, fmt.Sprintf("%06d.part", i))
		in, err := os.Open(partPath)
		if err != nil {
			utils.HandleError(err, "打开分片文件")
			h.sendErrorResponse(Conn, err)
			return true
		}

		n, err := io.Copy(out, in)
		in.Close()
		if err != nil {
			utils.HandleError(err, "合并分片数据")
			h.sendErrorResponse(Conn, err)
			return true
		}
		totalWritten += n
	}

	// 清理临时目录
	os.RemoveAll(upDir)

	fileSize := float64(totalWritten) / (1024 * 1024)
	color.Green("✓ 分片视频已保存: %s (%.2f MB)", finalPath, fileSize)

	responseData := map[string]interface{}{
		"success": true,
		"path":    finalPath,
		"size":    fileSize,
	}
	responseBytes, err := json.Marshal(responseData)
	if err != nil {
		utils.HandleError(err, "生成响应JSON")
		h.sendErrorResponse(Conn, err)
		return true
	}

	utils.Info("✅ complete_upload: 返回响应: %s", string(responseBytes))
	h.sendJSONResponse(Conn, 200, responseBytes)
	return true
}

// HandleSaveVideo 处理直接保存视频文件请求
func (h *UploadHandler) HandleSaveVideo(Conn *SunnyNet.HttpConn) bool {
	path := Conn.Request.URL.Path
	if path != "/__wx_channels_api/save_video" {
		return false
	}

	utils.Info("🔄 save_video: 开始处理请求")

	// 解析multipart表单
	err := Conn.Request.ParseMultipartForm(h.config.MaxUploadSize)
	if err != nil {
		utils.HandleError(err, "解析表单数据")
		h.sendErrorResponse(Conn, err)
		return true
	}

	utils.Info("✅ save_video: 表单解析成功")

	file, header, err := Conn.Request.FormFile("video")
	if err != nil {
		utils.HandleError(err, "获取视频文件")
		h.sendErrorResponse(Conn, err)
		return true
	}
	defer file.Close()

	utils.Info("接收上传: %s, 报告大小: %d bytes", header.Filename, header.Size)

	filename := Conn.Request.FormValue("filename")
	authorName := Conn.Request.FormValue("authorName")
	isEncrypted := Conn.Request.FormValue("isEncrypted") == "true"

	// 创建作者文件夹路径
	authorFolder := utils.CleanFolderName(authorName)

	baseDir, err := utils.GetBaseDir()
	if err != nil {
		utils.HandleError(err, "获取基础目录")
		h.sendErrorResponse(Conn, err)
		return true
	}

	downloadsDir := filepath.Join(baseDir, h.config.DownloadsDir)
	savePath := filepath.Join(downloadsDir, authorFolder)

	utils.Info("保存目录: %s", savePath)
	if err := utils.EnsureDir(savePath); err != nil {
		utils.HandleError(err, "创建文件夹")
		h.sendErrorResponse(Conn, err)
		return true
	}

	// 清理文件名
	cleanFilename := utils.CleanFilename(filename)
	cleanFilename = utils.EnsureExtension(cleanFilename, ".mp4")

	// 生成唯一文件名
	filePath := filepath.Join(savePath, cleanFilename)
	if _, statErr := os.Stat(filePath); statErr == nil {
		base := strings.TrimSuffix(cleanFilename, filepath.Ext(cleanFilename))
		ext := filepath.Ext(cleanFilename)
		for i := 1; i < 1000; i++ {
			candidate := filepath.Join(savePath, fmt.Sprintf("%s(%d)%s", base, i, ext))
			if _, existsErr := os.Stat(candidate); os.IsNotExist(existsErr) {
				filePath = candidate
				break
			}
		}
	}

	// 保存文件
	out, err := os.Create(filePath)
	if err != nil {
		utils.HandleError(err, "创建目标文件")
		h.sendErrorResponse(Conn, err)
		return true
	}
	defer out.Close()

	// 流式拷贝
	if seeker, ok := file.(io.Seeker); ok {
		_, _ = seeker.Seek(0, io.SeekStart)
	}

	written, err := io.Copy(out, file)
	if err != nil {
		utils.HandleError(err, "写入视频数据")
		h.sendErrorResponse(Conn, err)
		return true
	}

	fileSize := float64(written) / (1024 * 1024)
	statusMsg := ""
	if isEncrypted {
		statusMsg = " [已解密]"
	}
	utils.Info("✓ 视频已保存: %s (%.2f MB)%s", filePath, fileSize, statusMsg)

	responseData := map[string]interface{}{
		"success": true,
		"path":    filePath,
		"size":    fileSize,
	}
	responseBytes, err := json.Marshal(responseData)
	if err != nil {
		utils.HandleError(err, "生成响应JSON")
		h.sendErrorResponse(Conn, err)
		return true
	}

	utils.Info("✅ save_video: 返回响应: %s", string(responseBytes))
	h.sendJSONResponse(Conn, 200, responseBytes)
	return true
}

// sendSuccessResponse 发送成功响应
func (h *UploadHandler) sendSuccessResponse(Conn *SunnyNet.HttpConn) {
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	headers.Set("Pragma", "no-cache")
	headers.Set("Expires", "0")
	Conn.StopRequest(200, `{"success":true}`, headers)
}

// sendJSONResponse 发送JSON响应
func (h *UploadHandler) sendJSONResponse(Conn *SunnyNet.HttpConn, statusCode int, body []byte) {
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	headers.Set("Pragma", "no-cache")
	headers.Set("Expires", "0")
	Conn.StopRequest(statusCode, string(body), headers)
}

// sendErrorResponse 发送错误响应
func (h *UploadHandler) sendErrorResponse(Conn *SunnyNet.HttpConn, err error) {
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	errorMsg := fmt.Sprintf(`{"success":false,"error":"%s"}`, err.Error())
	Conn.StopRequest(500, errorMsg, headers)
}


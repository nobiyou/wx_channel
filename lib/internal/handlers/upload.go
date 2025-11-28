package handlers

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"wx_channel/internal/config"
	"wx_channel/internal/storage"
	"wx_channel/internal/utils"

	"github.com/fatih/color"
	"github.com/qtgolang/SunnyNet/SunnyNet"
)

// UploadHandler 文件上传处理器
type UploadHandler struct {
	config     *config.Config
	csvManager *storage.CSVManager
	chunkSem   chan struct{}
	mergeSem   chan struct{}
}

// NewUploadHandler 创建上传处理器
func NewUploadHandler(cfg *config.Config, csvManager *storage.CSVManager) *UploadHandler {
	ch := cfg.UploadChunkConcurrency
	if ch <= 0 {
		ch = 4
	}
	mg := cfg.UploadMergeConcurrency
	if mg <= 0 {
		mg = 1
	}
	return &UploadHandler{
		config:     cfg,
		csvManager: csvManager,
		chunkSem:   make(chan struct{}, ch),
		mergeSem:   make(chan struct{}, mg),
	}
}

// HandleInitUpload 处理分片上传初始化请求
func (h *UploadHandler) HandleInitUpload(Conn *SunnyNet.HttpConn) bool {
	path := Conn.Request.URL.Path
	if path != "/__wx_channels_api/init_upload" {
		return false
	}

	if h.config != nil && h.config.SecretToken != "" {
		if Conn.Request.Header.Get("X-Local-Auth") != h.config.SecretToken {
			headers := http.Header{}
			headers.Set("Content-Type", "application/json")
			headers.Set("X-Content-Type-Options", "nosniff")
			Conn.StopRequest(401, `{"success":false,"error":"unauthorized"}`, headers)
			return true
		}
	}
	if h.config != nil && len(h.config.AllowedOrigins) > 0 {
		origin := Conn.Request.Header.Get("Origin")
		if origin != "" {
			allowed := false
			for _, o := range h.config.AllowedOrigins {
				if o == origin {
					allowed = true
					break
				}
			}
			if !allowed {
				headers := http.Header{}
				headers.Set("Content-Type", "application/json")
				headers.Set("X-Content-Type-Options", "nosniff")
				Conn.StopRequest(403, `{"success":false,"error":"forbidden_origin"}`, headers)
				return true
			}
		}
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
		utils.LogUploadInit(uploadId, false)
		h.sendErrorResponse(Conn, err)
		return true
	}

	// 记录上传初始化成功
	utils.LogUploadInit(uploadId, true)

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
	// 并发限流（分片）
	if h.chunkSem != nil {
		h.chunkSem <- struct{}{}
		defer func() { <-h.chunkSem }()
	}
	path := Conn.Request.URL.Path
	if path != "/__wx_channels_api/upload_chunk" {
		return false
	}

	if h.config != nil && h.config.SecretToken != "" {
		if Conn.Request.Header.Get("X-Local-Auth") != h.config.SecretToken {
			headers := http.Header{}
			headers.Set("Content-Type", "application/json")
			headers.Set("X-Content-Type-Options", "nosniff")
			Conn.StopRequest(401, `{"success":false,"error":"unauthorized"}`, headers)
			return true
		}
	}
	if h.config != nil && len(h.config.AllowedOrigins) > 0 {
		origin := Conn.Request.Header.Get("Origin")
		if origin != "" {
			allowed := false
			for _, o := range h.config.AllowedOrigins {
				if o == origin {
					allowed = true
					break
				}
			}
			if !allowed {
				headers := http.Header{}
				headers.Set("Content-Type", "application/json")
				headers.Set("X-Content-Type-Options", "nosniff")
				Conn.StopRequest(403, `{"success":false,"error":"forbidden_origin"}`, headers)
				return true
			}
		}
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

	utils.Info("[分片上传] 接收分片: uploadId=%s, 分片索引=%d/%d", uploadId, index+1, total)

	file, _, err := Conn.Request.FormFile("chunk")
	if err != nil {
		utils.HandleError(err, "获取分片文件")
		h.sendErrorResponse(Conn, err)
		return true
	}
	defer file.Close()

	checksum := Conn.Request.FormValue("checksum")
	algo := strings.ToLower(Conn.Request.FormValue("algo"))
	if algo == "" {
		algo = "md5"
	}
	var expectedSize int64 = -1
	if sz := Conn.Request.FormValue("size"); sz != "" {
		if v, convErr := strconv.ParseInt(sz, 10, 64); convErr == nil {
			expectedSize = v
		}
	}

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

	var written int64
	if checksum != "" {
		switch algo {
		case "md5":
			hsh := md5.New()
			n, err := io.Copy(io.MultiWriter(out, hsh), file)
			if err != nil {
				utils.HandleError(err, "写入分片数据")
				h.sendErrorResponse(Conn, err)
				return true
			}
			sum := fmt.Sprintf("%x", hsh.Sum(nil))
			if !strings.EqualFold(sum, checksum) {
				_ = out.Close()
				_ = os.Remove(partPath)
				utils.Error("[分片上传] 校验失败: uploadId=%s, 分片索引=%d, 算法=%s, 期望=%s, 实际=%s", uploadId, index, algo, checksum, sum)
				h.sendErrorResponse(Conn, fmt.Errorf("checksum_mismatch"))
				return true
			}
			written = n
			utils.Info("[分片上传] 校验通过: uploadId=%s, 分片索引=%d, 算法=%s, 大小=%.2fMB", uploadId, index, algo, float64(written)/(1024*1024))
		case "sha256":
			hsh := sha256.New()
			n, err := io.Copy(io.MultiWriter(out, hsh), file)
			if err != nil {
				utils.HandleError(err, "写入分片数据")
				h.sendErrorResponse(Conn, err)
				return true
			}
			sum := fmt.Sprintf("%x", hsh.Sum(nil))
			if !strings.EqualFold(sum, checksum) {
				_ = out.Close()
				_ = os.Remove(partPath)
				utils.Error("[分片上传] 校验失败: uploadId=%s, 分片索引=%d, 算法=%s, 期望=%s, 实际=%s", uploadId, index, algo, checksum, sum)
				h.sendErrorResponse(Conn, fmt.Errorf("checksum_mismatch"))
				return true
			}
			written = n
			utils.Info("[分片上传] 校验通过: uploadId=%s, 分片索引=%d, 算法=%s, 大小=%.2fMB", uploadId, index, algo, float64(written)/(1024*1024))
		default:
			h.sendErrorResponse(Conn, fmt.Errorf("unsupported_algo"))
			return true
		}
	} else {
		n, err := io.Copy(out, file)
		if err != nil {
			utils.HandleError(err, "写入分片数据")
			h.sendErrorResponse(Conn, err)
			return true
		}
		written = n
	}

	// 尺寸校验（可选字段 + 上限保护）
	if expectedSize >= 0 && written != expectedSize {
		_ = out.Close()
		_ = os.Remove(partPath)
		utils.Error("[分片上传] 尺寸不匹配: uploadId=%s, 分片索引=%d, 期望=%d, 实际=%d", uploadId, index, expectedSize, written)
		h.sendErrorResponse(Conn, fmt.Errorf("size_mismatch"))
		return true
	}
	if h.config != nil && h.config.ChunkSize > 0 && written > h.config.ChunkSize*2 { // 容忍放宽至2倍
		_ = out.Close()
		_ = os.Remove(partPath)
		utils.Error("[分片上传] 分片过大: uploadId=%s, 分片索引=%d, 大小=%d, 限制=%d", uploadId, index, written, h.config.ChunkSize*2)
		h.sendErrorResponse(Conn, fmt.Errorf("chunk_too_large"))
		return true
	}
	if err != nil {
		utils.HandleError(err, "写入分片数据")
		h.sendErrorResponse(Conn, err)
		return true
	}

	sizeMB := float64(written) / (1024 * 1024)
	utils.Info("[分片上传] 分片已保存: uploadId=%s, 分片索引=%d/%d, 大小=%.2fMB, 路径=%s", uploadId, index+1, total, sizeMB, partPath)

	// 记录分片上传成功
	utils.LogUploadChunk(uploadId, index, total, sizeMB, true)

	h.sendSuccessResponse(Conn)
	return true
}

// HandleCompleteUpload 处理分片上传完成请求
func (h *UploadHandler) HandleCompleteUpload(Conn *SunnyNet.HttpConn) bool {
	// 并发限流（合并）
	if h.mergeSem != nil {
		h.mergeSem <- struct{}{}
		defer func() { <-h.mergeSem }()
	}
	path := Conn.Request.URL.Path
	if path != "/__wx_channels_api/complete_upload" {
		return false
	}

	if h.config != nil && h.config.SecretToken != "" {
		if Conn.Request.Header.Get("X-Local-Auth") != h.config.SecretToken {
			headers := http.Header{}
			headers.Set("Content-Type", "application/json")
			headers.Set("X-Content-Type-Options", "nosniff")
			Conn.StopRequest(401, `{"success":false,"error":"unauthorized"}`, headers)
			return true
		}
	}
	if h.config != nil && len(h.config.AllowedOrigins) > 0 {
		origin := Conn.Request.Header.Get("Origin")
		if origin != "" {
			allowed := false
			for _, o := range h.config.AllowedOrigins {
				if o == origin {
					allowed = true
					break
				}
			}
			if !allowed {
				headers := http.Header{}
				headers.Set("Content-Type", "application/json")
				headers.Set("X-Content-Type-Options", "nosniff")
				Conn.StopRequest(403, `{"success":false,"error":"forbidden_origin"}`, headers)
				return true
			}
		}
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
		utils.Error("[分片合并] 缺少必要字段: uploadId=%s, total=%d, filename=%s", req.UploadId, req.Total, req.Filename)
		h.sendErrorResponse(Conn, fmt.Errorf("missing fields"))
		return true
	}
	utils.Info("[分片合并] 开始合并: uploadId=%s, 文件名=%s, 作者=%s, 分片数=%d", req.UploadId, req.Filename, req.AuthorName, req.Total)

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

	// 基本存在性与数量校验
	for i := 0; i < req.Total; i++ {
		partPath := filepath.Join(upDir, fmt.Sprintf("%06d.part", i))
		if _, err := os.Stat(partPath); err != nil {
			utils.Error("[分片合并] 分片缺失: uploadId=%s, 分片索引=%d, 路径=%s", req.UploadId, i, partPath)
			h.sendErrorResponse(Conn, fmt.Errorf("missing_part_%06d", i))
			return true
		}
	}

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
	utils.Info("[分片合并] 合并完成: uploadId=%s, 文件名=%s, 作者=%s, 路径=%s, 大小=%.2fMB, 分片数=%d", req.UploadId, req.Filename, req.AuthorName, finalPath, fileSize, req.Total)
	color.Green("✓ 分片视频已保存: %s (%.2f MB)", finalPath, fileSize)

	// 记录分片合并成功
	utils.LogUploadMerge(req.UploadId, req.Filename, req.AuthorName, req.Total, fileSize, true)

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

	if h.config != nil && h.config.SecretToken != "" {
		if Conn.Request.Header.Get("X-Local-Auth") != h.config.SecretToken {
			headers := http.Header{}
			headers.Set("Content-Type", "application/json")
			headers.Set("X-Content-Type-Options", "nosniff")
			Conn.StopRequest(401, `{"success":false,"error":"unauthorized"}`, headers)
			return true
		}
	}
	if h.config != nil && len(h.config.AllowedOrigins) > 0 {
		origin := Conn.Request.Header.Get("Origin")
		if origin != "" {
			allowed := false
			for _, o := range h.config.AllowedOrigins {
				if o == origin {
					allowed = true
					break
				}
			}
			if !allowed {
				headers := http.Header{}
				headers.Set("Content-Type", "application/json")
				headers.Set("X-Content-Type-Options", "nosniff")
				Conn.StopRequest(403, `{"success":false,"error":"forbidden_origin"}`, headers)
				return true
			}
		}
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

	// 记录直接上传成功
	utils.LogDirectUpload(filename, authorName, fileSize, isEncrypted, true)

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

// HandleUploadStatus 查询已上传的分片列表
func (h *UploadHandler) HandleUploadStatus(Conn *SunnyNet.HttpConn) bool {
	path := Conn.Request.URL.Path
	if path != "/__wx_channels_api/upload_status" {
		return false
	}

	if h.config != nil && h.config.SecretToken != "" {
		if Conn.Request.Header.Get("X-Local-Auth") != h.config.SecretToken {
			headers := http.Header{}
			headers.Set("Content-Type", "application/json")
			headers.Set("X-Content-Type-Options", "nosniff")
			Conn.StopRequest(401, `{"success":false,"error":"unauthorized"}`, headers)
			return true
		}
	}
	if h.config != nil && len(h.config.AllowedOrigins) > 0 {
		origin := Conn.Request.Header.Get("Origin")
		if origin != "" {
			allowed := false
			for _, o := range h.config.AllowedOrigins {
				if o == origin {
					allowed = true
					break
				}
			}
			if !allowed {
				headers := http.Header{}
				headers.Set("Content-Type", "application/json")
				headers.Set("X-Content-Type-Options", "nosniff")
				Conn.StopRequest(403, `{"success":false,"error":"forbidden_origin"}`, headers)
				return true
			}
		}
	}

	body, err := io.ReadAll(Conn.Request.Body)
	if err != nil {
		h.sendErrorResponse(Conn, err)
		return true
	}
	_ = Conn.Request.Body.Close()

	var req struct {
		UploadId string `json:"uploadId"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		h.sendErrorResponse(Conn, err)
		return true
	}
	if req.UploadId == "" {
		h.sendErrorResponse(Conn, fmt.Errorf("missing_uploadId"))
		return true
	}

	baseDir, err := utils.GetBaseDir()
	if err != nil {
		h.sendErrorResponse(Conn, err)
		return true
	}
	upDir := filepath.Join(baseDir, h.config.DownloadsDir, ".uploads", req.UploadId)
	entries, err := os.ReadDir(upDir)
	if err != nil {
		h.sendErrorResponse(Conn, err)
		return true
	}

	parts := []int{}
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".part") && len(name) >= 10 {
			idxStr := strings.TrimSuffix(name, ".part")
			if n, convErr := strconv.Atoi(strings.TrimLeft(idxStr, "0")); convErr == nil {
				parts = append(parts, n)
			} else if idxStr == "000000" { // 0 特判
				parts = append(parts, 0)
			}
		}
	}

	resp := map[string]interface{}{"success": true, "parts": parts}
	b, _ := json.Marshal(resp)
	h.sendJSONResponse(Conn, 200, b)
	return true
}

// sendSuccessResponse 发送成功响应
func (h *UploadHandler) sendSuccessResponse(Conn *SunnyNet.HttpConn) {
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	headers.Set("Pragma", "no-cache")
	headers.Set("Expires", "0")
	headers.Set("X-Content-Type-Options", "nosniff")
	if h.config != nil && len(h.config.AllowedOrigins) > 0 {
		origin := Conn.Request.Header.Get("Origin")
		if origin != "" {
			for _, o := range h.config.AllowedOrigins {
				if o == origin {
					headers.Set("Access-Control-Allow-Origin", origin)
					headers.Set("Vary", "Origin")
					headers.Set("Access-Control-Allow-Headers", "Content-Type, X-Local-Auth")
					headers.Set("Access-Control-Allow-Methods", "POST, OPTIONS")
					break
				}
			}
		}
	}
	Conn.StopRequest(200, `{"success":true}`, headers)
}

// sendJSONResponse 发送JSON响应
func (h *UploadHandler) sendJSONResponse(Conn *SunnyNet.HttpConn, statusCode int, body []byte) {
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	headers.Set("Pragma", "no-cache")
	headers.Set("Expires", "0")
	headers.Set("X-Content-Type-Options", "nosniff")
	if h.config != nil && len(h.config.AllowedOrigins) > 0 {
		origin := Conn.Request.Header.Get("Origin")
		if origin != "" {
			for _, o := range h.config.AllowedOrigins {
				if o == origin {
					headers.Set("Access-Control-Allow-Origin", origin)
					headers.Set("Vary", "Origin")
					headers.Set("Access-Control-Allow-Headers", "Content-Type, X-Local-Auth")
					headers.Set("Access-Control-Allow-Methods", "POST, OPTIONS")
					break
				}
			}
		}
	}
	Conn.StopRequest(statusCode, string(body), headers)
}

// sendErrorResponse 发送错误响应
func (h *UploadHandler) sendErrorResponse(Conn *SunnyNet.HttpConn, err error) {
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("X-Content-Type-Options", "nosniff")
	if h.config != nil && len(h.config.AllowedOrigins) > 0 {
		origin := Conn.Request.Header.Get("Origin")
		if origin != "" {
			for _, o := range h.config.AllowedOrigins {
				if o == origin {
					headers.Set("Access-Control-Allow-Origin", origin)
					headers.Set("Vary", "Origin")
					headers.Set("Access-Control-Allow-Headers", "Content-Type, X-Local-Auth")
					headers.Set("Access-Control-Allow-Methods", "POST, OPTIONS")
					break
				}
			}
		}
	}
	errorMsg := fmt.Sprintf(`{"success":false,"error":"%s"}`, err.Error())
	Conn.StopRequest(500, errorMsg, headers)
}

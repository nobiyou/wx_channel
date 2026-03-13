package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"wx_channel/internal/database"
	"wx_channel/internal/utils"
	"wx_channel/internal/websocket"
)

// RadarService 负责定时轮询并下载对标账号的新视频
// Requirements: Competitor 24-hour Silent Radar
type RadarService struct {
	repo         *database.RadarRepository
	queueService *QueueService
	hub          *websocket.Hub
	settings     *database.SettingsRepository

	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	wg     sync.WaitGroup

	ticker *time.Ticker
}

// NewRadarService 创建一个新的雷达服务
func NewRadarService(repo *database.RadarRepository, queueService *QueueService, hub *websocket.Hub) *RadarService {
	ctx, cancel := context.WithCancel(context.Background())
	return &RadarService{
		repo:         repo,
		queueService: queueService,
		hub:          hub,
		settings:     database.NewSettingsRepository(),
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Start 启动雷达服务轮询器
func (s *RadarService) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ticker != nil {
		return // 已启动
	}

	// 默认每分钟检查一次，但实际是否触发取决于每个 target 的 interval_minutes
	s.ticker = time.NewTicker(time.Minute)
	s.wg.Add(1)

	go func() {
		defer s.wg.Done()
		utils.LogInfo("Radar Service (24h静默雷达) 已启动")

		// 启动时立即执行一次检测（延迟10秒，等待WebSocket连接建立）
		time.Sleep(10 * time.Second)
		s.checkTargets()

		for {
			select {
			case <-s.ctx.Done():
				utils.LogInfo("Radar Service 已停止")
				return
			case <-s.ticker.C:
				s.checkTargets()
			}
		}
	}()
}

// Stop 停止雷达服务
func (s *RadarService) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cancel()
	if s.ticker != nil {
		s.ticker.Stop()
		s.ticker = nil
	}
	s.wg.Wait()
}

// checkTargets 遍历并检查所有活动的雷达目标
func (s *RadarService) checkTargets() {
	targets, err := s.repo.GetActive()
	if err != nil {
		utils.LogError("获取活动雷达目标失败: %v", err)
		return
	}

	if len(targets) == 0 {
		return
	}

	now := time.Now()
	hasClient := s.hub.ClientCount() > 0

	for _, target := range targets {
		// 检查是否到了该检测的时间
		if target.LastCheckTime != nil {
			elapsed := now.Sub(*target.LastCheckTime)
			if elapsed < time.Duration(target.IntervalMinutes)*time.Minute {
				continue // 还没到时间
			}
		}

		if !hasClient {
			// 更新最后检测时间
			_ = s.repo.UpdateLastCheckTime(target.ID, now)
			// 插入错误日志
			_ = s.repo.AddLog(&database.RadarLog{
				TargetID:     target.ID,
				CheckTime:    now,
				Status:       "error",
				ErrorMessage: "微信客户端未连接或被关闭，请重新注入",
			})
			continue // 跳过实际检测
		}

		// 执行检测
		s.processTarget(target)
	}
}

// processTarget 处理单个雷达监控目标的拉取与对比逻辑
func (s *RadarService) processTarget(target database.RadarTarget) {
	utils.LogInfo("[Radar] 开始检测账号: %s (%s)", target.AuthorName, target.Username)

	// 更新最后检测时间
	now := time.Now()
	if err := s.repo.UpdateLastCheckTime(target.ID, now); err != nil {
		utils.LogError("[Radar] 更新检测时间失败 [%s]: %v", target.ID, err)
	}

	// 初始化日志记录
	radarLog := &database.RadarLog{
		TargetID:  target.ID,
		CheckTime: now,
		Status:    "success",
	}

	// 1. 调用 WebSocket 获取用户视频列表 (feed_list)
	body := websocket.FeedListBody{
		Username:   target.Username,
		NextMarker: "", // 只需要第一页最新数据
	}

	// 限制 30 秒超时
	data, err := s.hub.CallAPI("key:channels:feed_list", body, 30*time.Second)
	if err != nil {
		radarLog.Status = "error"
		if strings.Contains(err.Error(), "no available client") {
			radarLog.ErrorMessage = "微信客户端未连接或已退出"
			utils.LogWarn("[Radar] 检测失败 [%s]: %s", target.AuthorName, radarLog.ErrorMessage)
		} else {
			radarLog.ErrorMessage = err.Error()
			utils.LogError("[Radar] 获取视频列表失败 [%s]: %v", target.AuthorName, err)
		}
		_ = s.repo.AddLog(radarLog)
		return
	}

	// 2. 解析返回列表数据
	var rawResp struct {
		Data struct {
			BaseResponse struct {
				Ret int `json:"Ret"`
			} `json:"BaseResponse"`
			ObjectList []interface{} `json:"objectList"`
			Object     []interface{} `json:"object"`
		} `json:"data"`
	}

	if err := json.Unmarshal(data, &rawResp); err != nil {
		radarLog.Status = "error"
		radarLog.ErrorMessage = "解析返回数据失败: " + err.Error()
		utils.LogError("[Radar] 解析视频列表失败 [%s]: %v", target.AuthorName, err)
		_ = s.repo.AddLog(radarLog)
		return
	}

	if rawResp.Data.BaseResponse.Ret != 0 {
		radarLog.Status = "error"
		radarLog.ErrorMessage = fmt.Sprintf("微信接口返回失败，状态码: %d (可能是请求过于频繁或账号异常)", rawResp.Data.BaseResponse.Ret)
		utils.LogWarn("[Radar] 账号 [%s] 获取数据被微信拒绝(Ret:%d)", target.AuthorName, rawResp.Data.BaseResponse.Ret)
		_ = s.repo.AddLog(radarLog)
		return
	}

	// 兼容老版本或新版本 WeChat 可能返回的字段
	allObjects := rawResp.Data.ObjectList
	if len(allObjects) == 0 && len(rawResp.Data.Object) > 0 {
		allObjects = rawResp.Data.Object
	}

	radarLog.FoundVideos = len(allObjects)

	if radarLog.FoundVideos == 0 {
		utils.LogInfo("[Radar] 账号 [%s] 暂无视频数据(Raw Data Size: %d)", target.AuthorName, len(data))
		_ = s.repo.AddLog(radarLog)
		return
	}

	// 3. 提取视频 ID 并检查库中是否已存在
	newVideoCount := 0
	settings, _ := s.settings.Load()
	if settings == nil {
		settings = database.DefaultSettings()
	}

	// 获取下载记录的 Repo
	downloadRepo := database.NewDownloadRecordRepository()

	// 用于记录本次扫描的所有视频摘要
	var videoSummaries []database.RadarVideoSummary

	for _, objInter := range allObjects {
		objMap, ok := objInter.(map[string]interface{})
		if !ok {
			continue
		}

		// 检查必要字段
		idInter, ok := objMap["id"]
		if !ok || idInter == "" {
			continue
		}
		videoID := fmt.Sprintf("%v", idInter)

		// 从 objectDesc 里提取标题和媒体信息（与订阅功能一致，无需再调 feed_profile）
		title := ""
		videoURL := ""
		coverURL := ""
		decodeKey := ""
		var fileSize int64
		var duration int64
		resolution := ""

		if descInter, ok := objMap["objectDesc"]; ok {
			if descMap, ok := descInter.(map[string]interface{}); ok {
				if t, ok := descMap["description"].(string); ok {
					title = t
				}
				// 遍历媒体列表，取第一条视频媒体
				if mediaList, ok := descMap["media"].([]interface{}); ok && len(mediaList) > 0 {
					if m, ok := mediaList[0].(map[string]interface{}); ok {
						rawURL, _ := m["url"].(string)
						urlToken, _ := m["urlToken"].(string)
						if rawURL != "" {
							videoURL = rawURL + urlToken
						}
						coverURL, _ = m["thumbUrl"].(string)
						decodeKey, _ = m["decodeKey"].(string)
						if fs, ok := m["fileSize"].(float64); ok {
							fileSize = int64(fs)
						}
						if dur, ok := m["videoDuration"].(float64); ok {
							duration = int64(dur)
						}
						if r, ok := m["videoResolution"].(string); ok {
							resolution = r
						}
					}
				}
			}
		}

		if title == "" {
			title = fmt.Sprintf("RadarV_%s", videoID)
		}

		// 4. 判断是否需要下载
		isNew := true

		record, _ := downloadRepo.GetByVideoID(videoID)
		if record != nil && (record.Status == database.DownloadStatusCompleted || record.Status == database.DownloadStatusInProgress) {
			isNew = false
		}

		if isNew {
			queueItem, _ := s.queueService.GetByVideoID(videoID)
			if queueItem != nil && (queueItem.Status == database.QueueStatusPending || queueItem.Status == database.QueueStatusDownloading || queueItem.Status == database.QueueStatusCompleted) {
				isNew = false
			}
		}

		// 记录视频摘要
		videoSummaries = append(videoSummaries, database.RadarVideoSummary{
			VideoID: videoID,
			Title:   title,
			IsNew:   isNew,
		})

		if isNew {
			if videoURL == "" {
				utils.LogWarn("[Radar] 新视频 [%s] 无法提取 URL，跳过: %s", target.AuthorName, videoID)
				continue
			}
			utils.LogInfo("[Radar] 发现新视频 [%s]: %s (%s)", target.AuthorName, title, videoID)
			newVideoCount++

			// 直接从 feed_list 数据入队，无需额外请求 feed_profile
			req := []VideoInfo{{
				VideoID:    videoID,
				Title:      title,
				Author:     target.AuthorName,
				VideoURL:   videoURL,
				CoverURL:   coverURL,
				Size:       fileSize,
				DecryptKey: decodeKey,
				Duration:   duration,
				Resolution: resolution,
			}}
			if _, err := s.queueService.AddToQueue(req); err != nil {
				utils.LogError("[Radar] 添加视频到下载队列失败 [%s]-[%s]: %v", target.AuthorName, title, err)
			} else {
				utils.LogInfo("[Radar] 成功加入队列: %s", title)
			}
		}
	}

	radarLog.NewVideos = newVideoCount

	// 将视频摘要序列化后存入日志
	if len(videoSummaries) > 0 {
		if b, err := json.Marshal(videoSummaries); err == nil {
			radarLog.VideoList = string(b)
		}
	}

	_ = s.repo.AddLog(radarLog)

	if newVideoCount > 0 {
		utils.LogInfo("[Radar] 账号 [%s] 检测完毕，新增 %d 个视频并加入下载队列", target.AuthorName, newVideoCount)
	}
}

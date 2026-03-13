package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	"wx_channel/hub_server/database"
)

// GetDatabaseStats 获取数据库统计信息
func GetDatabaseStats(w http.ResponseWriter, r *http.Request) {
	type TableStats struct {
		TableName    string `json:"table_name"`
		RecordCount  int64  `json:"record_count"`
		SizeMB       string `json:"size_mb"`
		OldestRecord string `json:"oldest_record"`
		NewestRecord string `json:"newest_record"`
	}

	stats := make([]TableStats, 0)

	// 获取数据库总大小
	var dbSize struct {
		SizeMB float64
	}
	database.DB.Raw(`
		SELECT 
			CAST(page_count * page_size AS REAL) / 1024.0 / 1024.0 as size_mb
		FROM pragma_page_count(), pragma_page_size()
	`).Scan(&dbSize)

	// 统计各表记录数和估算大小
	tableRecords := make(map[string]int64)
	
	// 用户表
	var userCount int64
	database.DB.Raw("SELECT COUNT(*) FROM users").Scan(&userCount)
	tableRecords["users"] = userCount
	
	// 设备表
	var deviceCount int64
	database.DB.Raw("SELECT COUNT(*) FROM nodes").Scan(&deviceCount)
	tableRecords["nodes"] = deviceCount
	
	// 订阅表
	var subCount int64
	database.DB.Raw("SELECT COUNT(*) FROM subscriptions").Scan(&subCount)
	tableRecords["subscriptions"] = subCount
	
	// 订阅视频表
	var videoCount int64
	database.DB.Raw("SELECT COUNT(*) FROM subscribed_videos").Scan(&videoCount)
	tableRecords["subscribed_videos"] = videoCount
	
	// 任务表
	var taskCount int64
	database.DB.Raw("SELECT COUNT(*) FROM tasks").Scan(&taskCount)
	tableRecords["tasks"] = taskCount
	
	// 交易表
	var txCount int64
	database.DB.Raw("SELECT COUNT(*) FROM transactions").Scan(&txCount)
	tableRecords["transactions"] = txCount
	
	// 同步状态表
	var syncStatusCount int64
	database.DB.Raw("SELECT COUNT(*) FROM sync_status").Scan(&syncStatusCount)
	tableRecords["sync_status"] = syncStatusCount

	// 浏览记录统计
	var browseStats struct {
		Count  int64
		Oldest time.Time
		Newest time.Time
	}
	database.DB.Raw(`
		SELECT 
			COUNT(*) as count,
			COALESCE(MIN(browse_time), '0001-01-01') as oldest,
			COALESCE(MAX(browse_time), '0001-01-01') as newest
		FROM hub_browse_history
	`).Scan(&browseStats)

	oldestBrowse := "-"
	newestBrowse := "-"
	if !browseStats.Oldest.IsZero() && browseStats.Oldest.Year() > 1 {
		oldestBrowse = browseStats.Oldest.Format("2006-01-02 15:04")
	}
	if !browseStats.Newest.IsZero() && browseStats.Newest.Year() > 1 {
		newestBrowse = browseStats.Newest.Format("2006-01-02 15:04")
	}

	// 下载记录统计
	var downloadStats struct {
		Count  int64
		Oldest time.Time
		Newest time.Time
	}
	database.DB.Raw(`
		SELECT 
			COUNT(*) as count,
			COALESCE(MIN(download_time), '0001-01-01') as oldest,
			COALESCE(MAX(download_time), '0001-01-01') as newest
		FROM hub_download_records
	`).Scan(&downloadStats)

	oldestDownload := "-"
	newestDownload := "-"
	if !downloadStats.Oldest.IsZero() && downloadStats.Oldest.Year() > 1 {
		oldestDownload = downloadStats.Oldest.Format("2006-01-02 15:04")
	}
	if !downloadStats.Newest.IsZero() && downloadStats.Newest.Year() > 1 {
		newestDownload = downloadStats.Newest.Format("2006-01-02 15:04")
	}

	// 同步历史统计
	var syncHistoryStats struct {
		Count  int64
		Oldest time.Time
		Newest time.Time
	}
	database.DB.Raw(`
		SELECT 
			COUNT(*) as count,
			COALESCE(MIN(sync_time), '0001-01-01') as oldest,
			COALESCE(MAX(sync_time), '0001-01-01') as newest
		FROM sync_history
	`).Scan(&syncHistoryStats)

	oldestSync := "-"
	newestSync := "-"
	if !syncHistoryStats.Oldest.IsZero() && syncHistoryStats.Oldest.Year() > 1 {
		oldestSync = syncHistoryStats.Oldest.Format("2006-01-02 15:04")
	}
	if !syncHistoryStats.Newest.IsZero() && syncHistoryStats.Newest.Year() > 1 {
		newestSync = syncHistoryStats.Newest.Format("2006-01-02 15:04")
	}
	
	// 计算总记录数
	totalRecords := browseStats.Count + downloadStats.Count + syncHistoryStats.Count
	for _, count := range tableRecords {
		totalRecords += count
	}
	
	// 估算每个表的大小（基于记录数占比）
	estimateSize := func(count int64) string {
		if totalRecords == 0 {
			return "0.00"
		}
		ratio := float64(count) / float64(totalRecords)
		size := dbSize.SizeMB * ratio
		return fmt.Sprintf("%.2f", size)
	}

	// 添加主要数据表（按记录数排序）
	type tableInfo struct {
		name        string
		displayName string
		count       int64
		oldest      string
		newest      string
	}
	
	allTables := []tableInfo{
		{"subscribed_videos", "订阅视频 (subscribed_videos)", videoCount, "-", "-"},
		{"hub_browse_history", "浏览记录 (hub_browse_history)", browseStats.Count, oldestBrowse, newestBrowse},
		{"hub_download_records", "下载记录 (hub_download_records)", downloadStats.Count, oldestDownload, newestDownload},
		{"transactions", "交易记录 (transactions)", txCount, "-", "-"},
		{"tasks", "任务 (tasks)", taskCount, "-", "-"},
		{"nodes", "设备 (nodes)", deviceCount, "-", "-"},
		{"subscriptions", "订阅 (subscriptions)", subCount, "-", "-"},
		{"users", "用户 (users)", userCount, "-", "-"},
		{"sync_status", "同步状态 (sync_status)", syncStatusCount, "-", "-"},
		{"sync_history", "同步历史 (sync_history)", syncHistoryStats.Count, oldestSync, newestSync},
	}
	
	// 按记录数排序
	for i := 0; i < len(allTables); i++ {
		for j := i + 1; j < len(allTables); j++ {
			if allTables[j].count > allTables[i].count {
				allTables[i], allTables[j] = allTables[j], allTables[i]
			}
		}
	}
	
	// 生成统计信息
	for _, table := range allTables {
		stats = append(stats, TableStats{
			TableName:    table.displayName,
			RecordCount:  table.count,
			SizeMB:       estimateSize(table.count),
			OldestRecord: table.oldest,
			NewestRecord: table.newest,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{
			"tables":        stats,
			"size_mb":       fmt.Sprintf("%.2f", dbSize.SizeMB),
			"total_records": totalRecords,
			"table_count":   len(allTables),
		},
	})
}

// OptimizeDatabase 优化数据库
func OptimizeDatabase(w http.ResponseWriter, r *http.Request) {
	sqlDB, err := database.DB.DB()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    1,
			"message": "Failed to get database connection",
		})
		return
	}

	// 执行优化操作
	operations := []struct {
		Name string
		SQL  string
	}{
		{"设置缓存大小", "PRAGMA cache_size=-64000"},
		{"启用内存映射", "PRAGMA mmap_size=268435456"},
		{"更新统计信息", "ANALYZE"},
		{"清理碎片", "VACUUM"},
	}

	results := make([]map[string]interface{}, 0)
	for _, op := range operations {
		start := time.Now()
		_, err := sqlDB.Exec(op.SQL)
		duration := time.Since(start)

		result := map[string]interface{}{
			"operation": op.Name,
			"duration":  fmt.Sprintf("%.2fs", duration.Seconds()),
			"success":   err == nil,
		}
		if err != nil {
			result["error"] = err.Error()
		}
		results = append(results, result)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code":    0,
		"message": "优化完成",
		"data":    results,
	})
}

// ArchiveOldData 归档旧数据
func ArchiveOldData(w http.ResponseWriter, r *http.Request) {
	type ArchiveRequest struct {
		BrowseMonths  int `json:"browse_months"`  // 保留几个月的浏览记录
		DownloadYears int `json:"download_years"` // 保留几年的下载记录
		HistoryMonths int `json:"history_months"` // 保留几个月的同步历史
	}

	var req ArchiveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    1,
			"message": "Invalid request",
		})
		return
	}

	// 默认值
	if req.BrowseMonths == 0 {
		req.BrowseMonths = 6
	}
	if req.DownloadYears == 0 {
		req.DownloadYears = 1
	}
	if req.HistoryMonths == 0 {
		req.HistoryMonths = 3
	}

	// 统计将要删除的记录数
	var browseCount, downloadCount, historyCount int64

	database.DB.Raw(fmt.Sprintf(`
		SELECT COUNT(*) FROM hub_browse_history 
		WHERE browse_time < datetime('now', '-%d months')
	`, req.BrowseMonths)).Scan(&browseCount)

	database.DB.Raw(fmt.Sprintf(`
		SELECT COUNT(*) FROM hub_download_records 
		WHERE download_time < datetime('now', '-%d years')
	`, req.DownloadYears)).Scan(&downloadCount)

	database.DB.Raw(fmt.Sprintf(`
		SELECT COUNT(*) FROM sync_history 
		WHERE sync_time < datetime('now', '-%d months')
	`, req.HistoryMonths)).Scan(&historyCount)

	// 执行删除
	tx := database.DB.Begin()

	// 删除旧的浏览记录
	result := tx.Exec(fmt.Sprintf(`
		DELETE FROM hub_browse_history 
		WHERE browse_time < datetime('now', '-%d months')
	`, req.BrowseMonths))
	if result.Error != nil {
		tx.Rollback()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    1,
			"message": "Failed to delete browse records",
		})
		return
	}

	// 删除旧的下载记录
	result = tx.Exec(fmt.Sprintf(`
		DELETE FROM hub_download_records 
		WHERE download_time < datetime('now', '-%d years')
	`, req.DownloadYears))
	if result.Error != nil {
		tx.Rollback()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    1,
			"message": "Failed to delete download records",
		})
		return
	}

	// 删除旧的同步历史
	result = tx.Exec(fmt.Sprintf(`
		DELETE FROM sync_history 
		WHERE sync_time < datetime('now', '-%d months')
	`, req.HistoryMonths))
	if result.Error != nil {
		tx.Rollback()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    1,
			"message": "Failed to delete sync history",
		})
		return
	}

	tx.Commit()

	// 优化数据库
	sqlDB, _ := database.DB.DB()
	sqlDB.Exec("ANALYZE")
	sqlDB.Exec("VACUUM")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code":    0,
		"message": "归档完成",
		"data": map[string]interface{}{
			"deleted_browse":   browseCount,
			"deleted_download": downloadCount,
			"deleted_history":  historyCount,
			"total_deleted":    browseCount + downloadCount + historyCount,
		},
	})
}

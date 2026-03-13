package database

import (
	"database/sql"
	"time"

	"wx_channel/internal/utils"
)

// RadarTargetStatus 定义雷达监控目标的运行状态
type RadarTargetStatus string

const (
	RadarStatusActive RadarTargetStatus = "active" // 监控中
	RadarStatusPaused RadarTargetStatus = "paused" // 已暂停
)

// RadarTarget 表示一个雷达监控目标
type RadarTarget struct {
	ID              string            `json:"id"`
	Username        string            `json:"username"`
	AuthorName      string            `json:"author_name"`
	IntervalMinutes int               `json:"interval_minutes"` // 监控频率 (分钟)
	LastCheckTime   *time.Time        `json:"last_check_time"`  // 上次检测时间 (可能为 nil)
	Status          RadarTargetStatus `json:"status"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

// RadarRepository 处理雷达配置相关的数据库操作
type RadarRepository struct{}

// NewRadarRepository 创建一个新的雷达仓库
func NewRadarRepository() *RadarRepository {
	return &RadarRepository{}
}

// targetFromRow 从数据库行扫描 Target 数据
func (r *RadarRepository) targetFromRow(scanner interface{ Scan(...interface{}) error }) (*RadarTarget, error) {
	var target RadarTarget
	var lastCheckTimeStr sql.NullString
	var createdAtStr, updatedAtStr string

	err := scanner.Scan(
		&target.ID,
		&target.Username,
		&target.AuthorName,
		&target.IntervalMinutes,
		&lastCheckTimeStr,
		&target.Status,
		&createdAtStr,
		&updatedAtStr,
	)
	if err != nil {
		return nil, err
	}

	// 转换时间
	if lastCheckTimeStr.Valid && lastCheckTimeStr.String != "" {
		if t, err := time.Parse(time.RFC3339, lastCheckTimeStr.String); err == nil {
			target.LastCheckTime = &t
		}
	}

	target.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	target.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)

	return &target, nil
}

// Add 添加一个新的监控目标
func (r *RadarRepository) Add(target *RadarTarget) error {
	if target.ID == "" {
		target.ID = utils.RandomString(12)
	}

	now := time.Now().Format(time.RFC3339)
	var lastCheckTime interface{}
	if target.LastCheckTime != nil {
		lastCheckTime = target.LastCheckTime.Format(time.RFC3339)
	}

	query := `
		INSERT INTO radar_targets (
			id, username, author_name, interval_minutes, last_check_time, status, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := db.Exec(query,
		target.ID,
		target.Username,
		target.AuthorName,
		target.IntervalMinutes,
		lastCheckTime,
		target.Status,
		now,
		now,
	)
	return err
}

// Update 更新监控目标配置或状态
func (r *RadarRepository) Update(target *RadarTarget) error {
	now := time.Now().Format(time.RFC3339)
	var lastCheckTime interface{}
	if target.LastCheckTime != nil {
		lastCheckTime = target.LastCheckTime.Format(time.RFC3339)
	}

	query := `
		UPDATE radar_targets 
		SET username = ?, author_name = ?, interval_minutes = ?, last_check_time = ?, status = ?, updated_at = ?
		WHERE id = ?
	`
	_, err := db.Exec(query,
		target.Username,
		target.AuthorName,
		target.IntervalMinutes,
		lastCheckTime,
		target.Status,
		now,
		target.ID,
	)
	return err
}

// UpdateLastCheckTime 仅更新上次检测时间
func (r *RadarRepository) UpdateLastCheckTime(id string, lastCheckTime time.Time) error {
	now := time.Now().Format(time.RFC3339)
	query := `
		UPDATE radar_targets 
		SET last_check_time = ?, updated_at = ?
		WHERE id = ?
	`
	_, err := db.Exec(query, lastCheckTime.Format(time.RFC3339), now, id)
	return err
}

// GetAll 获取所有监控目标
func (r *RadarRepository) GetAll() ([]RadarTarget, error) {
	query := `
		SELECT id, username, author_name, interval_minutes, last_check_time, status, created_at, updated_at
		FROM radar_targets
		ORDER BY created_at DESC
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var targets []RadarTarget
	for rows.Next() {
		target, err := r.targetFromRow(rows)
		if err != nil {
			return nil, err
		}
		targets = append(targets, *target)
	}
	return targets, nil
}

// GetActive 获取所有活动状态的监控目标
func (r *RadarRepository) GetActive() ([]RadarTarget, error) {
	query := `
		SELECT id, username, author_name, interval_minutes, last_check_time, status, created_at, updated_at
		FROM radar_targets
		WHERE status = 'active'
		ORDER BY created_at DESC
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var targets []RadarTarget
	for rows.Next() {
		target, err := r.targetFromRow(rows)
		if err != nil {
			return nil, err
		}
		targets = append(targets, *target)
	}
	return targets, nil
}

// GetByID 通过 ID 获取监控目标
func (r *RadarRepository) GetByID(id string) (*RadarTarget, error) {
	query := `
		SELECT id, username, author_name, interval_minutes, last_check_time, status, created_at, updated_at
		FROM radar_targets
		WHERE id = ?
	`
	row := db.QueryRow(query, id)
	return r.targetFromRow(row)
}

// Delete 删除监控目标
func (r *RadarRepository) Delete(id string) error {
	query := "DELETE FROM radar_targets WHERE id = ?"
	_, err := db.Exec(query, id)
	return err
}

// UpdateStatus 更新监控目标状态 (暂停/恢复)
func (r *RadarRepository) UpdateStatus(id string, status RadarTargetStatus) error {
	now := time.Now().Format(time.RFC3339)
	query := "UPDATE radar_targets SET status = ?, updated_at = ? WHERE id = ?"
	_, err := db.Exec(query, status, now, id)
	return err
}

// ======================== Radar Logs ========================

// AddLog 记录一条执行日志
func (r *RadarRepository) AddLog(log *RadarLog) error {
	if log.ID == "" {
		log.ID = utils.RandomString(12)
	}
	if log.CheckTime.IsZero() {
		log.CheckTime = time.Now()
	}

	query := `
		INSERT INTO radar_logs (
			id, target_id, check_time, found_videos, new_videos, status, error_message, video_list
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := db.Exec(query,
		log.ID,
		log.TargetID,
		log.CheckTime.Format(time.RFC3339),
		log.FoundVideos,
		log.NewVideos,
		log.Status,
		log.ErrorMessage,
		log.VideoList,
	)
	return err
}

// GetLogsByTargetID 获取指定监控目标的最新执行日志，按时间倒序
func (r *RadarRepository) GetLogsByTargetID(targetID string, limit int) ([]RadarLog, error) {
	if limit <= 0 {
		limit = 50 // 默认最多返回最近50条
	}

	query := `
		SELECT id, target_id, check_time, found_videos, new_videos, status, error_message, COALESCE(video_list, '')
		FROM radar_logs
		WHERE target_id = ?
		ORDER BY check_time DESC
		LIMIT ?
	`
	rows, err := db.Query(query, targetID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []RadarLog
	for rows.Next() {
		var log RadarLog
		var checkTimeStr string
		err := rows.Scan(
			&log.ID,
			&log.TargetID,
			&checkTimeStr,
			&log.FoundVideos,
			&log.NewVideos,
			&log.Status,
			&log.ErrorMessage,
			&log.VideoList,
		)
		if err != nil {
			return nil, err
		}
		log.CheckTime, _ = time.Parse(time.RFC3339, checkTimeStr)
		logs = append(logs, log)
	}
	return logs, nil
}

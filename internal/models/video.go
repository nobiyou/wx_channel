package models

import "time"

// VideoProfile 视频信息模型
type VideoProfile struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Author       string    `json:"nickname"`
	AuthorType   string    `json:"author_type"`
	OfficialName string    `json:"official_name"`
	URL          string    `json:"url"`
	PageURL      string    `json:"page_url"`
	FileSize     string    `json:"file_size"`
	Duration     string    `json:"duration"`
	PlayCount    string    `json:"play_count"`
	LikeCount    string    `json:"like_count"`
	CommentCount string    `json:"comment_count"`
	FavCount     string    `json:"fav_count"`
	ForwardCount string    `json:"forward_count"`
	CreateTime   string    `json:"create_time"`
	IPRegion     string    `json:"ip_region"`
	DownloadAt   time.Time `json:"download_at"`
	// 原始数据字段
	RawData map[string]interface{} `json:"-"` // 不序列化到JSON
}

// VideoDownloadRecord 下载记录模型
type VideoDownloadRecord struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Author       string    `json:"nickname"`
	AuthorType   string    `json:"author_type"`
	OfficialName string    `json:"official_name"`
	URL          string    `json:"url"`
	PageURL      string    `json:"page_url"`
	FileSize     string    `json:"file_size"`
	Duration     string    `json:"duration"`
	PlayCount    string    `json:"play_count"`
	LikeCount    string    `json:"like_count"`
	CommentCount string    `json:"comment_count"`
	FavCount     string    `json:"fav_count"`
	ForwardCount string    `json:"forward_count"`
	CreateTime   string    `json:"create_time"`
	IPRegion     string    `json:"ip_region"`
	DownloadAt   time.Time `json:"download_at"`
}

// FromMap 从map创建VideoProfile
func (v *VideoProfile) FromMap(data map[string]interface{}) {
	if id, ok := data["id"].(string); ok {
		v.ID = id
	}
	if title, ok := data["title"].(string); ok {
		v.Title = title
	}
	if nickname, ok := data["nickname"].(string); ok {
		v.Author = nickname
	}
	// 可以从contact中获取更多信息
	if contact, ok := data["contact"].(map[string]interface{}); ok {
		if nickname, ok := contact["nickname"].(string); ok && v.Author == "" {
			v.Author = nickname
		}
	}

	v.RawData = data
}

// ToDownloadRecord 转换为下载记录
func (v *VideoProfile) ToDownloadRecord(pageURL string) *VideoDownloadRecord {
	return &VideoDownloadRecord{
		ID:           v.ID,
		Title:        v.Title,
		Author:       v.Author,
		AuthorType:   v.AuthorType,
		OfficialName: v.OfficialName,
		URL:          v.URL,
		PageURL:      pageURL,
		FileSize:     v.FileSize,
		Duration:     v.Duration,
		PlayCount:    v.PlayCount,
		LikeCount:    v.LikeCount,
		CommentCount: v.CommentCount,
		FavCount:     v.FavCount,
		ForwardCount: v.ForwardCount,
		CreateTime:   v.CreateTime,
		IPRegion:     v.IPRegion,
		DownloadAt:   time.Now(),
	}
}

// ToCSVRow 转换为CSV行
func (v *VideoDownloadRecord) ToCSVRow() []string {
	return []string{
		"ID_" + v.ID,
		v.Title,
		v.Author,
		v.AuthorType,
		v.OfficialName,
		v.URL,
		v.PageURL,
		v.FileSize,
		v.Duration,
		v.PlayCount,
		v.LikeCount,
		v.CommentCount,
		v.FavCount,
		v.ForwardCount,
		v.CreateTime,
		v.IPRegion,
		v.DownloadAt.Format("2006-01-02 15:04:05"),
	}
}

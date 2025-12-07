package models

import (
	"testing"
	"time"
)

func TestVideoProfile_FromMap(t *testing.T) {
	profile := &VideoProfile{}
	
	data := map[string]interface{}{
		"id":      "123",
		"title":   "测试视频",
		"nickname": "测试作者",
		"contact": map[string]interface{}{
			"nickname": "联系人昵称",
		},
	}
	
	profile.FromMap(data)
	
	if profile.ID != "123" {
		t.Errorf("FromMap() ID = %v, 期望 %v", profile.ID, "123")
	}
	
	if profile.Title != "测试视频" {
		t.Errorf("FromMap() Title = %v, 期望 %v", profile.Title, "测试视频")
	}
	
	if profile.Author != "测试作者" {
		t.Errorf("FromMap() Author = %v, 期望 %v", profile.Author, "测试作者")
	}
	
	if profile.RawData == nil {
		t.Error("FromMap() RawData 不应该为 nil")
	}
}

func TestVideoProfile_ToDownloadRecord(t *testing.T) {
	profile := &VideoProfile{
		ID:           "123",
		Title:        "测试视频",
		Author:       "测试作者",
		AuthorType:   "视频号",
		OfficialName: "公众号名称",
		URL:          "https://example.com/video.mp4",
		FileSize:     "10.5 MB",
		Duration:     "01:30",
		PlayCount:    "1000",
		LikeCount:    "100",
		CommentCount: "50",
		FavCount:     "25",
		ForwardCount: "10",
		CreateTime:   "2025-01-01 12:00:00",
		IPRegion:     "北京",
	}
	
	pageURL := "https://example.com/page"
	record := profile.ToDownloadRecord(pageURL)
	
	if record.ID != profile.ID {
		t.Errorf("ToDownloadRecord() ID = %v, 期望 %v", record.ID, profile.ID)
	}
	
	if record.Title != profile.Title {
		t.Errorf("ToDownloadRecord() Title = %v, 期望 %v", record.Title, profile.Title)
	}
	
	if record.PageURL != pageURL {
		t.Errorf("ToDownloadRecord() PageURL = %v, 期望 %v", record.PageURL, pageURL)
	}
	
	if record.DownloadAt.IsZero() {
		t.Error("ToDownloadRecord() DownloadAt 不应该为零值")
	}
}

func TestVideoDownloadRecord_ToCSVRow(t *testing.T) {
	record := &VideoDownloadRecord{
		ID:            "123",
		Title:         "测试视频",
		Author:        "测试作者",
		AuthorType:    "视频号",
		OfficialName:  "公众号名称",
		URL:           "https://example.com/video.mp4",
		PageURL:       "https://example.com/page",
		FileSize:      "10.5 MB",
		Duration:      "01:30",
		PlayCount:     "1000",
		LikeCount:     "100",
		CommentCount:  "50",
		FavCount:      "25",
		ForwardCount:  "10",
		CreateTime:    "2025-01-01 12:00:00",
		IPRegion:      "北京",
		DownloadAt:    time.Now(),
		PageSource:    "feed",
		SearchKeyword: "",
	}
	
	row := record.ToCSVRow()
	
	// 检查行数（应该有19个字段，包括PageSource和SearchKeyword）
	expectedFields := 19
	if len(row) != expectedFields {
		t.Errorf("ToCSVRow() 返回 %d 个字段, 期望 %d", len(row), expectedFields)
	}
	
	// 检查ID格式
	if row[0] != "ID_123" {
		t.Errorf("ToCSVRow() ID = %v, 期望 %v", row[0], "ID_123")
	}
	
	// 检查标题
	if row[1] != "测试视频" {
		t.Errorf("ToCSVRow() Title = %v, 期望 %v", row[1], "测试视频")
	}
	
	// 检查下载时间字段（索引16）应该有内容
	if row[16] == "" {
		t.Error("ToCSVRow() 下载时间不应该为空")
	}
	
	// 检查PageSource字段（索引17）
	if row[17] != "feed" {
		t.Errorf("ToCSVRow() PageSource = %v, 期望 %v", row[17], "feed")
	}
	
	// 检查SearchKeyword字段（索引18）
	if row[18] != "" {
		t.Errorf("ToCSVRow() SearchKeyword = %v, 期望空字符串", row[18])
	}
}

func TestVideoProfile_FromMap_EmptyContact(t *testing.T) {
	profile := &VideoProfile{}
	
	data := map[string]interface{}{
		"id":    "123",
		"title": "测试视频",
	}
	
	profile.FromMap(data)
	
	if profile.ID != "123" {
		t.Errorf("FromMap() ID = %v, 期望 %v", profile.ID, "123")
	}
	
	// 测试没有contact字段时的行为
	data2 := map[string]interface{}{
		"id":    "456",
		"title": "另一个视频",
		"nickname": "直接昵称",
	}
	
	profile2 := &VideoProfile{}
	profile2.FromMap(data2)
	
	if profile2.Author != "直接昵称" {
		t.Errorf("FromMap() Author = %v, 期望 %v", profile2.Author, "直接昵称")
	}
}


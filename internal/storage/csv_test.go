package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"wx_channel/internal/models"
)

func TestCSVManager_RecordExists(t *testing.T) {
	// 创建临时文件
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "test_records.csv")
	
	header := []string{"ID", "标题", "作者"}
	manager, err := NewCSVManager(csvPath, header)
	if err != nil {
		t.Fatalf("创建CSVManager失败: %v", err)
	}

	// 测试空ID
	exists, err := manager.RecordExists("")
	if err != nil {
		t.Errorf("RecordExists(\"\") 返回错误: %v", err)
	}
	if exists {
		t.Errorf("RecordExists(\"\") 应该返回false")
	}

	// 测试不存在的记录
	exists, err = manager.RecordExists("test123")
	if err != nil {
		t.Errorf("RecordExists(\"test123\") 返回错误: %v", err)
	}
	if exists {
		t.Errorf("RecordExists(\"test123\") 应该返回false（记录不存在）")
	}

	// 添加一条记录
	record := &models.VideoDownloadRecord{
		ID:         "test123",
		Title:      "测试视频",
		Author:     "测试作者",
		FileSize:   "10.5 MB",
		Duration:   "02:30",
		DownloadAt: time.Now(),
	}

	err = manager.AddRecord(record)
	if err != nil {
		t.Fatalf("AddRecord失败: %v", err)
	}

	// 测试存在的记录
	exists, err = manager.RecordExists("test123")
	if err != nil {
		t.Errorf("RecordExists(\"test123\") 返回错误: %v", err)
	}
	if !exists {
		t.Errorf("RecordExists(\"test123\") 应该返回true（记录已存在）")
	}

	// 测试重复添加
	err = manager.AddRecord(record)
	if err != nil {
		t.Errorf("重复AddRecord不应该返回错误: %v", err)
	}

	// 验证文件内容（应该只有一条记录）
	content, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatalf("读取CSV文件失败: %v", err)
	}

	lines := countLines(string(content))
	expectedLines := 2 // 1行表头 + 1行数据
	if lines != expectedLines {
		t.Errorf("CSV文件应该有%d行，实际有%d行", expectedLines, lines)
		t.Logf("文件内容:\n%s", string(content))
	}
}

func TestCSVManager_DuplicateRecords(t *testing.T) {
	// 创建临时文件
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "test_duplicates.csv")
	
	header := []string{"ID", "标题", "作者"}
	manager, err := NewCSVManager(csvPath, header)
	if err != nil {
		t.Fatalf("创建CSVManager失败: %v", err)
	}

	// 添加相同ID的记录多次
	for i := 0; i < 3; i++ {
		record := &models.VideoDownloadRecord{
			ID:         "duplicate123",
			Title:      "重复测试视频",
			Author:     "测试作者",
			FileSize:   "15.2 MB",
			Duration:   "03:45",
			DownloadAt: time.Now(),
		}

		err = manager.AddRecord(record)
		if err != nil {
			t.Errorf("第%d次AddRecord失败: %v", i+1, err)
		}
	}

	// 验证只有一条记录被保存
	content, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatalf("读取CSV文件失败: %v", err)
	}

	lines := countLines(string(content))
	expectedLines := 2 // 1行表头 + 1行数据
	if lines != expectedLines {
		t.Errorf("CSV文件应该有%d行，实际有%d行", expectedLines, lines)
		t.Logf("文件内容:\n%s", string(content))
	}

	// 验证RecordExists返回true
	exists, err := manager.RecordExists("duplicate123")
	if err != nil {
		t.Errorf("RecordExists返回错误: %v", err)
	}
	if !exists {
		t.Errorf("RecordExists应该返回true")
	}
}

// countLines 计算字符串中的行数
func countLines(s string) int {
	if s == "" {
		return 0
	}
	lines := 0
	for _, c := range s {
		if c == '\n' {
			lines++
		}
	}
	// 如果字符串不以换行符结尾，最后一行也要计算
	if len(s) > 0 && s[len(s)-1] != '\n' {
		lines++
	}
	return lines
}
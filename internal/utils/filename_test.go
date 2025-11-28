package utils

import (
	"strings"
	"testing"
)

func TestCleanFilename_LongTitle(t *testing.T) {
	// 测试超长的中文标题（构造一个超过100字符的标题）
	longTitle := strings.Repeat("这是一个非常长的视频标题", 15) // 15 * 13 = 195 字符
	cleaned := CleanFilename(longTitle)
	
	// 检查长度是否被限制
	runes := []rune(cleaned)
	if len(runes) > 103 { // 100 + "..." (3个字符)
		t.Errorf("文件名长度超过限制: 期望 <= 103, 实际 = %d", len(runes))
	}
	
	// 检查是否添加了省略号
	if !strings.HasSuffix(cleaned, "...") {
		t.Errorf("超长文件名应该以 '...' 结尾")
	}
	
	t.Logf("原始标题长度: %d 字符", len([]rune(longTitle)))
	t.Logf("清理后标题: %s", cleaned)
	t.Logf("清理后长度: %d 字符", len(runes))
}

func TestCleanFilename_NormalTitle(t *testing.T) {
	// 测试正常长度的标题
	normalTitle := "这是一个正常的视频标题"
	cleaned := CleanFilename(normalTitle)
	
	if cleaned != normalTitle {
		t.Errorf("正常标题不应该被修改: 期望 = %s, 实际 = %s", normalTitle, cleaned)
	}
}

func TestCleanFilename_IllegalChars(t *testing.T) {
	// 测试包含非法字符的标题
	illegalTitle := "视频<标题>:测试/文件\\名称|问号?星号*"
	cleaned := CleanFilename(illegalTitle)
	
	// 检查非法字符是否被替换
	illegalChars := `<>:"/\|?*`
	for _, char := range illegalChars {
		if strings.ContainsRune(cleaned, char) {
			t.Errorf("清理后的文件名仍包含非法字符: %c", char)
		}
	}
	
	t.Logf("原始标题: %s", illegalTitle)
	t.Logf("清理后标题: %s", cleaned)
}

func TestCleanFolderName_LongName(t *testing.T) {
	// 测试超长的作者名称（构造一个超过50字符的名称）
	longName := strings.Repeat("这是一个非常长的作者名称", 8) // 8 * 13 = 104 字符
	cleaned := CleanFolderName(longName)
	
	// 检查长度是否被限制
	runes := []rune(cleaned)
	if len(runes) > 53 { // 50 + "..." (3个字符)
		t.Errorf("文件夹名长度超过限制: 期望 <= 53, 实际 = %d", len(runes))
	}
	
	// 检查是否添加了省略号
	if !strings.HasSuffix(cleaned, "...") {
		t.Errorf("超长文件夹名应该以 '...' 结尾")
	}
	
	t.Logf("原始名称长度: %d 字符", len([]rune(longName)))
	t.Logf("清理后名称: %s", cleaned)
	t.Logf("清理后长度: %d 字符", len(runes))
}

func TestCleanFolderName_EmptyName(t *testing.T) {
	// 测试空名称
	cleaned := CleanFolderName("")
	
	if cleaned != "未知作者" {
		t.Errorf("空名称应该返回默认值: 期望 = 未知作者, 实际 = %s", cleaned)
	}
}

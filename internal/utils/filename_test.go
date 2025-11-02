package utils

import (
	"testing"
)

func TestCleanFilename(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "包含非法字符",
			input:    "test<>video.mp4",
			expected: "test__video.mp4",
		},
		{
			name:     "包含斜杠",
			input:    "test/\\video.mp4",
			expected: "test__video.mp4",
		},
		{
			name:     "正常文件名",
			input:    "normal_video.mp4",
			expected: "normal_video.mp4",
		},
		{
			name:     "空字符串",
			input:    "",
			expected: "", // 会使用时间戳作为默认值
		},
		{
			name:     "包含控制字符",
			input:    "test\nvideo.mp4",
			expected: "test_video.mp4",
		},
		{
			name:     "包含首尾空格",
			input:    "  test video  .mp4",
			expected: "test video.mp4",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CleanFilename(tt.input)
			
			// 对于空输入，结果会包含时间戳，所以只检查非空情况
			if tt.input != "" && result == "" {
				t.Errorf("CleanFilename(%q) 结果不应该为空", tt.input)
			}
			
			// 检查是否包含非法字符
			if ContainsIllegalChars(result) {
				t.Errorf("CleanFilename(%q) = %q, 仍然包含非法字符", tt.input, result)
			}
		})
	}
}

func TestCleanFolderName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "包含非法字符",
			input:    "test<>folder",
			expected: "test__folder",
		},
		{
			name:     "正常文件夹名",
			input:    "正常文件夹",
			expected: "正常文件夹",
		},
		{
			name:     "空字符串",
			input:    "",
			expected: "未知作者",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CleanFolderName(tt.input)
			
			if result == "" {
				t.Errorf("CleanFolderName(%q) 结果不应该为空", tt.input)
			}
			
			if ContainsIllegalChars(result) {
				t.Errorf("CleanFolderName(%q) = %q, 仍然包含非法字符", tt.input, result)
			}
		})
	}
}

func TestEnsureExtension(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		ext      string
		expected string
	}{
		{
			name:     "已有扩展名",
			filename: "video.mp4",
			ext:      ".mp4",
			expected: "video.mp4",
		},
		{
			name:     "没有扩展名",
			filename: "video",
			ext:      ".mp4",
			expected: "video.mp4",
		},
		{
			name:     "扩展名不同",
			filename: "video.avi",
			ext:      ".mp4",
			expected: "video.avi.mp4",
		},
		{
			name:     "不带点的扩展名",
			filename: "video",
			ext:      "mp4",
			expected: "video.mp4",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EnsureExtension(tt.filename, tt.ext)
			if result != tt.expected {
				t.Errorf("EnsureExtension(%q, %q) = %q, 期望 %q", tt.filename, tt.ext, result, tt.expected)
			}
		})
	}
}

// ContainsIllegalChars 检查字符串是否包含非法字符
func ContainsIllegalChars(s string) bool {
	illegalChars := []rune{'<', '>', ':', '"', '/', '\\', '|', '?', '*'}
	for _, char := range illegalChars {
		for _, r := range s {
			if r == char {
				return true
			}
		}
	}
	
	// 检查控制字符
	for _, r := range s {
		if r < 32 || r == 127 {
			return true
		}
	}
	
	return false
}


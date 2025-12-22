package utils

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SanitizePath 清理路径，防止路径遍历攻击
func SanitizePath(baseDir, path string) (string, error) {
	// 移除 .. 等危险路径
	cleaned := filepath.Clean(path)

	// 检查是否在允许的目录内
	fullPath, err := filepath.Abs(filepath.Join(baseDir, cleaned))
	if err != nil {
		return "", err
	}

	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return "", err
	}

	if !strings.HasPrefix(fullPath, baseAbs) {
		return "", errors.New("路径不安全：尝试访问基础目录之外")
	}

	return cleaned, nil
}

// EnsureDir 确保目录存在
func EnsureDir(dirPath string) error {
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return err
	}
	return nil
}

// GetBaseDir 获取程序基础目录
func GetBaseDir() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return os.Getwd()
	}
	return filepath.Dir(exePath), nil
}

// ResolveDownloadDir 解析下载目录路径
// 如果是绝对路径，直接使用；如果是相对路径，相对于程序基础目录
func ResolveDownloadDir(downloadDir string) (string, error) {
	// 如果是绝对路径，直接使用
	if filepath.IsAbs(downloadDir) {
		return downloadDir, nil
	}
	
	// 如果是相对路径，相对于程序基础目录
	baseDir, err := GetBaseDir()
	if err != nil {
		return "", err
	}
	
	return filepath.Join(baseDir, downloadDir), nil
}

// GetDownloadsDirFromConfig 从配置获取解析后的下载目录
func GetDownloadsDirFromConfig(cfg interface{}) (string, error) {
	// 使用反射或类型断言来获取DownloadsDir字段
	type ConfigWithDownloadsDir interface {
		GetDownloadsDir() string
	}
	
	if c, ok := cfg.(ConfigWithDownloadsDir); ok {
		return ResolveDownloadDir(c.GetDownloadsDir())
	}
	
	// 如果没有实现接口，尝试直接访问字段
	// 这里需要根据实际的配置结构来调整
	return "", fmt.Errorf("无法从配置获取下载目录")
}

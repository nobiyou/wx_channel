package utils

import (
	"errors"
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

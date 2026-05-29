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

// BuildTempDownloadPath 为目标文件生成稳定且独立的临时下载路径。
func BuildTempDownloadPath(targetPath, hint string) string {
	hint = strings.TrimSpace(hint)
	if hint == "" {
		hint = RandomString(8)
	}
	hint = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_':
			return r
		default:
			return '_'
		}
	}, hint)
	if hint == "" {
		hint = RandomString(8)
	}

	filename := filepath.Base(targetPath)
	return filepath.Join(filepath.Dir(targetPath), fmt.Sprintf("%s.%s.tmp", filename, hint))
}

// MoveFileToAvailablePath 将源文件移动到目标路径；若目标已存在，则自动选择不冲突的新路径。
func MoveFileToAvailablePath(srcPath, desiredPath string) (string, error) {
	if strings.TrimSpace(srcPath) == "" {
		return "", fmt.Errorf("源文件路径为空")
	}
	if strings.TrimSpace(desiredPath) == "" {
		return "", fmt.Errorf("目标文件路径为空")
	}

	if err := EnsureDir(filepath.Dir(desiredPath)); err != nil {
		return "", err
	}

	baseName := filepath.Base(desiredPath)
	dir := filepath.Dir(desiredPath)
	candidate := desiredPath

	for attempt := 0; attempt < 1000; attempt++ {
		if attempt > 0 || pathExists(candidate) {
			candidate = GenerateUniquePath(dir, baseName)
		}

		if err := os.Rename(srcPath, candidate); err == nil {
			return candidate, nil
		} else if pathExists(candidate) {
			continue
		} else {
			return "", err
		}
	}

	return "", fmt.Errorf("无法为文件分配可用路径: %s", desiredPath)
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
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

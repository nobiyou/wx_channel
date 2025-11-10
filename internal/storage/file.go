package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"wx_channel/internal/utils"
)

// FileManager 文件管理器
type FileManager struct {
	baseDir string
}

// NewFileManager 创建文件管理器
func NewFileManager(baseDir string) (*FileManager, error) {
	if err := utils.EnsureDir(baseDir); err != nil {
		return nil, fmt.Errorf("创建基础目录失败: %v", err)
	}

	return &FileManager{
		baseDir: baseDir,
	}, nil
}

// SaveFile 保存文件
func (fm *FileManager) SaveFile(filename, content string) (string, error) {
	filePath := filepath.Join(fm.baseDir, filename)

	file, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("创建文件失败: %v", err)
	}
	defer file.Close()

	// 写入UTF-8 BOM（如果需要）
	if _, err := file.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return "", fmt.Errorf("写入UTF-8 BOM失败: %v", err)
	}

	if _, err := file.WriteString(content); err != nil {
		return "", fmt.Errorf("写入文件内容失败: %v", err)
	}

	return filePath, nil
}

// SaveFileFromReader 从Reader保存文件
func (fm *FileManager) SaveFileFromReader(filename string, reader io.Reader) (string, int64, error) {
	filePath := filepath.Join(fm.baseDir, filename)

	file, err := os.Create(filePath)
	if err != nil {
		return "", 0, fmt.Errorf("创建文件失败: %v", err)
	}
	defer file.Close()

	written, err := io.Copy(file, reader)
	if err != nil {
		return "", 0, fmt.Errorf("写入文件失败: %v", err)
	}

	return filePath, written, nil
}

// EnsureDir 确保目录存在
func (fm *FileManager) EnsureDir(subDir string) (string, error) {
	fullPath := filepath.Join(fm.baseDir, subDir)
	if err := utils.EnsureDir(fullPath); err != nil {
		return "", err
	}
	return fullPath, nil
}

// GetFilePath 获取文件完整路径
func (fm *FileManager) GetFilePath(filename string) string {
	return filepath.Join(fm.baseDir, filename)
}

package storage

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"wx_channel/internal/models"
	"wx_channel/internal/utils"
)

// CSVManager CSV记录管理器
type CSVManager struct {
	filePath string
	mutex    sync.Mutex
	header   []string
}

// NewCSVManager 创建CSV管理器
func NewCSVManager(filePath string, header []string) (*CSVManager, error) {
	manager := &CSVManager{
		filePath: filePath,
		header:   header,
	}

	// 确保目录存在
	dir := filepath.Dir(filePath)
	if err := utils.EnsureDir(dir); err != nil {
		return nil, fmt.Errorf("创建目录失败: %v", err)
	}

	// 如果文件不存在，创建并写入表头
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		if err := manager.initFile(); err != nil {
			return nil, err
		}
	}

	return manager, nil
}

// initFile 初始化CSV文件，写入表头和UTF-8 BOM
func (m *CSVManager) initFile() error {
	file, err := os.Create(m.filePath)
	if err != nil {
		return fmt.Errorf("创建CSV文件失败: %v", err)
	}
	defer file.Close()

	// 写入UTF-8 BOM
	if _, err := file.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return fmt.Errorf("写入UTF-8 BOM失败: %v", err)
	}

	writer := csv.NewWriter(file)
	if err := writer.Write(m.header); err != nil {
		return fmt.Errorf("写入表头失败: %v", err)
	}
	writer.Flush()

	if err := writer.Error(); err != nil {
		return fmt.Errorf("写入表头时出错: %v", err)
	}

	return nil
}

// AddRecord 添加记录
func (m *CSVManager) AddRecord(record *models.VideoDownloadRecord) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 检查是否已存在
	exists, err := m.checkExists(record.ID)
	if err != nil {
		return fmt.Errorf("检查记录失败: %v", err)
	}
	if exists {
		return nil // 记录已存在，不重复添加
	}

	// 打开文件（追加模式）
	file, err := os.OpenFile(m.filePath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("打开CSV文件失败: %v", err)
	}
	defer file.Close()

	// 写入记录
	writer := csv.NewWriter(file)
	row := record.ToCSVRow()
	if err := writer.Write(row); err != nil {
		return fmt.Errorf("写入记录失败: %v", err)
	}
	writer.Flush()

	if err := writer.Error(); err != nil {
		return fmt.Errorf("写入记录时出错: %v", err)
	}

	return nil
}

// checkExists 检查记录是否存在
func (m *CSVManager) checkExists(id string) (bool, error) {
	if _, err := os.Stat(m.filePath); os.IsNotExist(err) {
		return false, nil
	}

	file, err := os.Open(m.filePath)
	if err != nil {
		return false, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	// 跳过标题行
	_, err = reader.Read()
	if err != nil {
		if err == io.EOF {
			return false, nil
		}
		return false, err
	}

	formattedID := "ID_" + id

	// 读取所有记录
	for {
		row, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return false, err
		}

		if len(row) > 0 && row[0] == formattedID {
			return true, nil
		}
	}

	return false, nil
}

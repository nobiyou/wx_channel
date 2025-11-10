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
    seenIDs  map[string]struct{}
}

// NewCSVManager 创建CSV管理器
func NewCSVManager(filePath string, header []string) (*CSVManager, error) {
	manager := &CSVManager{
		filePath: filePath,
		header:   header,
        seenIDs:  make(map[string]struct{}),
	}

	// 确保目录存在
	dir := filepath.Dir(filePath)
	if err := utils.EnsureDir(dir); err != nil {
		return nil, fmt.Errorf("创建目录失败: %v", err)
	}

    // 如果文件不存在，创建并写入表头；若存在则加载索引
    if _, err := os.Stat(filePath); os.IsNotExist(err) {
        if err := manager.initFile(); err != nil {
            return nil, err
        }
    } else {
        if err := manager.loadIndex(); err != nil {
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

    if record.ID == "" {
        return nil
    }

    formattedID := "ID_" + record.ID
    if _, ok := m.seenIDs[formattedID]; ok {
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

    // 更新内存索引
    m.seenIDs[formattedID] = struct{}{}

	return nil
}

// loadIndex 启动时加载 CSV 构建内存索引
func (m *CSVManager) loadIndex() error {
    file, err := os.Open(m.filePath)
    if err != nil {
        return err
    }
    defer file.Close()

    reader := csv.NewReader(file)
    // 跳过标题行
    _, err = reader.Read()
    if err != nil {
        if err == io.EOF {
            return nil
        }
        return err
    }

    for {
        row, err := reader.Read()
        if err != nil {
            if err == io.EOF {
                break
            }
            return err
        }
        if len(row) > 0 {
            m.seenIDs[row[0]] = struct{}{}
        }
    }
    return nil
}

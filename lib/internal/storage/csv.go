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
		// 记录重复跳过
		utils.LogCSVOperation("添加记录", record.ID, record.Title, false, "记录已存在")
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
		utils.LogCSVOperation("添加记录", record.ID, record.Title, false, err.Error())
		return fmt.Errorf("写入记录时出错: %v", err)
	}

	// 更新内存索引
	m.seenIDs[formattedID] = struct{}{}

	// 记录添加成功
	utils.LogCSVOperation("添加记录", record.ID, record.Title, true, "")

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
	// 允许字段数量不一致，跳过格式错误的行
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true

	// 跳过标题行
	_, err = reader.Read()
	if err != nil {
		if err == io.EOF {
			return nil
		}
		// 如果标题行读取失败，尝试重建文件
		return m.rebuildCSVFile()
	}

	lineNum := 1
	for {
		row, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			// 记录错误但继续处理其他行
			fmt.Printf("⚠️ CSV第%d行格式错误，已跳过: %v\n", lineNum+1, err)
			lineNum++
			continue
		}
		if len(row) > 0 {
			m.seenIDs[row[0]] = struct{}{}
		}
		lineNum++
	}
	return nil
}

// rebuildCSVFile 重建CSV文件（当文件损坏时）
func (m *CSVManager) rebuildCSVFile() error {
	// 备份原文件
	backupPath := m.filePath + ".backup"
	if err := os.Rename(m.filePath, backupPath); err != nil {
		// 如果重命名失败，直接删除原文件
		os.Remove(m.filePath)
	}

	// 创建新文件
	err := m.initFile()
	if err != nil {
		utils.LogCSVRebuild(m.filePath, false)
		return err
	}

	utils.LogCSVRebuild(m.filePath, true)
	return nil
}

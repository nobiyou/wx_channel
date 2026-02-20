package models

import (
	"time"
)

// User 用户模型
type User struct {
	ID           uint   `json:"id" gorm:"primaryKey"`
	Email        string `json:"email" gorm:"uniqueIndex;not null"`
	PasswordHash string `json:"-" gorm:"not null"` // 不返回给前端
	Credits      int64  `json:"credits" gorm:"default:0"`
	Role         string `json:"role" gorm:"default:'user'"` // 'user', 'admin'

	// Relations
	Devices      []Node        `json:"devices,omitempty" gorm:"foreignKey:UserID"`
	Transactions []Transaction `json:"-" gorm:"foreignKey:UserID"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Node 客户端节点模型
type Node struct {
	ID       string    `json:"id" gorm:"primaryKey"`
	Hostname string    `json:"hostname"`
	Version  string    `json:"version"`
	IP       string    `json:"ip"`
	Port     int       `json:"port" gorm:"default:2025"` // 客户端 API 端口
	Status   string    `json:"status"` // online, offline
	LastSeen time.Time `json:"last_seen"`

	// Binding Info
	UserID     uint `json:"user_id" gorm:"index"`
	BindStatus bool `json:"bind_status" gorm:"default:false"`

	// Device Management (Phase 2)
	DisplayName         string    `json:"display_name" gorm:"default:''"`                  // 用户自定义设备名称
	HardwareFingerprint string    `json:"hardware_fingerprint,omitempty" gorm:"type:text"` // 硬件指纹 JSON
	FirstSeen           time.Time `json:"first_seen"`                                      // 首次连接时间
	IsLocked            bool      `json:"is_locked" gorm:"default:false"`                  // 是否锁定（防止转移）
	DeviceGroup         string    `json:"device_group" gorm:"default:''"`                  // 设备分组
	SyncAPIURL          string    `json:"sync_api_url" gorm:"default:''"`                  // 同步 API 地址（可选，用于 NAT 穿透）

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Task 任务模型
type Task struct {
	ID     uint   `json:"id" gorm:"primaryKey"`
	Type   string `json:"type"` // search, download, play
	NodeID string `json:"node_id" gorm:"index"`

	// Optional: Requester Info
	UserID uint `json:"user_id" gorm:"index"`

	Payload string `json:"payload"` // JSON string input
	Result  string `json:"result"`  // JSON string output
	Status  string `json:"status"`  // pending, success, failed
	Error   string `json:"error"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Transaction 积分交易记录
type Transaction struct {
	ID          uint   `json:"id" gorm:"primaryKey"`
	UserID      uint   `json:"user_id" gorm:"index"`
	Amount      int64  `json:"amount" gorm:"not null"` // Positive = Earn, Negative = Spend
	Type        string `json:"type" gorm:"not null"`   // mining, search_task, download
	Description string `json:"description"`
	RelatedID   string `json:"related_id"` // Ensure traceability (TaskID or NodeID)

	CreatedAt time.Time `json:"created_at"`
}

// Setting 系统设置
type Setting struct {
	Key   string `json:"key" gorm:"primaryKey"`
	Value string `json:"value"`
}

// --- Helper Types for JSON Payload ---

type SearchPayload struct {
	Keyword string `json:"keyword"`
	Page    int    `json:"page"`
}

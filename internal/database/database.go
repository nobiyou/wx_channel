package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

// DB 是全局数据库实例
var (
	db          *sql.DB
	initialized bool
	initMu      sync.Mutex
)

// Config 包含数据库配置
type Config struct {
	DBPath string
}

// Initialize 初始化数据库连接并运行迁移。
// 如果之前初始化失败，允许重新尝试。
func Initialize(cfg *Config) error {
	initMu.Lock()
	defer initMu.Unlock()

	if initialized {
		return nil
	}

	// 确保目录存在
	dir := filepath.Dir(cfg.DBPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create database directory: %w", err)
	}

	// 打开数据库连接
	var err error
	db, err = sql.Open("sqlite", cfg.DBPath+"?_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// 测试连接
	if err := db.Ping(); err != nil {
		db.Close()
		db = nil
		return fmt.Errorf("failed to ping database: %w", err)
	}

	// 运行迁移
	if err := runMigrations(); err != nil {
		db.Close()
		db = nil
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	initialized = true
	return nil
}

// GetDB 返回数据库实例
func GetDB() *sql.DB {
	return db
}

// Close 关闭数据库连接
func Close() error {
	initMu.Lock()
	defer initMu.Unlock()
	if db != nil {
		err := db.Close()
		db = nil
		initialized = false
		return err
	}
	return nil
}

package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

// DB is the global database instance
var (
	db   *sql.DB
	once sync.Once
)

// Config holds database configuration
type Config struct {
	DBPath string
}

// Initialize initializes the database connection and runs migrations
func Initialize(cfg *Config) error {
	var initErr error
	once.Do(func() {
		// Ensure directory exists
		dir := filepath.Dir(cfg.DBPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			initErr = fmt.Errorf("failed to create database directory: %w", err)
			return
		}

		// Open database connection
		var err error
		db, err = sql.Open("sqlite3", cfg.DBPath+"?_foreign_keys=on&_journal_mode=WAL")
		if err != nil {
			initErr = fmt.Errorf("failed to open database: %w", err)
			return
		}

		// Test connection
		if err := db.Ping(); err != nil {
			initErr = fmt.Errorf("failed to ping database: %w", err)
			return
		}

		// Run migrations
		if err := runMigrations(); err != nil {
			initErr = fmt.Errorf("failed to run migrations: %w", err)
			return
		}
	})
	return initErr
}

// GetDB returns the database instance
func GetDB() *sql.DB {
	return db
}

// Close closes the database connection
func Close() error {
	if db != nil {
		return db.Close()
	}
	return nil
}

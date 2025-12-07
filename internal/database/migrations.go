package database

import (
	"fmt"
)

// Migration represents a database migration
type Migration struct {
	Version     int
	Description string
	Up          string
}

// migrations contains all database migrations in order
var migrations = []Migration{
	{
		Version:     1,
		Description: "Create initial schema with browse_history, download_records, queue, and settings tables",
		Up: `
-- Schema version tracking
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Browse history table (浏览记录)
CREATE TABLE IF NOT EXISTS browse_history (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    author TEXT NOT NULL,
    author_id TEXT,
    duration INTEGER DEFAULT 0,
    size INTEGER DEFAULT 0,
    cover_url TEXT,
    video_url TEXT,
    browse_time DATETIME NOT NULL,
    like_count INTEGER DEFAULT 0,
    comment_count INTEGER DEFAULT 0,
    fav_count INTEGER DEFAULT 0,
    forward_count INTEGER DEFAULT 0,
    page_url TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Index for browse_time sorting (descending)
CREATE INDEX IF NOT EXISTS idx_browse_history_browse_time ON browse_history(browse_time DESC);
-- Index for search by title and author
CREATE INDEX IF NOT EXISTS idx_browse_history_title ON browse_history(title);
CREATE INDEX IF NOT EXISTS idx_browse_history_author ON browse_history(author);

-- Download records table (下载记录)
CREATE TABLE IF NOT EXISTS download_records (
    id TEXT PRIMARY KEY,
    video_id TEXT NOT NULL,
    title TEXT NOT NULL,
    author TEXT NOT NULL,
    duration INTEGER DEFAULT 0,
    file_size INTEGER DEFAULT 0,
    file_path TEXT,
    format TEXT,
    resolution TEXT,
    status TEXT NOT NULL DEFAULT 'pending',
    download_time DATETIME NOT NULL,
    error_message TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Index for download_time sorting
CREATE INDEX IF NOT EXISTS idx_download_records_download_time ON download_records(download_time DESC);
-- Index for status filtering
CREATE INDEX IF NOT EXISTS idx_download_records_status ON download_records(status);
-- Index for date range queries
CREATE INDEX IF NOT EXISTS idx_download_records_date ON download_records(date(download_time));

-- Download queue table (下载队列)
CREATE TABLE IF NOT EXISTS download_queue (
    id TEXT PRIMARY KEY,
    video_id TEXT NOT NULL,
    title TEXT NOT NULL,
    author TEXT NOT NULL,
    video_url TEXT NOT NULL,
    total_size INTEGER DEFAULT 0,
    downloaded_size INTEGER DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'pending',
    priority INTEGER DEFAULT 0,
    added_time DATETIME NOT NULL,
    start_time DATETIME,
    speed INTEGER DEFAULT 0,
    chunk_size INTEGER DEFAULT 10485760,
    chunks_total INTEGER DEFAULT 0,
    chunks_completed INTEGER DEFAULT 0,
    retry_count INTEGER DEFAULT 0,
    error_message TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Index for priority-based sorting
CREATE INDEX IF NOT EXISTS idx_download_queue_priority ON download_queue(priority DESC, added_time ASC);
-- Index for status filtering
CREATE INDEX IF NOT EXISTS idx_download_queue_status ON download_queue(status);

-- Settings table (设置)
CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Insert default settings
INSERT OR IGNORE INTO settings (key, value) VALUES 
    ('download_dir', 'downloads'),
    ('chunk_size', '10485760'),
    ('concurrent_limit', '3'),
    ('auto_cleanup_enabled', 'false'),
    ('auto_cleanup_days', '30'),
    ('max_retries', '3'),
    ('theme', 'light');
`,
	},
	{
		Version:     2,
		Description: "Add decrypt_key column to browse_history and download_queue tables",
		Up: `
-- Add decrypt_key column to browse_history table for encrypted video support
ALTER TABLE browse_history ADD COLUMN decrypt_key TEXT DEFAULT '';

-- Add decrypt_key column to download_queue table for encrypted video support
ALTER TABLE download_queue ADD COLUMN decrypt_key TEXT DEFAULT '';
`,
	},
	{
		Version:     3,
		Description: "Add cover_url column to download_records and download_queue tables",
		Up: `
-- Add cover_url column to download_records table for cover image display
ALTER TABLE download_records ADD COLUMN cover_url TEXT DEFAULT '';

-- Add cover_url column to download_queue table for cover image display
ALTER TABLE download_queue ADD COLUMN cover_url TEXT DEFAULT '';
`,
	},
	{
		Version:     4,
		Description: "Add duration column to download_queue table",
		Up: `
-- Add duration column to download_queue table for video duration
ALTER TABLE download_queue ADD COLUMN duration INTEGER DEFAULT 0;
`,
	},
	{
		Version:     5,
		Description: "Add resolution column to download_queue table",
		Up: `
-- Add resolution column to download_queue table for video resolution
ALTER TABLE download_queue ADD COLUMN resolution TEXT DEFAULT '';
`,
	},
	{
		Version:     6,
		Description: "Add resolution column to browse_history table",
		Up: `
-- Add resolution column to browse_history table for video resolution
ALTER TABLE browse_history ADD COLUMN resolution TEXT DEFAULT '';
`,
	},
	{
		Version:     7,
		Description: "Migrate browse_history to add fav_count and forward_count, remove share_count",
		Up: `
-- Create new table with updated schema
CREATE TABLE IF NOT EXISTS browse_history_new (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    author TEXT NOT NULL,
    author_id TEXT,
    duration INTEGER DEFAULT 0,
    size INTEGER DEFAULT 0,
    resolution TEXT DEFAULT '',
    cover_url TEXT,
    video_url TEXT,
    decrypt_key TEXT,
    browse_time DATETIME NOT NULL,
    like_count INTEGER DEFAULT 0,
    comment_count INTEGER DEFAULT 0,
    fav_count INTEGER DEFAULT 0,
    forward_count INTEGER DEFAULT 0,
    page_url TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Copy data from old table to new table
-- Try to copy with share_count first, if it fails, copy without it
INSERT INTO browse_history_new 
SELECT 
    id, title, author, author_id, duration, size, 
    COALESCE(resolution, '') as resolution,
    cover_url, video_url, 
    COALESCE(decrypt_key, '') as decrypt_key,
    browse_time, like_count, comment_count,
    0 as fav_count,
    0 as forward_count,
    page_url, created_at, updated_at
FROM browse_history;

-- Drop old table
DROP TABLE browse_history;

-- Rename new table to original name
ALTER TABLE browse_history_new RENAME TO browse_history;

-- Recreate indexes
CREATE INDEX IF NOT EXISTS idx_browse_history_browse_time ON browse_history(browse_time DESC);
CREATE INDEX IF NOT EXISTS idx_browse_history_title ON browse_history(title);
CREATE INDEX IF NOT EXISTS idx_browse_history_author ON browse_history(author);
`,
	},
}

// runMigrations executes all pending migrations
func runMigrations() error {
	// Create migrations table if not exists
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Get current version
	var currentVersion int
	err = db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("failed to get current schema version: %w", err)
	}

	// Run pending migrations
	for _, m := range migrations {
		if m.Version > currentVersion {
			// Begin transaction
			tx, err := db.Begin()
			if err != nil {
				return fmt.Errorf("failed to begin transaction for migration %d: %w", m.Version, err)
			}

			// Execute migration
			_, err = tx.Exec(m.Up)
			if err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to execute migration %d (%s): %w", m.Version, m.Description, err)
			}

			// Record migration
			_, err = tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", m.Version)
			if err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to record migration %d: %w", m.Version, err)
			}

			// Commit transaction
			if err := tx.Commit(); err != nil {
				return fmt.Errorf("failed to commit migration %d: %w", m.Version, err)
			}

			fmt.Printf("Applied migration %d: %s\n", m.Version, m.Description)
		}
	}

	return nil
}

// GetSchemaVersion returns the current schema version
func GetSchemaVersion() (int, error) {
	var version int
	err := db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("failed to get schema version: %w", err)
	}
	return version, nil
}

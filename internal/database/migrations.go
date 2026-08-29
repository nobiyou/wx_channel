package database

import (
	"fmt"
)

// Migration 表示数据库迁移
type Migration struct {
	Version     int
	Description string
	Up          string
}

// migrations 按顺序包含所有数据库迁移
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
	{
		Version:     8,
		Description: "Add social stats columns to download_records table",
		Up: `
-- Add social stats columns to download_records table
ALTER TABLE download_records ADD COLUMN like_count INTEGER DEFAULT 0;
ALTER TABLE download_records ADD COLUMN comment_count INTEGER DEFAULT 0;
ALTER TABLE download_records ADD COLUMN forward_count INTEGER DEFAULT 0;
ALTER TABLE download_records ADD COLUMN fav_count INTEGER DEFAULT 0;
`,
	},
	{
		Version:     9,
		Description: "Add file_format column to browse_history table for video format identification",
		Up: `
-- Add file_format column to browse_history table for video format (e.g., xWT128, xWT111)
ALTER TABLE browse_history ADD COLUMN file_format TEXT DEFAULT '';
`,
	},
	{
		Version:     10,
		Description: "Create radar_targets table for competitor monitoring",
		Up: `
-- Radar targets table (24h静默雷达监控目标)
CREATE TABLE IF NOT EXISTS radar_targets (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL,
    author_name TEXT NOT NULL,
    interval_minutes INTEGER DEFAULT 60,
    last_check_time DATETIME,
    status TEXT NOT NULL DEFAULT 'active',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Index for username to prevent duplicates
CREATE UNIQUE INDEX IF NOT EXISTS idx_radar_targets_username ON radar_targets(username);
-- Index for status filtering
CREATE INDEX IF NOT EXISTS idx_radar_targets_status ON radar_targets(status);
`,
	},
	{
		Version:     13,
		Description: "Create radar_logs table for execution details",
		Up: `
CREATE TABLE IF NOT EXISTS radar_logs (
    id TEXT PRIMARY KEY,
    target_id TEXT NOT NULL,
    check_time DATETIME NOT NULL,
    found_videos INTEGER DEFAULT 0,
    new_videos INTEGER DEFAULT 0,
    status TEXT NOT NULL,
    error_message TEXT DEFAULT '',
    FOREIGN KEY(target_id) REFERENCES radar_targets(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_radar_logs_target_id ON radar_logs(target_id);
CREATE INDEX IF NOT EXISTS idx_radar_logs_check_time ON radar_logs(check_time);
`,
	},
	{
		Version:     14,
		Description: "Add video_list column to radar_logs for per-video details",
		Up:          `ALTER TABLE radar_logs ADD COLUMN video_list TEXT DEFAULT '';`,
	},
	{
		Version:     15,
		Description: "Create public-account article catalog, metric snapshots, and sync state",
		Up: `
-- Captured public-account profile metadata. Credentials remain outside the
-- catalog and are never returned by catalog queries or exports.
CREATE TABLE IF NOT EXISTS mp_accounts (
    biz TEXT PRIMARY KEY,
    nickname TEXT NOT NULL DEFAULT '',
    avatar_url TEXT NOT NULL DEFAULT '',
    author_id TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'capture',
    is_effective INTEGER NOT NULL DEFAULT 0,
    account_error TEXT NOT NULL DEFAULT '',
    discovered_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    last_seen_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    last_sync_at INTEGER,
    sync_status TEXT NOT NULL DEFAULT 'never',
    sync_error TEXT NOT NULL DEFAULT '',
    article_count INTEGER NOT NULL DEFAULT 0,
    archived_count INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_mp_accounts_nickname ON mp_accounts(nickname);
CREATE INDEX IF NOT EXISTS idx_mp_accounts_updated_at ON mp_accounts(updated_at DESC, biz DESC);
CREATE INDEX IF NOT EXISTS idx_mp_accounts_sync_status ON mp_accounts(sync_status);

-- One logical article record per stable archive identity.
CREATE TABLE IF NOT EXISTS mp_articles (
    article_key TEXT PRIMARY KEY,
    biz TEXT NOT NULL,
    mid TEXT NOT NULL DEFAULT '',
    idx INTEGER NOT NULL DEFAULT 0,
    file_id INTEGER NOT NULL DEFAULT 0,
    title TEXT NOT NULL DEFAULT '',
    digest TEXT NOT NULL DEFAULT '',
    author TEXT NOT NULL DEFAULT '',
    content_url TEXT NOT NULL DEFAULT '',
    source_url TEXT NOT NULL DEFAULT '',
    cover_url TEXT NOT NULL DEFAULT '',
    publish_time INTEGER,
    is_multi INTEGER NOT NULL DEFAULT 0,
    is_original INTEGER NOT NULL DEFAULT 0,
    is_paid INTEGER NOT NULL DEFAULT 0,
    is_pay_subscribe INTEGER NOT NULL DEFAULT 0,
    item_show_type INTEGER NOT NULL DEFAULT 0,
    source_deleted INTEGER NOT NULL DEFAULT 0,
    first_seen_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    last_seen_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    archive_status TEXT NOT NULL DEFAULT 'not_archived',
    archive_dir TEXT NOT NULL DEFAULT '',
    archive_html TEXT NOT NULL DEFAULT '',
    archive_manifest TEXT NOT NULL DEFAULT '',
    archived_at INTEGER,
    raw_metadata TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    FOREIGN KEY (biz) REFERENCES mp_accounts(biz) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_mp_articles_biz_publish
    ON mp_articles(biz, publish_time DESC, article_key DESC);
CREATE INDEX IF NOT EXISTS idx_mp_articles_biz_seen
    ON mp_articles(biz, last_seen_at DESC, article_key DESC);
CREATE INDEX IF NOT EXISTS idx_mp_articles_archive_status
    ON mp_articles(biz, archive_status, publish_time DESC);
CREATE INDEX IF NOT EXISTS idx_mp_articles_title ON mp_articles(title);

-- Resource metadata points at files, while article HTML and binaries stay on
-- the configured filesystem archive.
CREATE TABLE IF NOT EXISTS mp_article_assets (
    article_key TEXT NOT NULL,
    resource_key TEXT NOT NULL,
    kind TEXT NOT NULL DEFAULT '',
    role TEXT NOT NULL DEFAULT '',
    source_url TEXT NOT NULL DEFAULT '',
    local_path TEXT NOT NULL DEFAULT '',
    sha256 TEXT NOT NULL DEFAULT '',
    size INTEGER,
    status TEXT NOT NULL DEFAULT 'pending',
    error TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    PRIMARY KEY (article_key, resource_key),
    FOREIGN KEY (article_key) REFERENCES mp_articles(article_key) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_mp_article_assets_status
    ON mp_article_assets(status, updated_at DESC);

-- Metrics are append-only observations. Nullable counters preserve the
-- distinction between an upstream zero and an unavailable field.
CREATE TABLE IF NOT EXISTS mp_article_metric_snapshots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    article_key TEXT NOT NULL,
    observed_at INTEGER NOT NULL,
    source TEXT NOT NULL DEFAULT '',
    view_count INTEGER,
    like_count INTEGER,
    comment_count INTEGER,
    share_count INTEGER,
    collect_count INTEGER,
    reward_count INTEGER,
    raw_metadata TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (article_key) REFERENCES mp_articles(article_key) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_mp_metrics_article_observed
    ON mp_article_metric_snapshots(article_key, observed_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_mp_metrics_observed
    ON mp_article_metric_snapshots(observed_at DESC, id DESC);

-- A run is resumable at page boundaries and records partial failures instead
-- of converting an expired credential into an empty successful result.
CREATE TABLE IF NOT EXISTS mp_sync_runs (
    id TEXT PRIMARY KEY,
    biz TEXT NOT NULL,
    mode TEXT NOT NULL DEFAULT 'history',
    status TEXT NOT NULL DEFAULT 'queued',
    offset INTEGER NOT NULL DEFAULT 0,
    next_offset INTEGER NOT NULL DEFAULT 0,
    page_size INTEGER NOT NULL DEFAULT 10,
    page_count INTEGER NOT NULL DEFAULT 0,
    fetched INTEGER NOT NULL DEFAULT 0,
    inserted INTEGER NOT NULL DEFAULT 0,
    updated INTEGER NOT NULL DEFAULT 0,
    can_continue INTEGER NOT NULL DEFAULT 1,
    started_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    finished_at INTEGER,
    error TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (biz) REFERENCES mp_accounts(biz) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_mp_sync_runs_biz_started
    ON mp_sync_runs(biz, started_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_mp_sync_runs_status
    ON mp_sync_runs(status, started_at DESC);
`,
	},
	{
		Version:     16,
		Description: "Make public-account metric snapshot imports idempotent",
		Up: `
-- A deterministic fingerprint prevents importing the same observation twice
-- while preserving append-only history for observations with changed values.
ALTER TABLE mp_article_metric_snapshots ADD COLUMN snapshot_hash TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS uq_mp_article_metric_snapshot_hash
    ON mp_article_metric_snapshots(snapshot_hash)
    WHERE snapshot_hash <> '';
`,
	},
	{
		Version:     17,
		Description: "Add resumable public-account metric collection state and runs",
		Up: `
-- One latest collection state per article. Historical observations stay in
-- mp_article_metric_snapshots and are never overwritten by a retry.
CREATE TABLE IF NOT EXISTS mp_article_metric_states (
    article_key TEXT PRIMARY KEY,
    status TEXT NOT NULL DEFAULT 'pending',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    success_count INTEGER NOT NULL DEFAULT 0,
    unknown_count INTEGER NOT NULL DEFAULT 0,
    failure_count INTEGER NOT NULL DEFAULT 0,
    last_attempt_at INTEGER,
    last_success_at INTEGER,
    last_observed_at INTEGER,
    next_retry_at INTEGER,
    last_source TEXT NOT NULL DEFAULT '',
    last_error TEXT NOT NULL DEFAULT '',
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    FOREIGN KEY (article_key) REFERENCES mp_articles(article_key) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_mp_metric_states_retry
    ON mp_article_metric_states(status, next_retry_at, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_mp_metric_states_updated
    ON mp_article_metric_states(updated_at DESC, article_key ASC);

-- Independent metric runs keep article-history pagination and interaction
-- collection resumable without mixing their counters or cancellation state.
CREATE TABLE IF NOT EXISTS mp_metric_sync_runs (
    id TEXT PRIMARY KEY,
    biz TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued',
    force INTEGER NOT NULL DEFAULT 0,
    total INTEGER NOT NULL DEFAULT 0,
    attempted INTEGER NOT NULL DEFAULT 0,
    stored INTEGER NOT NULL DEFAULT 0,
    unknown INTEGER NOT NULL DEFAULT 0,
    failed INTEGER NOT NULL DEFAULT 0,
    started_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    finished_at INTEGER,
    error TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (biz) REFERENCES mp_accounts(biz) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_mp_metric_sync_runs_biz_started
    ON mp_metric_sync_runs(biz, started_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_mp_metric_sync_runs_status
    ON mp_metric_sync_runs(status, started_at DESC);
`,
	},
	{
		Version:     18,
		Description: "Add durable cursor fields to public-account metric sync runs",
		Up: `
-- Metric collection uses a keyset cursor so force-refresh jobs resume without
-- replaying the already checkpointed prefix after a process restart.
ALTER TABLE mp_metric_sync_runs ADD COLUMN after_publish_time INTEGER NOT NULL DEFAULT 0;
ALTER TABLE mp_metric_sync_runs ADD COLUMN after_article_key TEXT NOT NULL DEFAULT '';
`,
	},
	{
		Version:     19,
		Description: "Add public-account media metadata to article catalog",
		Up: `
ALTER TABLE mp_articles ADD COLUMN subtype INTEGER NOT NULL DEFAULT 0;
ALTER TABLE mp_articles ADD COLUMN copyright_stat INTEGER NOT NULL DEFAULT 0;
ALTER TABLE mp_articles ADD COLUMN duration INTEGER NOT NULL DEFAULT 0;
ALTER TABLE mp_articles ADD COLUMN audio_fileid INTEGER NOT NULL DEFAULT 0;
ALTER TABLE mp_articles ADD COLUMN play_url TEXT NOT NULL DEFAULT '';
ALTER TABLE mp_articles ADD COLUMN malicious_title_reason_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE mp_articles ADD COLUMN malicious_content_type INTEGER NOT NULL DEFAULT 0;
`,
	},
	{
		Version:     20,
		Description: "Add public-account video identity to article catalog",
		Up: `
ALTER TABLE mp_articles ADD COLUMN video_id TEXT NOT NULL DEFAULT '';
`,
	},
}

// runMigrations 执行所有待处理的迁移
func runMigrations() error {
	// 如果不存在则创建迁移表
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// 获取当前版本
	var currentVersion int
	err = db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("failed to get current schema version: %w", err)
	}

	// 运行待处理的迁移
	for _, m := range migrations {
		if m.Version > currentVersion {
			// 开启事务
			tx, err := db.Begin()
			if err != nil {
				return fmt.Errorf("failed to begin transaction for migration %d: %w", m.Version, err)
			}

			// 执行迁移
			_, err = tx.Exec(m.Up)
			if err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to execute migration %d (%s): %w", m.Version, m.Description, err)
			}

			// 记录迁移
			_, err = tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", m.Version)
			if err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to record migration %d: %w", m.Version, err)
			}

			// 提交事务
			if err := tx.Commit(); err != nil {
				return fmt.Errorf("failed to commit migration %d: %w", m.Version, err)
			}

			fmt.Printf("Applied migration %d: %s\n", m.Version, m.Description)
		}
	}

	return nil
}

// GetSchemaVersion 返回当前架构版本
func GetSchemaVersion() (int, error) {
	var version int
	err := db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("failed to get schema version: %w", err)
	}
	return version, nil
}

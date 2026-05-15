-- SQLite 性能优化脚本
-- 适用于 260MB+ 的数据库

-- 1. 启用 WAL 模式（Write-Ahead Logging）
-- 优点：提高并发性能，读写不互相阻塞
PRAGMA journal_mode=WAL;

-- 2. 设置缓存大小（默认 2MB，建议设置为 64MB）
-- 负数表示 KB，-64000 = 64MB
PRAGMA cache_size=-64000;

-- 3. 设置临时存储在内存中
PRAGMA temp_store=MEMORY;

-- 4. 设置同步模式（NORMAL 比 FULL 快，但仍然安全）
PRAGMA synchronous=NORMAL;

-- 5. 设置 mmap 大小（内存映射，加速读取）
-- 256MB 的内存映射
PRAGMA mmap_size=268435456;

-- 6. 分析数据库，更新统计信息
ANALYZE;

-- 7. 优化数据库（清理碎片，重建索引）
VACUUM;

-- 8. 创建额外的索引以优化常用查询

-- 浏览记录：按浏览时间查询
CREATE INDEX IF NOT EXISTS idx_browse_time 
ON hub_browse_history(machine_id, browse_time DESC);

-- 浏览记录：按同步时间查询
CREATE INDEX IF NOT EXISTS idx_browse_synced 
ON hub_browse_history(machine_id, synced_at DESC);

-- 下载记录：按下载时间查询
CREATE INDEX IF NOT EXISTS idx_download_time 
ON hub_download_records(machine_id, download_time DESC);

-- 下载记录：按同步时间查询
CREATE INDEX IF NOT EXISTS idx_download_synced 
ON hub_download_records(machine_id, synced_at DESC);

-- 同步历史：按时间查询
CREATE INDEX IF NOT EXISTS idx_sync_history_time 
ON sync_history(machine_id, sync_time DESC);

-- 9. 显示优化结果
SELECT '=== 优化完成 ===' as status;
PRAGMA journal_mode;
PRAGMA cache_size;
PRAGMA synchronous;
PRAGMA mmap_size;

-- 10. 显示数据库大小
SELECT 
    page_count * page_size / 1024.0 / 1024.0 as size_mb,
    page_count,
    page_size
FROM pragma_page_count(), pragma_page_size();

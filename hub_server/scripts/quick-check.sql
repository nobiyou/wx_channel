-- 快速检查同步状态
.mode column
.headers on

-- 同步状态
SELECT 
    machine_id,
    browse_record_count,
    download_record_count,
    last_sync_status,
    datetime(last_browse_sync_time, 'localtime') as last_browse_sync,
    datetime(last_download_sync_time, 'localtime') as last_download_sync,
    datetime(updated_at, 'localtime') as updated_at
FROM sync_status;

-- 最新的浏览记录（检查字段是否完整）
SELECT 
    '=== 最新浏览记录 ===' as info;
SELECT 
    substr(title, 1, 40) as title,
    author,
    duration,
    resolution,
    size,
    datetime(synced_at, 'localtime') as synced_at
FROM hub_browse_history
ORDER BY synced_at DESC
LIMIT 3;

-- 最新的下载记录（检查 file_size 是否有值）
SELECT 
    '=== 最新下载记录 ===' as info;
SELECT 
    substr(title, 1, 40) as title,
    author,
    duration,
    file_size,
    resolution,
    datetime(synced_at, 'localtime') as synced_at
FROM hub_download_records
ORDER BY synced_at DESC
LIMIT 3;

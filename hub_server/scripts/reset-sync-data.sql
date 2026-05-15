-- 重置同步数据，准备重新同步
-- 警告：此操作会删除所有同步的浏览和下载记录！

BEGIN TRANSACTION;

-- 1. 删除所有浏览记录
DELETE FROM hub_browse_history;

-- 2. 删除所有下载记录  
DELETE FROM hub_download_records;

-- 3. 重置同步状态
UPDATE sync_status SET
    browse_record_count = 0,
    download_record_count = 0,
    last_browse_sync_time = '1970-01-01 00:00:00',
    last_download_sync_time = '1970-01-01 00:00:00',
    last_sync_status = 'never',
    last_sync_error = '';

-- 4. 清空同步历史
DELETE FROM sync_history;

COMMIT;

-- 查看结果
SELECT 'Sync data reset completed' as status;
SELECT COUNT(*) as browse_count FROM hub_browse_history;
SELECT COUNT(*) as download_count FROM hub_download_records;
SELECT * FROM sync_status;

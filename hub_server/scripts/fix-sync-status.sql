-- 修复 sync_status 表，根据实际的浏览和下载记录更新统计
UPDATE sync_status 
SET 
    browse_record_count = (
        SELECT COUNT(*) 
        FROM hub_browse_history 
        WHERE hub_browse_history.machine_id = sync_status.machine_id
    ),
    download_record_count = (
        SELECT COUNT(*) 
        FROM hub_download_records 
        WHERE hub_download_records.machine_id = sync_status.machine_id
    ),
    last_browse_sync_time = (
        SELECT MAX(synced_at) 
        FROM hub_browse_history 
        WHERE hub_browse_history.machine_id = sync_status.machine_id
    ),
    last_download_sync_time = (
        SELECT MAX(synced_at) 
        FROM hub_download_records 
        WHERE hub_download_records.machine_id = sync_status.machine_id
    ),
    last_sync_status = 'success',
    last_sync_error = '',
    updated_at = datetime('now')
WHERE machine_id IN (
    SELECT DISTINCT machine_id FROM hub_browse_history
    UNION
    SELECT DISTINCT machine_id FROM hub_download_records
);

-- 查看修复结果
SELECT 
    machine_id,
    browse_record_count,
    download_record_count,
    last_browse_sync_time,
    last_download_sync_time,
    last_sync_status
FROM sync_status;

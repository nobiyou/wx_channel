-- 清理字段不完整的记录，保留完整的记录
-- 这样可以避免重新同步所有数据

BEGIN TRANSACTION;

-- 统计需要清理的记录
SELECT '=== 清理前统计 ===' as info;
SELECT 
    COUNT(*) as total_browse,
    COUNT(CASE WHEN cover_url = '' THEN 1 END) as no_cover
FROM hub_browse_history;

SELECT 
    COUNT(*) as total_download,
    COUNT(CASE WHEN file_size = 0 THEN 1 END) as no_filesize,
    COUNT(CASE WHEN cover_url = '' THEN 1 END) as no_cover
FROM hub_download_records;

-- 删除下载记录中 file_size = 0 的记录（这些是用旧代码同步的）
DELETE FROM hub_download_records WHERE file_size = 0;

-- 可选：删除浏览记录中没有封面的记录
-- DELETE FROM hub_browse_history WHERE cover_url = '';

-- 重置同步状态，让客户端重新推送
UPDATE sync_status SET
    browse_record_count = (SELECT COUNT(*) FROM hub_browse_history WHERE machine_id = sync_status.machine_id),
    download_record_count = (SELECT COUNT(*) FROM hub_download_records WHERE machine_id = sync_status.machine_id),
    last_download_sync_time = '1970-01-01 00:00:00';  -- 只重置下载记录的同步时间

COMMIT;

-- 查看结果
SELECT '=== 清理后统计 ===' as info;
SELECT COUNT(*) as browse_count FROM hub_browse_history;
SELECT COUNT(*) as download_count FROM hub_download_records;
SELECT * FROM sync_status;

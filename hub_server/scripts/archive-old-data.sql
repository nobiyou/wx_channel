-- 数据归档脚本 - 清理旧数据以保持数据库性能
-- 警告：此操作会永久删除旧数据，请先备份！

-- 显示清理前的统计
SELECT '=== 清理前统计 ===' as info;
SELECT 
    'hub_browse_history' as table_name,
    COUNT(*) as total_records,
    COUNT(CASE WHEN browse_time < datetime('now', '-6 months') THEN 1 END) as old_records,
    MIN(datetime(browse_time, 'localtime')) as oldest_record,
    MAX(datetime(browse_time, 'localtime')) as newest_record
FROM hub_browse_history
UNION ALL
SELECT 
    'hub_download_records',
    COUNT(*),
    COUNT(CASE WHEN download_time < datetime('now', '-1 year') THEN 1 END),
    MIN(datetime(download_time, 'localtime')),
    MAX(datetime(download_time, 'localtime'))
FROM hub_download_records
UNION ALL
SELECT 
    'sync_history',
    COUNT(*),
    COUNT(CASE WHEN sync_time < datetime('now', '-3 months') THEN 1 END),
    MIN(datetime(sync_time, 'localtime')),
    MAX(datetime(sync_time, 'localtime'))
FROM sync_history;

-- 开始事务
BEGIN TRANSACTION;

-- 1. 删除 6 个月前的浏览记录
DELETE FROM hub_browse_history 
WHERE browse_time < datetime('now', '-6 months');

-- 2. 删除 1 年前的下载记录
DELETE FROM hub_download_records 
WHERE download_time < datetime('now', '-1 year');

-- 3. 删除 3 个月前的同步历史
DELETE FROM sync_history 
WHERE sync_time < datetime('now', '-3 months');

-- 提交事务
COMMIT;

-- 更新统计信息
ANALYZE;

-- 清理碎片
VACUUM;

-- 显示清理后的统计
SELECT '=== 清理后统计 ===' as info;
SELECT 
    'hub_browse_history' as table_name,
    COUNT(*) as remaining_records,
    MIN(datetime(browse_time, 'localtime')) as oldest_record,
    MAX(datetime(browse_time, 'localtime')) as newest_record
FROM hub_browse_history
UNION ALL
SELECT 
    'hub_download_records',
    COUNT(*),
    MIN(datetime(download_time, 'localtime')),
    MAX(datetime(download_time, 'localtime'))
FROM hub_download_records
UNION ALL
SELECT 
    'sync_history',
    COUNT(*),
    MIN(datetime(sync_time, 'localtime')),
    MAX(datetime(sync_time, 'localtime'))
FROM sync_history;

-- 显示数据库大小
SELECT '=== 数据库大小 ===' as info;
SELECT 
    ROUND(page_count * page_size / 1024.0 / 1024.0, 2) as size_mb
FROM pragma_page_count(), pragma_page_size();

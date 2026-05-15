-- 检查实际数据和同步状态的差异
.mode column
.headers on

-- 1. 检查实际的记录数
SELECT '=== 实际记录数 ===' as info;
SELECT 
    'browse' as type,
    COUNT(*) as actual_count
FROM hub_browse_history
WHERE machine_id = 'DEV-UTc_bGO8vT6cIRun'
UNION ALL
SELECT 
    'download' as type,
    COUNT(*) as actual_count
FROM hub_download_records
WHERE machine_id = 'DEV-UTc_bGO8vT6cIRun';

-- 2. 检查sync_status表中的记录数
SELECT '=== sync_status 中的记录数 ===' as info;
SELECT 
    machine_id,
    browse_record_count,
    download_record_count,
    datetime(last_browse_sync_time, 'localtime') as last_browse_sync,
    datetime(last_download_sync_time, 'localtime') as last_download_sync,
    last_sync_status
FROM sync_status
WHERE machine_id = 'DEV-UTc_bGO8vT6cIRun';

-- 3. 检查最新的同步历史
SELECT '=== 最新同步历史 ===' as info;
SELECT 
    sync_type,
    records_synced,
    status,
    datetime(sync_time, 'localtime') as sync_time
FROM sync_history
WHERE machine_id = 'DEV-UTc_bGO8vT6cIRun'
ORDER BY sync_time DESC
LIMIT 5;

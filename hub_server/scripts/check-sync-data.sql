-- 检查同步数据
.mode column
.headers on

-- 1. 检查同步状态
SELECT '=== 同步状态 ===' as info;
SELECT 
    machine_id,
    browse_record_count,
    download_record_count,
    last_sync_status,
    datetime(last_browse_sync_time) as last_browse_sync,
    datetime(last_download_sync_time) as last_download_sync
FROM sync_status;

-- 2. 检查浏览记录数量
SELECT '=== 浏览记录统计 ===' as info;
SELECT 
    machine_id,
    COUNT(*) as total_records,
    COUNT(CASE WHEN duration > 0 THEN 1 END) as has_duration,
    COUNT(CASE WHEN resolution != '' THEN 1 END) as has_resolution,
    COUNT(CASE WHEN size > 0 THEN 1 END) as has_size
FROM hub_browse_history
GROUP BY machine_id;

-- 3. 检查下载记录数量
SELECT '=== 下载记录统计 ===' as info;
SELECT 
    machine_id,
    COUNT(*) as total_records,
    COUNT(CASE WHEN duration > 0 THEN 1 END) as has_duration,
    COUNT(CASE WHEN resolution != '' THEN 1 END) as has_resolution,
    COUNT(CASE WHEN file_size > 0 THEN 1 END) as has_file_size
FROM hub_download_records
GROUP BY machine_id;

-- 4. 查看浏览记录样本（检查字段是否有值）
SELECT '=== 浏览记录样本 ===' as info;
SELECT 
    id,
    substr(title, 1, 30) as title,
    duration,
    resolution,
    size,
    author
FROM hub_browse_history
LIMIT 5;

-- 5. 查看下载记录样本
SELECT '=== 下载记录样本 ===' as info;
SELECT 
    id,
    substr(title, 1, 30) as title,
    duration,
    resolution,
    file_size,
    status
FROM hub_download_records
LIMIT 5;

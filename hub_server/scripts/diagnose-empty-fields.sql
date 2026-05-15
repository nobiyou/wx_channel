-- 诊断空字段问题
.mode column
.headers on

-- 1. 检查浏览记录的字段完整性
SELECT '=== 浏览记录字段统计 ===' as info;
SELECT 
    COUNT(*) as total_records,
    COUNT(CASE WHEN title IS NULL OR title = '' THEN 1 END) as empty_title,
    COUNT(CASE WHEN author IS NULL OR author = '' THEN 1 END) as empty_author,
    COUNT(CASE WHEN duration IS NULL OR duration = 0 THEN 1 END) as empty_duration,
    COUNT(CASE WHEN resolution IS NULL OR resolution = '' THEN 1 END) as empty_resolution,
    COUNT(CASE WHEN size IS NULL OR size = 0 THEN 1 END) as empty_size,
    COUNT(CASE WHEN cover_url IS NULL OR cover_url = '' THEN 1 END) as empty_cover
FROM hub_browse_history;

-- 2. 查看几条完整记录
SELECT '=== 浏览记录样本（完整字段）===' as info;
SELECT 
    id,
    title,
    author,
    duration,
    resolution,
    size,
    cover_url,
    like_count,
    comment_count
FROM hub_browse_history
LIMIT 3;

-- 3. 检查下载记录的字段完整性
SELECT '=== 下载记录字段统计 ===' as info;
SELECT 
    COUNT(*) as total_records,
    COUNT(CASE WHEN title IS NULL OR title = '' THEN 1 END) as empty_title,
    COUNT(CASE WHEN author IS NULL OR author = '' THEN 1 END) as empty_author,
    COUNT(CASE WHEN duration IS NULL OR duration = 0 THEN 1 END) as empty_duration,
    COUNT(CASE WHEN resolution IS NULL OR resolution = '' THEN 1 END) as empty_resolution,
    COUNT(CASE WHEN file_size IS NULL OR file_size = 0 THEN 1 END) as empty_file_size,
    COUNT(CASE WHEN cover_url IS NULL OR cover_url = '' THEN 1 END) as empty_cover
FROM hub_download_records;

-- 4. 查看下载记录样本
SELECT '=== 下载记录样本（完整字段）===' as info;
SELECT 
    id,
    title,
    author,
    duration,
    resolution,
    file_size,
    format,
    status
FROM hub_download_records
LIMIT 3;

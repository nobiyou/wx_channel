-- 检查浏览记录中的分辨率字段

-- 1. 检查有多少记录有分辨率数据
SELECT 
    COUNT(*) as total_records,
    COUNT(CASE WHEN resolution IS NOT NULL AND resolution != '' THEN 1 END) as has_resolution,
    COUNT(CASE WHEN resolution IS NULL OR resolution = '' THEN 1 END) as no_resolution
FROM hub_browse_history;

-- 2. 查看分辨率的分布
SELECT 
    resolution,
    COUNT(*) as count
FROM hub_browse_history
WHERE resolution IS NOT NULL AND resolution != ''
GROUP BY resolution
ORDER BY count DESC;

-- 3. 查看一些样本数据
SELECT 
    id,
    title,
    resolution,
    video_url,
    decrypt_key
FROM hub_browse_history
LIMIT 10;

-- 4. 检查没有分辨率的记录
SELECT 
    id,
    title,
    resolution,
    video_url
FROM hub_browse_history
WHERE resolution IS NULL OR resolution = ''
LIMIT 5;

-- 5. 检查分辨率格式
SELECT 
    resolution,
    CASE 
        WHEN resolution LIKE '%1920%' OR resolution LIKE '%1080%' THEN '1080p'
        WHEN resolution LIKE '%1280%' OR resolution LIKE '%720%' THEN '720p'
        WHEN resolution LIKE '%640%' OR resolution LIKE '%480%' THEN '480p'
        ELSE 'other'
    END as quality,
    COUNT(*) as count
FROM hub_browse_history
WHERE resolution IS NOT NULL AND resolution != ''
GROUP BY resolution, quality
ORDER BY count DESC;

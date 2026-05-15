-- 检查 hub_browse_history 表结构
PRAGMA table_info(hub_browse_history);

-- 检查是否有 file_format 列
SELECT COUNT(*) as has_file_format_column 
FROM pragma_table_info('hub_browse_history') 
WHERE name = 'file_format';

-- 查看最近的浏览记录，包括 file_format 字段
SELECT 
    id,
    title,
    author,
    resolution,
    file_format,
    browse_time,
    synced_at
FROM hub_browse_history 
ORDER BY browse_time DESC 
LIMIT 20;

-- 统计有 file_format 的记录数量
SELECT 
    COUNT(*) as total_records,
    COUNT(CASE WHEN file_format != '' THEN 1 END) as records_with_format,
    COUNT(CASE WHEN file_format = '' OR file_format IS NULL THEN 1 END) as records_without_format
FROM hub_browse_history;

-- 查看不同的 file_format 值
SELECT 
    file_format,
    COUNT(*) as count
FROM hub_browse_history
WHERE file_format != '' AND file_format IS NOT NULL
GROUP BY file_format
ORDER BY count DESC;

-- 诊断数据库大小问题

-- 1. 数据库总体信息
SELECT '=== 数据库总体信息 ===' as info;
SELECT 
    CAST(page_count * page_size AS REAL) / 1024.0 / 1024.0 as size_mb,
    page_count,
    page_size,
    page_count * page_size as total_bytes
FROM pragma_page_count(), pragma_page_size();

-- 2. 所有表列表
SELECT '=== 所有表列表 ===' as info;
SELECT name, type, sql 
FROM sqlite_master 
WHERE type IN ('table', 'index')
ORDER BY type, name;

-- 3. 各表记录数
SELECT '=== 各表记录数 ===' as info;
SELECT 'users' as table_name, COUNT(*) as count FROM users
UNION ALL SELECT 'devices', COUNT(*) FROM devices
UNION ALL SELECT 'subscriptions', COUNT(*) FROM subscriptions
UNION ALL SELECT 'subscription_videos', COUNT(*) FROM subscription_videos
UNION ALL SELECT 'tasks', COUNT(*) FROM tasks
UNION ALL SELECT 'transactions', COUNT(*) FROM transactions
UNION ALL SELECT 'hub_browse_history', COUNT(*) FROM hub_browse_history
UNION ALL SELECT 'hub_download_records', COUNT(*) FROM hub_download_records
UNION ALL SELECT 'sync_history', COUNT(*) FROM sync_history
UNION ALL SELECT 'sync_status', COUNT(*) FROM sync_status
ORDER BY count DESC;

-- 4. 检查 subscription_videos 表结构（可能包含大字段）
SELECT '=== subscription_videos 表结构 ===' as info;
SELECT sql FROM sqlite_master WHERE name = 'subscription_videos';

-- 5. 检查是否有 BLOB 字段
SELECT '=== 检查大字段 ===' as info;
SELECT 
    m.name as table_name,
    p.name as column_name,
    p.type as column_type
FROM sqlite_master m
JOIN pragma_table_info(m.name) p
WHERE m.type = 'table' 
  AND (p.type LIKE '%BLOB%' OR p.type LIKE '%TEXT%')
ORDER BY m.name, p.name;

-- 6. 检查索引占用
SELECT '=== 索引列表 ===' as info;
SELECT name, tbl_name, sql 
FROM sqlite_master 
WHERE type = 'index' 
  AND sql IS NOT NULL
ORDER BY tbl_name, name;

-- 7. 数据库完整性检查
SELECT '=== 完整性检查 ===' as info;
PRAGMA integrity_check;

-- 8. 检查未使用的空间
SELECT '=== 空间使用情况 ===' as info;
PRAGMA freelist_count;

-- 9. 检查 subscription_videos 的平均记录大小
SELECT '=== subscription_videos 样本数据 ===' as info;
SELECT 
    id,
    LENGTH(title) as title_len,
    LENGTH(description) as desc_len,
    LENGTH(cover_url) as cover_len,
    LENGTH(video_url) as video_len
FROM subscription_videos 
LIMIT 10;

-- 10. 建议
SELECT '=== 诊断建议 ===' as info;
SELECT 
    CASE 
        WHEN (SELECT COUNT(*) FROM subscription_videos) > 1000 THEN 
            '订阅视频表记录数较多 (' || (SELECT COUNT(*) FROM subscription_videos) || ' 条)，可能是主要占用空间的表'
        WHEN (SELECT CAST(page_count * page_size AS REAL) / 1024.0 / 1024.0 FROM pragma_page_count(), pragma_page_size()) > 100 
             AND (SELECT SUM(cnt) FROM (
                 SELECT COUNT(*) as cnt FROM users
                 UNION ALL SELECT COUNT(*) FROM devices
                 UNION ALL SELECT COUNT(*) FROM hub_browse_history
                 UNION ALL SELECT COUNT(*) FROM hub_download_records
             )) < 1000 THEN
            '数据库大小异常，建议执行 VACUUM 清理碎片'
        ELSE
            '数据库大小正常'
    END as suggestion;

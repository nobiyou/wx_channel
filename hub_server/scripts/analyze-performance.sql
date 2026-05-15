-- 分析数据库性能和索引使用情况
.mode column
.headers on

-- 1. 数据库大小和表统计
SELECT '=== 数据库统计 ===' as info;
SELECT 
    name as table_name,
    (SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND tbl_name=m.name) as index_count
FROM sqlite_master m
WHERE type='table' AND name NOT LIKE 'sqlite_%'
ORDER BY name;

-- 2. 各表的记录数
SELECT '=== 表记录数 ===' as info;
SELECT 'hub_browse_history' as table_name, COUNT(*) as record_count FROM hub_browse_history
UNION ALL
SELECT 'hub_download_records', COUNT(*) FROM hub_download_records
UNION ALL
SELECT 'sync_status', COUNT(*) FROM sync_status
UNION ALL
SELECT 'sync_history', COUNT(*) FROM sync_history
UNION ALL
SELECT 'nodes', COUNT(*) FROM nodes
UNION ALL
SELECT 'users', COUNT(*) FROM users
UNION ALL
SELECT 'subscriptions', COUNT(*) FROM subscriptions
UNION ALL
SELECT 'subscribed_videos', COUNT(*) FROM subscribed_videos;

-- 3. 查看所有索引
SELECT '=== 索引列表 ===' as info;
SELECT 
    m.name as table_name,
    il.name as index_name,
    il.origin as index_type,
    GROUP_CONCAT(ii.name, ', ') as indexed_columns
FROM sqlite_master m
LEFT JOIN pragma_index_list(m.name) il
LEFT JOIN pragma_index_info(il.name) ii
WHERE m.type = 'table' 
  AND m.name IN ('hub_browse_history', 'hub_download_records', 'sync_status', 'sync_history')
GROUP BY m.name, il.name, il.origin
ORDER BY m.name, il.name;

-- 4. 检查是否有缺失的索引（常用查询字段）
SELECT '=== 索引建议 ===' as info;

-- 检查 hub_browse_history 的索引
SELECT 
    'hub_browse_history' as table_name,
    'machine_id' as should_index,
    CASE WHEN EXISTS (
        SELECT 1 FROM pragma_index_list('hub_browse_history') 
        WHERE name LIKE '%machine%'
    ) THEN '✓ 已索引' ELSE '✗ 缺失' END as status
UNION ALL
SELECT 
    'hub_browse_history',
    'synced_at',
    CASE WHEN EXISTS (
        SELECT 1 FROM pragma_index_list('hub_browse_history') 
        WHERE name LIKE '%synced%'
    ) THEN '✓ 已索引' ELSE '✗ 缺失' END
UNION ALL
SELECT 
    'hub_browse_history',
    'browse_time',
    CASE WHEN EXISTS (
        SELECT 1 FROM pragma_index_list('hub_browse_history') 
        WHERE name LIKE '%browse%'
    ) THEN '✓ 已索引' ELSE '✗ 缺失' END;

-- 5. 分析查询性能（EXPLAIN QUERY PLAN）
SELECT '=== 查询计划分析 ===' as info;
EXPLAIN QUERY PLAN
SELECT * FROM hub_browse_history 
WHERE machine_id = 'DEV-UTc_bGO8vT6cIRun' 
ORDER BY browse_time DESC 
LIMIT 20;

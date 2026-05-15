-- 检查数据库中所有表及其大小

-- 1. 列出所有表
SELECT name as table_name 
FROM sqlite_master 
WHERE type='table' 
ORDER BY name;

-- 2. 获取数据库总大小
SELECT 
    CAST(page_count * page_size AS REAL) / 1024.0 / 1024.0 as total_size_mb
FROM pragma_page_count(), pragma_page_size();

-- 3. 获取每个表的详细信息
SELECT 
    m.name as table_name,
    COUNT(*) as record_count
FROM sqlite_master m
LEFT JOIN pragma_table_info(m.name) p
WHERE m.type = 'table'
GROUP BY m.name
ORDER BY m.name;

-- 4. 检查各表的记录数
SELECT 'users' as table_name, COUNT(*) as count FROM users
UNION ALL
SELECT 'devices', COUNT(*) FROM devices
UNION ALL
SELECT 'subscriptions', COUNT(*) FROM subscriptions
UNION ALL
SELECT 'subscription_videos', COUNT(*) FROM subscription_videos
UNION ALL
SELECT 'tasks', COUNT(*) FROM tasks
UNION ALL
SELECT 'transactions', COUNT(*) FROM transactions
UNION ALL
SELECT 'hub_browse_history', COUNT(*) FROM hub_browse_history
UNION ALL
SELECT 'hub_download_records', COUNT(*) FROM hub_download_records
UNION ALL
SELECT 'sync_history', COUNT(*) FROM sync_history
UNION ALL
SELECT 'sync_status', COUNT(*) FROM sync_status;

-- 5. 检查是否有大字段（BLOB）
SELECT 
    name as table_name,
    sql
FROM sqlite_master 
WHERE type='table'
ORDER BY name;

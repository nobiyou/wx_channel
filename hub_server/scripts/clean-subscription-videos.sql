-- 清理订阅视频数据，准备重新更新
-- 执行此脚本后，需要在前端点击"一键更新全部"重新获取订阅视频

-- 1. 查看当前订阅视频数量
SELECT 
    s.wx_nickname as '订阅用户',
    COUNT(sv.id) as '视频数量',
    s.last_fetched_at as '最后更新时间'
FROM subscriptions s
LEFT JOIN subscribed_videos sv ON s.id = sv.subscription_id
GROUP BY s.id
ORDER BY s.last_fetched_at DESC;

-- 2. 检查 URL 长度分布（完整的 URL 通常超过 500 字符）
SELECT 
    CASE 
        WHEN LENGTH(video_url) = 0 THEN '空 URL'
        WHEN LENGTH(video_url) < 200 THEN '< 200 (严重不完整)'
        WHEN LENGTH(video_url) < 500 THEN '200-500 (可能不完整)'
        ELSE '> 500 (完整)'
    END as url_length_range,
    COUNT(*) as count,
    ROUND(COUNT(*) * 100.0 / (SELECT COUNT(*) FROM subscribed_videos), 2) as percentage
FROM subscribed_videos
GROUP BY url_length_range
ORDER BY 
    CASE url_length_range
        WHEN '空 URL' THEN 1
        WHEN '< 200 (严重不完整)' THEN 2
        WHEN '200-500 (可能不完整)' THEN 3
        ELSE 4
    END;

-- 3. 查看示例 URL（前 50 个字符）
SELECT 
    id,
    title,
    SUBSTR(video_url, 1, 50) as url_preview,
    LENGTH(video_url) as url_length,
    LENGTH(decrypt_key) as key_length
FROM subscribed_videos
ORDER BY published_at DESC
LIMIT 10;

-- 4. 删除所有订阅视频（保留订阅关系）
-- 取消注释下面的语句来执行删除
-- DELETE FROM subscribed_videos;

-- 5. 重置订阅的视频计数和最后更新时间
-- 取消注释下面的语句来执行重置
-- UPDATE subscriptions SET video_count = 0, last_fetched_at = NULL;

-- 6. 验证删除结果
-- SELECT COUNT(*) as remaining_videos FROM subscribed_videos;
-- SELECT id, wx_nickname, video_count, last_fetched_at FROM subscriptions;

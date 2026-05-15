# 修复订阅视频 URL 问题

## 问题描述

订阅视频保存的 `video_url` 不完整，只包含 `media.url`，缺少 `urlToken` 和其他重要参数，导致播放时返回 404 错误。

## 对比

### 不完整的 URL（旧数据）
```
https://finder.video.qq.com/251/20302/stodownload?encfilekey=...&hy=SZ&idx=1&m=...&uzid=7a206
```

### 完整的 URL（新数据）
```
https://finder.video.qq.com/251/20302/stodownload?encfilekey=...&hy=SH&idx=1&m=&uzid=1&token=...&basedata=...&sign=...&ctsc=141&web=1&extg=10f0000&svrbypass=...&svrnonce=...
```

缺少的关键参数：
- `token` - 访问令牌
- `basedata` - 基础数据
- `sign` - 签名
- `svrbypass` - 服务器绕过参数
- `svrnonce` - 服务器随机数

## 解决方案

### 1. 修改代码（已完成）

**文件**: `hub_server/controllers/subscription.go`

修改订阅视频保存逻辑，保存完整的 URL（包含 `urlToken`）：

```go
// 构建完整的视频 URL（包含 urlToken）
videoURL := getStringField(firstMedia, "url")
urlToken := getStringField(firstMedia, "urlToken")
if videoURL != "" && urlToken != "" {
    videoURL = videoURL + urlToken
}
```

### 2. 更新现有数据

由于旧的订阅视频数据中的 URL 不完整，有两种处理方式：

#### 方案 A: 删除旧数据，重新更新订阅（推荐）

```sql
-- 查看有多少订阅视频
SELECT COUNT(*) FROM subscribed_videos;

-- 删除所有订阅视频（保留订阅关系）
DELETE FROM subscribed_videos;

-- 重置订阅的视频计数
UPDATE subscriptions SET video_count = 0, last_fetched_at = NULL;
```

然后在前端点击"一键更新全部"按钮，重新获取订阅视频。

#### 方案 B: 标记旧数据为无效

```sql
-- 为旧数据添加标记（URL 长度较短的通常是不完整的）
-- 完整的 URL 通常超过 500 字符
UPDATE subscribed_videos 
SET video_url = '' 
WHERE LENGTH(video_url) < 500;
```

这样前端会自动回退到 API 请求方式。

### 3. 验证修复

重新编译 Hub Server：
```bash
cd hub_server
go build -o hub_server_subscription_fix.exe
```

启动 Hub Server 并测试：
1. 删除旧的订阅视频数据
2. 在前端点击"一键更新全部"
3. 查看日志，确认保存了完整的 URL：
   ```
   [Subscription] Video URL with token: https://finder.video.qq.com/251/20302/stodownload?... (length: 650)
   ```
4. 点击订阅视频播放，应该能正常播放

### 4. 检查数据

```sql
-- 查看订阅视频的 URL 长度分布
SELECT 
    CASE 
        WHEN LENGTH(video_url) < 200 THEN '< 200 (不完整)'
        WHEN LENGTH(video_url) < 500 THEN '200-500 (可能不完整)'
        ELSE '> 500 (完整)'
    END as url_length_range,
    COUNT(*) as count
FROM subscribed_videos
GROUP BY url_length_range;

-- 查看最近的订阅视频
SELECT 
    id,
    title,
    LENGTH(video_url) as url_length,
    LENGTH(decrypt_key) as key_length,
    published_at
FROM subscribed_videos
ORDER BY published_at DESC
LIMIT 10;
```

## 注意事项

1. **URL 时效性**: 微信视频号的 URL 包含时效性的 token，可能会过期
2. **重新更新**: 如果视频无法播放，尝试重新更新订阅
3. **API 回退**: 如果 URL 不完整，系统会自动回退到 API 请求方式

## 相关文件

- `hub_server/controllers/subscription.go` - 订阅视频保存逻辑
- `hub_server/frontend/src/views/SubscriptionVideos.vue` - 订阅视频列表
- `hub_server/frontend/src/views/VideoDetail.vue` - 视频详情页面
- `hub_server/SUBSCRIPTION_VIDEO_PLAY_OPTIMIZATION.md` - 优化文档

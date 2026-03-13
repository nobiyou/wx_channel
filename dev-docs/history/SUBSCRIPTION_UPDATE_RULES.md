# 订阅更新规则说明

## 更新策略

订阅更新采用**智能增量更新**策略：

### 基本规则

1. **获取所有视频**: 从微信 API 获取该作者的所有视频（分页获取，最多 50 页）
2. **检查是否存在**: 对每个视频，检查数据库中是否已存在
3. **智能判断**:
   - 如果视频不存在 → 保存新视频
   - 如果视频已存在且 URL 完整（长度 > 500） → 跳过
   - 如果视频已存在但 URL 不完整（长度 ≤ 500） → 更新视频

### 更新逻辑流程图

```
获取视频列表
    ↓
遍历每个视频
    ↓
检查是否存在？
    ├─ 不存在 → 保存新视频 ✓
    └─ 已存在 → 检查 URL 长度
                ├─ > 500 字符 → 跳过（URL 完整）
                └─ ≤ 500 字符 → 更新视频（修复不完整的 URL）✓
```

## 为什么是 500 字符？

### URL 长度对比

**不完整的 URL**（旧版本保存的）:
```
https://finder.video.qq.com/251/20302/stodownload?encfilekey=...&hy=SZ&idx=1&m=...&uzid=7a206
```
- 长度: 约 150-200 字符
- 缺少: token, basedata, sign, svrbypass, svrnonce 等参数

**完整的 URL**（新版本保存的）:
```
https://finder.video.qq.com/251/20302/stodownload?encfilekey=...&hy=SH&idx=1&m=&uzid=1&token=...&basedata=...&sign=...&ctsc=141&web=1&extg=10f0000&svrbypass=...&svrnonce=...
```
- 长度: 约 500-800 字符
- 包含: 所有必需的验证参数

因此，使用 500 字符作为阈值来判断 URL 是否完整。

## 优点

### 1. 高效
- 不会重复保存已有的完整数据
- 减少不必要的数据库写入

### 2. 自动修复
- 自动检测并更新不完整的 URL
- 无需手动清理旧数据

### 3. 保留历史
- 不会删除旧视频
- 保留原有的创建时间和 ID

### 4. 向后兼容
- 兼容旧版本保存的数据
- 逐步修复历史数据

## 使用场景

### 场景 1: 首次订阅
- 获取所有视频并保存
- 所有视频都是新的

### 场景 2: 定期更新
- 获取所有视频
- 只保存新发布的视频
- 跳过已存在的完整视频

### 场景 3: 修复旧数据
- 获取所有视频
- 检测到旧视频的 URL 不完整
- 自动更新为完整的 URL

### 场景 4: 升级后首次更新
- 代码升级后（修复了 URL 保存逻辑）
- 点击"一键更新全部"
- 自动修复所有不完整的 URL

## 日志输出

### 新视频
```
[Subscription] Page 0: Found 20 videos, total so far: 20
[Subscription] Video URL with token: https://finder.video.qq.com/... (length: 650)
```

### 更新不完整的视频
```
[Subscription] Updating incomplete video: 14836452953554556943 (URL length: 180)
[Subscription] Video URL with token: https://finder.video.qq.com/... (length: 650)
```

### 跳过完整的视频
```
(无日志输出，直接跳过)
```

## 手动操作

### 查看 URL 长度分布
```sql
SELECT 
    CASE 
        WHEN LENGTH(video_url) < 200 THEN '< 200 (不完整)'
        WHEN LENGTH(video_url) < 500 THEN '200-500 (可能不完整)'
        ELSE '> 500 (完整)'
    END as url_length_range,
    COUNT(*) as count
FROM subscribed_videos
GROUP BY url_length_range;
```

### 强制更新所有视频

如果需要强制更新所有视频（不推荐），可以：

```sql
-- 方案 1: 删除所有订阅视频
DELETE FROM subscribed_videos;
UPDATE subscriptions SET video_count = 0, last_fetched_at = NULL;

-- 方案 2: 清空所有视频的 URL（触发自动更新）
UPDATE subscribed_videos SET video_url = '';
```

然后点击"一键更新全部"。

## 性能考虑

### 首次订阅
- 时间: 取决于视频数量（约 1-5 分钟）
- 网络请求: 多次分页请求
- 数据库写入: 所有视频

### 定期更新
- 时间: 较快（只保存新视频）
- 网络请求: 多次分页请求（相同）
- 数据库写入: 只有新视频

### 修复旧数据
- 时间: 中等（更新不完整的视频）
- 网络请求: 多次分页请求（相同）
- 数据库写入: 不完整的视频

## 最佳实践

1. **定期更新**: 每天或每周更新一次订阅
2. **监控日志**: 查看是否有大量视频需要更新
3. **检查数据**: 定期检查 URL 长度分布
4. **升级后**: 代码升级后，点击"一键更新全部"修复旧数据

## 相关文件

- `hub_server/controllers/subscription.go` - 订阅更新逻辑
- `hub_server/scripts/fix-subscription-video-urls.md` - URL 修复指南
- `hub_server/scripts/clean-subscription-videos.sql` - 数据清理脚本
- `hub_server/SUBSCRIPTION_VIDEO_PLAY_OPTIMIZATION.md` - 播放优化文档

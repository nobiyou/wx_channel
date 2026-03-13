# Hub 同步快速入门指南

## 5 分钟快速配置

### 客户端配置（WebSocket 推送模式）

客户端在内网环境下，通过 WebSocket 主动推送数据到 Hub Server，无需公网 IP 或端口转发。

#### 1. 配置客户端

编辑 `config.yaml`：

```yaml
# 启用云端连接
cloud_enabled: true
cloud_hub_url: "ws://your-hub-server.com/ws/client"

# 启用 Hub 同步推送
hub_sync:
  enabled: true
  push_enabled: true      # 启用推送
  push_interval: 5m       # 每 5 分钟推送一次
  push_batch_size: 1000   # 每次最多 1000 条
```

#### 2. 启动客户端

```bash
./wx_channel.exe
```

#### 3. 验证

查看日志，应该看到：
```
✓ Hub 同步推送器已启动 (间隔: 5m0s, 批量: 1000)
✓ 已连接到云端 Hub
[SyncPusher] 推送 X 条浏览记录
```

✅ 完成！数据会自动推送到 Hub Server。

---

## 配置参数说明

### 必需参数

| 参数 | 说明 | 示例 |
|------|------|------|
| `cloud_enabled` | 是否启用云端连接 | `true` |
| `cloud_hub_url` | Hub Server 地址 | `ws://hub.example.com/ws/client` |
| `hub_sync.enabled` | 是否启用同步 | `true` |

### 推送模式参数

| 参数 | 说明 | 默认值 | 推荐值 |
|------|------|--------|--------|
| `push_enabled` | 是否启用推送 | `true` | `true` |
| `push_interval` | 推送间隔 | `5m` | `5m-15m` |
| `push_batch_size` | 批量大小 | `1000` | `500-2000` |

---

## 常见问题

### Q: 如何知道同步是否正常？

A: 查看日志：
- 客户端：`[SyncPusher] 推送 X 条记录`
- Hub Server：`成功同步 X 条记录`

### Q: 推送间隔设置多少合适？

A: 
- 数据量小：`5m-10m`
- 数据量大：`10m-30m`
- 测试环境：`1m`

### Q: 如何提高同步性能？

A:
1. 增加 `push_batch_size`（如 2000）
2. 启用数据压缩：`compression_enabled: true`
3. 调整推送间隔

### Q: 推送失败怎么办？

A: 系统会自动重试：
1. 下次定时任务继续推送
2. 使用增量同步，不会丢失数据

### Q: 如何禁用同步？

A: 设置 `hub_sync.enabled: false` 或 `hub_sync.push_enabled: false`

### Q: 客户端需要公网 IP 吗？

A: 不需要！WebSocket 推送模式专为内网环境设计，客户端只需能访问 Hub Server 即可。

---

## 故障排查

### 问题 1: 无法连接到 Hub

```
云端连接失败: connection refused
```

**解决**:
1. 检查 `cloud_hub_url` 是否正确
2. 检查 Hub Server 是否运行
3. 检查网络连接和防火墙

### 问题 2: 推送失败

```
[SyncPusher] 推送失败: connection closed
```

**解决**:
1. 检查 WebSocket 连接是否正常
2. 查看 Hub Server 日志
3. 尝试重启客户端

### 问题 3: 数据未同步

```
[SyncPusher] 推送 0 条记录
```

**原因**: 没有新数据

**验证**: 添加一些测试数据，等待下次推送

---

## 安全建议

### 生产环境

1. ✅ 使用 WSS 加密连接（`wss://`）
2. ✅ 配置 `cloud_secret` 密钥
3. ✅ 定期更新密钥

### 开发环境

1. 可以使用 WS 非加密连接（`ws://`）
2. 可以不设置 `cloud_secret`（仅测试）

---

## 下一步

- 📖 阅读 [完整实现文档](hub-sync-websocket-push-implementation.md)
- 🧪 查看 [测试指南](hub-sync-websocket-push-testing.md)
- 🔧 了解 [WebSocket 推送方案](hub-sync-websocket-push.md)

---

## 技术支持

遇到问题？
1. 查看日志文件：`logs/wx_channel.log`
2. 检查配置文件：`config.yaml`
3. 参考文档：`docs/` 目录
4. 提交 Issue：GitHub Issues

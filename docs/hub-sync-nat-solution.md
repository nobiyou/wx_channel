# Hub 同步 NAT 穿透解决方案

## 问题描述

当客户端在 NAT 后面或使用动态 IP 时，Hub Server 无法直接通过客户端的公网 IP 访问其 API，导致同步失败：

```
Get "http://172.71.98.150/api/sync/download": context deadline exceeded
```

## 解决方案

### 方案 1: 配置自定义同步 API URL（推荐）

为每个设备配置一个可访问的 API 地址。

#### 1.1 使用内网地址（局域网环境）

如果 Hub Server 和客户端在同一局域网：

```bash
# 通过 API 设置设备的同步 URL
curl -X POST http://localhost:8080/api/device/config \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "device_id": "your-device-id",
    "sync_api_url": "http://192.168.1.100:2025"
  }'
```

#### 1.2 使用端口转发

如果客户端有公网 IP 但端口不同：

```bash
curl -X POST http://localhost:8080/api/device/config \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "device_id": "your-device-id",
    "sync_api_url": "http://your-public-ip:8888"
  }'
```

然后在路由器上配置端口转发：`8888 -> 192.168.1.100:2025`

#### 1.3 使用域名

如果有域名和 DDNS：

```bash
curl -X POST http://localhost:8080/api/device/config \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "device_id": "your-device-id",
    "sync_api_url": "http://your-domain.com:2025"
  }'
```

### 方案 2: 使用反向代理（推荐用于生产环境）

#### 2.1 客户端配置 frp 客户端

安装 frp 客户端：
```bash
# 下载 frp
wget https://github.com/fatedier/frp/releases/download/v0.52.0/frp_0.52.0_windows_amd64.zip

# 配置 frpc.ini
[common]
server_addr = your-frp-server.com
server_port = 7000
token = your-secret-token

[wx_channel_api]
type = tcp
local_ip = 127.0.0.1
local_port = 2025
remote_port = 6000
```

启动 frp 客户端：
```bash
frpc -c frpc.ini
```

#### 2.2 配置设备同步 URL

```bash
curl -X POST http://localhost:8080/api/device/config \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "device_id": "your-device-id",
    "sync_api_url": "http://your-frp-server.com:6000"
  }'
```

### 方案 3: 使用 ngrok（快速测试）

#### 3.1 启动 ngrok

```bash
ngrok http 2025
```

会得到一个公网地址，如：`https://abc123.ngrok.io`

#### 3.2 配置设备

```bash
curl -X POST http://localhost:8080/api/device/config \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "device_id": "your-device-id",
    "sync_api_url": "https://abc123.ngrok.io"
  }'
```

### 方案 4: 配置端口（简单场景）

如果客户端使用非标准端口：

```bash
curl -X POST http://localhost:8080/api/device/config \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "device_id": "your-device-id",
    "port": 8080
  }'
```

## 前端配置界面（待实现）

在设备管理页面添加配置按钮：

```vue
<Dialog v-model:visible="dialogs.syncConfig" header="同步配置">
  <div class="flex flex-col gap-4">
    <div>
      <label>同步 API URL</label>
      <InputText 
        v-model="syncConfig.url" 
        placeholder="http://192.168.1.100:2025"
        class="w-full"
      />
      <small class="text-text-muted">
        留空则使用默认地址（IP:端口）
      </small>
    </div>
    
    <div>
      <label>API 端口</label>
      <InputNumber 
        v-model="syncConfig.port" 
        placeholder="2025"
        class="w-full"
      />
    </div>
    
    <div class="flex justify-end gap-2">
      <Button label="取消" text @click="dialogs.syncConfig = false" />
      <Button label="保存" @click="saveSyncConfig" />
    </div>
  </div>
</Dialog>
```

## 数据库更新

新增字段已自动迁移：
- `port` - API 端口（默认 2025）
- `sync_api_url` - 自定义同步 API URL

## API 文档

### 更新设备配置

**端点**: `POST /api/device/config`

**请求头**:
```
Authorization: Bearer YOUR_TOKEN
Content-Type: application/json
```

**请求体**:
```json
{
  "device_id": "device-001",
  "sync_api_url": "http://192.168.1.100:2025",  // 可选
  "port": 2025                                   // 可选
}
```

**响应**:
```json
{
  "code": 0,
  "message": "Device config updated successfully"
}
```

## 优先级规则

同步服务按以下优先级选择 API 地址：

1. **自定义 URL** (`sync_api_url`) - 最高优先级
2. **IP + 端口** (`ip:port`) - 如果没有自定义 URL
3. **默认端口** (2025) - 如果端口未设置

## 故障排查

### 问题 1: 仍然超时

**检查清单**:
- [ ] 确认客户端正在运行
- [ ] 确认配置的 URL 可以访问
- [ ] 测试 API 是否可达：`curl http://your-url/api/sync/stats`
- [ ] 检查防火墙设置
- [ ] 检查同步令牌配置

### 问题 2: 403 Forbidden

**原因**: IP 不在白名单

**解决**: 在客户端 `config.yaml` 中添加 Hub Server IP：
```yaml
hub_sync:
  allowed_ips:
    - "127.0.0.1"
    - "hub-server-ip"  # 添加这行
```

### 问题 3: 401 Unauthorized

**原因**: 令牌不匹配

**解决**: 确保客户端和 Hub Server 使用相同的令牌：

客户端 `config.yaml`:
```yaml
hub_sync:
  token: "same-token-123"
```

Hub Server `main.go`:
```go
services.InitSyncService(services.SyncConfig{
    Token: "same-token-123",
    // ...
})
```

## 测试步骤

### 1. 配置设备

```bash
# 获取设备列表
curl -H "Authorization: Bearer YOUR_TOKEN" \
  http://localhost:8080/api/device/list

# 配置同步 URL
curl -X POST http://localhost:8080/api/device/config \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "device_id": "your-device-id",
    "sync_api_url": "http://192.168.1.100:2025"
  }'
```

### 2. 测试连接

```bash
# 测试客户端 API
curl -H "X-Sync-Token: your-token" \
  "http://192.168.1.100:2025/api/sync/stats"
```

### 3. 触发同步

```bash
# 手动触发同步
curl -X POST http://localhost:8080/api/sync/trigger \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"machine_id": "your-device-id"}'
```

### 4. 查看日志

```bash
# Hub Server 日志
tail -f logs/hub_server.log | grep SyncService

# 客户端日志
tail -f logs/wx_channel.log | grep sync
```

## 最佳实践

### 生产环境

1. **使用反向代理** (frp/ngrok/cloudflare tunnel)
2. **配置 HTTPS** 保护数据传输
3. **使用域名** 而不是 IP 地址
4. **设置强令牌** 保护 API 安全
5. **限制 IP 白名单** 只允许 Hub Server 访问

### 开发环境

1. **使用内网地址** 简单快速
2. **配置端口转发** 如果需要远程访问
3. **使用 ngrok** 快速测试

## 未来改进

- [ ] 前端配置界面
- [ ] 自动检测可用地址
- [ ] 支持多个备用地址
- [ ] WebSocket 反向连接（客户端主动连接 Hub）
- [ ] 自动 NAT 穿透
- [ ] 健康检查和自动切换

## 相关文档

- [Hub 同步 API 文档](hub-sync-api.md)
- [Hub 同步测试指南](hub-sync-testing-guide.md)
- [Hub 同步实现总结](hub-sync-implementation-summary.md)

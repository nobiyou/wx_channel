# 🚀 监控功能快速启动指南

## 1️⃣ 启动客户端（启用监控）

```bash
wx_channel_metrics.exe
```

看到这条消息表示监控已启动：
```
✓ Prometheus 监控已启动: http://localhost:9090/metrics
```

## 2️⃣ 查看监控数据

### 方式 1: 直接访问 Prometheus 端点
浏览器打开: http://localhost:9090/metrics

### 方式 2: Insight 运营台
Insight 运营台会显示 Edge 节点和页面能力状态；Prometheus 指标仍通过上面的本地端点查看。

## 3️⃣ 监控指标说明

| 指标 | 说明 |
|------|------|
| 连接数 | 当前 WebSocket 连接数 |
| API 调用 | API 调用总次数和成功率 |
| 心跳 | 心跳发送和失败次数 |
| 重连 | 重连尝试和成功次数 |
| 压缩 | 数据压缩前后的大小 |

## 4️⃣ 配置文件

`config.yaml---`:
```yaml
# Prometheus 监控配置
metrics_enabled: true    # 启用监控
metrics_port: 9090       # 监控端口
```

## 5️⃣ 禁用监控

如需禁用，修改配置：
```yaml
metrics_enabled: false
```

---

**详细文档**: `dev-docs/PROMETHEUS_MONITORING_ENABLED.md`

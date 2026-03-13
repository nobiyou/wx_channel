# 微信视频号页面内存溢出问题解决方案

## 问题描述

微信视频号页面显示错误：
```
哎呦，崩溃啦！
显示此网页时出了点问题。
错误代码：Out of Memory
```

## 原因分析

### 1. 注入脚本累积
- 项目注入了多个 JS 脚本（core.js, decrypt.js, download.js, keep_alive.js 等）
- 脚本长时间运行可能存在内存泄漏
- 事件监听器和定时器未正确清理

### 2. 页面保活机制
- 保活脚本每 30 分钟刷新一次
- 如果在刷新前内存已溢出，会导致崩溃
- 需要更频繁的刷新来释放内存

### 3. 视频缓存累积
- 视频缓存监控脚本持续运行
- 浏览大量视频会累积缓存数据
- 微信视频号 SPA 架构导致 DOM 节点累积

### 4. 浏览器限制
- Chrome/Edge 单个标签页内存限制（通常 2-4GB）
- 长时间运行的单页应用容易触发
- 内存较小的设备更容易出现

## 解决方案

### 方案 1: 缩短自动刷新间隔（已实施）

**修改**: `internal/assets/inject/keep_alive.js`

将自动刷新间隔从 30 分钟缩短为 **15 分钟**：

```javascript
const REFRESH_INTERVAL = 15 * 60 * 1000; // 15 分钟
```

**优点**:
- 定期释放内存
- 防止内存累积过多
- 保持页面稳定运行

**缺点**:
- 刷新频率增加
- 可能中断用户操作

### 方案 2: 添加内存监控（可选）

监控内存使用情况，在接近限制时主动刷新：

```javascript
// 检查内存使用情况
if (performance.memory) {
    const usedMemory = performance.memory.usedJSHeapSize;
    const totalMemory = performance.memory.totalJSHeapSize;
    const memoryUsagePercent = (usedMemory / totalMemory) * 100;
    
    // 如果内存使用超过 80%，主动刷新
    if (memoryUsagePercent > 80) {
        console.warn('[内存监控] 内存使用率过高:', memoryUsagePercent.toFixed(2) + '%');
        window.__wx_keep_alive.performRefresh('内存使用率过高');
    }
}
```

### 方案 3: 优化脚本（长期）

1. **清理事件监听器**
   - 确保所有事件监听器在不需要时被移除
   - 使用 `AbortController` 管理事件

2. **减少定时器**
   - 合并多个定时器
   - 增加检查间隔

3. **优化 DOM 操作**
   - 减少不必要的 DOM 查询
   - 使用事件委托

4. **清理缓存数据**
   - 定期清理视频缓存监控数据
   - 限制保存的历史记录数量

### 方案 4: 用户操作建议

1. **定期手动刷新**
   - 每 10-15 分钟手动刷新页面
   - 使用 F5 或 Ctrl+R

2. **关闭不用的标签页**
   - 减少浏览器总内存占用
   - 只保留必要的标签页

3. **使用较新的浏览器**
   - Chrome/Edge 最新版本
   - 内存管理更优化

4. **增加系统内存**
   - 如果经常遇到此问题
   - 考虑升级设备内存

## 实施步骤

### 1. 重新编译客户端

```bash
cd wx_channel
go build -o wx_channel_memory_fix.exe
```

### 2. 启动客户端

```bash
./wx_channel_memory_fix.exe
```

### 3. 验证修复

1. 打开微信视频号页面
2. 查看浏览器控制台日志：
   ```
   [keep_alive.js] 页面保活模块加载完成 v3.3 (自动刷新已启用 - 15分钟间隔)
   [keep_alive.js] 页面将每15分钟自动刷新一次，防止内存溢出
   ```
3. 等待 15 分钟，页面应该自动刷新
4. 查看刷新日志：
   ```
   [页面保活] 🔄 执行刷新: 定期刷新（防止内存溢出）
   ```

### 4. 监控内存使用

在浏览器控制台运行：

```javascript
// 查看当前内存使用
if (performance.memory) {
    console.log('已使用内存:', (performance.memory.usedJSHeapSize / 1024 / 1024).toFixed(2) + ' MB');
    console.log('总内存:', (performance.memory.totalJSHeapSize / 1024 / 1024).toFixed(2) + ' MB');
    console.log('内存限制:', (performance.memory.jsHeapSizeLimit / 1024 / 1024).toFixed(2) + ' MB');
}

// 查看保活统计
window.getKeepAliveStats()
```

## 预期效果

### 修复前
- 页面运行 30+ 分钟后崩溃
- 内存使用持续增长
- 频繁出现 "Out of Memory" 错误

### 修复后
- 页面每 15 分钟自动刷新
- 内存定期释放
- 稳定运行，不再崩溃

## 监控指标

### 正常运行
```
内存使用: 200-500 MB
刷新间隔: 15 分钟
运行时长: 无限制（定期刷新）
```

### 异常情况
```
内存使用: > 1.5 GB
刷新失败: 页面无响应
错误提示: Out of Memory
```

## 故障排查

### 问题 1: 仍然出现内存溢出

**可能原因**:
- 刷新间隔仍然太长
- 浏览器内存限制太低
- 其他标签页占用过多内存

**解决方案**:
- 进一步缩短刷新间隔（10 分钟）
- 关闭其他标签页
- 重启浏览器

### 问题 2: 刷新过于频繁

**可能原因**:
- 刷新间隔设置太短
- 影响用户体验

**解决方案**:
- 适当延长刷新间隔（20 分钟）
- 添加用户提示

### 问题 3: 刷新时丢失数据

**可能原因**:
- 刷新时正在进行操作
- 未保存的数据丢失

**解决方案**:
- 使用 sessionStorage 保存状态
- 刷新前提示用户

## 长期优化建议

1. **代码审查**
   - 检查所有注入脚本的内存使用
   - 修复潜在的内存泄漏

2. **性能监控**
   - 添加内存使用监控
   - 记录内存增长趋势

3. **用户反馈**
   - 收集用户遇到的问题
   - 根据反馈调整策略

4. **浏览器兼容性**
   - 测试不同浏览器的表现
   - 针对性优化

## 相关文件

- `internal/assets/inject/keep_alive.js` - 页面保活脚本
- `internal/handlers/script.go` - 脚本注入逻辑
- `config.yaml` - 配置文件

## 参考资料

- [Chrome Memory Management](https://developer.chrome.com/docs/devtools/memory-problems/)
- [JavaScript Memory Leaks](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Memory_Management)
- [Performance API](https://developer.mozilla.org/en-US/docs/Web/API/Performance_API)

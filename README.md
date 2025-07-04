# 微信视频号下载工具

## 项目介绍

本项目是一个用于下载微信视频号内容的工具，基于[ltaoo的开源项目](https://github.com/ltaoo/wx_channels_download)进行功能扩展和界面优化。在此特别致敬原作者，感谢其开源贡献。

![软件启动画面](assets/zhujiemian.png)

## 主要功能

- **便捷下载**: 在视频详情页自动添加下载按钮
- **多种格式**: 支持多种分辨率和质量的视频下载
- **缓存提示**: 针对长视频提供缓存进度显示
- **数据导出**: 保存下载记录和运营数据为表格
- **界面优化**: 优化了界面显示和用户体验

## 使用指南

### 下载视频

1. 打开微信视频号中的视频详情页
2. 在视频下方操作栏中点击下载按钮（如下图所示）

![视频下载按钮](assets/shipinxiazai.png)

> 注意：如果没有看到下载按钮，请检查「更多」选项中是否有「下载视频」。下载功能仅在视频详情页可用。

### 长视频下载

对于较长的视频，软件提供了缓存进度显示功能：

1. 视频加载过程中会显示缓存进度
   ![视频缓存进度](assets/jindutixing.png)

2. 缓存完成后会有明显提示，此时可以进行下载
   ![缓存成功](assets/huancunwancheng.png)

> 提示：长视频需要完整缓存后才能下载，建议按顺序缓存（不要跳着点进度条）

## 视频格式参数对比

下表展示了不同视频格式的参数对比，可根据需求选择合适的格式：

|    文件名    |   分辨率   |  标识符  | 大小(MB) | 总比特率 | 帧速率 | 音频采样率 | 音频比特率 | 时长  |
| ------------ | ---------- | -------- | -------- | -------- | ------ | ---------- | ---------- | ----- |
| ..._WT112_1024x576.mp4 | 1024x576  | WT112    | 18.07    | 2116 Kbps | 30.000 fps | 44100 Hz   | 128 Kbps   | 71.61 秒 |
| ..._WT113_1024x576.mp4 | 1024x576  | WT113    | 14.13    | 1655 Kbps | 30.000 fps | 44100 Hz   | 128 Kbps   | 71.61 秒 |
| ..._WT114_1024x576.mp4 | 1024x576  | WT114    | 11.08    | 1298 Kbps | 30.000 fps | 44100 Hz   | 128 Kbps   | 71.61 秒 |
| ..._WT157_1024x576.mp4 | 1024x576  | WT157    | 14.37    | 1683 Kbps | 30.000 fps | 44100 Hz   | 128 Kbps   | 71.61 秒 |
| ..._WT158_1024x576.mp4 | 1024x576  | WT158    | 11.68    | 1368 Kbps | 30.000 fps | 44100 Hz   | 128 Kbps   | 71.61 秒 |
| ..._WT159_1024x576.mp4 | 1024x576  | WT159    | 9.44     | 1105 Kbps | 30.000 fps | 44100 Hz   | 128 Kbps   | 71.61 秒 |
| ..._WT111_1280x720.mp4 | 1280x720  | WT111    | 23.39    | 2740 Kbps | 30.000 fps | 44100 Hz   | 128 Kbps   | 71.61 秒 |
| ..._WT156_1280x720.mp4 | 1280x720  | WT156    | 18.44    | 2160 Kbps | 30.000 fps | 44100 Hz   | 128 Kbps   | 71.61 秒 |
| 原始视频              | 1920x1080 | 原始视频 | 130.04   | 15232 Kbps | 60.000 fps | 44100 Hz   | 128 Kbps   | 71.61 秒 |

## 版本更新历史
### 6.23版本
- 增加程序图标，小部件美化
- 优化小功能
  
### 6.9版本
- 增加缓存提醒功能，长视频需要完整缓存才能下载
- 优化缓存进度显示

### 5.25版本
- 增加保存运营数据功能
- 支持导出公众号信息、视频发布IP、点赞、收藏、转发等数据

### 5.19版本
- 增加保存下载视频记录为表格功能

### 5.18版本
- 更改顶部文字"ltaoo v5"，致敬原作者
- 添加下载保存记录表格功能

## 常见问题解答

### 1. 服务启动后视频详情一直在加载，终端无日志
**解决方案**: 在终端中按一次 `Ctrl+C` 即可恢复正常。

### 2. 解密失败，停止下载
**解决方案**: 关闭所有视频页面和窗口，然后重新打开尝试下载。

## 开发指南

### 环境要求
- Go语言环境
- 管理员权限（用于网络请求拦截）

### 运行方式
以管理员身份启动终端，然后执行：
```bash
go run main.go
```

### 打包发布
```bash
# 基本打包
go build -o wx_channel.exe

# 优化体积的打包
go build -ldflags="-s -w" -o wx_channel_mini.exe
```

打包后可以使用 `upx` 压缩工具进一步减小体积：
```bash
upx --best wx_channel.exe
```

## 技术实现

本项目的核心实现基于以下技术：

1. 网络请求拦截获取视频资源
2. 自定义界面元素添加下载按钮
3. 视频流处理和缓存管理
4. 数据分析和导出功能

### 参考项目
- [WechatVideoSniffer2.0](https://github.com/kanadeblisst00/WechatVideoSniffer2.0)
- [wx_channels_download](https://github.com/ltaoo/wx_channels_download)

### 核心依赖
- [SunnyNet](https://github.com/qtgolang/SunnyNet) - 网络请求拦截库

## 赞赏支持

如果本项目对您有所帮助，欢迎请作者喝杯咖啡 ☕️

![赞赏码](assets/zanshang.png)

## 许可证

本项目遵循与原项目相同的开源许可证条款。

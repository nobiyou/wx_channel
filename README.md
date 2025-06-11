# 微信视频号下载工具

在视频下方的操作按钮一栏，会多出一个下载按钮，如下所示

![视频下载按钮](assets/screenshot1.png)

> 如果没有，可以看看「更多」这里是否有「下载视频」按钮。<br> > ![下载按钮2](assets/screenshot10.png)

等待视频开始播放，然后暂停视频，点击下载按扭即可下载视频。下载成功后，会在上方显示已下载的文件，下载文件名最后面会标志该视频质量。

![视频下载成功](assets/screenshot2.png)

默认会下载下拉菜单中第一个质量视频。点开更多，可以下载其他质量的视频，包括原始视频。

![下载不同质量的视频](assets/screenshot13.png)
<br>

不同视频这里显示的选项是不同的，没有找到对 xWT111 具体的说明，属于什么分辨率、尺寸多大等等。
<br/>
经过测试，如果原始视频有 104MB，这里尺寸最大的是 xWT111 为 17MB，最小的是 xWT98 为 7MB。

![不同质量视频尺寸统计](assets/screenshot14.png)

仅供参考。

## 常见问题

1、服务启动了，打开视频详情后一直在加载，而且终端没有日志信息。
<br/>
尝试在终端 `Ctrl+C`，按一次即可。

2、解密失败，停止下载
<br/>
关闭全部视频页面、窗口。重新打开，就可以下载。

## 开发说明

先以 管理员身份 启动终端，然后 `go run main.go` 即可。

## 打包

```bash
go build -o wx_video_download.exe main.go
go build -ldflags="-s -w" -o wx_channels_download_min.exe
go build -ldflags="-s -w" -o wx_channels_download.exe
```

打包后可以使用 `upx` 压缩，体积可以从 17MB 压缩到 5MB。

## 其他

此程序大部分参考自以下项目代码
<br/>
https://github.com/kanadeblisst00/WechatVideoSniffer2.0

此程序的核心实现依赖以下库
<br/>
https://github.com/qtgolang/SunnyNet

## 我的赞赏码

如果我的项目对你有所帮助，可以请我喝杯咖啡 ☕️

[![Sponsors](https://sponsorkit-iota.vercel.app/api/sponsors)](https://sponsorkit-iota.vercel.app/api/sponsors)

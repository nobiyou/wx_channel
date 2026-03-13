package main

import (
	"runtime/debug"
	"wx_channel/cmd"
)

func main() {
	// 针对高吞吐量视频分片下载场景的 GC 调优
	// 让堆增长阈值放大，降低 GC 顿挫频率以换取更高的稳定性
	debug.SetGCPercent(200)

	cmd.Execute()
}

//go:build windows
// +build windows

package NFapi

// NF_EventHandler 传递到dll的结构体 所有字段皆为回调参数指针
type NF_EventHandler struct {
	ThreadStart       uintptr
	ThreadEnd         uintptr
	TcpConnectRequest uintptr
	TcpConnected      uintptr
	TcpClosed         uintptr
	TcpReceive        uintptr
	TcpSend           uintptr
	TcpCanReceive     uintptr
	TcpCanSend        uintptr
	UdpCreated        uintptr
	UdpConnectRequest uintptr
	UdpClosed         uintptr
	UdpReceive        uintptr
	UdpSend           uintptr
	UdpCanReceive     uintptr
	UdpCanSend        uintptr
}

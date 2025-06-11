//go:build windows
// +build windows

package NFapi

import "C"
import (
	"github.com/qtgolang/SunnyNet/public"
	"github.com/qtgolang/SunnyNet/src/nfapi/Driver"
	"unsafe"
)

func init() {
	Driver.Event.Go_threadStart = go_threadStart
	Driver.Event.Go_threadEnd = go_threadEnd
	Driver.Event.Go_tcpConnectRequest = go_tcpConnectRequest
	Driver.Event.Go_tcpConnected = go_tcpConnected
	Driver.Event.Go_tcpClosed = go_tcpClosed
	Driver.Event.Go_tcpReceive = go_tcpReceive
	Driver.Event.Go_tcpSend = go_tcpSend
	Driver.Event.Go_tcpCanReceive = go_tcpCanReceive
	Driver.Event.Go_tcpCanSend = go_tcpCanSend
	Driver.Event.Go_udpCreated = go_udpCreated
	Driver.Event.Go_udpConnectRequest = go_udpConnectRequest
	Driver.Event.Go_udpClosed = go_udpClosed
	Driver.Event.Go_udpReceive = go_udpReceive
	Driver.Event.Go_udpSend = go_udpSend
	Driver.Event.Go_udpCanReceive = go_udpCanReceive
	Driver.Event.Go_udpCanSend = go_udpCanSend
}

func go_threadStart() {
	threadStart()
}

func go_threadEnd() {
	threadEnd()
}

func go_tcpConnectRequest(id uint64, pConnInfo uintptr) {
	if pConnInfo == 0 {
		return
	}
	A := (*NF_TCP_CONN_INFO)(unsafe.Pointer(pConnInfo))
	tcpConnectRequest(id, A)
}

func go_tcpConnected(id uint64, pConnInfo uintptr) {
	if pConnInfo == 0 {
		return
	}
	A := (*NF_TCP_CONN_INFO)(unsafe.Pointer(pConnInfo))
	tcpConnected(id, A)
}

func go_tcpClosed(id uint64, pConnInfo uintptr) {
	if pConnInfo == 0 {
		return
	}
	A := (*NF_TCP_CONN_INFO)(unsafe.Pointer(pConnInfo))
	tcpClosed(id, A)
}

func go_tcpReceive(id uint64, buf *byte, len int32) {
	tcpReceive(id, buf, len)
}

func go_tcpSend(id uint64, buf *byte, len int32) {
	tcpSend(id, buf, len)
}

func go_tcpCanReceive(id uint64) {
	tcpCanReceive(id)
}

func go_tcpCanSend(id uint64) {
	tcpCanSend(id)
}

func go_udpCreated(id uint64, pConnInfo uintptr) {
	if pConnInfo == 0 {
		return
	}
	A := (*NF_UDP_CONN_INFO)(unsafe.Pointer(pConnInfo))
	udpCreated(id, A)
}

func go_udpConnectRequest(id uint64, pConnReq uintptr) {
	if pConnReq == 0 {
		return
	}
	A := (*NF_UDP_CONN_REQUEST)(unsafe.Pointer(pConnReq))
	udpConnectRequest(id, A)
}

func go_udpClosed(id uint64, pConnInfo uintptr) {
	if pConnInfo == 0 {
		return
	}
	A := (*NF_UDP_CONN_INFO)(unsafe.Pointer(pConnInfo))
	udpClosed(id, A)
}

func go_udpReceive(id uint64, remoteAddress uintptr, buf uintptr, length int32, options uintptr) {
	bs := public.CStringToBytes(buf, int(length))
	if remoteAddress == 0 || options == 0 {
		return
	}
	A := (*SockaddrInx)(unsafe.Pointer(remoteAddress))
	B := (*NF_UDP_OPTIONS)(unsafe.Pointer(options))
	udpReceive(id, A, bs, B)
}

func go_udpSend(id uint64, remoteAddress uintptr, buf uintptr, length int32, options uintptr) {
	bs := public.CStringToBytes(buf, int(length))
	if remoteAddress == 0 || options == 0 {
		return
	}
	A := (*SockaddrInx)(unsafe.Pointer(remoteAddress))
	B := (*NF_UDP_OPTIONS)(unsafe.Pointer(options))
	udpSend(id, A, bs, B)
}

func go_udpCanReceive(id uint64) {
	udpCanReceive(id)
}

func go_udpCanSend(id uint64) {
	udpCanSend(id)
}

//******************************************************

func CgoDriverInit(driverName string, InitAddr uintptr) int32 {
	return Driver.CgoDriverInit(driverName, InitAddr)
}

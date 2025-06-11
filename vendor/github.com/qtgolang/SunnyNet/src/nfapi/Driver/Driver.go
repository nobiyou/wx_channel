//go:build windows
// +build windows

package Driver

/*
#include "Driver.h"
*/
import "C"
import (
	"unsafe"
)

type event struct {
	Go_threadStart       func()
	Go_threadEnd         func()
	Go_tcpConnectRequest func(id uint64, pConnInfo uintptr)
	Go_tcpConnected      func(id uint64, pConnInfo uintptr)
	Go_tcpClosed         func(id uint64, pConnInfo uintptr)
	Go_tcpReceive        func(id uint64, buf *byte, len int32)
	Go_tcpSend           func(id uint64, buf *byte, len int32)
	Go_tcpCanReceive     func(id uint64)
	Go_tcpCanSend        func(id uint64)
	Go_udpCreated        func(id uint64, pConnInfo uintptr)
	Go_udpConnectRequest func(id uint64, pConnReq uintptr)
	Go_udpClosed         func(id uint64, pConnInfo uintptr)
	Go_udpReceive        func(id uint64, remoteAddress uintptr, buf uintptr, length int32, options uintptr)
	Go_udpSend           func(id uint64, remoteAddress uintptr, buf uintptr, length int32, options uintptr)
	Go_udpCanReceive     func(id uint64)
	Go_udpCanSend        func(id uint64)
}

var Event = &event{}

//export go_threadStart
func go_threadStart() {
	Event.Go_threadStart()
}

//export go_threadEnd
func go_threadEnd() {
	Event.Go_threadEnd()
}

//export go_tcpConnectRequest
func go_tcpConnectRequest(id C.ulonglong, pConnInfo uintptr) {
	Event.Go_tcpConnectRequest(uint64(id), pConnInfo)
}

//export go_tcpConnected
func go_tcpConnected(id C.ulonglong, pConnInfo uintptr) {
	Event.Go_tcpConnected(uint64(id), pConnInfo)
}

//export go_tcpClosed
func go_tcpClosed(id C.ulonglong, pConnInfo uintptr) {
	Event.Go_tcpClosed(uint64(id), pConnInfo)
}

//export go_tcpReceive
func go_tcpReceive(id C.ulonglong, buf *byte, len C.int) {
	Event.Go_tcpReceive(uint64(id), buf, int32(len))
}

//export go_tcpSend
func go_tcpSend(id C.ulonglong, buf *byte, len C.int) {
	Event.Go_tcpSend(uint64(id), buf, int32(len))
}

//export go_tcpCanReceive
func go_tcpCanReceive(id C.ulonglong) {
	Event.Go_tcpCanReceive(uint64(id))
}

//export go_tcpCanSend
func go_tcpCanSend(id C.ulonglong) {
	Event.Go_tcpCanSend(uint64(id))
}

//export go_udpCreated
func go_udpCreated(id C.ulonglong, pConnInfo uintptr) {
	Event.Go_udpCreated(uint64(id), pConnInfo)
}

//export go_udpConnectRequest
func go_udpConnectRequest(id C.ulonglong, pConnReq uintptr) {
	Event.Go_udpConnectRequest(uint64(id), pConnReq)
}

//export go_udpClosed
func go_udpClosed(id C.ulonglong, pConnInfo uintptr) {
	Event.Go_udpClosed(uint64(id), pConnInfo)
}

//export go_udpReceive
func go_udpReceive(id C.ENDPOINT_ID, remoteAddress uintptr, buf uintptr, length C.int, options uintptr) {
	Event.Go_udpReceive(uint64(id), remoteAddress, buf, int32(length), options)
}

//export go_udpSend
func go_udpSend(id C.ENDPOINT_ID, remoteAddress uintptr, buf uintptr, length C.int, options uintptr) {
	Event.Go_udpSend(uint64(id), remoteAddress, buf, int32(length), options)
}

//export go_udpCanReceive
func go_udpCanReceive(id C.ulonglong) {
	Event.Go_udpCanReceive(uint64(id))
}

//export go_udpCanSend
func go_udpCanSend(id C.ulonglong) {
	Event.Go_udpCanReceive(uint64(id))
}

//******************************************************

func CgoDriverInit(driverName string, InitAddr uintptr) int32 {
	return int32(C.NfDriverInit(C.CString(driverName), unsafe.Pointer(InitAddr)))
}

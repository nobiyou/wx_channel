//go:build windows
// +build windows

package iphlpapi

/*
#include "c_iphlpapi_tcp.h"
*/
import "C"
import (
	"strconv"
	"unsafe"
)

func init() {
	C.closeTcpConnectionInit()
}

// CloseCurrentSocket  关闭指定进程的所有TCP连接
func CloseCurrentSocket(PID int, ulAf uint) {
	C.closeTcpConnectionByPid(C.ulong(PID), C.ulong(ulAf))
}

// GetTcpInfoPID 用于获取指定 TCP 连接信息的 PID
func GetTcpInfoPID(tcpInfo string, SunnyPort int) string {
	CS := C.CString(tcpInfo)
	n := int(C.getTcpInfoPID(CS, C.int(SunnyPort)))
	C.free(unsafe.Pointer(CS))
	if n < 0 {
		return ""
	}
	return strconv.Itoa(n)
}

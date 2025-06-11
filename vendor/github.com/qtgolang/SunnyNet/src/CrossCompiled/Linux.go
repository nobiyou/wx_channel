//go:build !windows && !darwin
// +build !windows,!darwin

package CrossCompiled

import (
	NFapi "github.com/qtgolang/SunnyNet/src/nfapi"
)

func SetIeProxy(Off bool, Port int) bool {
	return false
}
func NFapi_SunnyPointer(a ...uintptr) uintptr {
	return 0
}
func NFapi_IsInit(a ...bool) bool {
	return false
}
func NFapi_ProcessPortInt(a ...uint16) uint16 {
	return 0
}
func NFapi_ApiInit() bool {
	return false
}
func NFapi_MessageBox(caption, text string, style uintptr) (result int) {
	return 0
}
func NFapi_SetHookProcess(open bool) {
}
func NFapi_ClosePidTCP(pid int) {
}
func NFapi_DelName(u string) {
}
func NFapi_AddName(u string) {
}
func NFapi_DelPid(pid uint32) {
}
func NFapi_AddPid(pid uint32) {
}
func NFapi_CloseNameTCP(u string) {
}
func NFapi_CancelAll() {
}
func NFapi_DelTcpConnectInfo(U uint16) {
}
func NFapi_GetTcpConnectInfo(U uint16) *NFapi.ProcessInfo {
	return nil
}

func NFapi_API_NfTcpClose(U uint64) {
}
func NFapi_UdpSendReceiveFunc(udp func(Type int8, Theoni int64, pid uint32, LocalAddress, RemoteAddress string, data []byte) []byte) func(Type int8, Theoni int64, pid uint32, LocalAddress, RemoteAddress string, data []byte) []byte {
	return nil
}

func NFapi_Api_NfUdpPostSend(id uint64, remoteAddress any, buf []byte, option any) (int32, error) {
	return 0, nil
}

func SetNetworkConnectNumber() {
}

// CloseCurrentSocket  关闭指定进程的所有TCP连接
func CloseCurrentSocket(PID int, ulAf uint) {
}

// GetTcpInfoPID 用于获取指定 TCP 连接信息的 PID
func GetTcpInfoPID(tcpInfo string, SunnyPort int) string {
	return ""
}

// InstallCert 安装证书 将证书安装到Windows系统内
func InstallCert(certificates []byte) string {
	return "no Windows"
}

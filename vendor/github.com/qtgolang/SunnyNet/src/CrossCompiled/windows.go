//go:build windows
// +build windows

package CrossCompiled

import "C"
import (
	"bufio"
	"bytes"
	"github.com/Trisia/gosysproxy"
	"github.com/qtgolang/SunnyNet/public"
	"github.com/qtgolang/SunnyNet/src/iphlpapi"
	NFapi "github.com/qtgolang/SunnyNet/src/nfapi"
	"io"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

func NFapi_SunnyPointer(a ...uintptr) uintptr {
	if len(a) > 0 {
		NFapi.SunnyPointer = a[0]
	}
	return NFapi.SunnyPointer
}
func NFapi_IsInit(a ...bool) bool {
	if len(a) > 0 {
		NFapi.IsInit = a[0]
	}
	return NFapi.IsInit
}
func NFapi_ProcessPortInt(a ...uint16) uint16 {
	if len(a) > 0 {
		NFapi.ProcessPortInt = a[0]
	}
	return NFapi.ProcessPortInt
}
func NFapi_ApiInit() bool {
	return NFapi.ApiInit()
}
func NFapi_MessageBox(caption, text string, style uintptr) (result int) {
	return NFapi.MessageBox(caption, text, style)
}
func NFapi_SetHookProcess(open bool) {
	NFapi.SetHookProcess(open)
}
func NFapi_ClosePidTCP(pid int) {
	NFapi.ClosePidTCP(pid)
}
func NFapi_DelName(u string) {
	NFapi.DelName(u)
}
func NFapi_AddName(u string) {
	NFapi.AddName(u)
}
func NFapi_DelPid(pid uint32) {
	NFapi.DelPid(pid)
}
func NFapi_AddPid(pid uint32) {
	NFapi.AddPid(pid)
}
func NFapi_CloseNameTCP(u string) {
	NFapi.CloseNameTCP(u)
}
func NFapi_CancelAll() {
	NFapi.CancelAll()
}
func NFapi_DelTcpConnectInfo(U uint16) {
	NFapi.DelTcpConnectInfo(U)
}
func NFapi_GetTcpConnectInfo(U uint16) *NFapi.ProcessInfo {
	return NFapi.GetTcpConnectInfo(U)
}

func NFapi_API_NfTcpClose(U uint64) {
	NFapi.Api.NfTcpClose(U)
}
func NFapi_UdpSendReceiveFunc(udp func(Type int8, Theoni int64, pid uint32, LocalAddress, RemoteAddress string, data []byte) []byte) func(Type int8, Theoni int64, pid uint32, LocalAddress, RemoteAddress string, data []byte) []byte {
	NFapi.UdpSendReceiveFunc = udp
	return NFapi.UdpSendReceiveFunc
}
func NFapi_Api_NfUdpPostSend(id uint64, remoteAddress *NFapi.SockaddrInx, buf []byte, option *NFapi.NF_UDP_OPTIONS) (NFapi.NF_STATUS, error) {
	return NFapi.Api.NfUdpPostSend(id, remoteAddress, buf, option)
}

func SetIeProxy(Off bool, Port int) bool {
	// "github.com/Tri sia/gos ysp roxy"
	if Off {
		_ = gosysproxy.Off()
		return true
	}
	ies := "127.0.0.1:" + strconv.Itoa(Port)
	_ = gosysproxy.SetGlobalProxy("http="+ies+";https="+ies, "")
	return true
}

// InstallCert 安装证书 将证书安装到Windows系统内
func InstallCert(certificates []byte) string {
	path, err := os.Getwd()
	if err != nil {
		return err.Error()
	}
	err = public.WriteBytesToFile(certificates, path+"\\ca.crt")
	if err != nil {
		return err.Error()
	}
	var args []string
	args = append(args, "-addstore")
	args = append(args, "root")
	args = append(args, path+"\\ca.crt")
	defer func() { _ = public.RemoveFile(path + "\\ca.crt") }()
	cmd := exec.Command("certutil", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err.Error()
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd.Start()
	var Buff bytes.Buffer
	reader := bufio.NewReader(stdout)
	for {
		line, err2 := reader.ReadBytes('\n')
		if err2 != nil || io.EOF == err2 {
			break
		}
		Buff.Write(line)
	}
	return Buff.String()
}

func SetNetworkConnectNumber() {
	//https://blog.csdn.net/PYJcsdn/article/details/126251054
	//尽量避免这个问题
	var args []string
	args = append(args, "int")
	args = append(args, "ipv4")
	args = append(args, "set")
	args = append(args, "dynamicport")
	args = append(args, "tcp")
	args = append(args, "start=10000")
	args = append(args, "num=55000")
	NFapi.ExecCommand("netsh", args)
	var args1 []string
	args1 = append(args1, "int")
	args1 = append(args1, "ipv6")
	args1 = append(args1, "set")
	args1 = append(args1, "dynamicport")
	args1 = append(args1, "tcp")
	args1 = append(args1, "start=10000")
	args1 = append(args1, "num=55000")
	NFapi.ExecCommand("netsh", args1)
}

// CloseCurrentSocket  关闭指定进程的所有TCP连接
func CloseCurrentSocket(PID int, ulAf uint) {
	iphlpapi.CloseCurrentSocket(PID, ulAf)
}

// GetTcpInfoPID 用于获取指定 TCP 连接信息的 PID
func GetTcpInfoPID(tcpInfo string, SunnyPort int) string {
	return iphlpapi.GetTcpInfoPID(tcpInfo, SunnyPort)
}

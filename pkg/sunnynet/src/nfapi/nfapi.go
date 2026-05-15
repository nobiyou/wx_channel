//go:build windows
// +build windows

package NFapi

import (
	"bytes"
	"errors"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type NF_STATUS int32

const (
	NF_STATUS_SUCCESS             NF_STATUS = 0
	NF_STATUS_FAIL                NF_STATUS = -1
	NF_STATUS_INVALID_ENDPOINT_ID NF_STATUS = -2
	NF_STATUS_NOT_INITIALIZED     NF_STATUS = -3
	NF_STATUS_IO_ERROR            NF_STATUS = -4
	NF_STATUS_REBOOT_REQUIRED     NF_STATUS = -5
	NF_DriverName                           = "SunnyFilter2"
	NF_DLLName                              = "SunnyFilter"
)

type NFApi struct {
	dll                          *windows.LazyDLL
	nf_init                      *windows.LazyProc
	nf_free                      *windows.LazyProc
	nf_registerDriver            *windows.LazyProc
	nf_registerDriverEx          *windows.LazyProc
	nf_unRegisterDriver          *windows.LazyProc
	nf_tcpSetConnectionState     *windows.LazyProc
	nf_tcpPostSend               *windows.LazyProc
	nf_tcpPostReceive            *windows.LazyProc
	nf_tcpClose                  *windows.LazyProc
	nf_setTCPTimeout             *windows.LazyProc
	nf_tcpDisableFiltering       *windows.LazyProc
	nf_udpSetConnectionState     *windows.LazyProc
	nf_udpPostSend               *windows.LazyProc
	nf_udpPostReceive            *windows.LazyProc
	nf_udpDisableFiltering       *windows.LazyProc
	nf_ipPostSend                *windows.LazyProc
	nf_ipPostReceive             *windows.LazyProc
	nf_addRule                   *windows.LazyProc
	nf_deleteRules               *windows.LazyProc
	nf_setRules                  *windows.LazyProc
	nf_addRuleEx                 *windows.LazyProc
	nf_setRulesEx                *windows.LazyProc
	nf_getConnCount              *windows.LazyProc
	nf_tcpSetSockOpt             *windows.LazyProc
	nf_getProcessNameA           *windows.LazyProc
	nf_getProcessNameW           *windows.LazyProc
	nf_getProcessNameFromKernel  *windows.LazyProc
	nf_adjustProcessPriviledges  *windows.LazyProc
	nf_tcpIsProxy                *windows.LazyProc
	nf_setOptions                *windows.LazyProc
	nf_completeTCPConnectRequest *windows.LazyProc
	nf_completeUDPConnectRequest *windows.LazyProc
	nf_getTCPConnInfo            *windows.LazyProc
	nf_getUDPConnInfo            *windows.LazyProc
	nf_setIPEventHandler         *windows.LazyProc
	nf_addFlowCtl                *windows.LazyProc
	nf_deleteFlowCtl             *windows.LazyProc
	nf_setTCPFlowCtl             *windows.LazyProc
	nf_setUDPFlowCtl             *windows.LazyProc
	nf_modifyFlowCtl             *windows.LazyProc
	nf_getFlowCtlStat            *windows.LazyProc
	nf_getTCPStat                *windows.LazyProc
	nf_getUDPStat                *windows.LazyProc
	nf_addBindingRule            *windows.LazyProc
	nf_deleteBindingRules        *windows.LazyProc
	nf_getDriverType             *windows.LazyProc
}

// 读取DLL
func (a *NFApi) Load(dll string) error {
	a.dll = windows.NewLazyDLL(dll)
	e := a.dll.Load()
	if e != nil {
		return e
	}
	a.nf_init = a.dll.NewProc("nf_init")

	a.nf_free = a.dll.NewProc("nf_free")
	a.nf_registerDriver = a.dll.NewProc("nf_registerDriver")
	a.nf_registerDriverEx = a.dll.NewProc("nf_registerDriverEx")
	a.nf_unRegisterDriver = a.dll.NewProc("nf_unRegisterDriver")
	a.nf_tcpSetConnectionState = a.dll.NewProc("nf_tcpSetConnectionState")
	a.nf_tcpPostSend = a.dll.NewProc("nf_tcpPostSend")
	a.nf_tcpPostReceive = a.dll.NewProc("nf_tcpPostReceive")
	a.nf_tcpClose = a.dll.NewProc("nf_tcpClose")
	a.nf_setTCPTimeout = a.dll.NewProc("nf_setTCPTimeout")
	a.nf_tcpDisableFiltering = a.dll.NewProc("nf_tcpDisableFiltering")

	a.nf_udpSetConnectionState = a.dll.NewProc("nf_udpSetConnectionState")
	a.nf_udpPostSend = a.dll.NewProc("nf_udpPostSend")
	a.nf_udpPostReceive = a.dll.NewProc("nf_udpPostReceive")
	a.nf_udpDisableFiltering = a.dll.NewProc("nf_udpDisableFiltering")

	a.nf_ipPostSend = a.dll.NewProc("nf_ipPostSend")
	a.nf_ipPostReceive = a.dll.NewProc("nf_ipPostReceive")

	a.nf_addRule = a.dll.NewProc("nf_addRule")
	a.nf_deleteRules = a.dll.NewProc("nf_deleteRules")
	a.nf_setRules = a.dll.NewProc("nf_setRules")
	a.nf_addRuleEx = a.dll.NewProc("nf_addRuleEx")
	a.nf_setRulesEx = a.dll.NewProc("nf_setRulesEx")

	a.nf_getConnCount = a.dll.NewProc("nf_getConnCount")
	a.nf_tcpSetSockOpt = a.dll.NewProc("nf_tcpSetSockOpt")

	a.nf_getProcessNameA = a.dll.NewProc("nf_getProcessNameA")
	a.nf_getProcessNameW = a.dll.NewProc("nf_getProcessNameW")
	a.nf_getProcessNameFromKernel = a.dll.NewProc("nf_getProcessNameFromKernel")
	a.nf_adjustProcessPriviledges = a.dll.NewProc("nf_adjustProcessPriviledges")
	a.nf_tcpIsProxy = a.dll.NewProc("nf_tcpIsProxy")
	a.nf_setOptions = a.dll.NewProc("nf_setOptions")
	a.nf_completeTCPConnectRequest = a.dll.NewProc("nf_completeTCPConnectRequest")
	a.nf_completeUDPConnectRequest = a.dll.NewProc("nf_completeUDPConnectRequest")
	a.nf_getTCPConnInfo = a.dll.NewProc("nf_getTCPConnInfo")
	a.nf_getUDPConnInfo = a.dll.NewProc("nf_getUDPConnInfo")

	a.nf_setIPEventHandler = a.dll.NewProc("nf_setIPEventHandler")
	a.nf_addFlowCtl = a.dll.NewProc("nf_addFlowCtl")
	a.nf_deleteFlowCtl = a.dll.NewProc("nf_deleteFlowCtl")
	a.nf_setTCPFlowCtl = a.dll.NewProc("nf_setTCPFlowCtl")
	a.nf_setUDPFlowCtl = a.dll.NewProc("nf_setUDPFlowCtl")
	a.nf_modifyFlowCtl = a.dll.NewProc("nf_modifyFlowCtl")
	a.nf_getFlowCtlStat = a.dll.NewProc("nf_getFlowCtlStat")
	a.nf_getTCPStat = a.dll.NewProc("nf_getTCPStat")
	a.nf_getUDPStat = a.dll.NewProc("nf_getUDPStat")
	a.nf_addBindingRule = a.dll.NewProc("nf_addBindingRule")
	a.nf_deleteBindingRules = a.dll.NewProc("nf_deleteBindingRules")
	a.nf_getDriverType = a.dll.NewProc("nf_getDriverType")
	return nil
}
func ret(r uintptr, _ uintptr, err error) (NF_STATUS, error) {
	if errors.Is(err, syscall.Errno(0)) {
		return NF_STATUS(r), nil
	}
	return NF_STATUS(r), err
}

// 初始化
func (a NFApi) NfInit() (NF_STATUS, error) {
	//这里使用CGO的方式去调用初始化，否则X86回调参数有问题，具体什么原因导致的，我也不知道
	x := CgoDriverInit(NF_DriverName, a.nf_init.Addr())
	if x == 0 {
		return 0, nil
	}
	/*
		直接调用DLL初始化

		sp, err := syscall.BytePtrFromString(NF_DriverName)
		if err != nil {
			return NF_STATUS_FAIL, err
		}
		return ret(a.nf_init.Call(uintptr(unsafe.Pointer(sp)), uintptr(unsafe.Pointer(Ev))))
	*/
	return NF_STATUS(x), nil
}

// 释放
func (a NFApi) NfFree() (NF_STATUS, error) {
	return ret(a.nf_free.Call())
}

// 注册驱动
func (a NFApi) NfRegisterDriver(driverName string) (NF_STATUS, error) {
	sp, err := syscall.BytePtrFromString(driverName)
	if err != nil {
		return NF_STATUS_FAIL, err
	}
	return ret(a.nf_registerDriver.Call(uintptr(unsafe.Pointer(sp))))
}

// 从其他位置注册驱动
func (a NFApi) NfRegisterDriverEx(driverName string, path string) (NF_STATUS, error) {
	sp, err := syscall.BytePtrFromString(driverName)
	if err != nil {
		return NF_STATUS_FAIL, err
	}
	pathp, err := syscall.BytePtrFromString(path)
	if err != nil {
		return NF_STATUS_FAIL, err
	}

	return ret(a.nf_registerDriverEx.Call(uintptr(unsafe.Pointer(sp)), uintptr(unsafe.Pointer(pathp))))
}

// 卸载驱动服务（需要重启或手动停止服务才可以重新注册）
func (a NFApi) NfUnRegisterDriver(driverName string) (NF_STATUS, error) {
	sp, err := syscall.BytePtrFromString(driverName)
	if err != nil {
		return NF_STATUS_FAIL, err
	}
	return ret(a.nf_unRegisterDriver.Call(uintptr(unsafe.Pointer(sp))))
}

// 设置TCP链接状态
func (a NFApi) NfTcpSetConnectionState(id uint64, suspended bool) (NF_STATUS, error) {
	var suspend int32 = 0
	if suspended {
		suspend = 1
	}
	if WindowsX64 {
		return ret(a.nf_tcpSetConnectionState.Call(uintptr(id), uintptr(suspend)))
	}
	id1 := *(*uint32)(unsafe.Pointer(&id))
	id2 := *(*uint32)(unsafe.Pointer(uintptr(unsafe.Pointer(&id)) + 4))
	return ret(a.nf_tcpSetConnectionState.Call(uintptr(id1), uintptr(id2), uintptr(suspend)))
}

// TCP数据发送
func (a NFApi) NfTcpPostSend(id uint64, bufer *byte, L int32) (NF_STATUS, error) {
	if WindowsX64 {
		return ret(a.nf_tcpPostSend.Call(uintptr(id), uintptr(unsafe.Pointer(bufer)), uintptr(L)))
	}
	id1 := *(*uint32)(unsafe.Pointer(&id))
	id2 := *(*uint32)(unsafe.Pointer(uintptr(unsafe.Pointer(&id)) + 4))
	return ret(a.nf_tcpPostSend.Call(uintptr(id1), uintptr(id2), uintptr(unsafe.Pointer(bufer)), uintptr(L)))
}

// TCP数据接受
func (a NFApi) NfTcpPostReceive(id uint64, bufer *byte, L int32) (NF_STATUS, error) {
	if WindowsX64 {
		return ret(a.nf_tcpPostReceive.Call(uintptr(id), uintptr(unsafe.Pointer(bufer)), uintptr(L)))
	}
	id1 := *(*uint32)(unsafe.Pointer(&id))
	id2 := *(*uint32)(unsafe.Pointer(uintptr(unsafe.Pointer(&id)) + 4))
	return ret(a.nf_tcpPostReceive.Call(uintptr(id1), uintptr(id2), uintptr(unsafe.Pointer(bufer)), uintptr(L)))
}

// 获取进程名 请注意 如果文件名有中文 那么获取到的文件名是GBK编码的
func (a NFApi) NfgetProcessNameA(ProcessId uint32) (NF_STATUS, error, string) {
	name := make([]byte, 256)
	v, b := ret(a.nf_getProcessNameA.Call(uintptr(ProcessId), uintptr(unsafe.Pointer(&name[0])), 256))
	var k bytes.Buffer
	for i := 0; i < 256; i++ {
		l := name[i]
		if l == 0 {
			break
		}
		k.WriteByte(l)
	}
	arr := strings.Split(k.String(), "\\")
	if len(arr) > 0 {
		k.Reset()
		k.WriteString(arr[len(arr)-1])
		return v, b, k.String()
	}
	return v, b, ""
}

// tcp关闭
func (a NFApi) NfTcpClose(id uint64) (NF_STATUS, error) {
	if WindowsX64 {
		return ret(a.nf_tcpClose.Call(uintptr(id)))
	}
	id1 := *(*uint32)(unsafe.Pointer(&id))
	id2 := *(*uint32)(unsafe.Pointer(uintptr(unsafe.Pointer(&id)) + 4))
	return ret(a.nf_tcpClose.Call(uintptr(id1), uintptr(id2)))
}

// tcp超时
func (a NFApi) NfSetTCPTimeout(id uint64) (NF_STATUS, error) {
	if WindowsX64 {
		return ret(a.nf_tcpClose.Call(uintptr(id)))
	}
	id1 := *(*uint32)(unsafe.Pointer(&id))
	id2 := *(*uint32)(unsafe.Pointer(uintptr(unsafe.Pointer(&id)) + 4))
	return ret(a.nf_tcpClose.Call(uintptr(id1), uintptr(id2)))
}

// 禁用TCP过滤
func (a NFApi) NfTcpDisableFiltering(id uint64) (NF_STATUS, error) {
	if WindowsX64 {
		return ret(a.nf_tcpDisableFiltering.Call(uintptr(id)))
	}
	id1 := *(*uint32)(unsafe.Pointer(&id))
	id2 := *(*uint32)(unsafe.Pointer(uintptr(unsafe.Pointer(&id)) + 4))
	return ret(a.nf_tcpDisableFiltering.Call(uintptr(id1), uintptr(id2)))
}

// UDP

// 设置UDP链接状态
func (a NFApi) NfUdpSetConnectionState(id uint64, suspended bool) (NF_STATUS, error) {
	var suspend int32 = 0
	if suspended {
		suspend = 1
	}
	if WindowsX64 {
		return ret(a.nf_udpSetConnectionState.Call(uintptr(id), uintptr(suspend)))
	}
	id1 := *(*uint32)(unsafe.Pointer(&id))
	id2 := *(*uint32)(unsafe.Pointer(uintptr(unsafe.Pointer(&id)) + 4))
	return ret(a.nf_udpSetConnectionState.Call(uintptr(id1), uintptr(id2), uintptr(suspend)))
}

// 发送UDP数据
func (a NFApi) NfUdpPostSend(id uint64, remoteAddress *SockaddrInx, buf []byte, option *NF_UDP_OPTIONS) (NF_STATUS, error) {
	if len(buf) < 1 {
		return -1, nil
	}
	bs := remoteAddress.ToBytes()
	if WindowsX64 {
		return ret(a.nf_udpPostSend.Call(uintptr(id), uintptr(unsafe.Pointer(&bs[0])), uintptr(unsafe.Pointer(&buf[0])), uintptr(int32(len(buf))), uintptr(unsafe.Pointer(option))))
	}
	id1 := *(*uint32)(unsafe.Pointer(&id))
	id2 := *(*uint32)(unsafe.Pointer(uintptr(unsafe.Pointer(&id)) + 4))
	return ret(a.nf_udpPostSend.Call(uintptr(id1), uintptr(id2), uintptr(unsafe.Pointer(&bs[0])), uintptr(unsafe.Pointer(&buf[0])), uintptr(int32(len(buf))), uintptr(unsafe.Pointer(option))))
}

// 接收UDP数据
func (a NFApi) NfUdpPostReceive(id uint64, remoteAddress *SockaddrInx, buf []byte, option *NF_UDP_OPTIONS) (NF_STATUS, error) {
	if len(buf) < 1 {
		return -1, nil
	}
	bs := remoteAddress.ToBytes()
	if WindowsX64 {
		return ret(a.nf_udpPostReceive.Call(
			uintptr(id),
			uintptr(unsafe.Pointer(&bs[0])),
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(int32(len(buf))),
			uintptr(unsafe.Pointer(option)),
		))
	}
	id1 := *(*uint32)(unsafe.Pointer(&id))
	id2 := *(*uint32)(unsafe.Pointer(uintptr(unsafe.Pointer(&id)) + 4))
	return ret(a.nf_udpPostReceive.Call(
		uintptr(id1), uintptr(id2),
		uintptr(unsafe.Pointer(&bs[0])),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(int32(len(buf))),
		uintptr(unsafe.Pointer(option)),
	))
}

// 禁用UDP过滤
func (a NFApi) NfUdpDisableFiltering(id uint64) (NF_STATUS, error) {
	if WindowsX64 {
		return ret(a.nf_udpDisableFiltering.Call(uintptr(id)))
	}
	id1 := *(*uint32)(unsafe.Pointer(&id))
	id2 := *(*uint32)(unsafe.Pointer(uintptr(unsafe.Pointer(&id)) + 4))
	return ret(a.nf_udpDisableFiltering.Call(uintptr(id1), uintptr(id2)))
}

//IP

// 发送IP数据
func (a NFApi) NfIpPostSend(buf []byte, option *NF_IP_PACKET_OPTIONS) (NF_STATUS, error) {
	return ret(a.nf_ipPostSend.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(int32(len(buf))),
		uintptr(unsafe.Pointer(option)),
	))
}

// 接收IP数据
func (a NFApi) NfIpPostReceive(buf []byte, option *NF_IP_PACKET_OPTIONS) (NF_STATUS, error) {
	return ret(a.nf_ipPostReceive.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(int32(len(buf))),
		uintptr(unsafe.Pointer(option)),
	))
}

// Rule

// 添加规则
func (a NFApi) NfAddRule(rule *NF_RULE, ToHead bool) (NF_STATUS, error) {
	var h int32 = 0
	if ToHead {
		h = 1
	}
	return ret(a.nf_addRule.Call(uintptr(unsafe.Pointer(rule)), uintptr(h)))
}

// 删除规则
func (a NFApi) NfDeleteRules() (NF_STATUS, error) {
	return ret(a.nf_deleteRules.Call())
}

// 设置规则
func (a NFApi) NfSetRules(rule []NF_RULE) (NF_STATUS, error) {
	return ret(a.nf_setRules.Call(uintptr(unsafe.Pointer(&rule)), uintptr(int32(len(rule)))))
}

// 添加扩展规则
func (a NFApi) NfAddRuleEx(rule *NF_RULE_EX, ToHead bool) (NF_STATUS, error) {
	var h int32 = 0
	if ToHead {
		h = 1
	}
	return ret(a.nf_addRuleEx.Call(uintptr(unsafe.Pointer(rule)), uintptr(h)))
}

// 设置扩展规则
func (a NFApi) NfSetRulesEx(rule []NF_RULE_EX) (NF_STATUS, error) {
	return ret(a.nf_setRulesEx.Call(uintptr(unsafe.Pointer(&rule)), uintptr(int32(len(rule)))))
}

// Debug routine
func (a NFApi) NfGetConnCount() (uint32, error) {
	r, _, err := a.nf_getConnCount.Call()
	return uint32(r), err
}

// 设置TCP链接参数
func (a NFApi) NfTcpSetSockOpt(id uint64, optname int32, optval []byte) (NF_STATUS, error) {
	if WindowsX64 {
		return ret(a.nf_tcpSetSockOpt.Call(
			uintptr(id),
			uintptr(optname),
			uintptr(unsafe.Pointer(&optval[0])),
			uintptr(int32(len(optval))),
		))
	}
	id1 := *(*uint32)(unsafe.Pointer(&id))
	id2 := *(*uint32)(unsafe.Pointer(uintptr(unsafe.Pointer(&id)) + 4))
	return ret(a.nf_tcpSetSockOpt.Call(
		uintptr(id1), uintptr(id2),
		uintptr(optname),
		uintptr(unsafe.Pointer(&optval[0])),
		uintptr(int32(len(optval))),
	))
}

// 获取进程名称
func (a NFApi) NfGetProcessNameW(processId uint32) (string, bool, error) {
	buf := [260]uint16{}
	stat, _, err := a.nf_getProcessNameW.Call(uintptr(processId), uintptr(unsafe.Pointer(&buf)), uintptr(uint16(260)))
	return syscall.UTF16ToString(buf[:]), stat == 1, err
}

// 获取进程名称(内核)
func (a NFApi) NfGetProcessNameFromKernel(processId uint32) (string, bool, error) {
	buf := [260]uint16{}
	stat, _, err := a.nf_getProcessNameFromKernel.Call(uintptr(processId), uintptr(unsafe.Pointer(&buf)), uintptr(uint16(260)))
	return syscall.UTF16ToString(buf[:]), stat == 1, err
}

// 运行当前进程查看所有进行名称
func (a NFApi) NfAdjustProcessPriviledges() {
	a.nf_adjustProcessPriviledges.Call()
}

// 进程TCP是否代理
func (a NFApi) NfTcpIsProxy(processId uint32) (bool, error) {
	b, _, err := a.nf_tcpIsProxy.Call(uintptr(processId))
	return b == 1, err
}

// 设置NFAPI选项
func (a NFApi) NfSetOptions(nThreads uint16, flag uint16) {
	a.nf_setOptions.Call(uintptr(nThreads), uintptr(flag))
}

// 完成TCP请求
func (a NFApi) NfCompleteTCPConnectRequest(id uint64, pConnInfo *NF_TCP_CONN_INFO) (NF_STATUS, error) {
	if WindowsX64 {
		return ret(a.nf_completeTCPConnectRequest.Call(uintptr(id), uintptr(unsafe.Pointer(pConnInfo))))
	}
	id1 := *(*uint32)(unsafe.Pointer(&id))
	id2 := *(*uint32)(unsafe.Pointer(uintptr(unsafe.Pointer(&id)) + 4))
	return ret(a.nf_completeTCPConnectRequest.Call(uintptr(id1), uintptr(id2), uintptr(unsafe.Pointer(pConnInfo))))
}

// 完成UDP请求
func (a NFApi) NfCompleteUDPConnectRequest(id uint64, pConnInfo *NF_UDP_CONN_INFO) (NF_STATUS, error) {
	if WindowsX64 {
		return ret(a.nf_completeUDPConnectRequest.Call(uintptr(id), uintptr(unsafe.Pointer(pConnInfo))))
	}
	id1 := *(*uint32)(unsafe.Pointer(&id))
	id2 := *(*uint32)(unsafe.Pointer(uintptr(unsafe.Pointer(&id)) + 4))
	return ret(a.nf_completeUDPConnectRequest.Call(uintptr(id1), uintptr(id2), uintptr(unsafe.Pointer(pConnInfo))))
}

// 获取TCP链接信息
func (a NFApi) NfGetTCPConnInfo(id uint64, pConnInfo *NF_TCP_CONN_INFO) (NF_STATUS, error) {
	if WindowsX64 {
		return ret(a.nf_getTCPConnInfo.Call(uintptr(id), uintptr(unsafe.Pointer(pConnInfo))))
	}
	id1 := *(*uint32)(unsafe.Pointer(&id))
	id2 := *(*uint32)(unsafe.Pointer(uintptr(unsafe.Pointer(&id)) + 4))
	return ret(a.nf_getTCPConnInfo.Call(uintptr(id1), uintptr(id2), uintptr(unsafe.Pointer(pConnInfo))))
}

// 获取UDP链接信息
func (a NFApi) NfGetUDPConnInfo(id uint64, pConnInfo *NF_UDP_CONN_INFO) (NF_STATUS, error) {
	if WindowsX64 {
		return ret(a.nf_getUDPConnInfo.Call(uintptr(id), uintptr(unsafe.Pointer(pConnInfo))))
	}
	id1 := *(*uint32)(unsafe.Pointer(&id))
	id2 := *(*uint32)(unsafe.Pointer(uintptr(unsafe.Pointer(&id)) + 4))
	return ret(a.nf_getUDPConnInfo.Call(uintptr(id1), uintptr(id2), uintptr(unsafe.Pointer(pConnInfo))))
}

//设置IP事件
//func (a NFApi) NfSetIPEventHandler()

func (a NFApi) NfAddFlowCtl(pData *NF_FLOWCTL_DATA, pFcHandle *uint32) (NF_STATUS, error) {
	return ret(a.nf_addFlowCtl.Call(uintptr(unsafe.Pointer(pData)), uintptr(unsafe.Pointer(pFcHandle))))
}
func (a NFApi) NfDeleteFlowCtl(fcHandle uint32) (NF_STATUS, error) {
	return ret(a.nf_deleteFlowCtl.Call(uintptr((fcHandle))))
}
func (a NFApi) NfSetTCPFlowCtl(id uint64, fcHandle uint32) (NF_STATUS, error) {
	if WindowsX64 {
		return ret(a.nf_setTCPFlowCtl.Call(uintptr(id), uintptr((fcHandle))))
	}
	id1 := *(*uint32)(unsafe.Pointer(&id))
	id2 := *(*uint32)(unsafe.Pointer(uintptr(unsafe.Pointer(&id)) + 4))
	return ret(a.nf_setTCPFlowCtl.Call(uintptr(id1), uintptr(id2), uintptr((fcHandle))))
}
func (a NFApi) NfSetUDPFlowCtl(id uint64, fcHandle uint32) (NF_STATUS, error) {
	if WindowsX64 {
		return ret(a.nf_setUDPFlowCtl.Call(uintptr(id), uintptr((fcHandle))))
	}
	id1 := *(*uint32)(unsafe.Pointer(&id))
	id2 := *(*uint32)(unsafe.Pointer(uintptr(unsafe.Pointer(&id)) + 4))
	return ret(a.nf_setUDPFlowCtl.Call(uintptr(id1), uintptr(id2), uintptr((fcHandle))))
}
func (a NFApi) NfModifyFlowCtl(fcHandle uint32, pData *NF_FLOWCTL_DATA) (NF_STATUS, error) {
	return ret(a.nf_modifyFlowCtl.Call(uintptr(fcHandle), uintptr(unsafe.Pointer(pData))))
}
func (a NFApi) NfGetFlowCtlStat(fcHandle uint32, pData *NF_FLOWCTL_STAT) (NF_STATUS, error) {
	return ret(a.nf_getFlowCtlStat.Call(uintptr(fcHandle), uintptr(unsafe.Pointer(pData))))
}
func (a NFApi) NfGetTCPStat(id uint64, pData *NF_FLOWCTL_STAT) (NF_STATUS, error) {
	if WindowsX64 {
		return ret(a.nf_getTCPStat.Call(uintptr(id), uintptr(unsafe.Pointer(pData))))
	}
	id1 := *(*uint32)(unsafe.Pointer(&id))
	id2 := *(*uint32)(unsafe.Pointer(uintptr(unsafe.Pointer(&id)) + 4))
	return ret(a.nf_getTCPStat.Call(uintptr(id1), uintptr(id2), uintptr(unsafe.Pointer(pData))))
}
func (a NFApi) NfGetUDPStat(id uint64, pData *NF_FLOWCTL_STAT) (NF_STATUS, error) {
	if WindowsX64 {
		return ret(a.nf_getUDPStat.Call(uintptr(id), uintptr(unsafe.Pointer(pData))))
	}
	id1 := *(*uint32)(unsafe.Pointer(&id))
	id2 := *(*uint32)(unsafe.Pointer(uintptr(unsafe.Pointer(&id)) + 4))
	return ret(a.nf_getUDPStat.Call(uintptr(id1), uintptr(id2), uintptr(unsafe.Pointer(pData))))
}
func (a NFApi) NfAddBindingRule(prule *NF_BINDING_RULE, toHead bool) (NF_STATUS, error) {
	var t int32 = 0
	if toHead {
		t = 1
	}
	return ret(a.nf_addBindingRule.Call(uintptr(unsafe.Pointer(prule)), uintptr(t)))
}
func (a NFApi) NfDeleteBindingRules() (NF_STATUS, error) {
	return ret(a.nf_deleteBindingRules.Call())
}
func (a NFApi) NfGetDriverType() (uint32, error) {
	r, _, err := a.nf_getDriverType.Call()
	return uint32(r), err
}

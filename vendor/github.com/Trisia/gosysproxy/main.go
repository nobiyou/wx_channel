//go:build windows
// +build windows

// windows系统代理配置
package gosysproxy

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
	"syscall"
	"unsafe"
)

var (
	wininet, _           = syscall.LoadLibrary("Wininet.dll")
	internetSetOption, _ = syscall.GetProcAddress(wininet, "InternetSetOptionW")
	// https://learn.microsoft.com/zh-cn/windows/win32/api/wininet/nf-wininet-internetqueryoptionw
	internetQueryOption, _ = syscall.GetProcAddress(wininet, "InternetQueryOptionA")
)

const (
	_INTERNET_OPTION_PER_CONNECTION_OPTION  = 75
	_INTERNET_OPTION_PROXY_SETTINGS_CHANGED = 95
	_INTERNET_OPTION_REFRESH                = 37
	_INTERNET_OPTION_PROXY                  = 38
)

const (
	_PROXY_TYPE_DIRECT         = 0x00000001 // direct to net
	_PROXY_TYPE_PROXY          = 0x00000002 // via named proxy
	_PROXY_TYPE_AUTO_PROXY_URL = 0x00000004 // autoproxy URL
	_PROXY_TYPE_AUTO_DETECT    = 0x00000008 // use autoproxy detection
)

const (
	_INTERNET_PER_CONN_FLAGS                        = 1
	_INTERNET_PER_CONN_PROXY_SERVER                 = 2
	_INTERNET_PER_CONN_PROXY_BYPASS                 = 3
	_INTERNET_PER_CONN_AUTOCONFIG_URL               = 4
	_INTERNET_PER_CONN_AUTODISCOVERY_FLAGS          = 5
	_INTERNET_PER_CONN_AUTOCONFIG_SECONDARY_URL     = 6
	_INTERNET_PER_CONN_AUTOCONFIG_RELOAD_DELAY_MINS = 7
	_INTERNET_PER_CONN_AUTOCONFIG_LAST_DETECT_TIME  = 8
	_INTERNET_PER_CONN_AUTOCONFIG_LAST_DETECT_URL   = 9
	_INTERNET_PER_CONN_FLAGS_UI                     = 10
)

const (
	INTERNET_OPEN_TYPE_PRECONFIG = 0 // use registry configuration
	INTERNET_OPEN_TYPE_DIRECT    = 1 // 禁用代理 direct to net
	INTERNET_OPEN_TYPE_PROXY     = 3 // 启用代理 via named proxy
)

type internetPerConnOptionList struct {
	dwSize        uint32
	pszConnection *uint16
	dwOptionCount uint32
	dwOptionError uint32
	pOptions      uintptr
}

type internetPreConnOption struct {
	dwOption uint32
	value    uint64
}

// internetProxyInfo https://learn.microsoft.com/zh-cn/windows/win32/api/wininet/ns-wininet-internet_proxy_info
type internetProxyInfo struct {
	dwAccessType    uint32
	lpszProxy       *uint16
	lpszProxyBypass *uint16
}

type ProxyStatus struct {
	// 代理类型
	// - 0: INTERNET_OPEN_TYPE_PRECONFIG: use registry configuration
	// - 1: INTERNET_OPEN_TYPE_DIRECT: 不代理 direct to net
	// - 3: INTERNET_OPEN_TYPE_PROXY: 使用代理服务器 via named proxy
	Type  uint32
	Proxy string // 代理IP地址与端口，IP:Port，例如："127.0.0.1:7890"
	// 请勿对以下列条目开头的地址使用代理服务器
	// 注意：
	// - 这里的地址是ASCII编码
	// - "<local>" 表示 本地(Intranet)地址，如果包含 "<local>" 则 DisableProxyIntranet 为 true
	//
	// 例如：["localhost","127.*"]，
	Bypass               []string
	DisableProxyIntranet bool // 请勿将代理服务器用于本地(Intranet)地址
}

// stringPtrAddr 获取C字符串(UTF16)的数组第一个位置的地址
func stringPtrAddr(str string) (uint64, error) {
	scriptLocPtr, err := syscall.UTF16PtrFromString(str)
	if err != nil {
		return 0, err
	}
	n := new(big.Int)
	n.SetString(fmt.Sprintf("%x\n", scriptLocPtr), 16)
	return n.Uint64(), nil
}

// newParam 创建参数序列容器
// n: 参数数量
func newParam(n int) internetPerConnOptionList {
	return internetPerConnOptionList{
		dwSize:        4,
		pszConnection: nil,
		dwOptionCount: uint32(n),
		dwOptionError: 0,
		pOptions:      0,
	}
}

// SetPAC 设置PAC代理模式
// scriptLoc: 脚本地址，如: "http://127.0.0.1:7777/pac"
func SetPAC(scriptLoc string) error {
	if scriptLoc == "" {
		return errors.New("PAC脚本地址(scriptLoc)配置为空")
	}

	scriptLocAddr, err := stringPtrAddr(scriptLoc)
	if err != nil {
		return err
	}

	param := newParam(2)
	// 利用Golang数组地址空间连续模拟 malloc
	options := []internetPreConnOption{
		{dwOption: _INTERNET_PER_CONN_FLAGS, value: _PROXY_TYPE_AUTO_PROXY_URL | _PROXY_TYPE_DIRECT},
		{dwOption: _INTERNET_PER_CONN_AUTOCONFIG_URL, value: scriptLocAddr},
	}
	param.pOptions = uintptr(unsafe.Pointer(&options[0]))
	ret, _, infoPtr := syscall.Syscall6(internetSetOption,
		4,
		0,
		_INTERNET_OPTION_PER_CONNECTION_OPTION,
		uintptr(unsafe.Pointer(&param)),
		unsafe.Sizeof(param),
		0, 0)
	// fmt.Printf(">> Ret [%d] Setting options: %s\n", ret, info)
	if ret != 1 {
		return errors.New(fmt.Sprintf("%s", infoPtr))
	}

	return Flush()
}

// SetGlobalProxy 设置全局代理
// - proxyServer: 代理服务器 host:port，例如: "127.0.0.1:7890"
// - bypass: 忽略代理列表,这些配置项开头的地址不进行代理，若包含 "<local>" 则 ”请勿将代理服务器用于本地(Intranet)地址“ 将勾选。
func SetGlobalProxy(proxyServer string, bypasses ...string) error {
	if proxyServer == "" {
		return errors.New("代理服务器(proxyServer)配置为空")
	}

	proxyServerPtrAddr, err := stringPtrAddr(proxyServer)
	if err != nil {
		return err
	}

	var bypassBuilder strings.Builder
	// 地址过滤配置
	if bypasses != nil {
		for _, item := range bypasses {
			bypassBuilder.WriteString(item)
			bypassBuilder.WriteByte(';')
		}
	} else {
		bypassBuilder.WriteString("<local>")
	}
	bypassAddr, err := stringPtrAddr(bypassBuilder.String())
	if err != nil {
		return err
	}

	param := newParam(3)
	options := []internetPreConnOption{
		{dwOption: _INTERNET_PER_CONN_FLAGS, value: _PROXY_TYPE_PROXY | _PROXY_TYPE_DIRECT},
		{dwOption: _INTERNET_PER_CONN_PROXY_SERVER, value: proxyServerPtrAddr},
		{dwOption: _INTERNET_PER_CONN_PROXY_BYPASS, value: bypassAddr},
	}
	param.pOptions = uintptr(unsafe.Pointer(&options[0]))
	ret, _, infoPtr := syscall.Syscall6(internetSetOption,
		4,
		0,
		_INTERNET_OPTION_PER_CONNECTION_OPTION,
		uintptr(unsafe.Pointer(&param)),
		unsafe.Sizeof(param),
		0, 0)
	// fmt.Printf(">> Ret [%d] Setting options: %s\n", ret, infoPtr)
	if ret != 1 {
		return errors.New(fmt.Sprintf("%s", infoPtr))
	}

	return Flush()
}

// Off 关闭代理
func Off() error {
	param := newParam(1)
	option := internetPreConnOption{
		dwOption: _INTERNET_PER_CONN_FLAGS,
		//value:    _PROXY_TYPE_AUTO_DETECT | _PROXY_TYPE_DIRECT}
		value: _PROXY_TYPE_DIRECT}
	param.pOptions = uintptr(unsafe.Pointer(&option))
	ret, _, infoPtr := syscall.Syscall6(internetSetOption,
		4,
		0,
		_INTERNET_OPTION_PER_CONNECTION_OPTION,
		uintptr(unsafe.Pointer(&param)),
		unsafe.Sizeof(param),
		0, 0)
	// fmt.Printf(">> Ret [%d] Setting options: %s\n", ret, info)
	if ret != 1 {
		return errors.New(fmt.Sprintf("%s", infoPtr))
	}
	return Flush()
}

// Flush 更新系统配置使生效
func Flush() error {
	ret, _, infoPtr := syscall.Syscall6(internetSetOption,
		4,
		0,
		_INTERNET_OPTION_PROXY_SETTINGS_CHANGED,
		0, 0,
		0, 0)
	// fmt.Println(">> Propagating changes:", fmt.Sprintf("%s", errno))
	if ret != 1 {
		return errors.New(fmt.Sprintf("%s", infoPtr))
	}

	ret, _, infoPtr = syscall.Syscall6(internetSetOption,
		4,
		0,
		_INTERNET_OPTION_REFRESH,
		0, 0,
		0, 0)
	// fmt.Println(">> Refreshing:", errno)
	if ret != 1 {
		return errors.New(fmt.Sprintf("%s", infoPtr))
	}
	return nil
}

// Status 获取当前系统代理配置
func Status() (*ProxyStatus, error) {
	var bufferLength uint32 = 1024 * 10
	buffer := make([]byte, bufferLength)
	ret, _, infoPtr := syscall.Syscall6(internetQueryOption,
		4,
		0,
		_INTERNET_OPTION_PROXY,
		uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&bufferLength)),
		0, 0)
	if ret != 1 {
		return nil, errors.New(fmt.Sprintf("%s", infoPtr))
	}
	//fmt.Println(hex.Dump(buffer[:bufferLength]))
	proxyInfo := (*internetProxyInfo)(unsafe.Pointer(&buffer[0]))
	res := &ProxyStatus{
		Type:  proxyInfo.dwAccessType,
		Proxy: asciiPtrToString(proxyInfo.lpszProxy),
	}
	bypassArr := asciiPtrToString(proxyInfo.lpszProxyBypass)
	res.Bypass = strings.SplitN(bypassArr, " ", -1)
	for _, bypass := range res.Bypass {
		if bypass == "<local>" {
			res.DisableProxyIntranet = true
			break
		}
	}
	return res, nil
}

// ASCIIPtrToString 将UTF16指针转换为ASCII字符串
func asciiPtrToString(p *uint16) string {
	if p == nil {
		return ""
	}
	res := []byte{}
	end := unsafe.Pointer(p)
	for {
		if *(*uint8)(end) == 0 {
			break
		}
		res = append(res, *(*uint8)(end))
		end = unsafe.Pointer(uintptr(end) + 1)
	}
	return string(res)
}

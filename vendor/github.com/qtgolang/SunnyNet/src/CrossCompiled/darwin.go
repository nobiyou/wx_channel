//go:build darwin
// +build darwin

package CrossCompiled

import (
	NFapi "github.com/qtgolang/SunnyNet/src/nfapi"
	"os/exec"
	"strconv"
	"strings"
)

type netInterface struct{}

// 获取所有网络接口名称
func (c *netInterface) getAllInterfaceNames() []string {
	cmd := exec.Command("networksetup", "-listallnetworkservices")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	var interfaceNames []string
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "An asterisk (*) ") || strings.HasPrefix(line, " ") || line == "" {
			continue
		}
		interfaceNames = append(interfaceNames, line)
	}

	return interfaceNames
}

func (c *netInterface) SetProxy(proxyHost string, Port int) bool {
	AllInterfaceName := c.getAllInterfaceNames()
	if len(AllInterfaceName) < 1 {
		return false
	}
	proxyPort := strconv.Itoa(Port)
	for _, interfaceName := range AllInterfaceName {
		// 设置 HTTP 代理
		setWebProxyCmd := exec.Command("networksetup", "-setwebproxy", interfaceName, proxyHost, proxyPort)
		_ = setWebProxyCmd.Run()

		// 设置 HTTPS 代理
		setSecureWebProxyCmd := exec.Command("networksetup", "-setsecurewebproxy", interfaceName, proxyHost, proxyPort)
		_ = setSecureWebProxyCmd.Run()

		// 设置 SOCKS 代理
		setSocksProxyCmd := exec.Command("networksetup", "-setsocksfirewallproxy", interfaceName, proxyHost, proxyPort)
		_ = setSocksProxyCmd.Run()
	}
	return true
}

func (c *netInterface) DisableProxy() bool {
	AllInterfaceName := c.getAllInterfaceNames()
	if len(AllInterfaceName) < 1 {
		return false
	}
	for _, interfaceName := range AllInterfaceName {
		// 关闭 HTTP 代理
		disableWebProxyCmd := exec.Command("networksetup", "-setwebproxystate", interfaceName, "off")
		_ = disableWebProxyCmd.Run()
		// 关闭 HTTPS 代理
		disableSecureWebProxyCmd := exec.Command("networksetup", "-setsecurewebproxystate", interfaceName, "off")
		_ = disableSecureWebProxyCmd.Run()
		// 关闭 SOCKS 代理
		disableSocksProxyCmd := exec.Command("networksetup", "-setsocksfirewallproxystate", interfaceName, "off")
		_ = disableSocksProxyCmd.Run()
	}
	return true
}

func SetIeProxy(Off bool, Port int) bool {
	Inter := &netInterface{}
	if Off {
		return Inter.DisableProxy()
	}
	return Inter.SetProxy("127.0.0.1", Port)
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

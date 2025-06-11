//go:build windows
// +build windows

package NFapi

/*
#include <windows.h>
#include <stdlib.h>

char* getSystemDirectory() {
    char* buffer = (char*)malloc(MAX_PATH);
    if (buffer == NULL) {
        return NULL;
    }
    DWORD result = GetSystemDirectoryA(buffer, MAX_PATH);
    if (result == 0) {
        free(buffer);
        return NULL;
    }
    return buffer;
}
*/
import "C"
import "C"
import (
	"fmt"
	"net"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

var apiLoad bool
var apiNfInit bool

func GetSystemDirectory() string {
	buffer := C.getSystemDirectory()
	if buffer == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(buffer))
	return C.GoString(buffer)
}
func GetWindowsDirectory() string {
	winDir := os.Getenv("windir")
	if winDir == "" {
		// 如果 windir 不存在，则获取 SystemRoot 环境变量
		winDir = os.Getenv("SystemRoot")
	}
	if winDir[len(winDir)-1:] != "\\" {
		winDir += "\\"
	}
	return winDir
}

func MessageBox(caption, text string, style uintptr) (result int) {
	user32, _ := syscall.LoadLibrary("user32.dll")
	messageBox, _ := syscall.GetProcAddress(user32, "MessageBoxW")
	ret, _, callErr := syscall.SyscallN(messageBox, 4,
		0,                                                          // hwnd
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(text))),    // Text
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(caption))), // Caption
		style,                                                      // type
		0,
		0)
	if callErr != 0 {
	}
	result = int(ret)
	return
}

func ApiInit() bool {
	if apiLoad == false {
		DLLPath := Install()
		er := Api.Load(DLLPath)
		if er != nil {
			fmt.Println("LoadDLLPathErr=", er)
			return false
		}
		apiLoad = true
	}
	if apiNfInit == false {
		_, v := Api.NfRegisterDriver(NF_DriverName)
		if v != nil {
			errorText := v.Error()
			errorText = strings.ReplaceAll(errorText, "Windows cannot verify the digital signature for this file. A recent hardware or software change might have installed a file that is signed incorrectly or damaged, or that might be malicious software from an unknown source.", "Windows无法验证此驱动文件的数字签名。\r\n\r\n最近的硬件或软件更改可能安装了签名错误或损坏的文件，或者可能是来自未知来源的恶意软件。")
			errorText = strings.ReplaceAll(errorText, "This sys has been blocked from loading", "此驱动程序已被阻止加载。\r\n\r\n可能使用了和 Windows 位数不配的驱动文件。")
			errorText = strings.ReplaceAll(errorText, "The system cannot find the file specified.", "系统找不到指定的驱动文件。")
			errorText = strings.ReplaceAll(errorText, "The specified service has been marked for deletion.", "指定的服务已标记为删除。")
			MessageBox("载入驱动失败：", errorText, 0x00000010)
			return false
		}
		a, er := Api.NfInit()
		if er != nil {
			return false
		}
		if a != 0 {
			return false
		}
		_, er = AddRule(false, IPPROTO_TCP, 0, D_OUT, 0, 0, AF_INET, "", "", "", "", NF_INDICATE_CONNECT_REQUESTS)  //TCP
		_, er = AddRule(false, IPPROTO_TCP, 0, D_OUT, 0, 0, AF_INET6, "", "", "", "", NF_INDICATE_CONNECT_REQUESTS) //TCP

		_, er = AddRule(false, IPPROTO_UDP, 0, D_OUT, 0, 0, AF_INET, "", "", "", "", NF_FILTER)  //UDP
		_, er = AddRule(false, IPPROTO_UDP, 0, D_OUT, 0, 0, AF_INET6, "", "", "", "", NF_FILTER) //UDP

		_, er = AddRule(false, IPPROTO_UDP, 0, D_IN, 0, 0, AF_INET, "", "", "", "", NF_FILTER)  //UDP
		_, er = AddRule(false, IPPROTO_UDP, 0, D_IN, 0, 0, AF_INET6, "", "", "", "", NF_FILTER) //UDP

		//_, er = AddRule(false, IPPROTO_UDP, 0, 0, 0, 0, 0, "", "", "", "", NF_FILTER)                               //UDP
		/*
			_, er = AddRule(false, IPPROTO_UDP, 0, D_OUT, 0, 0, AF_INET, "", "", "", "", NF_FILTER)                     //UDP
			_, er = AddRule(false, IPPROTO_UDP, 0, D_OUT, 0, 0, AF_INET6, "", "", "", "", NF_FILTER)                    //UDP
			_, er = AddRule(false, IPPROTO_UDP, 0, D_IN, 0, 0, AF_INET, "", "", "", "", NF_FILTER)                      //UDP
			_, er = AddRule(false, IPPROTO_UDP, 0, D_IN, 0, 0, AF_INET6, "", "", "", "", NF_FILTER)                     //UDP
			_, er = AddRule(true, IPPROTO_UDP, 0, 0, 0, 0, AF_INET6, "", "", "", "", NF_PEND_CONNECT_REQUEST)           //UDP
			_, er = AddRule(true, IPPROTO_UDP, 0, 0, 0, 0, AF_INET, "", "", "", "", NF_PEND_CONNECT_REQUEST)            //UDP
		*/
		if er != nil {
			return false
		}
		apiNfInit = true
	}
	return true
}

func AddRule(toHead bool, _Protocol, pid int32, _Direction DIRECTION, _LocalPort, _RemotePort, family int16, LocalIp, LocalMask, RemoteIp, RemoteMask string, Flag FILTERING_FLAG) (NF_STATUS, error) {
	r := new(NF_RULE)
	var Protocol INT32
	Protocol.Set(_Protocol)
	r.Protocol = Protocol

	var processId UINT32
	processId.Set(uint32(pid))
	r.ProcessId = processId

	r.Direction = uint8(_Direction)

	var LocalPort UINT16
	LocalPort.Set(uint16(_LocalPort))
	r.LocalPort = LocalPort

	var RemotePort UINT16
	RemotePort.Set(uint16(_RemotePort))
	r.RemotePort = RemotePort

	var ipFamily INT16
	ipFamily.Set(family)
	r.IpFamily = ipFamily

	r.LocalIpAddress.SetIP(true, net.ParseIP(LocalIp))
	r.LocalIpAddressMask.SetIP(true, net.ParseIP(LocalMask))
	r.RemoteIpAddress.SetIP(true, net.ParseIP(RemoteIp))
	r.RemoteIpAddressMask.SetIP(true, net.ParseIP(RemoteMask))

	var FilteringFlag UINT32
	FilteringFlag.Set(uint32(Flag))
	r.FilteringFlag = FilteringFlag
	return Api.NfAddRule(r, toHead)
}

//go:build windows
// +build windows

package NFapi

import (
	"github.com/qtgolang/SunnyNet/src/iphlpapi"
	"golang.org/x/text/encoding/simplifiedchinese"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// ClosePidTCP 关闭指定进程的所有TCP连接
func ClosePidTCP(PID int) {
	iphlpapi.CloseCurrentSocket(PID, AF_INET)
	iphlpapi.CloseCurrentSocket(PID, AF_INET6)

}

// CloseNameTCP 关闭指定进程的所有TCP连接
func CloseNameTCP(processName string) {
	a := GetPIDByName(processName)
	for i := 0; i < len(a); i++ {
		iphlpapi.CloseCurrentSocket(a[i], AF_INET)
		iphlpapi.CloseCurrentSocket(a[i], AF_INET6)
	}
}

// GetPIDByName 根据进程名获取进程 PID
func GetPIDByName(processName string) []int {
	//这里使用的是命令行方式获取，也可以使用Windows API方式获取
	//但是要考虑到进程名称有中文的问题,但是不同编程语言传递进来的目标进程名称的字符编码不同

	// 创建一个空的 PID 数组
	var pidArr []int
	// 创建一个执行命令的实例，命令为 tasklist，参数为 /FO CSV /NH
	cmd := exec.Command("tasklist", "/FO", "CSV", "/NH")
	// 隐藏命令行窗口
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	// 执行命令，并获取输出结果和错误信息
	output, err := cmd.Output()
	// 如果执行命令出错，则返回空的 PID 数组
	if err != nil {
		return pidArr
	}
	// 创建一个 GBK 解码器，并将命令输出结果转换为 UTF-8 编码
	decoder := simplifiedchinese.GBK.NewDecoder()
	utf8Bytes, err := decoder.Bytes(output)
	// 将命令输出结果和解码后的结果拼接成一个字符串，再将字符串按行分割为数组

	//这里是因为 如果进程名称包含中文,这里出现的结果是GBK编码,
	//但是不同编程语言传递进来的目标进程名称的字符编码不同，例如易语言传进来是GBK,GO语言传进来是UTF8,所以转换一下

	processes := strings.Split(string(output)+"\r\n"+string(utf8Bytes), "\r\n")
	// 遍历每个进程信息
	for _, process := range processes {
		// 将进程信息按逗号分割为数组，获取进程名和 PID
		processDetails := strings.Split(process, ",")
		if len(processDetails) >= 2 {
			name, _ := strconv.Unquote(processDetails[0])
			pidStr, _ := strconv.Unquote(processDetails[1])
			// 如果进程名与要查找的名字相同，则将 PID 转换为整数并添加到 PID 数组中
			if strings.ToLower(name) == strings.ToLower(processName) {
				pid, err := strconv.Atoi(pidStr)
				if err == nil {
					pidArr = append(pidArr, pid)
				}
			}
		}
	}
	// 返回 PID 数组
	return pidArr
}

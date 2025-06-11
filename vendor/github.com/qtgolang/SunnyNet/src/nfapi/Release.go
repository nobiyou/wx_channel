//go:build windows
// +build windows

package NFapi

/*
#include <windows.h>

BOOL disableWow64FsRedirection(PVOID* oldValue) {
    return Wow64DisableWow64FsRedirection(oldValue);
}

BOOL revertWow64FsRedirection(PVOID oldValue) {
    return Wow64RevertWow64FsRedirection(oldValue);
}
*/
import "C"
import (
	"bufio"
	_ "embed"
	"fmt"
	"github.com/qtgolang/SunnyNet/Resource"
	"github.com/qtgolang/SunnyNet/public"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// 生成指定长度的随机字母串
func randomLetters(length int) string {
	// 设置随机种子
	rand.Seed(time.Now().UnixNano())

	// 生成指定长度的随机字母
	letters := []rune("abcdefghijklmnopqrstuvwxyz")
	result := make([]rune, length)

	for i := range result {
		result[i] = letters[rand.Intn(len(letters))]
	}

	return string(result)
}

// 删除旧的驱动文件
func deleteOldFiles() {
	System32Dir := GetSystemDirectory()
	OldFileName := System32Dir + "\\drivers\\SunnyFilter.sys"
	//复制到临时目录去系统重启后才可删除
	_, _ = MoveFileToTempDir(OldFileName, "SunnyFilter"+randomLetters(5)+".TMP")
}

// MoveFileToTempDir 将指定文件移动到 Windows 临时目录
// srcFile: 源文件路径，destFileName：目标文件名
// 返回值：目标文件的路径，以及可能出现的错误
func MoveFileToTempDir(srcFile, destFileName string) (string, error) {
	// 获取临时目录路径
	tempDir := os.Getenv("TEMP")

	// 拼接目标文件路径
	destPath := filepath.Join(tempDir, destFileName)

	// 移动文件
	err := os.Rename(srcFile, destPath)
	if err != nil {
		return "", fmt.Errorf("无法移动文件：%s", err)
	}

	return destPath, nil
}

func Install() string {
	deleteOldFiles()
	//XP直接打开不程序，所以就直接忽略
	s := []string{"OS", "Get", "Caption"}
	IsWin7 := strings.Index(ExecCommand("Wmic", s), "Windows 7") != -1

	// C:\Windows\system32\
	System32Dir := GetSystemDirectory()

	var oldValue uintptr
	if Is64Windows {
		//如果是32位进程 禁止文件重定向 驱动只能写到 system32 目录
		if !WindowsX64 {
			oldValue = Wow64DisableWow64FsRedirection()
		}
	}
	if !Exists(System32Dir + "\\drivers\\" + NF_DriverName + ".sys") {
		Path := System32Dir + "\\drivers\\" + NF_DriverName + ".sys"
		if IsWin7 {
			if Is64Windows {
				WriteFile(Path, Resource.TdiAmd64Netfilter2)

			} else {
				WriteFile(Path, Resource.TdiI386Netfilter2)
			}
		} else {
			if Is64Windows {
				WriteFile(Path, Resource.WfpAmd64Netfilter2)

			} else {
				WriteFile(Path, Resource.WfpI386Netfilter2)

			}
		}
	}
	if Is64Windows {
		//如果是32位进程 恢复文件重定向
		if !WindowsX64 {
			Wow64RevertWow64FsRedirection(oldValue)
		}
	}
	WindowsDirectory := GetWindowsDirectory()
	DrDLL := ""
	if WindowsX64 {
		DrDLL = WindowsDirectory + NF_DLLName + "64.DLL"
		WriteFile(DrDLL, Resource.NfapiX64Nfapi)
	} else {
		DrDLL = WindowsDirectory + NF_DLLName + "32.DLL"
		WriteFile(DrDLL, Resource.NfapiWin32Nfapi)
	}
	return DrDLL
}

// Exists 判断所给路径文件/文件夹是否存在
func Exists(path string) bool {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsExist(err) {
			return true
		}
		return false
	}
	return true

}

// Wow64DisableWow64FsRedirection 禁用调用线程的文件系统重定向，默认情况下启用文件系统重定向。此功能对于想要访问本机system32目录的32位应用程序很有用。
func Wow64DisableWow64FsRedirection() uintptr {
	var oldValue C.PVOID
	success := C.disableWow64FsRedirection(&oldValue)
	if success == 0 {
		fmt.Println("禁用文件系统重定向 失败")
		return 0
	}
	return uintptr(oldValue)
}

// Wow64RevertWow64FsRedirection 恢复调用线程的文件系统重定向。
func Wow64RevertWow64FsRedirection(oldValue uintptr) bool {
	success := 0
	if oldValue == 0 {
		var oldValues C.PVOID
		success = int(C.revertWow64FsRedirection(oldValues))
	} else {
		success = int(C.revertWow64FsRedirection(C.PVOID(oldValue)))
	}
	if success == 0 {
		fmt.Println("恢复文件系统重定向 失败")
		return false
	}

	return true
}
func WriteFile(path string, data []byte) {
	if checkFileIsExist(path) {
		err := os.Remove(path)
		if err != nil {
			return
		}
	}
	f, err1 := os.Create(path) //创建文件
	if err1 == nil {
		_, err1 = f.Write(data)
		if err1 != nil {

			return
		}
		err1 = f.Close()
		if err1 != nil {

			return
		}
	} else {
		if err1 != nil {
			return
		}
	}
}
func checkFileIsExist(filename string) bool {
	var exist = true
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		exist = false
	}
	return exist
}
func ExecCommand(commandName string, params []string) string {
	cmd := exec.Command(commandName, params...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err.Error()
	}
	if runtime.GOOS == "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	}
	_ = cmd.Start()
	var s []byte
	reader := bufio.NewReader(stdout)
	for {
		line, err2 := reader.ReadBytes('\n')
		if err2 != nil || io.EOF == err2 {
			break
		}
		s = public.BytesCombine(s, line)
	}
	return string(s)
}

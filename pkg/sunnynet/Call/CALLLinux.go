//go:build !windows
// +build !windows

package Call

/*
#include <stdlib.h>
#include "LinuxCall.h"
*/
import "C"
import (
	"unsafe"
)

func Call(address int, arg ...interface{}) int {
	if address < 10 {
		return 0
	}
	var args []uintptr
	var Frees []*C.char
	for _, name := range arg {
		switch val := name.(type) {
		case uintptr:
			args = append(args, val)
		case int:
			args = append(args, uintptr(val))
		case int8:
			args = append(args, uintptr(val))
		case int16:
			args = append(args, uintptr(val))
		case int32:
			args = append(args, uintptr(val))
		case int64:
			args = append(args, uintptr(val))
		case bool:
			if val {
				args = append(args, uintptr(1))
			} else {
				args = append(args, uintptr(0))
			}
		case string:
			n := C.CString(val)
			Frees = append(Frees, n)
			args = append(args, uintptr(unsafe.Pointer(n)))
		case []byte:
			n := C.CString(string(val))
			Frees = append(Frees, n)
			args = append(args, uintptr(unsafe.Pointer(n)))
		default:
			return -1 //如果有其他参数类型 直接报错返回
		}
	}
	Len := len(args)
	for index := 0; index < (18 - Len); index++ {
		args = append(args, uintptr(0))
	}
	var ret = uintptr(0)
	ch <- true
	addr := unsafe.Pointer(uintptr(address))
	switch Len {
	case 0:
		ret = uintptr(C.LinuxCall0(addr))
		break
	case 1:
		ret = uintptr(C.LinuxCall1(addr, unsafe.Pointer(args[0])))
		break
	case 2:
		ret = uintptr(C.LinuxCall2(addr, unsafe.Pointer(args[0]), unsafe.Pointer(args[1])))
		break
	case 3:
		ret = uintptr(C.LinuxCall3(addr, unsafe.Pointer(args[0]), unsafe.Pointer(args[1]), unsafe.Pointer(args[2])))
		break
	case 4:
		ret = uintptr(C.LinuxCall4(addr, unsafe.Pointer(args[0]), unsafe.Pointer(args[1]), unsafe.Pointer(args[2]), unsafe.Pointer(args[3])))
		break
	case 5:
		ret = uintptr(C.LinuxCall5(addr, unsafe.Pointer(args[0]), unsafe.Pointer(args[1]), unsafe.Pointer(args[2]), unsafe.Pointer(args[3]), unsafe.Pointer(args[4])))
		break
	case 6:
		ret = uintptr(C.LinuxCall6(addr, unsafe.Pointer(args[0]), unsafe.Pointer(args[1]), unsafe.Pointer(args[2]), unsafe.Pointer(args[3]), unsafe.Pointer(args[4]), unsafe.Pointer(args[5])))
		break
	case 7:
		ret = uintptr(C.LinuxCall7(addr, unsafe.Pointer(args[0]), unsafe.Pointer(args[1]), unsafe.Pointer(args[2]), unsafe.Pointer(args[3]), unsafe.Pointer(args[4]), unsafe.Pointer(args[5]), unsafe.Pointer(args[6])))
		break
	case 8:
		ret = uintptr(C.LinuxCall8(addr, unsafe.Pointer(args[0]), unsafe.Pointer(args[1]), unsafe.Pointer(args[2]), unsafe.Pointer(args[3]), unsafe.Pointer(args[4]), unsafe.Pointer(args[5]), unsafe.Pointer(args[6]), unsafe.Pointer(args[7])))
		break
	case 9:
		ret = uintptr(C.LinuxCall9(addr, unsafe.Pointer(args[0]), unsafe.Pointer(args[1]), unsafe.Pointer(args[2]), unsafe.Pointer(args[3]), unsafe.Pointer(args[4]), unsafe.Pointer(args[5]), unsafe.Pointer(args[6]), unsafe.Pointer(args[7]), unsafe.Pointer(args[8])))
		break
	case 10:
		ret = uintptr(C.LinuxCall10(addr, unsafe.Pointer(args[0]), unsafe.Pointer(args[1]), unsafe.Pointer(args[2]), unsafe.Pointer(args[3]), unsafe.Pointer(args[4]), unsafe.Pointer(args[5]), unsafe.Pointer(args[6]), unsafe.Pointer(args[7]), unsafe.Pointer(args[8]), unsafe.Pointer(args[9])))
		break
	default:
		return -1
	}
	<-ch
	for index := 0; index < len(Frees); index++ {
		C.free(unsafe.Pointer(Frees[index]))
	}
	return int(ret)
}

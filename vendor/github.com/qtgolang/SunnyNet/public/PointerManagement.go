package public

/*
#include <stdio.h>
#include <stdlib.h>
*/
import "C"
import (
	"bytes"
	"unsafe"
)

func Free(id uintptr) {
	if id < 1 {
		return
	}
	C.free(unsafe.Pointer(id))
}
func PointerPtr(data interface{}) uintptr {
	var b bytes.Buffer
	switch v := data.(type) {
	case string:
		b.WriteString(v)
		break
	case []byte:
		b.Write(v)
		break
	default:
		panic(nil)
	}
	b.WriteByte(0)
	bs := CopyBytes(b.Bytes())
	if len(bs) < 1 {
		return 0
	}
	Cs := C.CString(string(bs))
	return uintptr(unsafe.Pointer(Cs))
}

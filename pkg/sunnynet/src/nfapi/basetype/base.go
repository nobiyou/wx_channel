// Applicable to unaligned structures
package basetype

import (
	"encoding/binary"
	"fmt"
	"unsafe"
)

var hostByteOrder binary.ByteOrder

func init() {
	var i int32 = 0x01020304
	if *(*byte)(unsafe.Pointer(&i)) == 0x04 {
		hostByteOrder = binary.LittleEndian
	} else {
		hostByteOrder = binary.BigEndian
	}
}

func printAsBinary(bytes []byte) {

	for i := 0; i < len(bytes); i++ {
		for j := 0; j < 8; j++ {
			zeroOrOne := bytes[i] >> (7 - j) & 1
			fmt.Printf("%c", '0'+zeroOrOne)
		}
		fmt.Printf(" %p\n", &bytes[i])
	}
	fmt.Println()
}

type INT16 [2]byte

// Get by hostByteOrder
func (i *INT16) Get() int16 {
	return int16(hostByteOrder.Uint16(i[:]))
}
func (i *INT16) Set(in int16) {
	hostByteOrder.PutUint16(i[:], uint16(in))
}
func (i INT16) LittleEndianGet() int16 {
	return int16(binary.LittleEndian.Uint16(i[:]))
}
func (i *INT16) LittleEndianSet(in int16) {
	binary.LittleEndian.PutUint16(i[:], uint16(in))
}
func (i INT16) BigEndianGet() int16 {
	return int16(binary.BigEndian.Uint16(i[:]))
}
func (i *INT16) BigEndianSet(in int16) {
	binary.BigEndian.PutUint16(i[:], uint16(in))
}

type INT32 [4]byte

// Get by hostByteOrder
func (i *INT32) Get() int32 {
	return int32(hostByteOrder.Uint32(i[:]))
}
func (i *INT32) Set(in int32) {
	hostByteOrder.PutUint32(i[:], uint32(in))
}
func (i *INT32) LittleEndianGet() int32 {
	return int32(binary.LittleEndian.Uint32(i[:]))
}
func (i *INT32) LittleEndianSet(in int32) {
	binary.LittleEndian.PutUint32(i[:], uint32(in))
}
func (i *INT32) BigEndianGet() int32 {
	return int32(binary.BigEndian.Uint32(i[:]))
}
func (i *INT32) BigEndianSet(in int32) {
	binary.BigEndian.PutUint32(i[:], uint32(in))
}

type UINT16 [2]byte

// Get by hostByteOrder
func (i *UINT16) Get() uint16 {
	return hostByteOrder.Uint16(i[:])
}
func (i *UINT16) Set(in uint16) {
	hostByteOrder.PutUint16(i[:], in)
}
func (i *UINT16) LittleEndianGet() uint16 {
	return binary.LittleEndian.Uint16(i[:])
}
func (i *UINT16) LittleEndianSet(in uint16) {
	binary.LittleEndian.PutUint16(i[:], in)
}
func (i *UINT16) BigEndianGet() uint16 {
	return binary.BigEndian.Uint16(i[:])
}
func (i *UINT16) BigEndianSet(in uint16) {
	binary.BigEndian.PutUint16(i[:], in)
}

type UINT32 [4]byte

// Get by hostByteOrder
func (i *UINT32) Get() uint32 {
	return hostByteOrder.Uint32(i[:])
}
func (i *UINT32) Set(in uint32) {
	hostByteOrder.PutUint32(i[:], in)
}

func (i *UINT32) LittleEndianGet() uint32 {
	return binary.LittleEndian.Uint32(i[:])
}
func (i *UINT32) LittleEndianSet(in uint32) {
	binary.LittleEndian.PutUint32(i[:], in)
}

func (i *UINT32) BigEndianGet() uint32 {
	return binary.BigEndian.Uint32(i[:])
}
func (i *UINT32) BigEndianSet(in uint32) {
	binary.BigEndian.PutUint32(i[:], in)
}

type UINT64 [8]byte

// Get by hostByteOrder
func (i *UINT64) Get() uint64 {
	return hostByteOrder.Uint64(i[:])
}
func (i *UINT64) Set(in uint64) {
	hostByteOrder.PutUint64(i[:], in)
}
func (i *UINT64) LittleEndianGet() uint64 {
	return binary.LittleEndian.Uint64(i[:])
}
func (i *UINT64) LittleEndianSet(in uint64) {
	binary.LittleEndian.PutUint64(i[:], in)
}

func (i *UINT64) BigEndianGet() uint64 {
	return binary.BigEndian.Uint64(i[:])
}
func (i *UINT64) BigEndianSet(in uint64) {
	binary.BigEndian.PutUint64(i[:], in)
}

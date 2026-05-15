package GoWinHttp

import (
	"net"
)

type MyConn struct {
	net.Conn
	hook   func(b []byte)
	inject func(Conn net.Conn, b []byte, hook func(b []byte)) (int, error)
	state  byte
}

var AUTOCRLF = []byte("\r\n\r\n")

func validHeaderValueByte(c byte) bool {
	// mask is a 128-bit bitmap with 1s for allowed bytes,
	// so that the byte c can be tested with a shift and an and.
	// If c >= 128, then 1<<c and 1<<(c-64) will both be zero.
	// Since this is the obs-text range, we invert the mask to
	// create a bitmap with 1s for disallowed bytes.
	const mask = 0 |
		(1<<(0x7f-0x21)-1)<<0x21 | // VCHAR: %x21-7E
		1<<0x20 | // SP: %x20
		1<<0x09 // HTAB: %x09
	return ((uint64(1)<<c)&^(mask&(1<<64-1)) |
		(uint64(1)<<(c-64))&^(mask>>64)) == 0
}

func (c *MyConn) Read(b []byte) (int, error) {
	return c.inject(c.Conn, b, c.hook)
	/*
		if c.state == 1 {
			n, err := c.Conn.Read(b)
			c.hook(b[0:n])
			return n, err
		}
		i := cap(b)
		bs := make([]byte, i)
		n, err := c.Conn.Read(bs)
		bs1 := bs[0:n]
		c.hook(bs1)
		var input bytes.Buffer
		input.Reset()
		//因为返回协议头不重要，在c.hook会重写Header,所以这里检查协议头中如果有乱码，讲乱码设置为 ";"号
		crlfIndex := bytes.Index(bs1, AUTOCRLF)
		if crlfIndex != -1 {
			c.state = 1
			for k, v := range bs1 {
				if k >= crlfIndex {
					break
				}
				if !validHeaderValueByte(v) {
					bs1[k] = 59
				}
			}
		} else {
			for k, v := range bs1 {
				if !validHeaderValueByte(v) {
					bs1[k] = 59
				}
			}
		}
		input.Write(bs1)
		n, _ = input.Read(b)
		return n, err
	*/
}

func (c *MyConn) Write(b []byte) (int, error) {
	return c.Conn.Write(b)
}

func (c *MyConn) Close() error {
	if c.Conn == nil {
		return nil
	}
	return c.Conn.Close()
}

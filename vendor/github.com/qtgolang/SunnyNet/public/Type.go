// Package public /*
/*

									 Package public
------------------------------------------------------------------------------------------------
                                   程序所用到的所有公共类型及接口
------------------------------------------------------------------------------------------------
*/
package public

import (
	"bufio"
	"bytes"
	"github.com/qtgolang/SunnyNet/src/websocket"
	"io"
	"net"
	"sync"
)

type WebsocketMsg struct {
	Data    bytes.Buffer
	Server  *websocket.Conn
	Client  *websocket.Conn
	Mt      int
	Sync    *sync.Mutex
	tcp     net.Conn //TCP相关
	TcpIp   string   //TCP相关
	TcpUser string   //TCP相关
	TcpPass string   //TCP相关
}

type TcpMsg struct {
	Data    bytes.Buffer
	TcpIp   string //TCP相关
	TcpUser string //TCP相关
	TcpPass string //TCP相关
}

type TCP struct {
	Send       *TcpMsg
	Receive    *TcpMsg
	L          sync.Mutex
	ConnSend   net.Conn
	ConnServer net.Conn
	SendBw     *bufio.Writer
	ReceiveBw  *bufio.Writer
}

// ReadWriteObject 数据读写流
type ReadWriteObject struct {
	*bufio.ReadWriter
}

func (w *ReadWriteObject) Write(b []byte) (nn int, err error) {
	i, e := w.Writer.Write(b)
	e = w.Writer.Flush()
	return i, e
}
func (w *ReadWriteObject) WriteString(b string) (nn int, err error) {
	i, e := w.Writer.Write([]byte(b))
	e = w.Flush()
	return i, e
}

// ZlibCompress 主要用于Zlib 压缩
type ZlibCompress struct {
	io.Writer
	b bytes.Buffer
}

func (w *ZlibCompress) Write(p []byte) (n int, err error) {
	return w.b.Write(p)
}
func (w *ZlibCompress) Bytes() []byte {
	return w.b.Bytes()
}
func (w *ZlibCompress) Close() {
	w.b.Reset()
}

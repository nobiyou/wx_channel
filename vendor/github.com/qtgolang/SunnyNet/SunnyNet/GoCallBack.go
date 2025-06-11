package SunnyNet

import (
	"bytes"
	"github.com/qtgolang/SunnyNet/public"
	"github.com/qtgolang/SunnyNet/src/GoWinHttp"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

type TcpConn struct {
	SunnyContext int
	Theology     int //唯一ID
	MessageId    int
	c            *public.TcpMsg //事件消息
	Type         int            //事件类型_ 例如  public.SunnyNetMsgTypeTCP.....
	LocalAddr    string         //本地地址
	RemoteAddr   string         //远程地址
	Pid          int            //Pid
}

// SetAgent Set仅支持S5代理 例如 socket5://admin:123456@127.0.0.1:8888
func (k *TcpConn) SetAgent(ProxyUrl string) bool {
	if k.Type != public.SunnyNetMsgTypeTCPAboutToConnect {
		return false
	}
	if k.c == nil {
		return false
	}
	k.c.TcpIp = public.NULL
	k.c.TcpUser = public.NULL
	k.c.TcpPass = public.NULL
	proxy, err := url.Parse(ProxyUrl)
	if err != nil || proxy == nil {
		return false
	}
	if proxy.Scheme != "socket5" && proxy.Scheme != "socket" {
		return false
	}
	if len(proxy.Host) < 3 {
		return false
	}
	k.c.TcpIp = proxy.Host
	k.c.TcpUser = proxy.User.Username()
	p, ok := proxy.User.Password()
	if ok {
		k.c.TcpPass = p
	}
	return true
}

// SetBody 修改 TCP/发送接收数据
func (k *TcpConn) SetBody(data []byte) bool {
	if k.Type != public.SunnyNetMsgTypeTCPClientReceive && k.Type != public.SunnyNetMsgTypeTCPClientSend {
		return false
	}
	k.c.Data.Reset()
	k.c.Data.Write(data)
	return true
}

// Close 关闭TCP连接
func (k *TcpConn) Close() bool {
	if k.Type == public.SunnyNetMsgTypeTCPAboutToConnect {
		return false
	}
	TcpSceneLock.Lock()
	w := TcpStorage[k.Theology]
	TcpSceneLock.Unlock()
	if w == nil {
		return false
	}
	w.L.Lock()
	_ = w.ConnSend.Close()
	_ = w.ConnServer.Close()
	w.L.Unlock()
	return true
}

// SetConnectionIP 修改目标连接地址 目标地址必须带端口号 例如 baidu.com:443 [仅限即将连接时使用]
func (k *TcpConn) SetConnectionIP(ip string) bool {
	if k.Type == public.SunnyNetMsgTypeTCPAboutToConnect {
		k.c.Data.Reset()
		k.c.Data.WriteString(ip)
		return true
	}
	return false
}

// SendToServer 模拟客户端向服务器端主动发送数据
func (k *TcpConn) SendToServer(data []byte) int {
	TcpSceneLock.Lock()
	w := TcpStorage[k.Theology]
	TcpSceneLock.Unlock()
	if w == nil {
		return 0
	}
	if w.Send == nil {
		return 0
	}
	w.L.Lock()
	defer w.L.Unlock()
	if len(data) > 0 {
		x, e := w.ReceiveBw.Write(data)
		if e == nil {
			_ = w.ReceiveBw.Flush()
		}
		return x
	}
	return 0
}

// SendToClient  模拟服务器端向客户端主动发送数据
func (k *TcpConn) SendToClient(data []byte) int {
	TcpSceneLock.Lock()
	w := TcpStorage[k.Theology]
	TcpSceneLock.Unlock()
	if w == nil {
		return 0
	}
	if w.Receive == nil {
		return 0
	}
	if len(data) > 0 {
		w.L.Lock()
		defer w.L.Unlock()
		x, e := w.SendBw.Write(data)
		if e == nil {
			_ = w.SendBw.Flush()
		}
		return x
	}
	return 0
}

// GetBody  获取发送、接收的数据
func (k *TcpConn) GetBody() []byte {
	if k == nil {
		return []byte{}
	}
	if k.c == nil {
		return []byte{}
	}
	return public.CopyBytes(k.c.Data.Bytes())
}

// GetBodyLen  获取发送、接收的数据长度
func (k *TcpConn) GetBodyLen() int {
	if k == nil {
		return 0
	}
	if k.c == nil {
		return 0
	}
	return k.c.Data.Len()
}

type WsConn struct {
	c            *public.WebsocketMsg
	SunnyContext int
	MessageId    int           //仅标识消息ID,不能用于API函数
	Pid          int           //Pid
	Type         int           //消息类型 	public.Websocket...
	Url          string        //连接请求地址
	Theology     int           //请求唯一ID
	ClientIP     string        //来源IP地址,请求从哪里来
	Request      *http.Request //请求体
}

// GetMessageBody 获取 消息消息
func (k *WsConn) GetMessageBody() []byte {
	k.c.Sync.Lock()
	defer k.c.Sync.Unlock()
	return public.CopyBytes(k.c.Data.Bytes())
}

// GetMessageType 获取 消息类型
// Text=1 Binary=2 Close=8 Ping=9 Pong=10 Invalid=-1/255
func (k *WsConn) GetMessageType() int {
	k.c.Sync.Lock()
	defer k.c.Sync.Unlock()
	return k.c.Mt
}

// GetMessageBodyLen 获取 消息长度
func (k *WsConn) GetMessageBodyLen() int {
	k.c.Sync.Lock()
	defer k.c.Sync.Unlock()
	return k.c.Data.Len()
}

// SetMessageBody 修改 消息
func (k *WsConn) SetMessageBody(data []byte) bool {
	k.c.Sync.Lock()
	defer k.c.Sync.Unlock()
	k.c.Data.Reset()
	k.c.Data.Write(data)
	return true
}

// SendToServer 主动向Websocket服务器发送消息
func (k *WsConn) SendToServer(MessageType int, data []byte) bool {
	k.c.Sync.Lock()
	defer k.c.Sync.Unlock()
	if k.c.Server != nil {
		e := k.c.Server.WriteMessage(MessageType, data)
		if e != nil {
			return false
		}
	}
	return true
}

// SendToClient 主动向Websocket客户端发送消息
func (k *WsConn) SendToClient(MessageType int, data []byte) bool {
	k.c.Sync.Lock()
	defer k.c.Sync.Unlock()
	if k.c.Client != nil {
		e := k.c.Client.WriteMessage(MessageType, data)
		if e != nil {
			return false
		}
	}
	return true
}

// Close 关闭Websocket连接
func (k *WsConn) Close() bool {
	k.c.Sync.Lock()
	defer k.c.Sync.Unlock()
	if k.c.Server != nil {
		_ = k.c.Server.Close()
	}
	if k.c.Client != nil {
		_ = k.c.Client.Close()
	}
	return true
}

type HttpConn struct {
	SunnyContext int
	Theology     int //唯一ID
	MessageId    int //消息ID,仅标识消息ID,不能用于API函数
	PID          int
	Type         int              //请求类型 例如 public.HttpSendRequest  public.Http....
	ClientIP     string           //来源IP地址,请求从哪里来
	Request      *http.Request    //请求体
	Response     *http.Response   //响应体
	err          string           //错误信息
	proxy        *GoWinHttp.Proxy //代理信息
}

// StopRequest 阻止请求,仅支持在发起请求时使用
// StatusCode要响应的状态码
// Data=要响应的数据 可以是string 也可以是[]byte
// Header=要响应的Header 可以忽略
func (h *HttpConn) StopRequest(StatusCode int, Data any, Header ...http.Header) {
	if h.Type != public.HttpSendRequest {
		return
	}
	var ResponseData []byte
	switch v := Data.(type) {
	case string:
		ResponseData = []byte(v)
		break
	case []byte:
		ResponseData = v
		break
	default:
		return
	}
	h.Response = new(http.Response)
	if StatusCode < 100 {
		h.Response.StatusCode = 200
	} else {
		h.Response.StatusCode = StatusCode
	}
	h.Response.Body = io.NopCloser(bytes.NewBuffer(ResponseData))
	if len(Header) > 0 {
		h.Response.Header = Header[0]
	}
	if h.Response.Header == nil {
		h.Response.Header = make(http.Header)
		h.Response.Header.Set("Server", "Sunny")
		h.Response.Header.Set("Accept-Ranges", "bytes")
		h.Response.Header.Set("Connection", "Close")
	}
	h.Response.Header.Set("Content-Length", strconv.Itoa(len(ResponseData)))
	h.Response.ContentLength = int64(len(ResponseData))
}
func (h *HttpConn) GetError() string {
	return h.err
}

// SetAgent 设置HTTP/S请求代理，仅支持Socket5和http 例如 socket5://admin:123456@127.0.0.1:8888 或 http://admin:123456@127.0.0.1:8888
func (h *HttpConn) SetAgent(ProxyUrl string) bool {
	if h.Type != public.HttpSendRequest {
		return false
	}
	if h.proxy == nil {
		return false
	}
	h.proxy.S5TypeProxy = false
	h.proxy.Address = public.NULL
	h.proxy.User = public.NULL
	h.proxy.Pass = public.NULL
	proxy, err := url.Parse(ProxyUrl)
	h.proxy.S5TypeProxy = false
	h.proxy.Address = ""
	h.proxy.User = ""
	h.proxy.Pass = ""
	if err != nil || proxy == nil {
		return false
	}
	if proxy.Scheme != "http" && proxy.Scheme != "socket5" && proxy.Scheme != "socket" && proxy.Scheme != "socks5" {
		return false
	}
	h.proxy.S5TypeProxy = proxy.Scheme != "http"
	if len(proxy.Host) < 3 {
		return false
	}
	h.proxy.Address = proxy.Host
	h.proxy.User = proxy.User.Username()
	p, o := proxy.User.Password()
	if o {
		h.proxy.Pass = p
	}
	return true
}

type UDPConn struct {
	SunnyContext  int
	Theology      int64 //唯一ID
	MessageId     int   //消息ID
	Type          int8  //请求类型 例如 public.SunnyNetUDPType...
	Pid           int
	LocalAddress  string
	RemoteAddress string
	Data          []byte
}

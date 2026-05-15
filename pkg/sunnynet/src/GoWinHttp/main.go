/*
	Package GoWinHttp

------------------------------------------------------------------------------------------------

	By：秦天 -> 2022/11/01

---------------------------------------- GoWinHttp ---------------------------------------------
GoWinHttp 是根据 WinHttp 特性仿制的

	同一个请求访问之后,底层TCP不会端口连接
	下次向相同地址发送数据直接发送,不用再次建立连接
	优点 1.节省了建立连接的时间,2.如果是HTTPS请求也节省了握手的时间,3.对于大量对同一地址进行请求有很好的效果
	缺点 可能增加内存开销,建议修改 自动清理时间来优化

特性-相当于自带TCP连接池。
Go自带的http (不具备获取conn功能,因为长链接得手动设置超时)网上可能有其他的,但是我没找到，就自己封装了一下，可能有BUG
其他语言的请求底层都有这个功能(测试方法很简单，连续请求10次，记录请求时间,你会发现第一次慢，后面几次的就快了)
还有就是Go自带的HTTP请求/响应 对象,会自动处理协议头大小写,必须要 不处理 大小写，不然客户端可能会异常
------------------------------------------------------------------------------------------------

	！！！如果发现 GoWinHttp 不稳定，或者 有BUG，有能力的自行修复，或者替换为官方的http库！！！！

------------------------------------------------------------------------------------------------

	By：秦天 -> 2022/11/01

------------------------------------------------------------------------------------------------
*/
package GoWinHttp

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io/ioutil"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"
)

var dnsLock sync.Mutex
var dnsMap = make(map[string]*IpValue)

type IpValue struct {
	Value string
	Time  time.Time
}
type WinHttp struct {
	request           *http.Request
	Response          *http.Response
	Proxy             *Proxy
	ReadTimeout       time.Duration //读取超时
	ConnectionTimeout time.Duration //连接超时
	SendTimeout       time.Duration
	location          int
	cfg               *tls.Config
	WinPool           *PoolInfo
	PoolName          string
	GetTLSValues      func() []uint16
}

type Proxy struct {
	S5TypeProxy bool
	Address     string //127.0.0.1:8888
	User        string
	Pass        string
	Timeout     int //毫秒
}
type PoolInfo struct {
	Conn      net.Conn
	Time      time.Time
	Bw        *bufio.Reader
	ConnHeads http.Header
	prohibit  bool
	_h        bytes.Buffer
	_hh       http.Header
	ib        bool
	state     bool
}

func (e *PoolInfo) inject(Conn net.Conn, b []byte, hook func(b []byte)) (int, error) {
	if e.state {
		n, err := Conn.Read(b)
		hook(b[0:n])
		return n, err
	}
	i := cap(b)
	bs := make([]byte, i)
	n, err := Conn.Read(bs)
	bs1 := bs[0:n]
	hook(bs1)
	var input bytes.Buffer
	input.Reset()
	//因为返回协议头不重要，在 hook 函数会重写Header,所以这里检查协议头中如果有乱码，讲乱码设置为 ";"号
	crlfIndex := bytes.Index(bs1, AUTOCRLF)
	if crlfIndex != -1 {
		e.state = true
		for k, v := range bs1 {
			if k >= crlfIndex {
				break
			}
			if v == 13 || v == 10 {
				continue
			}
			if !validHeaderValueByte(v) {
				bs1[k] = 59
			}
		}
	} else {
		for k, v := range bs1 {
			if v == 13 || v == 10 {
				continue
			}
			if !validHeaderValueByte(v) {
				bs1[k] = 59
			}
		}
	}
	input.Write(bs1)
	n, _ = input.Read(b)
	return n, err
}
func (e *PoolInfo) clearHeads() {
	e._h.Reset() // 重置用于存储头部数据的缓冲区
	e._hh = nil  // 将头部字典设置为 nil
}
func (e *PoolInfo) SetHeads(b []byte) {
	if e.ib == true { // 检查 PoolInfo 实例是否被标记为无效
		return // 如果是，直接返回
	}
	e._h.Write(b)                           // 将字节切片写入缓冲区
	m := e._h.Bytes()                       // 获取缓冲区的字节切片
	y := bytes.Index(m, []byte("\r\n\r\n")) // 查找字节切片中第一个 "\r\n\r\n" 的位置
	if y != -1 {                            // 如果找到了
		e._hh = make(http.Header)                               // 创建一个新的 http.Header 实例
		arr := strings.Split(string(CopyBytes(m[0:y])), "\r\n") // 将头部数据按行分割成字符串切片
		e._h.Reset()                                            // 重置缓冲区
		for _, v := range arr {                                 // 遍历每一行头部数据
			arr2 := strings.Split(v, ":") // 将该行数据按冒号分割成键值对
			if len(arr2) >= 1 {           // 如果键值对长度大于等于 1
				if len(v) >= len(arr2[0])+1 { // 如果该行数据长度大于等于键的长度加 1
					da := strings.TrimSpace(v[len(arr2[0])+1:]) // 提取值并去除首尾空格
					Name := arr2[0]
					//这些协议头强制大写 MIME 风格
					if Name == "content-length" || Name == "content-type" || Name == "cache-control" ||
						Name == "expires" || Name == "pragma" || Name == "accept" ||
						Name == "content-disposition" || Name == "transfer-encoding" {
						Name = textproto.CanonicalMIMEHeaderKey(Name)
						if len(e._hh[Name]) > 0 {
							continue
						}
					}
					if len(e._hh[Name]) > 0 { // 如果该键已经存在
						e._hh[Name] = append(e._hh[Name], da) // 将该值追加到该键对应的值列表中
					} else { // 否则
						e._hh[Name] = []string{da} // 创建一个只包含该值的值列表，并将其赋给该键
					}
				}
			}
		}
		e.ib = true // 标记 PoolInfo 实例为有效
	}
}
func (e *PoolInfo) SetReadDeadline(time time.Time) error {
	return e.Conn.SetReadDeadline(time)
}
func (e *PoolInfo) SetDeadline(time time.Time) error {
	return e.Conn.SetDeadline(time)
}
func (e *PoolInfo) SetWriteDeadline(time time.Time) error {
	return e.Conn.SetWriteDeadline(time)
}
func (e *PoolInfo) Read(p []byte) (int, error) {
	return e.Bw.Read(p)
}
func (e *PoolInfo) Write(p []byte) (int, error) {
	return e.Conn.Write(p)
}

var WinPool = func() *Pool {
	k := &Pool{}
	k.MAP = make(map[string]map[int]*PoolInfo)
	return k
}()

type Pool struct {
	MAP  map[string]map[int]*PoolInfo
	Lock sync.Mutex
}

func init() {
	go WinPool.Release()
}
func (l *Pool) Release() {
	for {
		time.Sleep(5 * time.Second)
		l.Lock.Lock()
		for k, v := range l.MAP {
			for kk, vv := range v {
				if time.Now().Sub(vv.Time) > 5*time.Second {
					if vv != nil {
						vv.clearHeads()
						if vv.Conn != nil {
							_ = vv.Conn.Close()
						}
						vv.Conn = nil
						vv.Bw = nil
						vv = nil
					}
					delete(v, kk)
					if len(v) == 0 {
						delete(l.MAP, k)
					}

				}
			}
		}
		l.Lock.Unlock()
	}
}

func (l *Pool) Put(Name string, value *PoolInfo) {
	if value.prohibit {
		return
	}
	if value != nil {
		l.Lock.Lock()
		if l.MAP[Name] == nil {
			l.MAP[Name] = make(map[int]*PoolInfo)
		}
		l.MAP[Name][int(uintptr(unsafe.Pointer(&value)))] = value
		l.Lock.Unlock()
	}
}
func (l *Pool) GET(Name string) *PoolInfo {
	l.Lock.Lock()
	ar := l.MAP[Name]
	for k, v := range ar {
		delete(l.MAP[Name], k)
		if len(l.MAP[Name]) == 0 {
			delete(l.MAP, Name)
		}
		l.Lock.Unlock()
		return v
	}
	l.Lock.Unlock()
	return nil
}

/*
ConnectS5 将已经连接到的S5的TCP连接进行发送S5必要的协议信息

参数

	c：			已经连接到的S5的TCP连接
	Proxy：		代理信息
	Host：		要连接的目标地址
	PortNum：	要连接的目标端口号

返回值

	认证通过返回 True 若S5账号或密码错返回 False
*/
func ConnectS5(c *net.Conn, Proxy *Proxy, Host string, PortNum uint16) bool {
	var bm bytes.Buffer
	var bs = make([]byte, 128)
	if Proxy.User != "" {
		_, _ = (*c).Write([]byte{5, 1, 2})
	} else {
		_, _ = (*c).Write([]byte{5, 1, 0})
	}
	i, _ := (*c).Read(bs)
	if i < 2 {
		return false
	}
	if bs[1] == 0 {
		//无需验证
	} else if bs[1] == 2 {
		//需要账号密码验证
		if Proxy.User == "" {
			return false
		}
		bm.WriteByte(1)
		bm.WriteByte(byte(len(Proxy.User)))
		bm.WriteString(Proxy.User)
		bm.WriteByte(byte(len(Proxy.Pass)))
		bm.WriteString(Proxy.Pass)
		_, _ = (*c).Write(bm.Bytes())
		i, _ = (*c).Read(bs)
		if i < 2 {
			return false
		}
	} else {
		return false
	}
	bm.Reset()
	bm.Write([]byte{5, 1, 0, 3})
	bm.WriteByte(byte(len(Host)))
	bm.WriteString(Host)
	Port := make([]byte, 2)
	binary.BigEndian.PutUint16(Port, PortNum)
	bm.Write(Port)
	_, _ = (*c).Write(bm.Bytes())
	i, _ = (*c).Read(bs)
	if i < 2 {
		return false
	}
	bm.Reset()
	if bs[1] == 0 {
		if i < 10 {
			_ = (*c).SetDeadline(time.Now().Add(1000 * time.Millisecond))
			i, _ = (*c).Read(bs)
			_ = (*c).SetDeadline(time.Time{})
		}
		return true
	}
	return false
}

func getHost(u *url.URL) string {
	Port := u.Port()
	info := ""
	if u.Scheme == "https" {
		if Port == "" {
			info = u.Host + ":443"
		} else {
			info = u.Host
		}
	} else {
		if Port == "" {
			info = u.Host + ":80"
		} else {
			info = u.Host
		}
	}
	return info
}

func parseURI(u *url.URL, proxy *Proxy) string {
	us := u.String()

	if proxy != nil && proxy.Address != "" && u.Scheme == "http" && proxy.S5TypeProxy == false {
		return us
	}
	/*
		if u.Scheme == "https" {
			us = strings.Replace(us, u.Scheme+"://"+u.Host, "", 1)
			if us == "" {
				us = "/"
			}
		}
	*/
	us = strings.Replace(us, u.Scheme+"://"+u.Host, "", 1)
	if us == "" {
		us = "/"
	}
	return us
}
func IpDns(r string) string {
	dnsLock.Lock()
	defer dnsLock.Unlock()
	s := dnsMap[r]
	if s != nil {
		//30分钟 更新一次DNS解析
		if time.Now().Sub(s.Time) < time.Minute*30 {
			return s.Value
		}
	}
	u, _ := url.Parse("https://" + r)
	host := u.Hostname()
	ip, e := net.ResolveIPAddr("ip", host)
	if e != nil {
		return ""
	}
	if ip.IP == nil {
		return ""
	}
	Port := u.Port()
	if ip.IP.To4() == nil {
		s = &IpValue{Value: "[" + ip.IP.String() + "]:" + Port, Time: time.Now()}
	} else {
		s = &IpValue{Value: ip.IP.String() + ":" + Port, Time: time.Now()}
	}

	dnsMap[r] = s
	return s.Value
}

// SetProxyIP 设置代理地址 使用本命令就无需使用 SetProxy
func (w *WinHttp) SetProxyIP(Address string) *WinHttp {

	if w.Proxy == nil {
		w.Proxy = &Proxy{Address: Address, Timeout: 60 * 1000}
		return w
	}
	w.Proxy.Address = Address
	return w
}

// SetProxyUser 设置代理用户名,密码 使用本命令就无需使用 SetProxy
func (w *WinHttp) SetProxyUser(User, Pass string) *WinHttp {
	if w.Proxy == nil {
		w.Proxy = &Proxy{User: User, Pass: Pass, Timeout: 60 * 1000}
		return w
	}
	w.Proxy.User = User
	w.Proxy.Pass = Pass
	return w
}

// SetProxyType 设置代理类型 参数为是否S5代理 仅支持HTTP/Socket5类型代理 使用本命令就无需使用 SetProxy
func (w *WinHttp) SetProxyType(S5TypeProxy bool) *WinHttp {
	if w.Proxy == nil {
		w.Proxy = &Proxy{S5TypeProxy: S5TypeProxy, Timeout: 60 * 1000}
		return w
	}
	w.Proxy.S5TypeProxy = S5TypeProxy
	return w
}

// SetProxyTimeout 设置代理超时
func (w *WinHttp) SetProxyTimeout(Timeout int) *WinHttp {
	if w.Proxy == nil {
		w.Proxy = &Proxy{Timeout: 60 * 1000}
		return w
	}
	w.Proxy.Timeout = Timeout
	return w
}

// SetProxy 设置代理信息 使用本命令就无需使用 SetProxyIP SetProxyUser SetProxyType 命令
func (w *WinHttp) SetProxy(Proxy *Proxy) *WinHttp {
	w.Proxy = Proxy
	return w
}

// SetRedirect 设置是否允许重定向
func (w *WinHttp) SetRedirect(Location bool) *WinHttp {
	if Location {
		w.location = 10
	} else {
		w.location = 0
	}
	return w
}

// Open 设置请求连接信息 例如 method=GET _url=https://www.baidu.com
func (w *WinHttp) Open(method, _url string) *WinHttp {
	w.request, _ = http.NewRequest(strings.ToUpper(method), _url, nil)
	if w.request.Header == nil {
		w.request.Header = make(http.Header)
	}
	w.request.Header.Set("host", w.request.URL.Host)
	return w
}

// SetHeader 设置协议头
func (w *WinHttp) SetHeader(Name, Value string) *WinHttp {
	if w.request.Header[Name] == nil {
		w.request.Header[Name] = []string{Value}
	} else {
		w.request.Header[Name] = append(w.request.Header[Name], Value)
	}
	return w
}

// SetOutTime 设置超时
func (w *WinHttp) SetOutTime(readTimeout, connectionTimeout, sendTimeout int) *WinHttp {
	if readTimeout == 0 {
		readTimeout = 60 * 1000
	}
	if connectionTimeout == 0 {
		connectionTimeout = 10 * 1000
	}
	if sendTimeout == 0 {
		sendTimeout = 60 * 1000
	}
	w.ReadTimeout = time.Duration(readTimeout) * time.Millisecond
	w.ConnectionTimeout = time.Duration(connectionTimeout) * time.Millisecond
	w.SendTimeout = time.Duration(sendTimeout) * time.Millisecond
	return w
}

func cloneURL(u *url.URL) *url.URL {
	if u == nil {
		return nil
	}
	u2 := new(url.URL)
	*u2 = *u
	if u.User != nil {
		u2.User = new(url.Userinfo)
		*u2.User = *u.User
	}
	return u2
}
func cloneURLValues(v url.Values) url.Values {
	if v == nil {
		return nil
	}
	// http.Header and url.Values have the same representation, so temporarily
	// treat it like http.Header, which does have a clone:
	return url.Values(http.Header(v).Clone())
}

func cloneMultipartForm(f *multipart.Form) *multipart.Form {
	if f == nil {
		return nil
	}
	f2 := &multipart.Form{
		Value: (map[string][]string)(http.Header(f.Value).Clone()),
	}
	if f.File != nil {
		m := make(map[string][]*multipart.FileHeader)
		for k, vv := range f.File {
			vv2 := make([]*multipart.FileHeader, len(vv))
			for i, v := range vv {
				vv2[i] = cloneMultipartFileHeader(v)
			}
			m[k] = vv2
		}
		f2.File = m
	}
	return f2
}
func cloneMultipartFileHeader(fh *multipart.FileHeader) *multipart.FileHeader {
	if fh == nil {
		return nil
	}
	fh2 := new(multipart.FileHeader)
	*fh2 = *fh
	fh2.Header = textproto.MIMEHeader(http.Header(fh.Header).Clone())
	return fh2
}

func cloneRequest(r *http.Request) *http.Request {
	r2 := new(http.Request)
	*r2 = *r
	r2.URL = cloneURL(r.URL)
	if r.Header != nil {
		r2.Header = r.Header.Clone()
	}
	if r.Trailer != nil {
		r2.Trailer = r.Trailer.Clone()
	}
	if s := r.TransferEncoding; s != nil {
		s2 := make([]string, len(s))
		copy(s2, s)
		r2.TransferEncoding = s2
	}
	r2.Form = cloneURLValues(r.Form)
	r2.PostForm = cloneURLValues(r.PostForm)
	r2.MultipartForm = cloneMultipartForm(r.MultipartForm)
	return r2
}

// Do 直接发送*http.Request请求
func (w *WinHttp) Do(request *http.Request) (*http.Response, error) {
	w.request = cloneRequest(request)
	defer func() {
		if w.request != nil {
			if w.request.Body != nil {
				_ = w.request.Body.Close()
			}
		}
	}()
	var b []byte
	if request.Body != nil {
		b, _ = ioutil.ReadAll(request.Body)
		_ = request.Body.Close()
		request.Body = ioutil.NopCloser(bytes.NewBuffer(b))
	}
	return w.Send(b)
}

// Send 发送 参数可以为字节数组也可以为字符串
func (w *WinHttp) Send(data any) (_r *http.Response, _e error) {
	var resp *http.Response
	defer func() {
		if w.Response != nil && w.Response.Body != nil {
			_ = w.Response.Body.Close()
		}
		/*
			w.Response = _r
			if w.Response != nil {
				w.Response.Status = strconv.Itoa(w.Response.StatusCode) + " " + http.StatusText(w.Response.StatusCode)
			}
		*/
		if _e != nil {
			if w != nil {
				w.PoolName = ""
				if w.WinPool != nil {
					if w.WinPool.Conn != nil {
						_ = w.WinPool.Conn.Close()
					}
					w.WinPool.Conn = nil
					w.WinPool.Bw = nil
					w.WinPool.prohibit = true
				}
				w.Proxy = nil
			}

			if _r != nil {
				if _r.Body != nil {
					_ = _r.Body.Close()
					_r.Body = nil
				}
				_r = nil
			}
		}
	}()
	var ok bool
	var err error
	var IsNewConn bool
	for i := 0; i < 3; i++ {
		ok, IsNewConn, resp, err = w.connect()
		if resp != nil {
			return resp, nil
		}
		if ok {
			_ = w.WinPool.Conn.SetReadDeadline(time.Now().Add(1 * time.Millisecond))
			_, b := w.WinPool.Bw.Peek(1)
			if b != nil && strings.Index(b.Error(), "timeout") != -1 {
				if w.ReadTimeout == 0 {
					w.ReadTimeout = 60 * time.Second
				}
				if w.SendTimeout == 0 {
					w.SendTimeout = 60 * time.Second
				}
				break
			} else {
				if w.WinPool != nil {
					_ = w.WinPool.Conn.Close()
					w.WinPool.Conn = nil
					w.WinPool.Bw = nil
				}
				w.WinPool = nil
				err = errors.New("[Sunny] 连接超时... ")
				resp = nil
				ok = false
				if !IsNewConn {
					i--
				}
			}
		}
	}
	if resp != nil {
		return resp, nil
	}
	if !ok {
		return nil, err
	}
	_ = w.WinPool.Conn.SetWriteDeadline(time.Now().Add(w.SendTimeout))
	SendData := w.formatMsg(data)

	_, err = w.WinPool.Conn.Write(SendData)
	//SendData = make([]byte, 0)
	if err != nil {
		_ = w.WinPool.Conn.Close()
		return nil, err
	}
	_ = w.WinPool.Conn.SetDeadline(time.Now().Add(w.ReadTimeout))
	//由于 http.ReadResponse 会自动将 返回的协议头 转为 MIME-style格式。所以安装了一个钩子获取到Heads 字符串,我们自己重写解析
	w.WinPool.clearHeads()
	w.WinPool.state = false
	w.WinPool.ib = false
	ret, err := http.ReadResponse(w.WinPool.Bw, nil)
	if ret != nil {
		ret.Header = w.WinPool._hh
	}
	w.WinPool.clearHeads()
	if ret != nil {
		location := ret.Header["Location"]
		if w.location > 0 && len(location) > 0 {
			Ul := CopyString(location[0])
			if !strings.HasPrefix(Ul, "http") {
				if !strings.HasPrefix(Ul, "/") {
					Ul = w.request.URL.Scheme + "://" + w.request.URL.Host + "/" + Ul
				} else {
					Ul = w.request.URL.Scheme + "://" + w.request.URL.Host + Ul
				}
			}
			L, e := url.Parse(CopyString(Ul))
			if e != nil {
				return ret, nil
			}
			_ = ret.Body.Close()
			w.location--
			w.request.Method = "GET"
			w.request.URL = L
			return w.Send("")
		}
	}
	return ret, err
}

// formatMsg 格式化提交信息
func (w *WinHttp) formatMsg(data any) []byte {
	var vv []byte
	switch v := data.(type) {
	case string:
		vv = []byte(v)
		break
	case []byte:
		vv = v
		break
	default:
		break
	}
	var buff bytes.Buffer
	buff.WriteString(w.request.Method + " " + parseURI(w.request.URL, w.Proxy) + " HTTP/1.1\r\n")
	var lengthN = ""
	for k, v := range w.request.Header {
		if strings.ToLower(k) == "content-length" {
			lengthN = k
			continue
		}
		if strings.ToLower(k) == "host" {
			buff.WriteString(k + ": " + w.request.URL.Host + "\r\n")
			continue
		}
		if len(v) > 0 {
			for _, vvv := range v {
				buff.WriteString(k + ": " + vvv + "\r\n")
			}
		} else {
			buff.WriteString(k + ": \r\n")
		}
	}
	if w.Proxy != nil {
		if w.Proxy.S5TypeProxy == false {
			if w.Proxy.User != "" {
				buff.WriteString("Proxy-Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(w.Proxy.User+":"+w.Proxy.Pass)) + "\r\n")
			}
		}
	}
	if len(vv) != 0 || w.request.Method == "POST" || w.request.Method == "PUT" {
		if lengthN == "" {
			buff.WriteString("Content-Length: " + strconv.Itoa(len(vv)) + "\r\n")
		} else {
			buff.WriteString(lengthN + ": " + strconv.Itoa(len(vv)) + "\r\n")
		}

	}
	buff.WriteString("\r\n")
	if vv != nil {
		buff.Write(vv)
	}
	return CopyBytes(buff.Bytes())
}

// CopyBytes 拷贝 字节数组避免内存泄漏
func CopyBytes(src []byte) []byte {
	dst := make([]byte, len(src))
	copy(dst, src)
	return dst
}

// CopyString 拷贝字符串 避免内存泄漏
func CopyString(src string) string {
	dst := make([]byte, len(src))
	copy(dst, src)
	return string(dst)
}

func (w *WinHttp) Save() {
	if w == nil {
		return
	}
	//是否需要直接断开会话,不进行保存会话
	if w.PoolName != "" && w != nil && w.request != nil {
		if w.request.Header != nil {
			//如果请求体 中指定了短连接标识 (说明客户端不希望长连接) 则不进行保存会话
			if w.request.Header.Get("Connection") == "close" {
				//会话名称设置为空 一会进行断开连接操作
				w.PoolName = ""
			}
		}
		if w.PoolName != "" && w.Response != nil && w.Response.Header != nil {
			//如果响应体 中指定了短连接标识 (说明服务端不希望长连接) 则不进行保存会话
			if w.Response.Header.Get("Connection") == "close" {
				//会话名称设置为空 一会进行断开连接操作
				w.PoolName = ""
			}
		}
		if w.PoolName != "" && w.request.URL != nil {
			if w.request.URL.Scheme != "https" && w.WinPool != nil && w.WinPool.Conn != nil {
				//如果 请求体 和 响应体中都没有指定短连接标识 那么如果是 http 的请求 则都不进行保存会话
				w.PoolName = ""
			}
		}
	}
	if w.PoolName == "" {
		//进行断开会话，销毁会话，不进行储存会话
		w.Shutdown()
		return
	}
	if w.WinPool != nil {
		//释放响应体中的Body
		if w.Response != nil {
			if w.Response.Body != nil {
				_ = w.Response.Body.Close()
			}
			w.Response = nil
		}
		//释放请求头中的Body
		if w.request != nil {
			if w.request.Body != nil {
				_ = w.request.Body.Close()
			}
			w.request = nil
		}
		//储存会话
		WinPool.Put(w.PoolName, w.WinPool)
		w.WinPool = nil
		w.PoolName = ""
	}
}
func (w *WinHttp) Shutdown() {
	if w == nil {
		return
	}
	if w.WinPool != nil {
		w.Proxy = nil
		if w.WinPool.Conn != nil {
			_ = w.WinPool.Conn.Close()
		}
		w.WinPool.Conn = nil
		w.WinPool.Bw = nil
		w.WinPool = nil
	}
	w.cfg = nil
	w.PoolName = ""
	if w.request != nil {
		if w.request.Body != nil {
			_ = w.request.Body.Close()
		}
		w.request = nil
	}
	if w.Response != nil {
		if w.Response.Body != nil {
			_ = w.Response.Body.Close()
		}
		w.Response = nil
	}
}

// connect 建立连接
func (w *WinHttp) connect() (_a bool, _b bool, _c *http.Response, _d error) {
	defer func() {
		if r := recover(); r != nil {
			_a = false
			_b = false
			_c = nil
			switch err := r.(type) {
			case string:
				_d = errors.New(err)
			case error:
				_d = err
			default:
				_d = errors.New("WinHttp.connect Unknown panic")
			}
		}
	}()
	if w.request == nil {
		return false, true, nil, errors.New("Request Pointer is null ")
	}
	if w.request.URL == nil {
		return false, true, nil, errors.New("Request URL is null ")
	}
	host := getHost(w.request.URL)
	Info, _tls := w.getConnectInfo()
	if Info == "" {
		return false, true, nil, errors.New("target address is null ")
	}
	w.PoolName = host + Info
	cnn := WinPool.GET(w.PoolName)
	if cnn != nil {
		w.WinPool = cnn
		return true, false, nil, nil
	}
	var firstHost string
	var err error
	if firstHost, _, err = net.SplitHostPort(Info); err != nil {
		return false, true, nil, err
	}
	if w.ConnectionTimeout == 0 {
		w.SetOutTime(60*1000, 10*1000, 60*1000)
	}
	addr := IpDns(Info)
	if addr == "" {
		return false, true, nil, errors.New("DNS Lookup for \"" + firstHost + "\" failed. ")
	}
	if w.Proxy != nil {
		if len(w.Proxy.Address) > 3 {
			w.SetOutTime(w.Proxy.Timeout, w.Proxy.Timeout, w.Proxy.Timeout)
		}
	}
	w.WinPool = &PoolInfo{}
	w.WinPool.Conn, err = net.DialTimeout("tcp", addr, w.ConnectionTimeout)
	if err != nil {
		return false, true, nil, errors.New(strings.Replace(err.Error(), addr, Info, 1))
	}
	var fHost string
	if fHost, _, err = net.SplitHostPort(w.request.URL.Host); err != nil {
		fHost = w.request.URL.Host
	}
	if w.Proxy != nil {
		if w.Proxy.Address != "" {
			w.SetOutTime(w.Proxy.Timeout, w.Proxy.Timeout, w.Proxy.Timeout)
			if w.Proxy.S5TypeProxy == false {
				auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(w.Proxy.User+":"+w.Proxy.Pass))
				if w.Proxy.User != "" {
					w.SetHeader("Proxy-Authorization", auth)
				}
				if _tls {
					if w.Proxy.User != "" {
						_, _ = w.WinPool.Conn.Write([]byte("CONNECT " + host + " HTTP/1.1\r\nHost: " + host + "\r\nProxy-Authorization: " + auth + "\r\n\r\n"))
					} else {
						_, _ = w.WinPool.Conn.Write([]byte("CONNECT " + host + " HTTP/1.1\r\nHost: " + host + "\r\n\r\n"))
					}
					ret, err := http.ReadResponse(bufio.NewReader(w.WinPool.Conn), w.request)
					if err != nil {
						return false, true, nil, err
					}
					if ret.StatusCode != 200 {
						w.WinPool.Bw = bufio.NewReader(&MyConn{Conn: w.WinPool.Conn, hook: w.WinPool.SetHeads, inject: w.WinPool.inject})
						w.WinPool.prohibit = true
						return true, true, ret, nil
					}

				}
			} else {
				PortNum := uint16(0)
				if w.request.URL.Port() == "" {
					if _tls {
						PortNum = 443
					} else {
						PortNum = 80
					}
				} else {
					Ports, _ := strconv.Atoi(w.request.URL.Port())
					PortNum = uint16(Ports)
				}
				if ConnectS5(&w.WinPool.Conn, w.Proxy, fHost, PortNum) == false {
					return false, true, nil, errors.New("Socket5 Authentication failed ")
				}
			}
		}
	}
	if _tls {
		cfg := w.cfg
		if cfg == nil {
			cfg = &tls.Config{ServerName: fHost}
		}
		if cfg.ServerName == "" {
			cfg.ServerName = fHost
		}
		if w.GetTLSValues != nil {
			obj := w.GetTLSValues()
			if len(obj) > 0 {
				cfg.CipherSuites = obj
			}
		}
		cfg.InsecureSkipVerify = true
		tlsConn := tls.Client(w.WinPool.Conn, cfg)
		err = tlsConn.Handshake()
		if err != nil {
			return false, true, nil, errors.New("\"" + firstHost + "\" Handshake failed. ")
		}
		w.WinPool.Conn = tlsConn
	}
	w.WinPool.Bw = bufio.NewReader(&MyConn{Conn: w.WinPool.Conn, hook: w.WinPool.SetHeads, inject: w.WinPool.inject})
	w.WinPool.prohibit = false
	return true, true, nil, nil

}
func (w *WinHttp) SetTlsConfig(t *tls.Config) *WinHttp {
	w.cfg = t
	return w
}

// getConnectInfo 获取连接信息
func (w *WinHttp) getConnectInfo() (string, bool) {
	info := ""
	t := false
	if w.Proxy != nil {
		if len(w.Proxy.Address) > 3 {
			info = w.Proxy.Address
			t = w.request.URL.Scheme == "https"
			return info, t
		}
	}
	info = getHost(w.request.URL)
	return info, w.request.URL.Scheme == "https"
}

func NewGoWinHttp() *WinHttp {
	return &WinHttp{}
}

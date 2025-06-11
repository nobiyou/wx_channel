package SunnyNet

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	crypto "crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/qtgolang/SunnyNet/Resource"
	"github.com/qtgolang/SunnyNet/public"
	"github.com/qtgolang/SunnyNet/src/Certificate"
	"github.com/qtgolang/SunnyNet/src/CrossCompiled"
	"github.com/qtgolang/SunnyNet/src/GoWinHttp"
	"github.com/qtgolang/SunnyNet/src/HttpCertificate"
	"github.com/qtgolang/SunnyNet/src/crypto/tls"
	"github.com/qtgolang/SunnyNet/src/websocket"
	"io"
	"io/ioutil"
	mrand "math/rand"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

func init() {
	//使用全部-1个CPU性能,例如你电脑CPU是4核心 那么就使用4-1 使用3核心的的CPU性能
	runtime.GOMAXPROCS(runtime.NumCPU() - 1)
	CrossCompiled.SetNetworkConnectNumber()
}

// TargetInfo 请求连接信息
type TargetInfo struct {
	Host string //带端口号
	Port uint16
	IPV6 bool
}

// Remove 清除信息
func (s *TargetInfo) Remove() {
	s.Host = public.NULL
	s.Port = 0
}

// 解析IPV6地址
func parseIPv6Address(address string) (string, uint16) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		// 没有端口号
		host = address
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return "", 0
	} else if ip.To4() != nil {
		// 如果是IPv4地址，返回空字符串
		return "", 0
	}

	var portNumber uint16
	if port != "" {
		portInt, err := strconv.ParseUint(port, 10, 16)
		if err != nil {
			return "", 0
		}
		portNumber = uint16(portInt)
	}

	return ip.String(), portNumber
}

// Parse 解析连接信息
func (s *TargetInfo) Parse(HostName string, Port interface{}, IPV6 ...bool) {
	//如果是8.8.8.8 则端口号不变
	//如果是8.8.8.8:8888 Host和端口都变
	//如果是Host="" Port=8888 Host不变,端口变
	//如果是8.8.8.8:8888 Port=8889 那么端口=8888
	if s == nil {
		return
	}
	Host := HostName
	p := uint16(0)
	s.IPV6 = len(IPV6) > 0
	if s.IPV6 {
		s.IPV6 = IPV6[0]
	}

	_s, _p := parseIPv6Address(Host)
	if _s != "" {
		s.Host = _s
		p = _p
		s.IPV6 = true
	}

	if strings.Index(Host, ":") == -1 || s.IPV6 {
		switch v := Port.(type) {
		case string:
			a, _ := strconv.Atoi(v)
			if a > 0 {
				p = uint16(a)
			}
			break
		case uint16:
			p = v
			break
		default:
			a, _ := strconv.Atoi(fmt.Sprintf("%d", v))
			if a > 0 {
				p = uint16(a)
			}
			break
		}
	} else {
		arr := strings.Split(Host, ":")
		if len(arr) == 2 {
			Host = arr[0]
			a, _ := strconv.Atoi(arr[1])
			if a > 0 {
				p = uint16(a)
			}
		}
	}
	if p > 0 {
		s.Port = p
	}
	if Host != "" {
		if _s == "" {
			s.Host = Host
		}
	}

}

// String 格式化信息返回格式127.0.0.1:8888
func (s *TargetInfo) String() string {
	if s.IPV6 {
		return fmt.Sprintf("[%s]:%d", s.Host, s.Port)
	}
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

// ProxyRequest 请求信息
type ProxyRequest struct {
	Conn      net.Conn                //请求的原始TCP连接
	RwObj     *public.ReadWriteObject //读写对象
	Theology  int                     //中间件回调唯一ID
	Target    *TargetInfo             //目标连接信息
	ProxyHost string                  //请求之上的代理
	Pid       string                  //s5连接过来的pid
	Global    *Sunny                  //继承全局中间件信息
	//WinHttp   *GoWinHttp.WinHttp      //WinHTTP请求对象
	WinHttp      *GoWinHttp.WinHttp   //WinHTTP请求对象
	Request      *http.Request        //要发送的请求体
	Response     *http.Response       //HTTP响应体
	TCP          public.TCP           //TCP收发数据
	Websocket    *public.WebsocketMsg //Websocket会话
	Proxy        *GoWinHttp.Proxy     //设置指定代理
	HttpCall     int                  //http 请求回调地址
	TcpCall      int                  //TCP请求回调地址
	wsCall       int                  //ws回调地址
	HttpGoCall   func(Conn *HttpConn) //http 请求回调地址
	TcpGoCall    func(Conn *TcpConn)  //TCP请求回调地址
	wsGoCall     func(Conn *WsConn)   //ws回调地址
	NoRepairHttp bool                 //不要纠正Http
	Lock         sync.Mutex
}

var sUser = make(map[int]string)
var sL sync.Mutex

// 设置s5连接账号
func (s *ProxyRequest) setSocket5User(user string) {
	sL.Lock()
	sUser[s.Theology] = user
	sL.Unlock()
}
func (s *ProxyRequest) IsMustTcpRules(Host string) bool {
	s.Global.lock.Lock()
	defer s.Global.lock.Unlock()
	if s.Global.mustTcpRegexp == nil {
		return false
	}
	return s.Global.mustTcpRegexp.MatchString(Host)
}

// 更新唯一ID以及s5连接账号
func (s *ProxyRequest) updateSocket5User() {
	sL.Lock()
	user := sUser[s.Theology]
	delete(sUser, s.Theology)
	s.Theology = int(atomic.AddInt64(&public.Theology, 1))
	if user != "" {
		sUser[s.Theology] = user
	}
	sL.Unlock()
}

// 清除唯一ID对应的s5连接账号
func (s *ProxyRequest) delSocket5User() {
	sL.Lock()
	delete(sUser, s.Theology)
	sL.Unlock()
}

// 获取唯一ID对应的s5连接账号
func GetSocket5User(TheologyId int) string {
	sL.Lock()
	user := sUser[TheologyId]
	sL.Unlock()
	return user
}

// AuthMethod S5代理鉴权
func (s *ProxyRequest) AuthMethod() (bool, string) {
	av, err := s.RwObj.ReadByte()
	if err != nil || av != 1 {
		//fmt.Println(ID, "Socks5 auth version invalid")
		return false, public.NULL
	}

	uLen, err := s.RwObj.ReadByte()
	if err != nil || uLen <= 0 || uLen > 255 {
		//fmt.Println(ID, "Socks5 auth user length invalid")
		return false, public.NULL
	}

	uBuf := make([]byte, uLen)
	nr, err := s.RwObj.Read(uBuf)
	if err != nil || nr != int(uLen) {
		//fmt.Println(ID, "Socks5 auth user error", nr)
		return false, public.NULL
	}

	user := string(uBuf)

	pLen, err := s.RwObj.ReadByte()
	if err != nil || pLen <= 0 || pLen > 255 {
		//fmt.Println(ID, "Socks5 auth passwd length invalid", pLen)
		return false, public.NULL
	}

	pBuf := make([]byte, pLen)
	nr, err = s.RwObj.Read(pBuf)
	if err != nil || nr != int(pLen) {
		//fmt.Println(ID, "Socks5 auth passwd error", pLen, nr)
		return false, public.NULL
	}

	passwd := string(pBuf)
	if s.Global.socket5VerifyUser {
		if len(user) > 0 && len(passwd) > 0 {
			s.Global.socket5VerifyUserLock.Lock()
			if passwd == s.Global.socket5VerifyUserList[user] {
				s.Global.socket5VerifyUserLock.Unlock()
				_ = s.RwObj.WriteByte(0x01)
				_ = s.RwObj.WriteByte(0x00)
				s.setSocket5User(user)
				return true, passwd
			}
			s.Global.socket5VerifyUserLock.Unlock()
		}
	} else {
		if len(user) > -1 || len(passwd) > -1 {
			//fmt.Println(1, user, passwd)
			_ = s.RwObj.WriteByte(0x01)
			_ = s.RwObj.WriteByte(0x00)
			return true, passwd
		}
	}

	_ = s.RwObj.WriteByte(0x01)
	_ = s.RwObj.WriteByte(0x01)
	return false, public.NULL
}

// Socks5ProxyVerification S5代理验证
func (s *ProxyRequest) Socks5ProxyVerification() bool {
	version, err := s.RwObj.ReadByte()
	if err != nil {
		return false
	}
	if version != public.Socks5Version {
		return false
	}
	methods, err := s.RwObj.ReadByte()
	if err != nil {
		return false
	}

	if methods < 0 || methods > 255 {
		return false
	}
	supportAuth := false
	method := public.Socks5AuthNone
	for i := 0; i < int(methods); i++ {
		method, err = s.RwObj.ReadByte()
		if err != nil {
			return false
		}
		if method == public.Socks5Auth {
			supportAuth = true
		}
	}

	err = s.RwObj.WriteByte(version)
	if err != nil {
		return false
	}

	// 支持加密, 则回复加密方法.
	if supportAuth {
		method = public.Socks5Auth
		err = s.RwObj.WriteByte(method)
		if err != nil {
			return false
		}
	} else {
		// 服务器不支持加密, 直接通过.
		method = public.Socks5AuthNone
		err = s.RwObj.WriteByte(method)
		if err != nil {
			return false
		}
	}
	_ = s.RwObj.Flush()
	ok := false
	// Auth mode, read user passwd.
	// 暂时没啥用 现在设置的不要密码或任意账号密码都通过
	if supportAuth {
		ok, _ = s.AuthMethod()
		if !ok {
			return false
		}
		_ = s.RwObj.Flush()
	} else if s.Global.socket5VerifyUser {
		return false
	}

	handshakeVersion, err := s.RwObj.ReadByte()
	if err != nil || handshakeVersion != public.Socks5Version {
		if err != nil {
		}
		return false
	}
	command, err := s.RwObj.ReadByte()
	if err != nil {
		//fmt.Println(ID, "Socks5 read command error", err.Error())
		return false
	}
	if command != public.Socks5CmdConnect &&
		command != public.Socks5CmdBind &&
		command != public.Socks5CmdUDP {
		return false
	}

	_, _ = s.RwObj.ReadByte() // rsv byte
	aTyp, err := s.RwObj.ReadByte()
	if err != nil {
		return false
	}
	if aTyp != public.Socks5typeDomainName &&
		aTyp != public.Socks5typeIpv4 &&
		aTyp != public.Socks5typeIpv6 {
		return false
	}

	hostname := public.NULL
	switch {
	case aTyp == public.Socks5typeIpv4:
		{
			IPv4Buf := make([]byte, 4)
			nr, err := s.RwObj.Read(IPv4Buf)
			if err != nil || nr != 4 {
				return false
			}

			ip := net.IP(IPv4Buf)
			hostname = ip.String()
		}
	case aTyp == public.Socks5typeIpv6:
		{
			IPv6Buf := make([]byte, 16)
			nr, err := s.RwObj.Read(IPv6Buf)
			if err != nil || nr != 16 {
				return false
			}

			ip := net.IP(IPv6Buf)
			hostname = ip.String()
		}
	case aTyp == public.Socks5typeDomainName:
		{
			dnLen, err := s.RwObj.ReadByte()
			if err != nil || int(dnLen) < 0 {
				return false
			}

			domain := make([]byte, dnLen)
			nr, err := s.RwObj.Read(domain)
			if err != nil || nr != int(dnLen) {
				return false
			}
			hostname = string(domain)
		}
	}
	portNum1, err := s.RwObj.ReadByte()
	if err != nil {
		return false
	}
	portNum2, err := s.RwObj.ReadByte()
	if err != nil {
		return false
	}
	port := uint16(portNum1)<<8 + uint16(portNum2)
	hostname = fmt.Sprintf("%s:%d", hostname, port)

	_ = s.RwObj.WriteByte(public.Socks5Version)

	if command == public.Socks5CmdUDP {
		ipArr := strings.Split(s.Conn.LocalAddr().String(), ":")
		_ = s.RwObj.WriteByte(0) // SOCKS5_SUCCEEDED
		_ = s.RwObj.WriteByte(0)
		if len(ipArr) != 2 {
			_ = s.RwObj.WriteByte(public.Socks5typeIpv4)
			_, _ = s.RwObj.Write(net.ParseIP("0.0.0.0").To4())
			_ = s.RwObj.WriteByte(portNum1)
			_ = s.RwObj.WriteByte(portNum2)
		} else {
			host := ipArr[0]
			if public.IsIPv4(host) {
				_ = s.RwObj.WriteByte(public.Socks5typeIpv4)
				_, _ = s.RwObj.Write(net.ParseIP(host).To4())
			} else if public.IsIPv6(host) {
				_ = s.RwObj.WriteByte(public.Socks5typeIpv6)
				_, _ = s.RwObj.Write(net.ParseIP(host).To16())
			} else {
				_ = s.RwObj.WriteByte(public.Socks5typeDomainName)
				_ = s.RwObj.WriteByte(byte(len(hostname)))
				_, _ = s.RwObj.WriteString(hostname)
			}
			portNum, _ := strconv.Atoi(ipArr[1])
			portNum1 = byte(portNum >> 8)
			portNum2 = byte(portNum)
			_ = s.RwObj.WriteByte(portNum1)
			_ = s.RwObj.WriteByte(portNum2)
		}
		_ = s.RwObj.Flush()
		for {
			b := make([]byte, 10)
			_, e := s.RwObj.Read(b)
			if e != nil {
				break
			}
		}
		return false
	}
	//var RemoteTCP net.Conn
	err = nil
	a := strings.Split(s.Conn.RemoteAddr().String(), ":")
	if len(a) >= 2 {
		hostname = strings.ReplaceAll(hostname, "127.0.0.1", a[0])
	}
	if err != nil {
		//fmt.Println(hostname, err)
		_ = s.RwObj.WriteByte(1) // SOCKS5_GENERAL_SOCKS_SERVER_FAILURE
	} else {
		_ = s.RwObj.WriteByte(0) // SOCKS5_SUCCEEDED
	}
	_ = s.RwObj.WriteByte(0)

	if err == nil {
		host := GoWinHttp.IpDns(hostname)
		if host == "" {
			host = hostname
		}
		u, _ := url.Parse("https://" + host)
		host = u.Hostname()
		if public.IsIPv4(host) {
			_ = s.RwObj.WriteByte(public.Socks5typeIpv4)
			_, _ = s.RwObj.Write(net.ParseIP(host).To4())
		} else {
			_ = s.RwObj.WriteByte(public.Socks5typeDomainName)
			_ = s.RwObj.WriteByte(byte(len(hostname)))
			_, _ = s.RwObj.WriteString(hostname)
		}
	} else {
		_ = s.RwObj.WriteByte(public.Socks5typeDomainName)
		_ = s.RwObj.WriteByte(byte(len(hostname)))
		_, _ = s.RwObj.WriteString(hostname)
	}

	_ = s.RwObj.WriteByte(portNum1)
	_ = s.RwObj.WriteByte(portNum2)

	_ = s.RwObj.Flush()
	if err != nil {
		return false
	}
	s.Target.Parse(hostname, port)
	return true
}

type loop struct {
	LastTime time.Time
	Num      int
}

var LoopMap = make(map[string]*loop)
var loopLock sync.Mutex

func init() {
	go loopCheckFunc()
}
func loopCheckFunc() {
	for {
		time.Sleep(time.Second * 2)
		loopLock.Lock()
		for k, v := range LoopMap {
			if time.Now().Sub(v.LastTime) > time.Second {
				delete(LoopMap, k)
			}
		}
		loopLock.Unlock()
	}
}
func loopAdd(Host string) bool {
	ok := false
	loopLock.Lock()
	m := LoopMap[Host]
	if m != nil {
		m.Num++
		m.LastTime = time.Now()
		ok = m.Num > 5
	} else {
		LoopMap[Host] = &loop{
			LastTime: time.Now(),
			Num:      1,
		}
	}
	loopLock.Unlock()
	return ok
}

// MustTcpProcessing 强制走TCP处理过程
// aheadData 提取获取的数据
func (s *ProxyRequest) MustTcpProcessing(aheadData []byte, Tag string) {
	if s.Target == nil {
		return
	}
	if s.Target.Port == uint16(s.Global.port) {
		//检测环路循环检测
		if loopAdd(s.Target.Host) {
			return
		}
	}
	var err error
	var isClose = false
	as := &public.TcpMsg{}
	as.Data.WriteString(Tag)
	s.CallbackTCPRequest(public.SunnyNetMsgTypeTCPAboutToConnect, as)
	if Tag != as.Data.String() {
		s.Target.Parse(as.Data.String(), 0)
	}
	Portly := &GoWinHttp.Proxy{Timeout: 60 * 1000}
	if as.TcpIp != public.NULL {
		Portly.S5TypeProxy = true
		Portly.Address = as.TcpIp
		Portly.User = as.TcpUser
		Portly.Pass = as.TcpPass
	} else if s.Global.proxy != nil {
		if !s.Global.proxyRegexp.MatchString(s.Target.Host) {
			Portly.S5TypeProxy = s.Global.proxy.S5TypeProxy
			Portly.Address = s.Global.proxy.Address
			Portly.User = s.Global.proxy.User
			Portly.Pass = s.Global.proxy.Pass
		}
	}
	defer func() {
		if !isClose {
			s.CallbackTCPRequest(public.SunnyNetMsgTypeTCPClose, nil)
		}
		if as != nil {
			as.Data.Reset()
		}
		as = nil
		Portly = nil
		aheadData = make([]byte, 0)
		aheadData = nil
		s.releaseTcp()
	}()
	var RemoteTCP net.Conn
	if Portly.S5TypeProxy {
		if s.Target.Port == 0 {
			return
		}
		c, err := net.DialTimeout("tcp", Portly.Address, 15*time.Second)
		if err != nil {
			return
		}
		if GoWinHttp.ConnectS5(&c, Portly, s.Target.Host, s.Target.Port) == false {
			_ = c.Close()
			return
		}
		RemoteTCP = c
	} else {
		RemoteTCP, err = net.DialTimeout("tcp", s.Target.String(), 15*time.Second)

	}
	defer func() {
		if RemoteTCP != nil {
			_ = RemoteTCP.Close()
		}
	}()
	if RemoteTCP != nil && Tag == public.TagTcpSSLAgreement {
		certificate, er := s.Global.getCertificate(s.Target.String())
		if er != nil {
			return
		}
		fig := &tls.Config{Certificates: []tls.Certificate{*certificate}, ServerName: HttpCertificate.ParsingHost(s.Target.String())}
		fig.InsecureSkipVerify = true
		obj := s.Global.GetTLSValues()
		if obj != nil {
			fig.CipherSuites = obj
		}
		tlsConn := tls.Client(RemoteTCP, fig)
		err = tlsConn.Handshake()
		RemoteTCP = tlsConn
	}
	if err == nil && RemoteTCP != nil {
		tw := public.NewReadWriteObject(RemoteTCP)
		{
			//构造结构体数据,主动发送，关闭等操作时需要用
			if s.TCP.Send == nil {
				s.TCP.Send = &public.TcpMsg{}
			}
			if s.TCP.Receive == nil {
				s.TCP.Receive = &public.TcpMsg{}
			}
			s.TCP.SendBw = s.RwObj.Writer
			s.TCP.ReceiveBw = tw.Writer
			s.TCP.ConnSend = s.Conn
			s.TCP.ConnServer = RemoteTCP
			TcpSceneLock.Lock()
			TcpStorage[s.Theology] = &s.TCP
			TcpSceneLock.Unlock()
		}
		s.CallbackTCPRequest(public.SunnyNetMsgTypeTCPConnectOK, nil)
		if len(aheadData) > 0 {
			as.Data.Reset()
			as.Data.Write(aheadData)
			s.CallbackTCPRequest(public.SunnyNetMsgTypeTCPClientSend, as)
			_, _ = RemoteTCP.Write(as.Data.Bytes())
			if as != nil {
				as.Data.Reset()
			}
			as = nil
			aheadData = make([]byte, 0)
			aheadData = nil
		}
		isClose = s.TcpCallback(&RemoteTCP, Tag, tw)
	} else {
		_ = s.Conn.Close()
	}

	return
}

// 释放tcp关联的数据
func (s *ProxyRequest) releaseTcp() {
	//================================================================================================================================
	if s == nil {
		return
	}
	if s.TCP.Send != nil {
		s.TCP.Send.Data.Reset()
	}
	if s.TCP.Receive != nil {
		s.TCP.Receive.Data.Reset()
	}
	s.TCP.Send = nil
	s.TCP.SendBw = nil
	s.TCP.ConnSend = nil //=========================  释放相关数据
	s.TCP.Receive = nil
	s.TCP.ReceiveBw = nil
	s.TCP.ConnServer = nil
	TcpSceneLock.Lock()
	TcpStorage[s.Theology] = nil
	delete(TcpStorage, s.Theology)
	TcpSceneLock.Unlock()
	//================================================================================================================================
}

// TcpCallback TCP消息处理 返回 是否已经调用 通知 回调函数 TCP已经关闭
func (s *ProxyRequest) TcpCallback(RemoteTCP *net.Conn, Tag string, tw *public.ReadWriteObject) bool {
	if RemoteTCP == nil {
		return false
	}
	if *RemoteTCP == nil {
		return false
	}
	var wg sync.WaitGroup
	wg.Add(1)
	isHttpReq := false //是否纠正HTTP请求，可能由于某些原因 客户端发送数据不及时判断为了TCP请求，后续TCP处理时纠正为HTTP请求
	//读取客户端消息转发给服务端
	go func() {
		s.SocketForward(*tw.Writer, s.RwObj, public.SunnyNetMsgTypeTCPClientSend, s.Conn, *RemoteTCP, &s.TCP, &isHttpReq)
		wg.Done()
	}()
	//读取服务器消息转发给客户端
	s.SocketForward(*s.RwObj.Writer, tw, public.SunnyNetMsgTypeTCPClientReceive, *RemoteTCP, s.Conn, &s.TCP, &isHttpReq)
	wg.Wait()
	s.releaseTcp()
	if isHttpReq {
		//可能由于某些原因 客户端发送数据不及时判断为了TCP请求,此时纠正为HTTP请求
		s.CallbackTCPRequest(public.SunnyNetMsgTypeTCPClose, nil)
		s.updateSocket5User()
		//如果之前是HTTP请求识别错误 这里转由HTTP请求处理函数继续处理
		if Tag == public.TagTcpSSLAgreement {
			s.httpProcessing(nil, "443", Tag)
		} else {
			s.httpProcessing(nil, "80", Tag)
		}
		return true
	}
	return false
}

// ConnRead
// 从缓冲区读取字节流
// Dosage 如果为true 多读取一会
func (s *ProxyRequest) ConnRead(aheadData []byte, Dosage bool) (rs []byte, WhetherExceedsLength bool) {
	var st bytes.Buffer
	st.Write(aheadData)
	length := 512
	bs := make([]byte, length)
	var last []byte
	i := 0
	_ = s.Conn.SetWriteDeadline(time.Now().Add(1000 * time.Millisecond))
	_ = s.Conn.SetReadDeadline(time.Now().Add(1000 * time.Millisecond))
	_ = s.Conn.SetDeadline(time.Now().Add(1000 * time.Millisecond))
	cLength := 0
	var cmp = make(map[int]int)
	defer func() {
		_ = s.Conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
		_ = s.Conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		_ = s.Conn.SetDeadline(time.Now().Add(30 * time.Second))
		for k, _ := range cmp {
			delete(cmp, k)
		}
		cmp = nil
		bs = make([]byte, 0)
		bs = nil
		st.Reset()
	}()
	kl := 10
	if Dosage {
		kl = kl * 10
	}
	var NoHttpRequest = true
	var eRequest = errors.New("No http Request ")
	var Method = public.Nulls
	//验证HTTP请求体
	//islet=是否有 HTTP Body 长度
	//ok 是否验证成功
	//bodyLen 已出Body长度
	//isHttpRequest 是否HTTP请求
	var islet, ok, isHttpRequest bool
	var bodyLen, ContentLength int
	for {
		sx, e := s.RwObj.Read(bs)
		//2024-01-20 之前版本的代码
		/*
			if !NoHttpRequest {
				for n := 0; n < sx; n++ {
					if bs[n] < 9 || bs[n] == 11 || bs[n] == 12 || (bs[n] >= 14 && bs[n] < 21) {
						NoHttpRequest = true
						break
					}
				}
			}
		*/
		st.Write(bs[0:sx])
		//-----------------------------------------
		//----  下面是2024-01-20 修改的代码 ----------
		bsm := st.Bytes()
		if NoHttpRequest {
			_index := bytes.Index(bsm, []byte("\r\n\r\n"))
			if _index > 11 {
				va := bsm[0:_index]
				if public.IsHttpMethod(public.GetMethod(va)) {
					NoHttpRequest = false
				}
			}
		}
		//-----------------------------------------
		if NoHttpRequest {
			e = eRequest
		}
		if e != nil {
			if st.Len() < length {
				islet, ok, bodyLen, ContentLength, isHttpRequest = public.LegitimateRequest(st.Bytes())
				if !islet && !isHttpRequest {
					// 如果已读入字节数 小于 512 并且 超过 10次 已读入数没有变动，那么直接返回
					cmp[st.Len()]++
					if cmp[st.Len()] >= kl || NoHttpRequest {
						bs = make([]byte, 0)
						last = public.CopyBytes(st.Bytes())
						if last == nil {
							last = []byte{}
						}
						_ = s.Conn.SetReadDeadline(time.Now().Add(30 * time.Second))
						return last, false
					} else {
						_ = s.Conn.SetReadDeadline(time.Now().Add(3 * time.Millisecond))
						continue
					}
				}
			}
			//如果错误是超时那么进行判断 如果不是超时 那么直接返回
			if strings.Index(e.Error(), public.Timeout) != -1 || NoHttpRequest {
				if NoHttpRequest && Method != public.Nulls {
					Method = public.GetMethod(st.Bytes())
					if Method == public.NULL {
						last = public.CopyBytes(st.Bytes())
						break
					}
				}
				i++
				islet, ok, bodyLen, ContentLength, isHttpRequest = public.LegitimateRequest(st.Bytes())
				if ContentLength > public.MaxUploadLength {
					last = public.CopyBytes(st.Bytes())
					return last, true
				}
				if ok == false {
					//Body中没有长度
					if islet == false {
						_ = s.Conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
						//验证失败
						if st.Len() < length {
							if i < 10 {
								continue
							}
						} else if i < 15 {
							continue
						}
					} else {
						_ = s.Conn.SetReadDeadline(time.Now().Add(3 * time.Millisecond))
						if cLength == bodyLen {
							if i < 1000 {
								continue
							}
						} else {
							cLength = bodyLen
							i--
							continue
						}
					}
				}
			}
			last = public.CopyBytes(st.Bytes())
			break
		} else {
			_ = s.Conn.SetReadDeadline(time.Now().Add(3 * time.Millisecond))
		}
	}
	return last, false
}

// transparentProcessing 透明代理请求 处理过程
func (s *ProxyRequest) transparentProcessing() {
	//将数据全部取出，稍后重新放进去
	_bytes, _ := s.RwObj.Peek(s.RwObj.Reader.Buffered())
	//升级到TLS客户端
	fig := &tls.Config{InsecureSkipVerify: true}
	T := tls.Client(s.Conn, fig)
	//将数据重新写进去
	T.Reset(_bytes)
	//进行握手处理
	msg, serverName, e := T.ClientHello()
	if e == nil {
		//从握手信息中取出要连接的服务器域名
		if serverName == public.NULL {
			//如果没有取出 则按照连接地址处理
			serverName = s.Conn.LocalAddr().String()
		}
		//将地址写到请求中间件连接信息中
		s.Target.Parse(serverName, public.HttpsDefaultPort)
		//进行生成证书，用于服务器返回握手信息
		certificate, er := s.Global.getCertificate(s.Target.String())
		if er != nil {
			_ = T.Close()
			return
		}
		//将证书和域名信息设置到TLS客户端中
		T.SetServer(&tls.Config{Certificates: []tls.Certificate{*certificate}, ServerName: HttpCertificate.ParsingHost(s.Target.String())})
		//进行与客户端握手
		e = T.ServerHandshake(msg)
		if e == nil {
			//如果握手过程中没有发生意外， 则重写客户端会话
			s.Conn = T                             //将TLS会话替换原始会话
			s.RwObj = public.NewReadWriteObject(T) //重新包装读写对象
			//开始按照HTTP请求流程处理
			s.httpProcessing(nil, public.HttpsDefaultPort, public.TagTcpSSLAgreement)
		}
	} else {
		//如果握手失败 直接返回，不做任何处理
	}
}

// httpProcessing http请求处理过程
func (s *ProxyRequest) httpProcessing(aheadData []byte, DefaultPort, Tag string) {

	defer func() {
		aheadData = make([]byte, 0)
		aheadData = nil
	}()
	//缓冲区读取字节流
	ReadData, WhetherExceedsLength := s.ConnRead(aheadData, false)
	//从字节流中取出HOST
	host := public.GetHost(string(ReadData))
	if host != public.NULL && host != s.Target.Host {
		s.Target.Parse(host, DefaultPort)
	}
	//提交数据是否超过 public.MaxUploadLength 字节，若是超过  public.MaxUploadLength  设定的最大字节数，改请求将转为TCP方式请求
	//或者符合强制TCP的规则
	if WhetherExceedsLength || s.IsMustTcpRules(s.Target.Host) {
		s.NoRepairHttp = true
		s.MustTcpProcessing(ReadData, Tag)
		s.NoRepairHttp = false
		return
	}
	//继续HTTP处理
	s.StartHTTPProcessing(public.CopyBytes(ReadData), public.NULL, Tag, DefaultPort)
	//不去循环监听 一次请求完成直接断开连接
	//return
	//下面这样写是因为 HTTP/s 请求 发送一次请求之后 TCP不会断开连接（长连接）下次请求会直接发送
	for {
		s.Response = nil
		s.Request = nil
		s.WinHttp = nil
		s.updateSocket5User()
		//超过3秒无数据的长连接就直接断开吧
		_ = s.Conn.SetDeadline(time.Now().Add(public.WaitingTime))
		_, e := s.RwObj.Peek(1)
		if e != nil {
			return
		}
		ReadData, WhetherExceedsLength = s.ConnRead(nil, false)
		host = public.GetHost(string(ReadData))
		if host != "" {
			s.Target.Parse(host, DefaultPort)
		}
		//提交数据是否超过 public.MaxUploadLength 字节，若是超过  public.MaxUploadLength  设定的最大字节数，改请求将转为TCP方式请求
		if WhetherExceedsLength {
			s.NoRepairHttp = true
			s.MustTcpProcessing(ReadData, Tag)
			s.NoRepairHttp = false
			return
		}
		s.StartHTTPProcessing(public.CopyBytes(ReadData), public.NULL, Tag, DefaultPort)
	}
}

// StartHTTPProcessing ...
func (s *ProxyRequest) StartHTTPProcessing(RawBytes []byte, sProxy, Tag, DefaultPort string) {
	var setProxyHost = func(sS string) {
		s.ProxyHost = sS
	}
	method := public.NULL
	if len(RawBytes) > 12 {
		//从原始数据中取出 Method
		method = public.GetMethod(RawBytes)
	}
	//判断是否开启了强制走TCP 和 method 是否符合正常HTTP/S Method 否则按照TCP处理
	if (s.Global.isMustTcp && method != public.HttpMethodCONNECT) || !public.IsHttpMethod(method) {
		if s.Target.Host == "" {
			return
		}
		if s.Global.isMustTcp {
			s.MustTcpProcessing(RawBytes, public.TagMustTCP)
			return
		}
		s.MustTcpProcessing(RawBytes, Tag)
		return
	}
	source := ""
	arr := strings.Split(s.Conn.RemoteAddr().String(), ":")
	if len(arr) >= 1 {
		source = arr[0] + ""
	}
	arr = nil
	req, BodyLen := public.BuildRequest(RawBytes, s.Target.String(), source, DefaultPort, setProxyHost, s.RwObj)
	defer func() {
		if req != nil {
			if req.Header != nil {
				//如果请求期望的是短连接，则关闭会话
				if req.Header.Get("Connection") == "close" {
					_ = s.Conn.Close()
				}
			} else {
				_ = s.Conn.Close()
			}

			if req.URL != nil {
				req.URL = nil
			}
		} else {
			_ = s.Conn.Close()
		}

		if req != nil {
			if req.Body != nil {
				_ = req.Body.Close()
			}
			if req.URL != nil {
				req.URL = nil
			}
			req = nil
		}
	}()
	if req == nil {
		return
	}
	if req.URL == nil {
		return
	}
	if req.ContentLength > 0 && BodyLen == 0 {
		_, _ = s.RwObj.WriteString(public.HttpResponseStatus100)
		if req.Body != nil {
			_ = req.Body.Close()
		}
		BytesRaw, WhetherExceedsLength := s.ConnRead(nil, true)
		//提交数据是否超过 public.MaxUploadLength 字节，若是超过  public.MaxUploadLength  设定的最大字节数，改请求将转为TCP方式请求
		if WhetherExceedsLength {
			s.NoRepairHttp = true
			s.MustTcpProcessing(BytesRaw, Tag)
			s.NoRepairHttp = false
			return
		}
		req.Body = ioutil.NopCloser(bytes.NewBuffer(BytesRaw))
		req.ContentLength = int64(len(BytesRaw))
	}
	if method == public.HttpMethodCONNECT {
		s.ProxyHost = sProxy
		s.sendHttps(req)
		return
	}
	s.sendHttp(req)
	return
}

func (s *ProxyRequest) isCerDownloadPage(request *http.Request) bool {
	if public.IsCerRequest(request, s.Global.port) {
		if request.URL != nil {
			defer func() { _ = s.Conn.Close() }()
			if request.URL.Path == "/favicon.ico" {

			}

			if request.URL.Path == "/" || request.URL.Path == "/ssl" || request.URL.Path == public.NULL {
				_, _ = s.RwObj.Write(public.LocalBuildBody("text/html", `<html><head><meta http-equiv="Content-Type" content="text/html; charset=UTF-8"><title>证书安装</title></head><body style="font-family: arial,sans-serif;"><h1>[Sunny中间件] 证书安装</h1><br /><ul><li>您可以下载 <a href="SunnyRoot.cer">SunnyRoot 证书</a></ul><ul><li>您也可以下载 <a href="install.html">查看证书安装教程</a></ul></body></html>`))
				return true
			}
			if request.URL.Path == "/SunnyRoot.cer" || request.URL.Path == "SunnyRoot.cer" {
				_, _ = s.RwObj.Write(public.LocalBuildBody("application/x-x509-ca-cert", s.Global.ExportCert()))
				return true
			}
			if request.URL.Path == "/install.html" || request.URL.Path == "install.html" {
				_, _ = s.RwObj.Write(public.LocalBuildBody("text/html", Resource.CertInstallDocument))
				return true
			}
			_, _ = s.RwObj.Write(public.LocalBuildBody("text/html", "404 Not Found"))

			return true
		}
	}
	return false
}
func (s *ProxyRequest) Error(error error) {
	if error != public.ProvideForwardingServiceOnly {
		if s.Response != nil {
			if s.Response.Body != nil {
				_ = s.Response.Body.Close()
			}
			s.Response = nil
		}
	}
	s.CallbackError(error)
	if error != public.ProvideForwardingServiceOnly {
		if s.Response != nil {
			_ = s.Conn.SetDeadline(time.Now().Add(10 * time.Second))
			_, _ = s.RwObj.Write(public.StructureBody(s.Response))
			_ = s.RwObj.Flush()
			return
		}
	}
	if error == public.ProvideForwardingServiceOnly {
		return
	}
	if s.Request.Header.Get("ErrorClose") == "true" {
		return
	}
	if s.RwObj != nil {
		er := []byte("")
		if error != nil {
			er = []byte(error.Error())
		}
		_ = s.Conn.SetDeadline(time.Now().Add(10 * time.Second))
		_, _ = s.RwObj.WriteString(fmt.Sprintf("HTTP/1.1 %d %s\r\nContent-Length: %d\r\n\r\n", http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError), len(er)))
		_, _ = s.RwObj.Write(er)
		_ = s.RwObj.Flush()
	}
}
func (s *ProxyRequest) doRequest() error {
	if s.Request == nil {
		return errors.New("request is nil")
	}
	if s.Request.URL == nil {
		return errors.New("request.url is nil")
	}
	cfg := HttpCertificate.GetTlsConfigCrypto(s.Request.URL.Host, public.CertificateRequestManagerRulesSend)
	if cfg == nil {
		cfg = &crypto.Config{}
	}
	cfg.ServerName = HttpCertificate.ParsingHost(s.Request.URL.Host)
	cfg.InsecureSkipVerify = true
	if s.WinHttp == nil {
		s.WinHttp = GoWinHttp.NewGoWinHttp()
	}
	s.WinHttp.GetTLSValues = s.Global.GetTLSValues
	s.WinHttp.SetTlsConfig(cfg)
	s.WinHttp.SetProxy(s.Proxy)
	A, B := s.WinHttp.Do(s.Request)
	s.Response = A
	return B
}
func (s *ProxyRequest) sendHttps(req *http.Request) {
	s.Target.Parse(req.Host, public.HttpsDefaultPort)
	if req.URL.Port() != public.NULL {
		Port, _ := strconv.Atoi(req.URL.Port())
		s.Target.Port = uint16(Port)
	}
	_, _ = s.RwObj.WriteString(public.TunnelConnectionEstablished)
	s.https()
}
func (s *ProxyRequest) https() {
	//判断有没有连接信息，没有连接地址信息就直接返回
	if s.Target.Host == public.NULL || s.Target.Port < 1 {
		return
	}
	//是否开启了强制走TCP
	if s.Global.isMustTcp || s.IsMustTcpRules(s.Target.Host) {
		if s.Global.disableTCP {
			return
		}
		//开启了强制走TCP，则按TCP流程处理
		s.MustTcpProcessing(nil, public.TagMustTCP)
		return
	}
	//创建要握手的证书
	certificate, err := s.Global.getCertificate(s.Target.String())
	if err != nil {
		return
	}
	var tlsConn *tls.Conn
	var serverName string
	var HelloMsg *tls.ClientHelloMsg
	//普通会话升级到TLS会话，并且设置生成的握手证书,限制tls最大版本为1.2,因为1.3可能存在算法不支持
	//如果某些服务器只支持tls1.3,将会在 tlsConn.ClientHello() 函数中自动纠正为 tls1.3
	//tlsConfig := &tls.Config{Certificates: []tls.Certificate{*certificate}, MaxVersion: tls.VersionTLS12, NextProtos: []string{"h2", "http/1.1"}}
	tlsConfig := &tls.Config{Certificates: []tls.Certificate{*certificate}, MaxVersion: tls.VersionTLS12}
	tlsConn = tls.Server(s.Conn, tlsConfig)
	defer func() {
		//函数退出时 清理TLS会话
		tlsConn.RReset()
		_ = tlsConn.Close()
		tlsConn = nil
	}()
	//设置1秒的超时 来判断是否 https 请求 因为正常的非HTTPS TCP 请求也会进入到这里来，需要判断一下
	_ = tlsConn.SetDeadline(time.Now().Add(1 * time.Second))
	//取出第一个字节，判断是否TLS
	peek := tlsConn.Peek(1)
	if len(peek) == 1 && (peek[0] == 22 || peek[0] == 23) {
		//发送数据 如果 不是 HEX 16 或 17 那么肯定不是HTTPS 或TLS-TCP
		//HEX 16=ANSI 22 HEX 17=ANSI 23
		//如果是TLS请求设置3秒超时来处理握手信息
		_ = tlsConn.SetDeadline(time.Now().Add(3 * time.Second))
		//开始握手
		msg, _serverName, _err := tlsConn.ClientHello()
		if _serverName != "" && s.IsMustTcpRules(_serverName) {
			if s.Global.disableTCP {
				return
			}
			bs := tlsConn.Read_Handshake_bytes()
			s.MustTcpProcessing(bs, public.TagMustTCP)
			return
		}
		HelloMsg = msg
		//得到握手信息后 恢复30秒的读写超时
		_ = tlsConn.SetDeadline(time.Now().Add(30 * time.Second))
		serverName = _serverName
		err = _err
		if HelloMsg != nil {
			//如果得到了握手信息
			if err != nil {
				//但是有错误 ，就直接返回，不在继续处理
				return
			}
			//没有错误 则继续握手
			if serverName != public.NULL {
				s.Target.Parse(serverName, 0)
				//根据握手的服务器域名 重新创建证书
				certificate, err = s.Global.getCertificate(s.Target.String())
				if certificate != nil {
					//因为tlsConfig是指针类型，所以这里可以直接对它进行修改,而不用重新赋值
					tlsConfig.Certificates = []tls.Certificate{*certificate}
					tlsConfig.ServerName = HttpCertificate.ParsingHost(s.Target.String())
					//tlsConn.SetServer(&tls.Config{MaxVersion: tlsConfig.MaxVersion, Certificates: []tls.Certificate{*certificate}, ServerName: HttpCertificate.ParsingHost(s.Target.String())})
				}
			}
			//继续握手
			err = tlsConn.ServerHandshake(HelloMsg)
		}
	} else {
		err = errors.New("No HTTPS ")
	}
	if err != nil {
		//以上握手过程中 有错误产生 有错误则不是TLS
		//判断这些错误信息，是否还能继续处理
		if s.Global.isMustTcp == false && (err == io.EOF || strings.Index(err.Error(), "An existing connection was forcibly closed by the remote host.") != -1 || strings.Index(err.Error(), "An established connection was aborted by the software in your host machine") != -1) {
			s.Request = new(http.Request)
			s.Request.URL, _ = url.Parse(public.HttpsRequestPrefix + strings.ReplaceAll(s.Target.Host, public.Space, public.NULL))
			s.Request.Host = strings.ReplaceAll(s.Target.Host, public.Space, public.NULL)
			s.Error(errors.New("The client closes the connection "))
			return
		}
		//将TLS握手过程中的信息取出来
		bs := tlsConn.Read_last_time_bytes()
		if len(bs) == 0 {
			//如果没有客户端没有主动发送数据的话
			//强制走TCP，按TCP流程处理
			s.MustTcpProcessing(nil, public.TagTcpAgreement)
			return
		}
		//证书无效
		if s.Global.isMustTcp == false && strings.Index(err.Error(), "unknown certificate") != -1 || strings.Index(err.Error(), "client offered only unsupported versions") != -1 {
			s.Request = new(http.Request)
			if serverName == public.NULL {
				s.Request.URL, _ = url.Parse(public.HttpsRequestPrefix + s.Target.Host)
				s.Request.Host = strings.ReplaceAll(s.Target.Host, public.Space, public.NULL)
			} else {
				s.Request.URL, _ = url.Parse(public.HttpsRequestPrefix + serverName)
				s.Request.Host = strings.ReplaceAll(serverName, public.Space, public.NULL)
			}
			s.Error(err)
			return
		}
		//如果是其他错误，进行http处理流程，继续判断
		tlsConn.RReset()
		s.httpProcessing(bs, public.HttpDefaultPort, public.TagTcpAgreement)
		return
	}
	// 以上握手过程中 没有错误产生 说明是https 或TLS-TCP
	s.Conn = tlsConn                             //重新保存TLS会话
	s.RwObj = public.NewReadWriteObject(tlsConn) //重新包装读写对象
	//s.MustTcpProcessing(nil, public.TagTcpSSLAgreement)
	s.httpProcessing(nil, public.HttpsDefaultPort, public.TagTcpSSLAgreement)
}
func (s *ProxyRequest) handleWss() bool {
	if s.Request == nil || s.Request.Header == nil {
		return true
	}
	//判断是否是websocket的请求体 如果不是直接返回继续正常处理请求

	ok := strings.ToLower(s.Request.Header.Get("Upgrade")) == "websocket"
	if !ok {
		m := s.Request.Header["upgrade"]
		if len(m) > 0 {
			ok = strings.ToLower(m[0]) == "websocket"
		}
	}
	if ok {
		Method := "wss"
		Url := s.Request.URL.String()
		if strings.HasPrefix(Url, "net://") || strings.HasPrefix(Url, "http://") {
			Method = "ws"
		}
		var dialer *websocket.Dialer
		if s.Request.URL.Scheme == "https" {
			//选择是否使用指定的证书
			cfg := HttpCertificate.GetTlsConfigCrypto(s.Request.URL.Host, public.CertificateRequestManagerRulesSend)
			if cfg == nil {
				cfg = &crypto.Config{}
			}
			cfg.ServerName = HttpCertificate.ParsingHost(s.Request.URL.Host)
			cfg.InsecureSkipVerify = true
			obj := s.Global.GetTLSValues()
			if obj != nil {
				cfg.CipherSuites = obj
			}
			dialer = &websocket.Dialer{TLSClientConfig: cfg}
		} else {
			dialer = &websocket.Dialer{}
		}
		//构造代理信息
		ProxyUrl := ""
		if len(s.Proxy.Address) > 3 {
			if s.Proxy.S5TypeProxy {
				ProxyUrl = "socks5://"
			} else {
				ProxyUrl = public.HttpRequestPrefix
			}
			ProxyUrl += s.Proxy.User + ":" + s.Proxy.Pass + "@" + s.Proxy.Address
		}
		//发送请求
		Server, r, er := dialer.ConnDialContext(s.Request, ProxyUrl)
		s.Response = r
		defer func() {
			if Server != nil {
				_ = Server.Close()
			}
		}()
		if er != nil {
			//如果发送错误
			s.Error(er)
			return true
		}
		//通知http请求完成回调
		s.CallbackBeforeResponse()
		//将当前客户端的连接升级为Websocket会话
		upgrade := &websocket.Upgrader{}
		Client, er := upgrade.UpgradeClient(s.Request, r, s.Conn)
		if er != nil {
			return true
		}
		defer func() {
			if Client != nil {
				_ = Client.Close()
			}
		}()
		var sc sync.Mutex
		var wg sync.WaitGroup
		wg.Add(1)

		s.Websocket = &public.WebsocketMsg{Mt: 255, Server: Server, Client: Client, Sync: &sc}
		messageIdLock.Lock()
		httpStorage[s.Theology] = s
		messageIdLock.Unlock()
		//开始转发消息
		receive := func() {
			as := &public.WebsocketMsg{Mt: 255, Server: Server, Client: Client, Sync: &sc}
			MessageId := 0
			for {
				{
					//清除上次的 MessageId
					messageIdLock.Lock()
					wsStorage[MessageId] = nil
					delete(wsStorage, MessageId)
					messageIdLock.Unlock()

					//构造一个新的MessageId
					MessageId = NewMessageId()

					//储存对象
					messageIdLock.Lock()
					wsStorage[MessageId] = as
					messageIdLock.Unlock()
				}
				as.Data.Reset()
				mt, message, err := Server.ReadMessage()
				if err != nil {
					as.Data.Reset()
					break
				}
				as.Data.Write(message)
				as.Mt = mt
				s.CallbackWssRequest(public.WebsocketServerSend, Method, Url, as, MessageId)
				sc.Lock()
				err = Client.WriteMessage(as.Mt, as.Data.Bytes())
				sc.Unlock()
				if err != nil {
					as.Data.Reset()
					break
				}
			}
			messageIdLock.Lock()
			wsStorage[MessageId] = nil
			delete(wsStorage, MessageId)
			messageIdLock.Unlock()
			wg.Done()
		}
		as := &public.WebsocketMsg{Mt: 255, Server: Server, Client: Client, Sync: &sc}
		MessageId := NewMessageId()
		messageIdLock.Lock()
		wsStorage[MessageId] = as
		messageIdLock.Unlock()
		s.CallbackWssRequest(public.WebsocketConnectionOK, Method, Url, as, MessageId)
		go receive()
		for {
			{
				//清除上次的 MessageId
				messageIdLock.Lock()
				wsStorage[MessageId] = nil
				delete(wsStorage, MessageId)
				messageIdLock.Unlock()

				//构造一个新的MessageId
				MessageId = NewMessageId()

				//储存对象
				messageIdLock.Lock()
				wsStorage[MessageId] = as
				messageIdLock.Unlock()
			}
			as.Data.Reset()
			mt, message1, err := Client.ReadMessage()
			as.Data.Write(message1)
			as.Mt = mt
			if err != nil {
				as.Data.Reset()
				s.CallbackWssRequest(public.WebsocketDisconnect, Method, Url, as, MessageId)
				break
			}
			s.CallbackWssRequest(public.WebsocketUserSend, Method, Url, as, MessageId)
			sc.Lock()
			err = Server.WriteMessage(as.Mt, as.Data.Bytes())
			sc.Unlock()
			if err != nil {
				as.Data.Reset()
				s.CallbackWssRequest(public.WebsocketDisconnect, Method, Url, as, MessageId)
				break
			}
		}
		wg.Wait()
		messageIdLock.Lock()
		wsStorage[MessageId] = nil
		delete(wsStorage, MessageId)
		httpStorage[s.Theology] = nil
		delete(httpStorage, s.Theology)
		messageIdLock.Unlock()
		return true
	}
	return false
}
func (s *ProxyRequest) CompleteRequest(req *http.Request) {
	//储存 要发送的请求体
	s.Request = req
	defer func() {
		if s.WinHttp != nil {
			s.WinHttp.Save()
		}
		s.WinHttp = nil
		if s.Request != nil {
			if s.Request.Body != nil {
				_ = s.Request.Body.Close()
			}
		}
		s.Request = nil

		if s.Response != nil {
			if s.Response.Body != nil {
				_ = s.Response.Body.Close()
			}
		}
		s.Response = nil
		s.Proxy = nil
		req = nil
	}()
	//继承全局上游代理
	if !s.Global.proxyRegexp.MatchString(s.Target.Host) {
		if s.Proxy == nil {
			s.Proxy = &GoWinHttp.Proxy{Address: s.Global.proxy.Address, S5TypeProxy: s.Global.proxy.S5TypeProxy, User: s.Global.proxy.User, Pass: s.Global.proxy.Pass, Timeout: s.Global.proxy.Timeout}
		} else {
			s.Proxy.S5TypeProxy = s.Global.proxy.S5TypeProxy
			s.Proxy.User = s.Global.proxy.User
			s.Proxy.Address = s.Global.proxy.Address
			s.Proxy.Pass = s.Global.proxy.Pass
			s.Proxy.Timeout = s.Global.proxy.Timeout
		}
	} else if s.Proxy == nil {
		s.Proxy = &GoWinHttp.Proxy{Timeout: 60 * 1000}
	}
	if s.ProxyHost != public.NULL && s.ProxyHost != req.Host+":"+public.HttpDefaultPort && s.ProxyHost != req.Host+":"+public.HttpsDefaultPort && s.ProxyHost != req.Host {
		s.Proxy.Address = s.ProxyHost
	}
	//通知回调 即将开始发送请求
	s.CallbackBeforeRequest()
	//回调中设置 不发送 直接响应指定数据 或终止发送
	if s.Response != nil {
		_, _ = s.RwObj.Write(public.StructureBody(s.Response))
		return
	}
	var err error
	//验证处理是否websocket请求,如果是直接处理
	if s.handleWss() {
		return
	}
	err = s.doRequest()

	if err != nil || s.Response == nil {
		if s.Response == nil && err == nil {
			err = errors.New("[Sunny]No data obtained")
		}
		s.Error(err)
		return
	}
	if s.Response.Header == nil {
		err = errors.New("[Sunny]Response.Header=null")
		s.Error(err)
		return
	}
	setOut := func() {
		//大数据转发时调用，避免超时问题
		if s.WinHttp != nil {
			if s.WinHttp.WinPool != nil {
				_ = s.WinHttp.WinPool.SetDeadline(time.Time{})
			}
		}

		if s.Conn != nil {
			_ = s.Conn.SetDeadline(time.Time{})
		}
	}
	SetReqHeadsValue := func(DataLen string) []byte {
		if s.Response != nil {
			if s.Response.Header != nil {
				for k, _ := range s.Response.Header {
					ks := strings.ToUpper(k)
					if ks == "CONTENT-LENGTH" {
						delete(s.Response.Header, k)
					}
				}
			}
		}
		if DataLen != "-1" {
			s.Response.Header.Set("Content-Length", DataLen)
		}
		return public.ResponseToHeader(s.Response)
	}
	SetBodyValue := func(bs []byte, err error) []byte {
		if s.Response == nil {
			return []byte{}
		}
		if err != nil {
			s.Error(err)
			return nil
		}
		if s.Response.Body != nil {
			_ = s.Response.Body.Close()
		}
		s.Response.Body = ioutil.NopCloser(bytes.NewBuffer(bs))
		//通知回调，已经请求完成
		s.CallbackBeforeResponse()
		b, _ := s.ReadAll(s.Response.Body)
		if s.Response.Body != nil {
			_ = s.Response.Body.Close()
		}
		return b
	}
	Length, _ := strconv.Atoi(s.Response.Header.Get("Content-Length"))
	if Length < 1 {
		if s.Response != nil {
			if s.Response.Header != nil {
				for k, v := range s.Response.Header {
					ks := strings.ToUpper(k)
					if ks == "CONTENT-LENGTH" {
						if len(v) > 0 {
							Length, _ = strconv.Atoi(v[0])
							break
						}
					}
				}
			}
		}
	}
	Method := ""
	if req != nil {
		Method = req.Method
	}

	public.CopyBuffer(s.RwObj, s.Response.Body, s.Conn, s.WinHttp.WinPool, SetBodyValue, Length, SetReqHeadsValue, s.Response.Header.Get("Content-Type"), setOut, Method)
}
func (s *ProxyRequest) sendHttp(req *http.Request) {
	if req.URL == nil {
		return
	}
	if s.isCerDownloadPage(req) { // 安装移动端证书
		return
	}
	s.CompleteRequest(req) // HTTP 请求

}
func (s *ProxyRequest) ReadAll(r io.Reader) ([]byte, error) {
	var bufBuffer bytes.Buffer
	b := make([]byte, 4096)
	defer func() {
		b = make([]byte, 0)
		bufBuffer.Reset()
		bufBuffer.Grow(0)
		b = nil
	}()
	for {
		n, err := r.Read(b[0:])
		bufBuffer.Write(b[0:n])
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return public.CopyBytes(bufBuffer.Bytes()), err
		}
	}
}

/*
SocketForward
MsgType ==1 dst=服务器端 src=客户端
MsgType ==2 dst=客户端 src=服务器端
*/
func (s *ProxyRequest) SocketForward(dst bufio.Writer, src *public.ReadWriteObject, MsgType int, t1, t2 net.Conn, TCP *public.TCP, isHttpReq *bool) {
	as := &public.TcpMsg{}
	buf := make([]byte, 4096)
	defer func() {
		if err := recover(); err != nil {
			fmt.Println("SocketForward 出了错：", err)
		}
		as.Data.Reset()
		//是否已经纠正为了HTTP请求
		if !*isHttpReq {
			//如果没有纠正，退出SocketForward函数时将关闭socket会话
			buf = nil
			if t1 != nil {
				_ = t1.Close()
			}
			if t2 != nil {
				_ = t2.Close()
			}
		}
	}()
	if t1 == nil {
		return
	}
	firstRequest := true //是否是首次接收请求
	if s.Global.isMustTcp || s.NoRepairHttp {
		firstRequest = false
	}
	for {
		TCP.L.Lock()
		_ = t1.SetDeadline(time.Now().Add(165 * time.Second))
		_ = t2.SetDeadline(time.Now().Add(165 * time.Second))
		TCP.L.Unlock()
		if firstRequest {
			//是否是客户端发送数据
			if MsgType == public.SunnyNetMsgTypeTCPClientSend {
				{
					//提取取出1个字节,
					peek, e := src.Peek(1)
					if e == nil {
						if len(peek) > 0 {
							//判断是否是HTTP请求
							if public.IsHTTPRequest(peek[0], src) {
								_ = t2.Close()
								//如果是，那么关闭本次连接服务器的socket，并且纠正为HTTP请求，后续交给HTTP请求处理函数继续处理
								*isHttpReq = true
								return
							}
						}
					}
					if s.Global.disableTCP {
						_ = t1.Close()
						_ = t2.Close()
						return
					}
				}
			}
			firstRequest = false
		}

		nr, er := src.Read(buf[0:]) // io.ReadAtLeast(src, buf[0:], 1)
		if nr > 0 {
			as.Data.Reset()
			as.Data.Write(buf[0:nr])
			s.CallbackTCPRequest(MsgType, as)
			if as.Data.Len() < 1 {
				continue
			}
			TCP.L.Lock()
			nw, ew := dst.Write(as.Data.Bytes())
			er = dst.Flush()
			TCP.L.Unlock()
			if nw != as.Data.Len() || ew != nil {
				break
			}
		}
		if er != nil {
			return
		}

	}
}

// Sunny  请使用 NewSunny 方法 请不要直接构造
type Sunny struct {
	certCache             *Cache
	disableTCP            bool                 //禁止TCP连接
	certificates          []byte               //CA证书原始数据
	rootCa                *x509.Certificate    //中间件CA证书
	rootKey               *rsa.PrivateKey      // 证书私钥
	initCertOK            bool                 // 是否已经初始化证书
	port                  int                  //启动的端口号
	Error                 error                //错误信息
	tcpSocket             *net.Listener        //TcpSocket服务器
	udpSocket             *net.UDPConn         //UdpSocket服务器
	connList              map[int64]net.Conn   //会话连接客户端、停止服务器时可以全部关闭
	lock                  sync.Mutex           //会话连接互斥锁
	socket5VerifyUser     bool                 //S5代理是否需要验证账号密码
	socket5VerifyUserList map[string]string    //S5代理需要验证的账号密码列表
	socket5VerifyUserLock sync.Mutex           //S5代理验证时的锁
	isMustTcp             bool                 //强制走TCP
	httpCallback          int                  //http 请求回调地址
	tcpCallback           int                  //TCP请求回调地址
	websocketCallback     int                  //ws请求回调地址
	udpCallback           int                  //udp请求回调地址
	goHttpCallback        func(Conn *HttpConn) //http请求GO回调地址
	goTcpCallback         func(Conn *TcpConn)  //TCP请求GO回调地址
	goWebsocketCallback   func(Conn *WsConn)   //ws请求GO回调地址
	goUdpCallback         func(Conn *UDPConn)  //UDP请求GO回调地址
	proxy                 *GoWinHttp.Proxy     //全局上游代理
	proxyRegexp           *regexp.Regexp       //上游代理使用规则
	mustTcpRegexp         *regexp.Regexp       //强制走TCP规则,如果 isMustTcp 打开状态,本功能则无效
	isRun                 bool                 //是否在运行中
	fixedTLS              []uint16             //固定的TLS指纹
	isRandomTLS           bool                 //是否随机使用TLS指纹
	randomTLSValue        []uint16             //tls 指纹选项合集
	SunnyContext          int
}

var defaultManager = func() int {
	i := Certificate.CreateCertificate()
	c := Certificate.LoadCertificateContext(i)
	if c == nil {
		panic(errors.New("创建证书管理器错误！！"))
	}
	c.LoadX509Certificate(public.NULL, public.RootCa, public.RootKey)
	return i
}()

// NewSunny 创建一个中间件
func NewSunny() *Sunny {
	SunnyContext := NewMessageId()
	a, _ := regexp.Compile("ALL")
	s := &Sunny{SunnyContext: SunnyContext, certCache: NewCache(), connList: make(map[int64]net.Conn), socket5VerifyUserList: make(map[string]string), proxy: &GoWinHttp.Proxy{}, proxyRegexp: a}
	s.randomTLSValue = make([]uint16, public.RandomTLSValueArrayLen)
	copy(s.randomTLSValue, public.RandomTLSValueArray)
	s.SetCert(defaultManager)
	SunnyStorageLock.Lock()
	SunnyStorage[s.SunnyContext] = s
	SunnyStorageLock.Unlock()
	return s
}

// SetRandomTLS 是否使用随机TLS指纹
func (s *Sunny) SetRandomTLS(open bool) {
	if s == nil {
		return
	}
	s.lock.Lock()
	s.isRandomTLS = open
	s.fixedTLS = nil
	s.lock.Unlock()
}

// GetTLSValues 获取固定的TLS指纹列表或随机TLS指纹列表,如果未开启使用随机TLS指纹,并且未设置固定TLS指纹,则返回nil
func (s *Sunny) GetTLSValues() []uint16 {
	if s == nil {
		return nil
	}
	s.lock.Lock()
	if !s.isRandomTLS {
		if len(s.fixedTLS) > 0 {
			s.lock.Unlock()
			return s.fixedTLS
		}
		s.lock.Unlock()
		return nil
	}
	n := mrand.Intn(public.RandomTLSValueArrayLen) + 1
	for i := public.RandomTLSValueArrayLen - 1; i > 0; i-- {
		j := mrand.Intn(i + 1)
		s.randomTLSValue[i], s.randomTLSValue[j] = s.randomTLSValue[j], s.randomTLSValue[i]
	}
	shuffledArray := make([]uint16, n)
	copy(shuffledArray, s.randomTLSValue[:n])
	s.lock.Unlock()
	return shuffledArray
}

// GetTLSTestValues 随机生成一个TLS指纹列表
func (s *Sunny) GetTLSTestValues() []uint16 {
	if s == nil {
		return nil
	}
	s.lock.Lock()
	n := mrand.Intn(public.RandomTLSValueArrayLen) + 1
	for i := public.RandomTLSValueArrayLen - 1; i > 0; i-- {
		j := mrand.Intn(i + 1)
		s.randomTLSValue[i], s.randomTLSValue[j] = s.randomTLSValue[j], s.randomTLSValue[i]
	}
	shuffledArray := make([]uint16, n)
	copy(shuffledArray, s.randomTLSValue[:n])
	s.lock.Unlock()
	return shuffledArray
}

// SetRandomFixedTLS 是否使用固定的TLS指纹 请注意这个函数是一个全局函数
func (s *Sunny) SetRandomFixedTLS(value string) {
	m := strings.Split(value, ",")
	array := make([]uint16, 0)
	for _, v := range m {
		zm, _ := strconv.Atoi(strings.TrimSpace(v))
		array = append(array, uint16(zm))
	}
	s.lock.Lock()
	s.fixedTLS = array
	s.lock.Unlock()
}

// SetMustTcpRegexp 设置强制走TCP规则,如果 打开了全部强制走TCP状态,本功能则无效
func (s *Sunny) SetMustTcpRegexp(RegexpList string) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	r := strings.ReplaceAll("^"+strings.ReplaceAll(RegexpList, " ", "")+"$", "\r", "")
	r = strings.ReplaceAll(r, "\t", "")
	r = strings.ReplaceAll(r, "\n", ";")
	r = strings.ReplaceAll(r, ";", "$|^")
	r = strings.ReplaceAll(r, ".", "\\.")
	r = strings.ReplaceAll(r, "*", ".*.?")
	if r == "" {
		r = "ALL"
	}
	a, e := regexp.Compile(r)
	if e == nil {
		s.mustTcpRegexp = a
	} else {
		s.mustTcpRegexp = nil
	}
	return e
}

// CompileProxyRegexp 创建上游代理使用规则
func (s *Sunny) CompileProxyRegexp(Regexp string) error {
	r := strings.ReplaceAll("^"+strings.ReplaceAll(Regexp, " ", "")+"$", "\r", "")
	r = strings.ReplaceAll(r, "\t", "")
	r = strings.ReplaceAll(r, "\n", ";")
	r = strings.ReplaceAll(r, ";", "$|^")
	r = strings.ReplaceAll(r, ".", "\\.")
	r = strings.ReplaceAll(r, "*", ".*.?")
	if r == "" {
		r = "ALL" //让其全部匹配失败，也就是全部使用上游代理代理
	}
	a, e := regexp.Compile(r)
	if e == nil {
		s.proxyRegexp = a
	} else {
		a1, _ := regexp.Compile("ALL")
		s.proxyRegexp = a1 //让其全部匹配失败，也就是全部使用上游代理代理
	}
	return e
}

// MustTcp 设置是否强制全部走TCP
func (s *Sunny) MustTcp(open bool) {
	s.isMustTcp = open
}
func (s *Sunny) generatePem(host string) ([]byte, []byte, error) {
	serialNumber, _ := rand.Int(rand.Reader, public.MaxBig)
	template := x509.Certificate{
		SerialNumber: serialNumber, // SerialNumber 是 CA 颁布的唯一序列号，在此使用一个大随机数来代表它
		Subject: pkix.Name{ //Name代表一个X.509识别名。只包含识别名的公共属性，额外的属性被忽略。
			CommonName: host,
		},
		NotBefore:      time.Now().AddDate(0, 0, -1),
		NotAfter:       time.Now().AddDate(0, 0, 365),
		KeyUsage:       x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature, //KeyUsage 与 ExtKeyUsage 用来表明该证书是用来做服务器认证的
		ExtKeyUsage:    []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},               // 密钥扩展用途的序列
		EmailAddresses: []string{"forward.nice.cp@gmail.com"},
	}
	if ip := net.ParseIP(host); ip != nil {
		template.IPAddresses = []net.IP{ip}
	} else {
		template.DNSNames = []string{host}
	}

	cer, err := x509.CreateCertificate(rand.Reader, &template, s.rootCa, &s.rootKey.PublicKey, s.rootKey)
	if err != nil {
		return nil, nil, err
	}

	return pem.EncodeToMemory(&pem.Block{ // 证书
			Type:  "CERTIFICATE",
			Bytes: cer,
		}), pem.EncodeToMemory(&pem.Block{ // 私钥
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(s.rootKey),
		}), err
}

func (s *Sunny) getCertificate(host string) (*tls.Certificate, error) {
	in := HttpCertificate.GetTlsConfigSunny(host, public.CertificateRequestManagerRulesReceive)
	if in != nil {
		if len(in.Certificates) > 0 {
			return &in.Certificates[0], nil
		}
	}
	certificate, err := s.certCache.GetOrStore(host, func() (interface{}, error) {
		mHost, _, err := public.SplitHostPort(host)
		if err != nil {
			return nil, err
		}
		certByte, priByte, err := s.generatePem(mHost)
		if err != nil {
			return nil, err
		}
		certificate, err := tls.X509KeyPair(certByte, priByte)
		if err != nil {
			return nil, err
		}
		return certificate, nil
	})
	if certificate == nil {
		return nil, err
	}
	i := certificate.(tls.Certificate)
	return &i, err
}

// Socket5VerifyUser S5代理是否需要验证账号密码
func (s *Sunny) Socket5VerifyUser(n bool) *Sunny {
	s.socket5VerifyUser = n
	return s
}

// Socket5AddUser S5代理添加需要验证的账号密码
func (s *Sunny) Socket5AddUser(u, p string) *Sunny {
	s.socket5VerifyUserLock.Lock()
	s.socket5VerifyUserList[u] = p
	s.socket5VerifyUserLock.Unlock()
	return s
}

// Socket5DelUser S5代理删除需要验证的账号
func (s *Sunny) Socket5DelUser(u string) *Sunny {
	s.socket5VerifyUserLock.Lock()
	delete(s.socket5VerifyUserList, u)
	s.socket5VerifyUserLock.Unlock()
	return s
}

// ExportCert 获取证书原内容
func (s *Sunny) ExportCert() []byte {
	ar := strings.Split(strings.ReplaceAll(string(s.certificates), "\r", public.NULL), "\n")
	var b bytes.Buffer
	for _, v := range ar {
		if strings.Index(v, ": ") == -1 && len(v) > 0 {
			b.WriteString(v + "\r\n")
		}
	}
	return public.CopyBytes(b.Bytes())
}

// SetIeProxy 设置IE代理 [Off=true 取消] [Off=false 设置] 在中间件设置端口后调用
func (s *Sunny) SetIeProxy(Off bool) bool {
	return CrossCompiled.SetIeProxy(Off, s.Port())
}

// SetGlobalProxy 设置全局上游代理 仅支持Socket5和http 例如 socket5://admin:123456@127.0.0.1:8888 或 http://admin:123456@127.0.0.1:8888
func (s *Sunny) SetGlobalProxy(ProxyUrl string) bool {
	if s.proxy == nil {
		s.proxy = &GoWinHttp.Proxy{Timeout: 60 * 1000}
	}
	s.proxy.Address = ""
	s.proxy.Pass = ""
	s.proxy.User = ""
	proxy, err := url.Parse(ProxyUrl)
	if err != nil || proxy == nil {
		return false
	}

	if proxy.Scheme != "http" && proxy.Scheme != "socks5" && proxy.Scheme != "socket5" && proxy.Scheme != "socket" {
		return false
	}
	if len(proxy.Host) < 3 {
		s.proxy.S5TypeProxy = false
		s.proxy.Address = ""
		s.proxy.User = ""
		s.proxy.Pass = ""
		return false
	}
	s.proxy.S5TypeProxy = proxy.Scheme != "http"
	s.proxy.Address = proxy.Host
	s.proxy.User = proxy.User.Username()
	p, ok := proxy.User.Password()
	if ok {
		s.proxy.Pass = p
	}
	return true
}

// InstallCert 安装证书 将证书安装到Windows系统内
func (s *Sunny) InstallCert() string {
	return CrossCompiled.InstallCert(s.certificates)
}

// SetCert 设置证书
func (s *Sunny) SetCert(ManagerId int) *Sunny {
	Manager := Certificate.LoadCertificateContext(ManagerId)
	if Manager == nil {
		s.Error = errors.New("CertificateManager invalid ")
		return s
	}

	var err error
	s.initCertOK = false
	p, _ := pem.Decode([]byte(Manager.ExportCA()))
	s.certificates = nil
	s.rootCa, err = x509.ParseCertificate(p.Bytes)
	if err != nil {
		s.Error = err
		return s
	}
	s.certificates = []byte(Manager.ExportCA())
	p1, _ := pem.Decode([]byte(Manager.ExportKEY()))
	if p1 == nil {
		s.Error = errors.New("Key证书解析失败 ")
		return s
	}
	s.rootKey, err = x509.ParsePKCS1PrivateKey(p1.Bytes)
	if err != nil {
		k, e := x509.ParsePKCS8PrivateKey(p1.Bytes)
		if e != nil {
			s.Error = errors.New(err.Error() + " or " + e.Error())
			return s
		}
		kk := k.(*rsa.PrivateKey)
		if kk == nil {
			s.Error = err
			return s
		}
		s.rootKey = kk
	}
	s.initCertOK = true
	return s
}

// SetPort 设置端口号
func (s *Sunny) SetPort(Port int) *Sunny {
	s.port = Port
	return s
}

// DisableTCP 禁用TCP
func (s *Sunny) DisableTCP(disable bool) {
	s.disableTCP = disable
}

// Port 获取端口号
func (s *Sunny) Port() int {
	return s.port
}

// SetCallback 设置回调地址
func (s *Sunny) SetCallback(httpCall, tcpCall, wsCall, udpCall int) *Sunny {
	s.httpCallback = httpCall
	s.tcpCallback = tcpCall
	s.websocketCallback = wsCall
	s.udpCallback = udpCall
	return s
}

// SetGoCallback 设置Go回调地址
func (s *Sunny) SetGoCallback(httpCall func(Conn *HttpConn), tcpCall func(Conn *TcpConn), wsCall func(Conn *WsConn), udpCall func(Conn *UDPConn)) *Sunny {
	s.goHttpCallback = httpCall
	s.goTcpCallback = tcpCall
	s.goWebsocketCallback = wsCall
	s.goUdpCallback = udpCall
	return s
}

// StartProcess 开始进程代理
func (s *Sunny) StartProcess() bool {
	if CrossCompiled.NFapi_IsInit() {
		if CrossCompiled.NFapi_ProcessPortInt() != 0 && CrossCompiled.NFapi_SunnyPointer() != uintptr(unsafe.Pointer(s)) {
			CrossCompiled.NFapi_MessageBox("启动失败：", "已在其他Sunny对象启动\r\n\r\n不能多次加载驱动", 0x00000010)
			return false
		}
		CrossCompiled.NFapi_SunnyPointer(uintptr(unsafe.Pointer(s)))
		return true
	}
	CrossCompiled.NFapi_SunnyPointer(uintptr(unsafe.Pointer(s)))
	CrossCompiled.NFapi_ProcessPortInt(uint16(s.Port()))
	CrossCompiled.NFapi_IsInit(CrossCompiled.NFapi_ApiInit())
	CrossCompiled.NFapi_UdpSendReceiveFunc(s.udpNFSendReceive)
	return CrossCompiled.NFapi_IsInit()
}

// ProcessALLName 是否允许所有进程通过 所有 SunnyNet 通用,
// 请注意GoLang调试时候，请不要使用此命令，因为不管开启或关闭，都会将当前所有TCP链接断开一次
// 因为如果不断开的一次的话,已经建立的TCP链接无法抓包。
// Go程序调试，是通过TCP连接的，若使用此命令将无法调试。
func (s *Sunny) ProcessALLName(open bool) *Sunny {
	CrossCompiled.NFapi_SetHookProcess(open)
	//CrossCompiled.NFapi_ClosePidTCP(-1)
	return s
}

// ProcessDelName 删除进程名  所有 SunnyNet 通用
func (s *Sunny) ProcessDelName(name string) *Sunny {
	CrossCompiled.NFapi_DelName(name)
	//CrossCompiled.NFapi_CloseNameTCP(name)
	return s
}

// ProcessAddName 进程代理 添加进程名 所有 SunnyNet 通用
func (s *Sunny) ProcessAddName(Name string) *Sunny {
	CrossCompiled.NFapi_AddName(Name)
	//CrossCompiled.NFapi_CloseNameTCP(Name)
	return s
}

// ProcessDelPid 删除PID  所有 SunnyNet 通用
func (s *Sunny) ProcessDelPid(Pid int) *Sunny {
	CrossCompiled.NFapi_DelPid(uint32(Pid))
	//CrossCompiled.NFapi_ClosePidTCP(Pid)
	return s
}

// ProcessAddPid 进程代理 添加PID 所有 SunnyNet 通用
func (s *Sunny) ProcessAddPid(Pid int) *Sunny {
	CrossCompiled.NFapi_AddPid(uint32(Pid))
	//CrossCompiled.NFapi_ClosePidTCP(Pid)
	return s
}

// ProcessCancelAll 进程代理 取消全部已设置的进程名
func (s *Sunny) ProcessCancelAll() *Sunny {
	CrossCompiled.NFapi_CancelAll()
	//CrossCompiled.NFapi_ClosePidTCP(-1)
	return s
}

// Start 开始启动  调用 Error 获取错误信息 成功=nil
func (s *Sunny) Start() *Sunny {
	if s.isRun {
		s.Error = errors.New("已在运行中")
		return s
	}

	if s.port == 0 {
		s.Error = errors.New("The port number is not set ")
		return s
	}
	if !s.initCertOK {
		return s
	}
	tcpListen, err := net.Listen("tcp", "0.0.0.0:"+strconv.Itoa(s.port))
	if err != nil {
		s.Error = err
		return s
	}
	udpListenAddr, err := net.ResolveUDPAddr("udp", "0.0.0.0:"+strconv.Itoa(s.port))
	if err != nil {
		s.Error = err
		_ = tcpListen.Close()
		return s
	}
	udpListen, err := net.ListenUDP("udp", udpListenAddr)
	if err != nil {
		s.Error = err
		_ = tcpListen.Close()
		return s
	}
	s.udpSocket = udpListen
	s.tcpSocket = &tcpListen
	s.Error = err
	s.isRun = true
	if CrossCompiled.NFapi_SunnyPointer() == uintptr(unsafe.Pointer(s)) {
		CrossCompiled.NFapi_ProcessPortInt(uint16(s.port))
		CrossCompiled.NFapi_UdpSendReceiveFunc(s.udpNFSendReceive)
	}
	go s.listenTcpGo()
	go s.listenUdpGo()
	return s
}

// Close 关闭服务器
func (s *Sunny) Close() *Sunny {
	if s.tcpSocket != nil {
		_ = (*s.tcpSocket).Close()
	}
	if s.udpSocket != nil {
		_ = s.udpSocket.Close()
	}
	s.lock.Lock()
	for k, conn := range s.connList {
		_ = conn.Close()
		delete(s.connList, k)
	}
	if CrossCompiled.NFapi_SunnyPointer() == uintptr(unsafe.Pointer(s)) {
		CrossCompiled.NFapi_ProcessPortInt(0)
	}
	s.lock.Unlock()
	return s
}

// listenTcpGo 循环监听
func (s *Sunny) listenTcpGo() {
	defer func() {
		if s.tcpSocket != nil || s.udpSocket != nil {
			s.Close()
		}
	}()
	defer func() { s.isRun = false }()
	for {
		c, err := (*s.tcpSocket).Accept()
		if err != nil && strings.Index(err.Error(), "timeout") == -1 {
			s.Error = err
			break
		}
		if err == nil {
			go s.handleClientConn(c, nil)
		}
	}
}

func (s *Sunny) handleClientConn(conn net.Conn, tgt *TargetInfo) {
	Theoni := atomic.AddInt64(&public.Theology, 1)
	//存入会话列表 方便停止时，将所以连接断开
	s.lock.Lock()
	s.connList[Theoni] = conn
	s.lock.Unlock()
	//构造一个请求中间件
	req := &ProxyRequest{Global: s, TcpCall: s.tcpCallback, HttpCall: s.httpCallback, wsCall: s.websocketCallback, TcpGoCall: s.goTcpCallback, HttpGoCall: s.goHttpCallback, wsGoCall: s.goWebsocketCallback} //原始请求对象
	defer req.delSocket5User()
	defer func() {
		//当 handleClientConn 函数 即将退出时 从会话列表中删除当前会话
		_ = conn.Close()
		s.lock.Lock()
		delete(s.connList, Theoni)
		s.lock.Unlock()
		//当 handleClientConn 函数 即将退出时 销毁 请求中间件 中的一些信息，避免内存泄漏
		req.RwObj = nil
		req.Conn = nil
		req.Global = nil
		req.WinHttp = nil
		req.Response = nil
		req.Request = nil
		req.Target = nil
		conn = nil
		req = nil
	}()
	//请求中间件一些必要参数赋值
	req.Conn = conn                             //请求会话
	req.RwObj = public.NewReadWriteObject(conn) //构造客户端读写对象
	req.Theology = int(Theoni)                  //当前请求唯一ID
	if tgt == nil {
		req.Target = &TargetInfo{} //构建一个请求连接信息，后续解析到值后会进行赋值
	} else {
		req.Target = tgt
	}
	switch addr := conn.RemoteAddr().(type) {
	case *net.TCPAddr:
		u := uint16(addr.Port)
		//这里是判断 是否是通过 NFapi 驱动进来的数据
		if runtime.GOOS == "windows" {
			info := CrossCompiled.NFapi_GetTcpConnectInfo(u)
			if info != nil {
				//如果是 通过 NFapi 驱动进来的数据 对连接信息进行赋值
				req.Pid = info.Pid
				req.Target.Parse(info.RemoteAddress, info.RemoteProt, info.V6)
				req.Target.Parse(req.Target.String(), 0)
				//然后进行数据处理,按照HTTPS数据进行处理
				req.https()
				CrossCompiled.NFapi_DelTcpConnectInfo(u)
				CrossCompiled.NFapi_API_NfTcpClose(info.Id)
				return
			}
		}
		break
	default:
		break
	}
	req.Pid = CrossCompiled.GetTcpInfoPID(conn.RemoteAddr().String(), s.port)
	//若不是 通过 NFapi 驱动进来的数据 那么就是通过代理传递过来的数据
	//进行预读1个字节的数据
	peek, err := req.RwObj.Peek(1)
	if err != nil {
		//读取1个字节失败直接返回
		return
	}
	//如果第一个字节是0x05 说明是通过S5代理连接的
	if peek[0] == 0x05 {
		//进行S5鉴权
		if req.Socks5ProxyVerification() == false {
			return
		}
		if s.isMustTcp {
			if s.disableTCP {
				return
			}
			//如果开启了强制走TCP ，则按TCP处理流程处理
			req.MustTcpProcessing(nil, public.TagMustTCP)
			return
		}
		//如果没有开启强制走TCP，则按https 数据进行处理
		req.https()
		return
	}
	//如果没有开启用户身份验证 且 第一个字节是 22 或 23 说明可能是透明代理
	if s.socket5VerifyUser == false && (peek[0] == 22 || peek[0] == 23) {
		//按透明代理处理流程处理
		req.transparentProcessing()
		return
	}
	//如果没有开启用户身份验证 且 第一个字节符合HTTP/S 请求头
	if s.socket5VerifyUser == false && public.IsHTTPRequest(peek[0], req.RwObj) {
		//按照http请求处理
		req.httpProcessing(nil, "80", public.TagTcpAgreement)
	}
}

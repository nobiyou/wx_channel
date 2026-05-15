package SunnyNet

import (
	"bytes"
	"fmt"
	"github.com/qtgolang/SunnyNet/Call"
	"github.com/qtgolang/SunnyNet/public"
	NFapi "github.com/qtgolang/SunnyNet/src/nfapi"
	"net"
	"sync/atomic"
	"time"
)

func getFromLen(b []byte) int {
	if len(b) < 1 {
		return 0
	}
	var startPos = 1
	var addrLen int
	switch b[0] {
	case public.Socks5typeDomainName:
		if len(b) < 2 {
			return 0
		}
		startPos++
		addrLen = int(b[1])
	case public.Socks5typeIpv4:
		addrLen = net.IPv4len
	case public.Socks5typeIpv6:
		addrLen = net.IPv6len
	default:
		return 0
	}
	endPos := startPos + addrLen + 2
	if len(b) < endPos {
		return 0
	}
	return endPos + 3
}
func resolveConnectionAddress(LocalAddress *net.UDPAddr, a []byte) *udpInfo {
	if len(a) < 10 {
		return nil
	}
	var RwObj bytes.Buffer
	RwObj.Write(a)
	_, _ = RwObj.ReadByte()     //不知道为啥前面多了个 0
	_, _ = RwObj.ReadByte()     //保留位
	_, _ = RwObj.ReadByte()     //分片位
	aTyp, _ := RwObj.ReadByte() //地址类型
	if aTyp != public.Socks5typeDomainName &&
		aTyp != public.Socks5typeIpv4 &&
		aTyp != public.Socks5typeIpv6 {
		return nil
	}
	hostname := public.NULL
	switch {
	case aTyp == public.Socks5typeIpv4:
		{
			IPv4Buf := make([]byte, 4)
			nr, err := RwObj.Read(IPv4Buf)
			if err != nil || nr != 4 {
				return nil
			}
			ip := net.IP(IPv4Buf)
			hostname = ip.String()
		}
	case aTyp == public.Socks5typeIpv6:
		{
			IPv6Buf := make([]byte, 16)
			nr, err := RwObj.Read(IPv6Buf)
			if err != nil || nr != 16 {
				return nil
			}
			ip := net.IP(IPv6Buf)
			hostname = ip.String()
		}
	case aTyp == public.Socks5typeDomainName:
		{
			dnLen, err := RwObj.ReadByte()
			if err != nil || int(dnLen) < 0 {
				return nil
			}

			domain := make([]byte, dnLen)
			nr, err := RwObj.Read(domain)
			if err != nil || nr != int(dnLen) {
				return nil
			}
			hostname = string(domain)
		}
	}
	portNum1, err := RwObj.ReadByte()
	if err != nil {
		return nil
	}
	portNum2, err := RwObj.ReadByte()
	if err != nil {
		return nil
	}
	port := uint16(portNum1)<<8 + uint16(portNum2)
	FromLen := getFromLen(a[3:])
	return &udpInfo{LocalAddress: LocalAddress, RemoteAddress: fmt.Sprintf("%s:%d", hostname, port), Data: RwObj.Bytes(), From: a[0:FromLen]}
}

type udpInfo struct {
	LocalAddress  *net.UDPAddr
	RemoteAddress string
	Data          []byte
	From          []byte
}

// 实现 Sunny 结构体的 listenUdpGo 方法，用于循环监听 UDP 连接
func (s *Sunny) listenUdpGo() {
	defer func() {
		if s.tcpSocket != nil || s.udpSocket != nil {
			s.Close()
		}
	}()
	defer func() { s.isRun = false }()
	// 创建指定大小的缓冲区
	buffer := make([]byte, 65536)
	// 循环接收 UDP 数据
	for {
		// 从 UDP Socket 中读取数据
		n, addr, err := s.udpSocket.ReadFromUDP(buffer)
		if err != nil {
			break
		}
		bs := public.CopyBytes(buffer[:n])
		// 解析连接地址并生成唯一键值
		_info := resolveConnectionAddress(addr, bs)
		if _info == nil {
			continue
		}
		k := addr.String() + _info.RemoteAddress
		// 如果连接池中不存在该连接，则新建连接并添加到连接池中
		if c, Tid := NFapi.UdpSenders.Get(addr.String() + _info.RemoteAddress); c == nil {
			Tid = atomic.AddInt64(&public.Theology, 1)
			serverAddr, er := net.ResolveUDPAddr("udp", _info.RemoteAddress)
			if er != nil {
				continue
			}
			conn, er := net.DialUDP("udp", nil, serverAddr)
			if er != nil {
				continue
			}
			NFapi.UdpSenders.Add(k, conn, Tid, nil, nil, s.udpSocket, _info.LocalAddress, _info.From)
			NFapi.NfAddTid(0, Tid, k)
			go s.goUdp(_info, Tid, addr.String(), _info.RemoteAddress, conn)
		}
		// 获取连接并发送数据
		conn, Tid := NFapi.UdpSenders.Get(k)
		if conn != nil {
			bs := s.udpNFSendReceive(public.SunnyNetUDPTypeSend, Tid, 0, addr.String(), _info.RemoteAddress, _info.Data)
			_, _ = conn.Write(bs)
		}
	}
}

// 实现 Sunny 结构体的 goUdp 方法，用于处理 UDP 连接
func (s *Sunny) goUdp(info *udpInfo, tid int64, Local, Remote string, conn *net.UDPConn) {
	// 创建指定大小的缓冲区
	buff := make([]byte, 65536)
	// 循环读取 UDP 数据
	for {
		// 设置读取超时时间并读取 UDP 数据
		_ = conn.SetReadDeadline(time.Now().Add(time.Duration(5) * time.Second))
		nt, _, _ := conn.ReadFromUDP(buff)
		if nt == 0 {
			break
		}
		// 调用 udpNFSendReceive 方法发送并接收数据，并将返回的数据添加来源信息
		bs := s.udpNFSendReceive(public.SunnyNetUDPTypeReceive, tid, 0, Local, Remote, buff[:nt])
		if len(bs) < 1 {
			continue
		}
		var data []byte
		data = append(data, info.From...)
		data = append(data, bs...)
		// 将处理后的数据写入 Socket 中
		_, _ = s.udpSocket.WriteToUDP(data, info.LocalAddress)
	}
	// 从连接池中移除 UDP 连接并发送关闭连接的消息
	NFapi.UdpSenders.Del(info.LocalAddress.String() + info.RemoteAddress)
	s.udpNFSendReceive(public.SunnyNetUDPTypeClosed, tid, 0, Local, Remote, nil)
	// 删除 唯一ID
	NFapi.NfDelTid(tid)
}

func (s *Sunny) udpNFSendReceive(Type int8, Theoni int64, pid uint32, LocalAddress, RemoteAddress string, data []byte) []byte {
	// 如果回调函数小于 10，则尝试调用Go回调函数
	if s.udpCallback < 10 {
		if s.goUdpCallback != nil {
			n := &UDPConn{Theology: Theoni, MessageId: NewMessageId(), Type: Type, SunnyContext: s.SunnyContext, Pid: int(pid), LocalAddress: LocalAddress, RemoteAddress: RemoteAddress, Data: data}
			s.goUdpCallback(n)
			return n.Data
		}
		return data
	}
	// 生成消息 ID 并将数据写入 buffer 中
	MessageId := NewMessageId()
	var buff bytes.Buffer
	buff.Write(data)

	// 获取锁并将 buffer 存储到 UdpMap 中
	NFapi.UdpSync.Lock()
	NFapi.UdpMap[MessageId] = &buff
	NFapi.UdpSync.Unlock()
	// 调用回调函数，并传入相关参数
	Call.Call(s.udpCallback, s.SunnyContext, LocalAddress, RemoteAddress, int(Type), MessageId, int(Theoni), int(pid))
	// 获取锁并从 UdpMap 中获取返回值
	NFapi.UdpSync.Lock()
	rBody := NFapi.UdpMap[MessageId]
	delete(NFapi.UdpMap, MessageId)
	NFapi.UdpSync.Unlock()
	// 如果返回值为空，则返回原始数据
	if rBody == nil {
		return data
	}
	// 否则返回返回值的字节切片
	return rBody.Bytes()
}

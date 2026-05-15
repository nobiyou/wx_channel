package NFapi

import (
	"bytes"
	"net"
	"sync"
)

// 定义 UdpConnectionManagement 结构体，用于管理 UDP 连接
var UdpSenders UdpConnectionManagement
var UdpLock sync.Mutex

type NfSend struct {
	Id            uint64
	RemoteAddress *SockaddrInx
	options       *NF_UDP_OPTIONS
}

// 实现 UdpConnectionManagement 结构体的 Add 方法，用于将 UDP 连接添加到连接池中，并返回添加的 UDP 选项
func (p *UdpConnectionManagement) Add(key string, conn *net.UDPConn,
	Tid int64, Send *NfSend, receive *NfSend,
	ClientConn *net.UDPConn, ClientAddress *net.UDPAddr, ClientFrom []byte) {
	// 获取锁
	p.l.Lock()

	// 如果连接池为空，则创建一个新的连接池
	if p.m == nil {
		p.m = make(map[string]*UdpConnection)
	}

	// 将 UDP 连接添加到连接池中
	p.m[key] = &UdpConnection{Send: Send, Receive: receive, Theoni: Tid, Conn: conn, ClientConn: ClientConn, ClientAddress: ClientAddress, ClientFrom: ClientFrom}

	// 释放锁并返回 UDP 选项
	p.l.Unlock()
}

// 实现 UdpConnectionManagement 结构体的 Del 方法，用于从连接池中移除指定的 UDP 连接
func (p *UdpConnectionManagement) Del(key string) {
	// 获取锁
	p.l.Lock()
	// 如果连接池为空，则创建一个新的连接池
	if p.m == nil {
		p.m = make(map[string]*UdpConnection)
	}

	// 从连接池中移除指定的 UDP 连接
	delete(p.m, key)

	// 释放锁
	p.l.Unlock()
}

// 实现 UdpConnectionManagement 结构体的 Get 方法，用于获取指定 UDP 连接的相关信息
func (p *UdpConnectionManagement) Get(key string) (*net.UDPConn, int64) {
	// 获取锁
	p.l.Lock()
	// 如果连接池为空，则创建一个新的连接池
	if p.m == nil {
		p.m = make(map[string]*UdpConnection)
	}

	// 获取指定的 UDP 连接
	u := p.m[key]

	// 释放锁并返回 UDP 连接的相关信息
	p.l.Unlock()
	if u == nil {
		return nil, -1
	}
	return u.Conn, u.Theoni
}

// 实现 UdpConnectionManagement 结构体的 GetObj 方法，用于获取指定 UDP 连接的 UdpConnection 结构体指针
func (p *UdpConnectionManagement) GetObj(key string) *UdpConnection {
	// 获取锁
	p.l.Lock()
	// 如果连接池为空，则创建一个新的连接池
	if p.m == nil {
		p.m = make(map[string]*UdpConnection)
	}

	// 获取指定的 UDP 连接的 UdpConnection 结构体指针
	u := p.m[key]

	// 释放锁并返回 UdpConnection 结构体指针
	p.l.Unlock()
	return u
}

// 定义 UdpConnectionManagement 结构体，用于管理 UDP 连接
type UdpConnectionManagement struct {
	l sync.Mutex                // 互斥锁，用于保护数据访问
	m map[string]*UdpConnection // 用于存储 UDP 连接的 map，key 为 Local + Remote，value 为 udpConnection 结构体指针
}

// 定义 udpConnection 结构体，用于表示 UDP 连接
type UdpConnection struct {
	Theoni        int64        // 用于存储 唯一ID
	Conn          *net.UDPConn // 用于存储服务器端的 UDP 连接
	Send          *NfSend      // 用于存储 UDP 发送选项    			【NF驱动使用】
	Receive       *NfSend      // 用于存储 UDP 接收选项    			【NF驱动使用】
	ClientConn    *net.UDPConn // 用于存储客户端的 UDP 连接        【非驱动使用】
	ClientAddress *net.UDPAddr // 用于存储客户端地址				【非驱动使用】
	ClientFrom    []byte       // 用于存储客户端的来源信息			【非驱动使用】
}

// 实现 udpConnection 结构体的 SendServer 方法，用于向服务器发送数据并返回发送结果
func (p *UdpConnection) SendServer(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	if p == nil {
		return false
	}
	if p.Send != nil {
		r, _ := NFapi_Api_NfUdpPostSend(p.Send.Id, p.Send.RemoteAddress, data, p.Send.options)
		return r == 0
	}
	if p.Conn == nil {
		return false
	}
	_, er := p.Conn.Write(data)
	return er == nil
}

// 实现 udpConnection 结构体的 SendClient 方法，用于向客户端发送数据并返回发送结果
func (p *UdpConnection) SendClient(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	if p == nil {
		return false
	}
	if p.Receive != nil {
		r, _ := NFapi_Api_NfUdpPostSend(p.Receive.Id, p.Receive.RemoteAddress, data, p.Receive.options)
		return r == 0
	}
	if p.ClientAddress != nil && p.ClientConn != nil {
		var bs []byte
		bs = append(bs, p.ClientFrom...)
		bs = append(bs, data...)
		_, er := p.ClientConn.WriteToUDP(data, p.ClientAddress)
		return er == nil
	}
	return false
}

// 创建一个 int 类型到 *bytes.Buffer 映射的 map
var UdpMap = make(map[int]*bytes.Buffer)

// 创建一个互斥锁
var UdpSync sync.Mutex

// 创建一个 int64 类型到 string 映射的 map
var UdpTidMap = make(map[int64]string)

// ID 映射 唯一ID
var UdpIdTid = make(map[uint64]int64)

// 向服务器发送数据，返回是否发送成功
func UdpSendToServer(tid int64, data []byte) bool {
	if len(data) < 1 {
		return false
	}
	// 获取锁
	UdpSync.Lock()
	// 获取指定 tid 对应的 key
	key := UdpTidMap[tid]

	// 如果 key 不为空，则获取对应的 sender 并发送数据，最后释放锁并返回发送结果
	if key != "" {
		o := UdpSenders.GetObj(key)
		if o != nil {
			UdpSync.Unlock()
			return o.SendServer(data)
		}
	}
	// 如果发送失败，则释放锁并返回 false
	UdpSync.Unlock()
	return false
}

// 向客户端发送数据，返回是否发送成功
func UdpSendToClient(tid int64, data []byte) bool {
	if len(data) < 1 {
		return false
	}
	// 获取锁
	UdpSync.Lock()
	// 获取指定 tid 对应的 key
	key := UdpTidMap[tid]

	// 如果 key 不为空，则获取对应的 sender 并发送数据，最后释放锁并返回发送结果
	if key != "" {
		o := UdpSenders.GetObj(key)
		if o != nil {
			UdpSync.Unlock()
			return o.SendClient(data)
		}
	}

	// 如果发送失败，则释放锁并返回 false
	UdpSync.Unlock()
	return false
}

// 删除指定 tid 对应的 key，并从 UdpTidMap 中删除该 tid
func NfDelTid(tid int64) {
	// 获取锁
	UdpSync.Lock()
	// 获取指定 tid 对应的 key
	key := UdpTidMap[tid]
	// 如果 key 不为空，则删除 key 对应的 sender，并从 UdpTidMap 中删除该 tid
	if key != "" {
		o := UdpSenders.GetObj(key)
		if o != nil {
			if o.Send != nil {
				delete(UdpIdTid, o.Send.Id)
			}
			if o.Receive != nil {
				delete(UdpIdTid, o.Receive.Id)
			}
		}
		UdpSenders.Del(key)
		delete(UdpTidMap, tid)
	}
	// 释放锁
	UdpSync.Unlock()
}

// 将指定 tid 和 key 存储到 UdpTidMap 中
func NfAddTid(id uint64, tid int64, key string) {
	// 获取锁
	UdpSync.Lock()
	// 将指定 tid 和 key 存储到 UdpTidMap 中
	UdpTidMap[tid] = key
	if id > 0 {
		UdpIdTid[id] = tid
	}
	// 释放锁
	UdpSync.Unlock()
}

// 将指定 NFid 取唯一ID
func NfIdGetTid(id uint64) int64 {
	// 获取锁
	UdpSync.Lock()
	//获取Tid(唯一ID)
	tid := UdpIdTid[id]
	// 释放锁
	UdpSync.Unlock()
	return tid
}

// 将指定 唯一ID 获取 UDP对象
func NfTidGetObj(tid int64) *UdpConnection {
	// 获取锁
	UdpSync.Lock()
	key := UdpTidMap[tid]
	if key != "" {
		o := UdpSenders.GetObj(key)
		if o != nil {
			UdpSync.Unlock()
			return o
		}
	}
	// 释放锁
	UdpSync.Unlock()
	return nil
}

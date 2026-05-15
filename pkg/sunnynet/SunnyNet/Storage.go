package SunnyNet

import (
	"github.com/qtgolang/SunnyNet/public"
	"sync"
)

var SunnyStorageLock sync.Mutex
var SunnyStorage = make(map[int]*Sunny) //Sunny中间件储存对象

// httpStorage Sunny中间件http回调CALL时储存对象
var httpStorage = make(map[int]*ProxyRequest)

// TcpStorage Sunny中间件tcp回调CALL时储存对象
var TcpStorage = make(map[int]*public.TCP)

// WsStorage Sunny中间件http回调CALL时储存对象
var wsStorage = make(map[int]*public.WebsocketMsg)

// 储存管理 MessageId
// ---------------------------------------------
var messageIdLock sync.Mutex
var messageId = 1000

// NewMessageId 创建新的 messageId
func NewMessageId() int {
	messageIdLock.Lock()
	messageId++
	t := messageId
	if t < 0 || t > 2147483640 {
		t = 1000
		messageId = 1000
	}
	messageIdLock.Unlock()
	return t
}

//---------------------------------------------

// TcpSceneLock 储存管理TCP转发
var TcpSceneLock sync.Mutex

//---------------------------------------------

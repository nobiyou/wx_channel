package websocket

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"wx_channel/internal/utils"

	"github.com/coder/websocket"
	json "github.com/json-iterator/go"
)

// Client 表示一个 WebSocket 客户端连接
type Client struct {
	ID             string // 客户端 ID
	Conn           *websocket.Conn
	RemoteAddr     string // 远程地址
	send           chan []byte
	hub            *Hub
	mu             sync.Mutex
	ctx            context.Context
	cancel         context.CancelFunc
	closed         bool
	lastPing       time.Time
	lastSeen       time.Time
	pagePath       string
	href           string
	apiReady       bool
	methods        map[string]bool
	activeRequests int32 // 活跃请求数（原子操作）
}

// NewClient 创建新的客户端
func NewClient(conn *websocket.Conn, hub *Hub) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		Conn:       conn,
		RemoteAddr: "unknown",
		send:       make(chan []byte, 256),
		hub:        hub,
		ctx:        ctx,
		cancel:     cancel,
		lastPing:   time.Now(),
		lastSeen:   time.Now(),
		methods:    make(map[string]bool),
	}
}

// NewClientWithAddr 创建新的客户端（带远程地址）
func NewClientWithAddr(conn *websocket.Conn, hub *Hub, remoteAddr string) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		Conn:       conn,
		RemoteAddr: remoteAddr,
		send:       make(chan []byte, 256),
		hub:        hub,
		ctx:        ctx,
		cancel:     cancel,
		lastPing:   time.Now(),
		lastSeen:   time.Now(),
		methods:    make(map[string]bool),
	}
}

// ReadPump 从 WebSocket 连接读取消息
func (c *Client) ReadPump() {
	defer func() {
		if r := recover(); r != nil {
			utils.LogError("ReadPump panic 恢复: %v", r)
		}
		c.hub.unregister <- c
		c.Close()
	}()

	// 设置最大消息大小为 1MB (而不是之前的10MB，防止恶意大包内存撑爆)
	c.Conn.SetReadLimit(1 * 1024 * 1024)

	// 启动 ping 循环
	go c.pingLoop()

	// Create a worker pool to limit concurrent API response processing
	const numWorkers = 5
	msgChan := make(chan WSMessage, 50)
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for msg := range msgChan {
				func() {
					defer func() {
						if r := recover(); r != nil {
							utils.LogError("API 响应处理 panic: %v", r)
						}
					}()

					var resp APICallResponse
					if err := json.Unmarshal(msg.Data, &resp); err != nil {
						utils.LogError("API 响应解析失败: %v", err)
						return
					}
					c.hub.handleAPIResponse(resp)
				}()
			}
		}()
	}

	// Ensure workers are cleaned up
	defer func() {
		close(msgChan)
		wg.Wait()
	}()

	for {

		// 使用 context 控制读取超时
		ctx, cancel := context.WithTimeout(c.ctx, 300*time.Second)
		messageType, message, err := c.Conn.Read(ctx)
		cancel()

		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "context deadline exceeded") {
				utils.LogWarn("WebSocket 连接空闲超时，关闭连接: %s", c.RemoteAddr)
			} else if errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "context canceled") {
				utils.LogInfo("WebSocket 上下文已取消: %s", c.RemoteAddr)
			} else {
				// 检查是否是正常关闭
				status := websocket.CloseStatus(err)
				if status == websocket.StatusNormalClosure || status == websocket.StatusGoingAway {
					utils.LogInfo("WebSocket 正常关闭")
				} else {
					utils.LogError("WebSocket 异常关闭: %v (状态码: %d)", err, status)
				}
			}
			break
		}

		// 只处理文本消息
		if messageType != websocket.MessageText {
			continue
		}

		// 解析消息
		var msg WSMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			utils.LogError("消息解析失败: %v", err)
			continue
		}
		c.Touch()

		if msg.Type == WSMessageTypePing {
			c.TouchPing()
			pong, _ := json.Marshal(WSMessage{Type: WSMessageTypePong})
			_ = c.Send(pong)
			continue
		}

		if msg.Type == WSMessageTypeClientState {
			var state ClientStateBody
			if err := json.Unmarshal(msg.Data, &state); err != nil {
				utils.LogWarn("客户端状态解析失败: %v", err)
				continue
			}
			c.UpdateState(state)
			continue
		}

		// 处理 API 响应（使用工作池防止 Goroutine 泛滥）
		if msg.Type == WSMessageTypeAPIResponse {
			select {
			case msgChan <- msg:
				// Successfully pushed to worker pool
			default:
				utils.LogWarn("API Response queue is full, processing synchronously")
				// Fallback to synchronous processing if queue is full
				var resp APICallResponse
				if err := json.Unmarshal(msg.Data, &resp); err != nil {
					utils.LogError("API 响应解析失败: %v", err)
					continue
				}
				c.hub.handleAPIResponse(resp)
			}
		}
	}
}

// WritePump 向 WebSocket 连接写入消息
func (c *Client) WritePump() {
	defer func() {
		c.Close()
	}()

	for {
		select {
		case <-c.ctx.Done():
			return
		case message, ok := <-c.send:
			if !ok {
				return
			}

			ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
			err := c.Conn.Write(ctx, websocket.MessageText, message)
			cancel()

			if err != nil {
				utils.LogError("写入消息失败: %v", err)
				return
			}
		}
	}
}

// Send 发送消息到客户端
func (c *Client) Send(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return errors.New("client is closed")
	}

	select {
	case c.send <- data:
		return nil
	default:
		return errors.New("send buffer is full")
	}
}

// Close 关闭客户端连接
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.closed {
		c.closed = true
		c.cancel()
		c.Conn.Close(websocket.StatusNormalClosure, "")
		close(c.send)
	}
}

func (c *Client) pingLoop() {
	ticker := time.NewTicker(50 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
			err := c.Conn.Ping(ctx)
			cancel()

			if err != nil {
				utils.LogError("Ping 失败: %v", err)
				return
			}
			c.lastPing = time.Now()
		}
	}
}

func (c *Client) Touch() {
	c.mu.Lock()
	c.lastSeen = time.Now()
	c.mu.Unlock()
}

func (c *Client) TouchPing() {
	c.mu.Lock()
	now := time.Now()
	c.lastPing = now
	c.lastSeen = now
	c.mu.Unlock()
}

func (c *Client) UpdateState(state ClientStateBody) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.pagePath = state.PagePath
	c.href = state.Href
	c.apiReady = state.APIReady
	c.lastSeen = time.Now()
	if state.Timestamp > 0 {
		c.lastPing = time.UnixMilli(state.Timestamp)
	}

	c.methods = make(map[string]bool)
	for k, v := range state.Methods {
		c.methods[k] = v
	}

	utils.LogInfo("WebSocket 客户端状态更新: %s | page=%s | apiReady=%t", c.RemoteAddr, c.pagePath, c.apiReady)
}

func (c *Client) SupportsKey(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.apiReady {
		return false
	}

	if len(c.methods) == 0 {
		return false
	}

	switch key {
	case "key:channels:contact_list":
		return c.methods["finderSearch"]
	case "key:channels:feed_list":
		return c.methods["finderUserPage"]
	case "key:channels:feed_profile":
		return c.methods["finderGetCommentDetail"]
	case "key:channels:shared_feed_profile":
		return c.methods["finderGetCommentDetail"]
	case "key:channels:shared_feed_resolve":
		return c.methods["finderGetCommentDetail"]
	case "key:channels:fetch_feed_comment_list":
		return c.methods["finderGetCommentList"]
	default:
		return true
	}
}

func (c *Client) Status() ClientStatus {
	c.mu.Lock()
	defer c.mu.Unlock()

	methods := make(map[string]bool, len(c.methods))
	for k, v := range c.methods {
		methods[k] = v
	}

	lastSeenAt := ""
	if !c.lastSeen.IsZero() {
		lastSeenAt = c.lastSeen.Format(time.RFC3339)
	}
	lastPingAt := ""
	if !c.lastPing.IsZero() {
		lastPingAt = c.lastPing.Format(time.RFC3339)
	}

	return ClientStatus{
		RemoteAddr:      c.RemoteAddr,
		PagePath:        c.pagePath,
		Href:            c.href,
		APIReady:        c.apiReady,
		Methods:         methods,
		ActiveRequests:  int(atomic.LoadInt32(&c.activeRequests)),
		LastSeenAt:      lastSeenAt,
		LastPingAt:      lastPingAt,
		SupportsSearch:  c.apiReady && methods["finderSearch"],
		SupportsFeed:    c.apiReady && methods["finderUserPage"],
		SupportsProfile: c.apiReady && methods["finderGetCommentDetail"],
		SupportsComment: c.apiReady && methods["finderGetCommentList"],
	}
}

// GetActiveRequests 获取活跃请求数
func (c *Client) GetActiveRequests() int {
	return int(atomic.LoadInt32(&c.activeRequests))
}

// IncrementActiveRequests 增加活跃请求数
func (c *Client) IncrementActiveRequests() {
	atomic.AddInt32(&c.activeRequests, 1)
}

// DecrementActiveRequests 减少活跃请求数
func (c *Client) DecrementActiveRequests() {
	for {
		old := atomic.LoadInt32(&c.activeRequests)
		if old <= 0 {
			return
		}
		if atomic.CompareAndSwapInt32(&c.activeRequests, old, old-1) {
			return
		}
	}
}

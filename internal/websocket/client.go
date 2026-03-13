package websocket

import (
	"context"
	"errors"
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
		ctx, cancel := context.WithTimeout(c.ctx, 180*time.Second)
		messageType, message, err := c.Conn.Read(ctx)
		cancel()

		if err != nil {

			// 检查是否是正常关闭
			status := websocket.CloseStatus(err)
			if status == websocket.StatusNormalClosure || status == websocket.StatusGoingAway {
				utils.LogInfo("WebSocket 正常关闭")
			} else {
				utils.LogError("WebSocket 异常关闭: %v (状态码: %d)", err, status)
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

package ws

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sync"
	"time"
	"wx_channel/hub_server/cache"
	"wx_channel/hub_server/database"
	"wx_channel/hub_server/models"
	"wx_channel/hub_server/services"

	"github.com/coder/websocket"
)

type Client struct {
	ID       string
	Hostname string
	Version  string
	IP       string
	LastSeen time.Time
	Conn     *websocket.Conn
	mu       sync.Mutex
	ctx      context.Context
	cancel   context.CancelFunc

	respChannels map[string]chan ResponsePayload
	respMu       sync.RWMutex
	Hub          *Hub

	// 连接统计
	stats ConnectionStats
}

// ConnectionStats 连接统计信息
type ConnectionStats struct {
	ConnectedAt  time.Time
	PingCount    int64
	PongCount    int64
	LastPingTime time.Time
	LastPongTime time.Time
	AvgLatency   time.Duration
	FailureCount int
	MessagesSent int64
	MessagesRecv int64
}

func NewClient(id string, conn *websocket.Conn, hub *Hub, ip string) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		ID:           id,
		IP:           ip,
		LastSeen:     time.Now(),
		Conn:         conn,
		ctx:          ctx,
		cancel:       cancel,
		respChannels: make(map[string]chan ResponsePayload),
		Hub:          hub,
		stats: ConnectionStats{
			ConnectedAt: time.Now(),
		},
	}
}

func (c *Client) ReadPump() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("ReadPump panic 恢复: ClientID=%s, Error=%v", c.ID, r)
		}
		c.Hub.Unregister <- c
		c.Close()
	}()

	// 设置最大消息大小为 10MB
	c.Conn.SetReadLimit(10 * 1024 * 1024)

	// 启动 ping 循环
	go c.pingLoop()

	for {
		// 使用 context 控制读取超时
		ctx, cancel := context.WithTimeout(c.ctx, 90*time.Second)
		messageType, message, err := c.Conn.Read(ctx)
		cancel()

		if err != nil {
			// 检查是否是正常关闭
			status := websocket.CloseStatus(err)
			if status == websocket.StatusNormalClosure || status == websocket.StatusGoingAway {
				log.Printf("WebSocket 正常关闭: ClientID=%s", c.ID)
			} else {
				log.Printf("WebSocket 异常关闭: ClientID=%s, Error=%v, Status=%d", c.ID, err, status)
			}
			break
		}

		// 如果是二进制消息，说明是压缩数据，先解压
		if messageType == websocket.MessageBinary {
			decompressed, err := c.decompressData(message)
			if err != nil {
				log.Printf("解压失败: ClientID=%s, Error=%v", c.ID, err)
				continue
			}
			message = decompressed
		}

		var msg CloudMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("消息解析失败: ClientID=%s, Error=%v", c.ID, err)
			continue
		}

		// 更新统计
		c.mu.Lock()
		c.stats.MessagesRecv++
		c.mu.Unlock()

		// 使用 goroutine 处理消息，添加 panic 恢复
		go func(m CloudMessage) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("消息处理 panic: ClientID=%s, MessageType=%s, Error=%v", c.ID, m.Type, r)
				}
			}()
			c.handleMessage(m)
		}(msg)
	}
}

func (c *Client) handleMessage(msg CloudMessage) {
	now := time.Now()
	c.mu.Lock()
	c.LastSeen = now
	c.mu.Unlock()

	switch msg.Type {
	case MsgTypeHeartbeat:
		var p HeartbeatPayload
		json.Unmarshal(msg.Payload, &p)
		c.mu.Lock()
		c.Hostname = p.Hostname
		c.Version = p.Version
		c.mu.Unlock()

		// 更新数据库
		database.UpsertNode(&models.Node{
			ID:                  c.ID,
			Hostname:            p.Hostname,
			Version:             p.Version,
			IP:                  c.IP,
			Status:              "online",
			LastSeen:            now,
			HardwareFingerprint: p.HardwareFingerprint,
		})

		// 发送心跳响应（Pong）
		c.sendHeartbeatResponse(msg.ID)

	case "metrics":
		var payload struct {
			Metrics string `json:"metrics"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err == nil {
			cache.UpdateClientMetrics(c.ID, payload.Metrics)
		}

	case MsgTypeResponse:
		var resp ResponsePayload
		if err := json.Unmarshal(msg.Payload, &resp); err != nil {
			log.Printf("解析响应失败: ClientID=%s, Error=%v", c.ID, err)
			return
		}

		c.respMu.RLock()
		ch, ok := c.respChannels[resp.RequestID]
		c.respMu.RUnlock()

		if ok {
			// 使用 select 防止阻塞
			timer := time.NewTimer(5 * time.Second)
			select {
			case ch <- resp:
				// 响应已发送
			case <-timer.C:
				log.Printf("响应通道发送超时: ClientID=%s, RequestID=%s", c.ID, resp.RequestID)
			}
			timer.Stop()
		} else {
			log.Printf("未找到响应通道: ClientID=%s, RequestID=%s (可能已超时)", c.ID, resp.RequestID)
		}

	case MsgTypeBind:
		var payload struct {
			Token string `json:"token"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err == nil {
			err := services.ProcessBindRequest(c.ID, payload.Token)

			response := map[string]interface{}{
				"type":    "bind_result",
				"success": err == nil,
			}
			if err != nil {
				response["error"] = err.Error()
			}

			respBytes, _ := json.Marshal(response)
			c.WriteMessage(respBytes)
		}

	case MsgTypeSyncData:
		// 处理客户端推送的同步数据
		var payload SyncDataPayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			log.Printf("解析同步数据失败: ClientID=%s, Error=%v", c.ID, err)
			return
		}

		// 获取同步服务
		syncService := services.GetSyncService()
		if syncService == nil {
			log.Printf("同步服务不可用: ClientID=%s", c.ID)
			return
		}

		// 解析记录
		var records interface{}
		if payload.SyncType == "browse" {
			var browseRecords []services.BrowseRecord
			if err := json.Unmarshal(payload.Records, &browseRecords); err != nil {
				log.Printf("解析浏览记录失败: ClientID=%s, Error=%v", c.ID, err)
				return
			}
			records = browseRecords
		} else if payload.SyncType == "download" {
			var downloadRecords []services.DownloadRecord
			if err := json.Unmarshal(payload.Records, &downloadRecords); err != nil {
				log.Printf("解析下载记录失败: ClientID=%s, Error=%v", c.ID, err)
				return
			}
			records = downloadRecords
		} else {
			log.Printf("未知的同步类型: ClientID=%s, SyncType=%s", c.ID, payload.SyncType)
			return
		}

		// 处理同步数据
		if err := syncService.HandleSyncDataFromClient(c.ID, payload.SyncType, records); err != nil {
			log.Printf("处理同步数据失败: ClientID=%s, Error=%v", c.ID, err)
		} else {
			log.Printf("成功同步 %d 条 %s 记录 (客户端: %s)", 
				payload.Count, payload.SyncType, c.ID)
		}
	}
}

// decompressData 解压数据（限制最大 10MB 防止 gzip 炸弹）
func (c *Client) decompressData(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	const maxDecompressSize = 10 * 1024 * 1024 // 10MB
	return io.ReadAll(io.LimitReader(reader, maxDecompressSize))
}

func (c *Client) WriteMessage(msg []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
	defer cancel()

	err := c.Conn.Write(ctx, websocket.MessageText, msg)
	if err == nil {
		c.stats.MessagesSent++
	}
	return err
}

// sendHeartbeatResponse 发送心跳响应
func (c *Client) sendHeartbeatResponse(requestID string) {
	response := map[string]interface{}{
		"id":        fmt.Sprintf("pong-%s", requestID),
		"type":      "heartbeat_ack",
		"client_id": "hub-server",
		"timestamp": time.Now().Unix(),
	}

	respBytes, err := json.Marshal(response)
	if err != nil {
		log.Printf("序列化心跳响应失败: %v", err)
		return
	}

	if err := c.WriteMessage(respBytes); err != nil {
		log.Printf("发送心跳响应失败: %v", err)
	}
}

// GetStats 获取连接统计信息
func (c *Client) GetStats() ConnectionStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stats
}

// Close 关闭连接
func (c *Client) Close() {
	c.cancel()
	c.Conn.Close(websocket.StatusNormalClosure, "")

	// 记录连接统计
	stats := c.GetStats()
	uptime := time.Since(stats.ConnectedAt)
	log.Printf("连接关闭统计: ClientID=%s, Uptime=%v, Ping=%d, Pong=%d, AvgLatency=%v, Sent=%d, Recv=%d",
		c.ID, uptime, stats.PingCount, stats.PongCount, stats.AvgLatency, stats.MessagesSent, stats.MessagesRecv)
}

// pingLoop 定期发送 ping 保持连接活跃
func (c *Client) pingLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			start := time.Now()

			// 检查连接是否已关闭
			c.mu.Lock()
			if c.ctx.Err() != nil {
				c.mu.Unlock()
				return
			}
			c.mu.Unlock()

			ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
			err := c.Conn.Ping(ctx)
			cancel()

			latency := time.Since(start)

			c.mu.Lock()
			c.stats.PingCount++
			c.stats.LastPingTime = start

			if err != nil {
				c.stats.FailureCount++
				failureCount := c.stats.FailureCount
				c.mu.Unlock()

				log.Printf("Ping 失败: ClientID=%s, Error=%v, 连续失败=%d", c.ID, err, failureCount)

				// Ping 失败，触发连接清理
				log.Printf("Ping 失败，触发连接清理: ClientID=%s", c.ID)
				c.Hub.Unregister <- c
				return
			}

			// Ping 成功
			c.stats.PongCount++
			c.stats.LastPongTime = time.Now()
			c.stats.FailureCount = 0

			// 计算平均延迟（指数移动平均）
			if c.stats.AvgLatency == 0 {
				c.stats.AvgLatency = latency
			} else {
				c.stats.AvgLatency = (c.stats.AvgLatency*9 + latency) / 10
			}
			c.mu.Unlock()

			// 如果延迟过高，记录警告
			if latency > 5*time.Second {
				log.Printf("Ping 延迟过高: ClientID=%s, Latency=%v", c.ID, latency)
			}
		}
	}
}

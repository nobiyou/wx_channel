package ws

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
	"wx_channel/hub_server/database"
	"wx_channel/hub_server/models"

	"github.com/coder/websocket"
)

type Hub struct {
	Clients    map[string]*Client
	Register   chan *Client
	Unregister chan *Client
	mu         sync.RWMutex
}

func NewHub() *Hub {
	return &Hub{
		Clients:    make(map[string]*Client),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
	}
}

func (h *Hub) Run() {
	// 启动僵尸连接清理器
	go h.cleanupStaleConnections()

	for {
		select {
		case client := <-h.Register:
			h.mu.Lock()
			if old, ok := h.Clients[client.ID]; ok {
				old.Close()
			}
			h.Clients[client.ID] = client
			h.mu.Unlock()

			log.Printf("Client connected: %s from %s", client.ID, client.IP)
			// DB: Mark as online
			database.UpsertNodePresence(client.ID, client.IP, time.Now())

		case client := <-h.Unregister:
			h.mu.Lock()
			if _, ok := h.Clients[client.ID]; ok {
				delete(h.Clients, client.ID)
				client.Close()

				log.Printf("Client disconnected: %s", client.ID)
				// DB: Mark as offline
				database.UpdateNodeStatus(client.ID, "offline")
			}
			h.mu.Unlock()
		}
	}
}

// cleanupStaleConnections 清理僵尸连接
func (h *Hub) cleanupStaleConnections() {
	ticker := time.NewTicker(30 * time.Second) // 每 30 秒检查一次
	defer ticker.Stop()

	for range ticker.C {
		h.mu.RLock()
		staleIDs := []string{}
		// 增加超时阈值到 900 秒（15 分钟），以支持长时间的 API 调用
		threshold := time.Now().Add(-900 * time.Second)

		for _, client := range h.Clients {
			client.mu.Lock()
			lastSeen := client.LastSeen
			client.mu.Unlock()

			if lastSeen.Before(threshold) {
				staleIDs = append(staleIDs, client.ID)
			}
		}
		h.mu.RUnlock()

		// 在锁外直接清理，避免向 Unregister channel 发送（防死锁）
		for _, id := range staleIDs {
			h.mu.Lock()
			if client, ok := h.Clients[id]; ok {
				log.Printf("清理僵尸连接: %s (最后心跳: %v, 已超时 %v)",
					client.ID, client.LastSeen, time.Since(client.LastSeen))
				client.Close()
				delete(h.Clients, id)
				database.UpdateNodeStatus(id, "offline")
			}
			h.mu.Unlock()
		}
	}
}

func (h *Hub) RemoveClient(id string) {
	h.mu.Lock()
	if c, ok := h.Clients[id]; ok {
		c.Close()
		delete(h.Clients, id)
	}
	h.mu.Unlock()
}

// GetClient safely retrieves a client by ID
func (h *Hub) GetClient(id string) *Client {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.Clients[id]
}

// GetAllClientsStats 获取所有客户端统计信息
func (h *Hub) GetAllClientsStats() []map[string]interface{} {
	h.mu.RLock()
	defer h.mu.RUnlock()

	stats := make([]map[string]interface{}, 0, len(h.Clients))
	for _, client := range h.Clients {
		clientStats := client.GetStats()
		uptime := time.Since(clientStats.ConnectedAt)

		stats = append(stats, map[string]interface{}{
			"id":             client.ID,
			"hostname":       client.Hostname,
			"version":        client.Version,
			"ip":             client.IP,
			"connected_at":   clientStats.ConnectedAt,
			"uptime":         uptime.Round(time.Second).String(),
			"ping_count":     clientStats.PingCount,
			"pong_count":     clientStats.PongCount,
			"avg_latency":    clientStats.AvgLatency.Round(time.Millisecond).String(),
			"last_ping_time": clientStats.LastPingTime,
			"failure_count":  clientStats.FailureCount,
			"messages_sent":  clientStats.MessagesSent,
			"messages_recv":  clientStats.MessagesRecv,
		})
	}

	return stats
}

func (h *Hub) Call(userID uint, clientID string, action string, data interface{}, timeout time.Duration) (ResponsePayload, error) {
	h.mu.RLock()
	c, ok := h.Clients[clientID]
	h.mu.RUnlock()

	if !ok {
		return ResponsePayload{}, fmt.Errorf("client offline")
	}

	reqID := fmt.Sprintf("hub-%d", time.Now().UnixNano())
	payloadData, _ := json.Marshal(data)
	cmd := CommandPayload{Action: action, Data: payloadData}
	cmdData, _ := json.Marshal(cmd)

	// DB: Create Task
	task := &models.Task{
		Type:    action,
		NodeID:  clientID,
		UserID:  userID,
		Payload: string(payloadData),
		Status:  "pending",
	}
	database.CreateTask(task)

	msg := CloudMessage{
		ID:        reqID,
		Type:      MsgTypeCommand,
		ClientID:  "hub-server",
		Payload:   cmdData,
		Timestamp: time.Now().Unix(),
	}

	// 创建响应通道（增加缓冲区大小）
	respChan := make(chan ResponsePayload, 2)
	c.respMu.Lock()
	c.respChannels[reqID] = respChan
	c.respMu.Unlock()

	// 确保清理资源
	defer func() {
		c.respMu.Lock()
		delete(c.respChannels, reqID)
		c.respMu.Unlock()
		close(respChan) // 关闭通道防止泄漏
	}()

	msgData, _ := json.Marshal(msg)

	// 记录请求开始时间
	startTime := time.Now()
	log.Printf("发送远程调用: ID=%s, Action=%s, ClientID=%s, Timeout=%v", reqID, action, clientID, timeout)

	if err := c.WriteMessage(msgData); err != nil {
		log.Printf("发送消息失败: ID=%s, Error=%v", reqID, err)
		database.UpdateTaskResult(task.ID, "failed", "", err.Error())
		return ResponsePayload{}, fmt.Errorf("发送消息失败: %w", err)
	}

	// 创建超时 timer（可被 Stop 释放，避免 time.After 泄漏）
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case resp, ok := <-respChan:
		duration := time.Since(startTime)
		if !ok {
			log.Printf("响应通道已关闭: ID=%s, Duration=%v", reqID, duration)
			database.UpdateTaskResult(task.ID, "failed", "", "响应通道已关闭")
			return ResponsePayload{}, fmt.Errorf("响应通道已关闭")
		}

		resBytes, _ := json.Marshal(resp.Data)
		status := "success"
		if !resp.Success {
			status = "failed"
			log.Printf("远程调用失败: ID=%s, Duration=%v, Error=%s", reqID, duration, resp.Error)
		} else {
			log.Printf("远程调用成功: ID=%s, Duration=%v, DataSize=%d", reqID, duration, len(resBytes))
		}
		database.UpdateTaskResult(task.ID, status, string(resBytes), resp.Error)
		return resp, nil

	case <-timer.C:
		log.Printf("远程调用超时: ID=%s, Timeout=%v", reqID, timeout)
		database.UpdateTaskResult(task.ID, "timeout", "", "request timeout")
		return ResponsePayload{}, fmt.Errorf("请求超时")
	}
}

func (h *Hub) ServeWs(w http.ResponseWriter, r *http.Request) {
	if !isHubOriginAllowed(r.Header.Get("Origin")) {
		http.Error(w, "forbidden origin", http.StatusForbidden)
		return
	}

	if expected := strings.TrimSpace(os.Getenv("HUB_WS_TOKEN")); expected != "" {
		if extractHubToken(r) != expected {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	clientID := r.Header.Get("X-Client-ID")
	if clientID == "" {
		clientID = r.URL.Query().Get("client_id")
	}
	if clientID == "" {
		http.Error(w, "X-Client-ID required", 400)
		return
	}

	// 使用 nhooyr.io/websocket 升级连接
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionContextTakeover, // 启用压缩
	})
	if err != nil {
		log.Printf("Upgrade error: %v", err)
		return
	}

	// 获取客户端 IP 地址
	clientIP := r.Header.Get("X-Real-IP")
	if clientIP == "" {
		clientIP = r.Header.Get("X-Forwarded-For")
	}
	if clientIP == "" {
		clientIP = r.RemoteAddr
	}

	client := NewClient(clientID, conn, h, clientIP)
	h.Register <- client

	// Start reading (blocking until disconnect)
	go client.ReadPump()
}

func isHubOriginAllowed(origin string) bool {
	allowedRaw := strings.TrimSpace(os.Getenv("HUB_ALLOWED_ORIGINS"))
	if allowedRaw == "" {
		// 默认只允许本地浏览器来源；无 Origin（非浏览器客户端）仍允许。
		if origin == "" {
			return true
		}
		u, err := url.Parse(origin)
		if err != nil {
			return false
		}
		host := strings.ToLower(u.Hostname())
		return host == "localhost" || host == "127.0.0.1" || host == "::1" || strings.HasSuffix(host, ".localhost")
	}
	if origin == "" {
		return false
	}

	parts := strings.Split(allowedRaw, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "*" || p == origin {
			return true
		}
	}
	return false
}

func extractHubToken(r *http.Request) string {
	token := strings.TrimSpace(r.Header.Get("X-Local-Auth"))
	if token != "" {
		return token
	}

	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[len("Bearer "):])
	}

	return strings.TrimSpace(r.URL.Query().Get("token"))
}

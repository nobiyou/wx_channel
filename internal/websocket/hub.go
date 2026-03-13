package websocket

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"wx_channel/internal/utils"

	json "github.com/json-iterator/go"
)

// Hub 管理所有 WebSocket 客户端连接
type Hub struct {
	clients    map[*Client]bool
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
	lastClient *Client // 最后注册的客户端

	// API 调用管理
	requests   map[string]chan APICallResponse
	requestsMu sync.RWMutex
	reqSeq     uint64

	// 负载均衡选择器
	selector ClientSelector
}

// NewHub 创建新的 Hub
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		requests:   make(map[string]chan APICallResponse),
		selector:   NewLeastConnectionSelector(), // 默认使用最少连接选择器
	}
}

// Run 启动 Hub
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.lastClient = client // 记录最后注册的客户端
			h.mu.Unlock()
			utils.LogInfo("WebSocket 客户端已连接: %s", client.RemoteAddr)

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				addr := client.RemoteAddr
				delete(h.clients, client)
				client.Close()
				// 如果注销的是最后一个客户端，清除引用
				if h.lastClient == client {
					h.lastClient = nil
					// 尝试找到另一个活跃的客户端
					for c := range h.clients {
						h.lastClient = c
						break
					}
				}
				utils.LogInfo("WebSocket 客户端已断开: %s", addr)
			}
			h.mu.Unlock()
		}
	}
}

// RegisterClient 注册新客户端
func (h *Hub) RegisterClient(client *Client) {
	h.register <- client
}

// GetClient 获取一个可用的客户端（使用负载均衡选择器）
func (h *Hub) GetClient() (*Client, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// 使用负载均衡选择器选择客户端
	if h.selector != nil {
		return h.selector.Select(h.clients)
	}

	// 如果没有选择器，使用默认逻辑（向后兼容）
	// 优先使用最后注册的客户端
	if h.lastClient != nil {
		if _, ok := h.clients[h.lastClient]; ok {
			return h.lastClient, nil
		}
	}

	// 如果最后注册的客户端不可用，使用任意一个
	for client := range h.clients {
		return client, nil
	}

	return nil, errors.New("no available client")
}

// SetSelector 设置负载均衡选择器
func (h *Hub) SetSelector(selector ClientSelector) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.selector = selector
}

// ClientCount 返回当前连接的客户端数量
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// CallAPI 调用前端 API
func (h *Hub) CallAPI(key string, body interface{}, timeout time.Duration) (json.RawMessage, error) {
	client, err := h.GetClient()
	if err != nil {
		return nil, err
	}

	// 增加活跃请求计数
	client.IncrementActiveRequests()
	defer client.DecrementActiveRequests()

	// 生成请求 ID
	id := atomic.AddUint64(&h.reqSeq, 1)
	reqID := fmt.Sprintf("%d", id)

	// 创建响应通道（增加缓冲区大小以防止阻塞）
	respChan := make(chan APICallResponse, 2)
	h.requestsMu.Lock()
	h.requests[reqID] = respChan
	h.requestsMu.Unlock()

	// 确保清理响应通道
	defer func() {
		h.requestsMu.Lock()
		delete(h.requests, reqID)
		h.requestsMu.Unlock()
		close(respChan) // 关闭通道防止泄漏
	}()

	// 构建请求消息
	req := APICallRequest{
		ID:   reqID,
		Key:  key,
		Body: body,
	}

	reqData, err := json.Marshal(req)
	if err != nil {
		utils.LogError("序列化 API 请求失败: %v", err)
		return nil, fmt.Errorf("marshal request failed: %w", err)
	}

	msg := WSMessage{
		Type: WSMessageTypeAPICall,
		Data: reqData,
	}

	msgData, err := json.Marshal(msg)
	if err != nil {
		utils.LogError("序列化 WebSocket 消息失败: %v", err)
		return nil, fmt.Errorf("marshal message failed: %w", err)
	}

	// 记录请求开始时间
	startTime := time.Now()
	utils.LogInfo("发送 API 请求: ID=%s, Key=%s, Timeout=%v", reqID, key, timeout)

	// 发送请求
	if err := client.Send(msgData); err != nil {
		utils.LogError("发送 API 请求失败: ID=%s, Error=%v", reqID, err)
		return nil, fmt.Errorf("send request failed: %w", err)
	}

	// 等待响应
	select {
	case resp, ok := <-respChan:
		if !ok {
			utils.LogError("响应通道已关闭: ID=%s", reqID)
			return nil, errors.New("response channel closed")
		}

		duration := time.Since(startTime)
		if resp.ErrCode != 0 {
			utils.LogError("API 调用失败: ID=%s, Duration=%v, ErrCode=%d, ErrMsg=%s",
				reqID, duration, resp.ErrCode, resp.ErrMsg)
			return nil, fmt.Errorf("API error (code=%d): %s", resp.ErrCode, resp.ErrMsg)
		}

		utils.LogInfo("API 调用成功: ID=%s, Duration=%v, DataSize=%d",
			reqID, duration, len(resp.Data))
		return resp.Data, nil

	case <-time.After(timeout):
		utils.LogError("API 调用超时: ID=%s, Timeout=%v", reqID, timeout)
		return nil, fmt.Errorf("request timeout after %v", timeout)
	}
}

// handleAPIResponse 处理 API 响应
func (h *Hub) handleAPIResponse(resp APICallResponse) {
	h.requestsMu.RLock()
	respChan, ok := h.requests[resp.ID]
	h.requestsMu.RUnlock()

	if ok {
		// 使用 select 防止阻塞
		select {
		case respChan <- resp:
			// 响应已发送
		case <-time.After(5 * time.Second):
			utils.LogError("响应通道发送超时: ID=%s (可能接收方已超时)", resp.ID)
		}
	} else {
		utils.LogWarn("未找到响应通道: ID=%s (可能已超时或已清理)", resp.ID)
	}
}

// BroadcastCommand 向所有客户端广播指令
func (h *Hub) BroadcastCommand(action string, payload interface{}) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if len(h.clients) == 0 {
		return errors.New("no connected clients")
	}

	cmdData := map[string]interface{}{
		"action":  action,
		"payload": payload,
	}

	data, err := json.Marshal(cmdData)
	if err != nil {
		return err
	}

	msg := WSMessage{
		Type: WSMessageTypeCommand,
		Data: data,
	}

	msgData, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	for client := range h.clients {
		// 忽略发送错误，尽可能发送给所有客户端
		client.Send(msgData)
	}

	return nil
}

// Broadcast 广播任意消息到所有客户端
func (h *Hub) Broadcast(message interface{}) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if len(h.clients) == 0 {
		return nil
	}

	data, err := json.Marshal(message)
	if err != nil {
		return err
	}

	for client := range h.clients {
		client.Send(data)
	}

	return nil
}

package websocket

import (
	"context"
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

var errClientDisconnected = errors.New("websocket client disconnected")

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

// GetClientForKey 获取支持指定 API 的客户端
func (h *Hub) GetClientForKey(key string) (*Client, error) {
	return h.getClientForKey(key, nil)
}

func (h *Hub) getClientForKey(key string, excluded map[*Client]struct{}) (*Client, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	filtered := make(map[*Client]bool)
	for client := range h.clients {
		if _, skip := excluded[client]; skip || client.isClosed() || client.ctx.Err() != nil {
			continue
		}
		if client.SupportsKey(key) {
			filtered[client] = true
		}
	}

	if len(filtered) == 0 {
		return nil, fmt.Errorf("no ready client for key: %s", key)
	}

	if h.selector != nil {
		return h.selector.Select(filtered)
	}

	for client := range filtered {
		return client, nil
	}

	return nil, fmt.Errorf("no ready client for key: %s", key)
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

func (h *Hub) ClientStatuses() []ClientStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()

	statuses := make([]ClientStatus, 0, len(h.clients))
	for client := range h.clients {
		statuses = append(statuses, client.Status())
	}
	return statuses
}

func isRetryableAPICallKey(key string) bool {
	switch key {
	case "key:channels:contact_list",
		"key:channels:feed_list",
		"key:channels:feed_profile",
		"key:channels:shared_feed_profile",
		"key:channels:shared_feed_resolve",
		"key:channels:fetch_feed_comment_list":
		return true
	default:
		return false
	}
}

// CallAPI 调用前端 API
func (h *Hub) CallAPI(key string, body interface{}, timeout time.Duration) (json.RawMessage, error) {
	return h.CallAPIContext(context.Background(), key, body, timeout)
}

// CallAPIContext 调用前端 API，并允许调用方取消等待中的请求。
func (h *Hub) CallAPIContext(ctx context.Context, key string, body interface{}, timeout time.Duration) (json.RawMessage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	deadline := time.Now().Add(timeout)
	excluded := make(map[*Client]struct{})
	var lastDisconnect error

	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			if lastDisconnect != nil {
				return nil, lastDisconnect
			}
			return nil, fmt.Errorf("request timeout after %v", timeout)
		}

		client, err := h.getClientForKey(key, excluded)
		if err != nil {
			if lastDisconnect != nil {
				return nil, fmt.Errorf("no ready client after websocket disconnect: %w", err)
			}
			return nil, err
		}

		data, err := h.callAPIOnClient(ctx, client, key, body, remaining)
		if !errors.Is(err, errClientDisconnected) {
			return data, err
		}
		if !isRetryableAPICallKey(key) {
			return nil, err
		}

		lastDisconnect = err
		excluded[client] = struct{}{}
		utils.LogWarn("API 请求客户端已断开，切换下一个就绪客户端: Key=%s", key)
	}
}

func (h *Hub) callAPIOnClient(ctx context.Context, client *Client, key string, body interface{}, timeout time.Duration) (json.RawMessage, error) {

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
		// Do not close respChan here. A late WebSocket response may already have
		// obtained the channel from requests before this cleanup runs.
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
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := client.Send(msgData); err != nil {
		utils.LogError("发送 API 请求失败: ID=%s, Error=%v", reqID, err)
		if client.isClosed() || client.ctx.Err() != nil {
			return nil, fmt.Errorf("%w: %v", errClientDisconnected, err)
		}
		return nil, fmt.Errorf("send request failed: %w", err)
	}

	// 等待响应
	timer := time.NewTimer(timeout)
	defer timer.Stop()
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

	case <-client.ctx.Done():
		utils.LogWarn("API 调用客户端断开: ID=%s", reqID)
		return nil, errClientDisconnected

	case <-ctx.Done():
		utils.LogWarn("API 调用已取消: ID=%s", reqID)
		return nil, ctx.Err()

	case <-timer.C:
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
	msgData, err := marshalCommand(action, payload)
	if err != nil {
		return err
	}

	h.mu.RLock()
	clients := make([]*Client, 0, len(h.clients))
	for client := range h.clients {
		clients = append(clients, client)
	}
	h.mu.RUnlock()

	if len(clients) == 0 {
		return errors.New("no connected clients")
	}

	for _, client := range clients {
		// 忽略发送错误，尽可能发送给所有客户端
		client.Send(msgData)
	}

	return nil
}

// SendCommandToMatchingClient sends one command to the first open client that
// satisfies predicate. It is intentionally generic so lifecycle policy stays
// outside the WebSocket transport owner.
func (h *Hub) SendCommandToMatchingClient(predicate func(ClientStatus) bool, action string, payload interface{}) error {
	if predicate == nil {
		return errors.New("client predicate is nil")
	}

	msgData, err := marshalCommand(action, payload)
	if err != nil {
		return err
	}

	h.mu.RLock()
	var target *Client
	for client := range h.clients {
		if client.isClosed() {
			continue
		}
		if predicate(client.Status()) {
			target = client
			break
		}
	}
	h.mu.RUnlock()

	if target == nil {
		return errors.New("no matching websocket client")
	}
	if err := target.Send(msgData); err != nil {
		return fmt.Errorf("send command to matching client: %w", err)
	}
	return nil
}

func marshalCommand(action string, payload interface{}) ([]byte, error) {
	cmdData := map[string]interface{}{
		"action":  action,
		"payload": payload,
	}
	data, err := json.Marshal(cmdData)
	if err != nil {
		return nil, err
	}
	return json.Marshal(WSMessage{
		Type: WSMessageTypeCommand,
		Data: data,
	})
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

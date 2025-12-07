package handlers

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"wx_channel/internal/database"
	"wx_channel/internal/services"
	"wx_channel/internal/utils"
)

// WebSocket message types
const (
	MessageTypeDownloadProgress = "download_progress"
	MessageTypeQueueChange      = "queue_change"
	MessageTypeStatsUpdate      = "stats_update"
	MessageTypePing             = "ping"
	MessageTypePong             = "pong"
)

// Queue change action types
const (
	QueueActionAdd     = "add"
	QueueActionRemove  = "remove"
	QueueActionUpdate  = "update"
	QueueActionReorder = "reorder"
)

// DownloadProgressMessage represents a download progress update
// Requirements: 14.5, 10.6 - real-time download progress updates
type DownloadProgressMessage struct {
	Type       string `json:"type"`
	QueueID    string `json:"queueId"`
	Downloaded int64  `json:"downloaded"`
	Total      int64  `json:"total"`
	Speed      int64  `json:"speed"`
	Status     string `json:"status"`
	Chunks     int    `json:"chunks,omitempty"`
	ChunksDone int    `json:"chunksDone,omitempty"`
}

// QueueChangeMessage represents a queue change notification
// Requirements: 14.5 - broadcast queue changes
type QueueChangeMessage struct {
	Type   string               `json:"type"`
	Action string               `json:"action"`
	Item   *database.QueueItem  `json:"item,omitempty"`
	Queue  []database.QueueItem `json:"queue,omitempty"`
}

// StatsUpdateMessage represents a statistics update
// Requirements: 7.5 - update dashboard within 5 seconds
type StatsUpdateMessage struct {
	Type  string              `json:"type"`
	Stats *services.Statistics `json:"stats"`
}


// WebSocketClient represents a connected WebSocket client
type WebSocketClient struct {
	hub      *WebSocketHub
	conn     *websocket.Conn
	send     chan []byte
	id       string
	closedMu sync.Mutex
	closed   bool
}

// WebSocketHub manages all WebSocket connections
type WebSocketHub struct {
	// Registered clients
	clients map[*WebSocketClient]bool

	// Inbound messages from clients
	broadcast chan []byte

	// Register requests from clients
	register chan *WebSocketClient

	// Unregister requests from clients
	unregister chan *WebSocketClient

	// Mutex for thread-safe operations
	mu sync.RWMutex

	// Statistics service for stats updates
	statsService *services.StatisticsService

	// Queue service for queue updates
	queueService *services.QueueService
}

// Global WebSocket hub instance
var wsHub *WebSocketHub
var wsHubOnce sync.Once

// GetWebSocketHub returns the singleton WebSocket hub instance
func GetWebSocketHub() *WebSocketHub {
	wsHubOnce.Do(func() {
		wsHub = NewWebSocketHub()
		go wsHub.Run()
		// Start periodic stats update broadcaster
		// Requirements: 7.5 - update dashboard within 5 seconds
		go wsHub.startStatsUpdateBroadcaster()
	})
	return wsHub
}

// NewWebSocketHub creates a new WebSocket hub
func NewWebSocketHub() *WebSocketHub {
	return &WebSocketHub{
		clients:      make(map[*WebSocketClient]bool),
		broadcast:    make(chan []byte, 256),
		register:     make(chan *WebSocketClient),
		unregister:   make(chan *WebSocketClient),
		statsService: services.NewStatisticsService(),
		queueService: services.NewQueueService(),
	}
}

// Run starts the hub's main loop
func (h *WebSocketHub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			utils.Info("[WebSocket] Client connected: %s (total: %d)", client.id, len(h.clients))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			utils.Info("[WebSocket] Client disconnected: %s (total: %d)", client.id, len(h.clients))

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					// Client's send buffer is full, close connection
					h.mu.RUnlock()
					h.mu.Lock()
					close(client.send)
					delete(h.clients, client)
					h.mu.Unlock()
					h.mu.RLock()
				}
			}
			h.mu.RUnlock()
		}
	}
}


// ClientCount returns the number of connected clients
func (h *WebSocketHub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// startStatsUpdateBroadcaster starts a goroutine that periodically broadcasts stats updates
// Requirements: 7.5 - update dashboard within 5 seconds when statistics change
func (h *WebSocketHub) startStatsUpdateBroadcaster() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// Only broadcast if there are connected clients
		h.mu.RLock()
		clientCount := len(h.clients)
		h.mu.RUnlock()

		if clientCount > 0 {
			h.BroadcastStatsUpdate()
		}
	}
}

// StartProgressForwarder starts a goroutine that forwards download progress updates to WebSocket clients
// This connects the ChunkedDownloader's progress channel to the WebSocket hub
// Requirements: 14.5, 10.6 - real-time download progress updates via WebSocket
func (h *WebSocketHub) StartProgressForwarder(progressChan <-chan services.ProgressUpdate) {
	go func() {
		for update := range progressChan {
			h.BroadcastDownloadProgress(
				update.QueueID,
				update.DownloadedSize,
				update.TotalSize,
				update.Speed,
				update.Status,
				update.ChunksTotal,
				update.ChunksCompleted,
			)
		}
	}()
}

// BroadcastMessage sends a message to all connected clients
func (h *WebSocketHub) BroadcastMessage(message interface{}) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	h.broadcast <- data
	return nil
}

// BroadcastDownloadProgress broadcasts download progress to all clients
// Requirements: 14.5, 10.6 - real-time download progress updates via WebSocket
func (h *WebSocketHub) BroadcastDownloadProgress(queueID string, downloaded, total, speed int64, status string, chunks, chunksDone int) {
	msg := DownloadProgressMessage{
		Type:       MessageTypeDownloadProgress,
		QueueID:    queueID,
		Downloaded: downloaded,
		Total:      total,
		Speed:      speed,
		Status:     status,
		Chunks:     chunks,
		ChunksDone: chunksDone,
	}
	if err := h.BroadcastMessage(msg); err != nil {
		utils.Warn("[WebSocket] Failed to broadcast download progress: %v", err)
	}
}

// BroadcastQueueAdd broadcasts a queue item addition
func (h *WebSocketHub) BroadcastQueueAdd(item *database.QueueItem) {
	msg := QueueChangeMessage{
		Type:   MessageTypeQueueChange,
		Action: QueueActionAdd,
		Item:   item,
	}
	if err := h.BroadcastMessage(msg); err != nil {
		utils.Warn("[WebSocket] Failed to broadcast queue add: %v", err)
	}
}

// BroadcastQueueRemove broadcasts a queue item removal
func (h *WebSocketHub) BroadcastQueueRemove(itemID string) {
	msg := QueueChangeMessage{
		Type:   MessageTypeQueueChange,
		Action: QueueActionRemove,
		Item:   &database.QueueItem{ID: itemID},
	}
	if err := h.BroadcastMessage(msg); err != nil {
		utils.Warn("[WebSocket] Failed to broadcast queue remove: %v", err)
	}
}

// BroadcastQueueUpdate broadcasts a queue item update
func (h *WebSocketHub) BroadcastQueueUpdate(item *database.QueueItem) {
	msg := QueueChangeMessage{
		Type:   MessageTypeQueueChange,
		Action: QueueActionUpdate,
		Item:   item,
	}
	if err := h.BroadcastMessage(msg); err != nil {
		utils.Warn("[WebSocket] Failed to broadcast queue update: %v", err)
	}
}

// BroadcastQueueReorder broadcasts a queue reorder
func (h *WebSocketHub) BroadcastQueueReorder(queue []database.QueueItem) {
	msg := QueueChangeMessage{
		Type:   MessageTypeQueueChange,
		Action: QueueActionReorder,
		Queue:  queue,
	}
	if err := h.BroadcastMessage(msg); err != nil {
		utils.Warn("[WebSocket] Failed to broadcast queue reorder: %v", err)
	}
}

// BroadcastStatsUpdate broadcasts statistics update to all clients
// Requirements: 7.5 - update dashboard within 5 seconds
func (h *WebSocketHub) BroadcastStatsUpdate() {
	stats, err := h.statsService.GetStatistics()
	if err != nil {
		utils.Warn("[WebSocket] Failed to get statistics for broadcast: %v", err)
		return
	}

	msg := StatsUpdateMessage{
		Type:  MessageTypeStatsUpdate,
		Stats: stats,
	}
	if err := h.BroadcastMessage(msg); err != nil {
		utils.Warn("[WebSocket] Failed to broadcast stats update: %v", err)
	}
}


// WebSocket configuration
const (
	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second

	// Send pings to peer with this period (must be less than pongWait)
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer
	maxMessageSize = 512
)

// WebSocket upgrader with CORS support
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// Allow all origins for local service
	// Requirements: 14.6 - CORS support for remote console
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// readPump pumps messages from the WebSocket connection to the hub
func (c *WebSocketClient) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				utils.Warn("[WebSocket] Read error: %v", err)
			}
			break
		}

		// Handle incoming messages (e.g., ping/pong)
		var msg map[string]interface{}
		if err := json.Unmarshal(message, &msg); err == nil {
			if msgType, ok := msg["type"].(string); ok && msgType == MessageTypePing {
				// Respond with pong
				pong := map[string]string{"type": MessageTypePong}
				if data, err := json.Marshal(pong); err == nil {
					c.send <- data
				}
			}
		}
	}
}

// writePump pumps messages from the hub to the WebSocket connection
func (c *WebSocketClient) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// Send each message as a separate WebSocket frame
			// This ensures each JSON message can be parsed independently
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

			// Send any queued messages as separate frames
			n := len(c.send)
			for i := 0; i < n; i++ {
				if err := c.conn.WriteMessage(websocket.TextMessage, <-c.send); err != nil {
					return
				}
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}


// WebSocketHandler handles WebSocket upgrade requests
// Requirements: 14.5 - WebSocket endpoint for real-time download progress updates
type WebSocketHandler struct {
	hub *WebSocketHub
}

// NewWebSocketHandler creates a new WebSocket handler
func NewWebSocketHandler() *WebSocketHandler {
	return &WebSocketHandler{
		hub: GetWebSocketHub(),
	}
}

// HandleWebSocket handles WebSocket connection upgrade
// Endpoint: /ws
// Requirements: 14.5 - WebSocket endpoint for real-time updates
func (h *WebSocketHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		utils.Warn("[WebSocket] Upgrade failed: %v", err)
		return
	}

	// Generate client ID
	clientID := generateClientID()

	client := &WebSocketClient{
		hub:  h.hub,
		conn: conn,
		send: make(chan []byte, 256),
		id:   clientID,
	}

	// Register client
	h.hub.register <- client

	// Start read and write pumps in separate goroutines
	go client.writePump()
	go client.readPump()

	// Send initial stats update to the new client
	go func() {
		time.Sleep(100 * time.Millisecond) // Small delay to ensure client is ready
		stats, err := h.hub.statsService.GetStatistics()
		if err == nil {
			msg := StatsUpdateMessage{
				Type:  MessageTypeStatsUpdate,
				Stats: stats,
			}
			if data, err := json.Marshal(msg); err == nil {
				select {
				case client.send <- data:
				default:
				}
			}
		}

		// Also send current queue state
		queue, err := h.hub.queueService.GetQueue()
		if err == nil {
			msg := QueueChangeMessage{
				Type:   MessageTypeQueueChange,
				Action: QueueActionReorder,
				Queue:  queue,
			}
			if data, err := json.Marshal(msg); err == nil {
				select {
				case client.send <- data:
				default:
				}
			}
		}
	}()
}

// generateClientID generates a unique client ID
func generateClientID() string {
	return time.Now().Format("20060102150405.000000")
}

// ServeWs is a convenience function for handling WebSocket requests
// Can be used directly as an http.HandlerFunc
func ServeWs(w http.ResponseWriter, r *http.Request) {
	handler := NewWebSocketHandler()
	handler.HandleWebSocket(w, r)
}

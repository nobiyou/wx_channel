package poc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const (
	bridgeOrigin       = "https://channels.weixin.qq.com"
	bridgeProtocol     = "wx-poc-v1"
	bridgeMaxMessage   = 4 << 20
	bridgeMaxPagePath  = 2048
	bridgeWriteTimeout = 5 * time.Second
)

var allowedMethods = map[string]struct{}{
	"finderSearch":           {},
	"finderUserPage":         {},
	"finderGetCommentDetail": {},
	"finderGetCommentList":   {},
}

type BridgeState struct {
	PagePath string          `json:"page_path"`
	Visible  bool            `json:"visible"`
	Methods  map[string]bool `json:"methods"`
	LastSeen time.Time       `json:"last_seen"`
}

type Bridge interface {
	WaitReady(context.Context, []string) error
	Call(context.Context, string, any) ([]byte, error)
	State() BridgeState
	Close() error
}

type bridgeCallResult struct {
	data []byte
	err  error
}

type BridgeServer struct {
	tokenHash  [sha256.Size]byte
	tokenValid bool
	logger     SafeLogger

	mu           sync.Mutex
	conn         *websocket.Conn
	state        BridgeState
	stateChanged chan struct{}
	pending      map[string]chan bridgeCallResult
	nextID       uint64
	callActive   bool
	closed       bool
	writeMu      sync.Mutex
}

func NewBridgeServer(token string, logger SafeLogger) *BridgeServer {
	if logger == nil {
		logger = DiscardLogger{}
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	valid := err == nil && len(decoded) >= 32 && base64.RawURLEncoding.EncodeToString(decoded) == token
	return &BridgeServer{
		tokenHash:    sha256.Sum256([]byte(token)),
		tokenValid:   valid,
		logger:       logger,
		stateChanged: make(chan struct{}),
		pending:      make(map[string]chan bridgeCallResult),
	}
}

func (s *BridgeServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws/api", s.handleWebSocket)
	return mux
}

func (s *BridgeServer) handleWebSocket(writer http.ResponseWriter, request *http.Request) {
	if !s.authorizeRequest(request) {
		http.Error(writer, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return
	}
	conn, err := websocket.Accept(writer, request, &websocket.AcceptOptions{
		Subprotocols:   []string{bridgeProtocol},
		OriginPatterns: []string{bridgeOrigin},
	})
	if err != nil {
		return
	}
	conn.SetReadLimit(bridgeMaxMessage)

	s.mu.Lock()
	if s.closed || s.conn != nil {
		s.mu.Unlock()
		_ = conn.Close(websocket.StatusPolicyViolation, "bridge unavailable")
		return
	}
	s.conn = conn
	s.state = BridgeState{Methods: map[string]bool{}}
	s.signalStateChangedLocked()
	s.mu.Unlock()
	_ = s.logger.Event("bridge_connected", nil)

	defer func() {
		s.detach(conn)
		_ = conn.CloseNow()
		_ = s.logger.Event("bridge_disconnected", nil)
	}()
	s.readLoop(conn)
}

func (s *BridgeServer) authorizeRequest(request *http.Request) bool {
	if !s.tokenValid || request.Method != http.MethodGet || request.URL.Path != "/ws/api" || request.URL.RawQuery != "" || request.URL.ForceQuery {
		return false
	}
	if request.Header.Get("Origin") != bridgeOrigin {
		return false
	}
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return false
	}
	protocols := requestedSubprotocols(request.Header.Values("Sec-WebSocket-Protocol"))
	if len(protocols) != 2 || protocols[0] != bridgeProtocol || !strings.HasPrefix(protocols[1], "auth.") {
		return false
	}
	candidate := strings.TrimPrefix(protocols[1], "auth.")
	decoded, err := base64.RawURLEncoding.DecodeString(candidate)
	if err != nil || len(decoded) < 32 || base64.RawURLEncoding.EncodeToString(decoded) != candidate {
		return false
	}
	candidateHash := sha256.Sum256([]byte(candidate))
	return subtle.ConstantTimeCompare(candidateHash[:], s.tokenHash[:]) == 1
}

func requestedSubprotocols(values []string) []string {
	var protocols []string
	for _, value := range values {
		for _, protocol := range strings.Split(value, ",") {
			protocol = strings.TrimSpace(protocol)
			if protocol != "" {
				protocols = append(protocols, protocol)
			}
		}
	}
	return protocols
}

func (s *BridgeServer) readLoop(conn *websocket.Conn) {
	for {
		messageType, raw, err := conn.Read(context.Background())
		if err != nil {
			return
		}
		if messageType != websocket.MessageText {
			_ = conn.Close(websocket.StatusUnsupportedData, "text messages required")
			return
		}
		if err := s.handleClientMessage(conn, raw); err != nil {
			_ = conn.Close(websocket.StatusPolicyViolation, "invalid bridge message")
			return
		}
	}
}

type bridgeEnvelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func (s *BridgeServer) handleClientMessage(conn *websocket.Conn, raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var envelope bridgeEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("bridge message contains trailing JSON")
	}
	switch envelope.Type {
	case "client_state":
		return s.updateClientState(envelope.Data)
	case "api_response":
		return s.deliverAPIResponse(envelope.Data)
	case "ping":
		return s.writeMessage(context.Background(), conn, bridgeEnvelope{Type: "pong", Data: json.RawMessage(`{}`)})
	default:
		return errors.New("bridge message type is not allowed")
	}
}

func (s *BridgeServer) updateClientState(raw json.RawMessage) error {
	var incoming struct {
		PagePath string          `json:"pagePath"`
		Visible  bool            `json:"visible"`
		Methods  map[string]bool `json:"methods"`
	}
	if err := json.Unmarshal(raw, &incoming); err != nil {
		return err
	}
	if incoming.PagePath == "" || len(incoming.PagePath) > bridgeMaxPagePath || !strings.HasPrefix(incoming.PagePath, "/") || strings.ContainsAny(incoming.PagePath, "?#") {
		return errors.New("invalid page path")
	}
	methods := make(map[string]bool, len(allowedMethods))
	for method := range allowedMethods {
		methods[method] = incoming.Methods[method]
	}
	s.mu.Lock()
	s.state = BridgeState{
		PagePath: incoming.PagePath,
		Visible:  incoming.Visible,
		Methods:  methods,
		LastSeen: time.Now().UTC(),
	}
	s.signalStateChangedLocked()
	s.mu.Unlock()
	return nil
}

func (s *BridgeServer) deliverAPIResponse(raw json.RawMessage) error {
	var response struct {
		ID      string          `json:"id"`
		Data    json.RawMessage `json:"data"`
		ErrCode int             `json:"errCode"`
	}
	if err := json.Unmarshal(raw, &response); err != nil || response.ID == "" {
		return errors.New("invalid API response")
	}
	s.mu.Lock()
	pending, ok := s.pending[response.ID]
	s.mu.Unlock()
	if !ok {
		return errors.New("unknown API response ID")
	}
	result := bridgeCallResult{}
	if response.ErrCode != 0 {
		if response.ErrCode == -70003 {
			result.err = NewCategorizedError(ErrorTargetContext)
		} else {
			result.err = errors.New("page API failed")
		}
	} else {
		if len(response.Data) == 0 {
			response.Data = json.RawMessage(`null`)
		}
		result.data = append([]byte(nil), response.Data...)
	}
	select {
	case pending <- result:
		return nil
	default:
		return errors.New("duplicate API response")
	}
}

func (s *BridgeServer) WaitReady(ctx context.Context, methods []string) error {
	for _, method := range methods {
		if _, ok := allowedMethods[method]; !ok {
			return errors.New("required bridge method is not allowed")
		}
	}
	for {
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return errors.New("bridge is closed")
		}
		ready := s.conn != nil && s.state.Visible
		for _, method := range methods {
			ready = ready && s.state.Methods[method]
		}
		changed := s.stateChanged
		s.mu.Unlock()
		if ready {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}
}

func (s *BridgeServer) Call(ctx context.Context, method string, body any) ([]byte, error) {
	if _, ok := allowedMethods[method]; !ok {
		return nil, errors.New("bridge method is not allowed")
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, errors.New("bridge is closed")
	}
	if s.callActive {
		s.mu.Unlock()
		return nil, errors.New("concurrent bridge calls are not allowed")
	}
	if s.conn == nil || !s.state.Visible || !s.state.Methods[method] {
		s.mu.Unlock()
		return nil, errors.New("bridge method is not ready")
	}
	s.callActive = true
	s.nextID++
	id := fmt.Sprintf("call-%d", s.nextID)
	result := make(chan bridgeCallResult, 1)
	s.pending[id] = result
	conn := s.conn
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.pending, id)
		s.callActive = false
		s.mu.Unlock()
	}()

	data, err := json.Marshal(struct {
		ID     string `json:"id"`
		Method string `json:"method"`
		Body   any    `json:"body"`
	}{ID: id, Method: method, Body: body})
	if err != nil {
		return nil, errors.New("encode bridge call")
	}
	if len(data) > bridgeMaxMessage {
		return nil, errors.New("bridge call exceeds message limit")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	writeCtx, cancelWrite := context.WithTimeout(context.Background(), bridgeWriteTimeout)
	defer cancelWrite()
	if err := s.writeMessage(writeCtx, conn, bridgeEnvelope{Type: "api_call", Data: data}); err != nil {
		return nil, errors.New("send bridge call")
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case response := <-result:
		return response.data, response.err
	}
}

func (s *BridgeServer) writeMessage(ctx context.Context, conn *websocket.Conn, envelope bridgeEnvelope) error {
	raw, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	if len(raw) > bridgeMaxMessage {
		return errors.New("bridge message exceeds limit")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return conn.Write(ctx, websocket.MessageText, raw)
}

func (s *BridgeServer) State() BridgeState {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.state
	state.Methods = make(map[string]bool, len(s.state.Methods))
	for method, available := range s.state.Methods {
		state.Methods[method] = available
	}
	return state
}

func (s *BridgeServer) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	conn := s.conn
	s.failPendingLocked(errors.New("bridge is closed"))
	s.signalStateChangedLocked()
	s.mu.Unlock()
	if conn != nil {
		return conn.CloseNow()
	}
	return nil
}

func (s *BridgeServer) detach(conn *websocket.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn != conn {
		return
	}
	s.conn = nil
	s.state = BridgeState{Methods: map[string]bool{}}
	s.failPendingLocked(errors.New("bridge disconnected"))
	s.signalStateChangedLocked()
}

func (s *BridgeServer) failPendingLocked(err error) {
	for id, pending := range s.pending {
		select {
		case pending <- bridgeCallResult{err: err}:
		default:
		}
		delete(s.pending, id)
	}
}

func (s *BridgeServer) signalStateChangedLocked() {
	close(s.stateChanged)
	s.stateChanged = make(chan struct{})
}

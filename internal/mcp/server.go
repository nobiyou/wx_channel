// Package mcpserver exposes wx_channel's local data plane and explicitly
// approved local actions to MCP clients. It deliberately sits beside the
// existing HTTP API instead of reimplementing the WeChat page protocol.
package mcpserver

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	json "github.com/json-iterator/go"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"wx_channel/internal/api"
	"wx_channel/internal/config"
	"wx_channel/internal/websocket"
)

const (
	defaultMCPPortOffset = 2
	mcpTokenFileName     = ".wx_channel_mcp_token"
	mcpEndpointPath      = "/mcp"
	maxLiveResponseBytes = 4 << 20
	maxToolResponseBytes = 256 << 10
	maxRequestBytes      = 64 << 10
	maxMarkerBytes       = 8 << 10
	maxFieldRunes        = 4096
)

// Dependencies are the application-owned services used by the MCP adapter.
// The adapter never opens a second database or WebSocket connection.
type Dependencies struct {
	FFmpegPath         string // Optional trusted deployment path; never a tool argument.
	Config             *config.Config
	DownloadsDir       string
	VideoDownload      VideoDownloadFunc
	Hub                Hub
	RuntimeDiagnostics *api.RuntimeDiagnostics
	Version            string
}

// Hub is the minimum live-data surface needed by MCP. Keeping this contract
// small makes the page bridge replaceable in tests without creating another
// protocol implementation.
type Hub interface {
	ClientStatuses() []websocket.ClientStatus
	GetClientForKey(key string) (*websocket.Client, error)
	CallAPIContext(ctx context.Context, key string, body interface{}, timeout time.Duration) (json.RawMessage, error)
}

// Server owns the local MCP listener and its token-protected handler.
type Server struct {
	deps            Dependencies
	port            int
	endpoint        string
	token           string
	tokenPath       string
	tokenSource     string
	httpServer      *http.Server
	handler         http.Handler
	mcpServer       *mcp.Server
	querySlots      chan struct{}
	downloadSlots   chan struct{}
	downloadCtx     context.Context
	cancelDownloads context.CancelFunc
	downloadMu      sync.Mutex
	activeDownloads map[string]struct{}
	analysisCtx     context.Context
	cancelAnalysis  context.CancelFunc
	analysisMu      sync.Mutex
	analysisJobs    map[string]*videoAnalysisJob
	analysisSlots   chan struct{}
}

// NewServer builds an MCP server. It does not start a listener.
func NewServer(deps Dependencies) (*Server, error) {
	if deps.FFmpegPath == "" {
		deps.FFmpegPath = strings.TrimSpace(os.Getenv("WX_CHANNEL_FFMPEG_PATH"))
	}
	if deps.Config == nil {
		return nil, errors.New("mcp config is nil")
	}
	if strings.TrimSpace(deps.DownloadsDir) == "" {
		return nil, errors.New("mcp downloads directory is empty")
	}

	downloadsDir, err := filepath.Abs(deps.DownloadsDir)
	if err != nil {
		return nil, fmt.Errorf("resolve MCP downloads directory: %w", err)
	}
	if err := os.MkdirAll(downloadsDir, 0755); err != nil {
		return nil, fmt.Errorf("create MCP downloads directory: %w", err)
	}

	port := deps.Config.MCPPort
	if port <= 0 {
		port = deps.Config.Port + defaultMCPPortOffset
	}
	if port <= 0 || port > 65535 {
		return nil, fmt.Errorf("invalid MCP port: %d", port)
	}

	token, tokenPath, tokenSource, err := loadMCPToken(deps.Config.MCPToken, downloadsDir)
	if err != nil {
		return nil, err
	}

	version := strings.TrimSpace(deps.Version)
	if version == "" {
		version = "dev"
	}
	downloadCtx, cancelDownloads := context.WithCancel(context.Background())
	analysisCtx, cancelAnalysis := context.WithCancel(context.Background())
	s := &Server{
		deps:            deps,
		port:            port,
		endpoint:        fmt.Sprintf("http://127.0.0.1:%d%s", port, mcpEndpointPath),
		token:           token,
		tokenPath:       tokenPath,
		tokenSource:     tokenSource,
		querySlots:      make(chan struct{}, 2),
		downloadSlots:   make(chan struct{}, 1),
		downloadCtx:     downloadCtx,
		cancelDownloads: cancelDownloads,
		activeDownloads: make(map[string]struct{}),
		analysisCtx:     analysisCtx,
		cancelAnalysis:  cancelAnalysis,
		analysisJobs:    make(map[string]*videoAnalysisJob),
		analysisSlots:   make(chan struct{}, 1),
	}

	s.mcpServer = mcp.NewServer(
		&mcp.Implementation{Name: "wx-channel", Version: version},
		&mcp.ServerOptions{
			Instructions: "wx_channel video data access. Use get_runtime_status first; distinguish stored local records from live WeChat page data. Missing metrics must remain unavailable, never be changed to zero. Downloads and video analysis are independent actions: complete a local download before calling start_video_analysis; analysis tools never download implicitly. Local video analysis automatically attempts FFmpeg audio extraction, faster-whisper transcription, and PaddleOCR keyframe OCR when the local backends are installed; missing backends are reported in local_enrichment and do not silently become empty data. Default hybrid video analysis uses the current Codex session model without an additional API key. After preprocessing completes, read every shot using get_video_analysis_shots with page_size=1 and include_images=true, review the images and local evidence, and persist each shot with save_video_analysis_review. Visual analysis is complete only when review_status=completed. Third-party model APIs, including Gemini, require explicit use_external_model=true.",
			KeepAlive:    5 * time.Minute,
		},
	)
	registerTools(s)

	streamHandler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return s.mcpServer },
		&mcp.StreamableHTTPOptions{JSONResponse: true},
	)
	s.handler = s.authHandler(streamHandler)
	s.httpServer = &http.Server{
		Addr:              fmt.Sprintf("127.0.0.1:%d", port),
		Handler:           s.handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       2 * time.Minute,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       5 * time.Minute,
	}
	return s, nil
}

// Start listens only on the loopback interface and blocks until shutdown.
func (s *Server) Start(ctx context.Context) error {
	if s == nil || s.httpServer == nil {
		return errors.New("mcp server is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	listener, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.httpServer.Addr, err)
	}
	go func() {
		<-ctx.Done()
		s.cancelDownloads()
		s.cancelAnalysis()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.httpServer.Shutdown(shutdownCtx)
	}()

	err = s.httpServer.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Shutdown stops the MCP listener. It is safe to call after Start returned.
func (s *Server) Shutdown(ctx context.Context) error {
	if s == nil || s.httpServer == nil {
		return nil
	}
	if s.cancelDownloads != nil {
		s.cancelDownloads()
	}
	if s.cancelAnalysis != nil {
		s.cancelAnalysis()
	}
	return s.httpServer.Shutdown(ctx)
}

// Endpoint returns the local MCP endpoint without exposing the bearer token.
func (s *Server) Endpoint() string {
	if s == nil {
		return ""
	}
	return s.endpoint
}

// Port returns the loopback MCP port.
func (s *Server) Port() int {
	if s == nil {
		return 0
	}
	return s.port
}

// TokenInfo describes how a local MCP client can obtain authentication without
// returning the secret itself.
func (s *Server) TokenInfo() map[string]any {
	if s == nil {
		return map[string]any{"required": true}
	}
	info := map[string]any{
		"required": true,
		"source":   s.tokenSource,
	}
	if s.tokenPath != "" {
		info["file"] = mcpTokenFileName
	}
	return info
}

func (s *Server) authHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized","message":"MCP bearer token required"}`))
			return
		}
		if r.ContentLength > maxRequestBytes {
			http.Error(w, "MCP request exceeds size limit", http.StatusRequestEntityTooLarge)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
		next.ServeHTTP(w, r)
	})
}

func (s *Server) authorized(r *http.Request) bool {
	if s == nil || r == nil {
		return false
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(auth) < len("Bearer ") || !strings.EqualFold(auth[:len("Bearer ")], "Bearer ") {
		return false
	}
	provided := strings.TrimSpace(auth[len("Bearer "):])
	if provided == "" || s.token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(s.token)) == 1
}

func loadMCPToken(configToken, downloadsDir string) (token, tokenPath, source string, err error) {
	if value := strings.TrimSpace(os.Getenv("WX_CHANNEL_MCP_TOKEN")); value != "" {
		return value, "", "environment", nil
	}
	if value := strings.TrimSpace(configToken); value != "" {
		return value, "", "config", nil
	}

	tokenPath = filepath.Join(downloadsDir, mcpTokenFileName)
	if data, readErr := os.ReadFile(tokenPath); readErr == nil {
		if value := strings.TrimSpace(string(data)); value != "" {
			return value, tokenPath, "file", nil
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return "", "", "", fmt.Errorf("read MCP token file: %w", readErr)
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", "", fmt.Errorf("generate MCP token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	if err := os.WriteFile(tokenPath, []byte(token+"\n"), 0600); err != nil {
		if data, readErr := os.ReadFile(tokenPath); readErr == nil && strings.TrimSpace(string(data)) != "" {
			return strings.TrimSpace(string(data)), tokenPath, "file", nil
		}
		return "", "", "", fmt.Errorf("write MCP token file: %w", err)
	}
	return token, tokenPath, "file", nil
}

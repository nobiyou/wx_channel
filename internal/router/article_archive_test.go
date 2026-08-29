package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"wx_channel/internal/config"
	"wx_channel/internal/handlers"
	"wx_channel/internal/officialaccount"
	"wx_channel/internal/services"
	"wx_channel/internal/websocket"

	"github.com/qtgolang/SunnyNet/SunnyNet"
)

type routerArchiveDownloader struct{}

func (routerArchiveDownloader) DownloadSync(_ context.Context, sourceURL, target string, _ int, _ map[string]string, _ func(float64, int64, int64)) (string, error) {
	if err := os.WriteFile(target, []byte(sourceURL), 0644); err != nil {
		return "", err
	}
	return target, nil
}

func TestArticleArchiveDownloadRouteMountsOnAPIRouter(t *testing.T) {
	router := NewAPIRouter(&config.Config{Port: 2025}, websocket.NewHub(), SunnyNet.NewSunny())
	accountService := officialaccount.NewMemoryService()
	if err := accountService.Upsert(officialaccount.Account{Biz: "biz-router", Key: "key-router"}); err != nil {
		t.Fatalf("upsert account: %v", err)
	}
	router.SetOfficialAccountService(accountService)
	root := t.TempDir()
	handler := handlers.NewArticleArchiveHandler(accountService, routerArchiveDownloader{}, func() (string, error) {
		return root, nil
	})
	router.SetArticleArchiveHandler(handler)

	payload := `{"biz":"biz-router","article":{"title":"路由文章","content_url":"https://mp.weixin.qq.com/s/router-1"},"html":"<div id=\"js_content\"><p>正文</p></div>"}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/mp/archive/download", strings.NewReader(payload))
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected mounted archive route 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "路由文章") {
		t.Fatalf("mounted archive route response did not contain article result: %s", recorder.Body.String())
	}
}

var _ services.ArchiveFileDownloader = routerArchiveDownloader{}

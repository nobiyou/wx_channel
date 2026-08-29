package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"wx_channel/internal/officialaccount"
)

type handlerArchiveDownloader struct {
	lastHeaders map[string]string
}

func (d *handlerArchiveDownloader) DownloadSync(_ context.Context, sourceURL, target string, _ int, headers map[string]string, _ func(float64, int64, int64)) (string, error) {
	d.lastHeaders = headers
	if err := os.WriteFile(target, []byte("downloaded:"+sourceURL), 0644); err != nil {
		return "", err
	}
	return target, nil
}

func TestArticleArchiveHandlerDownloadsUsingCapturedAccountHeaders(t *testing.T) {
	accountService := officialaccount.NewMemoryService()
	if err := accountService.Upsert(officialaccount.Account{
		Biz:    "biz-handler",
		Key:    "key-handler",
		Cookie: "session=handler",
	}); err != nil {
		t.Fatalf("upsert account: %v", err)
	}

	downloader := &handlerArchiveDownloader{}
	root := t.TempDir()
	handler := NewArticleArchiveHandler(accountService, downloader, func() (string, error) {
		return root, nil
	})

	payload := `{"biz":"biz-handler","article":{"title":"处理器文章","content_url":"https://mp.weixin.qq.com/s/handler-1"},"html":"<div id=\"js_content\"><p>正文</p><img src=\"https://mmbiz.qpic.cn/image/handler.png\"></div>"}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/mp/archive/download", strings.NewReader(payload))
	handler.HandleDownload(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected archive download 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Code int `json:"code"`
		Data struct {
			HTMLPath   string `json:"html_path"`
			Downloaded int    `json:"downloaded"`
			Failed     int    `json:"failed"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode archive response: %v", err)
	}
	if response.Code != 0 || response.Data.Downloaded != 1 || response.Data.Failed != 0 {
		t.Fatalf("unexpected archive response: %+v", response)
	}
	if downloader.lastHeaders["Cookie"] != "session=handler" || downloader.lastHeaders["Referer"] != "https://mp.weixin.qq.com/s/handler-1" {
		t.Fatalf("captured account headers were not forwarded: %+v", downloader.lastHeaders)
	}
	htmlData, err := os.ReadFile(response.Data.HTMLPath)
	if err != nil {
		t.Fatalf("read downloaded article: %v", err)
	}
	if strings.Contains(string(htmlData), "mmbiz.qpic.cn") || !strings.Contains(string(htmlData), "assets/") {
		t.Fatalf("downloaded article did not reference local image: %s", htmlData)
	}
}

func TestArticleArchiveHandlerRequiresCapturedAccount(t *testing.T) {
	accountService := officialaccount.NewMemoryService()
	handler := NewArticleArchiveHandler(accountService, &handlerArchiveDownloader{}, func() (string, error) {
		return t.TempDir(), nil
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/mp/archive/download", strings.NewReader(`{"biz":"missing","article":{"content_url":"https://mp.weixin.qq.com/s/missing"},"html":"<div id=\"js_content\"><p>x</p></div>"}`))
	handler.HandleDownload(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected missing account 404, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

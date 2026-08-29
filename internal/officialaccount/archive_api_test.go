package officialaccount

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleArchivePlanReturnsSeparatedContentResourcesAndRelations(t *testing.T) {
	service := NewMemoryService()
	mux := http.NewServeMux()
	service.RegisterRoutes(mux)

	payload := `{"biz":"biz-api","article":{"title":"接口文章","content_url":"https://mp.weixin.qq.com/s/api-1","author":"作者"},"html":"<div id=\"js_content\"><p>正文</p><img src=\"https://mmbiz.qpic.cn/image/1.png\"></div>"}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/mp/archive/plan", strings.NewReader(payload))
	mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected plan 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Code int         `json:"code"`
		Data ArchivePlan `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode plan response: %v", err)
	}
	if response.Code != 0 || response.Data.Content.Biz != "biz-api" {
		t.Fatalf("unexpected plan response: %+v", response)
	}
	if len(response.Data.Resources) != 2 || len(response.Data.Relations) != 2 {
		t.Fatalf("expected body and image resources with relations: %+v", response.Data)
	}
}

func TestHandleArchivePlanRejectsMissingContentNode(t *testing.T) {
	service := NewMemoryService()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/mp/archive/plan", strings.NewReader(`{"biz":"biz-api","article":{"content_url":"https://mp.weixin.qq.com/s/api-2"},"html":"<p>正文</p>"}`))
	service.HandleArchivePlan(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected missing content node 400, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestHandleArchivePlanDoesNotExposeShortLivedCredentials(t *testing.T) {
	service := NewMemoryService()
	payload := `{"biz":"biz-api-secret","article":{"title":"脱敏文章","content_url":"https://mp.weixin.qq.com/s/api-secret?key=article-key"},"html":"<div id=\"js_content\"><p>正文</p><img data-src=\"https://mmbiz.qpic.cn/image/1.jpg?wx_fmt=jpeg&amp;appmsg_token=image-token\"><a href=\"https://mp.weixin.qq.com/s/other?token=link-token\">链接</a></div>"}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/mp/archive/plan", strings.NewReader(payload))
	service.HandleArchivePlan(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected plan 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	responseBody := recorder.Body.String()
	for _, secret := range []string{"article-key", "image-token", "link-token"} {
		if strings.Contains(responseBody, secret) {
			t.Fatalf("archive plan response leaked %q: %s", secret, responseBody)
		}
	}
	if !strings.Contains(responseBody, "wx_fmt=jpeg") {
		t.Fatalf("archive plan response lost non-sensitive image query: %s", responseBody)
	}
	if !strings.Contains(responseBody, "https://mmbiz.qpic.cn/image/1.jpg?wx_fmt=jpeg") {
		t.Fatalf("archive plan response did not retain sanitized image URL: %s", responseBody)
	}
}

func TestArchiveRequestHeadersUseCapturedCredentialsInternally(t *testing.T) {
	service := NewMemoryService()
	if err := service.Upsert(Account{Biz: "biz-header", Key: "key-secret", Cookie: "session=secret"}); err != nil {
		t.Fatalf("upsert account: %v", err)
	}
	headers, err := service.ArchiveRequestHeaders("biz-header", "https://mp.weixin.qq.com/s/article")
	if err != nil {
		t.Fatalf("archive request headers: %v", err)
	}
	if headers["Cookie"] != "session=secret" || headers["Referer"] == "" || headers["User-Agent"] == "" {
		t.Fatalf("unexpected archive headers: %+v", headers)
	}
}

func TestBuildArticleArchivePlanStripsCredentialsFromPersistedURLs(t *testing.T) {
	plan, err := BuildArticleArchivePlan("biz-private", ArticleItem{
		ContentURL: "https://mp.weixin.qq.com/s/private?__biz=biz-private&mid=1&key=secret-key&pass_ticket=secret-ticket&appmsg_token=secret-token",
		Cover:      "https://mmbiz.qpic.cn/cover/1?token=secret-cover-token",
	}, `<div id="js_content"><p>正文</p></div>`)
	if err != nil {
		t.Fatalf("build archive plan: %v", err)
	}
	for _, value := range []string{plan.Content.URL, plan.Content.SourceURL, plan.Content.CoverURL} {
		for _, secret := range []string{"secret-key", "secret-ticket", "secret-token", "secret-cover-token"} {
			if strings.Contains(value, secret) {
				t.Fatalf("archive metadata URL leaked %q: %s", secret, value)
			}
		}
	}
	if !strings.Contains(plan.Content.URL, "__biz=biz-private") || !strings.Contains(plan.Content.URL, "mid=1") {
		t.Fatalf("archive metadata URL lost stable query fields: %s", plan.Content.URL)
	}
}

func TestSanitizeArchivePlanForResponseRedactsMediaSignature(t *testing.T) {
	plan, err := BuildArticleArchivePlan("biz-media", ArticleItem{
		Title:      "视频文章",
		ContentURL: "https://mp.weixin.qq.com/s/media?__biz=biz-media&mid=7&idx=1",
		PlayURL:    "https://vd.example.test/video.mp4?token=short-lived&expires=123&format=mp4",
	}, `<div id="js_content"><p>视频正文</p></div>`)
	if err != nil {
		t.Fatalf("build archive plan: %v", err)
	}
	if !strings.Contains(plan.Content.PlayURL, "short-lived") {
		t.Fatalf("expected in-memory plan to retain the media URL for the current request: %s", plan.Content.PlayURL)
	}

	responsePlan := SanitizeArchivePlanForResponse(plan)
	if responsePlan.Content.PlayURL != "https://vd.example.test/video.mp4" {
		t.Fatalf("media signature was not removed from response plan: %s", responsePlan.Content.PlayURL)
	}
}

func TestSanitizeArchiveMetadataURLRemovesShortLivedCredentials(t *testing.T) {
	raw := "https://mmbiz.qpic.cn/image.png?wx_fmt=png&key=secret-key&appmsg_token=secret-token&uin=secret-uin&pass_ticket=secret-ticket"
	sanitized := SanitizeArchiveMetadataURL(raw)
	for _, secret := range []string{"secret-key", "secret-token", "secret-uin", "secret-ticket"} {
		if strings.Contains(sanitized, secret) {
			t.Fatalf("sanitized URL leaked %q: %s", secret, sanitized)
		}
	}
	if !strings.Contains(sanitized, "wx_fmt=png") {
		t.Fatalf("sanitized URL lost non-sensitive query fields: %s", sanitized)
	}
}

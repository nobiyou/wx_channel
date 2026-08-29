package officialaccount

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCaptureRequestStoresCredentialBearingArticleRequest(t *testing.T) {
	service := NewMemoryService()
	request := httptest.NewRequest(http.MethodGet, "https://mp.weixin.qq.com/s/article-id?__biz=biz-1&uin=uin-1&key=key-1&pass_ticket=ticket-1&appmsg_token=token-1", nil)
	request.Header.Set("Cookie", "slave_user=cookie-value")

	if !service.CaptureRequest(request) {
		t.Fatal("expected credential-bearing article request to be captured")
	}

	account, ok := service.accountSnapshot("biz-1")
	if !ok {
		t.Fatal("captured account was not stored")
	}
	if account.Uin != "uin-1" || account.Key != "key-1" || account.PassTicket != "ticket-1" || account.AppmsgToken != "token-1" {
		t.Fatalf("unexpected captured credential metadata: %+v", account)
	}
	if account.Cookie != "slave_user=cookie-value" || !account.IsEffective {
		t.Fatalf("request cookie or effective state was not captured: %+v", account)
	}
	if account.CookieExpiration <= time.Now().Unix() {
		t.Fatalf("captured cookie was not marked usable: %+v", account)
	}
}

func TestCaptureRequestAcceptsUpstreamAccountAliases(t *testing.T) {
	service := NewMemoryService()
	request := httptest.NewRequest(http.MethodGet, "https://mp.weixin.qq.com/mp/author?bizuin=biz-alias&user_name=gh-alias&user_uin=uin-alias&key=key-alias", nil)
	request.Header.Set("Cookie", "slave_user=alias-cookie")

	if !service.CaptureRequest(request) {
		t.Fatal("expected upstream account aliases to be captured")
	}
	account, ok := service.accountSnapshot("biz-alias")
	if !ok {
		t.Fatal("captured alias account was not stored")
	}
	if account.AuthorID != "gh-alias" || account.Uin != "uin-alias" || !strings.Contains(account.Cookie, "alias-cookie") {
		t.Fatalf("unexpected alias account: %+v", account)
	}
}

func TestCaptureRequestUsesOfficialAccountRefererAndRejectsUnsafeRequests(t *testing.T) {
	service := NewMemoryService()
	request := httptest.NewRequest(http.MethodGet, "https://mp.weixin.qq.com/mp/profile_ext?action=getmsg&key=key-2", nil)
	request.Header.Set("Referer", "https://mp.weixin.qq.com/s/article-id?__biz=biz-2&uin=uin-2&pass_ticket=ticket-2")

	if !service.CaptureRequest(request) {
		t.Fatal("expected official-account referer to complete request credentials")
	}
	if _, ok := service.accountSnapshot("biz-2"); !ok {
		t.Fatal("referer account was not stored")
	}

	cases := []struct {
		name string
		url  string
	}{
		{name: "untrusted host", url: "https://example.com/s/article-id?__biz=biz-3&key=key-3"},
		{name: "unrelated path", url: "https://mp.weixin.qq.com/other?__biz=biz-4&key=key-4"},
		{name: "missing key", url: "https://mp.weixin.qq.com/s/article-id?__biz=biz-5"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if service.CaptureRequest(httptest.NewRequest(http.MethodGet, tc.url, nil)) {
				t.Fatal("unsafe or incomplete request was captured")
			}
		})
	}
}

func TestCaptureRequestAcceptsArticleMetricEndpoint(t *testing.T) {
	service := NewMemoryService()
	request := httptest.NewRequest(http.MethodGet, "https://mp.weixin.qq.com/mp/getappmsgext?__biz=biz-metric&mid=1&idx=1&uin=uin-metric&key=key-metric", nil)
	request.Header.Set("Cookie", "slave_user=metric-cookie")
	if !service.CaptureRequest(request) {
		t.Fatal("expected getappmsgext request to be captured")
	}
	account, ok := service.accountSnapshot("biz-metric")
	if !ok || account.Key != "key-metric" || account.Uin != "uin-metric" {
		t.Fatalf("unexpected metric endpoint account: %+v", account)
	}
}

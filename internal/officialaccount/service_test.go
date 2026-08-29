package officialaccount

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestFetchMsgListFlattensNestedArticles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mp/profile_ext" || r.URL.Query().Get("action") != "getmsg" {
			t.Fatalf("unexpected upstream request: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(sampleMessageListResponse())
	}))
	defer server.Close()

	service := NewMemoryService()
	service.SetUpstreamBaseURL(server.URL)
	service.SetHTTPClient(server.Client())
	if err := service.Upsert(Account{Biz: "biz-1", Key: "key-1", Uin: "uin-1", PassTicket: "ticket-1"}); err != nil {
		t.Fatalf("upsert account: %v", err)
	}

	data, err := service.FetchMsgList("biz-1", 10)
	if err != nil {
		t.Fatalf("fetch message list: %v", err)
	}
	if len(data.List) != 1 {
		t.Fatalf("expected one message, got %d", len(data.List))
	}
	if len(data.Articles) != 2 {
		t.Fatalf("expected parent and child articles, got %d", len(data.Articles))
	}
	if data.Articles[1].Title != "child" || data.Articles[1].PublishTime != 1700000000 {
		t.Fatalf("unexpected child article: %+v", data.Articles[1])
	}
	if data.Articles[0].Subtype != 7 || data.Articles[0].CopyrightStat != 1 || data.Articles[0].VideoID != "video-parent" ||
		data.Articles[0].Duration != 42 || data.Articles[0].AudioFileID != 23 ||
		data.Articles[0].PlayURL != "https://vd.example.test/video.mp4?token=short-lived" {
		t.Fatalf("parent media metadata was truncated: %+v", data.Articles[0])
	}
	if data.Articles[1].Duration != 84 || data.Articles[1].AudioFileID != 24 || data.Articles[1].VideoID != "video-child" ||
		data.Articles[1].PlayURL != "https://vd.example.test/child.mp4?token=short-lived" {
		t.Fatalf("child media metadata was truncated: %+v", data.Articles[1])
	}
}

func TestFetchMsgListRejectsMalformedNestedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ret":0,"general_msg_list":"{malformed"}`))
	}))
	defer server.Close()

	service := NewMemoryService()
	service.SetUpstreamBaseURL(server.URL)
	service.SetHTTPClient(server.Client())
	if err := service.Upsert(Account{Biz: "biz-1", Key: "key-1"}); err != nil {
		t.Fatalf("upsert account: %v", err)
	}
	if _, err := service.FetchMsgList("biz-1", 0); err == nil || !strings.Contains(err.Error(), "general_msg_list") {
		t.Fatalf("expected malformed nested JSON error, got %v", err)
	}
}

func TestHandleRSSProducesAtomWithoutCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(sampleMessageListResponse())
	}))
	defer server.Close()

	service := NewMemoryService()
	service.SetUpstreamBaseURL(server.URL)
	service.SetAPIOrigin("http://127.0.0.1:2026")
	service.SetHTTPClient(server.Client())
	if err := service.Upsert(Account{
		Biz:         "biz-1",
		Nickname:    "测试公众号",
		AvatarURL:   "https://mmbiz.qpic.cn/avatar/1",
		Key:         "secret-key",
		Uin:         "uin-1",
		PassTicket:  "secret-ticket",
		AppmsgToken: "secret-appmsg-token",
	}); err != nil {
		t.Fatalf("upsert account: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/rss/mp?biz=biz-1&proxy=1&proxy_cover=1", nil)
	recorder := httptest.NewRecorder()
	service.HandleRSS(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Header().Get("Content-Type"), "application/atom+xml") {
		t.Fatalf("unexpected content type: %s", recorder.Header().Get("Content-Type"))
	}
	var feed AtomFeed
	if err := xml.Unmarshal(recorder.Body.Bytes(), &feed); err != nil {
		t.Fatalf("decode atom feed: %v\n%s", err, recorder.Body.String())
	}
	if feed.Title != "测试公众号" || len(feed.Entry) != 2 {
		t.Fatalf("unexpected feed: %+v", feed)
	}
	body := recorder.Body.String()
	for _, secret := range []string{"secret-key", "secret-ticket", "secret-appmsg-token"} {
		if strings.Contains(body, secret) {
			t.Fatalf("RSS leaked credential %q: %s", secret, body)
		}
	}
	if !strings.Contains(body, "/mp/proxy?url=") {
		t.Fatalf("expected proxy URL in RSS: %s", body)
	}
	thumbnailURLs := rssThumbnailURLs(t, recorder.Body.Bytes())
	if len(thumbnailURLs) == 0 {
		t.Fatalf("expected at least one media thumbnail")
	}
	for index, thumbnailURL := range thumbnailURLs {
		assertProxyTarget(t, thumbnailURL, "mmbiz.qpic.cn")
		if !strings.HasPrefix(thumbnailURL, "http://127.0.0.1:2026/mp/proxy?url=") {
			t.Fatalf("thumbnail %d is not a local proxy URL: %q", index, thumbnailURL)
		}
	}
	for index, entry := range feed.Entry {
		if len(entry.Link) != 1 {
			t.Fatalf("entry %d has unexpected links: %+v", index, entry.Link)
		}
		assertProxyTarget(t, entry.Link[0].Href, "mp.weixin.qq.com")
		if strings.Contains(entry.Summary.Body, "<img") && !strings.Contains(entry.Summary.Body, `src="http://127.0.0.1:2026/mp/proxy?url=`) {
			t.Fatalf("entry %d summary does not contain a local cover URL: %s", index, entry.Summary.Body)
		}
	}
}

func TestHandleProxyRejectsUntrustedHost(t *testing.T) {
	service := NewMemoryService()
	request := httptest.NewRequest(http.MethodGet, "/mp/proxy?url="+url.QueryEscape("https://example.com/article"), nil)
	recorder := httptest.NewRecorder()
	service.HandleProxy(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestRewriteProxyDocumentRewritesQPicAssets(t *testing.T) {
	service := NewMemoryService()
	service.SetAPIOrigin("http://127.0.0.1:2026")
	request := httptest.NewRequest(http.MethodGet, "/mp/proxy", nil)
	body := []byte(`<img src="https://mmbiz.qpic.cn/mmbiz_jpg/demo/0?wx_fmt=jpeg">`)
	rewritten := string(service.rewriteProxyDocument(request, body))
	if !strings.Contains(rewritten, "http://127.0.0.1:2026/mp/proxy?url=") {
		t.Fatalf("asset was not rewritten: %s", rewritten)
	}
	if strings.Contains(rewritten, "mmbiz.qpic.cn/mmbiz_jpg/demo/0?wx_fmt=jpeg\">") {
		t.Fatalf("original asset URL remained: %s", rewritten)
	}
}

func TestHandleProxyServesArticleAndImageThroughLocalURLs(t *testing.T) {
	service := NewMemoryService()
	service.SetAPIOrigin("http://127.0.0.1:2026")
	service.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Hostname() {
		case "mp.weixin.qq.com":
			return testProxyResponse(http.StatusOK, "text/html; charset=utf-8", `<html><body><div id="js_content"><p>正文</p><img src="https://mmbiz.qpic.cn/mmbiz_jpg/demo/0?wx_fmt=jpeg"></div></body></html>`), nil
		case "mmbiz.qpic.cn":
			return testProxyResponse(http.StatusOK, "image/jpeg", "fake-image"), nil
		default:
			return nil, errors.New("unexpected proxy host: " + r.URL.Hostname())
		}
	})})

	articleRequest := httptest.NewRequest(http.MethodGet, "/mp/proxy?url="+url.QueryEscape("https://mp.weixin.qq.com/s?mid=1"), nil)
	articleRecorder := httptest.NewRecorder()
	service.HandleProxy(articleRecorder, articleRequest)
	if articleRecorder.Code != http.StatusOK {
		t.Fatalf("expected article 200, got %d: %s", articleRecorder.Code, articleRecorder.Body.String())
	}
	articleBody := articleRecorder.Body.String()
	marker := `src="http://127.0.0.1:2026/mp/proxy?url=`
	start := strings.Index(articleBody, marker)
	if start < 0 {
		t.Fatalf("article image was not rewritten: %s", articleBody)
	}
	start += len(`src="`)
	end := strings.Index(articleBody[start:], `"`)
	if end < 0 {
		t.Fatalf("rewritten image URL was not terminated: %s", articleBody)
	}
	imageURL := articleBody[start : start+end]

	imageRequest := httptest.NewRequest(http.MethodGet, imageURL, nil)
	imageRecorder := httptest.NewRecorder()
	service.HandleProxy(imageRecorder, imageRequest)
	if imageRecorder.Code != http.StatusOK {
		t.Fatalf("expected image 200, got %d: %s", imageRecorder.Code, imageRecorder.Body.String())
	}
	if imageRecorder.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("unexpected image content type: %q", imageRecorder.Header().Get("Content-Type"))
	}
	if imageRecorder.Body.String() != "fake-image" {
		t.Fatalf("unexpected image body: %q", imageRecorder.Body.String())
	}
}

func TestExtractArticleContentNormalizesLazyImages(t *testing.T) {
	content := extractArticleContent([]byte(`<html><body><div id="js_content"><p>正文</p><img data-src="//mmbiz.qpic.cn/image/1"></div></body></html>`))
	if !strings.Contains(content, "正文") || !strings.Contains(content, `src="https://mmbiz.qpic.cn/image/1"`) {
		t.Fatalf("unexpected extracted content: %s", content)
	}
}

func TestFetchArticleListRefreshesCookie(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("action") == "show" {
			http.SetCookie(w, &http.Cookie{Name: "slave_user", Value: "cookie-value"})
			return
		}
		if r.URL.Query().Get("action") != "get_articles" {
			t.Fatalf("unexpected author request: %s", r.URL.String())
		}
		if !strings.Contains(r.Header.Get("Cookie"), "slave_user=cookie-value") {
			t.Fatalf("refreshed cookie was not sent: %q", r.Header.Get("Cookie"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ret":0,"articles":[{"title":"article","url":"https://mp.weixin.qq.com/s?id=1"}],"base_resp":{"ret":0}}`))
	}))
	defer server.Close()

	service := NewMemoryService()
	service.SetUpstreamBaseURL(server.URL)
	service.SetHTTPClient(server.Client())
	if err := service.Upsert(Account{
		Biz:         "biz-1",
		AuthorID:    "author-1",
		Key:         "key-1",
		Uin:         "uin-1",
		PassTicket:  "ticket-1",
		AppmsgToken: "token-1",
	}); err != nil {
		t.Fatalf("upsert account: %v", err)
	}

	data, err := service.FetchArticleList("biz-1")
	if err != nil {
		t.Fatalf("fetch article list: %v", err)
	}
	if len(data.Articles) != 1 || data.Articles[0].Title != "article" {
		t.Fatalf("unexpected articles: %+v", data.Articles)
	}
}

func TestUpdateMetadataMergesWithoutTouchingCredentials(t *testing.T) {
	service := NewMemoryService()
	if err := service.Upsert(Account{
		Biz:        "biz-1",
		Nickname:   "旧名称",
		AuthorID:   "old-author",
		Key:        "key-1",
		Uin:        "uin-1",
		PassTicket: "ticket-1",
		RefreshURI: "https://mp.weixin.qq.com/s?__biz=biz-1",
	}); err != nil {
		t.Fatalf("upsert account: %v", err)
	}
	before, ok := service.accountSnapshot("biz-1")
	if !ok {
		t.Fatal("account was not stored")
	}

	if err := service.UpdateMetadata(Account{
		Biz:       "biz-1",
		Nickname:  "新名称",
		AvatarURL: "https://mmbiz.qpic.cn/avatar/1",
		AuthorID:  "new-author",
	}); err != nil {
		t.Fatalf("update metadata: %v", err)
	}
	after, ok := service.accountSnapshot("biz-1")
	if !ok {
		t.Fatal("account disappeared after metadata update")
	}
	if after.Nickname != "新名称" || after.AvatarURL == "" || after.AuthorID != "new-author" {
		t.Fatalf("metadata was not merged: %+v", after)
	}
	if after.Key != before.Key || after.Uin != before.Uin || after.PassTicket != before.PassTicket || after.RefreshURI != before.RefreshURI {
		t.Fatalf("metadata update changed credentials: before=%+v after=%+v", before, after)
	}
	if after.UpdateTime != before.UpdateTime || after.CreatedAt != before.CreatedAt {
		t.Fatalf("metadata update changed credential timestamps: before=%+v after=%+v", before, after)
	}

	if err := service.UpdateMetadata(Account{Biz: "biz-1"}); !errors.Is(err, ErrMissingMetadata) {
		t.Fatalf("expected missing metadata error, got %v", err)
	}
	if err := service.UpdateMetadata(Account{Biz: "missing", Nickname: "名称"}); !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("expected missing account error, got %v", err)
	}
}

func TestAccountPersistenceAndSafeSummary(t *testing.T) {
	store := filepath.Join(t.TempDir(), "mp.json")
	service, err := NewService(store)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if err := service.Upsert(Account{
		Biz:         "biz-1",
		Nickname:    "公众号",
		Key:         "secret-key",
		RefreshURI:  "https://mp.weixin.qq.com/s?__biz=biz-1&key=secret-key&pass_ticket=secret-ticket&mid=1",
		AppmsgToken: "secret-appmsg-token",
	}); err != nil {
		t.Fatalf("upsert account: %v", err)
	}

	reloaded, err := NewService(store)
	if err != nil {
		t.Fatalf("reload service: %v", err)
	}
	list := reloaded.ListAccounts()
	if len(list) != 1 || list[0].Nickname != "公众号" {
		t.Fatalf("unexpected summaries: %+v", list)
	}
	encoded, _ := json.Marshal(list)
	if strings.Contains(string(encoded), "secret-key") || strings.Contains(string(encoded), "secret-ticket") {
		t.Fatalf("safe summary leaked credential: %s", encoded)
	}
	if list[0].RefreshURI != "https://mp.weixin.qq.com/s?__biz=biz-1&mid=1" {
		t.Fatalf("refresh URI was not sanitized: %s", list[0].RefreshURI)
	}
}

func TestHandleListFiltersCapturedAccountsByNicknameOrBiz(t *testing.T) {
	service := NewMemoryService()
	if err := service.Upsert(Account{Biz: "MzI1NjA0MDg2Mw==", Nickname: "示例公众号", Key: "secret-one"}); err != nil {
		t.Fatalf("upsert first account: %v", err)
	}
	if err := service.Upsert(Account{Biz: "other-biz", Nickname: "另一个账号", Key: "secret-two"}); err != nil {
		t.Fatalf("upsert second account: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/mp/list?keyword=%E7%A4%BA%E4%BE%8B", nil)
	recorder := httptest.NewRecorder()
	service.HandleList(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected list response 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var payload struct {
		Code int `json:"code"`
		Data struct {
			List    []AccountSummary `json:"list"`
			Total   int              `json:"total"`
			Keyword string           `json:"keyword"`
		} `json:"data"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&payload); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if payload.Code != 0 || payload.Data.Total != 1 || payload.Data.Keyword != "示例" {
		t.Fatalf("unexpected filtered response: %+v", payload)
	}
	if len(payload.Data.List) != 1 || payload.Data.List[0].Biz != "MzI1NjA0MDg2Mw==" {
		t.Fatalf("unexpected filtered accounts: %+v", payload.Data.List)
	}
	if strings.Contains(recorder.Body.String(), "secret-one") || strings.Contains(recorder.Body.String(), "secret-two") {
		t.Fatalf("filtered response leaked credentials: %s", recorder.Body.String())
	}

	byBiz := service.ListAccountsByKeyword("OTHER-BIZ")
	if len(byBiz) != 1 || byBiz[0].Biz != "other-biz" {
		t.Fatalf("expected case-insensitive biz match, got %+v", byBiz)
	}
}

func sampleMessageListResponse() []byte {
	general, _ := json.Marshal(map[string]interface{}{
		"list": []map[string]interface{}{
			{
				"comm_msg_info": map[string]interface{}{"datetime": 1700000000},
				"app_msg_ext_info": map[string]interface{}{
					"title":          "parent",
					"digest":         "digest",
					"fileid":         10,
					"video_id":       "video-parent",
					"author":         "author",
					"subtype":        7,
					"copyright_stat": 1,
					"duration":       42,
					"audio_fileid":   23,
					"play_url":       "https://vd.example.test/video.mp4?token=short-lived",
					"cover":          "https://mmbiz.qpic.cn/mmbiz_jpg/demo/0",
					"content_url":    "https://mp.weixin.qq.com/s?mid=1",
					"multi_app_msg_item_list": []map[string]interface{}{
						{"title": "child", "fileid": 11, "video_id": "video-child", "duration": 84, "audio_fileid": 24, "play_url": "https://vd.example.test/child.mp4?token=short-lived", "content_url": "https://mp.weixin.qq.com/s?mid=1&idx=2"},
					},
				},
			},
		},
	})
	outer, _ := json.Marshal(map[string]interface{}{
		"ret":              0,
		"general_msg_list": string(general),
		"can_msg_continue": 1,
		"next_offset":      20,
	})
	return outer
}

func assertProxyTarget(t *testing.T, raw, expectedHost string) {
	t.Helper()
	proxyURL, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse proxy URL %q: %v", raw, err)
	}
	if proxyURL.Scheme != "http" || proxyURL.Host != "127.0.0.1:2026" || proxyURL.Path != "/mp/proxy" {
		t.Fatalf("unexpected local proxy URL %q", raw)
	}
	target, err := url.Parse(proxyURL.Query().Get("url"))
	if err != nil || target.Hostname() != expectedHost || target.Scheme != "https" {
		t.Fatalf("unexpected proxy target in %q: %v", raw, err)
	}
}

func rssThumbnailURLs(t *testing.T, payload []byte) []string {
	t.Helper()
	decoder := xml.NewDecoder(strings.NewReader(string(payload)))
	urls := make([]string, 0)
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return urls
		}
		if err != nil {
			t.Fatalf("decode RSS tokens: %v", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "thumbnail" {
			continue
		}
		if start.Name.Space != "http://search.yahoo.com/mrss/" {
			t.Fatalf("unexpected thumbnail namespace: %q", start.Name.Space)
		}
		for _, attribute := range start.Attr {
			if attribute.Name.Local == "url" {
				urls = append(urls, attribute.Value)
			}
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func testProxyResponse(status int, contentType, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

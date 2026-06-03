package services

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"wx_channel/internal/config"
)

const (
	defaultSphParseURL    = "https://yuanbao.tencent.com/api/weixin/get_parse_result"
	defaultSphFeedInfoURL = "https://channels.weixin.qq.com/finder-preview/api/feed/get_feed_info"
)

type SphService struct {
	client      *http.Client
	cfgProvider func() SphServiceConfig
	parseURL    string
	feedInfoURL string
}

type SphServiceConfig struct {
	SphHostname string
	SphCookie   string
}

type SphParseResponse struct {
	Code int          `json:"code"`
	Msg  string       `json:"msg"`
	Data SphParseData `json:"data"`
}

type SphParseData struct {
	WxExportID  string `json:"wx_export_id"`
	CoverURL    string `json:"cover_url"`
	Author      string `json:"author"`
	Desc        string `json:"desc"`
	PlayableURL string `json:"playable_url"`
}

type SphFeedResponse struct {
	Data    SphFeedData `json:"data"`
	ErrCode int         `json:"errCode"`
	ErrMsg  string      `json:"errMsg"`
}

type SphFeedData struct {
	AuthorInfo SphAuthorInfo `json:"authorInfo"`
	FeedInfo   SphFeedInfo   `json:"feedInfo"`
	SceneInfo  SphSceneInfo  `json:"sceneInfo"`
}

type SphAuthorInfo struct {
	Nickname    string `json:"nickname"`
	HeadImgURL  string `json:"headImgUrl"`
	AuthIconURL string `json:"authIconUrl"`
}

type SphFeedInfo struct {
	VideoURL        string       `json:"videoUrl"`
	OriginVideoURL  string       `json:"originVideoUrl"`
	Description     string       `json:"description"`
	MediaType       int          `json:"mediaType"`
	FavCountFmt     string       `json:"favCountFmt"`
	LikeCountFmt    string       `json:"likeCountFmt"`
	ForwardCountFmt string       `json:"forwardCountFmt"`
	CommentCountFmt string       `json:"commentCountFmt"`
	H264VideoInfo   SphVideoInfo `json:"h264VideoInfo"`
	H265VideoInfo   SphVideoInfo `json:"h265VideoInfo"`
	CreateTime      int64        `json:"createtime"`
	CoverURL        string       `json:"coverUrl"`
}

type SphVideoInfo struct {
	VideoURL string `json:"videoUrl"`
}

type SphSceneInfo struct {
	DynamicExportID string `json:"dynamicExportId"`
}

func NewSphService() *SphService {
	return &SphService{
		client: &http.Client{Timeout: 30 * time.Second},
		cfgProvider: func() SphServiceConfig {
			cfg := config.Get()
			if cfg == nil {
				return SphServiceConfig{}
			}
			return SphServiceConfig{
				SphHostname: cfg.Cloudflare.SphHostname,
				SphCookie:   cfg.Cloudflare.SphCookie,
			}
		},
		parseURL:    defaultSphParseURL,
		feedInfoURL: defaultSphFeedInfoURL,
	}
}

func NewSphServiceWithConfigProvider(provider func() SphServiceConfig) *SphService {
	service := NewSphService()
	if provider != nil {
		service.cfgProvider = provider
	}
	return service
}

func (s *SphService) Enabled() bool {
	cfg := s.config()
	return strings.TrimSpace(cfg.SphHostname) != "" || strings.TrimSpace(cfg.SphCookie) != ""
}

func (s *SphService) FetchVideoProfile(ctx context.Context, shareURL string) (*SphFeedResponse, error) {
	shareURL = strings.TrimSpace(shareURL)
	if shareURL == "" {
		return nil, fmt.Errorf("url parameter is required")
	}

	cfg := s.config()
	if workerHost := normalizeSphHostname(cfg.SphHostname); workerHost != "" {
		return s.fetchViaWorker(ctx, workerHost, shareURL)
	}

	cookie := strings.TrimSpace(cfg.SphCookie)
	if cookie == "" {
		return nil, fmt.Errorf("cloudflare.sphHostname or cloudflare.sphCookie not configured")
	}

	return s.fetchDirect(ctx, shareURL, cookie)
}

func (s *SphService) config() SphServiceConfig {
	if s.cfgProvider == nil {
		return SphServiceConfig{}
	}
	return s.cfgProvider()
}

func (s *SphService) fetchViaWorker(ctx context.Context, workerHost, shareURL string) (*SphFeedResponse, error) {
	endpoint := strings.TrimRight(workerHost, "/") + "/api/fetch_video_profile"
	body, err := json.Marshal(map[string]string{"url": shareURL})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		message := readResponseBody(resp.Body)
		if message == "" {
			message = resp.Status
		}
		return nil, fmt.Errorf("worker fetch failed: %s", message)
	}

	var result SphFeedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if result.ErrCode != 0 {
		message := strings.TrimSpace(result.ErrMsg)
		if message == "" {
			message = fmt.Sprintf("errCode=%d", result.ErrCode)
		}
		return nil, fmt.Errorf("worker feed profile failed: %s", message)
	}
	result.Data.FeedInfo.OriginVideoURL = CleanSharedVideoURL(result.Data.FeedInfo.OriginVideoURL)
	if result.Data.FeedInfo.OriginVideoURL == "" {
		result.Data.FeedInfo.OriginVideoURL = CleanSharedVideoURL(result.Data.FeedInfo.VideoURL)
	}
	return &result, nil
}

func (s *SphService) fetchDirect(ctx context.Context, shareURL, cookie string) (*SphFeedResponse, error) {
	parseResp, err := s.parseShareURL(ctx, shareURL, cookie)
	if err != nil {
		return nil, fmt.Errorf("parse share url: %w", err)
	}

	token, exportID := extractPlayableTokenAndExportID(parseResp.Data.PlayableURL)
	if exportID == "" {
		exportID = parseResp.Data.WxExportID
	}
	if exportID == "" {
		return nil, fmt.Errorf("parse share url: missing export id")
	}
	if token == "" {
		return nil, fmt.Errorf("parse share url: missing general token")
	}

	feedResp, err := s.getFeedInfo(ctx, exportID, token)
	if err != nil {
		return nil, fmt.Errorf("get feed info: %w", err)
	}

	if feedResp.Data.SceneInfo.DynamicExportID == "" {
		feedResp.Data.SceneInfo.DynamicExportID = exportID
	}
	feedResp.Data.FeedInfo.OriginVideoURL = CleanSharedVideoURL(feedResp.Data.FeedInfo.VideoURL)
	return feedResp, nil
}

func (s *SphService) parseShareURL(ctx context.Context, shareURL, cookie string) (*SphParseResponse, error) {
	payload, err := json.Marshal(map[string]interface{}{
		"type":  "video_channel_url",
		"url":   shareURL,
		"scene": 1,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.parseURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}

	for k, v := range defaultSphParseHeaders(cookie) {
		req.Header.Set(k, v)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		message := readResponseBody(resp.Body)
		if message == "" {
			message = resp.Status
		}
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, message)
	}

	var result SphParseResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if result.Code != 0 {
		message := strings.TrimSpace(result.Msg)
		if message == "" {
			message = fmt.Sprintf("code=%d", result.Code)
		}
		return nil, fmt.Errorf("%s", message)
	}
	if strings.TrimSpace(result.Data.PlayableURL) == "" && strings.TrimSpace(result.Data.WxExportID) == "" {
		return nil, fmt.Errorf("missing playable_url and wx_export_id")
	}
	return &result, nil
}

func (s *SphService) getFeedInfo(ctx context.Context, exportID, generalToken string) (*SphFeedResponse, error) {
	payload, err := json.Marshal(map[string]interface{}{
		"baseReq": map[string]string{
			"generalToken": generalToken,
		},
		"exportId": exportID,
	})
	if err != nil {
		return nil, err
	}

	apiURL := fmt.Sprintf("%s?_rid=%s&_pageUrl=https:%%2F%%2Fchannels.weixin.qq.com%%2Ffinder-preview%%2Fpages%%2Ffeed", s.feedInfoURL, generateSphRid())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}

	for k, v := range defaultSphFeedInfoHeaders(exportID, generalToken) {
		req.Header.Set(k, v)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		message := readResponseBody(resp.Body)
		if message == "" {
			message = resp.Status
		}
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, message)
	}

	var result SphFeedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if result.ErrCode != 0 {
		message := strings.TrimSpace(result.ErrMsg)
		if message == "" {
			message = fmt.Sprintf("errCode=%d", result.ErrCode)
		}
		return nil, fmt.Errorf("%s", message)
	}
	return &result, nil
}

func CleanSharedVideoURL(videoURL string) string {
	u, err := url.Parse(strings.TrimSpace(videoURL))
	if err != nil {
		return ""
	}
	fileKey := strings.TrimSpace(u.Query().Get("encfilekey"))
	token := strings.TrimSpace(u.Query().Get("token"))
	if fileKey == "" || token == "" {
		return ""
	}

	clean := url.URL{
		Scheme: u.Scheme,
		Host:   u.Host,
		Path:   u.Path,
	}
	query := url.Values{}
	query.Set("encfilekey", fileKey)
	query.Set("token", token)
	clean.RawQuery = query.Encode()
	return clean.String()
}

func normalizeSphHostname(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	return strings.TrimRight(raw, "/")
}

func extractPlayableTokenAndExportID(playableURL string) (string, string) {
	u, err := url.Parse(strings.TrimSpace(playableURL))
	if err != nil {
		return "", ""
	}
	return strings.TrimSpace(u.Query().Get("token")), strings.TrimSpace(u.Query().Get("eid"))
}

func generateSphRid() string {
	ts := fmt.Sprintf("%x", time.Now().Unix())
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return ts + "-00000000"
	}
	return ts + "-" + hex.EncodeToString(buf)
}

func readResponseBody(r io.Reader) string {
	body, err := io.ReadAll(io.LimitReader(r, 2048))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(body))
}

func defaultSphParseHeaders(cookie string) map[string]string {
	return map[string]string{
		"accept":             "application/json, text/plain, */*",
		"accept-language":    "zh-CN,zh;q=0.9,en;q=0.8",
		"content-type":       "application/json",
		"origin":             "https://yuanbao.tencent.com",
		"referer":            "https://yuanbao.tencent.com/chat/naQivTmsDa/cf4d0079-ed1b-4c55-a3f3-2ca1379727d1",
		"user-agent":         "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36",
		"sec-ch-ua":          `"Chromium";v="148", "Google Chrome";v="148", "Not/A)Brand";v="99"`,
		"sec-ch-ua-mobile":   "?0",
		"sec-ch-ua-platform": `"macOS"`,
		"sec-fetch-dest":     "empty",
		"sec-fetch-mode":     "cors",
		"sec-fetch-site":     "same-origin",
		"t-userid":           "b9575f6b0a8c4a55a08096904a5ef20a",
		"x-agentid":          "naQivTmsDa/cf4d0079-ed1b-4c55-a3f3-2ca1379727d1",
		"x-commit-tag":       "72282a0d",
		"x-device-id":        "1921b001708100d7fa31002b9646bd0cc15a3e2e1f",
		"x-hy106":            "",
		"x-hy92":             "e963067ffa31002b9646bd0c03000008b1951a",
		"x-hy93":             "1921b001708100d7fa31002b9646bd0cc15a3e2e1f",
		"x-id":               "b9575f6b0a8c4a55a08096904a5ef20a",
		"x-instance-id":      "5",
		"x-language":         "zh-CN",
		"x-os_version":       "Mac OS(10.15.7)-Blink",
		"x-platform":         "mac",
		"x-requested-with":   "XMLHttpRequest",
		"x-source":           "web",
		"x-web-third-source": "main",
		"x-webdriver":        "0",
		"x-webversion":       "2.69.0",
		"x-ybuitest":         "0",
		"cookie":             cookie,
	}
}

func defaultSphFeedInfoHeaders(exportID, generalToken string) map[string]string {
	referer := fmt.Sprintf(
		"https://channels.weixin.qq.com/finder-preview/pages/feed?entry_card_type=48&comment_scene=39&appid=0&token=%s&entry_scene=0&eid=%s",
		url.QueryEscape(generalToken),
		url.QueryEscape(exportID),
	)

	return map[string]string{
		"accept":             "application/json, text/plain, */*",
		"accept-language":    "zh-CN,zh;q=0.9,en;q=0.8",
		"connection":         "keep-alive",
		"content-type":       "application/json",
		"origin":             "https://channels.weixin.qq.com",
		"referer":            referer,
		"sec-fetch-dest":     "empty",
		"sec-fetch-mode":     "cors",
		"sec-fetch-site":     "same-origin",
		"user-agent":         "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36",
		"sec-ch-ua":          `"Chromium";v="148", "Google Chrome";v="148", "Not/A)Brand";v="99"`,
		"sec-ch-ua-mobile":   "?0",
		"sec-ch-ua-platform": `"macOS"`,
	}
}

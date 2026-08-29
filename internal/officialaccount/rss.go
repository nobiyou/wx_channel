package officialaccount

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const maxArticleBody = 8 << 20

func (s *Service) HandleRSS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		responseError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	biz := strings.TrimSpace(r.URL.Query().Get("biz"))
	offset := parseOffset(r.URL.Query().Get("offset"))
	data, err := s.FetchMsgList(biz, offset)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	feed := s.buildFeed(r, biz, data, r.URL.Query().Get("proxy") == "1", r.URL.Query().Get("proxy_cover") == "1", r.URL.Query().Get("content") == "1")
	payload, err := xml.MarshalIndent(feed, "", "  ")
	if err != nil {
		writeServiceError(w, fmt.Errorf("encode RSS feed: %w", err))
		return
	}

	w.Header().Set("Content-Type", "application/atom+xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(append([]byte(xml.Header), payload...))
}

func (s *Service) buildFeed(r *http.Request, biz string, data *MessageListResponse, proxy, proxyCover, includeContent bool) AtomFeed {
	account, _ := s.accountSnapshot(biz)
	feedTitle := firstNonEmpty(account.Nickname, biz)
	feedURI := s.publicProfileURL(account.Biz)
	entries := make([]AtomEntry, 0, len(data.Articles))

	for _, article := range data.Articles {
		published := formatPublishedTime(article.PublishTime)
		contentURL := normalizeArticleURL(article.ContentURL)
		if contentURL != "" && strings.HasPrefix(contentURL, "/") {
			contentURL = s.upstream() + contentURL
		}
		entryURL := contentURL
		if proxy && entryURL != "" {
			entryURL = s.proxyURL(r, entryURL)
		}
		coverURL := normalizeArticleURL(article.Cover)
		if proxy || proxyCover {
			if coverURL != "" {
				coverURL = s.proxyURL(r, coverURL)
			}
		}
		description := article.Digest
		if coverURL != "" {
			description = fmt.Sprintf(`<img src="%s" /><br/>%s`, coverURL, article.Digest)
		}
		content := description
		if includeContent && contentURL != "" {
			if fullContent := s.fetchArticleContent(contentURL); fullContent != "" {
				content = fullContent
			}
		}
		if entryURL == "" {
			entryURL = fmt.Sprintf("%s#%d", biz, article.FileID)
		}
		entryID := entryURL
		if proxy && contentURL != "" {
			entryID = contentURL
		}
		if entryID == "" {
			entryID = fmt.Sprintf("%s#%d", biz, article.FileID)
		}
		entryAuthor := firstNonEmpty(article.Author, feedTitle)
		entries = append(entries, AtomEntry{
			ID:        entryID,
			Title:     article.Title,
			Updated:   published,
			Published: published,
			Author: AtomAuthor{
				Name: entryAuthor,
			},
			Link: []AtomLink{{Rel: "alternate", Href: entryURL}},
			Content: AtomContent{
				Type: "html",
				Body: content,
			},
			Summary: AtomContent{
				Type: "html",
				Body: description,
			},
			MediaThumbnail: buildThumbnail(coverURL),
		})
	}

	links := []AtomLink{
		{Rel: "self", Href: s.requestOrigin(r) + requestURL(r)},
		{Rel: "alternate", Href: feedURI},
	}
	if data.CanMsgContinue != 0 && data.NextOffset > offsetFromRequest(r) {
		next := *r.URL
		query := next.Query()
		query.Set("offset", fmt.Sprintf("%d", data.NextOffset))
		next.RawQuery = query.Encode()
		links = append(links, AtomLink{Rel: "next", Href: s.requestOrigin(r) + next.String()})
	}

	return AtomFeed{
		XMLNS:      "http://www.w3.org/2005/Atom",
		XMLNSMedia: "http://search.yahoo.com/mrss/",
		Title:      feedTitle,
		ID:         biz,
		Updated:    time.Now().UTC().Format(time.RFC3339),
		Generator:  "wx_channel",
		Icon:       account.AvatarURL,
		Category:   []AtomCategory{{Term: "微信公众号"}},
		Link:       links,
		Author: AtomAuthor{
			Name: feedTitle,
			URI:  feedURI,
		},
		Entry: entries,
	}
}

func (s *Service) publicProfileURL(biz string) string {
	u, _ := url.Parse(s.upstream() + "/mp/profile_ext")
	query := u.Query()
	query.Set("action", "home")
	query.Set("__biz", biz)
	query.Set("scene", "124")
	u.RawQuery = query.Encode()
	return u.String()
}

func (s *Service) fetchArticleContent(rawURL string) string {
	target, err := parseAllowedProxyURL(rawURL)
	if err != nil || target.Hostname() != "mp.weixin.qq.com" {
		return ""
	}
	req, err := http.NewRequest(http.MethodGet, target.String(), nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	req.Header.Set("User-Agent", defaultUserAgent)
	resp, err := s.client().Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxArticleBody+1))
	if err != nil || len(body) > maxArticleBody {
		return ""
	}
	return extractArticleContent(body)
}

func extractArticleContent(body []byte) string {
	document, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return ""
	}
	contentNode := findArticleContentNode(document)
	if contentNode == nil {
		return ""
	}
	normalizeArticleImages(contentNode)
	var output bytes.Buffer
	for child := contentNode.FirstChild; child != nil; child = child.NextSibling {
		if err := html.Render(&output, child); err != nil {
			return ""
		}
	}
	return strings.TrimSpace(output.String())
}

func findArticleContentNode(node *html.Node) *html.Node {
	if node.Type == html.ElementNode {
		for _, attr := range node.Attr {
			if attr.Key == "id" && attr.Val == "js_content" {
				return node
			}
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findArticleContentNode(child); found != nil {
			return found
		}
	}
	return nil
}

func normalizeArticleImages(node *html.Node) {
	if node.Type == html.ElementNode {
		hasSrc := false
		dataSrc := ""
		filtered := node.Attr[:0]
		for _, attr := range node.Attr {
			switch attr.Key {
			case "src":
				hasSrc = true
				if strings.HasPrefix(attr.Val, "//") {
					attr.Val = "https:" + attr.Val
				}
			case "data-src":
				dataSrc = attr.Val
				continue
			}
			filtered = append(filtered, attr)
		}
		if !hasSrc && dataSrc != "" {
			if strings.HasPrefix(dataSrc, "//") {
				dataSrc = "https:" + dataSrc
			}
			filtered = append(filtered, html.Attribute{Key: "src", Val: dataSrc})
		}
		node.Attr = filtered
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		normalizeArticleImages(child)
	}
}

func buildThumbnail(url string) *MediaThumbnail {
	if url == "" {
		return nil
	}
	return &MediaThumbnail{
		XMLNSMedia: "http://search.yahoo.com/mrss/",
		URL:        url,
		Width:      1200,
		Height:     630,
	}
}

func formatPublishedTime(unix int64) string {
	if unix <= 0 {
		return time.Now().UTC().Format(time.RFC3339)
	}
	return time.Unix(unix, 0).UTC().Format(time.RFC3339)
}

func offsetFromRequest(r *http.Request) int {
	if r == nil {
		return 0
	}
	return parseOffset(r.URL.Query().Get("offset"))
}

func requestURL(r *http.Request) string {
	if r == nil || r.URL == nil {
		return ""
	}
	return r.URL.String()
}

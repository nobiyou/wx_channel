package officialaccount

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

var ErrArticleIdentity = errors.New("article identity not found")

const (
	ArchiveResourceKindHTML  = "html"
	ArchiveResourceKindImage = "image"

	ArchiveResourceRoleArticleBody = "article_body"
	ArchiveResourceRoleAttachment  = "attachment"

	ArchiveRelationContains = "contains"

	archiveBodySortOrder      = 0
	archiveImageSortOrderBase = 100
)

// ArchivePlan is an in-memory article archive plan. It deliberately has no
// persistence or transport concerns so a later caller can map it to its own
// storage and download models.
type ArchivePlan struct {
	Content   ArchiveContent    `json:"content"`
	Resources []ArchiveResource `json:"resources"`
	Relations []ArchiveRelation `json:"relations"`
}

// ArchiveRequest is the transport payload shared by the plan and download
// endpoints. HTML is the article page (or an element containing #js_content),
// while Article keeps the metadata separate from its resources.
type ArchiveRequest struct {
	Biz     string      `json:"biz"`
	Article ArticleItem `json:"article"`
	HTML    string      `json:"html"`
	Force   bool        `json:"force,omitempty"`
}

func (r ArchiveRequest) BuildPlan() (ArchivePlan, error) {
	if strings.TrimSpace(r.Biz) == "" {
		return ArchivePlan{}, ErrMissingBiz
	}
	return BuildArticleArchivePlan(r.Biz, r.Article, r.HTML)
}

// ArchiveContent contains the stable article metadata shared by all resources
// in a plan.
type ArchiveContent struct {
	Key                    string `json:"key"`
	Biz                    string `json:"biz,omitempty"`
	Mid                    string `json:"mid,omitempty"`
	Idx                    int    `json:"idx,omitempty"`
	FileID                 int    `json:"file_id,omitempty"`
	VideoID                string `json:"video_id,omitempty"`
	Title                  string `json:"title"`
	Description            string `json:"description,omitempty"`
	Author                 string `json:"author,omitempty"`
	URL                    string `json:"url,omitempty"`
	SourceURL              string `json:"source_url,omitempty"`
	CoverURL               string `json:"cover_url,omitempty"`
	PublishTime            int64  `json:"publish_time,omitempty"`
	IsMulti                int    `json:"is_multi,omitempty"`
	IsOriginal             int    `json:"is_original,omitempty"`
	IsPaid                 int    `json:"is_paid,omitempty"`
	IsPaySubscribe         int    `json:"is_pay_subscribe,omitempty"`
	ItemShowType           int    `json:"item_show_type,omitempty"`
	Subtype                int    `json:"subtype,omitempty"`
	CopyrightStat          int    `json:"copyright_stat,omitempty"`
	Duration               int    `json:"duration,omitempty"`
	AudioFileID            int    `json:"audio_fileid,omitempty"`
	PlayURL                string `json:"play_url,omitempty"`
	MaliciousTitleReasonID int    `json:"malicious_title_reason_id,omitempty"`
	MaliciousContentType   int    `json:"malicious_content_type,omitempty"`
}

// ArchiveResource describes either the inline HTML body or one remote image.
// InlineBody is populated only for resources that can be represented without
// a network request.
type ArchiveResource struct {
	Key        string `json:"key"`
	Kind       string `json:"kind"`
	Role       string `json:"role"`
	Name       string `json:"name"`
	MIMEType   string `json:"mime_type,omitempty"`
	SourceURL  string `json:"source_url,omitempty"`
	InlineBody string `json:"inline_body,omitempty"`
	SortOrder  int    `json:"sort_order"`
}

// ArchiveRelation connects the article to one of its resources.
type ArchiveRelation struct {
	SourceKey string `json:"source_key"`
	TargetKey string `json:"target_key"`
	Type      string `json:"type"`
	SortOrder int    `json:"sort_order"`
}

// BuildArticleArchivePlan converts an ArticleItem and fetched article HTML to
// a stable, in-memory resource plan. pageHTML is expected to be the article
// page; its #js_content node becomes the inline HTML resource.
func BuildArticleArchivePlan(biz string, article ArticleItem, pageHTML string) (ArchivePlan, error) {
	biz = strings.TrimSpace(biz)
	if len(pageHTML) > maxArticleBody {
		return ArchivePlan{}, fmt.Errorf("%w: maximum article HTML size is %d bytes", ErrArticleTooLarge, maxArticleBody)
	}
	identity := buildArticleArchiveIdentity(biz, article)
	if identity == "" {
		return ArchivePlan{}, fmt.Errorf("%w: content URL, source URL, or file ID is required", ErrArticleIdentity)
	}

	contentHTML := extractArticleContent([]byte(pageHTML))
	if strings.TrimSpace(contentHTML) == "" {
		return ArchivePlan{}, fmt.Errorf("%w: #js_content is empty or missing", ErrContentNotFound)
	}

	contentURL := sanitizeArchiveMetadataURL(article.ContentURL)
	sourceURL := sanitizeArchiveMetadataURL(article.SourceURL)
	if sourceURL == "" {
		sourceURL = contentURL
	}
	bodySourceURL := contentURL
	if bodySourceURL == "" {
		bodySourceURL = sourceURL
	}

	content := ArchiveContent{
		Key:                    "article:" + archiveStableDigest(identity),
		Biz:                    biz,
		Mid:                    strings.TrimSpace(article.Mid),
		Idx:                    article.Idx,
		FileID:                 article.FileID,
		VideoID:                strings.TrimSpace(article.VideoID),
		Title:                  strings.TrimSpace(article.Title),
		Description:            strings.TrimSpace(article.Digest),
		Author:                 strings.TrimSpace(article.Author),
		URL:                    contentURL,
		SourceURL:              sourceURL,
		CoverURL:               sanitizeArchiveMetadataURL(article.Cover),
		PublishTime:            article.PublishTime,
		IsMulti:                article.IsMulti,
		IsOriginal:             article.IsOriginal,
		IsPaid:                 article.IsPaid,
		IsPaySubscribe:         article.IsPaySubscribe,
		ItemShowType:           article.ItemShowType,
		Subtype:                article.Subtype,
		CopyrightStat:          article.CopyrightStat,
		Duration:               article.Duration,
		AudioFileID:            article.AudioFileID,
		PlayURL:                article.PlayURL,
		MaliciousTitleReasonID: article.MaliciousTitleReasonID,
		MaliciousContentType:   article.MaliciousContentType,
	}

	resourceName := content.Title
	if resourceName == "" {
		resourceName = "article"
	}
	resources := []ArchiveResource{
		{
			Key:        "body:html",
			Kind:       ArchiveResourceKindHTML,
			Role:       ArchiveResourceRoleArticleBody,
			Name:       resourceName,
			MIMEType:   "text/html",
			SourceURL:  bodySourceURL,
			InlineBody: contentHTML,
			SortOrder:  archiveBodySortOrder,
		},
	}

	for index, imageURL := range archiveImageURLs(contentHTML) {
		digest := archiveStableDigest(imageURL)
		resources = append(resources, ArchiveResource{
			Key:       "inline_image:" + digest,
			Kind:      ArchiveResourceKindImage,
			Role:      ArchiveResourceRoleAttachment,
			Name:      digest,
			SourceURL: imageURL,
			SortOrder: archiveImageSortOrderBase + index,
		})
	}

	relations := make([]ArchiveRelation, 0, len(resources))
	for _, resource := range resources {
		relations = append(relations, ArchiveRelation{
			SourceKey: content.Key,
			TargetKey: resource.Key,
			Type:      ArchiveRelationContains,
			SortOrder: resource.SortOrder,
		})
	}

	return ArchivePlan{
		Content:   content,
		Resources: resources,
		Relations: relations,
	}, nil
}

func buildArticleArchiveIdentity(biz string, article ArticleItem) string {
	for _, rawURL := range []string{article.ContentURL, article.SourceURL} {
		if identity := stableArticleArchiveURLIdentity(rawURL); identity != "" {
			return "biz:" + biz + "|" + identity
		}
	}
	if article.FileID > 0 {
		return "biz:" + biz + "|file:" + strconv.Itoa(article.FileID)
	}
	articleURL := firstNonEmpty(
		normalizeArchiveURL(article.ContentURL),
		normalizeArchiveURL(article.SourceURL),
	)
	if articleURL != "" {
		return "biz:" + biz + "|url:" + canonicalArticleArchiveURL(articleURL)
	}
	return ""
}

func stableArticleArchiveURLIdentity(raw string) string {
	normalized := normalizeArchiveURL(raw)
	if normalized == "" {
		return ""
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return ""
	}

	query := parsed.Query()
	idx := strings.TrimSpace(query.Get("idx"))
	if idx == "" {
		idx = "1"
	}
	if mid := strings.TrimSpace(query.Get("mid")); mid != "" {
		return "mid:" + mid + "|idx:" + idx
	}
	if sn := strings.TrimSpace(query.Get("sn")); sn != "" {
		return "sn:" + sn + "|idx:" + idx
	}
	return ""
}

var archiveIdentityEphemeralQueryKeys = []string{
	"uin",
	"key",
	"pass_ticket",
	"appmsg_token",
	"wxtoken",
	"token",
	"scene",
	"rscene",
	"ascene",
	"chksm",
	"sessionid",
	"session_id",
}

func canonicalArticleArchiveURL(raw string) string {
	raw = normalizeArchiveURL(raw)
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}

	query := parsed.Query()
	for _, key := range archiveIdentityEphemeralQueryKeys {
		query.Del(key)
	}
	parsed.RawQuery = query.Encode()
	parsed.Fragment = ""
	return parsed.String()
}

func sanitizeArchiveMetadataURL(raw string) string {
	normalized := normalizeArchiveURL(raw)
	parsed, err := url.Parse(normalized)
	if err != nil {
		return normalized
	}
	query := parsed.Query()
	changed := false
	for _, key := range []string{
		"uin",
		"key",
		"pass_ticket",
		"appmsg_token",
		"wxtoken",
		"token",
	} {
		if _, exists := query[key]; exists {
			query.Del(key)
			changed = true
		}
	}
	if !changed {
		return normalized
	}
	parsed.RawQuery = query.Encode()
	parsed.Fragment = ""
	return parsed.String()
}

// SanitizeArchiveMetadataURL removes short-lived WeChat credentials before a
// URL crosses the archive persistence boundary. The raw URL remains available
// in the in-memory plan for the downloader to use.
func SanitizeArchiveMetadataURL(raw string) string {
	return sanitizeArchiveMetadataURL(raw)
}

// SanitizeCatalogMediaURL removes the complete query string from short-lived
// media URLs. Unlike article URLs, media URLs commonly carry opaque playback
// signatures whose names are not stable enough for an allowlist.
func SanitizeCatalogMediaURL(raw string) string {
	normalized := normalizeArchiveURL(raw)
	parsed, err := url.Parse(normalized)
	if err != nil {
		return normalized
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

// SanitizeArchivePlanForResponse returns a detached plan safe to cross the
// HTTP response boundary. BuildArticleArchivePlan intentionally retains raw
// image URLs for the internal downloader, so the response view must not reuse
// those slices or inline HTML directly.
func SanitizeArchivePlanForResponse(plan ArchivePlan) ArchivePlan {
	plan.Content.URL = sanitizeArchiveMetadataURL(plan.Content.URL)
	plan.Content.SourceURL = sanitizeArchiveMetadataURL(plan.Content.SourceURL)
	plan.Content.CoverURL = sanitizeArchiveMetadataURL(plan.Content.CoverURL)
	plan.Content.PlayURL = SanitizeCatalogMediaURL(plan.Content.PlayURL)

	if plan.Resources != nil {
		resources := make([]ArchiveResource, len(plan.Resources))
		copy(resources, plan.Resources)
		for i := range resources {
			resources[i].SourceURL = sanitizeArchiveMetadataURL(resources[i].SourceURL)
			if resources[i].InlineBody != "" {
				resources[i].InlineBody = sanitizeArchiveHTMLURLs(resources[i].InlineBody)
			}
		}
		plan.Resources = resources
	}
	if plan.Relations != nil {
		plan.Relations = append([]ArchiveRelation(nil), plan.Relations...)
	}
	return plan
}

func sanitizeArchiveHTMLURLs(content string) string {
	document, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return content
	}
	sanitizeArchiveHTMLNode(document)

	body := findArchiveHTMLBody(document)
	if body == nil {
		return content
	}
	var output bytes.Buffer
	for child := body.FirstChild; child != nil; child = child.NextSibling {
		if err := html.Render(&output, child); err != nil {
			return content
		}
	}
	return strings.TrimSpace(output.String())
}

func sanitizeArchiveHTMLNode(node *html.Node) {
	if node.Type == html.ElementNode {
		for i := range node.Attr {
			switch strings.ToLower(node.Attr[i].Key) {
			case "src", "data-src", "href", "poster", "data-original", "data-url":
				node.Attr[i].Val = sanitizeArchiveMetadataURL(node.Attr[i].Val)
			}
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		sanitizeArchiveHTMLNode(child)
	}
}

func findArchiveHTMLBody(node *html.Node) *html.Node {
	if node.Type == html.ElementNode && strings.EqualFold(node.Data, "body") {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if body := findArchiveHTMLBody(child); body != nil {
			return body
		}
	}
	return nil
}

func archiveImageURLs(contentHTML string) []string {
	document, err := html.Parse(strings.NewReader(contentHTML))
	if err != nil {
		return nil
	}

	urls := make([]string, 0)
	seen := make(map[string]struct{})
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.ElementNode && strings.EqualFold(node.Data, "img") {
			rawURL := archiveAttribute(node, "src")
			if rawURL == "" {
				rawURL = archiveAttribute(node, "data-src")
			}
			imageURL := normalizeArchiveURL(rawURL)
			if isArchiveRemoteURL(imageURL) {
				if _, exists := seen[imageURL]; !exists {
					seen[imageURL] = struct{}{}
					urls = append(urls, imageURL)
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(document)
	return urls
}

func archiveAttribute(node *html.Node, key string) string {
	for _, attribute := range node.Attr {
		if strings.EqualFold(attribute.Key, key) {
			return strings.TrimSpace(attribute.Val)
		}
	}
	return ""
}

func isArchiveRemoteURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return true
	default:
		return false
	}
}

func normalizeArchiveURL(raw string) string {
	normalized := normalizeArticleURL(raw)
	if strings.HasPrefix(normalized, "//") {
		return "https:" + normalized
	}
	return normalized
}

// The digest is an identity key, matching the existing wxmp adapter's
// inline-image naming convention; it is not used for security decisions.
func archiveStableDigest(value string) string {
	digest := md5.Sum([]byte(value))
	return hex.EncodeToString(digest[:])
}

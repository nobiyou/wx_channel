package officialaccount

import (
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	SyncModeRecent  = "recent"
	SyncModeHistory = "history"

	SyncStatusQueued    = "queued"
	SyncStatusRunning   = "running"
	SyncStatusCompleted = "completed"
	SyncStatusPartial   = "partial"
	SyncStatusFailed    = "failed"
	SyncStatusCancelled = "cancelled"
	SyncStatusNever     = "never"

	ArchiveStatusNotArchived = "not_archived"
	ArchiveStatusQueued      = "queued"
	ArchiveStatusArchived    = "archived"
	ArchiveStatusPartial     = "partial"
	ArchiveStatusFailed      = "failed"
)

var ErrCatalogUnavailable = errors.New("official account catalog is unavailable")

// AccountPage is the bounded local account result used by the console API.
type AccountPage struct {
	Items      []AccountSummary `json:"items"`
	Total      int64            `json:"total"`
	Page       int              `json:"page"`
	PageSize   int              `json:"page_size"`
	TotalPages int              `json:"total_pages"`
}

// ArticleRecord contains article metadata only. The body and binary resources
// remain files managed by the archive service.
type ArticleRecord struct {
	Key                    string `json:"key"`
	Biz                    string `json:"biz"`
	Mid                    string `json:"mid,omitempty"`
	Idx                    int    `json:"idx,omitempty"`
	FileID                 int    `json:"file_id,omitempty"`
	VideoID                string `json:"video_id,omitempty"`
	Title                  string `json:"title"`
	Digest                 string `json:"digest,omitempty"`
	Author                 string `json:"author,omitempty"`
	ContentURL             string `json:"content_url,omitempty"`
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

	FirstSeenAt     int64                  `json:"first_seen_at"`
	LastSeenAt      int64                  `json:"last_seen_at"`
	SourceDeleted   bool                   `json:"source_deleted"`
	ArchiveStatus   string                 `json:"archive_status"`
	ArchiveDir      string                 `json:"archive_dir,omitempty"`
	ArchiveHTML     string                 `json:"archive_html,omitempty"`
	ArchiveManifest string                 `json:"archive_manifest,omitempty"`
	ArchivedAt      int64                  `json:"archived_at,omitempty"`
	RawMetadata     string                 `json:"raw_metadata,omitempty"`
	Metrics         *ArticleMetricSnapshot `json:"metrics,omitempty"`
	MetricState     *ArticleMetricState    `json:"metric_state,omitempty"`
	Assets          []ArticleAsset         `json:"assets,omitempty"`
}

// ArticleAsset is a file/resource row associated with an article archive.
// Size is a pointer because a failed or not-yet-downloaded resource has no
// trustworthy size to report.
type ArticleAsset struct {
	ArticleKey  string `json:"article_key"`
	ResourceKey string `json:"resource_key"`
	Kind        string `json:"kind"`
	Role        string `json:"role"`
	SourceURL   string `json:"source_url,omitempty"`
	LocalPath   string `json:"local_path,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
	Size        *int64 `json:"size,omitempty"`
	Status      string `json:"status"`
	Error       string `json:"error,omitempty"`
	CreatedAt   int64  `json:"created_at,omitempty"`
	UpdatedAt   int64  `json:"updated_at,omitempty"`
}

type ArticleQuery struct {
	Biz           string
	Keyword       string
	ArchiveStatus string
	Page          int
	PageSize      int
	Sort          string
	Descending    bool
}

type ArticlePage struct {
	Items      []ArticleRecord `json:"items"`
	Total      int64           `json:"total"`
	Page       int             `json:"page"`
	PageSize   int             `json:"page_size"`
	TotalPages int             `json:"total_pages"`
}

type ArticleUpsertStats struct {
	Seen     int `json:"seen"`
	Inserted int `json:"inserted"`
	Updated  int `json:"updated"`
}

// ArticleMetricSnapshot intentionally uses pointers. WeChat omits several
// counters depending on page/session state; omitted is different from zero.
type ArticleMetricSnapshot struct {
	ID           int64  `json:"id,omitempty"`
	ArticleKey   string `json:"article_key"`
	ObservedAt   int64  `json:"observed_at"`
	Source       string `json:"source"`
	ViewCount    *int64 `json:"view_count,omitempty"`
	LikeCount    *int64 `json:"like_count,omitempty"`
	CommentCount *int64 `json:"comment_count,omitempty"`
	ShareCount   *int64 `json:"share_count,omitempty"`
	CollectCount *int64 `json:"collect_count,omitempty"`
	RewardCount  *int64 `json:"reward_count,omitempty"`
	RawMetadata  string `json:"raw_metadata,omitempty"`
}

type ArticleMetricPayload struct {
	ViewCount    *int64 `json:"view_count"`
	LikeCount    *int64 `json:"like_count"`
	CommentCount *int64 `json:"comment_count"`
	ShareCount   *int64 `json:"share_count"`
	CollectCount *int64 `json:"collect_count"`
	RewardCount  *int64 `json:"reward_count"`
}

const (
	MetricStatePending = "pending"
	MetricStateSuccess = "success"
	MetricStateUnknown = "unknown"
	MetricStateFailed  = "failed"
)

// ArticleMetricState is the durable latest-attempt state for one article. It
// is separate from snapshots so a missing upstream response is observable and
// resumable without manufacturing a zero-valued metric snapshot.
type ArticleMetricState struct {
	ArticleKey     string `json:"article_key"`
	Status         string `json:"status"`
	AttemptCount   int    `json:"attempt_count"`
	SuccessCount   int    `json:"success_count"`
	UnknownCount   int    `json:"unknown_count"`
	FailureCount   int    `json:"failure_count"`
	LastAttemptAt  int64  `json:"last_attempt_at,omitempty"`
	LastSuccessAt  int64  `json:"last_success_at,omitempty"`
	LastObservedAt int64  `json:"last_observed_at,omitempty"`
	NextRetryAt    int64  `json:"next_retry_at,omitempty"`
	LastSource     string `json:"last_source,omitempty"`
	LastError      string `json:"last_error,omitempty"`
	UpdatedAt      int64  `json:"updated_at"`
}

// MetricSyncRun tracks a resumable interaction-metric collection job. Total
// is the eligible work set at start time; it may be zero for an empty catalog.
type MetricSyncRun struct {
	ID               string `json:"id"`
	Biz              string `json:"biz"`
	Status           string `json:"status"`
	Force            bool   `json:"force"`
	Total            int    `json:"total"`
	Attempted        int    `json:"attempted"`
	Stored           int    `json:"stored"`
	Unknown          int    `json:"unknown"`
	Failed           int    `json:"failed"`
	AfterPublishTime int64  `json:"after_publish_time,omitempty"`
	AfterArticleKey  string `json:"after_article_key,omitempty"`
	StartedAt        int64  `json:"started_at"`
	FinishedAt       int64  `json:"finished_at,omitempty"`
	Error            string `json:"error,omitempty"`
}

type MetricSyncRequest struct {
	Biz    string `json:"biz"`
	Force  bool   `json:"force,omitempty"`
	Resume bool   `json:"resume,omitempty"`
}

// ArticleMetricCandidatePage is keyset-paginated to keep a full-history scan
// bounded even when the article catalog becomes large.
type ArticleMetricCandidatePage struct {
	Items           []ArticleRecord `json:"items"`
	NextPublishTime int64           `json:"next_publish_time,omitempty"`
	NextArticleKey  string          `json:"next_article_key,omitempty"`
	HasMore         bool            `json:"has_more"`
}

type MetricRequest struct {
	Biz        string               `json:"biz"`
	Article    ArticleItem          `json:"article"`
	Metrics    ArticleMetricPayload `json:"metrics"`
	HTML       string               `json:"html,omitempty"`
	Source     string               `json:"source,omitempty"`
	ObservedAt int64                `json:"observed_at,omitempty"`
}

type MetricCaptureResult struct {
	ArticleKey string               `json:"article_key"`
	Stored     bool                 `json:"stored"`
	ObservedAt int64                `json:"observed_at,omitempty"`
	Source     string               `json:"source,omitempty"`
	Metrics    ArticleMetricPayload `json:"metrics"`
	State      *ArticleMetricState  `json:"state,omitempty"`
}

type ArticleArchiveState struct {
	ArticleKey   string
	Status       string
	Directory    string
	HTMLPath     string
	ManifestPath string
	ArchivedAt   int64
	Assets       []ArticleAsset
}

type SyncRun struct {
	ID          string `json:"id"`
	Biz         string `json:"biz"`
	Mode        string `json:"mode"`
	Status      string `json:"status"`
	Offset      int    `json:"offset"`
	NextOffset  int    `json:"next_offset"`
	PageSize    int    `json:"page_size"`
	PageCount   int    `json:"page_count"`
	Fetched     int    `json:"fetched"`
	Inserted    int    `json:"inserted"`
	Updated     int    `json:"updated"`
	CanContinue bool   `json:"can_continue"`
	StartedAt   int64  `json:"started_at"`
	FinishedAt  int64  `json:"finished_at,omitempty"`
	Error       string `json:"error,omitempty"`
}

type SyncRequest struct {
	Biz    string `json:"biz"`
	Mode   string `json:"mode,omitempty"`
	Resume bool   `json:"resume,omitempty"`
}

// CatalogRepository is the persistence boundary used by the account service.
// Implementations must keep batch article writes transactional and must not
// persist credential-bearing fields from Account.
type CatalogRepository interface {
	UpsertAccount(account Account) error
	ListAccounts(keyword string, page, pageSize int) (AccountPage, error)
	UpsertArticles(biz string, articles []ArticleRecord, seenAt time.Time) (ArticleUpsertStats, error)
	ListArticles(query ArticleQuery) (ArticlePage, error)
	GetArticle(key string) (*ArticleRecord, error)
	RecordArticleMetrics(metrics []ArticleMetricSnapshot) error
	LatestArticleMetrics(keys []string) (map[string]ArticleMetricSnapshot, error)
	ListArticleMetricStates(keys []string) (map[string]ArticleMetricState, error)
	ListArticleMetricCandidates(biz string, force bool, now time.Time, afterPublishTime int64, afterArticleKey string, limit int) (ArticleMetricCandidatePage, error)
	CountArticleMetricCandidates(biz string, force bool, now time.Time) (int, error)
	SaveArticleMetricResult(state ArticleMetricState, snapshot *ArticleMetricSnapshot) error
	UpdateArticleArchive(state ArticleArchiveState) error
	CreateSyncRun(run SyncRun) error
	UpdateSyncRun(run SyncRun) error
	GetSyncRun(id string) (*SyncRun, error)
	GetLatestSyncRun(biz, mode string) (*SyncRun, error)
	UpsertArticleAssets(articleKey string, assets []ArticleAsset) error
	ListArticleAssets(keys []string) (map[string][]ArticleAsset, error)
	CreateMetricSyncRun(run MetricSyncRun) error
	UpdateMetricSyncRun(run MetricSyncRun) error
	GetMetricSyncRun(id string) (*MetricSyncRun, error)
	GetLatestMetricSyncRun(biz string) (*MetricSyncRun, error)
}

// ArticleRecordFromItem converts an upstream item into a stable local record.
// ArticleKey uses the same identity algorithm as article archives so an
// archive can update the corresponding catalog row without a second mapping.
func ArticleRecordFromItem(biz string, item ArticleItem, seenAt time.Time) (ArticleRecord, bool) {
	biz = strings.TrimSpace(biz)
	if biz == "" {
		return ArticleRecord{}, false
	}
	key := StableArticleKey(biz, item)
	if key == "" {
		return ArticleRecord{}, false
	}
	if seenAt.IsZero() {
		seenAt = time.Now()
	}
	mid, idx := articleURLParts(item)
	if strings.TrimSpace(item.Mid) != "" {
		mid = strings.TrimSpace(item.Mid)
	}
	if item.Idx != 0 {
		idx = item.Idx
	}
	return ArticleRecord{
		Key:                    key,
		Biz:                    biz,
		Mid:                    mid,
		Idx:                    idx,
		FileID:                 item.FileID,
		VideoID:                strings.TrimSpace(item.VideoID),
		Title:                  strings.TrimSpace(item.Title),
		Digest:                 strings.TrimSpace(item.Digest),
		Author:                 strings.TrimSpace(item.Author),
		ContentURL:             SanitizeArchiveMetadataURL(item.ContentURL),
		SourceURL:              SanitizeArchiveMetadataURL(item.SourceURL),
		CoverURL:               SanitizeArchiveMetadataURL(item.Cover),
		PublishTime:            item.PublishTime,
		IsMulti:                item.IsMulti,
		IsOriginal:             item.IsOriginal,
		IsPaid:                 item.IsPaid,
		IsPaySubscribe:         item.IsPaySubscribe,
		ItemShowType:           item.ItemShowType,
		Subtype:                item.Subtype,
		CopyrightStat:          item.CopyrightStat,
		Duration:               item.Duration,
		AudioFileID:            item.AudioFileID,
		PlayURL:                SanitizeCatalogMediaURL(item.PlayURL),
		MaliciousTitleReasonID: item.MaliciousTitleReasonID,
		MaliciousContentType:   item.MaliciousContentType,
		SourceDeleted:          item.DelFlag != 0,
		FirstSeenAt:            seenAt.Unix(),
		LastSeenAt:             seenAt.Unix(),
		ArchiveStatus:          ArchiveStatusNotArchived,
		RawMetadata:            articleMetadataForStorage(item),
	}, true
}

func ArticleItemFromArticle(article Article) ArticleItem {
	return ArticleItem{
		Title:                  article.Title,
		ContentURL:             article.URL,
		VideoID:                strings.TrimSpace(article.VideoID),
		Mid:                    article.Mid,
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
		PlayURL:                SanitizeCatalogMediaURL(article.PlayURL),
		MaliciousTitleReasonID: article.MaliciousTitleReasonID,
		MaliciousContentType:   article.MaliciousContentType,
	}
}

// StableArticleKey is deterministic across tracking parameters and URL changes.
func StableArticleKey(biz string, item ArticleItem) string {
	identity := buildArticleArchiveIdentity(strings.TrimSpace(biz), item)
	if identity == "" {
		return ""
	}
	return "article:" + archiveStableDigest(identity)
}

func articleURLParts(item ArticleItem) (string, int) {
	for _, raw := range []string{item.ContentURL, item.SourceURL} {
		u, err := url.Parse(strings.TrimSpace(raw))
		if err != nil {
			continue
		}
		query := u.Query()
		mid := strings.TrimSpace(query.Get("mid"))
		idx := 0
		if rawIdx := strings.TrimSpace(query.Get("idx")); rawIdx != "" {
			idx, _ = strconv.Atoi(rawIdx)
		}
		if mid != "" {
			return mid, idx
		}
	}
	return "", 0
}

const maxCatalogRawMetadataBytes = 128 << 10

// SanitizeCatalogArticle applies the persistence/transport boundary to an
// already-normalized article. It is also used when reading older rows created
// before credential redaction was enforced.
func SanitizeCatalogArticle(article ArticleRecord) ArticleRecord {
	article.VideoID = strings.TrimSpace(article.VideoID)
	article.ContentURL = SanitizeArchiveMetadataURL(article.ContentURL)
	article.SourceURL = SanitizeArchiveMetadataURL(article.SourceURL)
	article.CoverURL = SanitizeArchiveMetadataURL(article.CoverURL)
	article.PlayURL = SanitizeCatalogMediaURL(article.PlayURL)
	if article.Mid == "" || article.Idx == 0 {
		mid, idx := articleURLParts(ArticleItem{
			ContentURL: article.ContentURL,
			SourceURL:  article.SourceURL,
		})
		if article.Mid == "" {
			article.Mid = mid
		}
		if article.Idx == 0 {
			article.Idx = idx
		}
	}
	article.RawMetadata = SanitizeCatalogRawMetadata(article.RawMetadata)
	if article.Metrics != nil {
		metric := SanitizeCatalogMetric(*article.Metrics)
		article.Metrics = &metric
	}
	if article.Assets != nil {
		assets := make([]ArticleAsset, len(article.Assets))
		for i, asset := range article.Assets {
			assets[i] = SanitizeCatalogAsset(asset)
		}
		article.Assets = assets
	}
	return article
}

func SanitizeCatalogAsset(asset ArticleAsset) ArticleAsset {
	asset.ArticleKey = strings.TrimSpace(asset.ArticleKey)
	asset.ResourceKey = strings.TrimSpace(asset.ResourceKey)
	asset.Kind = strings.TrimSpace(asset.Kind)
	asset.Role = strings.TrimSpace(asset.Role)
	asset.SourceURL = SanitizeArchiveMetadataURL(asset.SourceURL)
	asset.LocalPath = strings.TrimSpace(asset.LocalPath)
	asset.SHA256 = strings.TrimSpace(asset.SHA256)
	asset.Status = strings.TrimSpace(asset.Status)
	asset.Error = strings.TrimSpace(asset.Error)
	return asset
}

func SanitizeCatalogMetric(metric ArticleMetricSnapshot) ArticleMetricSnapshot {
	metric.ArticleKey = strings.TrimSpace(metric.ArticleKey)
	metric.Source = strings.TrimSpace(metric.Source)
	metric.RawMetadata = SanitizeCatalogRawMetadata(metric.RawMetadata)
	return metric
}

// SanitizeCatalogMetricState applies the transport boundary to the durable
// latest-attempt state. State errors are diagnostic text and must never be
// allowed to grow without bound through an import or API response.
func SanitizeCatalogMetricState(state ArticleMetricState) ArticleMetricState {
	state.ArticleKey = strings.TrimSpace(state.ArticleKey)
	state.Status = strings.ToLower(strings.TrimSpace(state.Status))
	state.LastSource = strings.TrimSpace(state.LastSource)
	state.LastError = SanitizeCatalogError(state.LastError)
	return state
}

func SanitizeCatalogError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 1024 {
		value = value[:1024]
	}
	return value
}

// SanitizeCatalogRawMetadata keeps structured diagnostic metadata while
// removing credential-bearing fields and URL query parameters. Invalid or
// oversized metadata is discarded rather than copied across a boundary.
func SanitizeCatalogRawMetadata(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxCatalogRawMetadataBytes {
		return ""
	}
	var value interface{}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return ""
	}
	value = sanitizeCatalogMetadataValue(value)
	data, err := json.Marshal(value)
	if err != nil || len(data) > maxCatalogRawMetadataBytes {
		return ""
	}
	return string(data)
}

func sanitizeCatalogMetadataValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		cleaned := make(map[string]interface{}, len(typed))
		for key, child := range typed {
			if isSensitiveCatalogMetadataKey(key) {
				continue
			}
			cleaned[key] = sanitizeCatalogMetadataValue(child)
		}
		return cleaned
	case []interface{}:
		cleaned := make([]interface{}, len(typed))
		for i, child := range typed {
			cleaned[i] = sanitizeCatalogMetadataValue(child)
		}
		return cleaned
	case string:
		return sanitizeCatalogMetadataString(typed)
	default:
		return value
	}
}

func isSensitiveCatalogMetadataKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	switch normalized {
	case "key", "uin", "pass_ticket", "appmsg_token", "wxtoken", "token", "cookie", "refresh_uri", "authorization":
		return true
	default:
		return strings.HasSuffix(normalized, "_token") || strings.HasSuffix(normalized, "_cookie")
	}
}

func sanitizeCatalogMetadataString(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "//") {
		return SanitizeArchiveMetadataURL(value)
	}
	return value
}

func articleMetadataForStorage(item ArticleItem) string {
	// Article content is not catalog metadata and can contain a full HTML body;
	// it is fetched into the archive path separately.
	item.Content = ""
	item.ContentURL = SanitizeArchiveMetadataURL(item.ContentURL)
	item.SourceURL = SanitizeArchiveMetadataURL(item.SourceURL)
	item.Cover = SanitizeArchiveMetadataURL(item.Cover)
	item.PlayURL = SanitizeCatalogMediaURL(item.PlayURL)
	data, err := json.Marshal(item)
	if err != nil || len(data) > maxCatalogRawMetadataBytes {
		return ""
	}
	return string(data)
}

package officialaccount

import (
	"context"
	"errors"
	"io"
)

const (
	CatalogExportFormatVersion = 1
	// CatalogLatestSchemaVersion is the newest database schema whose durable
	// catalog fields are understood by this build. Older exports remain
	// importable when their fields are still compatible.
	CatalogLatestSchemaVersion = 20
)

var ErrCatalogConflict = errors.New("catalog import conflict")

// CatalogExport is the portable, credential-free public-account catalog.
// Article bodies and binary files remain in the archive directory; this
// document carries their metadata and paths so the catalog can be rebuilt.
type CatalogExport struct {
	FormatVersion int                     `json:"format_version"`
	SchemaVersion int                     `json:"schema_version"`
	ExportedAt    string                  `json:"exported_at"`
	Accounts      []CatalogAccountRecord  `json:"accounts"`
	Articles      []ArticleRecord         `json:"articles"`
	Assets        []ArticleAsset          `json:"assets"`
	Metrics       []ArticleMetricSnapshot `json:"metrics"`
	MetricStates  []ArticleMetricState    `json:"metric_states"`
}

// CatalogAccountRecord contains only durable account metadata. It intentionally
// has no credential fields, even when the source account is currently usable.
type CatalogAccountRecord struct {
	Biz           string `json:"biz"`
	Nickname      string `json:"nickname,omitempty"`
	AvatarURL     string `json:"avatar_url,omitempty"`
	AuthorID      string `json:"author_id,omitempty"`
	Source        string `json:"source,omitempty"`
	IsEffective   bool   `json:"is_effective"`
	AccountError  string `json:"account_error,omitempty"`
	DiscoveredAt  int64  `json:"discovered_at,omitempty"`
	LastSeenAt    int64  `json:"last_seen_at,omitempty"`
	LastSyncAt    int64  `json:"last_sync_at,omitempty"`
	SyncStatus    string `json:"sync_status,omitempty"`
	SyncError     string `json:"sync_error,omitempty"`
	ArticleCount  int64  `json:"article_count,omitempty"`
	ArchivedCount int64  `json:"archived_count,omitempty"`
	CreatedAt     int64  `json:"created_at,omitempty"`
	UpdatedAt     int64  `json:"updated_at,omitempty"`
}

type CatalogExportStats struct {
	Accounts     int64 `json:"accounts"`
	Articles     int64 `json:"articles"`
	Assets       int64 `json:"assets"`
	Metrics      int64 `json:"metrics"`
	MetricStates int64 `json:"metric_states"`
}

const (
	CatalogConflictMerge = "merge"
	CatalogConflictSkip  = "skip"
	CatalogConflictError = "error"
)

type CatalogImportOptions struct {
	DryRun         bool
	ConflictPolicy string
}

type CatalogImportSummary struct {
	DryRun              bool     `json:"dry_run"`
	ConflictPolicy      string   `json:"conflict_policy"`
	AccountsSeen        int      `json:"accounts_seen"`
	AccountsAdded       int      `json:"accounts_added"`
	AccountsUpdated     int      `json:"accounts_updated"`
	AccountsSkipped     int      `json:"accounts_skipped"`
	ArticlesSeen        int      `json:"articles_seen"`
	ArticlesAdded       int      `json:"articles_added"`
	ArticlesUpdated     int      `json:"articles_updated"`
	ArticlesSkipped     int      `json:"articles_skipped"`
	AssetsSeen          int      `json:"assets_seen"`
	AssetsAdded         int      `json:"assets_added"`
	AssetsUpdated       int      `json:"assets_updated"`
	AssetsSkipped       int      `json:"assets_skipped"`
	MetricsSeen         int      `json:"metrics_seen"`
	MetricsAdded        int      `json:"metrics_added"`
	MetricsSkipped      int      `json:"metrics_skipped"`
	MetricStatesSeen    int      `json:"metric_states_seen"`
	MetricStatesAdded   int      `json:"metric_states_added"`
	MetricStatesUpdated int      `json:"metric_states_updated"`
	MetricStatesSkipped int      `json:"metric_states_skipped"`
	Conflicts           int      `json:"conflicts"`
	Warnings            []string `json:"warnings,omitempty"`
}

// CatalogTransferRepository is optional so legacy in-memory test doubles and
// integrations can keep the basic catalog contract without implementing file
// transfer. The SQLite repository is the production implementation.
type CatalogTransferRepository interface {
	CatalogRepository
	ExportCatalog(context.Context, io.Writer) (CatalogExportStats, error)
	ImportCatalog(context.Context, CatalogExport, CatalogImportOptions) (CatalogImportSummary, error)
}

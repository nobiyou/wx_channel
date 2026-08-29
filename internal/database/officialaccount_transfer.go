package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"wx_channel/internal/officialaccount"
)

const maxCatalogImportItems = 1_000_000

// ExportCatalog streams a versioned credential-free catalog document. Keeping
// each table as a separate JSON array avoids building a second copy of a
// potentially large article library in memory.
func (r *OfficialAccountRepository) ExportCatalog(ctx context.Context, writer io.Writer) (officialaccount.CatalogExportStats, error) {
	var stats officialaccount.CatalogExportStats
	if err := r.ready(); err != nil {
		return stats, err
	}
	if writer == nil {
		return stats, errors.New("catalog export writer is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var schemaVersion int
	if err := r.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&schemaVersion); err != nil {
		return stats, fmt.Errorf("read catalog schema version: %w", err)
	}
	if err := writeCatalogObjectPrefix(writer, schemaVersion, r.now().UTC().Format(time.RFC3339)); err != nil {
		return stats, err
	}

	if err := streamCatalogAccounts(ctx, r.db, writer, &stats); err != nil {
		return stats, err
	}
	if _, err := io.WriteString(writer, "],\"articles\":["); err != nil {
		return stats, err
	}
	if err := streamCatalogArticles(ctx, r.db, writer, &stats); err != nil {
		return stats, err
	}
	if _, err := io.WriteString(writer, "],\"assets\":["); err != nil {
		return stats, err
	}
	if err := streamCatalogAssets(ctx, r.db, writer, &stats); err != nil {
		return stats, err
	}
	if _, err := io.WriteString(writer, "],\"metrics\":["); err != nil {
		return stats, err
	}
	if err := streamCatalogMetrics(ctx, r.db, writer, &stats); err != nil {
		return stats, err
	}
	if _, err := io.WriteString(writer, "],\"metric_states\":["); err != nil {
		return stats, err
	}
	if err := streamCatalogMetricStates(ctx, r.db, writer, &stats); err != nil {
		return stats, err
	}
	if _, err := io.WriteString(writer, "]}"); err != nil {
		return stats, err
	}
	return stats, nil
}

func writeCatalogObjectPrefix(writer io.Writer, schemaVersion int, exportedAt string) error {
	if _, err := io.WriteString(writer, "{\"format_version\":"); err != nil {
		return err
	}
	if err := writeCatalogJSONValue(writer, officialaccount.CatalogExportFormatVersion); err != nil {
		return err
	}
	if _, err := io.WriteString(writer, ",\"schema_version\":"); err != nil {
		return err
	}
	if err := writeCatalogJSONValue(writer, schemaVersion); err != nil {
		return err
	}
	if _, err := io.WriteString(writer, ",\"exported_at\":"); err != nil {
		return err
	}
	if err := writeCatalogJSONValue(writer, exportedAt); err != nil {
		return err
	}
	if _, err := io.WriteString(writer, ",\"accounts\":["); err != nil {
		return err
	}
	return nil
}

func writeCatalogJSONValue(writer io.Writer, value interface{}) error {
	encoder := json.NewEncoder(writer)
	return encoder.Encode(value)
}

type catalogJSONArrayWriter struct {
	writer io.Writer
	first  bool
}

func (w *catalogJSONArrayWriter) Write(value interface{}) error {
	if !w.first {
		if _, err := io.WriteString(w.writer, ","); err != nil {
			return err
		}
	}
	w.first = false
	return writeCatalogJSONValue(w.writer, value)
}

func streamCatalogAccounts(ctx context.Context, db *sql.DB, writer io.Writer, stats *officialaccount.CatalogExportStats) error {
	rows, err := db.QueryContext(ctx, `
SELECT biz, nickname, avatar_url, author_id, source, is_effective,
       account_error, discovered_at, last_seen_at, last_sync_at,
       sync_status, sync_error, article_count, archived_count, created_at, updated_at
FROM mp_accounts ORDER BY biz ASC`)
	if err != nil {
		return fmt.Errorf("export official accounts: %w", err)
	}
	defer rows.Close()
	array := &catalogJSONArrayWriter{writer: writer, first: true}
	for rows.Next() {
		var item officialaccount.CatalogAccountRecord
		var effective int
		var lastSync sql.NullInt64
		if err := rows.Scan(&item.Biz, &item.Nickname, &item.AvatarURL, &item.AuthorID,
			&item.Source, &effective, &item.AccountError, &item.DiscoveredAt, &item.LastSeenAt,
			&lastSync, &item.SyncStatus, &item.SyncError, &item.ArticleCount,
			&item.ArchivedCount, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return fmt.Errorf("scan account for export: %w", err)
		}
		// A catalog export never transfers credentials, so an exported account
		// must not claim to have a usable session after import.
		item.IsEffective = false
		if lastSync.Valid {
			item.LastSyncAt = lastSync.Int64
		}
		item.AccountError = sanitizeCatalogError(item.AccountError)
		if err := array.Write(item); err != nil {
			return fmt.Errorf("write account export: %w", err)
		}
		stats.Accounts++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate account export: %w", err)
	}
	return nil
}

func streamCatalogArticles(ctx context.Context, db *sql.DB, writer io.Writer, stats *officialaccount.CatalogExportStats) error {
	rows, err := db.QueryContext(ctx, `
SELECT article_key, biz, mid, idx, file_id, title, digest, author,
       video_id,
       content_url, source_url, cover_url, publish_time, is_multi, is_original,
       is_paid, is_pay_subscribe, item_show_type, subtype, copyright_stat, duration,
       audio_fileid, play_url, malicious_title_reason_id, malicious_content_type,
       source_deleted, first_seen_at, last_seen_at, archive_status, archive_dir,
       archive_html, archive_manifest, archived_at, raw_metadata
FROM mp_articles ORDER BY biz ASC, publish_time DESC, article_key ASC`)
	if err != nil {
		return fmt.Errorf("export official articles: %w", err)
	}
	defer rows.Close()
	array := &catalogJSONArrayWriter{writer: writer, first: true}
	for rows.Next() {
		item, err := scanArticleRecord(rows)
		if err != nil {
			return fmt.Errorf("scan article for export: %w", err)
		}
		item.Assets = nil
		item.Metrics = nil
		if err := array.Write(item); err != nil {
			return fmt.Errorf("write article export: %w", err)
		}
		stats.Articles++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate article export: %w", err)
	}
	return nil
}

func streamCatalogAssets(ctx context.Context, db *sql.DB, writer io.Writer, stats *officialaccount.CatalogExportStats) error {
	rows, err := db.QueryContext(ctx, `
SELECT article_key, resource_key, kind, role, source_url, local_path, sha256,
       size, status, error, created_at, updated_at
FROM mp_article_assets ORDER BY article_key ASC, resource_key ASC`)
	if err != nil {
		return fmt.Errorf("export article assets: %w", err)
	}
	defer rows.Close()
	array := &catalogJSONArrayWriter{writer: writer, first: true}
	for rows.Next() {
		item, err := scanArticleAsset(rows)
		if err != nil {
			return fmt.Errorf("scan asset for export: %w", err)
		}
		if err := array.Write(item); err != nil {
			return fmt.Errorf("write asset export: %w", err)
		}
		stats.Assets++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate asset export: %w", err)
	}
	return nil
}

func streamCatalogMetrics(ctx context.Context, db *sql.DB, writer io.Writer, stats *officialaccount.CatalogExportStats) error {
	rows, err := db.QueryContext(ctx, `
SELECT id, article_key, observed_at, source, view_count, like_count,
       comment_count, share_count, collect_count, reward_count, raw_metadata
FROM mp_article_metric_snapshots ORDER BY article_key ASC, observed_at ASC, id ASC`)
	if err != nil {
		return fmt.Errorf("export article metrics: %w", err)
	}
	defer rows.Close()
	array := &catalogJSONArrayWriter{writer: writer, first: true}
	for rows.Next() {
		item, err := scanMetricSnapshot(rows)
		if err != nil {
			return fmt.Errorf("scan metric for export: %w", err)
		}
		if err := array.Write(officialaccount.SanitizeCatalogMetric(item)); err != nil {
			return fmt.Errorf("write metric export: %w", err)
		}
		stats.Metrics++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate metric export: %w", err)
	}
	return nil
}

func streamCatalogMetricStates(ctx context.Context, db *sql.DB, writer io.Writer, stats *officialaccount.CatalogExportStats) error {
	rows, err := db.QueryContext(ctx, `
SELECT article_key, status, attempt_count, success_count, unknown_count,
       failure_count, last_attempt_at, last_success_at, last_observed_at,
       next_retry_at, last_source, last_error, updated_at
FROM mp_article_metric_states ORDER BY article_key ASC`)
	if err != nil {
		return fmt.Errorf("export article metric states: %w", err)
	}
	defer rows.Close()
	array := &catalogJSONArrayWriter{writer: writer, first: true}
	for rows.Next() {
		state, err := scanArticleMetricState(rows)
		if err != nil {
			return fmt.Errorf("scan metric state for export: %w", err)
		}
		if err := array.Write(officialaccount.SanitizeCatalogMetricState(state)); err != nil {
			return fmt.Errorf("write metric state export: %w", err)
		}
		stats.MetricStates++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate metric state export: %w", err)
	}
	return nil
}

func sanitizeCatalogError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 1024 {
		value = value[:1024]
	}
	return value
}

func (r *OfficialAccountRepository) ImportCatalog(ctx context.Context, document officialaccount.CatalogExport, options officialaccount.CatalogImportOptions) (officialaccount.CatalogImportSummary, error) {
	var summary officialaccount.CatalogImportSummary
	if err := r.ready(); err != nil {
		return summary, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	options.ConflictPolicy = normalizeCatalogConflictPolicy(options.ConflictPolicy)
	summary.DryRun = options.DryRun
	summary.ConflictPolicy = options.ConflictPolicy
	normalizeCatalogDocument(&document)
	if err := validateCatalogDocument(document); err != nil {
		return summary, err
	}

	accountKeys, err := loadCatalogKeySet(ctx, r.db, "mp_accounts", "biz")
	if err != nil {
		return summary, err
	}
	articleKeys, err := loadCatalogKeySet(ctx, r.db, "mp_articles", "article_key")
	if err != nil {
		return summary, err
	}
	articleOwners, err := loadCatalogArticleOwners(ctx, r.db)
	if err != nil {
		return summary, err
	}
	assetKeys, err := loadCatalogAssetKeySet(ctx, r.db)
	if err != nil {
		return summary, err
	}
	metricKeys, err := loadCatalogMetricKeySet(ctx, r.db)
	if err != nil {
		return summary, err
	}
	metricStateKeys, err := loadCatalogKeySet(ctx, r.db, "mp_article_metric_states", "article_key")
	if err != nil {
		return summary, err
	}
	if err := validateCatalogReferences(document, articleOwners); err != nil {
		return summary, err
	}

	if err := assessCatalogConflicts(document, accountKeys, articleKeys, assetKeys, metricKeys, metricStateKeys, options.ConflictPolicy, &summary); err != nil {
		return summary, err
	}
	if options.DryRun {
		populateCatalogDryRunCounts(document, accountKeys, articleKeys, assetKeys, metricKeys, metricStateKeys, &summary)
		return summary, nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return summary, fmt.Errorf("begin catalog import: %w", err)
	}
	rollback := func(err error) (officialaccount.CatalogImportSummary, error) {
		_ = tx.Rollback()
		return summary, err
	}

	importedAccounts := make(map[string]struct{}, len(document.Accounts))
	for _, account := range document.Accounts {
		if _, exists := accountKeys[account.Biz]; exists && options.ConflictPolicy == officialaccount.CatalogConflictSkip {
			summary.AccountsSkipped++
			continue
		}
		if err := importCatalogAccount(ctx, tx, account); err != nil {
			return rollback(err)
		}
		importedAccounts[account.Biz] = struct{}{}
		if _, exists := accountKeys[account.Biz]; exists {
			summary.AccountsUpdated++
		} else {
			summary.AccountsAdded++
		}
	}

	articleActions := make(map[string]bool, len(document.Articles))
	for _, article := range document.Articles {
		if _, exists := articleKeys[article.Key]; exists && options.ConflictPolicy == officialaccount.CatalogConflictSkip {
			summary.ArticlesSkipped++
			articleActions[article.Key] = false
			continue
		}
		if _, exists := importedAccounts[article.Biz]; !exists {
			if _, exists := accountKeys[article.Biz]; !exists {
				if err := ensureImportedAccount(ctx, tx, article.Biz); err != nil {
					return rollback(err)
				}
				summary.Warnings = append(summary.Warnings, "article imported with metadata-only account: "+article.Biz)
			}
		}
		if err := importCatalogArticle(ctx, tx, article); err != nil {
			return rollback(err)
		}
		articleActions[article.Key] = true
		if _, exists := articleKeys[article.Key]; exists {
			summary.ArticlesUpdated++
		} else {
			summary.ArticlesAdded++
		}
	}

	for _, asset := range document.Assets {
		if allowed, exists := articleActions[asset.ArticleKey]; exists && !allowed {
			summary.AssetsSkipped++
			continue
		}
		key := asset.ArticleKey + "\x00" + asset.ResourceKey
		if _, exists := assetKeys[key]; exists && options.ConflictPolicy == officialaccount.CatalogConflictSkip {
			summary.AssetsSkipped++
			continue
		}
		if err := importCatalogAsset(ctx, tx, asset); err != nil {
			return rollback(err)
		}
		if _, exists := assetKeys[key]; exists {
			summary.AssetsUpdated++
		} else {
			summary.AssetsAdded++
		}
	}

	for _, metric := range document.Metrics {
		inserted, err := insertMetricSnapshotTx(ctx, tx, metric)
		if err != nil {
			return rollback(err)
		}
		if inserted {
			summary.MetricsAdded++
		} else {
			summary.MetricsSkipped++
		}
	}

	for _, state := range document.MetricStates {
		if _, exists := metricStateKeys[state.ArticleKey]; exists && options.ConflictPolicy == officialaccount.CatalogConflictSkip {
			summary.MetricStatesSkipped++
			continue
		}
		changed, err := importCatalogMetricState(ctx, tx, state)
		if err != nil {
			return rollback(err)
		}
		if changed {
			if _, exists := metricStateKeys[state.ArticleKey]; exists {
				summary.MetricStatesUpdated++
			} else {
				summary.MetricStatesAdded++
			}
		} else {
			summary.MetricStatesSkipped++
		}
	}

	if err := refreshCatalogCounts(ctx, tx, document); err != nil {
		return rollback(err)
	}
	if err := tx.Commit(); err != nil {
		return summary, fmt.Errorf("commit catalog import: %w", err)
	}
	return summary, nil
}

func validateCatalogDocument(document officialaccount.CatalogExport) error {
	if document.FormatVersion != officialaccount.CatalogExportFormatVersion {
		return fmt.Errorf("unsupported catalog format version %d", document.FormatVersion)
	}
	if document.SchemaVersion < 0 || document.SchemaVersion > officialaccount.CatalogLatestSchemaVersion {
		return fmt.Errorf("unsupported catalog schema version %d", document.SchemaVersion)
	}
	if len(document.Accounts) > maxCatalogImportItems || len(document.Articles) > maxCatalogImportItems ||
		len(document.Assets) > maxCatalogImportItems || len(document.Metrics) > maxCatalogImportItems ||
		len(document.MetricStates) > maxCatalogImportItems {
		return fmt.Errorf("catalog import exceeds the %d item limit", maxCatalogImportItems)
	}
	accountKeys := make(map[string]struct{}, len(document.Accounts))
	articleKeys := make(map[string]struct{}, len(document.Articles))
	assetKeys := make(map[string]struct{}, len(document.Assets))
	metricStateKeys := make(map[string]struct{}, len(document.MetricStates))
	for _, account := range document.Accounts {
		if strings.TrimSpace(account.Biz) == "" {
			return errors.New("catalog account biz is required")
		}
		if _, exists := accountKeys[account.Biz]; exists {
			return fmt.Errorf("duplicate catalog account %q", account.Biz)
		}
		accountKeys[account.Biz] = struct{}{}
	}
	for _, article := range document.Articles {
		if strings.TrimSpace(article.Key) == "" || strings.TrimSpace(article.Biz) == "" {
			return errors.New("catalog article key and biz are required")
		}
		if _, exists := articleKeys[article.Key]; exists {
			return fmt.Errorf("duplicate catalog article %q", article.Key)
		}
		articleKeys[article.Key] = struct{}{}
		if !validCatalogPath(article.ArchiveDir) || !validCatalogPath(article.ArchiveHTML) || !validCatalogPath(article.ArchiveManifest) {
			return fmt.Errorf("unsafe archive path in catalog article %q", article.Key)
		}
	}
	for _, asset := range document.Assets {
		if strings.TrimSpace(asset.ArticleKey) == "" || strings.TrimSpace(asset.ResourceKey) == "" {
			return errors.New("catalog asset article_key and resource_key are required")
		}
		key := asset.ArticleKey + "\x00" + asset.ResourceKey
		if _, exists := assetKeys[key]; exists {
			return fmt.Errorf("duplicate catalog asset %q/%q", asset.ArticleKey, asset.ResourceKey)
		}
		assetKeys[key] = struct{}{}
		if !validCatalogPath(asset.LocalPath) {
			return fmt.Errorf("unsafe local path in catalog asset %q/%q", asset.ArticleKey, asset.ResourceKey)
		}
	}
	for _, metric := range document.Metrics {
		if strings.TrimSpace(metric.ArticleKey) == "" {
			return errors.New("catalog metric article_key is required")
		}
		if metric.ObservedAt <= 0 {
			return fmt.Errorf("invalid metric observation time for %q", metric.ArticleKey)
		}
	}
	for _, state := range document.MetricStates {
		if strings.TrimSpace(state.ArticleKey) == "" {
			return errors.New("catalog metric state article_key is required")
		}
		if _, exists := metricStateKeys[state.ArticleKey]; exists {
			return fmt.Errorf("duplicate catalog metric state %q", state.ArticleKey)
		}
		metricStateKeys[state.ArticleKey] = struct{}{}
		if state.Status == "" {
			return fmt.Errorf("catalog metric state %q has no status", state.ArticleKey)
		}
		if state.Status != officialaccount.MetricStatePending && state.Status != officialaccount.MetricStateSuccess && state.Status != officialaccount.MetricStateUnknown && state.Status != officialaccount.MetricStateFailed {
			return fmt.Errorf("catalog metric state %q has invalid status %q", state.ArticleKey, state.Status)
		}
		if state.AttemptCount < 0 || state.SuccessCount < 0 || state.UnknownCount < 0 || state.FailureCount < 0 {
			return fmt.Errorf("catalog metric state %q has negative counters", state.ArticleKey)
		}
		if state.LastAttemptAt < 0 || state.LastSuccessAt < 0 || state.LastObservedAt < 0 || state.NextRetryAt < 0 || state.UpdatedAt < 0 {
			return fmt.Errorf("catalog metric state %q has invalid timestamp", state.ArticleKey)
		}
	}
	return nil
}

func validateCatalogReferences(document officialaccount.CatalogExport, existingArticleOwners map[string]string) error {
	articleKeys := make(map[string]struct{}, len(document.Articles)+len(existingArticleOwners))
	for key := range existingArticleOwners {
		articleKeys[key] = struct{}{}
	}
	for _, article := range document.Articles {
		if owner, exists := existingArticleOwners[article.Key]; exists && owner != article.Biz {
			return fmt.Errorf("catalog article %q belongs to biz %q, cannot import as %q", article.Key, owner, article.Biz)
		}
		articleKeys[article.Key] = struct{}{}
	}
	for _, asset := range document.Assets {
		if _, exists := articleKeys[asset.ArticleKey]; !exists {
			return fmt.Errorf("catalog asset %q references unknown article %q", asset.ResourceKey, asset.ArticleKey)
		}
	}
	for _, metric := range document.Metrics {
		if _, exists := articleKeys[metric.ArticleKey]; !exists {
			return fmt.Errorf("catalog metric references unknown article %q", metric.ArticleKey)
		}
	}
	for _, state := range document.MetricStates {
		if _, exists := articleKeys[state.ArticleKey]; !exists {
			return fmt.Errorf("catalog metric state references unknown article %q", state.ArticleKey)
		}
	}
	return nil
}

func normalizeCatalogDocument(document *officialaccount.CatalogExport) {
	if document == nil {
		return
	}
	for i := range document.Accounts {
		account := &document.Accounts[i]
		account.Biz = strings.TrimSpace(account.Biz)
		account.Nickname = strings.TrimSpace(account.Nickname)
		account.AvatarURL = officialaccount.SanitizeArchiveMetadataURL(account.AvatarURL)
		account.AuthorID = strings.TrimSpace(account.AuthorID)
		account.Source = strings.TrimSpace(account.Source)
		account.AccountError = sanitizeCatalogError(account.AccountError)
		// Credentials are intentionally not part of the wire type; imported
		// metadata is always marked unusable until a page refresh captures them.
		account.IsEffective = false
	}
	for i := range document.Articles {
		article := officialaccount.SanitizeCatalogArticle(document.Articles[i])
		article.Key = strings.TrimSpace(article.Key)
		article.Biz = strings.TrimSpace(article.Biz)
		article.ArchiveDir = strings.TrimSpace(strings.ReplaceAll(article.ArchiveDir, "\\", "/"))
		article.ArchiveHTML = strings.TrimSpace(strings.ReplaceAll(article.ArchiveHTML, "\\", "/"))
		article.ArchiveManifest = strings.TrimSpace(strings.ReplaceAll(article.ArchiveManifest, "\\", "/"))
		article.Assets = nil
		article.Metrics = nil
		document.Articles[i] = article
	}
	for i := range document.Assets {
		asset := officialaccount.SanitizeCatalogAsset(document.Assets[i])
		asset.LocalPath = strings.TrimSpace(strings.ReplaceAll(asset.LocalPath, "\\", "/"))
		document.Assets[i] = asset
	}
	for i := range document.Metrics {
		document.Metrics[i] = officialaccount.SanitizeCatalogMetric(document.Metrics[i])
	}
	for i := range document.MetricStates {
		document.MetricStates[i] = officialaccount.SanitizeCatalogMetricState(document.MetricStates[i])
	}
}

func normalizeCatalogConflictPolicy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case officialaccount.CatalogConflictSkip:
		return officialaccount.CatalogConflictSkip
	case officialaccount.CatalogConflictError:
		return officialaccount.CatalogConflictError
	default:
		return officialaccount.CatalogConflictMerge
	}
}

func loadCatalogKeySet(ctx context.Context, db *sql.DB, table, column string) (map[string]struct{}, error) {
	rows, err := db.QueryContext(ctx, "SELECT "+column+" FROM "+table)
	if err != nil {
		return nil, fmt.Errorf("load existing %s keys: %w", table, err)
	}
	defer rows.Close()
	result := make(map[string]struct{})
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		result[key] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func loadCatalogArticleOwners(ctx context.Context, db *sql.DB) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, "SELECT article_key, biz FROM mp_articles")
	if err != nil {
		return nil, fmt.Errorf("load existing article owners: %w", err)
	}
	defer rows.Close()
	result := make(map[string]string)
	for rows.Next() {
		var articleKey, biz string
		if err := rows.Scan(&articleKey, &biz); err != nil {
			return nil, fmt.Errorf("scan existing article owner: %w", err)
		}
		result[articleKey] = biz
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate existing article owners: %w", err)
	}
	return result, nil
}

func loadCatalogAssetKeySet(ctx context.Context, db *sql.DB) (map[string]struct{}, error) {
	rows, err := db.QueryContext(ctx, "SELECT article_key, resource_key FROM mp_article_assets")
	if err != nil {
		return nil, fmt.Errorf("load existing asset keys: %w", err)
	}
	defer rows.Close()
	result := make(map[string]struct{})
	for rows.Next() {
		var articleKey, resourceKey string
		if err := rows.Scan(&articleKey, &resourceKey); err != nil {
			return nil, err
		}
		result[articleKey+"\x00"+resourceKey] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func loadCatalogMetricKeySet(ctx context.Context, db *sql.DB) (map[string]struct{}, error) {
	rows, err := db.QueryContext(ctx, `
SELECT article_key, observed_at, source, view_count, like_count,
       comment_count, share_count, collect_count, reward_count
FROM mp_article_metric_snapshots`)
	if err != nil {
		return nil, fmt.Errorf("load existing metric keys: %w", err)
	}
	defer rows.Close()
	result := make(map[string]struct{})
	for rows.Next() {
		var metric officialaccount.ArticleMetricSnapshot
		var view, like, comment, share, collect, reward sql.NullInt64
		if err := rows.Scan(&metric.ArticleKey, &metric.ObservedAt, &metric.Source, &view, &like, &comment, &share, &collect, &reward); err != nil {
			return nil, fmt.Errorf("scan existing metric key: %w", err)
		}
		metric.ViewCount = nullableInt64Pointer(view)
		metric.LikeCount = nullableInt64Pointer(like)
		metric.CommentCount = nullableInt64Pointer(comment)
		metric.ShareCount = nullableInt64Pointer(share)
		metric.CollectCount = nullableInt64Pointer(collect)
		metric.RewardCount = nullableInt64Pointer(reward)
		result[metricImportKey(metric)] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate existing metric keys: %w", err)
	}
	return result, nil
}

func metricImportKey(metric officialaccount.ArticleMetricSnapshot) string {
	metric = officialaccount.SanitizeCatalogMetric(metric)
	observedAt := metric.ObservedAt
	if observedAt == 0 {
		observedAt = 1
	}
	source := strings.TrimSpace(metric.Source)
	if source == "" {
		source = "import"
	}
	return metricSnapshotHash(metric, observedAt, source)
}

func assessCatalogConflicts(document officialaccount.CatalogExport, accountKeys, articleKeys, assetKeys, metricKeys, metricStateKeys map[string]struct{}, policy string, summary *officialaccount.CatalogImportSummary) error {
	summary.AccountsSeen = len(document.Accounts)
	summary.ArticlesSeen = len(document.Articles)
	summary.AssetsSeen = len(document.Assets)
	summary.MetricsSeen = len(document.Metrics)
	summary.MetricStatesSeen = len(document.MetricStates)
	for _, account := range document.Accounts {
		if _, exists := accountKeys[account.Biz]; exists {
			summary.Conflicts++
		}
	}
	for _, article := range document.Articles {
		if _, exists := articleKeys[article.Key]; exists {
			summary.Conflicts++
		}
	}
	for _, asset := range document.Assets {
		if _, exists := assetKeys[asset.ArticleKey+"\x00"+asset.ResourceKey]; exists {
			summary.Conflicts++
		}
	}
	seenMetrics := make(map[string]struct{}, len(document.Metrics))
	for _, metric := range document.Metrics {
		key := metricImportKey(metric)
		if _, exists := metricKeys[key]; exists {
			summary.Conflicts++
		}
		if _, exists := seenMetrics[key]; exists {
			summary.Conflicts++
		}
		seenMetrics[key] = struct{}{}
	}
	for _, state := range document.MetricStates {
		if _, exists := metricStateKeys[state.ArticleKey]; exists {
			summary.Conflicts++
		}
	}
	if policy == officialaccount.CatalogConflictError && summary.Conflicts > 0 {
		return fmt.Errorf("%w: catalog import found %d conflicts", officialaccount.ErrCatalogConflict, summary.Conflicts)
	}
	return nil
}

func populateCatalogDryRunCounts(document officialaccount.CatalogExport, accountKeys, articleKeys, assetKeys, metricKeys, metricStateKeys map[string]struct{}, summary *officialaccount.CatalogImportSummary) {
	for _, account := range document.Accounts {
		if _, exists := accountKeys[account.Biz]; exists {
			switch summary.ConflictPolicy {
			case officialaccount.CatalogConflictSkip:
				summary.AccountsSkipped++
			default:
				summary.AccountsUpdated++
			}
		} else {
			summary.AccountsAdded++
		}
	}
	for _, article := range document.Articles {
		if _, exists := articleKeys[article.Key]; exists {
			switch summary.ConflictPolicy {
			case officialaccount.CatalogConflictSkip:
				summary.ArticlesSkipped++
			default:
				summary.ArticlesUpdated++
			}
		} else {
			summary.ArticlesAdded++
		}
	}
	for _, asset := range document.Assets {
		if _, exists := assetKeys[asset.ArticleKey+"\x00"+asset.ResourceKey]; exists {
			switch summary.ConflictPolicy {
			case officialaccount.CatalogConflictSkip:
				summary.AssetsSkipped++
			default:
				summary.AssetsUpdated++
			}
		} else {
			summary.AssetsAdded++
		}
	}
	seenMetrics := make(map[string]struct{}, len(document.Metrics))
	for _, metric := range document.Metrics {
		key := metricImportKey(metric)
		if _, exists := metricKeys[key]; exists {
			summary.MetricsSkipped++
		} else if _, exists := seenMetrics[key]; exists {
			summary.MetricsSkipped++
		} else {
			summary.MetricsAdded++
		}
		seenMetrics[key] = struct{}{}
	}
	for _, state := range document.MetricStates {
		if _, exists := metricStateKeys[state.ArticleKey]; exists {
			if summary.ConflictPolicy == officialaccount.CatalogConflictSkip {
				summary.MetricStatesSkipped++
			} else {
				summary.MetricStatesUpdated++
			}
		} else {
			summary.MetricStatesAdded++
		}
	}
}

func importCatalogAccount(ctx context.Context, tx *sql.Tx, account officialaccount.CatalogAccountRecord) error {
	now := time.Now().Unix()
	_, err := tx.ExecContext(ctx, `
INSERT INTO mp_accounts (
    biz, nickname, avatar_url, author_id, source, is_effective, account_error,
    discovered_at, last_seen_at, last_sync_at, sync_status, sync_error,
    article_count, archived_count, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, 0, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(biz) DO UPDATE SET
    nickname = CASE WHEN excluded.nickname <> '' THEN excluded.nickname ELSE mp_accounts.nickname END,
    avatar_url = CASE WHEN excluded.avatar_url <> '' THEN excluded.avatar_url ELSE mp_accounts.avatar_url END,
    author_id = CASE WHEN excluded.author_id <> '' THEN excluded.author_id ELSE mp_accounts.author_id END,
    source = CASE WHEN excluded.source <> '' THEN excluded.source ELSE mp_accounts.source END,
    is_effective = 0,
    account_error = excluded.account_error,
    discovered_at = CASE WHEN excluded.discovered_at <> 0 THEN excluded.discovered_at ELSE mp_accounts.discovered_at END,
    last_seen_at = CASE WHEN excluded.last_seen_at <> 0 THEN excluded.last_seen_at ELSE mp_accounts.last_seen_at END,
    updated_at = ?`, account.Biz, account.Nickname, account.AvatarURL, account.AuthorID,
		account.Source, account.AccountError, catalogTimeOrNow(account.DiscoveredAt, now),
		catalogTimeOrNow(account.LastSeenAt, now), nullableUnix(account.LastSyncAt), account.SyncStatus,
		account.SyncError, account.ArticleCount, account.ArchivedCount,
		catalogTimeOrNow(account.CreatedAt, now), catalogTimeOrNow(account.UpdatedAt, now), now)
	if err != nil {
		return fmt.Errorf("import account %q: %w", account.Biz, err)
	}
	return nil
}

func ensureImportedAccount(ctx context.Context, tx *sql.Tx, biz string) error {
	_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO mp_accounts (biz, source, is_effective) VALUES (?, 'catalog_import', 0)`, biz)
	if err != nil {
		return fmt.Errorf("ensure imported account %q: %w", biz, err)
	}
	return nil
}

func importCatalogArticle(ctx context.Context, tx *sql.Tx, article officialaccount.ArticleRecord) error {
	now := time.Now().Unix()
	_, err := tx.ExecContext(ctx, `
INSERT INTO mp_articles (
    article_key, biz, mid, idx, file_id, video_id, title, digest, author,
    content_url, source_url, cover_url, publish_time, is_multi, is_original,
    is_paid, is_pay_subscribe, item_show_type, subtype, copyright_stat, duration,
    audio_fileid, play_url, malicious_title_reason_id, malicious_content_type,
    source_deleted, first_seen_at, last_seen_at, archive_status, archive_dir,
    archive_html, archive_manifest, archived_at, raw_metadata, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(article_key) DO UPDATE SET
    biz = excluded.biz,
    mid = CASE WHEN excluded.mid <> '' THEN excluded.mid ELSE mp_articles.mid END,
    idx = CASE WHEN excluded.idx <> 0 THEN excluded.idx ELSE mp_articles.idx END,
    file_id = CASE WHEN excluded.file_id <> 0 THEN excluded.file_id ELSE mp_articles.file_id END,
    video_id = CASE WHEN excluded.video_id <> '' THEN excluded.video_id ELSE mp_articles.video_id END,
    title = CASE WHEN excluded.title <> '' THEN excluded.title ELSE mp_articles.title END,
    digest = CASE WHEN excluded.digest <> '' THEN excluded.digest ELSE mp_articles.digest END,
    author = CASE WHEN excluded.author <> '' THEN excluded.author ELSE mp_articles.author END,
    content_url = CASE WHEN excluded.content_url <> '' THEN excluded.content_url ELSE mp_articles.content_url END,
    source_url = CASE WHEN excluded.source_url <> '' THEN excluded.source_url ELSE mp_articles.source_url END,
    cover_url = CASE WHEN excluded.cover_url <> '' THEN excluded.cover_url ELSE mp_articles.cover_url END,
    publish_time = COALESCE(excluded.publish_time, mp_articles.publish_time),
    is_multi = CASE WHEN excluded.is_multi <> 0 THEN excluded.is_multi ELSE mp_articles.is_multi END,
    is_original = CASE WHEN excluded.is_original <> 0 THEN excluded.is_original ELSE mp_articles.is_original END,
    is_paid = CASE WHEN excluded.is_paid <> 0 THEN excluded.is_paid ELSE mp_articles.is_paid END,
    is_pay_subscribe = CASE WHEN excluded.is_pay_subscribe <> 0 THEN excluded.is_pay_subscribe ELSE mp_articles.is_pay_subscribe END,
    item_show_type = CASE WHEN excluded.item_show_type <> 0 THEN excluded.item_show_type ELSE mp_articles.item_show_type END,
    subtype = CASE WHEN excluded.subtype <> 0 THEN excluded.subtype ELSE mp_articles.subtype END,
    copyright_stat = CASE WHEN excluded.copyright_stat <> 0 THEN excluded.copyright_stat ELSE mp_articles.copyright_stat END,
    duration = CASE WHEN excluded.duration <> 0 THEN excluded.duration ELSE mp_articles.duration END,
    audio_fileid = CASE WHEN excluded.audio_fileid <> 0 THEN excluded.audio_fileid ELSE mp_articles.audio_fileid END,
    play_url = CASE WHEN excluded.play_url <> '' THEN excluded.play_url ELSE mp_articles.play_url END,
    malicious_title_reason_id = CASE WHEN excluded.malicious_title_reason_id <> 0 THEN excluded.malicious_title_reason_id ELSE mp_articles.malicious_title_reason_id END,
    malicious_content_type = CASE WHEN excluded.malicious_content_type <> 0 THEN excluded.malicious_content_type ELSE mp_articles.malicious_content_type END,
    source_deleted = excluded.source_deleted,
    first_seen_at = CASE WHEN mp_articles.first_seen_at = 0 THEN excluded.first_seen_at ELSE mp_articles.first_seen_at END,
    last_seen_at = CASE WHEN excluded.last_seen_at <> 0 THEN excluded.last_seen_at ELSE mp_articles.last_seen_at END,
    archive_status = CASE WHEN mp_articles.archive_status = 'not_archived' THEN excluded.archive_status ELSE mp_articles.archive_status END,
    archive_dir = CASE WHEN excluded.archive_dir <> '' THEN excluded.archive_dir ELSE mp_articles.archive_dir END,
    archive_html = CASE WHEN excluded.archive_html <> '' THEN excluded.archive_html ELSE mp_articles.archive_html END,
    archive_manifest = CASE WHEN excluded.archive_manifest <> '' THEN excluded.archive_manifest ELSE mp_articles.archive_manifest END,
    archived_at = COALESCE(mp_articles.archived_at, excluded.archived_at),
    raw_metadata = CASE WHEN excluded.raw_metadata <> '' THEN excluded.raw_metadata ELSE mp_articles.raw_metadata END,
		updated_at = ?`, article.Key, article.Biz, article.Mid, article.Idx, article.FileID, article.VideoID, article.Title,
		article.Digest, article.Author, article.ContentURL, article.SourceURL, article.CoverURL,
		nullableUnix(article.PublishTime), article.IsMulti, article.IsOriginal, article.IsPaid,
		article.IsPaySubscribe, article.ItemShowType, article.Subtype, article.CopyrightStat,
		article.Duration, article.AudioFileID, article.PlayURL, article.MaliciousTitleReasonID,
		article.MaliciousContentType, boolInt(article.SourceDeleted),
		catalogTimeOrNow(article.FirstSeenAt, now), catalogTimeOrNow(article.LastSeenAt, now),
		firstNonEmptyImport(article.ArchiveStatus, officialaccount.ArchiveStatusNotArchived), article.ArchiveDir,
		article.ArchiveHTML, article.ArchiveManifest, nullableUnix(article.ArchivedAt), article.RawMetadata,
		catalogTimeOrNow(article.FirstSeenAt, now), catalogTimeOrNow(article.LastSeenAt, now), now)
	if err != nil {
		return fmt.Errorf("import article %q: %w", article.Key, err)
	}
	return nil
}

func importCatalogAsset(ctx context.Context, tx *sql.Tx, asset officialaccount.ArticleAsset) error {
	now := time.Now().Unix()
	_, err := tx.ExecContext(ctx, `
INSERT INTO mp_article_assets (
    article_key, resource_key, kind, role, source_url, local_path, sha256,
    size, status, error, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(article_key, resource_key) DO UPDATE SET
    kind = CASE WHEN excluded.kind <> '' THEN excluded.kind ELSE mp_article_assets.kind END,
    role = CASE WHEN excluded.role <> '' THEN excluded.role ELSE mp_article_assets.role END,
    source_url = CASE WHEN excluded.source_url <> '' THEN excluded.source_url ELSE mp_article_assets.source_url END,
    local_path = CASE WHEN excluded.local_path <> '' THEN excluded.local_path ELSE mp_article_assets.local_path END,
    sha256 = CASE WHEN excluded.sha256 <> '' THEN excluded.sha256 ELSE mp_article_assets.sha256 END,
    size = COALESCE(excluded.size, mp_article_assets.size),
    status = excluded.status,
    error = excluded.error,
    updated_at = ?`, asset.ArticleKey, asset.ResourceKey, asset.Kind, asset.Role, asset.SourceURL,
		asset.LocalPath, asset.SHA256, nullableInt64(asset.Size), firstNonEmptyImport(asset.Status, "pending"),
		asset.Error, catalogTimeOrNow(asset.CreatedAt, now), catalogTimeOrNow(asset.UpdatedAt, now), now)
	if err != nil {
		return fmt.Errorf("import asset %q/%q: %w", asset.ArticleKey, asset.ResourceKey, err)
	}
	return nil
}

func insertMetricSnapshotTx(ctx context.Context, tx *sql.Tx, metric officialaccount.ArticleMetricSnapshot) (bool, error) {
	metric = officialaccount.SanitizeCatalogMetric(metric)
	observedAt := metric.ObservedAt
	if observedAt == 0 {
		observedAt = time.Now().Unix()
	}
	source := strings.TrimSpace(metric.Source)
	if source == "" {
		source = "import"
	}
	hash := metricSnapshotHash(metric, observedAt, source)
	exists, err := metricSnapshotExistsTx(ctx, tx, metric, observedAt, source)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	result, err := tx.ExecContext(ctx, `
INSERT OR IGNORE INTO mp_article_metric_snapshots (
    article_key, observed_at, source, view_count, like_count, comment_count,
    share_count, collect_count, reward_count, raw_metadata, snapshot_hash
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, metric.ArticleKey, observedAt, source,
		nullableInt64(metric.ViewCount), nullableInt64(metric.LikeCount), nullableInt64(metric.CommentCount),
		nullableInt64(metric.ShareCount), nullableInt64(metric.CollectCount), nullableInt64(metric.RewardCount),
		metric.RawMetadata, hash)
	if err != nil {
		return false, fmt.Errorf("import metric for %q: %w", metric.ArticleKey, err)
	}
	rows, _ := result.RowsAffected()
	return rows > 0, nil
}

func importCatalogMetricState(ctx context.Context, tx *sql.Tx, state officialaccount.ArticleMetricState) (bool, error) {
	state = officialaccount.SanitizeCatalogMetricState(state)
	now := time.Now().Unix()
	if state.Status == "" {
		state.Status = officialaccount.MetricStatePending
	}
	if state.LastSource == "" {
		state.LastSource = "import"
	}
	if state.UpdatedAt == 0 {
		state.UpdatedAt = now
	}
	current, err := scanArticleMetricState(tx.QueryRowContext(ctx, `
SELECT article_key, status, attempt_count, success_count, unknown_count,
       failure_count, last_attempt_at, last_success_at, last_observed_at,
       next_retry_at, last_source, last_error, updated_at
FROM mp_article_metric_states WHERE article_key = ?`, state.ArticleKey))
	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.ExecContext(ctx, `
INSERT INTO mp_article_metric_states (
    article_key, status, attempt_count, success_count, unknown_count,
    failure_count, last_attempt_at, last_success_at, last_observed_at,
    next_retry_at, last_source, last_error, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, state.ArticleKey, state.Status,
			state.AttemptCount, state.SuccessCount, state.UnknownCount, state.FailureCount,
			nullableUnix(state.LastAttemptAt), nullableUnix(state.LastSuccessAt), nullableUnix(state.LastObservedAt),
			nullableUnix(state.NextRetryAt), state.LastSource, sanitizeCatalogError(state.LastError), state.UpdatedAt)
		if err != nil {
			return false, fmt.Errorf("import metric state for %q: %w", state.ArticleKey, err)
		}
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("read existing metric state %q: %w", state.ArticleKey, err)
	}
	if !catalogMetricStateIsNewer(state, current) {
		return false, nil
	}
	_, err = tx.ExecContext(ctx, `
UPDATE mp_article_metric_states SET
    status = ?, attempt_count = ?, success_count = ?, unknown_count = ?,
    failure_count = ?, last_attempt_at = ?, last_success_at = ?,
    last_observed_at = ?, next_retry_at = ?, last_source = ?, last_error = ?,
    updated_at = ?
WHERE article_key = ?`, state.Status, state.AttemptCount, state.SuccessCount,
		state.UnknownCount, state.FailureCount, nullableUnix(state.LastAttemptAt),
		nullableUnix(state.LastSuccessAt), nullableUnix(state.LastObservedAt), nullableUnix(state.NextRetryAt),
		state.LastSource, sanitizeCatalogError(state.LastError), state.UpdatedAt, state.ArticleKey)
	if err != nil {
		return false, fmt.Errorf("update imported metric state %q: %w", state.ArticleKey, err)
	}
	return true, nil
}

func catalogMetricStateIsNewer(incoming, current officialaccount.ArticleMetricState) bool {
	if incoming.UpdatedAt != current.UpdatedAt {
		return incoming.UpdatedAt > current.UpdatedAt
	}
	if incoming.LastAttemptAt != current.LastAttemptAt {
		return incoming.LastAttemptAt > current.LastAttemptAt
	}
	return incoming.AttemptCount > current.AttemptCount
}

func metricSnapshotExistsTx(ctx context.Context, tx *sql.Tx, metric officialaccount.ArticleMetricSnapshot, observedAt int64, source string) (bool, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `
SELECT id FROM mp_article_metric_snapshots
WHERE article_key = ? AND observed_at = ? AND source = ?
  AND view_count IS ? AND like_count IS ? AND comment_count IS ?
  AND share_count IS ? AND collect_count IS ? AND reward_count IS ?
LIMIT 1`, metric.ArticleKey, observedAt, source, nullableInt64(metric.ViewCount),
		nullableInt64(metric.LikeCount), nullableInt64(metric.CommentCount), nullableInt64(metric.ShareCount),
		nullableInt64(metric.CollectCount), nullableInt64(metric.RewardCount)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check metric snapshot %q: %w", metric.ArticleKey, err)
	}
	return id > 0, nil
}

func refreshCatalogCounts(ctx context.Context, tx *sql.Tx, document officialaccount.CatalogExport) error {
	bizKeys := make(map[string]struct{}, len(document.Accounts)+len(document.Articles))
	for _, account := range document.Accounts {
		bizKeys[account.Biz] = struct{}{}
	}
	for _, article := range document.Articles {
		bizKeys[article.Biz] = struct{}{}
	}
	for biz := range bizKeys {
		if _, err := tx.ExecContext(ctx, `
UPDATE mp_accounts
SET article_count = (SELECT COUNT(*) FROM mp_articles WHERE biz = ?),
    archived_count = (SELECT COUNT(*) FROM mp_articles WHERE biz = ? AND archive_status = ?),
    updated_at = ?
WHERE biz = ?`, biz, biz, officialaccount.ArchiveStatusArchived, time.Now().Unix(), biz); err != nil {
			return fmt.Errorf("refresh imported account counts %q: %w", biz, err)
		}
	}
	return nil
}

func validCatalogPath(value string) bool {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" {
		return true
	}
	if strings.HasPrefix(value, "/") || strings.Contains(value, ":") {
		return false
	}
	clean := path.Clean(value)
	return clean != ".." && !strings.HasPrefix(clean, "../")
}

func catalogTimeOrNow(value, fallback int64) int64 {
	if value > 0 {
		return value
	}
	return fallback
}

func firstNonEmptyImport(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

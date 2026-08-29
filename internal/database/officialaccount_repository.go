package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"wx_channel/internal/officialaccount"
)

const (
	defaultOfficialAccountPageSize = 30
	maxOfficialAccountPageSize     = 100
	defaultArticlePageSize         = 50
	maxArticlePageSize             = 100
	defaultMetricCandidatePageSize = 40
	maxMetricCandidatePageSize     = 100
)

// OfficialAccountRepository is the SQLite owner for public-account metadata,
// article catalog rows, metric observations, archive state, and sync runs.
// Credential-bearing Account fields are deliberately ignored by this type.
type OfficialAccountRepository struct {
	db  *sql.DB
	now func() time.Time
}

func NewOfficialAccountRepository() *OfficialAccountRepository {
	return NewOfficialAccountRepositoryWithDB(GetDB())
}

func NewOfficialAccountRepositoryWithDB(db *sql.DB) *OfficialAccountRepository {
	return &OfficialAccountRepository{db: db, now: time.Now}
}

func (r *OfficialAccountRepository) ready() error {
	if r == nil || r.db == nil {
		return officialaccount.ErrCatalogUnavailable
	}
	return nil
}

func (r *OfficialAccountRepository) UpsertAccount(account officialaccount.Account) error {
	if err := r.ready(); err != nil {
		return err
	}
	biz := strings.TrimSpace(account.Biz)
	if biz == "" {
		return officialaccount.ErrMissingBiz
	}
	now := account.UpdateTime
	if now == 0 {
		now = r.now().Unix()
	}
	createdAt := account.CreatedAt
	if createdAt == 0 {
		createdAt = now
	}
	_, err := r.db.Exec(`
INSERT INTO mp_accounts (
    biz, nickname, avatar_url, author_id, source, is_effective, account_error,
    discovered_at, last_seen_at, created_at, updated_at
) VALUES (?, ?, ?, ?, 'capture', ?, ?, ?, ?, ?, ?)
ON CONFLICT(biz) DO UPDATE SET
    nickname = CASE WHEN excluded.nickname <> '' THEN excluded.nickname ELSE mp_accounts.nickname END,
    avatar_url = CASE WHEN excluded.avatar_url <> '' THEN excluded.avatar_url ELSE mp_accounts.avatar_url END,
    author_id = CASE WHEN excluded.author_id <> '' THEN excluded.author_id ELSE mp_accounts.author_id END,
    is_effective = excluded.is_effective,
    account_error = excluded.account_error,
    last_seen_at = excluded.last_seen_at,
    updated_at = excluded.updated_at
`, biz, strings.TrimSpace(account.Nickname), strings.TrimSpace(account.AvatarURL),
		strings.TrimSpace(account.AuthorID), boolInt(account.IsEffective), strings.TrimSpace(account.Error),
		createdAt, now, createdAt, now)
	if err != nil {
		return fmt.Errorf("upsert official account %q: %w", biz, err)
	}
	return nil
}

func (r *OfficialAccountRepository) ListAccounts(keyword string, page, pageSize int) (officialaccount.AccountPage, error) {
	if err := r.ready(); err != nil {
		return officialaccount.AccountPage{}, err
	}
	page, pageSize = normalizePage(page, pageSize, defaultOfficialAccountPageSize, maxOfficialAccountPageSize)
	where, args := accountKeywordClause(keyword)
	var total int64
	if err := r.db.QueryRow("SELECT COUNT(*) FROM mp_accounts "+where, args...).Scan(&total); err != nil {
		return officialaccount.AccountPage{}, fmt.Errorf("count official accounts: %w", err)
	}

	query := `
SELECT biz, nickname, avatar_url, is_effective, created_at, updated_at,
       account_error, article_count, archived_count, last_sync_at,
       sync_status, sync_error
FROM mp_accounts ` + where + `
ORDER BY updated_at DESC, biz ASC
LIMIT ? OFFSET ?`
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return officialaccount.AccountPage{}, fmt.Errorf("list official accounts: %w", err)
	}
	defer rows.Close()

	items := make([]officialaccount.AccountSummary, 0, pageSize)
	for rows.Next() {
		var item officialaccount.AccountSummary
		var effective int
		var lastSyncAt sql.NullInt64
		if err := rows.Scan(&item.Biz, &item.Nickname, &item.AvatarURL, &effective,
			&item.CreatedAt, &item.UpdateTime, &item.Error, &item.ArticleCount,
			&item.ArchivedCount, &lastSyncAt, &item.SyncStatus, &item.SyncError); err != nil {
			return officialaccount.AccountPage{}, fmt.Errorf("scan official account: %w", err)
		}
		item.IsEffective = effective != 0
		if lastSyncAt.Valid {
			item.LastSyncAt = lastSyncAt.Int64
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return officialaccount.AccountPage{}, fmt.Errorf("iterate official accounts: %w", err)
	}
	return officialaccount.AccountPage{
		Items:      items,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages(total, pageSize),
	}, nil
}

func (r *OfficialAccountRepository) UpsertArticles(biz string, articles []officialaccount.ArticleRecord, seenAt time.Time) (officialaccount.ArticleUpsertStats, error) {
	if err := r.ready(); err != nil {
		return officialaccount.ArticleUpsertStats{}, err
	}
	biz = strings.TrimSpace(biz)
	if biz == "" {
		return officialaccount.ArticleUpsertStats{}, officialaccount.ErrMissingBiz
	}
	if seenAt.IsZero() {
		seenAt = r.now()
	}
	tx, err := r.db.Begin()
	if err != nil {
		return officialaccount.ArticleUpsertStats{}, fmt.Errorf("begin article upsert: %w", err)
	}
	rollback := func(err error) (officialaccount.ArticleUpsertStats, error) {
		_ = tx.Rollback()
		return officialaccount.ArticleUpsertStats{}, err
	}
	if _, err := tx.Exec(`INSERT OR IGNORE INTO mp_accounts (biz, source, discovered_at, last_seen_at) VALUES (?, 'sync', ?, ?)`, biz, seenAt.Unix(), seenAt.Unix()); err != nil {
		return rollback(fmt.Errorf("ensure official account for articles: %w", err))
	}

	normalized := make([]officialaccount.ArticleRecord, 0, len(articles))
	seenKeys := make(map[string]struct{}, len(articles))
	for _, article := range articles {
		article = officialaccount.SanitizeCatalogArticle(article)
		article.Key = strings.TrimSpace(article.Key)
		if article.Key == "" {
			continue
		}
		if article.Biz == "" {
			article.Biz = biz
		} else if article.Biz != biz {
			return rollback(fmt.Errorf("article %q belongs to biz %q, expected %q", article.Key, article.Biz, biz))
		}
		if _, exists := seenKeys[article.Key]; exists {
			continue
		}
		seenKeys[article.Key] = struct{}{}
		if article.FirstSeenAt == 0 {
			article.FirstSeenAt = seenAt.Unix()
		}
		if article.LastSeenAt == 0 {
			article.LastSeenAt = seenAt.Unix()
		}
		if article.ArchiveStatus == "" {
			article.ArchiveStatus = officialaccount.ArchiveStatusNotArchived
		}
		normalized = append(normalized, article)
	}

	keys := make([]string, 0, len(normalized))
	for _, article := range normalized {
		keys = append(keys, article.Key)
	}
	existingOwners, err := loadArticleOwnersTx(context.Background(), tx, keys)
	if err != nil {
		return rollback(err)
	}
	stats := officialaccount.ArticleUpsertStats{}
	for _, article := range normalized {
		if owner, exists := existingOwners[article.Key]; exists && owner != biz {
			return rollback(fmt.Errorf("article %q belongs to biz %q, expected %q", article.Key, owner, biz))
		}
		_, exists := existingOwners[article.Key]
		_, err := tx.Exec(`
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
    last_seen_at = excluded.last_seen_at,
    raw_metadata = CASE WHEN excluded.raw_metadata <> '' THEN excluded.raw_metadata ELSE mp_articles.raw_metadata END,
    updated_at = excluded.updated_at
		`, article.Key, biz, strings.TrimSpace(article.Mid), article.Idx, article.FileID, strings.TrimSpace(article.VideoID),
			strings.TrimSpace(article.Title), strings.TrimSpace(article.Digest), strings.TrimSpace(article.Author),
			strings.TrimSpace(article.ContentURL), strings.TrimSpace(article.SourceURL), strings.TrimSpace(article.CoverURL),
			nullableUnix(article.PublishTime), article.IsMulti, article.IsOriginal, article.IsPaid,
			article.IsPaySubscribe, article.ItemShowType, article.Subtype, article.CopyrightStat, article.Duration,
			article.AudioFileID, strings.TrimSpace(article.PlayURL), article.MaliciousTitleReasonID,
			article.MaliciousContentType, boolInt(article.SourceDeleted), article.FirstSeenAt,
			article.LastSeenAt, article.ArchiveStatus, strings.TrimSpace(article.ArchiveDir),
			strings.TrimSpace(article.ArchiveHTML), strings.TrimSpace(article.ArchiveManifest), nullableUnix(article.ArchivedAt),
			strings.TrimSpace(article.RawMetadata), seenAt.Unix(), seenAt.Unix())
		if err != nil {
			return rollback(fmt.Errorf("upsert article %q: %w", article.Key, err))
		}
		stats.Seen++
		if !exists {
			stats.Inserted++
		} else {
			stats.Updated++
		}
	}
	if _, err := tx.Exec(`
UPDATE mp_accounts
SET article_count = (SELECT COUNT(*) FROM mp_articles WHERE biz = ?),
    archived_count = (SELECT COUNT(*) FROM mp_articles WHERE biz = ? AND archive_status = ?),
    last_seen_at = ?, updated_at = ?
WHERE biz = ?`, biz, biz, officialaccount.ArchiveStatusArchived, seenAt.Unix(), seenAt.Unix(), biz); err != nil {
		return rollback(fmt.Errorf("refresh account article counts: %w", err))
	}
	if err := tx.Commit(); err != nil {
		return officialaccount.ArticleUpsertStats{}, fmt.Errorf("commit article upsert: %w", err)
	}
	return stats, nil
}

func loadArticleOwnersTx(ctx context.Context, tx *sql.Tx, keys []string) (map[string]string, error) {
	owners := make(map[string]string)
	cleanKeys := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		cleanKeys = append(cleanKeys, key)
	}
	if len(cleanKeys) == 0 {
		return owners, nil
	}

	// Keep IN clauses below SQLite's variable limit when a history page or
	// import batch contains many articles.
	const maxKeysPerQuery = 500
	for start := 0; start < len(cleanKeys); start += maxKeysPerQuery {
		end := start + maxKeysPerQuery
		if end > len(cleanKeys) {
			end = len(cleanKeys)
		}
		batch := cleanKeys[start:end]
		placeholders := make([]string, len(batch))
		args := make([]interface{}, len(batch))
		for i, key := range batch {
			placeholders[i] = "?"
			args[i] = key
		}
		rows, err := tx.QueryContext(ctx, fmt.Sprintf(`
SELECT article_key, biz
FROM mp_articles
WHERE article_key IN (%s)`, strings.Join(placeholders, ",")), args...)
		if err != nil {
			return nil, fmt.Errorf("load article owners: %w", err)
		}
		for rows.Next() {
			var key, owner string
			if err := rows.Scan(&key, &owner); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("scan article owner: %w", err)
			}
			owners[key] = owner
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("iterate article owners: %w", err)
		}
		if err := rows.Close(); err != nil {
			return nil, fmt.Errorf("close article owners: %w", err)
		}
	}
	return owners, nil
}

func (r *OfficialAccountRepository) ListArticles(query officialaccount.ArticleQuery) (officialaccount.ArticlePage, error) {
	if err := r.ready(); err != nil {
		return officialaccount.ArticlePage{}, err
	}
	query.Page, query.PageSize = normalizePage(query.Page, query.PageSize, defaultArticlePageSize, maxArticlePageSize)
	where, args := articleWhereClause(query)
	var total int64
	if err := r.db.QueryRow("SELECT COUNT(*) FROM mp_articles "+where, args...).Scan(&total); err != nil {
		return officialaccount.ArticlePage{}, fmt.Errorf("count official articles: %w", err)
	}
	sortColumn := "publish_time"
	switch strings.ToLower(strings.TrimSpace(query.Sort)) {
	case "last_seen_at":
		sortColumn = "last_seen_at"
	case "title":
		sortColumn = "title"
	case "updated_at":
		sortColumn = "updated_at"
	}
	direction := "DESC"
	if !query.Descending {
		direction = "ASC"
	}
	statement := `
SELECT article_key, biz, mid, idx, file_id, title, digest, author,
       video_id,
       content_url, source_url, cover_url, publish_time, is_multi, is_original,
       is_paid, is_pay_subscribe, item_show_type, subtype, copyright_stat, duration,
       audio_fileid, play_url, malicious_title_reason_id, malicious_content_type,
       source_deleted, first_seen_at, last_seen_at, archive_status, archive_dir,
       archive_html, archive_manifest, archived_at, raw_metadata
FROM mp_articles ` + where + `
ORDER BY ` + sortColumn + ` ` + direction + `, article_key ASC
LIMIT ? OFFSET ?`
	args = append(args, query.PageSize, (query.Page-1)*query.PageSize)
	rows, err := r.db.Query(statement, args...)
	if err != nil {
		return officialaccount.ArticlePage{}, fmt.Errorf("list official articles: %w", err)
	}
	defer rows.Close()
	items := make([]officialaccount.ArticleRecord, 0, query.PageSize)
	for rows.Next() {
		item, err := scanArticleRecord(rows)
		if err != nil {
			return officialaccount.ArticlePage{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return officialaccount.ArticlePage{}, fmt.Errorf("iterate official articles: %w", err)
	}
	keys := make([]string, 0, len(items))
	for _, item := range items {
		keys = append(keys, item.Key)
	}
	metrics, err := r.LatestArticleMetrics(keys)
	if err != nil {
		return officialaccount.ArticlePage{}, err
	}
	for i := range items {
		if metric, ok := metrics[items[i].Key]; ok {
			copy := metric
			items[i].Metrics = &copy
		}
	}
	assets, err := r.ListArticleAssets(keys)
	if err != nil {
		return officialaccount.ArticlePage{}, err
	}
	states, err := r.ListArticleMetricStates(keys)
	if err != nil {
		return officialaccount.ArticlePage{}, err
	}
	for i := range items {
		items[i].Assets = assets[items[i].Key]
		if state, ok := states[items[i].Key]; ok {
			copy := state
			items[i].MetricState = &copy
		} else {
			items[i].MetricState = &officialaccount.ArticleMetricState{
				ArticleKey: items[i].Key,
				Status:     officialaccount.MetricStatePending,
			}
		}
		items[i] = officialaccount.SanitizeCatalogArticle(items[i])
	}
	return officialaccount.ArticlePage{
		Items:      items,
		Total:      total,
		Page:       query.Page,
		PageSize:   query.PageSize,
		TotalPages: totalPages(total, query.PageSize),
	}, nil
}

func (r *OfficialAccountRepository) GetArticle(key string) (*officialaccount.ArticleRecord, error) {
	if err := r.ready(); err != nil {
		return nil, err
	}
	row := r.db.QueryRow(`
SELECT article_key, biz, mid, idx, file_id, title, digest, author,
       video_id,
       content_url, source_url, cover_url, publish_time, is_multi, is_original,
       is_paid, is_pay_subscribe, item_show_type, subtype, copyright_stat, duration,
       audio_fileid, play_url, malicious_title_reason_id, malicious_content_type,
       source_deleted, first_seen_at, last_seen_at, archive_status, archive_dir,
       archive_html, archive_manifest, archived_at, raw_metadata
FROM mp_articles WHERE article_key = ?`, strings.TrimSpace(key))
	item, err := scanArticleRecord(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if metrics, err := r.LatestArticleMetrics([]string{item.Key}); err != nil {
		return nil, err
	} else if metric, ok := metrics[item.Key]; ok {
		item.Metrics = &metric
	}
	if states, err := r.ListArticleMetricStates([]string{item.Key}); err != nil {
		return nil, err
	} else if state, ok := states[item.Key]; ok {
		item.MetricState = &state
	} else {
		item.MetricState = &officialaccount.ArticleMetricState{ArticleKey: item.Key, Status: officialaccount.MetricStatePending}
	}
	assets, err := r.ListArticleAssets([]string{item.Key})
	if err != nil {
		return nil, err
	}
	item.Assets = assets[item.Key]
	item = officialaccount.SanitizeCatalogArticle(item)
	return &item, nil
}

func (r *OfficialAccountRepository) UpsertArticleAssets(articleKey string, assets []officialaccount.ArticleAsset) error {
	if err := r.ready(); err != nil {
		return err
	}
	articleKey = strings.TrimSpace(articleKey)
	if articleKey == "" {
		return errors.New("article key is required")
	}
	if len(assets) == 0 {
		return nil
	}
	var existingArticle string
	if err := r.db.QueryRow("SELECT article_key FROM mp_articles WHERE article_key = ?", articleKey).Scan(&existingArticle); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sql.ErrNoRows
		}
		return fmt.Errorf("check article for assets %q: %w", articleKey, err)
	}
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin article asset upsert: %w", err)
	}
	rollback := func(err error) error {
		_ = tx.Rollback()
		return err
	}
	now := r.now().Unix()
	for _, asset := range assets {
		asset = officialaccount.SanitizeCatalogAsset(asset)
		if asset.ArticleKey == "" {
			asset.ArticleKey = articleKey
		} else if asset.ArticleKey != articleKey {
			return rollback(fmt.Errorf("asset %q belongs to article %q, expected %q", asset.ResourceKey, asset.ArticleKey, articleKey))
		}
		if asset.ResourceKey == "" {
			continue
		}
		if asset.Status == "" {
			asset.Status = "pending"
		}
		createdAt := asset.CreatedAt
		if createdAt == 0 {
			createdAt = now
		}
		updatedAt := asset.UpdatedAt
		if updatedAt == 0 {
			updatedAt = now
		}
		if _, err := tx.Exec(`
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
    updated_at = excluded.updated_at
`, asset.ArticleKey, asset.ResourceKey, asset.Kind, asset.Role, asset.SourceURL,
			asset.LocalPath, asset.SHA256, nullableInt64(asset.Size), asset.Status,
			asset.Error, createdAt, updatedAt); err != nil {
			return rollback(fmt.Errorf("upsert article asset %q/%q: %w", asset.ArticleKey, asset.ResourceKey, err))
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit article asset upsert: %w", err)
	}
	return nil
}

func (r *OfficialAccountRepository) ListArticleAssets(keys []string) (map[string][]officialaccount.ArticleAsset, error) {
	if err := r.ready(); err != nil {
		return nil, err
	}
	result := make(map[string][]officialaccount.ArticleAsset)
	cleanKeys := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		cleanKeys = append(cleanKeys, key)
	}
	if len(cleanKeys) == 0 {
		return result, nil
	}
	placeholders := make([]string, len(cleanKeys))
	args := make([]interface{}, len(cleanKeys))
	for i, key := range cleanKeys {
		placeholders[i] = "?"
		args[i] = key
	}
	rows, err := r.db.Query(fmt.Sprintf(`
SELECT article_key, resource_key, kind, role, source_url, local_path, sha256,
       size, status, error, created_at, updated_at
FROM mp_article_assets
WHERE article_key IN (%s)
ORDER BY article_key ASC, resource_key ASC`, strings.Join(placeholders, ",")), args...)
	if err != nil {
		return nil, fmt.Errorf("list article assets: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		asset, err := scanArticleAsset(rows)
		if err != nil {
			return nil, err
		}
		result[asset.ArticleKey] = append(result[asset.ArticleKey], asset)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate article assets: %w", err)
	}
	return result, nil
}

func (r *OfficialAccountRepository) RecordArticleMetrics(metrics []officialaccount.ArticleMetricSnapshot) error {
	if err := r.ready(); err != nil {
		return err
	}
	if len(metrics) == 0 {
		return nil
	}
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin metric insert: %w", err)
	}
	for _, metric := range metrics {
		if strings.TrimSpace(metric.ArticleKey) == "" {
			continue
		}
		metric = officialaccount.SanitizeCatalogMetric(metric)
		observedAt := metric.ObservedAt
		if observedAt == 0 {
			observedAt = r.now().Unix()
		}
		source := strings.TrimSpace(metric.Source)
		if source == "" {
			source = "unknown"
		}
		snapshotHash := metricSnapshotHash(metric, observedAt, source)
		exists, err := metricSnapshotExistsTx(context.Background(), tx, metric, observedAt, source)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("check metric snapshot for %q: %w", metric.ArticleKey, err)
		}
		if exists {
			continue
		}
		if _, err := tx.Exec(`
INSERT OR IGNORE INTO mp_article_metric_snapshots (
    article_key, observed_at, source, view_count, like_count, comment_count,
    share_count, collect_count, reward_count, raw_metadata, snapshot_hash
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, metric.ArticleKey, observedAt, source,
			nullableInt64(metric.ViewCount), nullableInt64(metric.LikeCount), nullableInt64(metric.CommentCount),
			nullableInt64(metric.ShareCount), nullableInt64(metric.CollectCount), nullableInt64(metric.RewardCount),
			strings.TrimSpace(metric.RawMetadata), snapshotHash); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("insert metric snapshot for %q: %w", metric.ArticleKey, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit metric insert: %w", err)
	}
	return nil
}

func (r *OfficialAccountRepository) LatestArticleMetrics(keys []string) (map[string]officialaccount.ArticleMetricSnapshot, error) {
	if err := r.ready(); err != nil {
		return nil, err
	}
	result := make(map[string]officialaccount.ArticleMetricSnapshot)
	cleanKeys := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		cleanKeys = append(cleanKeys, key)
	}
	if len(cleanKeys) == 0 {
		return result, nil
	}
	placeholders := make([]string, len(cleanKeys))
	args := make([]interface{}, len(cleanKeys))
	for i, key := range cleanKeys {
		placeholders[i] = "?"
		args[i] = key
	}
	query := fmt.Sprintf(`
	SELECT m.id, m.article_key, m.observed_at, m.source,
	       COALESCE(m.view_count, (
	           SELECT older.view_count FROM mp_article_metric_snapshots older
	           WHERE older.article_key = m.article_key AND older.view_count IS NOT NULL
	           ORDER BY older.observed_at DESC, older.id DESC LIMIT 1
	       )),
	       COALESCE(m.like_count, (
	           SELECT older.like_count FROM mp_article_metric_snapshots older
	           WHERE older.article_key = m.article_key AND older.like_count IS NOT NULL
	           ORDER BY older.observed_at DESC, older.id DESC LIMIT 1
	       )),
	       COALESCE(m.comment_count, (
	           SELECT older.comment_count FROM mp_article_metric_snapshots older
	           WHERE older.article_key = m.article_key AND older.comment_count IS NOT NULL
	           ORDER BY older.observed_at DESC, older.id DESC LIMIT 1
	       )),
	       COALESCE(m.share_count, (
	           SELECT older.share_count FROM mp_article_metric_snapshots older
	           WHERE older.article_key = m.article_key AND older.share_count IS NOT NULL
	           ORDER BY older.observed_at DESC, older.id DESC LIMIT 1
	       )),
	       COALESCE(m.collect_count, (
	           SELECT older.collect_count FROM mp_article_metric_snapshots older
	           WHERE older.article_key = m.article_key AND older.collect_count IS NOT NULL
	           ORDER BY older.observed_at DESC, older.id DESC LIMIT 1
	       )),
	       COALESCE(m.reward_count, (
	           SELECT older.reward_count FROM mp_article_metric_snapshots older
	           WHERE older.article_key = m.article_key AND older.reward_count IS NOT NULL
	           ORDER BY older.observed_at DESC, older.id DESC LIMIT 1
	       )),
	       m.raw_metadata
	FROM mp_article_metric_snapshots m
	WHERE m.article_key IN (%s)
	  AND NOT EXISTS (
	      SELECT 1 FROM mp_article_metric_snapshots newer
	      WHERE newer.article_key = m.article_key
	        AND (newer.observed_at > m.observed_at
	             OR (newer.observed_at = m.observed_at AND newer.id > m.id))
	  )`, strings.Join(placeholders, ","))
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list latest article metrics: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		metric, err := scanMetricSnapshot(rows)
		if err != nil {
			return nil, err
		}
		result[metric.ArticleKey] = officialaccount.SanitizeCatalogMetric(metric)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate latest article metrics: %w", err)
	}
	return result, nil
}

func (r *OfficialAccountRepository) ListArticleMetricStates(keys []string) (map[string]officialaccount.ArticleMetricState, error) {
	if err := r.ready(); err != nil {
		return nil, err
	}
	result := make(map[string]officialaccount.ArticleMetricState)
	cleanKeys := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		cleanKeys = append(cleanKeys, key)
	}
	if len(cleanKeys) == 0 {
		return result, nil
	}
	placeholders := make([]string, len(cleanKeys))
	args := make([]interface{}, len(cleanKeys))
	for i, key := range cleanKeys {
		placeholders[i] = "?"
		args[i] = key
	}
	rows, err := r.db.Query(fmt.Sprintf(`
SELECT article_key, status, attempt_count, success_count, unknown_count,
       failure_count, last_attempt_at, last_success_at, last_observed_at,
       next_retry_at, last_source, last_error, updated_at
FROM mp_article_metric_states
WHERE article_key IN (%s)`, strings.Join(placeholders, ",")), args...)
	if err != nil {
		return nil, fmt.Errorf("list article metric states: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		state, err := scanArticleMetricState(rows)
		if err != nil {
			return nil, err
		}
		result[state.ArticleKey] = state
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate article metric states: %w", err)
	}
	return result, nil
}

func (r *OfficialAccountRepository) ListArticleMetricCandidates(biz string, force bool, now time.Time, afterPublishTime int64, afterArticleKey string, limit int) (officialaccount.ArticleMetricCandidatePage, error) {
	if err := r.ready(); err != nil {
		return officialaccount.ArticleMetricCandidatePage{}, err
	}
	biz = strings.TrimSpace(biz)
	if biz == "" {
		return officialaccount.ArticleMetricCandidatePage{}, officialaccount.ErrMissingBiz
	}
	if limit <= 0 {
		limit = defaultMetricCandidatePageSize
	}
	if limit > maxMetricCandidatePageSize {
		limit = maxMetricCandidatePageSize
	}
	where, args := metricCandidateWhere(biz, force, now)
	if strings.TrimSpace(afterArticleKey) != "" {
		where += ` AND (COALESCE(a.publish_time, 0) < ? OR
            (COALESCE(a.publish_time, 0) = ? AND a.article_key > ?))`
		args = append(args, afterPublishTime, afterPublishTime, strings.TrimSpace(afterArticleKey))
	}
	query := `
SELECT a.article_key, a.biz, a.mid, a.idx, a.file_id, a.title, a.digest, a.author,
       a.video_id,
       a.content_url, a.source_url, a.cover_url, a.publish_time, a.is_multi,
       a.is_original, a.is_paid, a.is_pay_subscribe, a.item_show_type,
       a.subtype, a.copyright_stat, a.duration, a.audio_fileid, a.play_url,
       a.malicious_title_reason_id, a.malicious_content_type, a.source_deleted,
       a.first_seen_at, a.last_seen_at, a.archive_status, a.archive_dir,
       a.archive_html, a.archive_manifest, a.archived_at, a.raw_metadata
FROM mp_articles a
LEFT JOIN mp_article_metric_states state ON state.article_key = a.article_key
` + where + `
ORDER BY COALESCE(a.publish_time, 0) DESC, a.article_key ASC
LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return officialaccount.ArticleMetricCandidatePage{}, fmt.Errorf("list article metric candidates: %w", err)
	}
	defer rows.Close()
	items := make([]officialaccount.ArticleRecord, 0, limit)
	for rows.Next() {
		item, err := scanArticleRecord(rows)
		if err != nil {
			return officialaccount.ArticleMetricCandidatePage{}, err
		}
		items = append(items, officialaccount.SanitizeCatalogArticle(item))
	}
	if err := rows.Err(); err != nil {
		return officialaccount.ArticleMetricCandidatePage{}, fmt.Errorf("iterate article metric candidates: %w", err)
	}
	page := officialaccount.ArticleMetricCandidatePage{Items: items, HasMore: len(items) == limit}
	if len(items) > 0 {
		last := items[len(items)-1]
		page.NextPublishTime = last.PublishTime
		page.NextArticleKey = last.Key
	}
	return page, nil
}

func (r *OfficialAccountRepository) CountArticleMetricCandidates(biz string, force bool, now time.Time) (int, error) {
	if err := r.ready(); err != nil {
		return 0, err
	}
	where, args := metricCandidateWhere(strings.TrimSpace(biz), force, now)
	var count int
	if err := r.db.QueryRow("SELECT COUNT(*) FROM mp_articles a LEFT JOIN mp_article_metric_states state ON state.article_key = a.article_key "+where, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count article metric candidates: %w", err)
	}
	return count, nil
}

func metricCandidateWhere(biz string, force bool, now time.Time) (string, []interface{}) {
	clauses := []string{"a.biz = ?"}
	args := []interface{}{biz}
	if !force {
		clauses = append(clauses, `(state.article_key IS NULL OR state.status = ? OR
			(state.status = ? AND (state.next_retry_at IS NULL OR state.next_retry_at <= ?)) OR
			(state.status = ? AND (
				COALESCE(state.last_source, '') <> ? OR
				(COALESCE(state.last_error, '') <> '' AND (state.next_retry_at IS NULL OR state.next_retry_at <= ?)) OR
				((state.next_retry_at IS NULL OR state.next_retry_at <= ?) AND NOT EXISTS (
					SELECT 1 FROM mp_article_metric_snapshots metric
					WHERE metric.article_key = a.article_key AND metric.comment_count IS NOT NULL
				))
			)))`)
		args = append(args, officialaccount.MetricStatePending, officialaccount.MetricStateFailed, now.Unix(),
			officialaccount.MetricStateSuccess, "getappmsgext", now.Unix(), now.Unix())
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

// SaveArticleMetricResult atomically records the latest attempt state and an
// optional append-only snapshot. This is the recovery boundary for one item.
func (r *OfficialAccountRepository) SaveArticleMetricResult(state officialaccount.ArticleMetricState, snapshot *officialaccount.ArticleMetricSnapshot) error {
	if err := r.ready(); err != nil {
		return err
	}
	state.ArticleKey = strings.TrimSpace(state.ArticleKey)
	state.Status = strings.TrimSpace(state.Status)
	if state.ArticleKey == "" {
		return errors.New("article metric state article key is required")
	}
	if state.Status != officialaccount.MetricStatePending && state.Status != officialaccount.MetricStateSuccess && state.Status != officialaccount.MetricStateUnknown && state.Status != officialaccount.MetricStateFailed {
		return fmt.Errorf("invalid article metric state %q", state.Status)
	}
	if snapshot != nil {
		copy := officialaccount.SanitizeCatalogMetric(*snapshot)
		snapshot = &copy
		if snapshot.ArticleKey == "" {
			snapshot.ArticleKey = state.ArticleKey
		} else if snapshot.ArticleKey != state.ArticleKey {
			return fmt.Errorf("metric snapshot belongs to article %q, expected %q", snapshot.ArticleKey, state.ArticleKey)
		}
	}
	now := r.now().Unix()
	if state.UpdatedAt == 0 {
		state.UpdatedAt = now
	}
	if state.LastSource == "" {
		state.LastSource = "unknown"
	}
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin article metric result: %w", err)
	}
	rollback := func(err error) error {
		_ = tx.Rollback()
		return err
	}
	var exists int
	if err := tx.QueryRow("SELECT COUNT(*) FROM mp_articles WHERE article_key = ?", state.ArticleKey).Scan(&exists); err != nil {
		return rollback(fmt.Errorf("check article metric state %q: %w", state.ArticleKey, err))
	}
	if exists == 0 {
		return rollback(sql.ErrNoRows)
	}
	if snapshot != nil {
		observedAt := snapshot.ObservedAt
		if observedAt == 0 {
			observedAt = now
		}
		source := strings.TrimSpace(snapshot.Source)
		if source == "" {
			source = state.LastSource
		}
		snapshotHash := metricSnapshotHash(*snapshot, observedAt, source)
		if _, err := tx.Exec(`
INSERT OR IGNORE INTO mp_article_metric_snapshots (
    article_key, observed_at, source, view_count, like_count, comment_count,
    share_count, collect_count, reward_count, raw_metadata, snapshot_hash
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, snapshot.ArticleKey, observedAt, source,
			nullableInt64(snapshot.ViewCount), nullableInt64(snapshot.LikeCount), nullableInt64(snapshot.CommentCount),
			nullableInt64(snapshot.ShareCount), nullableInt64(snapshot.CollectCount), nullableInt64(snapshot.RewardCount),
			strings.TrimSpace(snapshot.RawMetadata), snapshotHash); err != nil {
			return rollback(fmt.Errorf("insert article metric snapshot %q: %w", state.ArticleKey, err))
		}
	}
	if _, err := tx.Exec(`
INSERT INTO mp_article_metric_states (
    article_key, status, attempt_count, success_count, unknown_count,
    failure_count, last_attempt_at, last_success_at, last_observed_at,
    next_retry_at, last_source, last_error, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(article_key) DO UPDATE SET
    status = excluded.status,
    attempt_count = excluded.attempt_count,
    success_count = excluded.success_count,
    unknown_count = excluded.unknown_count,
    failure_count = excluded.failure_count,
    last_attempt_at = excluded.last_attempt_at,
    last_success_at = excluded.last_success_at,
    last_observed_at = excluded.last_observed_at,
    next_retry_at = excluded.next_retry_at,
    last_source = excluded.last_source,
    last_error = excluded.last_error,
    updated_at = excluded.updated_at`, state.ArticleKey, state.Status, state.AttemptCount,
		state.SuccessCount, state.UnknownCount, state.FailureCount, nullableUnix(state.LastAttemptAt),
		nullableUnix(state.LastSuccessAt), nullableUnix(state.LastObservedAt), nullableUnix(state.NextRetryAt),
		state.LastSource, sanitizeCatalogError(state.LastError), state.UpdatedAt); err != nil {
		return rollback(fmt.Errorf("upsert article metric state %q: %w", state.ArticleKey, err))
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit article metric result %q: %w", state.ArticleKey, err)
	}
	return nil
}

func upsertArticleAssetTx(tx *sql.Tx, asset officialaccount.ArticleAsset, articleKey string, now int64) error {
	asset = officialaccount.SanitizeCatalogAsset(asset)
	if asset.ArticleKey == "" {
		asset.ArticleKey = articleKey
	} else if asset.ArticleKey != articleKey {
		return fmt.Errorf("asset %q belongs to article %q, expected %q", asset.ResourceKey, asset.ArticleKey, articleKey)
	}
	if asset.ResourceKey == "" {
		return nil
	}
	if !validCatalogPath(asset.LocalPath) {
		return fmt.Errorf("asset %q/%q has unsafe local path", articleKey, asset.ResourceKey)
	}
	if asset.SHA256 != "" {
		if len(asset.SHA256) != sha256.Size*2 {
			return fmt.Errorf("asset %q/%q has invalid SHA-256", articleKey, asset.ResourceKey)
		}
		if _, err := hex.DecodeString(asset.SHA256); err != nil {
			return fmt.Errorf("asset %q/%q has invalid SHA-256: %w", articleKey, asset.ResourceKey, err)
		}
	}
	if asset.Size != nil && *asset.Size < 0 {
		return fmt.Errorf("asset %q/%q has negative size", articleKey, asset.ResourceKey)
	}
	if asset.Status == "" {
		asset.Status = "pending"
	}
	createdAt := asset.CreatedAt
	if createdAt == 0 {
		createdAt = now
	}
	updatedAt := asset.UpdatedAt
	if updatedAt == 0 {
		updatedAt = now
	}
	if _, err := tx.Exec(`
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
    updated_at = excluded.updated_at
`, asset.ArticleKey, asset.ResourceKey, asset.Kind, asset.Role, asset.SourceURL,
		asset.LocalPath, asset.SHA256, nullableInt64(asset.Size), asset.Status,
		asset.Error, createdAt, updatedAt); err != nil {
		return fmt.Errorf("upsert article asset %q/%q: %w", asset.ArticleKey, asset.ResourceKey, err)
	}
	return nil
}

func (r *OfficialAccountRepository) UpdateArticleArchive(state officialaccount.ArticleArchiveState) error {
	if err := r.ready(); err != nil {
		return err
	}
	key := strings.TrimSpace(state.ArticleKey)
	if key == "" {
		return errors.New("article key is required")
	}
	status := strings.TrimSpace(state.Status)
	if status == "" {
		return errors.New("article archive status is required")
	}
	archivedAt := nullableUnix(state.ArchivedAt)
	if state.ArchivedAt == 0 && status == officialaccount.ArchiveStatusArchived {
		archivedAt = r.now().Unix()
	}
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin article archive update %q: %w", key, err)
	}
	rollback := func(err error) error {
		_ = tx.Rollback()
		return err
	}
	updatedAt := r.now().Unix()
	result, err := tx.Exec(`
UPDATE mp_articles
SET archive_status = ?,
    archive_dir = CASE WHEN ? <> '' THEN ? ELSE archive_dir END,
    archive_html = CASE WHEN ? <> '' THEN ? ELSE archive_html END,
    archive_manifest = CASE WHEN ? <> '' THEN ? ELSE archive_manifest END,
    archived_at = COALESCE(?, archived_at),
    updated_at = ?
WHERE article_key = ?`, status, state.Directory, state.Directory, state.HTMLPath, state.HTMLPath,
		state.ManifestPath, state.ManifestPath, archivedAt, updatedAt, key)
	if err != nil {
		return rollback(fmt.Errorf("update article archive %q: %w", key, err))
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return rollback(sql.ErrNoRows)
	}
	var biz string
	if err := tx.QueryRow("SELECT biz FROM mp_articles WHERE article_key = ?", key).Scan(&biz); err != nil {
		return rollback(fmt.Errorf("load archived article account: %w", err))
	}
	if state.Assets != nil {
		if _, err := tx.Exec("DELETE FROM mp_article_assets WHERE article_key = ?", key); err != nil {
			return rollback(fmt.Errorf("replace article assets %q: %w", key, err))
		}
		for _, asset := range state.Assets {
			if err := upsertArticleAssetTx(tx, asset, key, updatedAt); err != nil {
				return rollback(err)
			}
		}
	}
	if _, err := tx.Exec(`UPDATE mp_accounts SET archived_count = (SELECT COUNT(*) FROM mp_articles WHERE biz = ? AND archive_status = ?), updated_at = ? WHERE biz = ?`, biz, officialaccount.ArchiveStatusArchived, updatedAt, biz); err != nil {
		return rollback(fmt.Errorf("refresh archived article count: %w", err))
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit article archive update %q: %w", key, err)
	}
	return nil
}

func (r *OfficialAccountRepository) CreateSyncRun(run officialaccount.SyncRun) error {
	if err := r.ready(); err != nil {
		return err
	}
	if strings.TrimSpace(run.ID) == "" || strings.TrimSpace(run.Biz) == "" {
		return errors.New("sync run id and biz are required")
	}
	if run.StartedAt == 0 {
		run.StartedAt = r.now().Unix()
	}
	if run.PageSize <= 0 {
		run.PageSize = 10
	}
	if run.Status == "" {
		run.Status = officialaccount.SyncStatusQueued
	}
	_, err := r.db.Exec(`
INSERT INTO mp_sync_runs (
    id, biz, mode, status, offset, next_offset, page_size, page_count,
    fetched, inserted, updated, can_continue, started_at, finished_at, error
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, run.ID, run.Biz, run.Mode,
		run.Status, run.Offset, run.NextOffset, run.PageSize, run.PageCount, run.Fetched,
		run.Inserted, run.Updated, boolInt(run.CanContinue), run.StartedAt, nullableUnix(run.FinishedAt), run.Error)
	if err != nil {
		return fmt.Errorf("create sync run %q: %w", run.ID, err)
	}
	if _, err := r.db.Exec(`UPDATE mp_accounts SET sync_status = ?, sync_error = '', updated_at = ? WHERE biz = ?`, run.Status, r.now().Unix(), run.Biz); err != nil {
		return fmt.Errorf("mark account sync queued: %w", err)
	}
	return nil
}

func (r *OfficialAccountRepository) UpdateSyncRun(run officialaccount.SyncRun) error {
	if err := r.ready(); err != nil {
		return err
	}
	finishedAt := nullableUnix(run.FinishedAt)
	_, err := r.db.Exec(`
UPDATE mp_sync_runs SET
    status = ?, offset = ?, next_offset = ?, page_size = ?, page_count = ?,
    fetched = ?, inserted = ?, updated = ?, can_continue = ?, finished_at = ?, error = ?
WHERE id = ?`, run.Status, run.Offset, run.NextOffset, run.PageSize, run.PageCount,
		run.Fetched, run.Inserted, run.Updated, boolInt(run.CanContinue), finishedAt, run.Error, run.ID)
	if err != nil {
		return fmt.Errorf("update sync run %q: %w", run.ID, err)
	}
	terminal := run.Status == officialaccount.SyncStatusCompleted || run.Status == officialaccount.SyncStatusPartial || run.Status == officialaccount.SyncStatusFailed || run.Status == officialaccount.SyncStatusCancelled
	lastSync := interface{}(nil)
	if terminal {
		value := run.FinishedAt
		if value == 0 {
			value = r.now().Unix()
		}
		lastSync = value
	}
	if _, err := r.db.Exec(`
UPDATE mp_accounts SET sync_status = ?, sync_error = ?,
    last_sync_at = COALESCE(?, last_sync_at), updated_at = ?
WHERE biz = ?`, run.Status, run.Error, lastSync, r.now().Unix(), run.Biz); err != nil {
		return fmt.Errorf("update account sync state: %w", err)
	}
	return nil
}

func (r *OfficialAccountRepository) GetSyncRun(id string) (*officialaccount.SyncRun, error) {
	if err := r.ready(); err != nil {
		return nil, err
	}
	return scanSyncRun(r.db.QueryRow(`
SELECT id, biz, mode, status, offset, next_offset, page_size, page_count,
       fetched, inserted, updated, can_continue, started_at, finished_at, error

FROM mp_sync_runs WHERE id = ?`, strings.TrimSpace(id)))
}

func (r *OfficialAccountRepository) GetLatestSyncRun(biz, mode string) (*officialaccount.SyncRun, error) {
	if err := r.ready(); err != nil {
		return nil, err
	}
	query := `
SELECT id, biz, mode, status, offset, next_offset, page_size, page_count,
       fetched, inserted, updated, can_continue, started_at, finished_at, error
FROM mp_sync_runs
WHERE biz = ?`
	args := []interface{}{strings.TrimSpace(biz)}
	if strings.TrimSpace(mode) != "" {
		query += " AND mode = ?"
		args = append(args, strings.TrimSpace(mode))
	}
	query += " ORDER BY started_at DESC, id DESC LIMIT 1"
	return scanSyncRun(r.db.QueryRow(query, args...))
}

func (r *OfficialAccountRepository) CreateMetricSyncRun(run officialaccount.MetricSyncRun) error {
	if err := r.ready(); err != nil {
		return err
	}
	if strings.TrimSpace(run.ID) == "" || strings.TrimSpace(run.Biz) == "" {
		return errors.New("metric sync run id and biz are required")
	}
	if run.StartedAt == 0 {
		run.StartedAt = r.now().Unix()
	}
	if run.Status == "" {
		run.Status = officialaccount.SyncStatusQueued
	}
	_, err := r.db.Exec(`
INSERT INTO mp_metric_sync_runs (
    id, biz, status, force, total, attempted, stored, unknown, failed,
	    after_publish_time, after_article_key, started_at, finished_at, error
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, run.ID, run.Biz, run.Status,
		boolInt(run.Force), run.Total, run.Attempted, run.Stored, run.Unknown, run.Failed,
		run.AfterPublishTime, strings.TrimSpace(run.AfterArticleKey), run.StartedAt,
		nullableUnix(run.FinishedAt), sanitizeCatalogError(run.Error))
	if err != nil {
		return fmt.Errorf("create metric sync run %q: %w", run.ID, err)
	}
	return nil
}

func (r *OfficialAccountRepository) UpdateMetricSyncRun(run officialaccount.MetricSyncRun) error {
	if err := r.ready(); err != nil {
		return err
	}
	_, err := r.db.Exec(`
UPDATE mp_metric_sync_runs SET
    status = ?, force = ?, total = ?, attempted = ?, stored = ?, unknown = ?,
    failed = ?, after_publish_time = ?, after_article_key = ?, finished_at = ?, error = ?
WHERE id = ?`, run.Status, boolInt(run.Force), run.Total, run.Attempted, run.Stored,
		run.Unknown, run.Failed, run.AfterPublishTime, strings.TrimSpace(run.AfterArticleKey),
		nullableUnix(run.FinishedAt), sanitizeCatalogError(run.Error), run.ID)
	if err != nil {
		return fmt.Errorf("update metric sync run %q: %w", run.ID, err)
	}
	return nil
}

func (r *OfficialAccountRepository) GetMetricSyncRun(id string) (*officialaccount.MetricSyncRun, error) {
	if err := r.ready(); err != nil {
		return nil, err
	}
	return scanMetricSyncRun(r.db.QueryRow(`
SELECT id, biz, status, force, total, attempted, stored, unknown, failed,
       after_publish_time, after_article_key, started_at, finished_at, error
FROM mp_metric_sync_runs WHERE id = ?`, strings.TrimSpace(id)))
}

func (r *OfficialAccountRepository) GetLatestMetricSyncRun(biz string) (*officialaccount.MetricSyncRun, error) {
	if err := r.ready(); err != nil {
		return nil, err
	}
	return scanMetricSyncRun(r.db.QueryRow(`
SELECT id, biz, status, force, total, attempted, stored, unknown, failed,
       after_publish_time, after_article_key, started_at, finished_at, error
FROM mp_metric_sync_runs
WHERE biz = ?
ORDER BY started_at DESC, id DESC LIMIT 1`, strings.TrimSpace(biz)))
}

func scanSyncRun(row rowScanner) (*officialaccount.SyncRun, error) {
	var run officialaccount.SyncRun
	var continueFlag int
	var finished sql.NullInt64
	err := row.Scan(
		&run.ID, &run.Biz, &run.Mode, &run.Status, &run.Offset, &run.NextOffset,
		&run.PageSize, &run.PageCount, &run.Fetched, &run.Inserted, &run.Updated,
		&continueFlag, &run.StartedAt, &finished, &run.Error)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan sync run: %w", err)
	}
	run.CanContinue = continueFlag != 0
	if finished.Valid {
		run.FinishedAt = finished.Int64
	}
	return &run, nil
}

func scanArticleMetricState(row rowScanner) (officialaccount.ArticleMetricState, error) {
	var state officialaccount.ArticleMetricState
	var lastAttempt, lastSuccess, lastObserved, nextRetry sql.NullInt64
	if err := row.Scan(&state.ArticleKey, &state.Status, &state.AttemptCount, &state.SuccessCount,
		&state.UnknownCount, &state.FailureCount, &lastAttempt, &lastSuccess, &lastObserved,
		&nextRetry, &state.LastSource, &state.LastError, &state.UpdatedAt); err != nil {
		return officialaccount.ArticleMetricState{}, fmt.Errorf("scan article metric state: %w", err)
	}
	if lastAttempt.Valid {
		state.LastAttemptAt = lastAttempt.Int64
	}
	if lastSuccess.Valid {
		state.LastSuccessAt = lastSuccess.Int64
	}
	if lastObserved.Valid {
		state.LastObservedAt = lastObserved.Int64
	}
	if nextRetry.Valid {
		state.NextRetryAt = nextRetry.Int64
	}
	return state, nil
}

func scanMetricSyncRun(row rowScanner) (*officialaccount.MetricSyncRun, error) {
	var run officialaccount.MetricSyncRun
	var force int
	var finished sql.NullInt64
	if err := row.Scan(&run.ID, &run.Biz, &run.Status, &force, &run.Total, &run.Attempted,
		&run.Stored, &run.Unknown, &run.Failed, &run.AfterPublishTime, &run.AfterArticleKey,
		&run.StartedAt, &finished, &run.Error); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("scan metric sync run: %w", err)
	}
	run.Force = force != 0
	if finished.Valid {
		run.FinishedAt = finished.Int64
	}
	return &run, nil
}

func accountKeywordClause(keyword string) (string, []interface{}) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return "", nil
	}
	pattern := "%" + strings.ToLower(keyword) + "%"
	return "WHERE LOWER(nickname) LIKE ? OR LOWER(biz) LIKE ?", []interface{}{pattern, pattern}
}

func articleWhereClause(query officialaccount.ArticleQuery) (string, []interface{}) {
	clauses := make([]string, 0, 3)
	args := make([]interface{}, 0, 4)
	if biz := strings.TrimSpace(query.Biz); biz != "" {
		clauses = append(clauses, "biz = ?")
		args = append(args, biz)
	}
	if keyword := strings.TrimSpace(query.Keyword); keyword != "" {
		pattern := "%" + strings.ToLower(keyword) + "%"
		clauses = append(clauses, "(LOWER(title) LIKE ? OR LOWER(digest) LIKE ? OR LOWER(author) LIKE ?)")
		args = append(args, pattern, pattern, pattern)
	}
	if status := strings.TrimSpace(query.ArchiveStatus); status != "" {
		clauses = append(clauses, "archive_status = ?")
		args = append(args, status)
	}
	if len(clauses) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanMetricSnapshot(row rowScanner) (officialaccount.ArticleMetricSnapshot, error) {
	var metric officialaccount.ArticleMetricSnapshot
	var view, like, comment, share, collect, reward sql.NullInt64
	if err := row.Scan(&metric.ID, &metric.ArticleKey, &metric.ObservedAt, &metric.Source,
		&view, &like, &comment, &share, &collect, &reward, &metric.RawMetadata); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return officialaccount.ArticleMetricSnapshot{}, err
		}
		return officialaccount.ArticleMetricSnapshot{}, fmt.Errorf("scan article metric snapshot: %w", err)
	}
	metric.ViewCount = nullableInt64Pointer(view)
	metric.LikeCount = nullableInt64Pointer(like)
	metric.CommentCount = nullableInt64Pointer(comment)
	metric.ShareCount = nullableInt64Pointer(share)
	metric.CollectCount = nullableInt64Pointer(collect)
	metric.RewardCount = nullableInt64Pointer(reward)
	return metric, nil
}

func scanArticleAsset(row rowScanner) (officialaccount.ArticleAsset, error) {
	var asset officialaccount.ArticleAsset
	var size sql.NullInt64
	if err := row.Scan(&asset.ArticleKey, &asset.ResourceKey, &asset.Kind, &asset.Role,
		&asset.SourceURL, &asset.LocalPath, &asset.SHA256, &size, &asset.Status,
		&asset.Error, &asset.CreatedAt, &asset.UpdatedAt); err != nil {
		return officialaccount.ArticleAsset{}, fmt.Errorf("scan article asset: %w", err)
	}
	if size.Valid {
		value := size.Int64
		asset.Size = &value
	}
	return officialaccount.SanitizeCatalogAsset(asset), nil
}

func nullableInt64Pointer(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}

func scanArticleRecord(row rowScanner) (officialaccount.ArticleRecord, error) {
	var item officialaccount.ArticleRecord
	var publish, archived sql.NullInt64
	var deleted int
	if err := row.Scan(&item.Key, &item.Biz, &item.Mid, &item.Idx, &item.FileID, &item.Title,
		&item.Digest, &item.Author, &item.VideoID, &item.ContentURL, &item.SourceURL, &item.CoverURL,
		&publish, &item.IsMulti, &item.IsOriginal, &item.IsPaid, &item.IsPaySubscribe,
		&item.ItemShowType, &item.Subtype, &item.CopyrightStat, &item.Duration, &item.AudioFileID,
		&item.PlayURL, &item.MaliciousTitleReasonID, &item.MaliciousContentType, &deleted,
		&item.FirstSeenAt, &item.LastSeenAt, &item.ArchiveStatus,
		&item.ArchiveDir, &item.ArchiveHTML, &item.ArchiveManifest, &archived, &item.RawMetadata); err != nil {
		return officialaccount.ArticleRecord{}, fmt.Errorf("scan official article: %w", err)
	}
	item.SourceDeleted = deleted != 0
	if publish.Valid {
		item.PublishTime = publish.Int64
	}
	if archived.Valid {
		item.ArchivedAt = archived.Int64
	}
	return officialaccount.SanitizeCatalogArticle(item), nil
}

func metricSnapshotHash(metric officialaccount.ArticleMetricSnapshot, observedAt int64, source string) string {
	value := strings.Join([]string{
		metric.ArticleKey,
		strconv.FormatInt(observedAt, 10),
		source,
		metricValueForHash(metric.ViewCount),
		metricValueForHash(metric.LikeCount),
		metricValueForHash(metric.CommentCount),
		metricValueForHash(metric.ShareCount),
		metricValueForHash(metric.CollectCount),
		metricValueForHash(metric.RewardCount),
	}, "\x00")
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func metricValueForHash(value *int64) string {
	if value == nil {
		return "null"
	}
	return strconv.FormatInt(*value, 10)
}

func normalizePage(page, pageSize, defaultSize, maxSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = defaultSize
	}
	if pageSize > maxSize {
		pageSize = maxSize
	}
	return page, pageSize
}

func totalPages(total int64, pageSize int) int {
	if pageSize <= 0 || total == 0 {
		return 0
	}
	return int((total + int64(pageSize) - 1) / int64(pageSize))
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nullableInt64(value *int64) interface{} {
	if value == nil {
		return nil
	}
	return *value
}

func nullableUnix(value int64) interface{} {
	if value == 0 {
		return nil
	}
	return value
}

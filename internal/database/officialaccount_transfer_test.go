package database

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"wx_channel/internal/officialaccount"
)

func TestOfficialAccountCatalogTransferRoundTripAndMetricDeduplication(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	repository := NewOfficialAccountRepository()
	article := officialaccount.ArticleRecord{
		Key:             "article:transfer-1",
		Biz:             "biz-transfer",
		VideoID:         "video-transfer-1",
		Title:           "可迁移文章",
		ContentURL:      "https://mp.weixin.qq.com/s/transfer-1",
		ArchiveStatus:   officialaccount.ArchiveStatusArchived,
		ArchiveDir:      "公众号文章/账号/可迁移文章",
		ArchiveHTML:     "公众号文章/账号/可迁移文章/index.html",
		ArchiveManifest: "公众号文章/账号/可迁移文章/article.json",
	}
	metricValue := int64(123)
	document := officialaccount.CatalogExport{
		FormatVersion: officialaccount.CatalogExportFormatVersion,
		SchemaVersion: 16,
		ExportedAt:    "2026-08-28T00:00:00Z",
		Accounts: []officialaccount.CatalogAccountRecord{{
			Biz:         "biz-transfer",
			Nickname:    "迁移账号",
			AvatarURL:   "https://mmbiz.qpic.cn/avatar.jpg?key=must-not-survive",
			IsEffective: true,
		}},
		Articles: []officialaccount.ArticleRecord{article},
		Assets: []officialaccount.ArticleAsset{{
			ArticleKey:  article.Key,
			ResourceKey: "inline_image:1",
			Kind:        officialaccount.ArchiveResourceKindImage,
			Role:        officialaccount.ArchiveResourceRoleAttachment,
			SourceURL:   "https://mmbiz.qpic.cn/image.jpg?appmsg_token=must-not-survive&wx_fmt=jpeg",
			LocalPath:   "公众号文章/账号/可迁移文章/assets/image.jpg",
			SHA256:      strings.Repeat("a", 64),
			Size:        &metricValue,
			Status:      "downloaded",
		}},
		Metrics: []officialaccount.ArticleMetricSnapshot{{
			ArticleKey: article.Key,
			ObservedAt: 100,
			Source:     "article_page",
			LikeCount:  &metricValue,
		}},
		MetricStates: []officialaccount.ArticleMetricState{{
			ArticleKey:     article.Key,
			Status:         officialaccount.MetricStateSuccess,
			AttemptCount:   1,
			SuccessCount:   1,
			LastSuccessAt:  100,
			LastObservedAt: 100,
			LastSource:     "article_page",
			UpdatedAt:      100,
		}},
	}

	summary, err := repository.ImportCatalog(context.Background(), document, officialaccount.CatalogImportOptions{ConflictPolicy: officialaccount.CatalogConflictMerge})
	if err != nil {
		t.Fatalf("import catalog: %v", err)
	}
	if summary.AccountsAdded != 1 || summary.ArticlesAdded != 1 || summary.AssetsAdded != 1 || summary.MetricsAdded != 1 || summary.MetricStatesAdded != 1 {
		t.Fatalf("unexpected first import summary: %+v", summary)
	}

	if err := repository.RecordArticleMetrics(document.Metrics); err != nil {
		t.Fatalf("repeat metric insert: %v", err)
	}
	var metricCount int
	if err := repository.db.QueryRow("SELECT COUNT(*) FROM mp_article_metric_snapshots WHERE article_key = ?", article.Key).Scan(&metricCount); err != nil {
		t.Fatalf("count metrics: %v", err)
	}
	if metricCount != 1 {
		t.Fatalf("duplicate metric snapshot was stored: %d", metricCount)
	}

	second, err := repository.ImportCatalog(context.Background(), document, officialaccount.CatalogImportOptions{ConflictPolicy: officialaccount.CatalogConflictMerge})
	if err != nil {
		t.Fatalf("repeat catalog import: %v", err)
	}
	if second.Conflicts != 5 || second.AccountsUpdated != 1 || second.ArticlesUpdated != 1 || second.AssetsUpdated != 1 || second.MetricsSkipped != 1 || second.MetricStatesSkipped != 1 {
		t.Fatalf("unexpected repeat import summary: %+v", second)
	}

	var exported bytes.Buffer
	stats, err := repository.ExportCatalog(context.Background(), &exported)
	if err != nil {
		t.Fatalf("export catalog: %v", err)
	}
	if stats.Accounts != 1 || stats.Articles != 1 || stats.Assets != 1 || stats.Metrics != 1 || stats.MetricStates != 1 {
		t.Fatalf("unexpected export stats: %+v", stats)
	}
	if strings.Contains(exported.String(), "must-not-survive") || strings.Contains(exported.String(), "appmsg_token") {
		t.Fatalf("catalog export leaked credential-bearing metadata: %s", exported.String())
	}
	var decoded officialaccount.CatalogExport
	if err := json.Unmarshal(exported.Bytes(), &decoded); err != nil {
		t.Fatalf("decode exported catalog: %v", err)
	}
	if len(decoded.Accounts) != 1 || decoded.Accounts[0].IsEffective || len(decoded.MetricStates) != 1 || decoded.MetricStates[0].Status != officialaccount.MetricStateSuccess {
		t.Fatalf("exported account should be credential-free: %+v", decoded.Accounts)
	}
	if len(decoded.Articles) != 1 || decoded.Articles[0].VideoID != "video-transfer-1" {
		t.Fatalf("exported article lost video identity: %+v", decoded.Articles)
	}
}

func TestOfficialAccountCatalogImportDryRunDoesNotWrite(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	repository := NewOfficialAccountRepository()
	document := officialaccount.CatalogExport{
		FormatVersion: officialaccount.CatalogExportFormatVersion,
		SchemaVersion: 16,
		Accounts:      []officialaccount.CatalogAccountRecord{{Biz: "biz-dry-run", Nickname: "预检账号"}},
	}
	summary, err := repository.ImportCatalog(context.Background(), document, officialaccount.CatalogImportOptions{
		DryRun:         true,
		ConflictPolicy: officialaccount.CatalogConflictMerge,
	})
	if err != nil {
		t.Fatalf("dry-run import: %v", err)
	}
	if !summary.DryRun || summary.AccountsSeen != 1 || summary.AccountsAdded != 1 {
		t.Fatalf("unexpected dry-run summary: %+v", summary)
	}
	var count int
	if err := repository.db.QueryRow("SELECT COUNT(*) FROM mp_accounts WHERE biz = ?", "biz-dry-run").Scan(&count); err != nil {
		t.Fatalf("check dry-run account: %v", err)
	}
	if count != 0 {
		t.Fatalf("dry-run changed the database: %d", count)
	}
}

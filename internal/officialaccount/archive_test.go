package officialaccount

import (
	"errors"
	"strings"
	"testing"
)

func TestBuildArticleArchivePlanBuildsHTMLImagesAndRelations(t *testing.T) {
	article := ArticleItem{
		Title:                  "  文章标题  ",
		Digest:                 "文章摘要",
		ContentURL:             "https://mp.weixin.qq.com/s/article-1?__biz=biz-1&mid=7&idx=1",
		SourceURL:              "https://example.com/source/article-1",
		Cover:                  "//mmbiz.qpic.cn/cover/1",
		Author:                 "作者",
		PublishTime:            1720000000,
		Mid:                    "mid-7",
		Idx:                    2,
		FileID:                 77,
		IsMulti:                1,
		IsOriginal:             1,
		IsPaid:                 1,
		IsPaySubscribe:         1,
		ItemShowType:           9,
		Subtype:                7,
		CopyrightStat:          1,
		Duration:               42,
		AudioFileID:            23,
		PlayURL:                "https://vd.example.test/video.mp4?token=short-lived",
		MaliciousTitleReasonID: 2,
		MaliciousContentType:   3,
	}
	pageHTML := `<html><body><div id="js_content"><p>正文</p><img data-src="//mmbiz.qpic.cn/image/1?wx_fmt=jpeg"><p><img src="https://mmbiz.qpic.cn/image/2"></p></div></body></html>`

	plan, err := BuildArticleArchivePlan("biz-1", article, pageHTML)
	if err != nil {
		t.Fatalf("BuildArticleArchivePlan() error = %v", err)
	}

	if plan.Content.Key == "" || !strings.HasPrefix(plan.Content.Key, "article:") {
		t.Fatalf("unexpected content key: %q", plan.Content.Key)
	}
	if plan.Content.Biz != "biz-1" {
		t.Fatalf("content biz = %q, want biz-1", plan.Content.Biz)
	}
	if plan.Content.Title != "文章标题" || plan.Content.Description != "文章摘要" || plan.Content.Author != "作者" {
		t.Fatalf("unexpected content metadata: %+v", plan.Content)
	}
	if plan.Content.URL != article.ContentURL || plan.Content.SourceURL != article.SourceURL {
		t.Fatalf("unexpected content URLs: %+v", plan.Content)
	}
	if plan.Content.CoverURL != "https://mmbiz.qpic.cn/cover/1" {
		t.Fatalf("cover URL = %q, want normalized HTTPS URL", plan.Content.CoverURL)
	}
	if plan.Content.PublishTime != article.PublishTime {
		t.Fatalf("publish time = %d, want %d", plan.Content.PublishTime, article.PublishTime)
	}
	if plan.Content.Mid != article.Mid || plan.Content.Idx != article.Idx || plan.Content.FileID != article.FileID ||
		plan.Content.IsMulti != article.IsMulti || plan.Content.IsOriginal != article.IsOriginal ||
		plan.Content.IsPaid != article.IsPaid || plan.Content.IsPaySubscribe != article.IsPaySubscribe ||
		plan.Content.ItemShowType != article.ItemShowType || plan.Content.Subtype != article.Subtype ||
		plan.Content.CopyrightStat != article.CopyrightStat || plan.Content.Duration != article.Duration ||
		plan.Content.AudioFileID != article.AudioFileID || plan.Content.PlayURL != article.PlayURL ||
		plan.Content.MaliciousTitleReasonID != article.MaliciousTitleReasonID ||
		plan.Content.MaliciousContentType != article.MaliciousContentType {
		t.Fatalf("article media metadata was not retained in archive plan: %+v", plan.Content)
	}

	if len(plan.Resources) != 3 {
		t.Fatalf("resource count = %d, want 3", len(plan.Resources))
	}
	body := plan.Resources[0]
	if body.Key != "body:html" || body.Kind != ArchiveResourceKindHTML || body.Role != ArchiveResourceRoleArticleBody {
		t.Fatalf("unexpected HTML resource: %+v", body)
	}
	if body.Name != "文章标题" || body.MIMEType != "text/html" || body.SourceURL != article.ContentURL {
		t.Fatalf("unexpected HTML resource metadata: %+v", body)
	}
	if !strings.Contains(body.InlineBody, "正文") || !strings.Contains(body.InlineBody, `src="https://mmbiz.qpic.cn/image/1?wx_fmt=jpeg"`) {
		t.Fatalf("HTML resource did not preserve normalized article body: %s", body.InlineBody)
	}
	if strings.Contains(body.InlineBody, "data-src=") {
		t.Fatalf("HTML resource retained lazy image attribute: %s", body.InlineBody)
	}

	wantImages := []string{
		"https://mmbiz.qpic.cn/image/1?wx_fmt=jpeg",
		"https://mmbiz.qpic.cn/image/2",
	}
	for i, wantURL := range wantImages {
		resource := plan.Resources[i+1]
		if resource.Kind != ArchiveResourceKindImage || resource.Role != ArchiveResourceRoleAttachment {
			t.Fatalf("unexpected image resource %d: %+v", i, resource)
		}
		if resource.SourceURL != wantURL {
			t.Fatalf("image %d URL = %q, want %q", i, resource.SourceURL, wantURL)
		}
		if resource.Key == "" || resource.Name == "" || resource.SortOrder != 100+i {
			t.Fatalf("unexpected image resource %d identity/order: %+v", i, resource)
		}
	}

	if len(plan.Relations) != len(plan.Resources) {
		t.Fatalf("relation count = %d, want %d", len(plan.Relations), len(plan.Resources))
	}
	for i, relation := range plan.Relations {
		if relation.SourceKey != plan.Content.Key || relation.TargetKey != plan.Resources[i].Key || relation.Type != ArchiveRelationContains {
			t.Fatalf("unexpected relation %d: %+v", i, relation)
		}
		if relation.SortOrder != plan.Resources[i].SortOrder {
			t.Fatalf("relation %d sort order = %d, want %d", i, relation.SortOrder, plan.Resources[i].SortOrder)
		}
	}
}

func TestEnsureArticleArchiveRecordRetainsMediaMetadata(t *testing.T) {
	service := NewMemoryService()
	store := newSyncCatalogStub()
	if err := service.SetCatalogRepository(store); err != nil {
		t.Fatalf("set catalog repository: %v", err)
	}

	plan := ArchivePlan{Content: ArchiveContent{
		Key:                    "article:archive-media",
		Biz:                    "biz-archive-media",
		Mid:                    "mid-archive-media",
		Idx:                    2,
		FileID:                 77,
		Title:                  "视频文章",
		Description:            "摘要",
		Author:                 "作者",
		URL:                    "https://mp.weixin.qq.com/s/media?__biz=biz-archive-media&mid=7&idx=2",
		Duration:               42,
		AudioFileID:            23,
		PlayURL:                "https://vd.example.test/video.mp4?token=short-lived",
		Subtype:                7,
		CopyrightStat:          1,
		MaliciousTitleReasonID: 2,
		MaliciousContentType:   3,
	}}
	if err := service.EnsureArticleArchiveRecord(plan); err != nil {
		t.Fatalf("ensure archive record: %v", err)
	}

	store.mu.Lock()
	record := store.articles[plan.Content.Key]
	store.mu.Unlock()
	if record.Duration != 42 || record.AudioFileID != 23 || record.PlayURL != "https://vd.example.test/video.mp4" ||
		record.Subtype != 7 || record.CopyrightStat != 1 || record.Mid != "mid-archive-media" || record.Idx != 2 {
		t.Fatalf("archive record lost media metadata: %+v", record)
	}
}

func TestBuildArticleArchivePlanDeduplicatesImagesAndKeepsStableKeys(t *testing.T) {
	articleA := ArticleItem{
		Title:       "原始标题",
		ContentURL:  "https://mp.weixin.qq.com/s/article-2?__biz=biz-2&mid=8&idx=1&key=first&appmsg_token=token-a",
		PublishTime: 1720000000,
	}
	articleB := articleA
	articleB.Title = "后来标题"
	articleB.ContentURL = "https://mp.weixin.qq.com/s/article-2?appmsg_token=token-b&idx=1&mid=8&key=second&__biz=biz-2"
	pageHTML := `<html><body><div id="js_content"><img data-src="//mmbiz.qpic.cn/image/1?wx_fmt=jpeg&amp;v=1"><img src="https://mmbiz.qpic.cn/image/1?wx_fmt=jpeg&amp;v=1"><img src="data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yw="><img data-src="https://mmbiz.qpic.cn/image/2"></div></body></html>`

	planA, err := BuildArticleArchivePlan("biz-2", articleA, pageHTML)
	if err != nil {
		t.Fatalf("BuildArticleArchivePlan(articleA) error = %v", err)
	}
	planB, err := BuildArticleArchivePlan("biz-2", articleB, pageHTML)
	if err != nil {
		t.Fatalf("BuildArticleArchivePlan(articleB) error = %v", err)
	}

	if planA.Content.Key != planB.Content.Key {
		t.Fatalf("article key changed with credential/query ordering: %q != %q", planA.Content.Key, planB.Content.Key)
	}
	if len(planA.Resources) != 3 {
		t.Fatalf("resource count = %d, want body plus two unique remote images", len(planA.Resources))
	}
	if planA.Resources[1].SourceURL != "https://mmbiz.qpic.cn/image/1?wx_fmt=jpeg&v=1" || planA.Resources[2].SourceURL != "https://mmbiz.qpic.cn/image/2" {
		t.Fatalf("unexpected deduplicated image URLs: %+v", planA.Resources)
	}
	if planA.Resources[1].Key != planB.Resources[1].Key || planA.Resources[2].Key != planB.Resources[2].Key {
		t.Fatalf("image keys are not stable: A=%+v B=%+v", planA.Resources, planB.Resources)
	}
}

func TestBuildArticleArchivePlanUsesStableWeChatArticleIdentity(t *testing.T) {
	articleA := ArticleItem{
		Title:       "同一篇文章",
		ContentURL:  "https://mp.weixin.qq.com/s/first-path?__biz=biz-stable&mid=12345&idx=1&sn=stable-sn&scene=125&chksm=first&sessionid=first",
		PublishTime: 1720000000,
	}
	articleB := articleA
	articleB.ContentURL = "https://mp.weixin.qq.com/s/second-path?sessionid=second&idx=1&mid=12345&scene=142&chksm=second&sn=stable-sn"
	pageHTML := `<div id="js_content"><p>正文</p></div>`

	planA, err := BuildArticleArchivePlan("biz-stable", articleA, pageHTML)
	if err != nil {
		t.Fatalf("BuildArticleArchivePlan(articleA) error = %v", err)
	}
	planB, err := BuildArticleArchivePlan("biz-stable", articleB, pageHTML)
	if err != nil {
		t.Fatalf("BuildArticleArchivePlan(articleB) error = %v", err)
	}
	if planA.Content.Key != planB.Content.Key {
		t.Fatalf("same WeChat article changed key with path/dynamic query changes: %q != %q", planA.Content.Key, planB.Content.Key)
	}

	articleB.ContentURL = "https://mp.weixin.qq.com/s/second-path?mid=12345&idx=2&sn=stable-sn&scene=142"
	planDifferentIndex, err := BuildArticleArchivePlan("biz-stable", articleB, pageHTML)
	if err != nil {
		t.Fatalf("BuildArticleArchivePlan(articleB idx=2) error = %v", err)
	}
	if planA.Content.Key == planDifferentIndex.Content.Key {
		t.Fatalf("different WeChat article index was merged: %q", planA.Content.Key)
	}
}

func TestBuildArticleArchivePlanUsesStableSNWhenMIDIsMissing(t *testing.T) {
	articleA := ArticleItem{
		Title:      "仅有 SN 的文章",
		ContentURL: "https://mp.weixin.qq.com/s/first?sn=stable-sn&scene=125&chksm=first&sessionid=first",
	}
	articleB := articleA
	articleB.ContentURL = "https://mp.weixin.qq.com/s/second?sessionid=second&sn=stable-sn&scene=142&chksm=second"
	pageHTML := `<div id="js_content"><p>正文</p></div>`

	planA, err := BuildArticleArchivePlan("biz-sn", articleA, pageHTML)
	if err != nil {
		t.Fatalf("BuildArticleArchivePlan(articleA) error = %v", err)
	}
	planB, err := BuildArticleArchivePlan("biz-sn", articleB, pageHTML)
	if err != nil {
		t.Fatalf("BuildArticleArchivePlan(articleB) error = %v", err)
	}
	if planA.Content.Key != planB.Content.Key {
		t.Fatalf("same SN changed key with path/dynamic query changes: %q != %q", planA.Content.Key, planB.Content.Key)
	}
}

func TestBuildArticleArchivePlanUsesFileIDFallback(t *testing.T) {
	plan, err := BuildArticleArchivePlan("biz-3", ArticleItem{
		Title:  "无 URL 文章",
		FileID: 42,
	}, `<html><body><div id="js_content"><p>正文</p></div></body></html>`)
	if err != nil {
		t.Fatalf("BuildArticleArchivePlan() error = %v", err)
	}
	if plan.Content.Key == "" || !strings.HasPrefix(plan.Content.Key, "article:") {
		t.Fatalf("unexpected fallback content key: %q", plan.Content.Key)
	}
}

func TestBuildArticleArchivePlanRejectsMissingIdentityOrBody(t *testing.T) {
	validBody := `<html><body><div id="js_content"><p>正文</p></div></body></html>`

	_, err := BuildArticleArchivePlan("biz-4", ArticleItem{Title: "缺少身份"}, validBody)
	if !errors.Is(err, ErrArticleIdentity) {
		t.Fatalf("missing identity error = %v, want ErrArticleIdentity", err)
	}

	article := ArticleItem{ContentURL: "https://mp.weixin.qq.com/s/article-4"}
	_, err = BuildArticleArchivePlan("biz-4", article, "")
	if !errors.Is(err, ErrContentNotFound) {
		t.Fatalf("missing HTML error = %v, want ErrContentNotFound", err)
	}

	_, err = BuildArticleArchivePlan("biz-4", article, `<html><body><p>没有正文节点</p></body></html>`)
	if !errors.Is(err, ErrContentNotFound) {
		t.Fatalf("missing content node error = %v, want ErrContentNotFound", err)
	}
}

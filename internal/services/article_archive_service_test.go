package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"wx_channel/internal/officialaccount"
)

type fakeArchiveDownloader struct {
	calls  []string
	failed map[string]error
}

type parallelArchiveDownloader struct {
	entered chan struct{}
	release chan struct{}
	mu      sync.Mutex
	active  int
	max     int
}

func (d *parallelArchiveDownloader) DownloadSync(_ context.Context, sourceURL, target string, _ int, _ map[string]string, _ func(float64, int64, int64)) (string, error) {
	d.mu.Lock()
	d.active++
	if d.active > d.max {
		d.max = d.active
	}
	d.mu.Unlock()
	defer func() {
		d.mu.Lock()
		d.active--
		d.mu.Unlock()
	}()
	d.entered <- struct{}{}
	<-d.release
	if err := os.WriteFile(target, []byte("image:"+sourceURL), 0644); err != nil {
		return "", err
	}
	return target, nil
}

func (d *fakeArchiveDownloader) DownloadSync(_ context.Context, sourceURL, target string, _ int, headers map[string]string, _ func(float64, int64, int64)) (string, error) {
	d.calls = append(d.calls, sourceURL+"|"+headers["Referer"])
	if err := d.failed[sourceURL]; err != nil {
		return target, err
	}
	if err := os.WriteFile(target, []byte("image:"+sourceURL), 0644); err != nil {
		return "", err
	}
	return filepath.ToSlash(target), nil
}

func TestArticleArchiveDownloadPersistsLocalizedHTMLAndManifest(t *testing.T) {
	plan, err := officialaccount.BuildArticleArchivePlan("biz-1", officialaccount.ArticleItem{
		Title:      "归档文章",
		Author:     "文章作者",
		ContentURL: "https://mp.weixin.qq.com/s/article-1",
	}, `<html><body><div id="js_content"><p>正文</p><img data-src="https://mmbiz.qpic.cn/image/1?wx_fmt=jpeg"><img src="https://mmbiz.qpic.cn/image/2.png"></div></body></html>`)
	if err != nil {
		t.Fatalf("build archive plan: %v", err)
	}

	downloader := &fakeArchiveDownloader{failed: map[string]error{}}
	service := NewArticleArchiveDownloadService(downloader)
	result, err := service.Download(context.Background(), t.TempDir(), plan, map[string]string{"Referer": plan.Content.URL}, false)
	if err != nil {
		t.Fatalf("download archive: %v", err)
	}
	if result.Downloaded != 2 || result.Skipped != 0 || result.Failed != 0 {
		t.Fatalf("unexpected download counters: %+v", result)
	}
	if len(result.Assets) != 3 || len(result.HTMLSHA256) != 64 || result.HTMLSize == nil || *result.HTMLSize <= 0 {
		t.Fatalf("archive metadata is incomplete: %+v", result)
	}
	for _, asset := range result.Assets {
		if asset.Status != "downloaded" || len(asset.SHA256) != 64 || asset.Size == nil || *asset.Size <= 0 {
			t.Fatalf("archive asset metadata is incomplete: %+v", asset)
		}
	}
	if len(downloader.calls) != 2 || !strings.Contains(downloader.calls[0], plan.Content.URL) {
		t.Fatalf("unexpected downloader calls: %+v", downloader.calls)
	}

	htmlData, err := os.ReadFile(result.HTMLPath)
	if err != nil {
		t.Fatalf("read localized HTML: %v", err)
	}
	htmlText := string(htmlData)
	if strings.Contains(htmlText, "mmbiz.qpic.cn") || !strings.Contains(htmlText, `src="assets/`) {
		t.Fatalf("HTML was not localized: %s", htmlText)
	}

	manifestData, err := os.ReadFile(result.ManifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	manifestText := string(manifestData)
	for _, required := range []string{`"content"`, `"resources"`, `"relations"`, `"downloaded"`, `"role"`, `"sha256"`, `"size"`, `"assets/`} {
		if !strings.Contains(manifestText, required) {
			t.Fatalf("manifest missing %s: %s", required, manifestText)
		}
	}
	if strings.Contains(manifestText, "正文") {
		t.Fatalf("manifest duplicated inline article HTML: %s", manifestText)
	}
}

func TestArticleArchiveDownloadKeepsFailedImagesRemoteAndSkipsExisting(t *testing.T) {
	plan, err := officialaccount.BuildArticleArchivePlan("biz-2", officialaccount.ArticleItem{
		Title:      "部分成功",
		ContentURL: "https://mp.weixin.qq.com/s/article-2",
	}, `<div id="js_content"><img src="https://mmbiz.qpic.cn/image/ok.png"><img src="https://mmbiz.qpic.cn/image/fail.png"></div>`)
	if err != nil {
		t.Fatalf("build archive plan: %v", err)
	}

	downloader := &fakeArchiveDownloader{failed: map[string]error{
		"https://mmbiz.qpic.cn/image/fail.png": errors.New("upstream image unavailable"),
	}}
	root := t.TempDir()
	service := NewArticleArchiveDownloadService(downloader)
	first, err := service.Download(context.Background(), root, plan, nil, false)
	if err != nil {
		t.Fatalf("first archive download: %v", err)
	}
	if first.Downloaded != 1 || first.Failed != 1 {
		t.Fatalf("unexpected first counters: %+v", first)
	}

	second, err := service.Download(context.Background(), root, plan, nil, false)
	if err != nil {
		t.Fatalf("second archive download: %v", err)
	}
	if second.Skipped != 1 || second.Failed != 1 || second.Downloaded != 0 {
		t.Fatalf("unexpected repeated counters: %+v", second)
	}
	if len(second.Assets) != 3 || second.Assets[1].Status != "skipped" || second.Assets[1].Size == nil || len(second.Assets[1].SHA256) != 64 {
		t.Fatalf("repeated archive did not retain verified asset metadata: %+v", second.Assets)
	}
	if len(downloader.calls) != 3 {
		t.Fatalf("existing image should be skipped, calls = %d (%+v)", len(downloader.calls), downloader.calls)
	}

	htmlData, err := os.ReadFile(filepath.Join(first.Directory, "index.html"))
	if err != nil {
		t.Fatalf("read partial HTML: %v", err)
	}
	htmlText := string(htmlData)
	if !strings.Contains(htmlText, "assets/") || !strings.Contains(htmlText, "https://mmbiz.qpic.cn/image/fail.png") {
		t.Fatalf("partial HTML did not preserve failed image source: %s", htmlText)
	}
}

func TestArticleArchiveDownloadReusesDirectoryForWeChatURLVariants(t *testing.T) {
	pageHTML := `<div id="js_content"><p>正文</p><img src="https://mmbiz.qpic.cn/image/same.png"></div>`
	articleA := officialaccount.ArticleItem{
		Title:       "重复文章",
		ContentURL:  "https://mp.weixin.qq.com/s/first-path?__biz=biz-variant&mid=54321&idx=1&scene=125&chksm=first&sessionid=first",
		PublishTime: 1720000000,
	}
	articleB := articleA
	articleB.ContentURL = "https://mp.weixin.qq.com/s/second-path?mid=54321&idx=1&scene=142&chksm=second&sessionid=second"
	planA, err := officialaccount.BuildArticleArchivePlan("biz-variant", articleA, pageHTML)
	if err != nil {
		t.Fatalf("build first archive plan: %v", err)
	}
	planB, err := officialaccount.BuildArticleArchivePlan("biz-variant", articleB, pageHTML)
	if err != nil {
		t.Fatalf("build second archive plan: %v", err)
	}

	downloader := &fakeArchiveDownloader{failed: map[string]error{}}
	root := t.TempDir()
	service := NewArticleArchiveDownloadService(downloader)
	first, err := service.Download(context.Background(), root, planA, nil, false)
	if err != nil {
		t.Fatalf("first archive download: %v", err)
	}
	second, err := service.Download(context.Background(), root, planB, nil, false)
	if err != nil {
		t.Fatalf("second archive download: %v", err)
	}
	if first.Directory != second.Directory {
		t.Fatalf("same WeChat article created different directories: %q != %q", first.Directory, second.Directory)
	}
	if first.Downloaded != 1 || second.Downloaded != 0 || second.Skipped != 1 || second.Failed != 0 {
		t.Fatalf("unexpected repeated download counters: first=%+v second=%+v", first, second)
	}
	if len(downloader.calls) != 1 {
		t.Fatalf("same WeChat image was downloaded more than once: calls=%d (%+v)", len(downloader.calls), downloader.calls)
	}
}

func TestArticleArchiveDownloadAllowsDifferentArticlesInParallel(t *testing.T) {
	makePlan := func(title, path string) officialaccount.ArchivePlan {
		plan, err := officialaccount.BuildArticleArchivePlan("biz-parallel", officialaccount.ArticleItem{
			Title:      title,
			ContentURL: "https://mp.weixin.qq.com/s/" + path,
		}, `<div id="js_content"><img src="https://mmbiz.qpic.cn/image/`+path+`.png"></div>`)
		if err != nil {
			t.Fatalf("build %s plan: %v", title, err)
		}
		return plan
	}

	downloader := &parallelArchiveDownloader{entered: make(chan struct{}, 2), release: make(chan struct{})}
	service := NewArticleArchiveDownloadService(downloader)
	root := t.TempDir()
	plans := []officialaccount.ArchivePlan{makePlan("并行一", "one"), makePlan("并行二", "two")}
	errorsCh := make(chan error, len(plans))
	var group sync.WaitGroup
	for _, plan := range plans {
		plan := plan
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := service.Download(context.Background(), root, plan, nil, false)
			errorsCh <- err
		}()
	}
	select {
	case <-downloader.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("first article did not reach downloader")
	}
	select {
	case <-downloader.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("different article was blocked by a global archive lock")
	}
	close(downloader.release)
	group.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatalf("parallel archive failed: %v", err)
		}
	}
	downloader.mu.Lock()
	maxActive := downloader.max
	downloader.mu.Unlock()
	if maxActive < 2 {
		t.Fatalf("different articles were not downloaded concurrently: max active=%d", maxActive)
	}
}

func TestArticleArchiveDownloadDoesNotPersistImageCredentials(t *testing.T) {
	secretURL := "https://mmbiz.qpic.cn/image/secret.png?wx_fmt=png&key=secret-key&appmsg_token=secret-token"
	plan, err := officialaccount.BuildArticleArchivePlan("biz-secret", officialaccount.ArticleItem{
		Title:      "凭据脱敏",
		ContentURL: "https://mp.weixin.qq.com/s/article-secret?key=article-key",
	}, `<div id="js_content"><img src="`+secretURL+`"></div>`)
	if err != nil {
		t.Fatalf("build archive plan: %v", err)
	}

	downloader := &fakeArchiveDownloader{failed: map[string]error{secretURL: errors.New("upstream image unavailable")}}
	root := t.TempDir()
	service := NewArticleArchiveDownloadService(downloader)
	result, err := service.Download(context.Background(), root, plan, nil, false)
	if err != nil {
		t.Fatalf("download archive: %v", err)
	}

	manifestData, err := os.ReadFile(result.ManifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	manifestText := string(manifestData)
	for _, secret := range []string{"secret-key", "secret-token", "article-key"} {
		if strings.Contains(manifestText, secret) {
			t.Fatalf("manifest leaked %q: %s", secret, manifestText)
		}
	}
	htmlData, err := os.ReadFile(result.HTMLPath)
	if err != nil {
		t.Fatalf("read HTML: %v", err)
	}
	htmlText := string(htmlData)
	if strings.Contains(htmlText, "secret-key") || strings.Contains(htmlText, "secret-token") {
		t.Fatalf("HTML fallback leaked image credentials: %s", htmlText)
	}
	if !strings.Contains(htmlText, "wx_fmt=png") {
		t.Fatalf("HTML fallback lost non-sensitive image query: %s", htmlText)
	}
	if len(result.Files) != 1 || strings.Contains(result.Files[0].SourceURL, "secret-") || strings.Contains(result.Files[0].Error, "secret-") {
		t.Fatalf("download result leaked image credentials: %+v", result.Files)
	}
}

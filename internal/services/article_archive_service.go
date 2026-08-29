package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	htmlstd "html"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"

	"wx_channel/internal/officialaccount"
	"wx_channel/internal/utils"
)

const defaultArchiveConnections = 8

// ArchiveFileDownloader is the small part of Gopeed needed by article
// archives. Keeping the interface here makes the archive flow testable without
// starting the full downloader engine.
type ArchiveFileDownloader interface {
	DownloadSync(context.Context, string, string, int, map[string]string, func(float64, int64, int64)) (string, error)
}

// ArticleArchiveDownloadService owns article-specific persistence and maps the
// content/resource/relationship plan onto the existing file downloader.
type ArticleArchiveDownloadService struct {
	downloader   ArchiveFileDownloader
	connections  int
	mu           sync.Mutex
	articleLocks map[string]*articleArchiveLock
}

type articleArchiveLock struct {
	mu   sync.Mutex
	refs int
}

type ArticleArchiveFileResult struct {
	ResourceKey  string `json:"resource_key"`
	Kind         string `json:"kind"`
	Role         string `json:"role"`
	SourceURL    string `json:"source_url,omitempty"`
	Status       string `json:"status"`
	Path         string `json:"path,omitempty"`
	RelativePath string `json:"relative_path,omitempty"`
	SHA256       string `json:"sha256,omitempty"`
	Size         *int64 `json:"size,omitempty"`
	Error        string `json:"error,omitempty"`
}

type ArticleArchiveDownloadResult struct {
	ContentKey           string                         `json:"content_key"`
	Directory            string                         `json:"directory"`
	RelativeDirectory    string                         `json:"relative_directory"`
	HTMLPath             string                         `json:"html_path"`
	RelativeHTMLPath     string                         `json:"relative_html_path"`
	ManifestPath         string                         `json:"manifest_path"`
	RelativeManifestPath string                         `json:"relative_manifest_path"`
	HTMLSHA256           string                         `json:"html_sha256,omitempty"`
	HTMLSize             *int64                         `json:"html_size,omitempty"`
	Files                []ArticleArchiveFileResult     `json:"files"`
	Assets               []officialaccount.ArticleAsset `json:"assets"`
	Downloaded           int                            `json:"downloaded"`
	Skipped              int                            `json:"skipped"`
	Failed               int                            `json:"failed"`
}

type articleArchiveManifestFile struct {
	ResourceKey  string `json:"resource_key"`
	Kind         string `json:"kind"`
	Role         string `json:"role"`
	SourceURL    string `json:"source_url,omitempty"`
	Status       string `json:"status"`
	RelativePath string `json:"relative_path,omitempty"`
	SHA256       string `json:"sha256,omitempty"`
	Size         *int64 `json:"size,omitempty"`
	Error        string `json:"error,omitempty"`
}

type articleArchiveManifest struct {
	Content   officialaccount.ArchiveContent    `json:"content"`
	Resources []officialaccount.ArchiveResource `json:"resources"`
	Relations []officialaccount.ArchiveRelation `json:"relations"`
	Files     []articleArchiveManifestFile      `json:"files"`
	HTMLPath  string                            `json:"html_path"`
	SavedAt   string                            `json:"saved_at"`
}

func NewArticleArchiveDownloadService(downloader ArchiveFileDownloader) *ArticleArchiveDownloadService {
	return &ArticleArchiveDownloadService{
		downloader:   downloader,
		connections:  defaultArchiveConnections,
		articleLocks: make(map[string]*articleArchiveLock),
	}
}

func (s *ArticleArchiveDownloadService) SetConnections(connections int) {
	if s == nil || connections <= 0 {
		return
	}
	s.mu.Lock()
	s.connections = connections
	s.mu.Unlock()
}

func (s *ArticleArchiveDownloadService) archiveConnections() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.connections <= 0 {
		return defaultArchiveConnections
	}
	return s.connections
}

func (s *ArticleArchiveDownloadService) acquireArticleLock(key string) func() {
	s.mu.Lock()
	if s.articleLocks == nil {
		s.articleLocks = make(map[string]*articleArchiveLock)
	}
	lock := s.articleLocks[key]
	if lock == nil {
		lock = &articleArchiveLock{}
		s.articleLocks[key] = lock
	}
	lock.refs++
	s.mu.Unlock()

	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()
		s.mu.Lock()
		lock.refs--
		if lock.refs == 0 && s.articleLocks[key] == lock {
			delete(s.articleLocks, key)
		}
		s.mu.Unlock()
	}
}

// Download persists an article archive under root. It intentionally does not
// touch video records or the video queue; only image resources are delegated
// to Gopeed.
func (s *ArticleArchiveDownloadService) Download(ctx context.Context, root string, plan officialaccount.ArchivePlan, headers map[string]string, force bool) (*ArticleArchiveDownloadResult, error) {
	if s == nil || s.downloader == nil {
		return nil, errors.New("article archive downloader is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("article archive root is empty")
	}

	body, err := articleArchiveBodyResource(plan)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(plan.Content.Key) == "" {
		return nil, errors.New("article archive content key is empty")
	}

	// A deterministic article directory makes a repeated click idempotent and
	// keeps the stable content key available for later indexing. Only repeated
	// work for the same article is serialized; different articles can proceed
	// concurrently.
	releaseArticleLock := s.acquireArticleLock(plan.Content.Key)
	defer releaseArticleLock()

	root, err = filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve article archive root: %w", err)
	}
	contentDigest := strings.TrimPrefix(plan.Content.Key, "article:")
	if len(contentDigest) > 12 {
		contentDigest = contentDigest[:12]
	}
	if contentDigest == "" {
		contentDigest = "archive"
	}
	bizName := archivePathSegment(plan.Content.Biz, "unknown-biz")
	titleName := archivePathSegment(plan.Content.Title, "article")
	archiveDir := filepath.Join(root, "公众号文章", bizName, titleName+"_"+contentDigest)
	assetsDir := filepath.Join(archiveDir, "assets")
	if err := os.MkdirAll(assetsDir, 0755); err != nil {
		return nil, fmt.Errorf("create article archive directory: %w", err)
	}

	htmlPath := filepath.Join(archiveDir, "index.html")
	manifestPath := filepath.Join(archiveDir, "article.json")
	result := &ArticleArchiveDownloadResult{
		ContentKey:   plan.Content.Key,
		Directory:    archiveDir,
		HTMLPath:     htmlPath,
		ManifestPath: manifestPath,
		Files:        make([]ArticleArchiveFileResult, 0, len(plan.Resources)),
		Assets:       make([]officialaccount.ArticleAsset, 0, len(plan.Resources)),
	}
	result.RelativeDirectory = archiveRelativePath(root, archiveDir)
	result.RelativeHTMLPath = archiveRelativePath(root, htmlPath)
	result.RelativeManifestPath = archiveRelativePath(root, manifestPath)

	localBySource := make(map[string]string)
	manifestFiles := make([]articleArchiveManifestFile, 0, len(plan.Resources))
	for _, resource := range plan.Resources {
		if resource.Key == body.Key {
			continue
		}
		fileResult := ArticleArchiveFileResult{
			ResourceKey: resource.Key,
			Kind:        resource.Kind,
			Role:        resource.Role,
			SourceURL:   officialaccount.SanitizeArchiveMetadataURL(resource.SourceURL),
		}
		if resource.Kind != officialaccount.ArchiveResourceKindImage {
			fileResult.Status = "failed"
			fileResult.Error = "unsupported archive resource kind"
			result.Failed++
			result.Files = append(result.Files, fileResult)
			manifestFiles = append(manifestFiles, manifestFileFromResult(fileResult))
			continue
		}
		if strings.TrimSpace(resource.SourceURL) == "" {
			fileResult.Status = "failed"
			fileResult.Error = "image source URL is empty"
			result.Failed++
			result.Files = append(result.Files, fileResult)
			manifestFiles = append(manifestFiles, manifestFileFromResult(fileResult))
			continue
		}

		fileName := archiveImageFileName(resource)
		targetPath := filepath.Join(assetsDir, fileName)
		fileResult.Path = targetPath
		fileResult.RelativePath = archiveRelativePath(archiveDir, targetPath)
		if !force && isNonEmptyArchiveFile(targetPath) {
			fileResult.SHA256, fileResult.Size, err = archiveFileMetadata(targetPath)
			if err != nil {
				fileResult.Status = "failed"
				fileResult.Error = fmt.Errorf("verify existing archive file: %w", err).Error()
				result.Failed++
				result.Files = append(result.Files, fileResult)
				manifestFiles = append(manifestFiles, manifestFileFromResult(fileResult))
				continue
			}
			fileResult.Status = "skipped"
			result.Skipped++
			localBySource[normalizeArchiveAssetURL(resource.SourceURL)] = filepath.ToSlash(fileResult.RelativePath)
			result.Files = append(result.Files, fileResult)
			manifestFiles = append(manifestFiles, manifestFileFromResult(fileResult))
			continue
		}

		if force {
			if err := removeArchiveFile(targetPath); err != nil {
				fileResult.Status = "failed"
				fileResult.Error = err.Error()
				result.Failed++
				result.Files = append(result.Files, fileResult)
				manifestFiles = append(manifestFiles, manifestFileFromResult(fileResult))
				continue
			}
		}

		if err := s.downloadImage(ctx, archiveDir, targetPath, resource.SourceURL, headers); err != nil {
			if ctx.Err() != nil {
				return result, ctx.Err()
			}
			fileResult.Status = "failed"
			fileResult.Error = err.Error()
			// Keep the exported HTML usable without persisting a short-lived
			// credential in the fallback URL.
			localBySource[normalizeArchiveAssetURL(resource.SourceURL)] = officialaccount.SanitizeArchiveMetadataURL(resource.SourceURL)
			result.Failed++
		} else {
			fileResult.SHA256, fileResult.Size, err = archiveFileMetadata(targetPath)
			if err != nil {
				_ = removeArchiveFile(targetPath)
				fileResult.Status = "failed"
				fileResult.Error = fmt.Errorf("verify downloaded archive file: %w", err).Error()
				localBySource[normalizeArchiveAssetURL(resource.SourceURL)] = officialaccount.SanitizeArchiveMetadataURL(resource.SourceURL)
				result.Failed++
			} else {
				fileResult.Status = "downloaded"
				result.Downloaded++
				localBySource[normalizeArchiveAssetURL(resource.SourceURL)] = filepath.ToSlash(fileResult.RelativePath)
			}
		}
		result.Files = append(result.Files, fileResult)
		manifestFiles = append(manifestFiles, manifestFileFromResult(fileResult))
	}

	localizedHTML, err := rewriteArchiveHTML(body.InlineBody, localBySource)
	if err != nil {
		return result, fmt.Errorf("rewrite article HTML: %w", err)
	}
	if err := writeArchiveFile(htmlPath, []byte(localizedHTML), 0644); err != nil {
		return result, fmt.Errorf("write article HTML: %w", err)
	}
	result.HTMLSHA256, result.HTMLSize, err = archiveFileMetadata(htmlPath)
	if err != nil {
		return result, fmt.Errorf("verify article HTML: %w", err)
	}
	result.Assets = articleArchiveAssets(plan, body, result)

	manifestResources := make([]officialaccount.ArchiveResource, len(plan.Resources))
	copy(manifestResources, plan.Resources)
	for i := range manifestResources {
		// The body is already persisted as index.html; avoid duplicating the full
		// article in the portable manifest.
		manifestResources[i].InlineBody = ""
		manifestResources[i].SourceURL = officialaccount.SanitizeArchiveMetadataURL(manifestResources[i].SourceURL)
	}
	manifest := articleArchiveManifest{
		Content:   plan.Content,
		Resources: manifestResources,
		Relations: append([]officialaccount.ArchiveRelation(nil), plan.Relations...),
		Files:     manifestFiles,
		HTMLPath:  filepath.ToSlash(result.RelativeHTMLPath),
		SavedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return result, fmt.Errorf("encode article manifest: %w", err)
	}
	if err := writeArchiveFile(manifestPath, manifestData, 0644); err != nil {
		return result, fmt.Errorf("write article manifest: %w", err)
	}

	return result, nil
}

func articleArchiveBodyResource(plan officialaccount.ArchivePlan) (officialaccount.ArchiveResource, error) {
	for _, resource := range plan.Resources {
		if resource.Kind == officialaccount.ArchiveResourceKindHTML && resource.Role == officialaccount.ArchiveResourceRoleArticleBody {
			if strings.TrimSpace(resource.InlineBody) == "" {
				return officialaccount.ArchiveResource{}, errors.New("article archive HTML resource is empty")
			}
			return resource, nil
		}
	}
	return officialaccount.ArchiveResource{}, errors.New("article archive HTML resource is missing")
}

func (s *ArticleArchiveDownloadService) downloadImage(ctx context.Context, archiveDir, targetPath, sourceURL string, headers map[string]string) error {
	tempPath := targetPath + ".part"
	_ = removeArchiveFile(tempPath)
	actualPath, err := s.downloader.DownloadSync(ctx, sourceURL, tempPath, s.archiveConnections(), cloneArchiveHeaders(headers), nil)
	if err != nil {
		cleanupArchiveDownloadPath(archiveDir, actualPath)
		cleanupArchiveDownloadPath(archiveDir, tempPath)
		return fmt.Errorf("download image %s: %w", officialaccount.SanitizeArchiveMetadataURL(sourceURL), err)
	}
	if actualPath == "" {
		actualPath = tempPath
	}
	if !archivePathWithin(archiveDir, actualPath) {
		return fmt.Errorf("downloader returned path outside article archive: %s", actualPath)
	}
	if !archivePathsEqual(actualPath, tempPath) {
		if err := replaceArchiveFile(actualPath, tempPath); err != nil {
			cleanupArchiveDownloadPath(archiveDir, actualPath)
			cleanupArchiveDownloadPath(archiveDir, tempPath)
			return fmt.Errorf("stage downloaded image: %w", err)
		}
	}
	if !isNonEmptyArchiveFile(tempPath) {
		cleanupArchiveDownloadPath(archiveDir, tempPath)
		return errors.New("downloaded image is empty")
	}
	if err := replaceArchiveFile(tempPath, targetPath); err != nil {
		cleanupArchiveDownloadPath(archiveDir, tempPath)
		return fmt.Errorf("finalize downloaded image: %w", err)
	}
	return nil
}

func archivePathSegment(raw, fallback string) string {
	if strings.TrimSpace(raw) == "" {
		return fallback
	}
	cleaned := strings.TrimRight(strings.TrimSpace(utils.CleanFilename(raw)), " .")
	if cleaned == "" || cleaned == "." || cleaned == ".." {
		return fallback
	}
	return cleaned
}

func archiveImageFileName(resource officialaccount.ArchiveResource) string {
	base := archivePathSegment(resource.Name, "image")
	if ext := archiveImageExtension(resource.SourceURL); ext != "" {
		return base + ext
	}
	return base + ".jpg"
}

func archiveImageExtension(rawURL string) string {
	parsed, err := url.Parse(normalizeArchiveAssetURL(rawURL))
	if err != nil {
		return ""
	}
	if ext := strings.ToLower(filepath.Ext(parsed.Path)); isArchiveImageExtension(ext) {
		return ext
	}
	for _, key := range []string{"wx_fmt", "format", "fmt"} {
		if value := strings.ToLower(strings.TrimSpace(parsed.Query().Get(key))); value != "" {
			if !strings.HasPrefix(value, ".") {
				value = "." + value
			}
			if isArchiveImageExtension(value) {
				return value
			}
		}
	}
	return ""
}

func isArchiveImageExtension(ext string) bool {
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".avif", ".bmp", ".svg", ".ico":
		return true
	default:
		return false
	}
}

func normalizeArchiveAssetURL(raw string) string {
	normalized := htmlstd.UnescapeString(strings.TrimSpace(raw))
	if strings.HasPrefix(normalized, "//") {
		return "https:" + normalized
	}
	return normalized
}

func rewriteArchiveHTML(content string, localBySource map[string]string) (string, error) {
	document, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return "", err
	}
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.ElementNode && strings.EqualFold(node.Data, "img") {
			source := archiveNodeAttribute(node, "src")
			if source == "" {
				source = archiveNodeAttribute(node, "data-src")
			}
			if localPath, ok := localBySource[normalizeArchiveAssetURL(source)]; ok {
				setArchiveNodeAttribute(node, "src", localPath)
				removeArchiveNodeAttribute(node, "data-src")
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(document)

	body := findArchiveHTMLNode(document, "body")
	if body == nil {
		return "", errors.New("article HTML body is missing")
	}
	var output bytes.Buffer
	for child := body.FirstChild; child != nil; child = child.NextSibling {
		if err := html.Render(&output, child); err != nil {
			return "", err
		}
	}
	return strings.TrimSpace(output.String()), nil
}

func findArchiveHTMLNode(node *html.Node, name string) *html.Node {
	if node.Type == html.ElementNode && strings.EqualFold(node.Data, name) {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findArchiveHTMLNode(child, name); found != nil {
			return found
		}
	}
	return nil
}

func archiveNodeAttribute(node *html.Node, key string) string {
	for _, attribute := range node.Attr {
		if strings.EqualFold(attribute.Key, key) {
			return strings.TrimSpace(attribute.Val)
		}
	}
	return ""
}

func setArchiveNodeAttribute(node *html.Node, key, value string) {
	for i := range node.Attr {
		if strings.EqualFold(node.Attr[i].Key, key) {
			node.Attr[i].Val = value
			return
		}
	}
	node.Attr = append(node.Attr, html.Attribute{Key: key, Val: value})
}

func removeArchiveNodeAttribute(node *html.Node, key string) {
	filtered := node.Attr[:0]
	for _, attribute := range node.Attr {
		if !strings.EqualFold(attribute.Key, key) {
			filtered = append(filtered, attribute)
		}
	}
	node.Attr = filtered
}

func cloneArchiveHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	clone := make(map[string]string, len(headers))
	for key, value := range headers {
		clone[key] = value
	}
	return clone
}

func archiveRelativePath(root, target string) string {
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return filepath.Base(target)
	}
	return filepath.ToSlash(relative)
}

func archivePathWithin(base, target string) bool {
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return false
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(baseAbs, targetAbs)
	if err != nil || relative == ".." {
		return false
	}
	return !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func archivePathsEqual(left, right string) bool {
	leftAbs, err := filepath.Abs(left)
	if err != nil {
		return false
	}
	rightAbs, err := filepath.Abs(right)
	if err != nil {
		return false
	}
	return filepath.Clean(leftAbs) == filepath.Clean(rightAbs)
}

func cleanupArchiveDownloadPath(base, target string) {
	if target != "" && archivePathWithin(base, target) {
		_ = removeArchiveFile(target)
	}
}

func isNonEmptyArchiveFile(path string) bool {
	stat, err := os.Stat(path)
	return err == nil && !stat.IsDir() && stat.Size() > 0
}

func removeArchiveFile(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func replaceArchiveFile(source, target string) error {
	if source == target {
		return nil
	}
	if err := removeArchiveFile(target); err != nil {
		return err
	}
	return os.Rename(source, target)
}

func writeArchiveFile(target string, data []byte, mode os.FileMode) error {
	file, err := os.CreateTemp(filepath.Dir(target), ".wx-archive-*")
	if err != nil {
		return err
	}
	tempPath := file.Name()
	defer func() {
		_ = file.Close()
		_ = os.Remove(tempPath)
	}()
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Chmod(mode); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return replaceArchiveFile(tempPath, target)
}

func archiveFileMetadata(path string) (string, *int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", nil, err
	}
	if info.IsDir() {
		return "", nil, errors.New("archive metadata target is a directory")
	}
	hasher := sha256.New()
	size, err := io.Copy(hasher, file)
	if err != nil {
		return "", nil, err
	}
	if size <= 0 {
		return "", nil, errors.New("archive metadata target is empty")
	}
	return hex.EncodeToString(hasher.Sum(nil)), &size, nil
}

func articleArchiveAssets(plan officialaccount.ArchivePlan, body officialaccount.ArchiveResource, result *ArticleArchiveDownloadResult) []officialaccount.ArticleAsset {
	if result == nil {
		return nil
	}
	files := make(map[string]ArticleArchiveFileResult, len(result.Files))
	for _, file := range result.Files {
		files[file.ResourceKey] = file
	}
	assets := make([]officialaccount.ArticleAsset, 0, len(plan.Resources))
	assets = append(assets, officialaccount.ArticleAsset{
		ArticleKey:  plan.Content.Key,
		ResourceKey: body.Key,
		Kind:        body.Kind,
		Role:        body.Role,
		SourceURL:   officialaccount.SanitizeArchiveMetadataURL(body.SourceURL),
		LocalPath:   filepath.ToSlash(result.RelativeHTMLPath),
		SHA256:      result.HTMLSHA256,
		Size:        result.HTMLSize,
		Status:      "downloaded",
	})
	for _, resource := range plan.Resources {
		if resource.Key == body.Key {
			continue
		}
		file, ok := files[resource.Key]
		if !ok {
			continue
		}
		localPath := ""
		if file.Status == "downloaded" || file.Status == "skipped" {
			localPath = filepath.ToSlash(file.RelativePath)
		}
		assets = append(assets, officialaccount.ArticleAsset{
			ArticleKey:  plan.Content.Key,
			ResourceKey: resource.Key,
			Kind:        resource.Kind,
			Role:        resource.Role,
			SourceURL:   officialaccount.SanitizeArchiveMetadataURL(resource.SourceURL),
			LocalPath:   localPath,
			SHA256:      file.SHA256,
			Size:        file.Size,
			Status:      file.Status,
			Error:       file.Error,
		})
	}
	return assets
}

func manifestFileFromResult(result ArticleArchiveFileResult) articleArchiveManifestFile {
	return articleArchiveManifestFile{
		ResourceKey:  result.ResourceKey,
		Kind:         result.Kind,
		Role:         result.Role,
		SourceURL:    result.SourceURL,
		Status:       result.Status,
		RelativePath: result.RelativePath,
		SHA256:       result.SHA256,
		Size:         result.Size,
		Error:        result.Error,
	}
}

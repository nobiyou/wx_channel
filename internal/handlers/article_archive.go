package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"wx_channel/internal/officialaccount"
	"wx_channel/internal/response"
	"wx_channel/internal/services"
)

const maxArticleArchiveRequestBody = 16 << 20

// ArticleArchiveHandler is the HTTP adapter for article archive downloads.
// The official-account service owns credentials; this handler only translates
// the request into a plan and delegates file work to the archive service.
type ArticleArchiveHandler struct {
	accountService      *officialaccount.Service
	archiveService      *services.ArticleArchiveDownloadService
	resolveDownloadsDir func() (string, error)
}

func NewArticleArchiveHandler(
	accountService *officialaccount.Service,
	downloader services.ArchiveFileDownloader,
	resolveDownloadsDir func() (string, error),
) *ArticleArchiveHandler {
	return &ArticleArchiveHandler{
		accountService:      accountService,
		archiveService:      services.NewArticleArchiveDownloadService(downloader),
		resolveDownloadsDir: resolveDownloadsDir,
	}
}

func (h *ArticleArchiveHandler) SetConnections(connections int) {
	if h == nil || h.archiveService == nil {
		return
	}
	h.archiveService.SetConnections(connections)
}

func (h *ArticleArchiveHandler) RegisterRoutes(mux *http.ServeMux) {
	if h == nil || mux == nil {
		return
	}
	mux.HandleFunc("/api/mp/archive/download", h.HandleDownload)
}

func (h *ArticleArchiveHandler) HandleDownload(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.archiveService == nil || h.accountService == nil {
		response.ErrorWithStatus(w, http.StatusServiceUnavailable, http.StatusServiceUnavailable, "article archive service is unavailable")
		return
	}
	if r.Method != http.MethodPost {
		response.ErrorWithStatus(w, http.StatusMethodNotAllowed, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.resolveDownloadsDir == nil {
		response.ErrorWithStatus(w, http.StatusInternalServerError, http.StatusInternalServerError, "article archive download directory is unavailable")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxArticleArchiveRequestBody)
	defer r.Body.Close()
	var request officialaccount.ArchiveRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		response.ErrorWithStatus(w, http.StatusBadRequest, http.StatusBadRequest, "invalid archive download payload")
		return
	}
	plan, err := request.BuildPlan()
	if err != nil {
		writeArticleArchiveError(w, err)
		return
	}
	referer := plan.Content.URL
	if strings.TrimSpace(referer) == "" {
		referer = plan.Content.SourceURL
	}
	headers, err := h.accountService.ArchiveRequestHeaders(request.Biz, referer)
	if err != nil {
		writeArticleArchiveError(w, err)
		return
	}
	if err := h.accountService.EnsureArticleArchiveRecord(plan); err != nil {
		writeArticleArchiveError(w, err)
		return
	}
	if err := h.accountService.UpdateArticleArchive(officialaccount.ArticleArchiveState{
		ArticleKey: plan.Content.Key,
		Status:     officialaccount.ArchiveStatusQueued,
	}); err != nil {
		writeArticleArchiveError(w, err)
		return
	}
	root, err := h.resolveDownloadsDir()
	if err != nil {
		response.ErrorWithStatus(w, http.StatusInternalServerError, http.StatusInternalServerError, fmt.Sprintf("resolve article archive directory: %v", err))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()
	result, err := h.archiveService.Download(ctx, root, plan, headers, request.Force)
	if err != nil {
		if result != nil {
			_ = h.accountService.UpdateArticleArchive(officialaccount.ArticleArchiveState{
				ArticleKey:   plan.Content.Key,
				Status:       officialaccount.ArchiveStatusFailed,
				Directory:    result.RelativeDirectory,
				HTMLPath:     result.RelativeHTMLPath,
				ManifestPath: result.RelativeManifestPath,
				Assets:       result.Assets,
			})
		} else {
			_ = h.accountService.UpdateArticleArchive(officialaccount.ArticleArchiveState{
				ArticleKey: plan.Content.Key,
				Status:     officialaccount.ArchiveStatusFailed,
			})
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			response.ErrorWithStatus(w, http.StatusRequestTimeout, http.StatusRequestTimeout, err.Error())
			return
		}
		response.ErrorWithStatus(w, http.StatusInternalServerError, http.StatusInternalServerError, err.Error())
		return
	}
	archiveStatus := officialaccount.ArchiveStatusArchived
	if result.Failed > 0 {
		archiveStatus = officialaccount.ArchiveStatusPartial
	}
	if err := h.accountService.UpdateArticleArchive(officialaccount.ArticleArchiveState{
		ArticleKey:   plan.Content.Key,
		Status:       archiveStatus,
		Directory:    result.RelativeDirectory,
		HTMLPath:     result.RelativeHTMLPath,
		ManifestPath: result.RelativeManifestPath,
		Assets:       result.Assets,
	}); err != nil {
		response.ErrorWithStatus(w, http.StatusInternalServerError, http.StatusInternalServerError, fmt.Sprintf("persist article archive state: %v", err))
		return
	}
	response.Success(w, result)
}

func writeArticleArchiveError(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	code := http.StatusBadGateway
	switch {
	case errors.Is(err, officialaccount.ErrMissingBiz),
		errors.Is(err, officialaccount.ErrArticleIdentity),
		errors.Is(err, officialaccount.ErrContentNotFound):
		status = http.StatusBadRequest
		code = http.StatusBadRequest
	case errors.Is(err, officialaccount.ErrArticleTooLarge):
		status = http.StatusRequestEntityTooLarge
		code = http.StatusRequestEntityTooLarge
	case errors.Is(err, officialaccount.ErrAccountNotFound):
		status = http.StatusNotFound
		code = http.StatusNotFound
	case errors.Is(err, officialaccount.ErrAccountExpired):
		status = http.StatusUnauthorized
		code = http.StatusUnauthorized
	}
	response.ErrorWithStatus(w, status, code, err.Error())
}

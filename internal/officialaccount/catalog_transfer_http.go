package officialaccount

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"wx_channel/internal/response"
)

const maxCatalogTransferBody = 128 << 20

// HandleCatalogExport streams the credential-free catalog. ZIP is a transport
// envelope around catalog.json; archive files stay at their configured local
// paths and are represented by the asset index.
func (s *Service) HandleCatalogExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.ErrorWithStatus(w, http.StatusMethodNotAllowed, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	repository, ok := s.catalogTransferRepository()
	if !ok {
		response.ErrorWithStatus(w, http.StatusServiceUnavailable, http.StatusServiceUnavailable, "catalog transfer service is unavailable")
		return
	}
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "json"
	}
	filename := "wx_channels_official_account_catalog_" + time.Now().UTC().Format("20060102_150405")
	switch format {
	case "json":
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`.json"`)
		if _, err := repository.ExportCatalog(r.Context(), w); err != nil {
			log.Printf("official account catalog export failed: %v", err)
		}
	case "zip":
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`.zip"`)
		archive := zip.NewWriter(w)
		entry, err := archive.Create("catalog.json")
		if err == nil {
			_, err = repository.ExportCatalog(r.Context(), entry)
		}
		if closeErr := archive.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			log.Printf("official account ZIP catalog export failed: %v", err)
		}
	default:
		response.ErrorWithStatus(w, http.StatusBadRequest, http.StatusBadRequest, "unsupported catalog export format")
	}
}

// HandleCatalogImport accepts either a raw catalog document or an envelope:
// {"catalog": {...}, "dry_run": true, "conflict_policy": "skip"}.
func (s *Service) HandleCatalogImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.ErrorWithStatus(w, http.StatusMethodNotAllowed, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	repository, ok := s.catalogTransferRepository()
	if !ok {
		response.ErrorWithStatus(w, http.StatusServiceUnavailable, http.StatusServiceUnavailable, "catalog transfer service is unavailable")
		return
	}
	if r.ContentLength > maxCatalogTransferBody {
		response.ErrorWithStatus(w, http.StatusRequestEntityTooLarge, http.StatusRequestEntityTooLarge, "catalog import body is too large")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxCatalogTransferBody)
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "request body too large") {
			response.ErrorWithStatus(w, http.StatusRequestEntityTooLarge, http.StatusRequestEntityTooLarge, "catalog import body is too large")
			return
		}
		response.ErrorWithStatus(w, http.StatusBadRequest, http.StatusBadRequest, "read catalog import body: "+err.Error())
		return
	}
	if len(bytes.TrimSpace(body)) == 0 {
		response.ErrorWithStatus(w, http.StatusBadRequest, http.StatusBadRequest, "catalog import body is empty")
		return
	}

	document, envelopeOptions, err := decodeCatalogImportBody(body, r.Header.Get("Content-Type"))
	if err != nil {
		response.ErrorWithStatus(w, http.StatusBadRequest, http.StatusBadRequest, err.Error())
		return
	}
	options := envelopeOptions
	if value := strings.TrimSpace(r.URL.Query().Get("conflict_policy")); value != "" {
		options.ConflictPolicy = value
	}
	if value := strings.TrimSpace(r.URL.Query().Get("dry_run")); value != "" {
		parsed, parseErr := strconv.ParseBool(value)
		if parseErr != nil {
			response.ErrorWithStatus(w, http.StatusBadRequest, http.StatusBadRequest, "invalid dry_run value")
			return
		}
		options.DryRun = parsed
	}
	summary, err := repository.ImportCatalog(r.Context(), document, options)
	if err != nil {
		status := http.StatusInternalServerError
		code := http.StatusInternalServerError
		if errors.Is(err, ErrCatalogConflict) {
			status = http.StatusConflict
			code = http.StatusConflict
		}
		response.ErrorWithStatus(w, status, code, err.Error())
		return
	}
	response.Success(w, summary)
}

func (s *Service) catalogTransferRepository() (CatalogTransferRepository, bool) {
	if s == nil {
		return nil, false
	}
	repository := s.catalogRepository()
	transfer, ok := repository.(CatalogTransferRepository)
	return transfer, ok && transfer != nil
}

func decodeCatalogImportBody(body []byte, contentType string) (CatalogExport, CatalogImportOptions, error) {
	if isCatalogZIP(body, contentType) {
		catalogJSON, err := catalogJSONFromZIP(body)
		if err != nil {
			return CatalogExport{}, CatalogImportOptions{}, err
		}
		body = catalogJSON
	}

	var fields map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(&fields); err != nil {
		return CatalogExport{}, CatalogImportOptions{}, fmt.Errorf("decode catalog import JSON: %w", err)
	}
	if len(fields) == 0 {
		return CatalogExport{}, CatalogImportOptions{}, errors.New("catalog import JSON must be an object")
	}

	var document CatalogExport
	var options CatalogImportOptions
	if raw, exists := fields["catalog"]; exists && len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &document); err != nil {
			return CatalogExport{}, options, fmt.Errorf("decode catalog envelope: %w", err)
		}
		if raw, exists := fields["dry_run"]; exists {
			if err := json.Unmarshal(raw, &options.DryRun); err != nil {
				return CatalogExport{}, options, errors.New("invalid dry_run value")
			}
		}
		if raw, exists := fields["conflict_policy"]; exists {
			if err := json.Unmarshal(raw, &options.ConflictPolicy); err != nil {
				return CatalogExport{}, options, errors.New("invalid conflict_policy value")
			}
		}
		return document, options, nil
	}
	if err := json.Unmarshal(body, &document); err != nil {
		return CatalogExport{}, options, fmt.Errorf("decode catalog document: %w", err)
	}
	return document, options, nil
}

func isCatalogZIP(body []byte, contentType string) bool {
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	return contentType == "application/zip" || contentType == "application/x-zip-compressed" || bytes.HasPrefix(body, []byte("PK\x03\x04"))
}

func catalogJSONFromZIP(body []byte) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, fmt.Errorf("open catalog ZIP: %w", err)
	}
	for _, file := range reader.File {
		name := path.Clean(strings.ReplaceAll(file.Name, "\\", "/"))
		if name != "catalog.json" {
			continue
		}
		if file.UncompressedSize64 > maxCatalogTransferBody {
			return nil, errors.New("catalog ZIP entry is too large")
		}
		stream, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("open catalog ZIP entry: %w", err)
		}
		limited := io.LimitReader(stream, maxCatalogTransferBody+1)
		data, readErr := io.ReadAll(limited)
		closeErr := stream.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read catalog ZIP entry: %w", readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close catalog ZIP entry: %w", closeErr)
		}
		if len(data) > maxCatalogTransferBody {
			return nil, errors.New("catalog ZIP entry is too large")
		}
		return data, nil
	}
	return nil, errors.New("catalog ZIP is missing catalog.json")
}

package officialaccount

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type catalogTransferHTTPStub struct {
	syncCatalogStub
	exportDocument CatalogExport
	importDocument CatalogExport
	importOptions  CatalogImportOptions
	importError    error
}

func newCatalogTransferHTTPStub() *catalogTransferHTTPStub {
	return &catalogTransferHTTPStub{
		syncCatalogStub: *newSyncCatalogStub(),
		exportDocument: CatalogExport{
			FormatVersion: CatalogExportFormatVersion,
			SchemaVersion: 16,
			ExportedAt:    "2026-08-28T00:00:00Z",
			Accounts: []CatalogAccountRecord{{
				Biz:         "biz-http",
				Nickname:    "HTTP 测试账号",
				IsEffective: false,
			}},
		},
	}
}

func (s *catalogTransferHTTPStub) ExportCatalog(_ context.Context, writer io.Writer) (CatalogExportStats, error) {
	if writer == nil {
		return CatalogExportStats{}, errors.New("writer is nil")
	}
	data, err := json.Marshal(s.exportDocument)
	if err != nil {
		return CatalogExportStats{}, err
	}
	if _, err := writer.Write(data); err != nil {
		return CatalogExportStats{}, err
	}
	return CatalogExportStats{Accounts: int64(len(s.exportDocument.Accounts))}, nil
}

func (s *catalogTransferHTTPStub) ImportCatalog(_ context.Context, document CatalogExport, options CatalogImportOptions) (CatalogImportSummary, error) {
	s.importDocument = document
	s.importOptions = options
	if s.importError != nil {
		return CatalogImportSummary{}, s.importError
	}
	return CatalogImportSummary{
		DryRun:         options.DryRun,
		ConflictPolicy: options.ConflictPolicy,
		AccountsSeen:   len(document.Accounts),
		AccountsAdded:  len(document.Accounts),
		ArticlesSeen:   len(document.Articles),
		ArticlesAdded:  len(document.Articles),
		MetricsSeen:    len(document.Metrics),
		MetricsAdded:   len(document.Metrics),
	}, nil
}

func testCatalogTransferService(t *testing.T, stub *catalogTransferHTTPStub) *Service {
	t.Helper()
	service := NewMemoryService()
	if err := service.SetCatalogRepository(stub); err != nil {
		t.Fatalf("set catalog repository: %v", err)
	}
	return service
}

func TestHandleCatalogExportSupportsJSONAndZIP(t *testing.T) {
	stub := newCatalogTransferHTTPStub()
	service := testCatalogTransferService(t, stub)

	jsonRecorder := httptest.NewRecorder()
	jsonRequest := httptest.NewRequest(http.MethodGet, "/api/mp/catalog/export?format=json", nil)
	service.HandleCatalogExport(jsonRecorder, jsonRequest)
	if jsonRecorder.Code != http.StatusOK {
		t.Fatalf("JSON export status = %d: %s", jsonRecorder.Code, jsonRecorder.Body.String())
	}
	if jsonRecorder.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("JSON content type = %q", jsonRecorder.Header().Get("Content-Type"))
	}
	var exported CatalogExport
	if err := json.Unmarshal(jsonRecorder.Body.Bytes(), &exported); err != nil {
		t.Fatalf("decode JSON export: %v", err)
	}
	if len(exported.Accounts) != 1 || exported.Accounts[0].IsEffective {
		t.Fatalf("unexpected JSON export: %+v", exported)
	}

	zipRecorder := httptest.NewRecorder()
	zipRequest := httptest.NewRequest(http.MethodGet, "/api/mp/catalog/export?format=zip", nil)
	service.HandleCatalogExport(zipRecorder, zipRequest)
	if zipRecorder.Code != http.StatusOK {
		t.Fatalf("ZIP export status = %d: %s", zipRecorder.Code, zipRecorder.Body.String())
	}
	reader, err := zip.NewReader(bytes.NewReader(zipRecorder.Body.Bytes()), int64(zipRecorder.Body.Len()))
	if err != nil {
		t.Fatalf("open ZIP export: %v", err)
	}
	if len(reader.File) != 1 || reader.File[0].Name != "catalog.json" {
		t.Fatalf("unexpected ZIP entries: %+v", reader.File)
	}
	entry, err := reader.File[0].Open()
	if err != nil {
		t.Fatalf("open ZIP catalog: %v", err)
	}
	defer entry.Close()
	var zipped CatalogExport
	if err := json.NewDecoder(entry).Decode(&zipped); err != nil {
		t.Fatalf("decode ZIP catalog: %v", err)
	}
	if len(zipped.Accounts) != 1 || zipped.Accounts[0].Biz != "biz-http" {
		t.Fatalf("unexpected ZIP catalog: %+v", zipped)
	}
}

func TestHandleCatalogImportAcceptsJSONZIPAndQueryOverrides(t *testing.T) {
	stub := newCatalogTransferHTTPStub()
	service := testCatalogTransferService(t, stub)
	document := CatalogExport{
		FormatVersion: CatalogExportFormatVersion,
		SchemaVersion: 16,
		Accounts:      []CatalogAccountRecord{{Biz: "biz-import"}},
	}
	documentBytes, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal catalog: %v", err)
	}

	jsonRecorder := httptest.NewRecorder()
	jsonRequest := httptest.NewRequest(http.MethodPost, "/api/mp/catalog/import?dry_run=true&conflict_policy=merge", bytes.NewReader(documentBytes))
	jsonRequest.Header.Set("Content-Type", "application/json")
	service.HandleCatalogImport(jsonRecorder, jsonRequest)
	if jsonRecorder.Code != http.StatusOK {
		t.Fatalf("JSON import status = %d: %s", jsonRecorder.Code, jsonRecorder.Body.String())
	}
	if !stub.importOptions.DryRun || stub.importOptions.ConflictPolicy != CatalogConflictMerge {
		t.Fatalf("query options were not applied: %+v", stub.importOptions)
	}
	if stub.importDocument.Accounts[0].Biz != "biz-import" {
		t.Fatalf("unexpected imported JSON document: %+v", stub.importDocument)
	}

	var zipBuffer bytes.Buffer
	zipWriter := zip.NewWriter(&zipBuffer)
	entry, err := zipWriter.Create("catalog.json")
	if err != nil {
		t.Fatalf("create ZIP catalog: %v", err)
	}
	if _, err := entry.Write(documentBytes); err != nil {
		t.Fatalf("write ZIP catalog: %v", err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatalf("close ZIP catalog: %v", err)
	}

	zipRecorder := httptest.NewRecorder()
	zipRequest := httptest.NewRequest(http.MethodPost, "/api/mp/catalog/import", bytes.NewReader(zipBuffer.Bytes()))
	zipRequest.Header.Set("Content-Type", "application/zip")
	service.HandleCatalogImport(zipRecorder, zipRequest)
	if zipRecorder.Code != http.StatusOK {
		t.Fatalf("ZIP import status = %d: %s", zipRecorder.Code, zipRecorder.Body.String())
	}
	if stub.importDocument.Accounts[0].Biz != "biz-import" {
		t.Fatalf("unexpected imported ZIP document: %+v", stub.importDocument)
	}
}

func TestHandleCatalogImportMapsConflictAndOversizedBody(t *testing.T) {
	stub := newCatalogTransferHTTPStub()
	stub.importError = ErrCatalogConflict
	service := testCatalogTransferService(t, stub)
	document := CatalogExport{FormatVersion: CatalogExportFormatVersion, SchemaVersion: 16}
	body, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal catalog: %v", err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/mp/catalog/import", bytes.NewReader(body))
	service.HandleCatalogImport(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("conflict status = %d: %s", recorder.Code, recorder.Body.String())
	}

	largeRecorder := httptest.NewRecorder()
	largeRequest := httptest.NewRequest(http.MethodPost, "/api/mp/catalog/import", bytes.NewReader([]byte("{}")))
	largeRequest.ContentLength = maxCatalogTransferBody + 1
	service.HandleCatalogImport(largeRecorder, largeRequest)
	if largeRecorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized status = %d: %s", largeRecorder.Code, largeRecorder.Body.String())
	}
}

func TestHandleCatalogExportRejectsUnsupportedFormat(t *testing.T) {
	service := testCatalogTransferService(t, newCatalogTransferHTTPStub())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/mp/catalog/export?format=csv", nil)
	service.HandleCatalogExport(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("unsupported format status = %d: %s", recorder.Code, recorder.Body.String())
	}
}

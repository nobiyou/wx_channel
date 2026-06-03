package worker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"path/filepath"
	"strings"
)

type DeployBody struct {
	AccountID         string
	AuthToken         string
	WorkerName        string
	ScriptContent     []byte
	CompatibilityDate string
	Bindings          []Binding
	MainModule        string
	AdditionalFiles   map[string][]byte
}

type Metadata struct {
	MainModule        string    `json:"main_module"`
	CompatibilityDate string    `json:"compatibility_date"`
	Bindings          []Binding `json:"bindings"`
}

type Binding struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	NamespaceID string `json:"namespace_id,omitempty"`
	Text        string `json:"text,omitempty"`
	ID          string `json:"id,omitempty"`
}

type DeployResult struct {
	Success bool            `json:"success"`
	Errors  []any           `json:"errors"`
	Result  DeployResultRef `json:"result"`
}

type DeployResultRef struct {
	ID string `json:"id"`
}

func detectContentType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".js", ".mjs":
		return "application/javascript+module"
	case ".json":
		return "application/json"
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".txt":
		return "text/plain; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	default:
		return "application/octet-stream"
	}
}

func Deploy(deployBody DeployBody) (string, error) {
	mainModule := strings.TrimSpace(deployBody.MainModule)
	if mainModule == "" {
		mainModule = "index.js"
	}

	compatibilityDate := strings.TrimSpace(deployBody.CompatibilityDate)
	if compatibilityDate == "" {
		compatibilityDate = "2024-01-01"
	}

	metadataJSON, err := json.Marshal(Metadata{
		MainModule:        mainModule,
		CompatibilityDate: compatibilityDate,
		Bindings:          deployBody.Bindings,
	})
	if err != nil {
		return "", fmt.Errorf("marshal worker metadata: %w", err)
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	metaHeader := make(textproto.MIMEHeader)
	metaHeader.Set("Content-Disposition", `form-data; name="metadata"`)
	metaHeader.Set("Content-Type", "application/json")
	metaPart, err := writer.CreatePart(metaHeader)
	if err != nil {
		return "", fmt.Errorf("create metadata part: %w", err)
	}
	if _, err := metaPart.Write(metadataJSON); err != nil {
		return "", fmt.Errorf("write metadata part: %w", err)
	}

	scriptHeader := make(textproto.MIMEHeader)
	scriptHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, mainModule, mainModule))
	scriptHeader.Set("Content-Type", "application/javascript+module")
	scriptPart, err := writer.CreatePart(scriptHeader)
	if err != nil {
		return "", fmt.Errorf("create script part: %w", err)
	}
	if _, err := scriptPart.Write(deployBody.ScriptContent); err != nil {
		return "", fmt.Errorf("write script part: %w", err)
	}

	for filename, content := range deployBody.AdditionalFiles {
		fileHeader := make(textproto.MIMEHeader)
		fileHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, filename, filename))
		fileHeader.Set("Content-Type", detectContentType(filename))
		filePart, err := writer.CreatePart(fileHeader)
		if err != nil {
			return "", fmt.Errorf("create additional file part %s: %w", filename, err)
		}
		if _, err := filePart.Write(content); err != nil {
			return "", fmt.Errorf("write additional file part %s: %w", filename, err)
		}
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("close multipart body: %w", err)
	}

	apiURL := fmt.Sprintf(
		"https://api.cloudflare.com/client/v4/accounts/%s/workers/scripts/%s",
		deployBody.AccountID,
		deployBody.WorkerName,
	)
	req, err := http.NewRequest(http.MethodPut, apiURL, body)
	if err != nil {
		return "", fmt.Errorf("create deploy request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+deployBody.AuthToken)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return "", fmt.Errorf("deploy request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("deploy failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result DeployResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("decode deploy response: %w", err)
	}
	if !result.Success {
		return "", fmt.Errorf("deploy failed: %s", string(respBody))
	}

	_ = enableSubdomain(deployBody.AccountID, deployBody.AuthToken, deployBody.WorkerName)
	return result.Result.ID, nil
}

func enableSubdomain(accountID, authToken, workerName string) error {
	apiURL := fmt.Sprintf(
		"https://api.cloudflare.com/client/v4/accounts/%s/workers/scripts/%s/subdomain",
		accountID,
		workerName,
	)
	req, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewBufferString(`{"enabled":true}`))
	if err != nil {
		return fmt.Errorf("create subdomain request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+authToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return fmt.Errorf("enable subdomain request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("enable subdomain failed (status %d): %s", resp.StatusCode, string(body))
	}
	return nil
}

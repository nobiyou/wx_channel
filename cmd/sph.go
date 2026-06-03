package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"wx_channel/internal/assets"
	"wx_channel/internal/config"
	cfworker "wx_channel/pkg/cloudflare/worker"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var sphDeployCmd = &cobra.Command{
	Use:   "sph_deploy",
	Short: "部署分享链接解析 Cloudflare Worker",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSphDeploy(cmd.OutOrStdout())
	},
}

func init() {
	rootCmd.AddCommand(sphDeployCmd)
}

func runSphDeploy(w io.Writer) error {
	cfg := config.Load()

	accountID := strings.TrimSpace(cfg.Cloudflare.AccountID)
	apiToken := strings.TrimSpace(cfg.Cloudflare.APIToken)
	workerName := strings.TrimSpace(cfg.Cloudflare.SphWorkerName)
	sphCookie := strings.TrimSpace(cfg.Cloudflare.SphCookie)

	if accountID == "" || apiToken == "" {
		return fmt.Errorf("cloudflare.accountId and cloudflare.apiToken are required")
	}
	if workerName == "" {
		return fmt.Errorf("cloudflare.sphWorkerName is required")
	}
	if sphCookie == "" {
		return fmt.Errorf("cloudflare.sphCookie is required for worker deployment")
	}

	if _, err := cfworker.Deploy(cfworker.DeployBody{
		AccountID:         accountID,
		AuthToken:         apiToken,
		WorkerName:        workerName,
		ScriptContent:     assets.SphWorkerJS,
		CompatibilityDate: "2024-01-01",
		MainModule:        "worker.js",
		Bindings: []cfworker.Binding{
			{Type: "plain_text", Name: "COOKIE", Text: sphCookie},
		},
	}); err != nil {
		return err
	}

	subdomain, err := getWorkersSubdomain(accountID, apiToken)
	workerURL := fmt.Sprintf("https://%s.<your-subdomain>.workers.dev", workerName)
	if err == nil && subdomain != "" {
		workerURL = fmt.Sprintf("https://%s.%s.workers.dev", workerName, subdomain)
	}

	viper.Set("cloudflare.sphHostname", workerURL)
	if err := persistViperConfig(); err != nil {
		return fmt.Errorf("worker deployed but failed to persist cloudflare.sphHostname: %w", err)
	}

	fmt.Fprintln(w, "Cloudflare Worker 部署成功")
	fmt.Fprintf(w, "Worker Name: %s\n", workerName)
	fmt.Fprintf(w, "Worker URL: %s\n", workerURL)
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "下一步：")
	fmt.Fprintf(w, "1. 已自动写回 config.yaml: cloudflare.sphHostname = %q\n", workerURL)
	fmt.Fprintln(w, "2. 可选地清空 cloudflare.sphCookie，改为统一走 Worker")
	fmt.Fprintln(w, "3. 重启 wx_channel")
	fmt.Fprintln(w, "4. 调用 /api/channels/parse_sph?url=<分享链接> 验证")

	return nil
}

func getWorkersSubdomain(accountID, authToken string) (string, error) {
	apiURL := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/workers/subdomain", accountID)
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("create request failed: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+authToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var subdomainResp struct {
		Result struct {
			Subdomain string `json:"subdomain"`
		} `json:"result"`
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(body, &subdomainResp); err != nil {
		return "", fmt.Errorf("decode response failed: %w", err)
	}
	if !subdomainResp.Success {
		return "", fmt.Errorf("api returned success=false")
	}
	return strings.TrimSpace(subdomainResp.Result.Subdomain), nil
}

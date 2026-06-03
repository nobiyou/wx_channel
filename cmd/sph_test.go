package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wx_channel/internal/config"

	"github.com/spf13/viper"
)

func TestRunSphDeployRequiresCloudflareFields(t *testing.T) {
	t.Parallel()

	config.Reload()
	cfg := config.Get()
	cfg.Cloudflare = config.CloudflareConfig{}

	var out strings.Builder
	err := runSphDeploy(&out)
	if err == nil {
		t.Fatalf("expected error when cloudflare config is missing")
	}
	if !strings.Contains(err.Error(), "cloudflare.accountId and cloudflare.apiToken are required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPersistViperConfigWritesFile(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	viper.SetConfigFile(configPath)
	viper.Set("cloudflare.sphHostname", "https://sph.example.workers.dev")

	if err := persistViperConfig(); err != nil {
		t.Fatalf("persistViperConfig() error = %v", err)
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config file: %v", err)
	}
	if !strings.Contains(strings.ToLower(string(content)), "sphhostname: https://sph.example.workers.dev") {
		t.Fatalf("config content = %s", string(content))
	}
}

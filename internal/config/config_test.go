package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"wx_channel/internal/version"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func cleanupEnv() {
	for _, env := range os.Environ() {
		pair := strings.SplitN(env, "=", 2)
		if strings.HasPrefix(pair[0], "WX_CHANNEL_") {
			os.Unsetenv(pair[0])
		}
	}
}

func setupIsolatedTestEnv(t *testing.T) {
	t.Helper()
	viper.Reset()
	globalConfig = nil
	cleanupEnv()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Chdir(tmpDir)
}

func TestLoad_Defaults(t *testing.T) {
	setupIsolatedTestEnv(t)

	cfg := Load()

	assert.NotNil(t, cfg)
	assert.Equal(t, 2025, cfg.Port)
	assert.Equal(t, version.Current, cfg.Version)
	assert.Equal(t, int64(2<<20), cfg.ChunkSize)
	assert.Equal(t, "", cfg.DownloadFilenameTemplate)
	assert.Equal(t, 500*time.Millisecond, cfg.SaveDelay)
	assert.False(t, cfg.RadarEnabled)
}

func TestLoad_GeneratedConfigFileIncludesRadarEnabled(t *testing.T) {
	setupIsolatedTestEnv(t)

	_ = Load()

	content, err := os.ReadFile("config.yaml")
	if err != nil {
		t.Fatalf("无法读取自动生成的配置文件: %v", err)
	}

	assert.Contains(t, string(content), "download_filename_template: \"\"")
	assert.Contains(t, string(content), "radar_enabled: false")
}

func TestLoad_EnvVars(t *testing.T) {
	setupIsolatedTestEnv(t)

	t.Setenv("WX_CHANNEL_PORT", "9999")
	t.Setenv("WX_CHANNEL_LOG_FILE", "test.log")

	cfg := Load()

	assert.Equal(t, 9999, cfg.Port)
	assert.Equal(t, "test.log", cfg.LogFile)
}

func TestLoad_ConfigFile(t *testing.T) {
	setupIsolatedTestEnv(t)

	// 创建临时配置文件
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	content := []byte(`
port: 8888
version: "6.0.0"
download_dir: "/tmp/downloads"
`)
	if err := os.WriteFile(configFile, content, 0644); err != nil {
		t.Fatalf("无法创建配置文件: %v", err)
	}

	// 显式设置配置文件路径 (模拟 --config flag)
	viper.SetConfigFile(configFile)

	cfg := Load()

	assert.Equal(t, 8888, cfg.Port)
	assert.Equal(t, "6.0.0", cfg.Version)
	assert.Equal(t, "/tmp/downloads", cfg.DownloadsDir)
}

func TestSetPort(t *testing.T) {
	cfg := &Config{Port: 8080}
	cfg.SetPort(9090)
	assert.Equal(t, 9090, cfg.Port)
}

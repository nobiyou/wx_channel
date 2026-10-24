package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"wx_channel/internal/config"
	"wx_channel/internal/utils"
)

func TestNewAppLogInitialization(t *testing.T) {
	if scenario := os.Getenv("WX_CHANNEL_LOGGER_TEST_SCENARIO"); scenario != "" {
		logFile := "configured.log"
		if scenario == "configured-failure" {
			logFile = filepath.Join("logs", "custom.log")
		}
		application := NewApp(&config.Config{LogFile: logFile, MaxLogSizeMB: 5})
		defer utils.GetLogger().Close()
		if scenario == "configured-success" {
			if !strings.Contains(application.LogInitMsg, "日志已初始化") {
				t.Fatalf("missing initialization success message: %s", application.LogInitMsg)
			}
			content, err := os.ReadFile(logFile)
			if err != nil || !strings.Contains(string(content), "加载配置") {
				t.Fatalf("startup did not use the configured log file: content=%s, error=%v", content, err)
			}
		} else if application.LogInitMsg != "" {
			t.Fatalf("failed file initialization was reported as successful: %s", application.LogInitMsg)
		}
		return
	}

	for _, scenario := range []string{"configured-success", "configured-failure"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "logs"), []byte("blocking file"), 0600); err != nil {
				t.Fatal(err)
			}
			command := exec.Command(os.Args[0], "-test.run=^TestNewAppLogInitialization$", "-test.v")
			command.Dir = dir
			command.Env = append(os.Environ(), "WX_CHANNEL_LOGGER_TEST_SCENARIO="+scenario)
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("startup failed: %v\n%s", err, output)
			}
			if scenario == "configured-failure" && !strings.Contains(string(output), "日志文件初始化失败") {
				t.Fatalf("initialization failure was not visible: %s", output)
			}
		})
	}
}

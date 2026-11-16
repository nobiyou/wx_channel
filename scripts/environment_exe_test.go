//go:build windows

package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnvironmentExecutableRunsWithoutScriptFiles(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(t.TempDir(), "中文目录 空格 & 符号")
	if err := os.Mkdir(fixture, 0755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(fixture, "wx-channel-environment.exe")
	build := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-ldflags=-s -w", "-o", exe, filepath.Join(cwd, "environment_tool_windows.go"))
	build.Env = append(os.Environ(), "GOOS=windows", "GOARCH=386", "CGO_ENABLED=0")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build standalone executable: %v\n%s", err, output)
	}

	run := func(t *testing.T, input string, env []string, args ...string) (string, error) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, exe, args...)
		cmd.Dir = t.TempDir() // Working directory differs from the software directory.
		cmd.Env = append(os.Environ(), env...)
		cmd.Stdin = strings.NewReader(input)
		cmd.WaitDelay = 3 * time.Second
		output, err := cmd.CombinedOutput()
		if ctx.Err() != nil {
			t.Fatalf("executable did not finish: %v\n%s", ctx.Err(), output)
		}
		return string(output), err
	}

	t.Run("help_uses_executable_directory", func(t *testing.T) {
		output, err := run(t, "", nil, "-Action", "help")
		if err != nil {
			t.Fatalf("help failed: %v\n%s", err, output)
		}
		if strings.Contains(output, "#< CLIXML") {
			t.Fatal("help output contains PowerShell serialization markup")
		}
		for _, want := range []string{"【使用说明】", "脚本文件：wx-channel-environment.exe", "软件目录：" + fixture} {
			if !strings.Contains(output, want) {
				t.Errorf("missing %q in help output:\n%s", want, output)
			}
		}
	})

	t.Run("menu_and_exit_keep_console", func(t *testing.T) {
		output, err := run(t, "5\n\n0\n\n", nil)
		if err != nil {
			t.Fatalf("menu failed: %v\n%s", err, output)
		}
		for _, want := range []string{"检查运行环境", "【使用说明】", "已退出。", "按 Enter 关闭窗口"} {
			if !strings.Contains(output, want) {
				t.Errorf("missing %q in menu output:\n%s", want, output)
			}
		}
	})

	t.Run("invalid_application_directory_reports_error", func(t *testing.T) {
		missing := filepath.Join(fixture, "不存在的目录")
		output, err := run(t, "", nil, "-AppRoot", missing, "-Action", "check")
		if err == nil || !strings.Contains(output, "不存在的目录") {
			t.Fatalf("missing contextual startup error (process: %v):\n%s", err, output)
		}
	})

	t.Run("unwritable_temporary_directory", func(t *testing.T) {
		blocked := filepath.Join(t.TempDir(), "not-a-directory")
		if err := os.WriteFile(blocked, []byte("blocked"), 0600); err != nil {
			t.Fatal(err)
		}
		output, err := run(t, "", []string{"TMP=" + blocked, "TEMP=" + blocked}, "-Action", "help")
		if err != nil || !strings.Contains(output, "【使用说明】") {
			t.Fatalf("startup needs a writable temporary directory (process: %v):\n%s", err, output)
		}
	})

	t.Run("missing_powershell_keeps_error_visible", func(t *testing.T) {
		output, err := run(t, "\n", []string{"SystemRoot=" + filepath.Join(fixture, "missing-windows")})
		if err == nil || !strings.Contains(output, "Windows PowerShell") || !strings.Contains(output, "按 Enter 关闭窗口") {
			t.Fatalf("missing retained startup error (process: %v):\n%s", err, output)
		}
	})

	t.Run("json_report_has_no_powershell_progress_markup", func(t *testing.T) {
		appRoot := t.TempDir()
		if err := os.WriteFile(filepath.Join(appRoot, "config.yaml"), []byte("port: 65534\n"), 0600); err != nil {
			t.Fatal(err)
		}
		output, runErr := run(t, "", nil, "-Action", "check", "-AppRoot", appRoot, "-SkipNetwork", "-Json", "-TimeoutSeconds", "1")
		var report struct {
			AppRoot string            `json:"app_root"`
			Checks  []json.RawMessage `json:"checks"`
		}
		if err := json.Unmarshal([]byte(output), &report); err != nil {
			t.Fatalf("invalid JSON report (process: %v): %v\n%s", runErr, err, output)
		}
		if report.AppRoot != appRoot || len(report.Checks) == 0 {
			t.Fatalf("incomplete JSON report: %s", output)
		}
	})

	t.Run("inherited_modules_preserve_native_diagnostic_results", func(t *testing.T) {
		appRoot := t.TempDir()
		if err := os.Mkdir(filepath.Join(appRoot, "downloads"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(appRoot, "config.yaml"), []byte("port: 65534\n"), 0600); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		host := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
		baseline := exec.CommandContext(ctx, host, "-NoLogo", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command",
			`$ProgressPreference = 'SilentlyContinue'; [Console]::OutputEncoding = New-Object Text.UTF8Encoding($false); & $env:WX_ENV_DIAGNOSTIC_TOOL -AppRoot $env:WX_ENV_DIAGNOSTIC_FIXTURE -SkipNetwork -Json -TimeoutSeconds 1`)
		for _, value := range os.Environ() {
			name, _, _ := strings.Cut(value, "=")
			if !strings.EqualFold(name, "PSModulePath") {
				baseline.Env = append(baseline.Env, value)
			}
		}
		baseline.Env = append(baseline.Env, "WX_ENV_DIAGNOSTIC_TOOL="+filepath.Join(cwd, "check-environment.ps1"), "WX_ENV_DIAGNOSTIC_FIXTURE="+appRoot)
		baseline.WaitDelay = 3 * time.Second
		before, beforeErr := baseline.CombinedOutput()
		after, afterErr := run(t, "", nil, "-Action", "check", "-AppRoot", appRoot, "-SkipNetwork", "-Json", "-TimeoutSeconds", "1")
		statuses := func(output []byte, runErr error) map[string]string {
			t.Helper()
			var report struct {
				Checks []struct {
					ID     string `json:"id"`
					Status string `json:"status"`
				} `json:"checks"`
			}
			if err := json.Unmarshal(output, &report); err != nil {
				t.Fatalf("invalid comparison report (process: %v): %v\n%s", runErr, err, output)
			}
			result := make(map[string]string)
			for _, check := range report.Checks {
				result[check.ID] = check.Status
			}
			return result
		}
		native, packaged := statuses(before, beforeErr), statuses([]byte(after), afterErr)
		for _, id := range []string{"certificate-store", "download-write"} {
			if native[id] == "" || native[id] != packaged[id] {
				t.Errorf("%s changed after packaging: native=%q, EXE=%q", id, native[id], packaged[id])
			}
		}
	})

	files, err := os.ReadDir(fixture)
	if err != nil || len(files) != 1 || files[0].Name() != filepath.Base(exe) {
		t.Fatalf("standalone executable left sidecar files: %v, %v", files, err)
	}
}

//go:build windows

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestEnvironmentDiagnosticsSeparatesPageConnectionFromAPIReadiness(t *testing.T) {
	host := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	if _, err := os.Stat(host); err != nil {
		t.Skip("Windows PowerShell 5.1 is unavailable")
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(t.TempDir(), "中文目录 含空格")
	if err := os.Mkdir(fixture, 0755); err != nil {
		t.Fatal(err)
	}
	harness := filepath.Join(fixture, "diagnose.ps1")
	if err := os.WriteFile(harness, []byte(environmentDiagnosticsHarness), 0600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name       string
		status     string
		connection string
		api        string
	}{
		{
			name:       "live_page_without_extension_api",
			status:     `{"connected":true,"clients":2,"ready_clients":0,"client_list":[{"application_fresh":true,"api_ready":false,"fresh":true},{"application_fresh":false,"api_ready":false,"fresh":false}]}`,
			connection: "PASS",
			api:        "INFO",
		},
		{
			name:       "stale_page_with_old_ready_state",
			status:     `{"connected":true,"clients":1,"ready_clients":1,"client_list":[{"application_fresh":false,"api_ready":true,"fresh":false}]}`,
			connection: "WARN",
			api:        "SKIP",
		},
		{
			name:       "no_connected_page",
			status:     `{"connected":false,"clients":0,"ready_clients":0,"client_list":[]}`,
			connection: "WARN",
			api:        "SKIP",
		},
		{
			name:       "live_page_with_failed_extension_probe",
			status:     `{"connected":true,"clients":1,"ready_clients":1,"client_list":[{"application_fresh":true,"api_ready":true,"fresh":false,"api_probe_status":"failed"}]}`,
			connection: "PASS",
			api:        "INFO",
		},
		{
			name:       "live_ready_page",
			status:     `{"connected":true,"clients":1,"ready_clients":1,"client_list":[{"application_fresh":true,"api_ready":true,"fresh":true,"api_probe_status":"ok"}]}`,
			connection: "PASS",
			api:        "PASS",
		},
		{
			name:       "older_status_without_heartbeat_fields",
			status:     `{"connected":true,"clients":1,"ready_clients":0}`,
			connection: "INFO",
			api:        "INFO",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(host, "-NoLogo", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", harness)
			cmd.Env = append(os.Environ(),
				"WX_ENV_DIAGNOSTIC_TOOL="+filepath.Join(cwd, "wx-channel-environment.cmd"),
				"WX_ENV_DIAGNOSTIC_FIXTURE="+fixture,
				"WX_ENV_DIAGNOSTIC_STATUS="+tc.status,
			)
			output, runErr := cmd.CombinedOutput()
			var report struct {
				Checks []struct {
					ID     string `json:"id"`
					Status string `json:"status"`
				} `json:"checks"`
				Info int `json:"info"`
				Warn int `json:"warn"`
				Fail int `json:"fail"`
			}
			if err := json.Unmarshal(output, &report); err != nil {
				t.Fatalf("invalid diagnostic output (process: %v): %v\n%s", runErr, err, output)
			}
			statuses := make(map[string]string)
			infoCount := 0
			for _, check := range report.Checks {
				statuses[check.ID] = check.Status
				if check.Status == "INFO" {
					infoCount++
				}
			}
			if got := statuses["channel-client"]; got != tc.connection {
				t.Errorf("page connection = %q, want %q", got, tc.connection)
			}
			if got := statuses["channel-api"]; got != tc.api {
				t.Errorf("extension API = %q, want %q", got, tc.api)
			}
			if report.Info != infoCount {
				t.Errorf("summary.info = %d, counted %d informational checks", report.Info, infoCount)
			}
			wantExit := 0
			if report.Fail > 0 {
				wantExit = 2
			} else if report.Warn > 0 {
				wantExit = 1
			}
			if got := cmd.ProcessState.ExitCode(); got != wantExit {
				t.Errorf("exit code = %d, want %d (process: %v)", got, wantExit, runErr)
			}
			if statuses["channel-client"] == "" {
				t.Logf("full diagnostic report: %s", output)
			}
		})
	}
}

func TestEnvironmentRepairPreviewPreservesUserFiles(t *testing.T) {
	host := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, fallback := range []string{"0", "1"} {
		t.Run("netstat_fallback_"+fallback, func(t *testing.T) {
			fixture := t.TempDir()
			appRoot := filepath.Join(fixture, "中文目录 含空格")
			if err := os.Mkdir(appRoot, 0755); err != nil {
				t.Fatal(err)
			}
			config := []byte("# 保留注释\r\nport: 2025\r\ndownload_dir: '视频 下载'\r\nlog_file: '日志/助手.log'\r\n")
			configPath := filepath.Join(appRoot, "config.yaml")
			if err := os.WriteFile(configPath, config, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(appRoot, "wx_channel.exe"), []byte("fixture"), 0600); err != nil {
				t.Fatal(err)
			}
			harness := filepath.Join(fixture, "preview.ps1")
			if err := os.WriteFile(harness, []byte(environmentDiagnosticsHarness), 0600); err != nil {
				t.Fatal(err)
			}
			reportPath := filepath.Join(fixture, "preview.json")
			cmd := exec.Command(host, "-NoLogo", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", harness)
			cmd.Env = append(os.Environ(),
				"WX_ENV_DIAGNOSTIC_TOOL="+filepath.Join(cwd, "wx-channel-environment.cmd"),
				"WX_ENV_DIAGNOSTIC_FIXTURE="+appRoot,
				"WX_ENV_DIAGNOSTIC_OPERATION=repair",
				"WX_ENV_DIAGNOSTIC_PORT_CONFLICT=1",
				"WX_ENV_DIAGNOSTIC_NETSTAT="+fallback,
				"WX_ENV_DIAGNOSTIC_REPORT="+reportPath,
			)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("repair preview failed: %v\n%s", err, output)
			}
			raw, err := os.ReadFile(reportPath)
			if err != nil {
				t.Fatal(err)
			}
			var report struct {
				Actions []struct {
					ID     string `json:"id"`
					Status string `json:"status"`
				} `json:"actions"`
				Fail int `json:"fail"`
			}
			if err := json.Unmarshal(raw, &report); err != nil {
				t.Fatal(err)
			}
			statuses := make(map[string]string)
			for _, action := range report.Actions {
				statuses[action.ID] = action.Status
			}
			for _, id := range []string{"download-directory", "log-directory", "config-port", "unblock-files"} {
				if statuses[id] != "SKIP" {
					t.Errorf("preview action %s = %q, want SKIP", id, statuses[id])
				}
			}
			if report.Fail != 0 || statuses["certificate"] != "DONE" {
				t.Errorf("certificate detection changed: %s", raw)
			}
			after, err := os.ReadFile(configPath)
			if err != nil || !bytes.Equal(config, after) {
				t.Fatalf("preview modified configuration: %v", err)
			}
			files, err := os.ReadDir(appRoot)
			if err != nil || len(files) != 2 {
				t.Fatalf("preview created directories or backups: %v, %v", files, err)
			}
		})
	}
}

// Exercise the real diagnostic entry points with fixed API responses. No live
// network, certificate store changes, or application processes are needed.
const environmentDiagnosticsHarness = `
$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = New-Object Text.UTF8Encoding($false)
function Get-NetTCPConnection {
    [CmdletBinding()]param([string]$State, [int]$LocalPort)
    if ($env:WX_ENV_DIAGNOSTIC_NETSTAT -eq '1') { throw 'CIM cmdlet unavailable' }
    if ($env:WX_ENV_DIAGNOSTIC_PORT_CONFLICT -eq '1' -and $LocalPort -ne 2025) { return }
    [pscustomobject]@{ OwningProcess = 123 }
}
function netstat.exe { '  TCP    127.0.0.1:2025    0.0.0.0:0    LISTENING    123' }
function Get-Process {
    [CmdletBinding()]param([int]$Id)
    if ($PSBoundParameters.ContainsKey('Id') -and $env:WX_ENV_DIAGNOSTIC_PORT_CONFLICT -eq '1') { [pscustomobject]@{ ProcessName = 'other_app' }; return }
    [pscustomobject]@{ ProcessName = 'wx_channel' }
    if (-not $PSBoundParameters.ContainsKey('Id')) { [pscustomobject]@{ ProcessName = 'WeChatAppEx' } }
}
function Import-Certificate { throw 'Unexpected certificate installation' }
function Unblock-File { throw 'Unexpected file unblock' }
function Get-ChildItem {
    [CmdletBinding()]param([string]$Path)
    if ($Path -like 'Cert:*') { [pscustomobject]@{ Subject = 'CN=SunnyNet' }; return }
    throw ('Unexpected enumeration: ' + $Path)
}
function Invoke-WebRequest {
    [CmdletBinding()]param([string]$Uri, [string]$Method, [int]$TimeoutSec, [switch]$UseBasicParsing, [hashtable]$Headers)
    switch (([uri]$Uri).AbsolutePath) {
        '/api/health' { $data = @{ status = 'ok' } }
        '/api/v1/certificate/status' { $data = @{ installed = $true } }
        '/api/v1/proxy/status' { $data = @{ running = $true } }
        '/api/channels/status' { $data = $env:WX_ENV_DIAGNOSTIC_STATUS | ConvertFrom-Json }
        default { throw ('Unexpected HTTP request: ' + $Uri) }
    }
    [pscustomobject]@{ StatusCode = 200; Content = (@{ code = 0; data = $data } | ConvertTo-Json -Depth 10) }
}
$tool = $env:WX_ENV_DIAGNOSTIC_TOOL
$raw = [IO.File]::ReadAllText($tool, [Text.Encoding]::UTF8)
$marker = ':WX_CHANNEL_ENV_PAYLOAD'
$payload = $raw.Substring($raw.LastIndexOf($marker) + $marker.Length)
$tokens = $null; $parseErrors = $null
$tree = [Management.Automation.Language.Parser]::ParseInput($payload, [ref]$tokens, [ref]$parseErrors)
if ($parseErrors.Count) { throw ($parseErrors | Out-String) }
foreach ($function in $tree.EndBlock.Statements) {
    if ($function -is [Management.Automation.Language.FunctionDefinitionAst]) { Invoke-Expression $function.Extent.Text }
}
if ($env:WX_ENV_DIAGNOSTIC_OPERATION -eq 'repair') {
    $report = Invoke-EnvironmentRepair -AppRoot $env:WX_ENV_DIAGNOSTIC_FIXTURE -RepairConfig -UnblockFiles -WhatIf -Json
    [IO.File]::WriteAllText($env:WX_ENV_DIAGNOSTIC_REPORT, ($report -join [Environment]::NewLine), (New-Object Text.UTF8Encoding($false)))
    exit $script:ToolExitCode
}
Invoke-EnvironmentCheck -AppRoot $env:WX_ENV_DIAGNOSTIC_FIXTURE -SkipNetwork -Json
exit $script:ToolExitCode
`

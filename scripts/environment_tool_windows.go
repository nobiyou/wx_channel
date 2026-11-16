//go:build ignore

// Build with build-environment-tool.ps1; the menu stays in the embedded CMD.
package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed wx-channel-environment.cmd
var environmentTool string

// Keep the payload in memory so startup works with a restricted TEMP directory.
const powerShellBootstrap = `
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
[Console]::OutputEncoding = New-Object Text.UTF8Encoding($false)
try {
    $bytes = [Convert]::FromBase64String($env:WX_CHANNEL_ENV_PAYLOAD)
    $stream = New-Object IO.MemoryStream
    $stream.Write($bytes, 0, $bytes.Length)
    $stream.Position = 0
    $gzip = New-Object IO.Compression.GZipStream($stream, [IO.Compression.CompressionMode]::Decompress)
    $reader = New-Object IO.StreamReader($gzip, [Text.Encoding]::UTF8)
    try { $payload = $reader.ReadToEnd() } finally { $reader.Dispose() }
    $parameters = @{}
    $options = $env:WX_CHANNEL_ENV_ARGUMENTS | ConvertFrom-Json
    foreach ($property in $options.PSObject.Properties) { $parameters[$property.Name] = $property.Value }
    & ([scriptblock]::Create($payload)) @parameters
} catch {
    Write-Host ($_ | Out-String) -ForegroundColor Red
    exit 2
}
`

func main() {
	code, err := run(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "启动失败：", err)
	}
	// Double-click launches have no arguments. Retain output, including errors.
	if len(os.Args) == 1 {
		fmt.Fprintln(os.Stdout, "\n按 Enter 关闭窗口……")
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	}
	os.Exit(code)
}

func run(args []string) (int, error) {
	executable, err := os.Executable()
	if err != nil {
		return 2, fmt.Errorf("无法确定工具所在目录：%w", err)
	}
	var powershell string
	root := os.Getenv("SystemRoot")
	// A 32-bit build uses Sysnative to reach native PowerShell on 64-bit Windows.
	if root != "" {
		for _, systemDirectory := range []string{"Sysnative", "System32"} {
			candidate := filepath.Join(root, systemDirectory, "WindowsPowerShell", "v1.0", "powershell.exe")
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				powershell = candidate
				break
			}
		}
	}
	if powershell == "" {
		return 2, fmt.Errorf("找不到系统 Windows PowerShell（SystemRoot=%s）", root)
	}
	const marker = ":WX_CHANNEL_ENV_PAYLOAD"
	start := strings.LastIndex(environmentTool, marker)
	if start < 0 {
		return 2, errors.New("内置环境工具脚本不完整，请重新获取 EXE 文件")
	}
	parameters := make(map[string]any)
	for i := 0; i < len(args); i++ {
		if !strings.HasPrefix(args[i], "-") {
			return 2, fmt.Errorf("无效参数：%s", args[i])
		}
		name := strings.TrimLeft(args[i], "-")
		switch strings.ToLower(name) {
		case "skipnetwork", "json", "whatif":
			parameters[name] = true
		case "approot", "action", "timeoutseconds", "authtoken":
			i++
			if i == len(args) {
				return 2, fmt.Errorf("参数 -%s 缺少值", name)
			}
			parameters[name] = args[i]
		default:
			return 2, fmt.Errorf("未知参数：%s", args[i])
		}
	}
	argumentJSON, err := json.Marshal(parameters)
	if err != nil {
		return 2, fmt.Errorf("无法准备启动参数：%w", err)
	}
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write([]byte(environmentTool[start+len(marker):])); err != nil {
		return 2, fmt.Errorf("无法准备内置脚本：%w", err)
	}
	if err := writer.Close(); err != nil {
		return 2, fmt.Errorf("无法准备内置脚本：%w", err)
	}
	// Only this fixed bootstrap is code; payload and arguments travel as data.
	cmd := exec.Command(powershell, "-NoLogo", "-NoProfile", "-ExecutionPolicy", "Bypass", "-OutputFormat", "Text", "-Command", powerShellBootstrap)
	var childEnvironment []string
	for _, value := range os.Environ() {
		name, _, _ := strings.Cut(value, "=")
		// Windows PowerShell must discover its own modules, not inherit PS 7 modules.
		if !strings.EqualFold(name, "PSModulePath") {
			childEnvironment = append(childEnvironment, value)
		}
	}
	// Parameters stay data, including paths containing quotes or shell characters.
	cmd.Env = append(childEnvironment,
		"WX_CHANNEL_ENV_TOOL_FILE="+executable,
		"WX_CHANNEL_ENV_PAYLOAD="+base64.StdEncoding.EncodeToString(compressed.Bytes()),
		"WX_CHANNEL_ENV_ARGUMENTS="+string(argumentJSON),
	)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			return exitError.ExitCode(), nil // Keep diagnostic warning/failure exit codes.
		}
		return 2, fmt.Errorf("无法运行 Windows PowerShell：%w", err)
	}
	return 0, nil
}

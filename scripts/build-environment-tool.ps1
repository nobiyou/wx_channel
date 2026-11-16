[CmdletBinding()]
param([string]$OutputPath = '')

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $OutputPath = Join-Path $repoRoot 'wx-channel-environment.exe'
}
$OutputPath = [IO.Path]::GetFullPath($OutputPath)
$null = New-Item -ItemType Directory -Path (Split-Path -Parent $OutputPath) -Force
$buildEnvironment = @{}
foreach ($name in @('GOOS', 'GOARCH', 'CGO_ENABLED')) {
    $buildEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
}
Push-Location -LiteralPath $repoRoot
try {
    $env:GOOS = 'windows'
    $env:GOARCH = '386'
    $env:CGO_ENABLED = '0'
    & go build -trimpath -buildvcs=false -ldflags '-s -w' -o $OutputPath (Join-Path $PSScriptRoot 'environment_tool_windows.go')
    if ($LASTEXITCODE -ne 0) { throw "EXE 编译失败，退出码：$LASTEXITCODE" }
    Write-Host "已生成：$OutputPath"
    Write-Host '适用 Windows 10/11（32 位及 64 位），只需分发此 EXE 文件。'
} finally {
    Pop-Location
    foreach ($name in $buildEnvironment.Keys) {
        [Environment]::SetEnvironmentVariable($name, $buildEnvironment[$name], 'Process')
    }
}

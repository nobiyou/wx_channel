[CmdletBinding()]
param(
    [string]$RepoRoot = "",
    [string]$ExePath = "",
    [int]$Port = 2025,
    [switch]$EmitOnly
)

$ErrorActionPreference = "Stop"

function Resolve-DefaultRepoRoot {
    return (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot "..")).Path
}

if (-not $RepoRoot) { $RepoRoot = Resolve-DefaultRepoRoot }
else { $RepoRoot = (Resolve-Path -LiteralPath $RepoRoot).Path }

if (-not $ExePath) { $ExePath = Join-Path $RepoRoot "wx_channel.exe" }
$ExePath = (Resolve-Path -LiteralPath $ExePath).Path

$varDir = Join-Path $RepoRoot "var"
$stdoutLog = Join-Path $varDir "insight-data-plane.stdout.log"
$stderrLog = Join-Path $varDir "insight-data-plane.stderr.log"

$envPlan = [ordered]@{
    WX_CHANNEL_PORT = "$Port"
    WX_CHANNEL_RADAR_ENABLED = "false"
    WX_CHANNEL_CLOUD_ENABLED = "false"
}

if ($EmitOnly.IsPresent) {
    [pscustomobject]@{
        repoRoot = $RepoRoot
        exePath = $ExePath
        port = $Port
        proxyPort = $Port
        apiPort = $Port + 1
        statusURL = "http://127.0.0.1:$($Port + 1)/api/channels/status"
        env = $envPlan
        stdoutLog = $stdoutLog
        stderrLog = $stderrLog
        mode = "wx-channel-insight-data-plane"
    } | ConvertTo-Json -Depth 5
    return
}

New-Item -ItemType Directory -Force -Path $varDir | Out-Null
foreach ($entry in $envPlan.GetEnumerator()) {
    [Environment]::SetEnvironmentVariable([string]$entry.Key, [string]$entry.Value, "Process")
}

$proc = Start-Process -FilePath $ExePath -WorkingDirectory $RepoRoot -PassThru -WindowStyle Hidden `
    -RedirectStandardOutput $stdoutLog -RedirectStandardError $stderrLog

[pscustomobject]@{
    pid = $proc.Id
    repoRoot = $RepoRoot
    exePath = $ExePath
    proxyPort = $Port
    apiPort = $Port + 1
    statusURL = "http://127.0.0.1:$($Port + 1)/api/channels/status"
    env = $envPlan
    stdoutLog = $stdoutLog
    stderrLog = $stderrLog
} | ConvertTo-Json -Depth 5

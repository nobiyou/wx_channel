$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
$configPath = Join-Path $repoRoot 'internal\config\config.go'
$originalConfig = Get-Content -LiteralPath $configPath -Raw
$pattern = 'viper\.SetDefault\("cloud_enabled",\s*(true|false)\)'
$utf8NoBom = [System.Text.UTF8Encoding]::new($false)

function Write-Utf8File {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,
        [Parameter(Mandatory = $true)]
        [string]$Content
    )

    [System.IO.File]::WriteAllText($Path, $Content, $utf8NoBom)
}

function Set-CloudEnabledDefault {
    param(
        [Parameter(Mandatory = $true)]
        [bool]$Enabled
    )

    $replacement = if ($Enabled) {
        'viper.SetDefault("cloud_enabled", true)'
    } else {
        'viper.SetDefault("cloud_enabled", false)'
    }

    $current = Get-Content -LiteralPath $configPath -Raw
    $match = [System.Text.RegularExpressions.Regex]::Match($current, $pattern)
    if (-not $match.Success) {
        throw "未找到 cloud_enabled 默认值配置，无法切换构建模式。"
    }
    $updated = $current.Remove($match.Index, $match.Length).Insert($match.Index, $replacement)
    Write-Utf8File -Path $configPath -Content $updated
}

function Invoke-GoBuild {
    param(
        [Parameter(Mandatory = $true)]
        [string]$OutputName
    )

    Push-Location $repoRoot
    try {
        $env:GOOS = ''
        $env:GOARCH = ''
        $env:CGO_ENABLED = '1'
        $ldflags = "-w -s -extldflags '-static'"
        go build -mod=vendor "-ldflags=$ldflags" -o $OutputName
    }
    finally {
        Pop-Location
    }
}

try {
    Push-Location $repoRoot
    try {
        Write-Host '==> 生成 Windows 资源文件' -ForegroundColor Cyan
        go-winres make
    }
    finally {
        Pop-Location
    }

    Write-Host '==> 打包 Hub 版 (cloud_enabled=true)' -ForegroundColor Cyan
    Set-CloudEnabledDefault -Enabled $true
    Invoke-GoBuild -OutputName 'wx_channel_cloud.exe'

    Write-Host '==> 打包普通版 (cloud_enabled=false)' -ForegroundColor Cyan
    Set-CloudEnabledDefault -Enabled $false
    Invoke-GoBuild -OutputName 'wx_channel.exe'
}
finally {
    Write-Utf8File -Path $configPath -Content $originalConfig
    Write-Host '==> 已恢复 internal/config/config.go 原始内容' -ForegroundColor DarkGray
}

Write-Host '==> 打包完成' -ForegroundColor Green

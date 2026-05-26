$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
$configPath = Join-Path $repoRoot 'internal\config\config.go'
$utf8NoBom = [System.Text.UTF8Encoding]::new($false)
$originalConfigBytes = [System.IO.File]::ReadAllBytes($configPath)
$pattern = 'viper\.SetDefault\("cloud_enabled",\s*(true|false)\)'

function Write-Utf8File {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,
        [Parameter(Mandatory = $true)]
        [string]$Content
    )

    [System.IO.File]::WriteAllText($Path, $Content, $utf8NoBom)
}

function Read-Utf8File {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    return $utf8NoBom.GetString([System.IO.File]::ReadAllBytes($Path))
}

function Assert-LastExitCode {
    param(
        [Parameter(Mandatory = $true)]
        [string]$CommandName
    )

    if ($LASTEXITCODE -ne 0) {
        throw "$CommandName failed with exit code $LASTEXITCODE."
    }
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

    $current = Read-Utf8File -Path $configPath
    $match = [System.Text.RegularExpressions.Regex]::Match($current, $pattern)
    if (-not $match.Success) {
        throw 'Could not locate cloud_enabled default in internal/config/config.go.'
    }

    $updated = $current.Remove($match.Index, $match.Length).Insert($match.Index, $replacement)
    Write-Utf8File -Path $configPath -Content $updated
}

function Invoke-GoWinres {
    $command = Get-Command go-winres -ErrorAction SilentlyContinue
    if ($null -ne $command) {
        & $command.Source make
        Assert-LastExitCode -CommandName 'go-winres make'
        return
    }

    go run -mod=mod github.com/tc-hib/go-winres@latest make
    Assert-LastExitCode -CommandName 'go run github.com/tc-hib/go-winres@latest make'
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
        if (Test-Path $OutputName) {
            Remove-Item $OutputName -Force
        }
        go build -mod=vendor "-ldflags=$ldflags" -o $OutputName
        Assert-LastExitCode -CommandName ("go build -o " + $OutputName)
    }
    finally {
        Pop-Location
    }
}

try {
    Push-Location $repoRoot
    try {
        Write-Host '==> Generate Windows resources' -ForegroundColor Cyan
        Invoke-GoWinres
    }
    finally {
        Pop-Location
    }

    Write-Host '==> Build cloud variant (cloud_enabled=true)' -ForegroundColor Cyan
    Set-CloudEnabledDefault -Enabled $true
    Invoke-GoBuild -OutputName 'wx_channel_cloud.exe'

    Write-Host '==> Build standard variant (cloud_enabled=false)' -ForegroundColor Cyan
    Set-CloudEnabledDefault -Enabled $false
    Invoke-GoBuild -OutputName 'wx_channel.exe'
}
finally {
    [System.IO.File]::WriteAllBytes($configPath, $originalConfigBytes)
    Write-Host '==> Restored internal/config/config.go' -ForegroundColor DarkGray
}

Write-Host '==> Dual build complete' -ForegroundColor Green

$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
$configPath = Join-Path $repoRoot 'internal\config\config.go'
$versionPath = Join-Path $repoRoot 'internal\version\version.go'
$utf8NoBom = [System.Text.UTF8Encoding]::new($false)
$originalConfigBytes = [System.IO.File]::ReadAllBytes($configPath)
$originalConfig = $utf8NoBom.GetString($originalConfigBytes)

$patterns = @{
    'cloud_enabled' = 'viper\.SetDefault\("cloud_enabled",\s*(true|false)\)'
    'radar_enabled' = 'viper\.SetDefault\("radar_enabled",\s*(true|false)\)'
}

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

function Get-AppVersion {
    $content = Read-Utf8File -Path $versionPath
    $match = [System.Text.RegularExpressions.Regex]::Match($content, 'const Current = "([^"]+)"')
    if (-not $match.Success) {
        throw 'Could not read version from internal/version/version.go.'
    }
    return $match.Groups[1].Value
}

function Set-ConfigDefault {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Key,
        [Parameter(Mandatory = $true)]
        [bool]$Enabled
    )

    if ($Key -notin @('cloud_enabled', 'radar_enabled')) {
        throw "Unsupported config key: $Key"
    }

    $replacement = if ($Enabled) {
        "viper.SetDefault(""$Key"", true)"
    } else {
        "viper.SetDefault(""$Key"", false)"
    }

    $current = Read-Utf8File -Path $configPath
    $match = [System.Text.RegularExpressions.Regex]::Match($current, $patterns[$Key])
    if (-not $match.Success) {
        throw "Could not locate default config for $Key."
    }

    $updated = $current.Remove($match.Index, $match.Length).Insert($match.Index, $replacement)
    Write-Utf8File -Path $configPath -Content $updated
}

function Set-VariantDefaults {
    param(
        [Parameter(Mandatory = $true)]
        [bool]$CloudEnabled,
        [Parameter(Mandatory = $true)]
        [bool]$RadarEnabled
    )

    Set-ConfigDefault -Key 'cloud_enabled' -Enabled $CloudEnabled
    Set-ConfigDefault -Key 'radar_enabled' -Enabled $RadarEnabled
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

function New-ZipPackage {
    param(
        [Parameter(Mandatory = $true)]
        [string]$ReleaseExe,
        [Parameter(Mandatory = $true)]
        [string]$PackageExeName,
        [Parameter(Mandatory = $true)]
        [string]$ZipPath
    )

    $tempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("wx_channel_pkg_" + [System.Guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Force -Path $tempDir | Out-Null

    try {
        Copy-Item (Join-Path $repoRoot 'README.md') $tempDir -Force
        Copy-Item (Join-Path $repoRoot 'config.yaml.example') $tempDir -Force
        Copy-Item (Join-Path $repoRoot 'config.yaml.full') $tempDir -Force
        Copy-Item $ReleaseExe (Join-Path $tempDir $PackageExeName) -Force

        if (Test-Path $ZipPath) {
            Remove-Item $ZipPath -Force
        }
        Compress-Archive -Path (Join-Path $tempDir '*') -DestinationPath $ZipPath -Force
    }
    finally {
        if (Test-Path $tempDir) {
            Remove-Item $tempDir -Recurse -Force
        }
    }
}

$version = Get-AppVersion
$releaseDir = Join-Path $repoRoot ("release\v{0}" -f $version)
$variants = @(
    [pscustomobject]@{
        Label = 'standard'
        RootExe = 'wx_channel.exe'
        ReleaseName = "wx_channel_v$version.exe"
        ReleaseZip = "wx_channel_v$version.zip"
        CloudEnabled = $false
        RadarEnabled = $false
        PackageExeName = 'wx_channel.exe'
    },
    [pscustomobject]@{
        Label = 'cloud'
        RootExe = 'wx_channel_cloud.exe'
        ReleaseName = "wx_channel_cloud_v$version.exe"
        ReleaseZip = "wx_channel_cloud_v$version.zip"
        CloudEnabled = $true
        RadarEnabled = $false
        PackageExeName = 'wx_channel_cloud.exe'
    },
    [pscustomobject]@{
        Label = 'radar'
        RootExe = 'wx_channel_radar.exe'
        ReleaseName = "wx_channel_radar_v$version.exe"
        ReleaseZip = "wx_channel_radar_v$version.zip"
        CloudEnabled = $false
        RadarEnabled = $true
        PackageExeName = 'wx_channel_radar.exe'
    }
)

try {
    New-Item -ItemType Directory -Force -Path $releaseDir | Out-Null
    foreach ($staleItem in @(
        "wx_channel_v$version.exe",
        "wx_channel_v$version.zip",
        "wx_channel_cloud_v$version.exe",
        "wx_channel_cloud_v$version.zip",
        "wx_channel_radar_v$version.exe",
        "wx_channel_radar_v$version.zip"
    )) {
        $stalePath = Join-Path $releaseDir $staleItem
        if (Test-Path $stalePath) {
            Remove-Item $stalePath -Force
        }
    }

    Push-Location $repoRoot
    try {
        Write-Host '==> Generate Windows resources' -ForegroundColor Cyan
        Invoke-GoWinres
    }
    finally {
        Pop-Location
    }

    foreach ($variant in $variants) {
        Write-Host ("==> Build {0} (cloud_enabled={1}, radar_enabled={2})" -f $variant.Label, $variant.CloudEnabled.ToString().ToLowerInvariant(), $variant.RadarEnabled.ToString().ToLowerInvariant()) -ForegroundColor Cyan

        Set-VariantDefaults -CloudEnabled $variant.CloudEnabled -RadarEnabled $variant.RadarEnabled
        Invoke-GoBuild -OutputName $variant.RootExe

        $releaseExe = Join-Path $releaseDir $variant.ReleaseName
        $releaseZip = Join-Path $releaseDir $variant.ReleaseZip

        Copy-Item (Join-Path $repoRoot $variant.RootExe) $releaseExe -Force
        New-ZipPackage -ReleaseExe $releaseExe -PackageExeName $variant.PackageExeName -ZipPath $releaseZip
    }
}
finally {
    [System.IO.File]::WriteAllBytes($configPath, $originalConfigBytes)
    Write-Host '==> Restored internal/config/config.go' -ForegroundColor DarkGray
}

Get-ChildItem $releaseDir | Select-Object Name, Length, LastWriteTime | Format-Table -AutoSize
Write-Host '==> Variant build complete' -ForegroundColor Green

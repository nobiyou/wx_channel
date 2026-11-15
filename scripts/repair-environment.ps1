<#
.SYNOPSIS
    修复微信视频号下载助手常见的 Windows 环境问题。

.DESCRIPTION
    默认执行安全、局部的修复：创建下载和日志目录，并尝试安装 SunnyNet
    根证书。需要明确指定开关才会修改配置、解除文件阻止、添加防火墙规则或
    重启应用。修改配置前会自动生成带时间戳的备份。

.EXAMPLE
    .\scripts\repair-environment.ps1

.EXAMPLE
    .\scripts\repair-environment.ps1 -RepairConfig -UnblockFiles

.EXAMPLE
    .\scripts\repair-environment.ps1 -InstallMachineCertificate -AddFirewallRule -RestartApp
#>

[CmdletBinding(SupportsShouldProcess = $true, ConfirmImpact = 'Medium')]
param(
    [string]$AppRoot = '',
    [int]$Port = 0,
    [int]$TimeoutSeconds = 8,
    [string]$CertificatePath = '',
    [string]$AuthToken = '',
    [switch]$SkipCertificate,
    [switch]$RepairConfig,
    [switch]$UnblockFiles,
    [switch]$AddFirewallRule,
    [switch]$InstallMachineCertificate,
    [switch]$RestartApp,
    [switch]$Json
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$script:Actions = @()

if ([string]::IsNullOrWhiteSpace($AppRoot)) {
    $scriptPath = $MyInvocation.MyCommand.Path
    if ([string]::IsNullOrWhiteSpace($scriptPath)) { $scriptPath = $PSCommandPath }
    $scriptDirectory = Split-Path -Parent $scriptPath
    $AppRoot = Split-Path -Parent $scriptDirectory
}

function Add-Action {
    param(
        [string]$Id,
        [ValidateSet('DONE', 'SKIP', 'WARN', 'FAIL')]
        [string]$Status,
        [string]$Detail
    )
    $script:Actions += [pscustomobject]@{
        id     = $Id
        status = $Status
        detail = $Detail
    }
}

function Get-ConfigValue {
    param([string]$Path, [string]$Key, [string]$Default = '')
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { return $Default }
    try {
        $line = Get-Content -LiteralPath $Path -ErrorAction Stop |
            Where-Object { $_ -match "^\s*$([regex]::Escape($Key))\s*:" } |
            Select-Object -First 1
        if ($null -eq $line) { return $Default }
        $value = ($line -replace "^\s*$([regex]::Escape($Key))\s*:\s*", '').Trim()
        return (($value -replace '\s+#.*$', '').Trim('"', "'"))
    } catch {
        return $Default
    }
}

function Test-IsAdministrator {
    try {
        $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
        $principal = New-Object Security.Principal.WindowsPrincipal($identity)
        return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
    } catch {
        return $false
    }
}

function Get-ListenPortOwner {
    param([int]$TargetPort)
    try {
        if (Get-Command Get-NetTCPConnection -ErrorAction SilentlyContinue) {
            $connections = @(Get-NetTCPConnection -State Listen -LocalPort $TargetPort -ErrorAction SilentlyContinue)
            if ($connections.Count -gt 0) {
                return @($connections | ForEach-Object {
                    $name = ''
                    try { $name = (Get-Process -Id $_.OwningProcess -ErrorAction Stop).ProcessName } catch { }
                    if ($name) { "PID=$($_.OwningProcess), Process=$name" } else { "PID=$($_.OwningProcess)" }
                }) -join '; '
            }
        }
    } catch { }
    return ''
}

function Find-FreePort {
    param([int]$StartPort)
    for ($candidate = $StartPort; $candidate -le ($StartPort + 50); $candidate++) {
        if (-not (Get-ListenPortOwner -TargetPort $candidate) -and -not (Get-ListenPortOwner -TargetPort ($candidate + 1))) { return $candidate }
    }
    return 0
}

function Invoke-Api {
    param(
        [string]$Uri,
        [ValidateSet('GET', 'POST')]
        [string]$Method = 'GET',
        [string]$OutFile = ''
    )
    try {
        $request = @{
            Uri         = $Uri
            Method      = $Method
            TimeoutSec  = $TimeoutSeconds
            ErrorAction = 'Stop'
        }
        if ($AuthToken) { $request.Headers = @{ 'X-Local-Auth' = $AuthToken } }
        if ($PSVersionTable.PSVersion.Major -le 5) { $request.UseBasicParsing = $true }
        if ($OutFile) {
            $request.OutFile = $OutFile
            $response = Invoke-WebRequest @request
            return [pscustomobject]@{ ok = $true; body = ''; statusCode = [int]$response.StatusCode; error = '' }
        }
        $response = Invoke-WebRequest @request
        return [pscustomobject]@{ ok = $true; body = [string]$response.Content; statusCode = [int]$response.StatusCode; error = '' }
    } catch {
        return [pscustomobject]@{ ok = $false; body = ''; statusCode = 0; error = $_.Exception.Message }
    }
}

function Test-CertificateInstalled {
    foreach ($store in @('Cert:\CurrentUser\Root', 'Cert:\LocalMachine\Root')) {
        try {
            $matches = @(Get-ChildItem -Path $store -ErrorAction Stop | Where-Object { $_.Subject -match '(?i)SunnyNet' })
            if ($matches.Count -gt 0) { return $true }
        } catch { }
    }
    return $false
}

function Get-CertificatePathFromApi {
    param([string]$BaseUrl, [string]$Destination)
    $download = Invoke-Api -Uri "$BaseUrl/api/v1/certificate/download" -OutFile $Destination
    if (-not $download.ok) {
        Remove-Item -LiteralPath $Destination -Force -ErrorAction SilentlyContinue
        return $download.error
    }
    return ''
}

function Validate-CertificateFile {
    param([string]$Path)
    try {
        if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { return '文件不存在。' }
        $certificate = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2($Path)
        if ($certificate.Subject -notmatch '(?i)SunnyNet') {
            return "证书主题不是 SunnyNet：$($certificate.Subject)"
        }
        if ($certificate.NotAfter -lt (Get-Date)) { return '证书已过期。' }
        return ''
    } catch {
        return "证书文件无效：$($_.Exception.Message)"
    }
}

function Import-CertificateFile {
    param([string]$Path, [string]$Store)
    if (-not (Get-Command Import-Certificate -ErrorAction SilentlyContinue)) {
        return '系统没有 Import-Certificate 命令。'
    }
    try {
        $null = Import-Certificate -FilePath $Path -CertStoreLocation $Store -ErrorAction Stop
        return ''
    } catch {
        return $_.Exception.Message
    }
}

function Ensure-Directory {
    param([string]$Path, [string]$Label)
    if (Test-Path -LiteralPath $Path -PathType Container) {
        Add-Action -Id $Label -Status 'DONE' -Detail "目录已存在：$Path"
        return $true
    }
    if ($PSCmdlet.ShouldProcess($Path, '创建目录')) {
        try {
            New-Item -ItemType Directory -Path $Path -Force -ErrorAction Stop | Out-Null
            Add-Action -Id $Label -Status 'DONE' -Detail "已创建：$Path"
            return $true
        } catch {
            Add-Action -Id $Label -Status 'FAIL' -Detail $_.Exception.Message
            return $false
        }
    }
    Add-Action -Id $Label -Status 'SKIP' -Detail "WhatIf/用户取消：$Path"
    return $false
}

function Backup-File {
    param([string]$Path)
    $stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
    $backup = "$Path.bak-$stamp"
    Copy-Item -LiteralPath $Path -Destination $backup -Force -ErrorAction Stop
    return $backup
}

function Set-ConfigPort {
    param([string]$Path, [int]$NewPort)
    $content = Get-Content -LiteralPath $Path -Raw -ErrorAction Stop
    if ($content -match '(?m)^\s*port\s*:') {
        $content = [regex]::Replace($content, '(?m)^(\s*port\s*:\s*)\d+', { param($match) "$($match.Groups[1].Value)$NewPort" }, 1)
    } else {
        $content = "port: $NewPort$([Environment]::NewLine)$content"
    }
    Set-Content -LiteralPath $Path -Value $content -Encoding UTF8 -ErrorAction Stop
}

try {
    $resolvedRoot = (Resolve-Path -LiteralPath $AppRoot -ErrorAction Stop).Path
} catch {
    throw "应用目录不存在：$AppRoot"
}

$configPath = Join-Path $resolvedRoot 'config.yaml'
$configuredPortText = Get-ConfigValue -Path $configPath -Key 'port' -Default '2025'
$configuredPort = 2025
$null = [int]::TryParse($configuredPortText, [ref]$configuredPort)
if ($configuredPort -le 0) { $configuredPort = 2025 }
$effectivePort = $Port
if ($effectivePort -le 0) { $effectivePort = $configuredPort }

$downloadDirValue = Get-ConfigValue -Path $configPath -Key 'download_dir' -Default 'downloads'
$logFileValue = Get-ConfigValue -Path $configPath -Key 'log_file' -Default 'logs/wx_channel.log'
$certFileValue = Get-ConfigValue -Path $configPath -Key 'cert_file' -Default 'SunnyRoot.cer'
$downloadDir = if ([IO.Path]::IsPathRooted($downloadDirValue)) { $downloadDirValue } else { Join-Path $resolvedRoot $downloadDirValue }
$logFilePath = if ([IO.Path]::IsPathRooted($logFileValue)) { $logFileValue } else { Join-Path $resolvedRoot $logFileValue }
$logDir = Split-Path -Parent $logFilePath
$fallbackCertPath = Join-Path $downloadDir $certFileValue

if ($env:OS -ne 'Windows_NT') {
    Add-Action -Id 'platform' -Status 'FAIL' -Detail '此修复脚本只支持 Windows。'
} else {
    Add-Action -Id 'platform' -Status 'DONE' -Detail 'Windows 环境。'
}

$null = Ensure-Directory -Path $downloadDir -Label 'download-directory'
$null = Ensure-Directory -Path $logDir -Label 'log-directory'

if ($RepairConfig) {
    if (-not (Test-Path -LiteralPath $configPath -PathType Leaf)) {
        if ($PSCmdlet.ShouldProcess($configPath, '创建最小配置')) {
            try {
                $minimalConfig = @(
                    '# 由 repair-environment.ps1 创建的最小配置'
                    "port: $effectivePort"
                    'download_dir: downloads'
                    'log_file: logs/wx_channel.log'
                    'auto_open_channels: true'
                ) -join [Environment]::NewLine
                Set-Content -LiteralPath $configPath -Value $minimalConfig -Encoding UTF8 -ErrorAction Stop
                Add-Action -Id 'config-create' -Status 'DONE' -Detail "已创建配置：$configPath"
            } catch {
                Add-Action -Id 'config-create' -Status 'FAIL' -Detail $_.Exception.Message
            }
        } else {
            Add-Action -Id 'config-create' -Status 'SKIP' -Detail 'WhatIf/用户取消。'
        }
    } else {
        $owner = Get-ListenPortOwner -TargetPort $effectivePort
        $appRunning = @(Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.ProcessName -match '^wx_channel($|_)' }).Count -gt 0
        if ($owner -and -not ($appRunning -and $owner -match '(?i)wx_channel')) {
            $freePort = Find-FreePort -StartPort ([Math]::Max(2025, $effectivePort + 1))
            if ($freePort -gt 0 -and $freePort -ne $effectivePort) {
                if ($PSCmdlet.ShouldProcess($configPath, "将端口从 $effectivePort 调整为 $freePort")) {
                    try {
                        $backup = Backup-File -Path $configPath
                        Set-ConfigPort -Path $configPath -NewPort $freePort
                        $effectivePort = $freePort
                        Add-Action -Id 'config-port' -Status 'DONE' -Detail "端口被占用，已改为 $freePort；备份：$backup"
                    } catch {
                        Add-Action -Id 'config-port' -Status 'FAIL' -Detail $_.Exception.Message
                    }
                } else {
                    Add-Action -Id 'config-port' -Status 'SKIP' -Detail 'WhatIf/用户取消。'
                }
            } else {
                Add-Action -Id 'config-port' -Status 'FAIL' -Detail "端口 $effectivePort 被占用，且未找到可用端口。"
            }
        } else {
            Add-Action -Id 'config-port' -Status 'DONE' -Detail "配置端口 $effectivePort 可用或由当前应用占用。"
        }
    }
} else {
    Add-Action -Id 'config' -Status 'SKIP' -Detail '未指定 -RepairConfig，未修改配置文件。'
}

$baseUrl = "http://127.0.0.1:$effectivePort"
$apiHealth = Invoke-Api -Uri "$baseUrl/api/health"
$apiAvailable = $apiHealth.ok

if ($SkipCertificate) {
    Add-Action -Id 'certificate' -Status 'SKIP' -Detail '使用 -SkipCertificate 跳过证书修复。'
} elseif (Test-CertificateInstalled) {
    Add-Action -Id 'certificate' -Status 'DONE' -Detail 'SunnyNet 根证书已经安装。'
} else {
    $candidate = $CertificatePath
    if ($candidate -and -not [IO.Path]::IsPathRooted($candidate)) { $candidate = Join-Path $resolvedRoot $candidate }
    if (-not $candidate -and (Test-Path -LiteralPath $fallbackCertPath -PathType Leaf)) { $candidate = $fallbackCertPath }

    if ($apiAvailable -and -not $candidate) {
        if ($PSCmdlet.ShouldProcess($fallbackCertPath, '从运行中的应用下载 SunnyNet 证书')) {
            try {
                $apiError = Get-CertificatePathFromApi -BaseUrl $baseUrl -Destination $fallbackCertPath
                if ($apiError) { Add-Action -Id 'certificate-download' -Status 'WARN' -Detail $apiError }
                else { $candidate = $fallbackCertPath; Add-Action -Id 'certificate-download' -Status 'DONE' -Detail "证书已保存：$fallbackCertPath" }
            } catch {
                Add-Action -Id 'certificate-download' -Status 'WARN' -Detail $_.Exception.Message
            }
        } else {
            Add-Action -Id 'certificate-download' -Status 'SKIP' -Detail 'WhatIf/用户取消。'
        }
    }

    if (-not $candidate) {
        Add-Action -Id 'certificate' -Status 'FAIL' -Detail '未找到证书文件，且应用 API 不可用；请先启动 wx_channel.exe，或用 -CertificatePath 指定 SunnyRoot.cer。'
    } else {
        $validationError = Validate-CertificateFile -Path $candidate
        if ($validationError) {
            Add-Action -Id 'certificate' -Status 'FAIL' -Detail $validationError
        } else {
            $importError = ''
            if ($PSCmdlet.ShouldProcess('Cert:\CurrentUser\Root', "导入 $candidate")) {
                $importError = Import-CertificateFile -Path $candidate -Store 'Cert:\CurrentUser\Root'
            }
            if ($importError) {
                Add-Action -Id 'certificate-user' -Status 'FAIL' -Detail $importError
            } else {
                Add-Action -Id 'certificate-user' -Status 'DONE' -Detail '已安装到当前用户的受信任根证书存储。'
            }

            if ($InstallMachineCertificate) {
                if (-not (Test-IsAdministrator)) {
                    Add-Action -Id 'certificate-machine' -Status 'FAIL' -Detail '安装到本地计算机证书存储需要管理员权限。'
                } elseif ($PSCmdlet.ShouldProcess('Cert:\LocalMachine\Root', "导入 $candidate")) {
                    $machineError = Import-CertificateFile -Path $candidate -Store 'Cert:\LocalMachine\Root'
                    if ($machineError) { Add-Action -Id 'certificate-machine' -Status 'FAIL' -Detail $machineError }
                    else { Add-Action -Id 'certificate-machine' -Status 'DONE' -Detail '已安装到本地计算机的受信任根证书存储。' }
                } else {
                    Add-Action -Id 'certificate-machine' -Status 'SKIP' -Detail 'WhatIf/用户取消。'
                }
            }
        }
    }
}

if ($UnblockFiles) {
    $targets = @()
    foreach ($fileName in @('wx_channel.exe', 'wx_channel_cloud.exe', 'wx_channel_radar.exe', 'wx_channel_optimized.exe', 'wx_channel_packed.exe')) {
        $path = Join-Path $resolvedRoot $fileName
        if (Test-Path -LiteralPath $path -PathType Leaf) { $targets += $path }
    }
    if ($targets.Count -eq 0) {
        Add-Action -Id 'unblock-files' -Status 'SKIP' -Detail '未找到需要解除阻止的程序文件。'
    } elseif ($PSCmdlet.ShouldProcess(($targets -join ', '), '解除 Windows 下载阻止标记')) {
        try {
            foreach ($target in $targets) { Unblock-File -LiteralPath $target -ErrorAction Stop }
            Add-Action -Id 'unblock-files' -Status 'DONE' -Detail "已处理 $($targets.Count) 个程序文件。"
        } catch {
            Add-Action -Id 'unblock-files' -Status 'FAIL' -Detail $_.Exception.Message
        }
    } else {
        Add-Action -Id 'unblock-files' -Status 'SKIP' -Detail 'WhatIf/用户取消。'
    }
} else {
    Add-Action -Id 'unblock-files' -Status 'SKIP' -Detail '未指定 -UnblockFiles。'
}

if ($AddFirewallRule) {
    $exePath = Join-Path $resolvedRoot 'wx_channel.exe'
    if (-not (Test-Path -LiteralPath $exePath -PathType Leaf)) {
        Add-Action -Id 'firewall' -Status 'FAIL' -Detail "未找到程序：$exePath"
    } elseif (-not (Test-IsAdministrator)) {
        Add-Action -Id 'firewall' -Status 'FAIL' -Detail '添加防火墙规则需要管理员权限。'
    } elseif (-not (Get-Command New-NetFirewallRule -ErrorAction SilentlyContinue)) {
        Add-Action -Id 'firewall' -Status 'FAIL' -Detail '系统没有 New-NetFirewallRule 命令。'
    } elseif ($PSCmdlet.ShouldProcess($exePath, "允许本地端口 $effectivePort 和 $($effectivePort + 1)")) {
        try {
            $ruleName = 'wx_channel local proxy'
            Get-NetFirewallRule -DisplayName $ruleName -ErrorAction SilentlyContinue | Remove-NetFirewallRule -ErrorAction SilentlyContinue
            New-NetFirewallRule -DisplayName $ruleName -Direction Inbound -Action Allow -Profile Domain,Private -Program $exePath -Protocol TCP -LocalAddress 127.0.0.1 -LocalPort "$effectivePort,$($effectivePort + 1)" -ErrorAction Stop | Out-Null
            Add-Action -Id 'firewall' -Status 'DONE' -Detail "已创建本地回环防火墙规则：$ruleName"
        } catch {
            Add-Action -Id 'firewall' -Status 'FAIL' -Detail $_.Exception.Message
        }
    } else {
        Add-Action -Id 'firewall' -Status 'SKIP' -Detail 'WhatIf/用户取消。'
    }
} else {
    Add-Action -Id 'firewall' -Status 'SKIP' -Detail '未指定 -AddFirewallRule。'
}

if ($RestartApp) {
    $processes = @(Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.ProcessName -match '^wx_channel($|_)' })
    $exePath = Join-Path $resolvedRoot 'wx_channel.exe'
    if (-not (Test-Path -LiteralPath $exePath -PathType Leaf)) {
        Add-Action -Id 'restart' -Status 'FAIL' -Detail "未找到程序：$exePath"
    } elseif ($PSCmdlet.ShouldProcess($exePath, '重启下载助手')) {
        try {
            foreach ($process in $processes) { Stop-Process -Id $process.Id -Force -ErrorAction Stop }
            Start-Process -FilePath $exePath -WorkingDirectory $resolvedRoot -ErrorAction Stop | Out-Null
            Add-Action -Id 'restart' -Status 'DONE' -Detail '已重启 wx_channel.exe；请等待几秒后重新运行检测。'
        } catch {
            Add-Action -Id 'restart' -Status 'FAIL' -Detail $_.Exception.Message
        }
    } else {
        Add-Action -Id 'restart' -Status 'SKIP' -Detail 'WhatIf/用户取消。'
    }
} else {
    Add-Action -Id 'restart' -Status 'SKIP' -Detail '未指定 -RestartApp。'
}

$summary = [pscustomobject]@{
    generated_at = (Get-Date).ToUniversalTime().ToString('o')
    app_root     = $resolvedRoot
    port         = $effectivePort
    actions      = $script:Actions
    done         = @($script:Actions | Where-Object status -eq 'DONE').Count
    warn         = @($script:Actions | Where-Object status -eq 'WARN').Count
    fail         = @($script:Actions | Where-Object status -eq 'FAIL').Count
    skipped      = @($script:Actions | Where-Object status -eq 'SKIP').Count
}

if ($Json) {
    $summary | ConvertTo-Json -Depth 8
    exit $(if ($summary.fail -gt 0) { 2 } elseif ($summary.warn -gt 0) { 1 } else { 0 })
}

Write-Host '微信视频号下载助手环境修复' -ForegroundColor Cyan
Write-Host "目录：$resolvedRoot"
Write-Host "代理端口：$effectivePort"
Write-Host ''
foreach ($action in $script:Actions) {
    $color = switch ($action.status) {
        'DONE' { 'Green' }
        'WARN' { 'Yellow' }
        'FAIL' { 'Red' }
        default { 'DarkGray' }
    }
    Write-Host ("[{0,-4}] {1}：{2}" -f $action.status, $action.id, $action.detail) -ForegroundColor $color
}
Write-Host ''
$overallColor = 'Green'
if ($summary.fail -gt 0) { $overallColor = 'Red' }
elseif ($summary.warn -gt 0) { $overallColor = 'Yellow' }
Write-Host ("结果：完成 {0}，警告 {1}，失败 {2}，跳过 {3}" -f $summary.done, $summary.warn, $summary.fail, $summary.skipped) -ForegroundColor $overallColor
exit $(if ($summary.fail -gt 0) { 2 } elseif ($summary.warn -gt 0) { 1 } else { 0 })

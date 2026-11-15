@echo off
setlocal
set "WX_CHANNEL_ENV_TOOL_FILE=%~f0"
title wx_channel Environment Check and Repair
"%SystemRoot%\System32\WindowsPowerShell\v1.0\powershell.exe" -NoLogo -NoProfile -ExecutionPolicy Bypass -Command "$ErrorActionPreference='Stop'; try { $text=[IO.File]::ReadAllText($env:WX_CHANNEL_ENV_TOOL_FILE,[Text.Encoding]::UTF8); $marker=':WX_CHANNEL_ENV_PAYLOAD'; $offset=$text.LastIndexOf($marker); if($offset -lt 0){throw 'Tool payload is missing.'}; & ([scriptblock]::Create($text.Substring($offset+$marker.Length))) } catch { Write-Host ($_ | Out-String) -ForegroundColor Red; exit 2 }"
set "WX_CHANNEL_ENV_EXIT=%ERRORLEVEL%"
echo.
if not "%WX_CHANNEL_ENV_EXIT%"=="0" echo Tool error. Please review the message above. Exit code: %WX_CHANNEL_ENV_EXIT%
pause
exit /b %WX_CHANNEL_ENV_EXIT%

:WX_CHANNEL_ENV_PAYLOAD
<#
脚本名称：微信视频号助手环境检测与修复工具
使用方式：只需本 .cmd 文件。放到软件目录，双击后从菜单选择检查或修复。
兼容系统：Windows 10/11，系统自带的 Windows PowerShell 5.1。
#>
[CmdletBinding()]
param(
    [string]$AppRoot = '',
    [ValidateSet('menu', 'check', 'repair', 'help')]
    [string]$Action = 'menu',
    [ValidateRange(1, 30)]
    [int]$TimeoutSeconds = 5,
    [string]$AuthToken = '',
    [switch]$SkipNetwork,
    [switch]$Json,
    [switch]$WhatIf
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$script:ToolExitCode = 0
$script:ToolName = '微信视频号助手环境检测与修复工具'
$script:ToolFile = [Environment]::GetEnvironmentVariable('WX_CHANNEL_ENV_TOOL_FILE')
if ([string]::IsNullOrWhiteSpace($script:ToolFile)) { throw '请双击 wx-channel-environment.cmd 启动。' }
if ([string]::IsNullOrWhiteSpace($AppRoot)) {
    $AppRoot = Split-Path -Parent $script:ToolFile
    if ((Split-Path -Leaf $AppRoot) -eq 'scripts') { $AppRoot = Split-Path -Parent $AppRoot }
}
$AppRoot = (Resolve-Path -LiteralPath $AppRoot -ErrorAction Stop).Path
try {
    [Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false)
    $Host.UI.RawUI.WindowTitle = $script:ToolName
} catch { }
function Invoke-EnvironmentCheck {
<#
.SYNOPSIS
    检查微信视频号下载助手的 Windows 运行环境。

.DESCRIPTION
    这个脚本只做诊断，不会修改证书、配置、代理或防火墙。
    它检查应用 API、代理端口、WebSocket 端口、SunnyNet 证书、微信进程、
    下载目录写权限，以及会影响“下载按钮不显示/点击无反应”的常见网络条件。

.EXAMPLE
    双击 wx-channel-environment.cmd，选择菜单 1。

.EXAMPLE
    双击 wx-channel-environment.cmd，选择菜单 4 导出报告。
#>

[CmdletBinding()]
param(
    [string]$AppRoot = '',
    [int]$Port = 0,
    [int]$TimeoutSeconds = 5,
    [string]$AuthToken = '',
    [switch]$SkipNetwork,
    [switch]$Json
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$script:Checks = @()

if ([string]::IsNullOrWhiteSpace($AppRoot)) {
    $scriptPath = $MyInvocation.MyCommand.Path
    if ([string]::IsNullOrWhiteSpace($scriptPath)) { $scriptPath = $PSCommandPath }
    $scriptDirectory = Split-Path -Parent $scriptPath
    $AppRoot = Split-Path -Parent $scriptDirectory
}

function Add-Check {
    param(
        [string]$Id,
        [string]$Name,
        [ValidateSet('PASS', 'WARN', 'FAIL', 'SKIP', 'INFO')]
        [string]$Status,
        [string]$Detail,
        [string]$Fix = ''
    )

    $script:Checks += [pscustomobject]@{
        id     = $Id
        name   = $Name
        status = $Status
        detail = $Detail
        fix    = $Fix
    }
}

function Get-ConfigValue {
    param(
        [string]$Path,
        [string]$Key,
        [string]$Default = ''
    )

    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        return $Default
    }

    try {
        $line = Get-Content -LiteralPath $Path -Encoding UTF8 -ErrorAction Stop |
            Where-Object { $_ -match "^\s*$([regex]::Escape($Key))\s*:" } |
            Select-Object -First 1
        if ($null -eq $line) {
            return $Default
        }

        $value = ($line -replace "^\s*$([regex]::Escape($Key))\s*:\s*", '').Trim()
        $value = $value -replace '\s+#.*$', ''
        return $value.Trim('"', "'")
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
        $cmd = Get-Command Get-NetTCPConnection -ErrorAction SilentlyContinue
        if ($null -ne $cmd) {
            $connections = @(Get-NetTCPConnection -State Listen -LocalPort $TargetPort -ErrorAction SilentlyContinue)
            if ($connections.Count -gt 0) {
                $owners = @($connections | ForEach-Object {
                    $processName = ''
                    try { $processName = (Get-Process -Id $_.OwningProcess -ErrorAction Stop).ProcessName } catch { }
                    $suffix = ''
                    if ($processName) { $suffix = ", Process=$processName" }
                    "PID=$($_.OwningProcess)$suffix"
                })
                return ($owners -join '; ')
            }
        }
    } catch { }

    try {
        $lines = @(netstat.exe -ano -p tcp 2>$null | Where-Object {
            $_ -match "LISTENING\s+(\d+)\s*$" -and $_ -match ":$TargetPort\s"
        })
        if ($lines.Count -gt 0) {
            return ($lines -join '; ').Trim()
        }
    } catch { }

    return ''
}

function Invoke-LocalApi {
    param(
        [string]$Uri,
        [ValidateSet('GET', 'POST')]
        [string]$Method = 'GET'
    )

    try {
        $request = @{
            Uri         = $Uri
            Method      = $Method
            TimeoutSec  = $TimeoutSeconds
            ErrorAction = 'Stop'
        }
        if ($AuthToken) {
            $request.Headers = @{ 'X-Local-Auth' = $AuthToken }
        }
        if ($PSVersionTable.PSVersion.Major -le 5) {
            $request.UseBasicParsing = $true
        }
        $response = Invoke-WebRequest @request
        $parsed = $null
        try { $parsed = $response.Content | ConvertFrom-Json } catch { }
        $data = $parsed
        if ($null -ne $parsed -and $parsed.PSObject.Properties.Name -contains 'data') {
            $data = $parsed.data
        }
        return [pscustomobject]@{
            ok         = $true
            statusCode = [int]$response.StatusCode
            data       = $data
            body       = [string]$response.Content
            error      = ''
        }
    } catch {
        $statusCode = 0
        $responseProperty = $_.Exception.PSObject.Properties['Response']
        if ($null -ne $responseProperty -and $null -ne $responseProperty.Value) {
            try { $statusCode = [int]$responseProperty.Value.StatusCode } catch { }
        }
        return [pscustomobject]@{
            ok         = $false
            statusCode = $statusCode
            data       = $null
            body       = ''
            error      = $_.Exception.Message
        }
    }
}

function Get-PropertyValue {
    param(
        [object]$Object,
        [string]$Name,
        [object]$Default = $null
    )
    if ($null -eq $Object) { return $Default }
    $property = $Object.PSObject.Properties[$Name]
    if ($null -eq $property -or $null -eq $property.Value) { return $Default }
    return $property.Value
}

function Test-CertificateInstalled {
    $stores = @('Cert:\CurrentUser\Root', 'Cert:\LocalMachine\Root')
    foreach ($store in $stores) {
        try {
            $matches = @(Get-ChildItem -Path $store -ErrorAction Stop | Where-Object {
                $_.Subject -match '(?i)(^|\s|,)CN\s*=\s*SunnyNet($|,|\s)' -or
                $_.Subject -match '(?i)SunnyNet'
            })
            if ($matches.Count -gt 0) {
                return $true
            }
        } catch { }
    }
    return $false
}

function Test-DirectoryWritable {
    param([string]$Directory)
    try {
        if (-not (Test-Path -LiteralPath $Directory -PathType Container)) {
            return $false
        }
        $probe = Join-Path $Directory ('.wx-channel-env-check-' + [guid]::NewGuid().ToString('N'))
        [IO.File]::WriteAllText($probe, 'environment check')
        Remove-Item -LiteralPath $probe -Force -ErrorAction SilentlyContinue
        return $true
    } catch {
        return $false
    }
}

function Test-ProcessExists {
    param([string]$Pattern)
    try {
        return @(Get-Process -ErrorAction Stop | Where-Object { $_.ProcessName -match $Pattern }).Count -gt 0
    } catch {
        return $false
    }
}

try {
    $resolvedRoot = (Resolve-Path -LiteralPath $AppRoot -ErrorAction Stop).Path
} catch {
    throw "应用目录不存在：$AppRoot"
}

$configPath = Join-Path $resolvedRoot 'config.yaml'
$configuredPort = Get-ConfigValue -Path $configPath -Key 'port' -Default '2025'
$effectivePort = $Port
if ($effectivePort -le 0) {
    $parsedPort = 0
    if (-not [int]::TryParse($configuredPort, [ref]$parsedPort) -or $parsedPort -le 0) {
        $parsedPort = 2025
    }
    $effectivePort = $parsedPort
}

$downloadDirValue = Get-ConfigValue -Path $configPath -Key 'download_dir' -Default 'downloads'
$certFileValue = Get-ConfigValue -Path $configPath -Key 'cert_file' -Default 'SunnyRoot.cer'
$downloadDir = if ([IO.Path]::IsPathRooted($downloadDirValue)) {
    $downloadDirValue
} else {
    Join-Path $resolvedRoot $downloadDirValue
}

$osStatus = 'FAIL'
$osDetail = "$([Environment]::OSVersion.VersionString)"
$osFix = '请在 Windows 10 或更高版本运行。'
if ($env:OS -eq 'Windows_NT') {
    $osStatus = 'PASS'
    $bitness = '32 位'
    if ([Environment]::Is64BitOperatingSystem) { $bitness = '64 位' }
    $osDetail = "$osDetail，$bitness"
}
Add-Check -Id 'os' -Name 'Windows 系统' -Status $osStatus -Detail $osDetail -Fix $osFix

$isAdmin = Test-IsAdministrator
Add-Check -Id 'admin' -Name '管理员权限' -Status $(if ($isAdmin) { 'PASS' } else { 'WARN' }) -Detail $(if ($isAdmin) { '当前 PowerShell 以管理员身份运行。' } else { '当前不是管理员；当前用户证书可能仍可修复，进程注入可能失败。' }) -Fix '修复证书或注入失败时，请右键 PowerShell 选择“以管理员身份运行”。'

$configExists = Test-Path -LiteralPath $configPath -PathType Leaf
Add-Check -Id 'config' -Name '配置文件' -Status $(if ($configExists) { 'PASS' } else { 'WARN' }) -Detail "配置：$configPath；代理端口：$effectivePort；下载目录：$downloadDir" -Fix '缺少配置时可运行 主菜单 3 → 1。'

$appProcess = @(Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.ProcessName -match '^wx_channel($|_)' })
Add-Check -Id 'app-process' -Name '下载助手进程' -Status $(if ($appProcess.Count -gt 0) { 'PASS' } else { 'FAIL' }) -Detail $(if ($appProcess.Count -gt 0) { "已发现 $($appProcess.Count) 个进程。" } else { '未发现 wx_channel 进程。' }) -Fix '先启动 wx_channel.exe，再重新运行检测。'

$proxyOwner = Get-ListenPortOwner -TargetPort $effectivePort
Add-Check -Id 'proxy-port' -Name "代理端口 $effectivePort" -Status $(if ($proxyOwner) { 'PASS' } else { 'FAIL' }) -Detail $(if ($proxyOwner) { "端口正在监听：$proxyOwner" } else { '端口未监听，代理核心可能没有启动或端口被配置错误。' }) -Fix '检查日志；如端口冲突，可用 -Port 指定空闲端口并同步修改配置。'

$wsPort = $effectivePort + 1
$wsOwner = Get-ListenPortOwner -TargetPort $wsPort
Add-Check -Id 'websocket-port' -Name "页面通信端口 $wsPort" -Status $(if ($wsOwner) { 'PASS' } else { 'WARN' }) -Detail $(if ($wsOwner) { "端口正在监听：$wsOwner" } else { 'WebSocket 端口未监听；页面连接和按钮注入可能不可用。' }) -Fix '重启下载助手，并确认日志没有 WebSocket 服务启动失败。'

$certInstalled = Test-CertificateInstalled
$certPath = Join-Path $downloadDir $certFileValue
Add-Check -Id 'certificate-store' -Name 'SunnyNet 根证书' -Status $(if ($certInstalled) { 'PASS' } else { 'FAIL' }) -Detail $(if ($certInstalled) { '已在当前用户或本地计算机根证书存储中找到 SunnyNet。' } else { "未找到 SunnyNet 根证书；备用证书文件：$certPath" }) -Fix '运行 主菜单 2（基础修复） 安装证书，然后完全退出并重新打开微信视频号。'

$downloadDirExists = Test-Path -LiteralPath $downloadDir -PathType Container
Add-Check -Id 'download-dir' -Name '下载目录' -Status $(if ($downloadDirExists) { 'PASS' } else { 'WARN' }) -Detail $(if ($downloadDirExists) { $downloadDir } else { "目录不存在：$downloadDir" }) -Fix '运行 主菜单 2（基础修复） 创建目录。'

if ($downloadDirExists) {
    $writeOk = Test-DirectoryWritable -Directory $downloadDir
    Add-Check -Id 'download-write' -Name '下载目录写权限' -Status $(if ($writeOk) { 'PASS' } else { 'FAIL' }) -Detail $(if ($writeOk) { '临时文件写入和删除成功。' } else { '无法在下载目录创建临时文件。' }) -Fix '更换到有写权限的目录，或使用管理员权限运行修复脚本。'
} else {
    Add-Check -Id 'download-write' -Name '下载目录写权限' -Status 'SKIP' -Detail '目录不存在，未执行写入测试。' -Fix '先创建下载目录。'
}

$wechatProcess = Test-ProcessExists -Pattern '^WeChatAppEx$'
Add-Check -Id 'wechat-process' -Name '微信视频号进程' -Status $(if ($wechatProcess) { 'PASS' } else { 'WARN' }) -Detail $(if ($wechatProcess) { '已发现 WeChatAppEx.exe。' } else { '未发现 WeChatAppEx.exe；未打开视频号页面时这是正常的。' }) -Fix '打开微信并进入视频号页面，等待页面加载完成后再检查。'

$baseUrl = "http://127.0.0.1:$effectivePort"
$health = Invoke-LocalApi -Uri "$baseUrl/api/health"
Add-Check -Id 'api-health' -Name '本地 API 健康检查' -Status $(if ($health.ok) { 'PASS' } else { 'FAIL' }) -Detail $(if ($health.ok) { "HTTP $($health.statusCode)；$($health.body.Trim())" } else { $health.error }) -Fix '确认代理端口已监听；如果刚启动，请等待几秒后重试。'

if ($health.ok) {
    $certificateApi = Invoke-LocalApi -Uri "$baseUrl/api/v1/certificate/status"
    $certificateApiInstalled = [bool](Get-PropertyValue -Object $certificateApi.data -Name 'installed' -Default $false)
    $certificateApiStatus = 'WARN'
    if ($certificateApi.ok -and $certificateApiInstalled) { $certificateApiStatus = 'PASS' }
    elseif ($certificateApi.ok) { $certificateApiStatus = 'FAIL' }
    Add-Check -Id 'certificate-api' -Name '应用证书状态 API' -Status $certificateApiStatus -Detail $(if (-not $certificateApi.ok) { $certificateApi.error } elseif ($certificateApiInstalled) { '应用确认 SunnyNet 证书已安装。' } else { '应用确认 SunnyNet 证书未安装。' }) -Fix '运行 主菜单 2（基础修复） 安装证书。'

    $proxyApi = Invoke-LocalApi -Uri "$baseUrl/api/v1/proxy/status"
    $proxyRunning = [bool](Get-PropertyValue -Object $proxyApi.data -Name 'running' -Default $false)
    $proxyApiStatus = 'WARN'
    if ($proxyApi.ok -and $proxyRunning) { $proxyApiStatus = 'PASS' }
    Add-Check -Id 'proxy-api' -Name '应用代理状态 API' -Status $proxyApiStatus -Detail $(if (-not $proxyApi.ok) { $proxyApi.error } elseif ($proxyRunning) { '应用报告代理服务正在运行。' } else { '应用未报告代理服务正在运行。' }) -Fix '重启应用并检查代理核心启动错误。'

    $channelApi = Invoke-LocalApi -Uri "$baseUrl/api/channels/status"
    if ($channelApi.ok) {
        $runtime = Get-PropertyValue -Object $channelApi.data -Name 'runtime'
        $injection = Get-PropertyValue -Object $runtime -Name 'injection'
        $started = [bool](Get-PropertyValue -Object $injection -Name 'started' -Default $false)
        $injectionChecked = [bool](Get-PropertyValue -Object $injection -Name 'checked' -Default $false)
        $injectionError = [string](Get-PropertyValue -Object $injection -Name 'last_error' -Default '')
        $injectionStatus = 'WARN'
        if ($started) { $injectionStatus = 'PASS' }
        elseif ($injectionChecked) { $injectionStatus = 'FAIL' }
        $injectionDetail = '应用尚未完成注入引擎检查。'
        if ($started) { $injectionDetail = '应用报告微信页面注入引擎已启动。' }
        elseif ($injectionError) { $injectionDetail = $injectionError }
        Add-Check -Id 'injection' -Name '微信页面注入引擎' -Status $injectionStatus -Detail $injectionDetail -Fix '以管理员身份运行应用，并先启动微信视频号页面。'

        # ready_clients counts extension API readiness, not page connections.
        # fresh also includes API probe health; only application_fresh is a page heartbeat.
        $clientList = @(Get-PropertyValue -Object $channelApi.data -Name 'client_list' -Default @())
        $clientCount = [int](Get-PropertyValue -Object $channelApi.data -Name 'clients' -Default $clientList.Count)
        $connected = [bool](Get-PropertyValue -Object $channelApi.data -Name 'connected' -Default ($clientCount -gt 0))
        $heartbeatClients = @($clientList | Where-Object {
            $null -ne (Get-PropertyValue -Object $_ -Name 'application_fresh')
        })
        $activeClients = @($heartbeatClients | Where-Object {
            [bool](Get-PropertyValue -Object $_ -Name 'application_fresh' -Default $false)
        })
        $clientStatus = 'SKIP'
        $clientDetail = '未发现页面连接；未打开视频号时属于正常状态。'
        if ($activeClients.Count -gt 0) {
            $clientStatus = 'PASS'
            $clientDetail = "已连接 $clientCount 个页面，其中 $($activeClients.Count) 个页面心跳正常。"
        } elseif ($heartbeatClients.Count -gt 0 -and $heartbeatClients.Count -eq $clientList.Count) {
            $clientStatus = 'WARN'
            $clientDetail = "保留 $clientCount 个页面连接，但页面心跳均已过期。"
        } elseif ($connected -or $clientCount -gt 0) {
            $clientStatus = 'INFO'
            $clientDetail = "接口报告页面已连接（$clientCount 个），但未提供完整心跳数据，暂无法确认页面是否活跃。"
        } elseif ($wechatProcess) {
            $clientStatus = 'WARN'
            $clientDetail = '已发现微信视频号进程，但尚无页面连接。'
        }
        Add-Check -Id 'channel-client' -Name '视频号页面连接' -Status $clientStatus -Detail $clientDetail -Fix '打开或刷新微信视频号页面，等待页面加载和心跳恢复后再检查。'

        $readyClients = [int](Get-PropertyValue -Object $channelApi.data -Name 'ready_clients' -Default 0)
        $apiStatus = 'SKIP'
        $apiDetail = '没有活跃视频号页面，暂不检查扩展接口。'
        if ($clientStatus -in @('PASS', 'INFO')) {
            $apiStatus = 'INFO'
            $apiDetail = "搜索、评论等扩展接口尚未就绪（ready_clients=$readyClients）；手动下载请以页面操作结果为准。"
            $usableClients = @($activeClients | Where-Object {
                [bool](Get-PropertyValue -Object $_ -Name 'api_ready' -Default $false) -and
                [bool](Get-PropertyValue -Object $_ -Name 'fresh' -Default $false)
            })
            if ($usableClients.Count -gt 0) {
                $apiStatus = 'PASS'
                $apiDetail = "有 $($usableClients.Count) 个活跃页面的扩展接口已就绪（搜索、评论等）。"
            } elseif ($clientStatus -eq 'INFO') {
                $apiDetail = "页面心跳数据不完整，无法确认扩展接口可用性（ready_clients=$readyClients）。"
            } elseif ($readyClients -gt 0) {
                $apiDetail = '扩展接口探测未通过或状态已过期；搜索、评论等功能可能不可用，手动下载请以页面操作结果为准。'
            }
        }
        Add-Check -Id 'channel-api' -Name '搜索/评论等扩展接口' -Status $apiStatus -Detail $apiDetail
    } else {
        Add-Check -Id 'channel-status' -Name '视频号运行状态 API' -Status 'WARN' -Detail $channelApi.error -Fix '确认应用版本包含 /api/channels/status，并检查 WebSocket 端口。'
    }
}

$zoneFiles = @()
foreach ($fileName in @('wx_channel.exe', 'wx_channel_cloud.exe', 'wx_channel_radar.exe', 'wx_channel_optimized.exe')) {
    $filePath = Join-Path $resolvedRoot $fileName
    if (Test-Path -LiteralPath $filePath -PathType Leaf) {
        try {
            if (Get-Item -LiteralPath $filePath -Stream Zone.Identifier -ErrorAction SilentlyContinue) {
                $zoneFiles += $fileName
            }
        } catch { }
    }
}
Add-Check -Id 'downloaded-file' -Name '程序文件来源标记' -Status $(if ($zoneFiles.Count -eq 0) { 'PASS' } else { 'WARN' }) -Detail $(if ($zoneFiles.Count -eq 0) { '未发现需要解除阻止的程序文件。' } else { "发现 Zone.Identifier：$($zoneFiles -join ', ')" }) -Fix '运行 主菜单 3 → 1 解除 Windows 下载阻止标记。'

if ($SkipNetwork) {
    Add-Check -Id 'network' -Name '外网连通性' -Status 'SKIP' -Detail '使用 -SkipNetwork 跳过。' -Fix '需要时重新运行并移除 -SkipNetwork。'
} else {
    $dnsOk = $false
    try {
        if (Get-Command Resolve-DnsName -ErrorAction SilentlyContinue) {
            $dnsOk = @(Resolve-DnsName -Name 'channels.weixin.qq.com' -Type A -ErrorAction Stop).Count -gt 0
        } else {
            $dnsOk = ([Net.Dns]::GetHostAddresses('channels.weixin.qq.com')).Count -gt 0
        }
    } catch { $dnsOk = $false }
    Add-Check -Id 'dns' -Name '微信域名 DNS' -Status $(if ($dnsOk) { 'PASS' } else { 'FAIL' }) -Detail $(if ($dnsOk) { 'channels.weixin.qq.com 解析成功。' } else { 'DNS 解析失败。' }) -Fix '检查系统 DNS、代理软件或企业网络策略。'

    $httpsOk = $false
    $httpsError = ''
    try {
        $request = @{ Uri = 'https://channels.weixin.qq.com'; Method = 'Head'; TimeoutSec = $TimeoutSeconds; ErrorAction = 'Stop' }
        if ($PSVersionTable.PSVersion.Major -le 5) { $request.UseBasicParsing = $true }
        $null = Invoke-WebRequest @request
        $httpsOk = $true
    } catch {
        $responseProperty = $_.Exception.PSObject.Properties['Response']
        $statusCode = 0
        if ($null -ne $responseProperty -and $null -ne $responseProperty.Value) {
            try { $statusCode = [int]$responseProperty.Value.StatusCode } catch { }
        }
        if ($statusCode -gt 0 -and $statusCode -lt 500) {
            $httpsOk = $true
            $httpsError = "HTTPS 已连通，服务器返回 HTTP $statusCode（该站点可能不支持 HEAD）。"
        } else {
            $httpsError = $_.Exception.Message
        }
    }
    Add-Check -Id 'https' -Name '微信 HTTPS 连通性' -Status $(if ($httpsOk) { 'PASS' } else { 'WARN' }) -Detail $(if ($httpsOk) { 'HTTPS 请求成功。' } else { $httpsError }) -Fix '检查系统时间、根证书、代理软件和防火墙。'
}

$summary = [pscustomobject]@{
    generated_at = (Get-Date).ToUniversalTime().ToString('o')
    app_root     = $resolvedRoot
    port         = $effectivePort
    checks       = $script:Checks
    pass         = @($script:Checks | Where-Object status -eq 'PASS').Count
    warn         = @($script:Checks | Where-Object status -eq 'WARN').Count
    fail         = @($script:Checks | Where-Object status -eq 'FAIL').Count
    info         = @($script:Checks | Where-Object status -eq 'INFO').Count
    skipped      = @($script:Checks | Where-Object status -eq 'SKIP').Count
}

if ($Json) {
    $summary | ConvertTo-Json -Depth 10
    $script:ToolExitCode = $(if ($summary.fail -gt 0) { 2 } elseif ($summary.warn -gt 0) { 1 } else { 0 }); return
}

Write-Host '微信视频号下载助手环境检查' -ForegroundColor Cyan
Write-Host "目录：$resolvedRoot"
Write-Host "代理端口：$effectivePort，页面通信端口：$wsPort"
Write-Host ''
foreach ($check in $script:Checks) {
    $color = switch ($check.status) {
        'PASS' { 'Green' }
        'WARN' { 'Yellow' }
        'FAIL' { 'Red' }
        'INFO' { 'Cyan' }
        default { 'DarkGray' }
    }
    Write-Host ("[{0,-4}] {1}：{2}" -f $check.status, $check.name, $check.detail) -ForegroundColor $color
    if ($check.status -in @('WARN', 'FAIL') -and $check.fix) {
        Write-Host "       建议：$($check.fix)" -ForegroundColor DarkYellow
    }
}
Write-Host ''
$overallColor = 'Green'
if ($summary.fail -gt 0) { $overallColor = 'Red' }
elseif ($summary.warn -gt 0) { $overallColor = 'Yellow' }
Write-Host ("结果：通过 {0}，警告 {1}，失败 {2}，提示 {3}，跳过 {4}" -f $summary.pass, $summary.warn, $summary.fail, $summary.info, $summary.skipped) -ForegroundColor $overallColor
$script:ToolExitCode = $(if ($summary.fail -gt 0) { 2 } elseif ($summary.warn -gt 0) { 1 } else { 0 }); return

}
function Invoke-EnvironmentRepair {
<#
.SYNOPSIS
    修复微信视频号下载助手常见的 Windows 环境问题。

.DESCRIPTION
    默认执行安全、局部的修复：创建下载和日志目录，并尝试安装 SunnyNet
    根证书。高级菜单可以修复配置和解除文件阻止。
    修改已有配置前会自动生成带时间戳的备份。

.EXAMPLE
    双击 wx-channel-environment.cmd，选择菜单 2。

.EXAMPLE
    双击 wx-channel-environment.cmd，选择菜单 3 → 1。

.EXAMPLE
    双击 wx-channel-environment.cmd，选择菜单 3。
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
        $line = Get-Content -LiteralPath $Path -Encoding UTF8 -ErrorAction Stop |
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
    $content = Get-Content -LiteralPath $Path -Raw -Encoding UTF8 -ErrorAction Stop
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
                if ($importError) {
                    Add-Action -Id 'certificate-user' -Status 'FAIL' -Detail $importError
                } else {
                    Add-Action -Id 'certificate-user' -Status 'DONE' -Detail '已安装到当前用户的受信任根证书存储。'
                }
            } else {
                Add-Action -Id 'certificate-user' -Status 'SKIP' -Detail 'WhatIf/用户取消，未导入证书。'
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
    $script:ToolExitCode = $(if ($summary.fail -gt 0) { 2 } elseif ($summary.warn -gt 0) { 1 } else { 0 }); return
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
$script:ToolExitCode = $(if ($summary.fail -gt 0) { 2 } elseif ($summary.warn -gt 0) { 1 } else { 0 }); return

}
function Show-Header {
    Clear-Host
    Write-Host ('=' * 64) -ForegroundColor Cyan
    Write-Host ('  ' + $script:ToolName) -ForegroundColor White
    Write-Host ('  脚本文件：' + (Split-Path -Leaf $script:ToolFile))
    Write-Host '  用途：排查下载按钮不显示、无法下载等运行环境问题。'
    Write-Host ('  软件目录：' + $AppRoot) -ForegroundColor DarkGray
    Write-Host ('=' * 64) -ForegroundColor Cyan
    Write-Host ''
}
function Wait-ForMenu {
    Write-Host ''
    Read-Host '按 Enter 返回菜单（结果可在窗口中向上查看）' | Out-Null
}
function Invoke-ToolAction {
    param([ValidateSet('check', 'repair')][string]$Kind, [hashtable]$Options = @{})
    $script:ToolExitCode = 0
    $toolParameters = @{ AppRoot = $AppRoot; TimeoutSeconds = $TimeoutSeconds }
    if ($AuthToken) { $toolParameters.AuthToken = $AuthToken }
    if ($Kind -eq 'check' -and $SkipNetwork) { $toolParameters.SkipNetwork = $true }
    if ($Kind -eq 'repair' -and $WhatIf) { $toolParameters.WhatIf = $true }
    foreach ($key in $Options.Keys) { $toolParameters[$key] = $Options[$key] }
    try {
        Write-Host '正在执行，请稍候……' -ForegroundColor Cyan
        if ($Kind -eq 'check') { Invoke-EnvironmentCheck @toolParameters }
        else { Invoke-EnvironmentRepair @toolParameters }
    } catch {
        $script:ToolExitCode = 2
        Write-Host ('执行错误：' + $_.Exception.Message) -ForegroundColor Red
        Write-Host ($_ | Out-String) -ForegroundColor Red
    }
    Write-Host ''
    switch ($script:ToolExitCode) {
        0 { Write-Host '本次操作结束，未报告失败项。' -ForegroundColor Green }
        1 { Write-Host '存在警告，请查看上方建议。' -ForegroundColor Yellow }
        default { Write-Host '存在失败项，请查看上方错误和处理建议。' -ForegroundColor Red }
    }
}
function Show-Help {
    Show-Header
    Write-Host '【使用说明】' -ForegroundColor Yellow
    Write-Host '  1. 将本文件放在 wx_channel.exe 所在目录（也支持 scripts 子目录）。'
    Write-Host '  2. 先启动下载助手，打开微信视频号，再选择“检查运行环境”。'
    Write-Host '  3. 按检查结果选择修复；修复后重新打开视频号并再检查一次。'
    Write-Host ''
    Write-Host '【检查内容】' -ForegroundColor Yellow
    Write-Host '  进程、代理与页面通信端口、SunnyNet 根证书、下载目录写权限、'
    Write-Host '  微信页面注入、页面连接及心跳、扩展接口、本地 API、DNS 和 HTTPS。'
    Write-Host '  INFO 为状态提示，不计入环境警告或失败；扩展接口用于搜索、评论等功能。'
    Write-Host '  未发现问题仅代表这些检查项通过，仍需实际尝试下载。'
    Write-Host ''
    Write-Host '【修复范围】' -ForegroundColor Yellow
    Write-Host '  基础修复：创建下载/日志目录；按软件证书文件安装当前用户根证书。'
    Write-Host '  高级操作：修复配置、解除文件阻止；或以管理员权限启动软件。'
    Write-Host '  修改已有配置会生成 config.yaml.bak-* 备份，可手动复制恢复。'
    Write-Host '  证书修复需要软件正在运行，或下载目录中已有 SunnyRoot.cer。'
    Write-Host ''
    Write-Host '【报告与错误】' -ForegroundColor Yellow
    Write-Host '  选 4 保存 JSON 报告，可发给客服分析；其中可能包含本地目录。'
    Write-Host '  运行异常会保留原始错误；按 Enter 可回到菜单，关闭前可复制内容。'
}
function Show-AdvancedMenu {
    while ($true) {
        Show-Header
        Write-Host '【高级操作】' -ForegroundColor Yellow
        Write-Host '  1. 修复配置、解除程序文件阻止标记'
        Write-Host '     会备份后修改配置，并移除程序的 Windows 下载阻止标记。'
        Write-Host '  2. 以管理员身份启动下载助手（用于注入权限不足）'
        Write-Host '     请先手动关闭旧的下载助手，再选择此项。'
        Write-Host '  0. 返回主菜单'
        Write-Host ''
        $selection = Read-Host '请输入选项'
        switch ($selection) {
            '0' { return }
            '1' {
                Invoke-ToolAction -Kind repair -Options @{ RepairConfig = $true; UnblockFiles = $true }
                Wait-ForMenu
            }
            '2' {
                try {
                    $exePath = Join-Path $AppRoot 'wx_channel.exe'
                    if (-not (Test-Path -LiteralPath $exePath -PathType Leaf)) {
                        throw '当前目录没有 wx_channel.exe；请将此脚本放到软件目录。'
                    }
                    if ($WhatIf) {
                        Write-Host ('预览：以管理员权限启动 ' + $exePath)
                    } else {
                        Start-Process -FilePath $exePath -WorkingDirectory $AppRoot -Verb RunAs -WindowStyle Hidden -ErrorAction Stop | Out-Null
                        Write-Host '已提交启动请求，请在系统弹窗中授权，并观察软件是否启动。' -ForegroundColor Green
                    }
                } catch {
                    Write-Host ('启动失败：' + $_.Exception.Message) -ForegroundColor Red
                }
                Wait-ForMenu
            }
            default { Write-Host '输入无效，请输入 0、1 或 2。' -ForegroundColor Yellow; Wait-ForMenu }
        }
    }
}
function Export-EnvironmentReport {
    try {
        Write-Host '正在检查并生成报告，请稍候……' -ForegroundColor Cyan
        $reportParameters = @{ AppRoot = $AppRoot; TimeoutSeconds = $TimeoutSeconds; Json = $true }
        if ($AuthToken) { $reportParameters.AuthToken = $AuthToken }
        if ($SkipNetwork) { $reportParameters.SkipNetwork = $true }
        $reportJson = Invoke-EnvironmentCheck @reportParameters
        $null = $reportJson | ConvertFrom-Json -ErrorAction Stop
        $reportName = 'environment-report-' + (Get-Date -Format 'yyyyMMdd-HHmmss-fff') + '.json'
        $reportPath = Join-Path $AppRoot $reportName
        [IO.File]::WriteAllText($reportPath, ($reportJson -join [Environment]::NewLine), (New-Object Text.UTF8Encoding($true)))
        Write-Host ('报告已保存：' + $reportPath) -ForegroundColor Green
        Write-Host ('检查退出码：' + $script:ToolExitCode)
    } catch {
        $script:ToolExitCode = 2
        Write-Host ('导出失败：' + $_.Exception.Message) -ForegroundColor Red
    }
}
if ($Action -ne 'menu') {
    if ($Action -eq 'help') { Show-Help; return }
    $commandParameters = @{ AppRoot = $AppRoot; TimeoutSeconds = $TimeoutSeconds }
    if ($AuthToken) { $commandParameters.AuthToken = $AuthToken }
    if ($Json) { $commandParameters.Json = $true }
    if ($Action -eq 'check') {
        if ($SkipNetwork) { $commandParameters.SkipNetwork = $true }
        Invoke-EnvironmentCheck @commandParameters
    } else {
        if ($WhatIf) { $commandParameters.WhatIf = $true }
        Invoke-EnvironmentRepair @commandParameters
    }
    exit $script:ToolExitCode
}
while ($true) {
    Show-Header
    Write-Host '  1. 检查运行环境       检查下载所需的进程、端口、证书和网络'
    Write-Host '  2. 基础环境修复       创建目录、安装当前用户软件证书'
    Write-Host '  3. 高级操作           修复配置、解除阻止、管理员启动'
    Write-Host '  4. 导出检测报告       保存 JSON 报告，便于客服排查'
    Write-Host '  5. 名称与使用说明     查看用途、操作范围和使用步骤'
    Write-Host '  0. 退出'
    Write-Host ''
    Write-Host '  建议：先检查 → 按结果修复 → 重新打开视频号 → 再检查。' -ForegroundColor DarkGray
    $choice = Read-Host '请输入选项（0-5）'
    try {
        switch ($choice) {
            '1' { Invoke-ToolAction -Kind check; Wait-ForMenu }
            '2' { Invoke-ToolAction -Kind repair; Wait-ForMenu }
            '3' { Show-AdvancedMenu }
            '4' { Export-EnvironmentReport; Wait-ForMenu }
            '5' { Show-Help; Wait-ForMenu }
            '0' { Write-Host '已退出。'; return }
            default { Write-Host '输入无效，请输入 0-5。' -ForegroundColor Yellow; Wait-ForMenu }
        }
    } catch {
        Write-Host ('操作失败：' + $_.Exception.Message) -ForegroundColor Red
        Write-Host ($_ | Out-String) -ForegroundColor Red
        Wait-ForMenu
    }
}

<#
.SYNOPSIS
    检查微信视频号下载助手的 Windows 运行环境。

.DESCRIPTION
    这个脚本只做诊断，不会修改证书、配置、代理或防火墙。
    它检查应用 API、代理端口、WebSocket 端口、SunnyNet 证书、微信进程、
    下载目录写权限，以及会影响“下载按钮不显示/点击无反应”的常见网络条件。

.EXAMPLE
    .\scripts\check-environment.ps1

.EXAMPLE
    .\scripts\check-environment.ps1 -Json -SkipNetwork > environment.json
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
        $line = Get-Content -LiteralPath $Path -ErrorAction Stop |
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
Add-Check -Id 'config' -Name '配置文件' -Status $(if ($configExists) { 'PASS' } else { 'WARN' }) -Detail "配置：$configPath；代理端口：$effectivePort；下载目录：$downloadDir" -Fix '缺少配置时可运行 repair-environment.ps1 -RepairConfig。'

$appProcess = @(Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.ProcessName -match '^wx_channel($|_)' })
Add-Check -Id 'app-process' -Name '下载助手进程' -Status $(if ($appProcess.Count -gt 0) { 'PASS' } else { 'FAIL' }) -Detail $(if ($appProcess.Count -gt 0) { "已发现 $($appProcess.Count) 个进程。" } else { '未发现 wx_channel 进程。' }) -Fix '先启动 wx_channel.exe，再重新运行检测。'

$proxyOwner = Get-ListenPortOwner -TargetPort $effectivePort
Add-Check -Id 'proxy-port' -Name "代理端口 $effectivePort" -Status $(if ($proxyOwner) { 'PASS' } else { 'FAIL' }) -Detail $(if ($proxyOwner) { "端口正在监听：$proxyOwner" } else { '端口未监听，代理核心可能没有启动或端口被配置错误。' }) -Fix '检查日志；如端口冲突，可用 -Port 指定空闲端口并同步修改配置。'

$wsPort = $effectivePort + 1
$wsOwner = Get-ListenPortOwner -TargetPort $wsPort
Add-Check -Id 'websocket-port' -Name "页面通信端口 $wsPort" -Status $(if ($wsOwner) { 'PASS' } else { 'WARN' }) -Detail $(if ($wsOwner) { "端口正在监听：$wsOwner" } else { 'WebSocket 端口未监听；页面连接和按钮注入可能不可用。' }) -Fix '重启下载助手，并确认日志没有 WebSocket 服务启动失败。'

$certInstalled = Test-CertificateInstalled
$certPath = Join-Path $downloadDir $certFileValue
Add-Check -Id 'certificate-store' -Name 'SunnyNet 根证书' -Status $(if ($certInstalled) { 'PASS' } else { 'FAIL' }) -Detail $(if ($certInstalled) { '已在当前用户或本地计算机根证书存储中找到 SunnyNet。' } else { "未找到 SunnyNet 根证书；备用证书文件：$certPath" }) -Fix '运行 repair-environment.ps1 安装证书，然后完全退出并重新打开微信视频号。'

$downloadDirExists = Test-Path -LiteralPath $downloadDir -PathType Container
Add-Check -Id 'download-dir' -Name '下载目录' -Status $(if ($downloadDirExists) { 'PASS' } else { 'WARN' }) -Detail $(if ($downloadDirExists) { $downloadDir } else { "目录不存在：$downloadDir" }) -Fix '运行 repair-environment.ps1 创建目录。'

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
    Add-Check -Id 'certificate-api' -Name '应用证书状态 API' -Status $certificateApiStatus -Detail $(if (-not $certificateApi.ok) { $certificateApi.error } elseif ($certificateApiInstalled) { '应用确认 SunnyNet 证书已安装。' } else { '应用确认 SunnyNet 证书未安装。' }) -Fix '运行 repair-environment.ps1 安装证书。'

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
Add-Check -Id 'downloaded-file' -Name '程序文件来源标记' -Status $(if ($zoneFiles.Count -eq 0) { 'PASS' } else { 'WARN' }) -Detail $(if ($zoneFiles.Count -eq 0) { '未发现需要解除阻止的程序文件。' } else { "发现 Zone.Identifier：$($zoneFiles -join ', ')" }) -Fix '运行 repair-environment.ps1 -UnblockFiles 解除 Windows 下载阻止标记。'

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
    exit $(if ($summary.fail -gt 0) { 2 } elseif ($summary.warn -gt 0) { 1 } else { 0 })
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
exit $(if ($summary.fail -gt 0) { 2 } elseif ($summary.warn -gt 0) { 1 } else { 0 })

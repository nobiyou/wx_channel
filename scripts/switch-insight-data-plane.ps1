[CmdletBinding()]
param(
    [string]$RepoRoot = "",
    [string]$CandidateExePath = "",
    [string]$TargetExePath = "",
    [int]$Port = 2025,
    [switch]$Apply,
    [switch]$EmitOnly
)

$ErrorActionPreference = "Stop"

function Resolve-DefaultRepoRoot {
    return (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot "..")).Path
}

function Resolve-CandidateExe {
    param([string]$ResolvedRepoRoot)

    if ($CandidateExePath) {
        return (Resolve-Path -LiteralPath $CandidateExePath).Path
    }

    $cacheDir = Join-Path $ResolvedRepoRoot ".codex-cache"
    $candidate = Get-ChildItem -LiteralPath $cacheDir -Filter "wx_channel_team_ops_*.exe" -File -ErrorAction SilentlyContinue |
        Sort-Object LastWriteTime -Descending |
        Select-Object -First 1
    if ($null -eq $candidate) {
        throw "No wx_channel_team_ops_*.exe candidate found under $cacheDir. Build a candidate first or pass -CandidateExePath."
    }
    return $candidate.FullName
}

function Get-ListenerProcessIds {
    param([int[]]$Ports)

    $ids = @()
    foreach ($port in $Ports) {
        $connections = Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue
        foreach ($connection in @($connections)) {
            if ($connection.OwningProcess -gt 0 -and $ids -notcontains $connection.OwningProcess) {
                $ids += [int]$connection.OwningProcess
            }
        }
    }
    return $ids
}

function Quote-PowerShellArgument {
    param([string]$Value)

    return "'" + ($Value -replace "'", "''") + "'"
}

function Build-AdminRerunCommand {
    param(
        [string]$ScriptPath,
        [string]$ResolvedRepoRoot,
        [string]$ResolvedCandidateExe,
        [string]$ResolvedTargetExe,
        [int]$ProxyPort
    )

    return @(
        "powershell.exe",
        "-NoProfile",
        "-ExecutionPolicy Bypass",
        "-File $(Quote-PowerShellArgument $ScriptPath)",
        "-RepoRoot $(Quote-PowerShellArgument $ResolvedRepoRoot)",
        "-CandidateExePath $(Quote-PowerShellArgument $ResolvedCandidateExe)",
        "-TargetExePath $(Quote-PowerShellArgument $ResolvedTargetExe)",
        "-Port $ProxyPort",
        "-Apply"
    ) -join " "
}

function Stop-ListenerProcessIds {
    param([int[]]$ProcessIds)

    $errors = @()
    foreach ($processId in @($ProcessIds | Select-Object -Unique)) {
        $process = Get-Process -Id $processId -ErrorAction SilentlyContinue
        if ($null -eq $process) {
            continue
        }

        try {
            Stop-Process -Id $processId -Force -ErrorAction Stop
        }
        catch {
            $errors += "Stop-Process PID $processId failed: $($_.Exception.Message)"
            $taskkill = Get-Command taskkill.exe -ErrorAction SilentlyContinue
            if ($null -ne $taskkill) {
                & $taskkill.Source /PID $processId /F | Out-Null
                if ($LASTEXITCODE -ne 0) {
                    $errors += "taskkill PID $processId failed with exit code $LASTEXITCODE"
                }
            }
            else {
                $errors += "taskkill.exe is not available"
            }
        }
    }
    return $errors
}

function Wait-ForListenerRelease {
    param(
        [int[]]$Ports,
        [int]$TimeoutSeconds = 10,
        [string[]]$StopErrors = @(),
        [string]$AdminRerunCommand = ""
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        $remainingPids = @(Get-ListenerProcessIds -Ports $Ports)
        if ($remainingPids.Count -eq 0) {
            return
        }
        Start-Sleep -Milliseconds 300
    }

    $finalPids = @(Get-ListenerProcessIds -Ports $Ports)
    $message = "ports $($Ports -join ',') are still occupied by process(es): $($finalPids -join ',')"
    if ($StopErrors.Count -gt 0) {
        $message = "$message. Stop errors: $($StopErrors -join '; ')"
    }
    if ([string]::IsNullOrWhiteSpace($AdminRerunCommand) -eq $false) {
        $message = "$message. If the listener was started as administrator, close it manually or run from an elevated PowerShell: $AdminRerunCommand"
    }
    throw $message
}

function Wait-ForStatusURL {
    param(
        [string]$URL,
        [int]$TimeoutSeconds = 20
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    $lastError = ""
    while ((Get-Date) -lt $deadline) {
        try {
            $response = Invoke-WebRequest -UseBasicParsing -Uri $URL -TimeoutSec 3
            if ([int]$response.StatusCode -eq 200) {
                return
            }
            $lastError = "HTTP $($response.StatusCode)"
        }
        catch {
            $lastError = $_.Exception.Message
        }
        Start-Sleep -Milliseconds 500
    }
    throw "status URL $URL did not become ready: $lastError"
}

function Get-RequiredCapabilityURLs {
    param([string]$ResolvedBaseURL)

    $baseURL = $ResolvedBaseURL.TrimEnd("/")
    return [ordered]@{
        feedSearch    = ($baseURL + "/api/channels/feed/search?keyword=__insight_capability_probe__")
        feedList      = ($baseURL + "/api/channels/contact/feed/list?username=__insight_capability_probe__")
        commentExport = ($baseURL + "/api/channels/feed/comment/export")
    }
}

function Get-InsightCapabilityRouteProbe {
    param(
        [string]$Name,
        [string]$Method,
        [string]$URL,
        [string]$Body = "",
        [int]$TimeoutSeconds = 5
    )

    $statusCode = 0
    $errorText = ""
    try {
        $request = @{
            UseBasicParsing = $true
            Uri             = $URL
            Method          = $Method
            TimeoutSec      = $TimeoutSeconds
        }
        if (-not [string]::IsNullOrWhiteSpace($Body)) {
            $request["Body"] = $Body
            $request["ContentType"] = "application/json"
        }
        $response = Invoke-WebRequest @request
        $statusCode = [int]$response.StatusCode
    }
    catch {
        $errorText = $_.Exception.Message
        if ($_.Exception.Response -and $_.Exception.Response.StatusCode) {
            $statusCode = [int]$_.Exception.Response.StatusCode
        }
    }

    $routePresent = $true
    if ($statusCode -eq 0 -or $statusCode -eq 404 -or $statusCode -eq 405) {
        $routePresent = $false
    }
    return [pscustomobject]@{
        name = $Name
        method = $Method
        url = $URL
        statusCode = $statusCode
        routePresent = [bool]$routePresent
        error = $errorText
    }
}

function Get-InsightCapabilityStatus {
    param([string]$ResolvedBaseURL)

    $urls = Get-RequiredCapabilityURLs -ResolvedBaseURL $ResolvedBaseURL
    $probes = @()
    $probes += Get-InsightCapabilityRouteProbe -Name "feedSearch" -Method "GET" -URL $urls.feedSearch
    $probes += Get-InsightCapabilityRouteProbe -Name "feedList" -Method "GET" -URL $urls.feedList
    $probes += Get-InsightCapabilityRouteProbe `
        -Name "commentExport" `
        -Method "POST" `
        -URL $urls.commentExport `
        -Body '{"object_id":"__insight_capability_probe__","nonce_id":"__insight_capability_probe__","title":"Insight capability probe"}'

    $missing = @()
    foreach ($probe in $probes) {
        if (-not $probe.routePresent) {
            $missing += $probe.name
        }
    }
    return [pscustomobject]@{
        ok = ($missing.Count -eq 0)
        missing = @($missing)
        probes = @($probes)
    }
}

function Wait-ForInsightCapabilityRoutes {
    param(
        [string]$ResolvedBaseURL,
        [int]$TimeoutSeconds = 20
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    $lastStatus = $null
    while ((Get-Date) -lt $deadline) {
        $lastStatus = Get-InsightCapabilityStatus -ResolvedBaseURL $ResolvedBaseURL
        if ($lastStatus.ok) {
            return
        }
        Start-Sleep -Milliseconds 500
    }

    $details = @()
    if ($null -ne $lastStatus) {
        foreach ($probe in @($lastStatus.probes)) {
            if (-not $probe.routePresent) {
                $details += "$($probe.name):HTTP $($probe.statusCode) $($probe.error)"
            }
        }
    }
    throw "required Insight API routes did not become ready: $($details -join '; ')"
}

if (-not $RepoRoot) { $RepoRoot = Resolve-DefaultRepoRoot }
else { $RepoRoot = (Resolve-Path -LiteralPath $RepoRoot).Path }

$candidateExe = Resolve-CandidateExe -ResolvedRepoRoot $RepoRoot
if (-not $TargetExePath) { $TargetExePath = Join-Path $RepoRoot "wx_channel_cloud.exe" }
$targetExe = (Resolve-Path -LiteralPath $TargetExePath).Path

$apiPort = $Port + 1
$apiBaseURL = "http://127.0.0.1:$apiPort"
$statusURL = "$apiBaseURL/api/channels/status"
$requiredCapabilityURLs = Get-RequiredCapabilityURLs -ResolvedBaseURL $apiBaseURL
$backupDir = Join-Path $RepoRoot "release\backup"
$stamp = Get-Date -Format "yyyyMMdd-HHmmss"
$backupPath = Join-Path $backupDir ("{0}.{1}.bak" -f (Split-Path -Leaf $targetExe), $stamp)
$varDir = Join-Path $RepoRoot "var"
$stdoutLog = Join-Path $varDir "insight-data-plane.stdout.log"
$stderrLog = Join-Path $varDir "insight-data-plane.stderr.log"
$listenerPids = @(Get-ListenerProcessIds -Ports @($Port, $apiPort))
$adminRerunCommand = Build-AdminRerunCommand -ScriptPath $PSCommandPath -ResolvedRepoRoot $RepoRoot -ResolvedCandidateExe $candidateExe -ResolvedTargetExe $targetExe -ProxyPort $Port

$envPlan = [ordered]@{
    WX_CHANNEL_PORT = "$Port"
    WX_CHANNEL_RADAR_ENABLED = "false"
    WX_CHANNEL_CLOUD_ENABLED = "false"
}

$plan = [pscustomobject]@{
    mode = "wx-channel-insight-data-plane-switch"
    applyRequired = -not $Apply.IsPresent
    repoRoot = $RepoRoot
    candidateExePath = $candidateExe
    targetExePath = $targetExe
    backupPath = $backupPath
    proxyPort = $Port
    apiPort = $apiPort
    statusURL = $statusURL
    requiredCapabilityURLs = $requiredCapabilityURLs
    listenerProcessIds = $listenerPids
    env = $envPlan
    stdoutLog = $stdoutLog
    stderrLog = $stderrLog
    adminRerunInstruction = "If listenerProcessIds cannot be stopped because access is denied, run adminRerunCommand from an elevated PowerShell."
    adminRerunCommand = $adminRerunCommand
    actions = @(
        "stop listeners on proxy/api ports",
        "backup current target exe",
        "copy candidate exe to target",
        "start target exe with radar/cloud disabled",
        "wait for /api/channels/status",
        "wait for required Insight API routes"
    )
}

if ($EmitOnly.IsPresent -or -not $Apply.IsPresent) {
    $plan | ConvertTo-Json -Depth 6
    return
}

New-Item -ItemType Directory -Force -Path $backupDir | Out-Null
New-Item -ItemType Directory -Force -Path $varDir | Out-Null

$startedProcess = $null
try {
    $stopErrors = @(Stop-ListenerProcessIds -ProcessIds $listenerPids)
    Wait-ForListenerRelease -Ports @($Port, $apiPort) -StopErrors $stopErrors -AdminRerunCommand $adminRerunCommand

    Copy-Item -LiteralPath $targetExe -Destination $backupPath -Force
    Copy-Item -LiteralPath $candidateExe -Destination $targetExe -Force

    foreach ($entry in $envPlan.GetEnumerator()) {
        [Environment]::SetEnvironmentVariable([string]$entry.Key, [string]$entry.Value, "Process")
    }

    $startedProcess = Start-Process -FilePath $targetExe -WorkingDirectory $RepoRoot -PassThru -WindowStyle Hidden `
        -RedirectStandardOutput $stdoutLog -RedirectStandardError $stderrLog
    Wait-ForStatusURL -URL $statusURL
    Wait-ForInsightCapabilityRoutes -ResolvedBaseURL $apiBaseURL

    [pscustomobject]@{
        status = "ready"
        pid = $startedProcess.Id
        repoRoot = $RepoRoot
        candidateExePath = $candidateExe
        targetExePath = $targetExe
        backupPath = $backupPath
        proxyPort = $Port
        apiPort = $apiPort
        statusURL = $statusURL
        requiredCapabilityURLs = $requiredCapabilityURLs
        env = $envPlan
        stdoutLog = $stdoutLog
        stderrLog = $stderrLog
    } | ConvertTo-Json -Depth 6
}
catch {
    $originalError = $_
    if ($null -ne $startedProcess) {
        Stop-Process -Id $startedProcess.Id -Force -ErrorAction SilentlyContinue
        try {
            Wait-ForListenerRelease -Ports @($Port, $apiPort) -TimeoutSeconds 5
        }
        catch {
        }
    }
    if (Test-Path -LiteralPath $backupPath) {
        try {
            Copy-Item -LiteralPath $backupPath -Destination $targetExe -Force -ErrorAction Stop
        }
        catch {
            Write-Warning "failed to restore $targetExe from ${backupPath}: $($_.Exception.Message)"
        }
    }
    throw $originalError
}

param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^[a-zA-Z0-9-]{1,80}$')]
    [string]$JobId
)

$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$buildRoot = (Resolve-Path (Join-Path $repoRoot '.poc-build')).Path
$binaryPath = (Resolve-Path (Join-Path $buildRoot 'wx_channel_poc.exe')).Path
$runtimeRoot = (Resolve-Path (Join-Path $repoRoot '.poc-runtime')).Path
$secretsRoot = (Resolve-Path (Join-Path $repoRoot '.poc-secrets')).Path

function Assert-ApprovedJobPath {
    param([string]$Root, [string]$Candidate)
    $rootPrefix = $Root.TrimEnd([IO.Path]::DirectorySeparatorChar) + [IO.Path]::DirectorySeparatorChar
    $fullCandidate = [IO.Path]::GetFullPath($Candidate)
    if (-not $fullCandidate.StartsWith($rootPrefix, [StringComparison]::OrdinalIgnoreCase)) { throw 'cleanup path escapes approved root' }
    if (Test-Path -LiteralPath $fullCandidate) {
        $resolved = (Resolve-Path -LiteralPath $fullCandidate).Path
        if (-not $resolved.StartsWith($rootPrefix, [StringComparison]::OrdinalIgnoreCase)) { throw 'resolved cleanup path escapes approved root' }
        if ((Get-Item -LiteralPath $resolved -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'cleanup path is a reparse point' }
    }
}

if (-not $binaryPath.StartsWith($buildRoot + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) { throw 'cleanup binary escapes build root' }
$runtimeJob = Join-Path $runtimeRoot $JobId
$secretsJob = Join-Path $secretsRoot $JobId
Assert-ApprovedJobPath -Root $runtimeRoot -Candidate $runtimeJob
Assert-ApprovedJobPath -Root $secretsRoot -Candidate $secretsJob

& $binaryPath cleanup --job-id $JobId
if ($LASTEXITCODE -ne 0) { throw 'POC cleanup command failed' }

Assert-ApprovedJobPath -Root $runtimeRoot -Candidate $runtimeJob
Assert-ApprovedJobPath -Root $secretsRoot -Candidate $secretsJob
if (Test-Path -LiteralPath $runtimeJob) { throw 'runtime job directory remains after cleanup' }
if (Test-Path -LiteralPath $secretsJob) { throw 'secrets job directory remains after cleanup' }
Write-Output 'POC cleanup verified'

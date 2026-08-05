[CmdletBinding()]
param(
    [string]$RepoRoot = "",
    [string]$CandidateExePath = "",
    [string]$TargetExePath = "",
    [int]$Port = 2025,
    [switch]$EmitOnly
)

$ErrorActionPreference = "Stop"

function Resolve-DefaultRepoRoot {
    return (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot "..")).Path
}

function Quote-PowerShellArgument {
    param([string]$Value)

    return "'" + ($Value -replace "'", "''") + "'"
}

function Format-PowerShellArguments {
    param([string[]]$ArgumentList)

    $parts = @()
    foreach ($argument in $ArgumentList) {
        if ($argument -match "^[A-Za-z0-9_:/=+.,-]+$") {
            $parts += $argument
        }
        else {
            $parts += Quote-PowerShellArgument $argument
        }
    }
    return $parts -join " "
}

function Format-PowerShellCommand {
    param([string[]]$ArgumentList)

    return "powershell.exe " + (Format-PowerShellArguments -ArgumentList $ArgumentList)
}

if (-not $RepoRoot) { $RepoRoot = Resolve-DefaultRepoRoot }
else { $RepoRoot = (Resolve-Path -LiteralPath $RepoRoot).Path }

$switchScript = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot "switch-insight-data-plane.ps1")).Path
$argumentList = @(
    "-NoProfile",
    "-ExecutionPolicy",
    "Bypass",
    "-NoExit",
    "-File",
    $switchScript,
    "-RepoRoot",
    $RepoRoot,
    "-Port",
    "$Port",
    "-Apply"
)

if ($CandidateExePath) {
    $argumentList += @("-CandidateExePath", (Resolve-Path -LiteralPath $CandidateExePath).Path)
}
if ($TargetExePath) {
    $argumentList += @("-TargetExePath", (Resolve-Path -LiteralPath $TargetExePath).Path)
}

$preview = [pscustomobject]@{
    mode = "wx-channel-insight-data-plane-admin-switch"
    elevated = $true
    repoRoot = $RepoRoot
    switchScript = $switchScript
    proxyPort = $Port
    apiPort = $Port + 1
    argumentList = $argumentList
    argumentLine = Format-PowerShellArguments -ArgumentList $argumentList
    command = Format-PowerShellCommand -ArgumentList $argumentList
    action = "opens an elevated PowerShell with Start-Process -Verb RunAs, then runs switch-insight-data-plane.ps1 -Apply"
}

if ($EmitOnly.IsPresent) {
    $preview | ConvertTo-Json -Depth 6
    return
}

Start-Process -FilePath "powershell.exe" -ArgumentList $preview.argumentLine -Verb RunAs | Out-Null
$preview | ConvertTo-Json -Depth 6

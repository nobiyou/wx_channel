[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][ValidatePattern('^[A-Za-z0-9-]{1,80}$')][string]$RunId,
    [Parameter(Mandatory = $true)][string]$ShareUrl,
    [string]$RepoRoot = "",
    [string]$ApiBase = ""
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

[pscustomobject]@{ run_id = $RunId; status = "failed"; reason_code = "not_implemented" } | ConvertTo-Json -Compress
exit 1

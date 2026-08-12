[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][ValidatePattern('^[A-Za-z0-9-]{1,80}$')][string]$RunId,
    [Parameter(Mandatory = $true)][string]$ShareUrl,
    [string]$RepoRoot = "",
    [string]$ApiBase = ""
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
throw "ltaoo probe script is not implemented"

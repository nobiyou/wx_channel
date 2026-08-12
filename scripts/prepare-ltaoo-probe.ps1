[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$LtaooExePath,
    [string]$RepoRoot = "",
    [int]$ApiPort = 2022,
    [int]$ProxyPort = 2023
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
throw "ltaoo probe script is not implemented"

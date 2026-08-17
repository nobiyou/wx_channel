Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

Import-Module (Join-Path $PSScriptRoot 'LtaooRuntime.psm1')

function Assert-LtaooRouterBackend {
    param([Parameter(Mandatory = $true)][object]$Backend)
    if (
        $null -eq $Backend -or
        $Backend.PSObject.TypeNames.Count -eq 0 -or
        [string]$Backend.PSObject.TypeNames[0] -cne 'TrendRadar.MihomoBackend' -or
        [string]$Backend.Kind -cne 'mihomo'
    ) {
        throw 'router_backend_invalid'
    }
}

function New-LtaooRouterBackend {
    param(
        [Parameter(Mandatory = $true)][string]$RouterKind,
        [Parameter(Mandatory = $true)][string]$ExecutablePath,
        [Parameter(Mandatory = $true)][string]$ConfigPath
    )
    if ($RouterKind -cne 'mihomo') { throw 'router_kind_unsupported' }
    $backend = [pscustomobject]@{
        Kind = 'mihomo'
        ExecutablePath = [IO.Path]::GetFullPath($ExecutablePath)
        ConfigPath = [IO.Path]::GetFullPath($ConfigPath)
        Capabilities = @(
            'external_edit_protection'
            'live_reload'
            'loopback_upstream'
            'process_routing'
            'snapshot_restore'
            'syntax_validation'
        )
    }
    $backend.PSObject.TypeNames.Insert(0, 'TrendRadar.MihomoBackend')
    return $backend
}

function Add-LtaooRouterBlock {
    param(
        [Parameter(Mandatory = $true)][object]$Backend,
        [Parameter(Mandatory = $true)][string]$Text,
        [Parameter(Mandatory = $true)][string]$RunId,
        [Parameter(Mandatory = $true)][int]$ProxyPort
    )
    Assert-LtaooRouterBackend -Backend $Backend
    return Add-LtaooClashBlock -Text $Text -RunId $RunId -ProxyPort $ProxyPort
}

function Remove-LtaooRouterBlock {
    param(
        [Parameter(Mandatory = $true)][object]$Backend,
        [Parameter(Mandatory = $true)][string]$Text,
        [Parameter(Mandatory = $true)][string]$RunId
    )
    Assert-LtaooRouterBackend -Backend $Backend
    return Remove-LtaooClashBlock -Text $Text -RunId $RunId
}

function Get-LtaooRouterRecoveryAction {
    param(
        [Parameter(Mandatory = $true)][object]$Backend,
        [Parameter(Mandatory = $true)][string]$BaselineHash,
        [Parameter(Mandatory = $true)][string]$TemporaryHash,
        [Parameter(Mandatory = $true)][string]$CurrentHash,
        [Parameter(Mandatory = $true)][bool]$MarkerPresent
    )
    Assert-LtaooRouterBackend -Backend $Backend
    return Get-ClashRecoveryAction -BaselineHash $BaselineHash -TemporaryHash $TemporaryHash -CurrentHash $CurrentHash -MarkerPresent $MarkerPresent
}

function Get-LtaooRouterController {
    param(
        [Parameter(Mandatory = $true)][object]$Backend,
        [Parameter(Mandatory = $true)][string]$Text
    )
    Assert-LtaooRouterBackend -Backend $Backend
    return Get-LtaooClashController -Text $Text
}

function Test-LtaooRouterConfig {
    param(
        [Parameter(Mandatory = $true)][object]$Backend,
        [Parameter(Mandatory = $true)][string]$ConfigPath,
        [Parameter(Mandatory = $true)][string]$DataDirectory
    )
    Assert-LtaooRouterBackend -Backend $Backend
    Test-LtaooClashConfig -ClashExePath ([string]$Backend.ExecutablePath) -ConfigPath $ConfigPath -DataDirectory $DataDirectory
}

function Invoke-LtaooRouterReload {
    param(
        [Parameter(Mandatory = $true)][object]$Backend,
        [Parameter(Mandatory = $true)][object]$Controller,
        [Parameter(Mandatory = $true)][string]$ConfigPath
    )
    Assert-LtaooRouterBackend -Backend $Backend
    Invoke-LtaooClashReload -Controller $Controller -ConfigPath $ConfigPath
}

Export-ModuleMember -Function @(
    'New-LtaooRouterBackend',
    'Add-LtaooRouterBlock',
    'Remove-LtaooRouterBlock',
    'Get-LtaooRouterRecoveryAction',
    'Get-LtaooRouterController',
    'Test-LtaooRouterConfig',
    'Invoke-LtaooRouterReload'
)

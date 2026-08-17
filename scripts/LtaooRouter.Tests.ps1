$modulePath = Join-Path $PSScriptRoot 'LtaooRouter.psm1'
Import-Module $modulePath -Force

Describe 'TrendRadar generic router backend' {
    BeforeEach {
        $routerPath = Join-Path $TestDrive 'mihomo.exe'
        $configPath = Join-Path $TestDrive 'config.yaml'
        [IO.File]::WriteAllText($routerPath, 'mihomo fixture')
        [IO.File]::WriteAllText($configPath, "proxies: []`nrules: []`n")
    }

    It 'constructs the Mihomo backend with the complete capability set' {
        $backend = New-LtaooRouterBackend -RouterKind 'mihomo' -ExecutablePath $routerPath -ConfigPath $configPath

        $backend.PSTypeNames[0] | Should BeExactly 'TrendRadar.MihomoBackend'
        $backend.Kind | Should BeExactly 'mihomo'
        (@($backend.Capabilities) -join '|') | Should BeExactly 'external_edit_protection|live_reload|loopback_upstream|process_routing|snapshot_restore|syntax_validation'
    }

    It 'rejects an unknown router kind before any mutation' {
        $thrown = $false
        try { [void](New-LtaooRouterBackend -RouterKind 'sing-box' -ExecutablePath $routerPath -ConfigPath $configPath) }
        catch {
            $thrown = $true
            $_.Exception.Message | Should BeExactly 'router_kind_unsupported'
        }
        $thrown | Should Be $true
    }

    It 'routes the marked block transform through the Mihomo backend' {
        $backend = New-LtaooRouterBackend -RouterKind 'mihomo' -ExecutablePath $routerPath -ConfigPath $configPath
        $baseline = "proxies: []`nrules: []`n"

        $updated = Add-LtaooRouterBlock -Backend $backend -Text $baseline -RunId 'router-fixture-1' -ProxyPort 2023
        $restored = Remove-LtaooRouterBlock -Backend $backend -Text $updated -RunId 'router-fixture-1'

        $updated | Should Match 'TREND_RADAR_WX_BEGIN router-fixture-1'
        $restored | Should Not Match 'TREND_RADAR_WX_'
        $restored | Should Not Match 'trendradar-wx-router-fixture-1'
        $restored | Should Match '(?m)^proxies:'
        $restored | Should Match '(?m)^rules:'
    }

    It 'rejects a forged backend object' {
        $forged = [pscustomobject]@{ Kind = 'mihomo'; ExecutablePath = $routerPath; ConfigPath = $configPath }

        $thrown = $false
        try { [void](Add-LtaooRouterBlock -Backend $forged -Text "proxies: []`nrules: []`n" -RunId 'router-fixture-2' -ProxyPort 2023) }
        catch {
            $thrown = $true
            $_.Exception.Message | Should BeExactly 'router_backend_invalid'
        }
        $thrown | Should Be $true
    }
}

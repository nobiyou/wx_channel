$modulePath = Join-Path $PSScriptRoot 'LtaooRuntime.psm1'
Import-Module $modulePath -Force

Describe 'TrendRadar ltaoo Clash runtime transform' {
    It 'accepts a local Clash Verge named-pipe controller when HTTP is disabled' {
        $config = "external-controller: ''`nexternal-controller-pipe: '\\.\pipe\verge-mihomo'`nsecret: fixture-secret`n"

        $controller = Get-LtaooClashController -Text $config

        $controller.Kind | Should BeExactly 'pipe'
        $controller.PipeName | Should BeExactly 'verge-mihomo'
        $controller.Uri | Should BeExactly ''
        $controller.Secret | Should BeExactly 'fixture-secret'
    }

    It 'rejects remote or nested named-pipe controller paths' {
        { Get-LtaooClashController -Text "external-controller-pipe: '\\server\pipe\verge-mihomo'`n" } | Should Throw
        { Get-LtaooClashController -Text "external-controller-pipe: '\\.\pipe\nested\verge-mihomo'`n" } | Should Throw
    }

    It 'adds one loopback proxy and ordered process rules while preserving CRLF' {
        $baseline = "mixed-port: 7890`r`nproxies:`r`n  - name: existing`r`n    type: http`r`n    server: 127.0.0.1`r`n    port: 8080`r`nrules:`r`n  - MATCH,DIRECT`r`n"
        $updated = Add-LtaooClashBlock -Text $baseline -RunId 'fixture-run-1' -ProxyPort 2023

        $updated | Should Match '# TREND_RADAR_WX_BEGIN fixture-run-1 PROXY'
        $updated | Should Match 'server: 127.0.0.1'
        $updated | Should Match 'port: 2023'
        $updated | Should Match 'PROCESS-NAME,wx_video_download.exe,DIRECT'
        $updated | Should Match 'PROCESS-NAME,WeChatAppEx.exe,trendradar-wx-fixture-run-1'
        $updated | Should Match 'PROCESS-NAME,Weixin.exe,trendradar-wx-fixture-run-1'
        $updated | Should Match 'PROCESS-NAME,WeChat.exe,trendradar-wx-fixture-run-1'
        $updated.IndexOf('wx_video_download.exe') | Should BeLessThan $updated.IndexOf('Weixin.exe')
        ($updated -replace "`r`n", '').Contains("`n") | Should Be $false
        { Add-LtaooClashBlock -Text $updated -RunId 'fixture-run-1' -ProxyPort 2023 } | Should Throw
    }

    It 'removes only its marked blocks and preserves external edits' {
        $baseline = "proxies:`n  - name: existing`n    type: http`n    server: 127.0.0.1`n    port: 8080`nrules:`n  - MATCH,DIRECT`n"
        $updated = Add-LtaooClashBlock -Text $baseline -RunId 'fixture-run-2' -ProxyPort 2023
        $externallyChanged = $updated.Replace('mixed-port:', '# external edit`nmixed-port:') + "# external tail`n"
        $recovered = Remove-LtaooClashBlock -Text $externallyChanged -RunId 'fixture-run-2'

        $recovered | Should Not Match 'TREND_RADAR_WX_'
        $recovered | Should Match '# external tail'
        $recovered | Should Match 'name: existing'
    }

    It 'classifies exact restore marker removal and attention paths' {
        (Get-ClashRecoveryAction -BaselineHash 'base' -TemporaryHash 'temp' -CurrentHash 'temp' -MarkerPresent $true) | Should BeExactly 'restore_backup'
        (Get-ClashRecoveryAction -BaselineHash 'base' -TemporaryHash 'temp' -CurrentHash 'external' -MarkerPresent $true) | Should BeExactly 'remove_marker'
        (Get-ClashRecoveryAction -BaselineHash 'base' -TemporaryHash 'temp' -CurrentHash 'base' -MarkerPresent $false) | Should BeExactly 'already_restored'
        (Get-ClashRecoveryAction -BaselineHash 'base' -TemporaryHash 'temp' -CurrentHash 'external' -MarkerPresent $false) | Should BeExactly 'attention_required'
    }

    It 'supports inline empty lists and preserves a UTF-8 BOM round trip' {
        $baseline = "proxies: []`r`nrules: []`r`n"
        $bytes = ConvertTo-LtaooUtf8Bytes -Text $baseline -WithBom $true
        $decoded = ConvertFrom-LtaooUtf8Bytes -Bytes $bytes
        $updated = Add-LtaooClashBlock -Text $decoded.Text -RunId 'fixture-run-3' -ProxyPort 2023
        $roundTrip = ConvertTo-LtaooUtf8Bytes -Text $updated -WithBom $decoded.HasBom

        $roundTrip[0] | Should Be 239
        $roundTrip[1] | Should Be 187
        $roundTrip[2] | Should Be 191
        $updated | Should Match 'PROCESS-NAME,WeChat.exe,trendradar-wx-fixture-run-3'
    }

    It 'preserves indentless Clash Verge sequence style' {
        $baseline = "proxies:`n- name: existing`n  type: http`n  server: 127.0.0.1`n  port: 8080`nrules:`n- MATCH,DIRECT`n"

        $updated = Add-LtaooClashBlock -Text $baseline -RunId 'fixture-run-4' -ProxyPort 2023

        $updated | Should Match "(?m)^- name: trendradar-wx-fixture-run-4$"
        $updated | Should Match "(?m)^  type: http$"
        $updated | Should Match "(?m)^- PROCESS-NAME,wx_video_download.exe,DIRECT$"
        $updated | Should Match "(?m)^- PROCESS-NAME,WeChatAppEx.exe,trendradar-wx-fixture-run-4$"
        $updated | Should Match "(?m)^- MATCH,DIRECT$"
    }
}

Describe 'TrendRadar ltaoo one-shot grant' {
    It 'exports the ACL assertion used by the entry script' {
        (Get-Command Assert-LtaooOwnerOnlyAcl -ErrorAction Stop).CommandType | Should BeExactly 'Function'
    }

    BeforeEach {
        $runtimeRoot = Join-Path $TestDrive 'runtime'
        [void](New-Item -ItemType Directory -Path $runtimeRoot -Force)
        $requestPath = Join-Path $runtimeRoot 'request.json'
        $ltaooPath = Join-Path $runtimeRoot 'wx_video_download.exe'
        $clashPath = Join-Path $runtimeRoot 'clash.exe'
        $clashConfigPath = Join-Path $runtimeRoot 'config.yaml'
        $batchPath = Join-Path $runtimeRoot 'wx_channel_ltaoo_batch.exe'
        [IO.File]::WriteAllText($requestPath, '{"schema_version":1}')
        [IO.File]::WriteAllText($ltaooPath, 'ltaoo fixture')
        [IO.File]::WriteAllText($clashPath, 'clash fixture')
        [IO.File]::WriteAllText($clashConfigPath, 'proxies: []')
        [IO.File]::WriteAllText($batchPath, 'batch fixture')
        $runtimePaths = @($ltaooPath, $clashPath, $clashConfigPath, $batchPath, $runtimeRoot)
        $grantPath = Join-Path $runtimeRoot 'grant.json'
        $grant = [ordered]@{
            schema_version = 1
            authorization_mode = 'wechat-channels-local-runtime-v1'
            run_id = 'grant-fixture-1'
            windows_sid = [Security.Principal.WindowsIdentity]::GetCurrent().User.Value
            request_sha256 = Get-LtaooFileHash -LiteralPath $requestPath
            runtime_paths_sha256 = Get-LtaooRuntimePathsHash -LiteralPaths $runtimePaths
            ltaoo_executable_sha256 = Get-LtaooFileHash -LiteralPath $ltaooPath
            batch_executable_sha256 = Get-LtaooFileHash -LiteralPath $batchPath
            expires_at = [DateTimeOffset]::UtcNow.AddMinutes(5).ToString('yyyy-MM-ddTHH:mm:ssZ')
            actions = @('install_current_user_ca', 'modify_clash', 'start_ltaoo')
        }
    }

    It 'consumes a valid owner-only grant bound to the request and executables' {
        Write-LtaooJsonAtomic -Value $grant -LiteralPath $grantPath
        Set-LtaooOwnerOnlyAcl -LiteralPath $grantPath -Directory $false
        Assert-LtaooNoReparseInPath -LiteralPath $requestPath

        $result = Use-LtaooRunGrant -GrantPath $grantPath -RunId 'grant-fixture-1' -RequestPath $requestPath -RuntimePaths $runtimePaths -LtaooExePath $ltaooPath -BatchExePath $batchPath

        $result.run_id | Should BeExactly 'grant-fixture-1'
        (Test-Path -LiteralPath $grantPath) | Should Be $false
    }

    It 'rejects a request hash mismatch without consuming the grant' {
        $grant.request_sha256 = ('0' * 64)
        Write-LtaooJsonAtomic -Value $grant -LiteralPath $grantPath
        Set-LtaooOwnerOnlyAcl -LiteralPath $grantPath -Directory $false

        { Use-LtaooRunGrant -GrantPath $grantPath -RunId 'grant-fixture-1' -RequestPath $requestPath -RuntimePaths $runtimePaths -LtaooExePath $ltaooPath -BatchExePath $batchPath } | Should Throw
        (Test-Path -LiteralPath $grantPath) | Should Be $true
    }

    It 'rejects an untrusted explicit ACL principal' {
        $path = Join-Path $TestDrive 'untrusted-acl'
        [void](New-Item -ItemType Directory -Path $path)
        $current = [Security.Principal.WindowsIdentity]::GetCurrent().User
        $acl = [Security.AccessControl.DirectorySecurity]::new()
        $acl.SetOwner($current)
        $acl.SetAccessRuleProtection($true, $false)
        $currentRule = [Security.AccessControl.FileSystemAccessRule]::new(
            $current,
            [Security.AccessControl.FileSystemRights]::FullControl,
            [Security.AccessControl.InheritanceFlags]::ContainerInherit -bor [Security.AccessControl.InheritanceFlags]::ObjectInherit,
            [Security.AccessControl.PropagationFlags]::None,
            [Security.AccessControl.AccessControlType]::Allow
        )
        [void]$acl.AddAccessRule($currentRule)
        $everyone = [Security.Principal.SecurityIdentifier]::new('S-1-1-0')
        $rule = [Security.AccessControl.FileSystemAccessRule]::new(
            $everyone,
            [Security.AccessControl.FileSystemRights]::Read,
            [Security.AccessControl.AccessControlType]::Allow
        )
        [void]$acl.AddAccessRule($rule)
        Set-Acl -LiteralPath $path -AclObject $acl

        { Assert-LtaooOwnerOnlyAcl -LiteralPath $path } | Should Throw
    }
}

Describe 'TrendRadar ltaoo generic router grant' {
    BeforeEach {
        $runtimeRoot = Join-Path $TestDrive 'runtime-v2'
        [void](New-Item -ItemType Directory -Path $runtimeRoot -Force)
        $requestPath = Join-Path $runtimeRoot 'request.json'
        $ltaooPath = Join-Path $runtimeRoot 'wx_video_download.exe'
        $routerPath = Join-Path $runtimeRoot 'mihomo.exe'
        $routerConfigPath = Join-Path $runtimeRoot 'config.yaml'
        $otherRouterConfigPath = Join-Path $runtimeRoot 'other.yaml'
        $batchPath = Join-Path $runtimeRoot 'wx_channel_ltaoo_batch.exe'
        [IO.File]::WriteAllText($requestPath, '{"schema_version":1}')
        [IO.File]::WriteAllText($ltaooPath, 'ltaoo fixture')
        [IO.File]::WriteAllText($routerPath, 'mihomo fixture')
        [IO.File]::WriteAllText($routerConfigPath, 'proxies: []')
        [IO.File]::WriteAllText($otherRouterConfigPath, 'proxies: []')
        [IO.File]::WriteAllText($batchPath, 'batch fixture')
        $runtimePaths = @($ltaooPath, $routerPath, $routerConfigPath, $batchPath, $runtimeRoot)
        $grantPath = Join-Path $runtimeRoot 'grant.json'
        $capabilityFingerprint = ('a' * 64)
        $grant = [ordered]@{
            schema_version = 2
            authorization_mode = 'wechat-channels-local-runtime-v2'
            run_id = 'grant-fixture-v2'
            windows_sid = [Security.Principal.WindowsIdentity]::GetCurrent().User.Value
            request_sha256 = Get-LtaooFileHash -LiteralPath $requestPath
            runtime_paths_sha256 = Get-LtaooRuntimePathsHash -LiteralPaths $runtimePaths
            ltaoo_executable_sha256 = Get-LtaooFileHash -LiteralPath $ltaooPath
            batch_executable_sha256 = Get-LtaooFileHash -LiteralPath $batchPath
            router_kind = 'mihomo'
            router_executable_sha256 = Get-LtaooFileHash -LiteralPath $routerPath
            router_config_path_sha256 = Get-LtaooStringHash -Value ([IO.Path]::GetFullPath($routerConfigPath).ToLowerInvariant())
            router_capability_fingerprint = $capabilityFingerprint
            expires_at = [DateTimeOffset]::UtcNow.AddMinutes(5).ToString('o')
            actions = @('install_current_user_ca', 'modify_proxy_router', 'start_ltaoo')
        }
    }

    It 'consumes a valid v2 grant bound to the router and capabilities' {
        Write-LtaooJsonAtomic -Value $grant -LiteralPath $grantPath
        Set-LtaooOwnerOnlyAcl -LiteralPath $grantPath -Directory $false

        $result = Use-LtaooRunGrant -GrantPath $grantPath -RunId 'grant-fixture-v2' -RequestPath $requestPath -RuntimePaths $runtimePaths -LtaooExePath $ltaooPath -BatchExePath $batchPath -ExpectedAuthorizationMode 'wechat-channels-local-runtime-v2' -RouterKind 'mihomo' -RouterExePath $routerPath -RouterConfigPath $routerConfigPath -RouterCapabilityFingerprint $capabilityFingerprint

        $result.router_kind | Should BeExactly 'mihomo'
        (Test-Path -LiteralPath $grantPath) | Should Be $false
    }

    It 'rejects changed router identity without consuming the v2 grant' {
        $cases = @(
            [pscustomobject]@{ Name = 'kind'; GrantField = ''; GrantValue = ''; RouterKind = 'sing-box'; ConfigPath = $routerConfigPath; Fingerprint = $capabilityFingerprint; ChangeExecutable = $false },
            [pscustomobject]@{ Name = 'executable'; GrantField = ''; GrantValue = ''; RouterKind = 'mihomo'; ConfigPath = $routerConfigPath; Fingerprint = $capabilityFingerprint; ChangeExecutable = $true },
            [pscustomobject]@{ Name = 'config path'; GrantField = 'router_config_path_sha256'; GrantValue = ('0' * 64); RouterKind = 'mihomo'; ConfigPath = $routerConfigPath; Fingerprint = $capabilityFingerprint; ChangeExecutable = $false },
            [pscustomobject]@{ Name = 'capabilities'; GrantField = ''; GrantValue = ''; RouterKind = 'mihomo'; ConfigPath = $routerConfigPath; Fingerprint = ('b' * 64); ChangeExecutable = $false }
        )
        foreach ($case in $cases) {
            if ($case.GrantField -ne '') { $grant[$case.GrantField] = $case.GrantValue }
            Write-LtaooJsonAtomic -Value $grant -LiteralPath $grantPath
            Set-LtaooOwnerOnlyAcl -LiteralPath $grantPath -Directory $false
            if ($case.ChangeExecutable) { [IO.File]::AppendAllText($routerPath, ' changed') }
            $thrown = $false
            try {
                [void](Use-LtaooRunGrant -GrantPath $grantPath -RunId 'grant-fixture-v2' -RequestPath $requestPath -RuntimePaths $runtimePaths -LtaooExePath $ltaooPath -BatchExePath $batchPath -ExpectedAuthorizationMode 'wechat-channels-local-runtime-v2' -RouterKind $case.RouterKind -RouterExePath $routerPath -RouterConfigPath $case.ConfigPath -RouterCapabilityFingerprint $case.Fingerprint)
            } catch { $thrown = $true }
            $thrown | Should Be $true
            (Test-Path -LiteralPath $grantPath) | Should Be $true
            Remove-Item -LiteralPath $grantPath -Force
            if ($case.ChangeExecutable) { [IO.File]::WriteAllText($routerPath, 'mihomo fixture') }
            $grant.router_config_path_sha256 = Get-LtaooStringHash -Value ([IO.Path]::GetFullPath($routerConfigPath).ToLowerInvariant())
        }
    }

    It 'does not mix v1 and v2 argument groups' {
        Write-LtaooJsonAtomic -Value $grant -LiteralPath $grantPath
        Set-LtaooOwnerOnlyAcl -LiteralPath $grantPath -Directory $false
        { Use-LtaooRunGrant -GrantPath $grantPath -RunId 'grant-fixture-v2' -RequestPath $requestPath -RuntimePaths $runtimePaths -LtaooExePath $ltaooPath -BatchExePath $batchPath } | Should Throw
        (Test-Path -LiteralPath $grantPath) | Should Be $true

        Remove-Item -LiteralPath $grantPath -Force
        $grant.schema_version = 1
        $grant.authorization_mode = 'wechat-channels-local-runtime-v1'
        $grant.Remove('router_kind')
        $grant.Remove('router_executable_sha256')
        $grant.Remove('router_config_path_sha256')
        $grant.Remove('router_capability_fingerprint')
        $grant.actions = @('install_current_user_ca', 'modify_clash', 'start_ltaoo')
        Write-LtaooJsonAtomic -Value $grant -LiteralPath $grantPath
        Set-LtaooOwnerOnlyAcl -LiteralPath $grantPath -Directory $false
        { Use-LtaooRunGrant -GrantPath $grantPath -RunId 'grant-fixture-v2' -RequestPath $requestPath -RuntimePaths $runtimePaths -LtaooExePath $ltaooPath -BatchExePath $batchPath -ExpectedAuthorizationMode 'wechat-channels-local-runtime-v2' -RouterKind 'mihomo' -RouterExePath $routerPath -RouterConfigPath $routerConfigPath -RouterCapabilityFingerprint $capabilityFingerprint } | Should Throw
        (Test-Path -LiteralPath $grantPath) | Should Be $true
    }
}

Describe 'TrendRadar ltaoo recovery identity' {
    It 'waits for a tracked process to exit before reporting stopped' {
        $powershellPath = (Get-Process -Id $PID).Path
        $process = Start-Process -FilePath $powershellPath -ArgumentList '-NoProfile -Command "Start-Sleep -Seconds 2"' -PassThru -WindowStyle Hidden
        try {
            (Wait-LtaooProcessStopped -ProcessId $process.Id -TimeoutMilliseconds 3000 -PollMilliseconds 50) | Should Be $true
        } finally {
            Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue
        }
    }

    It 'does not report a still-running process as stopped after the bounded wait' {
        $powershellPath = (Get-Process -Id $PID).Path
        $process = Start-Process -FilePath $powershellPath -ArgumentList '-NoProfile -Command "Start-Sleep -Seconds 30"' -PassThru -WindowStyle Hidden
        try {
            (Wait-LtaooProcessStopped -ProcessId $process.Id -TimeoutMilliseconds 100 -PollMilliseconds 25) | Should Be $false
        } finally {
            Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue
        }
    }

    It 'matches all recorded process identity fields before permitting cleanup' {
        $powershellPath = (Get-Process -Id $PID).Path
        $process = Start-Process -FilePath $powershellPath -ArgumentList '-NoProfile -Command "Start-Sleep -Seconds 30"' -PassThru -WindowStyle Hidden
        try {
            $start = $process.StartTime.ToUniversalTime().ToString('o')
            $hash = Get-LtaooFileHash -LiteralPath $powershellPath

            (Test-LtaooProcessIdentity -ProcessId $process.Id -ExpectedPath $powershellPath -ExpectedStartTime $start -ExpectedSha256 $hash) | Should Be $true
            (Test-LtaooProcessIdentity -ProcessId $process.Id -ExpectedPath $powershellPath -ExpectedStartTime $start -ExpectedSha256 ('0' * 64)) | Should Be $false
        } finally {
            Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue
        }
    }

    It 'treats a process that vanished during identity validation as stopped' {
        (Test-LtaooProcessIdentityOrAbsent -ProcessId 2147483647 -ExpectedPath 'C:\missing\ltaoo.exe' -ExpectedStartTime 'missing' -ExpectedSha256 ('0' * 64)) | Should Be $true
    }

    It 'generates a fresh private CA without installing it into a certificate store' {
        $secretsRoot = Join-Path $TestDrive 'certificate-secrets'
        $certificate = New-LtaooRunCertificate -RunId 'certificate-fixture-1' -SecretsRoot $secretsRoot

        $certificate.Thumbprint | Should Match '^[A-F0-9]{40}$'
        (Get-Content -Raw -LiteralPath $certificate.CertificatePath) | Should Match 'BEGIN CERTIFICATE'
        (Get-Content -Raw -LiteralPath $certificate.PrivateKeyPath) | Should Match ('BEGIN ' + 'PRIVATE KEY')
        (Test-Path -LiteralPath ('Cert:\CurrentUser\Root\' + $certificate.Thumbprint)) | Should Be $false
    }
}

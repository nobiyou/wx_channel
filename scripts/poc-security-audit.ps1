$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$buildRoot = Join-Path $repoRoot '.poc-build'
$binaryPath = Join-Path $buildRoot 'wx_channel_poc.exe'
$portableToolBin = Join-Path $repoRoot '.poc-tools\tdm-gcc-10.3.0-2\bin'
$gccPath = Join-Path $portableToolBin 'gcc.exe'
$gxxPath = Join-Path $portableToolBin 'g++.exe'
$localGoPath = Join-Path $repoRoot '.poc-tools\go1.24.3\go\bin\go.exe'

function Resolve-ApprovedGoPath {
    $candidates = @()
    if ($env:GOROOT) {
        $candidates += Join-Path $env:GOROOT 'bin\go.exe'
    }
    $candidates += $localGoPath
    foreach ($candidate in $candidates) {
        if (-not (Test-Path -LiteralPath $candidate -PathType Leaf)) { continue }
        $version = (& $candidate version | Out-String).Trim()
        if ($LASTEXITCODE -eq 0 -and $version -match '^go version go1\.24\.3 windows/amd64$') {
            return (Resolve-Path -LiteralPath $candidate).Path
        }
    }
    throw 'approved Go 1.24.3 windows/amd64 toolchain is required'
}

if (-not (Test-Path -LiteralPath $binaryPath -PathType Leaf)) { throw 'POC binary is missing; run build-poc.ps1 first' }
if (-not (Test-Path -LiteralPath $gccPath -PathType Leaf) -or -not (Test-Path -LiteralPath $gxxPath -PathType Leaf)) { throw 'approved portable compiler is missing' }
$goPath = Resolve-ApprovedGoPath
$goRoot = Split-Path -Parent (Split-Path -Parent $goPath)

$env:GOROOT = $goRoot
$env:GOTOOLCHAIN = 'local'
$env:CGO_ENABLED = '1'
$env:PATH = "$(Join-Path $goRoot 'bin');$portableToolBin;$env:PATH"
$env:CC = $gccPath
$env:CXX = $gxxPath
if ((& $gccPath -dumpfullversion).Trim() -ne '10.3.0') { throw 'unexpected compiler version' }

Push-Location $repoRoot
try {
    & $goPath test ./internal/pocaudit -count=1
    if ($LASTEXITCODE -ne 0) { throw 'POC source audit failed' }
    & $goPath test github.com/qtgolang/SunnyNet/SunnyNet github.com/qtgolang/SunnyNet/src/nfapi github.com/qtgolang/SunnyNet/src/CrossCompiled -count=1
    if ($LASTEXITCODE -ne 0) { throw 'SunnyNet scoped tests failed' }

    $expectedHashes = [ordered]@{
        'pkg/sunnynet/Resource/nfapi/dll/win32/nfapi.dll' = 'B6AD927CE7A5281F1B71BE347B6EE4B920A8EF90F104C6A5CC56082FBA0C3528'
        'pkg/sunnynet/Resource/nfapi/dll/x64/nfapi.dll' = '1D6F3487D3AA707B978E1A81F8E98250D334120B856B89780408EB98DBBD0910'
        'pkg/sunnynet/Resource/nfapi/sys/tdi/amd64/netfilter2.sys' = '8B24C85B5325E2CEFF531651A74274409518FB2FF11EF258D2675377B0C9B5A2'
        'pkg/sunnynet/Resource/nfapi/sys/tdi/i386/netfilter2.sys' = '3ED01B98D86E0E85F71A8EC2856E7FA170DB6B34D0955A508F116B5EC1172B16'
        'pkg/sunnynet/Resource/nfapi/sys/wfp/amd64/netfilter2.sys' = 'D5E68A1C65280CB8497E7CB95BD0013D79CB728C30FE7821315915946B88251C'
        'pkg/sunnynet/Resource/nfapi/sys/wfp/i386/netfilter2.sys' = '01A7D8088C631988C03430795E80EDF07123047294EE4A6FC260EB29B7515346'
    }
    foreach ($relativePath in $expectedHashes.Keys) {
        $path = Join-Path $repoRoot ($relativePath -replace '/', '\')
        $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $path).Hash
        if ($actual -ne $expectedHashes[$relativePath]) { throw "pinned binary hash mismatch: $relativePath" }
        $signature = Get-AuthenticodeSignature -LiteralPath $path
        if ($relativePath.EndsWith('.sys') -and $signature.Status -ne 'Valid') { throw "driver signature is not valid: $relativePath" }
        if ($relativePath.EndsWith('.dll') -and $signature.Status -ne 'NotSigned') { throw "DLL signature status differs from provenance: $relativePath" }
    }

    $dependencies = @(& $goPath list -deps ./cmd/wx_channel_poc)
    if ($LASTEXITCODE -ne 0) { throw 'POC dependency listing failed' }
    $forbiddenDependencies = @(
        'wx_channel/internal/cloud', 'wx_channel/internal/metrics', 'wx_channel/internal/database',
        'wx_channel/internal/services', 'wx_channel/internal/app', 'github.com/GopeedLab/gopeed',
        'github.com/mattn/go-sqlite3', 'github.com/go-llsqlite/crawshaw'
    )
    foreach ($dependency in $forbiddenDependencies) {
        if ($dependencies -contains $dependency) { throw "forbidden POC dependency: $dependency" }
    }

    $javascriptPath = Join-Path $repoRoot 'internal\pocassets\poc_api_client.js'
    $javascript = Get-Content -Raw -LiteralPath $javascriptPath
    if ($javascript -match '(?i)\b(like|publish|upload|addcomment|replycomment|postcomment)\b') { throw 'POC page client contains a write-shaped method' }
    if ($javascript -match '(?i)console\.|fetch\s*\(') { throw 'POC page client contains logging or direct fetch' }

    $binaryText = [Text.Encoding]::ASCII.GetString([IO.File]::ReadAllBytes($binaryPath))
    $rsaKeyMarker = '-----BEGIN ' + 'RSA PRIVATE KEY-----'
    $keyMarker = '-----BEGIN ' + 'PRIVATE KEY-----'
    foreach ($forbiddenText in @($rsaKeyMarker, $keyMarker, 'api.cloudflare.com', 'workers.dev', 'github.com/nobiyou/wx_channel/releases', 'insight-data-plane')) {
        if ($binaryText.IndexOf($forbiddenText, [StringComparison]::OrdinalIgnoreCase) -ge 0) { throw "forbidden binary text: $forbiddenText" }
    }

    $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $binaryPath).Hash
    $moduleInfo = ((& $goPath version -m $binaryPath) | Out-String).Trim()
    if ($LASTEXITCODE -ne 0) { throw 'read POC build metadata failed' }
    if ($moduleInfo -notmatch '^\S+: go1\.24\.3') { throw 'POC binary was not built with approved Go 1.24.3' }
    $provenance = [ordered]@{
        schema_version = 'wx-channel-comment-poc/build-1'
        binary_sha256 = $hash
        compiler = 'TDM-GCC 10.3.0'
        go_version_m = $moduleInfo
    }
    $provenance | ConvertTo-Json -Depth 4 | Set-Content -Encoding UTF8 -LiteralPath (Join-Path $buildRoot 'provenance.json')
    Write-Output 'POC security audit passed'
}
finally {
    Pop-Location
}

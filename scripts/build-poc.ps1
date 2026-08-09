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

if (-not (Test-Path -LiteralPath $gccPath -PathType Leaf) -or -not (Test-Path -LiteralPath $gxxPath -PathType Leaf)) {
    throw 'approved portable TDM-GCC 10.3.0 is required'
}
$goPath = Resolve-ApprovedGoPath
$goRoot = Split-Path -Parent (Split-Path -Parent $goPath)

New-Item -ItemType Directory -Force -Path $buildRoot | Out-Null
Push-Location $repoRoot
try {
    $env:GOROOT = $goRoot
    $env:GOTOOLCHAIN = 'local'
    $env:CGO_ENABLED = '1'
    $env:PATH = "$(Join-Path $goRoot 'bin');$portableToolBin;$env:PATH"
    $env:CC = $gccPath
    $env:CXX = $gxxPath
    if ((& $gccPath -dumpfullversion).Trim() -ne '10.3.0') {
        throw 'approved TDM-GCC 10.3.0 is required'
    }
    & $goPath mod verify
    if ($LASTEXITCODE -ne 0) { throw 'go mod verify failed' }
    & $goPath build -trimpath -o $binaryPath ./cmd/wx_channel_poc
    if ($LASTEXITCODE -ne 0) { throw 'POC build failed' }
    $probeCommand = '""{0}" 2>&1"' -f $binaryPath
    $probeOutput = (& $env:ComSpec /d /s /c $probeCommand | Out-String).Trim()
    $probeExitCode = $LASTEXITCODE
    if ($probeExitCode -ne 2 -or $probeOutput -ne 'usage: wx_channel_poc <preflight|cert-smoke|run|cleanup>') {
        throw 'POC binary failed the no-side-effect loader probe'
    }
    Get-FileHash -Algorithm SHA256 -LiteralPath $binaryPath | ConvertTo-Json -Compress
}
finally {
    Pop-Location
}

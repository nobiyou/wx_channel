$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$buildRoot = Join-Path $repoRoot '.poc-build'
$binaryPath = Join-Path $buildRoot 'wx_channel_poc.exe'
$portableToolBin = Join-Path $repoRoot '.poc-tools\tdm-gcc-10.3.0-2\bin'
$gccPath = Join-Path $portableToolBin 'gcc.exe'
$gxxPath = Join-Path $portableToolBin 'g++.exe'

if (-not (Test-Path -LiteralPath $gccPath -PathType Leaf) -or -not (Test-Path -LiteralPath $gxxPath -PathType Leaf)) {
    throw 'approved portable TDM-GCC 10.3.0 is required'
}

New-Item -ItemType Directory -Force -Path $buildRoot | Out-Null
Push-Location $repoRoot
try {
    $env:CGO_ENABLED = '1'
    $env:PATH = "$portableToolBin;$env:PATH"
    $env:CC = $gccPath
    $env:CXX = $gxxPath
    if ((& $gccPath -dumpfullversion).Trim() -ne '10.3.0') {
        throw 'approved TDM-GCC 10.3.0 is required'
    }
    go mod verify
    if ($LASTEXITCODE -ne 0) { throw 'go mod verify failed' }
    go build -trimpath -o $binaryPath ./cmd/wx_channel_poc
    if ($LASTEXITCODE -ne 0) { throw 'POC build failed' }
    Get-FileHash -Algorithm SHA256 -LiteralPath $binaryPath | ConvertTo-Json -Compress
}
finally {
    Pop-Location
}

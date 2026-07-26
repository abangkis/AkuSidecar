param(
    [ValidateSet('aku-sidecar.exe', 'aku-sidecar.next.exe')]
    [string] $OutputName = 'aku-sidecar.exe'
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
$runtimeDir = Join-Path $repoRoot 'runtime\dev'
$cacheRoot = Join-Path $repoRoot '.go-build'

$previousGoCache = $env:GOCACHE
$previousGoModCache = $env:GOMODCACHE
$previousGoTmpDir = $env:GOTMPDIR
$env:GOCACHE = Join-Path $cacheRoot 'build'
$env:GOMODCACHE = Join-Path $cacheRoot 'mod'
$env:GOTMPDIR = Join-Path $cacheRoot 'tmp'

@($runtimeDir, $env:GOCACHE, $env:GOMODCACHE, $env:GOTMPDIR) | ForEach-Object {
    New-Item -ItemType Directory -Path $_ -Force | Out-Null
}

$output = Join-Path $runtimeDir $OutputName
Push-Location $repoRoot
try {
    & go build -trimpath -o $output .\cmd\akusidecar
    if ($LASTEXITCODE -ne 0) {
        throw "Go build failed with exit code $LASTEXITCODE."
    }
}
finally {
    $env:GOCACHE = $previousGoCache
    $env:GOMODCACHE = $previousGoModCache
    $env:GOTMPDIR = $previousGoTmpDir
    Pop-Location
}

Write-Host "Built AkuSidecar: $output"

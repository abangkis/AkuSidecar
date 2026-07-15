param(
    [ValidateSet('aku-sidecar.exe', 'aku-sidecar.next.exe')]
    [string] $OutputName = 'aku-sidecar.exe'
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
$workspaceRoot = Split-Path -Parent $repoRoot
$runtimeDir = Join-Path $repoRoot 'runtime\dev'
$cacheRoot = Join-Path $workspaceRoot '.go-cache'

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
    Pop-Location
}

Write-Host "Built AkuSidecar: $output"

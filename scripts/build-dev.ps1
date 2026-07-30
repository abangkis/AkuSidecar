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
$provenancePath = "$output.runtime-state.json"
Push-Location $repoRoot
try {
    & go build -trimpath -o $output .\cmd\akusidecar
    if ($LASTEXITCODE -ne 0) {
        throw "Go build failed with exit code $LASTEXITCODE."
    }

    $domainSource = Get-Content -LiteralPath (Join-Path $repoRoot 'internal\domain\types.go') -Raw
    if ($domainSource -notmatch 'ApplicationVersion\s*=\s*"([^"]+)"') {
        throw 'AkuSidecar ApplicationVersion could not be read for development provenance.'
    }
    $applicationVersion = $Matches[1]
    $sourceCommit = (& git -C $repoRoot rev-parse HEAD | Out-String).Trim()
    if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($sourceCommit)) {
        throw 'AkuSidecar source commit could not be read for development provenance.'
    }
    $sourceDirty = -not [string]::IsNullOrWhiteSpace((& git -C $repoRoot status --porcelain | Out-String).Trim())
    $binaryHash = (Get-FileHash -LiteralPath $output -Algorithm SHA256).Hash.ToLowerInvariant()
    $provenance = [ordered]@{
        schemaVersion = 1
        component = 'AkuSidecar'
        version = $applicationVersion
        sourceCommit = $sourceCommit
        sourceDirty = $sourceDirty
        builtAtUtc = [DateTime]::UtcNow.ToString('o')
        binaryFile = $OutputName
        binarySha256 = $binaryHash
    }
    $temporaryProvenance = "$provenancePath.tmp"
    [IO.File]::WriteAllText(
        $temporaryProvenance,
        ($provenance | ConvertTo-Json -Depth 5),
        [Text.UTF8Encoding]::new($false)
    )
    Move-Item -LiteralPath $temporaryProvenance -Destination $provenancePath -Force
}
finally {
    $env:GOCACHE = $previousGoCache
    $env:GOMODCACHE = $previousGoModCache
    $env:GOTMPDIR = $previousGoTmpDir
    Pop-Location
}

Write-Host "Built AkuSidecar: $output"
Write-Host "Recorded development provenance: $provenancePath"

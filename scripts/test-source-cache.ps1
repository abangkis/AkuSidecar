$ErrorActionPreference = 'Stop'
$repo = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
. (Join-Path $PSScriptRoot 'source-cache.ps1')

function Assert-Equal([string]$actual, [string]$expected, [string]$message) {
    if ($actual -cne $expected) { throw "$message Expected '$expected', got '$actual'." }
}

$cursor = Get-Item -LiteralPath $repo
$sharedTempRoot = $null
while ($null -ne $cursor) {
    $candidate = Join-Path $cursor.FullName 'SharedTemp'
    if (Test-Path -LiteralPath $candidate -PathType Container) {
        $sharedTempRoot = [IO.Path]::GetFullPath($candidate)
        break
    }
    $cursor = $cursor.Parent
}
if (-not $sharedTempRoot) { throw 'A nearest existing SharedTemp is required for this regression.' }

$names = @('GOCACHE','GOMODCACHE','npm_config_cache')
$previous = @{}
foreach ($name in $names) { $previous[$name] = [Environment]::GetEnvironmentVariable($name, 'Process') }
$fixtureRoot = [IO.Path]::GetFullPath((Join-Path $sharedTempRoot ('source-cache-test-' + [guid]::NewGuid().ToString('N'))))
$boundary = $sharedTempRoot.TrimEnd('\') + '\'
try {
    if (-not $fixtureRoot.StartsWith($boundary, [StringComparison]::OrdinalIgnoreCase) -or
        [IO.Path]::GetDirectoryName($fixtureRoot) -ne $sharedTempRoot) { throw 'Fixture escaped its direct SharedTemp child scope.' }
    New-Item -ItemType Directory -Path $fixtureRoot | Out-Null
    if ((Get-Item -LiteralPath $fixtureRoot).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Fixture root must not be a reparse point.' }

    foreach ($name in $names) { [Environment]::SetEnvironmentVariable($name, $null, 'Process') }
    $configRepo = Join-Path $fixtureRoot 'repository'
    $configDirectory = Join-Path $configRepo '.cache'
    New-Item -ItemType Directory -Path $configDirectory -Force | Out-Null
    $configuredRoot = Join-Path $fixtureRoot 'configured-cache-root'
    $configPath = Join-Path $configDirectory 'source-build-cache.json'
    [IO.File]::WriteAllText($configPath, (@{schemaVersion=1;root=$configuredRoot} | ConvertTo-Json -Compress), [Text.UTF8Encoding]::new($false))
    $configured = Get-SourceBuildCachePaths -Repository $configRepo
    Assert-Equal $configured.GoBuild (Join-Path $configuredRoot 'go-build') 'Repository cache config Go build path mismatch.'
    Assert-Equal $configured.GoModules (Join-Path $configuredRoot 'go-mod') 'Repository cache config Go module path mismatch.'
    Assert-Equal $configured.Npm (Join-Path $configuredRoot 'npm') 'Repository cache config npm path mismatch.'
    if (Test-Path -LiteralPath $configuredRoot) { throw 'Resolver created the configured cache root.' }

    $invalidConfig = @{schemaVersion=2;root=$configuredRoot} | ConvertTo-Json -Compress
    [IO.File]::WriteAllText($configPath, $invalidConfig, [Text.UTF8Encoding]::new($false))
    $rejectedInvalidConfig = $false
    try { Get-SourceBuildCachePaths -Repository $configRepo | Out-Null } catch { $rejectedInvalidConfig = $true }
    if (-not $rejectedInvalidConfig) { throw 'Unsupported cache config schema was accepted.' }
    [IO.File]::WriteAllText($configPath, (@{schemaVersion=1;root='C:relative'} | ConvertTo-Json -Compress), [Text.UTF8Encoding]::new($false))
    $rejectedRelativeRoot = $false
    try { Get-SourceBuildCachePaths -Repository $configRepo | Out-Null } catch { $rejectedRelativeRoot = $true }
    if (-not $rejectedRelativeRoot) { throw 'Drive-relative cache config root was accepted.' }
    [IO.File]::WriteAllText($configPath, (@{schemaVersion=1;root=$configuredRoot} | ConvertTo-Json -Compress), [Text.UTF8Encoding]::new($false))

    $seededBuild = Join-Path $fixtureRoot 'seeded-go-build'
    New-Item -ItemType Directory -Path $seededBuild | Out-Null
    $seedMarker = Join-Path $seededBuild 'seed.marker'
    [IO.File]::WriteAllText($seedMarker, 'seeded-cache-preserved', [Text.UTF8Encoding]::new($false))
    $overrideModules = Join-Path $fixtureRoot 'incoming-go-mod'
    $overrideNpm = Join-Path $fixtureRoot 'incoming-npm'
    $env:GOCACHE = $seededBuild
    $env:GOMODCACHE = $overrideModules
    $env:npm_config_cache = $overrideNpm
    $overrides = Get-SourceBuildCachePaths -Repository $configRepo
    Assert-Equal $overrides.GoBuild $seededBuild 'Incoming GOCACHE was not honored.'
    Assert-Equal $overrides.GoModules $overrideModules 'Incoming GOMODCACHE was not honored.'
    Assert-Equal $overrides.Npm $overrideNpm 'Incoming npm cache was not honored.'
    Assert-Equal ([IO.File]::ReadAllText($seedMarker)) 'seeded-cache-preserved' 'Resolver changed seeded cache content.'
    if (Test-Path -LiteralPath $overrideModules -PathType Container) { throw 'Resolver created an incoming module cache directory.' }
    if (Test-Path -LiteralPath $overrideNpm -PathType Container) { throw 'Resolver created an incoming npm cache directory.' }

    $offline = Get-Content -LiteralPath (Join-Path $PSScriptRoot 'test-inference-sdk-offline.ps1') -Raw
    if ($offline -notmatch "GOPROXY\s*=\s*'off'" -or -not $offline.Contains('Join-Path $projectRoot ''.go-build\mod''')) {
        throw 'Offline SDK gate lost its explicit offline proxy or seeded local module cache.'
    }
    Write-Output 'PASS: configured/shared cache selection, incoming overrides, seeded cache reuse, no resolver side effects, and offline module-cache control.'
} finally {
    foreach ($name in $names) { [Environment]::SetEnvironmentVariable($name, $previous[$name], 'Process') }
    if (Test-Path -LiteralPath $fixtureRoot -PathType Container) {
        $resolved = [IO.Path]::GetFullPath((Resolve-Path -LiteralPath $fixtureRoot).ProviderPath)
        if ($resolved -ne $fixtureRoot -or [IO.Path]::GetDirectoryName($resolved) -ne $sharedTempRoot -or
            -not $resolved.StartsWith($boundary, [StringComparison]::OrdinalIgnoreCase) -or
            ((Get-Item -LiteralPath $resolved).Attributes -band [IO.FileAttributes]::ReparsePoint)) {
            throw 'Refusing cleanup outside the owned source-cache fixture.'
        }
        if (Get-ChildItem -LiteralPath $resolved -Recurse -Force | Where-Object { $_.Attributes -band [IO.FileAttributes]::ReparsePoint }) {
            throw 'Refusing cleanup through a reparse point.'
        }
        Remove-Item -LiteralPath $resolved -Recurse -Force
    }
}
foreach ($name in $names) {
    Assert-Equal ([Environment]::GetEnvironmentVariable($name, 'Process')) $previous[$name] "Process environment restoration failed for $name."
}
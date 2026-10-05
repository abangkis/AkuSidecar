$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
$cursor = Get-Item -LiteralPath $repoRoot
$sharedTempRoot = $null
while ($null -ne $cursor) {
    $candidate = Join-Path $cursor.FullName 'SharedTemp'
    if (Test-Path -LiteralPath $candidate -PathType Container) {
        $sharedTempRoot = $candidate
        break
    }
    $cursor = $cursor.Parent
}
if (-not $sharedTempRoot) { throw 'Workspace SharedTemp is required for reader regression tests.' }

$testTemp = Join-Path $sharedTempRoot ('akusidecar-reader-tests-' + [Guid]::NewGuid().ToString('n'))
$savedEnvironment = @{}
$liveTestVariables = @(Get-ChildItem Env: | Where-Object {
    $_.Name -match '^AKU_(?:.*SMOKE_|AUTH_JOURNEY_|READER_TAB_BINDING_|NATIVE_READER_TABS_)|^AKUBROWSER_TEST_'
} | ForEach-Object { $_.Name })
foreach ($name in (@('GOCACHE', 'GOMODCACHE', 'GOTMPDIR', 'TEMP', 'TMP') + $liveTestVariables)) {
    $savedEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
}
Push-Location $repoRoot
try {
    # Build validation never inherits approval flags from a previous live trial.
    foreach ($name in $liveTestVariables) {
        [Environment]::SetEnvironmentVariable($name, $null, 'Process')
    }
    $env:GOCACHE = Join-Path $repoRoot '.go-build\build'
    $env:GOMODCACHE = Join-Path $repoRoot '.go-build\mod'
    $env:GOTMPDIR = $testTemp
    $env:TEMP = $testTemp
    $env:TMP = $testTemp
    foreach ($directory in @($env:GOCACHE, $env:GOMODCACHE, $testTemp)) {
        New-Item -ItemType Directory -Path $directory -Force | Out-Null
    }
    # Run complete packages: filtering to helper-only tests misses the owner
    # rotation that occurs before native-post queue admission.
    & go test -count=1 -timeout=90s ./internal/httpapi ./internal/readerbroker ./internal/collection ./internal/appshell
    if ($LASTEXITCODE -ne 0) { throw 'Native reader lifecycle regression tests failed; candidate build stopped.' }
    $frontendTests = @(Get-ChildItem -LiteralPath (Join-Path $repoRoot 'test') -Filter '*.test.mjs' -File | ForEach-Object { $_.FullName })
    if ($frontendTests.Count -eq 0) { throw 'Frontend regression tests are missing.' }
    & node --test @frontendTests
    if ($LASTEXITCODE -ne 0) { throw 'Frontend regression tests failed; candidate build stopped.' }
}
finally {
    foreach ($name in $savedEnvironment.Keys) {
        [Environment]::SetEnvironmentVariable($name, $savedEnvironment[$name], 'Process')
    }
    Pop-Location
}
Write-Host 'Native reader regression gate passed (no browser launch or runtime restart).'

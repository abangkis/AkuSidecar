param([switch]$NoChrome, [ValidateSet('overlap','mutation','coverage_loss')][string]$Scenario)
$ErrorActionPreference = 'Stop'
$projectRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$qaParent = Join-Path $projectRoot 'runtime\qa\auth-session-sidecar'
New-Item -ItemType Directory -Path $qaParent -Force | Out-Null
if ((Get-Item -LiteralPath $qaParent).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'QA parent must not be a reparse point.' }
$runId = [guid]::NewGuid().ToString('N')
$taskRoot = Join-Path $qaParent ('run-x-concurrency-suite-' + $runId)
New-Item -ItemType Directory -Path $taskRoot | Out-Null
$temporaryRoot = Join-Path $taskRoot 'go-tmp'
New-Item -ItemType Directory -Path $temporaryRoot | Out-Null
$cacheRoot = Join-Path $projectRoot '.go-build\x-session-verifier'
$moduleCache = Join-Path $projectRoot '.go-build\mod'
foreach ($path in @($cacheRoot, $moduleCache)) {
    New-Item -ItemType Directory -Path $path -Force | Out-Null
    if ((Get-Item -LiteralPath $path).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Go cache must not be a reparse point.' }
}
$nodePath = Join-Path $projectRoot 'runtime\dev\headless-worker\node.exe'
$chromePath = 'C:\Program Files\Google\Chrome\Application\chrome.exe'
if (!(Test-Path -LiteralPath $nodePath -PathType Leaf) -or (!$NoChrome -and !(Test-Path -LiteralPath $chromePath -PathType Leaf))) { throw 'Fixture executable unavailable.' }
$markerPath = Join-Path $taskRoot 'cleanup-verified.marker'
$logPath = Join-Path $qaParent ('x-concurrency-' + $runId + '.log.txt')
$variables = @('TEMP','TMP','GOTMPDIR','GOCACHE','GOMODCACHE','GOPROXY','GOSUMDB','GOTOOLCHAIN','CGO_ENABLED','GOFLAGS',
    'AKU_CHROME_FIXTURE_ROOT','AKU_CHROME_FIXTURE_CLEANUP_MARKER','AKU_TEST_NODE_PATH','AKU_TEST_CHROME_PATH','AKU_X_CONCURRENCY_FIXTURE_ENABLED')
$previous = @{}
foreach ($name in $variables) { $previous[$name] = [Environment]::GetEnvironmentVariable($name, 'Process') }
$safeCleanup = [bool]$NoChrome
Push-Location -LiteralPath $projectRoot
try {
    $env:TEMP = $temporaryRoot
    $env:TMP = $temporaryRoot
    $env:GOTMPDIR = $temporaryRoot
    $env:GOCACHE = $cacheRoot
    $env:GOMODCACHE = $moduleCache
    $env:GOPROXY = 'off'
    $env:GOSUMDB = 'off'
    $env:GOTOOLCHAIN = 'local'
    $env:CGO_ENABLED = '0'
    $env:GOFLAGS = ''
    $env:AKU_X_CONCURRENCY_FIXTURE_ENABLED = ''
    $env:AKU_CHROME_FIXTURE_ROOT = $taskRoot
    $env:AKU_CHROME_FIXTURE_CLEANUP_MARKER = $markerPath
    $env:AKU_TEST_NODE_PATH = $nodePath
    $env:AKU_TEST_CHROME_PATH = $chromePath
    Write-Output ('Synthetic concurrency run ID: ' + $runId)
    & $nodePath --test --test-timeout=10000 --test-reporter=tap `
        scripts/auth-session-fixture/x-concurrency-harness.test.mjs `
        scripts/auth-session-fixture/x-concurrency-chrome-adapter.test.mjs 2>&1 | Tee-Object -FilePath $logPath
    if ($LASTEXITCODE -ne 0) { throw 'Concurrency offline Node checks failed.' }
    & go test -tags 'auth_session_dev auth_session_fixture' ./internal/engine -run '^TestXConcurrencyFixtureReportOffline$' -count=1 -timeout=60s -v 2>&1 | Tee-Object -FilePath $logPath -Append
    if ($LASTEXITCODE -ne 0) { throw 'Concurrency report boundary check failed.' }
    if ($NoChrome) {
        Write-Output 'Offline checks passed; no Chrome or network listener started.'
    } else {
        Write-Output 'Opening fresh synthetic Chrome reader and headless collector under the owned Job. No input or login is needed.'
        $env:AKU_X_CONCURRENCY_FIXTURE_ENABLED = '1'
        $livePattern = '^TestXConcurrencyChromeFixture$'
        if ($Scenario) { $livePattern += '/^' + $Scenario + '$' }
        & go test -tags 'auth_session_dev auth_session_fixture' ./internal/engine -run $livePattern -count=1 -timeout=3m -v 2>&1 | Tee-Object -FilePath $logPath -Append
        $testCode = $LASTEXITCODE
        if (Test-Path -LiteralPath $markerPath -PathType Leaf) {
            $safeCleanup = (Get-Content -LiteralPath $markerPath -Raw).Trim() -eq 'verified'
        }
        if (!$safeCleanup) { throw 'Owned Job cleanup unverified; retaining synthetic profiles.' }
        if ($testCode -ne 0) { throw 'Synthetic Chrome concurrency checks failed; sanitized log retained.' }
        Write-Output 'Synthetic Chrome checks passed; this is not native X qualification or Store authorization.'
    }
    Write-Output ('Sanitized log: ' + $logPath)
} finally {
    Pop-Location
    foreach ($name in $variables) { [Environment]::SetEnvironmentVariable($name, $previous[$name], 'Process') }
    if ($safeCleanup) {
        $resolved = (Resolve-Path -LiteralPath $taskRoot).ProviderPath
        if ($resolved -ne $taskRoot -or [IO.Path]::GetDirectoryName($resolved) -ne $qaParent -or
            ((Get-Item -LiteralPath $resolved).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Refusing cleanup outside owned fixture root.' }
        if (Get-ChildItem -LiteralPath $resolved -Recurse -Force | Where-Object { $_.Attributes -band [IO.FileAttributes]::ReparsePoint }) { throw 'Refusing cleanup through reparse point.' }
        Remove-Item -LiteralPath $resolved -Recurse -Force
        Write-Output 'Owned disposable fixture root removed.'
    } else {
        Write-Warning ('Owned fixture root retained pending cleanup proof: ' + $taskRoot)
    }
}

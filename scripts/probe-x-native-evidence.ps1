param([switch]$NoChrome)
$ErrorActionPreference = 'Stop'
$projectRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$qaParent = Join-Path $projectRoot 'runtime\qa\auth-session-sidecar'
New-Item -ItemType Directory -Path $qaParent -Force | Out-Null
if ((Get-Item -LiteralPath $qaParent).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'QA parent must not be a reparse point.' }
$runId = [guid]::NewGuid().ToString('N')
$taskRoot = Join-Path $qaParent ('run-x-native-metadata-' + $runId)
New-Item -ItemType Directory -Path $taskRoot | Out-Null
$progressPath = Join-Path $qaParent ('x-native-metadata-' + $runId + '.progress.jsonl')
$reportPath = Join-Path $qaParent ('x-native-metadata-' + $runId + '.report.json')
$logPath = Join-Path $qaParent ('x-native-metadata-' + $runId + '.log.txt')
$cacheRoot = Join-Path $projectRoot '.go-build\x-session-verifier'
$moduleCache = Join-Path $projectRoot '.go-build\mod'
foreach ($cachePath in @($cacheRoot, $moduleCache)) {
    New-Item -ItemType Directory -Path $cachePath -Force | Out-Null
    if ((Get-Item -LiteralPath $cachePath).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Go cache must not be a reparse point.' }
}
$nodePath = 'C:\nvm4w\nodejs\node.exe'
$chromePath = 'C:\Program Files\Google\Chrome\Application\chrome.exe'
if (!(Test-Path -LiteralPath $nodePath -PathType Leaf)) { throw 'Probe Node executable unavailable.' }
if (!$NoChrome -and !(Test-Path -LiteralPath $chromePath -PathType Leaf)) { throw 'Probe Chrome executable unavailable.' }
$variables = @('TEMP','TMP','GOTMPDIR','GOCACHE','GOMODCACHE','GOPROXY','GOSUMDB','GOTOOLCHAIN','CGO_ENABLED','GOFLAGS',
    'AKU_CHROME_FIXTURE_ROOT','AKU_CHROME_FIXTURE_CLEANUP_MARKER','AKU_TEST_NODE_PATH','AKU_TEST_CHROME_PATH',
    'AKU_X_NATIVE_METADATA_ENABLED','AKU_X_NATIVE_METADATA_PROGRESS_PATH','AKU_X_NATIVE_METADATA_REPORT_PATH')
$previous = @{}
foreach ($variable in $variables) { $previous[$variable] = [Environment]::GetEnvironmentVariable($variable, 'Process') }
$safeCleanup = $true
Push-Location -LiteralPath $projectRoot
try {
    $env:TEMP = $taskRoot
    $env:TMP = $taskRoot
    $env:GOTMPDIR = $taskRoot
    $env:GOCACHE = $cacheRoot
    $env:GOMODCACHE = $moduleCache
    $env:GOPROXY = 'off'
    $env:GOSUMDB = 'off'
    $env:GOTOOLCHAIN = 'local'
    $env:CGO_ENABLED = '0'
    $env:GOFLAGS = ''
    $env:AKU_X_NATIVE_METADATA_ENABLED = '0'
    Write-Output ('Sanitized probe log: ' + $logPath)
    & $nodePath --test --test-timeout=10000 --test-reporter=tap scripts/auth-session-fixture/x-native-metadata-probe-driver.test.mjs 2>&1 | Tee-Object -FilePath $logPath
    if ($LASTEXITCODE -ne 0) { throw 'Native metadata offline Node checks failed.' }
    & go test -tags 'auth_session_dev auth_session_fixture' ./internal/engine -run '^TestXNativeMetadataProbeOffline$' -count=1 -timeout=60s -v 2>&1 | Tee-Object -FilePath $logPath -Append
    if ($LASTEXITCODE -ne 0) { throw 'Native metadata Go preflight failed.' }
    if ($NoChrome) {
        Write-Output 'Offline preflight passed; no Chrome or native traffic started.'
    } else {
        $env:AKU_CHROME_FIXTURE_ROOT = $taskRoot
        $env:AKU_CHROME_FIXTURE_CLEANUP_MARKER = Join-Path $taskRoot 'cleanup-verified.marker'
        $env:AKU_TEST_NODE_PATH = $nodePath
        $env:AKU_TEST_CHROME_PATH = $chromePath
        $env:AKU_X_NATIVE_METADATA_PROGRESS_PATH = $progressPath
        $env:AKU_X_NATIVE_METADATA_REPORT_PATH = $reportPath
        $env:AKU_X_NATIVE_METADATA_ENABLED = '1'
        Write-Output ('Progress: ' + $progressPath)
        Write-Output ('Sanitized report: ' + $reportPath)
        Write-Output 'Opening a separate Chrome profile named AkuBrowser X Native QA. Manual X login only; no Network observation until signed-in Home UI is detected.'
        $safeCleanup = $false
        & go test -tags 'auth_session_dev auth_session_fixture' ./internal/engine -run '^TestXNativeMetadataProbe$' -count=1 -timeout=8m -v 2>&1 | Tee-Object -FilePath $logPath -Append
        $testCode = $LASTEXITCODE
        if (Test-Path -LiteralPath $env:AKU_CHROME_FIXTURE_CLEANUP_MARKER -PathType Leaf) {
            $safeCleanup = (Get-Content -LiteralPath $env:AKU_CHROME_FIXTURE_CLEANUP_MARKER -Raw).Trim() -eq 'verified'
        }
        if (!$safeCleanup) { throw 'Owned QA Job cleanup is unverified; retaining the owned root.' }
        if ($testCode -ne 0) { throw 'Native metadata observation failed; sanitized progress/log retained.' }
        Write-Output 'Native metadata sample completed; partial single-page coverage, no identity/authentication/continuity authorization.'
    }
} finally {
    Pop-Location
    foreach ($variable in $variables) { [Environment]::SetEnvironmentVariable($variable, $previous[$variable], 'Process') }
    if ($safeCleanup) {
        $resolved = (Resolve-Path -LiteralPath $taskRoot).ProviderPath
        if ($resolved -ne $taskRoot -or [IO.Path]::GetDirectoryName($resolved) -ne $qaParent -or ((Get-Item -LiteralPath $resolved).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Refusing cleanup outside owned QA root.' }
        if (Get-ChildItem -LiteralPath $resolved -Recurse -Force | Where-Object { $_.Attributes -band [IO.FileAttributes]::ReparsePoint }) { throw 'Refusing cleanup through reparse point.' }
        Remove-Item -LiteralPath $resolved -Recurse -Force
        if ($NoChrome) { Write-Output 'Owned offline temporary root removed.' }
        else { Write-Output 'Owned QA profile/root removed. Local deletion does not revoke the new server-side X session.' }
    } else { Write-Warning ('Owned QA root retained pending empty-Job proof: ' + $taskRoot) }
}

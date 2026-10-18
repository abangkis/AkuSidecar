param([switch]$NoChrome, [switch]$Variants, [switch]$Handoff, [switch]$ProjectionOnly)
$ErrorActionPreference = 'Stop'
if ($Variants -and $Handoff) { throw 'Choose one fixed local case set: Variants or Handoff.' }
if ($ProjectionOnly -and !$Handoff) { throw 'ProjectionOnly requires the fixed Handoff mode.' }
$projectRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$qaParent = Join-Path $projectRoot 'runtime\qa\auth-session-sidecar'
New-Item -ItemType Directory -Path $qaParent -Force | Out-Null
if ((Get-Item -LiteralPath $qaParent).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'QA parent must be ordinary.' }
$runId = [guid]::NewGuid().ToString('N')
$taskRoot = Join-Path $qaParent ('run-x-local-bootstrap-' + $runId)
New-Item -ItemType Directory -Path $taskRoot | Out-Null
$reportPath = Join-Path $qaParent ('x-local-bootstrap-' + $runId + '.report.json')
$logPath = Join-Path $qaParent ('x-local-bootstrap-' + $runId + '.log.txt')
$nodePath = 'C:\nvm4w\nodejs\node.exe'
$chromePath = 'C:\Program Files\Google\Chrome\Application\chrome.exe'
if (!(Test-Path -LiteralPath $nodePath -PathType Leaf) -or (!$NoChrome -and !(Test-Path -LiteralPath $chromePath -PathType Leaf))) { throw 'Reproduction executable unavailable.' }
$cacheRoot = Join-Path $projectRoot '.go-build\x-session-verifier'
$moduleCache = Join-Path $projectRoot '.go-build\mod'
foreach ($cachePath in @($cacheRoot, $moduleCache)) {
    New-Item -ItemType Directory -Path $cachePath -Force | Out-Null
    if ((Get-Item -LiteralPath $cachePath).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Go cache must be ordinary.' }
}
$variables = @('TEMP','TMP','GOTMPDIR','GOCACHE','GOMODCACHE','GOPROXY','GOSUMDB','GOTOOLCHAIN','CGO_ENABLED','GOFLAGS',
    'AKU_CHROME_FIXTURE_ROOT','AKU_CHROME_FIXTURE_CLEANUP_MARKER','AKU_TEST_NODE_PATH','AKU_TEST_CHROME_PATH',
    'AKU_X_LOCAL_BOOTSTRAP_ENABLED','AKU_X_LOCAL_BOOTSTRAP_REPORT_PATH','AKU_X_LOCAL_BOOTSTRAP_VARIANTS','AKU_X_LOCAL_BOOTSTRAP_HANDOFF','AKU_X_LOCAL_BOOTSTRAP_PROJECTION_ONLY')
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
    $env:AKU_X_LOCAL_BOOTSTRAP_ENABLED = '0'
    $env:AKU_X_LOCAL_BOOTSTRAP_VARIANTS = if ($Variants) { '1' } else { '0' }
    $env:AKU_X_LOCAL_BOOTSTRAP_HANDOFF = if ($Handoff) { '1' } else { '0' }
    $env:AKU_X_LOCAL_BOOTSTRAP_PROJECTION_ONLY = if ($ProjectionOnly) { '1' } else { '0' }
    $env:AKU_X_LOCAL_BOOTSTRAP_REPORT_PATH = $reportPath
    $env:AKU_CHROME_FIXTURE_ROOT = $taskRoot
    $env:AKU_CHROME_FIXTURE_CLEANUP_MARKER = Join-Path $taskRoot 'cleanup-verified.marker'
    Write-Output ('Accountless reproduction log: ' + $logPath)
    $nodeTests = @('scripts/auth-session-fixture/x-local-bootstrap-driver.test.mjs')
    if ($Handoff) { $nodeTests += 'scripts/auth-session-fixture/x-bootstrap-current-self.test.mjs' }
    & $nodePath --test --test-timeout=10000 --test-reporter=tap @nodeTests 2>&1 | Tee-Object -FilePath $logPath
    if ($LASTEXITCODE -ne 0) { throw 'Local reproduction Node preflight failed.' }
    & go test -tags 'auth_session_dev auth_session_fixture' ./internal/engine -run '^TestXLocalBootstrapOffline$' -count=1 -timeout=60s -v 2>&1 | Tee-Object -FilePath $logPath -Append
    if ($LASTEXITCODE -ne 0) { throw 'Local reproduction Go preflight failed.' }
    if ($NoChrome) { Write-Output 'Local reproduction offline checks passed; no Chrome or network started.' }
    else {
        $env:AKU_X_LOCAL_BOOTSTRAP_ENABLED = '1'
        $env:AKU_TEST_NODE_PATH = $nodePath
        $env:AKU_TEST_CHROME_PATH = $chromePath
        if ($ProjectionOnly) {
            Write-Output 'Opening one fresh accountless handoff projection case for the descriptor-only causal correction. Full side-effect rejection and all ID/location/context/cleanup gates remain enabled. No retained profile, X account, Store or user runtime is used.'
        } elseif ($Handoff) {
            Write-Output 'Opening two fresh accountless handoff cases: full-guard true control, then synthetic bootstrap projection. Instrumentation installs a normal breakpoint only; evaluation requires its exact returned ID/location and current root/default-context generation. Both breakpoint classes must be removed before a fresh local Home check and late read. No retained profile, X account, Store or user runtime is used.'
        } elseif ($Variants) {
            Write-Output 'Opening four accountless mechanism cases in fresh separate Jobs/profiles: instrumentation resume-only, minimal callframe true, ordinary debugger-statement with full callframe true, and instrumentation Runtime true. Actual pause causes and the disclosed ordinary-pause eligibility adapter are recorded separately. No login, retained profile, external page, Store, user runtime or bootstrap projection is used by these cases.'
        } else {
            Write-Output 'Opening two accountless loopback QA cases, one at a time, with fresh disposable profiles and separate Windows Jobs. No login, retained profile, external page, Store or user runtime is used. The tiny constant control and full projector produce value-free chronology; a reproduced command failure is evidence, not capture qualification.'
        }
        $safeCleanup = $false
        $nativeTimeout = if ($Variants) { '6m' } else { '3m' }
        & go test -tags 'auth_session_dev auth_session_fixture' ./internal/engine -run '^TestXLocalBootstrapRepro$' -count=1 "-timeout=$nativeTimeout" -v 2>&1 | Tee-Object -FilePath $logPath -Append
        $testCode = $LASTEXITCODE
        if (Test-Path -LiteralPath $env:AKU_CHROME_FIXTURE_CLEANUP_MARKER -PathType Leaf) {
            $safeCleanup = (Get-Content -LiteralPath $env:AKU_CHROME_FIXTURE_CLEANUP_MARKER -Raw).Trim() -eq 'verified'
        }
        if (!$safeCleanup) { throw 'Local reproduction Job cleanup unverified; owned root retained.' }
        Write-Output ('Sanitized accountless reproduction report: ' + $reportPath)
        if ($testCode -ne 0) { throw 'Local reproduction incomplete; bounded diagnostics retained.' }
        Write-Output 'The bounded local cases completed and owned Jobs stopped. Read each diagnostic to distinguish transport failure from capture qualification.'
    }
} finally {
    Pop-Location
    foreach ($variable in $variables) { [Environment]::SetEnvironmentVariable($variable, $previous[$variable], 'Process') }
    if ($safeCleanup) {
        $resolved = (Resolve-Path -LiteralPath $taskRoot).ProviderPath
        if ($resolved -ne $taskRoot -or [IO.Path]::GetDirectoryName($resolved) -ne $qaParent -or
            ((Get-Item -LiteralPath $resolved).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Refusing cleanup outside owned reproduction root.' }
        if (Get-ChildItem -LiteralPath $resolved -Recurse -Force | Where-Object { $_.Attributes -band [IO.FileAttributes]::ReparsePoint }) { throw 'Refusing cleanup through reparse point.' }
        Remove-Item -LiteralPath $resolved -Recurse -Force
        Write-Output 'Owned accountless reproduction root and fresh profiles removed.'
    } else { Write-Warning ('Owned reproduction root retained pending cleanup proof: ' + $taskRoot) }
}

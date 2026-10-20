param([switch]$RunNativeQA, [switch]$AllowCookieMetadataRead, [switch]$SettingsTypesOnly, [switch]$AuthenticationControl, [switch]$DirectAuthenticationAB)
$ErrorActionPreference = 'Stop'
if ($DirectAuthenticationAB -and (!$RunNativeQA -or $AllowCookieMetadataRead -or $SettingsTypesOnly -or $AuthenticationControl)) { throw 'DirectAuthenticationAB requires native QA and forbids combined modes or cookie access.' }
if ($AuthenticationControl -and (!$RunNativeQA -or $AllowCookieMetadataRead -or $SettingsTypesOnly)) { throw 'AuthenticationControl requires native QA and forbids cookie access or combined modes.' }
if ($AllowCookieMetadataRead -and (!$RunNativeQA -or $SettingsTypesOnly)) { throw 'Cookie metadata permission requires native execution without SettingsTypesOnly.' }
if ($RunNativeQA -and !$DirectAuthenticationAB -and !$AuthenticationControl -and !$SettingsTypesOnly -and !$AllowCookieMetadataRead) { throw 'Native cookie inspection requires explicit AllowCookieMetadataRead. Use SettingsTypesOnly for no cookie access.' }
$projectRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$qaParent = Join-Path $projectRoot 'runtime\qa\auth-session-sidecar'
New-Item -ItemType Directory -Path $qaParent -Force | Out-Null
if ((Get-Item -LiteralPath $qaParent).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'QA parent must not be a reparse point.' }
$runId = [guid]::NewGuid().ToString('N')
$taskRoot = Join-Path $qaParent ('run-x-qa-source-' + $runId)
New-Item -ItemType Directory -Path $taskRoot | Out-Null
$cache = Join-Path $projectRoot '.go-build\x-session-verifier'
$modules = Join-Path $projectRoot '.go-build\mod'
foreach ($path in @($cache, $modules)) {
    New-Item -ItemType Directory -Path $path -Force | Out-Null
    if ((Get-Item -LiteralPath $path).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Go cache must not be a reparse point.' }
}
$node = Join-Path $projectRoot 'runtime\dev\headless-worker\node.exe'
$chrome = 'C:\Program Files\Google\Chrome\Application\chrome.exe'
if (!(Test-Path -LiteralPath $node -PathType Leaf) -or ($RunNativeQA -and !(Test-Path -LiteralPath $chrome -PathType Leaf))) { throw 'QA executable unavailable.' }
$marker = Join-Path $taskRoot 'cleanup-verified.marker'
$log = Join-Path $qaParent ('x-qa-source-' + $runId + '.log.txt')
$report = Join-Path $qaParent ('x-qa-source-' + $runId + '.report.json')
$variables = @('TEMP','TMP','GOTMPDIR','GOCACHE','GOMODCACHE','GOPROXY','GOSUMDB','GOTOOLCHAIN','CGO_ENABLED','GOFLAGS',
    'AKU_CHROME_FIXTURE_ROOT','AKU_CHROME_FIXTURE_CLEANUP_MARKER','AKU_TEST_NODE_PATH','AKU_TEST_CHROME_PATH',
    'AKU_X_QA_SOURCE_QUALIFICATION_ENABLED','AKU_X_QA_COOKIE_METADATA_ALLOWED','AKU_X_QA_SETTINGS_TYPES_ONLY','AKU_X_QA_AUTH_CONTROL','AKU_X_QA_DIRECT_AB','AKU_X_QA_SOURCE_REPORT_PATH')
$previous = @{}
foreach ($name in $variables) { $previous[$name] = [Environment]::GetEnvironmentVariable($name, 'Process') }
$safeCleanup = $true
Push-Location -LiteralPath $projectRoot
try {
    $env:TEMP = $taskRoot; $env:TMP = $taskRoot; $env:GOTMPDIR = $taskRoot
    $env:GOCACHE = $cache; $env:GOMODCACHE = $modules
    $env:GOPROXY = 'off'; $env:GOSUMDB = 'off'; $env:GOTOOLCHAIN = 'local'; $env:CGO_ENABLED = '0'; $env:GOFLAGS = ''
    $env:AKU_CHROME_FIXTURE_ROOT = $taskRoot; $env:AKU_CHROME_FIXTURE_CLEANUP_MARKER = $marker
    $env:AKU_TEST_NODE_PATH = $node; $env:AKU_TEST_CHROME_PATH = $chrome
    $env:AKU_X_QA_SOURCE_QUALIFICATION_ENABLED = '0'; $env:AKU_X_QA_COOKIE_METADATA_ALLOWED = '0'
    $env:AKU_X_QA_SETTINGS_TYPES_ONLY = '0'
    $env:AKU_X_QA_AUTH_CONTROL = '0'
    $env:AKU_X_QA_DIRECT_AB = '0'
    $env:AKU_X_QA_SOURCE_REPORT_PATH = $report
    Write-Output ('QA source preflight run: ' + $runId)
    & $node --test --test-timeout=10000 --test-reporter=tap `
        scripts/auth-session-fixture/x-qa-source-qualification.test.mjs `
        scripts/auth-session-fixture/x-qa-auth-control.test.mjs `
        scripts/auth-session-fixture/x-qa-direct-ab.test.mjs `
        scripts/auth-session-fixture/x-native-plan-offline.test.mjs `
        scripts/auth-session-fixture/x-native-metadata-probe-driver.test.mjs `
        scripts/auth-session-fixture/x-retained-account-settings-driver.test.mjs `
        scripts/auth-session-fixture/x-account-settings-body.test.mjs `
        scripts/auth-session-fixture/x-account-settings-capture.test.mjs 2>&1 | Tee-Object -FilePath $log
    if ($LASTEXITCODE -ne 0) { throw 'QA source Node preflight failed.' }
    & go test -tags 'auth_session_dev auth_session_fixture' ./internal/engine -run '^TestX(QADirectAB|QASourceQualification|RetainedQAProfile|AccountSettingsProjection|TwoPhaseMetadata|StandardChromeControl|NativeMetadataProbe|PopupDiagnostics)Offline$' -count=1 -timeout=60s -v 2>&1 | Tee-Object -FilePath $log -Append
    if ($LASTEXITCODE -ne 0) { throw 'QA source Go preflight failed.' }
    if ($RunNativeQA) {
        if ($DirectAuthenticationAB) { Write-Output 'Fixed direct-navigation A/B: retained QA context and one empty BrowserContext. Two bounded response projections; no cookie/header extraction, session transfer, login, logout or Store operation.' }
        elseif ($AuthenticationControl) { Write-Output 'Retained QA plus one new empty BrowserContext for natural Home comparison. No cookie reads, transfer, constructed API request, logout, new login or Store operation.' }
        elseif ($SettingsTypesOnly) { Write-Output 'Opening only retained QA for six fixed settings field types. Cookie reads disabled. No raw values, transfer, new login, logout or Store operation.' }
        else { Write-Output 'Opening only retained QA. Scoped CDP cookie replies include transient values; only attributes are retained. No transfer, new login, logout or Store operation.' }
        $env:AKU_X_QA_SOURCE_QUALIFICATION_ENABLED = '1'
        $env:AKU_X_QA_COOKIE_METADATA_ALLOWED = if ($AllowCookieMetadataRead) { '1' } else { '0' }
        $env:AKU_X_QA_SETTINGS_TYPES_ONLY = if ($SettingsTypesOnly) { '1' } else { '0' }
        $env:AKU_X_QA_AUTH_CONTROL = if ($AuthenticationControl) { '1' } else { '0' }
        $env:AKU_X_QA_DIRECT_AB = if ($DirectAuthenticationAB) { '1' } else { '0' }
        $safeCleanup = $false
        & go test -tags 'auth_session_dev auth_session_fixture' ./internal/engine -run '^TestXQASourceQualification$' -count=1 -timeout=2m -v 2>&1 | Tee-Object -FilePath $log -Append
        $testCode = $LASTEXITCODE
        if (Test-Path -LiteralPath $marker -PathType Leaf) { $safeCleanup = (Get-Content -LiteralPath $marker -Raw).Trim() -eq 'verified' }
        if (!$safeCleanup) { throw 'Owned Job cleanup unverified; retaining control root.' }
        if ($testCode -ne 0) { throw 'QA source run failed; fixed diagnostics retained.' }
        Write-Output ('QA attribute/source report: ' + $report)
    } else { Write-Output 'Offline preflight complete. No retained QA profile, Chrome or network listener opened.' }
    Write-Output ('Sanitized log: ' + $log)
} finally {
    Pop-Location
    foreach ($name in $variables) { [Environment]::SetEnvironmentVariable($name, $previous[$name], 'Process') }
    if ($safeCleanup) {
        $resolved = (Resolve-Path -LiteralPath $taskRoot).ProviderPath
        if ($resolved -ne $taskRoot -or [IO.Path]::GetDirectoryName($resolved) -ne $qaParent -or
            ((Get-Item -LiteralPath $resolved).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Refusing cleanup outside owned QA root.' }
        if (Get-ChildItem -LiteralPath $resolved -Recurse -Force | Where-Object { $_.Attributes -band [IO.FileAttributes]::ReparsePoint }) { throw 'Refusing cleanup through reparse point.' }
        Remove-Item -LiteralPath $resolved -Recurse -Force
        Write-Output 'Owned disposable control root removed; retained profile is never deleted by this helper.'
    } else { Write-Warning ('Owned control root retained: ' + $taskRoot) }
}

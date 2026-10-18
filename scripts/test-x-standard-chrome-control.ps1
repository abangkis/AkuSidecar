param(
    [switch]$NoChrome,
    [switch]$TwoPhase,
    [switch]$AccountSettings,
    [switch]$RetainQA,
    [switch]$BootstrapSelf,
    [switch]$BootstrapEarly,
    [switch]$LocalPopup,
    [switch]$PhysicalPopup,
    [ValidatePattern('^[a-f0-9]{32}$')][string]$CompleteRun,
    [ValidateSet('home_visible','google_rejected','x_rejected','other_error','not_attempted','cancelled')][string]$Outcome,
    [ValidateSet('not_observed','no_visible_change','blocked_icon_visible','popup_hidden_or_blank')][string]$PopupObservation
)
$ErrorActionPreference = 'Stop'
$projectRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$qaParent = Join-Path $projectRoot 'runtime\qa\auth-session-sidecar'
New-Item -ItemType Directory -Path $qaParent -Force | Out-Null
if ((Get-Item -LiteralPath $qaParent).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'QA parent must not be a reparse point.' }
if ($LocalPopup -and ($TwoPhase -or $CompleteRun -or $Outcome -or $PopupObservation)) { throw 'LocalPopup is an accountless standalone mode.' }
if ($PhysicalPopup -and !$LocalPopup) { throw 'PhysicalPopup requires LocalPopup.' }
if ($AccountSettings -and !$TwoPhase) { throw 'AccountSettings requires the two-phase workflow.' }
if ($BootstrapSelf -and (!$RetainQA -or !$TwoPhase -or $AccountSettings -or $LocalPopup)) { throw 'BootstrapSelf requires TwoPhase RetainQA without AccountSettings.' }
if ($BootstrapEarly -and !$BootstrapSelf) { throw 'BootstrapEarly requires BootstrapSelf.' }
if ($RetainQA -and (!$TwoPhase -or (!$AccountSettings -and !$BootstrapSelf) -or $LocalPopup)) { throw 'RetainQA requires a fixed TwoPhase diagnostic mode.' }
if ($PopupObservation -and (!$TwoPhase -or !$CompleteRun)) { throw 'PopupObservation requires TwoPhase CompleteRun.' }
$artifactPrefix = if ($LocalPopup) { 'x-local-popup' } elseif ($TwoPhase) { 'x-two-phase' } else { 'x-standard-chrome' }
if ($CompleteRun) {
    if (!$Outcome -or $NoChrome) { throw 'CompleteRun requires one fixed outcome and no NoChrome switch.' }
    $activeRoot = Join-Path $qaParent ('run-' + $artifactPrefix + '-' + $CompleteRun)
    $progress = Join-Path $qaParent ($artifactPrefix + '-' + $CompleteRun + '.progress.jsonl')
    if (!(Test-Path -LiteralPath $activeRoot -PathType Container) -or !(Test-Path -LiteralPath $progress -PathType Leaf)) { throw 'Owned control run is not active.' }
    $resultPath = Join-Path $qaParent ($artifactPrefix + '-' + $CompleteRun + '.result.json')
    $staging = $resultPath + '.pending'
    $fixedResult = @{outcome=$Outcome}
    if ($PopupObservation) { $fixedResult.popupObservation = $PopupObservation }
    $bytes = [Text.Encoding]::UTF8.GetBytes(($fixedResult | ConvertTo-Json -Compress))
    $stream = [IO.File]::Open($staging, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None)
    try { $stream.Write($bytes, 0, $bytes.Length) } finally { $stream.Dispose() }
    [IO.File]::Move($staging, $resultPath)
    Write-Output ('Fixed manual outcome submitted: ' + $Outcome + '. The owned QA workflow will continue or clean up.')
    return
}
if ($Outcome) { throw 'Outcome requires CompleteRun.' }
$runId = [guid]::NewGuid().ToString('N')
$taskRoot = Join-Path $qaParent ('run-' + $artifactPrefix + '-' + $runId)
New-Item -ItemType Directory -Path $taskRoot | Out-Null
$progressPath = Join-Path $qaParent ($artifactPrefix + '-' + $runId + '.progress.jsonl')
$resultPath = Join-Path $qaParent ($artifactPrefix + '-' + $runId + '.result.json')
$reportPath = Join-Path $qaParent ($artifactPrefix + '-' + $runId + '.report.json')
$logPath = Join-Path $qaParent ($artifactPrefix + '-' + $runId + '.log.txt')
$cacheRoot = Join-Path $projectRoot '.go-build\x-session-verifier'
$moduleCache = Join-Path $projectRoot '.go-build\mod'
foreach ($cachePath in @($cacheRoot, $moduleCache)) {
    New-Item -ItemType Directory -Path $cachePath -Force | Out-Null
    if ((Get-Item -LiteralPath $cachePath).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Go cache must not be a reparse point.' }
}
$nodePath = 'C:\nvm4w\nodejs\node.exe'
$chromePath = 'C:\Program Files\Google\Chrome\Application\chrome.exe'
if (!(Test-Path -LiteralPath $nodePath -PathType Leaf) -or (!$NoChrome -and !(Test-Path -LiteralPath $chromePath -PathType Leaf))) { throw 'Control executable unavailable.' }
$variables = @('TEMP','TMP','GOTMPDIR','GOCACHE','GOMODCACHE','GOPROXY','GOSUMDB','GOTOOLCHAIN','CGO_ENABLED','GOFLAGS',
    'AKU_CHROME_FIXTURE_ROOT','AKU_CHROME_FIXTURE_CLEANUP_MARKER','AKU_TEST_NODE_PATH','AKU_TEST_CHROME_PATH',
    'AKU_X_STANDARD_CONTROL_ENABLED','AKU_X_STANDARD_PROGRESS_PATH','AKU_X_STANDARD_RESULT_PATH','AKU_X_STANDARD_REPORT_PATH',
    'AKU_X_TWO_PHASE_ENABLED','AKU_X_TWO_PHASE_PROGRESS_PATH','AKU_X_TWO_PHASE_RESULT_PATH','AKU_X_TWO_PHASE_REPORT_PATH',
    'AKU_X_LOCAL_POPUP_ENABLED','AKU_X_LOCAL_POPUP_REPORT_PATH','AKU_X_LOCAL_POPUP_PHYSICAL','AKU_X_LOCAL_POPUP_PROGRESS_PATH',
    'AKU_X_ACCOUNT_SETTINGS_ENABLED','AKU_X_RETAINED_QA_ENABLED','AKU_X_BOOTSTRAP_SELF_ENABLED','AKU_X_BOOTSTRAP_EARLY_ENABLED')
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
    $env:AKU_X_STANDARD_CONTROL_ENABLED = '0'
    $env:AKU_X_TWO_PHASE_ENABLED = '0'
    $env:AKU_X_LOCAL_POPUP_ENABLED = '0'
    $env:AKU_X_LOCAL_POPUP_PHYSICAL = '0'
    $env:AKU_X_ACCOUNT_SETTINGS_ENABLED = '0'
    $env:AKU_X_RETAINED_QA_ENABLED = '0'
    $env:AKU_X_BOOTSTRAP_SELF_ENABLED = '0'
    $env:AKU_X_BOOTSTRAP_EARLY_ENABLED = '0'
    $env:AKU_CHROME_FIXTURE_ROOT = $taskRoot
    Write-Output ('Control log: ' + $logPath)
    $nodeTests = @('scripts/auth-session-fixture/x-standard-chrome-control.test.mjs')
    if ($TwoPhase) {
        $nodeTests += 'scripts/auth-session-fixture/x-native-metadata-probe-driver.test.mjs'
        $nodeTests += 'scripts/auth-session-fixture/x-two-phase-metadata-driver.test.mjs'
    }
    if ($LocalPopup) { $nodeTests += 'scripts/auth-session-fixture/x-local-popup-control.test.mjs' }
    if ($AccountSettings) {
        $nodeTests += 'scripts/auth-session-fixture/x-account-settings-body.test.mjs'
        $nodeTests += 'scripts/auth-session-fixture/x-account-settings-capture.test.mjs'
    }
    if ($RetainQA) { $nodeTests += 'scripts/auth-session-fixture/x-retained-account-settings-driver.test.mjs' }
    if ($BootstrapSelf) { $nodeTests += 'scripts/auth-session-fixture/x-bootstrap-current-self.test.mjs' }
    if ($BootstrapEarly) {
        $nodeTests += 'scripts/auth-session-fixture/x-early-bootstrap-capture.test.mjs'
        $nodeTests += 'scripts/auth-session-fixture/x-bootstrap-breakpoint-handoff.test.mjs'
    }
    & $nodePath --test --test-timeout=10000 --test-reporter=tap @nodeTests 2>&1 | Tee-Object -FilePath $logPath
    if ($LASTEXITCODE -ne 0) { throw 'Control launch-only checks failed.' }
    $offlinePattern = if ($LocalPopup) { '^TestX(LocalPopup|PopupDiagnostics|TwoPhaseMetadata)Offline$' } elseif ($AccountSettings) { '^TestX(AccountSettingsProjection|TwoPhaseMetadata|StandardChromeControl|NativeMetadataProbe|PopupDiagnostics)Offline$' } elseif ($TwoPhase) { '^TestX(TwoPhaseMetadata|StandardChromeControl|NativeMetadataProbe|PopupDiagnostics)Offline$' } else { '^TestXStandardChromeControlOffline$' }
    if ($RetainQA) { $offlinePattern = '^TestX(RetainedQAProfile|AccountSettingsProjection|TwoPhaseMetadata|StandardChromeControl|NativeMetadataProbe|PopupDiagnostics)Offline$' }
    if ($BootstrapSelf) { $offlinePattern = '^TestX(BootstrapCurrentSelf|RetainedQAProfile|TwoPhaseMetadata|StandardChromeControl|NativeMetadataProbe|PopupDiagnostics)Offline$' }
    if ($BootstrapEarly) { $offlinePattern = '^TestX(EarlyBootstrap|BootstrapCurrentSelf|RetainedQAProfile|TwoPhaseMetadata|StandardChromeControl|NativeMetadataProbe|PopupDiagnostics)Offline$' }
    & go test -tags 'auth_session_dev auth_session_fixture' ./internal/engine -run $offlinePattern -count=1 -timeout=60s -v 2>&1 | Tee-Object -FilePath $logPath -Append
    if ($LASTEXITCODE -ne 0) { throw 'Control manual-outcome preflight failed.' }
    if ($NoChrome) { Write-Output 'Control offline preflight passed; no Chrome or network started.' }
    else {
        $env:AKU_CHROME_FIXTURE_ROOT = $taskRoot
        $env:AKU_CHROME_FIXTURE_CLEANUP_MARKER = Join-Path $taskRoot 'cleanup-verified.marker'
        $env:AKU_TEST_NODE_PATH = $nodePath
        $env:AKU_TEST_CHROME_PATH = $chromePath
        if ($LocalPopup) {
            $env:AKU_X_LOCAL_POPUP_ENABLED = '1'
            $env:AKU_X_LOCAL_POPUP_REPORT_PATH = $reportPath
            $env:AKU_X_LOCAL_POPUP_PROGRESS_PATH = $progressPath
            $env:AKU_X_LOCAL_POPUP_PHYSICAL = if ($PhysicalPopup) { '1' } else { '0' }
        } elseif ($TwoPhase) {
            $env:AKU_X_ACCOUNT_SETTINGS_ENABLED = if ($AccountSettings) { '1' } else { '0' }
            $env:AKU_X_RETAINED_QA_ENABLED = if ($RetainQA) { '1' } else { '0' }
            $env:AKU_X_BOOTSTRAP_SELF_ENABLED = if ($BootstrapSelf) { '1' } else { '0' }
            $env:AKU_X_BOOTSTRAP_EARLY_ENABLED = if ($BootstrapEarly) { '1' } else { '0' }
            $env:AKU_X_TWO_PHASE_ENABLED = '1'
            $env:AKU_X_TWO_PHASE_PROGRESS_PATH = $progressPath
            $env:AKU_X_TWO_PHASE_RESULT_PATH = $resultPath
            $env:AKU_X_TWO_PHASE_REPORT_PATH = $reportPath
        } else {
            $env:AKU_X_STANDARD_CONTROL_ENABLED = '1'
            $env:AKU_X_STANDARD_PROGRESS_PATH = $progressPath
            $env:AKU_X_STANDARD_RESULT_PATH = $resultPath
            $env:AKU_X_STANDARD_REPORT_PATH = $reportPath
        }
        Write-Output ('Control run ID: ' + $runId)
        Write-Output ('Progress: ' + $progressPath)
        if ($LocalPopup -and $PhysicalPopup) { Write-Output 'Opening AkuBrowser Local Popup QA: loopback only, no account or login. Wait for the ready instruction, click Open local popup once, then leave both QA windows open for automatic cleanup; two-minute click deadline.' }
        elseif ($LocalPopup) { Write-Output 'Opening AkuBrowser Local Popup QA: loopback pages only, no account or login. The test queues scoped mouse messages to its owned browser window without moving the cursor or forcing foreground, records bounded diagnostics, then closes its Job.' }
        elseif ($BootstrapEarly) { Write-Output 'Opening existing retained X QA: after Home, one bounded reload uses instrumentation only to install up to eight normal breakpoints; fixed guarded projection runs only at verified normal hits (64 pauses/5 seconds). Both breakpoint classes must be cleaned before a fresh Home check and late projection. No Network/body/source reads or new login; partial coverage remains diagnostic only.' }
        elseif ($BootstrapSelf) { Write-Output 'Opening existing retained X QA for one bootstrap shape diagnostic after Home. No Network observation or response-body reads. No login attempt; fixed diagnostic output only; no Store authority.' }
        elseif ($RetainQA) { Write-Output 'Opening dedicated retained X QA: first run requires manual login without CDP; later runs check the same owned profile automatically. Only this QA profile persists. Node/Chrome close each run; body values are not exported; no Store authority.' }
        elseif ($TwoPhase -and $AccountSettings) { Write-Output 'Opening AkuBrowser X QA Two Phase: manual phone login, then same-profile reopen. After Home detection, one natural account-settings response may be read once through the bounded shape projector; only fixed diagnostics are saved. No raw body/values are exported; no Store authority.' }
        elseif ($TwoPhase) { Write-Output 'Opening AkuBrowser X QA Two Phase: log in manually in standard Chrome; report Home in chat. The helper will close only owned windows, verify release, then reopen the same QA profile for bounded observation without a second login.' }
        else { Write-Output 'Opening AkuBrowser X QA Control: new profile, standard Chrome, no CDP, page reads, extensions or network observers installed by the helper. Report the result in chat; five-minute limit.' }
        $safeCleanup = $false
        $livePattern = if ($LocalPopup) { '^TestXLocalPopup$' } elseif ($TwoPhase) { '^TestXTwoPhaseMetadata$' } else { '^TestXStandardChromeControl$' }
        & go test -tags 'auth_session_dev auth_session_fixture' ./internal/engine -run $livePattern -count=1 -timeout=8m -v 2>&1 | Tee-Object -FilePath $logPath -Append
        $testCode = $LASTEXITCODE
        if (Test-Path -LiteralPath $env:AKU_CHROME_FIXTURE_CLEANUP_MARKER -PathType Leaf) {
            $safeCleanup = (Get-Content -LiteralPath $env:AKU_CHROME_FIXTURE_CLEANUP_MARKER -Raw).Trim() -eq 'verified'
        }
        if (!$safeCleanup) { throw 'Control owned Job cleanup unverified; retaining owned root.' }
        if ($testCode -ne 0) { throw 'Control harness failed; sanitized evidence retained.' }
        Write-Output ('QA lifecycle completed; diagnostics cannot authorize Store work. Report: ' + $reportPath)
    }
} finally {
    Pop-Location
    foreach ($variable in $variables) { [Environment]::SetEnvironmentVariable($variable, $previous[$variable], 'Process') }
    if ($safeCleanup) {
        $resolved = (Resolve-Path -LiteralPath $taskRoot).ProviderPath
        if ($resolved -ne $taskRoot -or [IO.Path]::GetDirectoryName($resolved) -ne $qaParent -or ((Get-Item -LiteralPath $resolved).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Refusing cleanup outside owned control root.' }
        if (Get-ChildItem -LiteralPath $resolved -Recurse -Force | Where-Object { $_.Attributes -band [IO.FileAttributes]::ReparsePoint }) { throw 'Refusing cleanup through reparse point.' }
        Remove-Item -LiteralPath $resolved -Recurse -Force
        if ($RetainQA -and !$NoChrome) { Write-Output 'Ephemeral control root removed; dedicated QA profile retained under its ownership guard. All owned processes stopped.' }
        else { Write-Output 'Owned control root/profile removed. Local deletion does not revoke any new server-side login session.' }
    } else { Write-Warning ('Owned control root retained pending cleanup proof: ' + $taskRoot) }
}

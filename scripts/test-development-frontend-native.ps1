param([Parameter(Mandatory=$true)][string]$TaskRoot,[string]$ApprovedHostName = '',[ValidateSet('normal','foreground_only','reader_open_parent_death')][string]$Scenario = 'normal',[switch]$Verify)
$ErrorActionPreference = 'Stop'
$project = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$parent = Join-Path $project 'runtime\qa\auth-session-sidecar'
$root = (Resolve-Path -LiteralPath $TaskRoot).ProviderPath
if ([IO.Path]::GetDirectoryName($root) -ne $parent -or ![IO.Path]::GetFileName($root).StartsWith('run-') -or ((Get-Item -LiteralPath $root).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Native frontend root escaped exact QA scope.' }
$node = 'C:\nvm4w\nodejs\node.exe'
$expectedChrome = 'b0123437c55a3893e8988328f576ffcbe68cee7743d3653ffe865c73633b1ef4'
$receiptPath = Join-Path $root 'qa-host-receipt.json'
$preparedPath = Join-Path $root 'qa-frontend-prepared.json'
function Hash-QAFile([string]$path) { return (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant() }
function Write-QAJson([string]$path,$value) { [IO.File]::WriteAllText($path,($value | ConvertTo-Json -Depth 6),[Text.UTF8Encoding]::new($false)) }
if (!$Verify) {
    if ($ApprovedHostName -notmatch '^com\.akubrowser\.qa_reader_[a-f0-9]{32}$') { throw 'Preparation requires the explicitly approved QA hostname, never a newly generated host.' }
    if (Test-Path -LiteralPath $preparedPath) { throw 'Native preparation is not replayable.' }
    & $node --test (Join-Path $PSScriptRoot 'auth-session-fixture\chrome-window-lifecycle.test.mjs')
    if ($LASTEXITCODE -ne 0) { throw 'Fixture window inventory/closure contracts failed.' }
    $payload = Join-Path $project 'runtime\chromium'
    if (!(Test-Path -LiteralPath $node -PathType Leaf) -or (Hash-QAFile (Join-Path $payload 'bin\chrome.exe')) -ne $expectedChrome) { throw 'Pinned local Node/CfT payload unavailable or changed.' }
    New-Item -ItemType Directory -Path (Join-Path $root 'chromium') | Out-Null
    Copy-Item -LiteralPath (Join-Path $payload 'bin') -Destination (Join-Path $root 'chromium\bin') -Recurse
    Copy-Item -LiteralPath (Join-Path $payload 'pin.json') -Destination (Join-Path $root 'chromium\pin.json')
    & go build -o (Join-Path $root 'aku-sidecar-default-check.exe') ./cmd/akusidecar
    if ($LASTEXITCODE -ne 0) { throw 'Default main build failed.' }
    & go build -tags auth_session_dev -o (Join-Path $root 'aku-sidecar-dev-unavailable.exe') ./cmd/akusidecar
    if ($LASTEXITCODE -ne 0) { throw 'Dev without adapter build failed.' }
    & go build -tags 'auth_session_dev auth_session_fixture' -o (Join-Path $root 'akusidecar.exe') ./cmd/akusidecar
    if ($LASTEXITCODE -ne 0) { throw 'Actual native frontend main build failed.' }
    & go build -tags auth_session_fixture -o (Join-Path $root 'aku-reader-broker.exe') ./cmd/aku-reader-broker
    if ($LASTEXITCODE -ne 0) { throw 'Unchanged private-transport native helper build failed.' }
    & go test -c -tags 'auth_session_dev auth_session_fixture' -o (Join-Path $root 'engine.test.exe') ./internal/engine
    if ($LASTEXITCODE -ne 0) { throw 'Scoped actual-main controller build failed.' }
    $hostName = $ApprovedHostName
    $manifestPath = Join-Path $root ($hostName + '.json')
    $helperPath = Join-Path $root 'aku-reader-broker.exe'
    $mainPath = Join-Path $root 'akusidecar.exe'
    Write-QAJson $manifestPath ([ordered]@{name=$hostName;description='AkuBrowser isolated synthetic native frontend QA';path=$helperPath;type='stdio';allowed_origins=@('chrome-extension://dlibmmlopdahibfniinemhnghlifiple/')})
    Write-QAJson $receiptPath ([ordered]@{TaskRoot=$root;HostName=$hostName;ManifestPath=$manifestPath;ManifestSHA256=(Hash-QAFile $manifestPath);HelperPath=$helperPath;HelperSHA256=(Hash-QAFile $helperPath);MainPath=$mainPath;MainSHA256=(Hash-QAFile $mainPath);Pipe=('\\.\pipe\AkuBrowser.fixture-reader.' + [guid]::NewGuid().ToString('N'));RegistryPath=('HKCU\Software\Google\Chrome\NativeMessagingHosts\' + $hostName);RegistryView='Registry64';DefaultValue=$manifestPath})
}
if (!(Test-Path -LiteralPath $receiptPath -PathType Leaf)) { throw 'Native staging receipt missing.' }
$receipt = Get-Content -LiteralPath $receiptPath -Raw | ConvertFrom-Json
$env:AKU_CHROME_FIXTURE_ROOT = $root
$env:AKU_CHROME_FIXTURE_CLEANUP_MARKER = Join-Path $root 'cleanup-verified.marker'
$env:AKU_TEST_NODE_PATH = $node
$env:AKU_TEST_CHROME_PATH = Join-Path $root 'chromium\bin\chrome.exe'
$env:AKU_DEV_FRONTEND_PROJECT = $project
$env:AKU_DEV_FRONTEND_ENABLED = '1'
$env:AKU_DEV_FRONTEND_NATIVE_ENABLED = '1'
$env:AKU_DEV_FRONTEND_NATIVE_RECEIPT = $receiptPath
$env:AKU_DEV_FRONTEND_NATIVE_SCENARIO = $Scenario
$env:AKU_DEV_FRONTEND_HEADLESS = '0'
$env:AKU_BROKER_FIXTURE_PIPE = $receipt.Pipe
$env:AKU_DEV_BROKER_ENABLED = $null
$env:AKU_DEV_SESSION_TEST_ROOT = $root
if (!$Verify) {
    & go test -tags 'auth_session_dev auth_session_fixture' ./cmd/akusidecar ./internal/readerbroker ./internal/httpapi ./internal/engine ./internal/appshell ./internal/store -run '^TestDevelopmentFrontendNative(Staging|CLI|Observed|Seed|PauseEvidence|Window|Scenario|Start)|^TestDevelopmentFrontend(Staging|Presentation|LiveReader)|^TestDevelopmentLiveReader|^TestFixtureLiveReaderCommandEvidence|^TestFixtureNativeHost|^TestDevelopmentSessionDrainCaptureAndReasoningBeforeReader$' -count=1 -timeout=60s -v
    if ($LASTEXITCODE -ne 0) { throw 'Native configured-source/manifest/CLI read-only contracts failed.' }
    Push-Location -LiteralPath (Join-Path $project 'internal\engine')
    try { & (Join-Path $root 'engine.test.exe') '-test.run=^TestDevelopmentFrontendNativeUnregisteredStartup$' '-test.count=1' '-test.timeout=60s' '-test.v=true' }
    finally { Pop-Location }
    if ($LASTEXITCODE -ne 0) { throw 'Actual main did not reject unregistered frame before Node/Chrome/state.' }
    & (Join-Path $PSScriptRoot 'qa-reader-host.ps1') -ReceiptPath $receiptPath
    if ($LASTEXITCODE -ne 0) { throw 'Read-only registrar preview failed.' }
    Write-QAJson $preparedPath ([ordered]@{Stage='PREPARED';TaskRoot=$root;Scenario=$Scenario;ReceiptSHA256=(Hash-QAFile $receiptPath);ControllerSHA256=(Hash-QAFile (Join-Path $root 'engine.test.exe'));ChromeSHA256=(Hash-QAFile $env:AKU_TEST_CHROME_PATH);NativeHostRegistered=$false;VisibleUILaunched=$false;SourceInputs=@(
        @{Path=(Join-Path $project 'ui-reader-broker\content.js');SHA256=(Hash-QAFile (Join-Path $project 'ui-reader-broker\content.js'))},
        @{Path=(Join-Path $project 'ui-reader-broker\service-worker.js');SHA256=(Hash-QAFile (Join-Path $project 'ui-reader-broker\service-worker.js'))},
        @{Path=(Join-Path $project 'ui-reader-broker\manifest.json');SHA256=(Hash-QAFile (Join-Path $project 'ui-reader-broker\manifest.json'))},
        @{Path=(Join-Path $project 'scripts\auth-session-fixture\chrome-engine-driver.mjs');SHA256=(Hash-QAFile (Join-Path $project 'scripts\auth-session-fixture\chrome-engine-driver.mjs'))}
        @{Path=(Join-Path $project 'scripts\auth-session-fixture\chrome-window-lifecycle.mjs');SHA256=(Hash-QAFile (Join-Path $project 'scripts\auth-session-fixture\chrome-window-lifecycle.mjs'))},
        @{Path=(Join-Path $project 'cmd\akusidecar\development_frontend_native_windows.go');SHA256=(Hash-QAFile (Join-Path $project 'cmd\akusidecar\development_frontend_native_windows.go'))},
        @{Path=(Join-Path $project 'cmd\akusidecar\development_frontend_lifetime_windows.go');SHA256=(Hash-QAFile (Join-Path $project 'cmd\akusidecar\development_frontend_lifetime_windows.go'))},
        @{Path=(Join-Path $project 'cmd\akusidecar\development_frontend_windows.go');SHA256=(Hash-QAFile (Join-Path $project 'cmd\akusidecar\development_frontend_windows.go'))},
        @{Path=(Join-Path $project 'internal\appshell\development_frontend_windows.go');SHA256=(Hash-QAFile (Join-Path $project 'internal\appshell\development_frontend_windows.go'))},
        @{Path=(Join-Path $project 'cmd\akusidecar\development_frontend_native_scenario_windows_test.go');SHA256=(Hash-QAFile (Join-Path $project 'cmd\akusidecar\development_frontend_native_scenario_windows_test.go'))},
        @{Path=(Join-Path $project 'internal\engine\development_process_windows.go');SHA256=(Hash-QAFile (Join-Path $project 'internal\engine\development_process_windows.go'))},
        @{Path=(Join-Path $project 'internal\engine\development_frontend_native_windows_test.go');SHA256=(Hash-QAFile (Join-Path $project 'internal\engine\development_frontend_native_windows_test.go'))},
        @{Path=(Join-Path $project 'internal\engine\development_frontend_native_scenario_windows_test.go');SHA256=(Hash-QAFile (Join-Path $project 'internal\engine\development_frontend_native_scenario_windows_test.go'))},
        @{Path=(Join-Path $project 'internal\engine\development_live_reader_queue_windows.go');SHA256=(Hash-QAFile (Join-Path $project 'internal\engine\development_live_reader_queue_windows.go'))},
        @{Path=(Join-Path $project 'internal\engine\development_live_reader_queue_windows_test.go');SHA256=(Hash-QAFile (Join-Path $project 'internal\engine\development_live_reader_queue_windows_test.go'))},
        @{Path=(Join-Path $project 'internal\store\fixture_command_evidence.go');SHA256=(Hash-QAFile (Join-Path $project 'internal\store\fixture_command_evidence.go'))},
        @{Path=(Join-Path $project 'internal\store\fixture_command_evidence_test.go');SHA256=(Hash-QAFile (Join-Path $project 'internal\store\fixture_command_evidence_test.go'))},
        @{Path=(Join-Path $project 'scripts\test-development-frontend-native.ps1');SHA256=(Hash-QAFile (Join-Path $project 'scripts\test-development-frontend-native.ps1'))}
    )})
    Write-Output ('NATIVE_PREPARED scenario=' + $Scenario + ' receipt=' + $receiptPath + ' host=' + $receipt.HostName + ' manifest_sha256=' + $receipt.ManifestSHA256 + ' helper_sha256=' + $receipt.HelperSHA256 + ' main_sha256=' + $receipt.MainSHA256 + ' chrome_sha256=' + $expectedChrome + ' Registry64=true actual_native_call_configured=true registry_written=false visible_ui_launched=false')
    return
}
$prepared = Get-Content -LiteralPath $preparedPath -Raw | ConvertFrom-Json
if ($prepared.Stage -ne 'PREPARED' -or $prepared.TaskRoot -ne $root -or $prepared.Scenario -ne $Scenario -or $prepared.ReceiptSHA256 -ne (Hash-QAFile $receiptPath) -or $prepared.ControllerSHA256 -ne (Hash-QAFile (Join-Path $root 'engine.test.exe')) -or $prepared.ChromeSHA256 -ne $expectedChrome -or (Hash-QAFile $env:AKU_TEST_CHROME_PATH) -ne $expectedChrome) { throw 'Prepared native scenario/receipt/controller/payload changed or scenario mismatched.' }
foreach ($inputFile in $prepared.SourceInputs) { if ((Hash-QAFile $inputFile.Path) -ne $inputFile.SHA256) { throw 'Prepared native source inputs changed.' } }
if (!(Test-Path -LiteralPath (Join-Path $root 'qa-host-applied.json') -PathType Leaf)) { throw 'Coordinator-owned QA registration has not been applied; no UI launched.' }
$started = Join-Path $root 'native-physical.started.json'
if (Test-Path -LiteralPath $started) { throw 'Physical native attempt is not replayable; retain receipts for review.' }
$env:AKU_DEV_FRONTEND_CUE = Join-Path $root 'native-frontend-operator-cue.json'
Write-QAJson $started @{Stage='STARTED';TaskRoot=$root;Scenario=$Scenario;ReceiptSHA256=$prepared.ReceiptSHA256}
if (Test-Path -LiteralPath $env:AKU_CHROME_FIXTURE_CLEANUP_MARKER) { Remove-Item -LiteralPath $env:AKU_CHROME_FIXTURE_CLEANUP_MARKER }
Write-Output ('NATIVE_FRONTEND scenario=' + $Scenario + ' task_root=' + $root + ' cue=' + $env:AKU_DEV_FRONTEND_CUE + ' ready=' + $env:AKU_DEV_FRONTEND_CUE + '.ready.json; title="AkuBrowser QA - Synthetic native reader" target="Open source evidence" card="QA native frontend: synthetic reader"; native clocks unchanged')
Push-Location -LiteralPath (Join-Path $project 'internal\engine')
try {
    $testName = if ($Scenario -eq 'reader_open_parent_death') { '^TestDevelopmentFrontendReaderOpenParentDeathPhysicalNative$' } elseif ($Scenario -eq 'foreground_only') { '^TestDevelopmentFrontendForegroundOnlyPhysicalNative$' } else { '^TestDevelopmentFrontendActualMainPhysicalNative$' }
    & (Join-Path $root 'engine.test.exe') ('-test.run=' + $testName) '-test.count=1' '-test.timeout=12m' '-test.v=true'
}
finally { Pop-Location }
if ($LASTEXITCODE -ne 0) { throw ('Actual frontend physical/native scenario failed: ' + $Scenario + '; no automatic retry. Native scope retained for owned registry cleanup.') }
if (!(Test-Path -LiteralPath $env:AKU_CHROME_FIXTURE_CLEANUP_MARKER -PathType Leaf)) { throw 'Native frontend owned process tree emptiness not proved.' }
Write-Output ('Actual frontend native QA scenario passed exit=0 scenario=' + $Scenario + '; exact QA registration and retained artifacts require Coordinator cleanup in that order.')

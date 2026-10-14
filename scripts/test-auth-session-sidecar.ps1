param([switch]$VerifyDefault,[switch]$VerifyChrome,[switch]$VerifyBroker,[switch]$VerifyProcessRestart,[switch]$VerifyDevelopment,[switch]$PrepareDevelopmentBroker,[switch]$VerifyDevelopmentBroker,[string]$DevelopmentBrokerTaskRoot = '',[string]$ChromeTestRegex = '^TestChromeFixture',[string]$BrokerTestRegex = '^TestBrokerFixtureOSReaderHandoff$',[string]$ProcessTestRegex = '^TestProcessFixture')
$ErrorActionPreference = 'Stop'
if ($PrepareDevelopmentBroker -and $VerifyDevelopmentBroker) { throw 'Preparation and physical broker acceptance are separate invocations.' }
if ($VerifyChrome -and $ChromeTestRegex -notmatch '^[A-Za-z0-9_.$|()^]+$') { throw 'Chrome test filter contains unsupported characters.' }
if ($VerifyBroker -and $BrokerTestRegex -notmatch '^[A-Za-z0-9_.$|()^]+$') { throw 'Broker test filter contains unsupported characters.' }
if ($VerifyProcessRestart -and $ProcessTestRegex -notmatch '^[A-Za-z0-9_.$|()^]+$') { throw 'Process test filter contains unsupported characters.' }
$projectRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$fixtureParent = Join-Path $projectRoot 'runtime\qa\auth-session-sidecar'
New-Item -ItemType Directory -Path $fixtureParent -Force | Out-Null
$fixtureParent = (Resolve-Path -LiteralPath $fixtureParent).ProviderPath
if ($fixtureParent -ne (Join-Path $projectRoot 'runtime\qa\auth-session-sidecar')) { throw 'Fixture parent escaped project scope.' }
$taskRoot = Join-Path $fixtureParent ('run-' + [guid]::NewGuid().ToString('N'))
if ($DevelopmentBrokerTaskRoot -ne '') {
    $requestedRoot = [IO.Path]::GetFullPath($DevelopmentBrokerTaskRoot)
    if ([IO.Path]::GetDirectoryName($requestedRoot) -ne $fixtureParent -or ![IO.Path]::GetFileName($requestedRoot).StartsWith('run-') -or (Test-Path -LiteralPath $requestedRoot)) { throw 'Development broker task root must be a fresh direct run- directory in project QA.' }
    $taskRoot = $requestedRoot
}
New-Item -ItemType Directory -Path $taskRoot | Out-Null
$previous = @{}
$variables = @('TEMP','TMP','GOTMPDIR','GOPROXY','GOSUMDB','GOTOOLCHAIN','CGO_ENABLED','AKU_CHROME_FIXTURE_ROOT','AKU_CHROME_FIXTURE_CLEANUP_MARKER','AKU_TEST_NODE_PATH','AKU_TEST_CHROME_PATH','AKU_BROKER_FIXTURE_ENABLED','AKU_BROKER_FIXTURE_PIPE','AKU_PROCESS_FIXTURE_ENABLED','AKU_DEV_SESSION_TEST_ROOT','AKU_DEV_BROKER_ENABLED','AKU_DEV_BROKER_CUE')
foreach ($variable in $variables) { $previous[$variable] = [Environment]::GetEnvironmentVariable($variable, 'Process') }
Push-Location -LiteralPath $projectRoot
try {
    $env:TEMP = $taskRoot
    $env:TMP = $taskRoot
    $env:GOTMPDIR = $taskRoot
    $env:GOPROXY = 'off'
    $env:GOSUMDB = 'off'
    $env:GOTOOLCHAIN = 'local'
    $env:CGO_ENABLED = '0'
    $env:AKU_DEV_BROKER_ENABLED = $null
    & go test -tags auth_session_fixture ./internal/engine ./internal/store ./internal/httpapi -run 'TestSyntheticExclusive|TestFixtureCapture' -count=1 -timeout=60s
    if ($LASTEXITCODE -ne 0) { throw 'Sidecar synthetic integration tests failed.' }
    & go test -tags auth_session_fixture ./internal/readerbroker ./cmd/aku-reader-broker ./cmd/auth-session-fixture-ui -count=1 -timeout=60s
    if ($LASTEXITCODE -ne 0) { throw 'Reader broker contract or fixture helper compile checks failed.' }
    if ($VerifyChrome -or $VerifyBroker -or $VerifyProcessRestart -or $VerifyDevelopment -or $VerifyDevelopmentBroker -or $PrepareDevelopmentBroker) {
        $node = 'C:\nvm4w\nodejs\node.exe'
        $chrome = 'C:\Program Files\Google\Chrome\Application\chrome.exe'
        if (!(Test-Path -LiteralPath $node -PathType Leaf)) { throw 'The configured Node executable is unavailable.' }
        if (!(Test-Path -LiteralPath $chrome -PathType Leaf)) { throw 'The configured Chrome executable is unavailable.' }
        $env:AKU_CHROME_FIXTURE_ROOT = $taskRoot
        $env:AKU_CHROME_FIXTURE_CLEANUP_MARKER = Join-Path $taskRoot 'cleanup-verified.marker'
        $env:AKU_TEST_NODE_PATH = $node
        $env:AKU_TEST_CHROME_PATH = $chrome
        if ($PrepareDevelopmentBroker -or $VerifyDevelopmentBroker) {
            & go build -tags 'auth_session_dev auth_session_fixture' -o (Join-Path $taskRoot 'akusidecar.exe') ./cmd/akusidecar
            if ($LASTEXITCODE -ne 0) { throw 'Actual development main broker composition build failed.' }
            & go build -tags auth_session_fixture -o (Join-Path $taskRoot 'aku-reader-broker.exe') ./cmd/aku-reader-broker
            if ($LASTEXITCODE -ne 0) { throw 'Unchanged isolated native helper build failed.' }
            & go build -tags auth_session_fixture -o (Join-Path $taskRoot 'aku-fixture-ui.exe') ./cmd/auth-session-fixture-ui
            if ($LASTEXITCODE -ne 0) { throw 'Owned development UI build failed.' }
            if ($PrepareDevelopmentBroker) {
                & go test -tags 'auth_session_dev auth_session_fixture' ./internal/engine ./cmd/auth-session-fixture-ui ./cmd/akusidecar -run '^TestDevelopmentBrokerStaged|^TestDevelopmentActionHelper|^TestDevelopmentCollectionRuntime' -count=1 -timeout=60s -v
                if ($LASTEXITCODE -ne 0) { throw 'Non-foreground development broker preparation contracts failed.' }
                Write-Output 'Development broker preparation passed. No UI or physical native broker test was launched.'
            }
            if ($VerifyDevelopmentBroker) {
                $env:AKU_PROCESS_FIXTURE_ENABLED = '1'
                $env:AKU_DEV_BROKER_ENABLED = '1'
                $env:AKU_BROKER_FIXTURE_PIPE = '\\.\pipe\AkuBrowser.fixture-reader.' + [guid]::NewGuid().ToString('N')
                $env:AKU_DEV_BROKER_CUE = Join-Path $taskRoot 'operator-cue.json'
                Write-Output ('DEV3 task_root=' + $taskRoot + ' cue=' + $env:AKU_DEV_BROKER_CUE + ' ready=' + $env:AKU_DEV_BROKER_CUE + '.ready.json')
                & go test -tags 'auth_session_dev auth_session_fixture' ./internal/engine -run '^TestDevelopmentSessionActualMainBrokerFirstPhysicalOpen$' -count=1 -timeout=12m -v
                if ($LASTEXITCODE -ne 0) { throw 'Actual main physical UI/native broker acceptance failed; no retry or fallback counts as acceptance.' }
                if (!(Test-Path -LiteralPath $env:AKU_CHROME_FIXTURE_CLEANUP_MARKER -PathType Leaf)) { throw 'Development broker did not prove all owned trees empty.' }
                $env:AKU_DEV_BROKER_ENABLED = $null
            }
        }
        if ($VerifyDevelopment) {
            $env:AKU_DEV_SESSION_TEST_ROOT = $taskRoot
            $env:AKU_PROCESS_FIXTURE_ENABLED = '1'
            & go build -o (Join-Path $taskRoot 'aku-sidecar-default-check.exe') ./cmd/akusidecar
            if ($LASTEXITCODE -ne 0) { throw 'Default main build failed.' }
            & go build -tags auth_session_dev -o (Join-Path $taskRoot 'aku-sidecar-dev-unavailable.exe') ./cmd/akusidecar
            if ($LASTEXITCODE -ne 0) { throw 'Dev without adapter build failed.' }
            & go build -tags 'auth_session_dev auth_session_fixture' -o (Join-Path $taskRoot 'aku-sidecar-dev.exe') ./cmd/akusidecar
            if ($LASTEXITCODE -ne 0) { throw 'Actual main synthetic integration build failed.' }
            & go test -tags 'auth_session_dev auth_session_fixture' ./cmd/akusidecar ./internal/engine -run '^TestDevelopmentSession' -count=1 -timeout=120s -v
            if ($LASTEXITCODE -ne 0) { throw 'Explicit development integration validation failed.' }
            if (!(Test-Path -LiteralPath $env:AKU_CHROME_FIXTURE_CLEANUP_MARKER -PathType Leaf)) { throw 'Development integration did not prove all owned trees empty.' }
        }
        if ($VerifyChrome) {
            & go test -tags auth_session_fixture ./internal/engine -run $ChromeTestRegex -count=1 -timeout=300s
            if ($LASTEXITCODE -ne 0) { throw 'Opt-in real Chrome fixture integration test failed.' }
        }
        if ($VerifyProcessRestart) {
            if (Test-Path -LiteralPath $env:AKU_CHROME_FIXTURE_CLEANUP_MARKER -PathType Leaf) { Remove-Item -LiteralPath $env:AKU_CHROME_FIXTURE_CLEANUP_MARKER }
            $env:AKU_PROCESS_FIXTURE_ENABLED = '1'
            & go build -tags auth_session_fixture -o (Join-Path $taskRoot 'auth-session-process-fixture.exe') ./cmd/auth-session-process-fixture
            if ($LASTEXITCODE -ne 0) { throw 'Dedicated process fixture build failed.' }
            & go test -tags auth_session_fixture ./internal/engine -run $ProcessTestRegex -count=1 -timeout=240s -v
            if ($LASTEXITCODE -ne 0) { throw 'Opt-in full OS process restart fixture failed.' }
            if (!(Test-Path -LiteralPath $env:AKU_CHROME_FIXTURE_CLEANUP_MARKER -PathType Leaf)) { throw 'Process fixture did not prove all owned trees empty.' }
        }
        if ($VerifyBroker) {
            if (Test-Path -LiteralPath $env:AKU_CHROME_FIXTURE_CLEANUP_MARKER -PathType Leaf) { Remove-Item -LiteralPath $env:AKU_CHROME_FIXTURE_CLEANUP_MARKER }
            $env:AKU_BROKER_FIXTURE_ENABLED = '1'
            $env:AKU_BROKER_FIXTURE_PIPE = '\\.\pipe\AkuBrowser.fixture-reader.' + [guid]::NewGuid().ToString('N')
            & go build -tags auth_session_fixture -o (Join-Path $taskRoot 'aku-reader-broker.exe') ./cmd/aku-reader-broker
            if ($LASTEXITCODE -ne 0) { throw 'Isolated native broker helper build failed.' }
            Copy-Item -LiteralPath (Join-Path $taskRoot 'aku-reader-broker.exe') -Destination (Join-Path $taskRoot 'wrong-helper.exe')
            & go build -tags auth_session_fixture -o (Join-Path $taskRoot 'aku-fixture-ui.exe') ./cmd/auth-session-fixture-ui
            if ($LASTEXITCODE -ne 0) { throw 'Isolated native fixture UI build failed.' }
            & go test -c -tags auth_session_fixture -o (Join-Path $taskRoot 'aku-sidecar.exe') ./internal/engine
            if ($LASTEXITCODE -ne 0) { throw 'Isolated broker fixture server build failed.' }
            $brokerArguments = @(('-test.run=' + $BrokerTestRegex), '-test.count=1', '-test.timeout=240s', '-test.v=true')
            Push-Location -LiteralPath (Join-Path $projectRoot 'internal\engine')
            try { & (Join-Path $taskRoot 'aku-sidecar.exe') @brokerArguments }
            finally { Pop-Location }
            if ($LASTEXITCODE -ne 0) { throw 'Opt-in OS reader broker fixture integration test failed.' }
            if (!(Test-Path -LiteralPath $env:AKU_CHROME_FIXTURE_CLEANUP_MARKER -PathType Leaf)) { throw 'Broker fixture did not run or did not verify all owned process trees empty.' }
            & go test ./internal/readerbroker -count=1 -timeout=60s
            if ($LASTEXITCODE -ne 0) { throw 'Native broker transport and activation contract regressions failed.' }
            & go test ./internal/appshell -run 'TestReader|TestNativeReader|TestBroker' -count=1 -timeout=60s
            if ($LASTEXITCODE -ne 0) { throw 'Native broker contract regressions failed.' }
        }
    }
    if ($VerifyDefault) {
        & go test ./internal/config ./cmd/akusidecar -count=1 -timeout=90s
        if ($LASTEXITCODE -ne 0) { throw 'Default config/main startup regressions failed.' }
        & go test ./internal/captureruntime ./internal/collection ./internal/engine ./internal/store ./internal/httpapi -run 'Test.*(CaptureRuntime|CaptureGeneration|Hybrid|NativeReader|HeadlessNative|SwitchWaits|RetainedPopup|Collector|ClaimCommand|Borrow|IdleHold|Manager|Replacement|AdditionalHeadless|SplitCapture)' -count=1 -timeout=120s
        if ($LASTEXITCODE -ne 0) { throw 'Default capture/reader regression tests failed.' }
        & go build -o (Join-Path $taskRoot 'aku-sidecar-default-check.exe') ./cmd/akusidecar
        if ($LASTEXITCODE -ne 0) { throw 'Default Sidecar build failed.' }
    }
    Write-Output 'auth-session-sidecar validation completed exit=0'
}
finally {
    Pop-Location
    foreach ($variable in $variables) { [Environment]::SetEnvironmentVariable($variable, $previous[$variable], 'Process') }
    $cleanupAllowed = !($VerifyChrome -or $VerifyBroker -or $VerifyProcessRestart -or $VerifyDevelopment -or $VerifyDevelopmentBroker) -or (Test-Path -LiteralPath (Join-Path $taskRoot 'cleanup-verified.marker') -PathType Leaf)
    if (!$cleanupAllowed) { Write-Warning 'The Chrome fixture did not prove its owned Job Object empty; fixture output was retained.' }
    if ($cleanupAllowed -and (Test-Path -LiteralPath $taskRoot)) {
        $resolved = (Resolve-Path -LiteralPath $taskRoot).ProviderPath
        $item = Get-Item -LiteralPath $resolved
        if ($resolved -ne $taskRoot -or !$resolved.StartsWith($fixtureParent + '\', [StringComparison]::OrdinalIgnoreCase) -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Refusing fixture cleanup outside the verified output scope.' }
        try { Remove-Item -LiteralPath $resolved -Recurse -Force -ErrorAction Stop }
        catch { Write-Warning 'Fixture files remain locked; no process was stopped or forced closed.' }
    }
}

param([switch]$VerifyDefault,[switch]$VerifyChrome,[switch]$VerifyBroker,[switch]$VerifyProcessRestart,[string]$ChromeTestRegex = '^TestChromeFixture',[string]$BrokerTestRegex = '^TestBrokerFixtureOSReaderHandoff$',[string]$ProcessTestRegex = '^TestProcessFixture')
$ErrorActionPreference = 'Stop'
if ($VerifyChrome -and $ChromeTestRegex -notmatch '^[A-Za-z0-9_.$|()^]+$') { throw 'Chrome test filter contains unsupported characters.' }
if ($VerifyBroker -and $BrokerTestRegex -notmatch '^[A-Za-z0-9_.$|()^]+$') { throw 'Broker test filter contains unsupported characters.' }
if ($VerifyProcessRestart -and $ProcessTestRegex -notmatch '^[A-Za-z0-9_.$|()^]+$') { throw 'Process test filter contains unsupported characters.' }
$projectRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$fixtureParent = Join-Path $projectRoot 'runtime\qa\auth-session-sidecar'
New-Item -ItemType Directory -Path $fixtureParent -Force | Out-Null
$fixtureParent = (Resolve-Path -LiteralPath $fixtureParent).ProviderPath
if ($fixtureParent -ne (Join-Path $projectRoot 'runtime\qa\auth-session-sidecar')) { throw 'Fixture parent escaped project scope.' }
$taskRoot = Join-Path $fixtureParent ('run-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $taskRoot | Out-Null
$previous = @{}
$variables = @('TEMP','TMP','GOTMPDIR','GOPROXY','GOSUMDB','GOTOOLCHAIN','CGO_ENABLED','AKU_CHROME_FIXTURE_ROOT','AKU_CHROME_FIXTURE_CLEANUP_MARKER','AKU_TEST_NODE_PATH','AKU_TEST_CHROME_PATH','AKU_BROKER_FIXTURE_ENABLED','AKU_BROKER_FIXTURE_PIPE','AKU_PROCESS_FIXTURE_ENABLED')
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
    & go test -tags auth_session_fixture ./internal/engine ./internal/store ./internal/httpapi -run 'TestSyntheticExclusive|TestFixtureCapture' -count=1 -timeout=60s
    if ($LASTEXITCODE -ne 0) { throw 'Sidecar synthetic integration tests failed.' }
    & go test -tags auth_session_fixture ./internal/readerbroker ./cmd/aku-reader-broker ./cmd/auth-session-fixture-ui -count=1 -timeout=60s
    if ($LASTEXITCODE -ne 0) { throw 'Reader broker contract or fixture helper compile checks failed.' }
    if ($VerifyChrome -or $VerifyBroker -or $VerifyProcessRestart) {
        $node = 'C:\nvm4w\nodejs\node.exe'
        $chrome = 'C:\Program Files\Google\Chrome\Application\chrome.exe'
        if (!(Test-Path -LiteralPath $node -PathType Leaf)) { throw 'The configured Node executable is unavailable.' }
        if (!(Test-Path -LiteralPath $chrome -PathType Leaf)) { throw 'The configured Chrome executable is unavailable.' }
        $env:AKU_CHROME_FIXTURE_ROOT = $taskRoot
        $env:AKU_CHROME_FIXTURE_CLEANUP_MARKER = Join-Path $taskRoot 'cleanup-verified.marker'
        $env:AKU_TEST_NODE_PATH = $node
        $env:AKU_TEST_CHROME_PATH = $chrome
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
        & go test ./internal/captureruntime ./internal/collection ./internal/engine ./internal/store ./internal/httpapi -run 'Test.*(CaptureRuntime|CaptureGeneration|Hybrid|NativeReader|HeadlessNative|SwitchWaits|RetainedPopup|Collector|ClaimCommand|Borrow|IdleHold|Manager|Replacement|AdditionalHeadless|SplitCapture)' -count=1 -timeout=120s
        if ($LASTEXITCODE -ne 0) { throw 'Default capture/reader regression tests failed.' }
        & go build -o (Join-Path $taskRoot 'aku-sidecar-default-check.exe') ./cmd/akusidecar
        if ($LASTEXITCODE -ne 0) { throw 'Default Sidecar build failed.' }
    }
}
finally {
    Pop-Location
    foreach ($variable in $variables) { [Environment]::SetEnvironmentVariable($variable, $previous[$variable], 'Process') }
    $cleanupAllowed = !($VerifyChrome -or $VerifyBroker -or $VerifyProcessRestart) -or (Test-Path -LiteralPath (Join-Path $taskRoot 'cleanup-verified.marker') -PathType Leaf)
    if (!$cleanupAllowed) { Write-Warning 'The Chrome fixture did not prove its owned Job Object empty; fixture output was retained.' }
    if ($cleanupAllowed -and (Test-Path -LiteralPath $taskRoot)) {
        $resolved = (Resolve-Path -LiteralPath $taskRoot).ProviderPath
        $item = Get-Item -LiteralPath $resolved
        if ($resolved -ne $taskRoot -or !$resolved.StartsWith($fixtureParent + '\', [StringComparison]::OrdinalIgnoreCase) -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Refusing fixture cleanup outside the verified output scope.' }
        try { Remove-Item -LiteralPath $resolved -Recurse -Force -ErrorAction Stop }
        catch { Write-Warning 'Fixture files remain locked; no process was stopped or forced closed.' }
    }
}

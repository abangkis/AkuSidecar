param()
$ErrorActionPreference = 'Stop'
$projectRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$qaParent = Join-Path $projectRoot 'runtime\qa\auth-session-sidecar'
foreach ($relative in @('runtime','runtime\qa','runtime\qa\auth-session-sidecar','.go-build','.go-build\x-session-verifier','.go-build\mod')) {
    $directory = Join-Path $projectRoot $relative
    if (!(Test-Path -LiteralPath $directory -PathType Container)) { New-Item -ItemType Directory -Path $directory | Out-Null }
    if ((Get-Item -LiteralPath $directory).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Native capture offline QA paths must be ordinary project directories.' }
}
$node = Join-Path $projectRoot 'runtime\dev\headless-worker\node.exe'
if (!(Test-Path -LiteralPath $node -PathType Leaf)) { throw 'Project Node executable unavailable.' }
$runId = [guid]::NewGuid().ToString('N')
$taskRoot = Join-Path $qaParent ('run-x-manual-native-capture-' + $runId)
New-Item -ItemType Directory -Path $taskRoot | Out-Null
$log = Join-Path $qaParent ('x-manual-native-capture-' + $runId + '.log.txt')
$variables = @('TEMP','TMP','GOTMPDIR','GOCACHE','GOMODCACHE','GOWORK','GOPROXY','GOSUMDB','GOTOOLCHAIN','CGO_ENABLED','GOFLAGS',
    'AKU_X_NATIVE_MANUAL_CAPTURE_OFFLINE','AKU_NATIVE_MANUAL_PROJECT_ROOT','AKU_CHROME_FIXTURE_NODE',
    'AKU_X_NATIVE_MANUAL_STARTUP_ALLOWED','AKU_X_NATIVE_MANUAL_OWNER_OFFLINE','AKU_NATIVE_MANUAL_CHROME_FIXTURE','AKU_MANUAL_CHROME_STORE_ENABLED','AKU_X_NATIVE_MANUAL_CAPTURE_ALLOWED')
$previous = @{}
foreach ($name in $variables) { $previous[$name] = [Environment]::GetEnvironmentVariable($name,'Process') }
$passed = $false
Push-Location -LiteralPath $projectRoot
try {
    $env:TEMP=$taskRoot; $env:TMP=$taskRoot; $env:GOTMPDIR=$taskRoot
    $env:GOCACHE=Join-Path $projectRoot '.go-build\x-session-verifier'; $env:GOMODCACHE=Join-Path $projectRoot '.go-build\mod'
    $env:GOWORK='off'; $env:GOPROXY='off'; $env:GOSUMDB='off'; $env:GOTOOLCHAIN='local'; $env:CGO_ENABLED='0'; $env:GOFLAGS=''
    $env:AKU_X_NATIVE_MANUAL_CAPTURE_OFFLINE='1'; $env:AKU_NATIVE_MANUAL_PROJECT_ROOT=$projectRoot; $env:AKU_CHROME_FIXTURE_NODE=$node
    $env:AKU_X_NATIVE_MANUAL_STARTUP_ALLOWED='0'; $env:AKU_X_NATIVE_MANUAL_OWNER_OFFLINE='0'
    $env:AKU_NATIVE_MANUAL_CHROME_FIXTURE='0'; $env:AKU_MANUAL_CHROME_STORE_ENABLED='0'; $env:AKU_X_NATIVE_MANUAL_CAPTURE_ALLOWED='0'
    Write-Output ('Offline native X capture -> Engine -> Store run: ' + $runId) | Tee-Object -FilePath $log
    & $node --test --test-timeout=15000 scripts/auth-session-fixture/x-capture-traffic-diagnostics.test.mjs scripts/auth-session-fixture/x-native-manual-capture.test.mjs scripts/auth-session-fixture/x-native-manual-owner-port.test.mjs 2>&1 | Tee-Object -FilePath $log -Append
    if ($LASTEXITCODE -ne 0) { throw 'Native capture Node offline regressions failed.' }
    & $node --test --test-timeout=15000 scripts/auth-session-fixture/x-native-metadata-probe-driver.test.mjs scripts/auth-session-fixture/x-qa-feasibility-collector.test.mjs internal/collection/headless/worker/test/worker.test.mjs internal/collection/headless/worker/test/x-capture-parity.test.mjs 2>&1 | Tee-Object -FilePath $log -Append
    if ($LASTEXITCODE -ne 0) { throw 'Existing worker/parser and private startup transport regressions failed.' }
    $production=@(& go list -tags 'auth_session_dev auth_session_fixture' -f '{{range .GoFiles}}{{println .}}{{end}}' ./internal/engine)
    if ($LASTEXITCODE -ne 0 -or $production.Count -eq 0) { throw 'Engine production source inventory unavailable.' }
    $engineRoot=Join-Path $projectRoot 'internal/engine'
    $sources=@($production | Where-Object { $_.Trim() -ne '' } | ForEach-Object { Join-Path $engineRoot $_ })
    $sources+=Join-Path $engineRoot 'development_native_pilot_store_test.go'
    $sources+=Join-Path $engineRoot 'development_native_pilot_x_capture_test.go'
    $sources+=Join-Path $engineRoot 'development_native_pilot_capture_diagnostic_test.go'
    & go test -tags 'auth_session_dev auth_session_fixture' -run '^TestNativePilotXCapture' -count=1 -timeout=90s -v @sources 2>&1 | Tee-Object -FilePath $log -Append
    if ($LASTEXITCODE -ne 0) { throw 'Native capture Engine/Store synthetic integration failed.' }
    # Compile the actual retained entry's safe failure reporting without running
    # its browser/profile/transfer recipe or the package-wide TestMain.
    $raw=& $node scripts/auth-session-fixture/stage-native-manual-chrome.mjs $taskRoot x-retained-capture
    if ($LASTEXITCODE -ne 0) { throw 'Retained capture diagnostic staging failed.' }
    $stage=$raw | ConvertFrom-Json
    $retainedSources=@($stage.files | ForEach-Object { Join-Path $stage.stage $_ })
    & go test -c -tags 'auth_session_dev auth_session_fixture' -o (Join-Path $taskRoot 'retained-capture-diagnostics.test.exe') @retainedSources 2>&1 | Tee-Object -FilePath $log -Append
    if ($LASTEXITCODE -ne 0) { throw 'Retained capture diagnostic entry compilation failed.' }
    & (Join-Path $taskRoot 'retained-capture-diagnostics.test.exe') '-test.run=^TestXManualCaptureDiagnostic(Code|Pipe)$' '-test.timeout=10s' '-test.v' 2>&1 | Tee-Object -FilePath $log -Append
    if ($LASTEXITCODE -ne 0) { throw 'Actual bridge sanitizer/diagnostic code regression failed.' }
    Write-Output 'PASS: retained capture entry compiled and pure diagnostic code test passed; no native execution.' | Tee-Object -FilePath $log -Append
    foreach ($mode in @('default','auth_session_dev','auth_session_fixture','auth_session_dev auth_session_fixture')) {
        $listArguments=@('list','-f','{{range .GoFiles}}{{println .}}{{end}}')
        if ($mode -ne 'default') { $listArguments+=@('-tags',$mode) }
        $files=@(& go @listArguments ./internal/engine)
        if ($LASTEXITCODE -ne 0) { throw 'Native capture tag inventory unavailable.' }
        if (($files -contains 'development_native_pilot_x_capture.go') -ne $mode.Contains('auth_session_dev')) { throw 'Native capture adapter tag boundary failed.' }
        Write-Output ('PASS: native capture tag gate ' + $mode) | Tee-Object -FilePath $log -Append
    }
    $passed=$true
    Write-Output 'PASS: native X capture composition with production worker/parser, fake CDP and synthetic databases only. No Chrome/profile, X network, session transfer or user runtime.' | Tee-Object -FilePath $log -Append
    Write-Output ('Evidence: ' + $log)
} finally {
    Pop-Location
    foreach ($name in $variables) { [Environment]::SetEnvironmentVariable($name,$previous[$name],'Process') }
    if ($passed -and (Test-Path -LiteralPath $taskRoot -PathType Container)) {
        $resolved=(Resolve-Path -LiteralPath $taskRoot).ProviderPath
        if ($resolved -ne $taskRoot -or [IO.Path]::GetDirectoryName($resolved) -ne $qaParent -or ((Get-Item -LiteralPath $resolved).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Refusing cleanup outside owned native capture QA root.' }
        if (Get-ChildItem -LiteralPath $resolved -Recurse -Force | Where-Object { $_.Attributes -band [IO.FileAttributes]::ReparsePoint }) { throw 'Refusing cleanup through a reparse point.' }
        Remove-Item -LiteralPath $resolved -Recurse -Force
        Write-Output 'Owned offline native capture databases and temporary artifacts removed.'
    } elseif (!$passed) { Write-Warning ('Offline QA artifacts retained: ' + $taskRoot) }
}

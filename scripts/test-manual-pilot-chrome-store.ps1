param([switch]$VerifyChrome)
$ErrorActionPreference = 'Stop'
$projectRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$qaParent = Join-Path $projectRoot 'runtime\qa\auth-session-sidecar'
foreach ($relative in @('runtime','runtime\qa','runtime\qa\auth-session-sidecar','.go-build','.go-build\x-session-verifier','.go-build\mod')) {
    $directory = Join-Path $projectRoot $relative
    if (!(Test-Path -LiteralPath $directory -PathType Container)) { New-Item -ItemType Directory -Path $directory | Out-Null }
    if ((Get-Item -LiteralPath $directory).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Chrome Store QA paths must be ordinary project directories.' }
}
$node = Join-Path $projectRoot 'runtime\dev\headless-worker\node.exe'
$chrome = 'C:\Program Files\Google\Chrome\Application\chrome.exe'
foreach ($image in @($node,$chrome)) { if (!(Test-Path -LiteralPath $image -PathType Leaf)) { throw 'Official QA runtime unavailable.' } }
$runId = [guid]::NewGuid().ToString('N')
$taskRoot = Join-Path $qaParent ('run-manual-pilot-chrome-store-' + $runId)
New-Item -ItemType Directory -Path $taskRoot | Out-Null
$log = Join-Path $qaParent ('manual-pilot-chrome-store-' + $runId + '.log.txt')
$marker = Join-Path $taskRoot 'cleanup.verified'
$variables = @('TEMP','TMP','GOTMPDIR','GOCACHE','GOMODCACHE','GOWORK','GOPROXY','GOSUMDB','GOTOOLCHAIN','CGO_ENABLED','GOFLAGS',
    'AKU_MANUAL_CHROME_STORE_ENABLED','AKU_NATIVE_MANUAL_PROJECT_ROOT','AKU_NATIVE_MANUAL_CHROME_ENABLED','AKU_NATIVE_MANUAL_CHROME_FIXTURE',
    'AKU_CHROME_FIXTURE_ROOT','AKU_CHROME_FIXTURE_CLEANUP_MARKER','AKU_CHROME_FIXTURE_NODE','AKU_CHROME_FIXTURE_EXE')
$previous = @{}
foreach ($name in $variables) { $previous[$name] = [Environment]::GetEnvironmentVariable($name,'Process') }
$canRemove = $false
Push-Location -LiteralPath $projectRoot
try {
    $env:TEMP=$taskRoot; $env:TMP=$taskRoot; $env:GOTMPDIR=$taskRoot
    $env:GOCACHE=Join-Path $projectRoot '.go-build\x-session-verifier'; $env:GOMODCACHE=Join-Path $projectRoot '.go-build\mod'
    $env:GOWORK='off'; $env:GOPROXY='off'; $env:GOSUMDB='off'; $env:GOTOOLCHAIN='local'; $env:CGO_ENABLED='0'; $env:GOFLAGS=''
    $env:AKU_MANUAL_CHROME_STORE_ENABLED='0'; $env:AKU_NATIVE_MANUAL_CHROME_ENABLED='0'; $env:AKU_NATIVE_MANUAL_CHROME_FIXTURE='0'
    $env:AKU_NATIVE_MANUAL_PROJECT_ROOT=$projectRoot; $env:AKU_CHROME_FIXTURE_ROOT=$taskRoot
    $env:AKU_CHROME_FIXTURE_CLEANUP_MARKER=$marker; $env:AKU_CHROME_FIXTURE_NODE=$node; $env:AKU_CHROME_FIXTURE_EXE=$chrome
    Write-Output ('Synthetic Chrome -> Engine -> Store run: ' + $runId) | Tee-Object -FilePath $log
    & $node --test scripts/auth-session-fixture/chrome-manual-pilot-driver.test.mjs scripts/auth-session-fixture/chrome-manual-pilot-capture.test.mjs 2>&1 | Tee-Object -FilePath $log -Append
    if ($LASTEXITCODE -ne 0) { throw 'Chrome capture boundary regressions failed.' }
    foreach ($mode in @('default','auth_session_dev','auth_session_fixture','auth_session_dev auth_session_fixture')) {
        $listArguments=@('list','-f','{{range .TestGoFiles}}{{println .}}{{end}}')
        if ($mode -ne 'default') { $listArguments+=@('-tags',$mode) }
        $files=@(& go @listArguments ./internal/engine)
        if ($LASTEXITCODE -ne 0) { throw 'Chrome Store build-tag inventory unavailable.' }
        if (($files -contains 'development_native_pilot_chrome_store_windows_test.go') -ne ($mode -eq 'auth_session_dev auth_session_fixture')) { throw 'Chrome Store test build-tag boundary failed.' }
        Write-Output ('PASS: Chrome Store tag gate ' + $mode) | Tee-Object -FilePath $log -Append
    }
    # All actual Engine production code, but only two explicitly selected test
    # files: local ownership helpers and this synthetic Chrome/Store acceptance.
    $production=@(& go list -tags 'auth_session_dev auth_session_fixture' -f '{{range .GoFiles}}{{println .}}{{end}}' ./internal/engine)
    if ($LASTEXITCODE -ne 0 -or $production.Count -eq 0) { throw 'Engine source inventory unavailable.' }
    $engineRoot=Join-Path $projectRoot 'internal/engine'
    $sources=@($production | Where-Object { $_.Trim() -ne '' } | ForEach-Object { Join-Path $engineRoot $_ })
    $sources+=Join-Path $engineRoot 'native_manual_pilot_chrome_windows_test.go'
    $sources+=Join-Path $engineRoot 'development_native_pilot_chrome_store_windows_test.go'
    $executable=Join-Path $taskRoot 'manual-pilot-chrome-store.test.exe'
    & go test -c -tags 'auth_session_dev auth_session_fixture' -o $executable @sources 2>&1 | Tee-Object -FilePath $log -Append
    if ($LASTEXITCODE -ne 0) { throw 'Actual Engine Chrome Store test compilation failed.' }
    $testArguments=@('-test.run=^TestNativePilotChromeStore$','-test.timeout=120s','-test.v')
    & $executable @testArguments 2>&1 | Tee-Object -FilePath $log -Append
    if ($LASTEXITCODE -ne 0) { throw 'Disabled Chrome Store entry failed.' }
    if (Test-Path -LiteralPath $marker) { throw 'Disabled entry executed Chrome ownership cleanup.' }
    Write-Output 'PASS: Chrome Store entry remains disabled without explicit process gates.' | Tee-Object -FilePath $log -Append
    if ($VerifyChrome) {
        $env:AKU_MANUAL_CHROME_STORE_ENABLED='1'; $env:AKU_NATIVE_MANUAL_CHROME_FIXTURE='1'
        & $executable @testArguments 2>&1 | Tee-Object -FilePath $log -Append
        $exitCode=$LASTEXITCODE
        $canRemove=(Test-Path -LiteralPath $marker -PathType Leaf) -and (Get-Content -LiteralPath $marker -Raw).Trim() -eq 'verified'
        if ($exitCode -ne 0) { throw 'Synthetic Chrome Engine Store acceptance failed.' }
        if (!$canRemove) { throw 'Owned Chrome/Node Job cleanup proof unavailable.' }
        Write-Output 'PASS: actual Chrome capture with synthetic login and fresh QA database; no native X or user-runtime activation.' | Tee-Object -FilePath $log -Append
    } else { $canRemove=$true; Write-Output 'Chrome execution not selected. Use -VerifyChrome for fresh-profile synthetic acceptance.' | Tee-Object -FilePath $log -Append }
    Write-Output ('Evidence: ' + $log)
} finally {
    Pop-Location
    foreach ($name in $variables) { [Environment]::SetEnvironmentVariable($name,$previous[$name],'Process') }
    if ($canRemove -and (Test-Path -LiteralPath $taskRoot -PathType Container)) {
        $resolved=(Resolve-Path -LiteralPath $taskRoot).ProviderPath
        if ($resolved -ne $taskRoot -or [IO.Path]::GetDirectoryName($resolved) -ne $qaParent -or ((Get-Item -LiteralPath $resolved).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Refusing cleanup outside owned Chrome Store QA root.' }
        if (Get-ChildItem -LiteralPath $resolved -Recurse -Force | Where-Object { $_.Attributes -band [IO.FileAttributes]::ReparsePoint }) { throw 'Refusing cleanup through a reparse point.' }
        Remove-Item -LiteralPath $resolved -Recurse -Force
        Write-Output 'Owned synthetic Chrome profiles, databases and test binary removed.'
    } else { Write-Warning ('QA artifacts retained: ' + $taskRoot) }
}

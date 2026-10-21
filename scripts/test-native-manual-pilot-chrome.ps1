param([switch]$VerifyChrome)
$ErrorActionPreference = 'Stop'
$projectRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$qaParent = Join-Path $projectRoot 'runtime\qa\auth-session-sidecar'
foreach ($relative in @('runtime', 'runtime\qa', 'runtime\qa\auth-session-sidecar', '.go-build', '.go-build\x-session-verifier', '.go-build\mod')) {
    $directory = Join-Path $projectRoot $relative
    if (!(Test-Path -LiteralPath $directory -PathType Container)) { New-Item -ItemType Directory -Path $directory | Out-Null }
    if ((Get-Item -LiteralPath $directory).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Chrome QA paths must not be reparse points.' }
}
$node = Join-Path $projectRoot 'runtime\dev\headless-worker\node.exe'
$chrome = 'C:\Program Files\Google\Chrome\Application\chrome.exe'
foreach ($path in @($node, $chrome)) { if (!(Test-Path -LiteralPath $path -PathType Leaf)) { throw 'Official QA executable unavailable.' } }
$runId = [guid]::NewGuid().ToString('N')
$taskRoot = Join-Path $qaParent ('run-native-manual-chrome-' + $runId)
New-Item -ItemType Directory -Path $taskRoot | Out-Null
$log = Join-Path $qaParent ('native-manual-chrome-' + $runId + '.log.txt')
$marker = Join-Path $taskRoot 'cleanup.verified'
$variables = @('TEMP','TMP','GOTMPDIR','GOCACHE','GOMODCACHE','GOWORK','GOPROXY','GOSUMDB','GOTOOLCHAIN','CGO_ENABLED','GOFLAGS',
    'AKU_NATIVE_MANUAL_PROJECT_ROOT','AKU_NATIVE_MANUAL_CHROME_ENABLED','AKU_NATIVE_MANUAL_CHROME_FIXTURE',
    'AKU_CHROME_FIXTURE_ROOT','AKU_CHROME_FIXTURE_CLEANUP_MARKER','AKU_CHROME_FIXTURE_NODE','AKU_CHROME_FIXTURE_EXE')
$previous = @{}
foreach ($name in $variables) { $previous[$name] = [Environment]::GetEnvironmentVariable($name, 'Process') }
$canRemove = $false
Push-Location -LiteralPath $projectRoot
try {
    $env:TEMP=$taskRoot; $env:TMP=$taskRoot; $env:GOTMPDIR=$taskRoot
    $env:GOCACHE=Join-Path $projectRoot '.go-build\x-session-verifier'; $env:GOMODCACHE=Join-Path $projectRoot '.go-build\mod'
    $env:GOWORK='off'; $env:GOPROXY='off'; $env:GOSUMDB='off'; $env:GOTOOLCHAIN='local'; $env:CGO_ENABLED='0'; $env:GOFLAGS=''
    $env:AKU_NATIVE_MANUAL_PROJECT_ROOT=$projectRoot
    $env:AKU_NATIVE_MANUAL_CHROME_ENABLED='0'; $env:AKU_NATIVE_MANUAL_CHROME_FIXTURE='0'
    $env:AKU_CHROME_FIXTURE_ROOT=$taskRoot; $env:AKU_CHROME_FIXTURE_CLEANUP_MARKER=$marker
    $env:AKU_CHROME_FIXTURE_NODE=$node; $env:AKU_CHROME_FIXTURE_EXE=$chrome
    Write-Output ('Native manual Chrome ownership run: ' + $runId) | Tee-Object -FilePath $log
    & $node --test scripts/auth-session-fixture/chrome-manual-pilot-driver.test.mjs 2>&1 | Tee-Object -FilePath $log -Append
    if ($LASTEXITCODE -ne 0) { throw 'Chrome driver offline boundary tests failed.' }
    foreach ($mode in @('default','auth_session_dev','auth_session_fixture','auth_session_dev auth_session_fixture')) {
        $arguments=@('list','-f','{{range .TestGoFiles}}{{println .}}{{end}}')
        if ($mode -ne 'default') { $arguments+=@('-tags',$mode) }
        $files=@(& go @arguments './internal/engine')
        if ($LASTEXITCODE -ne 0) { throw 'Chrome fixture tag inspection failed.' }
        $expected=$mode -eq 'auth_session_dev auth_session_fixture'
        if (($files -contains 'native_manual_pilot_chrome_windows_test.go') -ne $expected) { throw 'Chrome fixture entry build-tag boundary failed.' }
        Write-Output ('PASS: Chrome fixture entry tag gate ' + $mode) | Tee-Object -FilePath $log -Append
    }
    $stagedRaw = & $node scripts/auth-session-fixture/stage-native-manual-chrome.mjs $taskRoot
    if ($LASTEXITCODE -ne 0) { throw 'Standalone Chrome fixture staging failed.' }
    $staged = $stagedRaw | ConvertFrom-Json
    $sources = @($staged.files | ForEach-Object { Join-Path $staged.stage $_ })
    Copy-Item -LiteralPath (Join-Path $taskRoot 'staging-provenance.json') -Destination (Join-Path $qaParent ('native-manual-chrome-' + $runId + '.provenance.json'))
    $executable=Join-Path $taskRoot 'native-manual-chrome.test.exe'
    & go test -c -tags 'auth_session_dev auth_session_fixture' -o $executable @sources 2>&1 | Tee-Object -FilePath $log -Append
    if ($LASTEXITCODE -ne 0) { throw 'Standalone Chrome fixture compilation failed.' }
    Write-Output 'PASS: exact bridge/manual sources and original OS helpers compile without Engine/Store initialization.' | Tee-Object -FilePath $log -Append
    if ($VerifyChrome) {
        $env:AKU_NATIVE_MANUAL_CHROME_ENABLED='1'; $env:AKU_NATIVE_MANUAL_CHROME_FIXTURE='1'
        $testArguments=@('-test.run=^TestNativeManualPilotChrome$','-test.timeout=90s','-test.v')
        & $executable @testArguments 2>&1 | Tee-Object -FilePath $log -Append
        $testExit=$LASTEXITCODE
        $canRemove=(Test-Path -LiteralPath $marker -PathType Leaf) -and (Get-Content -LiteralPath $marker -Raw).Trim() -eq 'verified'
        if ($testExit -ne 0) { throw 'Synthetic Chrome ownership acceptance failed.' }
        if (!$canRemove) { throw 'Owned Job/process cleanup proof missing.' }
        Write-Output 'PASS: synthetic Chrome ownership and cleanup; no main login/profile, native X, Store or user runtime touched.' | Tee-Object -FilePath $log -Append
    } else { $canRemove=$true; Write-Output 'Chrome execution not selected; use -VerifyChrome for fresh-profile synthetic acceptance.' | Tee-Object -FilePath $log -Append }
    Write-Output ('Native manual Chrome log: ' + $log)
} finally {
    Pop-Location
    foreach ($name in $variables) { [Environment]::SetEnvironmentVariable($name, $previous[$name], 'Process') }
    if ($canRemove -and (Test-Path -LiteralPath $taskRoot -PathType Container)) {
        $resolved=(Resolve-Path -LiteralPath $taskRoot).ProviderPath
        if ($resolved -ne $taskRoot -or [IO.Path]::GetDirectoryName($resolved) -ne $qaParent -or ((Get-Item -LiteralPath $resolved).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Refusing Chrome cleanup outside owned QA root.' }
        if (Get-ChildItem -LiteralPath $resolved -Recurse -Force | Where-Object { $_.Attributes -band [IO.FileAttributes]::ReparsePoint }) { throw 'Refusing Chrome cleanup through reparse point.' }
        Remove-Item -LiteralPath $resolved -Recurse -Force
        Write-Output 'Owned synthetic Chrome test/profile root removed.'
    }
}

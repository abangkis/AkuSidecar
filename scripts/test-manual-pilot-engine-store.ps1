param()
$ErrorActionPreference = 'Stop'
$projectRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$qaParent = Join-Path $projectRoot 'runtime\qa\auth-session-sidecar'
foreach ($relative in @('runtime','runtime\qa','runtime\qa\auth-session-sidecar','.go-build','.go-build\x-session-verifier','.go-build\mod')) {
    $directory = Join-Path $projectRoot $relative
    if (!(Test-Path -LiteralPath $directory -PathType Container)) { New-Item -ItemType Directory -Path $directory | Out-Null }
    if ((Get-Item -LiteralPath $directory).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Engine Store QA paths must be ordinary project directories.' }
}
$runId = [guid]::NewGuid().ToString('N')
$taskRoot = Join-Path $qaParent ('run-manual-pilot-engine-store-' + $runId)
New-Item -ItemType Directory -Path $taskRoot | Out-Null
$log = Join-Path $qaParent ('manual-pilot-engine-store-' + $runId + '.log.txt')
$variables = @('TEMP','TMP','GOTMPDIR','GOCACHE','GOMODCACHE','GOWORK','GOPROXY','GOSUMDB','GOTOOLCHAIN','CGO_ENABLED','GOFLAGS')
$previous = @{}
foreach ($name in $variables) { $previous[$name] = [Environment]::GetEnvironmentVariable($name,'Process') }
$passed = $false
Push-Location -LiteralPath $projectRoot
try {
    $env:TEMP=$taskRoot; $env:TMP=$taskRoot; $env:GOTMPDIR=$taskRoot
    $env:GOCACHE=Join-Path $projectRoot '.go-build\x-session-verifier'
    $env:GOMODCACHE=Join-Path $projectRoot '.go-build\mod'
    $env:GOWORK='off'; $env:GOPROXY='off'; $env:GOSUMDB='off'; $env:GOTOOLCHAIN='local'; $env:CGO_ENABLED='0'; $env:GOFLAGS=''
    Write-Output ('Synthetic Engine/Store startup integration: ' + $runId) | Tee-Object -FilePath $log
    # Compile actual Engine production sources while excluding package TestMain
    # and every browser/profile/native QA test. Only the bounded tests below run.
    $production = @(& go list -tags 'auth_session_dev auth_session_fixture' -f '{{range .GoFiles}}{{println .}}{{end}}' ./internal/engine)
    if ($LASTEXITCODE -ne 0 -or $production.Count -eq 0) { throw 'Engine production source inventory unavailable.' }
    $engineRoot = Join-Path $projectRoot 'internal/engine'
    $sources = @($production | Where-Object { $_.Trim() -ne '' } | ForEach-Object { Join-Path $engineRoot $_ })
    $tests = @(Get-ChildItem -LiteralPath (Join-Path $projectRoot 'internal/engine') -Filter 'development_native_pilot_store*_test.go' -File)
    if ($tests.Count -eq 0) { throw 'Synthetic Engine Store integration tests unavailable.' }
    $sources += @($tests | ForEach-Object { $_.FullName })
    & go test -tags 'auth_session_dev auth_session_fixture' -run '^TestNativePilotStore' -count=1 -timeout=90s -v @sources 2>&1 | Tee-Object -FilePath $log -Append
    if ($LASTEXITCODE -ne 0) { throw 'Synthetic Engine Store integration failed.' }
    # The ordinary path shares the post-persistence continuation. These named
    # sources keep the regression focused and still exclude browser TestMain.
    $ordinary = @($production | Where-Object { $_.Trim() -ne '' } | ForEach-Object { Join-Path $engineRoot $_ })
    $ordinary += Join-Path $engineRoot 'engine_test.go'
    $ordinary += Join-Path $engineRoot 'capture_runtime_test.go'
    $ordinary += Join-Path $engineRoot 'adaptive_readiness_test.go'
    $ordinaryPattern = '^(TestProgressiveWaitStartsNextCaptureWhileReasoningContinues|TestFullWaitKeepsNextSourceQueuedUntilReasoningCompletes|TestShutdownPreservesAcceptedCaptureAndRestartResumesReasoning|TestFailedFollowUpFallsBackToAcceptedObservation)$'
    & go test -tags 'auth_session_dev auth_session_fixture' -run $ordinaryPattern -count=1 -timeout=90s -v @ordinary 2>&1 | Tee-Object -FilePath $log -Append
    if ($LASTEXITCODE -ne 0) { throw 'Ordinary post-observation continuation regression failed.' }
    $passed=$true
    Write-Output 'PASS: Engine/Store startup integration with synthetic evidence and project-owned databases only.' | Tee-Object -FilePath $log -Append
    Write-Output ('Evidence: ' + $log)
} finally {
    Pop-Location
    foreach ($name in $variables) { [Environment]::SetEnvironmentVariable($name,$previous[$name],'Process') }
    if ($passed -and (Test-Path -LiteralPath $taskRoot -PathType Container)) {
        $resolved=(Resolve-Path -LiteralPath $taskRoot).ProviderPath
        if ($resolved -ne $taskRoot -or [IO.Path]::GetDirectoryName($resolved) -ne $qaParent -or ((Get-Item -LiteralPath $resolved).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Refusing cleanup outside owned Engine Store QA root.' }
        if (Get-ChildItem -LiteralPath $resolved -Recurse -Force | Where-Object { $_.Attributes -band [IO.FileAttributes]::ReparsePoint }) { throw 'Refusing cleanup through a reparse point.' }
        Remove-Item -LiteralPath $resolved -Recurse -Force
        Write-Output 'Owned synthetic Engine Store databases and temporary artifacts removed.'
    } elseif (!$passed) { Write-Warning ('Synthetic QA artifacts retained: ' + $taskRoot) }
}

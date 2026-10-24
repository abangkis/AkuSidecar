param()
$ErrorActionPreference = 'Stop'
$projectRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$qaParent = Join-Path $projectRoot 'runtime\qa\auth-session-sidecar'
foreach ($relative in @('runtime', 'runtime\qa', 'runtime\qa\auth-session-sidecar', '.go-build', '.go-build\x-session-verifier', '.go-build\mod')) {
    $directory = Join-Path $projectRoot $relative
    if (!(Test-Path -LiteralPath $directory -PathType Container)) { New-Item -ItemType Directory -Path $directory | Out-Null }
    if ((Get-Item -LiteralPath $directory).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Manual connector QA paths must be ordinary directories.' }
}
$runId = [guid]::NewGuid().ToString('N')
$taskRoot = Join-Path $qaParent ('run-x-manual-runtime-' + $runId)
New-Item -ItemType Directory -Path $taskRoot | Out-Null
$log = Join-Path $qaParent ('x-manual-runtime-' + $runId + '.log.txt')
$variables = @('TEMP', 'TMP', 'GOTMPDIR', 'GOCACHE', 'GOMODCACHE', 'GOWORK', 'GOPROXY', 'GOSUMDB', 'GOTOOLCHAIN', 'CGO_ENABLED', 'GOFLAGS', 'AKU_DEV_SESSION_TEST_ROOT')
$previous = @{}
foreach ($name in $variables) { $previous[$name] = [Environment]::GetEnvironmentVariable($name, 'Process') }
$missingConfig = Join-Path $taskRoot 'absent-config\sidecar.json'
$missingDatabase = Join-Path $taskRoot 'absent-database\sidecar.db'
$argumentsBase = @('--dev-session-reuse=synthetic-x-manual-fixture', ('--config=' + $missingConfig), ('--database=' + $missingDatabase))

function Confirm-OrdinaryStateAbsent {
    if ((Test-Path -LiteralPath (Split-Path -Parent $missingConfig)) -or (Test-Path -LiteralPath (Split-Path -Parent $missingDatabase))) { throw 'Fixture entered ordinary configuration/database startup.' }
}

function Invoke-ManualFixture([string]$Executable, [string[]]$Arguments, [string]$Payload, [bool]$Accepted, [string]$Label) {
    $start = [Diagnostics.ProcessStartInfo]::new()
    $start.FileName = $Executable
    $start.UseShellExecute = $false
    $start.CreateNoWindow = $true
    $start.RedirectStandardInput = $true
    $start.RedirectStandardOutput = $true
    $start.RedirectStandardError = $true
    foreach ($argument in $Arguments) { $start.ArgumentList.Add($argument) }
    $process = [Diagnostics.Process]::new()
    $process.StartInfo = $start
    if (!$process.Start()) { throw 'Synthetic main executable unavailable.' }
    try {
        $stdout = $process.StandardOutput.ReadToEndAsync()
        $stderr = $process.StandardError.ReadToEndAsync()
        $process.StandardInput.Write($Payload)
        $process.StandardInput.Close()
        if (!$process.WaitForExit(15000)) { $process.Kill($true); throw 'Synthetic manual runtime timed out.' }
        $output = $stdout.GetAwaiter().GetResult()
        $errorOutput = $stderr.GetAwaiter().GetResult()
        if ($Accepted) {
            if ($process.ExitCode -ne 0 -or $errorOutput.Length -ne 0 -or $output.Length -gt 1024) { throw ('Synthetic manual runtime failed: ' + $Label + ' ' + $output + $errorOutput) }
            $reply = $output | ConvertFrom-Json -NoEnumerate
            $fields = @('provider', 'scenario', 'passed', 'offlineValidated', 'canAuthorize', 'nativeQualified', 'storePermit')
            if ($null -eq $reply -or $reply -is [array] -or (@($reply.PSObject.Properties.Name | Sort-Object) -join ',') -ne (($fields | Sort-Object) -join ',')) { throw 'Manual runtime report fields rejected.' }
            $expected = $Payload | ConvertFrom-Json
            if ($reply.provider -isnot [string] -or $reply.provider -ne 'synthetic-x-manual-fixture' -or $reply.scenario -isnot [string] -or $reply.scenario -ne $expected.scenario) { throw 'Manual runtime report scope rejected.' }
            foreach ($field in @('passed', 'offlineValidated', 'canAuthorize', 'nativeQualified', 'storePermit')) { if ($reply.$field -isnot [bool]) { throw 'Manual runtime report flag type rejected.' } }
            if (!$reply.passed -or !$reply.offlineValidated -or $reply.canAuthorize -or $reply.nativeQualified -or $reply.storePermit) { throw 'Synthetic runtime claimed native/Store authority.' }
        } elseif ($process.ExitCode -eq 0 -or ($output + $errorOutput) -notmatch '(development session|manual fixture|manual runtime|synthetic.*manual)') {
            throw ('Manual runtime early rejection failed: ' + $Label + ' ' + $output + $errorOutput)
        }
        Confirm-OrdinaryStateAbsent
        Write-Output ('PASS: ' + $Label) | Tee-Object -FilePath $log -Append
    } finally {
        if (!$process.HasExited) { $process.Kill($true); $process.WaitForExit() }
        $process.Dispose()
    }
}

Push-Location -LiteralPath $projectRoot
try {
    $env:TEMP = $taskRoot; $env:TMP = $taskRoot; $env:GOTMPDIR = $taskRoot
    $env:GOCACHE = Join-Path $projectRoot '.go-build\x-session-verifier'
    $env:GOMODCACHE = Join-Path $projectRoot '.go-build\mod'
    $env:GOWORK = 'off'; $env:GOPROXY = 'off'; $env:GOSUMDB = 'off'
    $env:GOTOOLCHAIN = 'local'; $env:CGO_ENABLED = '0'; $env:GOFLAGS = ''
    Write-Output ('Offline X manual runtime integration: ' + $runId) | Tee-Object -FilePath $log
    # Named sources exclude package TestMain and all database/native fixtures.
    $core = @('development_native_pilot_adapter.go', 'development_native_pilot_startup.go', 'development_native_pilot_controller.go', 'development_native_pilot_broker.go', 'development_native_pilot_runtime.go', 'development_native_pilot_runtime_fixture.go', 'development_native_pilot_runtime_fixture_test.go')
    $sources = @($core | ForEach-Object { Join-Path 'internal/engine' $_ })
    & go test -tags 'auth_session_dev auth_session_fixture' -run '^TestManualRuntime' -count=1 -timeout=30s -v @sources 2>&1 | Tee-Object -FilePath $log -Append
    if ($LASTEXITCODE -ne 0) { throw 'Manual runtime connector regression failed.' }
    $valid = '{"grant":"synthetic-x-manual-owned","scenario":"overlap"}'
    foreach ($mode in @('default', 'auth_session_dev', 'auth_session_fixture', 'auth_session_dev auth_session_fixture')) {
        $image = if ($mode -eq 'default') { 'aku-sidecar-default-check.exe' } elseif ($mode -eq 'auth_session_dev') { 'aku-sidecar-dev-unavailable.exe' } elseif ($mode -eq 'auth_session_fixture') { 'aku-sidecar-fixture-only.exe' } else { 'aku-sidecar-dev.exe' }
        $exe = Join-Path $taskRoot $image
        $build = @('build', '-o', $exe)
        if ($mode -ne 'default') { $build += @('-tags', $mode) }
        & go @build ./cmd/akusidecar 2>&1 | Tee-Object -FilePath $log -Append
        if ($LASTEXITCODE -ne 0) { throw ('Actual main compilation failed: ' + $mode) }
        Write-Output ('PASS: actual main build ' + $mode) | Tee-Object -FilePath $log -Append
        if ($mode -ne 'auth_session_dev auth_session_fixture') {
            Invoke-ManualFixture $exe ($argumentsBase + '--dev') $valid $false ($mode + ' rejects synthetic manual provider')
            continue
        }
        Invoke-ManualFixture $exe $argumentsBase $valid $false 'manual provider requires --dev'
        foreach ($invalid in @('{}', 'null', '[]', '{"grant":"wrong","scenario":"overlap"}', '{"grant":"synthetic-x-manual-owned","scenario":"native"}', '{"grant":"synthetic-x-manual-owned","scenario":"overlap","canAuthorize":true}', '{"grant":"synthetic-x-manual-owned","scenario":"overlap","scenario":"expiry"}', '{"Grant":"synthetic-x-manual-owned","scenario":"overlap"}', ($valid + '{}'), (' ' * 1025))) {
            Invoke-ManualFixture $exe ($argumentsBase + '--dev') $invalid $false 'bounded input rejects missing/forged/duplicate grants and native flags'
        }
        $scenarios = @('overlap', 'duplicate-result', 'expiry', 'opt-out', 'parent-cancel', 'owner-drift', 'last-window', 'source-rejection', 'broker-rejection', 'prepare-failure')
        foreach ($scenario in $scenarios) {
            $payload = @{grant = 'synthetic-x-manual-owned'; scenario = $scenario} | ConvertTo-Json -Compress
            Invoke-ManualFixture $exe ($argumentsBase + '--dev') $payload $true ('actual main manual-v2 ' + $scenario)
        }
        Invoke-ManualFixture $exe @('--dev', '--dev-session-reuse=native-x-manual', ('--config=' + $missingConfig), ('--database=' + $missingDatabase)) $valid $false 'real provider remains unavailable'
    }
    $env:AKU_DEV_SESSION_TEST_ROOT = $taskRoot
    & go test -tags 'auth_session_dev auth_session_fixture' ./cmd/akusidecar -run '^(TestDevelopmentXManual|TestDevelopmentSessionDefault|TestDevelopmentSessionCLI)' -count=1 -timeout=60s -v 2>&1 | Tee-Object -FilePath $log -Append
    if ($LASTEXITCODE -ne 0) { throw 'Main dispatch regression failed.' }
    & go test -tags auth_session_dev ./internal/collection -run '^(TestManualConcurrent|TestSwitchWaitsForLeaseAndPinsDriver|TestBrowserBorrowReleaseIsIdempotent|TestRetainedPopupAutomaticallyResumesHeadlessAfterNaturalExit)' -count=1 -timeout=30s -v 2>&1 | Tee-Object -FilePath $log -Append
    if ($LASTEXITCODE -ne 0) { throw 'Coordinator regression failed.' }
    Confirm-OrdinaryStateAbsent
    Write-Output 'PASS: main -> private manual broker -> Coordinator with synthetic owners only. No Store/database, browser/profile, credential transfer, native activation or user runtime accessed.' | Tee-Object -FilePath $log -Append
    Write-Output ('Integration log: ' + $log)
} finally {
    Pop-Location
    foreach ($name in $variables) { [Environment]::SetEnvironmentVariable($name, $previous[$name], 'Process') }
    if (Test-Path -LiteralPath $taskRoot -PathType Container) {
        $resolved = (Resolve-Path -LiteralPath $taskRoot).ProviderPath
        if ($resolved -ne $taskRoot -or [IO.Path]::GetDirectoryName($resolved) -ne $qaParent -or ((Get-Item -LiteralPath $resolved).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Refusing cleanup outside owned manual runtime root.' }
        if (Get-ChildItem -LiteralPath $resolved -Recurse -Force | Where-Object { $_.Attributes -band [IO.FileAttributes]::ReparsePoint }) { throw 'Refusing cleanup through reparse point.' }
        Remove-Item -LiteralPath $resolved -Recurse -Force
        Write-Output 'Owned synthetic integration build root removed.'
    }
}

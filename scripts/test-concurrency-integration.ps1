param()
$ErrorActionPreference = 'Stop'
$projectRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$qaParent = Join-Path $projectRoot 'runtime\qa\auth-session-sidecar'
foreach ($relative in @('runtime', 'runtime\qa', 'runtime\qa\auth-session-sidecar', '.go-build', '.go-build\x-session-verifier', '.go-build\mod')) {
    $directory = Join-Path $projectRoot $relative
    if (!(Test-Path -LiteralPath $directory -PathType Container)) { New-Item -ItemType Directory -Path $directory | Out-Null }
    if ((Get-Item -LiteralPath $directory).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Integration QA paths must not be reparse points.' }
}
$runId = [guid]::NewGuid().ToString('N')
$taskRoot = Join-Path $qaParent ('run-concurrent-integration-' + $runId)
New-Item -ItemType Directory -Path $taskRoot | Out-Null
$log = Join-Path $qaParent ('concurrent-integration-' + $runId + '.log.txt')
$variables = @('TEMP', 'TMP', 'GOTMPDIR', 'GOCACHE', 'GOMODCACHE', 'GOWORK', 'GOPROXY', 'GOSUMDB', 'GOTOOLCHAIN', 'CGO_ENABLED', 'GOFLAGS')
$previous = @{}
foreach ($name in $variables) { $previous[$name] = [Environment]::GetEnvironmentVariable($name, 'Process') }

function Invoke-Fixture([string]$Executable, [string[]]$Arguments, [string]$Payload, [bool]$Accepted, [string]$Label) {
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
    if (!$process.Start()) { throw 'Fixture process unavailable.' }
    try {
        $stdout = $process.StandardOutput.ReadToEndAsync()
        $stderr = $process.StandardError.ReadToEndAsync()
        $process.StandardInput.Write($Payload)
        $process.StandardInput.Close()
        if (!$process.WaitForExit(20000)) { $process.Kill($true); throw 'Synthetic fixture timed out.' }
        $result = $stdout.GetAwaiter().GetResult() + $stderr.GetAwaiter().GetResult()
        if ($Accepted) {
            if ($process.ExitCode -ne 0) { throw ('Fixture failed: ' + $Label + ' ' + $result) }
            $reply = $result | ConvertFrom-Json
            $expected = $Payload | ConvertFrom-Json
            if ($reply.passed -ne $true -or $reply.provider -ne 'synthetic-concurrent-fixture' -or $reply.scenario -ne $expected.scenario) { throw 'Synthetic acceptance response rejected.' }
        } elseif ($process.ExitCode -eq 0 -or $result -notmatch '(development session|synthetic concurrent)') {
            throw ('Fixture rejection gate failed: ' + $Label + ' ' + $result)
        }
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
    Write-Output ('Offline synthetic concurrency integration run: ' + $runId) | Tee-Object -FilePath $log
    $missingConfig = Join-Path $taskRoot 'absent-config\sidecar.json'
    $missingDatabase = Join-Path $taskRoot 'absent-database\sidecar.db'
    $baseArguments = @('--dev-session-reuse', 'synthetic-concurrent-fixture', '--config', $missingConfig, '--database', $missingDatabase)
    $valid = '{"grant":"synthetic-owned-fixture","scenario":"overlap"}'
    foreach ($mode in @('default', 'auth_session_dev', 'auth_session_fixture', 'auth_session_dev auth_session_fixture')) {
        $dual = $mode -eq 'auth_session_dev auth_session_fixture'
        $buildArguments = @('build', '-o', (Join-Path $taskRoot 'fixture.exe'))
        if ($mode -ne 'default') { $buildArguments += @('-tags', $mode) }
        $buildArguments += './cmd/akusidecar'
        & go @buildArguments 2>&1 | Tee-Object -FilePath $log -Append
        if ($LASTEXITCODE -ne 0) { throw ('Actual main build failed: ' + $mode) }
        Write-Output ('PASS: actual main build ' + $mode) | Tee-Object -FilePath $log -Append
        $executable = Join-Path $taskRoot 'fixture.exe'
        if (!$dual) { Invoke-Fixture $executable ($baseArguments + '--dev') $valid $false ($mode + ' rejects concurrent provider'); continue }
        Invoke-Fixture $executable $baseArguments $valid $false 'dual-tag requires --dev'
        foreach ($payload in @('{}', '{"grant":"wrong","scenario":"overlap"}', '{"grant":"synthetic-owned-fixture","scenario":"native"}', '{"grant":"synthetic-owned-fixture","scenario":"overlap","native":true}', ($valid + '{}'), (' ' * 1025))) {
            Invoke-Fixture $executable ($baseArguments + '--dev') $payload $false 'strict bounded input rejects ungranted/unsupported payload'
        }
        Invoke-Fixture $executable @('--dev', '--dev-session-reuse', 'native-concurrent', '--config', $missingConfig, '--database', $missingDatabase) $valid $false 'native provider unavailable'
        foreach ($scenario in @('overlap', 'last-close', 'last-close-denied', 'stale', 'cancel', 'revoked', 'parent-death', 'opt-out', 'strategy-gates', 'broker-gates', 'collector-loss')) {
            $payload = @{ grant = 'synthetic-owned-fixture'; scenario = $scenario } | ConvertTo-Json -Compress
            Invoke-Fixture $executable ($baseArguments + '--dev') $payload $true ('actual main ' + $scenario)
        }
        if ((Test-Path -LiteralPath $missingConfig) -or (Test-Path -LiteralPath $missingDatabase) -or (Test-Path -LiteralPath (Split-Path -Parent $missingConfig)) -or (Test-Path -LiteralPath (Split-Path -Parent $missingDatabase))) { throw 'Early dispatch sentinel was touched.' }
        Write-Output 'PASS: absent configuration/database sentinels remain absent after actual-main dispatch' | Tee-Object -FilePath $log -Append
    }
    & go test ./internal/collection -run '^(TestSwitchWaitsForLeaseAndPinsDriver|TestBrowserBorrowReleaseIsIdempotent|TestRetainedPopupAutomaticallyResumesHeadlessAfterNaturalExit)$' -count=1 -timeout=30s -v 2>&1 | Tee-Object -FilePath $log -Append
    if ($LASTEXITCODE -ne 0) { throw 'Default-nil Coordinator regression failed.' }
    Write-Output 'Synthetic integration passed; no Store/database, Chrome, login, native listener, activation, registry or user runtime was opened.' | Tee-Object -FilePath $log -Append
    Write-Output ('Synthetic integration log: ' + $log)
} finally {
    Pop-Location
    foreach ($name in $variables) { [Environment]::SetEnvironmentVariable($name, $previous[$name], 'Process') }
    if (Test-Path -LiteralPath $taskRoot -PathType Container) {
        $resolved = (Resolve-Path -LiteralPath $taskRoot).ProviderPath
        if ($resolved -ne $taskRoot -or [IO.Path]::GetDirectoryName($resolved) -ne $qaParent -or ((Get-Item -LiteralPath $resolved).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Refusing cleanup outside owned integration QA root.' }
        if (Get-ChildItem -LiteralPath $resolved -Recurse -Force | Where-Object { $_.Attributes -band [IO.FileAttributes]::ReparsePoint }) { throw 'Refusing cleanup through reparse point.' }
        Remove-Item -LiteralPath $resolved -Recurse -Force
        Write-Output 'Owned temporary integration build root removed.'
    }
}

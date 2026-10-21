param()
$ErrorActionPreference = 'Stop'
$projectRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$qaParent = Join-Path $projectRoot 'runtime\qa\auth-session-sidecar'
foreach ($relative in @('runtime', 'runtime\qa', 'runtime\qa\auth-session-sidecar', '.go-build', '.go-build\x-session-verifier', '.go-build\mod')) {
    $directory = Join-Path $projectRoot $relative
    if (!(Test-Path -LiteralPath $directory -PathType Container)) { New-Item -ItemType Directory -Path $directory | Out-Null }
    if ((Get-Item -LiteralPath $directory).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Native adapter QA paths must not be reparse points.' }
}
$sources = @('internal/engine/development_native_pilot_adapter.go', 'internal/engine/development_native_pilot_adapter_test.go')
foreach ($relative in $sources) {
    if (!(Test-Path -LiteralPath (Join-Path $projectRoot $relative) -PathType Leaf)) { throw 'Native adapter source unavailable.' }
}
$runId = [guid]::NewGuid().ToString('N')
$taskRoot = Join-Path $qaParent ('run-native-manual-adapter-' + $runId)
New-Item -ItemType Directory -Path $taskRoot | Out-Null
$log = Join-Path $qaParent ('native-manual-adapter-' + $runId + '.log.txt')
$variables = @('TEMP', 'TMP', 'GOTMPDIR', 'GOCACHE', 'GOMODCACHE', 'GOWORK', 'GOPROXY', 'GOSUMDB', 'GOTOOLCHAIN', 'CGO_ENABLED', 'GOFLAGS')
$previous = @{}
foreach ($name in $variables) { $previous[$name] = [Environment]::GetEnvironmentVariable($name, 'Process') }
Push-Location -LiteralPath $projectRoot
try {
    $env:TEMP = $taskRoot; $env:TMP = $taskRoot; $env:GOTMPDIR = $taskRoot
    $env:GOCACHE = Join-Path $projectRoot '.go-build\x-session-verifier'
    $env:GOMODCACHE = Join-Path $projectRoot '.go-build\mod'
    $env:GOWORK = 'off'; $env:GOPROXY = 'off'; $env:GOSUMDB = 'off'
    $env:GOTOOLCHAIN = 'local'; $env:CGO_ENABLED = '0'; $env:GOFLAGS = ''
    Write-Output ('Offline native manual adapter run: ' + $runId) | Tee-Object -FilePath $log
    foreach ($mode in @('default', 'auth_session_dev', 'auth_session_fixture', 'auth_session_dev auth_session_fixture')) {
        $arguments = @('list', '-f', '{{range .GoFiles}}{{println .}}{{end}}{{range .TestGoFiles}}{{println .}}{{end}}')
        if ($mode -ne 'default') { $arguments += @('-tags', $mode) }
        $arguments += './internal/engine'
        $files = @(& go @arguments)
        if ($LASTEXITCODE -ne 0) { throw 'Native adapter tag inspection failed.' }
        $expected = $mode -eq 'auth_session_dev' -or $mode -eq 'auth_session_dev auth_session_fixture'
        foreach ($relative in $sources) {
            if (($files -contains [IO.Path]::GetFileName($relative)) -ne $expected) { throw 'Native adapter build-tag boundary failed.' }
        }
        Write-Output ('PASS: native adapter source/test tag gate ' + $mode) | Tee-Object -FilePath $log -Append
    }
    # Named stdlib-only files avoid Engine initialization and Store package tests.
    & go test -tags 'auth_session_dev' -run '^TestNativePilot' -count=1 -timeout=60s -v @sources 2>&1 | Tee-Object -FilePath $log -Append
    if ($LASTEXITCODE -ne 0) { throw 'Native manual adapter offline regression failed.' }
    foreach ($mode in @('auth_session_dev', 'auth_session_dev auth_session_fixture')) {
        & go build -tags $mode ./internal/engine 2>&1 | Tee-Object -FilePath $log -Append
        if ($LASTEXITCODE -ne 0) { throw 'Native adapter Engine compilation failed.' }
        Write-Output ('PASS: Engine compilation ' + $mode) | Tee-Object -FilePath $log -Append
    }
    Write-Output 'PASS: offline private receipt validation only; no Chrome, profile, login, transfer, Store, network listener or user runtime opened. Native startup producer/controller remain unregistered.' | Tee-Object -FilePath $log -Append
    Write-Output ('Native manual adapter log: ' + $log)
} finally {
    Pop-Location
    foreach ($name in $variables) { [Environment]::SetEnvironmentVariable($name, $previous[$name], 'Process') }
    if (Test-Path -LiteralPath $taskRoot -PathType Container) {
        $resolved = (Resolve-Path -LiteralPath $taskRoot).ProviderPath
        if ($resolved -ne $taskRoot -or [IO.Path]::GetDirectoryName($resolved) -ne $qaParent -or
            ((Get-Item -LiteralPath $resolved).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Refusing cleanup outside native adapter QA root.' }
        if (Get-ChildItem -LiteralPath $resolved -Recurse -Force | Where-Object { $_.Attributes -band [IO.FileAttributes]::ReparsePoint }) { throw 'Refusing cleanup through native adapter QA reparse point.' }
        Remove-Item -LiteralPath $resolved -Recurse -Force
        Write-Output 'Owned native adapter temporary build root removed.'
    }
}

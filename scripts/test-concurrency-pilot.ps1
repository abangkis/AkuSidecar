param()
$ErrorActionPreference = 'Stop'
$projectRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$qaParent = Join-Path $projectRoot 'runtime\qa\auth-session-sidecar'
foreach ($relative in @('runtime', 'runtime\qa', 'runtime\qa\auth-session-sidecar', '.go-build', '.go-build\x-session-verifier', '.go-build\mod')) {
    $directory = Join-Path $projectRoot $relative
    if (!(Test-Path -LiteralPath $directory -PathType Container)) { New-Item -ItemType Directory -Path $directory | Out-Null }
    if ((Get-Item -LiteralPath $directory).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Pilot QA paths must not be reparse points.' }
}
$pilot = @('internal/engine/development_concurrent_pilot.go', 'internal/engine/development_concurrent_pilot_test.go', 'internal/engine/development_concurrent_pilot_boundary_test.go', 'internal/engine/development_concurrent_pilot_controller.go', 'internal/engine/development_concurrent_pilot_controller_test.go')
$strict = @('internal/engine/development_concurrent_authorization.go', 'internal/engine/development_concurrent_authorization_test.go')
$controller = @('internal/engine/development_concurrent_session.go', 'internal/engine/development_concurrent_session_test.go', 'internal/engine/development_concurrent_authorized_controller.go')
$sources = $pilot + $strict + $controller
foreach ($relative in $sources) {
    if (!(Test-Path -LiteralPath (Join-Path $projectRoot $relative) -PathType Leaf)) { throw 'Pilot model source unavailable.' }
}
$runId = [guid]::NewGuid().ToString('N')
$taskRoot = Join-Path $qaParent ('run-concurrent-pilot-' + $runId)
New-Item -ItemType Directory -Path $taskRoot | Out-Null
$log = Join-Path $qaParent ('concurrent-pilot-' + $runId + '.log.txt')
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
    Write-Output ('Offline bounded snapshot pilot run: ' + $runId) | Tee-Object -FilePath $log
    foreach ($mode in @('default', 'auth_session_dev', 'auth_session_fixture', 'auth_session_dev auth_session_fixture')) {
        $arguments = @('list', '-f', '{{range .GoFiles}}{{println .}}{{end}}{{range .TestGoFiles}}{{println .}}{{end}}')
        if ($mode -ne 'default') { $arguments += @('-tags', $mode) }
        $arguments += './internal/engine'
        $files = @(& go @arguments)
        if ($LASTEXITCODE -ne 0) { throw 'Pilot build-tag inspection failed.' }
        foreach ($relative in $pilot) {
            if (($files -contains [IO.Path]::GetFileName($relative)) -ne ($mode -eq 'auth_session_dev auth_session_fixture')) { throw 'Pilot source/test build-tag boundary failed.' }
        }
        Write-Output ('PASS: pilot source/test build-tag gate ' + $mode) | Tee-Object -FilePath $log -Append
    }
    # Compile named stdlib sources only. Do not execute Engine initializers or Store tests.
    # Existing controller/authority helpers support the seam tests; unrelated cases are not rerun.
    $selection = @(& go test -tags 'auth_session_dev auth_session_fixture' -list '^TestConcurrentPilot' @sources 2>&1)
    if ($LASTEXITCODE -ne 0 -or @($selection | Where-Object { $_ -match '^TestConcurrentPilot' }).Count -lt 2) { throw 'Pilot regression selection unavailable.' }
    & go test -tags 'auth_session_dev auth_session_fixture' -run '^TestConcurrentPilot' -count=1 -timeout=60s -v @sources 2>&1 | Tee-Object -FilePath $log -Append
    if ($LASTEXITCODE -ne 0) { throw 'Pilot offline regression failed.' }
    & go build -tags 'auth_session_dev auth_session_fixture' ./internal/engine 2>&1 | Tee-Object -FilePath $log -Append
    if ($LASTEXITCODE -ne 0) { throw 'Pilot combined Engine compilation failed.' }
    Write-Output 'PASS: dual-tag Engine compilation; no package runtime executed.' | Tee-Object -FilePath $log -Append
    Write-Output 'PASS: synthetic pilot model/controller only; no Store/database, browser, login, network listener, profile or user runtime opened. Native and Store authority remain unavailable.' | Tee-Object -FilePath $log -Append
    Write-Output ('Pilot regression log: ' + $log)
} finally {
    Pop-Location
    foreach ($name in $variables) { [Environment]::SetEnvironmentVariable($name, $previous[$name], 'Process') }
    if (Test-Path -LiteralPath $taskRoot -PathType Container) {
        $resolved = (Resolve-Path -LiteralPath $taskRoot).ProviderPath
        if ($resolved -ne $taskRoot -or [IO.Path]::GetDirectoryName($resolved) -ne $qaParent -or
            ((Get-Item -LiteralPath $resolved).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Refusing cleanup outside owned pilot QA root.' }
        if (Get-ChildItem -LiteralPath $resolved -Recurse -Force | Where-Object { $_.Attributes -band [IO.FileAttributes]::ReparsePoint }) { throw 'Refusing cleanup through a pilot QA reparse point.' }
        Remove-Item -LiteralPath $resolved -Recurse -Force
        Write-Output 'Owned temporary pilot build root removed.'
    }
}

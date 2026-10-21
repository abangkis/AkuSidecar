param()
$ErrorActionPreference = 'Stop'
$projectRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$qaParent = Join-Path $projectRoot 'runtime\qa\auth-session-sidecar'
foreach ($relative in @('runtime', 'runtime\qa', 'runtime\qa\auth-session-sidecar', '.go-build', '.go-build\x-session-verifier', '.go-build\mod')) {
    $directory = Join-Path $projectRoot $relative
    if (!(Test-Path -LiteralPath $directory -PathType Container)) { New-Item -ItemType Directory -Path $directory | Out-Null }
    if ((Get-Item -LiteralPath $directory).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Authorized controller QA paths must not be reparse points.' }
}
$legacy = @('internal/engine/development_concurrent_session.go', 'internal/engine/development_concurrent_session_test.go')
$authority = @('internal/engine/development_concurrent_authorization.go', 'internal/engine/development_concurrent_authorization_test.go')
$bridge = @('internal/engine/development_concurrent_authorized_controller.go', 'internal/engine/development_concurrent_authorized_controller_test.go')
$sources = $legacy + $authority + $bridge
foreach ($relative in $sources) {
    if (!(Test-Path -LiteralPath (Join-Path $projectRoot $relative) -PathType Leaf)) { throw 'Authorized controller source unavailable.' }
}
$runId = [guid]::NewGuid().ToString('N')
$taskRoot = Join-Path $qaParent ('run-concurrent-authorized-' + $runId)
New-Item -ItemType Directory -Path $taskRoot | Out-Null
$log = Join-Path $qaParent ('concurrent-authorized-controller-' + $runId + '.log.txt')
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
    Write-Output ('Offline authorized controller run: ' + $runId) | Tee-Object -FilePath $log
    foreach ($mode in @('default', 'auth_session_dev', 'auth_session_fixture', 'auth_session_dev auth_session_fixture')) {
        $arguments = @('list', '-f', '{{range .GoFiles}}{{println .}}{{end}}{{range .TestGoFiles}}{{println .}}{{end}}')
        if ($mode -ne 'default') { $arguments += @('-tags', $mode) }
        $arguments += './internal/engine'
        $files = @(& go @arguments)
        if ($LASTEXITCODE -ne 0) { throw 'Build-tag inspection failed.' }
        foreach ($relative in $sources) {
            $name = [IO.Path]::GetFileName($relative)
            if (($files -contains $name) -ne ($mode -eq 'auth_session_dev auth_session_fixture')) { throw 'Authorized controller build-tag boundary failed.' }
        }
        Write-Output ('Build-tag gate passed: ' + $mode) | Tee-Object -FilePath $log -Append
    }
    # Compile-only checks preserve the existing standalone helpers without repeating their tests.
    foreach ($standalone in @(@{ Name='legacy controller'; Files=$legacy }, @{ Name='pair authority'; Files=$authority })) {
        $arguments = @('test', '-tags', 'auth_session_dev auth_session_fixture', '-run', '^$', '-count=1', '-timeout=60s') + $standalone.Files
        & go @arguments 2>&1 | Tee-Object -FilePath $log -Append
        if ($LASTEXITCODE -ne 0) { throw 'Standalone source compatibility failed.' }
        Write-Output ('Standalone source compatibility passed: ' + $standalone.Name) | Tee-Object -FilePath $log -Append
    }
    # Execute only these named sources; exclude Engine initializers and every Store/database test.
    $arguments = @('test', '-tags', 'auth_session_dev auth_session_fixture', '-count=1', '-timeout=60s', '-v') + $sources
    & go @arguments 2>&1 | Tee-Object -FilePath $log -Append
    if ($LASTEXITCODE -ne 0) { throw 'Authorized controller offline regression failed.' }
    & go build -tags 'auth_session_dev auth_session_fixture' ./internal/engine 2>&1 | Tee-Object -FilePath $log -Append
    if ($LASTEXITCODE -ne 0) { throw 'Combined Engine compilation failed.' }
    Write-Output 'Combined dual-tag Engine compilation passed; no package runtime executed.' | Tee-Object -FilePath $log -Append
    Write-Output 'Authorized controller regression passed; no Store/database, Chrome, login, listener or user runtime was opened. Native authorization remains unavailable.' | Tee-Object -FilePath $log -Append
    Write-Output ('Synthetic test log: ' + $log)
} finally {
    Pop-Location
    foreach ($name in $variables) { [Environment]::SetEnvironmentVariable($name, $previous[$name], 'Process') }
    if (Test-Path -LiteralPath $taskRoot -PathType Container) {
        $resolved = (Resolve-Path -LiteralPath $taskRoot).ProviderPath
        if ($resolved -ne $taskRoot -or [IO.Path]::GetDirectoryName($resolved) -ne $qaParent -or
            ((Get-Item -LiteralPath $resolved).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Refusing cleanup outside owned authorized controller QA root.' }
        if (Get-ChildItem -LiteralPath $resolved -Recurse -Force | Where-Object { $_.Attributes -band [IO.FileAttributes]::ReparsePoint }) { throw 'Refusing cleanup through reparse point.' }
        Remove-Item -LiteralPath $resolved -Recurse -Force
        Write-Output 'Owned temporary authorized controller build root removed.'
    }
}

param()
$ErrorActionPreference = 'Stop'
$project = Split-Path -Parent $PSScriptRoot
$root = Join-Path $project ('build\stable-dev-regression-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $root | Out-Null
$names = @('TEMP','TMP','GOTMPDIR','GOCACHE','GOMODCACHE','GOWORK','GOPROXY','GOSUMDB','GOTOOLCHAIN','CGO_ENABLED','GOFLAGS','AKU_DEV_RUNTIME_TEST_IMAGES')
$previous = @{}
foreach ($name in $names) { $previous[$name] = [Environment]::GetEnvironmentVariable($name, 'Process') }
Push-Location -LiteralPath $project
try {
    $env:TEMP=$root; $env:TMP=$root; $env:GOTMPDIR=$root
    $env:GOCACHE=Join-Path $project '.go-build\x-development-entry'
    $env:GOMODCACHE=Join-Path $project '.go-build\mod'
    $env:GOWORK='off'; $env:GOPROXY='off'; $env:GOSUMDB='off'; $env:GOTOOLCHAIN='local'; $env:CGO_ENABLED='0'; $env:GOFLAGS=''
    $env:AKU_DEV_RUNTIME_TEST_IMAGES=$root
    $log=Join-Path $root 'offline.log.txt'
    foreach ($mode in @('default','dev','fixture','dual')) {
        $tags = switch ($mode) { 'dev' {'auth_session_dev'} 'fixture' {'auth_session_fixture'} 'dual' {'auth_session_dev auth_session_fixture'} }
        $arguments=@('build','-o',(Join-Path $root ($mode + '-diagnostic.exe')))
        if ($tags) { $arguments+=@('-tags',$tags) }
        & go @arguments ./cmd/akusidecar 2>&1 | Tee-Object -FilePath $log -Append
        if ($LASTEXITCODE -ne 0) { throw "Development diagnostic build failed: $mode" }
    }
    foreach ($mode in @('default','dev')) {
        $arguments=@('test','./cmd/akusidecar','-count=1','-timeout=90s','-v')
        if ($mode -eq 'dev') { $arguments+=@('-tags','auth_session_dev') }
        & go @arguments 2>&1 | Tee-Object -FilePath $log -Append
        if ($LASTEXITCODE -ne 0) { throw "Development main/CLI regression failed: $mode" }
    }
    & (Join-Path $project 'runtime\dev\headless-worker\node.exe') scripts/stage-dev-x-assets.mjs (Join-Path $root 'native-capabilities') 2>&1 | Tee-Object -FilePath $log -Append
    if ($LASTEXITCODE -ne 0) { throw 'Source-only native capability staging failed.' }
    & (Join-Path $PSScriptRoot 'test-dev-worker-promotion.ps1') -DiagnosticImage (Join-Path $root 'dev-diagnostic.exe') 2>&1 | Tee-Object -FilePath $log -Append
    & (Join-Path $PSScriptRoot 'test-dev-graceful-shutdown.ps1') -ResultPath (Join-Path $root 'graceful-shutdown.json') 2>&1 | Tee-Object -FilePath $log -Append
    & (Join-Path $PSScriptRoot 'test-dev-native-derivation-hold.ps1') 2>&1 | Tee-Object -FilePath $log -Append
    & (Join-Path $PSScriptRoot 'test-worker-backup-retention.ps1') 2>&1 | Tee-Object -FilePath $log -Append
    Write-Output "PASS: stable development build/restart synthetic regressions. Evidence: $log"
} finally {
    Pop-Location
    foreach ($name in $names) { [Environment]::SetEnvironmentVariable($name, $previous[$name], 'Process') }
}

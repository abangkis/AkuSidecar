param()
$ErrorActionPreference = 'Stop'
$projectRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$qaParent = Join-Path $projectRoot 'runtime\qa\auth-session-sidecar'
New-Item -ItemType Directory -Path $qaParent -Force | Out-Null
$qaParent = (Resolve-Path -LiteralPath $qaParent).ProviderPath
if ($qaParent -ne (Join-Path $projectRoot 'runtime\qa\auth-session-sidecar')) { throw 'QA parent escaped project scope.' }
if ((Get-Item -LiteralPath $qaParent).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'QA parent must not be a reparse point.' }
$taskRoot = Join-Path $qaParent ('run-pause-compatibility-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $taskRoot | Out-Null
$variables = @('TEMP','TMP','GOTMPDIR','GOPROXY','GOSUMDB','GOTOOLCHAIN','CGO_ENABLED','AKUSIDECAPTUREPAUSE_QA_ROOT','AKUSIDECAPTUREPAUSE_QA_STAGE')
$previous = @{}
foreach ($variable in $variables) { $previous[$variable] = [Environment]::GetEnvironmentVariable($variable, 'Process') }
$passed = $false
Push-Location -LiteralPath $projectRoot
try {
    $env:TEMP = $taskRoot
    $env:TMP = $taskRoot
    $env:GOTMPDIR = $taskRoot
    $env:GOPROXY = 'off'
    $env:GOSUMDB = 'off'
    $env:GOTOOLCHAIN = 'local'
    $env:CGO_ENABLED = '0'
    $env:AKUSIDECAPTUREPAUSE_QA_ROOT = $null
    $env:AKUSIDECAPTUREPAUSE_QA_STAGE = $null
    foreach ($build in @('default','fixture','dev')) {
        $binary = Join-Path $taskRoot ('store-' + $build + '.test.exe')
        $buildArguments = @('test','-c','-o',$binary)
        if ($build -eq 'fixture') { $buildArguments += @('-tags','auth_session_fixture') }
        if ($build -eq 'dev') { $buildArguments += @('-tags','auth_session_dev') }
        $buildArguments += './internal/store'
        & go @buildArguments
        if ($LASTEXITCODE -ne 0) { throw ('Store build failed: ' + $build) }
        & $binary '-test.run=^TestCapturePauseGuard' '-test.count=1' '-test.timeout=60s' '-test.v=true'
        if ($LASTEXITCODE -ne 0) { throw ('Common pause guard failed: ' + $build) }
    }
    $env:AKUSIDECAPTUREPAUSE_QA_ROOT = $taskRoot
    $stages = @(
        @{ Build = 'dev'; Stage = 'dev-pause' },
        @{ Build = 'default'; Stage = 'default-block' },
        @{ Build = 'fixture'; Stage = 'fixture-inspect' },
        @{ Build = 'fixture'; Stage = 'fixture-resume' },
        @{ Build = 'default'; Stage = 'default-claim' }
    )
    foreach ($stage in $stages) {
        $env:AKUSIDECAPTUREPAUSE_QA_STAGE = $stage.Stage
        Write-Output ('CROSS_BUILD_STAGE=' + $stage.Build + '/' + $stage.Stage)
        $binary = Join-Path $taskRoot ('store-' + $stage.Build + '.test.exe')
        & $binary '-test.run=^TestCapturePauseGuardCrossBuildStage$' '-test.count=1' '-test.timeout=60s' '-test.v=true'
        if ($LASTEXITCODE -ne 0) { throw ('Cross-build stage failed: ' + $stage.Stage) }
    }
    $passed = $true
    Write-Output 'Capture pause compatibility passed: three Store builds, five separate processes, one synthetic SQLite database. No browser or Sidecar runtime launched.'
}
finally {
    Pop-Location
    foreach ($variable in $variables) { [Environment]::SetEnvironmentVariable($variable, $previous[$variable], 'Process') }
    if ($passed -and (Test-Path -LiteralPath $taskRoot)) {
        $resolved = (Resolve-Path -LiteralPath $taskRoot).ProviderPath
        $item = Get-Item -LiteralPath $resolved
        if ($resolved -ne $taskRoot -or [IO.Path]::GetDirectoryName($resolved) -ne $qaParent -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Refusing cleanup outside exact QA root.' }
        $reparse = Get-ChildItem -LiteralPath $resolved -Recurse -Force | Where-Object { $_.Attributes -band [IO.FileAttributes]::ReparsePoint }
        if ($reparse) { throw 'Refusing cleanup through a reparse point.' }
        Remove-Item -LiteralPath $resolved -Recurse -Force
        Write-Output 'Synthetic database and test binaries cleaned.'
    }
    elseif (!$passed) { Write-Warning ('Synthetic failure artifacts retained at ' + $taskRoot) }
}

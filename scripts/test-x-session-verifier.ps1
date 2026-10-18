param()
$ErrorActionPreference = 'Stop'
$projectRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$qaParent = Join-Path $projectRoot 'runtime\qa\auth-session-sidecar'
New-Item -ItemType Directory -Path $qaParent -Force | Out-Null
if ((Get-Item -LiteralPath $qaParent).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'QA parent must not be a reparse point.' }
$taskRoot = Join-Path $qaParent ('run-x-session-verifier-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $taskRoot | Out-Null
$cacheRoot = Join-Path $projectRoot '.go-build\x-session-verifier'
New-Item -ItemType Directory -Path $cacheRoot -Force | Out-Null
if ((Get-Item -LiteralPath $cacheRoot).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Go cache must not be a reparse point.' }
$moduleCache = Join-Path $projectRoot '.go-build\mod'
New-Item -ItemType Directory -Path $moduleCache -Force | Out-Null
if ((Get-Item -LiteralPath $moduleCache).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Go module cache must not be a reparse point.' }
$variables = @('TEMP','TMP','GOTMPDIR','GOCACHE','GOMODCACHE','GOPROXY','GOSUMDB','GOTOOLCHAIN','CGO_ENABLED','GOFLAGS')
$previous = @{}
foreach ($variable in $variables) { $previous[$variable] = [Environment]::GetEnvironmentVariable($variable, 'Process') }
$passed = $false
Push-Location -LiteralPath $projectRoot
try {
    $env:TEMP = $taskRoot
    $env:TMP = $taskRoot
    $env:GOTMPDIR = $taskRoot
    $env:GOCACHE = $cacheRoot
    $env:GOMODCACHE = $moduleCache
    $env:GOPROXY = 'off'
    $env:GOSUMDB = 'off'
    $env:GOTOOLCHAIN = 'local'
    $env:CGO_ENABLED = '0'
    $env:GOFLAGS = ''
    & go test -tags 'auth_session_dev auth_session_fixture' ./internal/sessionverifier/x -count=1 -timeout=30s -v
    if ($LASTEXITCODE -ne 0) { throw 'Offline X verifier regression tests failed.' }
    $package = 'github.com/abangkis/AkuSidecar/internal/sessionverifier/x'
    $dependencies = @(& go list -deps ./cmd/akusidecar)
    if ($LASTEXITCODE -ne 0) { throw 'Ordinary main dependency inspection failed.' }
    if ($dependencies -contains $package) { throw 'Fixture verifier unexpectedly linked into ordinary main.' }
    foreach ($tags in @('', 'auth_session_dev', 'auth_session_fixture')) {
        $arguments = @('list', '-e', '-json')
        if ($tags -ne '') { $arguments += @('-tags', $tags) }
        $arguments += './internal/sessionverifier/x'
        $metadata = (& go @arguments) | ConvertFrom-Json
        if ($LASTEXITCODE -ne 0 -or ($null -ne $metadata.GoFiles -and @($metadata.GoFiles).Count -ne 0) -or $metadata.ImportPath -ne $package -or $metadata.Error.Err -notmatch 'build constraints exclude all Go files') { throw 'Verifier must require both fixture and development tags.' }
    }
    $passed = $true
    Write-Output 'Offline X verifier passed; both tags required; no normal-main linkage, browser, account access or runtime operation.'
} finally {
    Pop-Location
    foreach ($variable in $variables) { [Environment]::SetEnvironmentVariable($variable, $previous[$variable], 'Process') }
    if ($passed) {
        $resolved = (Resolve-Path -LiteralPath $taskRoot).ProviderPath
        if ($resolved -ne $taskRoot -or [IO.Path]::GetDirectoryName($resolved) -ne $qaParent -or ((Get-Item -LiteralPath $resolved).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Refusing cleanup outside owned QA root.' }
        if (Get-ChildItem -LiteralPath $resolved -Recurse -Force | Where-Object { $_.Attributes -band [IO.FileAttributes]::ReparsePoint }) { throw 'Refusing cleanup through reparse point.' }
        Remove-Item -LiteralPath $resolved -Recurse -Force
        Write-Output 'Owned offline verifier temporary artifacts cleaned; project-local cache retained.'
    } else { Write-Warning ('Offline verifier artifacts retained at ' + $taskRoot) }
}

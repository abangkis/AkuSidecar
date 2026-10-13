param([switch]$VerifyDefault)
$ErrorActionPreference = 'Stop'
$projectRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$fixtureParent = Join-Path $projectRoot 'runtime\qa\auth-session-sidecar'
New-Item -ItemType Directory -Path $fixtureParent -Force | Out-Null
$fixtureParent = (Resolve-Path -LiteralPath $fixtureParent).ProviderPath
if ($fixtureParent -ne (Join-Path $projectRoot 'runtime\qa\auth-session-sidecar')) { throw 'Fixture parent escaped project scope.' }
$taskRoot = Join-Path $fixtureParent ('run-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $taskRoot | Out-Null
$previous = @{}
$variables = @('TEMP','TMP','GOTMPDIR','GOPROXY','GOSUMDB','GOTOOLCHAIN','CGO_ENABLED')
foreach ($variable in $variables) { $previous[$variable] = [Environment]::GetEnvironmentVariable($variable, 'Process') }
Push-Location -LiteralPath $projectRoot
try {
    $env:TEMP = $taskRoot
    $env:TMP = $taskRoot
    $env:GOTMPDIR = $taskRoot
    $env:GOPROXY = 'off'
    $env:GOSUMDB = 'off'
    $env:GOTOOLCHAIN = 'local'
    $env:CGO_ENABLED = '0'
    & go test -tags auth_session_fixture ./internal/engine ./internal/store ./internal/httpapi -run 'TestSyntheticExclusive|TestFixtureCapture' -count=1 -timeout=60s
    if ($LASTEXITCODE -ne 0) { throw 'Sidecar synthetic integration tests failed.' }
    if ($VerifyDefault) {
        & go test ./internal/captureruntime ./internal/collection ./internal/engine ./internal/store ./internal/httpapi -run 'Test.*(CaptureRuntime|CaptureGeneration|Hybrid|NativeReader|HeadlessNative|SwitchWaits|RetainedPopup|Collector|ClaimCommand|Borrow|IdleHold|Manager|Replacement|AdditionalHeadless|SplitCapture)' -count=1 -timeout=120s
        if ($LASTEXITCODE -ne 0) { throw 'Default capture/reader regression tests failed.' }
        & go build -o (Join-Path $taskRoot 'aku-sidecar-default-check.exe') ./cmd/akusidecar
        if ($LASTEXITCODE -ne 0) { throw 'Default Sidecar build failed.' }
    }
}
finally {
    Pop-Location
    foreach ($variable in $variables) { [Environment]::SetEnvironmentVariable($variable, $previous[$variable], 'Process') }
    if (Test-Path -LiteralPath $taskRoot) {
        $resolved = (Resolve-Path -LiteralPath $taskRoot).ProviderPath
        $item = Get-Item -LiteralPath $resolved
        if ($resolved -ne $taskRoot -or !$resolved.StartsWith($fixtureParent + '\', [StringComparison]::OrdinalIgnoreCase) -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Refusing fixture cleanup outside the verified output scope.' }
        try { Remove-Item -LiteralPath $resolved -Recurse -Force -ErrorAction Stop }
        catch { Write-Warning 'Fixture files remain locked; no process was stopped or forced closed.' }
    }
}

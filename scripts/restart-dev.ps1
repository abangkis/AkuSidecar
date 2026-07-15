param(
    [ValidateSet('user', 'codex')]
    [string] $Actor = 'user'
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
$workspaceRoot = Split-Path -Parent $repoRoot
$runtimeDir = Join-Path $repoRoot 'runtime\dev'
$target = Join-Path $runtimeDir 'aku-sidecar.exe'
$candidate = Join-Path $runtimeDir 'aku-sidecar.next.exe'
$supervisor = Join-Path $workspaceRoot 'AkuSupervisor\target\dev\aku-supervisor.exe'

if (-not (Test-Path -LiteralPath $supervisor -PathType Leaf)) {
    throw "AkuSupervisor development executable was not found: $supervisor"
}

$activeResponse = $null
try {
    $activeResponse = Invoke-RestMethod `
        -Uri 'http://127.0.0.1:47821/api/sessions/active' `
        -Method Get `
        -TimeoutSec 2
}
catch {
    # A stopped or unavailable Sidecar has no session that can be interrupted.
}

if ($null -ne $activeResponse -and $null -ne $activeResponse.session) {
    throw "AkuSidecar has an active session. Finish or cancel it before rebuilding."
}

& (Join-Path $PSScriptRoot 'build-dev.ps1') -OutputName 'aku-sidecar.next.exe'
if ($LASTEXITCODE -ne 0) {
    throw "AkuSidecar candidate build failed."
}

& $supervisor stop akusidecar --actor $Actor --reason 'explicit Sidecar development rebuild'
if ($LASTEXITCODE -ne 0) {
    throw "AkuSupervisor could not stop akusidecar. Candidate remains at $candidate"
}

Move-Item -LiteralPath $candidate -Destination $target -Force

& $supervisor start akusidecar --actor $Actor --reason 'start explicitly rebuilt Sidecar'
if ($LASTEXITCODE -ne 0) {
    throw "AkuSupervisor could not start the rebuilt akusidecar service."
}

Write-Host 'AkuSidecar was rebuilt and restarted under AkuSupervisor ownership.'

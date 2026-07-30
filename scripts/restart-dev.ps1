param(
    [ValidateSet('user', 'codex')]
    [string] $Actor = 'user',
    [ValidateRange(0, 3600)]
    [int] $WaitForIdleSeconds = 900,
    [ValidateRange(1, 10)]
    [int] $PollSeconds = 2
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
$workspaceRoot = Split-Path -Parent $repoRoot
$runtimeDir = Join-Path $repoRoot 'runtime\dev'
$target = Join-Path $runtimeDir 'aku-sidecar.exe'
$candidate = Join-Path $runtimeDir 'aku-sidecar.next.exe'
$targetProvenance = "$target.runtime-state.json"
$candidateProvenance = "$candidate.runtime-state.json"
$supervisor = Join-Path $workspaceRoot 'AkuSupervisor\target\dev\aku-supervisor.exe'

if (-not (Test-Path -LiteralPath $supervisor -PathType Leaf)) {
    throw "AkuSupervisor development executable was not found: $supervisor"
}

& (Join-Path $PSScriptRoot 'build-dev.ps1') -OutputName 'aku-sidecar.next.exe'
if ($LASTEXITCODE -ne 0) {
    throw "AkuSidecar candidate build failed."
}
if (-not (Test-Path -LiteralPath $candidateProvenance -PathType Leaf)) {
    throw "AkuSidecar candidate provenance was not produced: $candidateProvenance"
}

$deadline = [DateTime]::UtcNow.AddSeconds($WaitForIdleSeconds)
$lastReason = ''
while ($true) {
    $ready = $true
    $reason = 'sidecar_unavailable'
    try {
        $readiness = Invoke-RestMethod `
            -Uri 'http://127.0.0.1:11122/api/runtime/update-readiness' `
            -Method Get `
            -TimeoutSec 3
        $ready = [bool]$readiness.ready
        $reason = [string]$readiness.reason
    }
    catch {
        try {
            # Compatibility fallback for a pre-readiness development binary.
            $activeResponse = Invoke-RestMethod `
                -Uri 'http://127.0.0.1:11122/api/sessions/active' `
                -Method Get `
                -TimeoutSec 2
            $ready = $null -eq $activeResponse.session
            $reason = if ($ready) { 'legacy_idle' } else { 'active_session' }
        }
        catch {
            # A stopped Sidecar is safe for Supervisor-owned replacement.
            $ready = $true
            $reason = 'sidecar_unavailable'
        }
    }
    if ($ready) {
        break
    }
    if ($WaitForIdleSeconds -eq 0 -or [DateTime]::UtcNow -ge $deadline) {
        throw "AkuSidecar did not become update-ready within $WaitForIdleSeconds seconds (reason: $reason). Candidate remains at $candidate"
    }
    if ($reason -ne $lastReason) {
        Write-Host "Waiting for AkuSidecar development runtime to become idle: $reason" -ForegroundColor Yellow
        $lastReason = $reason
    }
    Start-Sleep -Seconds $PollSeconds
}

& $supervisor stop akusidecar --actor $Actor --reason 'explicit Sidecar development rebuild'
if ($LASTEXITCODE -ne 0) {
    throw "AkuSupervisor could not stop akusidecar. Candidate remains at $candidate"
}

Move-Item -LiteralPath $candidate -Destination $target -Force
Move-Item -LiteralPath $candidateProvenance -Destination $targetProvenance -Force
$promotedProvenance = Get-Content -LiteralPath $targetProvenance -Raw | ConvertFrom-Json
$promotedProvenance.binaryFile = 'aku-sidecar.exe'
$promotedProvenance | Add-Member -NotePropertyName promotedAtUtc -NotePropertyValue ([DateTime]::UtcNow.ToString('o'))
$temporaryTargetProvenance = "$targetProvenance.tmp"
[IO.File]::WriteAllText(
    $temporaryTargetProvenance,
    ($promotedProvenance | ConvertTo-Json -Depth 5),
    [Text.UTF8Encoding]::new($false)
)
Move-Item -LiteralPath $temporaryTargetProvenance -Destination $targetProvenance -Force

& $supervisor start akusidecar --actor $Actor --reason 'start explicitly rebuilt Sidecar'
if ($LASTEXITCODE -ne 0) {
    throw "AkuSupervisor could not start the rebuilt akusidecar service."
}

$expected = Get-Content -LiteralPath $targetProvenance -Raw | ConvertFrom-Json
$health = $null
for ($attempt = 0; $attempt -lt 80; $attempt++) {
    try {
        $health = Invoke-RestMethod -Uri 'http://127.0.0.1:11122/api/health' -TimeoutSec 1
        if ($health.status -eq 'ok') {
            break
        }
    }
    catch {}
    Start-Sleep -Milliseconds 250
}
if ($null -eq $health -or $health.status -ne 'ok') {
    throw 'AkuSidecar was restarted, but its health endpoint did not become ready.'
}
if ($health.version -ne $expected.version) {
    throw "AkuSidecar restarted with version $($health.version), but development provenance expects $($expected.version)."
}

Write-Host "AkuSidecar $($health.version) was rebuilt and restarted under AkuSupervisor ownership."

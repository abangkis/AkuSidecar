param(
    [ValidateSet('user', 'codex')] [string] $Actor = 'user',
    [ValidateRange(0, 3600)] [int] $WaitForIdleSeconds = 900,
    [ValidateRange(1, 10)] [int] $PollSeconds = 2,
    [switch] $ReplaceReaderBrokerRegistration,
    [switch] $Verify,
    [switch] $PlanOnly,
    [switch] $PrepareOnly
)
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
. (Join-Path $PSScriptRoot 'worker-backup-retention.ps1')
. (Join-Path $PSScriptRoot 'dev-runtime-contract.ps1')
$runtimeDir = Join-Path $repoRoot 'runtime\dev'
$supervisor = Join-Path (Split-Path -Parent $repoRoot) 'AkuSupervisor\target\dev\aku-supervisor.exe'
$supervisorConfig = if ($env:AKU_SUPERVISOR_CONFIG) { [IO.Path]::GetFullPath($env:AKU_SUPERVISOR_CONFIG) }
    elseif ($env:LOCALAPPDATA) { Join-Path $env:LOCALAPPDATA 'AkuSupervisor\services.json' }
    else { throw 'LOCALAPPDATA is unavailable; set AKU_SUPERVISOR_CONFIG explicitly.' }

# Resolve the real service before builds, registration, DB inspection or control.
$plan = Get-DevRuntimeServicePlan $repoRoot $supervisorConfig
if ($ReplaceReaderBrokerRegistration) { throw 'Native-host migration requires a separate approved handoff; restart-dev never replaces registration.' }
if ($PlanOnly) {
    [pscustomobject]@{ schema = 'aku.dev-restart-plan.v1'; target = $plan.Target; buildTags = @('auth_session_dev');
        configChanged = $false; registrationChanged = $false; processConsentGranted = $false; entries = @(Get-DevRuntimeEntries) } |
        ConvertTo-Json -Depth 5
    return
}
if (-not (Test-Path -LiteralPath $supervisor -PathType Leaf)) { throw 'AkuSupervisor development executable is missing.' }
$configHash = (Get-FileHash -LiteralPath $supervisorConfig -Algorithm SHA256).Hash
$applicationConfigHash = (Get-FileHash -LiteralPath $plan.ConfigPath -Algorithm SHA256).Hash
$null = Assert-WorkerBackupCopyReady -RuntimeRoot $runtimeDir -WorkerRoot (Join-Path $runtimeDir 'headless-worker') -BackupPrefix '.restart-dev-headless-worker-rollback-'
& (Join-Path $PSScriptRoot 'build-dev.ps1') -OutputName 'aku-sidecar.next.exe' -Verify:$Verify
if ($LASTEXITCODE -ne 0) { throw 'AkuSidecar candidate build failed.' }
$candidate = Join-Path $runtimeDir 'aku-sidecar.next.exe'
$expected = Assert-DevRuntimeBundle $runtimeDir -Candidate

# Preserve ordinary arguments, including actual database and port overrides.
# Diagnostics exit before capture resources, provider or application startup.
Push-Location -LiteralPath $plan.Service.cwd
try {
$probeText = & $candidate @($plan.Arguments) --runtime-candidate-probe --runtime-candidate-probe-schema=2 2>&1
if ($LASTEXITCODE -ne 0) { throw "Development config probe failed; runtime untouched: $($probeText | Out-String)" }
$probe = ($probeText | Out-String) | ConvertFrom-Json
if ($probe.status -ne 'ok') { throw 'Development candidate config probe was rejected.' }
$inspectionText = & $candidate @($plan.Arguments) --database-inspect 2>&1
if ($LASTEXITCODE -ne 0) { throw "Development database inspection failed; runtime untouched: $($inspectionText | Out-String)" }
$inspection = ($inspectionText | Out-String) | ConvertFrom-Json
if ($inspection.status -notin @('absent', 'current', 'migratable')) { throw "Development database needs an explicit compatibility decision: $($inspection.status). Runtime untouched." }
} finally { Pop-Location }
if ((Get-FileHash -LiteralPath $plan.ConfigPath -Algorithm SHA256).Hash -ne $applicationConfigHash) { throw 'Application config changed during preparation; candidate retained, runtime untouched.' }
if ($PrepareOnly) { Write-Output 'PASS: development candidate prepared and inspected; no registration or runtime change.'; return }

# Ordinary restarts reuse the stable host path; they never claim another host.
$expectedHost = Join-Path $runtimeDir 'com.akubrowser.reader_activation.json'
foreach ($vendor in @('Chromium', 'Google\Chrome')) {
    $key = "HKCU:\Software\$vendor\NativeMessagingHosts\com.akubrowser.reader_activation"
    if (-not (Test-Path -LiteralPath $key) -or
        -not ([string](Get-Item -LiteralPath $key).GetValue('')).Equals($expectedHost, [StringComparison]::OrdinalIgnoreCase)) {
        throw 'Native-host handoff required for runtime/dev. No registration or running process was changed.'
    }
}
function Get-RegisteredDevService {
    $text = & $supervisor status --json --config $supervisorConfig
    if ($LASTEXITCODE -ne 0) { throw 'Unable to verify Supervisor process ownership.' }
    $status = ($text | Out-String) | ConvertFrom-Json
    $service = @($status.services | Where-Object { $_.id -eq 'akusidecar' })
    if ($service.Count -ne 1) { throw 'Supervisor did not report exactly one AkuSidecar service.' }
    return $service[0]
}
$deadline = [DateTime]::UtcNow.AddSeconds($WaitForIdleSeconds)
$requestedShutdown = $false
$gracefulDeadline = $null
$observedProcesses = @{}
while ($true) {
    $service = Get-RegisteredDevService
    $owned = @($service.ownedPids | Where-Object { $_ })
    if (-not $service.rootPid -and $owned.Count -eq 0) { break }
    foreach ($ownedId in @($owned) + @($service.rootPid | Where-Object { $_ })) {
        $ownedProcess = Get-CimInstance Win32_Process -Filter "ProcessId=$ownedId"
        if ($ownedProcess) { $observedProcesses[[string]$ownedId] = $ownedProcess.CreationDate }
    }
    # Without an existing control token the user can close the UI gracefully.
    # Never invoke Supervisor Stop while it could escalate against a live tree.
    if (-not $requestedShutdown -and $env:AKU_RUNTIME_CONTROL_TOKEN) {
        $port = @($plan.Service.ports)[0]
        try {
            $null = Invoke-RestMethod -Uri "http://127.0.0.1:$port/api/runtime/shutdown-if-idle" -Method Post -ContentType 'application/json' -Body '{}' -Headers @{
                'X-Aku-Runtime-Control-Token' = $env:AKU_RUNTIME_CONTROL_TOKEN
            } -TimeoutSec 3
            $requestedShutdown = $true
            $gracefulDeadline = [DateTime]::UtcNow.AddSeconds(30)
        } catch { Write-Host 'Waiting for graceful idle shutdown; runtime control has not released the process.' }
    } elseif (-not $requestedShutdown) {
        Write-Host 'Close the AkuBrowser UI gracefully to release the development runtime. No force-stop will be attempted.'
        $requestedShutdown = $true
        $gracefulDeadline = [DateTime]::UtcNow.AddSeconds(30)
    }
    $expired = if ($gracefulDeadline) { [DateTime]::UtcNow -ge $gracefulDeadline } else { [DateTime]::UtcNow -ge $deadline }
    if ($expired) { throw 'Graceful shutdown did not release all owned processes; candidate retained, no promotion.' }
    Start-Sleep -Seconds $PollSeconds
}
# Check the OS, bound ports and reader pipe independently before promotion.
$remaining = @(Get-CimInstance Win32_Process | Where-Object { $_.ExecutablePath -and $_.ExecutablePath.Equals($plan.Target, [StringComparison]::OrdinalIgnoreCase) })
if ($remaining.Count -gt 0) { throw 'A development Sidecar process remains outside Supervisor ownership; refusing promotion.' }
foreach ($ownedId in $observedProcesses.Keys) {
    $ownedProcess = Get-CimInstance Win32_Process -Filter "ProcessId=$ownedId"
    if ($ownedProcess -and $ownedProcess.CreationDate -eq $observedProcesses[$ownedId]) { throw 'A previously owned browser/worker process remains; refusing promotion.' }
}
foreach ($port in @($plan.Service.ports)) {
    if (@(Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue).Count -gt 0) { throw 'Development port is still owned; refusing promotion.' }
}
$pipeRoot = [string]::Concat([char]92, [char]92, '.', [char]92, 'pipe', [char]92)
$readerPipe = $pipeRoot + 'AkuBrowser.reader-activation.' + @($plan.Service.ports)[0]
if ([IO.Directory]::GetFiles($pipeRoot) -contains $readerPipe) { throw 'Reader pipe is still owned; refusing promotion.' }
if ((Get-FileHash -LiteralPath $supervisorConfig -Algorithm SHA256).Hash -ne $configHash) { throw 'Supervisor configuration changed during preparation; refusing promotion.' }
if ((Get-FileHash -LiteralPath $plan.ConfigPath -Algorithm SHA256).Hash -ne $applicationConfigHash) { throw 'Application config changed before promotion; refusing promotion.' }
$null = Get-DevRuntimeServicePlan $repoRoot $supervisorConfig
# Normalize desired state only after independent no-process proof.
& $supervisor stop akusidecar --actor $Actor --reason 'normalize already exited development service' --config $supervisorConfig
if ($LASTEXITCODE -ne 0) { throw 'Supervisor did not accept stopped-state normalization.' }
$previousBundle = Invoke-DevRuntimePromotion $runtimeDir
& $supervisor start akusidecar --actor $Actor --reason 'start verified ordinary development bundle' --config $supervisorConfig
if ($LASTEXITCODE -ne 0) { throw "Supervisor could not start the development bundle. Previous code retained at $previousBundle" }
$health = $null
$port = @($plan.Service.ports)[0]
for ($attempt = 0; $attempt -lt 80; $attempt++) {
    try { $health = Invoke-RestMethod -Uri "http://127.0.0.1:$port/api/health" -TimeoutSec 1; if ($health.status -eq 'ok') { break } } catch {}
    Start-Sleep -Milliseconds 250
}
if ($null -eq $health -or $health.status -ne 'ok' -or $health.version -ne $expected.version) { throw "Development health/version check failed. Previous code retained at $previousBundle" }
$service = Get-RegisteredDevService
$process = Get-CimInstance Win32_Process -Filter "ProcessId=$($service.rootPid)"
if (-not $process -or -not ([string]$process.ExecutablePath).Equals($plan.Target, [StringComparison]::OrdinalIgnoreCase)) { throw 'Started service image does not match the promoted development runtime.' }
$null = Assert-DevRuntimeBundle $runtimeDir
Write-Host "AkuSidecar $($health.version) restarted from runtime/dev; previous complete bundle retained at $previousBundle. Process derivation consent was not granted."

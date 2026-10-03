$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot
$source = Get-Content -LiteralPath (Join-Path $repo 'scripts\restart-dev.ps1') -Raw
$start = $source.IndexOf('# Promote worker assets only after Supervisor stopped the existing runtime.')
$finish = $source.IndexOf('Move-Item -LiteralPath $candidate -Destination $target -Force', $start)
if ($start -lt 0 -or $finish -lt $start) { throw 'Production promotion block boundaries changed.' }
$promotion = [scriptblock]::Create($source.Substring($start, $finish - $start))
$fixture = Join-Path (Join-Path $repo 'build') ('dev-worker-promotion-' + [Guid]::NewGuid().ToString('n'))

# Execute the production filesystem block in an isolated directory, with old
# and candidate marker files. No service, binary or registered profile is used.
$runtimeDir = Join-Path $fixture 'success'
$targetWorker = Join-Path $runtimeDir 'headless-worker'
$candidateWorker = Join-Path $runtimeDir 'headless-worker.next'
New-Item -ItemType Directory -Path $targetWorker,$candidateWorker -Force | Out-Null
Set-Content -LiteralPath (Join-Path $targetWorker 'marker') -Value 'old'
Set-Content -LiteralPath (Join-Path $candidateWorker 'marker') -Value 'candidate'
. $promotion
if ((Get-Content -LiteralPath (Join-Path $targetWorker 'marker')).Trim() -ne 'candidate') { throw 'Candidate was not promoted.' }
if ((Get-Content -LiteralPath (Join-Path $previousWorker 'marker')).Trim() -ne 'old') { throw 'Previous worker was not retained.' }

$runtimeDir = Join-Path $fixture 'rollback'
$targetWorker = Join-Path $runtimeDir 'headless-worker'
$candidateWorker = Join-Path $runtimeDir 'missing-candidate'
New-Item -ItemType Directory -Path $targetWorker -Force | Out-Null
Set-Content -LiteralPath (Join-Path $targetWorker 'marker') -Value 'old'
$failed = $false
try { . $promotion } catch { $failed = $true }
if (-not $failed) { throw 'Missing candidate unexpectedly succeeded.' }
if ((Get-Content -LiteralPath (Join-Path $targetWorker 'marker')).Trim() -ne 'old') { throw 'Previous worker was not restored.' }
Write-Output 'PASS: actual promotion block preserves previous assets and restores them on failed candidate move; no runtime interruption.'

$buildSource = Get-Content -LiteralPath (Join-Path $repo 'scripts\build-dev.ps1') -Raw
$buildStart = $buildSource.IndexOf('    $priorWorkerStage =')
$buildFinish = $buildSource.IndexOf('    $workerProvenance.destinationDirectory =', $buildStart)
if ($buildStart -lt 0 -or $buildFinish -lt $buildStart) { throw 'Development staging promotion boundaries changed.' }
$buildPromotion = [scriptblock]::Create($buildSource.Substring($buildStart, $buildFinish - $buildStart))
foreach ($scenario in @('success', 'rollback')) {
    $workerDestination = Join-Path $fixture ('build-' + $scenario + '\headless-worker.next')
    $workerStage = Join-Path $fixture ('build-' + $scenario + '\staged-worker')
    New-Item -ItemType Directory -Path $workerDestination -Force | Out-Null
    Set-Content -LiteralPath (Join-Path $workerDestination 'marker') -Value 'old'
    if ($scenario -eq 'success') {
        New-Item -ItemType Directory -Path $workerStage -Force | Out-Null
        Set-Content -LiteralPath (Join-Path $workerStage 'marker') -Value 'candidate'
        . $buildPromotion
        if ((Get-Content -LiteralPath (Join-Path $workerDestination 'marker')).Trim() -ne 'candidate') { throw 'Staged worker not promoted.' }
        if ((Get-Content -LiteralPath (Join-Path $priorWorkerStage 'marker')).Trim() -ne 'old') { throw 'Previous staged worker not retained.' }
    } else {
        $failed = $false
        try { . $buildPromotion } catch { $failed = $true }
        if (-not $failed -or (Get-Content -LiteralPath (Join-Path $workerDestination 'marker')).Trim() -ne 'old') { throw 'Failed build staging did not restore old candidate.' }
    }
}
Write-Output 'PASS: development build staging promotion retains old candidate assets and restores them on failure.'

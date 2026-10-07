$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
. (Join-Path $PSScriptRoot 'worker-backup-retention.ps1')
$fixtureRoot = Join-Path $repoRoot ('build\worker-backup-retention-' + [Guid]::NewGuid().ToString('n'))
$fixtureCreated = $false

function Assert-True([bool] $Condition, [string] $Message) {
    if (-not $Condition) { throw $Message }
}

function New-SyntheticWorkerBundle([string] $Path) {
    New-Item -ItemType Directory -Path $Path -Force | Out-Null
    $nodePath = Join-Path $Path 'node.exe'
    $licensePath = Join-Path $Path 'LICENSE'
    $sidecarLicensePath = Join-Path $Path 'LICENSE-AkuSidecar'
    [IO.File]::WriteAllBytes($nodePath, [Text.Encoding]::UTF8.GetBytes('synthetic node payload'))
    [IO.File]::WriteAllText($licensePath, 'synthetic node license')
    [IO.File]::WriteAllText($sidecarLicensePath, 'synthetic Sidecar license')
    [IO.File]::WriteAllText((Join-Path $Path 'worker.mjs'), 'export const fixture = true;')
    $pin = [ordered]@{
        protocol = 1
        nodeSha256 = (Get-FileHash -Algorithm SHA256 -LiteralPath $nodePath).Hash.ToLowerInvariant()
        licenseFile = 'LICENSE'
        licenseSha256 = (Get-FileHash -Algorithm SHA256 -LiteralPath $licensePath).Hash.ToLowerInvariant()
    }
    [IO.File]::WriteAllText((Join-Path $Path 'node.pin.json'), ($pin | ConvertTo-Json -Depth 4), [Text.UTF8Encoding]::new($false))
}

$script:workerBackupProcessScanError = $null
$script:workerBackupProcessInventory = @([pscustomobject]@{
    Name = 'fixture.exe'
    ProcessId = 1
    ExecutablePath = 'C:\Windows\System32\fixture.exe'
    CommandLine = 'fixture.exe'
})
function Get-WorkerBackupProcessInventory {
    if ($null -ne $script:workerBackupProcessScanError) {
        throw $script:workerBackupProcessScanError
    }
    return @($script:workerBackupProcessInventory)
}

try {
    Assert-True (-not (Test-Path -LiteralPath $fixtureRoot)) "Worker retention fixture path already exists: $fixtureRoot"
    New-Item -ItemType Directory -Path $fixtureRoot -Force | Out-Null
    $fixtureCreated = $true

    $runtimeRoot = Join-Path $fixtureRoot 'build-runtime'
    $workerRoot = Join-Path $runtimeRoot 'headless-worker'
    New-Item -ItemType Directory -Path $runtimeRoot -Force | Out-Null
    New-SyntheticWorkerBundle $workerRoot
    $unownedBackup = Join-Path $runtimeRoot 'headless-worker.previous-legacy'
    New-SyntheticWorkerBundle $unownedBackup
    $buildPrefix = '.build-dev-headless-worker-rollback-'
    Assert-WorkerBackupCopyReady -RuntimeRoot $runtimeRoot -WorkerRoot $workerRoot -BackupPrefix $buildPrefix -MinimumFreeBytes 1
    Assert-True (Test-Path -LiteralPath $unownedBackup -PathType Container) 'Copy guard changed an old unowned backup.'

    $ownedBackup = Join-Path $runtimeRoot ($buildPrefix + [Guid]::NewGuid().ToString('n'))
    Move-Item -LiteralPath $workerRoot -Destination $ownedBackup
    $workerFileLock = [IO.File]::Open((Join-Path $ownedBackup 'worker.mjs'), [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::None)
    try {
        $cleanedWhileLocked = Remove-WorkerBackupAfterSuccess -RuntimeRoot $runtimeRoot -BackupPath $ownedBackup -BackupPrefix $buildPrefix
        Assert-True (-not $cleanedWhileLocked) 'Cleanup reported success while the worker backup was locked.'
        Assert-True (Test-Path -LiteralPath $ownedBackup -PathType Container) 'Cleanup removed a locked worker backup.'
        Assert-True $workerFileLock.CanRead 'Cleanup closed the locked file handle.'
    } finally {
        $workerFileLock.Dispose()
    }
    $cleanBackup = Join-Path $runtimeRoot ($buildPrefix + [Guid]::NewGuid().ToString('n'))
    New-SyntheticWorkerBundle $cleanBackup
    Assert-True (Remove-WorkerBackupAfterSuccess -RuntimeRoot $runtimeRoot -BackupPath $cleanBackup -BackupPrefix $buildPrefix) 'Cleanup did not remove an unlocked invocation-owned backup.'
    Assert-True (-not (Test-Path -LiteralPath $cleanBackup)) 'Owned backup remains after successful cleanup.'
    Assert-True (Test-Path -LiteralPath $unownedBackup -PathType Container) 'Cleanup touched an old unowned backup.'

    $activeBackup = Join-Path $runtimeRoot ($buildPrefix + [Guid]::NewGuid().ToString('n'))
    New-SyntheticWorkerBundle $activeBackup
    $script:workerBackupProcessInventory = @([pscustomobject]@{
        Name = 'node.exe'
        ProcessId = 42
        ExecutablePath = Join-Path $activeBackup 'node.exe'
        CommandLine = "node.exe `"$(Join-Path $activeBackup 'worker.mjs')`""
    })
    $activeCleanup = Remove-WorkerBackupAfterSuccess -RuntimeRoot $runtimeRoot -BackupPath $activeBackup -BackupPrefix $buildPrefix
    Assert-True (-not $activeCleanup) 'Cleanup proceeded while a process referenced the worker backup.'
    Assert-True (Test-Path -LiteralPath (Join-Path $activeBackup 'node.exe') -PathType Leaf) 'Active-process guard touched the backup before refusing cleanup.'
    Assert-True (Test-Path -LiteralPath (Join-Path $activeBackup 'worker.mjs') -PathType Leaf) 'Active-process guard partially removed the backup.'
    $commandPath = (Join-Path $activeBackup 'worker.mjs').Replace('\', '/')
    $script:workerBackupProcessInventory = @([pscustomobject]@{
        Name = 'node.exe'
        ProcessId = 43
        ExecutablePath = 'C:\Program Files\nodejs\node.exe'
        CommandLine = "node.exe `"$commandPath`""
    })
    $commandLineCleanup = Remove-WorkerBackupAfterSuccess -RuntimeRoot $runtimeRoot -BackupPath $activeBackup -BackupPrefix $buildPrefix
    Assert-True (-not $commandLineCleanup) 'Cleanup ignored a process command line that referenced the worker backup.'
    Assert-True (Test-Path -LiteralPath (Join-Path $activeBackup 'worker.mjs') -PathType Leaf) 'Command-line process guard touched the backup before refusing cleanup.'

    $unavailableBackup = Join-Path $runtimeRoot ($buildPrefix + [Guid]::NewGuid().ToString('n'))
    New-SyntheticWorkerBundle $unavailableBackup
    $script:workerBackupProcessScanError = 'fixture process scan unavailable'
    $unavailableCleanup = Remove-WorkerBackupAfterSuccess -RuntimeRoot $runtimeRoot -BackupPath $unavailableBackup -BackupPrefix $buildPrefix
    Assert-True (-not $unavailableCleanup) 'Cleanup proceeded after the process scanner failed.'
    Assert-True (Test-Path -LiteralPath (Join-Path $unavailableBackup 'worker.mjs') -PathType Leaf) 'Unavailable process scan did not preserve the backup.'
    $script:workerBackupProcessScanError = $null
    $script:workerBackupProcessInventory = @([pscustomobject]@{
        Name = 'fixture.exe'
        ProcessId = 1
        ExecutablePath = 'C:\Windows\System32\fixture.exe'
        CommandLine = 'fixture.exe'
    })

    $restartPrefix = '.restart-dev-headless-worker-rollback-'
    $sensitiveBackup = Join-Path $runtimeRoot ($restartPrefix + [Guid]::NewGuid().ToString('n'))
    New-SyntheticWorkerBundle $sensitiveBackup
    [IO.File]::WriteAllText((Join-Path $sensitiveBackup 'cookies.json'), '{"fixture":"sensitive-state"}')
    $cleanedSensitive = Remove-WorkerBackupAfterSuccess -RuntimeRoot $runtimeRoot -BackupPath $sensitiveBackup -BackupPrefix $restartPrefix
    Assert-True (-not $cleanedSensitive) 'Cleanup accepted a worker backup containing sensitive state.'
    Assert-True (Test-Path -LiteralPath (Join-Path $sensitiveBackup 'cookies.json') -PathType Leaf) 'Cleanup removed sensitive state.'
    $copyBlocked = $false
    try {
        Assert-WorkerBackupCopyReady -RuntimeRoot $runtimeRoot -WorkerRoot $workerRoot -BackupPrefix $restartPrefix -MinimumFreeBytes 1
    } catch {
        $copyBlocked = $true
    }
    Assert-True $copyBlocked 'Copy guard allowed another worker copy after cleanup was skipped.'
    $otherWorkflowBlocked = $false
    try {
        Assert-WorkerBackupCopyReady -RuntimeRoot $runtimeRoot -WorkerRoot $workerRoot -BackupPrefix $buildPrefix -MinimumFreeBytes 1
    } catch {
        $otherWorkflowBlocked = $true
    }
    Assert-True $otherWorkflowBlocked 'Copy guard allowed another workflow to copy while an owned backup remains.'

    $capacityRuntimeRoot = Join-Path $fixtureRoot 'capacity-runtime'
    $capacityWorkerRoot = Join-Path $capacityRuntimeRoot 'headless-worker'
    New-Item -ItemType Directory -Path $capacityRuntimeRoot -Force | Out-Null
    New-SyntheticWorkerBundle $capacityWorkerRoot
    $capacityBlocked = $false
    try {
        Assert-WorkerBackupCopyReady -RuntimeRoot $capacityRuntimeRoot -WorkerRoot $capacityWorkerRoot -BackupPrefix '.build-dev-headless-worker.next-rollback-' -MinimumFreeBytes ([long]::MaxValue)
    } catch {
        $capacityBlocked = $true
    }
    Assert-True $capacityBlocked 'Copy guard ignored an insufficient free-space limit.'

    $buildSource = Get-Content -LiteralPath (Join-Path $PSScriptRoot 'build-dev.ps1') -Raw
    $buildGuardIndex = $buildSource.IndexOf('Assert-WorkerBackupCopyReady')
    $buildStageIndex = $buildSource.IndexOf('$workerProvenanceText = & $workerStager')
    $buildProvenanceIndex = $buildSource.IndexOf('Move-Item -LiteralPath $temporaryProvenance -Destination $provenancePath')
    $buildCleanupIndex = $buildSource.IndexOf('Remove-WorkerBackupAfterSuccess -RuntimeRoot $runtimeDir -BackupPath $priorWorkerStage')
    Assert-True ($buildGuardIndex -ge 0 -and $buildGuardIndex -lt $buildStageIndex -and $buildProvenanceIndex -lt $buildCleanupIndex) 'build-dev must guard before worker staging and clean only after provenance promotion.'

    $restartSource = Get-Content -LiteralPath (Join-Path $PSScriptRoot 'restart-dev.ps1') -Raw
    $restartGuardIndex = $restartSource.IndexOf('Assert-WorkerBackupCopyReady')
    $restartBuildIndex = $restartSource.IndexOf("build-dev.ps1') -OutputName")
    $restartHealthyVersionIndex = $restartSource.IndexOf('if ($health.version -ne $expected.version)')
    $restartCleanupIndex = $restartSource.IndexOf('Remove-WorkerBackupAfterSuccess -RuntimeRoot $runtimeDir -BackupPath $previousWorker')
    Assert-True ($restartGuardIndex -ge 0 -and $restartGuardIndex -lt $restartBuildIndex -and $restartHealthyVersionIndex -lt $restartCleanupIndex) 'restart-dev must guard before candidate build and clean only after health/version validation.'

    Write-Output 'Worker backup retention fixture tests passed.'
} finally {
    if ($fixtureCreated -and (Test-Path -LiteralPath $fixtureRoot -PathType Container)) {
        Remove-Item -LiteralPath $fixtureRoot -Recurse -Force
    }
}

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'worker-backup-retention.ps1')
. (Join-Path $PSScriptRoot 'native-stage-retention.ps1')
$build = Join-Path (Split-Path -Parent $PSScriptRoot) 'build'
$fixture = Join-Path $build ('native-stage-retention-test-' + [Guid]::NewGuid().ToString('N'))
$script:checks = 0
$script:scanCount = 0
$script:processMode = 'clear'
function Assert-True([bool] $Value, [string] $Message) {
    $script:checks++
    if (-not $Value) { throw $Message }
}
function Get-WorkerBackupProcessInventory {
    $script:scanCount++
    if ($script:processMode -eq 'unavailable') { throw 'Synthetic inventory failure' }
    if ($script:processMode -eq 'busy' -or ($script:processMode -eq 'late' -and $script:scanCount -gt 1)) {
        return [pscustomobject]@{ProcessId=42;ExecutablePath='C:\fixture\node.exe';CommandLine=('node "' + $script:stage + '\x-daily-owner.previous\x-daily-owner.cjs"')}
    }
    return [pscustomobject]@{ProcessId=1;ExecutablePath='C:\fixture\idle.exe';CommandLine='idle'}
}
function New-Case {
    $script:scanCount = 0
    $script:processMode = 'clear'
    $script:stage = Join-Path $fixture ('dev-native-stage-' + [Guid]::NewGuid().ToString('N'))
    foreach ($name in @('main-browser-transport', 'x-daily-owner')) {
        $directory = Join-Path $script:stage ($name + '.previous')
        New-Item -ItemType Directory -Path $directory -Force | Out-Null
        $driver = Join-Path $directory ($name + '.cjs')
        [IO.File]::WriteAllText($driver, "'use strict'; // disposable fixture")
        $manifest = @{schema='aku.x.application-owner-package.v1';driver=@{file=($name + '.cjs');sha256=(Get-FileHash -LiteralPath $driver -Algorithm SHA256).Hash.ToLowerInvariant()}}
        [IO.File]::WriteAllText((Join-Path $directory 'manifest.json'), ($manifest | ConvertTo-Json))
    }
}
function Clean { return Remove-NativeStageAfterSuccess -BuildRoot $fixture -StagePath $script:stage -BuildSucceeded }
function Assert-Intact {
    Assert-True (@(Get-ChildItem -LiteralPath $script:stage -Recurse -File).Count -eq 4) 'Cleanup partially removed a deferred backup.'
}
try {
    New-Item -ItemType Directory -Path $fixture | Out-Null
    New-Case
    $historical = $script:stage
    New-Case
    Assert-True (Clean) 'Successful cleanup failed.'
    Assert-True (-not (Test-Path -LiteralPath $script:stage)) 'Successful stage remains.'
    Assert-True (Test-Path -LiteralPath $historical) 'Cleanup touched another invocation.'

    New-Case
    Assert-True (-not (Remove-NativeStageAfterSuccess -BuildRoot $fixture -StagePath $script:stage)) 'Failed/unconfirmed build allowed cleanup.'
    Assert-Intact
    foreach ($mode in @('busy', 'late', 'unavailable')) {
        New-Case
        $script:processMode = $mode
        Assert-True (-not (Clean)) "Cleanup ignored $mode process state."
        Assert-Intact
    }
    New-Case
    $locked = Join-Path $script:stage 'x-daily-owner.previous\x-daily-owner.cjs'
    $handle = [IO.File]::Open($locked, [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::Read)
    try {
        Assert-True (-not (Clean)) 'Cleanup ignored a real open file handle.'
        Assert-Intact
    } finally { $handle.Dispose() }
    Assert-True (Clean) 'Cleanup did not recover after handle release.'

    New-Case
    [IO.File]::AppendAllText((Join-Path $script:stage 'x-daily-owner.previous\x-daily-owner.cjs'), 'changed')
    Assert-True (-not (Clean)) 'Cleanup ignored a mismatched manifest.'
    Assert-Intact
    New-Case
    [IO.File]::WriteAllText((Join-Path $script:stage 'unexpected.txt'), 'retain')
    Assert-True (-not (Clean)) 'Cleanup accepted unexpected files.'
    Assert-True (@(Get-ChildItem -LiteralPath $script:stage -Recurse -File).Count -eq 5) 'Unexpected contents were removed.'

    New-Case
    Assert-True (-not (Remove-NativeStageAfterSuccess -BuildRoot $build -StagePath $script:stage -BuildSucceeded)) 'Cleanup escaped the exact parent boundary.'
    Assert-Intact
    $empty = Join-Path $fixture ('dev-native-stage-' + [Guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $empty | Out-Null
    Assert-True (Remove-NativeStageAfterSuccess -BuildRoot $fixture -StagePath $empty -BuildSucceeded) 'Empty first-build stage was not removed.'

    $linkStage = Join-Path $fixture ('dev-native-stage-' + [Guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $linkStage | Out-Null
    $link = Join-Path $linkStage 'x-daily-owner.previous'
    $target = Join-Path $historical 'x-daily-owner.previous'
    New-Item -ItemType Junction -Path $link -Target $target | Out-Null
    try {
        Assert-True (-not (Remove-NativeStageAfterSuccess -BuildRoot $fixture -StagePath $linkStage -BuildSucceeded)) 'Cleanup followed a junction.'
        Assert-True (@(Get-ChildItem -LiteralPath $target -File).Count -eq 2) 'Junction target was changed.'
    } finally { [IO.Directory]::Delete($link, $false) }

    # Execute the actual success-tail statements with fixture commands. A failed
    # provenance write must prevent reaching the cleanup hook.
    $source = Get-Content -LiteralPath (Join-Path $PSScriptRoot 'build-dev.ps1') -Raw
    $start = $source.IndexOf('    Move-Item -LiteralPath $temporaryProvenance')
    $end = $source.IndexOf('    if ($workerStageBackedUp)', $start)
    Assert-True ($start -ge 0 -and $end -gt $start) 'Build success hook was not found.'
    $tail = [scriptblock]::Create($source.Substring($start, $end - $start))
    function Move-Item { throw 'Synthetic provenance failure' }
    New-Case
    $temporaryProvenance = 'fixture'; $provenancePath = 'fixture'; $repoRoot = $fixture; $nativeStage = $script:stage
    $failed = $false
    try { & $tail } catch { $failed = $true }
    Assert-True $failed 'Build failure fixture did not fail.'
    Assert-Intact
    Write-Output "Native staging retention: $script:checks assertions passed (offline)."
} finally {
    $resolved = [IO.Path]::GetFullPath($fixture)
    if ([IO.Path]::GetDirectoryName($resolved) -ne [IO.Path]::GetFullPath($build) -or
        [IO.Path]::GetFileName($resolved) -notmatch '^native-stage-retention-test-[0-9a-f]{32}$') { throw 'Unsafe fixture cleanup path.' }
    if (Test-Path -LiteralPath $resolved) { Remove-Item -LiteralPath $resolved -Recurse -Force }
}

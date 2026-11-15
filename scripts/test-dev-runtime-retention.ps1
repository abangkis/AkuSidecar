param([string] $ResultPath)
$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot
. (Join-Path $PSScriptRoot 'worker-backup-retention.ps1')
. (Join-Path $PSScriptRoot 'dev-runtime-contract.ps1')
$build = Join-Path $repo 'build'
$fixture = Join-Path $build ('dev-runtime-retention-' + [Guid]::NewGuid().ToString('N'))
$script:checks = 0
$script:warnings = @()
$script:processes = @([pscustomobject]@{Name='fixture';ProcessId=1;ExecutablePath='C:\fixture.exe';CommandLine='fixture'})
$script:scanFails = $false
$script:startFails = $false
$script:healthFails = $false
$script:versionFails = $false
$script:processFails = $false
$script:tamperAfterStart = $false
$hadExit = $null -ne (Get-Variable LASTEXITCODE -Scope Global -ErrorAction SilentlyContinue)
$priorExit = $global:LASTEXITCODE
function Assert-True([bool] $Condition, [string] $Message) {
    $script:checks++
    if (-not $Condition) { throw $Message }
}
function Write-Warning([string] $Message) { $script:warnings += $Message }
function Get-WorkerBackupProcessInventory {
    if ($script:scanFails) { throw 'fixture process inventory unavailable' }
    return $script:processes
}
function New-Bundle([string] $Root, [string] $Marker, [switch] $Candidate) {
    New-Item -ItemType Directory -Path $Root -Force | Out-Null
    foreach ($entry in Get-DevRuntimeEntries) {
        $name = if ($Candidate) { $entry.Candidate } else { $entry.Target }
        $path = Join-Path $Root $name
        if ($entry.Target -in @('headless-worker','ui-reader-broker','main-browser-transport','x-daily-owner')) {
            New-Item -ItemType Directory -Path $path | Out-Null
            [IO.File]::WriteAllText((Join-Path $path 'fixture.mjs'), "export const marker = '$Marker';")
        } else { [IO.File]::WriteAllText($path, $Marker) }
    }
    $worker = Join-Path $Root $(if ($Candidate) { 'headless-worker.next' } else { 'headless-worker' })
    foreach ($name in @('node.exe','LICENSE','LICENSE-AkuSidecar')) { [IO.File]::WriteAllText((Join-Path $worker $name), "$Marker-$name") }
    $pin = [ordered]@{
        protocol=1;nodeSha256=(Get-FileHash -LiteralPath (Join-Path $worker 'node.exe')).Hash
        licenseFile='LICENSE';licenseSha256=(Get-FileHash -LiteralPath (Join-Path $worker 'LICENSE')).Hash
    }
    [IO.File]::WriteAllText((Join-Path $worker 'node.pin.json'), ($pin | ConvertTo-Json))
    $artifacts = @(Get-DevRuntimeInventory $Root -Candidate:$Candidate)
    $state = [ordered]@{
        version='fixture';bundleSchema='aku.dev-runtime-bundle.v1';buildTags=@('auth_session_dev')
        binaryFile=$(if ($Candidate) {'aku-sidecar.next.exe'} else {'aku-sidecar.exe'})
        binarySha256=($artifacts | Where-Object path -EQ 'aku-sidecar.exe').sha256
        headlessWorker=@{destinationDirectory=$worker};artifacts=$artifacts
    }
    $stateName = if ($Candidate) {'aku-sidecar.next.exe.runtime-state.json'} else {'aku-sidecar.exe.runtime-state.json'}
    [IO.File]::WriteAllText((Join-Path $Root $stateName), ($state | ConvertTo-Json -Depth 8))
}
function New-Case([string] $Name) {
    $root = Join-Path $fixture $Name
    New-Bundle $root 'active'
    # The kept GUID sorts first; selection must use KeepPath.
    $keep = Join-Path $root ('.dev-runtime-rollback-' + ('0' * 32))
    $old = Join-Path $root ('.dev-runtime-rollback-' + ('f' * 32))
    New-Bundle $keep 'keep'
    New-Bundle $old 'old'
    return [pscustomobject]@{root=$root;keep=$keep;old=$old;sha=(Assert-DevRuntimeBundle $root).binarySha256}
}
function Clean-Case($Case) {
    return Remove-DevRuntimeRollbacksAfterSuccess -RuntimeRoot $Case.root -KeepPath $Case.keep -VerifiedBinarySha256 $Case.sha
}
function Assert-Retained($Case, [string] $Message) {
    Assert-True (Test-Path -LiteralPath $Case.old -PathType Container) $Message
    Assert-True (Test-Path -LiteralPath (Join-Path $Case.old 'aku-sidecar.exe') -PathType Leaf) ($Message + ' (partial deletion)')
}
function Invoke-RestMethod {
    return [pscustomobject]@{status=$(if ($script:healthFails) {'bad'} else {'ok'});version=$(if ($script:versionFails) {'wrong'} else {'fixture'})}
}
function Start-Sleep {}
function Get-RegisteredDevService { return [pscustomobject]@{rootPid=123} }
function Get-CimInstance {
    return [pscustomobject]@{ExecutablePath=$(if ($script:processFails) {'C:\wrong.exe'} else {$script:workflowTarget})}
}
$workflow = Get-Content -LiteralPath (Join-Path $PSScriptRoot 'dev-restart-workflow.ps1') -Raw
$start = $workflow.IndexOf('$previousBundle = Invoke-DevRuntimePromotion')
$finish = $workflow.IndexOf('Write-Host "AkuSidecar ')
Assert-True ($start -gt 0 -and $finish -gt $start) 'Workflow activation/retention boundary is missing.'
$activation = [scriptblock]::Create($workflow.Substring($start, $finish - $start))
try {
    New-Item -ItemType Directory -Path $fixture | Out-Null
    $case = New-Case 'success'
    $keepBefore = (Get-DevRuntimeInventory $case.keep | ConvertTo-Json -Compress)
    Assert-True (Clean-Case $case) 'Valid old rollback was not cleaned.'
    Assert-True (-not (Test-Path -LiteralPath $case.old)) 'Old rollback remains.'
    Assert-True ((Get-DevRuntimeInventory $case.keep | ConvertTo-Json -Compress) -ceq $keepBefore) 'Kept rollback changed.'
    Assert-True ((Assert-DevRuntimeBundle $case.root).binarySha256 -eq $case.sha) 'Active runtime changed.'
    Assert-True (Clean-Case $case) 'Repeated cleanup is not idempotent.'

    foreach ($relative in @('capture.log','screenshot.png','report.json','source.go','headless-worker\capture.db','headless-worker\session.json','headless-worker\capture.jsonl')) {
        $case = New-Case ('evidence-' + [Guid]::NewGuid().ToString('N'))
        $path = Join-Path $case.old $relative
        [IO.File]::WriteAllText($path, 'must survive')
        Assert-True (-not (Clean-Case $case)) "Unexpected evidence accepted: $relative"
        Assert-Retained $case "Evidence-bearing rollback removed: $relative"
        Assert-True ((Get-Content -LiteralPath $path -Raw) -eq 'must survive') "Evidence changed: $relative"
    }
    $case = New-Case 'tampered'
    [IO.File]::AppendAllText((Join-Path $case.old 'headless-worker\fixture.mjs'), 'changed')
    Assert-True (-not (Clean-Case $case)) 'Tampered backup accepted.'
    Assert-Retained $case 'Tampered backup removed.'
    $case = New-Case 'invalid-keep'
    [IO.File]::AppendAllText((Join-Path $case.keep 'aku-sidecar.exe'), 'changed')
    Assert-True (-not (Clean-Case $case)) 'Invalid kept rollback accepted.'
    Assert-Retained $case 'Old backup deleted before validating the kept backup.'
    $case = New-Case 'runtime-changed'
    $case.sha = 'a' * 64
    Assert-True (-not (Clean-Case $case)) 'Unexpected active runtime hash accepted.'
    Assert-Retained $case 'Backup removed after runtime changed.'
    $case = New-Case 'invalid-path'
    $case.keep = Join-Path $case.root '..\outside'
    Assert-True (-not (Clean-Case $case)) 'Outside path accepted.'
    Assert-Retained $case 'Outside path request changed rollback.'

    $case = New-Case 'locked'
    $fileLock = [IO.File]::Open((Join-Path $case.old 'headless-worker\fixture.mjs'), [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::Read)
    try {
        Assert-True (-not (Clean-Case $case)) 'Locked rollback accepted.'
        Assert-Retained $case 'Locked backup partially deleted.'
    } finally { $fileLock.Dispose() }

    foreach ($reference in @('executable','command')) {
        $case = New-Case ('process-' + $reference)
        $script:processes = @([pscustomobject]@{
            Name='fixture';ProcessId=99
            ExecutablePath=$(if ($reference -eq 'executable') {Join-Path $case.old 'node.exe'} else {'C:\fixture.exe'})
            CommandLine=('fixture "' + (Join-Path $case.old 'headless-worker\fixture.mjs').Replace('\','/') + '"')
        })
        Assert-True (-not (Clean-Case $case)) "Process reference ignored: $reference"
        Assert-Retained $case "Referenced backup removed: $reference"
    }
    $script:processes = @([pscustomobject]@{Name='fixture';ProcessId=1;ExecutablePath='C:\fixture.exe';CommandLine='fixture'})
    $case = New-Case 'scan-failed'
    $script:scanFails = $true
    try {
        Assert-True (-not (Clean-Case $case)) 'Unavailable process inventory accepted.'
        Assert-Retained $case 'Backup removed without a process inventory.'
    } finally { $script:scanFails = $false }

    $case = New-Case 'unknown-name'
    $unowned = Join-Path $case.root '.dev-runtime-rollback-user-backup'
    New-Item -ItemType Directory -Path $unowned | Out-Null
    [IO.File]::WriteAllText((Join-Path $unowned 'report.json'), 'evidence')
    Assert-True (Clean-Case $case) 'Valid cleanup failed beside an unrelated folder.'
    Assert-True (Test-Path -LiteralPath (Join-Path $unowned 'report.json')) 'Unowned folder changed.'

    $case = New-Case 'junction'
    $external = Join-Path $fixture 'external-evidence'
    New-Item -ItemType Directory -Path $external | Out-Null
    [IO.File]::WriteAllText((Join-Path $external 'capture.png'), 'must survive')
    $link = Join-Path $case.old 'headless-worker\external'
    New-Item -ItemType Junction -Path $link -Target $external | Out-Null
    try {
        Assert-True (-not (Clean-Case $case)) 'Reparse child accepted.'
        Assert-Retained $case 'Reparse rollback removed.'
        Assert-True ((Get-Content -LiteralPath (Join-Path $external 'capture.png') -Raw) -eq 'must survive') 'External evidence changed.'
    } finally { [IO.Directory]::Delete($link) }

    # Run the actual promotion/start/verification/retention statements with
    # isolated bundles and fake process, HTTP and Supervisor boundaries.
    foreach ($failure in @('start','health','version','process','tamper','none')) {
        $case = New-Case ('workflow-' + $failure)
        New-Bundle $case.root 'candidate' -Candidate
        $runtimeDir = $case.root
        $script:workflowTarget = Join-Path $runtimeDir 'aku-sidecar.exe'
        $plan = [pscustomobject]@{Target=$script:workflowTarget;Service=[pscustomobject]@{ports=@(11122)}}
        $expected = Assert-DevRuntimeBundle $runtimeDir -Candidate
        $supervisorConfig = 'fixture-config'
        $Actor = 'codex'
        $ActivationMode = 'daily'
        $script:startFails = $failure -eq 'start'
        $script:healthFails = $failure -eq 'health'
        $script:versionFails = $failure -eq 'version'
        $script:processFails = $failure -eq 'process'
        $script:tamperAfterStart = $failure -eq 'tamper'
        $supervisor = {
            $global:LASTEXITCODE = $(if ($script:startFails) {1} else {0})
            if ($script:tamperAfterStart) { [IO.File]::AppendAllText($script:workflowTarget, 'tampered') }
        }
        $failed = $false
        $operation = Enter-DevRuntimeOperation $runtimeDir
        try { & $activation } catch { $failed = $true }
        finally { $operation.ReleaseMutex(); $operation.Dispose() }
        $backups = @(Get-ChildItem -LiteralPath $runtimeDir -Directory -Force -Filter '.dev-runtime-rollback-*')
        if ($failure -eq 'none') {
            Assert-True (-not $failed) 'Successful activation failed.'
            Assert-True ($backups.Count -eq 1 -and $backups[0].FullName -notin @($case.keep,$case.old)) 'Successful activation did not retain exactly its own previous bundle.'
        } else {
            Assert-True $failed "Activation failure was not observed: $failure"
            Assert-True ($backups.Count -eq 3) "Cleanup ran before verification passed: $failure"
        }
    }

    $case = New-Case 'mutex'
    $operation = Enter-DevRuntimeOperation $case.root
    try {
        $child = Join-Path $fixture 'lock-child.ps1'
        $childText = @'
param($Scripts, $Runtime)
. (Join-Path $Scripts 'worker-backup-retention.ps1')
. (Join-Path $Scripts 'dev-runtime-contract.ps1')
try {
    $lease = Enter-DevRuntimeOperation $Runtime
    $lease.ReleaseMutex()
    $lease.Dispose()
    exit 0
} catch { Write-Output $_.Exception.Message; exit 7 }
'@
        [IO.File]::WriteAllText($child, $childText)
        $shell = (Get-Process -Id $PID).Path
        $arguments = @('-NoProfile','-NonInteractive','-File',('"' + $child + '"'),'-Scripts',('"' + $PSScriptRoot + '"'),'-Runtime',('"' + $case.root + '"'))
        $process = Start-Process -FilePath $shell -ArgumentList $arguments -WindowStyle Hidden -Wait -PassThru -RedirectStandardOutput (Join-Path $fixture 'lock.stdout.log') -RedirectStandardError (Join-Path $fixture 'lock.stderr.log')
        Assert-True ($process.ExitCode -eq 7) 'Concurrent operation acquired the same runtime.'
    } finally { $operation.ReleaseMutex(); $operation.Dispose() }
    $operation = Enter-DevRuntimeOperation $case.root
    $operation.ReleaseMutex()
    $operation.Dispose()
    Assert-True (Clean-Case $case) 'Operation lock was not released.'

    $cleanupAt = $workflow.IndexOf('Remove-DevRuntimeRollbacksAfterSuccess')
    Assert-True ($cleanupAt -gt $workflow.LastIndexOf('$null = Assert-DevRuntimeBundle $runtimeDir')) 'Cleanup precedes final bundle verification.'
    Assert-True ($workflow.IndexOf('Enter-DevRuntimeOperation') -lt $workflow.IndexOf("build-dev.ps1') -OutputName")) 'Restart does not serialize candidate preparation.'
    Assert-True ($workflow.IndexOf('Enter-DevRuntimeOperation') -gt $workflow.IndexOf('if ($PlanOnly)')) 'PlanOnly takes a mutation lock.'
    $buildSource = Get-Content -LiteralPath (Join-Path $PSScriptRoot 'build-dev.ps1') -Raw
    Assert-True ($buildSource.IndexOf('Enter-DevRuntimeOperation') -lt $buildSource.IndexOf('if ($Verify)')) 'Standalone build is not serialized.'
    Assert-True ($script:warnings.Count -ge 10) 'Deferred cleanup did not explain skipped paths.'
    $result = [ordered]@{status='passed';assertions=$script:checks;deferredWarnings=$script:warnings.Count;workflowCases=6;processContention='passed'}
    if ($ResultPath) { [IO.File]::WriteAllText([IO.Path]::GetFullPath($ResultPath), ($result | ConvertTo-Json)) }
    $result | ConvertTo-Json -Compress
} finally {
    if ($hadExit) { $global:LASTEXITCODE = $priorExit } else { Remove-Variable LASTEXITCODE -Scope Global -ErrorAction SilentlyContinue }
    if ((Split-Path -Parent ([IO.Path]::GetFullPath($fixture))) -ne [IO.Path]::GetFullPath($build)) { throw 'Invalid fixture cleanup boundary.' }
    if (Test-Path -LiteralPath $fixture) { Remove-Item -LiteralPath $fixture -Recurse -Force }
}

param([string] $ResultPath)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$buildRoot = Join-Path $projectRoot 'build'
$fixtureRoot = Join-Path $buildRoot ('dev-runtime-compatibility-' + [Guid]::NewGuid().ToString('N'))
$fixtureCreated = $false
$script:assertionCount = 0
$script:skipCount = 0
$hadLastExitCode = $null -ne (Get-Variable -Name LASTEXITCODE -Scope Global -ErrorAction SilentlyContinue)
$previousLastExitCode = if ($hadLastExitCode) { $global:LASTEXITCODE } else { $null }

. (Join-Path $PSScriptRoot 'worker-backup-retention.ps1')
. (Join-Path $PSScriptRoot 'dev-runtime-contract.ps1')

function Assert-True([bool] $Condition, [string] $Message) {
    $script:assertionCount++
    if (-not $Condition) { throw $Message }
}

function Assert-Throws([scriptblock] $Action, [string] $Message) {
    $threw = $false
    try { & $Action | Out-Null } catch { $threw = $true }
    Assert-True $threw $Message
}

function ConvertTo-CompactJson([object] $Value) {
    return ConvertTo-Json -InputObject $Value -Depth 12 -Compress
}

function New-SyntheticService([string] $State, [object] $RootPid, [object[]] $OwnedPids, [string] $Id = 'akusidecar') {
    return [pscustomobject][ordered]@{
        id = $Id
        status = $State
        rootPid = $RootPid
        ownedPids = @($OwnedPids)
        fixtureMarker = 'synthetic-only'
    }
}

function New-SyntheticRuntimeBundle([string] $RuntimeRoot, [switch] $Candidate) {
    New-Item -ItemType Directory -Path $RuntimeRoot -Force | Out-Null
    $directoryEntries = @('ui-reader-broker', 'headless-worker', 'main-browser-transport', 'x-daily-owner')
    $unicodeDirectory = 'nested space # % ' + [char]0x00E9
    $unicodeFile = 'marker # % ' + [char]0x6F22 + '.fixture'

    foreach ($entry in Get-DevRuntimeEntries) {
        $name = if ($Candidate) { $entry.Candidate } else { $entry.Target }
        $path = Join-Path $RuntimeRoot $name
        if ($directoryEntries -contains $entry.Target) {
            New-Item -ItemType Directory -Path $path -Force | Out-Null
            [IO.File]::WriteAllText((Join-Path $path 'fixture.marker'), 'synthetic-bundle-marker')
            New-Item -ItemType Directory -Path (Join-Path $path 'subdir') -Force | Out-Null
            [IO.File]::WriteAllText((Join-Path $path 'subdir\nested.fixture'), 'synthetic-nested-marker')
            $unicodePath = Join-Path (Join-Path $path $unicodeDirectory) $unicodeFile
            New-Item -ItemType Directory -Path (Split-Path -Parent $unicodePath) -Force | Out-Null
            [IO.File]::WriteAllText($unicodePath, 'synthetic-unicode-marker')
        } else {
            [IO.File]::WriteAllText($path, 'synthetic-binary-marker')
        }
    }

    $artifacts = @(Get-DevRuntimeInventory $RuntimeRoot -Candidate:$Candidate)
    $binary = @($artifacts | Where-Object { $_.path -ceq 'aku-sidecar.exe' })
    if ($binary.Count -ne 1) { throw 'Synthetic inventory did not contain exactly one sidecar marker.' }
    $stateName = if ($Candidate) { 'aku-sidecar.next.exe.runtime-state.json' } else { 'aku-sidecar.exe.runtime-state.json' }
    $binaryFile = if ($Candidate) { 'aku-sidecar.next.exe' } else { 'aku-sidecar.exe' }
    $workerName = if ($Candidate) { 'headless-worker.next' } else { 'headless-worker' }
    $state = [ordered]@{
        bundleSchema = 'aku.dev-runtime-bundle.v1'
        buildTags = @('auth_session_dev')
        binaryFile = $binaryFile
        binarySha256 = $binary[0].sha256
        headlessWorker = [ordered]@{ destinationDirectory = (Join-Path $RuntimeRoot $workerName) }
        artifacts = $artifacts
    }
    [IO.File]::WriteAllText((Join-Path $RuntimeRoot $stateName), (ConvertTo-CompactJson $state), [Text.UTF8Encoding]::new($false))
}

function Get-CanonicalRuntimeStatus([object] $Service) {
    return [pscustomobject]@{ response = [pscustomobject]@{ services = @($Service) } }
}

function Test-ValidRuntimeStatus([object] $Status, [string] $Label) {
    $selected = Get-DevRuntimeServiceStatus $Status
    $expected = if ($null -ne $Status.response) { $Status.response.services[0] } else { $Status.services[0] }
    Assert-True ($null -ne $selected) "${Label}: the service was not returned."
    Assert-True ([string]$selected.id -ceq 'akusidecar') "${Label}: the exact service id changed."
    Assert-True ((ConvertTo-CompactJson $selected) -ceq (ConvertTo-CompactJson $expected)) "${Label}: the selected service was changed."
}

function Get-StatusObject([string] $State, [object] $RootPid, [object[]] $OwnedPids, [string] $Id = 'akusidecar') {
    return New-SyntheticService $State $RootPid $OwnedPids $Id
}

function Get-InventoryFingerprint([object[]] $Inventory) {
    $json = ConvertTo-CompactJson @($Inventory)
    $bytes = [System.Text.Encoding]::UTF8.GetBytes($json)
    $sha = [System.Security.Cryptography.SHA256]::Create()
    try { return ([System.BitConverter]::ToString($sha.ComputeHash($bytes))).Replace('-', '').ToLowerInvariant() }
    finally { $sha.Dispose() }
}

function Assert-ContainedFixtureForCleanup([string] $CandidatePath, [string] $ParentPath) {
    $parent = [IO.Path]::GetFullPath($ParentPath).TrimEnd([IO.Path]::DirectorySeparatorChar, [IO.Path]::AltDirectorySeparatorChar)
    $candidate = [IO.Path]::GetFullPath($CandidatePath).TrimEnd([IO.Path]::DirectorySeparatorChar, [IO.Path]::AltDirectorySeparatorChar)
    $boundary = $parent + [IO.Path]::DirectorySeparatorChar
    if (-not $candidate.StartsWith($boundary, [StringComparison]::OrdinalIgnoreCase)) {
        throw "Refusing recursive cleanup outside the task fixture parent: $candidate"
    }
}

try {
    if (-not (Test-Path -LiteralPath $buildRoot -PathType Container)) {
        New-Item -ItemType Directory -Path $buildRoot -Force | Out-Null
    }
    $fixtureRoot = [IO.Path]::GetFullPath($fixtureRoot)
    $buildRoot = [IO.Path]::GetFullPath($buildRoot)
    $null = Get-WorkerBackupCanonicalRuntimeRoot $buildRoot
    Assert-ContainedFixtureForCleanup $fixtureRoot $buildRoot
    New-Item -ItemType Directory -Path $fixtureRoot -ErrorAction Stop | Out-Null
    $fixtureCreated = $true

    # Relative-path mapping preserves special filename characters and rejects lexical escapes.
    $pathRoot = Join-Path $fixtureRoot 'relative path root'
    New-Item -ItemType Directory -Path $pathRoot -Force | Out-Null
    $unicodeDirectory = 'nested space # % ' + [char]0x00E9
    $unicodeFile = 'file space # % ' + [char]0x6F22 + '.txt'
    $relativeFile = Join-Path (Join-Path $pathRoot $unicodeDirectory) $unicodeFile
    New-Item -ItemType Directory -Path (Split-Path -Parent $relativeFile) -Force | Out-Null
    [IO.File]::WriteAllText($relativeFile, 'relative-path-fixture')
    $relative = Get-DevRuntimeRelativeFilePath $pathRoot $relativeFile
    $expectedRelative = $unicodeDirectory + '/' + $unicodeFile
    Assert-True ($relative -ceq $expectedRelative) 'Relative path conversion changed Unicode, spaces, #, %, or slash normalization.'

    $outsideFile = Join-Path $fixtureRoot 'outside.txt'
    [IO.File]::WriteAllText($outsideFile, 'outside-fixture')
    $traversalFile = Join-Path $pathRoot ('..\outside.txt')
    $siblingPath = Join-Path $fixtureRoot 'relative path root sibling'
    New-Item -ItemType Directory -Path $siblingPath -Force | Out-Null
    $siblingFile = Join-Path $siblingPath 'sibling.txt'
    [IO.File]::WriteAllText($siblingFile, 'sibling-fixture')
    Assert-Throws { Get-DevRuntimeRelativeFilePath $pathRoot $outsideFile } 'Relative path conversion accepted an outside absolute file.'
    Assert-Throws { Get-DevRuntimeRelativeFilePath $pathRoot $traversalFile } 'Relative path conversion accepted a traversal escape.'
    Assert-Throws { Get-DevRuntimeRelativeFilePath $pathRoot $siblingFile } 'Relative path conversion accepted a sibling-prefix path.'
    Assert-Throws { Get-DevRuntimeRelativeFilePath $pathRoot $pathRoot } 'Relative path conversion accepted a path equal to its directory.'

    # Both published Supervisor status envelopes are accepted, and the service is returned unchanged.
    $flatRunning = [pscustomobject]@{ services = @((Get-StatusObject 'running' 101 @(202, 303))) }
    $wrappedRunning = Get-CanonicalRuntimeStatus (Get-StatusObject 'running' 101 @(202))
    $flatStopped = [pscustomobject]@{ services = @((Get-StatusObject 'stopped' $null @())) }
    $wrappedStopped = Get-CanonicalRuntimeStatus (Get-StatusObject 'stopped' $null @())
    foreach ($case in @(
        @{ Name = 'flat running'; Value = $flatRunning },
        @{ Name = 'enveloped running'; Value = $wrappedRunning },
        @{ Name = 'flat stopped'; Value = $flatStopped },
        @{ Name = 'enveloped stopped'; Value = $wrappedStopped }
    )) {
        $statusInput = $case.Value
        if ($null -ne $statusInput.response) { Test-ValidRuntimeStatus $statusInput $case.Name }
        else {
            $selected = Get-DevRuntimeServiceStatus $statusInput
            Assert-True ([string]$selected.id -ceq 'akusidecar') "$($case.Name): the service was not returned."
            Assert-True ((ConvertTo-CompactJson $selected) -ceq (ConvertTo-CompactJson $statusInput.services[0])) "$($case.Name): the selected service was changed."
        }
    }
    $maximumPid = [uint32]::MaxValue
    $boundaryStatus = [pscustomobject]@{ services = @((Get-StatusObject 'running' $maximumPid @([uint32]1, $maximumPid))) }
    $boundaryStatus = ConvertFrom-Json (ConvertTo-CompactJson $boundaryStatus)
    Assert-True ([string](Get-DevRuntimeServiceStatus $boundaryStatus).rootPid -ceq [string]$maximumPid) 'PID uint32 boundaries were not preserved.'

    # Malformed and ambiguous envelopes fail closed.
    $validService = Get-StatusObject 'stopped' $null @()
    $invalidStatuses = @(
        @{ Name = 'missing services'; Value = [pscustomobject]@{} },
        @{ Name = 'non-array services'; Value = [pscustomobject]@{ services = 'invalid' } },
        @{ Name = 'null services'; Value = [pscustomobject]@{ services = $null } },
        @{ Name = 'malformed service item'; Value = [pscustomobject]@{ services = @('invalid') } },
        @{ Name = 'missing canonical services'; Value = [pscustomobject]@{ response = [pscustomobject]@{} } },
        @{ Name = 'non-array canonical services'; Value = [pscustomobject]@{ response = [pscustomobject]@{ services = 'invalid' } } },
        @{ Name = 'null canonical response'; Value = [pscustomobject]@{ response = $null } },
        @{ Name = 'ambiguous flat and canonical envelopes'; Value = [pscustomobject]@{ services = @($validService); response = [pscustomobject]@{ services = @($validService) } } },
        @{ Name = 'missing target service'; Value = [pscustomobject]@{ services = @((Get-StatusObject 'stopped' $null @() 'other')) } },
        @{ Name = 'duplicate target services'; Value = [pscustomobject]@{ services = @($validService, $validService) } },
        @{ Name = 'non-exact target id'; Value = [pscustomobject]@{ services = @((Get-StatusObject 'stopped' $null @() 'AkuSidecar')) } },
        @{ Name = 'missing id'; Value = [pscustomobject]@{ services = @([pscustomobject]@{ status = 'stopped'; rootPid = $null; ownedPids = @() }) } },
        @{ Name = 'missing rootPid'; Value = [pscustomobject]@{ services = @([pscustomobject]@{ id = 'akusidecar'; ownedPids = @() }) } },
        @{ Name = 'missing ownedPids'; Value = [pscustomobject]@{ services = @([pscustomobject]@{ id = 'akusidecar'; rootPid = $null }) } },
        @{ Name = 'null ownedPids'; Value = [pscustomobject]@{ services = @([pscustomobject]@{ id = 'akusidecar'; rootPid = $null; ownedPids = $null }) } },
        @{ Name = 'non-array ownedPids'; Value = [pscustomobject]@{ services = @([pscustomobject]@{ id = 'akusidecar'; status = 'running'; rootPid = 12; ownedPids = 12 }) } },
        @{ Name = 'non-integer rootPid'; Value = [pscustomobject]@{ services = @((Get-StatusObject 'running' 12.5 @())) } },
        @{ Name = 'boolean rootPid'; Value = [pscustomobject]@{ services = @((Get-StatusObject 'running' $true @())) } },
        @{ Name = 'string rootPid'; Value = [pscustomobject]@{ services = @((Get-StatusObject 'running' '12' @())) } },
        @{ Name = 'zero rootPid'; Value = [pscustomobject]@{ services = @((Get-StatusObject 'running' 0 @())) } },
        @{ Name = 'negative rootPid'; Value = [pscustomobject]@{ services = @((Get-StatusObject 'running' -1 @())) } },
        @{ Name = 'oversized rootPid'; Value = [pscustomobject]@{ services = @((Get-StatusObject 'running' ([long]4294967296) @())) } },
        @{ Name = 'fractional ownedPid'; Value = [pscustomobject]@{ services = @((Get-StatusObject 'running' $null @(12.5))) } },
        @{ Name = 'boolean ownedPid'; Value = [pscustomobject]@{ services = @((Get-StatusObject 'running' $null @($true))) } },
        @{ Name = 'string ownedPid'; Value = [pscustomobject]@{ services = @((Get-StatusObject 'running' $null @('12'))) } },
        @{ Name = 'zero ownedPid'; Value = [pscustomobject]@{ services = @((Get-StatusObject 'running' $null @(0))) } },
        @{ Name = 'negative ownedPid'; Value = [pscustomobject]@{ services = @((Get-StatusObject 'running' $null @(-1))) } },
        @{ Name = 'oversized ownedPid'; Value = [pscustomobject]@{ services = @((Get-StatusObject 'running' $null @([long]4294967296))) } }
    )
    foreach ($case in $invalidStatuses) {
        $statusInput = $case.Value
        $label = [string]$case.Name
        Assert-Throws { Get-DevRuntimeServiceStatus $statusInput } "Service status parser accepted $label."
    }

    # Invoke only the actual consumer function from the workflow AST; never run its top-level build/restart body.
    $workflowPath = Join-Path $PSScriptRoot 'dev-restart-workflow.ps1'
    $parseTokens = $null
    $parseErrors = $null
    $workflowAst = [System.Management.Automation.Language.Parser]::ParseFile($workflowPath, [ref]$parseTokens, [ref]$parseErrors)
    if ($parseErrors.Count -gt 0) { throw 'Restart workflow has parse errors; cannot extract its status consumer.' }
    $consumerAst = @($workflowAst.FindAll({ param($node) $node -is [System.Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -ceq 'Get-RegisteredDevService' }, $true))
    if ($consumerAst.Count -ne 1) { throw 'Restart workflow did not contain exactly one Get-RegisteredDevService function.' }
    Invoke-Expression $consumerAst[0].Extent.Text
    $supervisorConfig = Join-Path $fixtureRoot 'synthetic-services.json'
    $script:mockSupervisorJson = ConvertTo-CompactJson $wrappedRunning
    $script:mockSupervisorExitCode = 0
    $script:mockSupervisorCalls = New-Object System.Collections.ArrayList
    $supervisor = {
        [void]$script:mockSupervisorCalls.Add(@($args | ForEach-Object { [string]$_ }))
        $global:LASTEXITCODE = $script:mockSupervisorExitCode
        Write-Output $script:mockSupervisorJson
    }
    $consumerResult = Get-RegisteredDevService
    Assert-True ([string]$consumerResult.id -ceq 'akusidecar') 'Registered service consumer did not return the parsed service.'
    Assert-True ($script:mockSupervisorCalls.Count -eq 1 -and (($script:mockSupervisorCalls[0] -join ' ') -ceq "status --json --config $supervisorConfig")) 'Registered service consumer did not issue the expected read-only status command.'
    $script:mockSupervisorJson = ConvertTo-CompactJson $flatStopped
    $consumerResult = Get-RegisteredDevService
    Assert-True ($null -eq $consumerResult.rootPid -and @($consumerResult.ownedPids).Count -eq 0) 'Registered service consumer rejected the flat stopped response.'
    $script:mockSupervisorJson = '{"services":[]}'
    Assert-Throws { Get-RegisteredDevService } 'Registered service consumer accepted a status with no target service.'
    $script:mockSupervisorExitCode = 1
    Assert-Throws { Get-RegisteredDevService } 'Registered service consumer accepted a failed CLI status command.'

    # Marker-only active and candidate bundles exercise hash inventory and provenance parity without real executables.
    $activeRoot = Join-Path $fixtureRoot 'active-runtime'
    $candidateRoot = Join-Path $fixtureRoot 'candidate-runtime'
    New-SyntheticRuntimeBundle $activeRoot
    New-SyntheticRuntimeBundle $candidateRoot -Candidate
    $activeState = Assert-DevRuntimeBundle $activeRoot
    $candidateState = Assert-DevRuntimeBundle $candidateRoot -Candidate
    $activeInventory = @(Get-DevRuntimeInventory $activeRoot)
    $candidateInventory = @(Get-DevRuntimeInventory $candidateRoot -Candidate)
    $activeInventoryJson = ConvertTo-CompactJson $activeInventory
    $candidateInventoryJson = ConvertTo-CompactJson $candidateInventory
    Assert-True ($activeInventoryJson -ceq $candidateInventoryJson) 'Active and candidate marker inventories differ.'
    Assert-True ($activeState.binarySha256 -ceq $candidateState.binarySha256) 'Active and candidate marker binary hashes differ.'
    Assert-True ($activeState.artifacts.Count -eq $candidateState.artifacts.Count) 'Active and candidate provenance artifact counts differ.'

    # The promotion rewrite must also retain UTF-8 names and exact provenance.
    New-SyntheticRuntimeBundle $activeRoot -Candidate
    $backupRoot = Invoke-DevRuntimePromotion $activeRoot
    $promoted = Assert-DevRuntimeBundle $activeRoot
    $backup = Assert-DevRuntimeBundle $backupRoot
    Assert-True ((ConvertTo-CompactJson @(Get-DevRuntimeInventory $activeRoot)) -ceq $activeInventoryJson) 'Promotion changed Unicode inventory paths or hashes.'
    Assert-True ($promoted.binaryFile -ceq 'aku-sidecar.exe') 'Promotion did not normalize the active binary name.'
    Assert-True ($promoted.headlessWorker.destinationDirectory -ceq (Join-Path $activeRoot 'headless-worker')) 'Promotion retained a candidate worker destination.'
    Assert-True ($backup.binarySha256 -ceq $activeState.binarySha256) 'Promotion did not preserve the previous provenance.'

    # Reparse-point coverage is opportunistic because Windows may require Developer Mode or elevation to create a link.
    $linkRoot = Join-Path $fixtureRoot 'reparse-runtime'
    New-SyntheticRuntimeBundle $linkRoot
    $linkSource = Join-Path (Join-Path $linkRoot 'headless-worker') 'fixture.marker'
    $linkPath = Join-Path (Join-Path $linkRoot 'headless-worker') 'synthetic-link.marker'
    try {
        New-Item -ItemType SymbolicLink -Path $linkPath -Target $linkSource -ErrorAction Stop | Out-Null
        Assert-Throws { Get-DevRuntimeInventory $linkRoot } 'Runtime inventory accepted a synthetic reparse point.'
    } catch {
        if ($_.Exception.Message -eq 'Runtime inventory accepted a synthetic reparse point.') { throw }
        $script:skipCount++
    }

    $inventoryFingerprint = Get-InventoryFingerprint $activeInventory
    $result = [ordered]@{
        schema = 'aku.dev-runtime-compatibility.v1'
        inventorySha256 = $inventoryFingerprint
        activeInventory = $activeInventory
        candidateInventory = $candidateInventory
        inventoriesMatch = $true
        activeBinarySha256 = $activeState.binarySha256
        candidateBinarySha256 = $candidateState.binarySha256
        provenanceMatches = $true
        assertions = $script:assertionCount
        skippedReparseCheck = ($script:skipCount -gt 0)
    }
    $resultJson = ConvertTo-Json -InputObject $result -Depth 12
    if (-not [string]::IsNullOrWhiteSpace($ResultPath)) {
        $resultFullPath = [IO.Path]::GetFullPath($ResultPath)
        $resultParent = Split-Path -Parent $resultFullPath
        if (-not (Test-Path -LiteralPath $resultParent -PathType Container)) { throw 'ResultPath parent directory does not exist.' }
        [IO.File]::WriteAllText($resultFullPath, $resultJson, [Text.UTF8Encoding]::new($false))
    }
    Write-Output ("PASS: development runtime compatibility checks passed ({0} assertions; inventory SHA-256 {1}; skipped reparse checks {2})." -f $script:assertionCount, $inventoryFingerprint, $script:skipCount)
    if (-not [string]::IsNullOrWhiteSpace($ResultPath)) { Write-Output "Result JSON: $resultFullPath" }
} finally {
    if ($hadLastExitCode) { $global:LASTEXITCODE = $previousLastExitCode }
    else { Remove-Variable -Name LASTEXITCODE -Scope Global -ErrorAction SilentlyContinue }
    if ($fixtureCreated -and (Test-Path -LiteralPath $fixtureRoot)) {
        $null = Get-WorkerBackupCanonicalRuntimeRoot $buildRoot
        Assert-ContainedFixtureForCleanup $fixtureRoot $buildRoot
        Remove-Item -LiteralPath $fixtureRoot -Recurse -Force
    }
}

param([string] $DiagnosticImage)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
. (Join-Path $PSScriptRoot 'worker-backup-retention.ps1')
. (Join-Path $PSScriptRoot 'dev-runtime-contract.ps1')
$buildRoot = Join-Path $repoRoot 'build'
$fixtureRoot = Join-Path $buildRoot ('dev-runtime-contract-' + [Guid]::NewGuid().ToString('N'))
$fixtureCreated = $false
$prepareEvidencePath = Join-Path $buildRoot ('dev-runtime-prepare-evidence-' + [Guid]::NewGuid().ToString('N') + '.log')
$script:prepareEvidenceEntries = [System.Collections.Generic.List[string]]::new()
if ([string]::IsNullOrWhiteSpace($DiagnosticImage)) {
    throw 'Provide -DiagnosticImage, or run scripts/test-stable-dev.ps1 to build the offline diagnostic images.'
}
$DiagnosticImage = [IO.Path]::GetFullPath($DiagnosticImage)
if (-not (Test-Path -LiteralPath $DiagnosticImage -PathType Leaf)) { throw "Diagnostic image is missing: $DiagnosticImage" }
$script:prepareEvidenceEntries.Add("Diagnostic image: $DiagnosticImage")
$script:prepareEvidenceEntries.Add("Diagnostic SHA-256: $((Get-FileHash -LiteralPath $DiagnosticImage -Algorithm SHA256).Hash.ToLowerInvariant())")

function Assert-True([bool] $Condition, [string] $Message) {
    if (-not $Condition) { throw $Message }
}

function New-SyntheticBundle([string] $RuntimeRoot, [switch] $Candidate, [string] $Marker) {
    New-Item -ItemType Directory -Path $RuntimeRoot -Force | Out-Null
    foreach ($entry in Get-DevRuntimeEntries) {
        $name = if ($Candidate) { $entry.Candidate } else { $entry.Target }
        $path = Join-Path $RuntimeRoot $name
        if ($name -match '^(headless-worker|ui-reader-broker|main-browser-transport|x-daily-owner)(\.next)?$') {
            New-Item -ItemType Directory -Path $path -Force | Out-Null
            [IO.File]::WriteAllText((Join-Path $path 'fixture.marker'), $Marker)
            New-Item -ItemType Directory -Path (Join-Path $path 'subdir') -Force | Out-Null
            [IO.File]::WriteAllText((Join-Path $path 'subdir\nested.fixture'), "nested-$Marker")
        } else {
            [IO.File]::WriteAllText($path, $Marker)
        }
    }
    $artifacts = @(Get-DevRuntimeInventory $RuntimeRoot -Candidate:$Candidate)
    $binary = $artifacts | Where-Object path -eq 'aku-sidecar.exe'
    $stateName = if ($Candidate) { 'aku-sidecar.next.exe.runtime-state.json' } else { 'aku-sidecar.exe.runtime-state.json' }
    $binaryFile = if ($Candidate) { 'aku-sidecar.next.exe' } else { 'aku-sidecar.exe' }
    $state = [ordered]@{
        bundleSchema = 'aku.dev-runtime-bundle.v1'
        buildTags = @('auth_session_dev')
        binaryFile = $binaryFile
        binarySha256 = $binary.sha256
        headlessWorker = [ordered]@{ destinationDirectory = (Join-Path $RuntimeRoot $(if ($Candidate) { 'headless-worker.next' } else { 'headless-worker' })) }
        artifacts = $artifacts
    }
    [IO.File]::WriteAllText((Join-Path $RuntimeRoot $stateName), ($state | ConvertTo-Json -Depth 8), [Text.UTF8Encoding]::new($false))
}

function Get-SyntheticPayload([string] $Path) {
    $item = Get-Item -LiteralPath $Path
    if ($item.PSIsContainer) { return Get-Content -LiteralPath (Join-Path $Path 'fixture.marker') -Raw }
    return Get-Content -LiteralPath $Path -Raw
}

function Write-ServiceConfig([string] $Path, [string] $Repository, [string] $Command, [string[]] $Arguments, [int] $Port) {
    $configuration = [ordered]@{ services = [ordered]@{ akusidecar = [ordered]@{
        command = $Command; cwd = $Repository; args = @($Arguments); ports = @($Port)
    } } }
    [IO.File]::WriteAllText($Path, ($configuration | ConvertTo-Json -Depth 8), [Text.UTF8Encoding]::new($false))
}

function go {
    if (-not $global:AkuSidecarDevRuntimeMockBuildEnabled) { throw 'Unexpected go invocation outside the copied build fixture.' }
    $call = @($args | ForEach-Object { [string]$_ })
    $global:AkuSidecarDevRuntimeMockGoCalls.Add(($call -join ' '))
    $outputIndex = [Array]::IndexOf($call, '-o')
    if ($outputIndex -lt 0 -or $outputIndex + 1 -ge $call.Count) { throw "Mock go received no output path: $($call -join ' ')" }
    Copy-Item -LiteralPath $global:AkuSidecarDevRuntimeMockBuildImage -Destination $call[$outputIndex + 1] -Force
    $global:LASTEXITCODE = 0
}
function git {
    if (-not $global:AkuSidecarDevRuntimeMockBuildEnabled) { throw 'Unexpected git invocation outside the copied build fixture.' }
    $call = @($args | ForEach-Object { [string]$_ })
    $global:AkuSidecarDevRuntimeMockGitCalls.Add(($call -join ' '))
    $global:LASTEXITCODE = 0
    if ($call -contains 'rev-parse') { Write-Output 'f0123456789abcdef0123456789abcdef0123456' }
}

function New-PrepareOnlyFixture([string] $WorkspaceRoot, [string] $SourceRepo, [string] $DiagnosticPath) {
    $fakeRepo = Join-Path $WorkspaceRoot 'AkuSidecar'
    $fakeScripts = Join-Path $fakeRepo 'scripts'
    $fakeBrowserScripts = Join-Path $WorkspaceRoot 'AkuBrowser\scripts'
    $fakeUi = Join-Path $fakeRepo 'ui-reader-broker'
    $fakeDomain = Join-Path $fakeRepo 'internal\domain'
    $fakeSupervisor = Join-Path $WorkspaceRoot 'AkuSupervisor\target\dev\aku-supervisor.exe'
    $fakeSupervisorDirectory = Split-Path -Parent $fakeSupervisor
    $fakeWorkerSource = Join-Path $WorkspaceRoot 'worker-source'
    $fakeRuntime = Join-Path $fakeRepo 'runtime\dev'
    New-Item -ItemType Directory -Path $fakeScripts,$fakeBrowserScripts,$fakeUi,$fakeDomain,$fakeWorkerSource,$fakeRuntime,$fakeSupervisorDirectory,(Join-Path $WorkspaceRoot 'SharedTemp') -Force | Out-Null
    foreach ($name in @('build-dev.ps1', 'source-cache.ps1', 'worker-backup-retention.ps1', 'dev-runtime-contract.ps1', 'restart-dev.ps1', 'dev-restart-workflow.ps1', 'dev-native-derivation-hold.ps1', 'stage-dev-x-assets.mjs', 'register-reader-broker-dev.ps1')) {
        if ($name -eq 'source-cache.ps1' -and -not (Test-Path -LiteralPath (Join-Path $SourceRepo "scripts\$name") -PathType Leaf)) { continue }
        Copy-Item -LiteralPath (Join-Path $SourceRepo "scripts\$name") -Destination (Join-Path $fakeScripts $name) -Force
    }
    foreach ($name in @('manifest.json', 'content.js', 'service-worker.js')) {
        [IO.File]::WriteAllText((Join-Path $fakeUi $name), "synthetic-$name")
    }
    [IO.File]::WriteAllText((Join-Path $fakeDomain 'types.go'), 'package domain; const ApplicationVersion = "0.0.0-synthetic"')
    [IO.File]::WriteAllText($fakeSupervisor, 'synthetic Supervisor executable placeholder; PrepareOnly must not invoke it')

    $sourceWorker = Join-Path $SourceRepo 'runtime\dev\headless-worker'
    foreach ($name in @('node.exe', 'node.pin.json', 'LICENSE', 'LICENSE-AkuSidecar')) {
        Copy-Item -LiteralPath (Join-Path $sourceWorker $name) -Destination (Join-Path $fakeWorkerSource $name) -Force
    }
    $workerStager = @'
param([Parameter(Mandatory)][string] $DestinationDirectory)
$ErrorActionPreference = 'Stop'
$source = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..\worker-source'))
New-Item -ItemType Directory -Path $DestinationDirectory -Force | Out-Null
foreach ($name in @('node.exe', 'node.pin.json', 'LICENSE', 'LICENSE-AkuSidecar')) {
    Copy-Item -LiteralPath (Join-Path $source $name) -Destination (Join-Path $DestinationDirectory $name) -Force
}
[IO.File]::WriteAllText((Join-Path $DestinationDirectory 'worker.mjs'), 'export const syntheticWorker = true;')
[ordered]@{ status = 'ok'; nodeVersion = '24.16.0-synthetic'; destinationDirectory = $DestinationDirectory } | ConvertTo-Json -Compress
'@
    [IO.File]::WriteAllText((Join-Path $fakeBrowserScripts 'stage-headless-worker.ps1'), $workerStager, [Text.UTF8Encoding]::new($false))
    $nativeStager = @'
import fs from 'node:fs';
import path from 'node:path';
const root = process.argv[2];
if (!root || process.argv.length !== 3) throw new Error('expected one fixture output directory');
for (const name of ['main-browser-transport', 'x-daily-owner']) {
  const directory = path.join(root, name);
  fs.mkdirSync(directory, { recursive: true });
  fs.writeFileSync(path.join(directory, 'fixture.marker'), `synthetic-${name}`);
}
'@
    [IO.File]::WriteAllText((Join-Path $fakeScripts 'stage-dev-x-assets.mjs'), $nativeStager, [Text.UTF8Encoding]::new($false))
    return [pscustomobject]@{ Repo = $fakeRepo; Runtime = $fakeRuntime; Supervisor = $fakeSupervisor; BrowserScripts = $fakeBrowserScripts; WorkerSource = $fakeWorkerSource }
}

function Write-SyntheticAppConfig([string] $Path, [ValidateSet('exclusive', 'concurrent-main')][string] $CaptureMode) {
    $capture = [ordered]@{ profile = 'standard'; visibility = 'quiet'; openMissingSource = $true; maxAcquisitionRounds = 1 }
    if ($CaptureMode -eq 'exclusive') { $capture.xMode = 'exclusive' }
    else { $capture.xMode = 'concurrent'; $capture.xReaderSource = 'main' }
    $config = [ordered]@{
        version = 1
        server = [ordered]@{ host = '127.0.0.1'; port = 11122 }
        database = [ordered]@{ path = 'runtime/absent-prepare-only.db' }
        reasoning = [ordered]@{
            provider = 'deterministic'; executable = ''; timeoutMs = 5000
            planning = [ordered]@{ model = 'planner'; effort = 'high' }
            evaluation = [ordered]@{ model = 'evaluator'; effort = 'high' }
            semanticEvent = [ordered]@{ model = 'event'; effort = 'high' }
            aiDetection = [ordered]@{ model = 'detector'; effort = 'high' }
        }
        capture = $capture
        bridge = [ordered]@{ trustedExtensionOrigins = @() }
        preference = [ordered]@{ mode = 'guarded_live' }
    }
    [IO.File]::WriteAllText($Path, ($config | ConvertTo-Json -Depth 8), [Text.UTF8Encoding]::new($false))
}

$script:promotionMoveLog = @()
$script:promotionFailureEnabled = $false
$script:promotionFailureInjected = $false
function Move-Item {
    param([string] $LiteralPath, [string] $Destination, [switch] $Force)
    $script:promotionMoveLog += "$LiteralPath -> $Destination"
    if ($script:promotionFailureEnabled -and -not $script:promotionFailureInjected -and
        [IO.Path]::GetFileName($LiteralPath) -eq 'x-daily-owner.next') {
        $script:promotionFailureInjected = $true
        throw 'synthetic midway promotion failure'
    }
    Microsoft.PowerShell.Management\Move-Item -LiteralPath $LiteralPath -Destination $Destination -Force:$Force
}

try {
    Assert-True (-not (Test-Path -LiteralPath $fixtureRoot)) "Fixture already exists: $fixtureRoot"
    New-Item -ItemType Directory -Path $fixtureRoot -Force | Out-Null
    $fixtureCreated = $true
    $candidateRegistrationRejected = $false
    try { & (Join-Path $PSScriptRoot 'register-reader-broker-dev.ps1') -RuntimeDirectory (Join-Path $fixtureRoot 'unregistered-candidate') -Candidate -Register }
    catch { $candidateRegistrationRejected = $_.Exception.Message -like '*candidate native host cannot be registered*' }
    Assert-True $candidateRegistrationRejected 'Candidate preparation must never register an unpromoted native host.'

    # Shared promotion installs the complete marker bundle, including broker and native assets.
    $successRoot = Join-Path $fixtureRoot 'promotion-success'
    New-SyntheticBundle $successRoot -Marker 'old'
    New-SyntheticBundle $successRoot -Candidate -Marker 'candidate'
    $backup = Invoke-DevRuntimePromotion $successRoot
    foreach ($entry in Get-DevRuntimeEntries) {
        $target = Join-Path $successRoot $entry.Target
        $targetPayload = 'candidate'
        $backupPayload = 'old'
        if ((Get-SyntheticPayload $target) -ne $targetPayload) {
            throw "Promotion did not install candidate bundle entry: $($entry.Target)"
        }
        if ((Get-SyntheticPayload (Join-Path $backup $entry.Target)) -ne $backupPayload) {
            throw "Promotion did not retain the previous bundle entry: $($entry.Target)"
        }
    }
    Assert-True ((Get-SyntheticPayload (Join-Path $backup 'aku-sidecar.exe')) -eq 'old') 'Previous Sidecar image was not retained.'
    Assert-True ((Get-SyntheticPayload (Join-Path $backup 'aku-reader-broker.exe')) -eq 'old') 'Previous reader broker was not retained.'
    $null = Assert-DevRuntimeBundle $successRoot

    # A single injected failure during the final native component must restore every prior component.
    $rollbackRoot = Join-Path $fixtureRoot 'promotion-rollback'
    New-SyntheticBundle $rollbackRoot -Marker 'old'
    New-SyntheticBundle $rollbackRoot -Candidate -Marker 'candidate'
    $script:promotionMoveLog = @()
    $script:promotionFailureEnabled = $true
    $script:promotionFailureInjected = $false
    $promotionFailed = $false
    try { $null = Invoke-DevRuntimePromotion $rollbackRoot } catch { $promotionFailed = $_.Exception.Message -like '*synthetic midway promotion failure*' }
    finally { $script:promotionFailureEnabled = $false }
    Assert-True $promotionFailed 'The synthetic midway move failure was not observed.'
    Assert-True $script:promotionFailureInjected 'The injected failure did not run.'
    Assert-True ($script:promotionMoveLog.Count -gt 10) 'The failure did not occur after several bundle entries had moved.'
    foreach ($entry in Get-DevRuntimeEntries) {
        $target = Join-Path $rollbackRoot $entry.Target
        $targetPayload = 'old'
        if ((Get-SyntheticPayload $target) -ne $targetPayload) {
            throw "Rollback failed to restore active bundle entry: $($entry.Target)"
        }
    }
    Assert-True ((Get-SyntheticPayload (Join-Path $rollbackRoot 'aku-sidecar.exe')) -eq 'old') 'Rollback failed to restore the active Sidecar image.'
    $null = Assert-DevRuntimeBundle $rollbackRoot

    # Candidate tampering is rejected before any active move or rollback directory is created.
    $tamperRoot = Join-Path $fixtureRoot 'promotion-tamper'
    New-SyntheticBundle $tamperRoot -Marker 'old'
    New-SyntheticBundle $tamperRoot -Candidate -Marker 'candidate'
    [IO.File]::AppendAllText((Join-Path $tamperRoot 'headless-worker.next\fixture.marker'), '-tampered')
    $activeBefore = @(Get-DevRuntimeInventory $tamperRoot | ConvertTo-Json -Depth 5 -Compress)
    $script:promotionMoveLog = @()
    $tamperRejected = $false
    try { $null = Invoke-DevRuntimePromotion $tamperRoot } catch { $tamperRejected = $_.Exception.Message -like '*hashes do not match provenance*' }
    $activeAfter = @(Get-DevRuntimeInventory $tamperRoot | ConvertTo-Json -Depth 5 -Compress)
    Assert-True $tamperRejected 'Tampered candidate bundle was not rejected by provenance validation.'
    Assert-True ($script:promotionMoveLog.Count -eq 0) 'Tampered candidate caused an active or rollback move before rejection.'
    Assert-True (($activeBefore -join '') -ceq ($activeAfter -join '')) 'Active bundle changed after tampered candidate rejection.'
    Assert-True (@(Get-ChildItem -LiteralPath $tamperRoot -Directory -Filter '.dev-runtime-rollback-*').Count -eq 0) 'Tamper rejection created a rollback directory.'

    # Pure service-plan validation preserves ordinary config/database/port overrides.
    $configPath = Join-Path $fixtureRoot 'services.json'
    $expectedTarget = Join-Path $repoRoot 'runtime\dev\aku-sidecar.exe'
    $ordinaryArgs = @('--config', 'C:\fixture\app.json', '--database', 'D:\fixture\store.db', '--port', '49123', '--dev', '--app-shell', '--windows-capture-split', '--ui-chromium-path', 'C:\fixture\chrome.exe', '--bridge-extension-origin', 'chrome-extension://fixture/')
    Write-ServiceConfig $configPath $repoRoot $expectedTarget $ordinaryArgs 49123
    $plan = Get-DevRuntimeServicePlan $repoRoot $configPath
    Assert-True (($plan.Arguments -join "`n") -ceq ($ordinaryArgs -join "`n")) 'Service plan did not preserve the configured ordinary arguments.'
    Assert-True ($plan.Target -eq $expectedTarget) 'Service plan returned a different development target.'

    $invalidCases = @(
        @{ Name = 'sealed experiment target'; Command = (Join-Path $repoRoot 'runtime\sealed-experiment\aku-sidecar.exe'); Args = $ordinaryArgs },
        @{ Name = 'duplicate dev flag'; Command = $expectedTarget; Args = ($ordinaryArgs + @('--dev')) },
        @{ Name = 'missing app-shell flag'; Command = $expectedTarget; Args = @($ordinaryArgs | Where-Object { $_ -ne '--app-shell' }) },
        @{ Name = 'false dev flag'; Command = $expectedTarget; Args = @($ordinaryArgs | Where-Object { $_ -ne '--dev' }) + @('--dev=false') },
        @{ Name = 'false app-shell flag'; Command = $expectedTarget; Args = @($ordinaryArgs | Where-Object { $_ -ne '--app-shell' }) + @('--app-shell=false') },
        @{ Name = 'false windows-capture-split flag'; Command = $expectedTarget; Args = @($ordinaryArgs | Where-Object { $_ -ne '--windows-capture-split' }) + @('--windows-capture-split=false') }
    )
    foreach ($case in $invalidCases) {
        Write-ServiceConfig $configPath $repoRoot $case.Command $case.Args 49123
        $rejected = $false
        try { $null = Get-DevRuntimeServicePlan $repoRoot $configPath } catch { $rejected = $true }
        Assert-True $rejected "Service plan accepted invalid case: $($case.Name)"
    }

    # Execute the real restart wrapper from a copied fixture checkout with synthetic Supervisor config.
    $wrapperRepo = Join-Path $fixtureRoot 'restart-wrapper-fixture'
    $wrapperScripts = Join-Path $wrapperRepo 'scripts'
    New-Item -ItemType Directory -Path $wrapperScripts -Force | Out-Null
    foreach ($name in @('restart-dev.ps1', 'dev-restart-workflow.ps1', 'dev-runtime-contract.ps1', 'worker-backup-retention.ps1', 'dev-native-derivation-hold.ps1')) {
        Copy-Item -LiteralPath (Join-Path $PSScriptRoot $name) -Destination (Join-Path $wrapperScripts $name)
    }
    $wrapperConfig = Join-Path $fixtureRoot 'wrapper-services.json'
    $wrapperRuntime = Join-Path $wrapperRepo 'runtime\dev'
    $wrapperTarget = Join-Path $wrapperRuntime 'aku-sidecar.exe'
    $savedConfigEnv = $env:AKU_SUPERVISOR_CONFIG
    try {
        $sealedCommand = Join-Path $wrapperRepo 'runtime\sealed-experiment\aku-sidecar.exe'
        Write-ServiceConfig $wrapperConfig $wrapperRepo $sealedCommand $ordinaryArgs 49123
        $mismatchHash = (Get-FileHash -LiteralPath $wrapperConfig -Algorithm SHA256).Hash
        $env:AKU_SUPERVISOR_CONFIG = $wrapperConfig
        $mismatchRejected = $false
        try { $null = & (Join-Path $wrapperScripts 'restart-dev.ps1') -PlanOnly } catch { $mismatchRejected = $_.Exception.Message -like '*Development handoff required*' }
        Assert-True $mismatchRejected 'Restart wrapper did not reject the sealed experiment target during PlanOnly preflight.'
        Assert-True ((Get-FileHash -LiteralPath $wrapperConfig -Algorithm SHA256).Hash -eq $mismatchHash) 'Mismatch PlanOnly changed synthetic Supervisor config.'
        Assert-True (-not (Test-Path -LiteralPath $wrapperRuntime)) 'Mismatch PlanOnly created runtime/dev.'

        Write-ServiceConfig $wrapperConfig $wrapperRepo $wrapperTarget $ordinaryArgs 49123
        $validHash = (Get-FileHash -LiteralPath $wrapperConfig -Algorithm SHA256).Hash
        $planOutput = & (Join-Path $wrapperScripts 'restart-dev.ps1') -PlanOnly
        $restartPlan = ($planOutput | Out-String) | ConvertFrom-Json
        Assert-True ($restartPlan.schema -eq 'aku.dev-restart-plan.v1' -and $restartPlan.buildTags[0] -eq 'auth_session_dev') 'Restart wrapper PlanOnly returned an invalid plan.'
        Assert-True ($restartPlan.target -eq $wrapperTarget -and $restartPlan.configChanged -eq $false) 'Restart wrapper PlanOnly did not report the expected target and read-only config state.'
        Assert-True ((Get-FileHash -LiteralPath $wrapperConfig -Algorithm SHA256).Hash -eq $validHash) 'Valid PlanOnly changed synthetic Supervisor config.'
        Assert-True (-not (Test-Path -LiteralPath $wrapperRuntime)) 'Valid PlanOnly created runtime/dev.'
    } finally {
        $env:AKU_SUPERVISOR_CONFIG = $savedConfigEnv
    }

    # Exercise the complete copied build/restart caller chain using a current diagnostic image,
    # mocked Go/Git calls, a fixture worker stager, and source-only native assets.
    $prepareWorkspace = Join-Path $fixtureRoot 'prepare-only-workspace'
    $prepare = New-PrepareOnlyFixture $prepareWorkspace $repoRoot $DiagnosticImage
    $fakeRepo = $prepare.Repo
    $fakeRuntime = $prepare.Runtime
    $fakeBuildScript = Join-Path $fakeRepo 'scripts\build-dev.ps1'
    $fakeRestartScript = Join-Path $fakeRepo 'scripts\restart-dev.ps1'
    $fakeConfigDirectory = Join-Path $fakeRepo 'config'
    New-Item -ItemType Directory -Path $fakeConfigDirectory -Force | Out-Null
    $fakeSupervisorConfig = Join-Path $prepareWorkspace 'services.json'
    $fakeServiceTarget = Join-Path $fakeRuntime 'aku-sidecar.exe'
    $syntheticDatabase = Join-Path $fakeRepo 'runtime\absent-prepare-only.db'
    $extensionOrigin = 'chrome-extension://abcdefghijklmnopabcdefghijklmnop/'
    $profilePaths = @((Join-Path $fakeRepo 'profiles'), (Join-Path $fakeRepo 'runtime\profiles'), (Join-Path $fakeRuntime 'profiles'))
    $savedEnvironment = @{
        AKU_SUPERVISOR_CONFIG = $env:AKU_SUPERVISOR_CONFIG
        GOCACHE = $env:GOCACHE
        GOMODCACHE = $env:GOMODCACHE
        GOTMPDIR = $env:GOTMPDIR
        npm_config_cache = $env:npm_config_cache
    }
    $savedLastExitCode = $global:LASTEXITCODE
    $priorMockBuildEnabled = Get-Variable -Name AkuSidecarDevRuntimeMockBuildEnabled -Scope Global -ErrorAction SilentlyContinue
    $priorMockBuildImage = Get-Variable -Name AkuSidecarDevRuntimeMockBuildImage -Scope Global -ErrorAction SilentlyContinue
    $priorMockGoCalls = Get-Variable -Name AkuSidecarDevRuntimeMockGoCalls -Scope Global -ErrorAction SilentlyContinue
    $priorMockGitCalls = Get-Variable -Name AkuSidecarDevRuntimeMockGitCalls -Scope Global -ErrorAction SilentlyContinue
    try {
        $env:AKU_SUPERVISOR_CONFIG = $fakeSupervisorConfig
        $env:GOCACHE = Join-Path $prepareWorkspace 'SharedTemp\go-build'
        $env:GOMODCACHE = Join-Path $prepareWorkspace 'SharedTemp\go-mod'
        $env:GOTMPDIR = Join-Path $prepareWorkspace 'SharedTemp\go-temp'
        $env:npm_config_cache = Join-Path $prepareWorkspace 'SharedTemp\npm-cache'
        $global:AkuSidecarDevRuntimeMockBuildEnabled = $true
        $global:AkuSidecarDevRuntimeMockBuildImage = $DiagnosticImage
        $global:AkuSidecarDevRuntimeMockGoCalls = [System.Collections.Generic.List[string]]::new()
        $global:AkuSidecarDevRuntimeMockGitCalls = [System.Collections.Generic.List[string]]::new()
        $global:LASTEXITCODE = 0

        $initialBuildOutput = (& $fakeBuildScript 6>&1 2>&1 | Out-String)
        if ($global:AkuSidecarDevRuntimeMockGoCalls.Count -ne 2 -or $global:AkuSidecarDevRuntimeMockGoCalls[0] -notmatch '-tags auth_session_dev') {
            throw "Copied default build did not request auth_session_dev; Go calls: $($global:AkuSidecarDevRuntimeMockGoCalls -join ' | ')"
        }
        $null = Assert-DevRuntimeBundle $fakeRuntime
        $activeInventoryBefore = (Get-DevRuntimeInventory $fakeRuntime | ConvertTo-Json -Depth 5 -Compress)
        $activeStatePath = Join-Path $fakeRuntime 'aku-sidecar.exe.runtime-state.json'
        $activeStateHashBefore = (Get-FileHash -LiteralPath $activeStatePath -Algorithm SHA256).Hash
        $supervisorHashBefore = (Get-FileHash -LiteralPath $prepare.Supervisor -Algorithm SHA256).Hash
        $script:prepareEvidenceEntries.Add('Copied default build output: ' + $initialBuildOutput.Trim())
        $script:prepareEvidenceEntries.Add('Initial active inventory: ' + $activeInventoryBefore)
        $script:prepareEvidenceEntries.Add('Default build compiler calls: ' + ($global:AkuSidecarDevRuntimeMockGoCalls -join ' | '))
        $script:prepareEvidenceEntries.Add('Synthetic source revision: f0123456789abcdef0123456789abcdef0123456; status mocked clean.')

        foreach ($captureMode in @('exclusive', 'concurrent-main')) {
            $appConfig = Join-Path $fakeConfigDirectory ("sidecar-$captureMode.json")
            Write-SyntheticAppConfig $appConfig $captureMode
            $configArgument = if ($captureMode -eq 'concurrent-main') { "config\sidecar-$captureMode.json" } else { $appConfig }
            $arguments = @('--config', $configArgument, '--database', $syntheticDatabase, '--port', '11122', '--dev', '--app-shell', '--windows-capture-split', '--ui-chromium-path', (Join-Path $prepareWorkspace 'synthetic-chrome.exe'), '--bridge-extension-origin', $extensionOrigin)
            Write-ServiceConfig $fakeSupervisorConfig $fakeRepo $fakeServiceTarget $arguments 11122
            Assert-True (-not (Test-Path -LiteralPath $syntheticDatabase)) 'Synthetic database existed before PrepareOnly.'
            foreach ($profilePath in $profilePaths) { Assert-True (-not (Test-Path -LiteralPath $profilePath)) "Synthetic profile path existed before PrepareOnly: $profilePath" }
            $serviceBytesBefore = [Convert]::ToBase64String([IO.File]::ReadAllBytes($fakeSupervisorConfig))
            $appConfigBytesBefore = [Convert]::ToBase64String([IO.File]::ReadAllBytes($appConfig))
            $serviceHashCaseBefore = (Get-FileHash -LiteralPath $fakeSupervisorConfig -Algorithm SHA256).Hash
            $appConfigHashBefore = (Get-FileHash -LiteralPath $appConfig -Algorithm SHA256).Hash
            $supervisorHashCaseBefore = (Get-FileHash -LiteralPath $prepare.Supervisor -Algorithm SHA256).Hash
            $goCallStart = $global:AkuSidecarDevRuntimeMockGoCalls.Count

            $prepareOutput = (& $fakeRestartScript -PrepareOnly 2>&1 | Out-String)
            if ($prepareOutput -notmatch 'PASS: development candidate prepared and inspected; no registration or runtime change\.') {
                throw "Copied restart -PrepareOnly did not complete for $captureMode. Output: $prepareOutput"
            }
            $candidateState = Assert-DevRuntimeBundle $fakeRuntime -Candidate
            if ($global:AkuSidecarDevRuntimeMockGoCalls.Count -ne ($goCallStart + 2) -or $global:AkuSidecarDevRuntimeMockGoCalls[$goCallStart] -notmatch '-tags auth_session_dev' -or $global:AkuSidecarDevRuntimeMockGoCalls[$goCallStart] -notmatch 'aku-sidecar\.next\.exe') {
                throw "PrepareOnly candidate build did not use the required development tag/output for $captureMode. Calls: $($global:AkuSidecarDevRuntimeMockGoCalls -join ' | ')"
            }
            $activeInventoryAfter = (Get-DevRuntimeInventory $fakeRuntime | ConvertTo-Json -Depth 5 -Compress)
            Assert-True ($activeInventoryAfter -ceq $activeInventoryBefore) "PrepareOnly changed the active bundle for $captureMode."
            Assert-True ((Get-FileHash -LiteralPath $activeStatePath -Algorithm SHA256).Hash -eq $activeStateHashBefore) "PrepareOnly changed active provenance for $captureMode."
            Assert-True ([Convert]::ToBase64String([IO.File]::ReadAllBytes($fakeSupervisorConfig)) -ceq $serviceBytesBefore) "PrepareOnly changed Supervisor config for $captureMode."
            Assert-True ([Convert]::ToBase64String([IO.File]::ReadAllBytes($appConfig)) -ceq $appConfigBytesBefore) "PrepareOnly changed app config for $captureMode."
            Assert-True ((Get-FileHash -LiteralPath $prepare.Supervisor -Algorithm SHA256).Hash -eq $supervisorHashCaseBefore -and $supervisorHashCaseBefore -eq $supervisorHashBefore) "PrepareOnly invoked or changed the synthetic Supervisor executable for $captureMode."
            Assert-True (-not (Test-Path -LiteralPath $syntheticDatabase)) "PrepareOnly created the synthetic database for $captureMode."
            foreach ($profilePath in $profilePaths) { Assert-True (-not (Test-Path -LiteralPath $profilePath)) "PrepareOnly created a synthetic profile path for ${captureMode}: $profilePath" }
            $script:prepareEvidenceEntries.Add("$captureMode PrepareOnly output: $($prepareOutput.Trim())")
            $script:prepareEvidenceEntries.Add("$captureMode candidate compiler calls: " + (($global:AkuSidecarDevRuntimeMockGoCalls | Select-Object -Skip $goCallStart -First 2) -join ' | '))
            $script:prepareEvidenceEntries.Add("$captureMode candidate binary SHA-256: $($candidateState.binarySha256)")
            $script:prepareEvidenceEntries.Add("$captureMode active inventory unchanged: $activeInventoryBefore")
            $script:prepareEvidenceEntries.Add("$captureMode app config SHA-256 unchanged: $appConfigHashBefore")
            $script:prepareEvidenceEntries.Add("$captureMode Supervisor config SHA-256 unchanged: $serviceHashCaseBefore")
            $script:prepareEvidenceEntries.Add("$captureMode synthetic DB absent; profile paths absent; Supervisor executable unchanged SHA-256: $supervisorHashBefore")

            # Remove only the synthetic candidate bundle so the next case starts without an owned rollback backup.
            foreach ($entry in Get-DevRuntimeEntries) {
                $candidatePath = Join-Path $fakeRuntime $entry.Candidate
                if (Test-Path -LiteralPath $candidatePath) { Remove-Item -LiteralPath $candidatePath -Recurse -Force }
            }
            $candidateProvenance = Join-Path $fakeRuntime 'aku-sidecar.next.exe.runtime-state.json'
            if (Test-Path -LiteralPath $candidateProvenance) { Remove-Item -LiteralPath $candidateProvenance -Force }
        }
        Assert-True ($global:AkuSidecarDevRuntimeMockGitCalls.Count -ge 4) 'Copied builds did not use the synthetic git metadata stub.'
        $script:prepareEvidenceEntries.Add('Synthetic git calls: ' + ($global:AkuSidecarDevRuntimeMockGitCalls -join ' | '))
    } catch {
        $script:prepareEvidenceEntries.Add('FAIL: ' + $_.Exception.Message)
        throw "Copied build/PrepareOnly regression failed; evidence log: $prepareEvidencePath. $($_.Exception.Message)"
    } finally {
        $global:AkuSidecarDevRuntimeMockBuildEnabled = $false
        foreach ($savedMock in @(
            @{ Name = 'AkuSidecarDevRuntimeMockBuildEnabled'; Variable = $priorMockBuildEnabled },
            @{ Name = 'AkuSidecarDevRuntimeMockBuildImage'; Variable = $priorMockBuildImage },
            @{ Name = 'AkuSidecarDevRuntimeMockGoCalls'; Variable = $priorMockGoCalls },
            @{ Name = 'AkuSidecarDevRuntimeMockGitCalls'; Variable = $priorMockGitCalls }
        )) {
            if ($null -eq $savedMock.Variable) { Remove-Variable -Name $savedMock.Name -Scope Global -ErrorAction SilentlyContinue }
            else { Set-Variable -Name $savedMock.Name -Value $savedMock.Variable.Value -Scope Global }
        }
        $env:AKU_SUPERVISOR_CONFIG = $savedEnvironment.AKU_SUPERVISOR_CONFIG
        $env:GOCACHE = $savedEnvironment.GOCACHE
        $env:GOMODCACHE = $savedEnvironment.GOMODCACHE
        $env:GOTMPDIR = $savedEnvironment.GOTMPDIR
        $env:npm_config_cache = $savedEnvironment.npm_config_cache
        $global:LASTEXITCODE = $savedLastExitCode
    }
    $script:prepareEvidenceEntries.Add('PASS: copied build-dev default plus restart -PrepareOnly passed exclusive and concurrent/main configs without active bundle, DB, profile, app config, service config, or Supervisor changes.')

    Write-Output "PASS: shared bundle promotion, rollback, tamper rejection, pure service-plan validation, copied restart PlanOnly, and copied build/restart PrepareOnly. Evidence: $prepareEvidencePath"
} finally {
    if ($script:prepareEvidenceEntries.Count -gt 0) {
        [IO.File]::WriteAllLines($prepareEvidencePath, $script:prepareEvidenceEntries, [Text.UTF8Encoding]::new($false))
    }
    if ($fixtureCreated -and (Test-Path -LiteralPath $fixtureRoot -PathType Container)) {
        $resolvedBuildRoot = [IO.Path]::GetFullPath($buildRoot).TrimEnd('\') + '\'
        $resolvedFixture = [IO.Path]::GetFullPath($fixtureRoot)
        if (-not $resolvedFixture.StartsWith($resolvedBuildRoot, [StringComparison]::OrdinalIgnoreCase)) {
            throw "Refusing to remove fixture outside project build directory: $resolvedFixture"
        }
        Remove-Item -LiteralPath $resolvedFixture -Recurse -Force
    }
}

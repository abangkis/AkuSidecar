param([string] $ResultPath)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'worker-backup-retention.ps1')
. (Join-Path $PSScriptRoot 'dev-runtime-contract.ps1')
. (Join-Path $PSScriptRoot 'development-package-handoff.ps1')
$repository = Split-Path -Parent $PSScriptRoot
$build = Join-Path $repository 'build'
$fixture = Join-Path $build ('package-handoff-retention-' + [Guid]::NewGuid().ToString('n'))
$checks = 0
function Check([bool] $Value, [string] $Message) {
    $script:checks++
    if (-not $Value) { throw $Message }
}
function New-FixtureBundle([string] $Root, [string] $Marker) {
    $null = New-Item -ItemType Directory -Path $Root
    foreach ($entry in Get-DevRuntimeEntries) {
        $path = Join-Path $Root $entry.Target
        if ($entry.Target -in @('headless-worker','ui-reader-broker','main-browser-transport','x-daily-owner')) {
            $null = New-Item -ItemType Directory -Path $path
            [IO.File]::WriteAllText((Join-Path $path 'fixture.mjs'), $Marker)
        } else { [IO.File]::WriteAllText($path, $Marker) }
    }
    $worker = Join-Path $Root 'headless-worker'
    foreach ($name in @('node.exe','LICENSE','LICENSE-AkuSidecar')) { [IO.File]::WriteAllText((Join-Path $worker $name), "$Marker-$name") }
    $pin = @{protocol=1;nodeSha256=(Get-FileHash -LiteralPath (Join-Path $worker 'node.exe')).Hash;
        licenseFile='LICENSE';licenseSha256=(Get-FileHash -LiteralPath (Join-Path $worker 'LICENSE')).Hash}
    [IO.File]::WriteAllText((Join-Path $worker 'node.pin.json'),($pin | ConvertTo-Json))
    $artifacts = @(Get-DevRuntimeInventory $Root)
    $state = @{version='fixture';bundleSchema='aku.dev-runtime-bundle.v1';buildTags=@('auth_session_dev');binaryFile='aku-sidecar.exe';
        binarySha256=($artifacts | Where-Object path -eq 'aku-sidecar.exe').sha256;
        headlessWorker=@{destinationDirectory=$worker};artifacts=$artifacts}
    [IO.File]::WriteAllText((Join-Path $Root 'aku-sidecar.exe.runtime-state.json'),($state | ConvertTo-Json -Depth 8))
}
try {
    $null = New-Item -ItemType Directory -Path $fixture
    $runtime = Join-Path $fixture 'runtime'
    New-FixtureBundle $runtime 'active'
    $keep = Join-Path $runtime ('.dev-runtime-rollback-' + ('0' * 32))
    $old = Join-Path $runtime ('.dev-runtime-rollback-' + ('f' * 32))
    New-FixtureBundle $keep 'keep'
    New-FixtureBundle $old 'old'
    $sha = (Assert-DevRuntimeBundle $runtime).binarySha256
    $statePath = Join-Path $runtime 'aku-sidecar.exe.runtime-state.json'
    $state = Get-Content -LiteralPath $statePath -Raw | ConvertFrom-Json
    $reordered = @($state.artifacts)
    [array]::Reverse($reordered)
    $state.artifacts = $reordered
    [IO.File]::WriteAllText($statePath,($state | ConvertTo-Json -Depth 8))
    Check ((Assert-DevRuntimeBundle $runtime).binarySha256 -eq $sha) 'Inventory ordering changed verification semantics.'

    $protected = @{}
    foreach ($name in @('data.db','config.json','report.json','run.log','screen.png','source.go')) {
        $path = Join-Path $runtime $name
        [IO.File]::WriteAllText($path, $name)
        $protected[$path] = (Get-FileHash -LiteralPath $path).Hash
    }
    Complete-DevelopmentPackageHandoff -RuntimeRoot $runtime -KeepPath $keep -VerifiedBinarySha256 ('1' * 64) -WarningVariable warning -WarningAction SilentlyContinue
    Check (Test-Path -LiteralPath $old) 'An invalid verification hash must preserve old rollback.'
    $operation = Enter-DevelopmentPackageHandoff $runtime
    try {
        # Reentrant use must interoperate with the current canonical runtime helper.
        $inner = Enter-DevRuntimeOperation $runtime
        try { Check ($null -ne $inner) 'Canonical mutex was not reentrant.' }
        finally { $inner.ReleaseMutex();$inner.Dispose() }
        Complete-DevelopmentPackageHandoff -RuntimeRoot $runtime -KeepPath $keep -VerifiedBinarySha256 $sha
    } finally { $operation.ReleaseMutex();$operation.Dispose() }
    Check (-not (Test-Path -LiteralPath $old)) 'The verified adapter did not remove old rollback.'
    Check (Test-Path -LiteralPath $keep) 'The exact latest rollback was removed.'
    Check ((Assert-DevRuntimeBundle $runtime).binarySha256 -eq $sha) 'The active fixture changed.'
    $null = Assert-DevRuntimeBundle $keep
    foreach ($path in $protected.Keys) { Check ((Get-FileHash -LiteralPath $path).Hash -eq $protected[$path]) 'A protected fixture file changed.' }
    $result = [pscustomobject]@{status='passed';assertions=$checks;engine=$PSVersionTable.PSVersion.ToString();scope='disposable fixture only'}
    if ($ResultPath) { $result | ConvertTo-Json -Compress | Set-Content -LiteralPath $ResultPath -Encoding utf8 }
    $result | ConvertTo-Json -Compress
} finally {
    if (Test-Path -LiteralPath $fixture) {
        $resolved = (Resolve-Path -LiteralPath $fixture).ProviderPath
        $parent = (Resolve-Path -LiteralPath $build).ProviderPath
        if ((Split-Path -Parent $resolved) -ne $parent -or (Split-Path -Leaf $resolved) -notmatch '^package-handoff-retention-[0-9a-f]{32}$' -or
            ((Get-Item -LiteralPath $resolved).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Fixture cleanup path is invalid.' }
        Remove-Item -LiteralPath $resolved -Recurse -Force
    }
}


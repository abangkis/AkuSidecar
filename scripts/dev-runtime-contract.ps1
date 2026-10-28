# Shared development bundle contract. Loading this file has no side effects.
function Get-DevRuntimeEntries {
    @(
        @{ Candidate = 'aku-sidecar.next.exe'; Target = 'aku-sidecar.exe' },
        @{ Candidate = 'aku-reader-broker.next.exe'; Target = 'aku-reader-broker.exe' },
        @{ Candidate = 'ui-reader-broker.next'; Target = 'ui-reader-broker' },
        @{ Candidate = 'com.akubrowser.reader_activation.next.json'; Target = 'com.akubrowser.reader_activation.json' },
        @{ Candidate = 'headless-worker.next'; Target = 'headless-worker' },
        @{ Candidate = 'main-browser-transport.next'; Target = 'main-browser-transport' },
        @{ Candidate = 'x-daily-owner.next'; Target = 'x-daily-owner' }
    )
}

function Get-DevRuntimeInventory([string] $RuntimeRoot, [switch] $Candidate) {
    $root = Get-WorkerBackupCanonicalRuntimeRoot $RuntimeRoot
    $rows = @()
    foreach ($entry in Get-DevRuntimeEntries) {
        $name = if ($Candidate) { $entry.Candidate } else { $entry.Target }
        $path = Join-Path $root $name
        if (-not (Test-Path -LiteralPath $path)) { throw "Development bundle entry is missing: $name" }
        $item = Get-Item -LiteralPath $path -Force
        $files = if ($item.PSIsContainer) { @(Get-ChildItem -LiteralPath $path -Recurse -Force) } else { @($item) }
        foreach ($file in @($item) + $files) {
            if (($file.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw 'Development bundle contains a reparse point.' }
        }
        foreach ($file in $files | Where-Object { -not $_.PSIsContainer }) {
            $relative = if ($item.PSIsContainer) { $entry.Target + '/' + [IO.Path]::GetRelativePath($path, $file.FullName).Replace([char]92, [char]47) } else { $entry.Target }
            $rows += [pscustomobject]@{ path = $relative; sha256 = (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant() }
        }
    }
    return @($rows | Sort-Object path)
}

function Assert-DevRuntimeBundle([string] $RuntimeRoot, [switch] $Candidate) {
    $name = if ($Candidate) { 'aku-sidecar.next.exe.runtime-state.json' } else { 'aku-sidecar.exe.runtime-state.json' }
    $path = Join-Path $RuntimeRoot $name
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw 'Development bundle provenance is missing.' }
    $state = Get-Content -LiteralPath $path -Raw | ConvertFrom-Json
    if ($state.bundleSchema -ne 'aku.dev-runtime-bundle.v1' -or @($state.buildTags).Count -ne 1 -or $state.buildTags[0] -ne 'auth_session_dev') {
        throw 'Development bundle capability/provenance contract is invalid.'
    }
    $actual = @(Get-DevRuntimeInventory $RuntimeRoot -Candidate:$Candidate)
    if (($actual | ConvertTo-Json -Depth 5 -Compress) -cne (@($state.artifacts) | ConvertTo-Json -Depth 5 -Compress)) {
        throw 'Development bundle hashes do not match provenance.'
    }
    if (($actual | Where-Object path -eq 'aku-sidecar.exe').sha256 -cne $state.binarySha256) { throw 'Development binary hash disagrees with provenance.' }
    return $state
}

function Get-DevRuntimeServicePlan([string] $Repository, [string] $ConfigurationPath) {
    if (-not (Test-Path -LiteralPath $ConfigurationPath -PathType Leaf)) { throw 'Supervisor configuration is missing.' }
    $configuration = Get-Content -LiteralPath $ConfigurationPath -Raw | ConvertFrom-Json
    $service = $configuration.services.akusidecar
    $target = Join-Path $Repository 'runtime\dev\aku-sidecar.exe'
    if ($null -eq $service -or -not [IO.Path]::IsPathRooted([string]$service.command) -or
        -not [IO.Path]::GetFullPath([string]$service.command).Equals([IO.Path]::GetFullPath($target), [StringComparison]::OrdinalIgnoreCase)) {
        throw 'Development handoff required: akusidecar must be registered to runtime/dev/aku-sidecar.exe. No registration or runtime was changed.'
    }
    if (-not [IO.Path]::GetFullPath([string]$service.cwd).Equals([IO.Path]::GetFullPath($Repository), [StringComparison]::OrdinalIgnoreCase)) {
        throw 'Development service cwd does not match this checkout.'
    }
    $arguments = @($service.args | ForEach-Object { [string]$_ })
    if (@($service.ports).Count -ne 1 -or [int]$service.ports[0] -lt 1024 -or [int]$service.ports[0] -gt 65535) { throw 'Development service requires one explicit valid port.' }
    foreach ($flag in @('--dev', '--app-shell', '--windows-capture-split')) {
        if (@($arguments | Where-Object { $_ -eq $flag }).Count -ne 1) { throw "Development service requires exactly one $flag argument." }
    }
    foreach ($flag in @('--config', '--database', '--port', '--ui-chromium-path', '--bridge-extension-origin')) {
        $count = 0
        for ($i = 0; $i -lt $arguments.Count; $i++) {
            if ($arguments[$i] -eq $flag) {
                if ($i + 1 -ge $arguments.Count -or $arguments[$i + 1].StartsWith('--')) { throw "Missing value for $flag." }
                $count++
            } elseif ($arguments[$i].StartsWith($flag + '=')) { $count++ }
        }
        if ($count -gt 1 -or ($flag -in @('--config', '--ui-chromium-path', '--bridge-extension-origin') -and $count -ne 1)) {
            throw "Ambiguous or missing development argument: $flag"
        }
    }
    foreach ($argument in $arguments) {
        if ($argument -match '^--(?:dev|app-shell|windows-capture-split)=') { throw 'Development boolean flags must not be overridden.' }
        if ($argument -match '^--(?:database-inspect|runtime-candidate-probe|dev-session-reuse|dev-x-manual-plan|dev-qa-pilot-plan)(?:=|$)') {
            throw 'Diagnostic/QA selectors cannot be registered as the daily development service.'
        }
    }
    $configArgument = $null
    for ($i = 0; $i -lt $arguments.Count; $i++) {
        if ($arguments[$i] -eq '--config') { $configArgument = $arguments[$i + 1] }
        elseif ($arguments[$i].StartsWith('--config=')) { $configArgument = $arguments[$i].Substring(9) }
    }
    if ([string]::IsNullOrWhiteSpace($configArgument)) { throw 'Development config path is empty.' }
    $configPath = if ([IO.Path]::IsPathRooted($configArgument)) { [IO.Path]::GetFullPath($configArgument) }
        else { [IO.Path]::GetFullPath((Join-Path $service.cwd $configArgument)) }
    return [pscustomobject]@{ Service = $service; Arguments = $arguments; Target = $target; ConfigPath = $configPath }
}

function Invoke-DevRuntimePromotion([string] $RuntimeRoot) {
    $root = Get-WorkerBackupCanonicalRuntimeRoot $RuntimeRoot
    $null = Assert-DevRuntimeBundle $root -Candidate
    $backup = Join-Path $root ('.dev-runtime-rollback-' + [Guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $backup | Out-Null
    $steps = @()
    try {
        $entries = @(Get-DevRuntimeEntries) + @(@{ Candidate = 'aku-sidecar.next.exe.runtime-state.json'; Target = 'aku-sidecar.exe.runtime-state.json' })
        foreach ($entry in $entries) {
            $step = @{ Entry = $entry; Saved = $false; Installed = $false }
            $steps += $step
            $destination = Join-Path $root $entry.Target
            if (Test-Path -LiteralPath $destination) {
                Move-Item -LiteralPath $destination -Destination (Join-Path $backup $entry.Target)
                $step.Saved = $true
            }
            Move-Item -LiteralPath (Join-Path $root $entry.Candidate) -Destination $destination
            $step.Installed = $true
        }
        $statePath = Join-Path $root 'aku-sidecar.exe.runtime-state.json'
        $state = Get-Content -LiteralPath $statePath -Raw | ConvertFrom-Json
        $state.binaryFile = 'aku-sidecar.exe'
        $state.headlessWorker.destinationDirectory = Join-Path $root 'headless-worker'
        [IO.File]::WriteAllText($statePath, ($state | ConvertTo-Json -Depth 8), [Text.UTF8Encoding]::new($false))
        $null = Assert-DevRuntimeBundle $root
    } catch {
        $cause = $_
        $restoreErrors = @()
        [array]::Reverse($steps)
        foreach ($step in $steps) {
            try {
                $destination = Join-Path $root $step.Entry.Target
                if ($step.Installed) { Move-Item -LiteralPath $destination -Destination (Join-Path $root $step.Entry.Candidate) }
                if ($step.Saved) { Move-Item -LiteralPath (Join-Path $backup $step.Entry.Target) -Destination $destination }
            } catch { $restoreErrors += $_.Exception.Message }
        }
        if ($restoreErrors.Count -gt 0) { throw "Development promotion failed: $cause; rollback incomplete at $backup : $($restoreErrors -join '; ')" }
        throw $cause
    }
    return $backup
}

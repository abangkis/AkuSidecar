function Get-WorkerBackupCanonicalRuntimeRoot([string] $RuntimeRoot) {
    if ([string]::IsNullOrWhiteSpace($RuntimeRoot)) {
        throw 'Worker backup runtime root is empty.'
    }
    $runtimePath = [IO.Path]::GetFullPath($RuntimeRoot).TrimEnd([IO.Path]::DirectorySeparatorChar)
    $cursor = [IO.DirectoryInfo]$runtimePath
    while ($null -ne $cursor) {
        if (Test-Path -LiteralPath $cursor.FullName) {
            $item = Get-Item -LiteralPath $cursor.FullName -Force
            if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
                throw "Worker backup runtime ancestry contains a reparse point: $($cursor.FullName)"
            }
        }
        $cursor = $cursor.Parent
    }
    return $runtimePath
}

function Get-WorkerBackupBundleInventory([string] $WorkerRoot) {
    if (-not (Test-Path -LiteralPath $WorkerRoot -PathType Container)) {
        throw "Worker bundle directory is missing: $WorkerRoot"
    }
    $rootItem = Get-Item -LiteralPath $WorkerRoot -Force
    if (($rootItem.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "Worker bundle directory is a reparse point: $WorkerRoot"
    }

    $entries = @(Get-ChildItem -LiteralPath $WorkerRoot -Recurse -Force -ErrorAction Stop)
    $files = @($entries | Where-Object { -not $_.PSIsContainer })
    $bytes = [long]0
    foreach ($entry in $entries) {
        if (($entry.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw "Worker bundle contains a reparse point: $($entry.FullName)"
        }
        if ($entry.Name -match '(?i)^\.env(?:\..*)?$|^(?:\.git|node_modules|cache|profile|profiles|user[-_]?data|cookies?|sessions?|credentials?|secrets?|tokens?)$' -or
            $entry.Name -match '(?i)(cookie|session|profile|user[-_]?data|credential|secret|token)' -or
            $entry.Name -match '(?i)\.(?:db|sqlite\d?(?:-(?:wal|shm))?|jsonl|pem|key|pfx|p12|jks|log)$') {
            throw "Worker bundle contains a state or sensitive item: $($entry.FullName)"
        }
        if (-not $entry.PSIsContainer) {
            $bytes += [long]$entry.Length
        }
    }

    $pinPath = Join-Path $WorkerRoot 'node.pin.json'
    $nodePath = Join-Path $WorkerRoot 'node.exe'
    if (-not (Test-Path -LiteralPath $pinPath -PathType Leaf) -or -not (Test-Path -LiteralPath $nodePath -PathType Leaf)) {
        throw "Worker bundle is missing its pinned Node.js files: $WorkerRoot"
    }
    $pin = Get-Content -LiteralPath $pinPath -Raw | ConvertFrom-Json
    if ([int]$pin.protocol -ne 1 -or [string]$pin.nodeSha256 -notmatch '^[0-9a-fA-F]{64}$' -or
        [string]$pin.licenseSha256 -notmatch '^[0-9a-fA-F]{64}$' -or
        [IO.Path]::GetFileName([string]$pin.licenseFile) -cne [string]$pin.licenseFile -or
        [string]::IsNullOrWhiteSpace([string]$pin.licenseFile)) {
        throw "Worker bundle Node.js pin is invalid: $pinPath"
    }
    $licensePath = Join-Path $WorkerRoot ([string]$pin.licenseFile)
    if (-not (Test-Path -LiteralPath $licensePath -PathType Leaf) -or
        -not (Test-Path -LiteralPath (Join-Path $WorkerRoot 'LICENSE-AkuSidecar') -PathType Leaf)) {
        throw "Worker bundle is missing its pinned license files: $WorkerRoot"
    }
    $nodeHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $nodePath).Hash.ToLowerInvariant()
    $licenseHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $licensePath).Hash.ToLowerInvariant()
    if ($nodeHash -ne ([string]$pin.nodeSha256).ToLowerInvariant() -or $licenseHash -ne ([string]$pin.licenseSha256).ToLowerInvariant()) {
        throw "Worker bundle files do not match node.pin.json: $WorkerRoot"
    }
    return [pscustomobject]@{ Bytes = $bytes; FileCount = $files.Count }
}

function Get-WorkerBackupProcessInventory {
    try {
        $processes = @(Get-CimInstance -ClassName Win32_Process -ErrorAction Stop)
    } catch {
        throw "The Windows process inventory is unavailable: $($_.Exception.Message)"
    }
    if ($processes.Count -eq 0) {
        throw 'The Windows process inventory is unavailable or empty.'
    }
    return $processes
}

function Test-WorkerBackupProcessReference([string] $BackupRoot) {
    $backupPath = [IO.Path]::GetFullPath($BackupRoot).TrimEnd([IO.Path]::DirectorySeparatorChar)
    $backupBoundary = $backupPath + [IO.Path]::DirectorySeparatorChar
    $processes = @(Get-WorkerBackupProcessInventory)
    foreach ($process in $processes) {
        $executablePath = [string]$process.ExecutablePath
        if (-not [string]::IsNullOrWhiteSpace($executablePath)) {
            try {
                $canonicalExecutable = [IO.Path]::GetFullPath($executablePath)
            } catch {
                throw "The Windows process inventory contains an unreadable executable path for PID $($process.ProcessId)."
            }
            if ($canonicalExecutable.StartsWith($backupBoundary, [StringComparison]::OrdinalIgnoreCase)) {
                return $process
            }
        }

        $commandLine = [string]$process.CommandLine
        if ([string]::IsNullOrWhiteSpace($commandLine)) { continue }
        $commandLine = $commandLine.Replace('/', [IO.Path]::DirectorySeparatorChar)
        $searchFrom = 0
        while ($searchFrom -lt $commandLine.Length) {
            $matchAt = $commandLine.IndexOf($backupPath, $searchFrom, [StringComparison]::OrdinalIgnoreCase)
            if ($matchAt -lt 0) { break }
            $afterPath = $matchAt + $backupPath.Length
            if ($afterPath -eq $commandLine.Length -or
                $commandLine[$afterPath] -in @([IO.Path]::DirectorySeparatorChar, [IO.Path]::AltDirectorySeparatorChar, '"', "'", ' ', "`t")) {
                return $process
            }
            $searchFrom = $afterPath
        }
    }
    return $null
}

function Assert-WorkerBackupCopyReady {
    param(
        [Parameter(Mandatory)] [string] $RuntimeRoot,
        [Parameter(Mandatory)] [string] $WorkerRoot,
        [Parameter(Mandatory)] [string] $BackupPrefix,
        [long] $MinimumFreeBytes = 268435456,
        [long] $SafetyMarginBytes = 67108864
    )

    if ($BackupPrefix -notmatch '^[A-Za-z0-9._-]+-$') {
        throw "Worker backup owner prefix is invalid: $BackupPrefix"
    }
    $ownedPrefixes = @(
        '.build-dev-headless-worker-rollback-',
        '.build-dev-headless-worker.next-rollback-',
        '.restart-dev-headless-worker-rollback-'
    )
    if ($ownedPrefixes -cnotcontains $BackupPrefix) {
        throw "Worker backup owner prefix is not recognized: $BackupPrefix"
    }
    $runtimePath = Get-WorkerBackupCanonicalRuntimeRoot $RuntimeRoot
    $workerPath = [IO.Path]::GetFullPath($WorkerRoot).TrimEnd([IO.Path]::DirectorySeparatorChar)
    if (-not [IO.Path]::GetDirectoryName($workerPath).TrimEnd([IO.Path]::DirectorySeparatorChar).Equals($runtimePath, [StringComparison]::OrdinalIgnoreCase)) {
        throw "Worker destination is not an immediate child of the runtime root: $workerPath"
    }

    if (Test-Path -LiteralPath $runtimePath -PathType Container) {
        $pending = @(Get-ChildItem -LiteralPath $runtimePath -Force | Where-Object {
            $matchesOwnedPrefix = $false
            foreach ($ownedPrefix in $ownedPrefixes) {
                if ($_.Name.StartsWith($ownedPrefix, [StringComparison]::OrdinalIgnoreCase)) {
                    $matchesOwnedPrefix = $true
                    break
                }
            }
            $matchesOwnedPrefix
        })
        if ($pending.Count -gt 0) {
            $pendingNames = @($pending | ForEach-Object { $_.Name }) -join ', '
            throw "Prior invocation-owned worker rollback backups remain under the runtime root; refusing another worker copy until reviewed: $pendingNames"
        }
    }

    $requiredBytes = [long]$MinimumFreeBytes
    if (Test-Path -LiteralPath $workerPath -PathType Container) {
        $inventory = Get-WorkerBackupBundleInventory $workerPath
        $requiredBytes = [Math]::Max($requiredBytes, [long](($inventory.Bytes * 2) + $SafetyMarginBytes))
    }
    $driveRoot = [IO.Path]::GetPathRoot($runtimePath)
    $drive = [IO.DriveInfo]::new($driveRoot)
    if (-not $drive.IsReady -or $drive.AvailableFreeSpace -lt $requiredBytes) {
        $availableBytes = if ($drive.IsReady) { $drive.AvailableFreeSpace } else { 0 }
        throw "Insufficient ready disk space for another worker copy: required $requiredBytes bytes, available $availableBytes bytes."
    }
}

function Remove-WorkerBackupAfterSuccess {
    param(
        [Parameter(Mandatory)] [string] $RuntimeRoot,
        [Parameter(Mandatory)] [string] $BackupPath,
        [Parameter(Mandatory)] [string] $BackupPrefix
    )

    try {
        $ownedPrefixes = @(
            '.build-dev-headless-worker-rollback-',
            '.build-dev-headless-worker.next-rollback-',
            '.restart-dev-headless-worker-rollback-'
        )
        if ($ownedPrefixes -cnotcontains $BackupPrefix) {
            throw "Worker backup owner prefix is not recognized: $BackupPrefix"
        }
        $runtimePath = Get-WorkerBackupCanonicalRuntimeRoot $RuntimeRoot
        $backup = [IO.Path]::GetFullPath($BackupPath).TrimEnd([IO.Path]::DirectorySeparatorChar)
        $backupParent = [IO.Path]::GetDirectoryName($backup).TrimEnd([IO.Path]::DirectorySeparatorChar)
        if (-not $backupParent.Equals($runtimePath, [StringComparison]::OrdinalIgnoreCase) -or
            [IO.Path]::GetFileName($backup) -notmatch ('^' + [regex]::Escape($BackupPrefix) + '[0-9a-f]{32}$')) {
            throw "Worker backup is not an invocation-owned path under the exact runtime root: $backup"
        }
        if (-not (Test-Path -LiteralPath $backup)) {
            return $true
        }

        $referencingProcess = Test-WorkerBackupProcessReference $backup
        if ($null -ne $referencingProcess) {
            $processLabel = if ([string]::IsNullOrWhiteSpace([string]$referencingProcess.Name)) { 'unknown process' } else { [string]$referencingProcess.Name }
            throw "Worker rollback backup is still referenced by $processLabel (PID $($referencingProcess.ProcessId))."
        }
        $null = Get-WorkerBackupBundleInventory $backup
        Remove-Item -LiteralPath $backup -Recurse -Force -ErrorAction Stop
        if (Test-Path -LiteralPath $backup) {
            throw "Worker backup remains after cleanup: $backup"
        }
        return $true
    } catch {
        Write-Warning "Worker rollback backup was retained after cleanup was skipped or failed. Future copies with this owner prefix will be blocked until it is reviewed. Path: $BackupPath. Reason: $($_.Exception.Message)"
        return $false
    }
}

# Only the current invocation may retire its native staging directory. Call
# after the complete development bundle and provenance have been written.
function Remove-NativeStageAfterSuccess {
    param(
        [Parameter(Mandatory)] [string] $BuildRoot,
        [Parameter(Mandatory)] [string] $StagePath,
        [switch] $BuildSucceeded
    )
    $handles = [Collections.Generic.List[IO.FileStream]]::new()
    try {
        if (-not $BuildSucceeded) { return $false }
        $root = Get-WorkerBackupCanonicalRuntimeRoot $BuildRoot
        $stage = Get-WorkerBackupCanonicalRuntimeRoot $StagePath
        if (-not [IO.Path]::GetDirectoryName($stage).Equals($root, [StringComparison]::OrdinalIgnoreCase) -or
            [IO.Path]::GetFileName($stage) -cnotmatch '^dev-native-stage-[0-9a-f]{32}$') {
            throw 'Native staging path is not an immediate invocation-owned build directory.'
        }
        if (-not (Test-Path -LiteralPath $stage -PathType Container)) { throw 'Native staging directory is missing.' }
        if ($null -ne (Test-WorkerBackupProcessReference $stage)) { throw 'Native staging is referenced by a process.' }
        $files = @()
        $directories = @(Get-ChildItem -LiteralPath $stage -Force -ErrorAction Stop)
        foreach ($directory in $directories) {
            if (-not $directory.PSIsContainer -or ($directory.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 -or
                $directory.Name -cnotin @('main-browser-transport.previous', 'x-daily-owner.previous')) {
                throw 'Native staging contains an unexpected entry.'
            }
            $driver = if ($directory.Name -ceq 'main-browser-transport.previous') { 'main-browser-transport.cjs' } else { 'x-daily-owner.cjs' }
            $entries = @(Get-ChildItem -LiteralPath $directory.FullName -Force -ErrorAction Stop)
            if ($entries.Count -ne 2) { throw 'Native backup inventory is incomplete or unexpected.' }
            foreach ($entry in $entries) {
                if ($entry.PSIsContainer -or ($entry.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 -or
                    $entry.Name -cnotin @('manifest.json', $driver)) { throw 'Native backup contains an unexpected file.' }
            }
            $manifest = Get-Content -LiteralPath (Join-Path $directory.FullName 'manifest.json') -Raw | ConvertFrom-Json
            if ($manifest.schema -cne 'aku.x.application-owner-package.v1' -or $manifest.driver.file -cne $driver -or
                $manifest.driver.sha256 -notmatch '^[a-f0-9]{64}$' -or
                (Get-FileHash -LiteralPath (Join-Path $directory.FullName $driver) -Algorithm SHA256).Hash -ine $manifest.driver.sha256) {
                throw 'Native backup driver does not match its manifest.'
            }
            foreach ($entry in $entries) {
                $files += [pscustomobject]@{Path=$entry.FullName; Hash=(Get-FileHash -LiteralPath $entry.FullName -Algorithm SHA256).Hash}
            }
        }
        # Hold every file through deletion. Delete sharing permits unlinking but
        # blocks new readers/writers; existing incompatible handles defer all cleanup.
        foreach ($file in $files) {
            $handle = [IO.File]::Open($file.Path, [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::Delete)
            $handles.Add($handle)
            $sha = [Security.Cryptography.SHA256]::Create()
            try { $actual = [BitConverter]::ToString($sha.ComputeHash($handle)).Replace('-', '') } finally { $sha.Dispose() }
            if ($actual -cne $file.Hash) { throw 'Native backup changed during cleanup preflight.' }
        }
        if ($null -ne (Test-WorkerBackupProcessReference $stage)) { throw 'Native staging became referenced by a process.' }
        foreach ($file in $files) { Remove-Item -LiteralPath $file.Path -Force -ErrorAction Stop }
        foreach ($handle in $handles) { $handle.Dispose() }
        $handles.Clear()
        # Nonrecursive removal refuses any unexpected late additions.
        foreach ($directory in $directories) { [IO.Directory]::Delete($directory.FullName, $false) }
        [IO.Directory]::Delete($stage, $false)
        return $true
    } catch {
        Write-Warning "Native staging cleanup deferred at $StagePath : $($_.Exception.Message)"
        return $false
    } finally {
        foreach ($handle in $handles) { $handle.Dispose() }
    }
}

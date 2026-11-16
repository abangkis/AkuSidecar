# Loading this adapter never starts/stops a service or changes runtime files.
function Enter-DevelopmentPackageHandoff([string] $RuntimeRoot) {
    $root = [IO.Path]::GetFullPath($RuntimeRoot).ToUpperInvariant()
    $sha = [Security.Cryptography.SHA256]::Create()
    try { $identity = [BitConverter]::ToString($sha.ComputeHash([Text.Encoding]::UTF8.GetBytes($root))).Replace('-', '') }
    finally { $sha.Dispose() }
    $mutex = [Threading.Mutex]::new($false, ('Local\AkuSidecar.DevRuntime.' + $identity))
    try {
        try { $owned = $mutex.WaitOne(0) }
        catch [Threading.AbandonedMutexException] {
            $mutex.ReleaseMutex()
            throw 'Previous development operation ended unexpectedly; inspect the runtime before retrying.'
        }
        if (-not $owned) { throw 'Another development operation owns this runtime.' }
        return $mutex
    } catch { $mutex.Dispose(); throw }
}
function Complete-DevelopmentPackageHandoff {
    param(
        [Parameter(Mandatory)] [string] $RuntimeRoot,
        [Parameter(Mandatory)] [string] $KeepPath,
        [Parameter(Mandatory)] [string] $VerifiedBinarySha256
    )
    # The caller must finish health, image, bundle and settings verification.
    # Loading current helpers in function scope preserves snapshot helpers.
    . (Join-Path $PSScriptRoot 'worker-backup-retention.ps1')
    . (Join-Path $PSScriptRoot 'dev-runtime-contract.ps1')
    $null = Remove-DevRuntimeRollbacksAfterSuccess -RuntimeRoot $RuntimeRoot -KeepPath $KeepPath -VerifiedBinarySha256 $VerifiedBinarySha256
}


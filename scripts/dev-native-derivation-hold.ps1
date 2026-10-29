# Loading this helper has no effects. The restart workflow arms the gate only
# after the old process has exited and before the verified new process starts.
function Resolve-DevActivationMode([string] $Actor, [string] $ActivationMode) {
    if ($ActivationMode) { return $ActivationMode }
    if ($Actor -eq 'codex') { return 'no-session-transfer' }
    return 'daily'
}

function Assert-DevNativeHoldCapability($Probe) {
    if ($null -eq $Probe -or $Probe.status -cne 'ok' -or
        $Probe.schema -cne 'aku.native-derivation-hold-capability.v1' -or $Probe.version -ne 1) {
        throw 'Candidate cannot enforce no-transfer activation; runtime untouched.'
    }
}

function Write-DevNativeDerivationHold([string] $Target) {
    $absolute = [IO.Path]::GetFullPath($Target)
    if (-not (Test-Path -LiteralPath $absolute -PathType Leaf)) { throw 'Verified activation target is missing.' }
    $path = $absolute + '.native-derivation-hold.json'
    $pending = $path + '.pending-' + [Guid]::NewGuid().ToString('N')
    $value = [ordered]@{ schema = 'aku.native-derivation-hold.v1'; activationId = [Guid]::NewGuid().ToString('N'); sources = @('x', 'instagram') }
    try {
        [IO.File]::WriteAllText($pending, ($value | ConvertTo-Json -Compress), [Text.UTF8Encoding]::new($false))
        if (Test-Path -LiteralPath $path) {
            $entry = Get-Item -LiteralPath $path -Force
            if ($entry.PSIsContainer -or ($entry.Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Activation hold target is not an ordinary file.' }
            [IO.File]::Replace($pending, $path, [System.Management.Automation.Language.NullString]::Value)
        } else { [IO.File]::Move($pending, $path) }
    } finally {
        if (Test-Path -LiteralPath $pending) { Remove-Item -LiteralPath $pending -Force }
    }
    return $path
}

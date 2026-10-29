param()
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'dev-native-derivation-hold.ps1')
$repo = Split-Path -Parent $PSScriptRoot
$boundary = [IO.Path]::GetFullPath((Join-Path $repo 'build'))
$root = Join-Path $boundary ('native-hold-test-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $root | Out-Null
function Assert-HoldTest([bool] $Value, [string] $Message) { if (-not $Value) { throw $Message } }
try {
    Assert-HoldTest ((Resolve-DevActivationMode user '') -eq 'daily') 'Ordinary restart no longer defaults to daily.'
    Assert-HoldTest ((Resolve-DevActivationMode codex '') -eq 'no-session-transfer') 'Agent activation does not default to no transfer.'
    Assert-HoldTest ((Resolve-DevActivationMode codex daily) -eq 'daily') 'Explicit daily mode was ignored.'
    Assert-DevNativeHoldCapability ([pscustomobject]@{status='ok';schema='aku.native-derivation-hold-capability.v1';version=1})
    foreach ($probe in @($null, [pscustomobject]@{status='ok';version=1}, [pscustomobject]@{status='ok';schema='aku.native-derivation-hold-capability.v1';version=0})) {
        $rejected=$false; try { Assert-DevNativeHoldCapability $probe } catch { $rejected=$true }
        Assert-HoldTest $rejected 'Unsupported candidate could reach activation.'
    }
    $target=Join-Path $root 'aku-sidecar.exe'
    [IO.File]::WriteAllText($target,'synthetic-non-executable')
    $imageHash=(Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash
    $marker=Write-DevNativeDerivationHold $target
    $first=Get-Content -LiteralPath $marker -Raw | ConvertFrom-Json
    Assert-HoldTest ($first.schema -eq 'aku.native-derivation-hold.v1' -and ($first.sources -join ',') -eq 'x,instagram') 'Wrong hold scope.'
    $null=Write-DevNativeDerivationHold $target
    $second=Get-Content -LiteralPath $marker -Raw | ConvertFrom-Json
    Assert-HoldTest ($second.activationId -ne $first.activationId) 'Activation generation was reused.'
    Assert-HoldTest ((Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash -eq $imageHash) 'Gate preparation changed the binary.'
    Assert-HoldTest (@(Get-ChildItem -LiteralPath $root -Filter '*.pending-*').Count -eq 0) 'Partial marker was left behind.'
    $workflow=Get-Content -LiteralPath (Join-Path $PSScriptRoot 'dev-restart-workflow.ps1') -Raw
    Assert-HoldTest ($workflow.IndexOf('Assert-DevNativeHoldCapability') -lt $workflow.IndexOf('Wait-DevRuntimeGracefulShutdown')) 'Capability is checked too late.'
    Assert-HoldTest ($workflow.IndexOf('Write-DevNativeDerivationHold') -gt $workflow.IndexOf('Invoke-DevRuntimePromotion')) 'Hold armed before old-process retirement.'
    Assert-HoldTest ($workflow.IndexOf('Write-DevNativeDerivationHold') -lt $workflow.IndexOf('& $supervisor start')) 'Hold armed after startup.'
    Assert-HoldTest ($workflow.IndexOf('Write-DevNativeDerivationHold') -gt $workflow.IndexOf('if ($PrepareOnly)')) 'Prepare-only arms an operational hold.'
    Write-Output 'PASS: native derivation maintenance preparation is source scoped, defaults safely, and precedes startup.'
} finally {
    $absolute=[IO.Path]::GetFullPath($root)
    if ((Split-Path -Parent $absolute) -ne $boundary) { throw 'Invalid synthetic cleanup boundary.' }
    if (Test-Path -LiteralPath $absolute) { Remove-Item -LiteralPath $absolute -Recurse -Force }
}

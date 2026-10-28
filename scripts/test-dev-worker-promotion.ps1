param([string] $DiagnosticImage)
$ErrorActionPreference = 'Stop'
& (Join-Path $PSScriptRoot 'test-dev-runtime-contract.ps1') -DiagnosticImage $DiagnosticImage

# Retain the build-side worker staging regression, whose guarded transaction remains in build-dev.ps1.
$repoRoot = Split-Path -Parent $PSScriptRoot
$buildSource = Get-Content -LiteralPath (Join-Path $PSScriptRoot 'build-dev.ps1') -Raw
$buildStart = $buildSource.IndexOf('    $priorWorkerStage =')
$buildFinish = $buildSource.IndexOf('    $workerProvenance.destinationDirectory =', $buildStart)
if ($buildStart -lt 0 -or $buildFinish -lt $buildStart) { throw 'Development worker staging boundaries changed.' }
$buildPromotion = [scriptblock]::Create($buildSource.Substring($buildStart, $buildFinish - $buildStart))
$buildRoot = Join-Path $repoRoot 'build'
$fixture = Join-Path $buildRoot ('dev-worker-staging-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $fixture -Force | Out-Null
try {
    foreach ($scenario in @('success', 'rollback')) {
        $runtimeDir = Join-Path $fixture $scenario
        $workerBackupPrefix = '.test-staging-rollback-'
        $workerDestination = Join-Path $runtimeDir 'headless-worker.next'
        $workerStage = Join-Path $runtimeDir 'staged-worker'
        New-Item -ItemType Directory -Path $workerDestination -Force | Out-Null
        Set-Content -LiteralPath (Join-Path $workerDestination 'marker') -Value 'old'
        if ($scenario -eq 'success') {
            New-Item -ItemType Directory -Path $workerStage -Force | Out-Null
            Set-Content -LiteralPath (Join-Path $workerStage 'marker') -Value 'candidate'
            . $buildPromotion
            if ((Get-Content -LiteralPath (Join-Path $workerDestination 'marker') -Raw).Trim() -ne 'candidate') { throw 'Staged worker was not promoted.' }
            if ((Get-Content -LiteralPath (Join-Path $priorWorkerStage 'marker') -Raw).Trim() -ne 'old') { throw 'Previous staged worker was not retained.' }
        } else {
            $failed = $false
            try { . $buildPromotion } catch { $failed = $true }
            if (-not $failed -or (Get-Content -LiteralPath (Join-Path $workerDestination 'marker') -Raw).Trim() -ne 'old') { throw 'Failed build staging did not restore the old candidate.' }
        }
    }
    Write-Output 'PASS: build-dev worker staging preserves old worker and restores it when candidate move fails.'
} finally {
    $resolvedBuildRoot = [IO.Path]::GetFullPath($buildRoot).TrimEnd('\') + '\'
    $resolvedFixture = [IO.Path]::GetFullPath($fixture)
    if (-not $resolvedFixture.StartsWith($resolvedBuildRoot, [StringComparison]::OrdinalIgnoreCase)) { throw "Refusing to remove fixture outside build/: $resolvedFixture" }
    if (Test-Path -LiteralPath $resolvedFixture -PathType Container) { Remove-Item -LiteralPath $resolvedFixture -Recurse -Force }
}

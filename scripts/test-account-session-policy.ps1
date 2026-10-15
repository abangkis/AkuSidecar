param()
$ErrorActionPreference = 'Stop'
$projectRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$qaParent = Join-Path $projectRoot 'runtime\qa\auth-session-sidecar'
if (!(Test-Path -LiteralPath $qaParent -PathType Container)) { throw 'Project QA parent is required.' }
if ((Get-Item -LiteralPath $qaParent).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'QA parent is a reparse point.' }
$taskRoot = Join-Path $qaParent ('run-session-policy-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $taskRoot | Out-Null
$variables = @('TEMP','TMP','GOTMPDIR','GOPROXY','GOSUMDB','GOTOOLCHAIN','CGO_ENABLED','GOWORK','AKUSIDECA_ACCOUNT_CHECKPOINT_QA_ROOT','AKU_POLICY_STAGE','AKU_POLICY_OLD_PROBE')
$previous = @{}
foreach ($variable in $variables) { $previous[$variable] = [Environment]::GetEnvironmentVariable($variable,'Process') }
$passed = $false
Push-Location -LiteralPath $projectRoot
try {
    $env:TEMP=$taskRoot; $env:TMP=$taskRoot; $env:GOTMPDIR=$taskRoot
    $env:GOPROXY='off'; $env:GOSUMDB='off'; $env:GOTOOLCHAIN='local'; $env:CGO_ENABLED='0'; $env:GOWORK='off'
    $env:AKUSIDECA_ACCOUNT_CHECKPOINT_QA_ROOT=$null; $env:AKU_POLICY_STAGE=$null; $env:AKU_POLICY_OLD_PROBE=$null
    foreach ($build in @('default','dev','fixture')) {
        $arguments = @('test','-c','-o',(Join-Path $taskRoot ($build+'.exe')))
        if ($build -ne 'default') { $arguments += @('-tags',('auth_session_'+$build)) }
        & go @arguments './internal/store'
        if ($LASTEXITCODE -ne 0) { throw ('Build failed: '+$build) }
        & (Join-Path $taskRoot ($build+'.exe')) '-test.run=^TestAccountSessionPolicy|^TestSyntheticAccountPolicy|^TestAccountCheckpoint|^TestSyntheticAccountCheckpoint' '-test.count=1' '-test.timeout=60s' '-test.v'
        if ($LASTEXITCODE -ne 0) { throw ('Contracts failed: '+$build) }
    }
    # Local immutable historical source, no fetch and no modification of this checkout.
    $baseline = '9fc79f38921752628e13e23428683199833631cd'
    $oldSource = Join-Path $taskRoot 'old-source'
    New-Item -ItemType Directory -Path $oldSource | Out-Null
    & git archive --format=tar -o (Join-Path $taskRoot 'old-source.tar') $baseline -- go.mod go.sum internal
    if ($LASTEXITCODE -ne 0) { throw 'Pinned baseline archive failed.' }
    & tar -xf (Join-Path $taskRoot 'old-source.tar') -C $oldSource
    if ($LASTEXITCODE -ne 0) { throw 'Pinned baseline extraction failed.' }
    foreach ($file in @('account_session_policy_compat_test.go','account_session_policy_old_fixture_probe_test.go')) {
        Copy-Item -LiteralPath (Join-Path $projectRoot ('internal\store\'+$file)) -Destination (Join-Path $oldSource ('internal\store\'+$file))
    }
    Push-Location -LiteralPath $oldSource
    try {
        foreach ($build in @('default','fixture')) {
            $arguments=@('test','-c','-o',(Join-Path $taskRoot ('old-'+$build+'.exe')))
            if ($build -eq 'fixture') { $arguments+=@('-tags','auth_session_fixture') }
            & go @arguments './internal/store'
            if ($LASTEXITCODE -ne 0) { throw ('Historical build failed: '+$build) }
        }
    } finally { Pop-Location }
    $env:AKUSIDECA_ACCOUNT_CHECKPOINT_QA_ROOT=$taskRoot
    foreach ($stage in @('enroll','migrate','enable','session-change','disable','reenable')) {
        $env:AKU_POLICY_STAGE=$stage
        & (Join-Path $taskRoot 'dev.exe') '-test.run=^TestSyntheticAccountPolicyCompatibilityStage$' '-test.count=1' '-test.timeout=60s' '-test.v'
        if ($LASTEXITCODE -ne 0) { throw ('Policy stage failed: '+$stage) }
        if ($stage -eq 'enroll') { continue }
        $env:AKU_POLICY_STAGE='closed'
        foreach ($build in @('default','fixture','old-default','old-fixture')) {
            $env:AKU_POLICY_OLD_PROBE = if ($build -eq 'old-fixture') { '1' } else { $null }
            Write-Output ('POLICY_DOWNGRADE='+$stage+'/'+$build+' baseline='+$baseline)
            & (Join-Path $taskRoot ($build+'.exe')) '-test.run=^TestAccountSessionPolicyDowngradeProbe$|^TestAccountSessionPolicyOldAuthorizationRejected$' '-test.count=1' '-test.timeout=60s' '-test.v'
            if ($LASTEXITCODE -ne 0) { throw ('Policy downgrade failed: '+$stage+'/'+$build) }
        }
        $env:AKU_POLICY_OLD_PROBE=$null
    }
    $passed=$true
    Write-Output 'Policy acceptance passed: five builds, 26 stage processes, same synthetic SQLite; no browser/login/Sidecar runtime.'
} finally {
    Pop-Location
    foreach ($variable in $variables) { [Environment]::SetEnvironmentVariable($variable,$previous[$variable],'Process') }
    if ($passed) {
        $resolved=(Resolve-Path -LiteralPath $taskRoot).ProviderPath
        if ([IO.Path]::GetDirectoryName($resolved) -ne $qaParent -or $resolved -ne $taskRoot) { throw 'Cleanup root escaped QA parent.' }
        $reparse=Get-ChildItem -LiteralPath $resolved -Recurse -Force | Where-Object { $_.Attributes -band [IO.FileAttributes]::ReparsePoint }
        if (((Get-Item -LiteralPath $resolved).Attributes -band [IO.FileAttributes]::ReparsePoint) -or $reparse) { throw 'Cleanup refuses reparse points.' }
        Remove-Item -LiteralPath $resolved -Recurse -Force
        Write-Output 'Policy QA database, historical source copy and binaries cleaned.'
    } else { Write-Warning ('Policy QA artifacts retained: '+$taskRoot) }
}

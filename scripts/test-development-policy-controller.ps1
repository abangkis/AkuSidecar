param()
$ErrorActionPreference='Stop'
$projectRoot=(Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$qaParent=Join-Path $projectRoot 'runtime\qa\auth-session-sidecar'
New-Item -ItemType Directory -Path $qaParent -Force | Out-Null
if ((Get-Item -LiteralPath $qaParent).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'QA parent must not be a reparse point.' }
$taskRoot=Join-Path $qaParent ('run-policy-controller-'+[guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $taskRoot | Out-Null
$variables=@('TEMP','TMP','GOTMPDIR','GOPROXY','GOSUMDB','GOTOOLCHAIN','CGO_ENABLED','AKU_CHROME_FIXTURE_ROOT','AKU_PROCESS_FIXTURE_ENABLED','AKUSIDECA_ACCOUNT_CHECKPOINT_QA_ROOT','AKUSIDECA_ACCOUNT_CHECKPOINT_QA_STAGE','AKU_POLICY_STAGE','AKU_POLICY_OLD_PROBE')
$previous=@{}
foreach ($variable in $variables) { $previous[$variable]=[Environment]::GetEnvironmentVariable($variable,'Process') }
$passed=$false
Push-Location -LiteralPath $projectRoot
try {
    $env:TEMP=$taskRoot; $env:TMP=$taskRoot; $env:GOTMPDIR=$taskRoot
    $env:GOPROXY='off'; $env:GOSUMDB='off'; $env:GOTOOLCHAIN='local'; $env:CGO_ENABLED='0'
    foreach ($variable in $variables | Where-Object { $_.StartsWith('AKU') }) { [Environment]::SetEnvironmentVariable($variable,$null,'Process') }
    & go test -tags 'auth_session_dev auth_session_fixture' ./internal/engine ./internal/store -run '^TestDevelopmentPolicy|^TestDevelopmentSession|^TestDevelopmentCheckpoint|^TestDevelopmentReader|^TestDevelopmentPause|^TestSyntheticAccountEnrollment|^TestSyntheticAccountPolicy|^TestAccountSessionPolicy' -count=1 -timeout=90s -v
    if ($LASTEXITCODE -ne 0) { throw 'Synthetic policy controller acceptance failed.' }
    $passed=$true
    Write-Output 'Policy controller acceptance passed: synthetic probes/confirmation, simulated owner, project-local SQLite; no Chrome, login, user runtime or registration.'
} finally {
    Pop-Location
    foreach ($variable in $variables) { [Environment]::SetEnvironmentVariable($variable,$previous[$variable],'Process') }
    if ($passed) {
        $resolved=(Resolve-Path -LiteralPath $taskRoot).ProviderPath
        if ($resolved -ne $taskRoot -or [IO.Path]::GetDirectoryName($resolved) -ne $qaParent -or ((Get-Item -LiteralPath $resolved).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Refusing cleanup outside owned QA root.' }
        if (Get-ChildItem -LiteralPath $resolved -Recurse -Force | Where-Object { $_.Attributes -band [IO.FileAttributes]::ReparsePoint }) { throw 'Refusing cleanup through reparse point.' }
        Remove-Item -LiteralPath $resolved -Recurse -Force
        Write-Output 'Policy controller QA artifacts cleaned.'
    } else { Write-Warning ('Policy controller QA artifacts retained at '+$taskRoot) }
}

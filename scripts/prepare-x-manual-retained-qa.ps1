param([switch]$RunPrepared,[string]$PreparedRoot,[switch]$TransferAuthorized)
$ErrorActionPreference='Stop'
$project=(Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$parent=Join-Path $project 'runtime\qa\auth-session-sidecar'
if($RunPrepared -and (!$TransferAuthorized -or !$PreparedRoot)){throw 'Execution requires a prepared root and a fresh explicit one-transfer authorization.'}
if(!$RunPrepared -and ($TransferAuthorized -or $PreparedRoot)){throw 'Preparation takes no execution authorization or existing root.'}
foreach($relative in @('runtime','runtime\qa','runtime\qa\auth-session-sidecar','.go-build','.go-build\x-session-verifier','.go-build\mod')){
 $path=Join-Path $project $relative
 if(!(Test-Path -LiteralPath $path)){New-Item -ItemType Directory -Path $path | Out-Null}
 if((Get-Item -LiteralPath $path).Attributes -band [IO.FileAttributes]::ReparsePoint){throw 'QA paths must be ordinary project directories.'}
}
$node=Join-Path $project 'runtime\dev\headless-worker\node.exe'
$chrome='C:\Program Files\Google\Chrome\Application\chrome.exe'
foreach($path in @($node,$chrome)){if(!(Test-Path -LiteralPath $path -PathType Leaf)){throw 'Configured official QA executable unavailable.'}}
if($RunPrepared){$root=[IO.Path]::GetFullPath($PreparedRoot)}else{$root=Join-Path $parent ('run-x-qa-source-'+[guid]::NewGuid().ToString('N'));New-Item -ItemType Directory -Path $root | Out-Null}
$resolved=(Resolve-Path -LiteralPath $root).ProviderPath
if($resolved -ne $root -or [IO.Path]::GetDirectoryName($resolved) -ne $parent -or [IO.Path]::GetFileName($resolved) -notmatch '^run-x-qa-source-[a-f0-9]{32}$' -or ((Get-Item -LiteralPath $resolved).Attributes -band [IO.FileAttributes]::ReparsePoint)){throw 'Prepared root must be an owned direct QA child.'}
if(Get-ChildItem -LiteralPath $root -Recurse -Force | Where-Object {$_.Attributes -band [IO.FileAttributes]::ReparsePoint}){throw 'QA root contains a reparse point.'}
$id=[IO.Path]::GetFileName($root).Substring(16)
$log=Join-Path $parent ('x-manual-retained-'+$id+'.'+$(if($RunPrepared){'run'}else{'prepare'})+'.log.txt')
$exe=Join-Path $root 'x-manual-retained.test.exe'
$planPath=Join-Path $root 'prepared-plan.json'
$marker=Join-Path $root 'cleanup.verified'
$names=@('TEMP','TMP','GOTMPDIR','GOCACHE','GOMODCACHE','GOWORK','GOPROXY','GOSUMDB','GOTOOLCHAIN','CGO_ENABLED','GOFLAGS',
 'AKU_NATIVE_MANUAL_PROJECT_ROOT','AKU_CHROME_FIXTURE_ROOT','AKU_CHROME_FIXTURE_CLEANUP_MARKER','AKU_CHROME_FIXTURE_NODE','AKU_CHROME_FIXTURE_EXE',
 'AKU_X_MANUAL_RETAINED_QA_ENABLED','AKU_X_MANUAL_RETAINED_TRANSFER_ONCE','AKU_X_QA_SHORT_TRANSFER_ALLOWED','AKU_X_NATIVE_MANUAL_STARTUP_ALLOWED')
$previous=@{};foreach($name in $names){$previous[$name]=[Environment]::GetEnvironmentVariable($name,'Process')}
$remove=$false
Push-Location -LiteralPath $project
try{
 $env:TEMP=$root;$env:TMP=$root;$env:GOTMPDIR=$root
 $env:GOCACHE=Join-Path $project '.go-build\x-session-verifier';$env:GOMODCACHE=Join-Path $project '.go-build\mod'
 $env:GOWORK='off';$env:GOPROXY='off';$env:GOSUMDB='off';$env:GOTOOLCHAIN='local';$env:CGO_ENABLED='0';$env:GOFLAGS=''
 $env:AKU_NATIVE_MANUAL_PROJECT_ROOT=$project;$env:AKU_CHROME_FIXTURE_ROOT=$root;$env:AKU_CHROME_FIXTURE_CLEANUP_MARKER=$marker
 $env:AKU_CHROME_FIXTURE_NODE=$node;$env:AKU_CHROME_FIXTURE_EXE=$chrome
 $env:AKU_X_MANUAL_RETAINED_QA_ENABLED='0';$env:AKU_X_MANUAL_RETAINED_TRANSFER_ONCE='0';$env:AKU_X_QA_SHORT_TRANSFER_ALLOWED='0';$env:AKU_X_NATIVE_MANUAL_STARTUP_ALLOWED='0'
 Write-Output ('Retained manual QA '+$(if($RunPrepared){'execution'}else{'preparation'})+': '+$id) | Tee-Object -FilePath $log
 if(!$RunPrepared){
  & $node --test --test-timeout=15000 scripts/auth-session-fixture/x-manual-retained-setup.test.mjs scripts/auth-session-fixture/x-native-metadata-probe-driver.test.mjs scripts/auth-session-fixture/x-manual-enrollment-capture.test.mjs scripts/auth-session-fixture/x-manual-startup-diagnostic.test.mjs 2>&1 | Tee-Object -FilePath $log -Append
  if($LASTEXITCODE -ne 0){throw 'Retained setup offline regressions failed.'}
  foreach($mode in @('default','auth_session_dev','auth_session_fixture','auth_session_dev auth_session_fixture')){
   $arguments=@('list','-f','{{range .TestGoFiles}}{{println .}}{{end}}')
   if($mode -ne 'default'){$arguments+=@('-tags',$mode)}
   $files=@(& go @arguments './internal/engine')
   if($LASTEXITCODE -ne 0 -or (($files -contains 'x_manual_pilot_retained_windows_test.go') -ne ($mode -eq 'auth_session_dev auth_session_fixture'))){throw 'Retained QA entry tag gate failed.'}
   Write-Output ('PASS: retained QA tag gate '+$mode) | Tee-Object -FilePath $log -Append
  }
  $raw=& $node scripts/auth-session-fixture/stage-native-manual-chrome.mjs $root x-retained-manual
  if($LASTEXITCODE -ne 0){throw 'Retained QA exact-source staging failed.'}
  $stage=$raw | ConvertFrom-Json
  $sources=@($stage.files | ForEach-Object {Join-Path $stage.stage $_})
  & go test -c -tags 'auth_session_dev auth_session_fixture' -o $exe @sources 2>&1 | Tee-Object -FilePath $log -Append
  if($LASTEXITCODE -ne 0){throw 'Retained QA standalone compilation failed.'}
  & $exe '-test.run=^TestXManualPilotRetained(QA|SeedDiagnostics|SeedNodeDiagnostic|StartupDiagnostic)$' '-test.timeout=10s' '-test.v' 2>&1 | Tee-Object -FilePath $log -Append
  if($LASTEXITCODE -ne 0){throw 'Default-off entry rejection failed.'}
  $provenance=Get-Content -LiteralPath (Join-Path $root 'staging-provenance.json') -Raw | ConvertFrom-Json
  $paths=@($provenance.copied.source)+@($provenance.extracted.source)+@($provenance.nodeSources.source)+@('scripts/auth-session-fixture/x-manual-retained-setup.mjs','scripts/auth-session-fixture/x-retained-source-qualification-driver.mjs','scripts/prepare-x-manual-retained-qa.ps1')
  $fingerprints=@($paths | Sort-Object -Unique | ForEach-Object {@{source=$_;sha256=(Get-FileHash -LiteralPath $_ -Algorithm SHA256).Hash.ToLower()}})
  $plan=@{schema='aku.x.manual-retained-preparation.v1';project=$project;root=$root;maxAttemptMS=60000;defaultOff=$true;executionApproved=$false;cookieNames=@('auth_token','ct0');sources=$fingerprints;
   executableSHA256=(Get-FileHash -LiteralPath $exe -Algorithm SHA256).Hash.ToLower();nodeSHA256=(Get-FileHash -LiteralPath $node -Algorithm SHA256).Hash.ToLower();chromeSHA256=(Get-FileHash -LiteralPath $chrome -Algorithm SHA256).Hash.ToLower()}
  $plan | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $planPath -Encoding utf8NoBOM
  Copy-Item -LiteralPath (Join-Path $root 'staging-provenance.json') -Destination (Join-Path $parent ('x-manual-retained-'+$id+'.provenance.json'))
  Write-Output 'PASS: prepared only; runtime entry skipped before lease/profile access. No Chrome, X request, transfer, Store or user runtime executed.' | Tee-Object -FilePath $log -Append
  Write-Output ('Prepared root: '+$root)
 }else{
  $plan=Get-Content -LiteralPath $planPath -Raw | ConvertFrom-Json
  if($plan.schema -ne 'aku.x.manual-retained-preparation.v1' -or $plan.project -ne $project -or $plan.root -ne $root -or $plan.maxAttemptMS -ne 60000 -or $plan.defaultOff -ne $true -or $plan.executionApproved -ne $false){throw 'Prepared plan does not match the bounded QA contract.'}
  if((Get-FileHash -LiteralPath $exe -Algorithm SHA256).Hash.ToLower() -ne $plan.executableSHA256 -or (Get-FileHash -LiteralPath $node -Algorithm SHA256).Hash.ToLower() -ne $plan.nodeSHA256 -or (Get-FileHash -LiteralPath $chrome -Algorithm SHA256).Hash.ToLower() -ne $plan.chromeSHA256){throw 'Prepared executable drift; prepare again before authorization.'}
  foreach($source in $plan.sources){if((Get-FileHash -LiteralPath (Join-Path $project $source.source) -Algorithm SHA256).Hash.ToLower() -ne $source.sha256){throw 'Prepared source drift; prepare again before authorization.'}}
  if(Test-Path -LiteralPath (Join-Path $root 'derived-headless')){throw 'Collector profile must not exist before the one-shot run.'}
  $claim=[IO.File]::Open((Join-Path $root 'execution.claim'),[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None);$claim.Dispose()
  $env:AKU_X_MANUAL_RETAINED_QA_ENABLED='1';$env:AKU_X_MANUAL_RETAINED_TRANSFER_ONCE='1';$env:AKU_X_QA_SHORT_TRANSFER_ALLOWED='1';$env:AKU_X_NATIVE_MANUAL_STARTUP_ALLOWED='1'
  & $exe '-test.run=^TestXManualPilotRetainedQA$' '-test.timeout=90s' '-test.v' 2>&1 | Tee-Object -FilePath $log -Append
  $exitCode=$LASTEXITCODE
  $remove=(Test-Path -LiteralPath $marker -PathType Leaf) -and (Get-Content -LiteralPath $marker -Raw).Trim() -eq 'verified'
  if(!$remove){throw 'Owned Job/lease cleanup unverified; prepared root retained, no retry.'}
  if($exitCode -ne 0){throw 'Integrated retained QA rejected; bounded log retained, no retry.'}
 }
}finally{
 Pop-Location
 foreach($name in $names){[Environment]::SetEnvironmentVariable($name,$previous[$name],'Process')}
 if($remove){
  $resolved=(Resolve-Path -LiteralPath $root).ProviderPath
  if($resolved -ne $root -or [IO.Path]::GetDirectoryName($resolved) -ne $parent -or ((Get-Item -LiteralPath $resolved).Attributes -band [IO.FileAttributes]::ReparsePoint)){throw 'Refusing cleanup outside owned prepared root.'}
  if(Get-ChildItem -LiteralPath $resolved -Recurse -Force | Where-Object {$_.Attributes -band [IO.FileAttributes]::ReparsePoint}){throw 'Refusing cleanup through reparse point.'}
  Remove-Item -LiteralPath $resolved -Recurse -Force
  Write-Output 'Owned collector/build root removed; retained reader profile preserved.'
 }
}

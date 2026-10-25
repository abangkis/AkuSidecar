param()
$ErrorActionPreference='Stop'
$projectRoot=(Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$qaParent=Join-Path $projectRoot 'runtime\qa\auth-session-sidecar'
foreach($relative in @('runtime','runtime\qa','runtime\qa\auth-session-sidecar','.go-build','.go-build\x-session-verifier','.go-build\mod')){
 $path=Join-Path $projectRoot $relative
 if(!(Test-Path -LiteralPath $path)){New-Item -ItemType Directory -Path $path | Out-Null}
 if((Get-Item -LiteralPath $path).Attributes -band [IO.FileAttributes]::ReparsePoint){throw 'Owner test paths must be ordinary project directories.'}
}
$node=Join-Path $projectRoot 'runtime\dev\headless-worker\node.exe'
if(!(Test-Path -LiteralPath $node -PathType Leaf)){throw 'Project Node executable unavailable.'}
$runId=[guid]::NewGuid().ToString('N')
$taskRoot=Join-Path $qaParent ('run-x-manual-owner-'+$runId)
New-Item -ItemType Directory -Path $taskRoot | Out-Null
$log=Join-Path $qaParent ('x-manual-owner-'+$runId+'.log.txt')
$variables=@('TEMP','TMP','GOTMPDIR','GOCACHE','GOMODCACHE','GOWORK','GOPROXY','GOSUMDB','GOTOOLCHAIN','CGO_ENABLED','GOFLAGS',
 'AKU_NATIVE_MANUAL_PROJECT_ROOT','AKU_CHROME_FIXTURE_NODE','AKU_X_NATIVE_MANUAL_STARTUP_ALLOWED','AKU_X_NATIVE_MANUAL_OWNER_OFFLINE')
$previous=@{};foreach($name in $variables){$previous[$name]=[Environment]::GetEnvironmentVariable($name,'Process')}
Push-Location -LiteralPath $projectRoot
try{
 $env:TEMP=$taskRoot;$env:TMP=$taskRoot;$env:GOTMPDIR=$taskRoot
 $env:GOCACHE=Join-Path $projectRoot '.go-build\x-session-verifier';$env:GOMODCACHE=Join-Path $projectRoot '.go-build\mod'
 $env:GOWORK='off';$env:GOPROXY='off';$env:GOSUMDB='off';$env:GOTOOLCHAIN='local';$env:CGO_ENABLED='0';$env:GOFLAGS=''
 $env:AKU_NATIVE_MANUAL_PROJECT_ROOT=$projectRoot;$env:AKU_CHROME_FIXTURE_NODE=$node
 $env:AKU_X_NATIVE_MANUAL_STARTUP_ALLOWED='0';$env:AKU_X_NATIVE_MANUAL_OWNER_OFFLINE='0'
 Write-Output ('Offline X manual owner run: '+$runId) | Tee-Object -FilePath $log
 $nodeTests=@('scripts/auth-session-fixture/x-native-manual-owner-port.test.mjs','scripts/auth-session-fixture/x-native-metadata-probe-driver.test.mjs',
  'scripts/auth-session-fixture/x-manual-startup-owner.test.mjs','scripts/auth-session-fixture/x-qa-feasibility-collector.test.mjs',
  'scripts/auth-session-fixture/x-manual-startup-first-cause.test.mjs','scripts/auth-session-fixture/x-native-manual-capture.test.mjs')
 & $node --test --test-timeout=15000 @nodeTests 2>&1 | Tee-Object -FilePath $log -Append
 if($LASTEXITCODE -ne 0){throw 'X owner Node regressions failed.'}
 $core=@('development_native_pilot_adapter.go','development_native_pilot_startup.go','development_native_pilot_controller.go','development_native_pilot_broker.go',
  'development_x_manual_pilot_owner.go','development_x_manual_pilot_owner_test.go','development_x_manual_pilot_owner_node_test.go')
 $coreFiles=@($core | ForEach-Object {Join-Path 'internal/engine' $_})
 & go test -tags auth_session_dev -run '^TestXManualPilotOwner' -count=1 -timeout=60s -v @coreFiles 2>&1 | Tee-Object -FilePath $log -Append
 if($LASTEXITCODE -ne 0){throw 'X owner standalone Go tests failed.'}
 foreach($mode in @('default','auth_session_dev','auth_session_fixture','auth_session_dev auth_session_fixture')){
  $arguments=@('list','-f','{{range .GoFiles}}{{println .}}{{end}}')
  if($mode -ne 'default'){$arguments+=@('-tags',$mode)}
  $files=@(& go @arguments './internal/engine')
  if($LASTEXITCODE -ne 0){throw 'X owner build-tag inspection failed.'}
  if(($files -contains 'development_x_manual_pilot_owner_windows.go') -ne ($mode -eq 'auth_session_dev auth_session_fixture')){throw 'Native X owner Windows tag gate failed.'}
  if(($files -contains 'development_x_manual_pilot_owner.go') -ne $mode.Contains('auth_session_dev')){throw 'X owner core tag gate failed.'}
  Write-Output ('PASS: X owner tag gate '+$mode) | Tee-Object -FilePath $log -Append
 }
 $raw=& $node scripts/auth-session-fixture/stage-native-manual-chrome.mjs $taskRoot x-owner-port
 if($LASTEXITCODE -ne 0){throw 'X owner exact-source staging failed.'}
 $stage=$raw | ConvertFrom-Json
 $sources=@($stage.files | ForEach-Object {Join-Path $stage.stage $_})
 Copy-Item -LiteralPath (Join-Path $taskRoot 'staging-provenance.json') -Destination (Join-Path $qaParent ('x-manual-owner-'+$runId+'.provenance.json'))
 & go test -c -tags 'auth_session_dev auth_session_fixture' -o (Join-Path $taskRoot 'x-owner-compile.test.exe') @sources 2>&1 | Tee-Object -FilePath $log -Append
 if($LASTEXITCODE -ne 0){throw 'Windows X owner backend compilation failed.'}
 Write-Output 'PASS: private X owner composition, accountless stdio, tag gates and Windows backend compilation. No Chrome/native QA/profile/session transfer/Store/user runtime executed.' | Tee-Object -FilePath $log -Append
 Write-Output ('Offline X owner log: '+$log)
}finally{
 Pop-Location
 foreach($name in $variables){[Environment]::SetEnvironmentVariable($name,$previous[$name],'Process')}
 $resolved=(Resolve-Path -LiteralPath $taskRoot).ProviderPath
 if($resolved -ne $taskRoot -or [IO.Path]::GetDirectoryName($resolved) -ne $qaParent -or ((Get-Item -LiteralPath $resolved).Attributes -band [IO.FileAttributes]::ReparsePoint)){throw 'Refusing cleanup outside owned X owner test root.'}
 if(Get-ChildItem -LiteralPath $resolved -Recurse -Force | Where-Object {$_.Attributes -band [IO.FileAttributes]::ReparsePoint}){throw 'Refusing cleanup through a reparse point.'}
 Remove-Item -LiteralPath $resolved -Recurse -Force
 Write-Output 'Owned accountless X owner build root removed.'
}

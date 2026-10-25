param()
$ErrorActionPreference='Stop'
$project=(Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$parent=Join-Path $project 'runtime\qa\auth-session-sidecar'
$id=[guid]::NewGuid().ToString('N')
$root=Join-Path $parent ('run-owned-browser-core-'+$id)
$ownerRoot=Join-Path $parent ('run-owned-browser-owner-'+$id)
foreach($path in @($parent,$root,$ownerRoot,(Join-Path $project '.go-build\x-session-verifier'),(Join-Path $project '.go-build\mod'))){
 if(!(Test-Path -LiteralPath $path)){New-Item -ItemType Directory -Path $path -Force | Out-Null}
 if((Get-Item -LiteralPath $path).Attributes -band [IO.FileAttributes]::ReparsePoint){throw 'Owned browser core paths must be ordinary directories.'}
}
$log=Join-Path $parent ('owned-browser-core-'+$id+'.log.txt')
$names=@('TEMP','TMP','GOTMPDIR','GOCACHE','GOMODCACHE','GOWORK','GOPROXY','GOSUMDB','GOTOOLCHAIN','CGO_ENABLED','GOFLAGS',
 'AKU_NATIVE_MANUAL_PROJECT_ROOT','AKU_CHROME_FIXTURE_NODE','AKU_X_NATIVE_MANUAL_CAPTURE_OFFLINE',
 'AKU_X_MANUAL_RETAINED_QA_ENABLED','AKU_X_MANUAL_RETAINED_TRANSFER_ONCE','AKU_X_QA_SHORT_TRANSFER_ALLOWED',
 'AKU_X_NATIVE_MANUAL_STARTUP_ALLOWED','AKU_X_NATIVE_MANUAL_CAPTURE_ALLOWED','AKU_X_NATIVE_MANUAL_OWNER_OFFLINE')
$previous=@{};foreach($name in $names){$previous[$name]=[Environment]::GetEnvironmentVariable($name,'Process')}
Push-Location -LiteralPath $project
try{
 $env:TEMP=$root;$env:TMP=$root;$env:GOTMPDIR=$root
 $env:GOCACHE=Join-Path $project '.go-build\x-session-verifier';$env:GOMODCACHE=Join-Path $project '.go-build\mod'
 $env:GOWORK='off';$env:GOPROXY='off';$env:GOSUMDB='off';$env:GOTOOLCHAIN='local';$env:CGO_ENABLED='0';$env:GOFLAGS=''
 $node=Join-Path $project 'runtime\dev\headless-worker\node.exe'
 $env:AKU_NATIVE_MANUAL_PROJECT_ROOT=$project;$env:AKU_CHROME_FIXTURE_NODE=$node;$env:AKU_X_NATIVE_MANUAL_CAPTURE_OFFLINE='1'
 foreach($name in @('AKU_X_MANUAL_RETAINED_QA_ENABLED','AKU_X_MANUAL_RETAINED_TRANSFER_ONCE','AKU_X_QA_SHORT_TRANSFER_ALLOWED','AKU_X_NATIVE_MANUAL_STARTUP_ALLOWED','AKU_X_NATIVE_MANUAL_CAPTURE_ALLOWED','AKU_X_NATIVE_MANUAL_OWNER_OFFLINE')){[Environment]::SetEnvironmentVariable($name,'0','Process')}
 Write-Output ('Offline owned browser core: '+$id) | Tee-Object -FilePath $log
 & go test ./internal/ownedrpc ./internal/ownedprocess -count=1 -timeout=30s -v 2>&1 | Tee-Object -FilePath $log -Append
 if($LASTEXITCODE -ne 0){throw 'Owned browser core regression failed.'}
 $production=@(& go list -tags 'auth_session_dev auth_session_fixture' -f '{{range .GoFiles}}{{println .}}{{end}}' ./internal/engine)
 if($LASTEXITCODE -ne 0){throw 'Engine source inventory unavailable.'}
 $sources=@($production | Where-Object {$_.Trim() -ne ''} | ForEach-Object {Join-Path $project ('internal/engine/'+$_)})
 foreach($file in @('development_native_pilot_store_test.go','development_native_pilot_x_capture_test.go','development_native_pilot_capture_diagnostic_test.go')){$sources+=Join-Path $project ('internal/engine/'+$file)}
 & go test -tags 'auth_session_dev auth_session_fixture' -run '^TestNativePilotXCapture' -count=1 -timeout=60s -v @sources 2>&1 | Tee-Object -FilePath $log -Append
 if($LASTEXITCODE -ne 0){throw 'Existing capture Engine/Store offline compatibility failed.'}
 foreach($mode in @('x-owner-port','x-retained-capture')){
  $stageRoot=if($mode -eq 'x-owner-port'){$ownerRoot}else{$root}
  $raw=& $node scripts/auth-session-fixture/stage-native-manual-chrome.mjs $stageRoot $mode
  if($LASTEXITCODE -ne 0){throw 'Owned transport source staging failed.'}
  $stage=$raw | ConvertFrom-Json
  $provenance=Get-Content -LiteralPath (Join-Path $stageRoot 'staging-provenance.json') -Raw | ConvertFrom-Json
  foreach($file in @('internal/ownedprocess/identity_windows.go','internal/ownedrpc/client.go')){
   $entry=@($provenance.moduleDependencies | Where-Object {$_.source -eq $file})
   if($entry.Count -ne 1 -or $entry[0].sha256 -ne (Get-FileHash -LiteralPath (Join-Path $project $file) -Algorithm SHA256).Hash.ToLower()){throw 'Shared ownership/transport source fingerprint missing or mismatched.'}
  }
  $stagedSources=@($stage.files | ForEach-Object {Join-Path $stage.stage $_})
  $image=Join-Path $stageRoot 'owned-transport.test.exe'
  & go test -c -tags 'auth_session_dev auth_session_fixture' -o $image @stagedSources 2>&1 | Tee-Object -FilePath $log -Append
  if($LASTEXITCODE -ne 0){throw ('Owned transport staging compile failed: '+$mode)}
  Copy-Item -LiteralPath (Join-Path $stageRoot 'staging-provenance.json') -Destination (Join-Path $parent ('owned-browser-core-'+$id+'.'+$mode+'.provenance.json'))
  Write-Output ('PASS: source-pinned compatibility staging '+$mode) | Tee-Object -FilePath $log -Append
  if($mode -eq 'x-retained-capture'){
   & $image '-test.run=^TestXManual(CaptureDiagnostic(Code|Pipe)|PilotRetained(SeedDiagnostics|SeedNodeDiagnostic|StartupDiagnostic|TimingProgress|TraceCodes))$' '-test.timeout=30s' '-test.v' 2>&1 | Tee-Object -FilePath $log -Append
   if($LASTEXITCODE -ne 0){throw 'Actual accountless Node/Go transport compatibility failed.'}
  }
 }
 Write-Output 'PASS: reusable correlation, local Windows ownership and synthetic Store compatibility; no Chrome, retained profile, X request, session transfer or user runtime.' | Tee-Object -FilePath $log -Append
 Write-Output ('Evidence: '+$log)
}finally{
 Pop-Location
 foreach($name in $names){[Environment]::SetEnvironmentVariable($name,$previous[$name],'Process')}
 foreach($ownedRoot in @($root,$ownerRoot)){
  $resolved=(Resolve-Path -LiteralPath $ownedRoot).ProviderPath
  if($resolved -ne $ownedRoot -or [IO.Path]::GetDirectoryName($resolved) -ne $parent -or ((Get-Item -LiteralPath $resolved).Attributes -band [IO.FileAttributes]::ReparsePoint)){throw 'Refusing cleanup outside owned core root.'}
  if(Get-ChildItem -LiteralPath $resolved -Recurse -Force | Where-Object {$_.Attributes -band [IO.FileAttributes]::ReparsePoint}){throw 'Refusing cleanup through reparse point.'}
  Remove-Item -LiteralPath $resolved -Recurse -Force
 }
}

param([switch]$MainOnly)
$ErrorActionPreference='Stop'
$project=(Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$parent=Join-Path $project 'runtime\qa\auth-session-sidecar'
$id=[guid]::NewGuid().ToString('N')
$root=Join-Path $parent ('run-x-development-entry-'+$id)
$log=Join-Path $parent ('x-development-entry-'+$id+'.log.txt')
foreach($path in @($parent,$root,(Join-Path $project '.go-build\x-development-entry'),(Join-Path $project '.go-build\mod'))){
 if(!(Test-Path -LiteralPath $path)){New-Item -ItemType Directory -Path $path -Force | Out-Null}
 if((Get-Item -LiteralPath $path).Attributes -band [IO.FileAttributes]::ReparsePoint){throw 'Development test roots must be ordinary directories.'}
}
$names=@('TEMP','TMP','GOTMPDIR','GOCACHE','GOMODCACHE','GOWORK','GOPROXY','GOSUMDB','GOTOOLCHAIN','CGO_ENABLED','GOFLAGS','AKU_CHROME_FIXTURE_NODE','AKU_CHROME_FIXTURE_ROOT','AKU_X_DEV_ENTRY_IMAGES')
$previous=@{};foreach($name in $names){$previous[$name]=[Environment]::GetEnvironmentVariable($name,'Process')}
Push-Location -LiteralPath $project
try{
 $env:TEMP=$root;$env:TMP=$root;$env:GOTMPDIR=$root
 $env:GOCACHE=Join-Path $project '.go-build\x-development-entry';$env:GOMODCACHE=Join-Path $project '.go-build\mod'
 $env:GOWORK='off';$env:GOPROXY='off';$env:GOSUMDB='off';$env:GOTOOLCHAIN='local';$env:CGO_ENABLED='0';$env:GOFLAGS=''
 $env:AKU_CHROME_FIXTURE_NODE=Join-Path $project 'runtime\dev\headless-worker\node.exe';$env:AKU_CHROME_FIXTURE_ROOT=$root
 $env:AKU_X_DEV_ENTRY_IMAGES=$root
 Write-Output ('Offline ordinary-development X entry: '+$id) | Tee-Object -FilePath $log
 if(!$MainOnly){
 & go test ./ui-reader-broker ./internal/config -count=1 -timeout=60s -v 2>&1 | Tee-Object -FilePath $log -Append
 if($LASTEXITCODE -ne 0){throw 'UI assets/config regressions failed.'}
 $production=@(& go list -tags 'auth_session_dev auth_session_fixture' -f '{{range .GoFiles}}{{println .}}{{end}}' ./internal/engine)
 if($LASTEXITCODE -ne 0){throw 'Engine inventory failed.'}
 $sources=@($production | Where-Object {$_.Trim() -ne ''} | ForEach-Object {Join-Path $project ('internal/engine/'+$_)})
 foreach($name in @('development_x_manual_entry_windows_test.go','development_x_manual_application_test.go','development_native_pilot_store_test.go','worker_drain_test.go')){$sources+=Join-Path $project ('internal/engine/'+$name)}
 & go test -tags 'auth_session_dev auth_session_fixture' -run '^(TestXDevelopment|TestXApplicationRoute|TestNativePilotStore|TestEngineIdleIncludesDeferredWorkerCleanup)' -count=1 -timeout=90s -v @sources 2>&1 | Tee-Object -FilePath $log -Append
 if($LASTEXITCODE -ne 0){throw 'Ordinary entry/route/Store regressions failed.'}
 & go test -tags 'auth_session_dev auth_session_fixture' ./internal/httpapi -run '^(TestXDevelopment|TestXManual|TestDirectNativeReader|TestReaderBroker)' -count=1 -timeout=60s -v 2>&1 | Tee-Object -FilePath $log -Append
 if($LASTEXITCODE -ne 0){throw 'HTTP consent, reader and stop regressions failed.'}
 }
 foreach($mode in @('default','auth_session_dev','auth_session_fixture','auth_session_dev auth_session_fixture')){
  $image=Join-Path $root (($mode -replace ' ','-')+'.exe');$args=@('build','-o',$image)
  if($mode -ne 'default'){$args+=@('-tags',$mode)}
  & go @args ./cmd/akusidecar 2>&1 | Tee-Object -FilePath $log -Append
  if($LASTEXITCODE -ne 0){throw ('Actual main compilation failed: '+$mode)}
  Write-Output ('PASS: actual main '+$mode) | Tee-Object -FilePath $log -Append
 }
 & go test -tags 'auth_session_dev auth_session_fixture' ./cmd/akusidecar -run '^TestXDevelopment' -count=1 -timeout=60s -v 2>&1 | Tee-Object -FilePath $log -Append
 if($LASTEXITCODE -ne 0){throw 'Actual main early-selection/profile regression failed.'}
 if($MainOnly){Write-Output 'PASS: focused ordinary-main bootstrap, early rejection, UI profile path and four builds; no native start or transfer.' | Tee-Object -FilePath $log -Append}
 else{Write-Output 'PASS: ordinary-development default-off entry, isolated paths/UI assets, synthetic startup/capture/reader/status/stop and four builds; no browser, native transfer, registry, user runtime or Auto Update change.' | Tee-Object -FilePath $log -Append}
 Write-Output ('Evidence: '+$log)
}finally{
 Pop-Location
 foreach($name in $names){[Environment]::SetEnvironmentVariable($name,$previous[$name],'Process')}
 if(Test-Path -LiteralPath $root){
  $resolved=(Resolve-Path -LiteralPath $root).ProviderPath
  if($resolved -ne $root -or [IO.Path]::GetDirectoryName($resolved) -ne $parent -or ((Get-Item -LiteralPath $resolved).Attributes -band [IO.FileAttributes]::ReparsePoint)){throw 'Refusing cleanup outside owned development test root.'}
  if(Get-ChildItem -LiteralPath $resolved -Recurse -Force | Where-Object {$_.Attributes -band [IO.FileAttributes]::ReparsePoint}){throw 'Refusing cleanup through a reparse point.'}
  Remove-Item -LiteralPath $resolved -Recurse -Force
 }
}

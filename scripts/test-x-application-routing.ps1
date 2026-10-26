param()
$ErrorActionPreference='Stop'
$project=(Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$parent=Join-Path $project 'runtime\qa\auth-session-sidecar'
$id=[guid]::NewGuid().ToString('N')
$root=Join-Path $parent ('run-x-application-routing-'+$id)
$log=Join-Path $parent ('x-application-routing-'+$id+'.log.txt')
foreach($path in @($parent,$root,(Join-Path $project '.go-build\x-session-verifier'),(Join-Path $project '.go-build\mod'))){
 if(!(Test-Path -LiteralPath $path)){New-Item -ItemType Directory -Path $path -Force | Out-Null}
 if((Get-Item -LiteralPath $path).Attributes -band [IO.FileAttributes]::ReparsePoint){throw 'Routing QA paths must be ordinary directories.'}
}
$names=@('TEMP','TMP','GOTMPDIR','GOCACHE','GOMODCACHE','GOWORK','GOPROXY','GOSUMDB','GOTOOLCHAIN','CGO_ENABLED','GOFLAGS')
$previous=@{};foreach($name in $names){$previous[$name]=[Environment]::GetEnvironmentVariable($name,'Process')}
Push-Location -LiteralPath $project
try{
 $env:TEMP=$root;$env:TMP=$root;$env:GOTMPDIR=$root
 $env:GOCACHE=Join-Path $project '.go-build\x-session-verifier';$env:GOMODCACHE=Join-Path $project '.go-build\mod'
 $env:GOWORK='off';$env:GOPROXY='off';$env:GOSUMDB='off';$env:GOTOOLCHAIN='local';$env:CGO_ENABLED='0';$env:GOFLAGS=''
 Write-Output ('Offline X application routing: '+$id) | Tee-Object -FilePath $log
 $production=@(& go list -tags 'auth_session_dev auth_session_fixture' -f '{{range .GoFiles}}{{println .}}{{end}}' ./internal/engine)
 if($LASTEXITCODE -ne 0){throw 'Engine inventory failed.'}
 $sources=@($production | Where-Object {$_.Trim() -ne ''} | ForEach-Object {Join-Path $project ('internal/engine/'+$_)})
 foreach($file in @('development_native_pilot_store_test.go','development_x_manual_application_test.go')){$sources+=Join-Path $project ('internal/engine/'+$file)}
 & go test -tags 'auth_session_dev auth_session_fixture' -run '^(TestXApplicationRoute|TestNativePilotStore)' -count=1 -timeout=60s -v @sources 2>&1 | Tee-Object -FilePath $log -Append
 if($LASTEXITCODE -ne 0){throw 'Application route/Store regression failed.'}
 & go test -tags 'auth_session_dev auth_session_fixture' ./internal/httpapi -run '^(TestXManualStatus|TestDirectNativeReader|TestReaderBroker)' -count=1 -timeout=60s -v 2>&1 | Tee-Object -FilePath $log -Append
 if($LASTEXITCODE -ne 0){throw 'HTTP/reader broker regression failed.'}
 foreach($mode in @('default','auth_session_dev','auth_session_fixture','auth_session_dev auth_session_fixture')){
  $args=@('build','-o',(Join-Path $root (($mode -replace ' ','-')+'.exe')))
  if($mode -ne 'default'){$args+=@('-tags',$mode)}
  & go @args ./cmd/akusidecar 2>&1 | Tee-Object -FilePath $log -Append
  if($LASTEXITCODE -ne 0){throw ('Actual main build failed: '+$mode)}
  Write-Output ('PASS: actual main '+$mode) | Tee-Object -FilePath $log -Append
 }
 Write-Output 'PASS: synthetic X route, reader, claim/result, status and stop; no browser, session transfer, user runtime or Auto Update changes.' | Tee-Object -FilePath $log -Append
 Write-Output ('Evidence: '+$log)
}finally{
 Pop-Location
 foreach($name in $names){[Environment]::SetEnvironmentVariable($name,$previous[$name],'Process')}
 $resolved=(Resolve-Path -LiteralPath $root).ProviderPath
 if($resolved -ne $root -or [IO.Path]::GetDirectoryName($resolved) -ne $parent -or ((Get-Item -LiteralPath $resolved).Attributes -band [IO.FileAttributes]::ReparsePoint)){throw 'Refusing cleanup outside owned routing root.'}
 if(Get-ChildItem -LiteralPath $resolved -Recurse -Force | Where-Object {$_.Attributes -band [IO.FileAttributes]::ReparsePoint}){throw 'Refusing cleanup through reparse point.'}
 Remove-Item -LiteralPath $resolved -Recurse -Force
}

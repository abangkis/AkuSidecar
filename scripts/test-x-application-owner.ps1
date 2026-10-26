param()
$ErrorActionPreference='Stop'
$project=(Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$parent=Join-Path $project 'runtime\qa\auth-session-sidecar'
$id=[guid]::NewGuid().ToString('N')
$root=Join-Path $parent ('run-x-application-owner-'+$id)
foreach($path in @($parent,$root,(Join-Path $project '.go-build\x-session-verifier'),(Join-Path $project '.go-build\mod'))){
 if(!(Test-Path -LiteralPath $path)){New-Item -ItemType Directory -Path $path -Force | Out-Null}
 if((Get-Item -LiteralPath $path).Attributes -band [IO.FileAttributes]::ReparsePoint){throw 'Application owner test paths must be ordinary directories.'}
}
$log=Join-Path $parent ('x-application-owner-'+$id+'.log.txt')
$names=@('TEMP','TMP','GOTMPDIR','GOCACHE','GOMODCACHE','GOWORK','GOPROXY','GOSUMDB','GOTOOLCHAIN','CGO_ENABLED','GOFLAGS','AKU_X_APPLICATION_OWNER_OFFLINE','AKU_CHROME_FIXTURE_NODE','AKU_OWNED_PROFILE_TEST_ROOT')
$previous=@{};foreach($name in $names){$previous[$name]=[Environment]::GetEnvironmentVariable($name,'Process')}
Push-Location -LiteralPath $project
$passed=$false
try{
 $env:TEMP=$root;$env:TMP=$root;$env:GOTMPDIR=$root
 $env:GOCACHE=Join-Path $project '.go-build\x-session-verifier';$env:GOMODCACHE=Join-Path $project '.go-build\mod'
 $env:GOWORK='off';$env:GOPROXY='off';$env:GOSUMDB='off';$env:GOTOOLCHAIN='local';$env:CGO_ENABLED='0';$env:GOFLAGS=''
 $env:AKU_X_APPLICATION_OWNER_OFFLINE='1';$env:AKU_CHROME_FIXTURE_NODE=Join-Path $project 'runtime\dev\headless-worker\node.exe'
 $env:AKU_OWNED_PROFILE_TEST_ROOT=$root
 Write-Output ('Offline application owner: '+$id) | Tee-Object -FilePath $log
 & go test ./internal/ownedprofile -count=1 -timeout=30s -v 2>&1 | Tee-Object -FilePath $log -Append
 if($LASTEXITCODE -ne 0){throw 'Application profile lease regression failed.'}
 $production=@(& go list -tags auth_session_dev -f '{{range .GoFiles}}{{println .}}{{end}}' ./internal/engine)
 if($LASTEXITCODE -ne 0){throw 'Application owner source inventory unavailable.'}
 $sources=@($production | Where-Object {$_.Trim() -ne ''} | ForEach-Object {Join-Path $project ('internal/engine/'+$_)})
 $sources+=Join-Path $project 'internal/engine/development_x_application_owner_windows_test.go'
 $sources+=Join-Path $project 'internal/engine/development_x_application_seed_windows_test.go'
 & go test -tags auth_session_dev -run '^TestXApplication' -count=1 -timeout=60s -v @sources 2>&1 | Tee-Object -FilePath $log -Append
 if($LASTEXITCODE -ne 0){throw 'Application owner regression failed.'}
 & go test ./internal/appshell -run '^(TestSession|TestPrepareWorkerCommand)' -count=1 -timeout=30s -v 2>&1 | Tee-Object -FilePath $log -Append
 if($LASTEXITCODE -ne 0){throw 'Existing application owner/session regression failed.'}
 $passed=$true
 Write-Output 'PASS: pinned application leases and suspended Job-owned worker with synthetic processes only. No Chrome, retained profile, X request, account evidence, session transfer, Store or user runtime.' | Tee-Object -FilePath $log -Append
 Write-Output ('Evidence: '+$log)
}finally{
 Pop-Location
 foreach($name in $names){[Environment]::SetEnvironmentVariable($name,$previous[$name],'Process')}
 if($passed){
  $resolved=(Resolve-Path -LiteralPath $root).ProviderPath
  if($resolved -ne $root -or [IO.Path]::GetDirectoryName($resolved) -ne $parent -or ((Get-Item -LiteralPath $resolved).Attributes -band [IO.FileAttributes]::ReparsePoint)){throw 'Refusing cleanup outside application owner root.'}
  if(Get-ChildItem -LiteralPath $resolved -Recurse -Force | Where-Object {$_.Attributes -band [IO.FileAttributes]::ReparsePoint}){throw 'Refusing cleanup through reparse point.'}
  Remove-Item -LiteralPath $resolved -Recurse -Force
 }else{Write-Warning ('Failed synthetic owner root retained: '+$root)}
}

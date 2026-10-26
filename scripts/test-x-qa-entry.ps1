param([switch]$StoreOnly,[switch]$StagingOnly)
$ErrorActionPreference='Stop'
if($StoreOnly -and $StagingOnly){throw 'Select at most one focused validation stage.'}
$project=(Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$parent=Join-Path $project 'runtime\qa\auth-session-sidecar'
$id=[guid]::NewGuid().ToString('N')
$root=Join-Path $parent ('run-x-qa-entry-'+$id)
$log=Join-Path $parent ('x-qa-entry-'+$id+'.log.txt')
foreach($path in @($parent,$root,(Join-Path $project '.go-build\x-qa-entry'),(Join-Path $project '.go-build\mod'))){
 if(!(Test-Path -LiteralPath $path)){New-Item -ItemType Directory -Path $path -Force | Out-Null}
 if((Get-Item -LiteralPath $path).Attributes -band [IO.FileAttributes]::ReparsePoint){throw 'QA entry test paths must be ordinary directories.'}
}
$names=@('TEMP','TMP','GOTMPDIR','GOCACHE','GOMODCACHE','GOWORK','GOPROXY','GOSUMDB','GOTOOLCHAIN','CGO_ENABLED','GOFLAGS','AKU_CHROME_FIXTURE_NODE','AKU_CHROME_FIXTURE_ROOT','AKU_X_QA_ENTRY_IMAGE','AKU_X_QA_ENTRY_DEFAULT','AKU_X_QA_ENTRY_DEV','AKU_X_QA_ENTRY_FIXTURE')
$previous=@{};foreach($name in $names){$previous[$name]=[Environment]::GetEnvironmentVariable($name,'Process')}
function Remove-OwnedQAStage([string]$stageRoot){
 if(!(Test-Path -LiteralPath $stageRoot)){return}
 $resolved=(Resolve-Path -LiteralPath $stageRoot).ProviderPath
 if($resolved -ne $stageRoot -or [IO.Path]::GetDirectoryName($resolved) -ne $parent -or [IO.Path]::GetFileName($resolved) -notmatch '^run-x-qa-entry-stage-[a-z-]+-[a-f0-9]{32}$'){throw 'Refusing staging cleanup outside owned QA namespace.'}
 if(((Get-Item -LiteralPath $resolved).Attributes -band [IO.FileAttributes]::ReparsePoint) -or (Get-ChildItem -LiteralPath $resolved -Recurse -Force | Where-Object {$_.Attributes -band [IO.FileAttributes]::ReparsePoint})){throw 'Refusing staging cleanup through reparse points.'}
 Remove-Item -LiteralPath $resolved -Recurse -Force
}
function Invoke-QAStagingCompile {
 foreach($mode in @('x-retained-manual','x-retained-capture')){
  $stageRoot=Join-Path $parent ('run-x-qa-entry-stage-'+$mode+'-'+$id)
  New-Item -ItemType Directory -Path $stageRoot | Out-Null
  try{
   $raw=& $env:AKU_CHROME_FIXTURE_NODE scripts/auth-session-fixture/stage-native-manual-chrome.mjs $stageRoot $mode
   if($LASTEXITCODE -ne 0){throw ('Source staging failed: '+$mode)}
   $stage=$raw | ConvertFrom-Json
   $files=@($stage.files | ForEach-Object {Join-Path $stage.stage $_})
   & go test -c -tags 'auth_session_dev auth_session_fixture' -o (Join-Path $root ($mode+'.test.exe')) @files 2>&1 | Tee-Object -FilePath $log -Append
   if($LASTEXITCODE -ne 0){throw ('Legacy staging compilation failed: '+$mode)}
   Write-Output ('PASS: compile only '+$mode+'; staged native tests not executed.') | Tee-Object -FilePath $log -Append
  }finally{Remove-OwnedQAStage $stageRoot}
 }
}
Push-Location -LiteralPath $project
try{
 $env:TEMP=$root;$env:TMP=$root;$env:GOTMPDIR=$root
 $env:GOCACHE=Join-Path $project '.go-build\x-qa-entry';$env:GOMODCACHE=Join-Path $project '.go-build\mod'
 $env:GOWORK='off';$env:GOPROXY='off';$env:GOSUMDB='off';$env:GOTOOLCHAIN='local';$env:CGO_ENABLED='0';$env:GOFLAGS=''
 $env:AKU_CHROME_FIXTURE_NODE=Join-Path $project 'runtime\dev\headless-worker\node.exe'
 $env:AKU_CHROME_FIXTURE_ROOT=$root
 Write-Output ('Offline isolated QA entry: '+$id) | Tee-Object -FilePath $log
 if($StagingOnly){Invoke-QAStagingCompile;Write-Output ('Evidence: '+$log);return}
 & go test ./internal/store -run '^(TestStoreRecordsTheRuntimeVersionThatLastOpenedData|TestStoreRejectsDataWrittenByANewerRuntime|TestRuntimeVersionComparisonRecognizesDowngradesAndPrereleases|TestDatabaseCompatibilityClassification)$' -count=1 -timeout=60s -v 2>&1 | Tee-Object -FilePath $log -Append
 if($LASTEXITCODE -ne 0){throw 'Ordinary Store runtime-marker/compatibility regression failed.'}
 if($StoreOnly){Write-Output ('Evidence: '+$log);return}
 $production=@(& go list -tags 'auth_session_dev auth_session_fixture' -f '{{range .GoFiles}}{{println .}}{{end}}' ./internal/engine)
 if($LASTEXITCODE -ne 0){throw 'Engine source inventory failed.'}
 $sources=@($production | Where-Object {$_.Trim() -ne ''} | ForEach-Object {Join-Path $project ('internal/engine/'+$_)})
 foreach($file in @('development_x_qa_plan_windows_test.go','development_x_application_post_windows_test.go','development_x_qa_evidence_test.go','development_x_retained_profile_windows_test.go','development_x_qa_materialize_windows_test.go','x_retained_qa_profile_windows_test.go')){$sources+=Join-Path $project ('internal/engine/'+$file)}
 & go test -tags 'auth_session_dev auth_session_fixture' -run '^(TestXQA|TestXApplicationPost|TestXRetained)' -count=1 -timeout=60s -v @sources 2>&1 | Tee-Object -FilePath $log -Append
 if($LASTEXITCODE -ne 0){throw 'Isolated QA plan/controller or post regression failed.'}
 foreach($mode in @('default','auth_session_dev','auth_session_fixture','auth_session_dev auth_session_fixture')){
  $image=Join-Path $root (($mode -replace ' ','-')+'.exe')
  $args=@('build','-o',$image);if($mode -ne 'default'){$args+=@('-tags',$mode)}
  & go @args ./cmd/akusidecar 2>&1 | Tee-Object -FilePath $log -Append
  if($LASTEXITCODE -ne 0){throw ('Main compilation failed: '+$mode)}
  if($mode -eq 'default'){$env:AKU_X_QA_ENTRY_DEFAULT=$image}elseif($mode -eq 'auth_session_dev'){$env:AKU_X_QA_ENTRY_DEV=$image}elseif($mode -eq 'auth_session_fixture'){$env:AKU_X_QA_ENTRY_FIXTURE=$image}else{$env:AKU_X_QA_ENTRY_IMAGE=$image}
  Write-Output ('PASS: actual main '+$mode) | Tee-Object -FilePath $log -Append
 }
 & go test -tags 'auth_session_dev auth_session_fixture' ./cmd/akusidecar -run '^TestXQA' -count=1 -timeout=60s -v 2>&1 | Tee-Object -FilePath $log -Append
 if($LASTEXITCODE -ne 0){throw 'Actual synthetic main QA entry failed.'}
 Invoke-QAStagingCompile
 Write-Output 'PASS: default-off isolated QA plan, owned synthetic database, start/post/capture/status/stop and four builds. Native selector not executed; no Chrome, real session transfer, main login/runtime or Auto Update change.' | Tee-Object -FilePath $log -Append
 Write-Output ('Evidence: '+$log)
}finally{
 Pop-Location
 foreach($name in $names){[Environment]::SetEnvironmentVariable($name,$previous[$name],'Process')}
 if(Test-Path -LiteralPath $root){
  $resolved=(Resolve-Path -LiteralPath $root).ProviderPath
  if($resolved -ne $root -or [IO.Path]::GetDirectoryName($resolved) -ne $parent -or ((Get-Item -LiteralPath $resolved).Attributes -band [IO.FileAttributes]::ReparsePoint)){throw 'Refusing cleanup outside owned QA entry root.'}
  if(Get-ChildItem -LiteralPath $resolved -Recurse -Force | Where-Object {$_.Attributes -band [IO.FileAttributes]::ReparsePoint}){throw 'Refusing cleanup through reparse point.'}
  Remove-Item -LiteralPath $resolved -Recurse -Force
 }
}

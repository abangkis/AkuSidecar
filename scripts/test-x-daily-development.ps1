param()
$ErrorActionPreference='Stop'
$project=(Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$root=Join-Path $project ('build\x-daily-offline-'+[guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $root | Out-Null
$names=@('TEMP','TMP','GOTMPDIR','GOCACHE','GOMODCACHE','GOWORK','GOPROXY','GOSUMDB','GOTOOLCHAIN','CGO_ENABLED','GOFLAGS')
$previous=@{};foreach($name in $names){$previous[$name]=[Environment]::GetEnvironmentVariable($name,'Process')}
Push-Location -LiteralPath $project
try {
 $env:TEMP=$root;$env:TMP=$root;$env:GOTMPDIR=$root
 $env:GOCACHE=Join-Path $project '.go-build\x-development-entry';$env:GOMODCACHE=Join-Path $project '.go-build\mod'
 $env:GOWORK='off';$env:GOPROXY='off';$env:GOSUMDB='off';$env:GOTOOLCHAIN='local';$env:CGO_ENABLED='0';$env:GOFLAGS=''
 $log=Join-Path $root 'offline.log.txt'
 & go test ./internal/config -count=1 -timeout=60s -v 2>&1 | Tee-Object -FilePath $log
 if($LASTEXITCODE -ne 0){throw 'Configuration regression failed'}
 foreach($mode in @('default','auth_session_dev')) {
  $args=@('test');if($mode -ne 'default'){$args+=@('-tags',$mode)}
  $args+=@('./internal/engine','./internal/httpapi','-run','^(TestXDaily|TestXManualStatusAndStopDefaultOff)','-count=1','-timeout=90s','-v')
  & go @args 2>&1 | Tee-Object -FilePath $log -Append
  if($LASTEXITCODE -ne 0){throw ('Daily composition regression failed: '+$mode)}
 }
 foreach($mode in @('default','auth_session_dev','auth_session_fixture','auth_session_dev auth_session_fixture')) {
  $args=@('build','-o',(Join-Path $root (($mode -replace ' ','-')+'.exe')))
  if($mode -ne 'default'){$args+=@('-tags',$mode)}
  & go @args ./cmd/akusidecar 2>&1 | Tee-Object -FilePath $log -Append
  if($LASTEXITCODE -ne 0){throw ('Main compilation failed: '+$mode)}
  Write-Output ('PASS: main build '+$mode) | Tee-Object -FilePath $log -Append
 }
 Write-Output ('PASS: offline daily config/composition only; no native owner, profile, runtime or transfer. Evidence: '+$log)
} finally {
 Pop-Location
 foreach($name in $names){[Environment]::SetEnvironmentVariable($name,$previous[$name],'Process')}
}

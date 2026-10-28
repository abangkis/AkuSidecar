param()
$ErrorActionPreference='Stop'
$project=(Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$root=Join-Path $project ('build\x-main-offline-'+[guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $root | Out-Null
$names=@('TEMP','TMP','GOTMPDIR','GOCACHE','GOMODCACHE','GOWORK','GOPROXY','GOSUMDB','GOTOOLCHAIN','CGO_ENABLED','GOFLAGS','AKU_CHROME_FIXTURE_NODE','AKU_MAIN_TRANSPORT_DRIVER','AKU_X_APPLICATION_OWNER_OFFLINE')
$previous=@{};foreach($name in $names){$previous[$name]=[Environment]::GetEnvironmentVariable($name,'Process')}
Push-Location -LiteralPath $project
try {
 $env:TEMP=$root;$env:TMP=$root;$env:GOTMPDIR=$root
 $env:GOCACHE=Join-Path $project '.go-build\x-development-entry';$env:GOMODCACHE=Join-Path $project '.go-build\mod'
 $env:GOWORK='off';$env:GOPROXY='off';$env:GOSUMDB='off';$env:GOTOOLCHAIN='local';$env:CGO_ENABLED='0';$env:GOFLAGS=''
 $env:AKU_CHROME_FIXTURE_NODE=Join-Path $project 'runtime\dev\headless-worker\node.exe';$env:AKU_X_APPLICATION_OWNER_OFFLINE='1'
 $bundleText=& $env:AKU_CHROME_FIXTURE_NODE -e "import('./scripts/build-x-native-owner.mjs').then(m=>m.buildMainBrowserTransportPackage(process.argv[1])).then(x=>process.stdout.write(JSON.stringify(x)))" (Join-Path $root 'transport')
 if($LASTEXITCODE -ne 0){throw 'Main transport packaging failed'}
 $bundle=$bundleText | ConvertFrom-Json;$env:AKU_MAIN_TRANSPORT_DRIVER=$bundle.driver
 $log=Join-Path $root 'offline.log.txt'
 & $env:AKU_CHROME_FIXTURE_NODE --test internal/collection/headless/worker/native-pilot/main-browser-transport.test.mjs internal/collection/headless/worker/native-pilot/daily-owner-protocol.test.mjs scripts/auth-session-fixture/x-application-reader-post.test.mjs scripts/auth-session-fixture/x-application-seed-grant.test.mjs scripts/build-x-daily-owner.test.mjs 2>&1 | Tee-Object -FilePath $log
 if($LASTEXITCODE -ne 0){throw 'Main/dedicated Node regressions failed'}
 foreach($mode in @('default','auth_session_dev')) {
  $args=@('test');if($mode -ne 'default'){$args+=@('-tags',$mode)}
  $args+=@('./internal/config','./internal/appshell','./internal/captureruntime','./internal/collection','./internal/engine','./internal/httpapi','./internal/store','-run','^(TestMain|TestPersistent|TestXDaily|TestXMainBrowser|TestHybrid|TestXManualStatusAndStopDefaultOff|TestDailyPilot|TestManualPilot)','-count=1','-timeout=120s','-v')
  & go @args 2>&1 | Tee-Object -FilePath $log -Append
  if($LASTEXITCODE -ne 0){throw ('Main-profile regression failed: '+$mode)}
 }
 foreach($mode in @('default','auth_session_dev','auth_session_fixture','auth_session_dev auth_session_fixture')) {
  $args=@('build','-o',(Join-Path $root (($mode -replace ' ','-')+'.exe')))
  if($mode -ne 'default'){$args+=@('-tags',$mode)}
  & go @args ./cmd/akusidecar 2>&1 | Tee-Object -FilePath $log -Append
  if($LASTEXITCODE -ne 0){throw ('Main compilation failed: '+$mode)}
  Write-Output ('PASS: main build '+$mode) | Tee-Object -FilePath $log -Append
 }
 Write-Output ('PASS: synthetic main transport/lifecycle and dedicated regressions. Evidence: '+$log)
} finally {Pop-Location;foreach($name in $names){[Environment]::SetEnvironmentVariable($name,$previous[$name],'Process')}}

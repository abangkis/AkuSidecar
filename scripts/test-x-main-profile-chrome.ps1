param(
 [switch]$RunChrome,
 [string]$ChromePath='C:\Program Files\Google\Chrome\Application\chrome.exe'
)
$ErrorActionPreference='Stop'
if(-not $RunChrome){throw 'Explicit -RunChrome and operational approval for one empty-profile local fixture are required.'}
$project=(Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
if(-not [IO.Path]::IsPathFullyQualified($ChromePath) -or -not (Test-Path -LiteralPath $ChromePath -PathType Leaf)){throw 'An existing absolute Chrome executable is required.'}
$root=Join-Path $project ('build\x-main-chrome-local-'+[guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $root | Out-Null
$names=@('TEMP','TMP','GOTMPDIR','GOCACHE','GOMODCACHE','GOWORK','GOPROXY','GOSUMDB','GOTOOLCHAIN','CGO_ENABLED','GOFLAGS','AKU_CHROME_FIXTURE_NODE','AKU_MAIN_TRANSPORT_DRIVER','AKU_MAIN_TRANSPORT_CHROME','AKU_MAIN_CHROME_FIXTURE_ROOT')
$previous=@{};foreach($name in $names){$previous[$name]=[Environment]::GetEnvironmentVariable($name,'Process')}
Push-Location -LiteralPath $project
try {
 $env:TEMP=$root;$env:TMP=$root;$env:GOTMPDIR=$root
 $env:GOCACHE=Join-Path $project '.go-build\x-development-entry';$env:GOMODCACHE=Join-Path $project '.go-build\mod'
 $env:GOWORK='off';$env:GOPROXY='off';$env:GOSUMDB='off';$env:GOTOOLCHAIN='local';$env:CGO_ENABLED='0';$env:GOFLAGS=''
 $env:AKU_CHROME_FIXTURE_NODE=Join-Path $project 'runtime\dev\headless-worker\node.exe'
 $bundleText=& $env:AKU_CHROME_FIXTURE_NODE -e "import('./scripts/build-x-native-owner.mjs').then(m=>m.buildMainBrowserTransportPackage(process.argv[1])).then(x=>process.stdout.write(JSON.stringify(x)))" (Join-Path $root 'transport')
 if($LASTEXITCODE -ne 0){throw 'Main transport packaging failed'}
 $bundle=$bundleText | ConvertFrom-Json;$env:AKU_MAIN_TRANSPORT_DRIVER=$bundle.driver
 $env:AKU_MAIN_TRANSPORT_CHROME=[IO.Path]::GetFullPath($ChromePath);$env:AKU_MAIN_CHROME_FIXTURE_ROOT=$root
 $attempt=@{schema='aku.x.main-chrome-local-attempt.v1';startedAtUtc=[DateTime]::UtcNow.ToString('o');maxExecutions=1;retry=$false;url='about:blank';loginAllowed=$false;sessionTransferAllowed=$false;chromeSHA256=(Get-FileHash -LiteralPath $ChromePath -Algorithm SHA256).Hash.ToLowerInvariant();driverSHA256=$bundle.sha256}
 [IO.File]::WriteAllText((Join-Path $root 'attempt.json'),($attempt | ConvertTo-Json -Depth 4))
 $log=Join-Path $root 'chrome-local.log.txt'
 & go test -tags auth_session_dev ./internal/appshell -run '^TestMainBrowserTransportChromeLocal$' -count=1 -timeout=90s -v 2>&1 | Tee-Object -FilePath $log
 if($LASTEXITCODE -ne 0){throw ('Local Chrome fixture failed; no retry. Evidence: '+$log)}
 Write-Output ('PASS: one local empty-profile Chrome fixture. Evidence: '+$log)
} finally {Pop-Location;foreach($name in $names){[Environment]::SetEnvironmentVariable($name,$previous[$name],'Process')}}

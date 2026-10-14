param([Parameter(Mandatory=$true)][string]$TaskRoot,[switch]$Verify)
$ErrorActionPreference = 'Stop'
$project = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$parent = Join-Path $project 'runtime\qa\auth-session-sidecar'
$resolved = (Resolve-Path -LiteralPath $TaskRoot).ProviderPath
if ([IO.Path]::GetDirectoryName($resolved) -ne $parent -or ![IO.Path]::GetFileName($resolved).StartsWith('run-')) { throw 'Frontend QA root escaped project scope.' }
$node = 'C:\nvm4w\nodejs\node.exe'
if (!(Test-Path -LiteralPath $node -PathType Leaf)) { throw 'Configured Node executable unavailable.' }
$payload = Join-Path $project 'runtime\chromium'
$pin = Get-Content -LiteralPath (Join-Path $payload 'pin.json') -Raw | ConvertFrom-Json
$chrome = Join-Path $payload 'bin\chrome.exe'
$expectedHash = 'b0123437c55a3893e8988328f576ffcbe68cee7743d3653ffe865c73633b1ef4'
if (!(Test-Path -LiteralPath $chrome -PathType Leaf) -or (Get-FileHash -LiteralPath $chrome -Algorithm SHA256).Hash.ToLowerInvariant() -ne $expectedHash) { throw 'Pinned CfT source hash mismatch.' }
New-Item -ItemType Directory -Path (Join-Path $resolved 'chromium') | Out-Null
Copy-Item -LiteralPath (Join-Path $payload 'bin') -Destination (Join-Path $resolved 'chromium\bin') -Recurse
Copy-Item -LiteralPath (Join-Path $payload 'pin.json') -Destination (Join-Path $resolved 'chromium\pin.json')
$env:AKU_CHROME_FIXTURE_ROOT = $resolved
$env:AKU_CHROME_FIXTURE_CLEANUP_MARKER = Join-Path $resolved 'cleanup-verified.marker'
$env:AKU_TEST_NODE_PATH = $node
$env:AKU_TEST_CHROME_PATH = Join-Path $resolved 'chromium\bin\chrome.exe'
$env:AKU_DEV_FRONTEND_PROJECT = $project
$env:AKU_DEV_FRONTEND_ENABLED = '1'
$env:AKU_DEV_FRONTEND_HEADLESS = '1'
$env:AKU_DEV_BROKER_ENABLED = $null
$env:AKU_DEV_FRONTEND_NATIVE_ENABLED = $null
$env:AKU_DEV_SESSION_TEST_ROOT = $resolved
Write-Output ('Frontend QA payload SHA256=' + $expectedHash + ' fresh_task_root=' + $resolved + ' native_host_registration=false')
& $node --test test/development-reader-priority.test.mjs test/native-post-availability.test.mjs test/native-post-routing.test.mjs test/split-ui-bridge.test.mjs
if ($LASTEXITCODE -ne 0) { throw 'Frontend source guards or real relay contract regressions failed.' }
& go build -o (Join-Path $resolved 'aku-sidecar-default-check.exe') ./cmd/akusidecar
if ($LASTEXITCODE -ne 0) { throw 'Default main build failed.' }
& go build -tags auth_session_dev -o (Join-Path $resolved 'aku-sidecar-dev-unavailable.exe') ./cmd/akusidecar
if ($LASTEXITCODE -ne 0) { throw 'Development without adapter build failed.' }
& go build -tags 'auth_session_dev auth_session_fixture' -o (Join-Path $resolved 'akusidecar.exe') ./cmd/akusidecar
if ($LASTEXITCODE -ne 0) { throw 'Actual frontend development main build failed.' }
& go test -tags 'auth_session_dev auth_session_fixture' ./cmd/akusidecar ./internal/httpapi ./internal/engine -run '^TestDevelopmentFrontend(Staging|CLI|Closed|RealRelay|Seed)' -count=1 -timeout=60s -v
if ($LASTEXITCODE -ne 0) { throw 'Non-foreground staging, typed admission or real server/relay preparation failed.' }
& go test ./internal/httpapi -run '^(TestSplitReader|TestReaderBroker)' -count=1 -timeout=60s
if ($LASTEXITCODE -ne 0) { throw 'Ordinary native-reader HTTP admission regressions failed.' }
if (Test-Path -LiteralPath $env:AKU_CHROME_FIXTURE_CLEANUP_MARKER -PathType Leaf) { Remove-Item -LiteralPath $env:AKU_CHROME_FIXTURE_CLEANUP_MARKER }
if (!$Verify) {
    Write-Output 'Starting owned headless frontend preparation: no visible UI, no input, no native activation.'
    & go test -tags 'auth_session_dev auth_session_fixture' ./internal/engine -run '^TestDevelopmentFrontendActualMainHeadlessPreparation$' -count=1 -timeout=180s -v
    if ($LASTEXITCODE -ne 0) { throw 'Actual headless frontend DOM/bootstrap/Timeline/readiness preparation failed.' }
    $images = @(Get-ChildItem -LiteralPath $resolved -Filter 'frontend-viewport.png' -Recurse -File)
    if ($images.Count -ne 1) { throw 'App-only frontend screenshot evidence missing.' }
    Copy-Item -LiteralPath $images[0].FullName -Destination (Join-Path $parent 'frontend-preparation-viewport.png')
    Write-Output ('App-only headless screenshot=' + (Join-Path $parent 'frontend-preparation-viewport.png'))
} else {
    $env:AKU_DEV_FRONTEND_HEADLESS = '0'
    $env:AKU_DEV_FRONTEND_CUE = Join-Path $resolved 'frontend-operator-cue.json'
    Write-Output ('FRONTEND task_root=' + $resolved + ' cue=' + $env:AKU_DEV_FRONTEND_CUE + ' ready=' + $env:AKU_DEV_FRONTEND_CUE + '.ready.json')
    & go test -tags 'auth_session_dev auth_session_fixture' ./internal/engine -run '^TestDevelopmentFrontendActualMainPhysicalDenial$' -count=1 -timeout=12m -v
    if ($LASTEXITCODE -ne 0) { throw 'Actual frontend physical denial validation failed; no automatic retry.' }
}
if (!(Test-Path -LiteralPath $env:AKU_CHROME_FIXTURE_CLEANUP_MARKER -PathType Leaf)) { throw 'Frontend QA did not prove all owned trees empty.' }
Write-Output 'Frontend QA completed exit=0; native activation remains CLOSED; full native messaging parity is untested.'

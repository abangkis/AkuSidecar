param([Parameter(Mandatory=$true)][string]$TaskRoot)
$ErrorActionPreference = 'Stop'
$project = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$parent = Join-Path $project 'runtime\qa\auth-session-sidecar'
$root = (Resolve-Path -LiteralPath $TaskRoot).ProviderPath
if ([IO.Path]::GetDirectoryName($root) -ne $parent -or ![IO.Path]::GetFileName($root).StartsWith('run-')) { throw 'Automatic lifetime root escaped project QA scope.' }
$payload = Join-Path $project 'runtime\chromium'
$chrome = Join-Path $payload 'bin\chrome.exe'
$expectedHash = 'b0123437c55a3893e8988328f576ffcbe68cee7743d3653ffe865c73633b1ef4'
if (!(Test-Path -LiteralPath $chrome -PathType Leaf) -or (Get-FileHash -LiteralPath $chrome -Algorithm SHA256).Hash.ToLowerInvariant() -ne $expectedHash) { throw 'Pinned CfT source hash mismatch.' }
$node = 'C:\nvm4w\nodejs\node.exe'
if (!(Test-Path -LiteralPath $node -PathType Leaf)) { throw 'Configured Node unavailable.' }
New-Item -ItemType Directory -Path (Join-Path $root 'chromium') | Out-Null
Copy-Item -LiteralPath (Join-Path $payload 'bin') -Destination (Join-Path $root 'chromium\bin') -Recurse
Copy-Item -LiteralPath (Join-Path $payload 'pin.json') -Destination (Join-Path $root 'chromium\pin.json')
$env:AKU_CHROME_FIXTURE_ROOT = $root
$env:AKU_CHROME_FIXTURE_CLEANUP_MARKER = Join-Path $root 'cleanup-verified.marker'
$env:AKU_TEST_NODE_PATH = $node
$env:AKU_TEST_CHROME_PATH = Join-Path $root 'chromium\bin\chrome.exe'
$env:AKU_DEV_SESSION_TEST_ROOT = $root
$env:AKU_DEV_FRONTEND_PROJECT = $project
$env:AKU_DEV_FRONTEND_ENABLED = '1'
$env:AKU_DEV_FRONTEND_HEADLESS = '0'
$env:AKU_DEV_FRONTEND_LIFETIME_ENABLED = '1'
$env:AKU_DEV_FRONTEND_NATIVE_ENABLED = $null
$env:AKU_DEV_BROKER_ENABLED = $null
Write-Output ('AUTOMATIC_READER_LIFETIME task_root=' + $root + ' native_activation=CLOSED helpers=0 manual_clicks=0 presentation_focus=false chrome_sha256=' + $expectedHash)
& go test -tags 'auth_session_dev auth_session_fixture' ./cmd/akusidecar ./internal/engine ./internal/store -run '^TestDevelopmentFrontend(Lifetime|LiveReader|NativeScenario|NativeStart)|^TestDevelopmentLiveReader|^TestFixtureLiveReaderCommandEvidence' -count=1 -timeout=60s -v
if ($LASTEXITCODE -ne 0) { throw 'Automatic lifetime startup/queue contracts failed.' }
& go build -tags 'auth_session_dev auth_session_fixture' -o (Join-Path $root 'akusidecar.exe') ./cmd/akusidecar
if ($LASTEXITCODE -ne 0) { throw 'Automatic actual main build failed.' }
& go test -c -tags 'auth_session_dev auth_session_fixture' -o (Join-Path $root 'engine.test.exe') ./internal/engine
if ($LASTEXITCODE -ne 0) { throw 'Automatic lifetime controller build failed.' }
Push-Location -LiteralPath (Join-Path $project 'internal\engine')
try {
    & (Join-Path $root 'engine.test.exe') '-test.run=^TestDevelopmentFrontendActualMainAutomaticReaderLifetime$' '-test.count=1' '-test.timeout=180s' '-test.v'
} finally { Pop-Location }
if ($LASTEXITCODE -ne 0) { throw 'Actual main automatic lifetime failed; retain evidence, no implicit retry.' }
if (!(Test-Path -LiteralPath $env:AKU_CHROME_FIXTURE_CLEANUP_MARKER -PathType Leaf)) { throw 'Automatic lifetime did not prove owned jobs empty.' }
Write-Output 'AUTOMATIC_READER_LIFETIME passed exit=0 cleanup_marker=true physical_foreground_claim=false'

param(
    [switch]$ChromeOnly,
    [ValidateSet('all','happy','missing_identity','logout','account_switch','malformed','oversized')]
    [string]$Case = 'all',
    [ValidateRange(1,5)][int]$Count = 1
)
$ErrorActionPreference = 'Stop'
$projectRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$qaParent = Join-Path $projectRoot 'runtime\qa\auth-session-sidecar'
New-Item -ItemType Directory -Path $qaParent -Force | Out-Null
if ((Get-Item -LiteralPath $qaParent).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'QA parent must not be a reparse point.' }
$taskRoot = Join-Path $qaParent ('run-x-evidence-chrome-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $taskRoot | Out-Null
$cacheRoot = Join-Path $projectRoot '.go-build\x-session-verifier'
$moduleCache = Join-Path $projectRoot '.go-build\mod'
foreach ($cachePath in @($cacheRoot, $moduleCache)) {
    New-Item -ItemType Directory -Path $cachePath -Force | Out-Null
    if ((Get-Item -LiteralPath $cachePath).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Go cache must not be a reparse point.' }
}
$nodePath = 'C:\nvm4w\nodejs\node.exe'
$chromePath = 'C:\Program Files\Google\Chrome\Application\chrome.exe'
if (!(Test-Path -LiteralPath $nodePath -PathType Leaf) -or !(Test-Path -LiteralPath $chromePath -PathType Leaf)) { throw 'Fixture Node or Chrome executable is unavailable.' }
$variables = @('TEMP','TMP','GOTMPDIR','GOCACHE','GOMODCACHE','GOPROXY','GOSUMDB','GOTOOLCHAIN','CGO_ENABLED','GOFLAGS',
    'AKU_CHROME_FIXTURE_ROOT','AKU_CHROME_FIXTURE_CLEANUP_MARKER','AKU_TEST_NODE_PATH','AKU_TEST_CHROME_PATH','AKU_X_EVIDENCE_CHROME_ENABLED')
$previous = @{}
foreach ($variable in $variables) { $previous[$variable] = [Environment]::GetEnvironmentVariable($variable, 'Process') }
$passed = $false
Push-Location -LiteralPath $projectRoot
try {
    $env:TEMP = $taskRoot
    $env:TMP = $taskRoot
    $env:GOTMPDIR = $taskRoot
    $env:GOCACHE = $cacheRoot
    $env:GOMODCACHE = $moduleCache
    $env:GOPROXY = 'off'
    $env:GOSUMDB = 'off'
    $env:GOTOOLCHAIN = 'local'
    $env:CGO_ENABLED = '0'
    $env:GOFLAGS = ''
    if (!$ChromeOnly) {
        & $nodePath --test --test-timeout=10000 --test-reporter=tap scripts/auth-session-fixture/x-evidence-ingress.test.mjs scripts/auth-session-fixture/x-evidence-body.test.mjs scripts/auth-session-fixture/x-evidence-request-clock.test.mjs
        if ($LASTEXITCODE -ne 0) { throw 'Bounded ingress/body unit regressions failed.' }
        & (Join-Path $PSScriptRoot 'test-x-session-verifier.ps1')
        if ($LASTEXITCODE -ne 0) { throw 'Fixture verifier regression or isolation check failed.' }
    } else { Write-Output 'ChromeOnly: prerequisite evidence must be reused only while its relevant inputs remain unchanged.' }
    $env:AKU_CHROME_FIXTURE_ROOT = $taskRoot
    $env:AKU_CHROME_FIXTURE_CLEANUP_MARKER = Join-Path $taskRoot 'cleanup-verified.marker'
    $env:AKU_TEST_NODE_PATH = $nodePath
    $env:AKU_TEST_CHROME_PATH = $chromePath
    $env:AKU_X_EVIDENCE_CHROME_ENABLED = '1'
    $testPattern = '^TestXEvidenceChromeFixture(ToVerifier|OversizedResponseIsCappedJSON)$'
    if ($Case -ne 'all') { $testPattern += '/^' + $Case + '$' }
    & go test -tags 'auth_session_dev auth_session_fixture' ./internal/engine -run $testPattern "-count=$Count" -failfast -timeout=180s -v
    if ($LASTEXITCODE -ne 0) { throw 'Actual fixture Chrome to verifier acceptance failed.' }
    if (!(Test-Path -LiteralPath $env:AKU_CHROME_FIXTURE_CLEANUP_MARKER -PathType Leaf)) { throw 'Owned fixture Jobs did not prove empty.' }
    if ((Get-Content -LiteralPath $env:AKU_CHROME_FIXTURE_CLEANUP_MARKER -Raw).Trim() -ne 'verified') { throw 'Fixture cleanup marker is invalid.' }
    $passed = $true
    Write-Output 'Fixture Chrome to verifier passed; new profiles, synthetic login, private pipe, bounded body, non-authorizing results and owned Jobs empty.'
} finally {
    Pop-Location
    foreach ($variable in $variables) { [Environment]::SetEnvironmentVariable($variable, $previous[$variable], 'Process') }
    if ($passed) {
        $resolved = (Resolve-Path -LiteralPath $taskRoot).ProviderPath
        if ($resolved -ne $taskRoot -or [IO.Path]::GetDirectoryName($resolved) -ne $qaParent -or ((Get-Item -LiteralPath $resolved).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Refusing cleanup outside owned QA root.' }
        if (Get-ChildItem -LiteralPath $resolved -Recurse -Force | Where-Object { $_.Attributes -band [IO.FileAttributes]::ReparsePoint }) { throw 'Refusing cleanup through reparse point.' }
        Remove-Item -LiteralPath $resolved -Recurse -Force
        Write-Output 'Owned X evidence Chrome fixture artifacts cleaned; project-local cache retained.'
    } else { Write-Warning ('Fixture artifacts retained at ' + $taskRoot) }
}

param()
$ErrorActionPreference = 'Stop'
$projectRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$runId = [guid]::NewGuid().ToString('N')
$gateRoot = Join-Path $projectRoot '.go-build\inference-sdk-v023'
$runRoot = Join-Path $gateRoot ('run-' + $runId)
foreach ($relative in @('.go-build', '.go-build\mod', '.go-build\sdk-v023-cache', '.go-build\inference-sdk-v023')) {
    $directory = Join-Path $projectRoot $relative
    if (!(Test-Path -LiteralPath $directory -PathType Container)) {
        New-Item -ItemType Directory -Path $directory | Out-Null
    }
    if ((Get-Item -LiteralPath $directory).Attributes -band [IO.FileAttributes]::ReparsePoint) {
        throw 'Offline SDK build paths must not be reparse points.'
    }
}
New-Item -ItemType Directory -Path $runRoot | Out-Null
$tempRoot = Join-Path $runRoot 'tmp'
New-Item -ItemType Directory -Path $tempRoot | Out-Null
$logPath = Join-Path $runRoot 'offline-results.log'
$variables = @('TEMP', 'TMP', 'GOTMPDIR', 'GOCACHE', 'GOMODCACHE', 'GOWORK', 'GOPROXY', 'GONOPROXY', 'GOSUMDB', 'GOTOOLCHAIN', 'CGO_ENABLED', 'GOFLAGS', 'AKU_CODEX_LIVE', 'AKU_GEMINI_LIVE', 'AKU_GROQ_LIVE', 'AKU_FAKE_CODEX_APP_SERVER', 'GIT_TERMINAL_PROMPT', 'GCM_INTERACTIVE')
$previous = @{}
foreach ($name in $variables) {
    $previous[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
}
function Invoke-OfflineGo {
    param([string[]]$GoArguments)
    Write-Output ('go ' + ($GoArguments -join ' ')) | Tee-Object -FilePath $logPath -Append
    & go @GoArguments 2>&1 | Tee-Object -FilePath $logPath -Append
    if ($LASTEXITCODE -ne 0) { throw 'Offline SDK gate failed; see the project-local log.' }
}
Push-Location -LiteralPath $projectRoot
try {
    $env:TEMP = $tempRoot; $env:TMP = $tempRoot; $env:GOTMPDIR = $tempRoot
    $env:GOCACHE = Join-Path $projectRoot '.go-build\sdk-v023-cache'
    $env:GOMODCACHE = Join-Path $projectRoot '.go-build\mod'
    $env:GOWORK = 'off'; $env:GOPROXY = 'off'; $env:GOSUMDB = 'off'
    # An inherited GOPRIVATE/GONOPROXY must not bypass GOPROXY=off.
    $env:GONOPROXY = 'none'; $env:GIT_TERMINAL_PROMPT = '0'; $env:GCM_INTERACTIVE = '0'
    $env:GOTOOLCHAIN = 'local'; $env:CGO_ENABLED = '0'; $env:GOFLAGS = ''
    $env:AKU_CODEX_LIVE = '0'; $env:AKU_GEMINI_LIVE = '0'; $env:AKU_GROQ_LIVE = '0'
    $env:AKU_FAKE_CODEX_APP_SERVER = '0'
    $modules = @(& go list -m -f '{{.Path}} {{.Version}}{{if .Replace}} replaced{{end}}' github.com/abangkis/ai4u-inference-sdk-go github.com/abangkis/ai4u-common-execution-profile-go)
    if ($LASTEXITCODE -ne 0 -or $modules.Count -ne 2 -or
        $modules[0] -ne 'github.com/abangkis/ai4u-inference-sdk-go v0.23.0' -or
        $modules[1] -ne 'github.com/abangkis/ai4u-common-execution-profile-go v0.2.0') {
        throw 'Published SDK v0.23.0 and Common Profile v0.2.0 must already be cached, without replacements.'
    }
    $modules | Tee-Object -FilePath $logPath
    # This legacy factory test resolves OS credentials before its JSON fallback.
    # Other Gemini tests inject a static fake resolver and remain enabled.
    Invoke-OfflineGo -GoArguments @('test', '-p', '1', '-count=1', '-timeout=120s', '-skip', '^TestNewProviderRoutesGeminiEntries$', './internal/config', './internal/reasoning')
    # Named files exclude Engine/HTTP TestMain and every Store-backed test.
    Invoke-OfflineGo -GoArguments @('test', '-count=1', '-timeout=60s', 'internal/engine/ollama_contract_readiness.go', 'internal/engine/ollama_contract_readiness_test.go')
    Invoke-OfflineGo -GoArguments @('test', '-count=1', '-timeout=60s', 'internal/httpapi/calibration_failure.go', 'internal/httpapi/calibration_failure_test.go')
    # Compile full package tests, but never execute these Store-backed suites.
    Invoke-OfflineGo -GoArguments @('test', '-c', '-o', (Join-Path $runRoot 'engine.test.exe'), './internal/engine')
    Invoke-OfflineGo -GoArguments @('test', '-c', '-o', (Join-Path $runRoot 'httpapi.test.exe'), './internal/httpapi')
    Invoke-OfflineGo -GoArguments @('build', '-o', (Join-Path $runRoot 'akusidecar-default.exe'), './cmd/akusidecar')
    Invoke-OfflineGo -GoArguments @('version', '-m', (Join-Path $runRoot 'akusidecar-default.exe'))
    Invoke-OfflineGo -GoArguments @('build', '-tags', 'auth_session_dev auth_session_fixture', '-o', (Join-Path $runRoot 'akusidecar-fixture.exe'), './cmd/akusidecar')
    Write-Output 'Offline SDK gate passed. No Store/database, live provider, browser or user runtime was opened.' | Tee-Object -FilePath $logPath -Append
    Write-Output ('Review artifacts: ' + $runRoot)
} finally {
    Pop-Location
    foreach ($name in $variables) {
        [Environment]::SetEnvironmentVariable($name, $previous[$name], 'Process')
    }
}

param()
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'source-cache.ps1')
$projectRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$cache = Get-SourceBuildCachePaths -Repository $projectRoot
$runRoot = Join-Path $projectRoot ('.go-build\codex-discovery\run-' + [guid]::NewGuid().ToString('N'))
$taskTemp = Join-Path $runRoot 'tmp'
New-Item -ItemType Directory -Path $taskTemp -Force | Out-Null
$variables = @('TEMP','TMP','GOTMPDIR','GOMODCACHE','GOCACHE','GOWORK','GOPROXY','GONOPROXY','GOSUMDB','GOTOOLCHAIN','GIT_TERMINAL_PROMPT','GCM_INTERACTIVE','AKU_CODEX_LIVE','AKU_GEMINI_LIVE','AKU_GROQ_LIVE','AKU_CODEX_PATH')
$previous = @{}
foreach ($name in $variables) { $previous[$name] = [Environment]::GetEnvironmentVariable($name, 'Process') }
function Invoke-Checked {
    param([string] $Program, [string[]] $Arguments, [string] $Log)
    & $Program @Arguments *> (Join-Path $runRoot $Log)
    if ($LASTEXITCODE -ne 0) {
        Get-Content -LiteralPath (Join-Path $runRoot $Log) -Tail 60
        throw "$Program failed: $Log"
    }
    Write-Output "Passed: $Log"
}
Push-Location -LiteralPath $projectRoot
try {
    $env:TEMP=$taskTemp; $env:TMP=$taskTemp; $env:GOTMPDIR=$taskTemp
    $env:GOMODCACHE=$cache.GoModules; $env:GOCACHE=$cache.GoBuild
    $env:GOWORK='off'; $env:GOPROXY='off'; $env:GONOPROXY='none'; $env:GOSUMDB='off'; $env:GOTOOLCHAIN='local'
    $env:GIT_TERMINAL_PROMPT='0'; $env:GCM_INTERACTIVE='0'
    $env:AKU_CODEX_LIVE='0'; $env:AKU_GEMINI_LIVE='0'; $env:AKU_GROQ_LIVE='0'; $env:AKU_CODEX_PATH=''
    $module = & go list -m -f '{{.Path}} {{.Version}}{{if .Replace}} replaced{{end}}' github.com/abangkis/ai4u-library-go
    if ($LASTEXITCODE -ne 0 -or $module -ne 'github.com/abangkis/ai4u-library-go v0.4.0') { throw 'Published LibraryGo v0.4.0 is required without replacements.' }
    $module | Set-Content -LiteralPath (Join-Path $runRoot 'module.txt')
    Invoke-Checked go @('mod','verify') 'modules.log'
    Invoke-Checked go @('test','-json','-count=1','-timeout=180s','./internal/codexselection','./internal/domain','./internal/config','./internal/store','./internal/reasoning','./internal/engine','./internal/httpapi','./cmd/akusidecar') 'go-tests.jsonl'
    Invoke-Checked go @('test','-tags','auth_session_dev auth_session_fixture','-count=1','-timeout=90s','./internal/engine','-run','^TestCodex(Automatic|Explicit|Selection)') 'development-tests.log'
    Invoke-Checked node @('--test','test/reasoning-executable-selection.test.mjs','test/reasoning-runtime-state.test.mjs','test/settings-render.test.mjs','test/onboarding-provider-credential.test.mjs') 'node-tests.log'
    Invoke-Checked go @('build','-o',(Join-Path $runRoot 'akusidecar.exe'),'./cmd/akusidecar') 'build.log'
    Invoke-Checked go @('build','-tags','auth_session_dev auth_session_fixture','-o',(Join-Path $runRoot 'akusidecar-development.exe'),'./cmd/akusidecar') 'development-build.log'
    Invoke-Checked go @('version','-m',(Join-Path $runRoot 'akusidecar.exe')) 'build-modules.log'
    Write-Output "Validated artifacts: $runRoot"
} finally {
    Pop-Location
    foreach ($name in $variables) { [Environment]::SetEnvironmentVariable($name, $previous[$name], 'Process') }
}

param([switch]$VerifyChrome, [string]$ChromePath = 'C:\Program Files\Google\Chrome\Application\chrome.exe')
$ErrorActionPreference = 'Stop'
$projectRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$qaParent = Join-Path $projectRoot 'runtime\qa\post-screenshot'
New-Item -ItemType Directory -Path $qaParent -Force | Out-Null
if ((Get-Item -LiteralPath $qaParent).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'QA parent must not be a reparse point.' }
$taskRoot = Join-Path $qaParent ([guid]::NewGuid().ToString('N'))
$temporary = Join-Path $taskRoot 'temporary'
New-Item -ItemType Directory -Path $temporary -Force | Out-Null
$variables = @('TEMP','TMP','GOTMPDIR','GOCACHE','GOMODCACHE','GOPROXY','GOSUMDB','CGO_ENABLED','GOTOOLCHAIN','AKU_POST_SCREENSHOT_SMOKE','AKU_POST_SCREENSHOT_ARTIFACTS','AKU_POST_SCREENSHOT_CHROME')
$previous = @{}
foreach ($variable in $variables) { $previous[$variable] = [Environment]::GetEnvironmentVariable($variable, 'Process') }
Push-Location -LiteralPath $projectRoot
$passed = $false
try {
    $env:TEMP=$temporary; $env:TMP=$temporary; $env:GOTMPDIR=$temporary
    $env:GOCACHE=Join-Path $projectRoot '.go-build\screenshot-cache'
    $env:GOMODCACHE=Join-Path $projectRoot '.go-build\mod'
    $env:GOPROXY='off'; $env:GOSUMDB='off'; $env:CGO_ENABLED='0'; $env:GOTOOLCHAIN='local'
    $env:AKU_POST_SCREENSHOT_SMOKE = if ($VerifyChrome) { '1' } else { '' }
    if ($VerifyChrome) {
        if (!(Test-Path -LiteralPath $ChromePath -PathType Leaf)) { throw 'The fixture Chrome executable is unavailable; pass -ChromePath explicitly.' }
        $env:AKU_POST_SCREENSHOT_CHROME = (Resolve-Path -LiteralPath $ChromePath).ProviderPath
    }
    $env:AKU_POST_SCREENSHOT_ARTIFACTS=$taskRoot
    $nodeTests = @(Get-ChildItem -LiteralPath (Join-Path $projectRoot 'test') -Filter 'post-screenshot*.test.mjs' -File | ForEach-Object { $_.FullName })
    if (!$nodeTests.Count) { throw 'Screenshot frontend regressions are missing.' }
    & node --test @nodeTests *> (Join-Path $taskRoot 'node-tests.txt')
    if ($LASTEXITCODE -ne 0) { throw 'Screenshot frontend regressions failed.' }
    & go test ./internal/httpapi -run '^TestPostScreenshot' -count=1 -timeout=180s -v *> (Join-Path $taskRoot 'go-tests.txt')
    if ($LASTEXITCODE -ne 0) { throw 'Screenshot API/compositor regressions failed.' }
    $passed=$true
    Write-Output 'Post screenshot regressions passed.'
} finally {
    Pop-Location
    foreach ($variable in $variables) { [Environment]::SetEnvironmentVariable($variable, $previous[$variable], 'Process') }
    if ($passed -and (Test-Path -LiteralPath $temporary)) {
        $resolved=(Resolve-Path -LiteralPath $temporary).ProviderPath
        if ($resolved -ne $temporary -or [IO.Path]::GetDirectoryName($resolved) -ne $taskRoot -or ((Get-Item -LiteralPath $resolved).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Refusing temporary cleanup outside owned QA root.' }
        if (Get-ChildItem -LiteralPath $resolved -Recurse -Force | Where-Object { $_.Attributes -band [IO.FileAttributes]::ReparsePoint }) { throw 'Refusing cleanup through reparse point.' }
        Remove-Item -LiteralPath $resolved -Recurse -Force
    }
    Write-Output ('SCREENSHOT_QA_ROOT=' + $taskRoot)
}

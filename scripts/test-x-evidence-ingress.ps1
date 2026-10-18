param()
$ErrorActionPreference = 'Stop'
$projectRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$qaParent = Join-Path $projectRoot 'runtime\qa\auth-session-sidecar'
New-Item -ItemType Directory -Path $qaParent -Force | Out-Null
if ((Get-Item -LiteralPath $qaParent).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'QA parent must not be a reparse point.' }
$taskRoot = Join-Path $qaParent ('run-x-evidence-ingress-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $taskRoot | Out-Null
$variables = @('TEMP', 'TMP')
$previous = @{}
foreach ($variable in $variables) { $previous[$variable] = [Environment]::GetEnvironmentVariable($variable, 'Process') }
$passed = $false
Push-Location -LiteralPath $projectRoot
try {
    $env:TEMP = $taskRoot
    $env:TMP = $taskRoot
    & node --check scripts/auth-session-fixture/x-evidence-ingress.mjs
    if ($LASTEXITCODE -ne 0) { throw 'Offline ingress syntax check failed.' }
    & node --check scripts/auth-session-fixture/x-evidence-ingress.test.mjs
    if ($LASTEXITCODE -ne 0) { throw 'Offline ingress test syntax check failed.' }
    & node --test --test-timeout=10000 --test-reporter=tap scripts/auth-session-fixture/x-evidence-ingress.test.mjs
    if ($LASTEXITCODE -ne 0) { throw 'Offline ingress regression tests failed.' }
    $passed = $true
    Write-Output 'Offline ingress passed; no browser, network, account access, Store policy change or user-runtime operation.'
} finally {
    Pop-Location
    foreach ($variable in $variables) { [Environment]::SetEnvironmentVariable($variable, $previous[$variable], 'Process') }
    if ($passed) {
        $resolved = (Resolve-Path -LiteralPath $taskRoot).ProviderPath
        if ($resolved -ne $taskRoot -or [IO.Path]::GetDirectoryName($resolved) -ne $qaParent -or ((Get-Item -LiteralPath $resolved).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Refusing cleanup outside owned QA root.' }
        if (Get-ChildItem -LiteralPath $resolved -Recurse -Force | Where-Object { $_.Attributes -band [IO.FileAttributes]::ReparsePoint }) { throw 'Refusing cleanup through reparse point.' }
        Remove-Item -LiteralPath $resolved -Recurse -Force
        Write-Output 'Owned offline ingress temporary artifacts cleaned.'
    } else { Write-Warning ('Offline ingress artifacts retained at ' + $taskRoot) }
}

param([switch]$VerifyChrome,[Alias('Scenario')][ValidateSet('overlap-last-close','opt-out','collector-loss','last-close-denied','mismatch','expiry','parent-cancel','parent-process-death','authorized-renewal','authorized-renewal-delay','authorized-renewal-failure','authorized-account-drift','authorized-policy-drift','authorized-coverage-gap','authorized-opt-out','authorized-last-close-denied','authorized-parent-process-death')][string[]]$SelectedScenarios = @('overlap-last-close','opt-out','collector-loss','last-close-denied','mismatch','expiry','parent-cancel','parent-process-death'))
$ErrorActionPreference = 'Stop'
$projectRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$qaParent = Join-Path $projectRoot 'runtime\qa\auth-session-sidecar'
foreach ($relative in @('runtime', 'runtime\qa', 'runtime\qa\auth-session-sidecar', '.go-build', '.go-build\x-session-verifier', '.go-build\mod')) {
    $directory = Join-Path $projectRoot $relative
    if (!(Test-Path -LiteralPath $directory -PathType Container)) { New-Item -ItemType Directory -Path $directory | Out-Null }
    if ((Get-Item -LiteralPath $directory).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Concurrent Chrome QA paths must not be reparse points.' }
}
$runId = [guid]::NewGuid().ToString('N')
$taskRoot = Join-Path $qaParent ('run-concurrent-chrome-helper-' + $runId)
New-Item -ItemType Directory -Path $taskRoot | Out-Null
$log = Join-Path $qaParent ('concurrent-chrome-integration-' + $runId + '.log.txt')
$node = Join-Path $projectRoot 'runtime\dev\headless-worker\node.exe'
$chrome = 'C:\Program Files\Google\Chrome\Application\chrome.exe'
foreach ($path in @($node,$chrome)) { if (!(Test-Path -LiteralPath $path -PathType Leaf)) { throw 'Configured official QA executable is unavailable.' } }
$variables = @('TEMP','TMP','GOTMPDIR','GOCACHE','GOMODCACHE','GOWORK','GOPROXY','GOSUMDB','GOTOOLCHAIN','CGO_ENABLED','GOFLAGS')
$previous = @{}
foreach ($name in $variables) { $previous[$name] = [Environment]::GetEnvironmentVariable($name,'Process') }

function Convert-ConcurrentChromeReceipt([string]$Raw,[string]$ExpectedScenario) {
    $reply = $Raw | ConvertFrom-Json -NoEnumerate -ErrorAction Stop
    if ($null -eq $reply -or $reply -is [array]) { throw 'Concurrent Chrome receipt object rejected.' }
    $fields = @($reply.PSObject.Properties.Name | Sort-Object)
    $allowed = @('canAuthorize','cleanupVerified','loginCount','nativeQualified','overlapVerified','passed','provider','scenario','seedCount' | Sort-Object)
    $authorized = $ExpectedScenario.StartsWith('authorized-', [StringComparison]::Ordinal)
    if ($authorized) { $allowed = @($allowed + @('authoritySelected','automaticRenewals','probesDuringCollect','originalTicketVersion','finalEvidenceVersion','protectedRequestMillis','staleDenied','duplicateDenied') | Sort-Object) }
    if (($fields -join ',') -ne ($allowed -join ',') -or $reply.provider -isnot [string] -or $reply.scenario -isnot [string] -or $reply.provider -ne 'synthetic-concurrent-chrome-fixture' -or $reply.scenario -ne $ExpectedScenario) { throw 'Concurrent Chrome bounded receipt fields rejected.' }
    foreach ($name in @('passed','cleanupVerified','overlapVerified','canAuthorize','nativeQualified')) { if ($reply.$name -isnot [bool]) { throw 'Concurrent Chrome receipt boolean type rejected.' } }
    if (!$reply.passed -or !$reply.cleanupVerified -or !$reply.overlapVerified -or $reply.canAuthorize -or $reply.nativeQualified) { throw 'Concurrent Chrome receipt authority/acceptance rejected.' }
    foreach ($name in @('loginCount','seedCount')) { if (($reply.$name -isnot [int] -and $reply.$name -isnot [long]) -or $reply.$name -ne 1) { throw 'Concurrent Chrome receipt one-login/seed ceiling rejected.' } }
    if ($authorized) {
        foreach ($name in @('authoritySelected','staleDenied','duplicateDenied')) { if ($reply.$name -isnot [bool]) { throw 'Authorized Chrome receipt flag type rejected.' } }
        if (!$reply.authoritySelected) { throw 'Authorized Chrome authority selection rejected.' }
        foreach ($name in @('automaticRenewals','probesDuringCollect','originalTicketVersion','finalEvidenceVersion','protectedRequestMillis')) {
            if (($reply.$name -isnot [int] -and $reply.$name -isnot [long]) -or $reply.$name -lt 0 -or $reply.$name -gt 100000) { throw 'Authorized Chrome receipt counter rejected.' }
        }
        if ($reply.protectedRequestMillis -gt 45000 -or $reply.originalTicketVersion -lt 1 -or $reply.finalEvidenceVersion -lt $reply.originalTicketVersion) { throw 'Authorized Chrome receipt duration/version rejected.' }
        if ($ExpectedScenario -eq 'authorized-renewal' -and ($reply.automaticRenewals -lt 3 -or $reply.probesDuringCollect -lt (2 * $reply.automaticRenewals) -or $reply.originalTicketVersion -ne 1 -or $reply.finalEvidenceVersion -lt ($reply.originalTicketVersion + $reply.automaticRenewals) -or $reply.protectedRequestMillis -lt 10000 -or !$reply.staleDenied -or !$reply.duplicateDenied)) { throw 'Authorized Chrome uninterrupted renewal evidence rejected.' }
    }
    return $reply
}

function Invoke-ConcurrentChrome([string]$Executable,[string[]]$Arguments,[string]$Payload,[bool]$Accepted,[string]$Label) {
    $start = [Diagnostics.ProcessStartInfo]::new()
    $start.FileName = $Executable; $start.WorkingDirectory = Join-Path $projectRoot 'internal\engine'
    $start.UseShellExecute = $false; $start.CreateNoWindow = $true
    $start.RedirectStandardInput = $true; $start.RedirectStandardOutput = $true; $start.RedirectStandardError = $true
    foreach ($argument in $Arguments) { $start.ArgumentList.Add($argument) }
    $process = [Diagnostics.Process]::new(); $process.StartInfo = $start
    if (!$process.Start()) { throw 'Concurrent Chrome fixture process unavailable.' }
    try {
        $stdout = $process.StandardOutput.ReadToEndAsync(); $stderr = $process.StandardError.ReadToEndAsync()
        $process.StandardInput.Write($Payload); $process.StandardInput.Close()
        if (!$process.WaitForExit(60000)) { throw 'Concurrent Chrome fixture timed out; profile roots retained.' }
        $result = $stdout.GetAwaiter().GetResult() + $stderr.GetAwaiter().GetResult()
        if ($Accepted) {
            if ($process.ExitCode -ne 0) { throw ('Concurrent Chrome fixture failed: ' + $Label + ' ' + $result) }
            $expected = $Payload | ConvertFrom-Json
            $reply = Convert-ConcurrentChromeReceipt $result $expected.scenario
            if (Test-Path -LiteralPath $expected.root) { throw 'Successful fixture retained an unverified profile root.' }
            if ($expected.scenario.StartsWith('authorized-', [StringComparison]::Ordinal)) { Write-Output ('RECEIPT: ' + ($reply | ConvertTo-Json -Compress)) | Tee-Object -FilePath $log -Append }
        } elseif ($process.ExitCode -eq 0 -or $result -notmatch '(development session|synthetic concurrent)') { throw ('Concurrent Chrome rejection gate failed: ' + $Label + ' ' + $result) }
        Write-Output ('PASS: ' + $Label) | Tee-Object -FilePath $log -Append
    } finally {
        if (!$process.HasExited) { $process.Kill($true); $process.WaitForExit() }
        $process.Dispose()
    }
}

Push-Location -LiteralPath $projectRoot
try {
    $env:TEMP=$taskRoot; $env:TMP=$taskRoot; $env:GOTMPDIR=$taskRoot
    $env:GOCACHE=Join-Path $projectRoot '.go-build\x-session-verifier'; $env:GOMODCACHE=Join-Path $projectRoot '.go-build\mod'
    $env:GOWORK='off'; $env:GOPROXY='off'; $env:GOSUMDB='off'; $env:GOTOOLCHAIN='local'; $env:CGO_ENABLED='0'; $env:GOFLAGS=''
    Write-Output ('Synthetic concurrent Chrome integration run: ' + $runId) | Tee-Object -FilePath $log
    $receipt = @{provider='synthetic-concurrent-chrome-fixture';scenario='overlap-last-close';passed=$true;cleanupVerified=$true;overlapVerified=$true;loginCount=1;seedCount=1;canAuthorize=$false;nativeQualified=$false}
    Convert-ConcurrentChromeReceipt ($receipt|ConvertTo-Json -Compress) 'overlap-last-close' | Out-Null
    foreach ($change in @('privateField','secretField','missingFlag','nullFlag','authority','cleanupFalse','extraLogin','zeroSeed','stringCount','fractionCount','negativeCount','missingOverlap','wrongType','wrongScenario','wrongProvider','arrayScenario','arrayProvider')) {
        $invalid=$receipt.Clone()
        switch ($change) {
            'privateField' { $invalid.accountId='unavailable' }
            'secretField' { $invalid.cookie='unavailable' }
            'missingFlag' { $invalid.Remove('canAuthorize') }
            'nullFlag' { $invalid.nativeQualified=$null }
            'authority' { $invalid.canAuthorize=$true }
            'cleanupFalse' { $invalid.cleanupVerified=$false }
            'extraLogin' { $invalid.loginCount=2 }
            'zeroSeed' { $invalid.seedCount=0 }
            'stringCount' { $invalid.seedCount='1' }
            'fractionCount' { $invalid.loginCount=1.5 }
            'negativeCount' { $invalid.loginCount=-1 }
            'missingOverlap' { $invalid.overlapVerified=$false }
            'wrongType' { $invalid.passed='true' }
            'wrongScenario' { $invalid.scenario='native' }
            'wrongProvider' { $invalid.provider='native' }
            'arrayScenario' { $invalid.scenario=@('overlap-last-close') }
            'arrayProvider' { $invalid.provider=@('synthetic-concurrent-chrome-fixture') }
        }
        $rejected=$false
        try { Convert-ConcurrentChromeReceipt ($invalid|ConvertTo-Json -Compress) 'overlap-last-close' | Out-Null } catch { $rejected=$true }
        if (!$rejected) { throw 'Malformed concurrent Chrome receipt accepted.' }
    }
    Write-Output 'PASS: fixed receipt fields/types, one-login/seed ceiling, false authority and malformed-report rejections' | Tee-Object -FilePath $log -Append
    $authorizedReceipt = $receipt.Clone()
    $authorizedReceipt.scenario = 'authorized-renewal'
    $authorizedReceipt.authoritySelected = $true; $authorizedReceipt.automaticRenewals = 3; $authorizedReceipt.probesDuringCollect = 6
    $authorizedReceipt.originalTicketVersion = 1; $authorizedReceipt.finalEvidenceVersion = 4; $authorizedReceipt.protectedRequestMillis = 10000
    $authorizedReceipt.staleDenied = $true; $authorizedReceipt.duplicateDenied = $true
    Convert-ConcurrentChromeReceipt ($authorizedReceipt | ConvertTo-Json -Compress) 'authorized-renewal' | Out-Null
    foreach ($change in @('unselected','missingRenewals','renewalsString','renewalsFraction','negativeProbes','insufficientRenewals','insufficientProbes','retaggedTicket','brokenVersionChain','shortCapture','overlongCapture','staleNotDenied','duplicateNotDenied','authority','privateField','arrayRoot','flagArray')) {
        $invalid = $authorizedReceipt.Clone()
        switch ($change) {
            'unselected' { $invalid.authoritySelected = $false }
            'missingRenewals' { $invalid.Remove('automaticRenewals') }
            'renewalsString' { $invalid.automaticRenewals = '3' }
            'renewalsFraction' { $invalid.automaticRenewals = 3.5 }
            'negativeProbes' { $invalid.probesDuringCollect = -1 }
            'insufficientRenewals' { $invalid.automaticRenewals = 2 }
            'insufficientProbes' { $invalid.probesDuringCollect = 5 }
            'retaggedTicket' { $invalid.originalTicketVersion = 2 }
            'brokenVersionChain' { $invalid.finalEvidenceVersion = 3 }
            'shortCapture' { $invalid.protectedRequestMillis = 9999 }
            'overlongCapture' { $invalid.protectedRequestMillis = 45001 }
            'staleNotDenied' { $invalid.staleDenied = $false }
            'duplicateNotDenied' { $invalid.duplicateDenied = $false }
            'authority' { $invalid.canAuthorize = $true }
            'privateField' { $invalid.accountId = 'unavailable' }
            'flagArray' { $invalid.authoritySelected = @($true) }
        }
        $raw = $invalid | ConvertTo-Json -Compress
        if ($change -eq 'arrayRoot') { $raw = '[' + $raw + ']' }
        $rejected = $false
        try { Convert-ConcurrentChromeReceipt $raw 'authorized-renewal' | Out-Null } catch { $rejected = $true }
        if (!$rejected) { throw 'Malformed authorized Chrome receipt accepted.' }
    }
    Write-Output 'PASS: authorized receipt exact fields/types, renewal/probe/version/duration thresholds and 17 malformed-report rejections' | Tee-Object -FilePath $log -Append
    & $node --test scripts/auth-session-fixture/concurrent-engine-commands.test.mjs 2>&1 | Tee-Object -FilePath $log -Append
    if ($LASTEXITCODE -ne 0) { throw 'Concurrent Chrome offline RPC boundaries failed.' }
    & $node --test scripts/auth-session-fixture/concurrent-driver-dispatch.test.mjs 2>&1 | Tee-Object -FilePath $log -Append
    if ($LASTEXITCODE -ne 0) { throw 'Concurrent Chrome bounded dispatch regression failed.' }
    & $node --test scripts/auth-session-fixture/concurrent-protected-collection.test.mjs 2>&1 | Tee-Object -FilePath $log -Append
    if ($LASTEXITCODE -ne 0) { throw 'Concurrent Chrome protected collection regression failed.' }
    & go test -tags 'auth_session_dev auth_session_fixture' -count=1 -timeout=30s -v internal/engine/development_concurrent_chrome_input.go internal/engine/development_concurrent_chrome_input_test.go 2>&1 | Tee-Object -FilePath $log -Append
    if ($LASTEXITCODE -ne 0) { throw 'Concurrent Chrome offline input boundaries failed.' }
    $executable = Join-Path $taskRoot 'akusidecar.exe'
    $missingConfig = Join-Path $taskRoot 'absent-config\sidecar.json'; $missingDatabase = Join-Path $taskRoot 'absent-database\sidecar.db'
    $arguments = @('--dev','--dev-session-reuse','synthetic-concurrent-chrome-fixture','--config',$missingConfig,'--database',$missingDatabase)
    foreach ($mode in @('default','auth_session_dev','auth_session_fixture','auth_session_dev auth_session_fixture')) {
        $build = @('build','-o',$executable); if ($mode -ne 'default') { $build += @('-tags',$mode) }; $build += './cmd/akusidecar'
        & go @build 2>&1 | Tee-Object -FilePath $log -Append
        if ($LASTEXITCODE -ne 0) { throw ('Concurrent Chrome actual-main build failed: ' + $mode) }
        Write-Output ('PASS: actual main build ' + $mode) | Tee-Object -FilePath $log -Append
        Invoke-ConcurrentChrome $executable $arguments '{}' $false ($mode + ' rejects ungranted Chrome fixture')
    }
    $root = Join-Path $qaParent ('run-concurrent-chrome-' + [guid]::NewGuid().ToString('N'))
    $valid = @{grant='synthetic-owned-chrome-fixture'; scenario='overlap-last-close';root=$root;nodePath=$node;chromePath=$chrome}
    New-Item -ItemType Directory -Path $root | Out-Null
    try {
        Invoke-ConcurrentChrome $executable @('--dev-session-reuse','synthetic-concurrent-chrome-fixture','--config',$missingConfig,'--database',$missingDatabase) ($valid|ConvertTo-Json -Compress) $false 'Chrome fixture requires --dev'
        New-Item -ItemType Directory -Path (Join-Path $root 'shared') | Out-Null
        Invoke-ConcurrentChrome $executable $arguments ($valid|ConvertTo-Json -Compress) $false 'existing profile/root reuse rejected before Chrome'
        Remove-Item -LiteralPath (Join-Path $root 'shared') -Force
        $aliasRequest=$valid.Clone()
        $aliasRequest.root=$qaParent + '\alias\..\' + [IO.Path]::GetFileName($root)
        Invoke-ConcurrentChrome $executable $arguments ($aliasRequest|ConvertTo-Json -Compress) $false 'canonical lexical root alias rejected before Chrome'
    } finally { if (Test-Path -LiteralPath $root) { Remove-Item -LiteralPath $root -Force } }
    if ((Test-Path -LiteralPath $missingConfig) -or (Test-Path -LiteralPath (Split-Path -Parent $missingConfig))) { throw 'Configuration sentinel touched.' }
    if ((Test-Path -LiteralPath $missingDatabase) -or (Test-Path -LiteralPath (Split-Path -Parent $missingDatabase))) { throw 'Database sentinel touched.' }
    Write-Output 'PASS: offline input/RPC/root-alias/reuse/default-off gates; no Chrome launched' | Tee-Object -FilePath $log -Append
    if ($VerifyChrome) {
        foreach ($scenario in $SelectedScenarios) {
            $root = Join-Path $qaParent ('run-concurrent-chrome-' + [guid]::NewGuid().ToString('N'))
            New-Item -ItemType Directory -Path $root | Out-Null
            $payload=@{grant='synthetic-owned-chrome-fixture';scenario=$scenario;root=$root;nodePath=$node;chromePath=$chrome}|ConvertTo-Json -Compress
            Invoke-ConcurrentChrome $executable $arguments $payload $true ('actual main fresh Chrome ' + $scenario)
        }
        if ((Test-Path -LiteralPath $missingConfig) -or (Test-Path -LiteralPath $missingDatabase) -or (Test-Path -LiteralPath (Split-Path -Parent $missingConfig)) -or (Test-Path -LiteralPath (Split-Path -Parent $missingDatabase))) { throw 'Early-dispatch sentinel touched.' }
        Write-Output ('PASS: selected fresh-Chrome scenarios use controlled loopback, memory sink and verified whole-role cleanup: ' + ($SelectedScenarios -join ', ')) | Tee-Object -FilePath $log -Append
    }
    Write-Output ('Concurrent Chrome log: ' + $log)
} finally {
    Pop-Location
    foreach ($name in $variables) { [Environment]::SetEnvironmentVariable($name,$previous[$name],'Process') }
    if (Test-Path -LiteralPath $taskRoot -PathType Container) {
        $resolved=(Resolve-Path -LiteralPath $taskRoot).ProviderPath
        if ($resolved -ne $taskRoot -or [IO.Path]::GetDirectoryName($resolved) -ne $qaParent -or ((Get-Item -LiteralPath $resolved).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Refusing cleanup outside owned Chrome helper root.' }
        if (Get-ChildItem -LiteralPath $resolved -Recurse -Force | Where-Object { $_.Attributes -band [IO.FileAttributes]::ReparsePoint }) { throw 'Refusing cleanup through reparse point.' }
        Remove-Item -LiteralPath $resolved -Recurse -Force
        Write-Output 'Owned helper build root removed; failed Chrome profile roots, if any, are retained.'
    }
}

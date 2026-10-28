param([string] $ResultPath)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$script:assertionCount = 0
$script:caseNames = [System.Collections.Generic.List[string]]::new()
$script:fakePathRoot = [IO.Path]::GetFullPath((Join-Path $projectRoot ('build\test-dev-graceful-shutdown-' + [Guid]::NewGuid().ToString('N'))))
$script:targetPath = Join-Path $script:fakePathRoot 'runtime\dev\aku-sidecar.exe'
$script:browserPath = Join-Path $script:fakePathRoot 'runtime\dev\chrome.exe'
$script:hadLastExitCode = $null -ne (Get-Variable -Name LASTEXITCODE -Scope Global -ErrorAction SilentlyContinue)
$script:previousLastExitCode = if ($script:hadLastExitCode) { $global:LASTEXITCODE } else { $null }
$script:hadControlToken = Test-Path Env:AKU_RUNTIME_CONTROL_TOKEN
$script:previousControlToken = $env:AKU_RUNTIME_CONTROL_TOKEN

. (Join-Path $PSScriptRoot 'dev-runtime-contract.ps1')
. (Join-Path $PSScriptRoot 'dev-graceful-shutdown.ps1')

function Assert-True([bool] $Condition, [string] $Message) {
    $script:assertionCount++
    if (-not $Condition) { throw $Message }
}

function Assert-Throws([scriptblock] $Action, [string] $Message) {
    $threw = $false
    try { & $Action | Out-Null } catch { $threw = $true }
    Assert-True $threw $Message
}

function Invoke-TestCase([string] $Name, [scriptblock] $Action) {
    & $Action
    $script:caseNames.Add($Name)
}

function New-FakeProcess([long] $Id, [long] $ParentId, [string] $Image, [DateTime] $Created, [string] $CommandLine, [string] $Name) {
    return [pscustomobject]@{
        ProcessId = $Id
        ParentProcessId = $ParentId
        ExecutablePath = $Image
        CreationDate = $Created
        CommandLine = $CommandLine
        Name = $Name
    }
}

function New-FakeWindow([long] $ProcessId, [IntPtr] $Handle, [string] $Title, [bool] $CloseResult) {
    $window = [pscustomobject]@{
        Id = $ProcessId
        MainWindowHandle = $Handle
        MainWindowTitle = $Title
        CloseResult = $CloseResult
    }
    Add-Member -InputObject $window -MemberType ScriptMethod -Name CloseMainWindow -Value {
        $script:fixture.CloseCalls.Add([long]$this.Id)
        return [bool]$this.CloseResult
    }
    return $window
}

function New-FakeService([object] $RootPid, [object[]] $OwnedPids) {
    return [pscustomobject]@{ id = 'akusidecar'; status = 'running'; rootPid = $RootPid; ownedPids = @($OwnedPids) }
}

function New-StoppedService {
    return [pscustomobject]@{ id = 'akusidecar'; status = 'stopped'; rootPid = $null; ownedPids = @() }
}

function New-TestPlan {
    return [pscustomobject]@{
        Target = $script:targetPath
        Arguments = @('--ui-chromium-path', $script:browserPath)
        Service = [pscustomobject]@{ cwd = $projectRoot; ports = @(11122) }
    }
}

function Reset-Fixture {
    $script:fixture = @{
        FakeNow = [DateTime]::SpecifyKind([DateTime]'2026-10-28T00:00:00', [DateTimeKind]::Utc)
        Processes = @{}
        ProcessSequences = @{}
        ProcessReads = @{}
        Windows = @{}
        WindowSequences = @{}
        WindowReads = @{}
        Readiness = @()
        ReadinessIndex = 0
        ReadinessCalls = [System.Collections.Generic.List[int]]::new()
        ShutdownReplies = @()
        ShutdownIndex = 0
        ShutdownCalls = [System.Collections.Generic.List[object]]::new()
        ServiceReads = @()
        ServiceIndex = 0
        Polls = [System.Collections.Generic.List[int]]::new()
        CloseCalls = [System.Collections.Generic.List[long]]::new()
    }
}

function Get-NextSequenceValue([object[]] $Values, [int] $Index) {
    if ($Values.Count -eq 0) { return $null }
    if ($Index -lt $Values.Count) { return $Values[$Index] }
    return $Values[$Values.Count - 1]
}

function New-ReadService {
    return {
        $index = $script:fixture.ServiceIndex
        $value = Get-NextSequenceValue $script:fixture.ServiceReads $index
        if ($index -lt $script:fixture.ServiceReads.Count) { $script:fixture.ServiceIndex++ }
        return $value
    }
}

function Get-RegisteredDevService { & (New-ReadService) }

function Get-GracefulWaitAssignment([string] $WorkflowPath) {
    $tokens = $null
    $parseErrors = $null
    $ast = [System.Management.Automation.Language.Parser]::ParseFile($WorkflowPath, [ref]$tokens, [ref]$parseErrors)
    if ($parseErrors.Count -gt 0) { throw 'Development restart workflow could not be parsed for the shutdown wiring assertion.' }
    $assignments = @($ast.FindAll({
        param($node)
        $node -is [System.Management.Automation.Language.AssignmentStatementAst] -and
            $node.Left -is [System.Management.Automation.Language.VariableExpressionAst] -and
            $node.Left.VariablePath.UserPath -ceq 'observedProcesses' -and
            $node.Extent.Text.Contains('Wait-DevRuntimeGracefulShutdown')
    }, $true))
    if ($assignments.Count -ne 1) { throw "Expected one observedProcesses assignment wired to Wait-DevRuntimeGracefulShutdown, found $($assignments.Count)." }
    return [scriptblock]::Create($assignments[0].Extent.Text)
}

# These function overrides replace all host process, window, time, readiness,
# shutdown, and sleep operations after both source files have been loaded.
function Get-DevRuntimeProcess([long] $ProcessId) {
    $key = [string]$ProcessId
    if (-not $script:fixture.ProcessReads.ContainsKey($key)) { $script:fixture.ProcessReads[$key] = 0 }
    $index = [int]$script:fixture.ProcessReads[$key]
    $script:fixture.ProcessReads[$key] = $index + 1
    if ($script:fixture.ProcessSequences.ContainsKey($key)) {
        return (Get-NextSequenceValue $script:fixture.ProcessSequences[$key] $index)
    }
    return $script:fixture.Processes[$key]
}

function Get-DevRuntimeWindow([long] $ProcessId) {
    $key = [string]$ProcessId
    if (-not $script:fixture.WindowSequences.ContainsKey($key)) { return $script:fixture.Windows[$key] }
    $index = if ($script:fixture.WindowReads.ContainsKey($key)) { [int]$script:fixture.WindowReads[$key] } else { 0 }
    $script:fixture.WindowReads[$key] = $index + 1
    return (Get-NextSequenceValue $script:fixture.WindowSequences[$key] $index)
}

function Get-DevRuntimeUtcNow { return $script:fixture.FakeNow }

function Wait-DevRuntimePoll([int] $Seconds) {
    $script:fixture.Polls.Add($Seconds)
    $script:fixture.FakeNow = $script:fixture.FakeNow.AddSeconds($Seconds)
}

function Get-DevRuntimeReadiness([int] $Port) {
    $script:fixture.ReadinessCalls.Add($Port)
    $index = $script:fixture.ReadinessIndex
    if ($index -lt $script:fixture.Readiness.Count) { $script:fixture.ReadinessIndex++ }
    return (Get-NextSequenceValue $script:fixture.Readiness $index)
}

function Invoke-DevRuntimeShutdown([int] $Port, [string] $Token) {
    $script:fixture.ShutdownCalls.Add([pscustomobject]@{ Port = $Port; Token = $Token })
    $index = $script:fixture.ShutdownIndex
    if ($index -lt $script:fixture.ShutdownReplies.Count) { $script:fixture.ShutdownIndex++ }
    return (Get-NextSequenceValue $script:fixture.ShutdownReplies $index)
}

function Set-ControlToken([string] $Value) {
    if ($null -eq $Value) { Remove-Item Env:AKU_RUNTIME_CONTROL_TOKEN -ErrorAction SilentlyContinue }
    else { $env:AKU_RUNTIME_CONTROL_TOKEN = $Value }
}

function Set-ValidRunningFixture([string] $App = 'http://127.0.0.1:11122/', [bool] $CloseResult = $true) {
    Reset-Fixture
    $root = New-FakeProcess 101 0 $script:targetPath $script:fixture.FakeNow.AddMinutes(-2) $script:targetPath 'aku-sidecar.exe'
    $ui = New-FakeProcess 202 101 $script:browserPath $script:fixture.FakeNow.AddMinutes(-1) ('"' + $script:browserPath + '" --app="' + $App + '"') 'chrome.exe'
    $script:fixture.Processes['101'] = $root
    $script:fixture.Processes['202'] = $ui
    $script:fixture.Windows['202'] = New-FakeWindow 202 ([IntPtr]1234) 'AkuBrowser' $CloseResult
    $script:fixture.ServiceReads = @((New-FakeService 101 @(202)), (New-StoppedService))
    $script:fixture.Readiness = @([pscustomobject]@{ ready = $true; instanceEpoch = 'epoch-1' })
    return [pscustomobject]@{
        Plan = New-TestPlan
        Service = New-FakeService 101 @(202)
        RootIdentity = [pscustomobject]@{ pid = 101; createdTicks = $root.CreationDate.Ticks; image = $script:targetPath }
    }
}

function Invoke-RequestAttempt($Case) {
    try {
        $outcome = Request-DevRuntimeUIShutdown $Case.Plan $Case.Service 'epoch-1' $Case.RootIdentity
        return [pscustomobject]@{ Outcome = [string]$outcome; Error = $null }
    } catch {
        return [pscustomobject]@{ Outcome = $null; Error = $_.Exception.Message }
    }
}

function Assert-NoCloseRequest($Case, [string] $Label) {
    $result = Invoke-RequestAttempt $Case
    Assert-True ($result.Outcome -cne 'requested') "$Label unexpectedly requested a UI close."
    Assert-True ($script:fixture.CloseCalls.Count -eq 0) "$Label called CloseMainWindow."
}

function Assert-ObservedDrain($Observed, [string] $Label) {
    Assert-True ($Observed -is [hashtable]) "$Label did not return the observed PID-to-creation-date hashtable."
    Assert-True ($Observed.ContainsKey('101')) "$Label did not retain the root PID creation date."
}

try {
    Set-ControlToken $null

    Invoke-TestCase 'app-origin-parser' {
        Assert-True (Get-DevRuntimeUIAppMatches 'chrome.exe --app=http://127.0.0.1:11122/' 11122) 'Unquoted IPv4 loopback app URL was rejected.'
        Assert-True (Get-DevRuntimeUIAppMatches '"chrome.exe" "--app=http://localhost:11122/#ui"' 11122) 'Quoted localhost app URL or fragment was rejected.'
        Assert-True (Get-DevRuntimeUIAppMatches 'chrome.exe --app="http://[::1]:11122/"' 11122) 'Quoted IPv6 loopback app URL was rejected.'
        foreach ($commandLine in @(
            'chrome.exe --app=http://127.0.0.1:11122/path',
            'chrome.exe --app=http://127.0.0.1:11123/',
            'chrome.exe --app=http://user@127.0.0.1:11122/',
            'chrome.exe --app=http://127.0.0.1:11122/?x=1',
            'chrome.exe --app=http://capture-host:11122/',
            'chrome.exe --app=https://127.0.0.1:11122/',
            'chrome.exe --app=http://127.0.0.1:11122/ --app=http://localhost:11122/',
            'chrome.exe --app=http://127.0.0.1:11122-extra/'
        )) { Assert-True (-not (Get-DevRuntimeUIAppMatches $commandLine 11122)) "Unsafe or non-root app URL matched: $commandLine" }
    }

    Invoke-TestCase 'tokenless-ui-close-and-drain' {
        $case = Set-ValidRunningFixture
        $observed = Wait-DevRuntimeGracefulShutdown $case.Plan (New-ReadService) 30 1
        Assert-ObservedDrain $observed 'Tokenless graceful close'
        Assert-True ($script:fixture.CloseCalls.Count -eq 1 -and $script:fixture.CloseCalls[0] -eq 202) 'The unique verified AkuBrowser UI was not closed exactly once.'
        Assert-True ($script:fixture.ShutdownCalls.Count -eq 0) 'Tokenless phase invoked runtime HTTP control.'
    }

    Invoke-TestCase 'ready-false-wait-zero' {
        $case = Set-ValidRunningFixture
        $script:fixture.ServiceReads = @((New-FakeService 101 @(202)))
        $script:fixture.Readiness = @([pscustomobject]@{ ready = $false; instanceEpoch = 'epoch-1' })
        Assert-Throws { Wait-DevRuntimeGracefulShutdown $case.Plan (New-ReadService) 0 1 } 'WaitForIdleSeconds=0 with ready=false did not fail closed.'
        Assert-True ($script:fixture.CloseCalls.Count -eq 0) 'A not-ready runtime received a UI close.'
    }

    Invoke-TestCase 'busy-then-idle' {
        $case = Set-ValidRunningFixture
        Set-ControlToken 'synthetic-token'
        $script:fixture.ShutdownReplies = @($null)
        $observed = Wait-DevRuntimeGracefulShutdown $case.Plan (New-ReadService) 30 1
        Assert-ObservedDrain $observed 'Busy then idle'
        Assert-True ($script:fixture.ShutdownCalls.Count -eq 1) 'Busy control was not requested exactly once before the service drained.'
        Assert-True ($script:fixture.CloseCalls.Count -eq 0) 'Busy HTTP control fell back to UI close.'
        Set-ControlToken $null
    }

    Invoke-TestCase 'unavailable-readiness-fails-closed' {
        $case = Set-ValidRunningFixture
        $script:fixture.Readiness = @($null)
        Assert-Throws { Wait-DevRuntimeGracefulShutdown $case.Plan (New-ReadService) 30 1 } 'Unavailable readiness did not fail closed.'
        Assert-True ($script:fixture.CloseCalls.Count -eq 0) 'Unavailable readiness triggered a UI close.'
    }

    Invoke-TestCase 'epoch-change-fails-closed' {
        $case = Set-ValidRunningFixture
        $script:fixture.Readiness = @([pscustomobject]@{ ready = $true; instanceEpoch = 'epoch-2' })
        Assert-NoCloseRequest $case 'Epoch change'
    }

    Invoke-TestCase 'fresh-ui-readiness-must-keep-the-same-epoch' {
        $case = Set-ValidRunningFixture
        $script:fixture.Readiness = @(
            [pscustomobject]@{ ready = $true; instanceEpoch = 'epoch-1' },
            [pscustomobject]@{ ready = $true; instanceEpoch = 'epoch-2' }
        )
        Assert-Throws { Wait-DevRuntimeGracefulShutdown $case.Plan (New-ReadService) 30 1 } 'Changed fresh UI readiness epoch did not fail closed.'
        Assert-True ($script:fixture.CloseCalls.Count -eq 0) 'A UI close was requested after the fresh epoch changed.'
    }

    Invoke-TestCase 'root-and-ui-birth-pinning' {
        $case = Set-ValidRunningFixture
        $changedRoot = New-FakeProcess 101 0 $script:targetPath $script:fixture.FakeNow.AddMinutes(-1) $script:targetPath 'aku-sidecar.exe'
        $script:fixture.ProcessSequences['101'] = @($script:fixture.Processes['101'], $changedRoot)
        Assert-NoCloseRequest $case 'Changed root birth'

        $case = Set-ValidRunningFixture
        $changedUi = New-FakeProcess 202 101 $script:browserPath $script:fixture.FakeNow.AddSeconds(-10) 'chrome.exe --app=http://127.0.0.1:11122/' 'chrome.exe'
        $script:fixture.ProcessSequences['202'] = @($script:fixture.Processes['202'], $changedUi)
        Assert-NoCloseRequest $case 'Changed UI birth'

        $case = Set-ValidRunningFixture
        $changedWindow = New-FakeWindow 202 ([IntPtr]4321) 'AkuBrowser' $true
        $script:fixture.WindowSequences['202'] = @($script:fixture.Windows['202'], $changedWindow)
        Assert-NoCloseRequest $case 'Changed UI window handle'
    }

    Invoke-TestCase 'unrelated-and-ambiguous-ui-rejected' {
        foreach ($kind in @('wrong-parent', 'wrong-image', 'wrong-title', 'zero-handle', 'native-reader', 'capture-host', 'ambiguous')) {
            $case = Set-ValidRunningFixture
            if ($kind -eq 'wrong-parent') { $script:fixture.Processes['202'].ParentProcessId = 999 }
            if ($kind -eq 'wrong-image') { $script:fixture.Processes['202'].ExecutablePath = (Join-Path $script:fakePathRoot 'other.exe') }
            if ($kind -eq 'wrong-title') { $script:fixture.Windows['202'].MainWindowTitle = 'Reader Broker' }
            if ($kind -eq 'zero-handle') { $script:fixture.Windows['202'].MainWindowHandle = [IntPtr]::Zero }
            if ($kind -eq 'native-reader') {
                $script:fixture.Processes['202'].ExecutablePath = (Join-Path $script:fakePathRoot 'com.akubrowser.reader_activation.exe')
            }
            if ($kind -eq 'capture-host') { $script:fixture.Processes['202'].CommandLine = 'chrome.exe --app=http://capture-host:11122/' }
            if ($kind -eq 'ambiguous') {
                $extra = New-FakeProcess 303 101 $script:browserPath $script:fixture.FakeNow.AddSeconds(-30) 'chrome.exe --app=http://localhost:11122/' 'chrome.exe'
                $script:fixture.Processes['303'] = $extra
                $script:fixture.Windows['303'] = New-FakeWindow 303 ([IntPtr]5678) 'AkuBrowser' $true
                $case.Service = New-FakeService 101 @(202, 303)
            }
            Assert-NoCloseRequest $case $kind
        }
    }

    Invoke-TestCase 'close-main-window-false' {
        $case = Set-ValidRunningFixture -CloseResult:$false
        $result = Invoke-RequestAttempt $case
        Assert-True ($result.Outcome -cne 'requested' -and -not [string]::IsNullOrWhiteSpace($result.Error)) 'CloseMainWindow false was not reported as a failed graceful-close request.'
        Assert-True ($script:fixture.CloseCalls.Count -eq 1) 'The synthetic false-returning close method was not exercised.'
    }

    Invoke-TestCase 'root-gone-with-children-waits-without-closing' {
        Reset-Fixture
        $child = New-FakeProcess 303 101 $script:browserPath $script:fixture.FakeNow.AddMinutes(-1) 'chrome.exe' 'chrome.exe'
        $script:fixture.Processes['303'] = $child
        $script:fixture.ServiceReads = @((New-FakeService $null @(303)), (New-StoppedService))
        $observed = Wait-DevRuntimeGracefulShutdown (New-TestPlan) (New-ReadService) 30 1
        Assert-True ($observed -is [hashtable]) 'Root-gone child cleanup did not return an observed-process hashtable.'
        Assert-True ($script:fixture.CloseCalls.Count -eq 0) 'Root-gone child cleanup attempted a UI close.'
        Assert-True ($script:fixture.ReadinessCalls.Count -eq 0 -and $script:fixture.ShutdownCalls.Count -eq 0) 'Root-gone cleanup attempted readiness or HTTP control.'
        Assert-True ($script:fixture.Polls.Count -gt 0) 'Root-gone child cleanup did not wait for the child to drain.'
    }

    Invoke-TestCase 'accepted-close-timeout-has-no-promotion-path' {
        $case = Set-ValidRunningFixture
        $script:fixture.ServiceReads = @((New-FakeService 101 @(202)))
        Assert-Throws { Wait-DevRuntimeGracefulShutdown $case.Plan (New-ReadService) 90 30 } 'Accepted close that never drains did not time out.'
        Assert-True ($script:fixture.CloseCalls.Count -eq 1) 'The timeout scenario did not issue exactly one synthetic close.'
        Assert-True ($script:fixture.Polls.Count -eq 1 -and $script:fixture.Polls[0] -eq 30) 'The timeout did not use fake 30-second clock advancement.'
    }

    Invoke-TestCase 'http-control-pins-epoch-and-token-without-ui' {
        $case = Set-ValidRunningFixture
        Set-ControlToken 'synthetic-control-token'
        $script:fixture.ShutdownReplies = @([pscustomobject]@{ accepted = $true; instanceEpoch = 'epoch-1' })
        $observed = Wait-DevRuntimeGracefulShutdown $case.Plan (New-ReadService) 30 1
        Assert-ObservedDrain $observed 'HTTP controlled drain'
        Assert-True ($script:fixture.ShutdownCalls.Count -eq 1) 'HTTP control was not called exactly once.'
        Assert-True ($script:fixture.ShutdownCalls[0].Port -eq 11122 -and $script:fixture.ShutdownCalls[0].Token -ceq 'synthetic-control-token') 'HTTP control used the wrong port or token.'
        Assert-True ($script:fixture.ReadinessCalls.Count -eq 1) 'HTTP control did not use the pinned readiness epoch.'
        Assert-True ($script:fixture.CloseCalls.Count -eq 0) 'HTTP control also attempted a UI close.'
        Set-ControlToken $null
    }

    Invoke-TestCase 'already-stopped-has-no-actions' {
        Reset-Fixture
        $script:fixture.ServiceReads = @((New-StoppedService))
        $observed = Wait-DevRuntimeGracefulShutdown (New-TestPlan) (New-ReadService) 0 1
        Assert-True ($observed -is [hashtable] -and $observed.Count -eq 0) 'Already-stopped service did not return an empty observed map.'
        Assert-True ($script:fixture.ProcessReads.Count -eq 0 -and $script:fixture.ReadinessCalls.Count -eq 0 -and $script:fixture.ShutdownCalls.Count -eq 0 -and $script:fixture.CloseCalls.Count -eq 0) 'Already-stopped service performed a shutdown-related action.'
    }

    Invoke-TestCase 'workflow-assignment-wires-drain-before-continuation' {
        $assignment = Get-GracefulWaitAssignment (Join-Path $PSScriptRoot 'dev-restart-workflow.ps1')
        $case = Set-ValidRunningFixture
        $plan = $case.Plan
        $WaitForIdleSeconds = 0
        $PollSeconds = 1
        $script:syntheticContinuationCount = 0
        . $assignment
        $script:syntheticContinuationCount++
        Assert-ObservedDrain $observedProcesses 'Workflow graceful shutdown assignment'
        Assert-True ($script:syntheticContinuationCount -eq 1) 'Successful workflow shutdown did not reach the synthetic continuation marker.'
        Assert-True ($script:fixture.CloseCalls.Count -eq 1) 'Workflow assignment did not request the verified UI close.'
    }

    Invoke-TestCase 'workflow-assignment-stops-on-busy' {
        $assignment = Get-GracefulWaitAssignment (Join-Path $PSScriptRoot 'dev-restart-workflow.ps1')
        $case = Set-ValidRunningFixture
        Set-ControlToken 'synthetic-busy-token'
        $script:fixture.ShutdownReplies = @($null)
        $plan = $case.Plan
        $WaitForIdleSeconds = 0
        $PollSeconds = 1
        $script:syntheticContinuationCount = 0
        $threw = $false
        try { . $assignment; $script:syntheticContinuationCount++ } catch { $threw = $true }
        Assert-True $threw 'Workflow assignment continued after busy control failed to release the runtime.'
        Assert-True ($script:syntheticContinuationCount -eq 0) 'Busy workflow assignment reached the synthetic continuation marker.'
        Assert-True ($script:fixture.CloseCalls.Count -eq 0) 'Busy workflow assignment attempted a UI close.'
        Set-ControlToken $null
    }

    Invoke-TestCase 'workflow-assignment-stops-on-close-timeout' {
        $assignment = Get-GracefulWaitAssignment (Join-Path $PSScriptRoot 'dev-restart-workflow.ps1')
        $case = Set-ValidRunningFixture
        $script:fixture.ServiceReads = @((New-FakeService 101 @(202)))
        $plan = $case.Plan
        $WaitForIdleSeconds = 0
        $PollSeconds = 30
        $script:syntheticContinuationCount = 0
        $threw = $false
        try { . $assignment; $script:syntheticContinuationCount++ } catch { $threw = $true }
        Assert-True $threw 'Workflow assignment continued after an accepted close timed out.'
        Assert-True ($script:syntheticContinuationCount -eq 0) 'Timed-out workflow assignment reached the synthetic continuation marker.'
        Assert-True ($script:fixture.CloseCalls.Count -eq 1) 'Timed-out workflow assignment did not issue its single synthetic close.'
    }

    $result = [ordered]@{ status = 'ok'; assertions = $script:assertionCount; cases = @($script:caseNames) }
    $json = ConvertTo-Json -InputObject $result -Depth 6 -Compress
    if (-not [string]::IsNullOrWhiteSpace($ResultPath)) {
        $outputPath = [IO.Path]::GetFullPath($ResultPath)
        [IO.File]::WriteAllText($outputPath, $json, [Text.UTF8Encoding]::new($false))
    }
    Write-Output $json
} finally {
    if ($script:hadControlToken) { $env:AKU_RUNTIME_CONTROL_TOKEN = $script:previousControlToken }
    else { Remove-Item Env:AKU_RUNTIME_CONTROL_TOKEN -ErrorAction SilentlyContinue }
    if ($script:hadLastExitCode) { $global:LASTEXITCODE = $script:previousLastExitCode }
    elseif (Test-Path Variable:global:LASTEXITCODE) { Remove-Variable -Name LASTEXITCODE -Scope Global -ErrorAction SilentlyContinue }
}

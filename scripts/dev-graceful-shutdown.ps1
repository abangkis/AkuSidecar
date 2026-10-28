# Graceful restart phase only. Loading this file performs no process control.
function Get-DevRuntimeProcess([long] $ProcessId) {
    return Get-CimInstance Win32_Process -Filter "ProcessId=$ProcessId" -ErrorAction Stop
}
function Get-DevRuntimeWindow([long] $ProcessId) {
    return Get-Process -Id $ProcessId -ErrorAction Stop
}
function Get-DevRuntimeUtcNow { return [DateTime]::UtcNow }
function Wait-DevRuntimePoll([int] $Seconds) { Start-Sleep -Seconds $Seconds }
function Get-DevRuntimeReadiness([int] $Port) {
    return Invoke-RestMethod -Uri "http://127.0.0.1:$Port/api/runtime/update-readiness" -TimeoutSec 3
}
function Invoke-DevRuntimeShutdown([int] $Port, [string] $Token) {
    try {
        return Invoke-RestMethod -Uri "http://127.0.0.1:$Port/api/runtime/shutdown-if-idle" -Method Post -ContentType 'application/json' -Body '{}' -Headers @{
            'X-Aku-Runtime-Control-Token' = $Token
        } -TimeoutSec 3
    } catch {
        if ($_.Exception.Response -and [int]$_.Exception.Response.StatusCode -eq 409) { return $null }
        throw 'Runtime control shutdown was not confirmed; candidate retained.'
    }
}
function Get-DevRuntimeUIAppMatches([string] $CommandLine, [int] $Port) {
    $appArguments = [regex]::Matches($CommandLine, '(?:^|\s)(?:"--app=([^"]+)"|--app=(?:"([^"]+)"|(\S+)))')
    if ($appArguments.Count -ne 1) { return $false }
    $value = $appArguments[0].Groups[1].Value + $appArguments[0].Groups[2].Value + $appArguments[0].Groups[3].Value
    $uri = $null
    if (-not [Uri]::TryCreate($value, [UriKind]::Absolute, [ref]$uri)) { return $false }
    # .NET Framework expands IPv6 host text; compare addresses, not formatting.
    $hostName = $uri.Host.Trim([char]91, [char]93)
    $loopback = $hostName -ceq 'localhost'
    $address = $null
    if ([Net.IPAddress]::TryParse($hostName, [ref]$address)) {
        $loopback = $address.Equals([Net.IPAddress]::Loopback) -or $address.Equals([Net.IPAddress]::IPv6Loopback)
    }
    return $uri.Scheme -ceq 'http' -and $loopback -and
        $uri.Port -eq $Port -and $uri.AbsolutePath -ceq '/' -and $uri.UserInfo -eq '' -and $uri.Query -eq ''
}
function Assert-DevRuntimeReadiness($Readiness, [string] $ExpectedEpoch) {
    if ($null -eq $Readiness -or $Readiness.ready -isnot [bool] -or
        $Readiness.instanceEpoch -isnot [string] -or [string]::IsNullOrWhiteSpace($Readiness.instanceEpoch)) {
        throw 'Runtime readiness could not be verified; no shutdown requested.'
    }
    if ($ExpectedEpoch -and $Readiness.instanceEpoch -cne $ExpectedEpoch) {
        throw 'Runtime epoch changed during graceful restart; candidate retained.'
    }
}
function Assert-DevRuntimeRootIdentity($Process, $Identity) {
    if ($null -eq $Process -or $Process.ProcessId -ne $Identity.pid -or
        ([DateTime]$Process.CreationDate).ToUniversalTime().Ticks -ne $Identity.createdTicks -or
        -not ([string]$Process.ExecutablePath).Equals($Identity.image, [StringComparison]::OrdinalIgnoreCase)) {
        throw 'Runtime process identity changed; no close request will be sent.'
    }
}
function Request-DevRuntimeUIShutdown($Plan, $Service, [string] $ExpectedEpoch, $RootIdentity) {
    $root = Get-DevRuntimeProcess $Service.rootPid
    if ($null -eq $root) { return 'root-exited' }
    Assert-DevRuntimeRootIdentity $root $RootIdentity
    $imageArgument = $null
    for ($i = 0; $i -lt $Plan.Arguments.Count; $i++) {
        if ($Plan.Arguments[$i] -eq '--ui-chromium-path' -and $i + 1 -lt $Plan.Arguments.Count) { $imageArgument = $Plan.Arguments[$i + 1] }
        elseif ($Plan.Arguments[$i].StartsWith('--ui-chromium-path=')) { $imageArgument = $Plan.Arguments[$i].Substring('--ui-chromium-path='.Length) }
    }
    if ([string]::IsNullOrWhiteSpace($imageArgument)) { throw 'The registered UI image is missing; no close request will be sent.' }
    $image = if ([IO.Path]::IsPathRooted($imageArgument)) { [IO.Path]::GetFullPath($imageArgument) }
        else { [IO.Path]::GetFullPath((Join-Path $Plan.Service.cwd $imageArgument)) }
    $port = [int]@($Plan.Service.ports)[0]
    $candidates = @()
    foreach ($ownedId in $Service.ownedPids) {
        if ($ownedId -eq $root.ProcessId) { continue }
        $process = Get-DevRuntimeProcess $ownedId
        if ($null -ne $process -and $process.ParentProcessId -eq $root.ProcessId -and
            ([string]$process.ExecutablePath).Equals($image, [StringComparison]::OrdinalIgnoreCase) -and
            (Get-DevRuntimeUIAppMatches ([string]$process.CommandLine) $port)) { $candidates += $process }
    }
    if ($candidates.Count -ne 1) { throw 'Expected exactly one owned AkuBrowser UI; no close request will be sent.' }
    $ui = $candidates[0]
    $created = ([DateTime]$ui.CreationDate).ToUniversalTime().Ticks
    $window = Get-DevRuntimeWindow $ui.ProcessId
    if ($window.Id -ne $ui.ProcessId -or $window.MainWindowHandle -eq [IntPtr]::Zero -or $window.MainWindowTitle -cne 'AkuBrowser') {
        throw 'AkuBrowser UI window could not be verified; no close request will be sent.'
    }
    $handle = $window.MainWindowHandle
    $ready = Get-DevRuntimeReadiness $port
    Assert-DevRuntimeReadiness $ready $ExpectedEpoch
    if (-not $ready.ready) { return 'busy' }
    Assert-DevRuntimeRootIdentity (Get-DevRuntimeProcess $root.ProcessId) $RootIdentity
    $current = Get-DevRuntimeProcess $ui.ProcessId
    if ($null -eq $current -or $current.ParentProcessId -ne $root.ProcessId -or
        ([DateTime]$current.CreationDate).ToUniversalTime().Ticks -ne $created -or
        -not ([string]$current.ExecutablePath).Equals($image, [StringComparison]::OrdinalIgnoreCase) -or
        -not (Get-DevRuntimeUIAppMatches ([string]$current.CommandLine) $port)) {
        throw 'AkuBrowser UI identity changed before close; candidate retained.'
    }
    $window = Get-DevRuntimeWindow $ui.ProcessId
    if ($window.Id -ne $ui.ProcessId -or $window.MainWindowHandle -ne $handle -or $window.MainWindowTitle -cne 'AkuBrowser') {
        throw 'AkuBrowser UI window changed before close; candidate retained.'
    }
    if (-not $window.CloseMainWindow()) { throw 'AkuBrowser did not accept graceful close; no force-stop will run.' }
    return 'requested'
}
function Wait-DevRuntimeGracefulShutdown {
    param($Plan, [scriptblock] $ReadService, [int] $WaitForIdleSeconds, [int] $PollSeconds)
    $deadline = (Get-DevRuntimeUtcNow).AddSeconds($WaitForIdleSeconds)
    $gracefulDeadline = $null
    $observed = @{}
    $rootIdentity = $null
    $epoch = ''
    $requested = $false
    $waitingLogged = $false
    $port = [int]@($Plan.Service.ports)[0]
    while ($true) {
        $service = & $ReadService
        $service = Get-DevRuntimeServiceStatus ([pscustomobject]@{ services = @($service) })
        $owned = @($service.ownedPids)
        if ($null -eq $service.rootPid -and $owned.Count -eq 0) { return $observed }
        foreach ($ownedId in @($owned) + @($service.rootPid | Where-Object { $_ })) {
            $process = Get-DevRuntimeProcess $ownedId
            if ($null -ne $process) { $observed[[string]$ownedId] = $process.CreationDate }
        }
        $root = if ($null -ne $service.rootPid) { Get-DevRuntimeProcess $service.rootPid } else { $null }
        if ($null -ne $root) {
            if ($null -eq $rootIdentity) {
                if (-not ([string]$root.ExecutablePath).Equals($Plan.Target, [StringComparison]::OrdinalIgnoreCase)) {
                    throw 'Registered development image does not own the runtime; no shutdown requested.'
                }
                $rootIdentity = [pscustomobject]@{ pid = $root.ProcessId; createdTicks = ([DateTime]$root.CreationDate).ToUniversalTime().Ticks; image = [string]$root.ExecutablePath }
            }
            Assert-DevRuntimeRootIdentity $root $rootIdentity
        }
        if (-not $requested -and $null -eq $root) {
            $gracefulDeadline = (Get-DevRuntimeUtcNow).AddSeconds(30)
            $requested = $true
            Write-Host 'Runtime root has exited; waiting for owned process cleanup. No force-stop will run.'
        } elseif (-not $requested) {
            $ready = Get-DevRuntimeReadiness $port
            Assert-DevRuntimeReadiness $ready $epoch
            if (-not $epoch) { $epoch = $ready.instanceEpoch }
            if ($ready.ready) {
                if ($env:AKU_RUNTIME_CONTROL_TOKEN) {
                    $reply = Invoke-DevRuntimeShutdown $port $env:AKU_RUNTIME_CONTROL_TOKEN
                    if ($null -ne $reply) {
                        if ($reply.accepted -isnot [bool] -or -not $reply.accepted -or $reply.instanceEpoch -cne $epoch) {
                            throw 'Graceful shutdown acknowledgement did not match the runtime; candidate retained.'
                        }
                        $requested = $true
                    }
                } else {
                    $outcome = Request-DevRuntimeUIShutdown $Plan $service $epoch $rootIdentity
                    if ($outcome -eq 'requested' -or $outcome -eq 'root-exited') { $requested = $true }
                }
                if ($requested) {
                    $gracefulDeadline = (Get-DevRuntimeUtcNow).AddSeconds(30)
                    Write-Host 'Graceful shutdown requested for the verified AkuBrowser runtime. No force-stop will run.'
                }
            }
        }
        if ($gracefulDeadline -and (Get-DevRuntimeUtcNow) -ge $gracefulDeadline) {
            throw 'Graceful shutdown did not release all owned processes; candidate retained, no promotion.'
        }
        if (-not $requested -and (Get-DevRuntimeUtcNow) -ge $deadline) {
            throw 'Runtime did not become idle or accept shutdown; candidate retained, no promotion.'
        }
        if (-not $requested -and -not $waitingLogged) { Write-Host 'Waiting for the verified runtime to become idle before graceful shutdown.'; $waitingLogged = $true }
        Wait-DevRuntimePoll $PollSeconds
    }
}

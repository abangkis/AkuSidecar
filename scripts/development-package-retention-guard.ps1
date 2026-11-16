param([Parameter(Mandatory)] [string] $Repository)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'worker-backup-retention.ps1')
. (Join-Path $PSScriptRoot 'dev-runtime-contract.ps1')
$operation = Enter-DevRuntimeOperation (Join-Path $Repository 'runtime\dev')
function Get-RetentionReferences {
    $all = @(Get-CimInstance Win32_Process -Property ProcessId,ExecutablePath,CommandLine -ErrorAction Stop)
    if ($all.Count -lt 50) { throw 'Native process inventory is incomplete.' }
    $references = @()
    foreach ($process in $all) {
        if ($process.ProcessId -ne $PID) {
            $references += [string]$process.ExecutablePath
            $references += [string]$process.CommandLine
        }
    }
    $config = if ($env:AKU_SUPERVISOR_CONFIG) { $env:AKU_SUPERVISOR_CONFIG } else { Join-Path $env:LOCALAPPDATA 'AkuSupervisor\services.json' }
    if (-not (Test-Path -LiteralPath $config -PathType Leaf)) { throw 'Supervisor configuration unavailable.' }
    $service = (Get-Content -LiteralPath $config -Raw | ConvertFrom-Json).services.akusidecar
    if ($null -eq $service -or -not $service.command) { throw 'AkuSidecar service configuration unavailable.' }
    $references += [string]$service.command
    $references += [string]$service.cwd
    $references += @($service.args | ForEach-Object { [string]$_ })
    foreach ($hive in @([Microsoft.Win32.RegistryHive]::CurrentUser,[Microsoft.Win32.RegistryHive]::LocalMachine)) {
        foreach ($view in @([Microsoft.Win32.RegistryView]::Registry32,[Microsoft.Win32.RegistryView]::Registry64)) {
            $base = [Microsoft.Win32.RegistryKey]::OpenBaseKey($hive,$view)
            try {
                foreach ($vendor in @('Google\Chrome','Chromium')) {
                    $key = $base.OpenSubKey("Software\$vendor\NativeMessagingHosts\com.akubrowser.reader_activation")
                    if ($null -ne $key) {
                        try {
                            $manifest = [string]$key.GetValue('')
                            $references += $manifest
                            if ($manifest -and $manifest.IndexOf($Repository,[StringComparison]::OrdinalIgnoreCase) -ge 0) {
                                if (-not (Test-Path -LiteralPath $manifest -PathType Leaf)) { throw 'Registered package manifest unavailable.' }
                                $references += [string]((Get-Content -LiteralPath $manifest -Raw | ConvertFrom-Json).path)
                            }
                        } finally { $key.Dispose() }
                    }
                }
            } finally { $base.Dispose() }
        }
    }
    return [pscustomobject]@{event='snapshot';complete=$true;processCount=$all.Count;references=$references}
}
try {
    [Console]::Out.WriteLine('{"event":"ready"}')
    while ($null -ne ($message = [Console]::In.ReadLine())) {
        if ($message -eq 'release') { break }
        if ($message.StartsWith('{')) {
            $request = $message | ConvertFrom-Json
            if ($request.event -ne 'preflight' -or $request.files.Count -lt 1 -or $request.files.Count -gt 3) { throw 'Unexpected preflight request.' }
            $handles = [Collections.Generic.List[IO.FileStream]]::new()
            try {
                foreach ($file in $request.files) {
                    $full = [IO.Path]::GetFullPath([string]$file)
                    $boundary = [IO.Path]::GetFullPath((Join-Path $Repository 'build')) + [IO.Path]::DirectorySeparatorChar
                    if (-not $full.StartsWith($boundary,[StringComparison]::OrdinalIgnoreCase) -or [IO.Path]::GetExtension($full) -ne '.exe') { throw 'Preflight path is outside the package build root.' }
                    $handles.Add([IO.File]::Open($full,[IO.FileMode]::Open,[IO.FileAccess]::Read,[IO.FileShare]::None))
                }
            } finally { foreach ($handle in $handles) { $handle.Dispose() } }
            [Console]::Out.WriteLine('{"event":"preflight","complete":true}')
            continue
        }
        if ($message -ne 'snapshot') { throw 'Unexpected retention guard command.' }
        [Console]::Out.WriteLine((Get-RetentionReferences | ConvertTo-Json -Depth 5 -Compress))
    }
} finally {
    try { $operation.ReleaseMutex() } finally { $operation.Dispose() }
}


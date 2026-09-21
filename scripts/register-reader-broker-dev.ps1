param(
    [string] $RuntimeDirectory = (Join-Path (Split-Path -Parent $PSScriptRoot) 'runtime\dev'),
    [switch] $Register,
    [switch] $ReplaceExistingRegistration
)
$ErrorActionPreference = 'Stop'
$runtimePath = [IO.Path]::GetFullPath($RuntimeDirectory)
$helper = Join-Path $runtimePath 'aku-reader-broker.exe'
$extension = Join-Path $runtimePath 'ui-reader-broker\manifest.json'
if (-not (Test-Path -LiteralPath $helper -PathType Leaf) -or -not (Test-Path -LiteralPath $extension -PathType Leaf)) {
    throw 'Build the reader broker helper and UI extension before registration.'
}
$manifestPath = Join-Path $runtimePath 'com.akubrowser.reader_activation.json'
$manifest = [ordered]@{
    name = 'com.akubrowser.reader_activation'
    description = 'Explicit Windows AkuBrowser reader activation'
    path = 'aku-reader-broker.exe'
    type = 'stdio'
    allowed_origins = @('chrome-extension://dlibmmlopdahibfniinemhnghlifiple/')
}
[IO.File]::WriteAllText($manifestPath, ($manifest | ConvertTo-Json -Depth 4), [Text.UTF8Encoding]::new($false))
if ($Register) {
    # Preflight both vendors before any registry write. Switching an installed
    # host to a dev host is an explicit operation, never a build side effect.
    foreach ($vendor in @('Chromium', 'Google\Chrome')) {
        $key = "HKCU:\Software\$vendor\NativeMessagingHosts\com.akubrowser.reader_activation"
        if (Test-Path -LiteralPath $key) {
            $current = [string](Get-Item -LiteralPath $key).GetValue('')
            if ($current -and -not $current.Equals($manifestPath, [StringComparison]::OrdinalIgnoreCase) -and -not $ReplaceExistingRegistration) {
                throw "A different runtime owns the reader broker registration: $current. Use -ReplaceExistingRegistration only for an explicitly approved runtime switch."
            }
        }
    }
    foreach ($vendor in @('Chromium', 'Google\Chrome')) {
        $key = "HKCU:\Software\$vendor\NativeMessagingHosts\com.akubrowser.reader_activation"
        New-Item -Path $key -Force | Out-Null
        Set-Item -LiteralPath $key -Value $manifestPath
        if ((Get-Item -LiteralPath $key).GetValue('') -ne $manifestPath) { throw "Native host registration readback failed: $key" }
    }
    Write-Host 'Registered the explicit reader broker for this development runtime. No browser/profile restart or modification was performed.'
} else {
    Write-Host "Prepared native-host manifest: $manifestPath (registry unchanged; use -Register only when authorized)"
}

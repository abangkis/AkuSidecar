param([Parameter(Mandatory=$true)][string]$ReceiptPath,[switch]$Apply,[switch]$Cleanup)
$ErrorActionPreference = 'Stop'
if ($Apply -and $Cleanup) { throw 'Apply and cleanup are separate operations.' }
$project = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$qaParent = Join-Path $project 'runtime\qa\auth-session-sidecar'
$receiptFile = (Resolve-Path -LiteralPath $ReceiptPath).ProviderPath
$receipt = Get-Content -LiteralPath $receiptFile -Raw | ConvertFrom-Json
$scope = (Resolve-Path -LiteralPath $receipt.TaskRoot).ProviderPath
if ([IO.Path]::GetDirectoryName($scope) -ne $qaParent -or ![IO.Path]::GetFileName($scope).StartsWith('run-') -or [IO.Path]::GetDirectoryName($receiptFile) -ne $scope -or ((Get-Item -LiteralPath $scope).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'QA registration receipt escaped exact task scope.' }
if ($receipt.HostName -notmatch '^com\.akubrowser\.qa_reader_[a-f0-9]{32}$' -or $receipt.RegistryView -ne 'Registry64') { throw 'QA host identity/view rejected.' }
$parentKey = 'Software\Google\Chrome\NativeMessagingHosts'
$leaf = $parentKey + '\' + $receipt.HostName
if ($receipt.RegistryPath -ne ('HKCU\' + $leaf) -or $receipt.DefaultValue -ne $receipt.ManifestPath) { throw 'QA registry target rejected.' }
$files = @(@($receipt.ManifestPath,$receipt.ManifestSHA256),@($receipt.HelperPath,$receipt.HelperSHA256),@($receipt.MainPath,$receipt.MainSHA256))
foreach ($file in $files) {
    $path = (Resolve-Path -LiteralPath $file[0]).ProviderPath
    if ([IO.Path]::GetDirectoryName($path) -ne $scope -or ((Get-Item -LiteralPath $path).Attributes -band [IO.FileAttributes]::ReparsePoint) -or (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant() -ne $file[1]) { throw 'QA manifest/binary ownership hash mismatch.' }
}
if ([IO.Path]::GetFileName($receipt.ManifestPath) -ne ($receipt.HostName + '.json') -or [IO.Path]::GetFileName($receipt.HelperPath) -ne 'aku-reader-broker.exe' -or [IO.Path]::GetFileName($receipt.MainPath) -ne 'akusidecar.exe' -or $receipt.Pipe -notmatch '^\\\\\.\\pipe\\AkuBrowser\.fixture-reader\.[a-f0-9]{32}$') { throw 'QA executable/private transport identity rejected.' }
$manifest = Get-Content -LiteralPath $receipt.ManifestPath -Raw | ConvertFrom-Json
if ($manifest.name -ne $receipt.HostName -or $manifest.path -ne $receipt.HelperPath -or $manifest.type -ne 'stdio' -or !$manifest.description -or @($manifest.allowed_origins).Count -ne 1 -or $manifest.allowed_origins[0] -ne 'chrome-extension://dlibmmlopdahibfniinemhnghlifiple/') { throw 'QA native manifest contract rejected.' }
$ownershipPath = Join-Path $scope 'qa-host-applied.json'
$savedTemp = $env:TEMP
$savedTmp = $env:TMP
$env:TEMP = $scope
$env:TMP = $scope
try {
function Initialize-QARegistryNative {
    if ('AkuQAReaderHostRegistry' -as [type]) { return }
    Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class AkuQAReaderHostRegistry {
 [DllImport("advapi32.dll",CharSet=CharSet.Unicode)] public static extern int RegCreateKeyEx(IntPtr key,string name,int reserved,string cls,int options,int access,IntPtr security,out IntPtr result,out uint disposition);
 [DllImport("advapi32.dll")] public static extern int RegCloseKey(IntPtr key);
 [DllImport("ntdll.dll")] static extern int NtQueryKey(IntPtr key,int info,IntPtr data,int length,out int required);
 static IntPtr Info(IntPtr key,int type) {int n;NtQueryKey(key,type,IntPtr.Zero,0,out n);if(n<4||n>65536)throw new InvalidOperationException("QA key identity unavailable");IntPtr p=Marshal.AllocHGlobal(n);if(NtQueryKey(key,type,p,n,out n)!=0){Marshal.FreeHGlobal(p);throw new InvalidOperationException("QA key identity failed");}return p;}
 public static string Name(IntPtr key) {IntPtr p=Info(key,3);try{return Marshal.PtrToStringUni(IntPtr.Add(p,4),Marshal.ReadInt32(p)/2);}finally{Marshal.FreeHGlobal(p);}}
 public static string LastWrite(IntPtr key) {IntPtr p=Info(key,0);try{return Marshal.ReadInt64(p).ToString("x16");}finally{Marshal.FreeHGlobal(p);}}
}
'@
}
function Get-QAKeyIdentity($key) {
    Initialize-QARegistryNative
    $name = [AkuQAReaderHostRegistry]::Name($key.Handle.DangerousGetHandle()).ToLowerInvariant()
    $sha = [Security.Cryptography.SHA256]::Create()
    try { $hash = [Convert]::ToHexString($sha.ComputeHash([Text.Encoding]::UTF8.GetBytes($name))).ToLowerInvariant() } finally { $sha.Dispose() }
    return @{CanonicalKeySHA256=$hash;LastWrite=[AkuQAReaderHostRegistry]::LastWrite($key.Handle.DangerousGetHandle())}
}
function Assert-QATreeEmpty {
    $peers = @(Get-CimInstance Win32_Process | Where-Object { $_.ProcessId -ne $PID -and (($_.ExecutablePath -and $_.ExecutablePath.StartsWith($scope + '\',[StringComparison]::OrdinalIgnoreCase)) -or ($_.CommandLine -and $_.CommandLine.IndexOf($scope + '\',[StringComparison]::OrdinalIgnoreCase) -ge 0)) })
    if ($peers.Count -ne 0) { throw 'QA processes still live; registry operation refused.' }
}
function Assert-OwnedLeaf($key) {
    if ($key.GetSubKeyNames().Count -ne 0 -or $key.GetValueNames().Count -ne 1 -or $key.GetValueNames()[0] -ne '' -or $key.GetValueKind('') -ne [Microsoft.Win32.RegistryValueKind]::String -or $key.GetValue('') -ne $receipt.ManifestPath) { throw 'QA leaf contents changed; retained for review.' }
}
if ($Cleanup) {
    if (!(Test-Path -LiteralPath $ownershipPath -PathType Leaf)) { throw 'QA applied ownership receipt missing; cleanup refused.' }
    $owned = Get-Content -LiteralPath $ownershipPath -Raw | ConvertFrom-Json
    if (!$owned.CreatedNewLeaf -or $owned.RegistryPath -ne $receipt.RegistryPath -or $owned.ManifestSHA256 -ne $receipt.ManifestSHA256 -or $owned.DefaultValue -ne $receipt.ManifestPath) { throw 'QA cleanup ownership receipt mismatch.' }
    Assert-QATreeEmpty
    $base = [Microsoft.Win32.RegistryKey]::OpenBaseKey([Microsoft.Win32.RegistryHive]::CurrentUser,[Microsoft.Win32.RegistryView]::Registry64)
    try {
        $parent = $base.OpenSubKey($parentKey,$true)
        if (!$parent) { throw 'Existing registry parent unavailable; cleanup refused.' }
        try {
            $key = $parent.OpenSubKey($receipt.HostName,$false)
            if (!$key) { throw 'QA leaf missing; cleanup ownership cannot be verified.' }
            try {
                Assert-OwnedLeaf $key
                $identity = Get-QAKeyIdentity $key
                if ($identity.CanonicalKeySHA256 -ne $owned.CanonicalKeySHA256 -or $identity.LastWrite -ne $owned.LastWrite) { throw 'QA key identity/write epoch changed; retained for review.' }
            } finally { $key.Dispose() }
            $parent.DeleteSubKey($receipt.HostName,$false)
            $remaining = $parent.OpenSubKey($receipt.HostName,$false)
            if ($remaining) { $remaining.Dispose(); throw 'QA leaf cleanup readback failed.' }
        } finally { $parent.Dispose() }
    } finally { $base.Dispose() }
    $owned.Stage = 'REMOVED'
    [IO.File]::WriteAllText($ownershipPath,($owned | ConvertTo-Json -Depth 5),[Text.UTF8Encoding]::new($false))
    Write-Output ('QA host removed: exact_leaf=' + $receipt.RegistryPath + ' trees_empty=true production_keys_unchanged=true')
    return
}
# Collision reads are confined to this newly generated QA hostname. No original
# host is inspected, replaced, or used as fallback.
foreach ($hive in @([Microsoft.Win32.RegistryHive]::CurrentUser,[Microsoft.Win32.RegistryHive]::LocalMachine)) {
    foreach ($view in @([Microsoft.Win32.RegistryView]::Registry32,[Microsoft.Win32.RegistryView]::Registry64)) {
        $base = [Microsoft.Win32.RegistryKey]::OpenBaseKey($hive,$view)
        try {
            $existing = $base.OpenSubKey($leaf,$false)
            if ($existing) { $existing.Dispose(); throw 'QA hostname collision; existing leaf was not modified.' }
        } finally { $base.Dispose() }
    }
}
$base = [Microsoft.Win32.RegistryKey]::OpenBaseKey([Microsoft.Win32.RegistryHive]::CurrentUser,[Microsoft.Win32.RegistryView]::Registry64)
try {
    $parent = $base.OpenSubKey($parentKey,$Apply.IsPresent)
    if (!$parent) { throw 'Required existing parent is absent; ancestors will not be created.' }
    try {
        if (!$Apply) {
            $identity = Get-QAKeyIdentity $parent
            if (!$identity.CanonicalKeySHA256 -or !$identity.LastWrite) { throw 'Read-only registry interop preflight failed.' }
            Write-Output ('PREVIEW_ONLY exact_leaf=' + $receipt.RegistryPath + ' manifest=' + $receipt.ManifestPath + ' manifest_sha256=' + $receipt.ManifestSHA256 + ' helper_sha256=' + $receipt.HelperSHA256 + ' main_sha256=' + $receipt.MainSHA256 + ' Registry64=true collision_checks=4 private_transport_configured=true registry_written=false')
            return
        }
        Assert-QATreeEmpty
        if (Test-Path -LiteralPath $ownershipPath) { throw 'Ownership receipt already exists; registration is not replayable.' }
        Initialize-QARegistryNative
        $handle = [IntPtr]::Zero
        $disposition = [uint32]0
        $result = [AkuQAReaderHostRegistry]::RegCreateKeyEx($parent.Handle.DangerousGetHandle(),$receipt.HostName,0,$null,0,0x2011f,[IntPtr]::Zero,[ref]$handle,[ref]$disposition)
        if ($result -ne 0 -or $disposition -ne 1) {
            if ($handle -ne [IntPtr]::Zero) { [void][AkuQAReaderHostRegistry]::RegCloseKey($handle) }
            throw 'Atomic new-leaf creation failed/collided; no existing value was written.'
        }
        # Preserve the exact freshly created handle through value write/readback;
        # never reopen by name and accidentally write a replacement key.
        $safe = [Microsoft.Win32.SafeHandles.SafeRegistryHandle]::new($handle,$true)
        $key = [Microsoft.Win32.RegistryKey]::FromHandle($safe,[Microsoft.Win32.RegistryView]::Registry64)
        $owned = [ordered]@{Stage='CREATED';CreatedNewLeaf=$true;RegistryPath=$receipt.RegistryPath;RegistryView='Registry64';ManifestSHA256=$receipt.ManifestSHA256;DefaultValue=$receipt.ManifestPath;AbsentBefore=@('HKCU/Registry32','HKCU/Registry64','HKLM/Registry32','HKLM/Registry64');CanonicalKeySHA256='';LastWrite='';HKCU32Alias=$false}
        try {
            [IO.File]::WriteAllText($ownershipPath,($owned | ConvertTo-Json -Depth 5),[Text.UTF8Encoding]::new($false))
            $key.SetValue('',$receipt.ManifestPath,[Microsoft.Win32.RegistryValueKind]::String)
            Assert-OwnedLeaf $key
            $identity = Get-QAKeyIdentity $key
            $owned.CanonicalKeySHA256 = $identity.CanonicalKeySHA256
            $owned.LastWrite = $identity.LastWrite
            $base32 = [Microsoft.Win32.RegistryKey]::OpenBaseKey([Microsoft.Win32.RegistryHive]::CurrentUser,[Microsoft.Win32.RegistryView]::Registry32)
            try {
                $alias = $base32.OpenSubKey($leaf,$false)
                if ($alias) { try {
                    Assert-OwnedLeaf $alias
                    $aliasIdentity = Get-QAKeyIdentity $alias
                    if ($aliasIdentity.CanonicalKeySHA256 -ne $identity.CanonicalKeySHA256 -or $aliasIdentity.LastWrite -ne $identity.LastWrite) { throw 'HKCU32 is a separate registration; no alias ownership was inferred.' }
                    $owned.HKCU32Alias = $true
                } finally { $alias.Dispose() } }
            } finally { $base32.Dispose() }
        } finally { if ($key) { $key.Dispose() } }
        $owned.Stage = 'APPLIED'
        [IO.File]::WriteAllText($ownershipPath,($owned | ConvertTo-Json -Depth 5),[Text.UTF8Encoding]::new($false))
        Write-Output ('QA host applied: exact_leaf=' + $receipt.RegistryPath + ' Registry64=true created_new=true readback_verified=true production_keys_unchanged=true')
    } finally { $parent.Dispose() }
} finally { $base.Dispose() }
} finally { $env:TEMP = $savedTemp; $env:TMP = $savedTmp }

param()
$ErrorActionPreference='Stop'
# Read-only snapshot. No pipe connection, registry mutation, process control,
# profile inspection, database access or HTTP request is performed.
$rows=@()
foreach($hive in @('CurrentUser','LocalMachine')) {
 foreach($view in @('Registry64','Registry32')) {
  foreach($vendor in @('Google\Chrome','Chromium')) {
   $base=$null; $key=$null
   try {
    $base=[Microsoft.Win32.RegistryKey]::OpenBaseKey([Microsoft.Win32.RegistryHive]$hive,[Microsoft.Win32.RegistryView]$view)
    $name="Software\$vendor\NativeMessagingHosts\com.akubrowser.reader_activation"
    $key=$base.OpenSubKey($name,$false)
    $present=$null -ne $key
    $hasDefault=$present -and ($key.GetValueNames() -contains '')
    $value=$null; $kind=$null
    if($hasDefault){$kind=[string]$key.GetValueKind('');$value=$key.GetValue('', $null, [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)}
    $rows+=@{hive=$hive;view=$view;key=$name;status='observed';keyPresent=$present;defaultPresent=$hasDefault;kind=$kind;value=$value}
   } catch {$rows+=@{hive=$hive;view=$view;vendor=$vendor;status='unavailable'}}
   finally {if($null -ne $key){$key.Dispose()};if($null -ne $base){$base.Dispose()}}
  }
 }
}
$pipe='unknown'
try {$pipe=if([IO.Directory]::GetFiles('\\.\pipe\') -contains '\\.\pipe\AkuBrowser.reader-activation.11122'){'present'}else{'absent'}}catch{}
@{schema='aku.x.daily-native-preflight.v1';observedAtUtc=[DateTime]::UtcNow.ToString('o');registrations=$rows;
 readerPipe=@{name='AkuBrowser.reader-activation.11122';presence=$pipe;owner='unverified';connected=$false};
 executionApproved=$false;runtimeChanged=$false} | ConvertTo-Json -Depth 8

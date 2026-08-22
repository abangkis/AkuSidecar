param(
    [ValidateSet('stable', 'beta', 'dev')]
    [string] $Channel = 'stable',
    [ValidateSet('win64', 'mac-arm64', 'mac-x64', 'linux64')]
    [string] $Platform = 'win64',
    [string] $OutputDir = 'runtime\chromium'
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
if (-not ([IO.Path]::IsPathRooted($OutputDir))) {
    $OutputDir = Join-Path $repoRoot $OutputDir
}
$binDir = Join-Path $OutputDir 'bin'
$pinPath = Join-Path $OutputDir 'pin.json'

if (Test-Path -LiteralPath $pinPath) {
    Write-Host "Pinned Chromium already present: $pinPath"
    Write-Host 'Delete runtime\chromium to re-download.'
    exit 0
}

$manifestUrl = 'https://googlechromelabs.github.io/chrome-for-testing/last-known-good-versions-with-downloads.json'
Write-Host "Fetching Chrome-for-Testing manifest ($Channel)..."
$manifest = Invoke-RestMethod -Uri $manifestUrl -TimeoutSec 60
$channelData = $manifest.channels.$Channel
if ($null -eq $channelData) {
    throw "Channel '$Channel' was not found in the Chrome-for-Testing manifest."
}

$download = $channelData.downloads.chrome |
    Where-Object { $_.platform -eq $Platform } |
    Select-Object -First 1
if ($null -eq $download) {
    throw "No chrome download for platform '$Platform' in channel '$Channel'."
}

$version = $channelData.version
$archivePath = Join-Path $env:TEMP ("chrome-for-testing-$version-$Platform" + [IO.Path]::GetExtension($download.url))
Write-Host "Downloading Chromium $version for $Platform..."
if (Get-Command curl.exe -ErrorAction SilentlyContinue) {
    & curl.exe -L --fail --retry 3 -C - -sS -o $archivePath $download.url
    if ($LASTEXITCODE -ne 0) {
        throw "Download failed with curl exit code $LASTEXITCODE."
    }
} else {
    $ProgressPreference = 'SilentlyContinue'
    Invoke-WebRequest -Uri $download.url -OutFile $archivePath -TimeoutSec 1800
}

New-Item -ItemType Directory -Path $binDir -Force | Out-Null
Write-Host "Extracting to $binDir..."
Expand-Archive -LiteralPath $archivePath -DestinationPath $OutputDir -Force

$extractedRoot = Get-ChildItem -LiteralPath $OutputDir -Directory |
    Where-Object { $_.Name -like 'chrome-*' } |
    Select-Object -First 1
if ($null -eq $extractedRoot) {
    throw 'Extraction did not produce a chrome-* directory.'
}
if ($Platform -like 'win*') {
    $executableName = 'chrome.exe'
    $executableRelPath = 'bin\chrome.exe'
} else {
    $executableName = 'chrome'
    $executableRelPath = 'bin/chrome'
}
$innerBin = Join-Path $extractedRoot.FullName $executableName
if (-not (Test-Path -LiteralPath $innerBin)) {
    throw "Expected executable was not found after extraction: $innerBin"
}

Move-Item -LiteralPath $extractedRoot.FullName -Destination (Join-Path $OutputDir 'staging') -Force
Remove-Item -LiteralPath $binDir -Recurse -Force
Move-Item -LiteralPath (Join-Path $OutputDir 'staging') -Destination $binDir -Force
Remove-Item -LiteralPath $archivePath -Force -ErrorAction SilentlyContinue

$hash = (Get-FileHash -LiteralPath (Join-Path $binDir $executableName) -Algorithm SHA256).Hash.ToLowerInvariant()
$pin = [ordered]@{
    schemaVersion    = 1
    component        = 'AkuBrowserAppShellChromium'
    channel          = $Channel
    platform         = $Platform
    version          = $version
    sourceUrl        = $download.url
    executable       = $executableRelPath
    executableSha256 = $hash
    fetchedAtUtc     = [DateTime]::UtcNow.ToString('o')
}
[IO.File]::WriteAllText(
    $pinPath,
    ($pin | ConvertTo-Json -Depth 4),
    [Text.UTF8Encoding]::new($false)
)

Write-Host "Pinned Chromium $version installed: $binDir"
Write-Host "Recorded pin provenance: $pinPath"

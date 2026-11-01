function Get-SourceBuildCachePaths {
    [CmdletBinding()]
    param([Parameter(Mandatory)][string] $Repository)

    $repositoryPath = [IO.Path]::GetFullPath($Repository)
    $configPath = Join-Path $repositoryPath '.cache\source-build-cache.json'
    $cacheRoot = $null
    if (Test-Path -LiteralPath $configPath -PathType Leaf) {
        try { $configuration = Get-Content -LiteralPath $configPath -Raw | ConvertFrom-Json -ErrorAction Stop }
        catch { throw "Source cache configuration is invalid JSON: $configPath" }
        if ($configuration.schemaVersion -ne 1) { throw "Source cache configuration schemaVersion must be 1: $configPath" }
        $configuredRoot = [string]$configuration.root
        $absoluteRoot = $configuredRoot -match '^[A-Za-z]:[\\/]' -or $configuredRoot -match '^\\\\[^\\]+\\[^\\]+'
        if ([string]::IsNullOrWhiteSpace($configuredRoot) -or -not $absoluteRoot) {
            throw "Source cache configuration root must be a nonempty absolute path: $configPath"
        }
        try { $cacheRoot = [IO.Path]::GetFullPath($configuredRoot) }
        catch { throw "Source cache configuration root is not a valid absolute path: $configPath" }
    } else {
        $cursor = [IO.DirectoryInfo]::new($repositoryPath)
        $sharedTempRoot = $null
        while ($null -ne $cursor) {
            $candidate = Join-Path $cursor.FullName 'SharedTemp'
            if (Test-Path -LiteralPath $candidate -PathType Container) {
                $sharedTempRoot = [IO.Path]::GetFullPath($candidate)
                break
            }
            $cursor = $cursor.Parent
        }
        if (-not $sharedTempRoot) {
            $sharedTempRoot = [IO.Path]::GetFullPath((Join-Path $repositoryPath 'SharedTemp'))
        }
        $cacheRoot = Join-Path $sharedTempRoot 'build-cache'
    }

    $goBuild = [Environment]::GetEnvironmentVariable('GOCACHE', 'Process')
    $goModules = [Environment]::GetEnvironmentVariable('GOMODCACHE', 'Process')
    $npm = [Environment]::GetEnvironmentVariable('npm_config_cache', 'Process')
    if ([string]::IsNullOrWhiteSpace($goBuild)) { $goBuild = Join-Path $cacheRoot 'go-build' }
    if ([string]::IsNullOrWhiteSpace($goModules)) { $goModules = Join-Path $cacheRoot 'go-mod' }
    if ([string]::IsNullOrWhiteSpace($npm)) { $npm = Join-Path $cacheRoot 'npm' }

    [pscustomobject]@{
        GoBuild = $goBuild
        GoModules = $goModules
        Npm = $npm
    }
}
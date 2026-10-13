$ErrorActionPreference = 'Stop'

try {
    Add-Type -AssemblyName System.Security | Out-Null
    $requestText = [Console]::In.ReadToEnd()
    $request = ConvertFrom-Json -InputObject $requestText -ErrorAction Stop
    if ($request.operation -notin @('protect', 'unprotect') -or
        $request.data -isnot [string] -or $request.data.Length -gt 350000) {
        throw 'INVALID_REQUEST'
    }

    $inputBytes = [Convert]::FromBase64String($request.data)
    $entropy = [Text.Encoding]::UTF8.GetBytes('AkuSidecar auth-session-fixture vault v1')
    if ($request.operation -eq 'protect') {
        $outputBytes = [Security.Cryptography.ProtectedData]::Protect(
            $inputBytes,
            $entropy,
            [Security.Cryptography.DataProtectionScope]::CurrentUser
        )
    }
    else {
        $outputBytes = [Security.Cryptography.ProtectedData]::Unprotect(
            $inputBytes,
            $entropy,
            [Security.Cryptography.DataProtectionScope]::CurrentUser
        )
    }

    [Console]::Out.Write([Convert]::ToBase64String($outputBytes))
    exit 0
}
catch {
    [Console]::Error.WriteLine('DPAPI_FAILED')
    exit 1
}

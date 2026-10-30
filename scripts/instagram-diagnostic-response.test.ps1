$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'instagram-diagnostic-response.ps1')
$actual = ConvertTo-InstagramDiagnosticApiFailure -Status 409 -Body '{"error":"instagram_diagnostic_busy","message":"PRIVATE_ACCOUNT","details":"PRIVATE_TOKEN"}'
if ($actual.code -cne 'instagram_diagnostic_busy' -or $actual.status -ne 409) { throw 'Flat API code lost' }
foreach ($body in @('{"error":{"code":"instagram_diagnostic_busy"}}','{"error":"PRIVATE_TOKEN"}','{"error":"instagram_diagnostic_busy PRIVATE"}','{"error":null}','PRIVATE', ('x' * 4097))) {
    $value = ConvertTo-InstagramDiagnosticApiFailure -Status 409 -Body $body
    if ($value.code -cne 'request_failed' -or ($value | ConvertTo-Json -Compress).Contains('PRIVATE')) { throw 'Untrusted error leaked' }
}
if ((ConvertTo-InstagramDiagnosticApiFailure -Status 0 -Body '{}').status -ne 0) { throw 'Invalid HTTP status accepted' }
if (($actual | ConvertTo-Json -Compress).Contains('PRIVATE')) { throw 'Private message leaked' }
$record = [Management.Automation.ErrorRecord]::new([Exception]::new('PRIVATE_EXCEPTION'), 'synthetic', [Management.Automation.ErrorCategory]::NotSpecified, $null)
$record.ErrorDetails = [Management.Automation.ErrorDetails]::new('{"error":"instagram_diagnostic_diagnostic_rejected","message":"PRIVATE"}')
$projected = ConvertFrom-InstagramDiagnosticException -Failure $record
if ($projected.code -cne 'instagram_diagnostic_diagnostic_rejected' -or ($projected | ConvertTo-Json -Compress).Contains('PRIVATE')) { throw 'Exception projection lost flat code or leaked data' }
Write-Output 'PASS: flat API code retained; unknown/private/oversized bodies excluded.'

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

$catalog = Get-Content -LiteralPath (Join-Path $PSScriptRoot '../internal/igdiagnostic/catalog.json') -Raw | ConvertFrom-Json
$evidence = [ordered]@{}
foreach ($key in $catalog.checks) { $evidence[$key] = 'not_observed' }
$trace = [ordered]@{
    schema = 'aku.ig-trace.v1'; stage = 'dispatch'; completed = 'none'; disposition = 'not_started'
    sequence = 2; cause = @{ stage = 'dispatch'; condition = 'public_pending' }
    cleanup = @{ state = 'not_started'; condition = 'none' }; complete = $true; elapsedMS = 2
    attempt = ('a' * 32); operation = 'reader'; revision = 'ig1'
    delivery = @{ state = 'pending'; condition = 'none' }; evidence = $evidence
    httpDelivery = @{ state = 'pending'; condition = 'none' }
    nodeElapsedMS = 1; nodeSequence = 1
    budgetMS = 30000; remainingMS = 29999
}
$body = @{ error = 'instagram_diagnostic_diagnostic_rejected'; message = 'PRIVATE_EXCEPTION'; details = @{ diagnostic = $trace } } | ConvertTo-Json -Depth 9 -Compress
$value = ConvertTo-InstagramDiagnosticApiFailure -Status 409 -Body $body
if ($null -eq $value.diagnostic -or $value.diagnostic.cause.condition -cne 'public_pending' -or $value.diagnostic.evidence.cookieMatched -cne 'not_observed' -or $value.retried -or ($value | ConvertTo-Json -Depth 9 -Compress).Contains('PRIVATE')) { throw 'Safe trace lost or private data exposed' }
foreach ($mode in @('field','cause','condition','number','missing','evidence','large_budget','zero_budget','missing_remaining','missing_budget','remaining_over_budget')) {
    $invalid = $trace | ConvertTo-Json -Depth 9 -Compress | ConvertFrom-Json
    switch ($mode) {
        'field' { $invalid | Add-Member -NotePropertyName token -NotePropertyValue 'PRIVATE_TOKEN' }
        'cause' { $invalid.cause | Add-Member -NotePropertyName message -NotePropertyValue 'PRIVATE_EXCEPTION' }
        'condition' { $invalid.cause.condition = 'PRIVATE_ACCOUNT' }
        'number' { $invalid.elapsedMS = -1 }
        'missing' { $invalid.PSObject.Properties.Remove('complete') }
        'evidence' { $invalid.evidence.cookieMatched = 'PRIVATE_COOKIE' }
        'large_budget' { $invalid.budgetMS = 60000; $invalid.remainingMS = 59000 }
        'zero_budget' { $invalid.budgetMS = 0; $invalid.remainingMS = 0 }
        'missing_remaining' { $invalid.remainingMS = $null }
        'missing_budget' { $invalid.budgetMS = $null }
        'remaining_over_budget' { $invalid.remainingMS = 30001 }
    }
    $badBody = @{ error = 'instagram_diagnostic_diagnostic_rejected'; details = @{ diagnostic = $invalid } } | ConvertTo-Json -Depth 9 -Compress
    $projected = ConvertTo-InstagramDiagnosticApiFailure -Status 409 -Body $badBody
    if ($null -ne $projected.diagnostic -or -not $projected.diagnosticIncomplete -or $projected.code -cne 'instagram_diagnostic_diagnostic_rejected' -or ($projected | ConvertTo-Json -Depth 9 -Compress).Contains('PRIVATE')) { throw "Invalid diagnostic accepted: $mode" }
}
Write-Output 'PASS: safe trace retained; malformed metadata excluded without replacing API code.'
foreach ($condition in @('read_command_failed','read_command_timeout','read_transport_closed','read_cdp_rejected','read_reply_invalid','read_empty_nonfinal')) {
    $trace.stage = 'body_read'
    $trace.cause = @{ stage = 'body_read'; condition = $condition }
    $body = @{ error = 'instagram_diagnostic_diagnostic_rejected'; details = @{ diagnostic = $trace } } | ConvertTo-Json -Depth 9 -Compress
    $projected = ConvertTo-InstagramDiagnosticApiFailure -Status 409 -Body $body
    if ($null -eq $projected.diagnostic -or $projected.diagnostic.cause.condition -cne $condition -or $projected.retried) { throw 'Read failure classification lost in operator projection' }
}
Write-Output 'PASS: all closed stream-read categories retained without retry.'
foreach ($stage in @('parsing','cleanup','handoff','cookie','post_check')) {
    $trace.stage = $stage
    $trace.cause = @{ stage = $stage; condition = 'document_within_navigation' }
    $body = @{ error = 'instagram_diagnostic_diagnostic_rejected'; message = 'PRIVATE_EXCEPTION'; details = @{ diagnostic = $trace } } | ConvertTo-Json -Depth 9 -Compress
    $projected = ConvertTo-InstagramDiagnosticApiFailure -Status 409 -Body $body
    if ($null -eq $projected.diagnostic -or $projected.diagnostic.stage -cne $stage -or $projected.diagnostic.cause.stage -cne $stage -or $projected.diagnostic.cause.condition -cne 'document_within_navigation' -or $projected.retried -or ($projected | ConvertTo-Json -Depth 9 -Compress).Contains('PRIVATE')) { throw 'Document observation stage lost or unsafe in operator projection' }
}
Write-Output 'PASS: bounded document observation stages retained without private data or retry.'
$trace.stage = 'cleanup'
$trace.sequence = 10000
$trace.nodeSequence = 10000
$trace.complete = $false
$trace.cause = @{ stage = 'root_delivery'; condition = 'root_request_duplicate' }
$trace.cleanup = @{ state = 'verified'; condition = 'none' }
$body = @{ error = 'instagram_diagnostic_diagnostic_rejected'; message = 'PRIVATE_EXCEPTION'; details = @{ diagnostic = $trace } } | ConvertTo-Json -Depth 9 -Compress
$projected = ConvertTo-InstagramDiagnosticApiFailure -Status 409 -Body $body
if ($null -eq $projected.diagnostic -or $projected.diagnostic.complete -or $projected.diagnostic.sequence -ne 10000 -or $projected.diagnostic.cause.condition -cne 'root_request_duplicate' -or $projected.diagnostic.cleanup.state -cne 'verified' -or $projected.retried -or ($projected | ConvertTo-Json -Depth 9 -Compress).Contains('PRIVATE')) { throw 'Saturated trace lost bounded failure or cleanup' }
Write-Output 'PASS: incomplete saturated trace retains first cause and independent cleanup.'

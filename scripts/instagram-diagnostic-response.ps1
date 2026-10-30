# Pure response projection. Loading this helper makes no requests or changes.
function ConvertTo-InstagramDiagnosticApiFailure {
    param([int] $Status, [string] $Body)
    $code = 'request_failed'
    $allowed = @('capture_epoch_mismatch', 'invalid_request', 'not_found', 'internal_error',
        'instagram_diagnostic_unavailable', 'instagram_diagnostic_canceled',
        'instagram_diagnostic_busy', 'instagram_diagnostic_already_used',
        'instagram_diagnostic_maintenance_required', 'instagram_diagnostic_source_busy',
        'instagram_diagnostic_owner_unavailable', 'instagram_diagnostic_diagnostic_rejected',
        'instagram_diagnostic_projection_invalid')
    if ($Body -and [Text.Encoding]::UTF8.GetByteCount($Body) -le 4096) {
        try {
            $response = $Body | ConvertFrom-Json -ErrorAction Stop
            # The server's error field is the code string, not a nested object.
            if ($response.error -is [string] -and $response.error -cin $allowed) {
                $code = $response.error
            }
        } catch { }
    }
    $safeStatus = if ($Status -ge 100 -and $Status -le 599) { $Status } else { 0 }
    [pscustomobject]@{ status = $safeStatus; code = $code; retried = $false }
}

# Use from an operator catch block; never print or persist ErrorDetails/raw body.
function ConvertFrom-InstagramDiagnosticException {
    param([System.Management.Automation.ErrorRecord] $Failure)
    $status = 0
    if ($Failure.Exception.Response) {
        try { $status = [int]$Failure.Exception.Response.StatusCode } catch { }
    }
    ConvertTo-InstagramDiagnosticApiFailure -Status $status -Body ([string]$Failure.ErrorDetails.Message)
}

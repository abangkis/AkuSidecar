# Pure response projection. Loading this helper makes no requests or changes.
function ConvertTo-InstagramDiagnosticTrace {
    param($Value)
    # Operator tooling uses its matched source catalog. A missing catalog yields
    # a code-only report, never an unvalidated diagnostic passthrough.
    $catalogPath = Join-Path $PSScriptRoot '../internal/igdiagnostic/catalog.json'
    if (-not (Test-Path -LiteralPath $catalogPath)) { return $null }
    try {
        $catalog = Get-Content -LiteralPath $catalogPath -Raw | ConvertFrom-Json -ErrorAction Stop
        $keys = @('schema','stage','completed','disposition','sequence','cause','cleanup','complete','elapsedMS','attempt','operation','revision','delivery','httpDelivery','evidence','nodeElapsedMS','nodeSequence','budgetMS','remainingMS')
        if ($Value -isnot [pscustomobject] -or @($Value.PSObject.Properties).Count -ne $keys.Count) { return $null }
        foreach ($property in $Value.PSObject.Properties) { if ($property.Name -cnotin $keys) { return $null } }
        if ($Value.schema -cne 'aku.ig-trace.v1' -or $Value.revision -cne 'ig1' -or $Value.attempt -isnot [string] -or $Value.attempt -cnotmatch '^[a-f0-9]{16,64}$') { return $null }
        if ($Value.operation -cnotin $catalog.operations -or $Value.stage -cnotin $catalog.stages -or $Value.completed -cnotin $catalog.stages -or $Value.disposition -cnotin $catalog.dispositions -or $Value.complete -isnot [bool]) { return $null }
        foreach ($key in @('sequence','nodeSequence')) {
            $n = $Value.$key
            if (($n -isnot [int] -and $n -isnot [long]) -or $n -lt 0 -or $n -gt 10000 -or ($key -ceq 'sequence' -and $n -eq 0)) { return $null }
        }
        foreach ($key in @('elapsedMS','nodeElapsedMS')) {
            $n = $Value.$key
            if ($null -ne $n -and (($n -isnot [int] -and $n -isnot [long]) -or $n -lt 0 -or $n -gt 60000)) { return $null }
        }
        if ($null -ne $Value.budgetMS -or $null -ne $Value.remainingMS) {
            if (($Value.budgetMS -isnot [int] -and $Value.budgetMS -isnot [long]) -or $Value.budgetMS -lt 1 -or $Value.budgetMS -gt 30000 -or ($Value.remainingMS -isnot [int] -and $Value.remainingMS -isnot [long]) -or $Value.remainingMS -lt 0 -or $Value.remainingMS -gt $Value.budgetMS) { return $null }
        }
        $cause = $null
        if ($null -ne $Value.cause) {
            if ($Value.cause -isnot [pscustomobject] -or @($Value.cause.PSObject.Properties).Count -ne 2 -or $Value.cause.PSObject.Properties.Name -cnotcontains 'stage' -or $Value.cause.PSObject.Properties.Name -cnotcontains 'condition' -or $Value.cause.stage -cnotin $catalog.stages -or $Value.cause.condition -cnotin $catalog.conditions -or $Value.cause.condition -ceq 'none') { return $null }
            $cause = [pscustomobject]@{ stage = $Value.cause.stage; condition = $Value.cause.condition }
        }
        $settlements = @{}
        foreach ($key in @('cleanup','delivery','httpDelivery')) {
            $v = $Value.$key
            $states = $catalog.delivery
            if ($key -ceq 'cleanup') { $states = $catalog.cleanup }
            if ($v -isnot [pscustomobject] -or @($v.PSObject.Properties).Count -ne 2 -or $v.PSObject.Properties.Name -cnotcontains 'state' -or $v.PSObject.Properties.Name -cnotcontains 'condition' -or $v.state -cnotin $states -or $v.condition -cnotin $catalog.conditions) { return $null }
            if ($key -ceq 'cleanup' -and $v.state -cne 'uncertain' -and $v.condition -cne 'none') { return $null }
            $settlements[$key] = [pscustomobject]@{ state = $v.state; condition = $v.condition }
        }
        if ($Value.evidence -isnot [pscustomobject] -or @($Value.evidence.PSObject.Properties).Count -ne @($catalog.checks).Count) { return $null }
        $evidence = [ordered]@{}
        foreach ($key in $catalog.checks) {
            if ($Value.evidence.$key -cnotin $catalog.evidence) { return $null }
            $evidence[$key] = $Value.evidence.$key
        }
        $projected = [pscustomobject][ordered]@{
            schema = $Value.schema; attempt = $Value.attempt; operation = $Value.operation; revision = $Value.revision
            stage = $Value.stage; completed = $Value.completed; disposition = $Value.disposition
            sequence = $Value.sequence; cause = $cause; cleanup = $settlements.cleanup
            delivery = $settlements.delivery; httpDelivery = $settlements.httpDelivery; complete = $Value.complete; elapsedMS = $Value.elapsedMS
            nodeElapsedMS = $Value.nodeElapsedMS; nodeSequence = $Value.nodeSequence; evidence = [pscustomobject]$evidence
            budgetMS = $Value.budgetMS; remainingMS = $Value.remainingMS
        }
        if ([Text.Encoding]::UTF8.GetByteCount(($projected | ConvertTo-Json -Depth 8 -Compress)) -gt 2048) { return $null }
        return $projected
    } catch { return $null }
}

function ConvertTo-InstagramDiagnosticApiFailure {
    param([int] $Status, [string] $Body)
    $code = 'request_failed'
    $diagnostic = $null
    $diagnosticIncomplete = $false
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
                if ($response.details -is [pscustomobject] -and $null -ne $response.details.diagnostic) {
                    $diagnostic = ConvertTo-InstagramDiagnosticTrace -Value $response.details.diagnostic
                    $diagnosticIncomplete = $null -eq $diagnostic
                }
            }
        } catch { }
    }
    $safeStatus = if ($Status -ge 100 -and $Status -le 599) { $Status } else { 0 }
    $safe = [ordered]@{ status = $safeStatus; code = $code; retried = $false }
    if ($null -ne $diagnostic) { $safe['diagnostic'] = $diagnostic }
    if ($diagnosticIncomplete) { $safe['diagnosticIncomplete'] = $true }
    [pscustomobject]$safe
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

param(
    [ValidateSet('user', 'codex')] [string] $Actor = 'user',
    [ValidateRange(0, 3600)] [int] $WaitForIdleSeconds = 900,
    [ValidateRange(1, 10)] [int] $PollSeconds = 2,
    [switch] $ReplaceReaderBrokerRegistration,
    [switch] $Verify,
    [switch] $PlanOnly,
    [switch] $PrepareOnly,
    [ValidateSet('daily', 'no-session-transfer')] [string] $ActivationMode
)
# Keep the established entry point; the workflow shares the dev bundle contract.
& (Join-Path $PSScriptRoot 'dev-restart-workflow.ps1') @PSBoundParameters

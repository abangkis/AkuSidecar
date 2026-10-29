# Instagram main-dev trial

The operator approved implementation and activation of this unqualified IG
recipe on 29 October 2026. This changes the earlier QA-first execution policy;
it does not set either native qualification record to true.

## Selection and permission

Set `capture.instagramMode` to `concurrent` and `capture.instagramTrial` to
`true`. Trial selection is rejected unless development mode and Windows split
capture are active. Ordinary/default builds do not attach a native provider.
The normal main browser, profile and native host remain the integration path.

Settings exposes a separate experimental IG panel. Start requires all three
explicit boolean fields: `acknowledgeLimitations`,
`allowProcessSessionDerivation` and `acknowledgeUnqualifiedRecipe`. Development,
same-origin and current process epoch checks remain mandatory. The private
bootstrap carries the trial acknowledgement only from this native composition.

Start prepares an owner for normal manual/automatic updates; it does not force a
batch. Following the operator's 29 October update, permission is saved for the
selected browser profile, including its Chrome subprofile, using the existing
source-scoped permission Store. Restart restores the choice through a new owner;
it never restores old account/session evidence. Each batch still requires fresh
verification. Stop removes the saved IG grant and ends process permission.
Restart is required before a consumed/failed trial can start again. X permission
is independent. The earlier process-only trial policy is superseded.

Only `sessionid`, `csrftoken` and `ds_user_id` can be derived to a temporary
collector. Every batch retains the existing 60-second maximum, paired fresh
account/login/session proof, source/profile/generation fencing and Store checks.
Account/session rejection or uncertain cleanup stops IG without turning off
Auto Update. Confirmed transient failures have bounded retries on subsequent
updates under [the shared retry policy](daily-source-retry-policy.md).
Owned Home/media targets close after the batch; user readers remain open. No
cookie value, raw account ID or response body is included in diagnostics.

## Activation and verification

Build and inspect a candidate using the established `scripts/restart-dev.ps1`
workflow, then use its graceful shutdown/promotion/startup path. Never replace
the native-host registration or restart Supervisor for this ordinary handoff.
Retain a consistent pre-migration database backup if moving from schema 30 to31.

Verify health, runtime provenance, the saved IG profile choice and fresh owner,
the experimental panel, and unchanged Auto Update/settings. Startup readiness is not proof that the
candidate endpoint works: the first actual batch performs that verification.
Do not report capture acceptance until the actual run and cleanup are observed.

## Identity rejection diagnostics

Identity rejection now retains a closed reader/collector category through the
private RPC, Engine status and first-failure log. For example,
`native_source_evidence_collector_identity_http_forbidden` identifies a denied
collector response without exporting its body or account/session values.

The page projection distinguishes page/login state, HTTP rejection class,
origin/MIME, bounded body reading/decoding/JSON parsing, response shape, account
field validity and page changes. Unknown or contradictory projections become
`identity_projection`; they cannot inject a server message into an error code.
The categories do not authorize evidence, add a retry or relax any verifier.
Cleanup remains independent of the originating identity rejection. Historical
`native_source_evidence_identity` failures cannot be classified retroactively.

## Rollback

Stop IG, wait for cleanup verification, set its selector to `browser`, remove or
disable `instagramTrial`, and use graceful restart with a schema-31-compatible
binary. Preserve all other settings. Do not lower database schema metadata.
Restoring an older schema-30 binary requires a separately approved pre-migration
database restoration, not merely replacing the executable.

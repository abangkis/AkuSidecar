# Daily X concurrency in ordinary development

## Target and selected policy (2026-10-27)

The MVP is ordinary, repeated X capture in AkuBrowser main dev, with the native
reader usable during collection and results entering the existing Timeline.
An isolated QA package is supporting evidence, not the delivered user journey.
Only X changes backend. Other sources, existing effective reasoning provider,
settings, database, pacing, challenge pause and Auto Update remain ordinary.

The user approved derivation authorization for the lifetime of the Sidecar
process, with a maximum of 60 seconds for each batch. Restart expires that
authorization; saved configuration is a mode selection, not an account proof,
cookie receipt or replayable Store permit. Each batch must obtain its own
verified account/login/ownership evidence and run-scoped claim/result authority.
The original attempt clock cannot reset inside a batch. A new normal batch may
derive only under a still-valid process authorization after verified cleanup.

## Three delivery increments

1. **Restart-required ordinary composition.** Add `capture.xMode`, either
   `exclusive` (omitted/default) or `concurrent`. Validate supported Windows
   `auth_session_dev` development selection before Store/provider/UI access.
   Use normal config, port, Store, effective settings/provider and UI, without
   requiring a fresh QA plan or database. Attach X routing before resume,
   HTTP serving and scheduling. A mode change takes effect only in a new
   gracefully restarted process. No hot switch is promised by this increment.
2. **Daily native batches.** Transfer ownership of the signed-in reader to the
   daily composition with no simultaneous profile owner. Keep the reader alive
   across disposable collectors. Route ordinary manual and scheduled X work
   through the existing parser/worker/Coordinator and per-batch Store permits.
   Preserve command budgets and source settings instead of manufacturing a
   one-command QA session. Wire process authorization, repeated derivation,
   60-second batch expiry, source Stop and verified cleanup. Do not silently
   fall back, retry a failed derivation or reuse a consumed batch permit.
3. **Daily-use acceptance and activation.** First validate offline/synthetic
   boundaries. After separate operational authorization, prove two batches in
   one normal app session, reader interaction during capture, normal Timeline
   results, Stop blocking subsequent X batches, and graceful config rollback
   from concurrent to exclusive with user data/provider/preferences preserved.

Full refresh/rotation qualification and hot switching remain deferred. Account
mismatch, observed logout, challenge, owner loss or uncertain cleanup pauses X
and requires explicit recovery. No automated challenge bypass is introduced.

## Increment 1 boundary

The mode is read once with the ordinary typed config. Unsupported/non-dev and
production selections fail before normal Store startup. Manual/QA selectors
cannot be mixed with daily concurrency. The normal development port and existing
database are permitted; no QA namespace or `OpenDevelopmentQADatabase` is used.

This increment owns no second reader, collector, profile lease, native factory,
cookie derivation or batch permission. The existing ordinary capture process
remains the sole current profile owner for non-X work. Ownership transfer to
the persistent reader belongs to increment 2.

Until that adapter is connected, status reports selected `concurrent`, effective
`unavailable`, state `not-ready`, reason `batch-adapter-required`, and no granted
derivation authority. Legacy X claims, dispatch and reader borrowing fail closed.
Mixed-source updates continue eligible non-X sources without rewriting saved
source choices or Auto Update. Queued X runs encountered after restart receive
an explicit readiness failure so later non-X work is not stranded. Closing or
stopping the selection never silently changes backends; restart with exclusive
configuration is the rollback path.

Illustrative config change, not an instruction to edit the live config:

```json
{"capture":{"xMode":"concurrent"}}
```

The rest of the existing capture/config fields must be retained. No runtime
restart, registry change, login access, user-database access or session transfer
is authorized by this design, implementation or its offline tests.

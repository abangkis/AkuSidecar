# Instagram bootstrap evidence for daily capture

Date: 2026-10-31

Scope: implement the approved shared private evidence source and validate it
offline. Runtime activation, account access and session transfer remain separate
operator decisions. This increment does not change the daily controller, Store
schema, cookie manifest, remembered consent or Auto Update.

Status: implementation and offline acceptance completed in the isolated source
checkout. No runtime activation or native capture acceptance is claimed.

## Evidence and remaining boundary

The real reader diagnostic on source `6c0c3a5` returned HTTP 200,
`reason=corroborated`, in 2877 ms. Delivery/body/document bounds, complete scans,
identity agreement and account-cookie comparison passed; cleanup was verified.
The public result remained `qualified=false`. No collector or session transfer
was involved. This establishes one reader observation, not daily capture or
collector qualification.

The production verifier still used the rejected `current_user` request at that
checkpoint. This implementation replaces that dependency with the same
bounded bootstrap acquisition used by the diagnostic. A cached diagnostic
success or its public booleans must never become batch authority.

Read-only inspection of the two permission metadata records found valid matching
profile scopes, X enabled and IG disabled. Restart faithfully restores the stored
choice. Start saves consent before owner verification; a proof failure does not
revoke it. Stop IG persists revocation. The record contains no action timestamp
or actor, so the initiating action cannot be attributed retrospectively. Do not
repair this by silently changing a disabled record to enabled.

## Implementation contract

- One private acquisition owns bounded root delivery, streaming, strict complete
  scans and delivered-body/live-document/account-cookie agreement. Its private
  identity can be consumed only by the native proof builder; public diagnostic
  projections remain value-free and `qualified=false`.
- The diagnostic wrapper owns and closes its dedicated target. Production
  evidence belongs to live batch-owned role documents. Acquisition releases its
  temporary interception, streams and listeners; batch cleanup retains the
  obligation to close those role targets. Never authorize from a closed target.
- Verification must not navigate a page while capture or media extraction is
  using its DOM. Reader and collector verification use independently owned
  evidence surfaces where navigation is required.
- Keep the existing required-cookie projection, HMAC account/session bindings,
  owner/profile/role/generation/nonce checks and paired proof validation. Raw
  account IDs, bodies and cookies remain transient private data, never logs,
  HTTP fields or persisted permission.
- Pair acquisition remains concurrent and final validation still rejects stale
  results. Use actual trusted delivery age; a later read must not refresh an old
  identity's timestamp. Keep the six-second evidence limit and 4 MiB body bound.
  Acquisition is capped by the remaining 60-second batch budget with cleanup
  reserved, rather than receiving a new independent 30-second allowance.
- Preserve the first failure and report cleanup independently. Unverified
  cleanup, identity mismatch, challenge and ownership loss remain blocking.

## Offline acceptance

Cover both owned roles, repeated paired verification, a bounded synthetic
capture using the real provider seam, and removal of the production
`current_user` dependency. Reject stale delivery, partial scans, late duplicate
or conflicting candidates, account/session mismatch, wrong role/profile/nonce/
generation, document/context changes and uncertain cleanup. Public diagnostics
must reject private fields and retain their exact existing schema.

Reuse the existing Engine/Store regressions for saved consent across reopen,
fresh verification after restart, proof failure preserving enabled consent,
source isolation and Stop persisting false. Do not replace those proven flows.

### Validation receipt

The final Node command passed 196 tests, including private acquisition, owned
page events, repeated paired provider verification, capture-document preservation,
public diagnostic compatibility, real synthetic CDP/control framing, scanner
rejections, X regressions and sealed package inclusion:

```text
node --test "internal/collection/headless/worker/native-pilot/*.test.mjs" internal/collection/headless/worker/test/chrome-temporary-page.test.mjs scripts/build-x-daily-owner.test.mjs
```

Targeted tagged Engine and Store suites passed for IG consent/reopen/fresh-proof
failure, X permission behavior, and per-source permission isolation. The new
finite bootstrap rejection vocabulary passed `TestSourceNative.*Failure`.
An isolated `auth_session_dev` main binary compiled successfully. Dependencies,
build cache and test temporary files were offline/project-local; no active
runtime directory, account session or user database was used by these tests.

Independent read-only review of the generic owned-page event wrappers found
that page retirement needed to detach subscriptions. Both wrappers now do so,
including failed target cleanup; four focused tests cover session filtering,
late events, idempotence and the 32-listener boundary. The reviewer closed the
finding after the correction. These are synthetic event/ownership tests, not
real Chrome/Instagram qualification.

## Concrete native trial after review

1. Review the complete source delta and offline receipts, then prepare a matched
   candidate and rollback using the ordinary development lifecycle. Do not
   activate or release a maintenance hold during preparation.
2. Obtain approval for graceful activation and the exact trial. Keep the trial
   gate and unqualified recipe designation. An explicitly disabled IG choice
   must be re-enabled through Start IG by the operator; preserve X and all other
   settings. Activation alone is not consent for a transfer.
3. With explicitly authorized derivation, observe seed, fresh reader/collector
   proof, capture/Store acceptance and verified cleanup in two normal updates,
   each at most 60 seconds. Preserve the reader and prove no batch-owned target
   or collector remains. Observe X independently in the mixed-source update.
4. After an approved graceful restart, verify remembered IG consent and a new
   owner with fresh proof, without login again. Exercise Stop IG and confirm
   saved revocation plus cleanup without affecting X or Auto Update.

The trial remains pending until these observations are recorded. One successful
reader diagnostic or offline suite does not qualify general native IG capture.

## IG-local timeout and worker lifecycle repair

The subsequent native trial failed with `native_source_wire_unavailable` and
uncertain cleanup. The worker was no longer present. Its exact initiating
command and process exit code were not retained; they cannot be inferred from
the generic wire error. A separate `reader_bootstrap_document_changed` result
belongs to an earlier run and is not the cause established for this incident.

Offline reproduction with production transport modules established a concrete
failure mechanism: an IG-owned `Target.createTarget` timeout used the ordinary
CDP call path, which closes the shared transport. That close could suppress the
operation's terminal reply and end the Node worker. The diagnostic bootstrap
scope already had request-local timeouts, but the native scope did not carry
that ownership capability or all late-disposal callbacks through its layers.

The repair keeps request-local failure private to IG-owned evidence resources.
It tracks late target, session and stream replies, retires only resources that
belong to that operation, and does not report cleanup as verified while late
creation remains unresolved. No late reply can authorize proof or replace a
terminal operation result. Ordinary calls and X keep their fatal timeout
behavior. Broken framing, wrong-session replies and transport EOF remain fatal.
Proof freshness, account/session bindings and batch deadlines are unchanged.

The private Node control channel now emits a bounded, closed-vocabulary first
close report before teardown. Go records it through the shared diagnostic
recorder, with control-read outcome, process exit and owned-tree cleanup as
independent observations. These fields are not retry authority. IG failure
status exposes a revalidated `workerDiagnostic` sideband; ordinary/X failures
retain their existing error contract. A validated lifecycle snapshot is logged
as `main_transport_worker`. No stderr, command parameters, resource IDs, account
identifiers, paths, response bodies or cookie/token values are included.
Missing delivery remains incomplete with an unknown cause, even if an exit
code is subsequently observed. An exit code of zero alone is not evidence that
the batch or cleanup succeeded.

This repair is offline-only until a matched candidate is reviewed and activated.
It does not authorize another native probe, session derivation, or capture trial.

### Timeout repair validation receipt

The affected Node suite passed **289/289** tests, including the production
transport's surviving terminal reply, late reader/collector resource cleanup,
post-retirement uncertainty, ordinary/X timeout and framing guards, fatal Node
exception/rejection behavior, and sealed-package module inclusion:

```text
node --test "internal/collection/headless/worker/native-pilot/*.test.mjs" "internal/collection/headless/worker/test/chrome*.test.mjs" scripts/build-x-daily-owner.test.mjs scripts/x-daily-development-package.test.mjs scripts/x-development-pilot-package.test.mjs
```

The Go appshell, owned RPC, shared trace and IG trace packages passed. Targeted
tagged Engine tests covered IG failure metadata, source isolation, source-native
failure classifications and existing X daily behavior. Ordinary and
`auth_session_dev` main binaries compiled offline; they were not executed.

An expanded run including `scripts/x-native-owner.test.mjs` passed 294/295.
Its existing `manual requests remain concurrent for rendezvous, cancel and
branded capture diagnostics` test also fails when every modified tracked MJS
module is restored in memory to baseline `5815244`. The observed last reply is
ID 2 instead of ID 4. This pre-existing test failure is recorded separately;
the unrelated owner protocol/test was not changed in this IG repair.

# LinkedIn pending diagnostics and Instagram identity contract review

Date: 2026-10-30. Scope: isolated source changes and offline tests only.
Baseline: Sidecar `efda84f`, Bridge `783065f`.
No runtime activation, browser access, real diagnostic request, session transfer,
login change, or Auto Update change is part of this work.

## LinkedIn pending diagnostics

The previously observed message, `Capture surface cleanup remains unverified.`,
comes from journal admission. It does not identify which ownership predicate
failed. Separately, a release can return `lease_mismatch` while its journal
receipt is complete with zero expected records. Receipt completeness alone is
not sufficient to acknowledge that release.

The additive `aku.surface-pending-diagnostic.v1` projection reports:

- Phase `admission` or `release`, first observed reason, and per-reason counts.
- Missing/unavailable/mismatched owner generation, window mismatch, pending
  navigation, non-owned URL, unavailable inventory, and prior unresolved pending
  status. Release also distinguishes another lease's claim, operation failure,
  unverified closure, and a runtime lease mismatch.
- Counts of pending records matching the requested lease, belonging to other
  leases, or lacking a lease. These count all pending journal records, whereas
  the existing cleanup receipt remains scoped to the requested lease/source.
- Current lease presence/equality at journal precheck; a separate nullable
  runtime equality result (`false` when the legacy operation rejects a mismatch,
  `null` when it did not report an equality result).

The first reason is observation order, not proof of historical causation. A
record can fail multiple predicates, so reason counts need not sum to the number
of pending records. Counts are bounded by the journal capacity of 4,096.
No tab/window IDs, lease values, URLs, API exception messages, or storage values
are added to this projection.

Admission evidence travels in the original error details and a `reconciled`
lifecycle event. Preparation wrapping preserves that event before appending
separate cleanup events. The existing worker failure serializer and surface
event POST retain the projection. Sidecar stores generic detail maps with an
8 KiB limit; no Store policy or schema change is required.

The loaded lifecycle revision and Sidecar policy fingerprint change together
to distinguish a worker with these diagnostics from a cached earlier module.
The guard decisions and strict cleanup receipt validator remain unchanged.
In particular, a previously pending record stays pending even when its current
tab predicates match; `prior_pending_unresolved` makes that behavior visible.
No new conclusion about the exact live LinkedIn cause can be made offline.

## Instagram missing username: Astra consultation

The retained passive result was `reason=shape`,
`shapeReason=missing_username`, `modulePresent=true`, `documentBound=true`,
`scanComplete=false`, and `qualified=false` / `serverFresh=false`.

The discovery parser requires an exact four-element `PolarisViewer` tuple,
empty dependencies, an object with an own `id`, then an own `username`.
ID type/format checks happen later. Therefore the result establishes only that
an encountered candidate lacks the assumed direct username property. The false
ID-shape flags do not prove an invalid ID. The scan ended before completeness
or uniqueness could be established.

This username requirement belongs to the explicitly unqualified synthetic
discovery grammar in `instagram-self-bootstrap-probe.mjs`, not to production
identity authority. `instagram-native-session.mjs` instead requires a successful
current-user response, authenticated nonanonymous user, valid canonical `user.pk`,
private corroboration with `ds_user_id`, and bound account/session proof.
It does not require username. The existing windowed extraction's authenticated
UI cues are not an established authenticated-current-user identity source.

Astra's recommendation was approved in the next increment and is now
implemented offline as diagnostic v2: classify direct IDs independently of
username presence, retain username failure observations, and finish the bounded
scan across malformed candidates. Duplicate/conflict checks compare only valid
canonical IDs. `qualified` and `serverFresh` remain false. The detailed v2
contract is in `instagram-self-bootstrap-qualification.md`; no new identity
source is accepted by this change.

Even an ID-shaped candidate would still require qualification of authenticated
self semantics, canonical ID namespace, actual server delivery and document
binding, plus independent collector evidence. A normal diagnostic HTTP 200 or
document continuity alone cannot supply that qualification. The earlier HTTP
400 from the current-user endpoint is a separate request-contract issue; this
offline observation does not resolve it.

## Offline validation

- Bridge capture-window runtime, failure serialization and strict cleanup
  receipts: 114 tests passed. The 12 pending-diagnostic cases were rechecked
  after adding the actual worker event-POST projection to the simulated path.
- Loaded policy identity, surface telemetry and extension contracts: 19 passed;
  one existing extension-contract assertion failed. The same failure was
  reproduced on unchanged Bridge `783065f`: it searches `content-script.js`
  for literal `login_required` after readiness moved to shared primitives.
  Neither that source nor that assertion is changed by this patch.
- Instagram bootstrap discovery and current native-session contract: 23 passed.
- Go `TestExecutingBridgeSurfacePolicy`: passed with local module/cache inputs,
  network resolution disabled, and isolated temporary output.
- Both worktree diffs passed `git diff --check`.

These tests do not establish a live LinkedIn cause or qualify Instagram identity.
Changes remain uncommitted in isolated worktrees and are not active in main dev.

## Read-only observation after the operator's next manual restart

Supervisor reported healthy Sidecar PID 5252. Runtime provenance was built at
2026-10-30 14:35:19 UTC from `efda84f` with local changes. The latest retained
batch `session_60866ba5f352680a493411900fe99b16` ran at 21:36 WIB: X completed,
Instagram failed, and LinkedIn failed `cleanup_pending` at 21:36:15 WIB.

LinkedIn's 56 reconciliation events observed through 21:38:49 WIB carried the
old executing policy fingerprint
`e8439a2c931876aed1b7156eb58a61a828f9e431183fbf76a37308653f4c40f7`.
None contained the new `cleanupDiagnostics`. All 56 reported pending despite an
empty complete receipt. Under that inspected source this is consistent with
the legacy `lease_mismatch` branch, but does not identify why admission's
earlier pending record failed. The preceding batch failed dispatch timeout.

The diagnostic implementation remains in isolated worktrees, so a restart of
the primary checkout did not activate it. The new expected fingerprint is
`81de92d4b6289fba020601ad2e4629b85d193ce1be2e167cbb0c607dc2b523ad`.
This observation used only Supervisor inspection and localhost GET requests;
no batch, transfer, IG probe, restart, or settings change was requested.

Validation of the v2 increment: 57 Node tests passed across discovery, diagnostic
scope, native-session and transport suites. The 21 discovery cases were checked
again after adding forged-combination and deadline-after-conflict assertions.
Targeted `TestInstagramBootstrap*` tests passed in both Go appshell and HTTP API
packages with `auth_session_dev`, including the synthetic operator route and
17 independent v2 projection cases. Diffs passed whitespace validation.

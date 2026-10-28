# Capture surface lifecycle and policy identity

Status: offline implementation; operational activation is a separate gate.

## Ownership and lifetime

Surface ownership is a creation/registration fact, not a conclusion drawn from
the current URL. The Bridge journal binds each capture tab or lease placeholder
to its lease, tab/window identity, role and browser-session generation. A worker
restart preserves that generation. A different browser generation cannot use an
old record as permission to close a current tab with the same numeric ID.
Extension reload/update can also clear session storage; this is treated as an
unverified generation boundary, not proof that old windows disappeared.

| Role | Lifetime | Cleanup owner |
| --- | --- | --- |
| `batch_capture` | Source/batch lease | Bridge managed capture runtime |
| `lease_placeholder` | Shared Browser lease | Bridge terminal lease cleanup |
| Main-profile X bootstrap | One batch | Native daily owner, after collector/enrollment cleanup |
| Dedicated fixture bootstrap | X process authorization | Existing dedicated owner, on Stop/owner close |
| User source/reader | Explicit user interaction | Existing source/reader owner; never the batch sweep |

Navigation outside a recognized feed preserves the tab and records pending
cleanup. It does not assert a transfer to the user or erase the record. Closure
is successful only after a successful scoped inventory verifies absence.
Failed storage, inventory or removal cannot manufacture a completed receipt.
Pending cleanup survives worker restart and prevents new Browser surfaces from
silently accumulating behind an unresolved predecessor.
Malformed journal data is not treated as an empty ledger. The journal is bounded
at 4,096 records without silently evicting unresolved ownership or tombstones;
capacity exhaustion reports pending cleanup and requires operator resolution.

## Receipt

`aku.surface-cleanup.v1` echoes the lease and optional source and accounts for
`expected`, `closed`, `transferred`, `retained` and `pending` surfaces. All counts
are bounded nonnegative integers and must balance. Success requires `complete`
and zero pending surfaces. A source release may retain an identified shared
placeholder; terminal Browser release must not retain batch-owned surfaces.

No current path infers user transfer from navigation. Unknown/native-post
navigation stays pending while the tab is preserved. An explicit future
user-transfer action must establish its own evidence before counting a transfer.
An empty or missing outcome, a lease mismatch, or an unverified legacy
`no_owned_surface` reply is not completion proof. Background dispatch, UI polling
and the Go collection-owner release validate the same receipt contract.

The split relay has two explicit no-Browser-owner release paths: effective
headless collection and direct-reader-only transport. Those paths return a
zero-count receipt bound to the validated request lease and source (including
an explicit `null` source for terminal release), because neither path can own
Bridge Browser capture surfaces. This bypass is limited to those known owner
modes; it does not make an empty response or a reason string sufficient proof.

The X status reports its cleanup scope as `batch_collector` and the main-profile
bootstrap lifetime as `batch`. Enabling X creates no Home target. A batch creates
its own exact Home target and closes it with verified absence before returning
idle. Explicit reader preparation uses process ownership plus fresh marker,
target, window, frame and document proof; it does not require a Home target to
remain open. A user reader is never part of automatic batch cleanup.

### Prepared observations and failed batch cleanup

PREPARE owns the observation's original deadline, including time held before
ACQUIRE. The private request carries an absolute orchestration deadline capped
by the original attempt; the Node actor converts it once to monotonic time.
ACQUIRE cannot reset or enlarge it and must reject expiry before releasing the
identity pause. Relative phase ages are diagnostics, never authorization.

Root controls must be restored through a failure-safe release path. Failure to
restore one control does not skip the second control or independent retirement
of the batch's collector, enrollment, attachments and exact Home target. The end
receipt contains `readerControls`, `collectorControls`, `collector`, `enrollment`,
`attachments` and `bootstrap`, each `verified`, `uncertain` or `not_needed`.
`closed` is true only when all six outcomes are known and none is uncertain.
Missing/malformed outcomes are not proof of completion. A later target close
does not erase an earlier restoration failure.

Go retains startup's first cause and reports cleanup separately. It attempts
independent collector-exit confirmation even when the end receipt is uncertain;
collector pins/profile may be released only after exit has been proved. Unknown
collector ownership cannot be treated as an empty process inventory. Borrowed
main Chrome and explicit user readers are outside batch retirement authority.

## Remembered permission and fresh authorization

Main-dev Settings remembers an explicit opt-in for the selected reader profile
across application restarts. The independent versioned `meta` record stores only
the enabled flag and a profile-scope fingerprint; it does not contain credentials,
cookies, account identifiers, actor handles or session evidence. Saving ordinary
Settings cannot accidentally erase this record. An absent, changed or invalid
scope never enables X by default.

After factory registration, startup scheduling restores the preference once and
creates a new process owner. The first and every subsequent batch must obtain
fresh account/login/session evidence and a fresh disposable collector. Neither
old runtime permission nor verification evidence is restored. The 60-second
batch boundary is unchanged. Restoration failure disables only X and is visible
in status; it does not turn off Auto Update or other sources.

Stop X durably revokes the preference before ending current work. Ordinary
shutdown closes runtime resources without revoking the saved preference. A
permission granted by an older build was memory-only: deployment must either
explicitly migrate that active user choice during an approved handoff, or ask
the user to enable X once in the new build. The new preference then survives
later restarts; selecting concurrency mode alone is not permission.

## Executing policy and activation

The worker hashes a deterministic descriptor of the feed rules and lifecycle
revision actually imported into that worker. It does not fetch the disk source
and mistake a new file for the policy running in an old cached module graph.
The fingerprint contains no account, URL history, cookie or token data.

Sidecar pins this policy fingerprint alongside Bridge runtime revision v118.
An absent or mismatched fingerprint reports reload required through the existing
compatibility boundary. A reload action completes only after a fresh heartbeat
matches both the expected build label and imported-policy fingerprint. Cleanup
telemetry carries the executing policy identity for later correlation.

This fingerprint covers the critical policy descriptor; it is not a byte-level
attestation of the entire module graph. Extension package verification separately
hashes packaged files. Updating policy requires updating its release reference
and passing the cross-repository fingerprint regression.

## Validation and rollout

Offline regressions cover repeated mixed-source batches, alternate feed routes,
pending ownership, worker restart, browser-generation replacement, already absent
tabs, failed or falsely successful close calls, stale/missing acknowledgements,
receipt correlation and stale imported policy. No live capture or session
derivation is needed for these checks.

Activation must reload the extension and verify its executing fingerprint before
an ordinary capture run. Older orphaned windows whose ownership was previously
deleted are not reclaimed by URL or closed in bulk. Their resolution remains an
explicit operator action. Login and Auto Update configuration are unchanged by
this implementation.

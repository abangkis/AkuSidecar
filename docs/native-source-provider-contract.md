# Native source providers: shared core and Instagram implementation

## Implementation boundary

This increment implements the native Instagram provider and its main-browser
transport composition. It is not a native acceptance result. The immutable Go
and Node qualification records remain false until the candidate evidence and
session recipe are checked in a separately authorized QA run. Configuration,
RPC parameters and synthetic evidence cannot qualify the recipe.

No signed-in profile, session transfer, user database, running service or Auto
Update setting is accessed or changed by this offline implementation. The main
configuration continues to select Browser for Instagram.

## Shared responsibilities

| Component | Responsibility | Current reuse |
| --- | --- | --- |
| `dailyController` | Saved source/profile permission, admission, generation, cancellation, first failure and cleanup state | Native X, synthetic IG, native IG |
| `sourceNativeOwner` / `sourceNativeBatch` | Fresh challenge proof, paired account/session binding, original deadline and private Store authority | Native IG and future native recipes |
| `captureSourceDailyPermit` | Verify, claim, capture, verify, commit through the existing Store | Synthetic IG and native IG |
| Existing Windows owner/pin checks | Actual image, parent, Job, process and profile-directory identity | Existing X and native IG |
| `native-source-kernel.mjs` | Attempt lifetime, bounded operations, late-resource cleanup, exact-target retirement and private diagnostics | Native IG and future native recipes |
| `source-owner-registry.mjs` | Sealed factory selection, one process owner per source, replay protection and terminal cancellation | Main native source lane |
| Packaged source asset catalog | Immutable source-specific parser/media assets selected by source | X and IG main transport |

The existing X `main_x` wire and native evidence recipe remain compatible. Its
specialized verifier is not replaced by the unqualified IG recipe. New source
providers use `main_source`; they do not need copies of the X experiment driver,
the Engine scheduler, Store, media pipeline or packaging framework.

## Private protocol and process ordering

The existing public CDP facade keeps its projected allowlist. Only sealed Node
composition supplies native factories; a request cannot provide a factory,
cookie manifest, evaluator or qualification flag.

The private request envelope is:

```text
{ id, op: "main_source", source: "instagram", operation, params }
```

The native owner implements this ordered contract:

1. `source_bootstrap`: bind the existing main reader PID/profile/subprofile and
   explicit process derivation permission. No session read occurs here.
2. `source_initialize` / `source_check`: return the source/recipe/reader
   descriptor. Go independently checks the actual pinned owner and directories.
3. `source_begin`: accept generation, unpredictable nonce, profile scope,
   collector directory and the original deadline. Create an empty headless
   collector and return its PID/profile descriptor. No cookie import occurs.
4. Go validates the actual empty collector process, parent Job, image and leased
   directories before allowing `source_seed`.
5. `source_seed`: observe the reader, import only the fixed manifest into the
   disposable collector, then return fresh paired account/login/session proof.
6. `source_proof`: accept a fresh challenge nonce for each subsequent proof.
   Retain the original attempt generation and deadline.
7. `source_capture`: use the existing Instagram `captureIsolated`, parser and
   structured-media recovery. Go checks proof/ownership around claim and commit.
8. `source_end`: close owned media/Home targets and the disposable collector;
   independently confirm process exit before disposing its profile and pins.
9. `source_close`: join process-owner cleanup. An earlier uncertain cleanup
   remains uncertain, including on repeated close.

Proof DTOs contain source, native domain, recipe, nonce, generation, role, owner
and profile binding, authentication, observation time, and keyed account/session
digests. Credentials and raw account IDs do not cross this protocol. Profile
paths and PIDs remain private ownership data, not public diagnostics.

An exclusive source operation blocks ordinary transport mutations. Terminal
cleanup may interrupt the same source's pending startup; it cannot interrupt
another source. The exclusive lane remains held until both startup and cleanup
settle. Capture in a separate collector leaves ordinary reader operations
available. Late replies cannot publish a canceled attempt or retire a successor.

## Candidate Instagram recipe

`instagram-native-v1` implements a fixed same-origin request to
`/api/v1/accounts/current_user/?edit=true` from an owned Home document. This is a
candidate endpoint, not a claim that Instagram documents or guarantees it.

- Stream at most 64 KiB of one JSON response; project at most 4 KiB of account
  and authentication data. No raw response is returned or logged.
- Disable the owned page's network cache and bypass its service worker before
  each evidence read. Pin the main frame, loader and exact Home URL before and
  after the read. A changed document rejects the proof.
- Require a positive authenticated server projection, strict decimal identity,
  and agreement with `ds_user_id`. A signed-in-looking feed or cookie presence
  alone is insufficient.
- The candidate manifest contains only `sessionid`, `csrftoken` and
  `ds_user_id`, with exact allowed Instagram domain/path/security attributes.
  Required names on foreign scopes, duplicates, partitioned cookies, missing
  values and expiry are rejected. Ordinary cookies are omitted. No optional
  cookie is imported speculatively.
- Account and session comparison uses process-keyed digests. Reader/collector
  proofs must agree and remain unchanged; each proof is at most six seconds old.
  The original batch deadline is at most 60 seconds and is never renewed.

The native recipe is exercised with injected offline CDP fixtures. This does
not establish that the endpoint or minimal cookie manifest succeeds against the
live service. A future native QA plan must name its profile and precise read/
transfer limits before executing it. Native qualification and runtime activation
remain separate from implementation and offline regression results.

## Adding another native source

Implement its bounded evidence and session recipe, supply the private native
transport/owner port, and add its sealed factory and asset bundle. Reuse the
existing controller, ownership checks, Store permit flow, collector/parser and
cleanup kernel. Prove source/role/profile/generation isolation and late cleanup
with fixtures first. Qualify that source's actual evidence and manifest before
registering it as available; another source's consent never grants permission.

There is no new Store schema in this increment. Schema 31 remains the X/IG
downgrade fence established by the shared-core implementation.

## Offline validation checkpoint (2026-10-29)

- 120 Node tests passed across the native IG recipe/kernel, source registry,
  main transport, existing X daily protocol, synthetic IG collector, sealed
  package builder and development-package verifier. Native-provider cases use
  the existing `sourceAssets`, `captureIsolated` and IG video recovery over
  injected CDP fixtures, not a signed-in browser.
- Tagged Engine/appshell regressions passed for native/source/Instagram/X daily
  flows and the separate source wire. Cases include proof scope/role/nonce/
  account/revision/expiry, independently rejected ownership, Store rejection,
  generation/replay and retained cleanup uncertainty.
- Actual application packages compile under default, `auth_session_dev` and
  combined `auth_session_dev,auth_session_fixture` builds. This is compilation,
  not execution of native process, profile or UI tests.
- A bounded read-only Astra review identified and checked fixes for two
  integration defects: capture data using the tiny control-message decoder, and
  transport exclusivity blocking terminal cancellation during source startup.

The broader Node run passed 128 of 129 tests. The existing test
`manual requests remain concurrent for rendezvous, cancel and branded capture
diagnostics` in `scripts/x-native-owner.test.mjs:73` still fails with terminal
reply ID `2` instead of `4`. The identical failure was reproduced against the
immutable pre-change `c336d21` source snapshot; its test and owner protocol were
not modified in this increment. It remains an existing test issue, not a passing
regression claim or a native IG acceptance result.

The whole-repository `go test ./... -run '^$'` also discovers disposable baseline
source snapshots under `build/` that contain incomplete/mixed packages. The
successful compile matrix targets `./cmd/... ./internal/... ./credentialstore
./ui-reader-broker`, covering application packages while excluding those
disposable artifacts. Those artifacts were preserved.

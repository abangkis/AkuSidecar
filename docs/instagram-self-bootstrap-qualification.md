# Instagram self-bootstrap qualification

Status: discovery implementation and offline tests only, 2026-10-30.
No native identity source is qualified by this document or its fixtures.
The current offline budget/completeness contract is in the final section below;
earlier sections retain the historical limits and acceptance evidence.

## Objective

Assess a normal-page self source before changing the candidate identity endpoint.
The authorized sequence is passive candidate discovery, reader provenance and
freshness qualification, then independent collector qualification. Runtime
activation and real session transfer remain separate gates.

## Implemented discovery boundary

`instagram-self-bootstrap-probe.mjs` reads an existing caller-owned Home page.
It performs only two frame-tree reads and one bounded evaluation. It allocates
no target, navigates nowhere, invokes no page module, fetches no endpoint, and
reads no cookies. The caller retains target ownership and cleanup responsibility.
It is connected only to the development diagnostic transport and operator
endpoint described below, not the source registry or Store issuer.

The candidate grammar is deliberately unqualified: an exact four-element
ServerJS-style `PolarisViewer` tuple with no dependencies and an object carrying
direct `id` and `username` fields. This is a synthetic discovery grammar, not a
claim that the current Instagram page uses it. Other shapes return a closed
rejection rather than triggering recursive guesses for account identity.

Only `application/json` script nodes are inspected: at most 96 scripts,
256 KiB per script, 512 KiB total, 12,000 traversal nodes and depth 32. Duplicate
or conflicting candidates reject. IDs must be positive decimal strings of at
most 30 digits or safe positive integers; unsafe numbers reject. Author objects
from feed media cannot become self candidates.

Output has a fixed schema and reason vocabulary, booleans for candidate shape
and document continuity, and no raw ID, username, HTML, script bytes or tokens.
Output is at most 1 KiB. `serverFresh` and `qualified` are always false.
Document continuity does not establish server delivery or authentication.

## Reader qualification still required

First observe only candidate presence and schema on an authorized reader page.
If absent, do not invent another module/path or fall back to post authors.
If present, independently establish exact authenticated-self semantics and the
canonical Instagram ID; multiple ID namespaces must not be treated as equal
without evidence of their mapping.

For a proof source, bind the initial document or identity-bearing response to
the exact owned main frame, loader, origin, generation and observation interval.
Require actual server delivery, a successful response and complete bounded
parsing. Cached/service-worker data or an old document read now cannot acquire
a fresh authentication timestamp. An authorized ordinary page load or passive
response observation is needed when an existing page cannot prove provenance.

The pure `assessInstagramBootstrapDelivery` function is an offline review aid:
it rejects missing/stale delivery, uncertain document/origin/main-frame/cache
provenance and an unqualified schema. Caller-provided metadata, including an
eligible result, grants no authority. It is not a replacement verifier.

Only after qualification should an exact private parser produce account/login
evidence. Retain document checks, login/challenge rejection and private
corroboration with `ds_user_id`; cookie identity alone is insufficient.

## Independent collector qualification still required

Reader discovery does not authorize creating or seeding a collector. After
separate native qualification/transfer authorization, observe the same source
independently in the collector. Do not copy reader proof or its timestamp.
Reuse the existing source proof envelope and `verifyInstagramPair`, including
role, nonce, profile, owner, generation, strict age and account/session equality.
Do not create a parallel authorization or Store policy.

## Evidence and remaining gates

42 offline tests passed: discovery bounds, no network/module execution, private
projection, author exclusion, duplicate/conflict rejection, unsafe IDs, document
replacement for both roles, stale/cache metadata and the existing IG provider
and identity suites. Both-role tests use simulated existing pages. They do not
demonstrate a real module, reader authentication, or collector operation.

No production verifier, runtime, login, Auto Update or session was changed.
Remaining: authorize activation and one passive reader observation, then qualify
server provenance before any native collector trial. No authenticated
observation or transfer is included in offline results.

## Diagnostic transport and operator entry

The private operation is `main_instagram_bootstrap_diagnostic` with exactly an
ID and a 48-hex nonce. The existing diagnostic lane excludes public operations,
X and IG owner activity. Uncertain cleanup blocks subsequent diagnostics.

The operator endpoint is `POST /api/diagnostics/instagram-bootstrap`, requiring
the current `X-Aku-Split-Epoch` and exactly
`{"allowPassiveBootstrapInspection":true}`. The existing admission controller
requires the development build, an idle retained main-browser owner, both
maintenance holds and no source/cleanup activity. Reader HTTP diagnostics and
bootstrap diagnostics share the one-attempt-per-process counter and drain gate.
No automatic invocation, scheduler hook or transfer path is added.

The transport selects exactly one existing non-private page target whose URL is
`https://www.instagram.com/`. Missing/ambiguous selection rejects. It attaches
its own CDP session, pins the default unique execution context to the main frame,
and runs only the sealed bootstrap expression. It cannot navigate, create or
close a target or access cookie APIs. Cleanup detaches only its own session;
failed or unresolved detach never claims verified cleanup. Late attachment
replies retain a detach obligation. User pages remain open.

The fixed Go/Node projection adds `cleanup` to the discovery schema and rejects
unknown/duplicate fields, private identifiers, forged qualification/freshness,
and stale consent/epochs. Package preparation uses an isolated source snapshot
with explicit overlays, never modifies the running runtime, and is not activation.

Transport acceptance: 67 JavaScript tests and targeted Go tests in four packages
passed offline. This verifies the diagnostic wiring, not native source presence,
fresh server authentication, or collector qualification.

## First-cause and main-frame correction

Bootstrap wire projection now includes closed `failureStage` / `failureCondition`
and independent `cleanupStage` / `cleanupCondition` fields. Missing and ambiguous
Home targets, attachment failure, document/context change and evaluation failure
are distinguishable. The first operational cause is retained when detach fails;
a cleanup-only failure does not invent an operational cause. No exception text,
target/session/context identifier or page data is exported. Node and Go reject
unknown, missing, duplicate and inconsistent diagnostic fields.

The scope pins the exact main frame and loader before enabling Runtime replay.
Only that frame's default unique context can become the evaluation context.
Child-frame creation, navigation, context destruction and detachment cannot
replace or invalidate it. Main-document/context change and a global context clear
still reject. Scope cleanup detaches only its own session.

The operator response helper `scripts/instagram-diagnostic-response.ps1` reads
the API's flat `error` string. Its exception adapter can be used in the operator
catch block instead of the previous incorrect `error.code` lookup. Only an exact
closed API error code and HTTP status are retained; raw messages and details are
discarded. This cannot recover the historical code that was already discarded.
Loading or testing this helper makes no HTTP requests.

These corrections are offline changes only. The earlier live HTTP 409 predicate
remains unproven until a separately approved diagnostic yields the new fields.

## Bound classification and irrelevant-script analysis

The latest authorized read found a bound Instagram document, no operational
failure and verified cleanup, but returned the legacy `bounds` category. This
does not show that a self module is absent and does not identify the bound.

Offline follow-up preserves every existing limit and its enforcement order,
while projecting the following exact closed reasons in both Node and Go:

| Category | Existing boundary |
| --- | --- |
| `bounds_script_count` | More than 96 JSON script nodes |
| `bounds_script_chars` | More than 262,144 UTF-16 code units in one script |
| `bounds_script_bytes` | More than 256 KiB UTF-8 in one script |
| `bounds_total_bytes` | More than 512 KiB UTF-8 across scanned scripts |
| `bounds_nodes` | More than 12,000 traversal nodes across candidate scripts |
| `bounds_depth` | Traversal depth greater than 32 |
| `bounds_fanout` | Children exceed the remaining node/pending-work budget |

Legacy `bounds` remains accepted for compatibility. No lengths, script content,
account values or arbitrary diagnostic strings are emitted. The original
boundary priority and first-cause/cleanup separation remain unchanged.

Offline fixtures demonstrate an integration limitation: script length and total
bytes are checked before filtering for the exact candidate marker. A large
unrelated script, or unrelated cumulative bytes, can prevent discovery even if
a later script contains a valid synthetic candidate. Non-matching scripts are
not JSON-parsed, but still consume inspection budget. Existing fanout accounting
also conservatively counts child entries before queueing only object children.
These are observations about current accounting, not proof of which condition
occurred on the live page. This increment changes diagnostics only.

Before changing filtering/accounting, retain a separate bound on inspected data:
moving all budgets after candidate filtering would remove the cost bound for
non-matching input. A later change should distinguish inspection cost from
candidate parse cost with explicit budgets and exhaustive-scan semantics.
Early stopping must not report a unique candidate if remaining bounded input
could contain a duplicate/conflict. No such policy change is implemented here.

Validation: 73 JavaScript tests and targeted tagged Go tests in four packages
passed offline, including every precise bound, exact accepted size/count edges,
Unicode byte growth, irrelevant-input rejection and closed cross-layer catalogs.
The running package was not rebuilt, staged, activated or inspected again.

## Separate adaptive inspection and parsing budgets

Implemented offline after the budget design consultation. All trusted limits
are centralized in `instagram-bootstrap-budget.mjs`, included and hash-sealed
as a normal transport module. The page and operator cannot select a policy.
The expression generator accepts only validated policies within package ceilings
for offline fixtures; the restricted scope executes only the default expression.

| Resource | Initial allowance | Absolute ceiling |
| --- | ---: | ---: |
| JSON scripts inspected | 256 | 512 |
| Total UTF-8 bytes inspected | 8 MiB | 16 MiB |
| One script inspected | 4 MiB | 8 MiB |
| One candidate script parsed | 2 MiB | 4 MiB |
| Total candidate bytes parsed | 4 MiB | 8 MiB |
| Traversed object/array containers | 64,000 | 128,000 |
| Inspected child entries, including primitives | 256,000 | 512,000 |
| Structural depth | 64 | 64 |

These remain engineering defaults, not measured Instagram requirements. Each
allowance may grow once to its ceiling within the same synchronous document
snapshot. Accumulated work and ambiguity flags are retained; no scan restarts,
navigation, request, diagnostic retry or original-deadline renewal occurs.

Inspection counts all JSON-script data. Only scripts with the literal candidate
marker or a backslash consume parsing/traversal budgets. Backslash-bearing
scripts cannot be excluded because an escaped JSON string could name the module.
They are conservatively parsed in full. Arbitrary substring tuple extraction is
not used. The existing packaged strict JSON parser is reused; duplicate keys,
invalid Unicode and unsafe JSON numeric integers reject rather than silently
losing data. This conservative parsing can reject a candidate-bearing script
because of unrelated unsafe numeric fields; it is not a native qualification.

Every recognized tuple is traversed further. Duplicate/conflict flags accumulate
through all eligible scripts, with conflict taking precedence over duplicate.
Containers and enumerated child entries have distinct counters. The parser's
value ceiling is derived from their hard ceilings; parser depth/value limits
produce closed rejection categories. No candidate is accepted from a prefix.

`scanComplete` is true only for an exhaustive, successfully parsed/traversed scan
ending in `candidate`, `missing`, `duplicate` or `conflict`. All size, work,
deadline, invalid-input and document failures report false. Operational failure
cannot claim a complete scan. `budgetExpanded` indicates whether any soft
allowance grew. Neither flag grants identity authority; `qualified` and
`serverFresh` remain false. Node and Go require these fields and reject
inconsistent/missing projections; binary and transport must be packaged together.

The original cooperative deadline is 1,000 ms, inside the 2,000 ms evaluation
envelope. Checks surround script access, encoding, strict parsing and traversal;
a regressed/non-finite clock rejects. Synchronous DOM access, encoding and parser
execution are not preemptible. An outer timeout is not proof of stopped page
execution, so byte/work ceilings remain essential. Cleanup and first-cause
diagnostics keep their independent contracts.

New closed reasons include `bounds_candidate_bytes`, `bounds_parse_bytes`,
`bounds_parse_tokens`, `bounds_entries`, `scan_deadline` and `scan_clock`.
Legacy bound labels remain accepted with `scanComplete:false`.

Validation: 77 JavaScript tests and targeted Go tests in four packages passed.
Fixtures cover separated budgets, same-scan growth, ceilings, deadline/clock
failure after a candidate was found, escaped names, nested duplicates/conflicts,
duplicate JSON keys and forged completeness. Package tests verify that the
central policy is sealed. No native probe, session transfer, runtime staging or
activation was performed. Previous prepared packages remain unchanged.

## Closed structural rejection predicates

The current offline increment keeps `reason: shape` and adds the required
`shapeReason` field. Its only values are `none`, `tuple_arity`, `dependencies`,
`data_object`, `missing_id`, `missing_username`, `id_type`, `id_format`,
`username_type` and `username_format`. `none` is required for every non-shape
outcome; it is rejected for a shape outcome. Unknown labels, missing fields,
duplicate fields, fabricated qualification and inconsistent completeness reject
at the Node and Go boundaries.

Shape outcomes require modulePresent=true, scanComplete=false and all identity
shape flags false. A malformed candidate after an earlier valid candidate also
clears those flags; it must retain the structural reason rather than turn into
an invalid projection. Tests cover later scripts and nested malformed tuples.
The parser still requires the same exact candidate schema and strict identity
types/formats. The new labels diagnose the failed requirement without relaxing
it or exporting the observed value, arbitrary keys or script/body content.

The preceding live observation encountered a module without budget expansion
but returned only the older aggregate `shape` category. Its exact structural
cause cannot be reconstructed from the retained projection. No new probe was
run in this implementation slice, and the source remains unqualified. A later
approved package must promote matching Go binary and Node transport together:
older projections without shapeReason are intentionally rejected by this
updated closed contract. These diagnostics alone are not a production identity
verifier or a replacement for account/login pair qualification.

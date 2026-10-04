# Browser and headless collection roadmap

Status: architecture approved; phase 1 committed; runtime/session/media ownership
and reader guard committed as `b376438`; split-action leases committed as
`86cd0c3`. Source/login-window tracking is implemented in the paired Sidecar and
Bridge checkpoint. Local candidate implementations for phases 2c-5 now exist,
but acceptance is incomplete. Do not install or release this candidate yet.
Owner: AkuSidecar integration, with source extraction shared with AkuBridge.

### Native reader visibility and timing follow-up (2026-10-04)

Timeline scroll follow-up: background refresh now waits for 350 ms of scroll
idle both before acquisition and before applying a response. Explicit user
refresh/reveal stays immediate. Back-to-top collisions use predicted candidate
rectangles from one read phase rather than moving the button to test positions;
Related Context reuses the same card measurements. Side-pane position variables
are scoped to the pane/toggle instead of inherited from the document root.
Targeted tests cover rapid scroll, in-flight acquisition, geometry read/write
ordering, gutter fallbacks and scoped CSS writes. These changes still require
restart and live user scroll acceptance; no FPS improvement is yet measured.

Latest owner click after activation: API total 490 ms, broker total 958 ms,
profile handoff 400 ms, owner release 74 ms, Chrome launch 4 ms, reader readiness
247 ms, coordinator wait 0 ms, target preparation 34 ms and accepted HWND
activation 18 ms. Post navigation dispatch after activation took 28 ms. This is
one successful sample, not full social-content load or universal latency proof.

A subsequent reader-close trial exposed automatic Browser probes queued against
the hostless reader. Pending leases delayed auto-return and produced the generic
capture-runtime error. The runtime later recovered without intervention. The
follow-up blocks UI ping/source probes and passive media lookup outside a ready
Browser collector, checks admission again on the server, and retires only
read-only ping/source probes belonging to an exited reader generation. Actual
user actions retain completion and lease guards. A generation-scoped owner-exit
observer runs outside manager locks; structured skipped probe results are quiet
in the UI while unrelated errors remain visible. The misleading source-access
review button is hidden while reader-only ownership is active. This follow-up
was activated and passed the owner's subsequent real close/auto-return trial:
runtime returned to headless ready, pending false, active leases zero. This
readback verifies recovery, not the absence of every possible UI banner.
Validation: HTTP API, capture-runtime and coordinator Go suites pass; 35 targeted
frontend tests and the Windows Sidecar build pass. Tests cover admission without
leases, natural owner-exit cleanup, generation fencing, retained user-action
leases, actual UI probe guards and structured passive-skip handling.

The same trial exposed collection contention: two native-reader requests timed
out after 30 seconds waiting for profile handoff; a later request completed in
5104 ms (5000 ms handoff). The new UI wait guard displays “Menunggu koleksi
selesai” on native links while a hybrid session or collection profile lease is
active, and restores access after cleanup. A single in-flight opening displays
“Membuka native post…” and prevents extra helper/action launches. Poster/label
children are preserved. The isolated trusted-click broker ignores guarded links;
live runtime routing also refuses a busy collection owner before dispatch.
There is deliberately no deferred replay of an old trusted click: the user
clicks again after collection finishes. Browser-only collection retains its
existing native route; an existing hostless reader remains usable. These new
wait controls require a runtime restart and real UI validation.

Owner approved option 1 optimization after the real click measured API 4754 ms,
broker 5208 ms, profile handoff 3800 ms (release 2529 ms, launch 15 ms, readiness
254 ms), target preparation 918 ms (post navigation dispatch 833 ms), and rejected
activation. Those are pre-optimization measurements, not a passing reader trial.
Intent changes now wake the single coordinator loop immediately, retaining its
one-second lifecycle fallback and all profile/active-lease guards. Coordinator
wait is logged separately. Chrome shutdown cancels its losing exit timeout so
the timeout cannot keep Node alive after verified Chrome exit; shutdown deadlines
and full tree cleanup remain intact. The local marker is bound and activated
before dispatching social navigation; rejected activation never dispatches the
post, and the completion callback cannot replay navigation. Fixed activation
reason categories distinguish UI foreground change, binding changes, Windows
rejection and exchange failure without logging arbitrary peer text. Real
before/after click timings and foreground acceptance still require activation.
Validation: five affected Go packages and Sidecar/helper Windows builds pass;
30 routing/broker frontend tests pass; the full worker suite reports 71 passed,
one opt-in smoke skipped, zero failures. A subprocess regression proves the
losing shutdown timer cannot hold Node open. The immediate-wake regression
passes ten runs, and failed activation cannot navigate a social URL or replay it.

The deeper Astra consultation is recorded in `auth-session-reuse-design.md`.
It is a research proposal only; no credentials/state/profile have been copied.

The two-profile collector/reader session-copy proposal is deferred by the owner.
It must prove login reuse, restart persistence and token/session refresh before
adoption; no profile or authentication-state duplication is implemented.

The native-reader launcher no longer requests minimized startup. Private CDP
remains an inherited pipe under process-tree ownership, with normal readers now
eligible for the existing scoped binding/retirement guard. Timing logs contain
only generated action IDs, generation, stage, duration and success, never URLs
or post content. Measurements distinguish owner readiness/release, Chrome launch,
reader readiness, aggregate profile handoff, broker attachment, target attachment,
marker navigation, HWND binding, post navigation dispatch and activation wait.
Aggregate timings include their child stages and must not be summed with them.
Navigation dispatch is not proof that social content finished rendering.
Real click timing and foreground acceptance remain pending activation/trial.
Five affected Go packages, 30 broker/routing frontend tests and the standalone
Windows Sidecar build pass. The owner-approved opt-in Chrome 154 local fixture
passes: process launch 9 ms, marker/HWND binding 300 ms; the owned reader is not
minimized, holds profile ownership while open, and naturally exits after its
only local page closes, with process-tree cleanup verified. These are empty-profile
local timings, not real social-post or headless-handoff timings. The approved
Supervisor restart completed cooperatively, and the runtime is Headless ready
with a healthy database. A real owner click is pending to measure the full path.

### Scroll/render performance follow-up (2026-10-04)

Scroll requests now share one animation-frame queue for context tabs, back-to-top,
side-pane placement and automatic batch reveal. Related Context reads all card
geometry before updating tab attributes/classes and measures each anchor once;
non-Timeline views skip those card reads. Unchanged position styles, context
attributes, status pills and runner text no longer rewrite the same DOM values.
The 70 focused frontend tests pass, including burst coalescing, requests during
flush, geometry-before-write ordering, tab visibility and unchanged-value writes.
HTTP API tests and the standalone Windows Sidecar build pass. This additional
source delta is not yet activated; actual scroll/frame profiling and owner trial
remain pending. Database-health contention remains a separate follow-up.

### Timeline hydration performance follow-up (2026-10-04)

User approved fixing the 12-card/1000-item detail-loading path. Collapse/hide
now select page IDs and duplicate relations before hydrating only selected
items. Existing ordering, unique offsets, trailing duplicate reports, prepared
visibility, corrections and the 1000-candidate horizon are retained; show_all
is unchanged. The lightweight selector can still inspect up to 1000 identities,
but no longer hydrates their full evidence/AI/preference/memory projections.

Read-only comparison against the current database verified deep equality of
the complete 13-item response for a 12-unique-item request in three runs:
old Store read 518/492/485 ms, new 18/19/25 ms. These are Store timings, not
endpoint, rendered-frame or installed-runtime acceptance. Regression fixtures
cover pagination/duplicate boundaries and ensure unselected history evidence
is not decoded while selected evidence errors remain visible. User-approved
Supervisor restart completed cooperatively (forced=false); runtime is Headless
ready with X/Instagram/LinkedIn selected and Auto Update off. Three GET Timeline
requests measured 144/189/123 ms versus the earlier 1070-1179 ms endpoint
baseline. Hostless reader assets are served, but the real native-post click
journey has not been exercised. Global scroll/layout and database-health contention
remain separate follow-ups; this change does not certify whole-app smoothness.

### Combined headless trial and remaining reader host gap (2026-10-04)

Owner ran X/Instagram/LinkedIn together at 19:36-19:37 Jakarta time. Session
`session_368c6567a06f8f722ec4fd6f42c32077` completed; all three commands carry
Headless collector/driver stamps in the same epoch and generation 2.
Instagram captured 2/added 0, LinkedIn captured 4/added 2, X captured 5/added 3.
All runs completed without errors. This validates combined acquisition, not
Facebook mixed routing, matched-content parity, or every media format.

Owner reports native-post trouble and the Experimental capture host still
appearing. The previous modularization only changed the UI route: the active
Browser factory still launches `/split-capture-host`. A separate hostless
reader role is now implemented in source, pending runtime activation and
trusted-click trial. Native reader foreground/result receipts at 19:38-19:39 were
accepted, but those do not certify first-click reliability or page-content
readiness. The screenshot's `browser_handoff_required` error comes from the
passive ping/probe or explicit reload branch, not native-post dispatch. The
new relay treats only passive ping/probe requests receiving that exact 409 as
skipped after auto-return; reader/reload and other failures remain visible.

The user approved correcting repeated Node extraction. The shared stager now
caches only node.exe/LICENSE by pinned archive hash and version. It validates
the archive and derives entry hashes from it before trusting extracted files;
worker source files are copied fresh on every stage. Cold/warm/fresh-source/
corrupt-cache cases passed in project-only fixtures. Production restart is not
needed to use this script change; the next development rebuild invokes it.

A separately authorized local Chrome test passed: an empty profile, exactly
one local reader page, owned HWND binding through private CDP, profile retained
while the reader remained open, then natural process-tree exit after closing
only that fixture page. Receipt is under
`build/hostless-reader-smoke-*/receipt.json`. Production runtime was not stopped.
This does not certify authenticated URLs or a real trusted UI click.

The new `native_reader` factory role launches normal Chrome using the same
configured executable/profile, without Bridge or `/split-capture-host`.
Extensions are disabled only for this process, not removed or reconfigured.
The exact broker request must attach before native URL navigation. Private CDP
reuses only the unchanged local placeholder for the first post and creates a
separate window for subsequent posts. Retirement closes only an untouched
placeholder; post/source windows must close naturally before profile reuse.
Reader-only generations reject source/Browser-collection borrows and expose
`nativeReaderOnly`; Browser/Facebook use the existing Bridge route. Native
reader natural exit permits verified recovery into Headless using retained
source authorization. No new public collection setting is introduced.

Fake protocol, coordinator and broker tests cover page ownership, URL/request
correlation, independent repeated windows, denied activation, cancellation,
Bridge exclusion, live-reader retention and auto-return after natural exit.
Validation: seven affected Go package suites passed, 62 frontend tests passed,
and standalone Windows Sidecar/helper builds passed without touching runtime.
One existing Settings-storage wording assertion was already stale in HEAD;
it now checks the actual preview/active-retention messages without changing
retention behavior or adding cleanup controls.
Activation still needs a separately approved development restart, followed by
owner first-click/repeated-click and close-to-auto-return trials. No extension
preferences, logged-in profile data, database, defaults, commit or push changed.

### Current-profile trial follow-up: Settings and native reader (2026-10-04)

User approved fixing Settings scrolling and modularizing native-post opening
by the live Chrome mode. The served old UI deliberately spent the first
Headless click preparing Browser and requested another click; this was not a
completed URL-open attempt. The new router reads current runtime status and
selects a Headless handoff route or an already-interactive Browser route,
including Browser temporarily borrowed under a Headless preference. Both
retain the existing authenticated profile, typed reader transport, trusted
click broker and native lifetime tracking. No second click or full bootstrap
rerender is needed. Headless collection remains headless; this is an explicit
interactive reader action. The existing Bridge control host is still used.

Cold preparation is bounded to 30 seconds for the helper and native action;
the HWND activation capability remains 5 seconds and OS identity/foreground
checks remain in force. Early broker failure cancels routing before a delayed
status response can dispatch the URL. Settings uses an opaque sticky header
instead of backdrop blur and preserves unchanged polling text/classes.

Validation: 44 focused Node tests passed, including both single-click routes,
temporary Browser ownership, failed/unknown runtime, early broker rejection,
unchanged DOM writes, existing broker permissions and Timeline contracts.
Go readerbroker/httpapi/collection/appshell suites passed; the protocol test
rejects extended activation expiry despite the longer preparation window.
One explicitly approved Supervisor restart completed cooperatively (no force).
Read-only checks confirmed health OK and byte-identical served app/CSS/router/
render modules. Runtime returned to Headless ready with all four authorized
sources, LinkedIn selected, Adaptive Fidelity and Auto Update disabled. No
native post or Update was issued. Rendered scrolling and the real trusted-click
cold reader journey remain pending owner trial. Mixed
Update step 5 remains deferred by the user; no social action, mode/default
change, database reset, commit or push is part of this follow-up.

### Approved hybrid default direction (2026-10-04)

The user approves headless as the intended default for X, Instagram and
LinkedIn, with Facebook explicitly routed through the existing Browser/Bridge
collector until its headless gap is resolved. This supersedes the earlier
all-four-headless default gate and the global-only execution assumption below.
Facebook headless research remains parked. Facebook's fallback must use the
existing collector, not experimental hidden Quiet. Retain Browser selection
as a rollback path for all sources.

Implementation proceeds in three acceptance stages:

1. Source-aware routing and sequential batch ownership. Preserve one Chrome
   profile owner; never run headless and Browser simultaneously on that profile.
   Assign collector/driver before admission and keep claimed-command authority
   immutable. A mixed Update must drain its headless batch before borrowing
   Browser for Facebook, then return to the requested mode. Session progress,
   cancellation, recapture, permission retention and auto-update must remain
   truthful across driver generations. Separate collection borrowing from
   interactive login/reader lifetime; neither may retire the other's live work.
2. Product validation: mixed-source Update, Facebook fallback behavior, rendered
   Settings and actual trusted reader click, and complete package/runtime
   identity. Existing LinkedIn/Instagram lifecycle receipts are reusable within
   their documented development-Bridge scope, not complete package acceptance.
3. Default migration only after those gates pass, with rollback and preserved
   authentication. No current runtime default change, installation or release
   publication is authorized by this implementation stage. Broader fresh source
   media/text qualification remains required for a production readiness claim.

The initial implementation is committed and pushed as `6fedd47`, with the remote
`main` SHA verified, but is not yet accepted end to end. Changing
the Settings default alone cannot implement this policy. Validate the
ownership/admission boundary first; do not select a Facebook Bridge command
while the profile is pinned to a headless session.

The collection coordinator now distinguishes Facebook collection borrowing
from interactive login/reader borrowing. A nonblocking collection intent allows
the old session lease to drain before Browser acquisition, avoiding a
same-profile handoff deadlock. Collection package tests pass, including
independent interactive lifetime, idempotent release, source routing, and
waiting for the old lease before replacing the owner.

Settings now explains the Facebook exception and keeps its existing capture
visibility selector usable when headless is selected. Four collection-mode
Node tests pass. The actual isolated Settings card and renderer were inspected
with a fresh empty system-Chrome profile; labels and the Facebook batch status
render correctly. Receipt: `build/hybrid-settings-render-eedb64e2-abac-4d22-8668-a300b88db937/`.
This is rendered component evidence, not a full authenticated Settings journey.

Terminal Browser collection needs an explicit lease-bound Bridge surface
cleanup acknowledgement before releasing its ownership intent. The existing
background cleanup is asynchronous, and source-only release can retain a
managed placeholder window. Host-only retirement deliberately preserves
ordinary tabs. A cleanup acknowledgement may close Bridge-owned surfaces;
it never authorizes closing adopted user tabs or killing the profile owner.
At that initial checkpoint the mixed-session cleanup and auto-return gate
remained open; the successful scoped receipt below supersedes it.

New headless sessions now persist `hybrid_headless_v1` with source driver
assignments and stable execution ordinals: X/Instagram/LinkedIn first, Facebook
last. Browser sessions retain the legacy plan. Follow-up collector stamps remain
immutable even if visibility settings change. Focused store/engine regressions
pass for the persisted plan, predecessor reasoning drain, fresh Facebook source
access, wrong-driver/collector claim rejection, cancellation and partial-session
cleanup, and mismatched recovery. Three HTTP cleanup tests pass for exact owner
binding, late acknowledgement without replay, and rejected cleanup outcomes.

Commit `6fedd47` used the interim `facebook_browser_recapture_required` guard
under saved headless selection. The local continuation replaces that guard with
asynchronous Browser borrowing for Facebook Recapture, as detailed below.
Authenticated live Recapture qualification remains open and blocks default
migration. This does not retire explicit Facebook headless PoC fixtures or
claim Facebook headless readiness.

The operator fixture now offers `--mixed-update`: one real four-source Update,
deterministic local reasoning, Facebook Adaptive Fidelity in an isolated DB,
explicit terminal cleanup, and natural return to headless. It requires separate
mixed-Update, profile and foreground acknowledgements. Compilation and four
wrapper guard tests pass; the live test was skipped without authorization.
Read-only preflight confirms healthy registered runtime/Bridge, one exact
profile owner and existing grants for all four sources. It needs no old post
baseline and cannot establish media/text parity. At this initial fixture
checkpoint no live mixed Update had run; later receipts below supersede that
status.

The full Go collection/store/engine/HTTP suites pass after integration
(`go test ./internal/collection ./internal/store ./internal/engine ./internal/httpapi -count=1 -timeout=180s`).
Recovery regressions additionally cover the no-adopted-lease predecessor drain
fence and restoration of active and terminal-but-unfinalized Facebook Browser
holds. The staged worker package tuple is reverified; the Go fixture exercises
the current working-tree engine with that unchanged worker and registered
development Bridge. This is not a newly rebuilt complete installed package.
Cleanup rejection or an invalid acknowledgement keeps Browser ownership pinned.
The continuation adds a per-lease cleanup failure projection; Settings displays
the reason and keeps collection paused. A focused coordinator regression verifies
that one successful cleanup cannot hide another pending failure, and Settings
requests cannot erase it. Five collection-mode Node tests pass. The failure state
was rendered and inspected at
`build/hybrid-settings-render-438e2045-e10b-4081-bc62-87e09e4573d8/settings.png`.
This remains component evidence; the integrated live failure journey is open.
The same release action is not silently replayed.

Automatic Facebook Recapture borrowing is implemented in the local continuation.
Its waiting/admitted marker lets the frontend poll the existing job while the
engine owns Browser admission and dispatch. Waiting jobs are unclaimable and
receive their immutable runtime stamp only after fresh Facebook access and the
Browser owner are ready. The HTTP dispatch helper pins the exact generation,
retains pending actions through caller cancellation, and accepts only a matching
completed-job receipt without replay. Two focused HTTP tests and six Recapture
transport/UI Node tests pass, including preservation of the explicit foreground
retry offer. Full Collection and HTTP suites pass after callback integration.

Three focused engine lifecycle tests pass: admission/dispatch with cleanup-gated
auto-return, fresh permission revocation before admission, and terminal cleanup
recovery after restart. Cleanup remains durable as `pending` until acknowledged
and saved as `released`; failed cleanup keeps the Browser hold and exposes its
reason. The pump excludes historical jobs whose cleanup is already released.
These use controlled process/Bridge fixtures; they do not establish authenticated
Recapture parity, a live mixed Update, or a complete package/product journey.

Final local validation passes across all four changed Go packages:
`go test ./internal/collection ./internal/httpapi -count=1 -timeout=180s` and
`go test ./internal/store ./internal/engine -count=1 -timeout=300s`.
The Store regression also verifies that `released` cannot regress to `pending`
when a late pump write races with acknowledgement persistence. Eleven combined
collection-mode and Recapture transport/UI Node tests pass. `git diff --check`
is clean. This checkpoint contains the continuation after pushed `6fedd47`;
the installed default remains Browser. The continuation was committed and pushed
as `9be120f`; remote `main` matched its full SHA.

One explicitly authorized live mixed-Update attempt stopped during admission:
the new isolated DB had not completed onboarding, so the normal Update API
rejected it before collection qualification. The fixture's combined error/run
count assertion hid the rejection reason. The original runtime was restored,
the test profile owner released, and original settings verified unchanged.
Receipt: `build/authenticated-source-handoff-dd7428fe-f3e1-4d30-99cf-f8ff26543f91/`.

The local fixture correction completes isolated onboarding through the engine,
disables fixture calibration for deterministic batch qualification, and reports
admission rejection separately from a missing-source count. The full HTTP suite
passes after this correction; the operator test compiles and stays opt-in.
The user separately approved one retry. All four source runs completed with
persisted collector/driver authority and real captured blocks: X/headless 12,
Instagram/headless 6, LinkedIn/headless 11, Facebook/Browser/Bridge 10. The Browser
was generation 3 after initial headless generation 2. Auto-return remained pinned
until the six-minute bound because the cleanup ACK parser rejected the receipt.
Receipt: `build/authenticated-source-handoff-bac0a43b-2308-41ae-95e4-5cc8f7dc08c1/`.
The original runtime was restored, the test profile owner released, and original
settings verified unchanged. Read-only post-restore preflight confirms healthy
runtime/Bridge, one exact profile owner, four grants and no active session.

The cleanup contract defect is identified: the existing Bridge release handler
returns `{outcome: ...}` and the split client preserves that object under
`result`; the Sidecar parser and its mock tests incorrectly expected flattened
fields. The local parser correction reads `result.outcome` and still rejects
missing, mismatched, unverified or unsupported cleanup outcomes. Timeout/late-ACK
tests now use the real nested envelope. Focused cleanup tests and the full HTTP
suite pass. No Bridge code, user settings or permissions changed in this fix.
The user approved one further attempt. The corrected candidate passes the real
mixed Update: X/headless 12 blocks, Instagram/headless 6, LinkedIn/headless 14,
Facebook/Browser/Bridge 2. All runs completed with persisted collector/driver
authority and nonempty observations. Initial headless generation 2 drained before
Facebook Browser generation 3; acknowledged cleanup returned naturally to
headless generation 4 with no remaining collection ownership. The configured
source order and headless selection in the isolated DB were preserved.
Receipt: `build/authenticated-source-handoff-8354db7d-0196-4a76-9e87-64a57fe8bba9/`;
the Go operator test passed in 127.64 seconds. The wrapper confirms original
runtime restoration, profile release and unchanged original settings.

This closes the scoped authenticated mixed-session routing/cleanup/auto-return
gate with the registered development Bridge and staged headless worker. It does
not establish matched media/text parity, trusted reader click, actual login,
automatic Facebook Recapture, or a complete installed-package identity journey.
Next: build and verify the current complete local candidate, then qualify those
remaining product gates before default migration. No installation, publication
or current runtime default migration is authorized by this candidate build.

The complete local candidate was rebuilt at
`AkuBrowser/build/headless-hybrid-collection-20261004-8354db7d/AkuBrowser-0.9.0-windows-x64-installed-app`.
The canonical builder and `test-windows-installed-app-builder.ps1` both pass:
417 declared payload files, 589,081,749 bytes, exact production-app Bridge
identity, pinned Chrome for Testing 152.0.7977.54, worker/Node hashes and licenses,
DB schema and binary candidate probe. The Sidecar executable SHA-256 is
`7701d405672d2b9f31b8cdbb493419a111233e56ffd01899d6a123207ac45101`.
This is a working-tree candidate on Sidecar `9be120f`, explicitly marked dirty;
the local fixture/ACK corrections are included. Untracked experiments remain
outside the payload. The builder used the prior staged c2patool executable after
verifying its release-pinned hash/version because the default SharedTemp source
was absent; no download or installation was needed.

The existing isolated acceptance launcher passes PlanOnly for this candidate.
No full candidate UI has been launched. Next preparation is a bounded operator
bootstrap fixture with isolated empty profile/credential namespace, exact
production Bridge identity, paired runtime-stop/foreground opt-ins and verified
original-runtime restoration. This will establish bootstrap identity rather than
authenticated parity or a trusted reader click. Never run packaged CfT152 against
the registered Chrome154 authenticated profile; retain separate empty package
test data. Browser remains the installed default.

The cleanup correction and mixed-Update evidence are checkpointed as
`c938f433a4bf98a3a6eb616f118ca6ebd3aff642`; remote `main` is verified at that
SHA. The package above retains its original dirty-on-`9be120f` provenance;
committing the source does not rewrite or requalify that artifact.

Current acceptance summary (later evidence supersedes historical statuses):

| Gate | Current result | Remaining boundary |
| --- | --- | --- |
| Mixed-source routing, cleanup and auto-return | Passed authenticated development-Bridge fixture | Complete-package journey |
| Complete candidate payload and identity | Canonical build/static validation and isolated live bootstrap passed | Authenticated product journey |
| Settings | Rendered component and controlled tests passed | Integrated product journey |
| Login/source window lifetime | LinkedIn/Instagram scoped lifecycle receipts available | Actual login and trusted reader click |
| Automatic Facebook Recapture borrowing | Controlled lifecycle/transport tests passed | Authenticated end-to-end Recapture |
| Headless default migration | Not applied | Close product gates and qualify fresh media/text evidence |

User-led trial preparation (2026-10-04): the user requests a guided trial.
A new canonical package is built from clean detached source checkouts under
`AkuBrowser/build/hybrid-trial-20261004-clean/`, without copying untracked
experiments. Source commits are AkuBrowser `036d839`, AkuSidecar `e2de038`, and
AkuBridge `8dd4d6d`; the install manifest records an empty sourceDirty list.
Artifact: `AkuBrowser/build/hybrid-trial-20261004-clean/AkuBrowser/build/trial-package/AkuBrowser-0.9.0-windows-x64-installed-app`.
Canonical builder validation passes with 417 payload files (589110406 bytes);
Sidecar SHA-256 is `5bacefae3598742b4a2de1ae9f598c2575ac1172432212432d6196cd43bafef1`.
This is a new artifact, not a relabeling of the earlier dirty candidate.

The bootstrap operator helper now offers `--manual-trial`: read-only unless
the existing paired runtime-stop/foreground flags are supplied. After exact
package health/Bridge validation with zero initial grants, it permits up to
30 minutes of user-led interaction. A local `finish-trial` marker or candidate
exit ends the wait; timeout follows the same cooperative shutdown and verified
restore fences. Twelve guards pass outside the Windows inspection sandbox.
Read-only preflight passes with healthy original runtime, four grants and no
active session. No new candidate has been launched, no login/grant performed,
and no installation/default migration applied. Fresh foreground/runtime-stop
permission is pending for this user-led cycle. Login is required only because
this complete-package trial intentionally uses a new isolated profile; mode
switching within that trial continues to use its same authenticated profile.

The user deferred that isolated-profile opening and instead approved a trial
with the existing signed-in profile and current database after a backup. No
database reset is authorized or performed. `scripts/activate-current-profile-hybrid-trial.mjs`
validates the candidate tuple and the executing candidate's read-only database
inspection (current schema 29 equals target 29), defaults to preflight-only,
and requires paired runtime-stop/foreground flags for activation. It uses the
existing Supervisor registration, configuration, Bridge and capture Chrome
154.0.8037.93. The separate UI keeps its already-pinned CfT152 UI profile;
CfT152 never opens the authenticated capture profile.

Live activation passes: after verified Supervisor stop, zero capture/UI profile
owners and no port listener, the database and existing companions are copied
and hash-verified. Original selected dev runtime components are retained;
the verified candidate executable, adjacent headless worker and reader broker
are staged into the existing registered dev output. Supervisor starts normally
without registration/configuration/Settings edits. The candidate reports healthy
compatible Bridge with all four original grants, unchanged Settings, no active
collection session and `headlessAvailable=true` with all four supported sources.
Requested/effective mode remains Browser, generation 1, with no active leases.
Backup/receipt: `build/current-profile-hybrid-trial-dc170dfa-e0b1-4105-984a-6178092b3427/`.
The SQLite backup is 156540928 bytes; DB schema remains 29. The supervised dev
candidate stays active for user-led Settings trials, with no automatic trial
timer, no installation and no default migration. This establishes activation,
not a completed manual Update, trusted reader click or collection parity.

User-led X trial checkpoint (2026-10-04): the user ran a Browser baseline,
then selected Headless through Settings and ran another manual Update. The
persisted Browser command for `run_ea6260081df8bd102089d813aedc3acc` is stamped
bridge/browser generation 1; the Headless command for
`run_92c65aeb16b565514c72bd407f4fe0c0` is stamped headless/headless generation 2
in the same epoch. Both sessions complete without a run error. Browser captured
5 candidates in a 55.783-second total session and added 4 items. Headless captured
4 candidates in 12.344 seconds: 3 unchanged native resurfaces skipped, one
evaluated but not selected because of prior knowledge overlap, and 0 added.
The four unique Headless blocks contain text; two contain image media. These
are unaligned feed windows with zero shared native post IDs, not matched parity
or a controlled speed comparison. Both expose partial bounded-viewport coverage
and degraded capture-performance summaries. Video quality remains untested.

The user reported sluggish Settings scrolling and authorized the first focused
fix. Global scroll work now gates Timeline side-pane/content-context work to
the Timeline view; the layout scheduler, its queued callback and tab-layout
function independently return before reading hidden Timeline DOM on other
views. Automatic next-batch scroll detection is also Timeline-only. Four
inactive-view behavior probes prove no Timeline DOM/window work, and a queued
frame followed by a view change exits safely. Fourteen existing related-context
tests and syntax/diff checks pass. The blur CSS is unchanged. Canonical dev
restart activates the embedded frontend; served app.js equals the updated source.
Saved mode remains Headless, X-only, Auto Update off, generation 2 ready with
zero active leases and all four source permissions retained. No fresh login,
database reset or default migration occurred. The user-visible improvement
still needs operator feedback; no measured frame-rate improvement is claimed.

The next operator fixture is `scripts/test-installed-hybrid-bootstrap.mjs`.
It defaults to a read-only plan and requires paired `--allow-runtime-stop` and
`--allow-foreground` flags for a live run. Its isolated LOCALAPPDATA, capture/UI
profiles and credential namespace are newly allocated under AkuBrowser/build;
it copies no authentication or grants. Candidate readiness is bounded to 120
seconds. Cleanup uses only the verified candidate Sidecar's cooperative shutdown
endpoint, with no forced-stop fallback; both profiles, launcher/Sidecar and TCP
port ownership must clear before original-runtime restoration. Original settings
are compared by digest. Controlled guards cover approval pairing, restore order,
blocked restoration, failed stop issuance and settings mismatch. No live package
bootstrap has been run.

Detailed preparation identified an origin-format difference: the server
canonicalizes heartbeat origins without a trailing slash, while package/config
origins include it. The fixture now normalizes these two exact-origin forms and
still rejects another extension ID, a path or query. Nine guard tests pass.
If an older heartbeat omits origin, original-runtime comparison explicitly
records limited configured-only evidence; the new candidate still requires an
actual production origin match.

Default read-only preflight passes after the causal correction: the registered
runtime/Bridge are healthy and compatible, one exact authenticated profile owner
matches the registered Chrome executable, four source grants remain, and no
session is active. Heartbeat and configured identity match. Both stop/launch
flags remain false; no test profile or acceptance directory is created. The live
package bootstrap now needs a fresh, separately scoped foreground/runtime-stop
approval because the preceding mixed-Update permission covered one completed run.

The user then authorized one package bootstrap cycle. Initial preflight paused
for an active original collection session; after it completed, one actual
stop/test/restore cycle launched the candidate with an empty isolated profile.
The helper returned `candidate_shutdown_unverified` / drain timeout and held
restoration. Follow-up inspection found no candidate process/profile owner or
port listener remaining. Original Supervisor runtime was restored and verified:
healthy compatible Bridge, one registered Chrome profile owner, four source
grants, no active sessions and unchanged original settings digest.
Receipt: `build/hybrid-package-bootstrap-20261004-attempt1-receipt.json`.

The causal fixture defect is Windows listener-query semantics: filtered
`Get-NetTCPConnection -LocalPort ... -State Listen -ErrorAction Stop` throws on
an empty match, so a free port was treated as unavailable inspection. Successful
unfiltered enumeration followed by filtering now distinguishes empty results
from failure. Ten guards pass, including a real local TCP listener followed by
empty-listener verification. No forced-stop fallback was added. The helper also
retains the bootstrap projection/failure separately before cleanup; the previous
run lost that primary result, so package bootstrap acceptance remains unknown.
One fresh live retry requires separate authorization; no installation, login,
grant, posting or default change occurred.

The user separately authorized one retry after the listener correction. That
retry passes (`status=complete`, exit 0): actual candidate health is healthy
production-installed-app 0.9.0, the Bridge reports the exact production-app
origin `chrome-extension://piijgldhnhknddmiljookikjpheiiddn/`, compatibility is
true, and the fresh isolated profile has zero grants. Settings projection is
valid. Cooperative runtime shutdown completes, both candidate profiles and its
process/listener ownership clear, and original runtime restoration is verified
with unchanged settings digest, healthy compatible Bridge, one registered Chrome
profile owner, four source grants and zero active sessions.
Receipt: `build/hybrid-package-bootstrap-20261004-attempt2-receipt.json`;
isolated package data is under
`AkuBrowser/build/headless-hybrid-bootstrap-1791106215158-ada9a1e1-f6fc-4f2d-9500-f0a949ba92fb`.

This closes complete-package bootstrap/production identity and cooperative
stop/restore qualification for the frozen candidate. It does not close an
authenticated package capture, integrated trusted reader/login, Facebook
Recapture, or fresh media/text parity. No install, grant, login, publication or
default migration occurred. Next product qualification should exercise automatic
Facebook Browser Recapture and the reader/source-window journey while retaining
the headless selection; use scoped operator approval for any new foreground
cycle and never reuse packaged CfT152 on the Chrome154 authenticated profile.

Next preparation: an opt-in authenticated Facebook Recapture fixture. Acquire
one fresh eligible Facebook media item through the existing Browser fallback
with the isolated DB still configured as headless; wait for cleanup and return
to headless, then call the normal timeline Recapture HTTP route for that exact
item. Verify the job's immutable Browser/Bridge stamp, target identity, terminal
media evidence, cleanup and a second return to headless. No old target baseline
is required. If fresh acquisition yields no supported media target, report that
limitation rather than inventing a target or accepting an empty result. Keep
the original registered profile/runtime/settings restoration fence and scoped
foreground opt-ins. This is fixture preparation, not live qualification.

Preparation is implemented as `--facebook-recapture` in the existing operator
wrapper, with a new opt-in Windows test. Five Node guards, the focused Go fresh
target/identity/media guard, Windows compile-only build and diff checks pass.
The target must match its newly created timeline session/run and that run's raw
observation EvidenceKey, permalink and opaque Facebook platform ID. Watch and
photo-only routes are excluded from this scoped test. A video poster cannot
pass the direct media/playback URL check. Saved mode remains headless; the
isolated acquisition uses Facebook-only Adaptive Fidelity with bounded budgets.
There is no video playback or CDN HEAD request in this fixture.

Read-only preflight passes after an active original session finished: healthy
registered runtime/Bridge, one exact Chrome154 profile owner, retained Facebook
grant and no active session. Bounds are explicit: acquisition 360 seconds,
Recapture status wait 75 seconds, overall Go fixture 480 seconds (setup and
cleanup included). The wrapper retains a separate restore allowance. Runtime
testing has not run. A new live cycle needs scoped approval for one fresh
Facebook Update plus one eligible Recapture, temporary runtime stop, possible
foreground windows and verified restore; the package bootstrap permission was
limited to its completed cycle.

The user approved one authenticated Facebook fixture cycle. One fresh Facebook
Update completed through Browser/Bridge with durable route evidence and returned
to headless generation 4. The fixture then found no newly acquired target that
met all eligibility fences (same session/run and raw observation identity,
unavailable media, supported route); it did not request Recapture. The operator
test therefore fails as unqualified sample coverage, not as an observed Recapture
failure. The test ran for 87.65 seconds. Receipt:
`build/authenticated-source-handoff-75adc8c4-dcc9-4c24-8729-d1de428601ca/`.
The wrapper verifies original runtime restoration, profile release and unchanged
original settings. Automatic Recapture's live gate remains untested.

Next decision: keep requiring a naturally unavailable fresh item (which needs a
new suitable sample), or qualify ownership/dispatch with a controlled missing
media state on a freshly acquired item in the isolated fixture DB. The latter
would retain the real target, source observation and native identity, modify only
the copied test media state, and exercise the actual authenticated Bridge/API
Recapture path. It would prove live borrowing/dispatch/cleanup, not a naturally
occurring media-recovery symptom; report those scopes separately. No controlled
state mode or second live cycle has been authorized or implemented yet.

### Current priority amendment (2026-10-03): defer Facebook

The user changes tactics after the accumulated Facebook investigation effort:
finish headless for the other sources first. This supersedes the execution
order below that required X/Facebook acceptance before Instagram/LinkedIn.

1. Close the remaining X gates: repeat multi-image evidence, continuation and
   reliability, usable media in AkuBrowser, and the authenticated interactive
   handoff/auto-return journey.
2. Implement and qualify Instagram and LinkedIn headless collection against
   their existing Browser collectors, preserving authentication, post/media
   ownership and truthful unsupported/partial states.
3. Park Facebook investigation and new Facebook qualification runs. Preserve
   the implemented optional path, diagnostics, tests and existing evidence;
   do not silently declare parity or remove the Browser fallback. Resume only
   when the user chooses to return to it.

Shared worker packaging, profile ownership, Settings, reader/login and
auto-return work remains relevant to all sources. A Facebook-specific gap
does not block progress or qualification of X/Instagram/LinkedIn.

Browser remains the default. This change authorizes the new implementation
priority and checkpoint commit/push, not installation or default migration.
The earlier all-four-source default gate remains until separately revised;
Use Browser mode when Facebook collection is needed while the other sources
are being qualified. Collection mode is still global; this amendment does not
introduce simultaneous per-source Browser/headless operation on the same profile.
Historical Facebook next-step proposals later in this ledger are parked.

### Instagram/LinkedIn implementation boundary (2026-10-03)

The user now authorizes implementing both additional headless sources. Reuse
the existing source adapters and keep native source identity, author/body,
media ownership and permission gates. Enable owned headless source contexts,
source asset loading, capture homes/targets, observation URL/ID validation,
backend capability/authority, Settings messaging and read-only QA selection.
Borrowed hidden Quiet remains limited to X/Facebook; do not expand that parked
mechanism. Source capability is not parity certification. Browser stays default.

Initial DOM extraction must preserve owned visible media, presentation and
LinkedIn attachments while marking unresolved video streams/partial text as
unknown. No social action, sustained playback, login reset or blanket media
attribution is authorized. Real profile QA uses the existing guarded headless
stop/test/restore route; a new foreground baseline requires scoped approval.
Saved Browser observations older than 30 minutes are supplementary evidence,
not fresh paired qualification. Only source acceptance evidence can close the
new Instagram/LinkedIn gates.

First implementation checkpoint: backend supported-source/retained-authority
gates now admit IG/LinkedIn only with source readiness, permission and registered
script. Owned Chrome source contexts and default feed URLs are extended; hidden
Quiet remains unchanged. Observation validation requires exact shortcode or
LinkedIn native URN agreement, and retains new-source attachments/links/direct
context. The authenticated QA tool adds explicit additional-source qualification
scope and selection without requiring a Facebook baseline; historical X/FB
baseline requirements remain intact. Settings labels all implemented collectors
experimental rather than claiming source acceptance.

Focused Node contract tests and full Go collection/engine/HTTP tests pass.
Privacy-safe saved Browser baseline from session_b907b90c39264916712362639d810c6a
contains two IG and three LinkedIn blocks. It is now over 30 minutes old and can
only support initial regression evidence, not fresh paired parity.

Canonical worker staging inspection also found the dev helper previously
passed runtime/dev to a release stager that only permits build/artifact roots.
Dev build now stages in a unique project build directory, promotes the staged
assets with retained previous directories, and restores previous candidate
assets if promotion fails. Isolated tests execute both actual filesystem
promotion blocks; both pass. No live dev build/restart or installation has
been performed for this amendment.

Initial packaged authenticated evidence (2026-10-03): the canonical candidate
build and payload verifier pass. Worker tests pass 59 with one optional skip;
the new-source asset test now executes the actual Bridge scripts, rather than
only checking their filenames. This exposed and fixes the mandatory bounded
capture policy dependency before media-post-processor initialization.

Guarded headless target and feed runs used registered Chrome 154 with the same
logged-in profile, not packaged Chrome 152 against that newer profile. Every
run released the worker/profile and restored one original Chrome owner with
healthy compatible Bridge. Instagram feed captured one native post with text
and image (repeated across two snapshots); its saved native target was empty
with authenticated UI present and no discovered candidates. LinkedIn target
captured the matching native ID and 613-character body; expected video remains
unresolved. LinkedIn feed rejected two visible candidates lacking identity;
the existing adapter has bounded menu/embed permalink recovery that the initial
headless wrapper does not yet invoke. Native target binding and reuse of that
read-only recovery are the concrete next fixes. No source parity gate is closed.

Private receipts remain ignored under build/authenticated-parity-1e6b7634-e369-
490d-bc5d-0593590d8af7 and build/authenticated-parity-d79d4b93-e98e-4c24-bdb1-
1e55afeabdd6. They are supplementary saved-target/feed evidence; not fresh
paired Browser/headless acceptance. The operator harness now supports feed-only
qualification to avoid repeating unrelated target failures, rejects conflicting
feed-only/targets-only flags, and reports privacy-safe failure-stage labels.

Combined causal fixes and final target evidence: headless now reuses LinkedIn's
bounded own-post menu/embed permalink recovery and dedicated main-world media
reader. Instagram uses its dedicated media reader, plus exact native-shortcode
structured target fallback when no article is rendered. The latter requires
the actual navigated URL, one matching candidate, canonical native ID, bounded
author/caption and source-safe media. It is explicitly labeled structured
target evidence rather than visible-DOM extraction. Post identity conflicts,
foreign CDN URLs and duplicate structured candidate bindings remain rejected.

Authenticated combined receipt build/authenticated-parity-77957cc2-aa4b-438c-
a255-b2ef59d8615d captures both feeds, including LinkedIn owned playback URLs.
Final target receipt build/authenticated-parity-15f7024c-97a5-41d3-8092-
12ad96bab916 captures both saved targets: IG exact native ID with 563-character
caption and image; LinkedIn exact native ID with 613-character body and video
playback URL. Capture resamples missing expected media for up to three seconds
within the source/deadline budget; no intentional video playback is performed.
All runs release the worker/profile and restore one original owner and healthy
compatible Bridge. Worker tests pass 63 with one optional skip; comparison tests
pass 20. Full Go collection/engine/HTTP evidence remains valid (Go unchanged).

The saved-target runs above remain supplementary. The fresh qualification
checkpoint below supersedes their pending baseline/continuation status, without
closing overall source parity or integrated product acceptance.

### Fresh Instagram/LinkedIn qualification checkpoint (2026-10-04)

One explicitly approved Browser Update selected Instagram/LinkedIn and Adaptive
Fidelity temporarily. Session session_fcfcbb732c7d7dce9bb884b0eba0c7b6 completed;
Quiet and the original four-source selection were restored. Browser remains
the default. One LinkedIn observation without native identity was excluded from
target comparison rather than assigned a fabricated ID.

The current packaged worker fixes source-container scrolling and frontier
restoration, reuses bounded LinkedIn own-post permalink recovery, preserves
owned DOM images independently of legacy recovery selectors, and excludes UI
expansion-button text from the post body. Instagram caption enrichment requires
the same native shortcode, canonical URL, author and compatible visible prefix;
foreign, ambiguous or conflicting structured candidates remain rejected.

Fresh paired receipts build/authenticated-parity-3d832183-0b97-43fd-8e80-
c879f8ee2dbf and build/authenticated-parity-844aa04e-2207-43d5-a8f0-
5712d643c1fb used the same baseline and candidate within 30 minutes. Both sources
passed feed, eligible continuation and native-target capture; the additional
LinkedIn long-text target also passed. Three native targets match identity,
author and normalized text: Instagram 576 characters, LinkedIn image post 430,
and LinkedIn long text 2257. Repeated snapshots are not unique-post/media counts.

Instagram's compared image has the same CDN host/path with different signed
query values. LinkedIn's compared image retains the same underlying asset
identifier but uses an observed 800 rendition versus Browser's 1280 rendition;
exact URL/path and image-quality parity remain unverified. Do not synthesize a
larger URL or relax comparisons to conceal this difference.

Owned video URLs were extracted for both sources without intentional playback.
After explicit approval for two CDN HEAD requests, both returned HTTP 200 and
video/mp4 metadata. Instagram uses the fresh receipt; LinkedIn video remains
supplementary saved-target evidence. HEAD metadata is not playback or rendered
player verification. Private signed URLs and receipts remain ignored under build/.

Worker tests pass 67 with one optional skip. The canonical quality-20261004
candidate package verifies 417 payload files. Authenticated QA used registered
Chrome 154 and the existing logged-in profile; packaged Chrome 152 was not
opened against that newer profile. Every guarded run released worker/profile
ownership and restored one original Chrome owner with healthy compatible Bridge.

Remaining gates: broader fresh content/media diversity, rendition quality,
timestamp fidelity, repeatability and the integrated reader/login/auto-return
journey. No installation, default migration or new commit/push occurred here.
Budget remains the authorized one-million cap with only partial measurement;
the last native cutoff was 326381 tokens and subsequent usage is unmetered.

### Resolution diagnostics after checkpoint 5500cb4 (2026-10-04)

The Instagram/LinkedIn implementation checkpoint was committed and pushed to
AkuSidecar main as 5500cb4266d467bf369cb22c5bf5e8dcc4543a3b; remote SHA matches.
Existing experiments and private build receipts were not included.

Static inspection finds both collectors prefer browser-selected currentSrc.
Bridge also parses srcset as a fallback after currentSrc/src; neither collector
ranks LinkedIn rendition tokens or synthesizes a larger URL. Headless uses a
1280 by 900 viewport at device scale 1. Viewport/layout/source state remain
possible causes; static inspection alone cannot establish their causal effect.

The worker now records bounded admitted-image diagnostics: rendered dimensions,
browser density-adjusted intrinsic dimensions, device pixel ratio, whether
currentSrc differs from src, and whether srcset exists. Extra signed URLs are
not copied to these diagnostics; foreign, profile and comment media remain
excluded. The packaged resolution-20261004 candidate passes 68 worker tests
with one optional skip. Receipt build/authenticated-parity-7e12e714-8272-4a64-
b6cb-a97d8dacdc6c captures the LinkedIn image target and restores runtime/Bridge.
It reports rendered 550 by 551, intrinsic 640 by 640, device pixel ratio 1,
srcset present and currentSrc different from src, despite URL rendition token
800. The token alone is therefore not an actual image-resolution measurement.
This receipt uses a baseline over six hours old and is supplementary only.

Comparison output now derives each target's baseline freshness from actual
observation and capture times rather than trusting a saved freshness label.
Missing times remain unknown, backwards time is invalid, and captures beyond
30 minutes are supplementary. Focused comparison tests pass 21.

One further explicitly approved Browser Update completed as
session_e4ea962f4034b3a95681fadf44d28b28 and restored Quiet plus the original
source selection. Its fresh baseline contains one Instagram image post and
two LinkedIn native targets, including a five-image promoted post. No targets
needed exclusion. Receipt build/authenticated-parity-21dac70c-37cc-476f-9ebe-
5246cac7cdf1 passes six feed/continuation/target captures; Instagram and the
LinkedIn text target match native identity, author and text within two minutes.
The promoted five-image post was not observed in the bounded feed; this is
feed-selection variation, not proof that the source lacks it.

Direct receipt build/authenticated-parity-cd924d35-0513-4b57-a68a-6c291f4d361f
binds that five-image post and matches author/text, but initially retains three
images versus Browser's five. Inspection identifies a scope mismatch: headless
requires each image to intersect the viewport while Browser reads rendered
images throughout the owned post. The new worker keeps the native post visibility
gate but admits its rendered own images below/outside the viewport. Hidden,
profile, comment and foreign media stay excluded; video handling is unchanged.
Boundary tests explicitly reject an entirely offscreen post and hidden/comment
images. The owned-images-20261004 candidate builds; worker tests pass 69 with
one optional skip. Receipt build/authenticated-parity-6304ad30-43a4-48b7-
89cf-f83ea18ee7ad confirms the correction on that same fresh target: five of
five images, exact media URL sets equal, native identity/author/text equal,
approximately four minutes after Browser observation. Runtime was restored.
This closes the demonstrated viewport-related missing-image regression for
this sample, not broad rendition quality or overall source acceptance.

Final same-candidate regression receipt build/authenticated-parity-b19fb1ca-
4492-40ca-b4d6-e108876dee64 passes six feed/continuation/target captures across
Instagram and LinkedIn. The Instagram target and LinkedIn text target match
native identity, author and normalized text within six minutes of the same
Browser baseline. Together with the preceding five-image target run, these
provide three fresh matched targets on the owned-images candidate. Instagram's
signed image URL differs, while the LinkedIn five-image target has exact media
URL equality. Do not aggregate repeated snapshots as unique assets or combine
this with earlier candidate receipts as a single immutable payload proof.
Each run confirms worker exit and runtime restoration; final read-only preflight
confirms Browser/Quiet, original four sources and compatible granted Bridge.
The diagnostics/fix/freshness checkpoint was subsequently committed and pushed
as 05d2e3c8f931f4c49285586346b6fe70410f61fe; remote main SHA matches. No
candidate installation or default migration occurred.

### Authenticated source-window journey fixture (2026-10-04)

The next gate is a real-source interactive lifetime journey rather than another
static popup smoke. The new operator wrapper reuses the authenticated parity
harness's registered-profile, permission, owner-drain, package and restoration
guards. Its default is read-only preflight. Execution requires separate explicit
runtime-stop and source-window foreground flags; both acknowledgements are also
required by the opt-in Go fixture. The registered Chrome executable and selected
subprofile are reused; packaged Chrome 152 is never substituted for the newer
authenticated profile. The fixture's database is project-local and separate.

The fixture requests headless through the isolated Settings API, requires actual
Bridge Ready/PermissionGranted/ScriptRegistered evidence without substituting
source readiness, captures a native target, and dispatches real Bridge
open_source through the product HTTP action route. Successful actual HWND
preparation must hold auto-return after dispatch leases drain. Closure is limited
to one new single-page source window distinct from the capture host; old,
foreign, changed or ambiguous targets are rejected. After closure, headless must
return and capture the same native target again. No source login submission,
social action, extension API invocation by CDP or production settings write is
part of this fixture. Internal private CDP stays within app-shell host bootstrap
and the owned fixture's window inventory/close operations.

Compile/default-skip, source-window discovery/close and retirement-capability
guards pass. Three Node operator approval/identity/scope tests plus the existing
21 comparison tests pass. The user approved one LinkedIn cycle. Initial receipt
`authenticated-source-handoff-827aaadd-036b-4b62-a8d9-898504939f68` stopped at
missing compatible heartbeat, before headless or opening a source window.
Diagnosis verified registered Google Chrome, whose branded build ignores
`--load-extension`, and distinct candidate/development Bridge manifest IDs.
The profile has the development ID installed; the fixture originally trusted
the candidate ID. The wrapper now verifies the registered manifest origin/path
and installed unpacked-profile identity before stopping anything. This qualifies
registered development Bridge plus staged candidate worker, not full packaged
Bridge identity/parity. The candidate payload verification remains separate.

The causal identity correction passed preflight. Receipt
`authenticated-source-handoff-cd396299-e565-4cf7-8217-3c2e33d9c96f` then established
healthy compatible Bridge and persisted isolated headless Settings, but the
runtime correctly held generation 1 Browser: host-only retirement was not
negotiated. No source window or auto-return capture ran. Both receipts verify
profile release, restored healthy runtime/Bridge, and unchanged original Settings.
This is a transport capability gate failure, not source media/auto-return failure.
Stale loaded Bridge code is a hypothesis; a compatible heartbeat alone cannot
prove fresh capture-host bootstrap. The fixture now checks negotiated host-only
retirement before changing isolated Settings and reports safe capability booleans
after a bounded wait instead of spending the whole journey timeout there.

An optional `--allow-bridge-reload` flag requires both runtime/source-window approvals,
clears inherited reload opt-ins, and permits exactly one product `reload_self`
control action only when initial host capabilities are absent. It then requires
fresh negotiation; it does not force takeover, downgrade Chrome, install an
extension, grant source access, or inject an extension API through CDP. The native reader
trusted-click, actual login, rendered Settings and installed-product gates remain
open; no acceptance gate is closed by these failed lifecycle attempts.

Authorized reload cycle continuation (2026-10-04): receipt
`authenticated-source-handoff-6c1df501-0896-4cd9-90f7-98244825162d` rejected the
fixture's synthetic reload action ID. No extension reload or source window
completed. The helper was corrected to create and claim the existing product
maintenance action, relay its exact ID, and verify the expected post-reload
heartbeat. A browser-free integration test against the real maintenance and
split HTTP routes now exercises action acceptance and result completion.

One causal retry, receipt
`authenticated-source-handoff-c1ec7dfa-e5ac-42e6-a151-b08007830b9b`, completed the
authorized single reload. It then passed initial headless generation 2 and
same-native-ID LinkedIn capture, Settings requested headless, actual Bridge
source-window preparation and borrowed Browser generation 3. It stopped at
source-window target discovery before the scoped close/auto-return assertions.
Both receipts verify original profile release, runtime/Bridge restoration and
unchanged original Settings. No post-close capture, reader click, fresh paired
parity or installed-package gate is claimed. This consumes the one source-window
cycle; no further foreground cycle has run.

Inspection shows Bridge acknowledges `tabs.update` before navigation commits;
the fixture's immediate source-host inventory could still see its loopback
intent page. The receipt's old generic failure did not retain the exact guard
reason, so this is a plausible timing cause, not a confirmed live diagnosis.
The fixture now binds the exact action's intent target during actual native
preparation, waits at most eight seconds for source navigation, and requires the
same target ID. Only absent source navigation is retried; ambiguity, unexpected
targets or shared windows remain terminal. Browser-free tests cover delayed
commit, mismatched prepared identity and no accidental closure. Five focused Go
guard/integration tests pass, with the opt-in live test skipped. A new one-cycle
source-window approval is needed to validate this correction; another automatic
reload is not planned.

LinkedIn authenticated lifecycle gate passed (2026-10-04): receipt
`authenticated-source-handoff-2443de01-1322-4a60-8aa6-3a2bd51710d5` ran the new
explicitly approved cycle without Bridge reload. Generation 2 headless captured
the exact native target ID; real Bridge open_source borrowed generation 3 and
prepared its native source-window lifetime. The exact action-bound target
committed its source navigation, occupied a unique single-page window separate
from the capture host, and was revalidated before close. Its live HWND blocked
auto-return after action leases drained. Closing only that target permitted
natural generation 4 headless return; the second capture preserved the same
native post ID. The opt-in live Go test passed in 24.51 seconds; wrapper exited
successfully and verified profile release, healthy runtime/compatible Bridge
restoration, and unchanged original Settings.

This closes the bounded LinkedIn source-window API lifetime/auto-return gate
for registered development Bridge plus staged candidate worker. It is not
trusted reader-click, credential login, rendered Settings, complete packaged
Bridge, or fresh paired media/text parity evidence. The old native target is a
lifecycle identity fixture, not a fresh Browser parity baseline. Browser remains
default; no installation occurred. Instagram read-only preflight passes on the
same fixture and selected logged-in profile. A separate single-cycle Instagram
foreground approval is required before exercising its source window.

Instagram authenticated lifecycle gate passed (2026-10-04): the user separately
approved its single cycle. Receipt
`authenticated-source-handoff-b4d8b025-b753-42f4-9667-fbafbc9f795b` passed without
Bridge reload. The same fixture verified generation 2 headless capture of the
exact native target ID, generation 3 real Bridge source-window preparation,
the action-bound source target's unique single-page window, the live HWND hold
after action leases drained, scoped close, natural generation 4 headless return,
and same-native-ID capture afterwards. The wrapper exited successfully and
verified profile release, restored healthy runtime/compatible Bridge, and
unchanged original Settings. This separately closes the bounded Instagram
source-window API lifetime/auto-return gate in the same registered-development
Bridge/staged-worker scope as LinkedIn. Neither test submitted credentials,
posted social content, installed a candidate, or changed the Browser default.

Both new sources now have authenticated source-window lifetime evidence.
Remaining integrated acceptance is actual trusted reader click, rendered
Settings switching, and the complete packaged Bridge identity/runtime journey.
Broader fresh media/text parity and reliability remain separate qualification
work; these old-target lifecycle receipts must not be relabeled as fresh paired
Browser acquisition evidence. Facebook stays parked. The new fixture/guard and
ledger changes are local; no new checkpoint commit/push is claimed here.

## Product contract

The user selects browser or headless collection in Settings and can switch back.
Browser remains the default, including when older settings omit the new field.
Headless remains opt-in in the current runtime during the transition. The
approved hybrid direction above makes it the intended default for X, Instagram
and LinkedIn, with the existing Browser collector retained for Facebook.
Default migration requires source and integrated hybrid product acceptance;
Facebook headless parity is no longer a prerequisite.

### Approved transition direction (2026-10-02)

This amendment supersedes the earlier assumption of permanent equal investment
in Browser and Headless, and the earlier Quiet productization completion gate.

1. Finish X and Facebook first, with source quality at least equivalent to the
   existing Bridge collector and a complete collection/interactive/auto-return
   journey. Compare against the existing collector, not only hidden Quiet.
2. Then implement and qualify Instagram and LinkedIn. Do not remove their current
   browser support or silently route them to an unsupported headless driver.
3. Only after all four sources pass, propose the default migration with explicit
   rollback and preserved authentication. Sunset the legacy collector after the
   replacement is proven; retain browser interaction for login/challenge/readers.

Freeze hidden Quiet as an independent feature. Its media parity and standalone
recovery are no longer prerequisites for headless acceptance. Preserve needed
process/target ownership and handoff components; isolate the experimental Quiet
collector from the legacy fallback before shipping the transition candidate.
During an interactive browser lease, headless acquisition waits and resumes after
verified auto-return; it need not run hidden Quiet concurrently.

Immediate priorities: align production and QA profile selection, bound worker
pipe writes and cleanup, validate the actual product journey, and resolve X/FB
media gaps with causal evidence. Existing receipts below retain their original
scope and do not establish parity with the complete legacy Bridge pipeline.

Owned headless Chrome must use ordinary background page targets with CDP focus
emulation for each source. A separate
`hidden:true` target suppresses animation-frame callbacks in the tested Chrome
154 even while DOM visibility is `visible`; focus emulation does not restore
them. Ordinary background tabs also need focus emulation so both sources keep
rendering after navigation. Headless process mode supplies window isolation. This correction is scoped
to owned headless Chrome; it does not alter the borrowed Quiet broker or permit
foreground activation. Require two-source rendering and passive native-window
verification before accepting the corrected packaged worker.

### Approved fresh-data replacement validation (2026-10-02)

The user requests practical quality at least equal to the existing mechanism,
not perfection. Prioritize X and Facebook on newly collected data. Park the old
unavailable Facebook URL as a regression fixture; it fails in both headless and
foreground and is not, by itself, a headless replacement blocker.

- Use the latest completed browser/Bridge acquisition as the reference and
  record its actual driver, adapter revision, profile, acquisition time and
  surface. Prefer raw acquisition observations over AI-selected Timeline items
  so downstream selection does not distort extraction quality comparisons.
- Target a paired collection gap of at most 30 minutes. Record the actual gap;
  older saved examples remain supplementary evidence. Freshly acquired is not
  necessarily newly published: report publication time separately when known.
- Compare the same posts wherever identity can be established. Keep a separate
  feed-yield comparison: personalization and changing feed order mean unequal
  feed IDs alone cannot establish a regression. Do not rewrite identity or
  accept unrelated recommendations to inflate successful matching.
- Begin with a bounded pilot, then repeat across at least two fresh acquisition
  windows and observed text, single-image, multi-image and video cases for each
  source. Missing content types remain untested, not failed or silently passed.
- Judge text completeness, author/post ownership, usable image count and video
  evidence/playback, continuation, capture reliability/duration, login retention,
  and focus interruption. Permit harmless formatting, relative-time labels and
  signed-CDN query differences when the underlying evidence is equivalent.
- A limitation shared by both mechanisms is recorded but does not automatically
  block replacement. Reproducible loss of usable text/media that the current
  mechanism obtains, wrong-post attribution, failed authentication/cleanup or
  broken handoff does block the affected gate. A visible post with unverified
  identity is not yet a verified successful capture.
- Report four outcomes explicitly: equivalent or better, headless regression,
  shared limitation, and unverified. No arbitrary success percentage or perfect
  field-equality target replaces the paired evidence and user-visible quality.

After X/Facebook source evidence, finish the integrated collection/reader/login/
auto-return journey. Instagram and LinkedIn remain next; only qualify a default
change after all four sources pass. No default migration, installation, release
or foreground activation is implied by this validation amendment.

### Current acceptance status (2026-10-03)

This table is the current execution order. The phase history below records
earlier checkpoints and must not be read as release acceptance.

| Gate | Current evidence | Remaining work |
| --- | --- | --- |
| Profile ownership and selected subprofile | Profile selection is pinned across replacements; blocked worker writes retire their owned tree; disposable Profile 2 launch passes | Integrated authenticated login/reader/auto-return journey |
| X collection | Fresh native-ID targets match text/media; avatar attribution fix verified in packaged worker; latest paired target matches text and both image assets/formats in a quote post | Repeat multi-image coverage, AkuBrowser player journey, continuation/reliability and full legacy Bridge comparison |
| Facebook collection — parked | Some fresh post/photo targets pass; another parent target has zero discovered posts while media-only recovery succeeds; no overall parity claim | Defer further investigation and qualification until the user resumes Facebook; keep existing Browser fallback |
| Browser fallback | Existing collector retained; unavailable Quiet readiness now excluded in source and a local candidate build; installed runtime unchanged | Validate integrated fallback and isolate experimental hidden Quiet routing without regressing safe handoff |
| Product journey | Settings/handoff fixtures and actual rendered in-app UI-to-worker/store single-photo recapture pass with preserved identity; served UI displays recovery and loaded media | Combined installed-app Settings, reader/login, auto-return and auto-update acceptance; fresh source parity and multi-image recovery |
| Instagram / LinkedIn | Packaged feed, eligible continuation and native targets pass; three fresh targets match native identity, author and text; owned images/video URLs extracted, two video HEAD checks return 200 | Broader media/content diversity, LinkedIn 800 versus Browser 1280 rendition, timestamp fidelity, repeatability and integrated user journey remain open; full parity is unverified |
| Default migration | Browser remains default | All four sources and integrated journey must pass before migration |

An absent continuation is not a failed capture and is not a passed continuation
test. An admitted observation is not a media-playback or complete parity proof.

`collectionMode` will be `browser` or `headless`. Existing `captureVisibility`
remains a browser-only policy, with its value preserved while headless is active.
Quiet is not headless. Settings must show the requested mode, effective mode,
pending transition and actionable failure separately.

Historical amendment (superseded as an independent feature gate by the approved
transition above): Quiet browser collection may use hidden Chrome
targets created as machine collectors from the beginning. Explicit interactive
source/login/reader windows and Adaptive/foreground behavior remain separate and
preserved. Update Quiet's single/multiple-window promises with the implementation;
do not silently reinterpret existing ordinary tabs as disposable hidden targets.
This amendment does not establish source parity or authorize installation/release.

Both drivers submit `domain.Observation` through the existing engine acceptance,
quality validation, reconciliation, reasoning and storage pipeline. A driver must
not write Timeline records directly or bypass durable command admission.

## Invariants and scope boundaries

- Exactly one managed capture process may own the authenticated capture profile.
  Use the configured capture Chrome executable in both modes; retain the separate
  UI Chrome for Testing profile. Do not clone profiles or synchronize cookies.
- A run pins its driver and runtime generation. Switching modes never changes a
  run halfway through acquisition or its follow-up round. Fence late callbacks.
- Stop new acquisition admission before a transition; drain active capture and
  interactive ownership before releasing the profile. Coordinate manual updates,
  auto-update, queued work, cancellation, restart recovery and media recapture.
- Replacing capture must not shut down Sidecar or UI. Own only managed processes;
  never terminate arbitrary user Chrome windows or an active native reader.
- No silent visible-browser fallback when headless is selected. A failed
  transition may restore the previous healthy owner, but effective mode and the
  failure must be shown explicitly. Never claim the requested mode is active.
- Login/session expiry and explicit Open native post may need a visible window.
  Use an interactive profile lease and resume the selected mode after release.
  Switching modes alone must not require a new login.
- Source support is explicit and gated by evidence. X and Facebook are initial
  candidates, not blanket production-ready claims. Unsupported sources fail with
  a clear capability status before a partial session is accidentally created.
- Findings from a production extractor run inside the headless PoC do not prove
  a defect in the installed browser collector. Shared extractor changes require
  browser-mode regression evidence; retain headless-specific fixes in that path
  until their common contract is proven.
- Hidden collectors must use the managed profile's default browser context and
  an immutable allowlist of created machine targets. Never adopt an interactive
  target into that list. Retire collector targets/sessions before the static host;
  preserve legacy ordinary tabs whose interactive ownership cannot be fenced.
- A headed capture's private CDP transport belongs to the persistent process
  owner, not to an extraction worker. Worker timeout/cancellation must not close
  the root pipe, invoke Browser.close or kill the Job while interactive windows
  survive. Pipe disconnection can close Chrome; retain it through natural exit.

## Code ownership

| Area | Responsibility |
| --- | --- |
| `internal/collection/request.go` | Acquisition intent and command-preparation contract; no browser execution |
| `internal/collection/bridge/` | Existing Bridge payload preparation; asynchronous transport remains intact |
| `internal/collection/coordinator.go` | Local candidate for driver routing, capability/readiness checks, admission and pinned run ownership |
| `internal/collection/headless/` | Packaged Node/CDP worker and common observation submission; production continuation fixtures pass, authenticated parity remains open |
| `internal/captureruntime/` | Managed Chrome lifecycle, profile ownership, generation and leases; local handoff/recovery implementation awaits final acceptance |
| `internal/engine/`, `internal/store/` | Durable commands/runs, result validation, reasoning and common data persistence |
| `internal/httpapi/` and embedded UI | Settings contract, transition status, source capabilities and error presentation |
| AkuBridge source runtime | Reusable extraction logic with separate Bridge/CDP host facilities |
| `experiments/x-headless/` | Fixtures and live comparison harness; not a product runtime dependency |

Do not add an unused driver registry or a Settings toggle ahead of a working
runtime owner. Phase 1 deliberately defines only preparation: the existing
collector is asynchronous, so a synchronous `Collect()` wrapper would hide its
durable ownership boundary. The complete driver execution interface belongs in
phase 2. `internal/capture` already handles reconciliation and remains separate
from Chrome process lifecycle.

## Phases and acceptance gates

### 1. Preserve the existing collection contract

Scope: introduce typed acquisition intent and a Bridge command builder; route
the existing engine payload helper through it. Keep all payload fields and their
JSON meaning, including null continuation and first-round mutation policies.

Gate: first and follow-up wire fixtures pass; existing engine acquisition,
follow-up and observation tests pass; application compiles. No settings/schema,
Chrome lifecycle, extractor, transport protocol or mode-default change.

### 2. Introduce coordinated runtime ownership

Scope: separate capture lifetime from UI/application lifetime; define the
execution driver boundary around persisted commands and accepted observations.
Add admission/drain, pinned driver/generation and ownership checks. Route
readiness through the selected driver's capabilities instead of assuming Bridge
readiness universally. Keep only browser active while proving this boundary.
Headless requires independent UI/capture ownership. For launches without the
existing split-profile architecture, define a supported transition or report the
mode as unavailable; do not close a combined UI/capture process or copy its
signed-in profile as an implicit migration.

Gate: intentional capture replacement preserves UI; a second profile owner is
rejected; queued/follow-up commands cannot be claimed by another driver; stale
results are rejected; active work and readers delay transition; cancellation,
shutdown and restart preserve accepted observations. Define timeout and failed
handoff recovery before adding headless. Any persisted ownership changes require
an explicit backward-compatible schema migration and restart tests.

Implementation checkpoints (all are required to complete phase 2):

- **2a: process-owner foundation.** Adopt the current split capture process in
  `captureruntime.Manager`. Distinguish intentional replacement from unexpected
  exit; wait for owned-tree cleanup, block replacements while leases exist, and
  expose a process-local generation fence. No replacement endpoint is exposed.
- **2b: collection admission and durable fencing.** Attach leases to actual runs,
  including follow-up rounds and media recapture. Pin driver/generation in durable
  command ownership; integrate claim/result rejection, cancellation, restart
  recovery, readiness and automatic updates. A local generation counter alone is
  not sufficient to authenticate or fence a persisted command.
- **2c: interactive ownership and replacement integration.** Hold leases through
  native-reader lifetime, not merely through dispatch of an open action. Rebind
  split transport, containment and reader callbacks to the new process. Define
  bounded transition contexts, failure presentation and verified-owner recovery.
  Do not call Replace from product code before 2b and 2c gates pass.
- **2d: approved Quiet machine surfaces.** First prove hidden-target support and
  private owner-held transport in the pinned Chrome, with disposable static
  fixtures. Then separate collector backend from browser/headless presentation,
  preserving durable admission, generation fencing and source capabilities.
  Require hidden default-context/cookie continuity, no tab/window promotion,
  independent interactive-window survival, worker-failure containment, natural
  owner/Job drain and automatic same-profile return. Update Quiet UI wording and
  browser regressions together. Experimental protocol support is a capability
  gate, not an assumed guarantee or an excuse to drop Adaptive/source parity.

### 3. Productize the headless driver behind an internal gate

Scope: use the configured capture Chrome through CDP; adapt the common source
core to host facilities for navigation, delay, hover and media resolution. Return
the existing Observation schema through authenticated/fenced ingestion.

Decide and record the CDP implementation first: native Go or a bounded packaged
Node worker. Reusing the PoC's Node worker requires versioned distribution,
provenance and cleanup; never depend on a user's PATH Node. Do not add another
Chromium distribution. The PoC wrapper that stops the entire Sidecar is excluded.

Gate: same-profile authentication, owned-process cleanup and timeout/cancellation
work; headless commands cannot be claimed by Bridge; extractor identity and media
evidence preserve unknown/unresolved states. Use the existing X/Facebook evidence
as the baseline, then close missing cases rather than rerunning all experiments.
Facebook native-ID aliases and unresolved video evidence remain open gates.

### 4. Expose opt-in Settings and interactive handoff

Scope: persist `collectionMode`, migrate old settings to browser, implement
validation and requested/effective/pending/error status across API and UI. Apply
at a documented safe boundary. Preserve captureVisibility selection. Implement
explicit login/Open native post leases and resume the selected mode afterward.

Gate: old settings behave identically; browser -> headless -> browser succeeds
without restarting UI or losing authentication; changing Settings during capture
does not mix drivers; unsupported source/mode combinations are explained; failed
startup reports actual effective mode; auto-update and media recapture respect
ownership. Session-expiry interaction resumes safely without unsolicited visible
collection. Do not offer a selectable mode that cannot complete its handoff.

### 5. Validate opt-in parity and package readiness

Scope: bounded read-only comparison on supported sources and release/runtime
verification. Compare explicit native identity, text, timestamps, media ownership,
pagination/coverage and duplicate behavior. Identical text/author is insufficient
to establish native-ID equivalence.

Gate: validate empty and media-only posts, long text, multiple images, video,
shared posts, follow-up capture, authentication challenge, worker failure and
mode-switch recovery. Distinguish fixture, live-headless and live-browser evidence.
Record remaining gaps and supported capabilities. Confirm background collection
visibility through live Windows observation; do not promise zero blinking merely
because DOM capture passed. Browser remains default after this gate.

## Progress ledger

| Phase | Status | Evidence / next action |
| --- | --- | --- |
| 1 | Complete; committed as `a263bcc` | Typed Request, Bridge Builder, engine integration and two wire fixtures; full engine tests and application build pass |
| 2a-2b | Foundation complete and committed; final integrated validation pending | Process ownership, durable driver/generation fencing, session/media/action leases, cancellation drain and native-reader/source lifetime tracking; commits `b376438`, `86cd0c3`, `d9b735a` and paired Bridge `8541337` |
| 2c | Static packaged Bridge handoff and auto-return pass; live-source acceptance incomplete | Real host-only ACK, natural owner drain, interactive HWND retention and generation-4 auto-return pass on a disposable profile. Finish live authenticated reader/login and recovery |
| 2d | Hidden Quiet backend and bounded authenticated capture pass; integrated acceptance incomplete | Production Quiet driver plus packaged worker passes X/Facebook 6/6 and natural profile drain. X hidden-media parity, startup visibility and real interactive journeys remain |
| 3 | Packaged authenticated X/Facebook feed/follow-up/target pass; broader source parity incomplete | Same configured Chrome/profile authentication and cleanup proven. Native watch/video URL contract fixed in worker and Go admission. Full video metadata, native aliases, freshness qualification and broader content cases remain |
| 4 | Rendered Settings fixture passes; integrated acceptance incomplete | Real UI/API/coordinator with fake processes proves switching, reload persistence, visibility restoration and unsupported-source rejection. Native reader/login, auto-update and recapture journey remain |
| 5 | Local tuple and bounded headless source/visibility proofs pass; product validation incomplete | Native X/Facebook 6/6 capture passes and 19 blocks validate in Go without ingestion. Passive Windows trace sees no root-window visibility/activation during about 75s. Integrated reader/login, Quiet visibility and full media/content parity remain |

The table describes code availability separately from acceptance. Candidate
Settings/driver wiring was implemented before the 2c gate passed; this is an open
roadmap sequencing deviation, not evidence that phases 3-4 are accepted. Keep the
candidate out of installed/released runtimes until the ownership gate is closed.

### Resume order after the local candidate checkpoint

1. Build and verify complete local packages with the committed Bridge scoped
   retirement. Retain the passing payload/frontier, permissions, rendered
   Settings and isolated Windows ownership evidence.
2. Close authenticated end-to-end Bridge handoff/recovery under the approved
   live-switch and auto-return contract.
3. Validate native-reader/login journeys, then
   bounded authenticated X/Facebook parity and background-window visibility.
4. Record supported capabilities and
   remaining source limitations before declaring any phase complete.

Snapshot commits preserve unfinished candidate work; they do not authorize
installation, runtime restart, push or publication and do not imply green tests.

### Phase 1 validation (2026-10-01)

- `go test ./internal/collection/... ./internal/engine -count=1`: passed. Engine
  tests include the new first/follow-up wire fixtures and existing acquisition,
  observation, progressive wait and follow-up behavior. Collection packages
  compile and are exercised through the engine contract tests.
- `go build -buildvcs=false -o build/collection-contract-check.exe ./cmd/akusidecar`:
  passed; generated binary is a local verification artifact, not installed or run.
  The initial normal build exited successfully with a denied external module
  stat-cache write; the build without VCS metadata completed without that warning.
- `gofmt -l` on changed Go files: empty; tracked diff whitespace check: passed.
- Settings, storage schema, Bridge wire payload, extraction, command transport and
  Chrome startup behavior remain unchanged. Existing PoC files were preserved.
- No live capture, installed runtime restart, commit, push or deployment performed.

### Phase 2a implementation boundary (2026-10-01)

- Split startup returns a managed capture owner to main. Intentional replacement
  does not end that manager; unexpected capture exit preserves the existing
  application shutdown policy. Combined UI/capture launches are unchanged.
- Replacement uses the existing `appshell.Window.CloseForRetry` ownership drain,
  not root exit alone. Failed cleanup retains the old owner and blocks admission.
  Failed/cancelled launch blocks admission and further launch attempts because
  ownership may be unverified; there is no silent fallback.
- Manager leases are process-local building blocks. Current source dispatch and
  readers do not yet acquire them, and generation checks are not yet connected to
  persisted commands. Product code therefore does not call Replace yet.
- Tests cover first/second lease drain, idempotent release, intentional replacement,
  late-generation rejection, root exit before owned-tree drain, cleanup/launch
  failure, cancellation before/during cleanup and launch, unexpected exit and
  shutdown. These are controlled-process tests, not live Chrome handoff evidence.
- `go test ./internal/captureruntime ./internal/collection/... ./internal/engine
  ./internal/httpapi ./cmd/akusidecar -count=1`: passed, including all 11 manager
  tests and the existing engine/API/app-entrypoint suites.
- `go build -buildvcs=false -o build/capture-runtime-check.exe ./cmd/akusidecar`:
  passed. The binary was not installed or executed.
- Race-detector execution is unavailable here: CGO is disabled and no GCC is on
  PATH. No compiler installation is needed for this checkpoint.
- First-stage commit `a263bcc` was explicitly authorized after phase 1. Phase 2a
  remains local work; existing PoC files are preserved. No push, installation,
  installed runtime restart or live browser capture has been performed.

### Next implementation checkpoint: 2b and 2c

Baseline inspection confirmed `startNext` admits initial acquisition and
`AcceptObservation` can admit the next source before reasoning finishes. A lease
must therefore cover durable session/run ownership, not only a synchronous
capture callback. Follow-up commands are queued later by reasoning; release on
the first observation would permit a driver change between rounds.

The split action transport tracks action completion and reader foreground
preparation, not the full lifetime of a native reader window. Action completion
is not evidence that the reader released its profile. Add explicit reader
lifetime tracking before connecting any replacement trigger. Existing callback
closures also point to the original containment instance and must be rebound.

### Phase 2b and native-reader guard (2026-10-01)

- Managed split startup attaches capture ownership to the engine before starting
  automatic updates. Existing durable active session/recapture leases are adopted
  before the manager is published to callbacks. Combined UI/capture launches keep
  the legacy unowned payload behavior.
- Session admission acquires ownership before writing a session. It holds through
  queued acquisition, both follow-up rounds, progressive-source scheduling and
  reasoning. Only durable terminal state plus worker drain releases the lease.
  Cancellation alone does not release an active reasoning worker's ownership.
- Managed command payloads persist an additive `captureRuntime` object with
  driver, Sidecar epoch and generation. Claim rejects stale commands before
  dispatch; observation admission checks that persisted owner before saving.
  Stale queued work follows the existing failed-command/fallback path rather than
  replaying an accepted capture. Legacy unstamped commands are admitted only in
  the initial managed generation. No database schema version change is required.
- Media recapture acquires its own lease and atomically persists the owner stamp
  with job creation. Claim, acceptance and failure check ownership; terminal jobs
  release their lease, including passive enrichment that completes an active job.
- Replacement checks a process readiness guard before terminating the old owner.
  Windows containment remembers explicitly bound reader HWNDs independently of
  consumed/expired foreground capability. Native existence, not visibility or
  action completion, controls release. Reused live HWNDs conservatively block;
  tracked live readers are bounded to 32. No native foreground behavior changes.
- Passed full tests for captureruntime, collection, engine, store, HTTP API and
  app entrypoint; passed targeted appshell reader/containment tests and application
  build. Added controlled tests for session cancellation/worker drain, unchanged
  owner across real follow-up workflow, stale-epoch dispatch/result rejection,
  malformed generations, unavailable runtime admission, media ownership/duplicate
  admission and native reader existence. No live Chrome handoff was performed.
- At that checkpoint phase 2 remained incomplete: pending/claimed split actions
  still needed admission leases; sign-in windows need full-lifetime protection;
  replacement must rotate split instance credentials and rebind containment and
  reader callbacks. Recovery after failed startup also needs verified ownership
  cleanup. Do not wire a product replacement trigger until these gates pass.

Complete these remaining ownership boundaries with targeted tests before opening
a mode switch. Do not retrofit a settings-only change around them.

### Split-action lease checkpoint (2026-10-01)

- Previous runtime/session/recapture/reader foundation committed as `b376438`,
  excluding `experiments/`. No push or runtime installation performed.
- Split action admission acquires a manager lease before enqueue. Startup adopts
  existing queued/claimed actions before publishing the runtime to admission.
- An unclaimed cancelled/timed-out request removes its action and releases the
  lease. A claimed request remains detached in its original queue slot until a
  late result arrives or the transport closes. It cannot be claimed again.
- Late results, including failure results, release ownership and remove detached
  actions. Explicit completion state rejects duplicates even after a waiting UI
  consumes the result channel. Unknown claimed outcomes stay unknown and block
  replacement; expiry alone never proves browser work has stopped.
- Detached actions count toward the existing 32-slot limit. Transport closure
  releases pending leases; the native reader-lifetime guard separately protects
  reader windows after action completion. Login-window lifetime and rebinding
  still need implementation. No replacement endpoint or headless setting exists.
- Added controlled API tests for client cancellation before/after claim, request
  timeout, successful/failed late results, duplicate result after consumption,
  existing-action adoption, unavailable runtime admission and transport shutdown.
  Production action timeout remains 115 seconds; tests use a shorter internal
  transport timeout. This is not live authenticated browser handoff evidence.
- Validation: full `go test ./internal/httpapi ./internal/captureruntime
  ./internal/engine ./cmd/akusidecar -count=1` passed, including seven new action
  lease tests; application build and diff whitespace check passed. New split-action
  checkpoint committed as `86cd0c3`; installed runtime has not been restarted.

### Source/login-window lifetime checkpoint (2026-10-01)

- Scope: protect the explicitly opened split source/permission window before
  source navigation, using a paired optional Sidecar/Bridge handshake. Sidecar
  records a uniquely marked native HWND in the capture job without foreground
  writes or reader foreground privileges. Keep the original browser default.
- The next-action response advertises native source tracking only when a native
  callback exists. New Bridge creates a local marker window, requests one-use
  authenticated preparation for its claimed `open_source` action, then navigates
  the same tab to permission/feed. Binding failure closes only that new marker
  window; navigation failure after binding preserves its tracked lifetime.
- Source windows share the bounded native lifetime guard with readers and block
  replacement while their HWND still exists, including when minimized. Tracking
  does not expire when the action acknowledges or its HTTP request finishes.
- Legacy Bridge/Sidecar combinations preserve direct source opening. They do not
  prove login-window tracking and cannot qualify for safe runtime handoff.
  Site-created popup windows and transport credential/callback rebinding still
  require evidence before phase 2 can pass. No mode switch is exposed.
- This paired checkpoint is committed separately in AkuSidecar and AkuBridge.
  No live authenticated capture, installation, runtime restart or headless
  activation is part of this checkpoint.
- Validation: Sidecar API/runtime/engine/entrypoint suites and targeted native
  containment/lifetime tests passed; Sidecar application build passed. Bridge
  syntax checks and 16 targeted tests passed, including negotiated/legacy source
  handshake, ordering, failed-binding cleanup and post-binding navigation failure.
  Additional API cases cover native capability advertisement, marker access,
  claimed/type/instance fencing, consumed/completed intent and failed-binding replay.

### Capability and popup readiness checkpoint (2026-10-01)

- Source/login-window checkpoint committed as Sidecar `d9b735a` and Bridge
  `8541337`. This next checkpoint remains local and uncommitted.
- Bridge declares version 1 of source-window tracking at authenticated split
  bootstrap. Legacy/unknown versions keep browser operation compatible but do
  not pass transport readiness for replacement. An unprepared successful source
  result records an unverified outcome; a reconnect cannot erase that uncertainty.
- Main registers an immutable transport-readiness callback on the manager. Its
  refusal restores the prior state without terminating the owner or increasing
  its generation. No product replacement trigger is exposed.
- Windows replacement readiness enumerates owned top-level browser windows,
  including hidden/minimized windows, without foreground property writes. It
  requires one capture-host marker and rejects additional Chrome windows even
  after their parent closes. Other user Chrome processes are outside ownership;
  known non-browser native helper windows are ignored. Unknown owned classes,
  failed enumeration, missing/ambiguous host markers fail closed.
- This is a conservative preflight, not atomic popup protection. A popup can
  appear after enumeration; quiescence and the final cleanup/transition boundary
  still need integrated evidence. A missing marker may conservatively block a
  healthy runtime. This guard currently describes browser capture, not headless.
- Transport rotation/rebinding, selected-driver readiness and verified recovery
  remain pending. Installed Chrome has not been restarted or tested live.
- Validation: full API/runtime/engine/entrypoint suites, targeted appshell
  containment/lifetime/popup tests and the application build passed. Sixteen
  Bridge tests and its client syntax check passed. New cases cover legacy/unknown
  capability versions, downgrade on reconnect, persistent unverified outcomes,
  verified preparation, closed transport, immutable manager gate and hidden,
  orphaned, unowned, missing-host and ambiguous-host windows. This is fixture and
  controlled-process evidence; it is not a live native popup handoff test.

### Local integration candidate and stop checkpoint (2026-10-01)

- Uncommitted candidate adds requested/effective/pending Settings status,
  browser/headless coordination, generation-pinned execution, interactive
  browser borrowing, split credential rotation/rebinding and verified recovery.
  The engine submits worker observations through its existing acceptance path.
- Dependency-free Node/CDP worker supports bounded X/Facebook extraction and
  canonical Observation IDs with explicit partial quality/video uncertainty.
  AkuBrowser builders now stage a checksum-pinned official Node runtime,
  worker sources and licenses beside Sidecar. Packaging helper fixture tests
  passed; a full package build and installed-runtime validation have not run.
- Coordinator/domain tests and three Settings-state JavaScript tests passed.
  The earlier focused Go suites passed before the latest integration changes.
  Latest runtime and HTTP suites passed, but the new engine cross-driver test
  failed because rejection returns a plain error rather than the expected stale
  owner sentinel. The rejection itself occurred; final suites are not green.
- Real Node/Go-owned Chrome smoke used an isolated test profile, without source
  navigation. It failed at init because the worker omitted bridgePath from its
  validated options. That source fix exists, but staged assets still predate it;
  restage and rerun the profile-exclusivity/reuse smoke before claiming success.
- Important remaining implementation: production payload names/budgets and
  acquisition-round continuation/frontier handling. Current worker still reads
  PoC-style maxScrolls/waitMs and navigates home for every capture. Do not claim
  production follow-up parity until this is fixed and tested, including source
  interleaving and recapture invalidation.
- Remaining acceptance: finish scoped integration review; final Go/JS checks;
  full packaging; rendered Settings/native-reader journey; authenticated X/FB
  parity; live profile handoff/recovery and native popup protection. Window
  enumeration is still a conservative preflight with a known TOCTOU gap, not
  proof of atomic popup safety. The product handoff acceptance gate is open.
- No installation, installed-runtime restart, authenticated live collection,
  new commit or push occurred. Existing experiments remain excluded. Work was
  stopped because the active 1,000,000-token goal budget was exhausted; agents
  were interrupted. Preserve all local edits when resuming.

### Post-checkpoint continuation and required product decision (2026-10-01)

- Checkpoint commits: Sidecar `8967dff` preserves the unfinished integration and
  updated roadmap; AkuBrowser `479d11b` adds pinned worker packaging. Neither was
  pushed or installed. The following continuation remains local until committed.
- Worker now accepts production Bridge payload/budget fields; uses per-source
  CDP tabs; preserves source frontier across interleaved captures; verifies
  round-2 URL, scroll and anchors; invalidates continuation on fresh navigation
  or recapture; rejects unverifiable continuation explicitly. Freshness policy
  remains `not_verified` in coverage, not a claim of live browser parity.
- Worker fixtures pass 6/6 and all worker module syntax checks pass. Staging was
  refreshed. Real Node/CDP/Windows Job smoke passes with an isolated profile:
  concurrent owner rejection, verified tree cleanup and profile reuse. WMI
  enumeration was denied in sandbox; the smoke passed outside sandbox. It did
  not use the installed profile, navigate to social feeds or open visible Chrome.
- Cross-driver claim/recapture errors now wrap the common stale-owner sentinel.
  Fresh browser heartbeat refreshes retained headless source authorization and
  clears revoked/incompatible access; a new regression test passes. Login and
  challenge failures preserve their codes and are not automatic-retry promises.
- Final focused suites passed for runtime, collection, domain, engine, HTTP and
  app entrypoint after these changes. Three collection Settings-state JavaScript
  tests, app syntax and diff whitespace checks pass. Earlier application build
  passed; no full package or rendered/live UI/source validation is claimed.
- A scoped lifecycle review confirms the unresolved safety race. Replace checks
  readiness before CloseForRetry; CloseForRetry sends WM_CLOSE to all owned
  top-level windows and can kill the Job after eight seconds. New native popups
  are not fenced by Go admission mutexes. Recover also needs the same live-headed
  safety policy; extra snapshots or a successful smoke do not prove atomicity.
- Product decision required: either preserve live switching/automatic return to
  headless and extend the headed browser's cooperation/quiescence design, or
  approve a reduced contract: headless at cold startup, browser sticky after
  interactive promotion until the user closes the application. The latter
  changes the approved resume-after-interaction behavior and requires a startup
  and source-consent design; it is not implemented or silently selected here.
- Keep the candidate out of installed/released runtimes. Rendered journeys,
  authenticated X/Facebook parity, complete packaging and visibility evidence
  still follow closure of the handoff gate. No additional profile/runtime action
  is authorized merely by this documentation checkpoint.

### Approved live-switch contract and scoped retirement (2026-10-01)

- The user selected immediate browser/headless switching and automatic return
  to headless after interaction. Cold-start-only or sticky browser mode is not
  the approved contract. Continuation changes above were committed as `284bfcc`.
- CDP renderer freezing and new-target pause do not establish a native popup
  creation barrier. Destructive WM_CLOSE-all or Job termination therefore cannot
  implement a safe headed handoff. A fail-closed guard now protects that boundary.
- The implementation direction is scoped retirement: an authenticated Bridge
  command removes only verified background capture tabs and its exact host tab;
  source, login and reader windows are preserved. Sidecar waits for natural
  Chrome exit and verified zero owned processes before launching the next mode.
  A popup race or timeout retains the old owner and a pending mode request;
  it never authorizes a forced close or profile reuse. Explicit application quit
  remains separate from collection-mode handoff.
- This protocol and lifecycle change are under implementation. Its acceptance
  requires non-destructive timeout/retry tests, preservation of independent
  interactive windows, and integrated Windows handoff evidence. Settings,
  authenticated X/Facebook parity, full packaging and visibility gates remain
  open until their respective evidence is recorded.
- The pinned Windows c2patool binary exists at Sidecar's project-local
  `runtime/dev/c2patool.exe` with the manifest SHA-256. Full packaging can use
  that explicit input without provisioning a binary outside the workspace.
- Go lifecycle implementation now binds scoped retirement to initial and
  replacement capture windows. Recovery applies readiness before cleanup;
  retirement timeouts retain the owner and retry automatically. Natural root
  exit retains the Windows Job handle until zero active processes is verified;
  it does not kill surviving children or close a kill-on-close handle early.
- The internal HTTP action is negotiated explicitly, admitted only while the
  manager is replacing a browser owner with no active leases, and omitted from
  public UI action admission. Claimed requests survive cancellation without
  replay; late ACK and completed rejection behavior are covered by tests.
- Focused appshell/runtime/collection/domain/engine/HTTP/entrypoint suites pass.
  The full package attempt reached launcher verification and exposed strict
  decoding of the new `headlessWorkerPath` field. Launcher now recognizes the
  optional adjacent worker directory and requires declared worker payloads;
  its full Go suite passes. The package builder still needs a final rerun.
- Actual independent-popup Windows smoke has not run. Bridge tab-retirement
  integration is the current remaining implementation delta, followed by full
  packaging, rendered journeys and authenticated source parity. This checkpoint
  does not establish zero blinking, popup safety in live Chrome, or release
  readiness. Installed runtime/profile remain untouched.

### Isolated popup and rendered Settings checkpoint (2026-10-02)

- Sidecar `3511147`, AkuBrowser `036d839` and Bridge `b7d7340` preserve scoped
  Go retirement, launcher worker-manifest support and capability negotiation.
  The following tests add acceptance evidence without changing production code.
- Opt-in Windows smoke passes (12.88 seconds) with project-local pinned Node,
  configured Chrome and a disposable profile. An independent minimized popup
  is created after Manager readiness, inside the host-close callback. Exact host
  removal times out without killing the popup, root or Job. Coordinator retry
  subsequently starts real headless generation 2 on the same profile after the
  popup is closed separately and the old owner exits naturally.
- This smoke substitutes an exact CDP host-target close for the authenticated
  Bridge/server callback. It proves the native ownership boundary for this popup
  scenario; it does not prove all native dialogs or end-to-end Bridge handoff.
  The sandbox denied loopback CDP; the unchanged smoke passed outside sandbox.
- The real rendered Settings UI/API/coordinator fixture passes (100.63 seconds).
  The browser/headless/browser/headless journey, reload persistence, restored
  browser visibility controls and unsupported LinkedIn rejection were exercised.
  Final assertions verify headless, retained quiet visibility, X/Facebook source
  settings and at least four generations. Capture processes and source consent
  are simulated; native reader, authentication and live parity are not covered.
- Evidence screenshots are local artifacts under
  `build/collection-ui-fixture-20261002/`; the final assertion run uses
  `build/collection-ui-final-20261002/`. Opt-in fixtures skip in ordinary suites.
- Bridge implementation is committed as `19719a5`. Both bootstrap and action
  envelope must advertise the internal close capability. Host identity is checked
  before managed retirement and again after result ACK; close polling waits for
  completion to prevent a terminal next-action response preempting host removal.
  Only exact tracked feed/placeholder tabs in minimized unfocused windows are
  retired. Transient, reader, source and navigated tabs are preserved. Failed ACK
  or host removal does not establish profile release. Focused tests pass 74/74;
  service-worker/client/runtime syntax checks and diff whitespace checks pass.
- Chrome has no atomic conditional tab removal. Navigation or user promotion
  between the final re-read and `tabs.remove` remains a residual race; repeated
  checks are not an atomic safety guarantee. End-to-end acceptance stays open.
- Remaining gates: authenticated end-to-end Bridge retirement, full package rerun and
  verifier, integrated native reader/login and recovery, authenticated X/Facebook
  parity including native IDs/media/coverage, and Windows visibility evidence.
  No installed runtime/profile, push or release was changed.

### Complete local package checkpoint (2026-10-02)

- Full installed-app tuple builder and independent builder verifier both report
  `status: ok`. Candidate directory:
  `AkuBrowser/build/headless-handoff-20261002/AkuBrowser-0.9.0-windows-x64-installed-app`.
  Its exact source tuple is Browser `036d839`, Sidecar `a3a148a`, Bridge `19719a5`.
  Sidecar is recorded dirty because untracked experiments remain preserved;
  `-AllowDirty` was used only for this local candidate.
- The payload contains 411 files / 588,711,115 bytes. Verifier checks the complete
  declared payload hashes, adjacent pinned Node/worker and both licenses, Chrome
  executable pin, Bridge identity, schema 29 provenance and Sidecar candidate
  probe. Launcher verify-only also succeeds; it does not launch the application.
- SHA-256: launcher
  `3e1223555c51273206c0b9fb77d414d5e28f3dd26e920d46a67b5a47401aef0e`;
  Sidecar
  `34329997ff32cd3aef46c4472f2fe9f54eb4e160e84fcd16ec4b9d21c5ebcd05`.
- Isolated Windows popup smoke passes again (13.71 seconds) using this package's
  Chrome, Node, worker and Bridge paths. The changed binary inputs justify the
  rerun. It retains the existing smoke's CDP callback substitution and does not
  close the authenticated end-to-end Bridge gate.
- Package metadata inherits the existing 0.9.0 release tuple labels. Successful
  packaging does not establish headless feature release readiness. No installer,
  installed-runtime launch/restart, profile access, push or publication occurred.

### End-to-end preparation and runtime boundary (2026-10-02)

- Read-only preflight identifies the active listener as the registered dev runtime
  (`runtime/dev/aku-sidecar.exe`, PID 2704), not an installed candidate. Supervisor
  reports healthy/running, no operator hold; Inbox has no active sessions. The
  registered capture profile has exactly one Google Chrome owner (PID 8372), and
  Bridge is healthy/compatible. These are observations, not a stop authorization.
- An alternative listener on `127.0.0.2:11122` bound successfully. Scoped Chrome
  resolver mapping to preserve the localhost origin did not produce a matching
  fixture request within the bounded probe: the expected target URL was present
  with an empty title and the isolated listener observed zero matching requests.
  The cause remains unverified. No DNS, extension allowlist or running service was
  changed; failed probe code is retained only as an ignored build artifact.
- `scripts/test-bridge-headless-handoff.mjs` prepares a controlled operator-only
  test. Default invocation performs read-only preflight. That mode and its syntax
  check pass. The explicit `--allow-runtime-stop` branch requires authorization:
  it verifies the candidate, rechecks idle Inbox and original ownership, stops the
  registered service through Supervisor, runs a disposable-profile smoke on the
  original port, and restores/verifies the original service in `finally`.
- End-to-end fixture execution requires a user decision because it temporarily
  closes the active product UI/capture runtime. It does not install the candidate,
  alter registration/settings, copy cookies or select the user's profile for the
  static Bridge smoke. Authenticated source parity remains a separate evidence
  gate after the static protocol/lifecycle smoke.

### Authorized static Bridge smoke and startup diagnosis (2026-10-02)

- The user approved controlled stop/test/restore after the fixture was ready.
  The wrapper and Windows-only `TestBridgeHeadlessHandoffWindowsSmoke` fixture
  were reviewed; default-skip compilation passes with project-local caches and
  temporary directories. The fixture binds the original loopback port before
  creating Chrome, uses an isolated store/profile and trusts the exact packaged
  extension identity. It would verify the real close ACK before transport rotation
  and direct Manager replacement, not an automatic Coordinator trigger.
- First opt-in run failed after 60.17 seconds waiting for authenticated capability
  bootstrap. It did not reach host retirement or profile handoff. Receipt:
  `build/bridge-handoff-dec61495-5ee7-4319-9549-b57513660e94/receipt.json`.
- A single diagnostic repeat added a direct HTTP host-page preflight, request
  counters and safe CDP booleans. It failed after 48.71 seconds with
  `host_http_hits=0`, `host_target=true`, `host_title=false`,
  `host_fragment=true`, `bridge_worker=true`, `bootstrap_requests=0`,
  `bootstrap_status=0`, `bootstrap_accepted=false`, `cdp=ok`. The direct HTTP
  preflight returns the expected page; its hit is excluded from Chrome counters.
  Receipt: `build/bridge-handoff-853137c6-3cdd-43dd-9d84-71106585a3e6/receipt.json`.
- Both runs restored the original registered service and verified healthy runtime,
  compatible Bridge and exactly one original-profile Chrome owner. A separate
  read-only preflight after the first restoration also confirmed zero active
  Inbox sessions. No candidate installation, registration change or authenticated
  source capture was performed.
- The failure is narrowed to initial host navigation: the packaged Bridge worker
  exists, but Chrome never requests the host document, so the capability handshake
  cannot begin. The navigation cause remains unverified; neither the host-close
  protocol nor end-to-end handoff is accepted. Do not repeat without a causal
  startup correction or new diagnostic evidence. The earlier popup smoke covers
  native lifecycle only and cannot fill this gap.

### Renderer startup diagnostic checkpoint (2026-10-02)

- A bounded read-only CDP diagnostic attaches to the disposable capture-host
  target and reports sanitized document, frame and extension state. It neither
  navigates/reloads the target nor changes production launch flags. Default-skip
  compilation, JavaScript syntax and diff whitespace checks pass.
- One authorized stop/test/restore run fails at capability bootstrap after
  48.80 seconds. Receipt:
  `build/bridge-handoff-ea285e40-1901-4999-9549-1361c656231c/receipt.json`.
  The original runtime is restored and its health, compatible Bridge and single
  original-profile Chrome owner are verified by the wrapper.
- Diagnostic: host HTTP hits 0; target found and attached; renderer document
  state `complete`, expected frame/title/host marker false, error class
  `unexpected_document`, unreachable false. Packaged extension worker exists,
  but its isolated world/content-script marker is absent from this document.
  Bootstrap requests remain 0. Target URL presence therefore does not establish
  navigation to the expected document. No handoff acceptance is claimed.
- Network counters are explicitly late observations: zero events after diagnostic
  attachment cannot prove that no earlier request or network failure occurred.
- An upstream CEF Windows minimized-startup issue identifies native occlusion as
  a possible hypothesis, not a verified cause in this Chrome build:
  https://github.com/chromiumembedded/cef/issues/3638 . A separate disposable
  loopback navigation comparison completed in 13.04 seconds: baseline and
  `--disable-features=CalculateNativeWinOcclusion` both observed 0 page hits and
  `loaded=false`. The probe completed, but the occlusion flag did not resolve
  this failure. It did not stop the original runtime or change its profile/settings.
- The user subsequently approved hidden Quiet collectors while retaining
  interactive and Adaptive windows. Pinned-Chrome hidden-target and private-pipe
  capability probes are the next bounded steps; no production backend change has
  been accepted or shipped on the basis of upstream source alone.

### Pinned-Chrome hidden-target capability checkpoint (2026-10-02)

- The isolated random-port diagnostic completes in 1.64 seconds using the
  existing full package tuple and reports actual `Chrome/152.0.7977.54`.
  No original runtime stop, authenticated profile or production configuration
  change is involved. The fixture's PASS means its diagnostic completed.
- Hidden targets created in a dedicated child browser session are supported;
  the default browser context is retained. The hidden fixture receives one HTTP
  request and reaches the expected complete document. Its native-window lookup
  returns a protocol error, whose exact reason was not classified in this run.
- Explicit Page.navigate of the known disposable host receives one HTTP request
  and reaches the complete host document. Natural startup had zero requests,
  an expected target URL but empty frame/history URLs. Initial document evaluation
  failed, so its exact initial document class remains unverified. This establishes
  an effective explicit-navigation path, not the underlying Chromium cause or
  authenticated Bridge bootstrap.
- A harmless fixture cookie seeded by the host is visible in the hidden target:
  default-context sharing passes without copying cookies. This is not live
  X/Facebook authentication evidence. Exact hidden disposal and child-session
  detachment pass while the root test-only WebSocket remains connected.
- After the run, the helper makes opaque-document cookie reads exception-safe,
  classifies window errors without raw text, and labels extension presence as
  `anyExtensionWorker` rather than claiming exact Bridge identity. Syntax passes;
  these diagnostic refinements have not received another native run.
- Go-owned private-pipe lifetime, worker-failure containment, interactive-window
  preservation and production collection integration remain separate gates.
  The loopback debugging endpoint exists only in this disposable probe.

### Go-owned Windows pipe capability checkpoint (2026-10-02)

- The first disposable private-pipe probe establishes Browser.getVersion as
  `Chrome/152.0.7977.54`, but fails after 0.32 seconds because an exact
  `about:blank` app target is unavailable. It closes no ambiguous target and
  cleans up only its synthetic owned Job. Natural shutdown remains unproven
  by that first run.
- The corrected fixture uses its own random-port `/pipe-host` URL and a bounded
  two-second wait for one exact page target. The second native run passes in
  0.38 seconds: pipe roundtrip, exact target closure, clean natural root exit and
  whole-Job zero are verified while both Go parent pipe ends remain open.
- The Windows transport uses inherited child-only anonymous pipe handles and
  `--remote-debugging-io-pipes`; it exposes no HTTP debugging port. No signed-in
  profile, original runtime interruption or global configuration change occurs.
- This is a transport/lifecycle capability proof. It does not yet prove the
  production Window API, hidden-collector integration, independent interactive
  window retention, worker-failure recovery or authenticated Bridge handoff.

### Managed private transport and static Bridge handoff checkpoint (2026-10-02)

- The opt-in managed Window foundation now exposes `LaunchOptions.PrivateCDP`
  and `Window.CaptureProtocol().Call`. It requires minimized capture ownership,
  rejects caller debugging switches, and uses Windows inherited anonymous pipes
  without an HTTP debug port. Ordinary launches retain their previous behavior.
  Product main/Quiet routing does not enable this option yet.
- Transport calls have bounded frames/requests/pending work. Cancellation leaves
  the owner-held connection open; late responses cannot resolve a newer request.
  The extraction backend must review these bounds before large script/results
  are passed through this bootstrap-oriented foundation.
- Focused transport/options tests and Linux cross-compilation pass. The managed
  Window native smoke passes in 0.45 seconds with Chrome/152.0.7977.54: exact
  target closure, natural root exit and empty Job are verified while parent pipe
  endpoints remain held until drain. Exceptional startup cleanup remains
  fail-closed: package-owned quarantine retains pipe and Job references until
  application exit when startup cleanup cannot be verified. A retention unit test
  passes; these error paths have not received native fault injection.
- The real Bridge fixture now explicitly navigates only the uniquely identified
  static host after its bindings exist. A first corrected run passes bootstrap
  and host-close ACK but fails its final assertion after 68.07 seconds:
  `Manager.watch` had consumed the single Window.Done result. This was a fixture
  assertion defect, not evidence that ownership remained live. Receipt:
  `build/bridge-handoff-c41f9ec0-6cd4-4f0d-a7e0-f21d56105e95/receipt.json`.
- After changing the fixture to repeatable CloseForRetry ownership readback,
  the authorized stop/test/restore smoke passes in 9.38 seconds. Authenticated
  Bridge bootstrap, real close ACK, natural owner drain and headless generation
  2 Ready on the same disposable profile are verified. Receipt:
  `build/bridge-handoff-120669ea-5960-4dce-8724-af7265790fc2/receipt.json`.
  Both runs restore the original healthy runtime, compatible Bridge and exactly
  one original-profile Chrome owner. No installation or registration change.
- This closes the static Bridge bootstrap/direct-Manager handoff smoke gap.
  It does not close phase 2d: hidden collector routing, independent interactive
  survival, extraction-worker failure containment, automatic Coordinator return,
  Quiet UI/source parity and authenticated X/Facebook comparisons remain open.
  The existing full package tuple supplies Chrome/Bridge/Node to these source
  fixtures; it does not contain the new managed transport binary yet.

### Quiet hidden backend implementation checkpoint (2026-10-02)

- Local code now binds a generation-specific browser Quiet backend to the Go-owned
  private protocol. X/Facebook in both Quiet policies use hidden targets created
  from birth; Adaptive and other sources retain Bridge. Browser ownership and
  captureRuntime stamps remain unchanged; captureCollector is persisted separately.
- Initial and replacement headed hosts use exact-target explicit navigation after
  bindings exist. Hidden source targets have independent flattened sessions and
  fixed 1280x900 page metrics in the default authenticated context. The broker
  rejects worker browser/target-management commands and tracks only created IDs.
- A borrowed worker owns Node only and forwards source-scoped CDP requests to Go.
  Its RPC replies bypass the capture command queue. Worker timeout/protocol failure
  stops Node and retires only hidden targets; root pipe and capture Job stay owned.
  Transport bounds are now 16 MiB response frames and 8 MiB requests, covering
  bounded extractor injection/results without making the stream unbounded.
- Durable routes fence claims before mutation, filter each consumers pending work,
  pin followups/media, preserve legacy Bridge commands, and fail closed on unknown
  or corrupt stamps. Quiet source authorization is checked again before capture.
  Per-route split dispatch acknowledges the internal pump while other Bridge
  actions and browser source coverage remain intact.
- Required host-only retirement is newly negotiated by Bridge. Go retires hidden
  targets before requesting hostOnly:true; the Bridge client skips ordinary-tab
  cleanup and revalidates/closes only the authenticated static host. Older Bridge
  capabilities block handoff explicitly. Legacy unset retirement stays unchanged.
  Untracked/legacy ordinary windows can therefore keep profile release pending.
- Full store/engine/httpapi suites passed at the routing checkpoint (8.433s/3.841s/
  2.599s); later host-only and corrupt-route focused checks pass. Collection,
  appshell and main checks pass; Node worker/collection-mode tests pass 12/12.
  Bridge split-client negotiation/legacy tests pass 23/23. UI Quiet labels now
  describe hidden X/Facebook only when that backend is configured; rendered
  validation for these new labels remains open.
- The isolated native worker-failure smoke passes in 8.86s with pinned Chrome152:
  two hidden targets, shared fixture cookie, independent minimized interactive
  window survival, retained root pipe, exact hidden cleanup and natural owner
  drain after the last ordinary window closes. Actual pinned Node RPC extraction
  with fake CDP passes in 8.523s, preserving IDs, partial quality and unresolved
  media. Neither fixture establishes authenticated source parity.
- New full package validation and automatic native borrow/return smoke are next.
  End-to-end login/reader-close auto-return, source parity, focus measurements and
  release/install acceptance remain open. No installed runtime/settings changed.

### Packaged Quiet and native handoff checkpoint (2026-10-02)

- Implementation commits: Sidecar `f8b3f2a` and `266f33c`, Bridge `e8136f7`.
  Exact owned-target disposal now polls for up to three seconds before child
  session detachment. Delayed disappearance and cancellation/retry tests pass
  (`0.104s`). No ordinary target is adopted during polling.
- Full local candidate:
  `AkuBrowser/build/quiet-hidden-verified-20261002/AkuBrowser-0.9.0-windows-x64-installed-app`.
  Builder and tuple verifier pass: 412 files / 588,821,744 bytes, Chrome
  152.0.7977.54. Sidecar binary SHA-256:
  `cc4ac7513c0004c1f83b7dde199c1b4ddebcf039c5c741aa29c80466dea608ea`.
  Source identities: Sidecar `266f33c`, Bridge `e8136f7`, Browser `036d839`.
  Untracked experiments were excluded from commits. Later test/documentation
  changes do not alter this candidate payload.
- Rendered isolated Settings fixture passes in `82.23s`: hidden Quiet wording,
  browser -> headless -> browser -> headless, browser-only visibility control,
  and headless selection retained after reload. Fake process/collector owners
  do not establish native handoff or source parity.
- Real static Bridge smoke now proves the initial browser -> headless handoff:
  negotiated host-only retirement, successful close ACK, hidden target disposal
  and natural owned-tree/profile drain. The former one-shot target listing raced
  Chrome's asynchronous destruction; bounded polling corrected that cause.
- The extended generation-3 interactive-window/auto-return fixture still fails
  its safety assertion. CDP window lookup reports another `about:blank` page in
  the fixture window, both after CDP target creation and Bridge windows API
  creation. Its provenance and native association remain unverified. The fixture
  refuses WM_CLOSE on a window containing an unknown page; production retention
  rules are unchanged. Closing only the fixture tab did not clear the tracked
  HWND. This does not establish completed auto-return acceptance.
- Latest diagnostic:
  `AkuSidecar/build/bridge-handoff-8ca282de-7993-4dd9-adf8-d195db2ffdaa`.
  Smoke fails in `5.54s` after the first handoff proof, with original Supervisor
  runtime restored and Bridge compatible. Every controlled run restored the
  original runtime. No candidate installation or settings replacement occurred.
- Next: identify that blank target without adopting or closing an ordinary tab;
  prove an isolated native reader/source window lifetime and automatic return
  after closure. Then finish authenticated X/Facebook parity, integrated login/
  native reader journeys, visibility and release gates. Authenticated profile
  collection remains separate from approved disposable static stop/test/restore.
  The new one-million-token segment is nearing its ceiling; continued diagnosis
  requires a budget/scope decision.

### Bounded blank-target provenance diagnostic (2026-10-02)

- A fixture-only protocol recorder now records successful hidden-target creation
  responses directly; it never adopts IDs from target inventories.
- The additional `about:blank` page does not match any recorded machine target
  in the borrowed browser generation. Receipt:
  `AkuSidecar/build/bridge-handoff-89fdcdce-8e71-430c-9aaa-371d9fc7739d`.
  Diagnostic fails its safety assertion in `5.53s`, after initial real Bridge
  handoff succeeds; original runtime restoration is verified. This rules out
  treating the unknown page as a Quiet collector or excluding it from safety
  checks on that basis. Its origin remains open.
- Next investigation should distinguish pages already present after browser
  re-entry from pages created with the interactive window. Do not close/adopt
  the unknown target or weaken production native lifetime checks. No production
  code or candidate payload changed in this diagnostic continuation.
- A subsequent before/after inventory proves the unknown blank target was not
  present before interactive-window creation. Receipt:
  `AkuSidecar/build/bridge-handoff-1f2afc62-fbc1-4b2e-a763-68c5fbc0c7ae`;
  diagnostic fails safely in `5.49s`, original runtime restored. This narrows the
  next investigation to window creation/CDP target representation. Compare the
  exact newly created Chrome window's actual tabs against its CDP page targets
  before interpreting either inventory as permission to close a native window.
- Chrome windows API subsequently reports three tabs in that window, only one
  matching the fixture URL. Receipt:
  `AkuSidecar/build/bridge-handoff-b6f83bb5-e15e-48d3-84e5-2afe680f96c2`;
  diagnostic fails safely in `5.54s`, original runtime restored. Two additional
  tabs remain unidentified. The reported zero blank-tab URLs does not prove
  absence because URL permissions/availability may differ from CDP. Do not
  classify this as merely a CDP representation artifact or close the window.

### Headless startup/session carryover diagnosis (2026-10-02)

- User added one million tokens to the previously reset one-million task ceiling:
  the cumulative ceiling is now two million, with the saved default unchanged.
  The previous native goal is unavailable; its last measured 979,770 tokens remain
  recorded. A new native goal measures only the additional one-million segment.
  Aggregate coverage remains partial; this does not reset historical usage.
- Same-profile disposable A/B proves the first ordinary window has one tab before
  the headless cycle and three afterwards. Receipt:
  `build/bridge-handoff-31b9b6d9-32d9-49a0-821b-afd609cd95ea` (5.49s).
  Missing `session.restore_on_startup` is preserved separately from explicit zero;
  Local State reports `was_restarted=false`. Original runtime restoration passes.
  Extra target IDs are new and do not match broker-owned hidden targets.
- The old standalone headless worker launches an ordinary blank startup page and
  eagerly creates a second ordinary blank page. The observed carryover fits
  Chromium session restoration when the later first ordinary window opens.
  No profile preferences, session files or unknown targets were changed.
- A no-startup-window/lazy-hidden implementation encountered Chromium's initial
  frame prerequisite: hidden creation fails when no remote-debuggable frame exists.
  The loopback-only helper reports the exact failed stage; native acceptance is
  pending a bounded owned-bootstrap correction. This is not a pipe-access failure.
- HTTP API, collection drivers and capture-runtime suites pass after the fixture
  diagnostics update (HTTP API 2.400s). Authenticated parity and native auto-return
  remain open; the existing packaged payload is unchanged.
- The bounded correction now passes native Chrome 152 loopback smoke (4.49s):
  lazy hidden X/Facebook targets share the default-context cookie, and the cookie
  persists through a clean restart of the same disposable profile. Only the exact
  temporary bootstrap target is closed; its disappearance is verified before
  the source context is returned. Chrome starts without ordinary startup tabs.
  Fixture: `internal/collection/headless/worker/test/headless-machine-smoke.mjs`,
  profile evidence under `build/headless-machine-smoke-20261002-c`.
  Focused worker tests pass 6/6 and syntax checks pass.
- The first corrected Bridge wrapper attempt stopped at preflight because an
  original Inbox session was running; no Supervisor interruption occurred.
  Native auto-return and packaged-worker verification remain pending.

### Packaged native auto-return acceptance (2026-10-02)

- Implementation checkpoint: Sidecar `4530d31`, Bridge `e8136f7`, Browser
  `036d839`. New candidate:
  `AkuBrowser/build/headless-machine-20261002/AkuBrowser-0.9.0-windows-x64-installed-app`.
  Full builder and verifier pass: 412 files, 588,823,401 bytes, Chrome
  152.0.7977.54 and pinned Node 24.16.0. Sidecar binary remains
  `cc4ac7513c0004c1f83b7dde199c1b4ddebcf039c5c741aa29c80466dea608ea`;
  this correction is in the separately hashed worker payload.
- Default packaged-worker static Bridge smoke passes in 17.52s, without a source
  worker override. Before and after the headless cycle, Chrome's window API reports
  exactly one fixture tab. Host-only negotiation, hidden-target retirement, real
  close ACK and natural profile drain pass. Browser generation 3 remains retained
  while the fixture native reader HWND exists; exact window closure automatically
  returns to headless generation 4 Ready. Receipt:
  `build/bridge-handoff-1b80fb31-affb-4b4a-b106-dd5915b6cade/receipt.json`.
- Supervisor restores the original healthy runtime, compatible Bridge and one
  original-profile Chrome owner. No candidate installation, registration change,
  authenticated social collection or original-profile cookie change occurred.
- This closes the static native auto-return gap, not the live login/native-post
  journey or source parity gates. The fixture substitutes source readiness and
  uses a loopback reader page; authenticated X/Facebook QA remains separate.
- Compatibility smoke also passes in 4.47s with the currently configured Google
  Chrome 154.0.8037.93, using another disposable loopback profile. Same-context
  cookies and restart persistence pass. Live authenticated QA must retain this
  configured capture executable, rather than move its profile to packaged CfT.
- Concrete next authorized-scope decision: a bounded read-only comparison of X
  and Facebook using the registered authenticated capture profile and configured
  Google Chrome. Preflight must require no active Inbox session, one exact profile
  owner and healthy Bridge. Supervisor stop/test/restore remains mandatory; keep
  results under project build, use production extractors/worker, preserve unknown
  identity/media states and restore original health/Bridge/profile ownership.
  Do not install the candidate, change registration/settings, copy cookies or
  perform account writes. Disposable-profile approval does not authorize this
  authenticated-profile run; obtain that concrete approval before execution.
- After interruption, native goal readback is paused at 335,669 tokens for the
  additional segment; supported tools cannot resume it. Prior 979,770 tokens and
  the cumulative two-million ceiling remain preserved. Total measured coverage
  is partial; current post-resume accounting/enforcement is unavailable.

### Authenticated packaged-worker comparison (2026-10-02)

- User approved the original authenticated capture profile for bounded read-only
  X/Facebook QA with Supervisor stop/test/restore. The operator harness defaults
  to read-only preflight; explicit `--allow-runtime-stop` is required. It uses the
  registered Google Chrome executable and profile, with candidate pinned Node and
  worker. Reports remain under ignored project build; no observations are imported.
- First packaged-worker run completes in 44.13s with Chrome 154.0.8037.93:
  `build/authenticated-parity-cee75349-fc50-431b-8dd8-ff9bbfc93f88`.
  X feed/followup/target succeed (5 unique native IDs, 13 observed blocks, 4 media
  observations); no within-snapshot duplicate ID is observed. X target identity
  and author match its saved Timeline baseline, but text/media parity does not.
  The text difference is one rendered URL token; all other tokens match. The
  saved video is absent in this target observation; DOM/hydration cause is open.
- Facebook feed returns `empty_unverified`, with no claim of true empty source.
  Native recapture succeeds with authenticated UI, matching native ID, author and
  normalized text. One image is present in both captures; its CDN host/path match,
  while query strings differ. Exact URL equality and full media parity remain
  unverified. Native structured-story time is observed in this recapture only.
  A second saved numeric-ID/pfbid-URL alias remains explicitly unverified.
- Explicit worker shutdown, worker exit and zero profile owner before restore
  pass. Original runtime returns healthy, Bridge compatible and one registered
  Chrome owner. No login/cookie copy, installation, configuration replacement or
  account write occurs. This compares saved Timeline evidence with sequential live
  headless capture; it does not establish simultaneous headed/headless parity.
- Comparison guards reject text-only matches, ambiguous same-ID bindings and
  unverified native-ID/URL relationships; unavailable media remains separate from
  observed zero. Focused comparison tests pass. Native baseline/continuation
  harness fixtures and syntax checks pass.
- Next diagnostic delta: one X target DOM sample at first admission/+2s/+5s,
  and bounded structural counts on Facebook `empty_unverified`. The diagnostic
  wrapper is project-build-only and explicitly recorded as instrumented. Empty
  diagnostics exclude page text, URLs and native IDs; privacy/unknown-state test
  passes. This instrumentation is evidence gathering, not source acceptance.

### Authenticated source diagnosis and bounded readiness correction (2026-10-02)

- Instrumented X target receipt:
  `build/authenticated-parity-3cd18e8b-1014-4799-b7d8-055126973223/report.json`.
  The first admitted snapshot has text and an empty attachment shell. At +2s
  and +5s, the same native post has a ready video element and poster accepted by
  the existing selector. This proves a hydration timing gap; no shared vendor
  selector change is justified. Original runtime restoration passes.
- Headless capture now re-samples X missing expected media URLs for up to three
  additional seconds, bounded by the source hydration and capture deadlines.
  Video stream resolution remains explicitly unresolved. The focused fixture
  admits the hydrated poster and preserves that uncertainty. Native packaged
  verification of this correction is pending.
- The X text mismatch is one rendered link: baseline URL 54 characters, current
  visible URL 19 characters. All other word tokens match in all diagnostic
  samples. Keep exact text unequal and compare URL-normalized prose separately;
  do not reconstruct a destination from a saved baseline.
- Official packaged Facebook diagnostic receipt:
  `build/authenticated-parity-6683d3d5-fe5d-4abf-935e-008e89c38ced/report.json`.
  Feed now returns `invalid_observation`; native target still succeeds with one
  image. The old QA report discarded the validation message. The wrapper now
  retains its bounded message privately, and invalid-observation responses carry
  bounded structural diagnostics excluding body, URLs, author and native IDs.
  The focused invalid-permalink fixture verifies that privacy boundary. No
  Facebook extractor correction is claimed before the precise failure is known.
- Each authenticated diagnostic explicitly shuts down the worker, waits for
  profile release and restores original healthy runtime, compatible Bridge and
  one configured Chrome owner. Source acceptance, actual social reader/login
  journeys and final visibility gates remain open.

### Packaged media correction and Facebook URL-contract cause (2026-10-02)

- Sidecar `e3322be`, Bridge `e8136f7`, Browser `036d839` candidate:
  `AkuBrowser/build/headless-media-ready-20261002/AkuBrowser-0.9.0-windows-x64-installed-app`.
  Builder and operator tuple verifier pass: 412 files, 588,826,200 bytes;
  pinned UI Chrome 152.0.7977.54 and Node 24.16.0 are unchanged. Authenticated
  collection retains configured Google Chrome 154.0.8037.93.
- Official packaged-worker receipt:
  `build/authenticated-parity-437a0549-0d0b-4d5c-91e9-07d733e42752` (63.62s).
  X feed/follow-up/target all succeed: 5 unique native IDs, 13 blocks, 7 media
  observations. The target now contains one video poster. Native identity/author
  and URL-normalized prose match; exact text and saved resolved video media
  remain different, with `unknownVideo=unresolved`. A second X feed identity
  has an author-binding mismatch against its older saved baseline; do not admit
  it as a parity match. Facebook target succeeds; feed validation remains open.
- Private instrumented Facebook receipt:
  `build/authenticated-parity-90a90467-389b-4018-b966-cbfef52f2bc3`.
  Rejected native permalink is HTTPS on an allowed Facebook host, exact watch
  path with a numeric video query matching its native post ID; no credentials or
  port. The shared adapter supports this canonical form, but headless URL
  validation omitted it. Correct the headless-only contract and ID check, then
  verify official packaged feed/follow-up capture. No shared collector change.
- Original runtime/profile/Bridge restoration passes both runs. No source data
  import, installation, registration change, cookie copy or account write occurs.
- A passive operator-only Windows observer reuses `internal/nativetrace`, starts
  before candidate Chrome and stops before restoring the original UI. It records
  bounded window metadata, never titles, contents, input or screenshots. Exact
  init PID attribution is separate from unrelated Chrome windows. Native helper
  smoke outside the sandbox confirms available trace start/end and clean exit;
  aggregation guards preserve partial coverage and never promise zero blinking.
  Live source visibility verification with the helper is pending.
- Pipeline inspection identifies the same missing watch/video URL form in Go
  `domain.CanonicalSourceURL`, used by engine observation validation and recapture.
  The correction aligns this shared URL contract with the existing adapter;
  it does not change the production collector. Only exact watch/video.php paths,
  HTTPS trusted hosts and a single numeric video ID of at most 32 digits are
  admitted. Worker observations additionally bind this ID to their native post ID.
  Domain URL and engine media-only browser/headless fixtures pass; full domain,
  engine and store suites pass (0.017s/3.691s/8.656s). Worker/comparator/native
  observer focused checks pass 17/17. Official packaged source QA remains next.
- The second X author mismatch is isolated to a relative-time token inside the
  shared adapter's author header. Handle, remaining header, native permalink and
  text match. Keep the strict comparison mismatch visible; do not silently strip
  metadata or equate authors from text alone. The saved sequential baseline is
  not proof of an incorrect native author binding.

### Official packaged Facebook feed and passive Windows acceptance (2026-10-02)

- Candidate tuple: Sidecar `51d5378`, Bridge `e8136f7`, Browser `036d839`.
  `AkuBrowser/build/headless-facebook-watch-20261002/AkuBrowser-0.9.0-windows-x64-installed-app`.
  Builder and tuple verifier pass: 412 files / 588,830,979 bytes. Sidecar binary
  SHA-256 `52f1d6fdfe44b87a1e579f5134a584d98b7263abffb874d79551a7ac04527930`.
- Official packaged-worker receipt:
  `build/authenticated-parity-15be6fe4-1430-4b52-8d1d-0b4995773066`.
  X and Facebook feed/follow-up/native-target all pass (6/6): X 5 unique IDs,
  14 blocks, 7 media observations; Facebook 2 unique IDs, 5 blocks, 5 media
  observations. Every capture reports authenticated UI, no login/challenge and
  no within-snapshot duplicate IDs. Facebook watch feed now passes the URL
  contract. Repeated frontier observations are not proof of full feed coverage.
- Worker shutdown/exit, profile release before restore, original runtime health,
  compatible Bridge and one configured capture owner all pass. No observations
  are imported, and original settings/registration remain unchanged.
- Passive native trace spans 05:09:29.129-05:10:43.973 UTC, starts before candidate
  Chrome and ends after worker exit, before restoring the original UI. Nine
  changed-state/event records, complete start/end with available hooks and no
  truncated window inventory. Exact init PID has zero sampled visible windows
  and zero foreground events. This closes bounded standalone headless visibility
  evidence, not a universal zero-blinking guarantee or Quiet/interactive evidence.
- Optional read-only engine check on the private receipt validates all six
  observations / 19 blocks with `validateObservation`. It requires an official
  packaged-worker lifecycle receipt under project build and never ingests or
  prints private source content. Focused Go check passes (0.022s).
- Saved Timeline comparison remains partial: X target has matched native identity,
  author and prose with URL tokens replaced, but only a video poster; Facebook
  target matches native identity/author/text and CDN image path, with changed
  query and explicitly estimated current timestamp. Unobserved older feed targets
  and unverified numeric/pfbid aliases remain distinct from proven source absence.
- Next implementation delta: reuse the existing Bridge self-contained MAIN-world
  structured video resolvers in the headless worker, with exact-ID and trusted
  media URL admission, bounded execution and provenance. Preserve unresolved
  media when evidence is absent or conflicting. The original Bridge collector
  stays unchanged. Then verify the new official package and remaining native
  interactive/Quiet journey gates.

### Quiet authenticated QA preparation (2026-10-02)

- Operator-only `scripts/quiet-authenticated-probe` borrows production
  `quiet.Targets` and `quiet.Worker`, with the candidate's pinned Node/worker.
  It launches a minimized unique loopback host through the Go-owned private CDP
  pipe, without loading or registering a new extension. The existing QA harness
  accepts an explicit `--quiet-probe` binary and preserves stop/test/restore
  guards. Reports and passive visibility summaries distinguish Quiet from
  standalone headless; a minimized visible host is not an exposed source window.
- Cleanup retires Node/hidden targets before closing only the exact unique host.
  No `Browser.close` or root termination is permitted in this adapter. Natural
  root/Job drain must complete before profile reuse. Remaining ordinary windows
  retain the private pipe and block restoration rather than being adopted or
  closed.
- Disposable-profile native fixture passes with configured Google Chrome
  154.0.8037.93 (0.813s): hidden source setup, exact host retirement, an independent
  fixture retaining the root pipe, then exact fixture closure and natural drain.
  Six host-binding cases reject unknown restored blanks, non-page targets,
  malformed and ambiguous identities. Comparator/visibility tests pass 11/11.
- This is QA tooling and fixture evidence. Authenticated Quiet source capture,
  comparison against headless, application Bridge/Settings and real source
  login/reader auto-return still require their own receipts.

### Structured video candidate (2026-10-02)

- The shared headless/Quiet worker now reuses the existing self-contained X and
  Facebook Bridge MAIN-world media resolvers. Original module bytes retain their
  own SHA-256 provenance; a serialized function is a separate execution binding.
  Missing optional resolver code leaves DOM collection intact and unresolved.
- Only exact native post ID/permalink bindings are requested, at most 16 IDs,
  with bounded traversal and CDP time inside the capture deadline. Trusted HTTPS
  poster/MP4 pairs must match the observed DOM poster host/path. Unmatched images,
  mixed posters, unknown aliases and conflicting candidate/playback bindings are
  preserved rather than synthesized. Verified own playback can clear that video's
  unknown flag; overall capture quality remains unverified.
- Worker/media focused checks pass 17/17. Coordinator checks pass 19/19 across
  resolver and receipt comparators, including wrong-source resolver revisions,
  foreign/unsafe media, mixed posters, ambiguity, provenance and playback URL
  comparisons. The Quiet/headless comparator accepts only proven official
  lifecycle receipts and requested native targets; it never claims full parity.
- Read-only Go receipt validation now accepts either official standalone headless
  or production Quiet-driver receipts and verifies the declared capture mode,
  worker exit and profile release. The earlier headless receipt still validates
  six observations / 19 blocks without ingestion (0.023s).
- The new candidate needs an immutable local build and authenticated headless /
  Quiet receipts. Operator helper binary digests are retained in new QA reports.
  This checkpoint does not close live-source or integrated interaction gates.

### Native structured-media / Quiet comparison checkpoint (2026-10-02)

- Local candidate:
  `AkuBrowser/build/headless-structured-media-20261002/AkuBrowser-0.9.0-windows-x64-installed-app`.
  Sidecar `557d721`, Bridge `e8136f7`, Browser `036d839`; verified payload 413
  files / 588,849,490 bytes. UI CfT 152.0.7977.54 and pinned Node 24.16.0.
  Authenticated source QA uses the configured Google Chrome 154.0.8037.93 and
  original profile. No installation, release, registration or account writes.
- Official headless receipt
  `build/authenticated-parity-513c2d75-5572-4c1b-a883-751c25514851/report.json`:
  X/Facebook feed/follow-up/target 6/6; 19 blocks pass the read-only Go engine
  observation validator (0.025s). Runtime/Bridge and one configured profile owner
  restore successfully after explicit worker exit and profile release.
- Production Quiet-driver / packaged-worker receipt
  `build/authenticated-parity-a1112111-92e6-4e18-8d77-d8c3a07f4070/report.json`:
  X/Facebook 6/6; 17 blocks validate in Go without ingestion (0.024s). Exact
  hidden-target/host cleanup, natural profile drain and original restoration pass.
- The saved `quiet-headless-target-comparison.json` proves matching target native
  identities, authors and exact text with identical Chrome product and extraction
  asset hashes. Facebook image host/path matches with CDN query differences.
  X is not media-equivalent: Quiet has no poster while headless has one. Both
  structured resolvers report no matching own playback and retain unknown video.
  Quiet X feed also observes image expectations without media URLs. Diagnose
  renderer hydration and resolver input boundaries before qualifying parity.
- Latest passive visibility receipts are partial because the shared inventory
  cap truncates other Chrome windows. Recorded headless samples have no visible
  root or root foreground event; this does not replace complete coverage. Quiet
  positively records an initial host show/foreground at 06:09:05.166 UTC, then
  minimized state at 06:09:05.291 UTC. Preserve this startup finding separately
  from later hidden source collection; no zero-blinking claim is justified.
- Remaining gates are still open: X media parity, explicit video expectations
  for native Facebook video URLs, qualified visibility, real login/reader return
  and integrated Settings/auto-update/recapture journeys. Native capture success
  and contract admission do not establish complete source quality or release.

### Native media diagnosis and exact-root observer (2026-10-02)

- Native Facebook watch/video/reel permalinks with an exact own ID now retain a
  video expectation even when the DOM exposes only an image. Missing structured
  playback stays unknown; unrelated images and unverified aliases are preserved.
- Bounded count-only structured-media diagnostics distinguish no exact returned
  candidate, no safe own poster/playback pair, and DOM path mismatch. Coverage
  retains at most eight snapshot summaries; unavailable counters remain null.
  Worker focused checks pass 17/17, including the video-expectation delta.
- QA-only Windows observation binds the exact source Chrome PID after worker
  initialization. Its independent aggregate scan cannot be displaced by other
  Chrome windows in the shared sixteen-window inventory. The startup/global
  trace remains separately qualified and retains the previously observed host
  activation. After-init sampling cannot establish absence of startup flashing
  or visibility shorter than the 100ms polling interval.
- Observer summary checks pass 4/4. The disposable Google Chrome 154 fixture
  observes its known minimized root without an exposed window (0.460s); a
  passive exact-PID handshake also reaches a clean start/end receipt. These are
  tool/fixture checks, not authenticated source acceptance for the new delta.
- Next: immutable candidate build and new official authenticated receipts to
  resolve the media diagnosis and qualify collection-phase visibility. Actual
  Settings/Bridge/social reader/login integration gates remain open.

### Transition implementation checkpoint (2026-10-02)

- Production pins the Chrome subprofile selected from Local State before initial
  headed launch and carries it through headless and browser replacement. Missing
  selected profiles and malformed metadata fail explicitly; no cookies are copied.
  Six selection cases pass, including a non-Default authenticated profile.
- Headless's request deadline now includes pipe writes. A blocked write retires
  the owned worker tree before returning. Two pipe tests pass; a real Windows
  owned-Node fixture with unread stdin verifies timeout and Job release (8.10s).
- Official candidate 13a113b headless receipt
  `build/authenticated-parity-7548d56f-e9ff-4c4a-b07a-0f6eb593ea8e/report.json`
  passes six captures and 17-block Go admission without ingestion. Its exact-root
  after-init trace completes over about 62 seconds with no visible/foreground
  root. Quiet receipt `build/authenticated-parity-50495749-1efc-4013-b8ab-3fa87127b185/report.json`
  also passes six captures, but still lacks the X poster and records startup
  foreground activity. Both restore the original healthy runtime and profile.
- Count-only X diagnostic receipt
  `build/authenticated-parity-72e2b2ba-da44-47fb-b134-7bdf366c673c/report.json`
  isolates a depth boundary: default depth 9 visits 266 nodes, depth 12 visits
  408, neither matches; a bounded diagnostic finds own Tweet objects at depth 13.
  This instrumented run is not an official parity receipt and restores cleanly.
- The shared X resolver now accepts an explicit depth up to 16 while retaining
  Bridge's default 9. Headless requests 16 with the same 1500-node and time caps.
  Bridge resolver/runtime checks pass 9/9, including deep own/quoted media and
  depth-cap fixtures; worker checks pass 17/17. New packaged/live proof is pending.

### X playback-format diagnosis (2026-10-02)

- Candidate Sidecar 435d7a1 / Bridge 208b705 passes immutable local packaging.
  Official target receipt `build/authenticated-parity-2daa58ed-93e9-4ea7-b16f-f31f960955be/report.json`
  now finds three matching structured objects / one candidate within 841 nodes,
  but retains unknown video because no safe poster/MP4 pair is returned.
- Count-only diagnostic `build/authenticated-parity-bafdf833-9a32-47c5-9093-fbda38cdef43/report.json`
  confirms a JPG poster paired to HLS plus four unpaired MP4 variants. Both runs
  release the profile and restore the original runtime. This is format evidence,
  not playback or full-parity acceptance.
- Headless now explicitly requests MP4 pairing by matching native video asset
  family/ID, preferring the available larger resolution. Bridge's unspecified
  default keeps existing pairing behavior. Foreign asset variants and HLS-only
  evidence stay unpaired. Ten Bridge resolver/runtime and nine worker checks pass.
  New official package/source validation is recorded in the next checkpoint.

### X MP4 and selected-profile native validation (2026-10-02)

- Immutable candidate `headless-transition-xmp4-20261002` uses Sidecar `107d905`,
  Bridge `8dd4d6d`, Browser `036d839`; 413 files / 588,866,906 bytes verify.
- Official receipt `build/authenticated-parity-366d44de-b797-4201-8c36-d4fbdd82b0b8/report.json`
  passes X feed/follow-up/target and Facebook feed/target: five observations /
  14 blocks pass Go admission without ingestion. Facebook offers no valid
  continuation in this run; its follow-up is untested, not a sixth success.
- X target media is now video with one own playback URL and resolved media
  recovery, without the unknown-video flag. This proves the returned metadata,
  not a decoded playback frame or full legacy parity.
- Exact-root after-init visibility observation completes over about 72 seconds
  with zero visible/exposed/foreground root samples. This excludes startup and
  intervals shorter than sampling. Worker exit, profile release and original
  runtime/Bridge restoration all pass.
- `TestAuthenticatedCaptureReceiptObservationContract` permits bounded official
  partial journeys to validate admission separately. The original parity test
  still requires every X/Facebook feed/follow-up/target label; neither accepts
  diagnostic workers or missing lifecycle proof.
- `TestOwnedChromeSelectedSubprofileSmoke` launches packaged worker and configured
  Google Chrome 154 against a disposable Local State selecting Profile 2. It
  verifies Profile 2 Preferences are created, Default Preferences are absent,
  and owned cleanup completes (3.428s). No authenticated profile is modified.
- Facebook Watch target receipt `build/authenticated-parity-a8e56acc-a107-4516-8ede-3f779236dbfa/report.json`
  reports `empty_unverified`, despite authenticated/ready state. Count-only
  diagnostic `build/authenticated-parity-f31de379-82eb-43c5-b339-a9bd7e97aa56/report.json`
  sees a Watch page with two video elements but no admitted post candidates and
  no structured media candidate within headless limits. Both restore cleanly.
  The target derives from an earlier headless feed observation, not a legacy
  acceptance baseline. Legacy content-script uses the same adapter discovery
  and admission; a live legacy comparison remains required before classifying
  this as a headless regression or a shared surface limitation.

### Broader target and decoder proof (2026-10-02)

- Count-only Facebook receipt `build/authenticated-parity-beb4e025-9fd6-45c2-8fd5-080662c39723/report.json`
  repeats the Watch diagnosis using both the headless limits and the legacy
  resolver defaults. Neither returns a candidate; shared adapter discovery
  reports zero structural/admitted candidates. Raising resolver limits alone
  does not repair this surface. Original runtime and profile restore pass.
- Instrumented X receipt `build/authenticated-parity-73d7e36e-92e4-4b12-8272-5d79ab57c481/report.json`
  captures the native video target and a temporary muted element decodes four
  frames at 1920x1080, readyState 4, without a media error. The probe removes its
  element and shuts down cleanly; profile release and original restoration pass.
  This is decoder proof in source Chrome, not the rendered AkuBrowser player.
  An earlier probe `c4f334bb-5894-4601-a3f4-61007de52591` timed out waiting for a
  presentation callback on an invisible element; keep that diagnostic separate.
- The operator harness now supports `--target-index 0|1`. It verifies the selected
  native ID/permalink before stopping the runtime and records the index. A prior
  headless or unverified baseline is no longer mislabeled as saved legacy Timeline
  evidence. Fourteen comparison/selection checks pass.
- Official second-X-target receipt `build/authenticated-parity-6bca3044-556a-4ba3-84c0-26f373fe9e46/report.json`
  captures one native post with exact baseline text and clean restoration. Strict
  comparison reports `author_binding_mismatch`: the saved author label has a
  trailing relative-time token absent on the native target; the handle and
  remaining label match. Do not turn this into a full-parity success implicitly.
- A fresh read-only Timeline baseline examines 417 items and selects two targets
  per source with verified native ID/permalink bindings:
  `build/authenticated-baseline-3617d6a3-abdd-4110-8dd5-c189f183c5c6/baseline.json`.
  The old second Facebook baseline had an unverified ID/URL binding. The fresh
  second-Facebook-target attempt was rejected at preflight with
  `inbox_active_or_unverifiable`; no stop was issued and no capture occurred.
- After the original runtime naturally became idle, official receipt
  `build/authenticated-parity-ad973da3-7542-4281-a9d5-ef91a22e0c22/report.json`
  captures the fresh second Facebook target. Native identity/author and all
  855 text characters match the saved baseline; the single image's host/path
  also matches. The observation passes Go admission, with quality still
  `unverified`. Worker/profile release and healthy original restoration pass.
  This closes that bounded image-post comparison, not Watch/video coverage or
  the complete integrated Facebook gate.

### Post-push Facebook target diagnosis (2026-10-02)

- Checkpoints Sidecar `cbd6eac`, Bridge `8dd4d6d` and Browser `036d839` were
  pushed to their existing main branches; remote SHAs match local commits.
- Legacy Bridge's `isNativePostUrl` rejects the tested Facebook Watch URL for
  native recapture. Preserve this existing scope limitation separately from
  headless regressions; ordinary Facebook feed-video coverage still needs proof.
- The fresh five-image saved Facebook post returns no observation in official
  receipt `build/authenticated-parity-d79555f0-3e6d-4df4-b513-ad7782631cd6/report.json`.
  The QA harness had used X's 12-second hydration budget for both sources. It
  now matches the source defaults: X 12 seconds, Facebook 25, while retaining
  the 45-second capture / six-minute interruption limits. Fifteen QA tests pass.
- Receipt `build/authenticated-parity-f174b771-a638-4b66-8b2d-ad5c80370dbb/report.json`
  remains empty with 25 seconds. Count-only diagnostic
  `build/authenticated-parity-1cef1c5a-690b-4e6b-8dc1-f8181809f2b2/report.json`
  sees a native post page with an explicit unavailable-content notice and zero
  articles, post bodies, post actions or admitted candidates. Both restore the
  original runtime/profile. This does not establish a five-image extraction loss
  or explain why that native page is unavailable in the current session.
- Headless now recognizes a bounded standalone unavailable notice only for an
  empty, exact requested Facebook native target, after readiness. Feed pages,
  redirects, loading shells and text inside posts remain excluded. The typed
  `target_unavailable` result describes this post/current session and disables
  automatic retry; it neither marks the account unavailable nor deletes data.
- Count-only error diagnostics now distinguish structural/admitted candidate
  counts and preserve unknown values. Twelve worker checks and the Go failure
  mapping check pass. Immutable packaging and native verification of the new
  error classification remain pending at this checkpoint.
- Candidate `headless-facebook-availability-20261002` (Sidecar `d6ec336`) verifies
  413 files / 588,868,578 bytes. Initial native receipt
  `build/authenticated-parity-310ef5bb-1330-4378-868d-8440966ec2cd/report.json`
  still returns `empty_unverified`. Count-only follow-up
  `build/authenticated-parity-38af4d76-afc6-4dd6-921e-1df862f396b7/report.json`
  identifies the exact notice variant ending in "right now" inside the main
  region and heading. Both restore cleanly. The bounded exact-line matcher now
  includes that observed variant; native verification must be repeated.
- Final candidate `headless-facebook-availability-v2-20261002` (Sidecar
  `9a869d7`, Bridge `8dd4d6d`, Browser `036d839`) verifies 413 files /
  588,868,593 bytes. Official receipt
  `build/authenticated-parity-9b3c8cd0-5b69-4739-a5f6-8baecf42edad/report.json`
  now returns the expected `target_unavailable` classification. Explicit worker
  exit, profile release and healthy original runtime restoration are confirmed.
  This passes the negative-case classification gate; it is not a successful
  post capture or a full source-parity result. Focused engine routing/failure
  regression checks also pass (0.162s). No installation or default change.

### Foreground-free Facebook rendering investigation (2026-10-02)

- Checkpoint `e8caa0f` was already pushed. Focus-only A/B/A receipt
  `build/authenticated-parity-acdb5bd6-7d11-400f-bbef-321a1c774b86/report.json`
  changes `document.hasFocus()` as requested but does not recover the unavailable
  five-image target. Its saved permalink is `media_parent_id`, inferred from an
  album parent ID and author path. ID/URL consistency is not proof of a working
  post URL. QA comparison now exposes inferred/observed/unknown link provenance.
- Read-only Timeline inventory examines 418 items, including 46 Facebook items.
  All five saved multi-image cases have inferred media-parent links; none is a
  direct-anchor multi-image control. This is a baseline limitation, not proof
  that these URLs are wrong or that focus never affects Facebook.
- Cold-target / feed / warm-target receipt
  `build/authenticated-parity-d8f4c1fa-af87-4f8e-8f15-f65930fd6ff0/report.json`
  still sees unavailable notices at both target visits. Timers run, but animation
  callbacks are zero in all three one-second samples; the bounded feed capture
  times out at 15 seconds. Do not generalize that diagnostic deadline to product
  capture failure. Original runtime and profile restoration pass.
- Disposable fixture `build/headless-frame-probe-ce274055-5c54-4ca1-9bf2-96ad8598e156/report.json`
  isolates the rendering defect: hidden targets return zero animation callbacks
  with focus emulation off or on; ordinary targets return approximately 60/sec.
  Authenticated ordinary-target diagnostic
  `build/authenticated-parity-78b82e2c-7cc4-4eab-b249-b5431e89330d/report.json`
  restores frames and captures two feed posts. The inferred target remains
  unavailable cold and warm. Restoration passes; this is diagnostic evidence,
  not packaged parity acceptance.
- Two-source fixture `build/headless-two-source-frames-1b03cc2a-1dcc-4f43-8366-cd78f047d59d/report.json`
  shows the second ordinary background tab also needs CDP focus emulation:
  its 1.2-second sample changes from zero frames/hidden visibility to 73 frames/
  visible. No OS activation command is used. All diagnostic exact-root observation
  windows record zero visible/foreground samples; global inventories are partial
  and sampling cannot guarantee absence of brief blinking.
- The owned headless launcher now creates ordinary background targets, enables
  focus emulation per target and removes the obsolete hidden-target bootstrap.
  Borrowed Quiet and legacy Bridge code are unchanged. The two-source cookie/
  navigation/restart smoke initially caught missing second-tab rendering, then
  passes with the combined correction. All 39 worker/QA tests pass. Packaged
  authenticated X/Facebook and window verification remain pending.
- Candidate `headless-rendering-20261002` contains Sidecar `1931a03`, Bridge
  `8dd4d6d` and Browser `036d839`; 413 files / 588,868,558 bytes verify. The native
  two-source rendering/cookie/restart smoke passes against Google Chrome 154
  and bundled Chrome for Testing 152. An initial authenticated preflight saw an
  active Inbox and issued no stop; execution resumed only after it became idle.
- Official receipt `build/authenticated-parity-e5b3756f-2dd1-4262-a359-7f901ecc5c89/report.json`
  passes five captures: X feed/followup/target and Facebook feed/target. All seven
  blocks pass Go observation admission without ingestion (0.024s). Facebook's
  direct-anchor target matches native identity, author, text (853 Unicode code
  points) and the image host/path; signed CDN URLs differ. Quality stays
  `unverified`. The X target retains the previously documented author-label
  mismatch. No Facebook continuation was offered, so this is not a six-capture
  or complete legacy-parity pass. Exact-root visibility observation for roughly
  29 seconds records zero visible/foreground samples; global inventory remains
  partial. Worker exit, profile release and healthy original runtime/Bridge
  restoration all pass. No installation or collection-default change.
- Official target regression
  `build/authenticated-parity-6031293f-ddc0-4163-80e9-56e9394acace/report.json`
  retains the X target's owned inline MP4. The inferred five-image Facebook target
  still returns `target_unavailable` with the corrected renderer, so it must
  remain an open URL/availability case rather than a solved foreground issue.
  Exact-root observation for 33 seconds records zero visible/foreground samples;
  worker exit, profile release and healthy original runtime/Bridge restoration
  pass. Next Facebook evidence should use a live directly observed permalink
  for multi-image/feed-video comparison; never invent a replacement post ID.

### Approved Facebook dialog extraction correction (2026-10-02)

The user approved implementing native-post dialog scoping in the headless
collector after read-only receipt `authenticated-parity-f5242ba5-5b8a-48c3-885b-9e11b33933a3`
reproduced unscoped rejection, scoped success, then unscoped rejection on one
post. Scoped and direct navigation matched identity, author, full text and media.
The original unavailable target was not found and remains unresolved.

Scope: bind a single visible dialog to the native route's exact post identity;
wait for identity hydration, exclude the feed underlay and preserve conflicts
within the dialog. No-dialog pages retain existing behavior; ambiguous dialogs
must not select a first/longest record. Legacy Bridge is unchanged. Validate
text, image/video preservation, ambiguity, readiness and native packaged capture
before accepting the change. No default migration or installation is authorized.

Implementation and validation:
- Worker boundary v4 selects a unique visible native-post dialog and keeps all
  bindings of its exact route identity for the existing author/text conflict
  check. Missing identities wait within the existing hydration deadline. Hover
  evidence is limited to that dialog and includes links whose href appears only
  after hover. No-dialog pages retain their existing extraction behavior.
- Forty-eight worker/QA tests pass, including text, multiple images, video
  evidence preservation, wrong IDs, ambiguous/nested/hidden dialogs, readiness,
  no-dialog conflicts and sanitized dialog diagnostics. These fixtures do not
  establish live Facebook video playback parity.
- Initial package `headless-dialog-20261002` passes X feed/followup/target but
  returns empty Facebook feed/target in receipt
  `authenticated-parity-f6ab5163-8490-4e5a-a473-5bc5cf7fb42b`. Feed diagnostics
  show missing identities; target diagnostics motivate the bounded identity
  investigation below. Do not count this run as a source parity pass.
- Instrumented packaged receipt
  `authenticated-parity-0be519a5-0de5-4f6d-bb7a-7e9ad167123c` confirms the new
  collector itself selects `native_post_dialog` after a feed-anchor click and
  direct navigation: both return the same native identity, author, text and
  five image paths. The additional diagnostic projection is no longer needed
  for the initial collection to succeed. This is a bounded diagnostic, not
  full legacy pipeline parity.
- Final candidate `headless-dialog-v2-20261002` verifies 413 files /
  588,871,321 bytes. It contains the uncommitted source delta on `2d486b9`, not
  an installed release. Official receipt
  `authenticated-parity-dd4b1c8f-7063-4785-a131-9c7648d5fb7c` passes Facebook
  feed, continuation and the existing direct-anchor image target. Three
  observations / four blocks pass Go admission without ingestion (0.022s).
  The target matches identity, author, text and image host/path; quality remains
  unverified and signed CDN URLs differ. Original runtime, Bridge and profile
  ownership restore successfully. Exact-root observation records no visible or
  foreground samples; sampling is bounded and does not guarantee zero blinking.
- Remaining identity case: receipt
  `authenticated-parity-43e97876-ea28-4314-9ceb-2542df7d884a` passes Facebook feed
  and continuation but holds the six-image comparison target at
  `dialog_pending_identity`. Count-only follow-up
  `authenticated-parity-d61a6ae0-ecf0-4e17-bfb3-7ea3ffa6162f` confirms that the
  dialog's own permalink ID differs from the requested native route, although
  author and six image paths match; no canonical metadata resolves the binding.
  Do not infer an alias or rewrite stored identity from that similarity alone.
  This limitation, the original unavailable target, and live video/full legacy
  parity remain open. Browser remains default; no installation performed.

### Facebook network and initial-route investigation (2026-10-02)

- Production dialog correction is committed as `e7bc7a2`. Subsequent probes are
  isolated operator diagnostics under ignored `build/`; no production collector
  changes, private API replay, installation or default migration were performed.
- Passive CDP receipt `authenticated-parity-f751aa65-3d99-4555-b3cc-b960971500c9`
  observes 108 Facebook document/Fetch/XHR requests and fully inspects seven
  selected content responses. A live control's feed click issues
  `CometSinglePostDialogContentQuery`; direct navigation carries its identity in
  the initial document. Both captures match identity, author, full text and four
  image host/path values. The initial probe `57442f4e-fec5-4894-a402-66d54590a87c`
  exhausted its body budget; do not use its skipped later bodies as negative data.
- Structured-route receipts `99a147a6-61a3-4c5b-b3e1-945273434b0b` and
  `e3f856d2-6647-42a6-9b2e-7dace9ce3acf` distinguish a server-delivered unavailable
  view from merely finding an error module name. The original target's
  `initialRouteInfo.route.rootView.props` and `hostableView.props` contain the
  unavailable title and `privacy: true`; `SiteData.ef_page` is null. The same
  result appears before and after control navigation. The flag does not prove
  that the post is private or identify the underlying availability/URL cause.
- A saved directly observed control permalink opens in a fresh Chrome process
  without a preceding feed click: its initial route contains `storyID` and
  `SiteData.ef_page = CometSinglePostDialogRoute`. Persistent profile caches were
  not cleared; this is not a cache-free experiment or a same-post comparison.
- A bounded saved Timeline audit examines 686 items, including 58 Facebook items.
  One record matches the original target, retaining `media_parent_id` provenance;
  no alternative observed permalink is recovered. Its internal-click path and
  matched foreground/headless behavior remain untested at this checkpoint.
- An active Inbox preflight blocked a route probe without stopping the runtime;
  execution resumed after the session finished naturally. All four completed
  diagnostic runs confirm worker exit, profile release and healthy original
  service/Bridge restoration. These are diagnostic receipts, not source-parity
  passes. Next evidence: test the exact URL in foreground with the same profile,
  and recover the original post's observed permalink or authoritative identity.

### Foreground control and saved-URL survey (2026-10-02)

- Foreground receipt `authenticated-parity-c0f2224f-b0cf-4df9-a439-3a570251fb38`
  uses the same registered Chrome 154 and authenticated profile, with headless
  and focus emulation disabled. Computer Use visually confirms the exact old
  URL remains unavailable and the control shows text/image. Native observation
  confirms foreground presence; it does not prove uninterrupted foreground.
  Initial error route properties match headless. Original runtime/Bridge restore.
- Survey snapshot contains 1,000 saved Timeline items, including 86 Facebook
  items: 11 lack a permalink; 75 unique destinations are tested. Among 40 post
  URLs, 31 are observed anchors and nine are inferred media-parent URLs. One
  inferred URL (the original foreground-tested case) explicitly reports
  unavailable: 1/40 post URLs, or 2.5% observed in this saved sample.
- The other post results are six exact-identity captures and 33 pages with post
  structure but unverified target identity. Do not count all 39 as parity passes.
  Among 35 photo URLs, 28 show matching loaded media; seven remain unverified.
  Tracking parameters are not photo identity; 23 initially inconclusive photo
  cases are rechecked using photo ID plus visible media host/path matching.
- The five batch receipts are `587b84e0-592d-45c4-89a7-d1cf7a26ebf4`,
  `76a379c6-b88d-46b7-a2fe-e66fce99d261`,
  `6a9169c9-90d2-46f6-9fff-997b788e4a76`,
  `eaa6f72c-2c3b-4736-a337-38bc4359dd61`, and
  `bd497971-1390-480a-8213-9bc1147b1c30`, each under
  `build/authenticated-parity-<id>/report.json`. All confirm worker exit, profile
  release and healthy service/Bridge restoration. Summary:
  `build/facebook-url-survey-summary-20261002.json` (private local diagnostics).
- This is a bounded availability survey of saved data, not a random Facebook
  prevalence estimate or a replacement-quality proof. Its unresolved identity
  cases motivate the fresh paired comparison above; historical URL archaeology
  is no longer the main implementation trajectory.

Fresh pilot preparation: raw observations from the latest completed Browser
runs are read with SQLite in read-only mode, preserving acquisition provenance
before AI selection. The available reference has six unique X candidates
(video evidence) and one Facebook candidate (image evidence), captured at
14:13:31Z and 14:12:48Z respectively. By pilot preparation completion these are
over the 30-minute target gap, so they are not accepted as a fresh paired proof.
Local `build/fresh-collection-pilot-worker.mjs` is prepared with a stale-baseline
guard; no live paired pilot has run at this checkpoint. The next proof requires
new Browser acquisition and broader content coverage, then prompt headless
comparison on the same profile. Latest is not automatically fresh enough.

### Fresh Browser versus headless pilot (2026-10-02)

After checkpoint `ed2e978`, a new normal Browser update completed. Raw SQLite
observations verify `aku-bridge` provenance: X captured five unique candidates
at 14:59:39Z (image/video), Facebook one at 14:58:55Z (image). Existing source
settings were preserved; only X/Facebook evidence enters this comparison.

The headless feed pilot ran on the same registered Chrome/profile about
1.35 minutes after X and 2.15 minutes after Facebook. It captured three X
candidates and one Facebook candidate, but neither source overlapped the
Browser candidate IDs. Different feed samples do not establish either parity
or lost content. This is a successful acquisition smoke, not a quality pass.

An immediate target pilot used URLs from that fresh Browser baseline:

- X: exact native post identity and all 30 text characters matched. The author
  differed only by the Browser's relative-time suffix. Both had inline video
  playback evidence; headless reported readyState 4, loaded=true and playback
  time about 2.37 seconds. Headless additionally returned a video poster, so
  raw media counts (one versus two) are not a missing-video regression.
  This supports one video target, not full video or image parity.
- Facebook: the Browser block's direct anchor is a `/photo?fbid=...` URL,
  although the block carries a `facebook:post` identity. Direct headless
  capture returned `empty_unverified`, not `target_unavailable`. This is an
  unresolved photo-route/extraction/identity case; neither content absence nor
  a headless-only regression has been established. Do not silently translate
  the photo ID into a post URL or count a loaded unrelated image as parity.

Both bounded stop/test/restore runs confirmed worker exit, profile release,
runtime restoration, healthy service and compatible Bridge. Private receipts:
`build/authenticated-parity-ad72fd2a-8a45-4b4c-b08e-e47123502dfe/report.json`
(feed) and
`build/authenticated-parity-5560d695-109c-41db-8953-cef709f51b5f/report.json`
(targets). The local diagnostic worker intentionally returns a diagnostic
completion code; the outer harness does not claim official parity success.
Detailed observations remain in ignored `build/fresh-collection-pilot-20261002.json`
and `build/fresh-target-pilot-20261002.json`, outside tracked source.

Next: distinguish Facebook photo-viewer evidence from post extraction using
the same fresh target, then extend matched fresh samples and continuation.
Repeat the baseline when the 30-minute pairing window expires. Headless stays
opt-in; no production collector change or default switch follows this pilot.

### Fresh photo-viewer diagnosis (2026-10-02)

After checkpoint `14ea20d`, the exact Facebook photo URL from the fresh Browser
baseline was inspected with packaged headless Chrome and the same registered
profile. Four observations at approximately 4/8/12/16 seconds consistently
showed the same `fbid`, a completed document, no login redirect and no explicit
unavailable notice. A visible, loaded image matched the baseline CDN host/path
and had natural dimensions 2048 x 1465. The normalized baseline text also
occurred in page text. The baseline was about 9.8 minutes old at completion.

The packaged extractor nevertheless returned zero posts: its discovery/scope
contained zero eligible candidates, with no subsequent identity rejection.
Two visible article elements existed, but no visible post-message markers and
no visible dialogs. This locates the observed failure at candidate discovery
on the photo surface, before post admission. The exact selector/admission
reason has not been separately measured. Static inspection confirms discovery
uses the shared Facebook adapter's feed/post selectors and admission rules;
the headless native-post dialog scope does not apply to `/photo` routes.

For this sample, foreground rendering is not needed to obtain the image.
Page-wide text presence plus matching image is photo availability evidence,
not a proven author/text/photo ownership binding or complete post parity.
Do not relax post admission or substitute the photo ID as a parent-post ID.
The next scoped work is a photo-surface evidence path bound to the observed
photo ID and existing baseline media, with ownership kept separate, followed
by fresh matched feed/post comparisons. No production source was changed.

Private evidence: `build/facebook-fresh-photo-probe-result.json` and
`build/authenticated-parity-81f5744e-8d73-486d-9710-b0b1d1494508/report.json`.
The receipt confirms worker exit, profile release, restored runtime, healthy
service and compatible Bridge. This remains a diagnostic result, not an
official parity pass or default-mode gate closure.

### Headless photo evidence implementation (2026-10-02)

Checkpoint `882d310` records the preceding diagnosis. The new headless-only
`facebook-photo-evidence.js` reads bounded JSON script records for the exact
photo-route ID, requires Photo metadata, a consistent owner and image path,
and matches that image to a visible loaded DOM image. It rejects conflicting
IDs/owners/images, hidden/unloaded media, untrusted media hosts and excessive
evidence. Photo identity remains separate from post identity. The module does
not infer a parent post, author or body from page-wide text.

The worker loads and hashes this asset, the Facebook snapshot carries photo
evidence, and empty-capture diagnostics expose only an allowlisted status and
the unresolved post binding. Existing post admission is unchanged: a photo
alone still cannot produce a successful post Observation. This implements the
photo evidence stage, not complete photo-to-post collection.

Live source-module injection into the existing packaged headless Chrome
returned `verified_photo_media` on all four observations of the same target:
exact photo ID, owner present and loaded 2048 x 1465 image. Baseline age at
completion was approximately 19.8 minutes. Receipt:
`build/authenticated-parity-43179115-7c57-4b25-90af-2219d73303ff/report.json`.
Runtime/Bridge restoration passed. This is source-module validation; the new
worker asset has not yet been rebuilt into a release package.

An earlier attempt timed out during diagnostic-worker initialization because
an asynchronous file read followed readline construction. Moving that read
before input construction fixed the local harness; its failed receipt also
confirms runtime restoration (`5b07a9dc-ae4f-40d3-bb83-ea10549f8f44`).

Next: validate the structured photo-to-container-story relationship and owned
text/author before adapting it to post admission. Preserve separate photo and
post IDs and compare against fresh Browser evidence. Full parity, continuation
and package validation remain open; Browser remains the default.

### Parent-story validation after photo-evidence push (2026-10-02)

Commit `3bb3151` (photo evidence implementation) is pushed to `origin/main`;
remote SHA verification matched. All 42 worker tests passed before push.

Read-only metadata inspection of the same photo found matching fragments for
both `container_story` and `creation_story`: one consistent numeric parent
post ID, one consistent native `/posts/pfbid...` URL, actor and story identity.
The photo ID differs from the parent post ID. The opaque URL identity and
numeric parent ID are linked by the structured story record, not inferred
through numeric URL construction. The story's author and 71-character text
match the Browser baseline. The photo's own message/owner match as well.

Following that exact metadata-provided parent URL with the existing packaged
headless post collector succeeded: native URL identity matched, author and
all text matched, and the one image matched the baseline CDN host/path.
Private receipts:
`build/authenticated-parity-75bf17ca-a439-4c49-ae3a-f57e97bdb46a/report.json`
(metadata) and
`build/authenticated-parity-6d6bfb82-460e-4a97-8de3-031ae64ddfbe/report.json`
(parent capture). Both restored the runtime with healthy compatible Bridge.
Detailed parent capture is in `build/facebook-parent-target-result.json`.

The baseline was approximately 46.5 minutes old at metadata completion and
48.4 minutes old at parent capture start. Therefore this validates a route
resolution mechanism and content agreement on one sample, not the <=30-minute
fresh parity gate. No automatic photo-to-parent navigation was introduced in
this validation step. Next implementation can use the explicitly bound native
parent URL with conflict checks, a bounded navigation budget and separate
photo/post identities; then repeat with fresh Browser samples. Complete
replacement/default readiness remains open.

### Integrated photo-parent navigation and fresh pilot (2026-10-02)

Checkpoint `0d56b22` records the preceding parent-route validation. Owned
headless target capture now permits one photo-to-parent navigation within the
original capture deadline. A metadata binding requires consistent story IDs,
numeric parent ID, explicit native URL, actor and body across fragments. The
loaded photo evidence is required first. After navigation, the exact native
parent page must corroborate the author, text and photo image path before its
post is returned. Photo ID and native parent identity remain separate in
coverage provenance. Borrowed Quiet behavior is excluded; the shared Browser
adapter is unchanged. Conflicts and redirects fail closed. All 46 worker tests
pass, including conflicting metadata, mismatching post evidence and Quiet.

A new normal Browser update provided a Facebook photo baseline captured at
15:53:52Z through `aku-bridge`. The source-worker integration pilot started
about 1.69 minutes later and completed capture in 6.2 seconds. It resolved the
photo to its parent and returned one parent post with identical author and all
70 text characters. The baseline image matched the loaded headless image's
CDN host/path (headless dimensions 960 x 503 versus Browser 570 x 299).

Headless also returned a second loaded image (261 x 196). Its relationship to
the post has not been independently checked. Raw media equality therefore
does not pass; do not call this complete media parity or silently discard the
additional image. This proves fresh route resolution and baseline-content
retention for one sample, with additional-media ownership still open.

Receipt: `build/authenticated-parity-767a9e7d-15aa-40e7-94d4-8d5d90f6d3ca/report.json`.
Private comparison: `build/fresh-photo-integration-result.json`. Worker exit,
profile release and restoration with healthy compatible Bridge all passed.
The test uses current source assets with packaged Chrome; rebuilding the
release package and downstream recapture/dedup handling of photo-to-parent
identity remain unverified. Next: inspect the additional image's ownership,
then broaden fresh samples and validate downstream identity handling before
declaring replacement readiness. No default-mode change is authorized here.

### Comment-media correction and downstream identity audit (2026-10-02)

The extra 261 x 196 image was independently traced to an article labeled as a
comment/reply, with no post-action header. The primary baseline image has no
such comment article ancestor. Evidence receipts:
`61775c1d-e335-4181-99bc-df88c949b61f` (DOM ancestry) and
`998a178c-df02-43a2-a21e-a69c6c874ffe` (comment-label confirmation), under
ignored `build/authenticated-parity-*/report.json`.

Headless ownership boundary v5 now treats explicitly labeled comment/reply
articles as separate owners even when they contain images. It retains the
existing behavior for unlabeled media-presentation wrappers. The shared Bridge
adapter is unchanged. English/Indonesian comment/reply labels have regression
coverage; other unobserved locale markup remains outside this proof. All 47
worker tests pass.

The repeated integration pilot using the same fresh Browser reference passed
at a 13.25-minute gap: verified photo-to-parent identity, equal author, all 70
text characters, one image on each side and equal media host/path. Capture
took 4.0 seconds. This closes the additional-comment-image discrepancy for
this sample. Receipt:
`build/authenticated-parity-3da15be4-8f9a-47ba-b862-a2e225cb4fb0/report.json`.
Runtime restoration and healthy compatible Bridge were verified.

Read-only downstream audit found an integration gap: store `recapturedBlock`
matches only the job evidence key or native URL; the parent block has neither
the old photo evidence key nor its URL. `nativeIdentityRelation` classifies
different nonempty platform IDs as conflicts. Consequently the successful
worker capture is not proof that recapture updates an existing photo-backed
item or deduplicates it against its parent. No store matcher or persisted
identity was changed during this audit.

Next: introduce a narrowly validated photo-to-parent relation for headless
recapture and identity reconciliation, with mismatch/forgery/ambiguity tests.
Keep photo and parent IDs distinct and avoid merging by text similarity alone.
Full product journey, broader fresh samples and package validation remain open.

### Internal recapture and persisted parent identity (2026-10-02)

Checkpoint `cae5e19` commits photo-parent navigation and comment-media exclusion.
The internal owned headless completion path now has a separate store entrypoint
for accepting a photo-parent result. Public Bridge completion retains its
existing matcher; caller-supplied recovery relation markers are stripped before
persistence. Headless job routing, exact photo ID, native parent URL/platform ID,
saved author/text and recovered image evidence must corroborate the transition.
An item with missing media can use its saved exact photo URL and nonempty caption;
existing saved images, when present, must all be corroborated.

Successful recapture keeps the timeline item's evidence key and real parent
platform identity separate, updates its evidence override, and creates no second
timeline row. The store issues the persisted relation marker itself. Future
parent observations may reuse this saved key only through that persisted proof,
with matching native parent URL and author. Short captions are supported without
the generic text-fingerprint threshold. Ambiguous saved keys are not selected.
Later direct recapture preserves an already verified relation instead of
accepting a replacement relation supplied by the caller.

Fixture validation covers internal versus Bridge entrypoints, wrong photo IDs,
conflicting query IDs, forged coverage, wrong author/text/media/parent URL,
one-item recapture, subsequent key reuse and retention on direct refresh. These
tests use isolated test stores, not the running user's database. No live data
migration, installation or runtime restart was performed for this backend change.

Scope limit: generic feed observations cannot create a new photo-parent alias
from coverage alone. Reuse currently requires a relation persisted by internal
recapture. First-time feed reconciliation without recapture, full UI-to-store
headless validation and rebuilt package smoke remain open. Browser remains the
default. This step does not claim four-source replacement readiness.

### Pushed backend and rebuilt candidate validation (2026-10-02)

Commit `28e0b37` pushes internal recapture and persisted parent-key reuse to
`origin/main`; remote SHA matched. A new fixture explicitly tests a first
parent-feed encounter with identical long author/text and claimed photo-parent
coverage but no persisted relation. It retains the parent's own evidence key
and reports an identity conflict rather than guessing an alias. Focused photo
recapture/authority/first-encounter store tests pass. This is a fail-closed
boundary proof, not automatic first-feed relationship discovery.

The local installed-app candidate was rebuilt at
`AkuBrowser/build/headless-parent-28e0b37-20261002/AkuBrowser-0.9.0-windows-x64-installed-app`.
Builder result: 414 files, 588900042 bytes; Sidecar SHA-256
`d07ec18175fb651107ec4edde1f3cc76ecd6fbf603879684627608f44eac1ccd`.
The default external c2patool path was absent; the builder accepted the pinned
binary from the prior candidate with its normal provenance checks. No new tool
download or installation was needed. The candidate declares dirty local source
state (including untracked experiments/test work); it is not a clean release.

Packaged Node and Chromium 152.0.7977.54 passed the headless machine smoke on a
new isolated profile: both source targets rendered animation frames and shared
and retained the loopback fixture cookie. Capture, Chrome control, worker and
Facebook extraction/boundary/photo-evidence assets were hash-compared against
current source and were identical. The smoke imports the identical source
Chrome module; it does not claim an authenticated four-source product test.

No installed runtime/profile/configuration was changed. Remaining gates include
authenticated rebuilt-package capture, UI-to-store recapture, broader fresh
source coverage and first-time relationship discovery without saved recapture
proof. The successful package and boundary checks do not close these gates.

### Authenticated rebuilt worker and UI recapture transport (2026-10-02)

Authenticated tests now import capture/source assets from the rebuilt candidate,
not source overrides. The harness uses the registered logged-in Chrome
154.0.8037.93/profile; packaged Chromium 152 was tested separately on fixtures.
The latest Browser update's Facebook run failed `capture_empty` while X, IG and
LinkedIn completed. The most recent completed Facebook reference (16:19:33Z)
remained within the 30-minute window for this diagnostic.

The rebuilt worker returned `empty_unverified` at 16.10- and 18.37-minute gaps.
It reported authenticated UI, completed document, loaded verified photo media,
no login/challenge, but no verified parent binding. Metadata inspection found
two distinct story IDs/post IDs/URLs across `container_story` and
`creation_story`: the container has a 945-character message; creation has no
message and its post ID equals the photo ID. The current consistency rule
correctly declines this case. Do not call this a package parity pass or treat
the two different relation roles as interchangeable identities.

Private receipts: `06e8da41-4f28-419c-bbf2-2fec6d5a8de2` and
`84d99fa3-871f-4fa5-b5a7-d53fcbbf88cd` (packaged capture),
`baebca72-6884-416f-833c-1a543c46874a` (relation diagnosis), under ignored
`build/authenticated-parity-*/report.json`. Runtime restoration passed.

UI inspection exposed a separate integration gap: Recapture still gated on
Bridge and awaited extension messages. It now uses collection readiness and
the job's durable collector stamp. Internal headless/Quiet jobs poll a bounded
read-only `/api/media-recaptures/{id}` projection (ID/status/outcome/error only),
while Bridge jobs retain extension dispatch. Poll requests have individual
timeouts, total waiting is bounded, and foreground offers are suppressed in
headless. No automatic mode switch or foreground action was added.

Four Node tests pass, including execution of the actual UI recapture function
with Bridge unavailable, timeline refresh, unavailable/failure states, mismatch
and timeout handling. Store, engine and HTTP suites pass, including loopback
origin checks for the new endpoint. These are fixture/contract checks, not a
live click-through of the new UI: the UI changes are not in the previously
built candidate or installed runtime yet.

### Distinct photo and containing-post diagnosis (2026-10-02)

The reference item has zero text and reports `already_complete`; it is not
evidence of a truncated caption. The creation story has no verified message
string and its metadata URL returns to `/photo.php`. The container story points
to a native `/posts/` route. Selecting creation as a native post was rejected
before navigation by the diagnostic route guard; these attempts are not
capture failures or parity results.

Read-only headless navigation to the explicit container URL succeeded: exact
native identity, matching author, 942 extracted text characters and four media
items, including the reference image. The reference has one image and no text.
Thus replacing the photo item with the container would change its content scope.
This is a mechanism diagnostic at a 31.41-minute reference gap, outside the
fresh-parity window. It is not a fresh parity pass. Receipt:
`build/authenticated-parity-60ef9c21-5007-4b38-8611-4bfe9afc6bea/report.json`.
Worker exit, profile release, runtime restoration and compatible Bridge health
were verified. No production binding rule was relaxed. A regression fixture
keeps the two identities distinct for both absent and explicitly empty photo
messages, even when their owner matches.

### Photo-only missing-media recovery (2026-10-03 local)

Implemented a recapture-only photo evidence path. For an explicit Facebook photo
URL, round one, owned headless and `recapture_media`/`missing_media`, the worker
can return exactly one verified visible photo without navigating to the parent.
Requested and actual photo IDs, structured metadata ID, owner, trusted image URL
and loaded dimensions must agree. This result does not claim live author/body
evidence or admit a new feed post. The module is included in source provenance.

The internal store completion path corroborates the saved permalink/evidence
key, requested photo ID, returned page/block/proof, headless job ownership and
missing-media reason. It accepts only one image and an originally unavailable
item with no media. It preserves saved caption (including empty), author,
platform ID, permalink, relationships and evidence key. It creates no parent
alias and no new timeline row. Bridge submissions and mixed/mismatching evidence
are rejected. Existing image replacement, playback-error repair and first-feed
photo admission are outside this path.

Authenticated source-worker diagnostic receipt:
`build/authenticated-parity-d51a7d93-2369-4e80-a54c-463b488cdb1a/report.json`.
Result: verified photo proof, one block, one image, matching reference image host
and path, no album caption or parent relation. The reference was 40.83 minutes
old, so this is mechanism evidence, not a fresh parity pass. It used current
source capture/assets with the earlier package's Chrome launcher and registered
authenticated Chrome, not a newly rebuilt package. Runtime stop/restore,
worker exit, profile release and compatible Bridge restoration passed.

Store, engine and HTTP suites pass. Worker/UI Node tests cover actual capture
routing, wrong IDs/redirects/unsafe media, saved-content preservation, single
timeline-item completion, and rejection through the Bridge entrypoint.

### Combined candidate and remaining integration evidence (2026-10-03)

The combined candidate was rebuilt at
`AkuBrowser/build/headless-photo-recapture-20261003/AkuBrowser-0.9.0-windows-x64-installed-app`.
It contains the current worker, store and embedded UI changes (415 files,
588919707 bytes; Sidecar SHA-256
`8c055ae2247ad18b44327b25974bb8fece3f26b11688a92e9778639b51dc7085`).
This is a dirty-source local candidate, not an installed release.

Packaged Node/Chromium passed the isolated machine smoke: rendered frames and
shared/retained loopback cookie. The source Chrome controller used by that smoke
does not establish an authenticated product journey. Authenticated diagnostic
`bd0ddc5a-4f13-4415-9955-a9221cc6f597` imports worker/capture/assets from this new
package and returns one verified photo block without parent content. Its URL
host differs from the reference but its image path matches; reference age was
46.16 minutes. Runtime/profile/Bridge restoration passed. This remains a
mechanism diagnostic, outside fresh parity.

The packaged binary ran on a separate loopback port and fresh fixture database
with no app shell. Its embedded app and recapture transport match source. The
actual UI recapture function polled the real HTTP endpoint, read a persisted
completed job and refreshed timeline once. Foreign origins were rejected and
private payload/result fields excluded. Receipt:
`build/packaged-recapture-ui-smoke-20261003-v3/report.json`. Queue response and
worker completion were fixture inputs: this does not close the full journey.

Current-state correction: re-reading the reference photo block shows three
distinct images with zero text, not a single-image feed baseline. The one-image
photo recovery demonstrates an individual attachment mechanism only. It does
not prove full feed-media parity or complete recovery of a multi-image item.
Historical single-image claims above must not be used to close that gate.

Engine integration fixture now seeds a Facebook photo through normal
claim/observation acceptance, switches ownership to headless, queues a real
recapture and verifies Bridge cannot claim it. Headless claim/acceptance updates
the store, preserves author/caption/platform identity and leaves exactly one
timeline row. Focused test
`TestHeadlessPhotoMediaRecapturePreservesSavedFacebookItem` passed. Its capture
observation and process are synthetic, so it is not live worker/UI proof.
The HTTP regression
`TestMediaRecaptureStatusProjectsDurableStatesWithoutPrivateEvidence` also
passed for all four durable states and an exact four-field response projection.

Fresh baseline attempt `session_554df7a91e6c52d0ba3190309f54b14d` completed
partially: Facebook and X failed `quiet_capture_failed` with Quiet collector
unavailable at dispatch; Instagram/LinkedIn completed. Loopback and compatible
Bridge health remained healthy. No settings/mode change or runtime installation
was made to bypass this baseline failure. This attempt supplies no fresh X/FB
parity pair, and its failure must not be attributed to headless capture.

### Quiet readiness and authenticated recapture chain (2026-10-03)

Inspection identified a false-ready state: coordinator availability checked a
bound backend pointer/generation, while the Quiet worker could already be failed
or retired. `CaptureAvailability` is now an optional nonblocking capability;
Quiet clears its atomic readiness flag before failure/retirement cleanup.
Coordinator status and new-command selection exclude unavailable workers.
The existing Browser selection can then choose its existing Bridge route for
new work; already queued Quiet jobs retain their stamped owner and fail
explicitly. No mode switch, automatic retry or reassignment of queued work was
added. Legacy backends without this optional capability retain their contract.

Collection/Quiet, engine and HTTP suites pass, including the engine regression
`TestUnavailableQuietWorkerDoesNotOwnNewCommandsOrChangePinnedRoute` and photo
recapture integration. A race-detector run was unavailable in the current
CGO/GCC environment; the shared readiness flag is atomic and worker lifecycle
fields remain serialized by its existing operation channel. The rebuilt
candidate above predates this readiness fix; installed runtime is unchanged.

An authenticated integration harness now connects the actual UI recapture
function, real HTTP queue, engine owned-capture loop, packaged headless worker
and database persistence. Receipt:
`build/authenticated-parity-c9f45427-c055-4e39-9215-a7bca5a2cbb3/report.json`;
detail `build/recapture-live-latest.json`. It passed: headless collector stamp,
completed/recovered job, one saved image, preserved evidence key/platform ID/
permalink/author/caption, exactly one timeline row, and one UI timeline refresh.
Worker exit/profile release and healthy compatible Bridge restoration passed.

The harness seeds an unavailable item in an isolated database and uses fixture
permission/initial-owner state. It runs the current compiled source engine/HTTP
with the rebuilt candidate's actual worker and registered authenticated Chrome.
The UI function executes in a VM, so this proves the transport/worker/store chain
but not a rendered button click or full installed-app behavior. The target was
old (802.69 minutes), so this is not fresh parity or multi-image equivalence.
Startup debugging was first isolated on an empty profile; no production trust
or access checks were disabled to make the fixture run.

The readiness fix has now been rebuilt into a separate local candidate at
`AkuBrowser/build/headless-quiet-readiness-20261003/AkuBrowser-0.9.0-windows-x64-installed-app`:
415 files, 588920219 bytes; Sidecar SHA-256
`e982689df15c40b6f4238f19710210c21d3f1153e2f565b1d665f2343db0970c`.
The manifest declares dirty Sidecar inputs. This candidate has not been installed.
Rendered UI validation remains open.

The user authorized one temporary Adaptive Fidelity Update to obtain a fresh
Browser baseline, followed by restoration of Quiet. The scoped helper passed
syntax checking and a read-only dry run against the current Browser/Quiet
settings. It preserves the other settings during restoration and observes the
same Update session without restarting it. Live receipt and source-specific
outcomes must be inspected before this supplies any parity baseline.

Adaptive baseline session `session_b76e8c24557050fb7e47ad25325ca1e2` finished
partial. Quiet was restored and collection mode remained Browser. Instagram,
LinkedIn and X completed; Facebook failed `capture_empty`. Facebook diagnostics
showed two action-anchored candidates rejected for missing required identity;
this is a Browser/Bridge baseline failure, not a headless comparison outcome.
Receipt: `build/adaptive-baseline-5538ba18-1092-4cca-9404-4197ebd78921/report.json`.

The exact-session raw X baseline contains eight blocks from `aku-bridge`,
captured at `2026-10-03T05:55:20.276Z`. Authenticated headless feed diagnostic
`762a8ae8-110b-4083-89f7-a633b02365fa` ran 1.50 minutes later and captured three
native X IDs with no baseline overlap. The diagnostic completed and runtime,
profile release and healthy compatible Bridge restoration passed. Different
feed results provide no matched content/media parity evidence; the next X
check must capture the baseline URLs directly. Detail:
`build/fresh-x-pilot-20261003.json`. Facebook has no fresh usable baseline from
this session.

Same-URL X diagnostic `e392df65-f3b1-4477-96b9-a766f04b096a` captured two
distinct targets from that raw Browser session 3.96 minutes after capture.
Both native IDs were observed once. Media counts matched (one image and zero
media respectively), including the image host/path. Raw author/text equality
was not universal: the image post's text differed only by the observed link's
`twclid` query parameter, and the other Browser author included a relative-time
suffix absent from headless. Preserve those raw mismatches; this bounded check
does not close feed, video playback, pagination, or full-platform parity.
Runtime/profile/healthy compatible Bridge restoration passed. Detail:
`build/fresh-x-exact-20261003.json`.

Field-difference diagnostic `build/fresh-x-differences-20261003.json` confirms
equal prose with URL tokens separated, matching link origin/path with only
`twclid` present in the Browser query, and matching authors after separating the
observed relative-time label. The raw strings remain preserved. These two cases
support bounded equivalence under the approved harmless-formatting rule; they
do not prove URL navigation behavior or close broader X source acceptance.

Current authenticated Facebook feed diagnostics were also run against the
readiness candidate's actual worker. First receipt
`2a843c03-f8ab-42e7-b3be-34d203dea5d0` returned `empty_unverified`. The helper
was corrected to retain error diagnostics, then receipt
`112b7ba5-79bd-49b5-9730-09f5758e0796` captured one native item containing five
images. No production extractor change occurred between these runs; the new
success cannot be attributed to a collector fix. Quality remains `unverified`
and text expansion was skipped under `detect_only`. Both runs restored the
runtime/profile and healthy compatible Bridge. This demonstrates a current
multi-image acquisition, but with no usable paired Browser item it does not
close Facebook completeness, stability or parity. Detail:
`build/facebook-current-feed-20261003.json`.

Rendered Recapture harness diagnosis remains scoped to disposable UI/capture
profiles. Its sandboxed run failed Windows profile-owner inspection before
capture Chrome launch; running outside the sandbox preserved the ownership
check and initialized successfully. A separate cascading missing startup
metadata error is now rejected explicitly by the harness. No production trust
or ownership guard was relaxed.

The escalated isolated run rendered a visible enabled Recapture button on a
separate packaged UI Chrome profile. The subsequent diagnostic hit-tested that
button but observed no document click event and no recapture HTTP request; the
application handler was not entered. This currently points to headless UI
input/focus dispatch, not a queue/backend rejection. The local harness requires
a focused verification after its input correction. These receipts do not yet
close the rendered product interaction gate.

The focused UI-headless input correction (page focus and explicit mouse button
state) still produced no DOM click or HTTP request. This result leaves the
rendered interaction unverified; it is not a failed backend capture. The next
bounded check uses the actual served UI in Codex's interactive in-app browser
while collection remains headless. Auto-review rejected opening that visible
fixture tab because the existing foreground authorization covered only one
Adaptive Update. Explicit permission for the visible fixture/Recapture action
has been requested. No alternate UI action may bypass that rejection.

The user then explicitly authorized the visible in-app fixture and Recapture
click. The first fixture expired while approval was pending and restored the
runtime without a recapture job. The following run queued and recovered the
photo, but the helper closed immediately after durable completion and raced
the UI's final status read. This was corrected in the local helper by retaining
the fixture briefly for operator UI evidence; no production code changed.

Authenticated receipt `83ca4fe1-fe46-4f57-a635-ccff96daac21` and fixture directory
`build/recapture-iab-a8a12251-ea8d-4c3d-8739-a545f5b7a042` now prove the rendered
single-photo journey: the observed Recapture button was clicked through Codex's
in-app browser, the UI showed its in-progress state, an actual headless job
completed/recovered, and the UI refreshed to display the recovery notice and
loaded image. `ui-evidence.json` records one Timeline card and no remaining
Recapture button; `ui-recovered.jpg` captures the completed UI. Backend evidence
preserves the evidence key, platform ID, permalink, author and caption, with one
image and exactly one Timeline row. UI evidence is a separate operator artifact
in that same fixture directory; do not require it to be embedded in the worker's
already-written report.

The worker exited, the authenticated profile was released, and runtime/healthy
compatible Bridge restoration passed. This uses a seeded unavailable item and
fixture permission/initial-owner state, current compiled engine/HTTP UI plus
the candidate's actual worker and registered authenticated Chrome. Its source
reference was 853.27 minutes old. It closes the served rendered Recapture
transport/worker/persistence gap for one photo, not fresh parity, full album
recovery, installed-app handoff, or four-source/default acceptance.

Next: acquire a second fresh X/Facebook window and a working Facebook Browser
baseline for media parity,
including multi-image items. The installed runtime has not received these
changes. Full parity remains open.

### Second fresh acquisition window and X avatar regression (2026-10-03)

The user authorized exactly one additional Adaptive Fidelity Update and Quiet
restoration. Session `session_b3dc395ed94c44ba41ef8bd4e1d83f8f` completed all
active sources. The exact-session Browser/Bridge export contains eight X blocks
and one Facebook block; it does not reuse failed or older source observations.
The helper verified restoration to Browser/Quiet.

Authenticated diagnostic receipt `1adf4aef-5ebe-4e23-9ad3-4f7d52e097d7` compares
the actual new baseline URLs using the packaged worker and registered Chrome
154/profile. Detailed results are private under
`build/fresh-window2-exact-20261003.json`. Its outer harness uses an older
preflight fixture and intentionally reports `fresh_window2_complete`; those
outer fixture IDs/counts are not the comparison targets or failed captures.

- Facebook: one exact native ID, author, text and image host/path match within
  4.67 minutes. This is usable fresh single-image evidence, not full Facebook
  acceptance; only one of the two Browser windows produced a usable FB baseline.
- X: two exact native IDs match within 4.01 minutes. Text is equal and author
  differences are relative-time labels. One native video has the same poster and
  MP4 host/path as Browser, but headless additionally emits the author's 48x48
  `pbs.twimg.com/profile_images/` avatar as `video_poster`. Record that extra
  media attribution as a headless regression, not harmless count variance.

The X-only headless extractor now excludes that CDN profile-image namespace
before classifying attachments. It retains author/avatar metadata, native
video posters and real images, with no Bridge production collector change.
The new rendered-extractor VM regression test passes; all 52 worker tests pass.
This correction is newer than the packaged readiness candidate and does not
update the installed runtime. Live source verification and package refresh
remain separate evidence requirements.

The first source retry exited before init because the temporary helper resolved
its source root against the candidate working directory. Receipt
`62ebdc20-3dfa-4299-860b-ac0eb561afb0` verifies healthy runtime/profile/Bridge
restoration. The helper was corrected to an explicit source root; this is an
environment harness failure and supplies no source-quality result.

After that path correction, receipt `1b9668fe-e17b-45f6-be4b-2e86352478b7`
verifies both same X URLs with the current source worker and packaged Bridge
assets/registered authenticated Chrome. The actual gap is 7.53 minutes; both
native IDs and texts match, relative-author labels are the only author variance,
and media counts are now 1/1 and 0/0 with matching host/paths. The unwanted
avatar is absent. The video retains the same poster and MP4 as Browser. This
is live source correction evidence, not a rebuilt-package or player proof.
Detailed before/after results remain separately preserved in ignored build
artifacts. Worker shutdown, profile release and healthy compatible Bridge
restoration passed; final settings independently read Browser/Quiet.

### Rebuilt X filter candidate checkpoint (2026-10-03)

Canonical `scripts/build-windows-installed-app.ps1 -AllowDirty` built
`AkuBrowser/build/headless-x-avatar-fix-20261003/AkuBrowser-0.9.0-windows-x64-installed-app`.
The default c2patool workspace location was absent, so the builder reused the
previous local candidate's binary through its explicit `-C2paToolPath` option.
The official release-pin SHA-256 and version checks remained enabled and passed;
no tool was downloaded or installed. Build manifest, staged config/schema probe,
Chrome pin and payload checks passed: 415 files, 588920533 bytes, Chrome for
Testing 152.0.7977.54. Sidecar remains
`e982689df15c40b6f4238f19710210c21d3f1153e2f565b1d665f2343db0970c`.

The packaged and source `vendor/x-extract.js` SHA-256 both equal
`4412d41c1110cf4a3a1e3bd7dd9a712ef4bed8a81d6e7f342b16e7e43cf0f354`.
This packages the live-verified source correction, but is not new packaged
authenticated capture or player evidence. The manifest records Sidecar as dirty
at HEAD `28e0b37`; this is a local candidate, not a release. Existing runtime,
configuration and default mode were not changed. No commit/push/install occurred.

The user requested a cumulative 2-million-token ceiling. The original native
segment reached `budgetLimited` at 1,000,427 tokens; no tool could modify that
active cap. After the user continued, native readback returned no goal. A new
segment was therefore configured for the remaining 999,573 tokens, retaining
the same full objective and the prior checkpoint as a cumulative baseline.
New-segment counts exclude earlier work; aggregate worker coverage remains
partial. No counter reset, old-goal completion or cap modification was claimed.

Authenticated receipt `bad58f9e-0390-4fae-a87c-76d5e847cfe1` then ran the rebuilt
candidate's actual worker on the same second-window URLs. The private detailed
report `build/fresh-window2-packaged-20261003.json` proves the Facebook native
ID/author/text/single image within 15.34 minutes and two X native IDs/text/media
within 14.68 minutes. The corrected X video produces exactly one media item
with matching poster/MP4; the unrelated avatar is absent. Relative-time author
labels remain harmless recorded variance. Profile release and healthy runtime/
compatible Bridge restoration passed, with independent Browser/Quiet readback.
This closes the updated packaged extraction check for these three examples;
it does not prove player, multi-image, installed handoff or full source parity.

### Additional Facebook windows: photo media is separate from post parity

The user authorized at most two more Adaptive Updates, restoring Quiet after
each. Both completed and restoration passed:
`session_093bdd2988c8f1ed52ae707f2a44afa2` and
`session_ea0dc377f15444addcfb8090a2e67a2e`. Exact exports preserve raw Bridge
observations separately. Each has one Facebook feed item with the same identity,
one image, empty caption and a photo permalink; no multi-image or video case was
observed. Observation page routes are the feed, so this is not evidence that
the Browser acquisition ran on the photo detail page. These windows are not
additional independent content-type coverage.

Receipt `7a51fd88-008d-48f6-8f2a-7fa2bf1a16b0` attempts the first window's exact
photo URL within 2.06 minutes using ordinary post capture. It reports
`empty_unverified`: authenticated UI is present, no login/challenge is detected,
and photo media is verified, but post binding is unverified. Do not admit that
photo as a parent post or invent author/caption to make the comparison pass.
The Browser feed block and a photo-detail extraction have different evidence
scopes; this result alone cannot establish a feed acquisition regression.

Receipt `68cb014a-21f2-407a-8c55-aafdd6397b95` separates those scopes against the
latest baseline within 3.59 minutes. Its headless feed captures two native IDs
and one image, with no overlap with the single Browser baseline item. Feed
personalization remains an unverified comparison, not a matching-post failure.
The explicit exact-photo missing-media capture returns one image with the same
host/path as the Browser reference, with no invented author or caption. This
proves fresh photo media availability for the bounded recovery path, not general
post admission, album completeness, or Timeline persistence in this diagnostic.
Worker exit, profile release and healthy compatible Bridge restoration pass.

The two-Update foreground allowance is now exhausted. A targeted read-only
foreground sample-selection request (at most ten Facebook posts, then Browser/
headless comparison and Quiet restoration) was approved by the user's
"ya silahkan lanjutkan". The bounded discovery found the public Physics Girl
video described below. This does not extend the exhausted two-Update allowance.
Do not keep repeating unrelated feed Updates or claim missing content types
as passed.

A read-only Luna scout also reconciled the integrated handoff evidence:
`TestCollectionSettingsRenderedFixture` proves Settings/UI/API persistence with
fake capture processes; `TestBridgeHeadlessHandoffWindowsSmoke` proves real
Chrome/Bridge HWND lifetime and post-close auto-return, but uses a synthetic
reader page, direct coordinator mode request and fixture permissions. Neither
proves the combined actual source-authenticated Settings/reader/login journey.
The next integrated proof must use the normal Settings and reader action paths,
retain the same profile authentication, and verify a subsequent headless source
check. The canonical Windows smoke can be extended, but its isolated foreground
runtime swap is separate authorization from the completed Adaptive Updates.

### Settings boundary prepared for the Windows handoff smoke

`TestBridgeHeadlessHandoffWindowsSmoke` now requests Headless through the real
`PUT /api/settings` endpoint instead of calling the coordinator's request method
directly. It preserves the existing visibility, selects the two supported
sources, and requires the HTTP response to expose persisted Headless plus
effective Browser/pending before the explicit initial replacement. Additional
assertions require the persisted/requested selection to remain Headless while
the actual native-window guard holds Browser, and require settings to stay
unchanged after auto-return. Existing retirement ACK/ownership assertions remain.

The opt-in test compiles with the live environment variables absent. This
invocation supplies no live Chrome proof: the test is gated, so do not report
the new Settings integration as passed Windows acceptance yet. The reader
page, permission admission override and initial replacement remain fixture
boundaries; the real source-authenticated rendered Settings/reader journey is
still required. No installed executable or runtime changed for this test-only
preparation. Run the extended live smoke only with appropriate foreground and
runtime-swap authorization after the active source-validation work is idle.

### Authenticated Watch video renders, but post extraction remains unverified

The approved foreground discovery inspected fewer than ten feed posts and
selected the visible Physics Girl video labelled "2 days ago". Clicking that
post's timestamp exposed the native URL
`https://www.facebook.com/watch/?v=1639349817600268`. The primary Chrome tab is
URL-discovery evidence only, not the managed Browser/Bridge baseline. Its
visible playback, author and caption are recorded in ignored
`build/fb-video-discovery-20261003/discovery.json` and `foreground.jpg`.

The current packaged worker was then run against that exact URL with the
registered authenticated collection profile. Receipt
`authenticated-parity-15bb7e3c-00b0-4ba0-bcd4-6f06fa39879f` reports
`empty_unverified`, zero structural/eligible candidates, authenticated UI and
no login requirement or challenge. A second bounded diagnostic added the
missing DOM evidence; receipt
`authenticated-parity-a76e3a69-4b5f-4b78-8142-31bec7504d10` confirms the same
target rendered its Physics Girl caption and a playing 720x720 video:
`readyState=4`, `currentTime=24.263753`, duration `232.633333` seconds.
The video element has no DOM poster URL. Both receipts confirm explicit worker
shutdown, profile release and restoration of the healthy compatible Bridge.
Browser/Quiet selection was preserved.

This target therefore does not demonstrate a foreground-only rendering
restriction. The measured gap is Watch-page post discovery/admission: the
shared adapter discovers feed post containers, whereas this Watch page has
no eligible feed post container. Its comment articles and recommended videos
must not be promoted to the requested post. The DOM diagnostic is private
test evidence and does not admit an item or prove a usable media URL.

Before expanding headless behavior, verify whether legacy Bridge can collect
the same Watch target and keep Watch detail support distinct from feed parity.
Any headless-specific fallback must require exact target identity, corroborated
owner and isolated caption/video evidence; it must fail closed on comments,
recommendations, conflicting identity and unresolved playback URLs. This
sample does not close Facebook video parity, multi-image completeness or the
installed Settings/reader/auto-return gate. No shared Bridge collector was
changed by this diagnostic.

### Headless collection pauses playback while retaining media evidence

The user clarified that collection needs media URLs, not sustained playback.
After approval to continue, the owned headless Chrome path now installs an
early document/frame guard for native `play` and `playing` events. The guard
immediately pauses HTML media elements, including repeated site playback
attempts. It does not replace `play()`, strip media sources, block metadata,
change persistent browser settings, or affect borrowed Quiet/interactive
contexts. Ordinary Chrome autoplay flags alone do not cover muted autoplay.

`playback-policy.test.mjs` verifies repeated attempts, retained source and
metadata, and unchanged native playback methods. Its opt-in real Chrome test
uses generated local media and an isolated profile; muted video autoplay and
script retries remain paused with loaded metadata through two navigations.
Both tests pass with packaged Chrome 152. The complete worker suite passes
53 tests and skips only this opt-in real Chrome case, which passed separately.

Candidate `AkuBrowser/build/headless-paused-media-20261003/` was rebuilt with
the current worker; packaging checks pass (415 payload files). Its authenticated
same-target Facebook diagnostic is receipt
`authenticated-parity-98d2498b-d30b-4dbb-87e6-9b181979afca`, using the registered
Chrome/profile rather than downgrading the live profile to bundled Chrome.
`build/fb-video-discovery-20261003/headless-paused.json` records the Physics Girl
video at `currentTime=0`, `paused=true`, `readyState=4`, 720x720 and duration
232.633333 seconds after the full hydration wait. Caption and author still
render. The recommended video is also paused. Worker exit, profile release and
healthy compatible Bridge restoration pass; Browser/Quiet is retained.

Watch post discovery remains `empty_unverified` with zero candidates. This
change closes sustained incidental playback in the measured target, not Watch
post admission, extraction of a playable media URL, or video parity. No package
was installed and no shared Bridge collector was changed. The next media
validation must keep exact video/post ownership and resolve an admissible URL
without depending on playback; retain the same-target legacy baseline gate.

### Progress checkpoint for the approved commit/push (2026-10-03)

The user authorized committing and pushing this validated checkpoint to
`abangkis/AkuSidecar`, branch `main`. Scope includes exact photo-only missing
media recovery with retained saved identity/content, durable Recapture UI
transport and bounded status endpoint, Quiet backend availability/routing,
X avatar exclusion, headless playback pause policy, the prepared Settings API
handoff assertions, and this evidence ledger. Untracked `experiments/` and
ignored build/private authenticated evidence are excluded.

Pre-commit Go tests pass for `internal/store`, `internal/engine`,
`internal/httpapi`, `internal/collection` and `internal/collection/quiet`.
The four Recapture transport/UI tests pass. Current worker evidence remains
53 passed plus one gated Chrome test, separately passed in actual packaged
Chrome; the rebuilt candidate and authenticated paused Facebook diagnostic
are recorded above. The opt-in Windows handoff remains gated, not a newly
passed live acceptance test. `git diff --check` passes.

Acceptance status and execution order:

1. Photo-only Recapture's actual UI-to-headless-to-isolated-store path has
   passed; retain single-image/exact-photo scope and saved item identity.
2. X has bounded fresh same-URL text/image/video evidence, including the
   packaged avatar fix. Broader collection stability, continuation and mixed
   media coverage remain open, including live checks after playback policy.
3. Facebook has bounded fresh native-post single-image evidence and verified
   photo media recovery. Watch renders while paused, but Watch discovery and
   a usable exact-owned video URL are not proved. Obtain a same-profile legacy
   Browser/Bridge observation of the exact sampled target before deciding
   whether Watch support is a parity fix or additional surface support.
4. Validate Facebook video URL acquisition without sustained playback and
   fresh multi-image completeness, then repeat aligned X/Facebook collection
   windows. Separate shared adapter gaps from headless regressions; do not
   modify the shared production collector just to make a PoC pass.
5. Close the integrated rendered Settings, authenticated reader/login,
   interactive-window close, auto-return and subsequent headless collection
   journey. Current fixture/smoke proofs cover separate boundaries.
6. Begin IG/LinkedIn only after X/Facebook qualify. Browser remains default
   until all four sources pass. Commit/push does not install this candidate
   or change the default/runtime on the user's machine.

The cumulative task ceiling is 2,000,000 tokens. Last native readback remains
195,849 for the continuation segment plus the prior 1,000,427 checkpoint
(1,196,276 cumulative, partial coverage). Its status is still `blocked` after
the user resumed; the available tool cannot reactivate/reset that counter, so
this is a stale measurement, not complete accounting of subsequent work.

### Legacy Watch target contract checked before a live baseline attempt

The same-profile Browser baseline preflight passed after checkpoint `b06a512`:
healthy runtime/compatible Bridge, one registered Chrome profile owner,
Browser/Quiet and no active capture. Further inspection found that the unchanged
production Bridge's Facebook `nativePostPath` in `source-catalog.js` excludes
`/watch/`. Executing its exported `isNativePostUrl` on the exact observed URL
returns false. `service-worker.js` uses this check in `assertRecaptureTarget`,
including targeted `collect_visible` commands. The private proof is
`build/fb-video-discovery-20261003/bridge-target-contract.json` and records the
source-catalog SHA256. This is native-target contract evidence, not a live
Browser observation or full parity verdict.

The proposed foreground fixture was therefore stopped before launch; no
permissions, production DB or shared Bridge code changed. The unused new
wrapper was removed. Do not force readiness, silently enable Watch URLs in
Bridge, or substitute a guessed native URL merely to produce a baseline.
This target is not eligible for comparison through the current native-target
route. The video-URL acquisition investigation can continue read-only using
exact target metadata with playback paused; a supported permalink must be
observed/corroborated before it becomes a legacy comparison target. Feed-video
capture parity and Watch-detail support remain separate outstanding evidence.

### Exact-target delivery URLs discovered with playback paused

Two bounded metadata diagnostics used the current packaged playback guard and
the registered authenticated profile. Receipts
`authenticated-parity-add4a610-60cf-48c5-8fbe-6f882cee0682` and
`authenticated-parity-931956c6-1237-4225-b8e4-fd22faf84b3d` confirm profile
release and healthy compatible Bridge restoration. The target video remained
paused at zero with `readyState=4` and the same 720x720 dimensions/duration.

The read-only JSON walk was bounded to 8 MiB, 80,000 nodes and depth 32. Exact
ID `1639349817600268` records include typed `Video` fragments, a consistent
Physics Girl owner ID, and modern `videoDeliveryResponseResult` /
`progressive_urls` delivery branches. These branches expose two distinct
trusted HTTPS `fbcdn.net` progressive MP4 URL candidates. Signed URLs remain
only in ignored private evidence; no playback or media download was used to
validate them, and no production item was admitted.

The same exact-ID typed metadata exposes the native URL/permalink
`https://www.facebook.com/reel/1639349817600268/`. This is observed evidence,
not an invented Watch alias. The unchanged Bridge's exported `isNativePostUrl`
accepts this Reel form. Private evidence and scope are recorded in
`build/fb-video-discovery-20261003/target-delivery-metadata.json` and
`target-delivery-summary.json`. The next same-video comparison should use this
observed supported URL on both sides. Its rendering, Bridge admission,
headless admission and reusable URL quality are still unverified; the old
Watch failure is not a parity verdict for the Reel route.

### Observed Reel permalink renders but item discovery remains unverified

Receipt `authenticated-parity-94cf88bb-0921-4641-97d7-3ef6c202b3b2`
tested the observed Reel permalink using the same registered authenticated
profile and packaged playback guard. The page retained the exact video ID,
Physics Girl author and matching caption. The target video was visible,
720x720, metadata-ready, paused at zero. A second suggested video was also
present, so any future media admission must bind the exact target rather than
selecting the first available media URL.

Normal capture returned `empty_unverified`: zero structural and eligible
candidates, with no login requirement or challenge. This identifies a Reel
item-discovery boundary; it does not establish a content-rendering restriction
or Browser/Bridge parity failure. Accepting the URL in the legacy target
contract is not proof that its collector admits the item. Private diagnostic:
`build/fb-video-discovery-20261003/reel-target-delivery-metadata.json`.

The worker shut down explicitly, released the profile, and the original
runtime restored healthy with one profile owner and compatible healthy Bridge.
Next: obtain a normal legacy observation on this supported permalink, then
decide whether a bounded headless discovery adapter is needed for parity or
additional detail-surface support. Preserve exact video/owner/poster binding,
including conflicting suggested-video evidence, before promoting delivery
URLs. Fresh feed-video and multi-image parity remain open.

Budget readback for this checkpoint: active native segment 355,287/803,724
tokens, plus 1,196,276 measured before this segment, for 1,551,563 cumulative
measured tokens against the approved 2,000,000 ceiling. Coverage remains
partial; this does not erase unmeasured activity or change the saved default.

### Resolver-only limit comparison on the same paused Reel

Receipts `authenticated-parity-166470d5-14e4-45e4-9f93-d0e0a21df6fa`
and `authenticated-parity-98566158-40be-4243-81b5-1000e34a2462`
tested the unchanged packaged Facebook resolver on the exact target ID after
ordinary item discovery returned zero. This explicit diagnostic request is
not an admitted post or a full live Browser/Bridge baseline.

Both headless limits (12 scripts, 131072 per script) and the resolver's Bridge
default limits (32 scripts, 256000 per script) inspected zero matching scripts
and returned zero candidates. The exact-ID script at index 11 was 257635
characters, exceeding both per-script caps; further exact-ID scripts appeared
after the first 32. Script lengths are JavaScript string lengths, not measured
UTF-8 byte counts. Thus raising only headless limits to current Bridge defaults
would not solve this observed sample. This is evidence of a bounded structured
data selection gap in addition to the separate item-discovery gap.

The first limits diagnostic failed in its own script-envelope selector, was
corrected, and rerun once. All three receipts verify healthy restoration and
compatible Bridge. Existing Bridge Facebook adapter/resolver tests pass 15/15.
Private sanitized evidence: `build/fb-video-discovery-20261003/reel-resolver-limits-summary.json`.
No production limits, source permissions or collector admission changed.
Next investigation should reuse these measured envelopes, preserving exact
ID/owner/poster and suggested-video isolation, rather than broadly increasing
limits or mistaking this diagnostic for feed parity.

### Updated Settings API and Windows handoff smoke passed

The staged paused-media candidate passed the actual opt-in
`TestBridgeHeadlessHandoffWindowsSmoke` in 18.45 seconds through the canonical
stop/test/restore wrapper. Receipt:
`build/bridge-handoff-09a1b592-e4b2-4c0e-bb83-6c8dd53cffcc/receipt.json`.
The wrapper verifies the original runtime restored and the test passed.

The test now exercised the real Settings PUT boundary: headless persisted,
visibility preserved, requested/effective pending state exposed. Actual Bridge
bootstrap/close ACK and natural process drain passed. An open disposable
interactive HWND retained Browser generation 3, then closing that window
allowed headless generation 4 without changing the saved selection. No extra
blank fixture tabs were observed.

This uses a disposable profile and a static local reader with fixture-only
source permission setup. It is not authenticated Facebook/X reader/login
acceptance, rendered Settings UI proof, or subsequent social collection proof.
Those integrated user-journey gates remain open; do not broaden this result to
claim them complete. No install, production settings or default changed.

### Packaged X target regression after playback guard passed

Receipt `authenticated-parity-535123c3-6d7d-45d6-b3bf-adb70e239b38`
used the packaged worker, registered authenticated profile and the previously
recorded target `x:status:2105265191986598158`. Capture succeeded, matched one
copy of the requested ID and returned its own video with a playback URL. A
separate reply was captured with zero media, keeping those IDs distinct.
The observation retains `headless_dom_observation` / `no_production_admission`
limitations. This is a current same-target regression result, not a fresh
aligned feed window or full parity claim based on the old saved baseline.
The original runtime restored healthy with one profile owner and compatible
Bridge. X video URL extraction still works with the packaged pause policy;
broader fresh collection and production journey gates remain open.

### Native target baseline fixture stopped at the production dispatch boundary

A bounded read-only fixture investigation made no helper or test changes and
launched nothing. The normal `/api/updates` request accepts an intent and builds
a feed catch-up command; it does not expose an arbitrary target URL. The target
route is Timeline media Recapture, where the store derives the permalink from
real saved evidence and enforces the media-recovery reason. Its Bridge dispatch
comes from the product UI. A source-catalog accepted URL alone cannot substitute
for that evidence, actual source permissions or registered readiness.

Do not seed synthetic Timeline evidence or inject extension/UI messages to
manufacture this parity baseline. Use real Browser observations and supported
product actions. The user approved one additional Adaptive Fidelity Update
after the previous bounded Update permissions were exhausted; the existing
helper passed its read-only preflight before this approval. This single Update
uses current active sources and must restore Quiet. Qualification analysis
remains limited to X/Facebook; no new IG/LinkedIn qualification is implied.

### Additional fresh acquisition window and bounded headless comparisons

The one approved Update completed as
`session_629befadde319e395243b5213881389c`, restored Quiet, and retained Browser.
Its exact raw observations were exported without fallback: X 9 unique blocks
(including one video), Facebook 1 photo block with 4000 characters of caption
and one image. Both source runs completed through `aku-bridge`.

Receipt `authenticated-parity-c5ccd614-4d54-41c2-a929-96d34ac807ff`
then captured fresh selected URLs through the packaged headless worker.
The X video `2105897298610037233` matched ID, normalized text (145 characters)
and media host/path sets. Author headers differ only by the separately measured
feed timestamp suffix; exact author strings are not equal. Facebook photo
`1101023269520351` returned verified photo-media evidence but unverified parent
post binding, so ordinary item capture remained `empty_unverified`. This does
not establish full item parity or a content-unavailable error.

Receipt `authenticated-parity-41561fc2-e3e6-49f8-9e60-b8dae3f3f362`
captured Home Feed with one bounded scroll: one distinct post, three distinct
images repeated in two snapshots. Its post differed from the Browser baseline,
so album completeness remains unpaired. Detect-only policy skipped caption
expansion. Receipt `authenticated-parity-4c13ec1b-5cfa-4458-959d-ae4b5b6d994d`
used normal content-expansion policy on a newly loaded feed and returned zero
blocks after candidate rejection for missing identity. This changed feed cannot
prove expansion caused the failure.

A subsequent instrumented normal-policy feed capture,
`authenticated-parity-e1ad0c5d-2486-4799-a861-1479bd400c05`, admitted video
`1868485030846148`, expanded its caption to 748 characters, and observed its
loaded poster. Stream URL resolution remained unresolved: zero traversed nodes
and zero resolver candidates. It is genuine headless feed-video discovery, not
usable video URL proof or legacy parity. All completed receipts verify healthy
runtime/Bridge restoration. One earlier feed attempt was refused by inbox
preflight before interruption; it was retried only after active-session state
was verified clear. No additional foreground Update was started.

Private evidence lives under `build/baseline-session_629befadde319e395243b5213881389c/`.
The outer stop/test/restore wrapper still carries its old preflight fixture;
the diagnostic reports preserve the actual new session, target IDs and times.
Never relabel the wrapper's old target summary as this fresh comparison.

The operator fixture investigation corrected its initial dispatch conclusion:
a test run can use real Store.StartRun and normal Bridge background polling.
The product UI ping/heartbeat/configure path provides real source readiness and
permissions; no arbitrary-target public API or synthetic Timeline row is
required for an isolated operator test. Prepare that bounded fixture without
injected UI messages or fabricated permissions, then validate it before launch.

### New accounting segment explicitly authorized

After the old native budget ended, the user explicitly requested accounting
from zero with a new 1,000,000-token cap. The host then reported no existing
goal; creation succeeded with active status, tokenBudget 1000000 and tokensUsed
0 at creation. This is a new accounting segment, not a reset of implementation
or evidence. Prior measured usage (1,196,276 plus 809,438 = 2,005,714, partial)
remains historical and is not deducted from the new segment. Saved budget
defaults were not changed. Delegate coverage remains unverified.

The first resumed resolver diagnostic was refused by preflight because actual
session `session_7b4fb1c4c6940b2fde24ed794d5196d9` was running. It launched no
worker and did not stop the runtime. Do not cancel or restart that session to
make room for QA; revalidate its terminal state before profile handoff.

### Shared headless/hidden-Quiet worker script selection correction

The resumed exact-ID video diagnostic found a small matching script at index
35. Current headless selection inspects only the first 12 scripts. A diagnostic
48-script envelope returned the exact video ID and a playback URL, while its
poster did not match visible DOM, so no item admission is justified by that
result. Correct only the headless request's script-count cap to 48, retaining
131072 per-script, 524288 aggregate, 6000-node and depth-24 bounds. Preserve
exact candidate ID and DOM poster matching; no shared Bridge collector change
or poster fallback is included. This module is also used by the borrowed
hidden-Quiet backend; the change is not exclusive to owned headless Chrome.
The legacy AkuBridge collector remains unchanged. Validate delayed valid evidence, foreign IDs,
and evidence beyond the count cap before accepting this change.

Validation: structured-media tests passed 10/10; the worker suite passed 54
tests with one opt-in real-Chrome playback test skipped (55 total, zero failures).
No package rebuild or authenticated 48-script-only admission proof is claimed.

The diagnostic receipt `authenticated-parity-d72bfd26-f439-4def-97a5-3108805fc106`
used the actual discovered feed video `1868485030846148` on its Watch route.
The original 12-script envelope traversed zero nodes. The broader diagnostic
envelope returned one exact-ID playback candidate, but its poster did not match
the visible DOM poster. It also raised byte/node/depth bounds, so this is not
proof that the production count-only correction resolves this target. Playback
remained paused at time zero; runtime and real Bridge readiness were restored.
Keep the mismatching candidate unadmitted until ownership evidence is adequate.

Session `session_7b4fb1c4c6940b2fde24ed794d5196d9` subsequently ended partial,
with both X and Facebook failing `quiet_capture_failed`; it is not a usable
Browser baseline. No Update was initiated to replace it.

Review of the new native Reel fixture found that Quiet plus
`foregroundAuthorized=false` cannot establish a foreground baseline. Correct
the fixture to an explicitly authorized Adaptive capture, retain private raw
observations for paired comparison, and keep production DB/settings untouched.
Compile and filesystem dry-run evidence do not authorize its live launch.

Readonly failure inspection identified the dev-runtime prerequisite gap:
`runtime/dev/headless-worker/node.pin.json` is absent. The subsequent generic
Quiet-unavailable errors come from a retired worker and do not establish a
social-content failure. The dev builder currently stages the reader broker but
omits the adjacent worker required by the executable. Add the existing official
pin-checked AkuBrowser worker staging helper to the dev build. Validate staging
in an isolated build directory first; do not replace the live runtime or restart
it as part of this build-path correction.

The isolated staging run passed at
`AkuBrowser/build/dev-worker-prerequisite-20261003/headless-worker` with 16
source files and official pinned Node 24.16.0; the helper verified distribution,
binary and license hashes. PowerShell parsing of the modified dev builder and
`git diff --check` passed. The live `runtime/dev` directory was not staged or
rebuilt, and no registration, settings or runtime process was changed.

Candidate dev builds stage `headless-worker.next`, preserving the active
worker directory. The restart helper checks the candidate prerequisite before
interruption, promotes worker assets after stop, restores the previous directory
if that move fails, and retains the old directory for diagnosis/rollback.
Its PowerShell syntax passed; live restart/promotion is still unverified.
`scripts/test-dev-worker-promotion.ps1` also exercised the actual production
promotion block in isolated directories: successful promotion retained old
assets, and an intentionally missing candidate restored the old directory.
No Supervisor, browser, binary or profile was involved in this check.

The corrected native-target fixture now uses the unchanged Bridge policy's
foreground path: `recapture_media`, `adaptive_fidelity`, and explicit foreground
authorization. It retains raw observations privately and a sanitized summary,
waits up to 90 seconds for genuine source readiness, and requires both runtime
stop and foreground opt-ins. Compile, syntax and filesystem dry-run passed;
root reviewed the revised policy/receipt path. One live Recapture approval was
requested separately; no run is authorized until that answer arrives.

Built candidate `AkuBrowser/build/headless-script-envelope-20261003/` from
Sidecar 9aafed4 with the current dirty source, Browser 036d839, Bridge 8dd4d6d.
Build passed (415 files, 588921372 bytes), retaining Sidecar executable hash
e982689df15c40b6f4238f19710210c21d3f1153e2f565b1d665f2343db0970c while staging
the revised external worker sources. This is an uninstalled candidate; bundled
Chrome 152 must never open the registered authenticated Chrome 154 profile.
The installed-app builder verifier passed for the same candidate tuple and
confirmed the 415-file payload, production Bridge identity and Chrome version.
The packaged structured-media source was inspected and contains the intended
48/131072/524288/6000/depth-24 envelope.

Prepared `build/facebook-feed-video-production-envelope-worker.mjs` to derive
the request directly from that packaged module, reject unexpected bounds, and
omit the broad metadata traversal and extended-limit probe. Its syntax passed.
The authorized headless stop/test/restore probe is now running against this
package; report its receipt and restoration outcome before making any result
claim. Its target-only diagnostic scope is not fresh feed parity.

That run completed as `authenticated-parity-ae38b6ac-5bc5-42f4-baa9-90321c5ebbd3`:
the exact production envelope inspected one 38552-character script, traversed
142 nodes and returned one exact-ID video with playback URL. Thus the count-only
correction reaches this evidence without larger byte/node/depth limits. The
poster still differs from both visible video posters; candidate admission stays
blocked. Both loaded videos remained paused at time zero. Normal Watch-target
capture failed discovery, so this does not establish a captured/admitted post.
The wrapper completed and recorded restored=true, healthy=true, compatible
healthy Bridge with registered Chrome/profile ownership verified.

Preparation for the paired Reel baseline found a comparison-only route gap:
the shared headless URL canonicalizer supports `/reel/<id>/`, but the parity
helper's native identity parser omits it. Extend that parser and its tests to
the same exact native ID contract. Retain explicit media-Recapture baseline
scope and fullParityVerified=false; do not label this as normal feed parity.
The comparison tests passed 14/14. Added a native-Bridge baseline projection
that requires actual Bridge transport, matching observed page/row identity and
capture time; it never fills missing row fields from the target URL. Empty or
neighboring evidence yields zero eligible targets, not an invented baseline.
The foreground wrapper now saves this private projection after the raw result.
Combined comparison/projection tests passed 17/17; wrapper syntax passed.

The user approved one foreground native-target Recapture, and that run was
consumed as `facebook-reel-bridge-baseline/run-032a99d6-59e2-4c96-99a6-2ed362e14203`.
Chrome 154.0.8037.93 and genuine Facebook Bridge readiness passed. The queued
command failed before observation with `visible_recovery_required`, so this
receipt contains no usable media baseline. Fixture cleanup also reported
`CloseForRetry` unsupported scoped host retirement. The wrapper subsequently
verified the original service healthy, one exact registered profile owner and
compatible ready Facebook Bridge (`restored=true`). That restoration does not
erase the fixture's scoped-cleanup failure or prove capture succeeded.

Investigate the genuine foreground/lease authorization path and supported
fixture-only cleanup wiring. Do not manufacture lease ACKs/readiness, change
shared production Bridge behavior, or run another foreground attempt under
the consumed one-run approval. Fix and validate the fixture before proposing
a further live test.

Accounting limitation after the user's resume: native `get_goal` still reports
blocked and 326381/1000000, unchanged during the approved live test. Available
goal tools expose no resume transition and cannot replace an unfinished goal.
Treat that count as the last measured cutoff with partial coverage; work after
resume is currently unmeasured by the native counter. Preserve the requested
one-million cap and historical usage rather than claiming zero/reset or raising
the cap. No completed-goal claim is justified.

Source review confirms the independent cleanup cause: minimized launch marks
the Window as a capture host, but the fixture never binds `SetCaptureHandoff`;
`CloseForRetry` therefore correctly refuses unsafe retirement. For the capture
failure, only the wrapped code survived: cause/details were not exported before
the isolated DB was cleaned up. Preserve genuine failed-run error metadata in a
private receipt before cleanup, and wire the real authenticated close-host
transport. Do not attribute `visible_recovery_required` to authorization or
Facebook surface behavior without its underlying error evidence.

Also preserve the production profile-slot contract: main resolves
`appshell.ResolveProfileDirectory` and passes `--profile-directory` explicitly.
The fixture currently supplies only UserDataDir; add the canonical resolved
slot to the launch and private metadata. Same user-data path alone is weaker
than verified same profile-slot selection. This gap is not yet a proven cause
of the wrapped capture failure.

Fixture-only cleanup correction selected after source review: this test opens
the ordinary product-root UI, not a static split capture host, and the wrapper
requires the registered profile to be fully drained before test launch. Use
the ordinary owned foreground UI lifecycle (`StartMinimized=false`) so its
supported `CloseForRetry` waits for owned-process cleanup. Do not tag that UI as
a hidden capture host, fabricate a close-host ACK, or change the production
retirement guard. Any further live approval must explicitly include the owned
visible test UI plus its one managed Recapture target and runtime restoration.

Root completed this bounded fixture correction after the investigation worker
was interrupted: explicit canonical profile-slot launch, ordinary owned UI
lifecycle, and private failed-run metadata export before DB cleanup. Gofmt,
wrapper syntax, filesystem-only dry-run and HTTP package compilation passed;
the compile command ran zero live tests. Normal owned-process drain remains a
live verification requirement, not a claim from dry-run. No second foreground
run has been issued and the previous one-run approval is consumed.

The explicitly approved retry was consumed as
`facebook-reel-bridge-baseline/run-a1140cdf-4cae-4482-b794-bd3f9bcf7687`.
It failed before Chrome launch (0.03 seconds in test), and the wrapper verified
runtime restoration. Root's previous fixture correction missed a Launch
contract: PrivateCDP requires StartMinimized=true. Correct the ordinary owned
UI fixture to use no private CDP; use canonical executable discovery/version
inspection instead, since its only CDP call was Browser.getVersion. Keep the
production private-CDP/minimized-window restriction intact. Preserve launch
failure metadata privately so early precondition errors are not discarded.

That correction is now applied: private CDP removed from ordinary UI launch,
version 154.x checked through appshell.Discover's Windows executable-resource
probe, early launch error saved privately, and production Launch restrictions
unchanged. Gofmt, HTTP-package compile (zero live tests), filesystem dry-run and
diff check passed. Another live capture has not been issued.

The newly approved no-CDP live run completed as
`facebook-reel-bridge-baseline/run-9ca5d122-7767-4726-a009-0e9a9fa85a3c`.
Registered executable version 154.0.8037.93 and real Facebook Bridge readiness
passed; ordinary owned browser cleanup no longer reported an error. The
wrapper verified restored=true, with healthy service/profile owner/Bridge.
The exact target command still failed before observation. Private error export
now proves causeCode=source_readiness_failed, readiness.state=page_shell, zero
selector/structural/semantic/action candidates, no feed root, complete document,
visual=true and loading=false. The service-worker path shows the authorized
foreground call occurs before target readiness preparation. Its generic error
text still says Quiet, although actual payload is authorized Adaptive Recapture.

This is native Bridge same-target readiness failure, not usable post/media
baseline evidence. Alongside headless discovery failure on the same target, it
supports a shared Reel discovery coverage gap; it does not prove media absent,
login failure, all-Facebook failure, or headless-only regression. Record this
target's unsupported comparison scope and return qualification work to recent
capturable Feed posts. Avoid another unchanged-target foreground retry or
relaxing admission/readiness solely to manufacture a baseline.

The user approved one further Adaptive Update. Session
`session_4b8992906336b466f0f712b68a44e369` completed, and the helper verified
Quiet restored with collectionMode=browser. Exact raw acquisitions exported:
X six blocks, one image, observed 14:15:32Z; Facebook one block, one image,
observed 14:14:51Z, both drivers aku-bridge. The Facebook direct anchor is a
photo route, not verified parent-post permalink; preserve that distinction.

Prepared a new private diagnostic from this session, selecting the X image and
one text target plus the directly observed Facebook photo URL. The 30-minute
freshness gate is enforced per capture. This is same-target media/body testing,
not simultaneous feed coverage. The outer stop/test/restore wrapper retains
its old preflight fixture; use the private session-specific report for actual
comparison IDs and times, never the outer fixture's case labels.

Headless receipt `authenticated-parity-c1f8e30d-7fa5-4d8c-ab7d-4434fc092ce7`
captured one X image target and the Facebook photo target within seven minutes
of Browser acquisition; both succeeded. The second selected X text row was
not requested by the outer wrapper, so mark it untested. X text matched exactly;
author differed only by feed `· 5h`, and the same PNG CDN asset appeared as
`.png` versus `?format=png`. Preserve raw differences while validating the
narrow author/asset representations in the comparison helper.

Facebook returned the native pfbid parent with identical author, 105-character
caption and image host/path. Crucially, coverage.photoParentResolution is
verified for exact photo ID 10164630943077347 and the returned native parent,
with provenance structured_photo_parent_and_matching_native_post. Source
review confirms this marker is emitted after structured binding plus matching
native-post DOM checks; this is stronger than content similarity. Extend the
comparison helper to report this explicit relationship without merging photo
and parent identities or inferring it for unproven cases. Keep full parity
unproven and capture-quality limitations visible.

Fresh-target comparison validation completed: 18/18 focused Node tests passed,
including rejected author handles/display names, conflicting image formats,
spoofed CDN hosts, absent photo-parent proof, wrong native bindings and multiple
conflicting parents. The helper preserves raw author and URL differences;
only the narrowly verified X display-time/PNG-format representations are
normalized. Other image URLs retain exact equality in the additional asset
comparison; host/path equality remains a separate diagnostic.

The combined private report confirms the first X image target has exact
normalized text (37 characters), URL-bound author identity and matching PNG
asset/format, with different raw URLs. Facebook has exact normalized text
(102 characters), exact author and matching image host/path, with different
signed URLs. Earlier 38/105 figures counted raw whitespace; these normalized
counts do not imply a text regression. Photo and native parent IDs remain
separate. Neither signed-URL equality nor overall production quality is proven.

A second headless-only stop/test/restore receipt,
`authenticated-parity-28a8ee9b-f4ec-4aad-b82e-6aa82431d3f5`, captured the
remaining X text target at 14:38:55Z, within 30 minutes of acquisition.
Its exact native ID and normalized 164-character text matched, URL-bound
author identity matched, and both sides observed zero media. The diagnostic
also returned surrounding rows; only the selected native target qualifies
for this comparison. The outer preflight fixture labels are not target evidence.
Both headless receipts verified restoration, healthy runtime/compatible Bridge
and one exact profile owner; the second also verified the registered Chrome
executable. The combined report remains bounded sequential target evidence,
with fullParityVerified=false and production admission unverified.

Remaining qualification gates: fresh Facebook multi-image/video comparison,
actual production admission/Recapture for supported recent targets, authenticated
Settings-reader-login-auto-return journey, and live dev-worker prerequisite
promotion. Instagram/LinkedIn and default migration remain subsequent gates.
The approved single foreground Update is consumed; no additional foreground
collection was performed for the second X headless test.

The next approved single Adaptive Update completed as
`session_b907b90c39264916712362639d810c6a` at 14:43:01Z with Quiet restored and
collectionMode=browser. Raw aku-bridge acquisition yielded seven X blocks
(three images) and one Facebook photo block (one image); no Facebook video or
multi-image baseline was obtained. Do not count that missing content-type
sample as failure or acceptance, and do not repeat foreground Updates without
new approval. Browser Facebook coverage was bounded: two viewport snapshots,
one scroll performed of two requested, stop=no_movement, one unique block.
This limited yield does not describe the user's whole feed.

Receipt `authenticated-parity-bcf20dd4-9f43-4726-9881-eb4e0f4e0afe` compared
fresh targets within three minutes. X quote post 2106324463054626846 matched
native identity, URL-bound author, normalized 340-character text and both image
assets/formats. Raw media URLs still differ. The second selected X text target
was not requested and is untested in this window. Facebook photo
10164631460962347 failed with photo_parent_unverified; the first photo's success
does not generalize to every fresh photo. Runtime and compatible Bridge were
restored healthy, with one registered executable/profile owner.

Added privacy-safe photo-parent corroboration error diagnostics in capture.mjs:
route equality plus bounded candidate/identity/author/text/image match counts,
without content, IDs or URLs. No admission check or timeout changed. Focused
navigation tests passed 3/3; full worker suite passed 54 with one opt-in real
Chrome playback test skipped. Candidate package still has the earlier capture
module; the subsequent diagnostic imports the local source module explicitly,
with packaged launch/assets. It is not verification of a rebuilt package.

The first source-diagnostic helper attempt exited before capture because its
relative import resolved under the package working directory; receipt
`authenticated-parity-93e71f2e-9c56-4e69-9fb8-a0b314e40066` verifies restoration.
After the causal absolute-import correction, receipt
`authenticated-parity-d249d318-7068-44e7-80ff-fc887a36e231` reproduced the photo
failure at 14:47:49Z. Diagnostics prove routeMatches=true and zero post/native
identity candidates after the bounded parent hydration wait. This is parent
post discovery failure, not mismatching caption/author/image, and does not by
itself prove Facebook returned a native content-unavailable notice.

Receipt `authenticated-parity-39b89f22-0791-4799-a663-3920c2083c7e` then tested
the existing media-only missing_media Recapture path for the same photo at
14:49:04Z. It succeeded with exact photo metadata and visible-image ownership,
facebook:photo:10164631460962347, one image and the same image host/path as
Browser. It did not navigate to or admit the unavailable-to-extractor parent.
This establishes recoverable media, not full post/text parity or a fresh Store
merge. Existing media-only Store rules retain saved body and identity; do not
promote photo identity into a post or enable a blind full-collection fallback.
Both completed diagnostic receipts verify healthy runtime, compatible Bridge,
one profile owner and registered executable after restoration.

Current classification: X two-image target is equivalent within this bounded
window; the new Facebook parent-target capture has a reproducible discovery
gap, with successful media-only recovery. Facebook feed replacement and
video/multi-image parity remain unverified. Next useful evidence is acquisition
of the same recent Facebook item on the headless feed plus actual media-only
Store admission, preserving body/identity and diagnosing discovery separately.
Avoid interpreting repeated generic Updates as guaranteed video sampling.

Receipt `authenticated-parity-477df82c-2213-4ffe-8df0-cb0330b544e7` searched
the authenticated headless Facebook feed with two bounded scrolls at 14:51:45Z.
It returned two observations of one different native pfbid post and one image
per observation. ID, author, text and image paths did not match the Browser
photo target. Feed order/yield differences remain unverified parity, not a
proven source regression. The final frontier had no anchors, so continuation
was not exercised. Healthy runtime/compatible Bridge and selected-profile
ownership were restored; no foreground collection was performed.

Added opt-in `TestPhotoMediaLiveEvidencePreservesSavedBlock` to replay the real
Browser photo baseline and successful headless media-only observation into an
isolated Store. The fixture deliberately removes the saved image to simulate
missing-media recovery; it does not edit production data. The actual owned
headless job claim/completion path recovered the captured image and preserved
every non-media field via deep comparison, with exactly one Timeline item.
The test passed using session_b907b90c39264916712362639d810c6a's evidence.
Default tests skip without explicit evidence-file environment variables. No
network or browser operation occurs in this replay, and this is not a real
installed-app UI click or proof the production item was missing its image.

Next foreground acquisition should be targeted toward a known Facebook video
or multi-image surface rather than another generic Update with the same short
feed. Prepare a concrete read-only fixture before requesting that scoped
approval. Installed-app authenticated Settings/reader/auto-return remains open;
no candidate installation, default change, commit or push occurred in this step.

Update this ledger with exact validation and unresolved gaps after each phase.
A phase is complete only when its acceptance gate passes. Changes to scope or
invariants must be recorded here before implementation. This roadmap does not
authorize commit, push, installation, runtime restart or release publication.

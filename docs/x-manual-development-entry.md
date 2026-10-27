# Manual X pilot: ordinary development entry

## Implemented boundary

The Windows `auth_session_dev` build now accepts `native-x-manual` with an
explicit `--dev-x-manual-plan`. It continues through the ordinary application
configuration, selected reasoning provider, Engine and HTTP server. It does not
enter the isolated QA stdin controller or substitute deterministic reasoning.
Default, fixture-only and non-Windows builds reject this selection. No flag means
unchanged ordinary startup, no pilot factory and no session derivation.

The first pilot uses the existing, physically pinned application resource plan
(`aku.x.application-qa.v1`) and its isolated database/profile namespace. Reusing
this resource validator does not turn QA metadata into account or Store authority.
Existing user databases and arbitrary browser profiles are deliberately not
accepted by this entry. Neither a consumed plan nor a removed collector can be
reused. Preparation and a passing prior QA do not grant execution consent.

Before configuration/SQLite access, selection requires `--dev`, the exact selector
and a canonical plan; rejects QA-selector mixing, database inspection/reset,
runtime probe/discovery modes, split-capture mode and profile/extension overrides;
and binds explicit database/port overrides to the selected plan. Configuration
must be the pinned `<instance>/config/sidecar.json`. Loaded root, database and
exact `127.0.0.1` server address are checked again before opening the database.
Shared validation retains directory/file identities, asset hashes, instance
lease, database/version checks and deferred retained-reader acquisition.

The selected configuration retains its declared provider/model and normal
application settings. Activation preparation must supply a reviewed, complete
instance-local configuration; this implementation does not copy the main config,
read a main browser profile or alter a user's model/settings.

## Explicit start, status and stop

`POST /api/development/x-manual/start` requires the ordinary same-origin/loopback
checks, current `X-Aku-Split-Epoch`, JSON content type and exactly these fields:

```json
{"acknowledgeLimitations":true,"allowOneSessionDerivation":true}
```

The body is limited to 1 KiB. Unknown/duplicate fields, missing consent, trailing
JSON, old epochs and non-development use reject. The body accepts no executable,
profile, database, cookie, account proof or backend selection. The attached
trusted selection consumes one start attempt, even after failure. Neither another
request, persisted opt-in nor restart can inherit its consent.

The existing X source gate closes before creating the pending one-round session
and command. The existing private factory then supplies owned Node/Chrome,
one-use seed, paired startup evidence, broker, separate reader/collector Managers,
Coordinator and Store issuance. Capture starts through the existing one-use
task. Existing transactional claim/result checks remain authoritative. Saved
Auto Update/settings are unchanged; non-X routes remain available. A failed
attempt cannot silently become ordinary X recapture or a retry.

`GET /api/collection/runtime` retains its safe `xManual` projection. A configured
but unstarted provider reports `disabled`, `explicit-start-required`, available;
that is operational availability, not capture authority. Start/stop reasons stay
separate from cleanup. Early setup records its first closed reason, without peer
text, account fields, credentials or raw body data.

The existing `POST /api/development/x-manual/stop` cancels startup and waits for
its completion before deciding whether a newly installed route must be stopped.
Stop retires the collector and revokes admission through the existing broker;
visible reader ownership stays with Sidecar. Whole-parent shutdown disposes all
owned children. Uncertain cleanup retains X fencing and selection pins; repeated
Close cannot turn that uncertainty into success.

The original 60-second seed attempt, startup/freshness limits and foreground
capability lifetimes are unchanged. No renewal, reseed or automatic fallback is
introduced.

## UI and native broker composition

Optional `--app-shell` requires the selected capture Chrome executable and the
separately packaged Chrome for Testing UI through existing `DiscoverSplitUI`
validation. Branded system Chrome cannot silently substitute for an extension-capable
UI. The UI gets a new leased `<instance>/profiles/ui/profile`, separate from both X owners. A
fresh copy of the embedded UI broker is restricted to the exact instance origin;
the extension key/ID, native host, active-tab/foreground checks and message protocol
remain unchanged. Source and prepared-controller hashes include these embedded
assets. There is no profile copy or registry write.

Ordinary main attaches the existing OS-authenticated reader pipe handler to this
UI too. HTTP split-action routing selects the private reader only for X, bypasses
the ordinary exclusive profile borrow, and still waits for the matching native
helper before preparation/foreground/navigation. Other sources retain their
ordinary path; no global direct-reader replacement is installed.

Deployment must include the compatible `aku-reader-broker.exe` and its existing
native-host registration at the same application directory. The existing named
pipe remains exclusive (`FILE_FLAG_FIRST_PIPE_INSTANCE`); a pilot UI cannot take
over another live application's broker. An operational activation plan must
check helper registration/pipe availability before launch. This source/test
increment neither registers a host nor stops another instance.

Illustrative process selection after separate operational approval:

```text
<development-tag-image> --dev --dev-session-reuse=native-x-manual
  --dev-x-manual-plan=<fresh-plan.json>
  --config=<instance>/config/sidecar.json
  --database=<instance>/state/pilot.db --port=<plan-port>
  --app-shell --chromium-path=<plan-chrome-path>
```

Preparing these files does not invoke start. The explicit authenticated start
request is a separate action. Rollback stops/drains the pilot, verifies owned
cleanup, then launches without the experimental selection using the prior
reviewed config/binary. Never reset schema, erase fenced commands, clear challenge
state, change Auto Update or close user reader windows as an implicit rollback.

## Offline validation and limits

Run `scripts/test-x-development-entry.ps1`; `-MainOnly` selects the four-build and
ordinary-main bootstrap checks. Windows sandbox networking can reject loopback
between the test process and its child; those bootstrap checks need the approved
outside-sandbox test environment, not a weaker application host/origin check.

Tests use fresh accountless profiles/database and a fixture-only private factory.
They cover selection before database access, config/profile/image scope, explicit
consent/replay, source admission, capture persistence, authenticated reader routing,
stop during startup, parent loss, first cause, uncertain Close and unchanged saved
settings. HTTP checks cover foreign origin, old epoch, duplicate/unknown fields,
oversized/trailing consent and missing provider. Four actual-main build modes are
compiled. A hidden ordinary dev image with a synthetic config/database reaches
HTTP readiness while still requiring consent, then shuts down through the existing
authorized idle-shutdown API. That check never calls native start.

Integration tests exposed an Engine drain gap: an active run entry was removed
before its deferred database/capture cleanup finished. Engine now counts the
whole worker through that defer; `WaitForIdle` includes the drain count. A
deterministic regression holds the real terminal-cleanup lock after active-entry
retirement and proves idle stays false until release. Completed-work fixtures
drain Engine, close SQLite, then make the final idempotent selection Close.

The 2026-10-27 commit review also found that a pending manual startup was outside
that process-worker count. HTTP graceful shutdown normally joins its handler,
but an expired HTTP grace period could leave startup cancellation cleanup using
SQLite after main had closed it. Engine idle now also includes the starter's
completion channel. Main joins selection cleanup before database closure; if
that bounded join is uncertain, it retains the database until process exit.
Accountless regressions hold startup before route publication and check busy
reporting, selection pin retention, ordered close and refusal to close SQLite
after uncertain cleanup. This does not extend the startup or attempt deadlines.

A separate investigation reproduced a pre-existing cancellation leak in
`modernc.org/sqlite v1.29.10` independently of Engine: a canceled query can discard
returned rows without finalizing their statement, and `sqlite3_close_v2` can leave
that orphan holding the file despite reporting success. A bounded synthetic
stress repro under `build/sqlite_cancel_diagnostic_test.go` and
`build/sqlite-cancel-diagnostic.txt` records the old failure; it is not a
deterministic scheduling test or native-account evidence. The separately
authorized [driver upgrade](sqlite-driver-upgrade.md) now selects v1.57.0 and its
matching libc. The original repro passed five times with zero orphan statements
on the initial v1.40.1 selection; current v1.57.0 evidence is recorded separately
in the upgrade note, including the public-API cancellation/file-release regression
and same-database old/new/old round trip. No unsafe statement sweeping, deadline
removal or Store policy/schema change is included. Completed-flow tests retain
natural reasoning drain; cancellation ownership has its separate regression.

Accepted final run `a2bc24a2cc804171a53b286d1781ee79` passed 67 top-level Go groups
and 59 nested cases, no skips/failures, and all four actual-main builds. Its
canonical log is `runtime/qa/auth-session-sidecar/x-development-entry-a2bc24a2cc804171a53b286d1781ee79.log.txt`;
the owned temporary test root was verified removed. The ordinary-main loopback
test ran outside the sandbox; no native start or browser was invoked.

Controller regression run `de8e9f27bc964e3cb3e900234f62c84d` passed 18 Node cases.
Staging run `a544b1be9f754b6ca66114154eb4de84` compiled both legacy retained manual
and capture modes without executing their native tests. Those logs are under
`runtime/qa/auth-session-sidecar/x-qa-entry-<run>.log.txt`. No source-cache work
from other tasks was included. Main config SHA256 remained
`52063d73180a8f6ab3e514d89cfaa81fb168e0b98381e58fa7ff1ae28d7aadae`;
Store source, schema, policy and dependency versions were unchanged.

These checks do not execute Chrome, transfer a real session, register a native
host, access the retained login, or change main-dev runtime. Physical UI/foreground
and real-X operation of this new ordinary entry are not claimed by synthetic
acceptance. Prior native QA remains evidence for its accepted payload, not implicit
permission for a new run or main-dev activation.

### Commit-scope review acceptance (2026-10-27)

The reviewed wiring and worker-drain payload was exported from Git's staged
tree, excluding local config, source-cache helpers and unrelated build-script
changes. `build/mdr-82d735d5/console.log` records 70 top-level Go groups,
59 nested cases and four actual-main builds, with no skipped or failed cases.
Its canonical helper used the existing local module/build cache and Node image;
only those environment paths were adjusted in the exported helper, leaving
the staged test/production source, deadlines and assertions unchanged. Owned
temporary run-directory cleanup was verified.

`build/mdr-82d735d5/controller.log` records 18 passing controller/gate cases
against the staged source. Both legacy retained manual/capture staging modes
also compiled without executing their native tests; their disposable build
roots were removed. The lifecycle correction above received a second read-only
review with no further findings in that correction. This is accountless source
acceptance, not authority to activate or run a native session transfer.

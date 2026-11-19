# SharedDOMEngine implementation roadmap

The roadmap turns the [SharedDOMEngine design](shared-dom-engine-design.md)
into staged work for AkuBrowser. The target is one source-independent
evaluator and one browser artifact shared by Windowed and Headless. X is the
first integration pilot. A stage is ready for review when its exit evidence
is available.

| Stage | Deliverable | Exit evidence |
| --- | --- | --- |
| 1. Characterize both consumers | Run the same sanitized X DOM corpus through current Windowed and Headless. Cover legacy and wrapped markup, mixed layouts, nested quotes, media-only posts, placeholders, delayed hydration, conflicting identities, empty feeds, and access failures. | The baseline reproduces the wrapper-only Headless identity gap and records existing readiness, candidate, and ownership decisions for each consumer. |
| 2. Build the standalone evaluator | Implement the data-only contract and deterministic diagnostics. Build one Node entry point, one browser artifact, and a manifest from the same source. | The evaluator runs without DOM or application dependencies. A synthetic source with custom elements and a non-URL identity uses the same evaluator without an engine change. Contract, scope, bounds, and privacy tests pass. |
| 3. Add shadow diagnostics | Connect both X consumers to the same browser artifact and project diagnostic results beside the current decisions. | Both consumers use the same artifact SHA-256. Shared fixtures verify classification and report intentional differences. Shadow evaluation causes zero extra capture mutations or Store writes. |
| 4. Migrate X identity | Move both X consumers to shared primary identity resolution for the legacy timestamp and wrapped status evidence. | Both modes agree on identity, quote ownership, and candidate decisions for equivalent inputs. Route, lease, deadline, round-two anchor, and dedup regression tests pass. Record separately authorized live verification for each mode. |
| 5. Onboard additional sources | Add Facebook, Instagram, and LinkedIn one at a time through the probe contract. | Each source passes its own fixture corpus through both consumers. New source-specific logic stays in the adapter and consumer registration. Unsupported capabilities are recorded. |
| 6. Qualify the release and rollback | Verify the paired runtime artifacts, run authorized mixed-source validation, and exercise rollback to a compatible version tuple. | Record offline, live Windowed, live Headless, and release qualification separately. Verify source failure isolation, packaging provenance, and rollback readback. |

Stages are ordered. Review shadow evidence before switching X admission to the
shared result. Review the X pilot before onboarding the next source. SI tracking
approval records the roadmap. Implementation, live runs, and production
deployment follow their own authorization boundaries.

[Stage 04 verification](shared-dom-engine-stage-04-verification.md) records the
offline fixture and guard results alongside the two authorized live X runs.

## Card-first adapter contract

- Define the port for discovery, owned card evidence and shared evaluation.
  Use the LinkedIn shadow probe as an initial integration.
- Migrate X, LinkedIn, Instagram and Facebook in slices. For each source,
  demonstrate that discovery can identify a card without a permanent ID and
  that nested or neighboring content cannot supply its primary fields.
- Run the same corpus through Windowed and Headless. Record source-specific
  exceptions and each mode's admission behavior before moving a source to
  shared production evaluation.

## Scoped locators and browser fixtures

- Pilot strict, scoped locators with real-browser fixtures. Cover re-rendered
  and recycled cards, zero or multiple matches, same-author cards and foreign
  popovers. Assert identity and side effects through both capture entry points.
- Use the same sanitized fixture and expectations for Windowed and Headless.
  Control clocks and scroll position where the behavior depends on them.
- Qualify one real adapter path before making scoped locators the default for
  additional interactions. Keep live evidence and source activation separate
  from fixture acceptance.

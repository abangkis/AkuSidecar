# Instagram readiness parity (B1)

Status: source implementation published and verified offline on 2026-11-15.
The ordinary development bundle was built and, after the owner's manual restart,
verified active with binary SHA-256
`dc59171e67fc0e18fc8a8e40a6499be543a3a92cd5e3021e470509ec2c2307f1`.
All 128 active artifacts and 114 worker files matched the verified B1 bundle;
Supervisor, API and Bridge health passed. This is B1 activation/health evidence,
not live Instagram or native-provider qualification. Unrelated packaging and
retention changes remain outside this delivery.

The Windowed readiness probe and Headless collector now call the same Instagram
`assessReadiness` policy through the AkuBridge adapter registry. Source-specific
discovery and selectors remain in `adapters/instagram-adapter.js`; no second
Instagram policy or recovery controller was introduced.

Headless supplies the same candidate, structural, visibility, document, loading,
and no-configuration visual-hydration facts used by Windowed. Instagram currently
has no visual-hydration contract: `visualHydrationRequired: false` with the legacy
default `visualHydrationReady: true` does not prove that the feed has settled.

## Observable behavior

| Fixture | Shared state | Shared diagnosis | Passive recovery hint |
| --- | --- | --- | --- |
| Structural article without native permalink | `feed_hydrating` | `post_permalink_unhydrated` | `reset_feed_top` |
| Offscreen native post | `feed_not_visible` | `post_outside_capture_viewport` | `align_first_candidate` |
| Loading interactive document | `loading` | `navigation_incomplete` | None |
| Loading complete document without eligible visible content | `loading` | `navigation_incomplete` | None |
| Loading structural article without permalink | `loading` | `navigation_incomplete` | None |
| Settled visible native post | `feed_ready` | `readiness_observed` | None |
| Unrecognized markup without visual proof | `selector_mismatch` | `feed_state_unverified` | None |
| Empty-feed fixture without recognized empty proof | `selector_mismatch` | `feed_state_unverified` | None |
| Incomplete page shell | `loading` | `navigation_incomplete` | None |
| Complete page shell without feed root | `page_shell` | `readiness_observed` | None |
| Login without feed | `login_required` | `login_required` | None |
| Login with feed shell | `login_required` | `login_required` | None |
| Login with visible native post and loader | `login_required` | `login_required` | None |

Availability and access blockers retain precedence. A loading base state cannot
be replaced by the adapter; a loading indicator also takes precedence when no
eligible candidate is visible. Existing base behavior for already visible native
content during background loading is retained.

`sourceReadinessAssessment` is bounded passive data in Headless snapshots,
observation coverage, and ordinary empty-capture diagnostics. Projection admits
known diagnosis codes and validated one-attempt hints, excludes arbitrary page
text and unknown fields, and leaves older snapshots and other sources compatible.

## Authority boundaries

Assessment does not call `recoverReadiness`, click, align, reset scrolling,
recreate a managed surface, or grant mutation authority. Controllers still own
route, acquisition-round, frontier, deadline and recovery decisions. No new
recovery execution path is enabled.

Readiness does not establish native account/document proof, authentication,
capture completeness, or an empty feed. Native provider, paired proof, capability
declarations and admission gates are unchanged. The native Instagram recipe
remains separately unqualified; B1 is not provider qualification.

## Offline verification

- `node --test --test-isolation=none` in AkuBridge: 638 passed.
- `node --test internal/collection/headless/worker/test/*.test.mjs` in AkuSidecar:
  330 passed, 4 skipped, 0 failed.
- Native Instagram session, provider and owned-bootstrap fixture suites:
  153 passed, including rejection of account mismatch, document replacement and
  invalid role/nonce/source evidence.
- The new `instagram-readiness.test.mjs` executes the real registry, adapter,
  Windowed message probe and Headless collector on the same 13 synthetic fixtures.
  It verifies equal normalized facts, state, diagnosis and hints; propagation into
  observations and empty diagnostics; unchanged coverage/proof; and zero network,
  DOM mutation or recovery execution.

The fixtures are synthetic Node VM DOMs. They do not prove live markup coverage,
rendering/geometry parity for every layout, or behavior of an activated bundle.
For B2 freshness/quality reconciliation and retained limitations, see
[Instagram adapter parity](instagram-adapter-parity.md). B1 activation does not
attest that later B2 source changes are loaded. Live qualification remains a
separate decision.

# Instagram adapter parity (B)

Scope: B1 shared readiness and B2 freshness/quality reconciliation under the
existing Headless adapter-parity roadmap. Native provider qualification and
SharedDOMEngine adapter onboarding retain their separate roadmap owners.

## Delivered behavior

B1 uses the actual Instagram adapter's read-only assessment in Windowed and
Headless with equivalent normalized facts. Login/loading priority and passive
recovery hints are preserved. No-candidate feeds remain unverified. See
[readiness evidence](instagram-readiness-parity.md).

B2 reuses the A4 quality-assessment implementation for Instagram. The provenance-
hashed Bridge quality policy is included in Instagram's executable source assets.
The collector emits a closed boolean fact schema, not a verdict or retry grant.
Final Headless projection runs the same policy and actual adapter semantics on
the final captured/recovered candidate with zero remaining retries. The existing
X API and policy remain supported by the same implementation.

At equal terminal budgets, real-adapter synthetic fixtures agree with Windowed's
actual quality call for caption, owned image, detected image without URL, video
poster, observed playback URL, collapsed caption and media-only content. Recovered
media is reassessed. A foreign schema, arbitrary verdict or malformed fact cannot
replace policy. Legacy/structured observations lacking DOM facts receive no
invented assessment.

`captureQuality.status` remains `unverified`. A `complete` assessment means only
that the existing `social-post-v2` requirements are met; it does not establish full
text, all carousel slides, fresh content, verified playback, authentication or
native admission. Collapsed text and unresolved/unverified video keep their
existing limitations. No new Instagram `isComplete` hook, acquisition retry,
navigation, recovery execution or controller authority is introduced.

## Freshness comparison and retained boundary

| Equivalent condition | Windowed | Ordinary Headless Instagram |
| --- | --- | --- |
| Active feed without pending control | `active_feed_ready` from dispatch contract | `not_verified`, `source_contract_unsupported` |
| Background feed, unchanged after bounded wake | `adapter_wake_settled` from adapter contract | Same explicit unsupported result; no wake/probe |
| Visible pending-content control | Reveal unsupported; `freshness_unavailable` | Same explicit unsupported result; no click |
| Follow-up acquisition | `follow_up_preserved`, no wake/reveal | Freshness contract remains disabled; no mutation |

The fixture uses the actual Instagram strategy, shared source probe, Windowed
freshness controller and Headless contract/controller. Headless performs zero
freshness page evaluations even when a permissive request asks for reveal.

Retain `instagram.freshness.headless.enabled: false` and `revealSupported: false`.
Windowed wake/settled-current evidence is not equivalent to owned Headless
primary-identity verification. Return to this limitation only when an Instagram
feed-route matcher, exact primary-identity contract and separately authorized
qualification demonstrate safe freshness evidence on that owned route. Reuse
the existing controller, deadlines, mutation consent and continuation guards.
Do not activate generic reveal or repeat account probes to close track B.

## Retained baseline and qualification ownership

- Caption/author/media extraction, presentation/links, bounded shared expansion
  and structured/native video recovery already existed and remain in use.
- Native Instagram remains feed/round-one and paired-proof bounded. Existing
  Headless Stage 04 (`1787862f-af5d-48bb-9983-b56b4bcad4d5`) owns account,
  bootstrap, document continuity and authenticated live qualification. Return
  there for a concrete provider failure or native live acceptance request.
- Existing Stage 03 (`4a199ec5-2dc1-4017-a48a-0beae6519e10`) owns further
  SharedDOMEngine adapter onboarding. A generic profile verdict is not that
  onboarding or provider acceptance.
- Source-specific video-completeness or expanded profile requirements require a
  concrete owned-media/quality failure and equivalent fixtures before selection.
  Playback and complete-carousel claims remain unqualified until separately
  demonstrated; B2 preserves rather than invents those capabilities.

## Verification and delivery limits

New fixture suite: `internal/collection/headless/worker/test/instagram-capability-parity.test.mjs`.
It covers seven equal-input quality cases, final-media/schema trust boundaries,
actual capture composition with real assets, and four freshness comparisons.
B1's 13 readiness fixtures remain covered with the actual quality asset loaded.

Worker regression: 343 passed, 4 skipped. Native Instagram session, provider and
owned-bootstrap fixtures: 153 passed. The retained Bridge source/policy is
unchanged from its 638-test B1 verification. Native recovery and account/document
rejection behavior remain unchanged.

B1 publication/activation is already verified. B2 implementation and offline
evidence are separate from B2 activation and any live capture. Closure of this
implementation/reconciliation scope must not close Headless Stage 03/04, enable
freshness, certify native Instagram, or imply full media/playback acceptance.

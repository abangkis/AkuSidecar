# LinkedIn adapter parity (C1)

## Selected corrections

Windowed and ordinary Headless now use the same `observeCandidateFacts` and
candidate-quality evaluator with the actual LinkedIn adapter's expected-kind and
media-completeness policy. Headless carries a closed boolean fact schema and
evaluates the final candidate after media enrichment, with zero remaining retries.
Page-supplied verdicts, profiles, arbitrary schemas and legacy observations without
facts cannot substitute an assessment. `captureQuality.status` stays `unverified`.

The original Windowed placeholder bug is not assumed to exist in Headless.
Headless already detects expected video and has bounded hydration/recovery.
Those controllers retain their distinct retry budgets, waits, deadlines,
candidate caps, cancellation, exact post/player guards and retained frontier.
No generic acquisition-engine migration or new mutation authority is needed.

An owned progressive MP4 observed directly in the DOM now retains truthful URL
observation through additional-media recovery and final projection. The existing
LinkedIn native identity and strict progressive-media completeness policies must
both pass, and the observation must carry the LinkedIn DOM fact schema and direct
DOM evidence. The outcome is `owned_playback_url_observed`, with
`video_playback_unverified`; it never becomes verified playback or native admission.
Missing/unsafe/ambiguous evidence remains unresolved. Historical data is unchanged.

## Equivalent-input offline evidence

`internal/collection/headless/worker/test/linkedin-capability-parity.test.mjs`
loads the actual adapter, Windowed quality call, Headless DOM collector, shared
structured reader, final projection and actual capture composition.

The fixture retains normalized fields from the existing v124 William Faucher
placeholder/cover fixture. Its DOM and player responses are reconstructed and
synthetic; the original source DOM was not captured. It covers empty video
placeholder, cover-only, existing DOM MP4, hydration recovery, unresolved result,
foreign owner/player, short deadline, schema/verdict trust boundaries and unsafe
URLs. Equal terminal budgets produce equal assessment fields. No fixture effect
can click, scroll or fetch live source content.

The existing Windowed v124 negative control remains an in-memory old-call-path
control. New source behavior uses the shared observer; the control still proves
why the historical fallback missed a placeholder with text and no media URL.

## Retained ownership

Shared parsing, text expansion/restoration, authored links, attachments, direct
context, freshness reveal, structured video and continuation are retained.
Headless Stage 03 owns SharedDOMEngine onboarding, and Stage 05 owns LinkedIn
continuation/live media acceptance. Track C records bounded source composition
parity, not those independent qualification outcomes. A complete quality profile
does not establish full media, actual playback, authentication or native proof.

Delivery uses the ordinary development bundle and coherent `source-adapters-v127`
identity, with `linkedin-dom-v24`. Publication, offline packaging, activation and
live qualification must be reported separately; this document does not infer
runtime or live acceptance from source tests.

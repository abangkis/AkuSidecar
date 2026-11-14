# Timeline background refresh

## Implemented contract

Background Timeline responses that match the last successfully rendered
presentation do not rebuild cards, dispose inline players, or restore scroll.
Explicit foreground refresh and batch reveal still render normally.

`internal/httpapi/web/timeline-background-refresh.js` owns the immutable
presentation checkpoint and canonical comparison. It does not fetch, poll,
route cards, access the DOM, or change settings. `timeline-rendering.js` supplies
the effective presentation inputs and orchestrates the existing refresh path.

The checkpoint conservatively includes ordered item payloads, check and batch
metadata, settings, effective inline/drawer/hidden routing, prepared batch count,
highlight input, and relevant control availability. Object property order does
not affect comparison. Full payload comparison intentionally favors showing new
evidence and detection details over suppressing every possible metadata-only
render; it is not a pixel-equivalence test.

Capture the candidate before rendering, invalidate the old checkpoint before
destructive rendering, and commit only after success. A render failure can leave
partial DOM, so a previous successful checkpoint must not survive that failure.

An unchanged response still:

- Updates bootstrap check, batch, and auto-update metadata and scheduler status.
- Rearms or cancels detection polling from the fresh response's pending flags.
- Schedules eligible passive media enrichment.
- Retains rendered entry objects, since existing handlers close over them.

The first increment was published as `1e1164f9ac7303bf57307b0e9a8e0f0687b4bec9`
and activated independently. It skips unchanged rendering while retaining the
original application timing for changed responses.

## Verification

`test/timeline-background-refresh.test.mjs` exercises the real rendering factory
with controlled API, timers, DOM ports, and responses. Cases cover identical
responses preserving card identity and avoiding teardown, repeated pending polls,
new detection details and routing, pending media, poll cancellation, metadata and
evidence changes, explicit foreground refresh, and failed-render recovery.

The tests do not establish real browser/GPU scroll performance or live playback.
An isolated browser fixture using assets fetched from the active runtime verified
that identical responses retain the actual card and video elements while polling
later admits a controlled detection result. No model calls or database writes were
used for that fixture.

## Fetch, stage, and apply

The second increment implements the owner-approved Astra design in separate
modules. `timeline-refresh-coordinator.js` owns serialized/coalesced acquisition,
independent detection polling, latest-snapshot staging, explicit application, and
request/context invalidation. It has no DOM or routing logic. Foreground refresh,
navigation, and settings/context changes invalidate obsolete background work.
Transient fetch failure rearms known pending detection; a failed foreground fetch
restores polling from the retained presentation without restoring an old stage.

`timeline-refresh-ui.js` owns the fixed update notice, reading guards, and
synchronous visible-card anchor restoration. The notice does not displace cards
or take focus. Changed responses remain staged until the user chooses **Apply
updates**. Application waits for scroll idle and rechecks active playback, text
selection, editors, and focused controls across both the inline Timeline and the
AI Signals drawer. A blocked or failed application retains its stage for retry.

The renderer remains responsible for presentation inputs and DOM construction.
It supplies the coordinator's ports and commits its immutable checkpoint only
after rendering succeeds. Fresh detection is acquired and polling is updated even
when application is deferred. The latest same-context staged result replaces
older staged results; the user does not need to apply each intermediate response.

`test/timeline-refresh-coordinator.test.mjs` covers acquisition, poll, stage,
context/epoch, failure, and concurrent-application sequences.
`test/timeline-refresh-ui.test.mjs` covers both reading surfaces and visible-card
anchor restoration. The browser fixture also verifies that a controlled new
detection leaves cards mounted, disables Apply during an actual text selection,
and applies the result after selection ends and the user clicks Apply.

## Remaining boundaries

Publication and runtime activation of the second increment are verified
separately in the delivery evidence and SI roadmap. Applying changed data still
rebuilds the list; keyed safe patches are
deferred. An explicit Apply can replace paused media and transient card state.
The reading anchor falls back to a surviving visible card, then current scroll
when all visible cards disappear. Server snapshot revisions are unavailable, so
local request identity is not a claim about server revision ordering.

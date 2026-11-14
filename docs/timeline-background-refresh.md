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

The existing scroll-idle and interaction guards remain in place. This change
does not alter application timing for responses that change presentation.

## Verification

`test/timeline-background-refresh.test.mjs` exercises the real rendering factory
with controlled API, timers, DOM ports, and responses. Cases cover identical
responses preserving card identity and avoiding teardown, repeated pending polls,
new detection details and routing, pending media, poll cancellation, metadata and
evidence changes, explicit foreground refresh, and failed-render recovery.

The tests do not establish real browser/GPU scroll performance or live playback.

## Astra consultation: proposed async follow-up, not adopted

Separate acquisition, staging, and presentation application. Keep fetching
detection results independently of whether destructive rendering is safe. Retain
the latest eligible staged snapshot and coalesce superseded snapshots within the
same presentation epoch.

For structural changes such as insertion, removal, routing, or layout-affecting
content, offer application at an explicit user action. A 350 ms scroll pause does
not establish that reading or playback has ended. Narrowly proven safe updates
can use keyed patches that preserve existing card and media elements, focus,
selection, and expansion state.

Use request generations and presentation epochs to reject responses superseded
by foreground navigation, reveal, or settings changes. Prefer server revision
ordering when available: request-start order alone does not prove freshness.
Protect active playback explicitly, rather than treating focused controls as a
complete reading-interaction signal.

This is a future design proposal. Staging UI, keyed patches, new request fencing,
and additional interaction policies are not implemented by the checkpoint gate.

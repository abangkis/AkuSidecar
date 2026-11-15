# Single-permalink capture

`POST /api/updates` accepts an optional `captureTarget` alongside `intent`:

```json
{
  "intent": "Check primary and quote timestamp ownership",
  "captureTarget": {
    "source": "x",
    "permalink": "https://x.com/author/status/123456789"
  }
}
```

The initial scope supports canonical HTTPS X status URLs only. The source must
already be enabled, granted, and allowed by ordinary capture admission. Native
daily routes are rejected. Requests without a target retain ordinary batch behavior.

The session freezes its target in coverage and limits delivery to one item.
Saved preferences do not change. Browser and Headless retain their normal lease,
collector, cancellation, and cleanup ownership. The capture command uses zero
scrolls, `preserve_target`, `detect_only`, and one acquisition round. Acquisition
planning cannot enqueue a follow-up. Both routes reuse their existing shared X
adapter, timestamp policy, and candidate-quality implementation.

Before persistence, the landed page must match the requested canonical URL. Only
primary blocks with that exact permalink are admitted; quoted targets and nearby
posts cannot satisfy the request. Missing target evidence fails closed. Nested
quote evidence is preserved under its primary post, including its independent
timestamp. This path never infers a missing timestamp from a quote or capture time.

Normal evaluation, selection, cooldown, and Timeline publication still apply: a
capture is not a promise of a new Timeline row. There is no historical backfill,
manual evidence override, media-repair reason, or separate package. Offline
verification does not activate the change or establish live qualification.

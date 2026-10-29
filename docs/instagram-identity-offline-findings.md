# Instagram identity acquisition: offline findings

29 October 2026. Source inspection and synthetic tests only. No browser,
signed-in profile, native request, session transfer or runtime change was used.

## Known failure and limits

The historical `native_source_evidence_identity` failure does not identify the
HTTP status, response schema, page state or failing reader/collector role. It
cannot establish that a missing request header caused the failure.

The current candidate makes one same-origin GET to
`/api/v1/accounts/current_user/?edit=true`, with credentials and an Accept
header. It requires HTTP 200 JSON, `status: "ok"` and a positive decimal
`user.pk` represented as a string or safe integer. It cross-checks that identity
against `ds_user_id`, pins the exact Home document before and after observation,
and enforces the existing response and projection limits.

Inspection of AkuSidecar, AkuBridge and AkuBrowser source found no other
current-user request recipe or established app-ID/CSRF/claim header contract
to reuse. The existing Bridge Instagram adapter and main-world resolvers
extract rendered feed/media data; they do not establish an authenticated
current-user API contract. Neither content-author identity nor cookie presence
can substitute for authenticated viewer identity.

## Implemented diagnostic prerequisite

The pending session diagnostic change returns closed, value-free rejection
categories and carries the reader/collector role through evidence validation.
It distinguishes page, HTTP, body and account-shape rejection without returning
server messages, raw responses or account values. Malformed projections fail
closed before session-cookie access.

This investigation adds no request header, alternate endpoint, account-field
fallback, relaxed guard or cookie-as-identity fallback. None is supported by the
available offline evidence. The recipe remains `nativeQualified: false`.

## Validation and next evidence

The following focused command passed all 31 tests during this investigation:

```text
node --test internal/collection/headless/worker/native-pilot/instagram-native-session.test.mjs internal/collection/headless/worker/native-pilot/instagram-native-provider.test.mjs
```

These fixtures verify bounded rejection projection, role propagation,
account/session matching, document continuity and private diagnostics. They do
not show that the candidate request works against Instagram or that the live
capture failure has been fixed.

Further acquisition changes require a separately authorized native observation
using the categorized diagnostics. Record only the closed role/category and
existing operational outcome; do not collect raw responses, account values or
credentials. An HTTP rejection alone would still not prove which header, if
any, is required. Any resulting recipe change needs evidence for that specific
contract before implementation.

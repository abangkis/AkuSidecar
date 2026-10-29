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

## Exact HTTP rejection diagnostics (offline follow-up)

The later closed `reader_identity_http_status` observation narrows the failure
to a rejected response status outside HTTP 200, 401, 403, 429 and 500–599 under
the previous classifier. It does not identify the exact status, demonstrate a
missing header, or establish a replacement endpoint contract. The status check
runs before response URL, headers, body, account projection or session cookies.

The diagnostic now preserves exact integer statuses from 100 through 599,
excluding successful 200, as `http_status_NNN`. Familiar 401, 403 and 429 retain
`http_unauthorized`, `http_forbidden` and `http_rate_limited`; these aliases each
identify one exact status. All other statuses in that finite interval, including
unassigned values and 5xx, receive the exact numeric category. Invalid or
out-of-range values produce the fixed `http_status` fallback without coercion or
interpolation. The legacy `http_server` category remains accepted across the
wire for compatibility, but new observations produce exact 5xx categories.

The four-field evidence projection is unchanged. JavaScript and Go enforce the
same finite allowlist contract; every HTTP rejection remains terminal. Only the
existing identity `fetch_timeout` category is transient. Synthetic response 302
coverage tests this classifier only: a real redirect with `redirect: 'error'`
can reject fetch and yield `fetch_failed`, without exposing a response status.

Focused validation passed: 34 session/provider Node tests, plus
`TestSourceNativeFailureRetainsSafeFirstCause` and
`TestSourceNativeHTTPStatusContract` with the `auth_session_dev` build tag and
offline Go dependencies. Exhaustive HTTP fixtures include 400, 404, 405 and the
100/599 boundaries, poison URL/header/body access, verify zero cookie reads and
role propagation, and reject forged numeric suffixes. Go tests exhaust the same
status interval through wire projection and confirm terminal retry classification.

No live Instagram request was made or fixed by this change. The next authorized
observation can report a more precise bounded code; it still cannot by itself
prove which request header or endpoint would resolve the rejection.

# Instagram reader-only diagnostic

This operation supplies diagnostic evidence for the rejected Instagram identity
request. It supplies no identity proof, Store permit, source permission, or
capture result, including when HTTP 200 is observed. Implementation and offline
tests do not authorize activation or an authenticated observation.

## Invocation and admission

The private main-browser transport accepts exactly
`{id, op: "main_instagram_reader_diagnostic", nonce}`. The nonce is 48 lowercase
hex characters. The optional Go `MainBrowserInstagramDiagnosticProtocol` exposes
`DiagnoseInstagramReader(ctx, nonce)`; it does not change existing interfaces.
The development-only operator endpoint is `POST /api/diagnostics/instagram-reader`.
It requires the current `X-Aku-Split-Epoch` header and exactly this consent body:

```json
{"allowAuthenticatedReaderDiagnostic":true,"acknowledgeBoundedErrorBody":true}
```

Unknown or duplicate fields, trailing JSON, query parameters, missing consent,
and stale epochs are rejected before dispatch. The server generates the nonce;
the caller cannot provide one. There is no UI button, scheduler hook, automatic
invocation, browser launcher, or profile selection in this operator path.

The endpoint is unavailable outside Windows `auth_session_dev`, development
deployment, split capture, the retained main-profile configuration, and the IG
trial configuration. Both X and IG maintenance holds must already be in force.
The engine requires an idle runtime, no pending worker/source startup or cleanup,
and no collection transition. Its existing operation mutex excludes normal
update, calibration, recapture and settings admission while the diagnostic runs;
an additional shared admission lock rejects X/IG startup before it can release
a maintenance hold, save permission or invoke a source factory. The Node lane
independently excludes public and source operations. Busy admission
fails immediately and does not pause or disable Auto Update.

Composition binds only the already-running capture window's optional diagnostic
protocol. A direct manager lease pins its existing PID and generation; it does
not borrow coordinator interaction intent or reopen a browser. Each process
admits at most one attempt, including a failed result or uncertain cleanup.
Rejected preconditions do not consume that attempt. Once admitted, a client
disconnect cannot skip cleanup: the call keeps a separate thirty-second bound.
The diagnostic is counted as an active cleanup worker. Shutdown closes admission
and waits for that diagnostic before retiring the browser owner; an expired
drain budget is reported as unsettled rather than successful cleanup.
The existing strict Go projection is checked again at the operator boundary,
and the response is not cached. No raw browser error is returned or logged.

The entry injects a sealed diagnostic function. Callers cannot choose an
expression, URL, headers, request parameters, profile, target, or CDP method.
The operation requires no public or source-owner calls in flight and holds its
own exclusive lane until the diagnostic and cleanup settle. It neither starts
nor revokes X/IG permission and does not release a maintenance hold.

## Request and data limits

One newly owned background target navigates once to Instagram Home. The probe
pins its main frame, loader, and default execution context, uses the unique
context ID, and attempts the same checks after evaluation even on rejection.
Network request/response metadata from that owned session independently records
whether exactly one matching GET and response were observed. `documentBound`
and `wireMatched` are separate facts, not account authorization.

The request remains `GET /api/v1/accounts/current_user/?edit=true` with
`credentials: "same-origin"`, `cache: "no-store"`, `redirect: "error"`, and only
the original explicit `Accept: application/json` header. The page aborts the
request/body read after four seconds. There is no retry or alternative endpoint.

HTTP 200 returns `unexpected_success` without reading response URL, headers, or
body. For a non-200 same-origin/path response, the page classifies MIME and may
stream at most 4096 bytes of JSON error data. Only exact, fixed error labels
leave the page; unknown strings become `unknown`. Account fields, server text,
challenge URLs, cookie/token values, snippets, and raw body are never returned
or logged. This is bounded body inspection inside the page, not a claim that no
body is read. The browser still attaches applicable session credentials to a
same-origin request in the usual way.

The exact server label `invalid_app_id` is a separate request-error category.
It reports that literal observation only; it does not establish a missing
header, select a replacement value, or authorize changing the request. Other
unrecognized server strings remain `unknown`.

The final projection is at most 1024 bytes and has exactly nine fields:
`schema`, `primaryCause`, `status`, `mime`, `bodyClass`, `errorCategory`,
`documentBound`, `wireMatched`, and `cleanup`. Node and Go validate all fields
against closed schemas. Diagnostic parsing never changes the production
identity parser or relaxes its proof requirements.

## Timeout and ownership

Setup has a ten-second budget, evaluation at most six seconds, the final frame
check one second, and cleanup a separate three seconds. A caller should allow
at least twenty seconds plus scheduling margin. The first operational cause is
retained independently of cleanup status.

Only the sealed diagnostic scope has request-local CDP timeouts. Ordinary CDP
calls retain their existing fail-closed policy. A late resource-allocation reply
is adopted and retired only by its own scope; missing acknowledgements remain
uncertain. Pins are released only after a valid inventory proves target absence.
Malformed inventories cannot establish cleanup. Uncertain cleanup blocks another
diagnostic in that transport, while X and ordinary lanes remain available.
Underlying shared pipe/protocol failure remains a shared ownership failure.

The probe never launches a browser/profile/collector or calls source lifecycle,
cookie, account-verifier, or capture operations. Raw CDP envelopes already stay
inside the private Node transport; the probe does not export them or subscribe
to extra-info cookie/header events. It closes only its own temporary target.

## Offline acceptance and later gates

Offline fixtures cover request options, four-second abort, 4096/4097-byte and
chunked bounds, unknown/private errors, successful responses with poisoned body
accessors, before/after document/context checks, real restricted CDP scope,
request-local timeout and late reply handling, strict inventories, source-lane
exclusion in both directions, fixed Go/Node projections, and peer continuity.

A later approved build/activation must include the complete packaged transport.
The operator wiring has only been exercised with offline callbacks and fixtures.
An authenticated one-shot observation needs separate permission. Until then,
the observed production HTTP 400 has no newly established server-side cause.

## First operator observation (2026-10-30)

The approved main-dev candidate with binary SHA-256 prefix `c095acec` was
activated through the graceful workflow with native derivation held. One
authorized reader-only diagnostic returned `primaryCause: document_changed`,
`status: 0`, `bodyClass: not_read`, `documentBound: false`,
`wireMatched: false`, and `cleanup: verified`. No collector or session transfer
was run. The capture owner remained ready with zero leases; saved settings and
Auto Update remained unchanged.

This result supplies no new evidence about the earlier HTTP 400. No valid
response projection was retained. `wireMatched: false` is not a request-dispatch
counter and cannot prove that the endpoint was never contacted. Offline review
must distinguish initial navigation readiness from the frozen document binding
used during request evaluation; the result alone does not identify which
navigation event occurred. A second authenticated observation is not authorized
by this one-shot result.

Offline fixtures subsequently reproduced premature comparison against the
navigation loader before its commit, and dispatch after a context invalidation
during instrumentation. Preparation now waits within its existing budget for
the originally requested frame and loader, confirms exact Home scope and the
default unique context across readiness checks, and only then freezes binding.
The frozen binding is checked again immediately before endpoint evaluation.
Foreign scope, a changed frozen document or context, and setup timeout still
reject; preparation never adopts an arbitrary replacement document. This is a
diagnostic correction, not a change to native account-verification policy or
evidence that either fixture ordering caused the live result.

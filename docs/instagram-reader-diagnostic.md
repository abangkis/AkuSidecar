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
There is no HTTP endpoint, UI action, scheduler hook, command-line probe, or
automatic invocation in this increment.

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
An authenticated one-shot observation needs separate permission. Until then,
the observed production HTTP 400 has no newly established server-side cause.

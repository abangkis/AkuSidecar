# Instagram bootstrap corroboration diagnostic

Status: the bounded reader diagnostic passed on source `6c0c3a5` on
2026-10-31: HTTP 200, `corroborated`, all six evidence checks passed, and
cleanup verified in 2877 ms. No collector or session transfer was involved;
native qualification remains false. Shared private acquisition now also powers
daily proofs in the isolated implementation; offline validation passed and
runtime activation remains pending. See the
[daily-proof boundary and native trial plan](instagram-bootstrap-daily-proof.md).

## Boundary

The earlier passive v2 probe found one syntactically valid string ID after a
complete bounded scan, without username, duplicates or conflicts. It did not
establish authenticated-self semantics or fresh server delivery. It remains
available unchanged; this diagnostic is a separate, explicitly consented lane.

The new lane compares three privately held inputs: an exact candidate from the
delivered Home response, the candidate in its pinned document, and the
name-selected `ds_user_id` cookie. Only closed categories and boolean observations
may cross into Go or HTTP. Corroboration is evidence for source qualification,
not production account proof. `qualified` is always false.

## Acquisition and limits

The Node adapter owns a dedicated diagnostic target and session. Instrumentation
must precede ordinary navigation to the exact Instagram Home URL. Request,
response, main frame, loader and default context must correlate. A successful
HTTP response without cache/service-worker involvement is necessary; replaying
an intercepted response does not create fresh server provenance.

The response is read through a response-stage Fetch stream in bounded IO chunks.
The HTML ceiling is 4 MiB and chunks are at most 64 KiB. The ceiling keeps a
base64 replay below the existing 8 MiB outbound CDP message cap; shared transport
limits are not enlarged. A response too large or an unavailable streaming
capability rejects. No Content-Length assertion replaces counted actual bytes.
The bounded body must be replayed only to the owned target after streaming;
failed or uncertain acquisition never changes a user-owned target.
Acquisition is limited to 24 seconds, stream retirement to one further second,
and target cleanup to three seconds, inside the existing 30-second Go deadline.
The remaining two seconds cover serialization and draining. Cleanup time is
reserved rather than added after the operation consumes the entire RPC budget.

The private response parser uses inert HTML parsing and the same strict JSON
candidate scanner as passive v2. It never executes a response script merely to
parse identity. A complete unique body candidate must equal the complete unique
candidate in the pinned document. Missing, malformed, duplicate or conflicting
candidates, changed documents, login/challenge, stale delivery and disagreement
are rejecting observations. Freshness uses trusted response observation time,
not the time at which old DOM content was read.

Cookie comparison uses only a name-selective Cookie Store accessor for
`ds_user_id`. If unavailable, the result is `cookie_unsupported`. There is no
fallback to `document.cookie`, `Network.getCookies`, `Storage.getCookies`, or
the production session-cookie manifest. This lane does not obtain `sessionid`
or `csrftoken` from a cookie jar, create a collector, or seed a session.

All raw response/CDP material remains private and temporary. Diagnostics never
persist or return raw HTML, account ID, cookie/token values, browser-owned
identifiers, arbitrary exception text, or headers. Cleanup is independently
reported; a cleanup failure cannot replace an earlier rejection reason.
CDP response events can inherently contain cookie-bearing HTTP headers. The
private replay preserves origin response headers; these are not a cookie-jar
read, are not used as account evidence, and never enter the public projection.
Do not describe this protocol as receiving no credential-bearing bytes.

## Sealed composition

The control operation is `main_instagram_bootstrap_corroboration`, with a
server-generated nonce and no caller-selected CDP method, expression, target,
cookie name, URL or timestamps. Its optional Go protocol method and owner
binding reuse the running persistent browser Manager lease and generation.

`POST /api/diagnostics/instagram-bootstrap-corroboration` requires the exact
current epoch and three true consent fields:

```json
{
  "allowBootstrapCorroboration": true,
  "acknowledgeBoundedDocumentBody": true,
  "acknowledgeAccountCookieComparison": true
}
```

It shares the existing development-only diagnostic admission barrier: idle
runtime, both native sources in maintenance hold, no active source owner, one
attempt shared with the existing reader/bootstrap diagnostics, 30-second overall
operation context, and cleanup/drain before browser retirement. It does not
alter saved permission or scheduling.

The maximum 1 KiB result is an exact record:
`schema`, `reason`, `bodyBound`, `documentBound`, `serverFresh`, `idMatched`,
`cookieMatched`, `scanComplete`, `qualified`, and `cleanup`.
Schema is `aku.instagram-bootstrap-corroboration.v1`. A `corroborated` result
requires all six observation booleans true, verified cleanup, and
`qualified=false`. Failure observations and cleanup are separate. Both Node
and Go reject extra, missing, duplicate, incorrectly typed or private fields.

## Historical initial live gate and current successor

The initial implementation admitted only offline synthetic tests. Live execution required
explicit approval for the dedicated Home navigation, bounded response-body
read and private account-cookie comparison. Offline success does not establish
availability of Cookie Store/streaming APIs in the deployed browser, real Home
body size, the meaning of `PolarisViewer.id`, or native collector compatibility.
Do not mark a recipe qualified from this diagnostic alone. The separately
approved daily-proof increment connects private acquisition to
`readInstagramEvidence` under the existing trial gate, retaining its
account/session HMAC, document binding and `verifyInstagramPair`. It does not
promote the public diagnostic result to authority or change Store. Collector
and repeated-capture acceptance remain pending under the linked trial plan.

Protocol reference: Chromium's official [Fetch domain](https://raw.githubusercontent.com/ChromeDevTools/devtools-protocol/master/pdl/domains/Fetch.pdl)
requires cancellation or a supplied body after taking the response stream;
the official [IO domain](https://raw.githubusercontent.com/ChromeDevTools/devtools-protocol/master/pdl/domains/IO.pdl)
defines bounded sequential reads and stream closure.

## Offline validation

The Node implementation exercised 87 distinct tests across the new combined
adapter/body-stream suites and the existing passive bootstrap, diagnostic scope,
reader diagnostic and shared transport regressions. The final focused new
suites passed 20/20 after the stream-cleanup, malformed-success and total-budget
corrections. Two package-builder tests also passed, including sealed inclusion
of every new executable module.

Targeted tagged Go tests passed in appshell, Engine, HTTP API and the main
composition package. Default-build tests passed in appshell, HTTP API and main,
including the new endpoint's unavailable path. Fixtures cover exact consent,
epoch/query rejection, shared one-shot admission, an existing owner lease,
state preservation, private-output rejection, strict projection validation and
trailing-slash routing. These use synthetic owners/databases only.

Node tests exercise the actual adapter, stream reader, restricted scope and
page expression with simulated CDP and DOM surfaces. They do not establish real
Chrome template parsing, Fetch/ExtraInfo timing, Cookie Store visibility,
Instagram body size, native qualification or successful capture. No browser,
real account, profile, live endpoint or session transfer was accessed. No
runtime, login, configuration, Auto Update or Store policy was changed.

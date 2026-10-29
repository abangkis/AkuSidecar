# X startup identity/settings gap: offline findings

29 October 2026. Offline reproduction only; no Chrome, account access, session
transfer, user database, runtime or Auto Update changes.

## Observed runtime boundary

The previously inspected startup failed after 4,575 ms with 13,424 ms remaining
in its 18-second envelope. Reader progress was `settings_wait` with
`request_before_identity`. Collector reached `rendezvous` and waited about
3,005 ms. The pair deadline and subsequent scope loss are not proof of an HTTP
settings failure or exhaustion of the overall startup budget.

## Reproduced contract mismatch

`EarlyBootstrapCapture` can stop as `captured` once a bootstrap object contains a
stable string ID candidate. `QABoundObservation` requires the stricter private
identity projection before accepting account evidence or entering the paired
identity rendezvous. These completion conditions are different.

A synthetic bootstrap containing only `session.user_id_str` reproduces early
capture success while strict identity remains absent. The reader can proceed
to the settings drain without joining its waiting collector. No account or
login receipt is produced; the peer eventually times out. The normal-breakpoint
handoff test now reproduces this sequence using the real projection and capture
classes. This demonstrates a mechanism consistent with the live progress; it
does not establish which actual bootstrap field was absent or rejected.

A separate regression shows that a request beginning before strict identity
cannot become eligible retroactively when identity later arrives, even if that
request completes successfully. A later eligible request is required by the
current contract. This is a second ordering condition, distinct from the
missing rendezvous participant.

## Follow-up implementation proposal

Make the native startup composition distinguish a captured candidate from an
accepted bound identity before entering settings/rendezvous. Report the failed
identity prerequisite with closed, value-free diagnostics. Preserve the generic
bootstrap probe's historical behavior, strict account validation and the rule
against retroactive request selection. If settings timing still prevents an
eligible observation, review a deliberate same-document acquisition separately
rather than extending deadlines or silently accepting the earlier request.

This turn adds reproductions and diagnostics only; it does not implement that
X startup behavior change or change the terminal batch-failure policy.

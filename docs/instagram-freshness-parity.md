# Instagram read-only freshness parity (B4)

The real Instagram adapter now provides `instagram-headless-current-v1` in the
explicit `settled_current` mode. Windowed and Headless consume the same source
strategy and its 3,500 ms observation interval. Headless has no tab to wake; it
observes the already owned source page without focus, navigation, reload, scroll,
pending-content activation, network requests, or additional account probes.

## Proof and failure boundaries

- The adapter accepts only the exact HTTPS `www.instagram.com/` feed route.
  Query and fragment are allowed but must remain exactly unchanged throughout
  observation. Credentials, explicit ports, alternate hosts, encoded/dot path
  aliases, control characters and backslashes cannot establish route proof.
- Every primary post must have a matching native `instagram:p`, `instagram:reel`
  or `instagram:tv` ID and canonical permalink. Quote data cannot supply identity.
  Missing, conflicting, duplicate, empty or incomplete samples remain unverified.
- The controller requires a ready document, the actual shared source probe,
  repeated route checks and owned-primary collection within the capture deadline.
  It reserves capture headroom and refuses a budget shorter than the full settling
  interval. Short budgets remain unavailable rather than shortening the proof.
- Naturally changed primary evidence yields `verified`; a complete unchanged
  interval yields `checked_no_pending` with `adapter_wake_contract` evidence.
  These outcomes establish only this bounded source observation, not a global
  newest-post guarantee or native/account admission.
- Pending content remains `freshness_unavailable`: the adapter supports no reveal
  in either consumer. Login, challenge, route and source-availability failures
  retain their blocking errors. No account probe is added.
- Follow-up rounds, explicit targets, Quiet backend pages and preserve policies
  cannot consume this mode to alter a retained frontier. Native Instagram keeps
  its independent feed/round-one, account, bootstrap and document proof gates.

## Offline evidence

`instagram-capability-parity.test.mjs` runs the actual Windowed freshness
controller and Headless controller on equivalent real-adapter fixtures: active
feed, background settled feed, pending control and continuation. The Headless
cases use the actual 3,500 ms contract, with zero clicks or network effects.

`instagram-headless-current.test.mjs` additionally checks strict route/primary
identity rules, natural owned-primary change, absent/invalid proof, typed access
failures, insufficient budgets, explicit targets, preserve policies and borrowed
pages. `source-freshness-integration.test.mjs` exercises actual capture composition.

The worker regression suite passes 351 tests (5 optional/environment cases are
skipped). The native Instagram session/provider/owned-bootstrap suites pass all
153 tests. Shared caption expansion has separate paired real-DOM coverage.
These are offline results; this document does not claim live/native qualification.

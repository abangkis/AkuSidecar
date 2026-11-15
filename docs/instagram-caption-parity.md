# Instagram primary-caption expansion parity

The no-wrapper collapse gap is corrected through the Instagram adapter's shared
caption-root, control and owner contract. Windowed's existing shared primitive
and Headless's collector both consume it. The independent Instagram ancestor
search in the collector is removed.

Supported caption roots include a primary `span[dir="auto"]` directly in its
article, a wrapped caption, and a text-equivalent wrapper containing a sibling
control. The entire article is never a fallback expansion root. Explicit comment
roots and nested articles cannot supply the primary caption. Multiple matching
controls remain partial without a click. Owned labels are removed without erasing
an authored trailing word when the actual control is separately observed.

Windowed pins the source's unique native primary identity before and after the
click through the shared primitive. Headless retains its outer exact-post,
route, deadline, mutation-authority and exact-control guards while operating on
the shared selected root. No account, document, profile or admission proof is
weakened, and no recovery permission is inferred from a diagnosis.

The Bridge runtime/adapter contracts are advanced to `source-adapters-v125` and
`instagram-dom-v7`, so reinjection cannot silently reuse the older expansion
primitive. Existing extraction formats and X/LinkedIn expansion remain covered.

Offline DOM verification uses actual source assets and Windowed's actual
`expandSourceContent` function against the same fixture as Headless. Eleven cases
cover direct/wrapped detect-only, allowed direct/wrapped/sibling expansion,
comment exclusion, ambiguous controls, deadline expiry, unresponsive controls,
changed primary identity and changed route. Chrome runs with a disposable
project profile, no user session and blocked network. All cases pass.

Regression evidence: AkuBridge 638 passed; worker 343 passed with 5 opt-in skips;
the new Chrome DOM test was separately run and passed. This is offline delivery
evidence, not activation or live Instagram qualification.

Run the real-DOM verifier with `AKU_OFFLINE_CHROME` set to the installed Chrome
executable and `node --test internal/collection/headless/worker/test/instagram-caption-expansion.test.mjs`.
Fixtures/results are under `build/instagram-caption-parity`; disposable browser
profiles are removed after execution.

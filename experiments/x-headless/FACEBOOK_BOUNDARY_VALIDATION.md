# Facebook PoC ownership and fragment fix

Validated 2026-10-01. This change affects only the existing experimental package.
Production AkuBridge/Sidecar source and settings were not changed, and observations
were not imported into Sidecar.

## Change

`facebook-boundary.js` provides a stable scoped view over real DOM nodes. A single
primary article inside a position wrapper is represented once. Queries exclude
other post boundaries, while media-only `role=article` elements stay inside their
own post. Unknown article structures stay separated. Header/body text, media and
timestamp queries use this ownership boundary; clicks and geometry still operate
on real owned elements. Same-ID records with conflicting author/text are rejected
rather than choosing the first or longest record.

The reused adapter's canonical permalink query is additionally restricted to
explicit nonempty href values. Fragment-only and query-only links cannot inherit
identity from the active document URL. Bounded headless hover observes header
links even when their initial resolved URL already looks like a native permalink;
the collector records whether the permalink changes. Body links are not hover
targets. These changes are PoC-only and do not modify the shared adapter file.

The wrapper can reuse an existing comparison report's exact targets via
`--comparison-baseline`. It validates native URLs/IDs and confines that input to
the package's existing `runs/` directory. Target selection occurs before stopping
Sidecar, and Inbox is rechecked immediately before shutdown.

## Evidence and diagnosis

The final same-target report is
`runs/2026-10-01T07-00-25-823Z-37724-facebook-comparison/capture.json`, SHA-256
`793d92bbf73025cf0ae2f7ab72e68b3d953754f039f3bb665713093b8dc78e9c`.
Its receipt is `runs/shared-profile-2026-10-01T07-00-24-184Z.json`.

All three background-post containers have a fragment link that resolves to the
current detail page path, and no explicit native post anchor. The old adapter
normalizes the resolved URL, removes its fragment and treats it as `post_anchor`.
This explains why the same unrelated background body appeared under three target
IDs. The PoC now rejects that container as `missing_identity` in each case.

The initial stricter role-based boundary was too broad: Facebook uses role=article
for media structures as well. It also retained the fragment identity problem.
Those failed intermediate reports remain saved as negative evidence:
`runs/2026-10-01T06-48-56-402Z-23456-facebook-comparison/capture.json` and
`runs/2026-10-01T06-55-27-431Z-23432-facebook-comparison/capture.json`.
Every run restored healthy Sidecar and a compatible Bridge.

## Final live result

| Selected target | PoC author/text equal the saved baseline | Current observed media | Exact baseline ID verified |
| --- | --- | --- | --- |
| Long text | 746 characters, exact text | 1 image | Yes |
| Multi-image | 988 characters, exact text | 2 images | No: a different native ID is exposed |
| Video candidate | 54 characters, exact text | 1 image; stream unresolved | No: a different native ID is exposed |

There is one accepted PoC record per live case, no conflicting same-ID PoC records,
and zero foreign authors attached to the requested IDs. All live cases retained
signed-in UI evidence. Matching author/text does **not** establish ID equivalence:
the two different native IDs remain unverified aliases and are not rewritten to
the requested IDs. Native URL/Story alias verification is separate remaining work.

The production extractor run in the harness still returns the background record
under the requested ID, both before and after PoC hydration. The production code
was intentionally unchanged. There are therefore zero valid production/PoC pairs
in either pass. The report's `empty_unverified` status and live command exit code 2
describe unresolved paired comparison; they do not mean that the PoC captured no
posts or that runtime restoration failed. The saved verifier confirms three
author/text observations, one exact-ID target, two unresolved native-ID differences,
zero ambiguous PoC IDs, zero foreign-author target records, and healthy restoration.

This remains an extractor comparison on the same headless DOM. Installed Bridge,
headed/headless runtime parity, full video acquisition, shared-post live behavior,
feed completeness and zero OS blinking are not established.

## Checks

- `node facebook-smoke.mjs`: passed in real headless Chrome. Includes broad-container
  ownership, duplicate identity rejection, embedded body/media exclusion, known-link
  hover hydration, role=article media preservation, fragment identity rejection,
  and prior timestamp/expansion/admission/login checks.
- `node facebook-comparison-smoke.mjs`: passed. Production text cap, exact multi-image
  URL parity, synthetic structured/deferred video enrichment, shared-post limitation
  and foreign same-ID rejection remain verified.
- `node smoke.mjs`: X regression passed after loader changes. Subsequent edits were
  Facebook-only; that unchanged X evidence was reused.
- Saved final live report/receipt verification passed. Production Bridge working tree
  remained clean, and live Bridge health readback was `healthy`, `compatible:true`.

Reproduce the exact cases from this package:

```powershell
node shared-profile-test.mjs --source facebook --facebook-compare --comparison-baseline runs/2026-10-01T06-21-59-671Z-29440-facebook-comparison/capture.json --wait-ms 30000 --max-scrolls 0 --max-posts 20
node facebook-comparison-verify.mjs runs/2026-10-01T07-00-25-823Z-37724-facebook-comparison/capture.json runs/shared-profile-2026-10-01T07-00-24-184Z.json
```

The first command briefly closes registered Sidecar/UI/capture and restores it;
the second only reads saved artifacts. Before migration, verify explicit native
alias bindings and resolve the production fragment identity bug through a separately
authorized production change, then repeat extraction and runtime comparisons.

# SharedDOMEngine stage 04 verification

X primary identity resolution migrated to SharedDOMEngine in AkuBridge
`dd9e379` and in the AkuSidecar Headless collector in `fe75348`. Both live
trials used AkuSidecar `3a1e033`. That build also fixes a downstream selection
issue where a model's event label could collide with an unrelated historical
label and prevent a new post from reaching Timeline.

The offline Chromium comparison passed 19 fixtures through the Windowed and
Headless loaders. It covered legacy timestamps, wrapped status links without
timestamps, mixed markup, owner and quote isolation, media-only posts,
placeholders, hydration, and conflicting identities. The loaders used the same
browser artifact SHA-256
`8b5b434569a86015e3a48525885a0fa303757dd7ccae9606ce8816926fd5197b`.
The 38 shadow evaluations recorded zero DOM mutations. The worker and Go
regression suites passed the targeted route, deadline, lease, continuation
anchor, dedup, and Timeline selection checks.

| Live X run | Collector | Stored observations | Unique candidates | Evaluated | Added to Timeline |
| --- | --- | ---: | ---: | ---: | ---: |
| `run_7a739267647cb1eea41caf499eda9de5` | Windowed via AkuBridge | 1 | 3 | 2 | 2 |
| `run_b1be9f8744415b90de279a957cbebd61` | Headless | 2 | 6 | 6 | 3 |

Both runs completed and returned HTTP 200 for the X document. A read-only
database check matched all five Timeline items to captured posts whose
permalink came from the wrapper and whose native timestamp was unavailable.
Windowed skipped one unchanged post during the seven-day cooldown. Headless
merged 15 snapshot occurrences into six unique candidates and evaluated each
once. Its one resurfaced candidate had changed, so the unchanged-post cooldown
did not trigger in that run. Each of the three Headless additions reused a model
label already associated with a different historical post and was stored under
its own source evidence identity.

Both trials kept Native X and Native Instagram disabled and used the existing
database. The Headless trial restored effective Windowed mode and the original
source and scheduler settings. This evidence covers the X pilot in Stage 04.
Additional sources and paired release qualification have their own roadmap
criteria.

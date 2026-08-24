# Semantic replay assessment

`cmd/semantic-replay` is a bounded, local assessment tool for the semantic
event resolver. It reads the existing SQLite ledger in read-only mode and
prints aggregate evidence for a future fail-closed optimization review.

Build and run it from the AkuSidecar repository with an explicit database
path. Keep the generated executable in the workspace SharedTemp so local
antivirus handling remains predictable:

```powershell
$sharedTemp = Resolve-Path '..\..\SharedTemp'
$replayDir = Join-Path $sharedTemp 'semantic-replay'
New-Item -ItemType Directory -Force -Path $replayDir | Out-Null
$executable = Join-Path $replayDir 'semantic-replay.exe'
go build -o $executable ./cmd/semantic-replay
& $executable -database .\runtime\aku-sidecar.db -limit 100
```

The tool does not load configuration, construct a reasoning provider, invoke
Codex/Ollama, start AkuSidecar, or write the database. `-limit` is bounded to
1–500 most recent completed sessions. The semantic-event corpus is bounded
internally to the 1,000 most recently seen events and each session to 30
reports.

The JSON report contains only aggregate counts: session and invocation
statuses, semantic relations, trigger overlap and trigger-rarity buckets,
active/undone correction counts, observed local bypasses, and
`counterfactualReviewCandidateSessions` versus `requiresModelSessions`.
For sessions processed with schema v10 or later, `signalReceipt` also
aggregates a privacy-safe pre-resolution receipt captured at shortlist
construction: candidate and shortlist counts, overlap, trigger rarity
counts, actor/object/event-key overlap counts, and objective nearest-event
time-distance buckets (within 24 hours, within 7 days outside the first
bucket, beyond 7 days, or unavailable). The receipt
contains only bounded integers and its version; it never stores the values
used to derive those counts.
Historical actor, object, time, and exact event-key compatibility are
explicitly reported as unavailable because the assigned event is post-
resolution state, not the original historical shortlist. The report never
emits raw candidate text, URLs, author names, canonical claims, evidence keys,
prompts, or model explanations.

`counterfactualReviewCandidateSessions` identifies completed model-invoked
sessions whose retained reports are all `new_event` and have no active or
undone correction. It is a subset of `requiresModelSessions` under the current
runtime policy, not a disjoint bucket and not a safe-to-skip decision.
`observedLocalBypassSessions` records completed local-index or explicit
bypassed sessions where `resolverInvoked` is false; it is not an inferred
safety result. `requiresModelSessions` and the review-candidate counts include
only sessions whose resolver was actually invoked. The analyzer cannot reproduce the original
prompt because the exact historical shortlist is not persisted. Therefore
this report cannot change runtime policy or prove that a future local gate is
safe; use it to design and review a separate gate, then validate that gate with
replay fixtures and live canaries.

Rows created before the schema-v10 migration have no receipt and remain
`legacyAllNewReviewCandidateSessions` when they meet the conservative
all-`new_event` review condition. A read-only replay of an existing v9
database can still analyze those rows, but it cannot reconstruct or backfill
pre-resolution evidence. Receipt-backed evidence therefore begins only for
sessions processed after the runtime opens and upgrades the database to v10.
`receiptBackedReviewCandidateSessions` and the legacy count are subcounts of
`counterfactualReviewCandidateSessions`; both are also included in
`requiresModelSessions` under the current runtime policy.

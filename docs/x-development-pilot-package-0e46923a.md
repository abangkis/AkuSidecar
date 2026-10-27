# Prepared X development activation package (2026-10-27)

Package: `0e46923af815440b8af7d6e63ffec576`.
Root: `build/x-application-qa-prepared/0e46923af815440b8af7d6e63ffec576/`.
Manifest SHA-256: `d933e6da0bc8c508054584bf8720806cda490bbf0a09a12aed9195a5fbe8f8e5`.
Isolated port: `55970` (not reserved).

## Corrective delta and provenance

The prior `fd755aa1904d4f2e80a6c9d25c249b93` package failed startup when provider
construction could not read an acquisition-plan schema. The handoff was rolled
back. That package and its operational receipts remain unchanged; its historical
manifest digest is still
`8d0dbac83dda29c4f1a8a2cef550dfdadf9456eb96514802e158613bf367d77e`.

This fresh package includes all six required startup schemas under the isolated
instance root derived from its config path. Preparation copies source bytes
unchanged and pins them. Verification requires the six assets independently of
the manifest inventory, checking inventory membership, hashes, JSON syntax and
root contracts. This is not full recursive JSON Schema validation. Schemas are
included in the clean compiled-source gate and scoped source digest.

The binary source revision is `0e2ea21fe1f0a0f810f35c6c2519d6ec66ec2549`, with
`auth_session_dev` only. The preparation scripts and runbook contain the local,
uncommitted correction, individually pinned by the package. Unrelated checkout
changes remain preserved. A commit ID alone does not identify the full package;
use its manifest hash, payload hashes and scoped source digest.

## Validation and limits

- All 26 accountless package regressions passed, including omission of each schema
  from both disk and inventory, wrong-root placement, malformed JSON, invalid
  root contracts and mismatched pins. The catalog is checked against startup
  provider/resolver source references.
- Both the source verifier and independent packaged verifier passed: 337 payload
  files, six runtime schemas and the pinned external capture Chrome.
- The preparation-only plan exit and early config/database-schema probe passed.
  They do not construct providers or demonstrate ordinary startup success.
- The fresh database is absent. No browser, retained profile, manual `/start`,
  session derivation, inference, registry write or main-dev restart occurred.
- The four on-disk main baseline files remained unchanged. The read-only HKCU
  snapshot was observed. Auto Update and login were not changed.

Reasoning settings are copied from the on-disk config. This selects
`codex-app-server`; it does not import the running main database's persisted
`gemini-flash-lite` preference. Provider/model parity with live main dev is not
claimed, and no inference is authorized by preparation.

Astra accepted the corrective diff without a blocking finding. Its review and
these offline checks do not establish successful runtime activation.

## Next approval boundary

The package remains default-off, `executionApproved:false`,
`activationReady:false`. Before launch, approve the exact new package and a new
native-host/reader-pipe handoff using the
[activation and rollback runbook](x-development-pilot-activation.md). Only
graceful shutdown is authorized; a stop path with forced process-tree fallback
must not be used. Separate one-use authorization remains mandatory for any
`auth_token`/`ct0` transfer. Do not retry or repair a consumed package in place.

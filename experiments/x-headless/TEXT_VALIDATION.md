# Text and quote validation

Validated 2026-10-01 (Asia/Jakarta). Changes are confined to this experimental package.

The collector reuses AkuBridge's X adapter `contentExpansion` policy and `extractQuotedPost`. Its structured text helper follows the content-script rules for paragraph breaks and emoji image alt text. Primary and quoted text have separate collapse/status fields. No collapse control means `visible_text_no_collapse_control`, not a guarantee of source completeness.

Feed Show more controls are deferred as `requires_permalink_capture`. Targeted permalink capture obtains the native detail view; if that view still has an own-post Show more control, a bounded expansion attempt uses the adapter policy. A failed attempt remains `expand_failed` and collapsed. Quote controls are not clicked because they can navigate to a different post; collapsed quotes require their own permalink capture. Quoted media is not collected in this change.

Real headless Chromium fixture passed: successful expansion, preserved paragraphs/emoji, failed expansion retaining partial status, independent collapsed quote status, quote identity/text separation, engagement, rejection, viewport bounds and browser cleanup.

Three previously collapsed feed identities were captured on their permalinks with the registered capture profile and original Chrome executable:

| Case | Before characters | Detail characters | Collapse control remained | Quote body exported |
| --- | ---: | ---: | --- | --- |
| Quote post | 274 | 312 | No | Yes |
| Post 2 | 274 | 721 | No | Not applicable |
| Post 3 | 280 | 382 | No | Not applicable |

All three runs observed authenticated UI with no login requirement/challenge. Saved-report verification also confirmed matching identities and retained opening text. Each wrapper verified profile release, restored Sidecar health and compatible Bridge. These live cases resolved through native detail rendering; they do not prove the inline click path against live X. Local fixtures verify that path and its failed-expansion status, including a quote-only post whose primary text must remain empty.

Private evidence is in ignored `runs/text-validation.json`, corresponding capture reports, and `shared-profile-*.json` lifecycle receipts. Public documentation excludes account identities and post bodies. Reproduce with `node text-test.mjs` (temporarily stops/restores the registered Sidecar); run `node smoke.mjs` for the isolated fixture.

Remaining boundaries: no production integration, no full quoted-media acquisition, no guarantee of complete long-form X articles, and no measured claim of zero operating-system blinking.

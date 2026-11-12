# Versioned Codex models and separate reasoning effort

## Supported selection

AkuBrowser exposes exactly these Codex models, in this UI order:

| Model label | SDK stable model ID | Native App Server model | Profile prefix |
| --- | --- | --- | --- |
| GPT-6.1 Sol | `codex-gpt-6.1-sol` | `gpt-6.1-sol` | `gpt61_sol_` |
| GPT-6.1 Terra | `codex-gpt-6.1-terra` | `gpt-6.1-terra` | `gpt61_terra_` |
| GPT-6 Luna | `codex-gpt-6-luna` | `gpt-6-luna` | `gpt6_luna_` |

The existing Inference SDK v0.28.0 already provides these descriptors. No
dependency upgrade or model alias substitution is needed. The old SDK IDs
`codex-sol`, `codex-terra`, and `codex-luna` still denote GPT-5.6 and are rejected
by the application's new Codex invocation boundary. GPT-6 Sol and Astra are
also excluded. Unsupported explicit IDs cannot fall through to a different
model, and conflicting native/stable model identities fail before transport.

Each model has separate Low, Medium, High, XHigh, and Max options, obtained from
its provider-owned Codex descriptor. The UI groups declared profiles by native
model and composes the selected model/effort back into one opaque profile ID.
It does not build wire model names from display labels. Switching models keeps
the current effort when supported; otherwise the bounded fallback is High.
Changing AI Detection also disables/enables both AI Deep Detection controls.
Other providers retain their existing profile controls.

The public model pages confirm the native slugs:
[Sol](https://developers.openai.com/api/docs/models/gpt-6.1-sol),
[Terra](https://developers.openai.com/api/docs/models/gpt-6.1-terra), and
[Luna](https://developers.openai.com/api/docs/models/gpt-6-luna).
API capabilities are not treated as account-specific Codex availability. For
example, the API pages include `none` for Terra/Luna, while the pinned Codex
descriptors omit it, so the picker does not advertise it. `ultra` is also not
an SDK-supported option. App Server's [model/list contract](https://learn.chatgpt.com/docs/app-server#models)
describes account/client-dependent catalogs; a local descriptor and synthetic
test do not prove that a live account can complete an inference request.

## Persisted settings

The settings API and Store keep their existing four profile fields. The model
and effort controls write one declared profile per workload. All 15 canonical
profiles round-trip through Engine SaveSettings and a temporary SQLite Store.
Unsupported profiles are rejected before persistence.

Startup and provider switching already call `EnsureResolvableProfile`. Its
provider-owned migration hook now performs these bounded translations:

| Previously advertised profile | Canonical profile |
| --- | --- |
| `luna_high`, `luna_xhigh`, `luna_max` | `gpt6_luna_high`, `gpt6_luna_xhigh`, `gpt6_luna_max` |
| `terra_high`, `terra_xhigh` | `gpt61_terra_high`, `gpt61_terra_xhigh` |
| `sol_medium` | `gpt61_sol_medium` |

This preserves family and effort. Unknown profiles use the bounded
`gpt6_luna_high` fallback. New defaults and the four tracked Codex workload
fallbacks use GPT-6 Luna High, preserving the previous workload role. Migration
occurs when the updated source is activated; this implementation/test pass does
not modify the user's current database or provider selection.

## Validation on 2026-11-12

- Full reasoning and domain suites passed. The synthetic transport test
  `TestCodexProfilesResolveBindAndSerializeNativeWire` verifies every one of the
  15 profiles through resolver, binding, `thread/start.model`, and
  `turn/start.model`/`effort`. No real inference is used.
- Full configuration, Engine, HTTP API, and command suites passed. The HTTP
  bootstrap expectation was updated for the new canonical default. Additional
  Engine tests verify persistence, rejected selections, and all six legacy
  migrations through provider-switch composition.
- Frontend tests verify the exact three model labels, hidden profile composition,
  preserved effort, and unknown/unsupported selections. Both default and
  development/fixture builds passed.
- An isolated browser preview uses the actual settings renderer and stylesheet,
  with catalog data exported from the real backend. Browser checks verified
  the exact three choices on all four workloads, effort retention across a
  model change, dirty-state/Save behavior, and AI Deep Detection disabling.
  Desktop and narrow layouts were visually inspected.

Local evidence is retained under `.go-build/codex-models/`, including
`integration-tests.jsonl`, `http-final.jsonl`, `model-settings-final.jsonl`,
`frontend-tests.log`, `browser-fixture.json`, and `settings-desktop.png`.
The initial HTTP default-expectation failure remains in the original log;
`http-final.jsonl` records the corrected full-suite result.

To reproduce the browser fixture, set `AKU_CODEX_MODEL_UI_FIXTURE` to an absolute
project-local `.go-build/codex-models/browser-fixture.json` and run:

```powershell
go test ./internal/engine -run '^TestCodexModelSettingsBrowserFixture$' -count=1
node test/helpers/codex-model-settings-preview.mjs .go-build/codex-models/browser-fixture.json
```

The preview listens only on loopback and never forwards to the live app. Its
Save operation stays in browser memory; durable settings validation is provided
by the Engine/Store integration tests. Build artifacts are separate from the
managed runtime. At this validation checkpoint the changes were local, with no
commit, push, or activation performed.

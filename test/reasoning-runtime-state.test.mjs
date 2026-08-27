import test from "node:test";
import assert from "node:assert/strict";

import { applyReasoningRuntimeResponse } from "../internal/httpapi/web/reasoning-runtime-state.js";

test("provider switch response updates the active provider projection", () => {
  const bootstrap = {
    provider: "codex-app-server",
    settings: { reasoningProvider: "codex-app-server" },
    reasoningRuntime: { provider: "codex-app-server" },
  };

  applyReasoningRuntimeResponse(bootstrap, {
    provider: "gemini-flash-lite",
    settings: { reasoningProvider: "gemini-flash-lite" },
    reasoningRuntime: { provider: "gemini-flash-lite" },
  });

  assert.equal(bootstrap.provider, "gemini-flash-lite");
  assert.equal(bootstrap.settings.reasoningProvider, "gemini-flash-lite");
  assert.equal(bootstrap.reasoningRuntime.provider, "gemini-flash-lite");
});

test("settings provider is a backward-compatible fallback", () => {
  const bootstrap = { provider: "codex-app-server" };
  applyReasoningRuntimeResponse(bootstrap, {
    settings: { reasoningProvider: "gemini-flash-lite" },
  });
  assert.equal(bootstrap.provider, "gemini-flash-lite");
});

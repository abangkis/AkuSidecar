import assert from "node:assert/strict";
import test from "node:test";
import { DeterministicReasoningProvider } from "../src/reasoning/deterministic-provider.mjs";
import { PROVIDER_CAPABILITIES } from "../src/reasoning/provider-capabilities.mjs";
import { runProviderConformance } from "../src/reasoning/provider-conformance.mjs";
import { providerConformanceFixture } from "../test-support/provider-conformance-fixture.mjs";

test("deterministic fallback passes contracts but is not pilot-quality eligible", async () => {
  const result = await runProviderConformance(
    new DeterministicReasoningProvider(),
    providerConformanceFixture(),
  );
  assert.equal(result.passed, true);
  assert.equal(result.pilotQualityEligible, false);
  assert.ok(result.checks.every((entry) => entry.passed));
});

test("provider conformance rejects incomplete candidate coverage", async () => {
  const provider = {
    name: "broken-provider",
    async planAcquisition() { return { decision: "finish", reason: "Fixture." }; },
    async analyze() {
      return {
        summary: "Broken fixture.",
        items: [],
        candidateAssessments: [],
        repeatedClaimsCollapsed: 0,
        deferredByBudget: 0,
        limitations: [],
      };
    },
  };
  const result = await runProviderConformance(provider, providerConformanceFixture(), {
    providerId: "deterministic",
  });
  assert.equal(result.passed, false);
  assert.equal(result.checks.find((entry) => entry.id === "candidate_coverage").passed, false);
});

test("every configured provider has an explicit capability manifest", () => {
  assert.deepEqual(Object.keys(PROVIDER_CAPABILITIES).sort(), ["codex-sdk", "deterministic"]);
  assert.equal(PROVIDER_CAPABILITIES["codex-sdk"].usageTelemetry, "provider_reported");
});

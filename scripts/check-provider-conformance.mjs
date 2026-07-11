import { DeterministicReasoningProvider } from "../src/reasoning/deterministic-provider.mjs";
import { runProviderConformance } from "../src/reasoning/provider-conformance.mjs";
import { providerConformanceFixture } from "../test-support/provider-conformance-fixture.mjs";

const result = await runProviderConformance(
  new DeterministicReasoningProvider(),
  providerConformanceFixture(),
);
console.log(JSON.stringify(result, null, 2));
if (!result.passed) process.exitCode = 1;

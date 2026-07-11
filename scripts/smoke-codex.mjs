import { loadConfig } from "../src/config.mjs";
import { CodexSdkReasoningProvider } from "../src/reasoning/codex-sdk-provider.mjs";
import {
  validateAcquisitionPlan,
  validateBridgeObservation,
  validateReasoningResult,
} from "../src/core/contracts.mjs";

const config = loadConfig({
  ...process.env,
  AKU_REASONING_PROVIDER: "codex-sdk",
});
const provider = new CodexSdkReasoningProvider(config.reasoning);
const run = {
  id: "synthetic-smoke-run",
  mode: "catch_up",
  source: "x",
  intent: "Verify the provider-neutral structured result contract.",
  maxItems: 1,
};
const observation = validateBridgeObservation({
    source: "x",
    pageUrl: "https://x.com/example/status/1",
    pageTitle: "Synthetic Gate 0 fixture",
    capturedAt: new Date().toISOString(),
    snapshots: [
      {
        capturedAt: new Date().toISOString(),
        blocks: [
          {
            text: "Synthetic evidence: Example Tool version 2 was released today with a new read-only export command.",
            author: "Example Tool",
            publishedAt: null,
            permalink: "https://x.com/example/status/1",
            links: [],
          },
        ],
      },
    ],
    coverage: {
      status: "partial",
      candidateCount: 1,
      notes: ["Synthetic fixture; no real browser data."],
    },
}, config.limits);

const planInvocation = await provider.planAcquisition({
    run,
    observation,
    budget: {
      currentRound: 1,
      maxRounds: 2,
      followUpScrolls: 1,
      sourceLocked: "x",
      continuationRequiresAnchor: true,
    },
  });
const plan = validateAcquisitionPlan(planInvocation.output);
const resultInvocation = await provider.analyze({
  run,
  observation,
});

const validated = validateReasoningResult(resultInvocation.output, 1);
console.log(
  JSON.stringify(
    {
      provider: provider.name,
      acquisitionDecision: plan.decision,
      acquisitionPlanSchemaValid: true,
      itemCount: validated.items.length,
      priorities: validated.items.map((item) => item.priority),
      provenanceKinds: validated.items.map((item) => item.sourceUrlKind),
      knowledgeDeltas: validated.items.map((item) => item.knowledgeDelta),
      schemaValid: true,
      model: config.reasoning.model ?? "Codex CLI default",
      planningEffort: config.reasoning.planningEffort,
      evaluationEffort: config.reasoning.evaluationEffort,
      usage: resultInvocation.telemetry,
    },
    null,
    2,
  ),
);

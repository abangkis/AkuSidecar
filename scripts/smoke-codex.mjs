import { loadConfig } from "../src/config.mjs";
import { CodexSdkReasoningProvider } from "../src/reasoning/codex-sdk-provider.mjs";
import { validateReasoningResult } from "../src/core/contracts.mjs";

const config = loadConfig({
  ...process.env,
  AKU_REASONING_PROVIDER: "codex-sdk",
});
const provider = new CodexSdkReasoningProvider(config.reasoning);

const result = await provider.analyze({
  run: {
    id: "synthetic-smoke-run",
    mode: "catch_up",
    source: "x",
    intent: "Verify the provider-neutral structured result contract.",
    maxItems: 1,
  },
  observation: {
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
  },
});

const validated = validateReasoningResult(result, 1);
console.log(
  JSON.stringify(
    {
      provider: provider.name,
      itemCount: validated.items.length,
      priorities: validated.items.map((item) => item.priority),
      schemaValid: true,
    },
    null,
    2,
  ),
);

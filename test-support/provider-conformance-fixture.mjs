export function providerConformanceFixture() {
  return {
    run: {
      id: "conformance-run",
      mode: "catch_up",
      source: "x",
      intent: "Material technical engineering and AI changes.",
      maxItems: 2,
    },
    observation: {
      source: "x",
      pageUrl: "https://x.com/home",
      pageTitle: "Home / X",
      capturedAt: "2026-07-12T00:00:00.000Z",
      snapshots: [{
        index: 0,
        capturedAt: "2026-07-12T00:00:00.000Z",
        scrollY: 0,
        viewportHeight: 800,
        blocks: [
          block("x:111111111111111111111111", "OpenAI released a material Codex engineering update with a new bounded workflow."),
          block("x:222222222222222222222222", "An engineering opinion explains practical tradeoffs in local model serving systems."),
        ],
      }],
      coverage: { status: "partial" },
    },
    knowledgeContext: { checkpoint: null, events: [] },
    budget: { remainingRounds: 1, followUpScrolls: 1 },
  };
}

function block(evidenceKey, text) {
  return {
    evidenceKey,
    text,
    author: "Conformance fixture",
    permalink: `https://x.com/fixture/status/${evidenceKey.slice(-6)}`,
    links: [],
    publishedAt: "2026-07-12T00:00:00.000Z",
  };
}

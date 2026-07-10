import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { JobEngine } from "../src/core/job-engine.mjs";
import { SqliteStateStore } from "../src/store/sqlite-state-store.mjs";

const limits = {
  maxItems: 5,
  maxScrolls: 2,
  maxBlocksPerSnapshot: 20,
  maxBlockCharacters: 4_000,
};

test("Gate 0 survives the browser-to-reasoning-to-SQLite flow and restart", async (context) => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "aku-browser-test-"));
  context.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  const databasePath = path.join(directory, "state.db");
  let store = new SqliteStateStore(databasePath);
  const provider = {
    name: "test-provider",
    async analyze({ run, observation }) {
      return {
        summary: "One bounded observation was classified.",
        items: [
          {
            id: "result-1",
            priority: "P1",
            whatChanged: observation.snapshots[0].blocks[0].text,
            whyItMatters: run.intent,
            source: run.source,
            sourceUrl: observation.snapshots[0].blocks[0].permalink,
            author: "Test author",
            publishedAt: null,
            confidence: 0.9,
            evidenceState: "primary",
          },
        ],
        repeatedClaimsCollapsed: 0,
        deferredByBudget: 0,
        limitations: [],
      };
    },
  };
  const engine = new JobEngine({ store, reasoningProvider: provider, limits });

  const run = engine.startRun({ mode: "catch_up", source: "x", maxItems: 1, scrolls: 0 });
  assert.equal(run.status, "waiting_for_bridge");
  const command = engine.claimBridgeCommand(run.id, "test-bridge");
  assert.equal(command.status, "claimed");
  assert.equal(command.payload.mode, "catch_up");

  engine.acceptBridgeObservation(command.id, run.id, sampleObservation());
  const completed = await engine.waitForRun(run.id);
  assert.equal(completed.status, "completed");
  assert.equal(completed.result.items.length, 1);
  assert.equal(completed.observations.length, 1);
  assert.match(completed.coverage.scopeStatement, /not a claim of complete feed coverage/i);

  store.close();
  store = new SqliteStateStore(databasePath);
  const restored = store.getRun(run.id);
  assert.equal(restored.status, "completed");
  assert.equal(restored.result.items[0].sourceUrl, "https://x.com/example/status/1");
  store.close();
});

test("bridge failures stop at an explicit stage", () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "aku-browser-test-"));
  const store = new SqliteStateStore(path.join(directory, "state.db"));
  try {
    const engine = new JobEngine({
      store,
      reasoningProvider: { name: "unused", analyze: async () => ({}) },
      limits,
    });
    const run = engine.startRun({ source: "linkedin", maxItems: 1, scrolls: 0 });
    const command = engine.claimBridgeCommand(run.id, "test-bridge");
    const failed = engine.failBridgeCommand(command.id, run.id, {
      message: "No signed-in tab was available.",
    });
    assert.equal(failed.status, "failed");
    assert.equal(failed.error.stage, "browser_capture");
  } finally {
    store.close();
    fs.rmSync(directory, { recursive: true, force: true });
  }
});

test("reasoning cannot invent provenance outside the browser observation", async () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "aku-browser-test-"));
  const store = new SqliteStateStore(path.join(directory, "state.db"));
  try {
    const engine = new JobEngine({
      store,
      reasoningProvider: {
        name: "hallucinating-provider",
        async analyze() {
          return {
            summary: "Invalid provenance fixture.",
            items: [
              {
                id: "invented-source",
                priority: "P1",
                whatChanged: "An unsupported claim.",
                whyItMatters: "This should never be accepted.",
                source: "x",
                sourceUrl: "https://example.invalid/not-observed",
                author: "Unknown",
                publishedAt: null,
                confidence: 0.9,
                evidenceState: "unverified",
              },
            ],
            repeatedClaimsCollapsed: 0,
            deferredByBudget: 0,
            limitations: [],
          };
        },
      },
      limits,
      logger: { error() {} },
    });
    const run = engine.startRun({ source: "x", maxItems: 1, scrolls: 0 });
    const command = engine.claimBridgeCommand(run.id, "test-bridge");
    engine.acceptBridgeObservation(command.id, run.id, sampleObservation());
    const failed = await engine.waitForRun(run.id);
    assert.equal(failed.status, "failed");
    assert.equal(failed.error.stage, "reasoning");
    assert.match(failed.error.message, /not present in the browser observation/i);
  } finally {
    store.close();
    fs.rmSync(directory, { recursive: true, force: true });
  }
});

function sampleObservation() {
  return {
    source: "x",
    pageUrl: "https://x.com/home",
    pageTitle: "Home / X",
    capturedAt: "2026-07-10T10:00:00Z",
    snapshots: [
      {
        capturedAt: "2026-07-10T10:00:00Z",
        scrollY: 0,
        viewportHeight: 900,
        blocks: [
          {
            text: "A material technical release was announced with concrete availability details.",
            author: "Test author",
            publishedAt: null,
            permalink: "https://x.com/example/status/1",
            links: [],
          },
        ],
      },
    ],
    coverage: {
      status: "partial",
      checkedThrough: "2026-07-10T10:00:00Z",
      candidateCount: 1,
      notes: ["One visible viewport; no scrolling."],
    },
  };
}

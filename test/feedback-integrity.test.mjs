import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { JobEngine } from "../src/core/job-engine.mjs";
import { SqliteStateStore } from "../src/store/sqlite-state-store.mjs";

const limits = {
  maxItems: 5,
  maxScrolls: 2,
  defaultScrolls: 2,
  maxAcquisitionRounds: 2,
  followUpScrolls: 1,
  maxContinuationAnchors: 3,
  maxKnowledgeContextEvents: 20,
  scrollFraction: 0.75,
  scrollSettleMs: 900,
  captureTimeoutMs: 45_000,
  pendingContentTimeoutMs: 5_000,
  pendingContentSettleMs: 700,
  maxBlocksPerSnapshot: 20,
  maxBlockCharacters: 4_000,
};

test("feedback boundaries are contextual, exclusive, and idempotent", async (context) => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "aku-feedback-test-"));
  context.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  const store = new SqliteStateStore(path.join(directory, "state.db"));
  const engine = new JobEngine({
    store,
    limits,
    reasoningProvider: {
      name: "feedback-test-provider",
      async analyze({ run, observation }) {
        if (run.intent.startsWith("Empty")) {
          return {
            summary: "Nothing material.",
            items: [],
            repeatedClaimsCollapsed: 0,
            deferredByBudget: 0,
            limitations: [],
          };
        }
        const block = observation.snapshots[0].blocks[0];
        return {
          summary: "One material item.",
          items: [
            {
              id: "promoted-item",
              priority: "P1",
              whatChanged: block.text,
              whyItMatters: run.intent,
              source: "x",
              sourceUrl: block.permalink,
              sourceUrlKind: "native_post",
              evidenceKey: block.evidenceKey,
              eventKey: "feedback-fixture",
              knowledgeDelta: "new_event",
              author: "Fixture",
              publishedAt: null,
              confidence: 0.9,
              evidenceState: "source_backed",
            },
          ],
          repeatedClaimsCollapsed: 0,
          deferredByBudget: 0,
          limitations: [],
        };
      },
    },
  });

  const empty = await completeRun(engine, "empty-1", "Empty technical intent");
  assert.throws(
    () => engine.addFeedback(empty.id, { kind: "missed", itemId: "fake", note: "Missing" }),
    /completed empty run without itemId/,
  );
  engine.addFeedback(empty.id, { kind: "correct_empty" });
  engine.addFeedback(empty.id, { kind: "correct_empty" });
  assert.equal(engine.getRun(empty.id).feedback.length, 1);
  assert.throws(
    () => engine.addFeedback(empty.id, { kind: "missed", note: "A release was omitted." }),
    /different verdict/,
  );

  const missed = await completeRun(engine, "empty-2", "Empty technical intent");
  engine.addFeedback(missed.id, { kind: "missed", note: "A release was omitted." });
  assert.equal(
    store.getConfirmedExcludedEvidenceKeys(
      "x",
      "catch_up",
      missed.intent,
      [missed.observations[0].payload.snapshots[0].blocks[0].evidenceKey],
    ).size,
    0,
  );

  const promoted = await completeRun(engine, "promoted-1", "Promoted technical intent");
  assert.throws(
    () => engine.addFeedback(promoted.id, { kind: "correct_empty" }),
    /completed empty run/,
  );
  assert.throws(
    () => engine.addFeedback(promoted.id, { kind: "useful" }),
    /itemId/,
  );
  assert.throws(
    () => engine.addFeedback(promoted.id, { kind: "useful", itemId: "unknown" }),
    /not present/,
  );
  engine.addFeedback(promoted.id, { kind: "useful", itemId: "promoted-item" });
  engine.addFeedback(promoted.id, { kind: "useful", itemId: "promoted-item" });
  assert.equal(engine.getRun(promoted.id).feedback.length, 1);
  store.close();
});

async function completeRun(engine, statusId, intent) {
  const run = engine.startRun({ mode: "catch_up", source: "x", maxItems: 1, scrolls: 0, intent });
  const command = engine.claimBridgeCommand(run.id, "feedback-test-bridge");
  engine.acceptBridgeObservation(command.id, run.id, observation(statusId));
  return engine.waitForRun(run.id);
}

function observation(statusId) {
  return {
    source: "x",
    pageUrl: "https://x.com/home",
    pageTitle: "Home / X",
    capturedAt: "2026-07-11T02:00:00Z",
    snapshots: [
      {
        capturedAt: "2026-07-11T02:00:00Z",
        scrollY: 0,
        viewportHeight: 900,
        blocks: [
          {
            text: `Fixture ${statusId}`,
            author: "Fixture",
            permalink: `https://x.com/fixture/status/${statusId}`,
            publishedAt: null,
            feedPosition: 1,
            links: [],
          },
        ],
      },
    ],
    coverage: {
      status: "partial",
      checkedThrough: "2026-07-11T02:00:00Z",
      candidateCount: 1,
    },
  };
}

import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { evidenceKeyForBlock, normalizeEventKey } from "../src/core/knowledge-continuity.mjs";
import { JobEngine } from "../src/core/job-engine.mjs";
import { SqliteStateStore } from "../src/store/sqlite-state-store.mjs";

const limits = {
  maxItems: 5,
  maxScrolls: 2,
  maxAcquisitionRounds: 2,
  followUpScrolls: 1,
  maxContinuationAnchors: 3,
  maxKnowledgeContextEvents: 20,
  defaultScrolls: 2,
  scrollFraction: 0.75,
  scrollSettleMs: 900,
  captureTimeoutMs: 45_000,
  pendingContentTimeoutMs: 5_000,
  pendingContentSettleMs: 700,
  maxBlocksPerSnapshot: 20,
  maxBlockCharacters: 4_000,
};

test("evidence identity is deterministic and event keys are normalized", () => {
  const first = evidenceKeyForBlock("x", {
    text: "Changed display text",
    permalink: "https://x.com/example/status/42",
  });
  const repeated = evidenceKeyForBlock("x", {
    text: "Another rendering of the same post",
    permalink: "https://x.com/example/status/42",
  });
  assert.equal(first, repeated);
  assert.match(first, /^x:[a-f0-9]{24}$/);
  assert.equal(normalizeEventKey(" OpenAI / Model Launch! "), "openai-model-launch");
});

test("checkpoint suppresses exact delivered evidence across restarts", async (context) => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "aku-knowledge-test-"));
  context.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  const databasePath = path.join(directory, "state.db");
  let analyzeCalls = 0;
  const provider = {
    name: "knowledge-test-provider",
    async analyze({ run, observation }) {
      analyzeCalls += 1;
      const block = observation.snapshots[0].blocks[0];
      return resultForBlock(run, block, "codex-reset-announcement", "new_event");
    },
  };
  let store = new SqliteStateStore(databasePath);
  let engine = new JobEngine({ store, reasoningProvider: provider, limits });

  const first = await executeRun(engine, observationFor("1", "A bankable Codex reset was announced."));
  assert.equal(first.status, "completed");
  assert.equal(first.result.items.length, 1);
  assert.equal(first.coverage.previousCheckpointRunId, null);
  assert.equal(first.coverage.checkpointAdvanced, true);
  const firstCheckpoint = store.getCheckpoint("x", "catch_up");
  assert.equal(firstCheckpoint.runId, first.id);

  store.close();
  store = new SqliteStateStore(databasePath);
  engine = new JobEngine({ store, reasoningProvider: provider, limits });
  const repeated = await executeRun(
    engine,
    observationFor("1", "A bankable Codex reset was announced."),
  );
  assert.equal(repeated.status, "completed");
  assert.equal(repeated.result.items.length, 0);
  assert.equal(repeated.coverage.exactDuplicatesSuppressed, 1);
  assert.equal(repeated.coverage.unseenEvidenceCount, 0);
  assert.equal(repeated.coverage.previousCheckpointRunId, first.id);
  assert.equal(analyzeCalls, 1);
  store.close();
});

test("correctly empty feedback suppresses evaluated evidence only for the same intent", async (context) => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "aku-negative-knowledge-test-"));
  context.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  const databasePath = path.join(directory, "state.db");
  let analyzeCalls = 0;
  let planCalls = 0;
  const provider = {
    name: "negative-knowledge-provider",
    async planAcquisition() {
      planCalls += 1;
      return {
        decision: "request_follow_up",
        reason: "The provider would inspect another viewport.",
      };
    },
    async analyze() {
      analyzeCalls += 1;
      return {
        summary: "Nothing material for this intent.",
        items: [],
        repeatedClaimsCollapsed: 0,
        deferredByBudget: 0,
        limitations: [],
      };
    },
  };
  let store = new SqliteStateStore(databasePath);
  let engine = new JobEngine({ store, reasoningProvider: provider, limits });
  const intent = "Technical engineering changes that affect my current work.";

  const first = await executeRun(
    engine,
    observationFor("excluded-1", "A generic lifestyle post."),
    intent,
  );
  engine.addFeedback(first.id, { kind: "correct_empty" });
  assert.equal(analyzeCalls, 1);

  store.close();
  store = new SqliteStateStore(databasePath);
  engine = new JobEngine({ store, reasoningProvider: provider, limits });
  const repeatedRun = engine.startRun({
    mode: "catch_up",
    source: "x",
    maxItems: 1,
    scrolls: 2,
    intent,
  });
  const repeatedCommand = engine.claimBridgeCommand(repeatedRun.id, "test-bridge");
  engine.acceptBridgeObservation(
    repeatedCommand.id,
    repeatedRun.id,
    nativeObservationFor("excluded-1", "A generic lifestyle post."),
  );
  const repeated = await engine.waitForRun(repeatedRun.id);
  assert.equal(repeated.result.items.length, 0);
  assert.equal(repeated.coverage.confirmedExcludedSuppressed, 1);
  assert.equal(repeated.coverage.unseenEvidenceCount, 0);
  assert.equal(repeated.coverage.acquisitionRounds, 1);
  assert.equal(repeated.coverage.providerFollowUpRequested, false);
  assert.equal(planCalls, 0);
  assert.equal(analyzeCalls, 1);

  const changedIntent = await executeRun(
    engine,
    observationFor("excluded-1", "A generic lifestyle post."),
    "Lifestyle inspiration and nature photography.",
  );
  assert.equal(changedIntent.coverage.confirmedExcludedSuppressed, 0);
  assert.equal(changedIntent.coverage.unseenEvidenceCount, 1);
  assert.equal(analyzeCalls, 2);
  store.close();
});

test("material updates append event history and advance the frontier", async (context) => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "aku-knowledge-test-"));
  context.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  const store = new SqliteStateStore(path.join(directory, "state.db"));
  let analyzeCalls = 0;
  const provider = {
    name: "event-version-provider",
    async analyze({ run, observation, knowledgeContext }) {
      analyzeCalls += 1;
      const block = observation.snapshots[0].blocks[0];
      if (analyzeCalls === 2) {
        assert.equal(knowledgeContext.events[0].eventKey, "codex-reset-announcement");
      }
      return resultForBlock(
        run,
        block,
        "codex-reset-announcement",
        analyzeCalls === 1 ? "new_event" : "material_update",
      );
    },
  };
  const engine = new JobEngine({ store, reasoningProvider: provider, limits });

  await executeRun(engine, observationFor("1", "A Codex reset was announced for next week."));
  const updated = await executeRun(
    engine,
    observationFor("2", "The Codex reset is now confirmed as bankable and starts tomorrow."),
  );
  assert.equal(updated.result.items[0].knowledgeDelta, "material_update");
  assert.equal(updated.coverage.knowledgeContextEvents, 1);
  const frontier = store.getKnowledgeContext("x", "catch_up", 20);
  assert.equal(frontier.events.length, 1);
  assert.match(frontier.events[0].claim, /bankable/);
  const history = store.getKnowledgeEventHistory(
    "x",
    "catch_up",
    "codex-reset-announcement",
  );
  assert.equal(history.length, 2);
  assert.deepEqual(
    history.map((version) => version.knowledgeDelta),
    ["new_event", "material_update"],
  );
  assert.equal(store.getCheckpoint("x", "manual_live"), null);
  store.close();
});

async function executeRun(engine, observation, intent = undefined) {
  const run = engine.startRun({
    mode: "catch_up",
    source: "x",
    maxItems: 1,
    scrolls: 0,
    ...(intent ? { intent } : {}),
  });
  const command = engine.claimBridgeCommand(run.id, "test-bridge");
  engine.acceptBridgeObservation(command.id, run.id, observation);
  return engine.waitForRun(run.id);
}

function observationFor(statusId, text) {
  return {
    source: "x",
    pageUrl: "https://x.com/home",
    pageTitle: "Home / X",
    capturedAt: "2026-07-11T04:00:00Z",
    snapshots: [
      {
        capturedAt: "2026-07-11T04:00:00Z",
        scrollY: 0,
        viewportHeight: 900,
        blocks: [
          {
            text,
            author: "OpenAI",
            permalink: `https://x.com/openai/status/${statusId}`,
            publishedAt: null,
            feedPosition: 1,
            links: [],
          },
        ],
      },
    ],
    coverage: {
      status: "partial",
      checkedThrough: "2026-07-11T04:00:00Z",
      candidateCount: 1,
    },
  };
}

function nativeObservationFor(statusId, text) {
  const observation = observationFor(statusId, text);
  const baseBlock = observation.snapshots[0].blocks[0];
  observation.snapshots = [0, 675, 1_350].map((scrollY, index) => ({
    ...observation.snapshots[0],
    scrollY,
    blocks: index === 0 ? [baseBlock] : [],
  }));
  observation.coverage = {
    ...observation.coverage,
    observedBlockCount: 1,
    browserAdapter: "aku-bridge",
    captureMethod: "native_dom",
    fallbackUsed: false,
    scrollContainer: "window",
    pendingNewContent: false,
    pendingNewContentLabel: "",
    pendingNewContentAction: "not_detected",
    pendingContentActivationEvidence: null,
    pendingContentPolicy: "reveal_if_present",
    feedMutation: false,
    sameTabMutation: false,
    restorationScope: "pre_run_position",
    preActionScrollY: 0,
    acquisitionRound: 1,
    continuationRequested: false,
    continuationAnchorMatched: false,
    captureStartScrollY: 0,
    requestedScrolls: 2,
    performedScrolls: 2,
    snapshotCount: 3,
    scrollDeltas: [675, 675],
    scrollStopReason: "budget_exhausted",
    originalScrollY: 0,
    finalScrollY: 0,
    restoreAttempted: true,
    restored: true,
    elapsedMs: 1_000,
    notes: [],
  };
  return observation;
}

function resultForBlock(run, block, eventKey, knowledgeDelta) {
  return {
    summary: "One bounded knowledge delta.",
    items: [
      {
        id: block.evidenceKey,
        priority: "P1",
        whatChanged: block.text,
        whyItMatters: run.intent,
        source: run.source,
        sourceUrl: block.permalink,
        sourceUrlKind: "native_post",
        evidenceKey: block.evidenceKey,
        eventKey,
        knowledgeDelta,
        author: block.author,
        publishedAt: block.publishedAt,
        confidence: 0.9,
        evidenceState: "primary",
      },
    ],
    repeatedClaimsCollapsed: 0,
    deferredByBudget: 0,
    limitations: [],
  };
}

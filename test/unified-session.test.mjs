import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import {
  JobEngine,
  mergeUnifiedItems,
} from "../src/core/job-engine.mjs";
import { validateBridgeObservation } from "../src/core/contracts.mjs";
import { SqliteStateStore } from "../src/store/sqlite-state-store.mjs";

const limits = {
  maxItems: 5,
  maxScrolls: 2,
  maxAcquisitionRounds: 2,
  followUpScrolls: 1,
  maxContinuationAnchors: 3,
  maxKnowledgeContextEvents: 20,
  defaultScrolls: 0,
  scrollFraction: 0.75,
  scrollSettleMs: 900,
  captureTimeoutMs: 45_000,
  pendingContentTimeoutMs: 5_000,
  pendingContentSettleMs: 700,
  maxBlocksPerSnapshot: 20,
  maxBlockCharacters: 4_000,
};

test("unified session runs X then LinkedIn and persists one finite merged result", async (context) => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "aku-unified-session-"));
  context.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  const databasePath = path.join(directory, "state.db");
  let store = new SqliteStateStore(databasePath);
  let engine = new JobEngine({ store, reasoningProvider: provider(), limits });

  let session = engine.startUnifiedSession({
    mode: "catch_up",
    intent: "Material engineering changes.",
  });
  assert.equal(session.status, "running");
  assert.equal(session.activeSource, "x");
  assert.equal(session.children[0].run.maxItems, 5);
  assert.equal(session.children[1].status, "queued");

  session = await completeActiveChild(engine, session);
  assert.equal(session.activeSource, "linkedin");
  assert.equal(session.children[0].status, "completed");
  assert.equal(session.children[1].status, "waiting_for_bridge");

  session = await completeActiveChild(engine, session);
  assert.equal(session.status, "completed");
  assert.equal(session.activeSource, null);
  assert.deepEqual(
    session.result.items.map((entry) => entry.item.source),
    ["x", "linkedin"],
  );
  assert.equal(session.coverage.resultCount, 2);
  assert.deepEqual(session.coverage.completedSources, ["x", "linkedin"]);

  const sessionId = session.id;
  store.close();
  store = new SqliteStateStore(databasePath);
  engine = new JobEngine({ store, reasoningProvider: provider(), limits });
  session = engine.getUnifiedSession(sessionId);
  assert.equal(session.status, "completed");
  assert.equal(session.result.items.length, 2);
  store.close();
});

test("unified session continues after one source failure and reports a partial result", async (context) => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "aku-unified-partial-"));
  context.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  const store = new SqliteStateStore(path.join(directory, "state.db"));
  const engine = new JobEngine({
    store,
    reasoningProvider: provider(),
    limits,
    logger: { error() {} },
  });

  let session = engine.startUnifiedSession({ intent: "Material engineering changes." });
  const xRun = session.children[0].run;
  const xCommand = engine.claimBridgeCommand(xRun.id, "unified-test-bridge");
  engine.failBridgeCommand(xCommand.id, xRun.id, { message: "Fixture X failure" });

  session = engine.getUnifiedSession(session.id);
  assert.equal(session.activeSource, "linkedin");
  session = await completeActiveChild(engine, session);
  assert.equal(session.status, "partial");
  assert.deepEqual(session.coverage.failedSources, ["x"]);
  assert.deepEqual(session.coverage.completedSources, ["linkedin"]);
  assert.equal(session.result.items.length, 1);
  store.close();
});

test("unified session resumes persisted reasoning after a Sidecar restart", async (context) => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "aku-unified-recovery-"));
  context.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  const databasePath = path.join(directory, "state.db");
  let store = new SqliteStateStore(databasePath);
  let engine = new JobEngine({ store, reasoningProvider: provider(), limits });
  const started = engine.startUnifiedSession({ intent: "Material engineering changes." });
  const xRun = started.children[0].run;
  const command = engine.claimBridgeCommand(xRun.id, "unified-recovery-bridge");
  const validated = validateBridgeObservation(observation("x"), limits);
  store.saveObservation(xRun.id, validated);
  store.completeBridgeCommand(command.id);
  store.setRunStatus(xRun.id, "reasoning");
  store.close();

  store = new SqliteStateStore(databasePath);
  engine = new JobEngine({ store, reasoningProvider: provider(), limits });
  let recovered = engine.getUnifiedSession(started.id);
  assert.equal(recovered.children[0].status, "reasoning");
  await engine.waitForRun(xRun.id);
  recovered = engine.getUnifiedSession(started.id);
  assert.equal(recovered.children[0].runId, xRun.id);
  assert.equal(recovered.children[0].status, "completed");
  assert.equal(recovered.activeSource, "linkedin");
  store.close();
});

test("pending-content reveal failure stops explicitly at source freshness", async (context) => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "aku-unified-reveal-recovery-"));
  context.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  const store = new SqliteStateStore(path.join(directory, "state.db"));
  const engine = new JobEngine({ store, reasoningProvider: provider(), limits: { ...limits, defaultScrolls: 2 } });
  const session = engine.startUnifiedSession({ intent: "Material engineering changes." });
  const xRun = session.children[0].run;
  const revealCommand = engine.claimBridgeCommand(xRun.id, "unified-recovery-bridge");
  const failed = engine.failBridgeCommand(revealCommand.id, xRun.id, {
    code: "freshness_unavailable",
    stage: "source_freshness",
    message:
      "x freshness unavailable: the pending-content reveal did not produce a changed feed.",
  });
  assert.equal(failed.status, "failed");
  assert.equal(failed.error.stage, "source_freshness");
  const advanced = engine.getUnifiedSession(session.id);
  assert.equal(advanced.children[0].status, "failed");
  assert.equal(advanced.activeSource, "linkedin");
  store.close();
});

test("zero-evidence capture becomes a source failure instead of a correctly-empty result", async (context) => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "aku-unified-no-evidence-"));
  context.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  const store = new SqliteStateStore(path.join(directory, "state.db"));
  const engine = new JobEngine({
    store,
    reasoningProvider: provider(),
    limits,
    logger: { error() {} },
  });
  let session = engine.startUnifiedSession({ intent: "Material engineering changes." });
  const xRun = session.children[0].run;
  const command = engine.claimBridgeCommand(xRun.id, "unified-no-evidence-bridge");
  engine.acceptBridgeObservation(command.id, xRun.id, emptyObservation("x"));
  await engine.waitForRun(xRun.id);
  session = engine.getUnifiedSession(session.id);
  assert.equal(session.children[0].status, "failed");
  assert.equal(session.children[0].run.error.stage, "source_readiness");
  assert.match(
    session.children[0].run.error.message,
    /did not become evidence-ready/,
  );
  assert.equal(session.activeSource, "linkedin");
  store.close();
});

test("unified merge round-robins sources while preserving platform order", () => {
  const children = [
    childFixture("x", [itemFixture("x-p1-a", "P1"), itemFixture("x-p1-b", "P1"), itemFixture("x-p2", "P2")]),
    childFixture("linkedin", [itemFixture("li-p1", "P1"), itemFixture("li-p2", "P2")]),
  ];
  const merged = mergeUnifiedItems("session-1", children, 10);
  assert.deepEqual(
    merged.map((entry) => entry.item.id),
    ["x-p1-a", "li-p1", "x-p1-b", "li-p2", "x-p2"],
  );
  assert.ok(merged.every((entry) => entry.sessionId === "session-1"));
});

async function completeActiveChild(engine, session) {
  const child = session.children.find(
    (candidate) => candidate.run && !["completed", "failed", "cancelled"].includes(candidate.run.status),
  );
  assert.ok(child);
  const command = engine.claimBridgeCommand(child.run.id, "unified-test-bridge");
  engine.acceptBridgeObservation(
    command.id,
    child.run.id,
    observation(child.source),
  );
  await engine.waitForRun(child.run.id);
  return engine.getUnifiedSession(session.id);
}

function provider() {
  return {
    name: "unified-test-provider",
    async analyze({ run, observation: value }) {
      const block = value.snapshots[0].blocks[0];
      return {
        summary: `One material ${run.source} item.`,
        items: [
          {
            id: `${run.source}-item`,
            priority: "P1",
            whatChanged: block.text,
            whyItMatters: run.intent,
            source: run.source,
            sourceUrl: block.permalink,
            sourceUrlKind: "native_post",
            evidenceKey: block.evidenceKey,
            eventKey: `${run.source}-unified-fixture`,
            knowledgeDelta: "new_event",
            author: block.author,
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
}

function observation(source, scrolls = 0) {
  const snapshots = [];
  for (let index = 0; index <= scrolls; index += 1) {
    snapshots.push({
      capturedAt: "2026-07-11T08:00:00Z",
      scrollY: index * 675,
      viewportHeight: 900,
      blocks: [
        {
          text: `Visible ${source} engineering release ${index}.`,
          author: "Fixture",
          permalink:
            source === "x"
              ? `https://x.com/fixture/status/unified-${index}`
              : `https://www.linkedin.com/posts/fixture-unified-${index}-activity-1234567890`,
          publishedAt: null,
          feedPosition: index + 1,
          links: [],
        },
      ],
    });
  }
  return {
    source,
    pageUrl: source === "x" ? "https://x.com/home" : "https://www.linkedin.com/feed/",
    pageTitle: source === "x" ? "Home / X" : "LinkedIn Feed",
    capturedAt: "2026-07-11T08:00:00Z",
    snapshots,
    coverage: {
      status: "partial",
      checkedThrough: "2026-07-11T08:00:00Z",
      candidateCount: snapshots.length,
      observedBlockCount: snapshots.length,
      browserAdapter: "aku-bridge",
      captureMethod: "native_dom",
      fallbackUsed: false,
      scrollContainer: "window",
      pendingNewContent: false,
      pendingNewContentLabel: "",
      pendingNewContentAction: "not_detected",
      pendingContentActivationEvidence: null,
      pendingContentPolicy: "reveal_if_present",
      sourceFreshness: sourceFreshness(source),
      feedMutation: false,
      sameTabMutation: false,
      restorationScope: "pre_run_position",
      preActionScrollY: 0,
      acquisitionRound: 1,
      continuationRequested: false,
      continuationAnchorMatched: false,
      captureStartScrollY: 0,
      requestedScrolls: scrolls,
      performedScrolls: scrolls,
      snapshotCount: snapshots.length,
      scrollDeltas: Array.from({ length: scrolls }, () => 675),
      scrollStopReason: scrolls === 0 ? "not_requested" : "budget_exhausted",
      originalScrollY: 0,
      finalScrollY: 0,
      restoreAttempted: true,
      restored: true,
      elapsedMs: 100,
      notes: [],
    },
  };
}

function sourceFreshness(source) {
  return {
    policyVersion: "source-freshness-recovery-v1",
    adapterFreshnessVersion: `${source}-freshness-v1`,
    source,
    status: "ready",
    outcome: "active_feed_ready",
    verification: "active_dispatch",
    evidence: "active_at_dispatch",
    backgroundAtDispatch: false,
    opened: false,
    wakeAttempted: false,
    activated: false,
    probeCount: 1,
    pendingContentDetected: false,
    pendingContentLabel: "",
    pendingContentAction: "not_detected",
    feedChanged: false,
    feedMutation: false,
    waitMs: 5,
    preActionScrollY: 0,
  };
}

function emptyObservation(source) {
  const value = observation(source);
  value.snapshots[0].blocks = [];
  value.coverage.status = "unavailable";
  value.coverage.candidateCount = 0;
  value.coverage.observedBlockCount = 0;
  return value;
}

function childFixture(source, items) {
  return {
    source,
    run: {
      id: `${source}-run`,
      status: "completed",
      result: { items, limitations: [], repeatedClaimsCollapsed: 0, deferredByBudget: 0 },
    },
  };
}

function itemFixture(id, priority) {
  return { id, priority };
}

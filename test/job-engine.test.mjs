import test from "node:test";
import assert from "node:assert/strict";
import { sourceFreshnessFixture } from "./source-freshness-fixture.mjs";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { JobEngine } from "../src/core/job-engine.mjs";
import { SqliteStateStore } from "../src/store/sqlite-state-store.mjs";

const limits = {
  maxItems: 5,
  maxScrolls: 2,
  maxAcquisitionRounds: 2,
  followUpScrolls: 1,
  maxContinuationAnchors: 3,
  defaultScrolls: 2,
  scrollFraction: 0.75,
  scrollSettleMs: 900,
  captureTimeoutMs: 45_000,
  pendingContentTimeoutMs: 5_000,
  pendingContentSettleMs: 700,
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
            sourceUrlKind: "native_post",
            evidenceKey: observation.snapshots[0].blocks[0].evidenceKey,
            eventKey: "gate-zero-test-event",
            knowledgeDelta: "new_event",
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

test("Gate 0B carries a native multi-viewport capture through reasoning and coverage", async () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "aku-browser-test-"));
  const store = new SqliteStateStore(path.join(directory, "state.db"));
  try {
    const provider = {
      name: "multi-viewport-provider",
      async analyze({ run, observation }) {
        const block = observation.snapshots[2].blocks[0];
        return {
          summary: "The strongest item was found after bounded native scrolling.",
          items: [
            {
              id: "after-scroll",
              priority: "P1",
              whatChanged: block.text,
              whyItMatters: run.intent,
              source: run.source,
              sourceUrl: block.permalink,
              sourceUrlKind: "native_post",
              evidenceKey: block.evidenceKey,
              eventKey: "multi-viewport-test-event",
              knowledgeDelta: "new_event",
              author: block.author,
              publishedAt: null,
              confidence: 0.85,
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
    const run = engine.startRun({ source: "linkedin", maxItems: 1, scrolls: 2 });
    const command = engine.claimBridgeCommand(run.id, "test-bridge");

    assert.equal(command.payload.browserAdapter, "aku-bridge");
    assert.equal(command.payload.scrollFraction, 0.75);
    assert.equal(command.payload.captureTimeoutMs, 45_000);
    assert.equal(command.payload.pendingContentPolicy, "reveal_if_present");
    assert.equal(command.payload.sameTabMutationAllowed, true);

    engine.acceptBridgeObservation(command.id, run.id, multiViewportObservation());
    const completed = await engine.waitForRun(run.id);

    assert.equal(completed.status, "completed");
    assert.equal(completed.result.items[0].id, "after-scroll");
    assert.equal(completed.coverage.snapshotCount, 3);
    assert.equal(completed.coverage.performedScrolls, 2);
    assert.equal(completed.coverage.restored, true);
    assert.equal(completed.coverage.fallbackUsed, false);
    assert.equal(completed.coverage.pendingNewContentAction, "not_detected");
    assert.equal(completed.coverage.feedMutation, false);
    assert.equal(completed.coverage.restorationScope, "pre_run_position");
  } finally {
    store.close();
    fs.rmSync(directory, { recursive: true, force: true });
  }
});

test("Gate 0B.3 permits one provider-requested anchored follow-up", async () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "aku-browser-test-"));
  const store = new SqliteStateStore(path.join(directory, "state.db"));
  try {
    const provider = {
      name: "bounded-planning-provider",
      async planAcquisition() {
        return {
          decision: "request_follow_up",
          reason: "One adjacent viewport may contain the missing material delta.",
        };
      },
      async analyze({ run, observation, observations }) {
        const block = observation.snapshots.at(-1).blocks[0];
        return {
          summary: "The follow-up completed the bounded evidence set.",
          items: [
            {
              id: "follow-up-result",
              priority: "P1",
              whatChanged: block.text,
              whyItMatters: run.intent,
              source: run.source,
              sourceUrl: block.permalink,
              sourceUrlKind: "native_post",
              evidenceKey: block.evidenceKey,
              eventKey: "follow-up-test-event",
              knowledgeDelta: "new_event",
              author: block.author,
              publishedAt: null,
              confidence: 0.85,
              evidenceState: "primary",
            },
          ],
          repeatedClaimsCollapsed: observations.length - 1,
          deferredByBudget: 0,
          limitations: [],
        };
      },
    };
    const engine = new JobEngine({ store, reasoningProvider: provider, limits });
    const run = engine.startRun({ source: "linkedin", maxItems: 1, scrolls: 2 });
    const firstCommand = engine.claimBridgeCommand(run.id, "test-bridge");

    engine.acceptBridgeObservation(firstCommand.id, run.id, multiViewportObservation());
    const waiting = await engine.waitForRun(run.id);
    assert.equal(waiting.status, "waiting_for_bridge");
    assert.equal(waiting.observations.length, 1);

    const followUpCommand = engine.claimBridgeCommand(run.id, "test-bridge");
    assert.equal(followUpCommand.payload.acquisitionRound, 2);
    assert.equal(followUpCommand.payload.scrolls, 1);
    assert.equal(followUpCommand.payload.continuation.startScrollY, 1_350);
    assert.equal(followUpCommand.payload.continuation.anchorKeys.length, 1);
    assert.equal(followUpCommand.payload.pendingContentPolicy, "detect_only");

    engine.acceptBridgeObservation(
      followUpCommand.id,
      run.id,
      followUpObservation(followUpCommand.payload.continuation),
    );
    const completed = await engine.waitForRun(run.id);
    assert.equal(completed.status, "completed");
    assert.equal(completed.observations.length, 2);
    assert.equal(completed.coverage.acquisitionRounds, 2);
    assert.equal(completed.coverage.providerFollowUpRequested, true);
    assert.equal(completed.coverage.providerFollowUpExecuted, true);
    assert.equal(completed.coverage.continuationRequested, true);
    assert.equal(completed.coverage.continuationAnchorMatched, true);
    assert.equal(completed.coverage.snapshotCount, 5);
    assert.equal(completed.coverage.performedScrolls, 3);
    assert.equal(completed.result.items[0].id, "follow-up-result");
  } finally {
    store.close();
    fs.rmSync(directory, { recursive: true, force: true });
  }
});

test("cancelling during Gate 0B.3 planning cannot resurrect the run", async () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "aku-browser-test-"));
  const store = new SqliteStateStore(path.join(directory, "state.db"));
  let releasePlanning;
  const planningGate = new Promise((resolve) => {
    releasePlanning = resolve;
  });
  let analyzeCalls = 0;
  try {
    const provider = {
      name: "cancellable-planning-provider",
      async planAcquisition() {
        await planningGate;
        return { decision: "finish", reason: "The bounded evidence is sufficient." };
      },
      async analyze() {
        analyzeCalls += 1;
        return {};
      },
    };
    const engine = new JobEngine({ store, reasoningProvider: provider, limits });
    const run = engine.startRun({ source: "linkedin", maxItems: 1, scrolls: 2 });
    const command = engine.claimBridgeCommand(run.id, "test-bridge");

    engine.acceptBridgeObservation(command.id, run.id, multiViewportObservation());
    const cancelled = engine.cancelRun(run.id);
    assert.equal(cancelled.status, "cancelled");
    releasePlanning();

    const settled = await engine.waitForRun(run.id);
    assert.equal(settled.status, "cancelled");
    assert.equal(analyzeCalls, 0);
  } finally {
    releasePlanning?.();
    store.close();
    fs.rmSync(directory, { recursive: true, force: true });
  }
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
        async analyze({ observation }) {
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
                sourceUrlKind: "source_page",
                evidenceKey: observation.snapshots[0].blocks[0].evidenceKey,
                eventKey: "unsupported-provenance-event",
                knowledgeDelta: "new_event",
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
    assert.match(failed.error.message, /outside its bound evidence block/i);
  } finally {
    store.close();
    fs.rmSync(directory, { recursive: true, force: true });
  }
});

test("reasoning cannot relabel an external reference as a native post", async () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "aku-browser-test-"));
  const store = new SqliteStateStore(path.join(directory, "state.db"));
  try {
    const engine = new JobEngine({
      store,
      reasoningProvider: {
        name: "wrong-provenance-lane-provider",
        async analyze({ observation }) {
          return {
            summary: "Invalid provenance-lane fixture.",
            items: [
              {
                id: "wrong-lane",
                priority: "P2",
                whatChanged: "A referenced page was observed.",
                whyItMatters: "The URL must retain its actual provenance lane.",
                source: "x",
                sourceUrl: "https://example.com/reference",
                sourceUrlKind: "native_post",
                evidenceKey: observation.snapshots[0].blocks[0].evidenceKey,
                eventKey: "external-reference-event",
                knowledgeDelta: "new_event",
                author: "Example",
                publishedAt: null,
                confidence: 0.7,
                evidenceState: "secondary",
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
    const observation = sampleObservation();
    observation.snapshots[0].blocks[0].links = [
      { text: "Reference", href: "https://example.com/reference" },
    ];
    engine.acceptBridgeObservation(command.id, run.id, observation);
    const failed = await engine.waitForRun(run.id);
    assert.equal(failed.status, "failed");
    assert.equal(failed.error.stage, "reasoning");
    assert.match(failed.error.message, /native_post URL outside its bound evidence block/i);
  } finally {
    store.close();
    fs.rmSync(directory, { recursive: true, force: true });
  }
});

test("source-page provenance is accepted when a native permalink is unavailable", async () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "aku-browser-test-"));
  const store = new SqliteStateStore(path.join(directory, "state.db"));
  try {
    const engine = new JobEngine({
      store,
      reasoningProvider: {
        name: "source-page-provider",
        async analyze({ observation }) {
          return {
            summary: "Honest source-page fallback fixture.",
            items: [
              {
                id: "source-page",
                priority: "P3",
                whatChanged: "A visible post had no native permalink in the DOM.",
                whyItMatters: "The feed URL remains an honest, lower-resolution source.",
                source: "x",
                sourceUrl: observation.pageUrl,
                sourceUrlKind: "source_page",
                evidenceKey: observation.snapshots[0].blocks[0].evidenceKey,
                eventKey: "source-page-fallback-event",
                knowledgeDelta: "new_event",
                author: "Example",
                publishedAt: null,
                confidence: 0.5,
                evidenceState: "unverified",
              },
            ],
            repeatedClaimsCollapsed: 0,
            deferredByBudget: 0,
            limitations: ["The native post URL was unavailable."],
          };
        },
      },
      limits,
    });
    const run = engine.startRun({ source: "x", maxItems: 1, scrolls: 0 });
    const command = engine.claimBridgeCommand(run.id, "test-bridge");
    const observation = sampleObservation();
    observation.snapshots[0].blocks[0].permalink = null;
    engine.acceptBridgeObservation(command.id, run.id, observation);
    const completed = await engine.waitForRun(run.id);
    assert.equal(completed.status, "completed");
    assert.equal(completed.result.items[0].sourceUrlKind, "source_page");
    assert.equal(completed.result.items[0].sourceUrl, "https://x.com/home");
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
      sourceFreshness: sourceFreshnessFixture("x"),
      checkedThrough: "2026-07-10T10:00:00Z",
      candidateCount: 1,
      notes: ["One visible viewport; no scrolling."],
    },
  };
}

function multiViewportObservation() {
  return {
    source: "linkedin",
    pageUrl: "https://www.linkedin.com/feed/",
    pageTitle: "Feed | LinkedIn",
    capturedAt: "2026-07-10T10:00:03Z",
    snapshots: [0, 1, 2].map((index) => ({
      index,
      adapterVersion: "linkedin-dom-v2",
      selectorCandidateCount: 8,
      visibleContainerCount: 1,
      newCandidateCount: 1,
      capturedAt: `2026-07-10T10:00:0${index}Z`,
      scrollY: index * 675,
      viewportHeight: 900,
      blocks: [
        {
          text: `Material professional update ${index} with enough detail to classify.`,
          author: "Example",
          publishedAt: null,
          permalink: `https://www.linkedin.com/feed/update/urn:li:activity:${index}`,
          feedPosition: index + 1,
          links: [],
        },
      ],
    })),
    coverage: {
      status: "partial",
      checkedThrough: "2026-07-10T10:00:03Z",
      candidateCount: 3,
      observedBlockCount: 3,
      browserAdapter: "aku-bridge",
      captureMethod: "native_dom",
      fallbackUsed: false,
      scrollContainer: "#workspace",
      pendingNewContent: false,
      pendingNewContentLabel: "",
      pendingNewContentAction: "not_detected",
      pendingContentActivationEvidence: null,
      pendingContentPolicy: "reveal_if_present",
      sourceFreshness: sourceFreshness("linkedin"),
      feedMutation: false,
      sameTabMutation: false,
      restorationScope: "pre_run_position",
      preActionScrollY: 0,
      requestedScrolls: 2,
      performedScrolls: 2,
      snapshotCount: 3,
      scrollDeltas: [675, 675],
      scrollStopReason: "budget_exhausted",
      originalScrollY: 0,
      finalScrollY: 0,
      restoreAttempted: true,
      restored: true,
      elapsedMs: 2_100,
      notes: ["Bounded native fixture."],
    },
  };
}

function followUpObservation(continuation) {
  return {
    source: "linkedin",
    pageUrl: "https://www.linkedin.com/feed/",
    pageTitle: "Feed | LinkedIn",
    capturedAt: "2026-07-10T10:00:06Z",
    snapshots: [0, 1].map((index) => ({
      index,
      adapterVersion: "linkedin-dom-v2",
      selectorCandidateCount: 8,
      visibleContainerCount: 1,
      newCandidateCount: 1,
      capturedAt: `2026-07-10T10:00:0${5 + index}Z`,
      scrollY: continuation.startScrollY + index * 675,
      viewportHeight: 900,
      blocks: [
        {
          text:
            index === 0
              ? "Material professional update 2 with enough detail to classify."
              : "A follow-up viewport contained a material release with concrete details.",
          author: "Example",
          publishedAt: null,
          permalink:
            index === 0
              ? "https://www.linkedin.com/feed/update/urn:li:activity:2"
              : "https://www.linkedin.com/feed/update/urn:li:activity:follow-up",
          feedPosition: 3 + index,
          links: [],
        },
      ],
    })),
    coverage: {
      status: "partial",
      checkedThrough: "2026-07-10T10:00:06Z",
      candidateCount: 2,
      observedBlockCount: 2,
      browserAdapter: "aku-bridge",
      captureMethod: "native_dom",
      fallbackUsed: false,
      scrollContainer: "#workspace",
      pendingNewContent: false,
      pendingNewContentLabel: "",
      pendingNewContentAction: "not_detected",
      pendingContentActivationEvidence: null,
      pendingContentPolicy: "detect_only",
      sourceFreshness: sourceFreshness("linkedin", "follow_up_preserved"),
      feedMutation: false,
      sameTabMutation: false,
      restorationScope: "pre_run_position",
      preActionScrollY: 0,
      acquisitionRound: 2,
      continuationRequested: true,
      continuationAnchorMatched: true,
      captureStartScrollY: continuation.startScrollY,
      requestedScrolls: 1,
      performedScrolls: 1,
      snapshotCount: 2,
      scrollDeltas: [675],
      scrollStopReason: "budget_exhausted",
      originalScrollY: 0,
      finalScrollY: 0,
      restoreAttempted: true,
      restored: true,
      elapsedMs: 1_200,
      notes: ["Bounded follow-up fixture."],
    },
  };
}

function sourceFreshness(source, outcome = "active_feed_ready") {
  const followUp = outcome === "follow_up_preserved";
  return {
    policyVersion: "source-freshness-recovery-v1",
    adapterFreshnessVersion: `${source}-freshness-v1`,
    source,
    status: "ready",
    outcome,
    verification: followUp ? "frontier_contract" : "active_dispatch",
    evidence: followUp ? "follow_up_no_freshness_mutation" : "active_at_dispatch",
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

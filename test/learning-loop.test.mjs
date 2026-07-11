import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { decideAcquisitionPlanning, JobEngine } from "../src/core/job-engine.mjs";
import { loadConfig } from "../src/config.mjs";
import { SqliteStateStore } from "../src/store/sqlite-state-store.mjs";

const limits = {
  maxItems: 5,
  maxScrolls: 2,
  defaultScrolls: 0,
  maxAcquisitionRounds: 1,
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

test("Codex model and phase effort are explicit configurable runtime metadata", () => {
  const config = loadConfig({
    AKU_REASONING_PROVIDER: "codex-sdk",
    AKU_CODEX_MODEL: "fixture-model",
    AKU_CODEX_PLANNING_EFFORT: "minimal",
    AKU_CODEX_EVALUATION_EFFORT: "medium",
  });
  assert.equal(config.reasoning.model, "fixture-model");
  assert.equal(config.reasoning.planningModel, "fixture-model");
  assert.equal(config.reasoning.evaluationModel, "fixture-model");
  assert.equal(config.reasoning.planningEffort, "minimal");
  assert.equal(config.reasoning.evaluationEffort, "medium");
  assert.equal(config.limits.missingSourceTabPolicy, "open_missing_tab");
  assert.equal(
    loadConfig({ AKU_MISSING_SOURCE_TAB_POLICY: "fail_fast" }).limits.missingSourceTabPolicy,
    "fail_fast",
  );
});

test("startup removes retired development preference kinds", () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "aku-retired-preferences-"));
  const databasePath = path.join(directory, "state.db");
  let store = new SqliteStateStore(databasePath);
  store.database.exec("PRAGMA foreign_keys = OFF");
  store.database.prepare(`
    INSERT INTO preference_feedback_events(
      id, run_id, evidence_key, kind, reason_code, note, created_at
    ) VALUES (?, ?, ?, ?, ?, ?, ?)
  `).run(
    "retired-event",
    "retired-run",
    "x:000000000000000000000000",
    "should_show",
    null,
    "development fixture",
    "2026-07-11T00:00:00.000Z",
  );
  store.close();

  store = new SqliteStateStore(databasePath);
  const count = store.database
    .prepare("SELECT COUNT(*) AS count FROM preference_feedback_events")
    .get().count;
  assert.equal(count, 0);
  store.close();
  fs.rmSync(directory, { recursive: true, force: true });
});

test("deterministic planning gate spends model tokens only on a sparse movable gap", () => {
  const coverage = {
    scrollStopReason: "budget_exhausted",
    requestedScrolls: 2,
    performedScrolls: 2,
  };
  assert.equal(
    decideAcquisitionPlanning({
      policy: "deterministic_sparse_gap",
      observation: { coverage },
      unseenEvidenceCount: 4,
    }).invokeProvider,
    false,
  );
  assert.equal(
    decideAcquisitionPlanning({
      policy: "deterministic_sparse_gap",
      observation: { coverage },
      unseenEvidenceCount: 2,
    }).invokeProvider,
    true,
  );
  assert.equal(
    decideAcquisitionPlanning({
      policy: "deterministic_sparse_gap",
      observation: {
        coverage: { ...coverage, scrollStopReason: "no_movement", performedScrolls: 1 },
      },
      unseenEvidenceCount: 1,
    }).invokeProvider,
    false,
  );
});

test("learning loop persists evaluated decisions, usage, and append-only corrections", async (context) => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "aku-learning-loop-"));
  context.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  const store = new SqliteStateStore(path.join(directory, "state.db"));
  const engine = new JobEngine({
    store,
    limits,
    reasoningProvider: {
      name: "telemetry-fixture",
      async analyze({ run, observation }) {
        const selected = observation.snapshots[0].blocks[0];
        return {
          output: {
            summary: "One selected candidate.",
            items: [{
              id: "selected-item",
              priority: "P1",
              whatChanged: selected.text,
              whyItMatters: run.intent,
              source: "x",
              sourceUrl: selected.permalink,
              sourceUrlKind: "native_post",
              evidenceKey: selected.evidenceKey,
              eventKey: "learning-loop-fixture",
              knowledgeDelta: "new_event",
              author: selected.author,
              publishedAt: null,
              confidence: 0.9,
              evidenceState: "primary",
            }],
            repeatedClaimsCollapsed: 0,
            deferredByBudget: 0,
            limitations: [],
            candidateAssessments: observation.snapshots[0].blocks.map((block, index) => ({
              evidenceKey: block.evidenceKey,
              topicTags: ["technical-change"],
              contentType: "announcement",
              recommendedPriority: index === 0 ? "P1" : "P3",
              intentRelevance: index === 0 ? 0.9 : 0.5,
              novelty: 0.8,
              urgency: index === 0 ? 0.7 : 0.2,
              actionability: index === 0 ? 0.8 : 0.3,
              rationale: "Fixture assessment grounded in the observed block.",
            })),
          },
          telemetry: {
            runId: run.id,
            phase: "candidate_evaluation",
            provider: "telemetry-fixture",
            model: "fixture-model",
            reasoningEffort: "low",
            durationMs: 12,
            status: "completed",
            inputTokens: 100,
            cachedInputTokens: 40,
            outputTokens: 20,
            reasoningOutputTokens: 5,
          },
        };
      },
    },
  });

  const started = engine.startRun({ source: "x", scrolls: 0, intent: "Technical changes" });
  const command = engine.claimBridgeCommand(started.id, "learning-loop-bridge");
  engine.acceptBridgeObservation(command.id, started.id, observation());
  const run = await engine.waitForRun(started.id);

  assert.equal(run.candidateEvaluations.length, 2);
  const selected = run.candidateEvaluations.find((entry) => entry.decision === "selected");
  const excluded = run.candidateEvaluations.find((entry) => entry.decision === "excluded");
  assert.equal(selected.itemId, "selected-item");
  assert.equal(excluded.reasonCode, "not_promoted_by_provider");
  assert.equal(selected.assessment.recommendedPriority, "P1");
  assert.equal(excluded.assessment.contentType, "announcement");
  assert.equal(run.reasoningInvocations[0].inputTokens, 100);

  engine.addPreferenceFeedback(run.id, {
    kind: "more_like_this",
    evidenceKey: excluded.evidenceKey,
    reasonCode: null,
    note: "",
  });
  engine.addPreferenceFeedback(run.id, {
    kind: "more_like_this",
    evidenceKey: excluded.evidenceKey,
    reasonCode: null,
    note: "",
  });
  engine.addPreferenceFeedback(run.id, {
    kind: "less_like_this",
    evidenceKey: excluded.evidenceKey,
    reasonCode: "low_signal",
    note: "",
  });
  engine.addPreferenceFeedback(run.id, {
    kind: "more_like_this",
    evidenceKey: selected.evidenceKey,
    reasonCode: null,
    note: "",
  });
  assert.equal(engine.getRun(run.id).preferenceFeedback.length, 3);
  assert.deepEqual(engine.getPreferenceProfile(), {
    version: 0,
    status: "collecting",
    feedbackEventCount: 2,
    moreLikeThisCount: 1,
    lessLikeThisCount: 1,
    selectedMoreLikeThisCount: 1,
    excludedMoreLikeThisCount: 0,
    updatedAt: engine.getPreferenceProfile().updatedAt,
  });
  assert.throws(() => engine.addPreferenceFeedback(run.id, {
    kind: "should_show",
    evidenceKey: excluded.evidenceKey,
    reasonCode: null,
    note: "retired alias",
  }), /unsupported preference feedback kind/);
  store.close();
});

function observation() {
  return {
    source: "x",
    pageUrl: "https://x.com/home",
    pageTitle: "Home / X",
    capturedAt: "2026-07-11T09:00:00Z",
    snapshots: [{
      capturedAt: "2026-07-11T09:00:00Z",
      scrollY: 0,
      viewportHeight: 900,
      blocks: [
        block("selected", 1),
        block("excluded", 2),
      ],
    }],
    coverage: {
      status: "partial",
      checkedThrough: "2026-07-11T09:00:00Z",
      candidateCount: 2,
    },
  };
}

function block(id, feedPosition) {
  return {
    text: `Candidate ${id}`,
    author: "Fixture",
    permalink: `https://x.com/fixture/status/${id}`,
    publishedAt: null,
    feedPosition,
    links: [],
  };
}

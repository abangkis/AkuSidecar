import assert from "node:assert/strict";
import test from "node:test";
import {
  buildBridgeHealth,
  createBridgeDiagnostics,
  sanitizeHeartbeat,
} from "../src/operations/bridge-diagnostics.mjs";

test("heartbeat keeps operational capabilities and drops arbitrary content", () => {
  const heartbeat = sanitizeHeartbeat({
    capabilities: {
      bridgeId: "aku-bridge",
      extensionVersion: "0.5.0",
      runtimeRevision: "bridge-diagnostics-v1",
      buildId: "fixture-build",
      adapterVersions: { x: "x-dom-v1", linkedin: "linkedin-dom-v2" },
      sources: ["x", "linkedin"],
      actions: ["collect_visible", "collect_visible"],
      authority: "read_only_bounded",
      captureLimits: { maxScrolls: 2, maxSnapshots: 3, maxBlocksPerSnapshot: 20 },
      rawPostText: "must not escape",
      token: "secret",
    },
  }, "2026-07-12T01:00:00.000Z");

  assert.equal(heartbeat.runtimeRevision, "bridge-diagnostics-v1");
  assert.equal(heartbeat.buildId, "fixture-build");
  assert.deepEqual(heartbeat.adapterVersions, { x: "x-dom-v1", linkedin: "linkedin-dom-v2" });
  assert.deepEqual(heartbeat.actions, ["collect_visible"]);
  assert.equal(heartbeat.rawPostText, undefined);
  assert.equal(heartbeat.token, undefined);
});

test("bridge report distinguishes unavailable, healthy, stale, and degraded", () => {
  const clock = { value: Date.parse("2026-07-12T01:00:00.000Z") };
  const diagnostics = createBridgeDiagnostics({ now: () => clock.value });
  assert.equal(diagnostics.report().status, "unavailable");

  diagnostics.recordHeartbeat(compatibleHeartbeat());
  assert.equal(diagnostics.report().status, "healthy");
  assert.equal(diagnostics.compatibility().compatible, true);

  clock.value += 90_001;
  assert.equal(diagnostics.report().status, "degraded");
});

test("bridge compatibility rejects stale versions, revisions, and adapters", () => {
  const diagnostics = createBridgeDiagnostics();
  diagnostics.recordHeartbeat({
    extensionVersion: "0.5.0",
    runtimeRevision: "old-runtime",
    buildId: "old-build",
    adapterVersions: { x: "x-dom-v1", linkedin: "linkedin-dom-v2" },
  });
  const compatibility = diagnostics.compatibility();
  assert.equal(compatibility.compatible, false);
  assert.equal(compatibility.reasons.length, 13);
  assert.ok(compatibility.reasons.some((reason) => reason.includes("report_capture_quality")));
});

test("source health exposes diagnostics but no captured evidence", () => {
  const report = buildBridgeHealth({
    heartbeat: sanitizeHeartbeat(compatibleHeartbeat(), "2026-07-12T01:00:00.000Z"),
    now: Date.parse("2026-07-12T01:00:01.000Z"),
    runs: [{
      source: "linkedin",
      observations: [{
        capturedAt: "2026-07-12T00:59:59.000Z",
        snapshots: [{ blocks: [{ text: "private post", author: "private author" }] }],
        coverage: {
          adapterVersion: "linkedin-dom-v2",
          adapterHealth: {
            state: "healthy",
            strategies: ["feed-update"],
            selectorCounts: { "feed-update": 8 },
            fieldCoverage: { publishedAt: { present: 7, total: 8 } },
            domSignature: "linkedin-dom-v2:8:3",
          },
          frontier: { newCandidateCount: 3, hasMoreCandidateSignal: true },
          sourceEvents: [{ type: "source_new_content_available" }],
          restoreAttempted: true,
          restored: true,
          restorationScope: "pre_run_position",
          sourceTabOwnership: "shared",
          sourceReadinessState: "feed_ready",
        },
      }],
    }],
  });

  assert.equal(report.status, "healthy");
  assert.equal(report.sources.linkedin.adapterVersion, "linkedin-dom-v2");
  assert.equal(report.sources.linkedin.sourceEvents.source_new_content_available, 1);
  assert.equal(JSON.stringify(report).includes("private post"), false);
  assert.equal(JSON.stringify(report).includes("private author"), false);
});

test("recent failures override a stale successful LinkedIn observation", () => {
  const successful = {
    source: "linkedin",
    status: "completed",
    createdAt: "2026-07-14T01:00:00.000Z",
    completedAt: "2026-07-14T01:00:10.000Z",
    observations: [{
      payload: {
        capturedAt: "2026-07-14T01:00:05.000Z",
        coverage: {
          adapterVersion: "linkedin-dom-v10",
          adapterHealth: { state: "healthy" },
          restored: true,
        },
      },
    }],
  };
  const failures = [1, 2].map((minute) => ({
    source: "linkedin",
    status: "failed",
    createdAt: `2026-07-14T01:0${minute}:00.000Z`,
    completedAt: `2026-07-14T01:0${minute}:10.000Z`,
    observations: [],
    error: { stage: "browser_capture", message: "LinkedIn readiness failed" },
  }));
  const report = buildBridgeHealth({
    heartbeat: sanitizeHeartbeat(compatibleHeartbeat(), "2026-07-14T01:03:00.000Z"),
    runs: [...failures, successful],
    now: Date.parse("2026-07-14T01:03:01.000Z"),
  });

  assert.equal(report.status, "degraded");
  assert.equal(report.sources.linkedin.status, "unhealthy");
  assert.equal(report.sources.linkedin.lastObservedAt, "2026-07-14T01:00:05.000Z");
  assert.deepEqual(report.sources.linkedin.rolling, {
    windowSize: 5,
    totalRuns: 3,
    completedRuns: 1,
    failedRuns: 2,
    completionRate: 1 / 3,
    consecutiveFailures: 2,
    latestRunStatus: "failed",
    latestRunAt: "2026-07-14T01:02:10.000Z",
  });
});

test("a recovery success does not erase two failures from rolling health", () => {
  const runs = [
    {
      source: "linkedin",
      status: "completed",
      createdAt: "2026-07-14T01:03:00.000Z",
      completedAt: "2026-07-14T01:03:10.000Z",
      observations: [{ payload: {
        capturedAt: "2026-07-14T01:03:05.000Z",
        coverage: { adapterHealth: { state: "healthy" }, restored: true },
      } }],
    },
    ...[1, 2].map((minute) => ({
      source: "linkedin",
      status: "failed",
      createdAt: `2026-07-14T01:0${minute}:00.000Z`,
      completedAt: `2026-07-14T01:0${minute}:10.000Z`,
      observations: [],
    })),
  ];
  const report = buildBridgeHealth({
    heartbeat: sanitizeHeartbeat(compatibleHeartbeat(), "2026-07-14T01:04:00.000Z"),
    runs,
    now: Date.parse("2026-07-14T01:04:01.000Z"),
  });

  assert.equal(report.sources.linkedin.rolling.consecutiveFailures, 0);
  assert.equal(report.sources.linkedin.rolling.completionRate, 1 / 3);
  assert.equal(report.sources.linkedin.status, "unhealthy");
  assert.equal(report.status, "degraded");
});

function compatibleHeartbeat() {
  return {
    extensionVersion: "0.5.39",
    runtimeRevision: "source-fidelity-v41",
    buildId: "aku-bridge-0.5.39-source-fidelity-v41",
    adapterVersions: { x: "x-dom-v15", linkedin: "linkedin-dom-v13" },
    actions: [
      "reload_self",
      "report_capture_quality",
      "probe_freshness",
      "recover_source_freshness",
      "recover_missing_media",
      "manage_capture_window",
      "release_capture_surface",
      "preserve_working_tab",
    ],
  };
}

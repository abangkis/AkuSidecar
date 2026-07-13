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
  assert.equal(compatibility.reasons.length, 6);
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

function compatibleHeartbeat() {
  return {
    extensionVersion: "0.5.17",
    runtimeRevision: "source-fidelity-v19",
    buildId: "aku-bridge-0.5.17-source-fidelity-v19",
    adapterVersions: { x: "x-dom-v12", linkedin: "linkedin-dom-v6" },
    actions: ["reload_self"],
  };
}

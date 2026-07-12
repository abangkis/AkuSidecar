import assert from "node:assert/strict";
import test from "node:test";
import { JobEngine } from "../src/core/job-engine.mjs";

test("bounded timeline retains newest updates and only enough older content to fill capacity", () => {
  const older = sessionFixture("older", 8, "2026-07-12T01:00:00.000Z");
  const newestTen = sessionFixture("newest-ten", 10, "2026-07-12T02:00:00.000Z");
  const engine = engineFixture([newestTen, older]);

  let timeline = engine.getTimelineFeed({ capacity: 12, limit: 12 });
  assert.equal(timeline.entries.length, 12);
  assert.equal(timeline.entries.filter((entry) => entry.sessionId === "newest-ten").length, 10);
  assert.equal(timeline.entries.filter((entry) => entry.sessionId === "older").length, 2);
  assert.equal(timeline.summary.latestAdditions, 10);

  const newestEight = sessionFixture("newest-eight", 8, "2026-07-12T03:00:00.000Z");
  timeline = engineFixture([newestEight, newestTen, older])
    .getTimelineFeed({ capacity: 12, limit: 12 });
  assert.equal(timeline.entries.filter((entry) => entry.sessionId === "newest-eight").length, 8);
  assert.equal(timeline.entries.filter((entry) => entry.sessionId === "newest-ten").length, 4);
  assert.equal(timeline.entries.some((entry) => entry.sessionId === "older"), false);
  assert.equal(timeline.summary.latestAdditions, 8);
});

test("latest completed check reports zero additions without erasing the retained timeline", () => {
  const emptyLatest = sessionFixture("empty-latest", 0, "2026-07-12T04:00:00.000Z");
  const older = sessionFixture("older", 5, "2026-07-12T03:00:00.000Z");
  const timeline = engineFixture([emptyLatest, older])
    .getTimelineFeed({ capacity: 12, limit: 12 });
  assert.equal(timeline.summary.latestAdditions, 0);
  assert.equal(timeline.summary.latestSessionId, "empty-latest");
  assert.equal(timeline.entries.length, 5);
});

test("timeline details are paged inside the configured rolling capacity", () => {
  const timeline = engineFixture([sessionFixture("latest", 12, "2026-07-12T03:00:00.000Z")])
    .getTimelineFeed({ capacity: 12, limit: 5, offset: 5 });
  assert.equal(timeline.pagination.total, 12);
  assert.equal(timeline.pagination.returned, 5);
  assert.equal(timeline.pagination.hasNext, true);
  assert.deepEqual(
    timeline.entries.map((entry) => entry.item.evidenceKey),
    ["x:latest-5", "x:latest-6", "x:latest-7", "x:latest-8", "x:latest-9"],
  );
});

function engineFixture(sessions) {
  return new JobEngine({
    store: {
      listPresentableUnifiedSessions() {
        return sessions;
      },
    },
    reasoningProvider: { name: "timeline-test-provider" },
    limits: {},
  });
}

function sessionFixture(id, itemCount, completedAt) {
  const run = {
    id: `${id}-run`,
    mode: "catch_up",
    source: "x",
    intent: "Default bounded catch-up intent.",
    status: "completed",
    completedAt,
    result: { items: [] },
    feedback: [],
    candidateEvaluations: [],
    preferenceFeedback: [],
    reasoningInvocations: [],
  };
  const items = Array.from({ length: itemCount }, (_, index) => ({
    runId: run.id,
    item: {
      id: `${id}-item-${index}`,
      source: "x",
      evidenceKey: `x:${id}-${index}`,
      priority: "P2",
      whatChanged: `${id} update ${index}`,
      whyItMatters: "Timeline fixture.",
      sourceUrl: `https://x.com/example/status/${id}-${index}`,
      confidence: 0.8,
      evidenceState: "primary",
    },
  }));
  run.result.items = items.map((entry) => entry.item);
  return {
    id,
    mode: "catch_up",
    status: "completed",
    completedAt,
    result: { items },
    children: [{ source: "x", runId: run.id, status: "completed", run }],
  };
}

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
  assert.equal(timeline.entries.filter((entry) => entry.isLatestAddition).length, 10);

  const newestEight = sessionFixture("newest-eight", 8, "2026-07-12T03:00:00.000Z");
  timeline = engineFixture([newestEight, newestTen, older])
    .getTimelineFeed({ capacity: 12, limit: 12 });
  assert.equal(timeline.entries.filter((entry) => entry.sessionId === "newest-eight").length, 8);
  assert.equal(timeline.entries.filter((entry) => entry.sessionId === "newest-ten").length, 4);
  assert.equal(timeline.entries.some((entry) => entry.sessionId === "older"), false);
  assert.equal(timeline.summary.latestAdditions, 8);
  assert.equal(timeline.entries.filter((entry) => entry.isLatestAddition).length, 8);
});

test("latest completed check reports zero additions without erasing the retained timeline", () => {
  const emptyLatest = sessionFixture("empty-latest", 0, "2026-07-12T04:00:00.000Z");
  const older = sessionFixture("older", 5, "2026-07-12T03:00:00.000Z");
  const timeline = engineFixture([emptyLatest, older])
    .getTimelineFeed({ capacity: 12, limit: 12 });
  assert.equal(timeline.summary.latestAdditions, 0);
  assert.equal(timeline.summary.latestSessionId, "empty-latest");
  assert.equal(timeline.summary.latestTerminalSessionId, "empty-latest");
  assert.equal(timeline.entries.length, 5);
  assert.equal(timeline.entries.some((entry) => entry.isLatestAddition), false);
});

test("later LinkedIn permalink recovery replaces the older fallback identity", () => {
  const older = linkedinSessionFixture({
    id: "linkedin-fallback",
    evidenceKey: "linkedin:fallback-hash",
    completedAt: "2026-07-12T03:00:00.000Z",
    sourceUrl: "https://www.linkedin.com/feed/",
    text: "Kargo Technologies is coming to Singapore. And I need one intern crazy enough to help me build it from zero.This is a 0-to-1 role. You'll work directly with me, coordinating our fleet operations and commercial partnerships as we bring the electric revolution to Singapore. Real ownership, real problems, real impact. I read every single application.Tag anyone you think I should meet!",
  });
  const enriched = linkedinSessionFixture({
    id: "linkedin-native",
    evidenceKey: "linkedin:native-permalink",
    completedAt: "2026-07-12T04:00:00.000Z",
    sourceUrl: "https://www.linkedin.com/feed/update/urn:li:share:7482432821714767872/",
    text: "Kargo Technologies is coming to Singapore. And I need one intern crazy enough to help me build it from zero.\n\nThis is a 0-to-1 role. You'll work directly with me, coordinating our fleet operations and commercial partnerships as we bring the electric revolution to Singapore.\n\nReal ownership, real problems, real impact. I read every single application.\n\nTag anyone you think I should meet!",
  });

  const timeline = engineFixture([enriched, older])
    .getTimelineFeed({ capacity: 12, limit: 12 });

  assert.equal(timeline.entries.length, 1);
  assert.equal(timeline.entries[0].sessionId, "linkedin-native");
  assert.equal(timeline.entries[0].item.evidenceKey, "linkedin:native-permalink");
  assert.equal(timeline.entries[0].item.sourceUrl, enriched.result.items[0].item.sourceUrl);
  assert.equal(timeline.summary.latestAdditions, 0);
  assert.deepEqual(timeline.summary.sources, { x: 0, linkedin: 1 });
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

function linkedinSessionFixture({ id, evidenceKey, completedAt, sourceUrl, text }) {
  const run = {
    id: `${id}-run`,
    mode: "catch_up",
    source: "linkedin",
    intent: "Default bounded LinkedIn catch-up intent.",
    status: "completed",
    completedAt,
    result: { items: [] },
    feedback: [],
    candidateEvaluations: [{
      evidenceKey,
      source: "linkedin",
      author: "⚡️🚚Tiger Fang",
      text,
      sourceUrl,
      presentation: {},
    }],
    preferenceFeedback: [],
    reasoningInvocations: [],
  };
  const item = {
    id: `${id}-item`,
    source: "linkedin",
    author: "⚡️🚚Tiger Fang",
    evidenceKey,
    priority: "P2",
    whatChanged: "Kargo Technologies is expanding to Singapore.",
    whyItMatters: "Timeline identity fixture.",
    sourceUrl,
    confidence: 0.9,
    evidenceState: "primary",
  };
  run.result.items = [item];
  return {
    id,
    mode: "catch_up",
    status: "completed",
    completedAt,
    result: { items: [{ runId: run.id, item }] },
    children: [{ source: "linkedin", runId: run.id, status: "completed", run }],
  };
}

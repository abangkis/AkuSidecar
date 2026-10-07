import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { formatTimelineCheckSummary } from "../internal/httpapi/web/timeline-check-summary.js";

const app = readFileSync(new URL("../internal/httpapi/web/app.js", import.meta.url), "utf8");

function diagnostics(overrides = {}) {
  return {
    capturedCandidates: 0,
    skippedResurfaces: 0,
    evaluatedCandidates: 0,
    failedCaptureRuns: 0,
    failedReasoningRuns: 0,
    failedPlanningRuns: 0,
    planningFallbackRuns: 0,
    ...overrides,
  };
}

test("summarizes each unsuccessful latest-check outcome with its stage and evidence", () => {
  assert.match(formatTimelineCheckSummary({
    outcome: "planning_failed",
    diagnostics: diagnostics({ capturedCandidates: 3, failedPlanningRuns: 1 }),
  }), /1 planning run failed before evaluation · 3 candidates captured/);

  assert.match(formatTimelineCheckSummary({
    outcome: "reasoning_failed",
    diagnostics: diagnostics({ capturedCandidates: 4, evaluatedCandidates: 2, failedReasoningRuns: 1 }),
  }), /1 evaluation run failed · 2 candidates evaluated/);

  assert.match(formatTimelineCheckSummary({
    outcome: "capture_failed",
    diagnostics: diagnostics({ failedCaptureRuns: 2 }),
  }), /2 source capture runs failed/);

  assert.equal(formatTimelineCheckSummary({
    outcome: "empty_capture",
    diagnostics: diagnostics(),
  }), "No candidates were captured");
});

test("distinguishes cooldown from a completed evaluation with no selection", () => {
  assert.equal(formatTimelineCheckSummary({
    outcome: "unchanged",
    diagnostics: diagnostics({ capturedCandidates: 2, skippedResurfaces: 2 }),
  }), "No new items · 2 unchanged candidates stayed in cooldown");

  assert.equal(formatTimelineCheckSummary({
    outcome: "no_selected_items",
    diagnostics: diagnostics({ capturedCandidates: 3, evaluatedCandidates: 3 }),
  }), "3 candidates evaluated; none were selected");

  const duplicateOnly = formatTimelineCheckSummary({
    outcome: "duplicate_reports_only",
    duplicateReports: 2,
    diagnostics: diagnostics(),
  });
  assert.match(duplicateOnly, /reports matched events already represented in the timeline/i);
  assert.doesNotMatch(duplicateOnly, /none were selected/i);
});

test("shows additions with partial failures and optional-planning fallback", () => {
  const summary = formatTimelineCheckSummary({
    addedItems: 2,
    outcome: "added",
    diagnostics: diagnostics({
      capturedCandidates: 5,
      failedCaptureRuns: 1,
      failedPlanningRuns: 1,
      failedReasoningRuns: 2,
      planningFallbackRuns: 1,
    }),
  });

  assert.equal(summary, "2 new items from 5 captured candidates · 1 capture run failed · 1 planning run failed · 1 evaluation run failed · Optional planning was skipped in 1 run; evaluation continued");
  assert.doesNotMatch(summary, /planning_failed|reasoning_failed|provider|candidate id/i);
});

test("does not count planning failures as failed evaluation runs", () => {
  const mixedFailures = formatTimelineCheckSummary({
    addedItems: 1,
    outcome: "added",
    diagnostics: diagnostics({ capturedCandidates: 4, failedPlanningRuns: 1, failedReasoningRuns: 2 }),
  });
  assert.match(mixedFailures, /1 planning run failed/);
  assert.match(mixedFailures, /1 evaluation run failed/);
  assert.doesNotMatch(mixedFailures, /2 evaluation runs failed/);

  const planningOnly = formatTimelineCheckSummary({
    addedItems: 1,
    outcome: "added",
    diagnostics: diagnostics({ capturedCandidates: 4, failedPlanningRuns: 1, failedReasoningRuns: 1 }),
  });
  assert.match(planningOnly, /1 planning run failed/);
  assert.doesNotMatch(planningOnly, /evaluation run failed/);
});

test("keeps legacy payloads generic and wires the formatter into timeline metadata", () => {
  assert.equal(formatTimelineCheckSummary({ addedItems: 0, outcome: "empty_capture" }), "No new items");
  assert.equal(formatTimelineCheckSummary({ addedItems: 2 }), "2 new items");

  assert.match(app, /import \{ formatTimelineCheckSummary \} from "\.\/timeline-check-summary\.js"/);
  assert.match(app, /const parts = \[formatTimelineCheckSummary\(latestCheck\)\]/);
  assert.match(app, /if \(duplicates\) parts\.push\(`\$\{duplicates\} duplicate report/);
  assert.match(app, /if \(routed\.drawer\.length\) parts\.push\(`\$\{routed\.drawer\.length\} in AI Signals`\)/);
});

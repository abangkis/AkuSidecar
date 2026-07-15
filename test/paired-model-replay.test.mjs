import assert from "node:assert/strict";
import test from "node:test";
import {
  parsePairedModelReport,
  runPairedModelReplay,
  selectPairedReplayCases,
} from "../src/core/paired-model-replay.mjs";

test("paired replay invokes every profile over the exact same stored observations", async () => {
  const cases = [replayCase("run-x", "x", "x:0123456789abcdef01234567", "more_like_this")];
  const seen = [];
  const report = await runPairedModelReplay({
    cases,
    profiles: profiles(),
    lunaCostRatios: [0.25, 0.5],
    generatedAt: "2026-07-15T00:00:00.000Z",
    async invoke({ profile, run, observation }) {
      seen.push({ profileId: profile.id, runId: run.id, observation });
      return invocation(profile, observation, profile.id !== "luna_xhigh", profile.id === "terra_high" ? 1_000 : 1_200);
    },
  });

  assert.deepEqual(seen.map((entry) => entry.profileId), ["terra_high", "luna_high", "luna_xhigh"]);
  assert.ok(seen.every((entry) => entry.runId === "run-x"));
  assert.ok(seen.every((entry) => entry.observation === cases[0].observation));
  assert.equal(report.mutatesTimeline, false);
  assert.equal(report.mutatesPreferenceRuntime, false);
  assert.equal(report.caseResults.length, 3);
  assert.deepEqual(report.caseResults[0].selectedKeys, ["x:0123456789abcdef01234567"]);
  assert.doesNotMatch(JSON.stringify(report), /Evidence run-x/);
  assert.equal(report.profiles[0].feedback.agreement, 1);
  assert.equal(report.profiles[2].feedback.agreement, 1);
  assert.equal(report.cost.breakEven.luna_high.breakEvenRelativeRate, 1_000 / 1_200);
  assert.equal(report.cost.scenarios[0].profiles.luna_high.savingsVsTerra, 0.7);
  assert.equal(report.routing.confidence, "insufficient");
  assert.equal(parsePairedModelReport(JSON.stringify(report)).generatedAt, report.generatedAt);
});

test("paired replay reports a provisional lowest-cost recommendation inside the quality band", async () => {
  const cases = [
    replayCase("run-x-1", "x", "x:0123456789abcdef01234567", "more_like_this"),
    replayCase("run-li-1", "linkedin", "linkedin:1123456789abcdef01234567", "less_like_this"),
    replayCase("run-x-2", "x", "x:2123456789abcdef01234567", "more_like_this"),
    replayCase("run-li-2", "linkedin", "linkedin:3123456789abcdef01234567", "less_like_this"),
  ];
  const report = await runPairedModelReplay({
    cases,
    profiles: profiles(),
    lunaCostRatios: [0.5],
    async invoke({ profile, observation }) {
      const kind = cases.find((entry) => entry.observation === observation).routineFeedback[0].kind;
      const selected = kind === "more_like_this";
      return invocation(profile, observation, selected, profile.id === "terra_high" ? 1_000 : 1_100);
    },
  });

  assert.equal(report.routing.confidence, "provisional");
  assert.equal(report.routing.feedbackComparisons, 4);
  assert.equal(report.routing.scenarios[0].profileId, "luna_high");
  assert.equal(report.pairwise.every((entry) => entry.selectionAgreement === 1), true);
});

test("case selection prioritizes routine feedback and alternates sources", () => {
  const runs = [
    replayCase("x-no-feedback", "x", "x:0123456789abcdef01234567").run,
    replayCase("x-feedback", "x", "x:1123456789abcdef01234567", "more_like_this").run,
    replayCase("li-feedback", "linkedin", "linkedin:2123456789abcdef01234567", "less_like_this").run,
    replayCase("li-calibration", "linkedin", "linkedin:3123456789abcdef01234567", "more_like_this", "calibration").run,
  ];
  const selected = selectPairedReplayCases(runs, { limit: 2 });
  assert.deepEqual(selected.map((entry) => entry.run.id), ["x-feedback", "li-feedback"]);
  assert.deepEqual(selected.map((entry) => entry.run.source), ["x", "linkedin"]);
});

function profiles() {
  return [
    { id: "terra_high", model: "gpt-5.6-terra", effort: "high" },
    { id: "luna_high", model: "gpt-5.6-luna", effort: "high" },
    { id: "luna_xhigh", model: "gpt-5.6-luna", effort: "xhigh" },
  ];
}

function replayCase(id, source, evidenceKey, kind = null, origin = "routine") {
  const permalink = source === "x"
    ? `https://x.com/example/status/${id}`
    : `https://www.linkedin.com/feed/update/${id}/`;
  const observation = {
    source,
    pageUrl: source === "x" ? "https://x.com/home" : "https://www.linkedin.com/feed/",
    pageUrls: [source === "x" ? "https://x.com/home" : "https://www.linkedin.com/feed/"],
    snapshots: [{ blocks: [{ evidenceKey, permalink, links: [], text: `Evidence ${id}` }] }],
  };
  const preferenceFeedback = kind ? [{ evidenceKey, kind, origin, reasonCode: null }] : [];
  return {
    run: {
      id,
      source,
      status: "completed",
      maxItems: 1,
      createdAt: `2026-07-15T00:00:0${id.length % 10}.000Z`,
      observations: [{ payload: observation }],
      preferenceFeedback,
    },
    observation,
    routineFeedback: origin === "routine" ? preferenceFeedback : [],
  };
}

function invocation(profile, observation, selected, observedUnits) {
  const block = observation.snapshots[0].blocks[0];
  const score = selected ? 0.9 : 0.1;
  return {
    output: {
      summary: "Paired diagnostic fixture.",
      items: [{
        id: `${profile.id}-item`,
        whatChanged: block.text,
        whyItMatters: "Fixture",
        source: observation.source,
        sourceUrl: block.permalink,
        sourceUrlKind: "native_post",
        evidenceKey: block.evidenceKey,
        eventKey: `${profile.id}-event`,
        knowledgeDelta: "new_event",
        author: "Fixture",
        publishedAt: null,
        confidence: 0.8,
        evidenceState: "primary",
      }],
      candidateAssessments: [{
        evidenceKey: block.evidenceKey,
        topicTags: ["fixture"],
        contentType: "news",
        novelty: score,
        urgency: score,
        actionability: score,
        materiality: score,
        evidenceStrength: score,
      }],
      repeatedClaimsCollapsed: 0,
      deferredByBudget: 0,
      limitations: [],
    },
    evaluatedEvidenceKeys: [block.evidenceKey],
    telemetry: {
      model: profile.model,
      reasoningEffort: profile.effort,
      durationMs: profile.id === "terra_high" ? 1_000 : 900,
      inputTokens: observedUnits - 100,
      outputTokens: 100,
      cachedInputTokens: 0,
      reasoningOutputTokens: 20,
    },
  };
}

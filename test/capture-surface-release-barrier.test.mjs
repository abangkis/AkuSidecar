import test from "node:test";
import assert from "node:assert/strict";
import {
  releaseCompletedSourceSurfaces,
  sourceCaptureSurfaceReleasable,
} from "../internal/httpapi/web/capture-surface-release-barrier.js";

test("candidate evaluation releases its source before the next capture may dispatch", async () => {
  const releasedSources = new Set();
  let confirmRelease;
  const releaseStarted = new Promise((resolve) => {
    confirmRelease = resolve;
  });
  let finishRelease;
  const releasePending = new Promise((resolve) => {
    finishRelease = resolve;
  });
  const barrier = releaseCompletedSourceSurfaces({
    session: {
      id: "session-1",
      status: "running",
      runs: [
        { source: "instagram", status: "reasoning", stage: "candidate_evaluation" },
        { source: "linkedin", status: "waiting_for_bridge", stage: "capture" },
      ],
    },
    releasedSources,
    releaseSource: async (sessionId, source) => {
      assert.equal(sessionId, "session-1");
      assert.equal(source, "instagram");
      confirmRelease();
      await releasePending;
    },
  });

  await releaseStarted;
  let settled = false;
  barrier.then(() => { settled = true; });
  await Promise.resolve();
  assert.equal(settled, false);

  finishRelease();
  assert.deepEqual(await barrier, { ready: true, released: 1, error: null });
  assert.equal(releasedSources.has("session-1:instagram"), true);
});

test("failed release blocks dispatch and remains retryable", async () => {
  const releasedSources = new Set();
  const failure = new Error("release acknowledgement timed out");
  const result = await releaseCompletedSourceSurfaces({
    session: {
      id: "session-2",
      status: "running",
      runs: [{ source: "instagram", status: "completed", stage: "completed" }],
    },
    releasedSources,
    releaseSource: async () => { throw failure; },
  });

  assert.equal(result.ready, false);
  assert.equal(result.error, failure);
  assert.equal(releasedSources.has("session-2:instagram"), false);
});

test("only terminal or candidate-evaluation runs release their source", () => {
  assert.equal(sourceCaptureSurfaceReleasable({ status: "completed" }), true);
  assert.equal(sourceCaptureSurfaceReleasable({ status: "failed" }), true);
  assert.equal(sourceCaptureSurfaceReleasable({ status: "reasoning", stage: "candidate_evaluation" }), true);
  assert.equal(sourceCaptureSurfaceReleasable({ status: "waiting_for_bridge", stage: "capture" }), false);
  assert.equal(sourceCaptureSurfaceReleasable({ status: "reasoning", stage: "acquisition_planning" }), false);
});

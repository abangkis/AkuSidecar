import test from "node:test";
import assert from "node:assert/strict";
import {
  ContractError,
  validateBridgeObservation,
  validateReasoningResult,
  validateRunRequest,
} from "../src/core/contracts.mjs";

const limits = {
  maxItems: 5,
  maxScrolls: 2,
  maxBlocksPerSnapshot: 20,
  maxBlockCharacters: 4_000,
};

test("run requests are bounded", () => {
  const run = validateRunRequest(
    { mode: "catch_up", source: "x", maxItems: 1, scrolls: 0 },
    limits,
  );
  assert.equal(run.mode, "catch_up");
  assert.equal(run.source, "x");
  assert.equal(run.maxItems, 1);
  assert.equal(run.scrolls, 0);

  assert.throws(
    () => validateRunRequest({ mode: "infinite", source: "x" }, limits),
    ContractError,
  );
  assert.throws(
    () => validateRunRequest({ source: "x", scrolls: 99 }, limits),
    /scrolls must be between/,
  );
});

test("browser observations accept only bounded http evidence", () => {
  const observation = validateBridgeObservation(
    {
      source: "x",
      pageUrl: "https://x.com/home",
      capturedAt: "2026-07-10T10:00:00Z",
      snapshots: [
        {
          capturedAt: "2026-07-10T10:00:00Z",
          scrollY: 0,
          viewportHeight: 900,
          blocks: [
            {
              text: "A visible technical update with enough context to be a bounded candidate.",
              permalink: "https://x.com/example/status/1",
              links: [
                { text: "valid", href: "https://example.com/" },
                { text: "invalid", href: "javascript:alert(1)" },
              ],
            },
          ],
        },
      ],
      coverage: { status: "partial", candidateCount: 1 },
    },
    limits,
  );

  assert.equal(observation.snapshots[0].blocks[0].links.length, 1);
  assert.equal(observation.snapshots[0].blocks[0].links[0].href, "https://example.com/");
  assert.equal(observation.coverage.status, "partial");
});

test("reasoning results require source-backed finite items", () => {
  const result = validateReasoningResult(
    {
      summary: "One material item.",
      items: [
        {
          id: "item-1",
          priority: "P1",
          whatChanged: "A release was announced.",
          whyItMatters: "It may change the current development workflow.",
          source: "x",
          sourceUrl: "https://x.com/example/status/1",
          sourceUrlKind: "native_post",
          author: "Example",
          publishedAt: null,
          confidence: 0.8,
          evidenceState: "primary",
        },
      ],
      repeatedClaimsCollapsed: 0,
      deferredByBudget: 0,
      limitations: [],
    },
    1,
  );
  assert.equal(result.items.length, 1);
  assert.equal(result.items[0].priority, "P1");
  assert.equal(result.items[0].sourceUrlKind, "native_post");

  assert.throws(
    () =>
      validateReasoningResult(
        {
          items: [
            {
              priority: "P1",
              sourceUrl: "javascript:alert(1)",
              sourceUrlKind: "native_post",
            },
          ],
        },
        1,
      ),
    /requires a sourceUrl/,
  );

  assert.throws(
    () =>
      validateReasoningResult(
        {
          items: [
            {
              priority: "P1",
              sourceUrl: "https://x.com/example/status/1",
              sourceUrlKind: "not-a-provenance-lane",
            },
          ],
        },
        1,
      ),
    /requires a valid sourceUrlKind/,
  );
});

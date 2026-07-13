import test from "node:test";
import assert from "node:assert/strict";
import {
  ContractError,
  validateAcquisitionPlan,
  validateBridgeObservation,
  validateFeedback,
  validateReasoningResult,
  validateRunRequest,
  validateUnifiedSessionRequest,
} from "../src/core/contracts.mjs";

const limits = {
  maxItems: 5,
  maxScrolls: 2,
  defaultScrolls: 2,
  scrollFraction: 0.75,
  scrollSettleMs: 900,
  captureTimeoutMs: 45_000,
  pendingContentTimeoutMs: 5_000,
  pendingContentSettleMs: 700,
  maxBlocksPerSnapshot: 20,
  maxBlockCharacters: 4_000,
  maxMediaPerBlock: 4,
};

test("empty-result feedback is stored at run level", () => {
  assert.deepEqual(validateFeedback({ kind: "correct_empty" }), {
    kind: "correct_empty",
    itemId: "",
    note: "",
  });
  assert.deepEqual(validateFeedback({ kind: "missed", note: "A release was omitted." }), {
    kind: "missed",
    itemId: "",
    note: "A release was omitted.",
  });
  assert.throws(
    () => validateFeedback({ kind: "missed" }),
    /missed feedback requires a note/,
  );
});

test("Gate 0B.3 acquisition plans expose only a finite decision", () => {
  assert.deepEqual(
    validateAcquisitionPlan({
      decision: "request_follow_up",
      reason: "One adjacent viewport can resolve a concrete evidence gap.",
    }),
    {
      decision: "request_follow_up",
      reason: "One adjacent viewport can resolve a concrete evidence gap.",
    },
  );
  assert.throws(
    () => validateAcquisitionPlan({ decision: "open_url", reason: "Try another page." }),
    /unsupported acquisition decision/,
  );
});

test("run requests are bounded", () => {
  const run = validateRunRequest(
    { mode: "catch_up", source: "x", maxItems: 1, scrolls: 0 },
    limits,
  );
  assert.equal(run.mode, "catch_up");
  assert.equal(run.source, "x");
  assert.equal(run.maxItems, 1);
  assert.equal(run.scrolls, 0);

  const defaultRun = validateRunRequest({ mode: "catch_up", source: "x" }, limits);
  assert.equal(defaultRun.scrolls, 2);

  assert.throws(
    () => validateRunRequest({ mode: "infinite", source: "x" }, limits),
    ContractError,
  );
  assert.throws(
    () => validateRunRequest({ source: "x", scrolls: 99 }, limits),
    /scrolls must be between/,
  );
});

test("unified session requests preserve canonical order for any active source subset", () => {
  const session = validateUnifiedSessionRequest(
    { mode: "catch_up", maxItemsPerSource: 5, intent: "Material engineering changes." },
    limits,
  );
  assert.deepEqual(session.sources, ["x", "linkedin"]);
  assert.equal(session.maxItemsPerSource, 5);
  assert.equal(session.maxItemsTotal, 10);
  const xOnly = validateUnifiedSessionRequest({ sources: ["x"] }, limits);
  assert.deepEqual(xOnly.sources, ["x"]);
  assert.equal(xOnly.maxItemsTotal, 5);
  assert.throws(
    () => validateUnifiedSessionRequest({ sources: ["linkedin", "x"] }, limits),
    /ordered non-empty subset/,
  );
  assert.throws(
    () => validateUnifiedSessionRequest({ sources: [] }, limits),
    /ordered non-empty subset/,
  );
  assert.throws(
    () => validateUnifiedSessionRequest({ maxItemsPerSource: 6 }, limits),
    /maxItemsPerSource must be between/,
  );
});

test("Gate 0B observations preserve bounded movement and platform-order evidence", () => {
  const observation = validateBridgeObservation(
    {
      source: "linkedin",
      pageUrl: "https://www.linkedin.com/feed/",
      capturedAt: "2026-07-10T10:00:00Z",
      snapshots: [
        gate0bSnapshot(0, 0, 1),
        gate0bSnapshot(1, 675, 2),
        gate0bSnapshot(2, 1_350, 3),
      ],
      coverage: {
        status: "partial",
        checkedThrough: "2026-07-10T10:00:05Z",
        candidateCount: 3,
        observedBlockCount: 3,
        browserAdapter: "aku-bridge",
        captureMethod: "native_dom",
        adapterVersion: "linkedin-dom-v2",
        adapterCapabilities: [
          {
            source: "linkedin",
            version: "linkedin-dom-v2",
            actions: ["probe_readiness", "collect_visible", "collect_visible"],
          },
        ],
        adapterHealth: {
          state: "healthy",
          strategies: ["[data-view-name=\"feed-full-update\"]"],
          selectorCounts: { "[data-view-name=\"feed-full-update\"]": 8 },
          fieldCoverage: { publishedAt: { present: 2, total: 3 } },
          domSignature: "linkedin-dom-v2:8:3",
        },
        frontier: {
          scrollY: 1_350,
          anchorKeys: ["urn:li:activity:2", "urn:li:activity:3"],
          newCandidateCount: 1,
          hasMoreCandidateSignal: true,
        },
        sourceEvents: [
          { type: "source_new_content_available", state: "activated", label: "New posts" },
        ],
        fallbackUsed: false,
        scrollContainer: "#workspace",
        pendingNewContent: true,
        pendingNewContentLabel: "New posts",
        pendingNewContentAction: "activated",
        pendingContentActivationEvidence: "feed_fingerprint_changed",
        pendingContentPolicy: "reveal_if_present",
        sourceReadinessState: "feed_ready",
        sourceReadinessWaitMs: 1_250,
        sourceSelectorCandidateCount: 8,
        sourceVisibleSelectorCandidateCount: 3,
        sourceLoadingIndicator: false,
        sourceFeedRootPresent: true,
        sourceTabOpened: true,
        sourceTabActivatedForReadiness: true,
        sourceTabBackgroundAtDispatch: true,
        sourceTabRecoveryCount: 1,
        sourceTabOwnership: "managed",
        sourceTabOpenedDisposition: "preserve",
        sourceTabClosedAfterCapture: false,
        sourceReadinessRetryCount: 1,
        feedMutation: true,
        sameTabMutation: true,
        restorationScope: "post_reveal_start",
        preActionScrollY: 1_024,
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
      },
    },
    limits,
  );

  assert.equal(observation.snapshots.length, 3);
  assert.equal(observation.snapshots[2].blocks[0].feedPosition, 3);
  assert.equal(observation.snapshots[1].blocks[0].relationshipType, "repost");
  assert.equal(observation.snapshots[2].blocks[0].contentKind, "document");
  assert.equal(observation.snapshots[0].blocks[0].engagement.like, "42");
  assert.equal(observation.coverage.performedScrolls, 2);
  assert.equal(observation.coverage.restored, true);
  assert.equal(observation.coverage.scrollContainer, "#workspace");
  assert.equal(observation.coverage.adapterVersion, "linkedin-dom-v2");
  assert.deepEqual(observation.coverage.adapterCapabilities, [
    {
      source: "linkedin",
      version: "linkedin-dom-v2",
      actions: ["probe_readiness", "collect_visible"],
    },
  ]);
  assert.equal(observation.coverage.adapterHealth.state, "healthy");
  assert.equal(observation.coverage.frontier.hasMoreCandidateSignal, true);
  assert.equal(observation.coverage.sourceEvents[0].type, "source_new_content_available");
  assert.equal(observation.coverage.sourceTabOwnership, "managed");
  assert.equal(observation.coverage.pendingNewContent, true);
  assert.equal(observation.coverage.pendingNewContentAction, "activated");
  assert.equal(observation.coverage.pendingContentActivationEvidence, "feed_fingerprint_changed");
  assert.equal(observation.coverage.feedMutation, true);
  assert.equal(observation.coverage.sourceReadinessState, "feed_ready");
  assert.equal(observation.coverage.sourceSelectorCandidateCount, 8);
  assert.equal(observation.coverage.sourceVisibleSelectorCandidateCount, 3);
  assert.equal(observation.coverage.sourceTabActivatedForReadiness, true);
  assert.equal(observation.coverage.sourceTabRecoveryCount, 1);
  assert.equal(observation.coverage.sourceReadinessRetryCount, 1);
  assert.equal(observation.coverage.restorationScope, "post_reveal_start");
  assert.deepEqual(observation.coverage.scrollDeltas, [675, 675]);
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
              text: "A visible technical update.\n\n1 First item\n2 Second item",
              permalink: "https://x.com/example/status/1",
              media: [
                { kind: "image", url: "https://pbs.twimg.com/media/example.jpg#fragment", alt: "Architecture diagram", width: 640, height: 360 },
                { kind: "image", url: "https://evil.example/tracker.png", width: 640, height: 360 },
              ],
              presentation: {
                socialContext: "Reza Lesmana likes this",
                socialContextAvatarUrl: "https://media.licdn.com/dms/image/context-avatar",
                headline: "Cybersecurity Leader | Executive",
                attributionText: "with Cassie Dell · Promoted · Partnership with LinkedIn",
                connectionDegree: "2nd",
                timestampText: "12h · Edited",
                edited: true,
                attachment: {
                  kind: "job",
                  title: "Management Intern",
                  subtitle: "Kargo Technologies",
                  detail: "Singapore (On-site)",
                  actionLabel: "View job",
                  footnote: "10 school alumni work here",
                  url: "https://www.linkedin.com/jobs/view/4439405587/",
                  imageUrl: "https://media.licdn.com/dms/image/job-logo",
                  verified: true,
                },
              },
              relationshipType: "quote",
              quotedPost: {
                author: "Ian Bremmer @ianbremmer · 18h",
                avatarUrl: "https://pbs.twimg.com/profile_images/ian-avatar.jpg",
                text: "A quoted post body.\n\nIts second paragraph remains distinct.",
                permalink: "https://x.com/ianbremmer/status/2076000000000000000",
                publishedAt: "2026-07-13T00:00:00.000Z",
                links: [{ text: "source", href: "https://example.com/quote" }],
                media: [],
              },
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
  assert.equal(observation.snapshots[0].blocks[0].relationshipType, "quote");
  assert.equal(
    observation.snapshots[0].blocks[0].text,
    "A visible technical update.\n\n1 First item\n2 Second item",
  );
  assert.equal(
    observation.snapshots[0].blocks[0].quotedPost.text,
    "A quoted post body.\n\nIts second paragraph remains distinct.",
  );
  assert.deepEqual(observation.snapshots[0].blocks[0].media, [{
    kind: "image",
    url: "https://pbs.twimg.com/media/example.jpg",
    posterUrl: null,
    playbackUrl: null,
    playbackMode: null,
    alt: "Architecture diagram",
    width: 640,
    height: 360,
  }]);
  assert.deepEqual(observation.snapshots[0].blocks[0].presentation, {
    socialContext: "Reza Lesmana likes this",
    socialContextAvatarUrl: "https://media.licdn.com/dms/image/context-avatar",
    headline: "Cybersecurity Leader | Executive",
    attributionText: "with Cassie Dell · Promoted · Partnership with LinkedIn",
    connectionDegree: "2nd",
    timestampText: "12h · Edited",
    edited: true,
    promoted: false,
    permalinkSource: "",
    permalinkReason: "",
    contentExpansion: "",
    attachment: {
      kind: "job",
      title: "Management Intern",
      subtitle: "Kargo Technologies",
      detail: "Singapore (On-site)",
      actionLabel: "View job",
      footnote: "10 school alumni work here",
      url: "https://www.linkedin.com/jobs/view/4439405587/",
      imageUrl: "https://media.licdn.com/dms/image/job-logo",
      verified: true,
    },
  });
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
          evidenceKey: "x:0123456789abcdef01234567",
          eventKey: "openai-example-release",
          knowledgeDelta: "new_event",
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
  assert.equal(result.items[0].evidenceKey, "x:0123456789abcdef01234567");
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

function gate0bSnapshot(index, scrollY, feedPosition) {
  return {
    index,
    adapterVersion: "linkedin-dom-v2",
    selectorCandidateCount: 8,
    visibleContainerCount: 1,
    newCandidateCount: 1,
    capturedAt: `2026-07-10T10:00:0${index}Z`,
    scrollY,
    viewportHeight: 900,
    blocks: [
      {
        text: `Visible professional update ${index} with enough detail for bounded evidence.`,
        author: "Example",
        permalink: `https://www.linkedin.com/feed/update/urn:li:activity:${index}`,
        contentKind: index === 2 ? "document" : "post",
        relationshipType: index === 1 ? "repost" : "original",
        parentPermalink: index === 1
          ? "https://www.linkedin.com/feed/update/urn:li:activity:0"
          : null,
        engagement: { like: "42", comment: "3" },
        feedPosition,
        links: [],
      },
    ],
  };
}

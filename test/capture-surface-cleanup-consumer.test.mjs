import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";
import { isVerifiedSurfaceCleanupReceipt } from "../internal/httpapi/web/capture-surface-cleanup-receipt.js";
import { releaseCompletedSourceSurfaces } from "../internal/httpapi/web/capture-surface-release-barrier.js";
import { isVerifiedSurfaceCleanupReceipt as isVerifiedBridgeCleanupReceipt } from "../../AkuBridge/surface-cleanup-receipt.js";

const appSource = readFileSync(new URL("../internal/httpapi/web/app.js", import.meta.url), "utf8");
const workerSource = readFileSync(new URL("../../AkuBridge/service-worker.js", import.meta.url), "utf8");

function extractRegion(source, startMarker, endMarker) {
  const start = source.indexOf(startMarker);
  const end = source.indexOf(endMarker, start + startMarker.length);
  if (start < 0 || end < 0) throw new Error(`Could not extract ${startMarker}`);
  return source.slice(start, end);
}

function cleanupReceipt(leaseId, source = null, overrides = {}) {
  return {
    schema: "aku.surface-cleanup.v1",
    leaseId,
    source,
    expected: 0,
    closed: 0,
    transferred: 0,
    retained: 0,
    pending: 0,
    complete: true,
    ...overrides,
  };
}

function createFakeWindow(nextOutcome) {
  const listeners = new Set();
  let timerID = 0;
  const fakeWindow = {
    setTimeout() { timerID += 1; return timerID; },
    clearTimeout() {},
    addEventListener(type, listener) { if (type === "message") listeners.add(listener); },
    removeEventListener(type, listener) { if (type === "message") listeners.delete(listener); },
    postMessage(message, origin) {
      const outcome = nextOutcome(message);
      queueMicrotask(() => {
        const data = {
          type: "AKU_BROWSER_CAPTURE_SURFACE_RELEASED",
          leaseId: message.leaseId,
        };
        if (message.source !== null) data.source = message.source;
        if (outcome !== undefined) data.outcome = outcome;
        for (const listener of [...listeners]) listener({ source: fakeWindow, origin, data });
      });
    },
  };
  return { window: fakeWindow, listenerCount: () => listeners.size };
}

const appCleanupFunctions = [
  extractRegion(appSource, "async function cancelSession()", "function startPolling()"),
  extractRegion(appSource, "function stopPolling()", "async function pollSession()"),
  extractRegion(appSource, "async function pollSession()", "async function releaseCaptureSurface("),
  extractRegion(appSource, "async function releaseCaptureSurface(", "function releaseCaptureSurfaceOnce("),
  extractRegion(appSource, "function releaseCaptureSurfaceOnce(", "function dispatch("),
].join("\n");

function createAppHarness({ nextOutcome, session, initialPoller = null }) {
  const state = {
    session,
    poller: initialPoller,
    pollInFlight: false,
    captureCleanupPending: null,
    dispatchKey: "queued-run",
    dispatchRetryAfter: new Map(),
    releasedCaptureSources: new Set(),
  };
  const calls = { clearedIntervals: [], startPolling: 0, api: [], warnings: [] };
  const fakeBrowserWindow = createFakeWindow(nextOutcome);
  const context = {
    state,
    terminalStatuses: new Set(["completed", "partial", "failed", "cancelled"]),
    endpoint: "http://127.0.0.1:11122",
    window: fakeBrowserWindow.window,
    isVerifiedSurfaceCleanupReceipt,
    releaseCompletedSourceSurfaces,
    api: async (path, options) => {
      calls.api.push({ path, options });
      return { session };
    },
    renderSession() {},
    renderAutoUpdateStatus() {},
    refreshTimeline: async () => {},
    showSessionOutcome() {},
    showSessionFailure() {},
    schedulePassiveMediaEnrichment() {},
    startPendingFirstCalibration: async () => null,
    requestAnimationFrame() {},
    $(selector) { return { scrollIntoView() {} }; },
    dispatch() {},
    showError(error) { throw error; },
    console: { warn(...args) { calls.warnings.push(args); } },
    clearInterval(id) { calls.clearedIntervals.push(id); },
    setInterval() { return "fake-poller"; },
    setTimeout(callback) { queueMicrotask(callback); return 1; },
    clearTimeout() {},
    startPolling() { calls.startPolling += 1; state.poller = "restarted-poller"; },
  };
  vm.runInNewContext(appCleanupFunctions, context, { filename: "AkuSidecar app cleanup functions" });
  return { context, state, calls, listenerCount: fakeBrowserWindow.listenerCount };
}

test("actual app releaseCaptureSurfaceOnce rejects missing, pending, and cross-lease acknowledgements", async () => {
  const cases = [
    undefined,
    { cleanup: cleanupReceipt("lease-1", null, { expected: 1, pending: 1, complete: false }) },
    { cleanup: cleanupReceipt("different-lease") },
    { cleanup: cleanupReceipt("lease-1", "x") },
  ];
  for (const outcome of cases) {
    const harness = createAppHarness({ nextOutcome: () => outcome, session: null });
    await assert.rejects(harness.context.releaseCaptureSurfaceOnce("lease-1", null), /verified capture-surface cleanup receipt/);
    assert.equal(harness.listenerCount(), 0);
  }
});

test("actual app pollSession keeps a terminal session polling until cleanup verifies", async () => {
  const session = { id: "lease-1", status: "completed", trigger: "scheduled", delivery: "timeline", runs: [] };
  let outcome = { cleanup: cleanupReceipt("lease-1", null, { expected: 1, pending: 1, complete: false }) };
  const harness = createAppHarness({ nextOutcome: () => outcome, session, initialPoller: "poller-1" });

  await harness.context.pollSession();
  assert.equal(harness.state.session.id, "lease-1");
  assert.equal(harness.state.captureCleanupPending, "lease-1");
  assert.equal(harness.state.poller, "poller-1");
  assert.deepEqual(harness.calls.clearedIntervals, []);

  outcome = { cleanup: cleanupReceipt("lease-1") };
  await harness.context.pollSession();
  assert.equal(harness.state.session, null);
  assert.equal(harness.state.captureCleanupPending, null);
  assert.equal(harness.state.poller, null);
  assert.deepEqual(harness.calls.clearedIntervals, ["poller-1"]);
});

test("actual app cancelSession preserves the cancelled session and restarts polling on unverified cleanup", async () => {
  const session = { id: "lease-2", status: "cancelled", trigger: "user", delivery: "timeline", runs: [] };
  const harness = createAppHarness({
    nextOutcome: () => ({ cleanup: cleanupReceipt("lease-2", null, { expected: 1, pending: 1, complete: false }) }),
    session,
    initialPoller: "poller-2",
  });

  await harness.context.cancelSession();
  assert.equal(harness.state.session.status, "cancelled");
  assert.equal(harness.state.captureCleanupPending, "lease-2");
  assert.equal(harness.state.poller, "restarted-poller");
  assert.equal(harness.calls.startPolling, 1);
  assert.deepEqual(harness.calls.clearedIntervals, ["poller-2"]);
});

function createUIElement() {
  const classes = new Set(["hidden"]);
  return {
    classList: {
      toggle(name, force) {
        if (force) classes.add(name);
        else classes.delete(name);
      },
      contains(name) { return classes.has(name); },
      add(name) { classes.add(name); },
      remove(name) { classes.delete(name); },
    },
    style: {},
    parentElement: { setAttribute() {} },
    setAttribute() {},
    removeAttribute() {},
    textContent: "",
    title: "",
    disabled: false,
  };
}

test("actual renderSession exposes pending cleanup and disables new capture actions", () => {
  const uiFunctions = [
    extractRegion(appSource, "function renderSession()", "function updatePolicyBadge("),
    extractRegion(appSource, "function syncRunButtons()", "function renderTimelineCalibrationProgress("),
    extractRegion(appSource, "function preparedBatchDisabledReason(", "function runDisabledReason()"),
    extractRegion(appSource, "function runDisabledReason()", "function bridgeUnavailableReason("),
  ].join("\n");
  const selectors = [
    "#processing-panel", "#processing-mode-badge", "#processing-title", "#processing-detail",
    "#timeline-runner-button", "#timeline-prepared-button", "#done-button", "#timeline-runner-status",
    "#timeline-runner-guidance", "#source-access-setup-button", "#open-reset-learning", "#open-full-reset",
  ];
  const elements = new Map(selectors.map((selector) => [selector, createUIElement()]));
  const state = {
    bootstrap: { autoUpdate: { preparedBatches: [{ sessionId: "prepared-1" }] } },
    bootstrapLoading: false,
    bootstrapError: null,
    session: { id: "lease-1", status: "completed", trigger: "scheduled", delivery: "timeline", runs: [] },
    captureCleanupPending: "lease-1",
    closingNativeReaders: false,
    mediaRecaptureActive: false,
    revealingPreparedBatch: false,
  };
  const context = {
    state,
    terminalStatuses: new Set(["completed", "partial", "failed", "cancelled"]),
    CAPTURE_CLEANUP_PENDING_MESSAGE: "Capture cleanup is pending. Keep related tabs unchanged until ownership is verified.",
    CAPTURE_CLEANUP_PENDING_DETAIL: "Cleanup ownership is unverified. Leave related tabs unchanged while cleanup retries.",
    $(selector) {
      if (!elements.has(selector)) elements.set(selector, createUIElement());
      return elements.get(selector);
    },
    document: { querySelectorAll: () => [], visibilityState: "visible" },
    syncNativePostLinks() {},
    updatePolicyBadge: () => null,
    shouldShowOnboardingLearning: () => false,
    syncOnboardingLearning() {},
    drainPlaybackRecovery() {},
    renderNativeReaderNotice() {},
    renderTimelineCalibrationProgress: () => false,
    sourceAccessNeedsAttention: () => false,
    setSettingsText(element, text) { element.textContent = text; },
  };
  vm.runInNewContext(uiFunctions, context, { filename: "AkuSidecar cleanup UI functions" });

  context.renderSession();
  assert.equal(elements.get("#processing-panel").classList.contains("hidden"), false);
  assert.equal(elements.get("#processing-title").textContent, "Capture cleanup pending");
  assert.match(elements.get("#processing-detail").textContent, /ownership is unverified/);
  assert.doesNotMatch(elements.get("#processing-detail").textContent, /close|remove|delete/i);
  assert.equal(elements.get("#timeline-runner-button").disabled, true);
  assert.equal(elements.get("#timeline-prepared-button").disabled, true);
  assert.equal(elements.get("#done-button").disabled, true);
  assert.equal(context.runDisabledReason(), context.CAPTURE_CLEANUP_PENDING_MESSAGE);
  assert.equal(context.preparedBatchDisabledReason(1), context.CAPTURE_CLEANUP_PENDING_MESSAGE);
});

function createWorkerHarness(releaseOutcomes) {
  const backgroundConfigKey = "background-dispatch-config";
  const storage = {
    [backgroundConfigKey]: {
      endpoint: "http://127.0.0.1:11122",
      token: "t".repeat(40),
      activeLeaseId: "lease-3",
      releasedSources: ["x"],
    },
  };
  const calls = { pendingCommands: 0, release: 0 };
  const functions = [
    extractRegion(workerSource, "async function releaseTerminalBackgroundLease(config)", "async function refreshBackgroundHeartbeat(config)"),
    extractRegion(workerSource, "async function pollBackgroundDispatch()", "function backgroundDispatchStageError("),
  ].join("\n");
  const context = {
    BACKGROUND_DISPATCH_CONFIG_KEY: backgroundConfigKey,
    BACKGROUND_DISPATCH_ALARM: "background-dispatch-alarm",
    backgroundDispatching: false,
    isVerifiedSurfaceCleanupReceipt: isVerifiedBridgeCleanupReceipt,
    bridgeHeaders: () => ({}),
    refreshBackgroundHeartbeat: async () => true,
    managedCaptureWindow: {
      release: async () => {
        calls.release += 1;
        return releaseOutcomes.shift();
      },
    },
    postCaptureSurfaceEvents: async () => {},
    chrome: {
      storage: { local: {
        async get(key) { return { [key]: storage[key] }; },
        async set(values) { Object.assign(storage, values); },
        async remove(key) { delete storage[key]; },
      } },
      alarms: { async clear() {} },
    },
    fetch: async (url) => {
      if (url.includes("/api/sessions/")) {
        return { status: 200, ok: true, async json() { return { session: { status: "completed", runs: [] } }; } };
      }
      if (url.endsWith("/api/bridge/commands/pending")) {
        calls.pendingCommands += 1;
        return { status: 204, ok: false };
      }
      throw new Error(`Unexpected worker fetch ${url}`);
    },
  };
  vm.runInNewContext(functions, context, { filename: "AkuBridge background cleanup functions" });
  return { context, storage, calls, backgroundConfigKey };
}

test("actual Bridge worker retains an incomplete background lease and stops dispatch until verified", async () => {
  const pending = { cleanup: cleanupReceipt("lease-3", null, { expected: 1, pending: 1, complete: false }) };
  const complete = { cleanup: cleanupReceipt("lease-3", null, { expected: 1, closed: 1 }) };
  const harness = createWorkerHarness([pending, complete]);
  const configKey = harness.backgroundConfigKey;

  await harness.context.pollBackgroundDispatch();
  assert.equal(harness.storage[configKey].activeLeaseId, "lease-3");
  assert.deepEqual(Array.from(harness.storage[configKey].releasedSources), ["x"]);
  assert.equal(harness.calls.pendingCommands, 0);

  await harness.context.pollBackgroundDispatch();
  assert.equal(Object.hasOwn(harness.storage[configKey], "activeLeaseId"), false);
  assert.equal(Object.hasOwn(harness.storage[configKey], "releasedSources"), false);
  assert.equal(harness.calls.pendingCommands, 1);
});

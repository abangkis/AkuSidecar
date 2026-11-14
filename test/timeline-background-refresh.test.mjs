import test from "node:test";
import assert from "node:assert/strict";
import { createTimelineBackgroundRefresh } from "../internal/httpapi/web/timeline-background-refresh.js";
import { createTimelineRendering } from "../internal/httpapi/web/timeline-rendering.js";

const copy = value => structuredClone(value);
const settle = async () => { for (let index = 0; index < 20; index++) await Promise.resolve(); };

function fixture() {
  return {
    items: [{ id: "post-1", sessionId: "batch-1", source: "x", item: { whatChanged: "Original report" },
      evidence: { text: "Original text", media: [] },
      aiDetection: { pendingDeep: true, pendingMedia: false, badgeLabel: "AI signal · Checking" } }],
    latestCheck: { sessionId: "batch-1", addedItems: 1, completedAt: "2026-11-14T08:00:00Z" },
    timelineBatches: [{ sessionId: "batch-1", itemCount: 1 }],
    autoUpdate: { preparedBatches: [], dailyTokensUsed: 10 },
  };
}

function harness(t, initial = fixture()) {
  const globals = { document: globalThis.document, window: globalThis.window, clearTimeout: globalThis.clearTimeout };
  t.after(() => Object.assign(globalThis, globals));
  class Element {
    constructor() {
      this.children = []; this.dataset = {}; this.classes = new Set();
      this.classList = { add: name => this.classes.add(name), remove: name => this.classes.delete(name),
        contains: name => this.classes.has(name), toggle: (name, enabled) => enabled ? this.classes.add(name) : this.classes.delete(name) };
    }
    append(...children) { this.children.push(...children); }
    replaceChildren(...children) { this.children = children; }
    setAttribute() {}
    querySelector() { return null; }
    querySelectorAll() { return []; }
    contains() { return false; }
  }
  const nodes = new Map();
  const $ = selector => {
    if (!nodes.has(selector)) nodes.set(selector, new Element());
    return nodes.get(selector);
  };
  $("#timeline-side-pane").classList.add("hidden");
  globalThis.document = { activeElement: null, createElement: () => new Element(), querySelector: () => null };
  const timers = new Map(); let timerID = 0;
  const counts = { fetches: 0, renders: 0, builds: 0, cleanups: 0, restores: 0, status: 0, enrichment: 0 };
  const viewport = { scrollY: 900, setTimeout(fn, delay) { const id = ++timerID; timers.set(id, { fn, delay }); return id; },
    scrollTo({ top }) { this.scrollY = top; counts.restores++; } };
  globalThis.window = viewport;
  globalThis.clearTimeout = id => timers.delete(id);
  const state = {
    bootstrap: { settings: { timelineCapacity: 12, aiDetectionEnabled: true, aiDetectionPresentation: "drawer" },
      latestCheck: initial.latestCheck, autoUpdate: initial.autoUpdate, onboarding: { status: "completed" } },
    currentView: "timeline", timelineItems: [], timelineBatches: [],
    passiveMediaEvidenceAttempts: new Map(), expandedTimelineText: new Set(), expandedAIDetails: new Set(),
    expandedAIFeedbackOptions: new Set(), timelineContentContext: new Map(), seenTimelineItems: new Set(),
    backgroundTimelineRefreshPending: false, backgroundTimelineRefreshInFlight: false,
  };
  let response = copy(initial), failBuild = false, fetchError = null;
  const errors = [], noop = () => {};
  const ui = createTimelineRendering({
    $, state, AI_DEEP_POLL_INTERVAL_MS: 5000,
    api: async () => { counts.fetches++; if (fetchError) throw fetchError; return copy(response); },
    buildTimelineItem: entry => { counts.builds++; if (failBuild) throw Error("Fixture render failed"); const node = new Element(); node.entry = entry; return node; },
    closeTimelineContentContextDrawer: noop, formatDate: String,
    renderAutoUpdateStatus: () => counts.status++, schedulePassiveMediaEnrichment: () => counts.enrichment++,
    getScrollPerformanceTrace: () => ({ measure(_name, work) { counts.renders++; return work(); } }),
    getScrollUIFrames: () => ({ schedule: noop }), showError: error => errors.push(error),
    stopObservingInlineVideoVisibilityWithin: () => counts.cleanups++,
    syncTimelineContentContextTabs: noop, scheduleTimelineContentContextPosition: noop,
    getTimelineScrollIdle: () => ({ wait: async () => {} }),
  });
  return {
    ui, state, counts, timers, errors, viewport,
    cards: () => $("#result-items").children.filter(node => node.dataset.timelineId),
    respond: value => { response = copy(value); }, failBuild: value => { failBuild = value; },
    async poll() {
      const timer = [...timers].find(([, value]) => value.delay === 5000);
      assert.ok(timer, "pending detection must retain a scheduled poll");
      timers.delete(timer[0]); timer[1].fn(); await settle();
    },
  };
}

test("checkpoint ignores object property order, preserves array order, and does not retain mutable payloads", () => {
  const gate = createTimelineBackgroundRefresh();
  const input = { items: [{ id: "one", text: "first" }, { id: "two" }], latestCheck: null };
  const checkpoint = gate.capture(input); assert.equal(gate.isUnchanged(checkpoint), false);
  gate.commit(checkpoint);
  assert.equal(gate.isUnchanged(gate.capture({ latestCheck: null, items: [{ text: "first", id: "one" }, { id: "two" }] })), true);
  input.items[0].text = "changed in place";
  assert.equal(gate.isUnchanged(gate.capture(input)), false);
  input.items[0].text = "first";
  input.items.reverse(); assert.equal(gate.isUnchanged(gate.capture(input)), false);
});

test("identical background responses keep cards, cleanup, and scroll restoration untouched", async t => {
  const h = harness(t); await h.ui.refreshTimeline();
  const card = h.cards()[0], renderedEntry = h.state.timelineItems[0];
  await h.ui.refreshTimeline({ background: true }); await h.ui.refreshTimeline({ background: true });
  assert.equal(h.counts.fetches, 3); assert.equal(h.counts.renders, 1);
  assert.equal(h.counts.cleanups, 1); assert.equal(h.counts.restores, 0);
  assert.equal(h.cards()[0], card); assert.equal(h.state.timelineItems[0], renderedEntry);
  assert.equal(h.counts.status, 3); assert.equal(h.counts.enrichment, 3);
  assert.equal(h.timers.size, 1);
});

test("unchanged pending responses keep polling until fresh detection arrives and then stop", async t => {
  const h = harness(t); await h.ui.refreshTimeline(); const card = h.cards()[0];
  await h.poll(); await h.poll();
  assert.equal(h.counts.renders, 1); assert.equal(h.cards()[0], card);
  const next = fixture(); next.items[0].aiDetection = { pendingDeep: false, pendingMedia: false, badgeLabel: "AI signal · Assessed", detail: "New detection result" };
  h.respond(next); await h.poll();
  assert.equal(h.counts.renders, 2); assert.notEqual(h.cards()[0], card);
  assert.equal(h.cards()[0].entry.aiDetection.detail, "New detection result");
  assert.equal(h.timers.size, 0);
  await h.ui.refreshTimeline({ background: true }); assert.equal(h.counts.renders, 2);
});

test("pending media polling remains alive even without pending text detection", async t => {
  const data = fixture(); data.items[0].aiDetection.pendingDeep = false; data.items[0].aiDetection.pendingMedia = true;
  const h = harness(t, data); await h.ui.refreshTimeline(); await h.poll(); await h.poll();
  assert.equal(h.counts.renders, 1); assert.equal(h.timers.size, 1);
});

test("a new detection result can route a card into the signals drawer", async t => {
  const h = harness(t); await h.ui.refreshTimeline();
  const next = fixture(); next.items[0].aiDetection = { pendingDeep: false, routeToSignals: true, badgeLabel: "AI signal · Strong" };
  h.respond(next); await h.poll();
  assert.equal(h.cards().length, 0); assert.equal(h.state.sidePaneItems[0].id, "post-1");
  assert.equal(h.timers.size, 0);
});

test("effective routing changes are not suppressed when response payloads remain equal", async t => {
  const data = fixture(); data.items[0].aiDetection.routeToSignals = true;
  const h = harness(t, data); await h.ui.refreshTimeline(); assert.equal(h.cards().length, 0);
  h.state.seenTimelineItems.add("post-1"); await h.ui.refreshTimeline({ background: true });
  assert.equal(h.counts.renders, 2); assert.equal(h.cards().length, 1);
});

test("status-only telemetry updates without rebuilding cards", async t => {
  const h = harness(t); await h.ui.refreshTimeline(); const next = fixture(); next.autoUpdate.dailyTokensUsed = 999;
  h.respond(next); await h.ui.refreshTimeline({ background: true });
  assert.equal(h.state.bootstrap.autoUpdate.dailyTokensUsed, 999);
  assert.equal(h.counts.status, 2); assert.equal(h.counts.renders, 1); assert.equal(h.timers.size, 1);
});

test("evidence, check metadata, batches, settings, and prepared count changes remain visible", async t => {
  const h = harness(t); await h.ui.refreshTimeline(); const next = fixture();
  next.items[0].evidence.media.push({ kind: "image", url: "https://example.org/new.jpg" });
  h.respond(next); await h.ui.refreshTimeline({ background: true }); assert.equal(h.counts.renders, 2);
  next.latestCheck.addedItems = 2; h.respond(next); await h.ui.refreshTimeline({ background: true }); assert.equal(h.counts.renders, 3);
  next.timelineBatches[0].itemCount = 2; h.respond(next); await h.ui.refreshTimeline({ background: true }); assert.equal(h.counts.renders, 4);
  h.state.bootstrap.settings.defaultPresentation = "brief";
  await h.ui.refreshTimeline({ background: true }); assert.equal(h.counts.renders, 5);
  next.autoUpdate.preparedBatches = [{ sessionId: "next" }]; h.respond(next);
  await h.ui.refreshTimeline({ background: true }); assert.equal(h.counts.renders, 6);
});

test("foreground refresh still forces a render for identical data", async t => {
  const h = harness(t); await h.ui.refreshTimeline(); await h.ui.refreshTimeline();
  assert.equal(h.counts.renders, 2);
  await h.ui.refreshTimeline({ background: true }); assert.equal(h.counts.renders, 2);
});

test("failed rendering never commits a checkpoint for content that was not displayed", async t => {
  const h = harness(t); h.failBuild(true); await h.ui.refreshTimeline({ background: true });
  assert.equal(h.errors.length, 1); h.failBuild(false);
  await h.ui.refreshTimeline({ background: true });
  assert.equal(h.counts.renders, 2); assert.equal(h.cards().length, 1);
});

test("a partial failed render invalidates the old checkpoint before the old response returns", async t => {
  const h = harness(t); await h.ui.refreshTimeline();
  const changed = fixture(); changed.items[0].item.whatChanged = "Changed report";
  h.respond(changed); h.failBuild(true); await h.ui.refreshTimeline({ background: true });
  assert.equal(h.errors.length, 1); assert.equal(h.cards().length, 0);
  h.respond(fixture()); h.failBuild(false); await h.ui.refreshTimeline({ background: true });
  assert.equal(h.counts.renders, 3); assert.equal(h.cards().length, 1);
  assert.equal(h.cards()[0].entry.item.whatChanged, "Original report");
});

test("a completed detection response cancels an outstanding poll timer", async t => {
  const h = harness(t); await h.ui.refreshTimeline(); assert.equal(h.timers.size, 1);
  const next = fixture(); next.items[0].aiDetection.pendingDeep = false;
  h.respond(next); await h.ui.refreshTimeline({ background: true });
  assert.equal(h.timers.size, 0);
});

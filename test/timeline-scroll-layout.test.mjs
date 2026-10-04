import test from "node:test";
import assert from "node:assert/strict";
import vm from "node:vm";
import { readFileSync } from "node:fs";
import { backToTopHorizontalPosition, createScrollIdleGate } from "../internal/httpapi/web/timeline-scroll-layout.js";

test("back-to-top chooses content gutter, then default, then left without touching layout", () => {
  const options = { anchor: { left: 220, right: 860 }, button: { width: 48, top: 720, bottom: 768 }, viewportWidth: 1200 };
  assert.deepEqual(backToTopHorizontalPosition(options), { left: "890px", right: "auto" });
  const content = { left: 880, right: 960, top: 710, bottom: 780 };
  assert.deepEqual(backToTopHorizontalPosition({ ...options, obstacles: [content] }), { left: "", right: "" });
  const drawer = { left: 1110, right: 1200, top: 710, bottom: 780 };
  assert.deepEqual(backToTopHorizontalPosition({ ...options, obstacles: [content, drawer] }), { left: "142px", right: "auto" });
  assert.deepEqual(backToTopHorizontalPosition({ ...options, anchor: { left: 10, right: 490 }, viewportWidth: 500 }), { left: "", right: "" });
});

test("idle gate coalesces waiters, delays during repeated scrolls, and resumes once", async () => {
  let clock = 0, callback; let delay, schedules = 0, cancels = 0, resumed = 0;
  const gate = createScrollIdleGate({ now: () => clock,
    schedule(fn, ms) { callback = fn; delay = ms; schedules++; return schedules; }, cancel() { cancels++; } });
  await gate.wait();
  gate.noteScroll();
  const a = gate.wait().then(() => resumed++), b = gate.wait().then(() => resumed++);
  clock = 200; gate.noteScroll();
  assert.equal(delay, 350);
  assert.equal(resumed, 0);
  clock = 400; callback();
  assert.equal(delay, 150);
  assert.equal(resumed, 0);
  clock = 550; callback();
  await Promise.all([a, b]);
  assert.equal(resumed, 2);
  assert.ok(cancels >= 2);
});

const app = readFileSync(new URL("../internal/httpapi/web/app.js", import.meta.url), "utf8");
test("scroll shares one geometry snapshot across collision placement and Related Context", () => {
  let reads = 0; let shared;
  const tabs = Array.from({ length: 50 }, (_, id) => ({ dataset: { timelineContentContextId: String(id) },
    closest: selector => selector === ".timeline-content-context-anchor" ? { getBoundingClientRect() { reads++; return { top: id * 500, bottom: id * 500 + 450, right: 800 }; } } : null,
    getBoundingClientRect() { reads++; return { width: 35, height: 75 }; }, classList: { toggle() {} } }));
  const context = { state: { currentView: "timeline", timelineContentContextDrawerOpen: false },
    document: { scrollingElement: { scrollTop: 800 }, documentElement: { clientWidth: 1200 }, querySelectorAll: () => tabs },
    window: { innerWidth: 1200, innerHeight: 900 }, $: () => ({ classList: { toggle() {} } }),
    BACK_TO_TOP_THRESHOLD_PX: 500, CONTENT_CONTEXT_TAB_DEFAULT_WIDTH: 35,
    selectContentContextViewportID: () => "1", contentContextTabFits: () => true, syncTimelineContentContextTab() {},
    syncBackToTopPosition: (_top, measurements) => { shared = measurements; } };
  vm.createContext(context);
  vm.runInContext(app.slice(app.indexOf("function measureTimelineContentContextTabs()"), app.indexOf("function timelineContentContextFeedbackKey")), context);
  vm.runInContext(app.slice(app.indexOf("function syncBackToTopNow()"), app.indexOf("function timelineContentContextObstacles(")) + "\nsyncBackToTopNow()", context);
  assert.equal(shared.length, 50);
  assert.equal(reads, 100, "one anchor and one tab read per card, with no second measurement pass");
});
test("actual back-to-top path finishes geometry reads before position writes", () => {
  const operations = [];
  function element(rect) { return { classList: { contains: () => false }, getBoundingClientRect() { operations.push("read"); return rect; } }; }
  const anchor = element({ left: 220, right: 860, top: 700, bottom: 1900, width: 640, height: 1200 });
  const button = element({ left: 1116, right: 1164, top: 720, bottom: 768, width: 48, height: 48 });
  const tab = { ...element({ width: 35, height: 75 }), closest: name => name === ".timeline-content-context-anchor" ? anchor : null };
  const context = { state: { currentView: "timeline" }, window: { innerWidth: 1200, innerHeight: 900 },
    document: { querySelector: () => anchor, querySelectorAll: () => [tab] },
    $: name => name === "#back-to-top" ? button : anchor,
    backToTopHorizontalPosition, setInlineStyle() { operations.push("write"); },
    syncBackToTopBoundaryPosition() { operations.push("write"); return 20; } };
  const obstacleStart = app.indexOf("function timelineContentContextObstacles(");
  const boundaryStart = app.indexOf("function syncBackToTopBoundaryPosition(");
  context.measurements = [{ rect: { right: 860, top: 700, bottom: 1900 }, width: 35, height: 75, eligible: true }];
  vm.runInNewContext(app.slice(obstacleStart, boundaryStart) + "\nsyncBackToTopPosition(800, measurements)", context);
  const firstWrite = operations.indexOf("write");
  assert.ok(firstWrite > 0);
  assert.ok(operations.slice(firstWrite).every(op => op === "write"), "no measurement after a position change");
});

test("side pane scroll position writes only to pane and toggle, never inherited page root", () => {
  const pane = {}, toggle = { getBoundingClientRect: () => ({ height: 72 }) };
  const anchor = { getBoundingClientRect: () => ({ left: 400, top: 80 }) };
  const writes = [];
  const context = { state: { currentView: "timeline" }, window: { innerHeight: 900, getComputedStyle: () => ({ borderTopLeftRadius: "14px" }) },
    document: { documentElement: {}, querySelector: () => anchor },
    $: selector => selector === "#timeline-side-pane-toggle" ? toggle : pane,
    setInlineStyle: (target, name) => writes.push({ target, name }) };
  vm.runInNewContext(app.slice(app.indexOf("function syncTimelineSidePanePosition()"), app.indexOf("function applyTimelineBatchGap")) + "\nsyncTimelineSidePanePosition()", context);
  assert.equal(writes.length, 5);
  assert.ok(writes.every(({ target }) => target === pane || target === toggle));
});

test("background refresh waits both before fetch and after an in-flight scroll", async () => {
  const state = { backgroundTimelineRefreshPending: true, backgroundTimelineRefreshInFlight: false };
  let idle, fetches = 0, applied = 0;
  const context = { state, timelineInteractionActive: () => false,
    timelineScrollIdle: { wait: () => new Promise(resolve => { idle = resolve; }) },
    refreshTimeline: async options => { assert.equal(options.background, true); fetches++; } };
  vm.createContext(context);
  vm.runInContext(app.slice(app.indexOf("async function flushBackgroundTimelineRefresh()"), app.indexOf('document.addEventListener("focusout"')), context);
  const result = context.flushBackgroundTimelineRefresh();
  const extra = context.flushBackgroundTimelineRefresh();
  assert.equal(fetches, 0);
  idle(); await Promise.all([result, extra]);
  assert.equal(fetches, 1);
  // Exercise the actual post-fetch gate independently of render implementation.
  const start = app.indexOf("async function refreshTimeline(options");
  const end = app.indexOf("    if (state.bootstrap) state.bootstrap.latestCheck", start);
  context.api = async () => ({ items: [], latestCheck: null });
  vm.runInContext(app.slice(start, end) + "applied(); } catch (error) { throw error; } }", Object.assign(context, { applied: () => applied++ }));
  const fetched = context.refreshTimeline({ background: true });
  await new Promise(setImmediate);
  assert.equal(applied, 0);
  idle(); await fetched;
  assert.equal(applied, 1);
});

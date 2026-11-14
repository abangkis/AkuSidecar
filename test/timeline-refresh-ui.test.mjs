import test from "node:test";
import assert from "node:assert/strict";
import { createTimelineRefreshUi, renderWithReadingAnchor } from "../internal/httpapi/web/timeline-refresh-ui.js";

function fixture() {
  const listeners = new Map();
  const notice = { hidden: true, classList: { toggle(_key, value) { notice.hidden = value; } } };
  const button = { disabled: true, addEventListener(name, fn) { listeners.set(`button:${name}`, fn); } };
  const detail = { textContent: "" };
  const root = { querySelector: () => null, contains: node => node?.inside === true };
  const ids = { "timeline-refresh-notice": notice, "timeline-refresh-apply": button, "timeline-refresh-detail": detail, "result-items": root };
  const document = { visibilityState: "visible", activeElement: null, getElementById: id => ids[id],
    querySelectorAll: () => [], addEventListener(name, fn) { listeners.set(name, fn); } };
  const viewport = { getSelection: () => null };
  let visible = true, applying = false, applies = 0;
  const ui = createTimelineRefreshUi({ document, viewport, onApply: async () => { applies++; },
    isVisible: () => visible, isApplying: () => applying });
  return { ui, document, viewport, root, ids, notice, button, detail, listeners, applies: () => applies,
    visible: value => { visible = value; }, applying: value => { applying = value; } };
}

test("staging only shows a notice; application requires a click and does not steal focus", async () => {
  const f = fixture(); const focus = { inside: false }; f.document.activeElement = focus;
  f.ui.show({ items: [] }); f.ui.show({ items: [1] });
  assert.equal(f.applies(), 0); assert.equal(f.notice.hidden, false); assert.equal(f.button.disabled, false);
  assert.equal(f.document.activeElement, focus);
  await f.listeners.get("button:click")(); assert.equal(f.applies(), 1);
  f.ui.show(null); assert.equal(f.notice.hidden, true); assert.equal(f.button.disabled, true);
});

test("playing video blocks application until its pause event", () => {
  const f = fixture(); const video = { paused: false, ended: false };
  f.document.querySelectorAll = () => [video]; f.ui.show({ items: [] });
  assert.equal(f.ui.canApply(), false); assert.equal(f.button.disabled, true); assert.match(f.detail.textContent, /Pause/);
  video.paused = true; f.listeners.get("pause")();
  assert.equal(f.ui.canApply(), true); assert.equal(f.button.disabled, false);
});

test("Timeline text selection, editors and focused controls are protected", () => {
  const f = fixture();
  f.viewport.getSelection = () => ({ isCollapsed: false, rangeCount: 1, getRangeAt: () => ({ intersectsNode: node => node === f.root }) });
  assert.equal(f.ui.canApply(), false);
  f.viewport.getSelection = () => null; f.root.querySelector = () => ({});
  assert.equal(f.ui.canApply(), false);
  f.root.querySelector = () => null; f.document.activeElement = { inside: true, matches: () => true };
  assert.equal(f.ui.canApply(), false);
  f.document.activeElement = null; assert.equal(f.ui.canApply(), true);
});

test("hidden or non-Timeline views cannot apply; application disables repeated clicks", () => {
  const f = fixture(); f.ui.show({}); f.visible(false); f.ui.sync();
  assert.equal(f.notice.hidden, true); assert.equal(f.ui.canApply(), false);
  f.visible(true); f.document.visibilityState = "hidden"; assert.equal(f.ui.canApply(), false);
  f.document.visibilityState = "visible"; f.applying(true); f.ui.sync(); assert.equal(f.button.disabled, true);
});

test("visible AI Signals drawer selection, feedback and focus are protected too", () => {
  const f = fixture();
  const pane = { querySelector: () => null, contains: node => node?.drawer === true };
  f.ids["timeline-side-pane"] = pane;
  f.viewport.getSelection = () => ({ isCollapsed: false, rangeCount: 1, getRangeAt: () => ({ intersectsNode: node => node === pane }) });
  assert.equal(f.ui.canApply(), false);
  f.viewport.getSelection = () => null; pane.querySelector = () => ({});
  assert.equal(f.ui.canApply(), false);
  pane.querySelector = () => null; f.document.activeElement = { drawer: true, matches: () => true };
  assert.equal(f.ui.canApply(), false);
  f.document.activeElement = null; assert.equal(f.ui.canApply(), true);
});

function card(id, top, bottom = top + 200) {
  return { dataset: { timelineId: id }, getBoundingClientRect: () => ({ top, bottom }) };
}

test("explicit apply keeps the first surviving visible post at its reading offset", () => {
  let cards = [card("above", -500, -100), card("reading", -30), card("next", 170)];
  const calls = [], document = { querySelectorAll: () => cards }, viewport = { scrollY: 900, innerHeight: 700, scrollTo: value => calls.push(value) };
  renderWithReadingAnchor(() => { cards = [card("new", -10), card("reading", 220), card("next", 420)]; }, document, viewport);
  assert.deepEqual(calls, [{ top: 1150, behavior: "instant" }]);
});

test("anchor removal falls back to the next visible survivor, then current scroll", () => {
  let cards = [card("removed", -30), card("survivor", 170)]; const calls = [];
  const document = { querySelectorAll: () => cards }, viewport = { scrollY: 900, innerHeight: 700, scrollTo: value => calls.push(value) };
  renderWithReadingAnchor(() => { cards = [card("survivor", 70)]; }, document, viewport);
  assert.equal(calls[0].top, 800);
  renderWithReadingAnchor(() => { cards = []; }, document, viewport);
  assert.equal(calls[1].top, 900);
});

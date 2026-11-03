import test from "node:test";
import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import vm from "node:vm";

const app = readFileSync(new URL("../internal/httpapi/web/app.js", import.meta.url), "utf8");
const moduleURL = new URL("../internal/httpapi/web/timeline-feedback.js", import.meta.url);
const extracted = existsSync(moduleURL) ? await import(moduleURL) : null;
function source(from, to) {
  const start = app.indexOf(from), end = app.indexOf(to, start + from.length);
  assert.ok(start >= 0 && end > start, `Missing production boundary: ${from}`);
  return app.slice(start, end);
}
class Element {
  constructor(tag) {
    this.tagName = tag; this.children = []; this.attributes = new Map(); this.listeners = new Map();
    this.className = ""; this.textContent = ""; this.title = ""; this.disabled = false;
    const classes = new Set();
    this.classList = { toggle: (name, enabled) => enabled ? classes.add(name) : classes.delete(name), contains: name => classes.has(name) };
  }
  append(...children) { this.children.push(...children); }
  setAttribute(name, value) { this.attributes.set(name, String(value)); }
  getAttribute(name) { return this.attributes.get(name); }
  addEventListener(type, callback) { const callbacks = this.listeners.get(type) || []; callbacks.push(callback); this.listeners.set(type, callbacks); }
  click() { return Promise.all((this.listeners.get("click") || []).map(callback => callback())); }
}
function deferred() { let resolve, reject; const promise = new Promise((yes, no) => { resolve = yes; reject = no; }); return { promise, resolve, reject }; }
function fixture({ request, realAPI = false } = {}) {
  const calls = [], events = [], errors = [], reloads = [];
  let now = 0, uuid = 0;
  const document = { createElement: tag => new Element(tag) };
  const context = vm.createContext({
    document, crypto: { randomUUID: () => `trace-${++uuid}` }, performance: { now: () => ++now },
    state: { bootstrap: { instanceEpoch: "epoch-one" }, sidecarEpochReloading: false },
    window: { location: { reload: () => reloads.push(true) } },
    api: async (path, options) => { calls.push({ path, options }); return request ? request(path, options) : { feedback: { direction: options.body.direction, origin: "routine" } }; },
    fetchFromSidecar: async (path, options) => { calls.push({ path, options }); return request(path, options); },
    invalidateLibraryStorage: () => events.push("invalidate"),
    showError: error => { errors.push(error); events.push("error"); },
    logInteractionTrace: (action, trace, phase, details) => events.push({ action, trace, phase, ...details }),
    buildSourceLink: () => new Element("a"), buildTimelineReadLaterAction: () => new Element("read-later"),
    buildSemanticCorrectionActions: () => new Element("correction"),
  });
  vm.runInContext(source("function feedbackButton(", "function logInteractionTrace("), context);
  if (realAPI) vm.runInContext(source("async function api(", "async function bridgeApi(") + source("function enforceCurrentSidecarEpoch(", "async function fetchFromSidecar("), context);
  if (extracted) {
    assert.match(app, /import \{ createTimelineFeedbackUI \} from "\.\/timeline-feedback\.js"/);
    const composition = source("const timelineFeedbackUI =", "const timelineScrollIdle =");
    vm.runInContext(composition, Object.assign(context, { createTimelineFeedbackUI: extracted.createTimelineFeedbackUI }));
    assert.match(app, /const feedback = timelineFeedbackUI\.build\(entry\);/);
  } else vm.runInContext(source("async function sendFeedback(", "function openMedia("), context);
  vm.runInContext(source("function buildActions(", "function timelineContentContextAnchor("), context);
  function build(entry) {
    const actions = context.buildActions(entry), controls = actions.children[1];
    return { actions, controls, more: controls.children[0], less: controls.children[1] };
  }
  return { build, calls, events, errors, reloads, context };
}
function selected(button) { return [button.classList.contains("selected"), button.getAttribute("aria-pressed")]; }

test("Timeline feedback initial DOM and calibration selection preserve toolbar composition", async t => {
  for (const feedback of [undefined, { direction: "more", origin: "routine" }, { direction: "more", origin: "calibration" }, { direction: "less", origin: "calibration" }]) {
    await t.test(JSON.stringify(feedback) || "none", () => {
      const f = fixture(), ui = f.build({ id: "one", feedback, semanticEvent: {} });
      assert.equal(ui.actions.className, "result-actions");
      assert.equal(ui.actions.children[0].className, "timeline-primary-actions");
      assert.deepEqual(ui.actions.children[0].children.map(x => x.tagName), ["a", "read-later"]);
      assert.equal(ui.actions.children[2].tagName, "correction");
      assert.equal(ui.controls.className, "feedback-actions");
      for (const [direction, button] of [["more", ui.more], ["less", ui.less]]) {
        assert.equal(button.type, "button"); assert.equal(button.className, "feedback-button");
        assert.equal(button.textContent, direction === "more" ? "More like this" : "Less like this");
        assert.deepEqual(selected(button), [feedback?.direction === direction, String(feedback?.direction === direction)]);
        assert.equal(button.title, feedback?.direction === direction && feedback.origin === "calibration" ? "Chosen during onboarding calibration" : "");
        assert.equal(button.disabled, false); assert.equal(button.listeners.get("click").length, 1);
      }
      assert.equal(f.calls.length, 0);
    });
  }
});

test("Timeline feedback success keeps request, cache, trace and mutation order", async t => {
  for (const direction of ["more", "less"]) await t.test(direction, async () => {
    const pending = deferred(), f = fixture({ request: () => pending.promise });
    let stored = { direction: "more", origin: "calibration" };
    const entry = { id: "item/with space", get feedback() { return stored; }, set feedback(value) { f.events.push("mutation"); stored = value; } };
    const ui = f.build(entry), click = ui[direction].click();
    assert.equal(f.calls.length, 1); assert.equal(stored.origin, "calibration");
    const call = f.calls[0];
    assert.equal(call.path, "/api/timeline/item%2Fwith%20space/feedback");
    assert.deepEqual(JSON.parse(JSON.stringify(call.options)), { method: "POST", body: { direction, reason: direction === "more" ? null : "not_interested" }, feedbackTraceId: "trace-1" });
    assert.equal(ui.more.disabled || ui.less.disabled, false);
    const response = { direction, origin: "routine" }; pending.resolve({ feedback: response }); await click;
    assert.equal(entry.feedback, response);
    assert.deepEqual(f.events.map(x => typeof x === "string" ? x : x.phase), ["click", "request_start", "invalidate", "request_end", "mutation", "rendered"]);
    assert.equal(f.events[3].outcome, "ok");
    assert.deepEqual(selected(ui[direction]), [true, "true"]); assert.equal(ui.more.title, ""); assert.equal(ui.less.title, "");
    assert.equal(f.errors.length, 0);
  });
});

test("Timeline feedback failure and empty result keep the previous selection", async t => {
  for (const failure of [new Error("offline"), Object.assign(new Error("epoch"), { recoveryInitiated: true }), null]) await t.test(failure?.message || "no feedback", async () => {
    const f = fixture({ request: async () => { if (failure) throw failure; return {}; } });
    const previous = { direction: "less", origin: "calibration" }, entry = { id: "one", feedback: previous }, ui = f.build(entry);
    await ui.more.click(); assert.equal(entry.feedback, previous); assert.deepEqual(selected(ui.less), [true, "true"]);
    assert.equal(f.errors.length, failure ? 1 : 0); if (failure) assert.equal(f.errors[0], failure);
    assert.equal(f.events.includes("invalidate"), !failure);
    assert.equal(f.events.some(x => x.phase === "rendered"), false);
    assert.equal(f.events.find(x => x.phase === "request_end").outcome, failure ? "error" : "ok");
  });
});

test("Timeline feedback preserves unblocked repeated clicks and response completion order", async () => {
  const pending = [deferred(), deferred(), deferred()]; let index = 0;
  const f = fixture({ request: () => pending[index++].promise }), entry = { id: "one", feedback: { direction: "more" } }, ui = f.build(entry);
  const clicks = [ui.more.click(), ui.more.click(), ui.less.click()];
  assert.equal(f.calls.length, 3); assert.equal(ui.more.disabled || ui.less.disabled, false);
  assert.deepEqual(f.calls.map(x => x.options.feedbackTraceId), ["trace-1", "trace-2", "trace-3"]);
  for (const [i, direction] of [[2, "less"], [0, "more"], [1, "more"]]) {
    const result = { direction, response: i }; pending[i].resolve({ feedback: result }); await clicks[i];
    assert.equal(entry.feedback, result); assert.deepEqual(selected(ui[direction]), [true, "true"]);
  }
  assert.deepEqual(f.events.filter(x => x.phase === "rendered").map(x => x.trace), ["trace-3", "trace-1", "trace-2"]);
});

test("Timeline feedback completion updates its captured card without resurrecting a replacement", async () => {
  const pending = deferred(), f = fixture({ request: () => pending.promise });
  const oldEntry = { id: "one" }, oldUI = f.build(oldEntry), click = oldUI.more.click();
  const replacement = { id: "one" }, newUI = f.build(replacement);
  pending.resolve({ feedback: { direction: "more" } }); await click;
  assert.equal(oldEntry.feedback.direction, "more"); assert.equal(replacement.feedback, undefined);
  assert.deepEqual(selected(newUI.more), [false, "false"]); assert.equal(f.calls.length, 1);
  assert.equal(newUI.more.listeners.get("click").length, 1);
});

test("Timeline feedback retains the actual API wrapper trace header and epoch rejection", async t => {
  for (const epoch of ["epoch-one", "epoch-two"]) await t.test(epoch, async () => {
    const f = fixture({ realAPI: true, request: async () => ({ ok: true, status: 200, headers: { get: () => epoch }, json: async () => ({ feedback: { direction: "more" } }) }) });
    const entry = { id: "one" }, ui = f.build(entry); await ui.more.click();
    const init = f.calls[0].options;
    assert.equal(init.cache, "no-store"); assert.equal(init.headers["X-Aku-Feedback-Trace"], "trace-1");
    assert.equal(init.headers["Content-Type"], "application/json"); assert.equal(init.body, '{"direction":"more","reason":null}');
    assert.equal(f.reloads.length, epoch === "epoch-two" ? 1 : 0);
    assert.equal(f.events.includes("invalidate"), epoch === "epoch-one");
    if (epoch === "epoch-two") { assert.equal(entry.feedback, undefined); assert.equal(f.errors[0].code, "sidecar_epoch_changed"); }
    else assert.equal(entry.feedback.direction, "more");
  });
});

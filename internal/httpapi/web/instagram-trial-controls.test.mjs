import test from "node:test";
import assert from "node:assert/strict";
import {createInstagramTrialControls, instagramTrialView} from "./instagram-trial-controls.js";
import {createXDailyControls} from "./x-daily-controls.js";

class FakeElement {
  hidden = false;
  disabled = false;
  checked = false;
  textContent = "";
  className = "";
  listeners = new Map();

  addEventListener(type, listener) {
    const handlers = this.listeners.get(type) ?? new Set();
    handlers.add(listener);
    this.listeners.set(type, handlers);
  }

  removeEventListener(type, listener) {
    this.listeners.get(type)?.delete(listener);
  }

  click() {
    if (this.disabled) return;
    for (const listener of this.listeners.get("click") ?? []) listener({preventDefault() {}});
  }
}

const retryable = Object.freeze({
  selectedMode: "concurrent",
  authorizationPolicy: "profile",
  maxBatchMs: 60000,
  state: "stopped",
  reason: "batch-failed",
  derivationAuthorized: false,
  cleanup: "verified",
  rememberedPermission: true,
  manualRetryAvailable: true,
});

function makeControls(status = retryable, api = async () => ({instagramCapture: retryable}), applyExpected = true) {
  const elements = Object.fromEntries(["panel", "status", "detail", "consent", "start", "stop", "feedback"].map(name => [name, new FakeElement()]));
  const context = {epoch: "current-epoch", development: true};
  const controls = createInstagramTrialControls({
    panel: elements.panel,
    status: elements.status,
    detail: elements.detail,
    consent: elements.consent,
    startButton: elements.start,
    stopButton: elements.stop,
    feedback: elements.feedback,
    readContext: () => context,
    readActiveSources: () => ["instagram"],
    api,
  });
  const ticket = controls.beginRead();
  assert.equal(controls.applyRead(ticket, {instagramCapture: status}), applyExpected);
  return {controls, elements, context};
}

test("IG manual retry is exposed only for a fresh failed batch with verified cleanup and remembered consent", () => {
  const view = instagramTrialView(retryable, {epoch: "current-epoch", development: true}, true, false, false);
  assert.equal(view.visible, true);
  assert.equal(view.startEnabled, false);
  assert.equal(view.retryEnabled, true);
  assert.equal(view.startLabel, "Retry X");
  assert.equal(view.statusText, "Failed — retry available");
  assert.match(view.detailText, /later manual or scheduled update/);

  for (const status of [
    {...retryable, cleanup: "pending"},
    {...retryable, cleanup: "uncertain"},
    {...retryable, rememberedPermission: false},
    {...retryable, state: "retry-wait"},
    {...retryable, reason: "reader-start-failed"},
    {...retryable, manualRetryAvailable: false},
    {...retryable, manualRetryAvailable: "true"},
  ]) {
    assert.equal(instagramTrialView(status, {epoch: "current-epoch", development: true}, true, false, false).retryEnabled, false);
  }
  assert.equal(instagramTrialView(retryable, {epoch: "current-epoch", development: true}, true, false, true).retryEnabled, false);
  assert.equal(instagramTrialView(retryable, {epoch: "current-epoch", development: true}, false, false, false).retryEnabled, false);
});

test("Retry IG uses the existing button and explicit retry route only after a user click", async () => {
  const calls = [];
  const {controls, elements} = makeControls(retryable, async (path, options) => {
    calls.push({path, options});
    return {instagramCapture: {...retryable, state: "ready", reason: "", derivationAuthorized: true, manualRetryAvailable: false}};
  });
  assert.equal(elements.start.textContent, "Retry IG");
  assert.equal(elements.start.disabled, false);
  assert.equal(calls.length, 0, "status refresh must not trigger a retry");

  elements.start.click();
  await new Promise(resolve => setTimeout(resolve, 0));
  assert.equal(calls.length, 1);
  assert.equal(calls[0].path, "/api/development/instagram-trial/retry");
  assert.deepEqual(calls[0].options, {
    method: "POST",
    body: {acknowledgeLimitations: true, allowProcessSessionDerivation: true, acknowledgeUnqualifiedRecipe: true},
    instanceEpoch: "current-epoch",
  });
  assert.equal(elements.start.textContent, "Start IG");
  controls.destroy();
});

test("stale or malformed IG status cannot enable Retry IG", () => {
  const {controls, elements} = makeControls();
  const ticket = controls.beginRead();
  controls.rejectRead(ticket);
  assert.equal(elements.start.disabled, true);
  elements.start.click();
  assert.equal(elements.feedback.textContent.includes("Retrying"), false);
  controls.destroy();

  const malformed = makeControls({...retryable, manualRetryAvailable: "true"}, undefined, false);
  assert.equal(malformed.elements.start.disabled, true);
  assert.equal(malformed.elements.panel.hidden, true);
  malformed.controls.destroy();
});

test("the shared X daily controls keep their original Start behavior", () => {
  const elements = Object.fromEntries(["panel", "status", "detail", "consent", "start", "stop", "feedback"].map(name => [name, new FakeElement()]));
  const controls = createXDailyControls({
    panel: elements.panel,
    status: elements.status,
    detail: elements.detail,
    consent: elements.consent,
    startButton: elements.start,
    stopButton: elements.stop,
    feedback: elements.feedback,
    readContext: () => ({epoch: "current-epoch", development: true}),
    api: async () => ({xCapture: retryable}),
  });
  const ticket = controls.beginRead();
  assert.equal(controls.applyRead(ticket, {xCapture: {...retryable, authorizationPolicy: "process"}}), true);
  assert.equal(elements.start.textContent, "Start X");
  assert.equal(elements.start.disabled, true);
  controls.destroy();
});

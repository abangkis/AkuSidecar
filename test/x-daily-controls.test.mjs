import test from "node:test";
import assert from "node:assert/strict";
import { createXDailyControls, xDailyView, xDailyCaptureNotice } from "../internal/httpapi/web/x-daily-controls.js";

class FakeElement {
  constructor() {
    this.hidden = false;
    this.disabled = false;
    this.checked = false;
    this.className = "";
    this.textContent = "";
    this.listeners = new Map();
  }

  addEventListener(type, handler) {
    const listeners = this.listeners.get(type) ?? new Set();
    listeners.add(handler);
    this.listeners.set(type, listeners);
  }

  removeEventListener(type, handler) {
    this.listeners.get(type)?.delete(handler);
  }

  emit(type) {
    for (const handler of this.listeners.get(type) ?? []) handler({ preventDefault() {} });
  }

  click() {
    if (!this.disabled) this.emit("click");
  }

  check(value = true) {
    this.checked = value;
    this.emit("change");
  }
}

const dailyStatus = (overrides = {}) => ({
  selectedMode: "concurrent",
  authorizationPolicy: "process",
  maxBatchMs: 60000,
  derivationAuthorized: false,
  state: "disabled",
  reason: "process-consent-required",
  ...overrides,
});

const rememberedStatus = (overrides = {}) => dailyStatus({
  authorizationPolicy: "profile", rememberedPermission: true,
  state: "ready", reason: "", derivationAuthorized: true, ...overrides,
});

function deferred() {
  let resolve;
  let reject;
  const promise = new Promise((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}

async function flush() {
  await Promise.resolve();
  await new Promise((resolve) => setImmediate(resolve));
}

function setup({ context = { epoch: "epoch-a", development: true }, request, notices = [], sources = ["x"], onReviewSettings } = {}) {
  const dom = {
    panel: new FakeElement(), status: new FakeElement(), detail: new FakeElement(), consent: new FakeElement(),
    startButton: new FakeElement(), stopButton: new FakeElement(), feedback: new FakeElement(),
  };
  const calls = [];
  const statuses = [];
  const api = (path, options) => {
    calls.push({ path, options });
    return request ? request(path, options, calls.length) : Promise.resolve({ xCapture: dailyStatus({ state: "ready", reason: "", derivationAuthorized: true }) });
  };
  const controller = createXDailyControls({
    ...dom,
    api,
    readContext: () => context,
    onStatus: (value) => statuses.push(value),
    notices,
    readActiveSources: () => sources,
    onReviewSettings,
  });
  return { ...dom, calls, statuses, controller, setContext: (next) => { context = next; } };
}

test("daily warning distinguishes unavailable X from normal capture without exposing diagnostic content", () => {
  const context = { epoch: "current", development: true };
  for (const state of ["ready", "capturing", "starting", "startup-in-progress", "stopping"]) {
    assert.equal(xDailyCaptureNotice(rememberedStatus({ state }), context, ["x"]).visible, false);
  }
  for (const status of [
    rememberedStatus({ state: "stopped", reason: "batch-failed", derivationAuthorized: false, cleanup: "uncertain" }),
    rememberedStatus({ state: "failed", reason: "reader-start-failed", derivationAuthorized: false }),
    dailyStatus(),
    rememberedStatus({ state: "not-ready", reason: "PRIVATE_ACCOUNT_ID" }),
  ]) {
    const notice = xDailyCaptureNotice({ ...status, failureCode: "PRIVATE_COOKIE", cleanupDiagnostic: { targetId: "PRIVATE_TARGET" } }, context, ["instagram", "linkedin", "x"]);
    assert.equal(notice.visible, true);
    assert.match(notice.detail, /without X.*Other available sources/);
    assert.equal(JSON.stringify(notice).includes("PRIVATE"), false);
  }
  const optedOut = xDailyCaptureNotice(rememberedStatus({ state: "stopped", reason: "opted-out", derivationAuthorized: false, rememberedPermission: false }), context, ["x"]);
  assert.equal(optedOut.title, "X capture is off");
  const uncertain = xDailyCaptureNotice(rememberedStatus(), context, ["x"], true);
  assert.match(uncertain.detail, /could not be confirmed/);
  assert.doesNotMatch(uncertain.detail, /without X/);
  for (const [status, ctx, sources] of [[rememberedStatus(), context, ["linkedin"]], [rememberedStatus(), { ...context, development: false }, ["x"]], [rememberedStatus({ selectedMode: "exclusive" }), context, ["x"]], [null, context, ["x"]]]) {
    assert.equal(xDailyCaptureNotice(status, ctx, sources).visible, false);
  }
});

test("Timeline and Inbox warnings survive stale polls, clear on recovery, and only navigate to Settings", () => {
  const notices = [0, 1].map(() => ({ panel: new FakeElement(), title: new FakeElement(), detail: new FakeElement(), button: new FakeElement() }));
  let settingsVisits = 0;
  const f = setup({ notices, onReviewSettings: () => settingsVisits++ });
  f.controller.reset(rememberedStatus({ state: "stopped", reason: "batch-failed", derivationAuthorized: false, cleanup: "uncertain" }));
  for (const notice of notices) {
    assert.equal(notice.panel.hidden, false);
    assert.match(notice.detail.textContent, /cleanup could not be confirmed/);
    notice.button.click();
  }
  assert.equal(settingsVisits, 2);assert.equal(f.calls.length, 0);
  const stale = f.controller.beginRead(), fresh = f.controller.beginRead();
  assert.equal(f.controller.applyRead(stale, rememberedStatus()), false);
  assert.equal(notices[0].panel.hidden, false);
  f.controller.rejectRead(fresh);
  assert.equal(notices[1].panel.hidden, false);
  assert.match(notices[1].title.textContent, /unavailable/);
  f.controller.applyRead(f.controller.beginRead(), rememberedStatus());
  assert(notices.every(item => item.panel.hidden));
  f.controller.reset(rememberedStatus({ state: "stopped", reason: "batch-failed", derivationAuthorized: false }));
  f.setContext({ epoch: "epoch-b", development: true });f.controller.beginRead();
  assert(notices.every(item => item.panel.hidden), "previous epoch must not supply failure state");
  f.controller.destroy();notices[0].button.click();assert.equal(settingsVisits, 2);
});

test("render, poll ticket, and reset never issue requests; missing status fails closed", () => {
  const f = setup();
  assert.equal(f.calls.length, 0);
  assert.equal(f.panel.hidden, true);
  const ticket = f.controller.beginRead();
  assert.deepEqual(ticket, { epoch: "epoch-a", revision: 1 });
  assert.equal(f.calls.length, 0);
  assert.equal(f.controller.applyRead(ticket, null), false);
  assert.equal(f.panel.hidden, true);
  f.controller.reset();
  assert.equal(f.calls.length, 0);
  assert.equal(f.consent.checked, false);
  f.controller.destroy();
});

test("saved profile permission is restored from fresh status across an epoch change without UI replay", () => {
  const f = setup();
  f.controller.reset(rememberedStatus());
  assert.equal(f.consent.checked, true);
  assert.equal(f.consent.disabled, true);
  assert.equal(f.stopButton.disabled, false);
  assert.match(f.detail.textContent, /saved.*profile.*verifies.*again/);
  f.setContext({ epoch: "epoch-b", development: true });
  const ticket = f.controller.beginRead();
  assert.equal(f.consent.checked, false, "old runtime status must not carry across epochs");
  assert.equal(f.controller.applyRead(ticket, { xCapture: rememberedStatus() }), true);
  assert.equal(f.consent.checked, true);
  assert.equal(f.calls.length, 0, "restoring saved status must not replay an old Start POST");
});

test("a failed poll keeps the saved choice visible but never enables unverified startup", () => {
  const f = setup();
  f.controller.reset(rememberedStatus());
  f.controller.rejectRead(f.controller.beginRead());
  assert.equal(f.consent.checked, true);
  assert.equal(f.startButton.disabled, true);
  assert.equal(f.stopButton.disabled, false);
});

test("Stop clears remembered profile permission from the confirmed server response", async () => {
  const f = setup({ request: async () => ({ xCapture: rememberedStatus({
    state: "stopped", reason: "opted-out", derivationAuthorized: false, rememberedPermission: false,
  }) }) });
  f.controller.reset(rememberedStatus());
  f.stopButton.click();
  await flush();
  assert.equal(f.consent.checked, false);
  assert.match(f.feedback.textContent, /saved permission has been removed/);
  assert.equal(f.calls.length, 1);
  assert.equal(f.calls[0].path, "/api/development/x-daily/stop");
});

test("requires process consent and posts exact payload pinned to the captured epoch", async () => {
  const pending = deferred();
  const f = setup({ request: () => pending.promise });
  f.controller.reset(dailyStatus());
  assert.equal(f.panel.hidden, false);
  assert.equal(f.startButton.disabled, true);
  f.startButton.click();
  assert.equal(f.calls.length, 0);
  f.consent.check(true);
  assert.equal(f.startButton.disabled, false);
  f.startButton.click();
  assert.deepEqual(f.calls, [{
    path: "/api/development/x-daily/start",
    options: {
      method: "POST",
      body: { acknowledgeLimitations: true, allowProcessSessionDerivation: true },
      instanceEpoch: "epoch-a",
    },
  }]);
  assert.equal(f.consent.checked, false);
  pending.resolve({ xCapture: dailyStatus({ state: "ready", reason: "", derivationAuthorized: true }) });
  await flush();
  assert.equal(f.status.textContent, "Ready");
  assert.equal(f.startButton.disabled, true);
  assert.equal(f.stopButton.disabled, false);
});

test("double clicks coalesce into one mutation and start success requires a fresh consent for any later attempt", async () => {
  const pending = deferred();
  const f = setup({ request: () => pending.promise });
  f.controller.reset(dailyStatus());
  f.consent.check(true);
  f.startButton.click();
  f.startButton.click();
  assert.equal(f.calls.length, 1);
  pending.resolve({ xCapture: dailyStatus({ state: "capturing", reason: "", derivationAuthorized: true }) });
  await flush();
  assert.equal(f.status.textContent, "Capturing");
  assert.equal(f.consent.checked, false);
  assert.equal(f.startButton.disabled, true);
  f.stopButton.click();
  await flush();
  assert.equal(f.calls.length, 2);
  assert.equal(f.calls[1].path, "/api/development/x-daily/stop");
  assert.deepEqual(f.calls[1].options.body, {});
});

test("stop success is terminal and explains restart is required for new permission", async () => {
  const f = setup({ request: async () => ({ xCapture: dailyStatus({
    state: "stopped", reason: "process-authorization-ended", derivationAuthorized: false,
  }) }) });
  f.controller.reset(dailyStatus({ state: "ready", reason: "", derivationAuthorized: true }));
  f.stopButton.click();
  await flush();
  assert.equal(f.status.textContent, "Stopped — restart required");
  assert.match(f.feedback.textContent, /Restart AkuSidecar/);
  assert.equal(f.stopButton.disabled, true);
  assert.equal(f.startButton.disabled, true);
  assert.equal(f.consent.disabled, true);
});

test("a poll begun before mutation cannot overwrite the mutation reply", async () => {
  const poll = deferred();
  const start = deferred();
  const f = setup({ request: (path) => path === "/api/collection/runtime" ? poll.promise : start.promise });
  f.controller.reset(dailyStatus());
  const ticket = f.controller.beginRead();
  f.consent.check(true);
  f.startButton.click();
  assert.equal(f.controller.applyRead(ticket, dailyStatus()), false);
  start.resolve({ xCapture: dailyStatus({ state: "ready", reason: "", derivationAuthorized: true }) });
  await flush();
  poll.resolve({ xCapture: dailyStatus() });
  assert.equal(f.controller.applyRead(ticket, dailyStatus()), false);
  assert.equal(f.status.textContent, "Ready");
});

test("epoch changes invalidate in-flight starts and do not replay or accept their reply", async () => {
  const start = deferred();
  const f = setup({ request: () => start.promise });
  f.controller.reset(dailyStatus());
  const priorUpdates = f.statuses.length;
  f.consent.check(true);
  f.startButton.click();
  f.setContext({ epoch: "epoch-b", development: true });
  start.resolve({ xCapture: dailyStatus({ state: "ready", reason: "", derivationAuthorized: true }) });
  await flush();
  assert.equal(f.calls.length, 1);
  assert.equal(f.calls[0].options.instanceEpoch, "epoch-a");
  assert.equal(f.consent.checked, false);
  assert.equal(f.status.textContent, "Checking status");
  assert.equal(f.statuses.length, priorUpdates);
  assert.equal(f.panel.hidden, true);
});

test("reset invalidates in-flight replies and clears ephemeral consent", async () => {
  const start = deferred();
  const f = setup({ request: () => start.promise });
  f.controller.reset(dailyStatus());
  f.consent.check(true);
  f.startButton.click();
  f.controller.reset(dailyStatus());
  const acceptedAtReset = f.statuses.length;
  start.resolve({ xCapture: dailyStatus({ state: "ready", reason: "", derivationAuthorized: true }) });
  await flush();
  assert.equal(f.consent.checked, false);
  assert.equal(f.status.textContent, "Consent required");
  assert.equal(f.statuses.length, acceptedAtReset);
});

test("failed post performs one pinned status refresh; a failed refresh keeps start closed but permits authorized stop", async () => {
  const calls = [];
  const f = setup({ request: async (path, options) => {
    calls.push({ path, options });
    if (path === "/api/development/x-daily/start") throw new Error("private raw server error");
    return { xCapture: dailyStatus({ state: "ready", reason: "", derivationAuthorized: true }) };
  } });
  f.controller.reset(dailyStatus());
  f.consent.check(true);
  f.startButton.click();
  await flush();
  assert.deepEqual(calls.map(({ path, options }) => [path, options.instanceEpoch]), [
    ["/api/development/x-daily/start", "epoch-a"],
    ["/api/collection/runtime", "epoch-a"],
  ]);
  assert.equal(f.status.textContent, "Ready");
  assert.equal(f.startButton.disabled, true);

  const gCalls = [];
  const g = setup({ request: async (path, options) => {
    gCalls.push({ path, options });
    if (path === "/api/development/x-daily/start") throw new Error("secret");
    throw new Error("secret");
  } });
  g.controller.reset(dailyStatus());
  g.consent.check(true);
  g.startButton.click();
  await flush();
  assert.equal(gCalls.length, 2);
  assert.equal(g.startButton.disabled, true);
  assert.match(g.feedback.textContent, /Starting remains disabled/);
  assert.doesNotMatch(g.feedback.textContent, /secret/);

  const hCalls = [];
  const h = setup({ request: async (path, options) => {
    hCalls.push({ path, options });
    throw new Error("secret");
  } });
  h.controller.reset(dailyStatus({ state: "ready", reason: "", derivationAuthorized: true }));
  h.stopButton.click();
  await flush();
  assert.equal(hCalls.length, 2);
  assert.equal(hCalls[0].path, "/api/development/x-daily/stop");
  assert.equal(hCalls[0].options.instanceEpoch, "epoch-a");
  assert.equal(h.stopButton.disabled, false);
  assert.equal(h.status.textContent, "Status unavailable");
});

test("unauthorized, malformed, nondevelopment, and exclusive statuses stay hidden and disabled", () => {
  for (const { context, value } of [
    { context: { epoch: "e", development: false }, value: dailyStatus() },
    { context: { epoch: "", development: true }, value: dailyStatus() },
    { context: { epoch: "e", development: true }, value: dailyStatus({ selectedMode: "exclusive" }) },
    { context: { epoch: "e", development: true }, value: dailyStatus({ authorizationPolicy: "session" }) },
    { context: { epoch: "e", development: true }, value: dailyStatus({ maxBatchMs: 300000 }) },
    { context: { epoch: "e", development: true }, value: { ...dailyStatus(), derivationAuthorized: "false" } },
    { context: { epoch: "e", development: true }, value: { ...dailyStatus(), state: "unknown" } },
  ]) {
    const f = setup({ context });
    const ticket = f.controller.beginRead();
    assert.equal(f.controller.applyRead(ticket, value), value?.state !== "unknown" && value?.derivationAuthorized !== "false" ? true : false);
    assert.equal(f.panel.hidden, true);
    assert.equal(f.startButton.disabled, true);
    assert.equal(f.stopButton.disabled, true);
  }
  assert.equal(xDailyView(dailyStatus(), { epoch: "e", development: true }, true).startEnabled, true);
  assert.equal(xDailyView(dailyStatus(), { epoch: "e", development: true }, true, false, true).startEnabled, false);
});

test("uncertain cleanup enables Stop while pending cleanup prevents Start", () => {
  const f = setup();
  f.controller.reset(dailyStatus({ state: "stopped", reason: "opted-out", cleanup: "uncertain" }));
  assert.equal(f.panel.hidden, false);
  assert.equal(f.stopButton.disabled, false);
  assert.equal(f.startButton.disabled, true);
});

test("capturing with pending cleanup stays labeled Capturing and stale authorization is shown as unavailable", async () => {
  const f = setup({ request: async () => { throw new Error("unavailable"); } });
  f.controller.reset(dailyStatus({ state: "capturing", reason: "", derivationAuthorized: true, cleanup: "pending" }));
  assert.equal(f.status.textContent, "Capturing");
  assert.match(f.detail.textContent, /Process permission is active/);
  assert.equal(f.stopButton.disabled, false);

  f.stopButton.click();
  await flush();
  assert.equal(f.status.textContent, "Status unavailable");
  assert.equal(f.stopButton.disabled, false);
  assert.equal(f.startButton.disabled, true);
});

test("context reset clears prior feedback", () => {
  const f = setup();
  f.controller.reset(dailyStatus());
  f.feedback.textContent = "Old action result";
  f.setContext({ epoch: "epoch-b", development: true });
  f.controller.beginRead();
  assert.equal(f.feedback.textContent, "");
  assert.equal(f.panel.hidden, true);
});

test("consent is not persisted and destroy removes event handlers", () => {
  const f = setup();
  f.controller.reset(dailyStatus());
  f.consent.check(true);
  assert.equal(f.consent.checked, true);
  f.controller.destroy();
  f.startButton.click();
  assert.equal(f.calls.length, 0);
  assert.equal(f.consent.checked, true);
});

test("failed regular polling closes stale Start consent but retains the authorized Stop action", () => {
  const f = setup();
  f.controller.reset(dailyStatus());
  f.consent.check(true);
  const ticket = f.controller.beginRead();
  assert.equal(f.controller.rejectRead(ticket), true);
  assert.equal(f.status.textContent, "Status unavailable");
  assert.equal(f.startButton.disabled, true);
  assert.equal(f.consent.checked, false);
  f.controller.reset(dailyStatus({ state: "ready", reason: "", derivationAuthorized: true }));
  assert.equal(f.controller.rejectRead(ticket), false, "a superseded failure must not overwrite newer status");
  assert.equal(f.controller.rejectRead(f.controller.beginRead()), true);
  assert.equal(f.stopButton.disabled, false);
  assert.equal(f.calls.length, 0);
});

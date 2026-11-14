import test from "node:test";
import assert from "node:assert/strict";
import { createTimelineRefreshCoordinator } from "../internal/httpapi/web/timeline-refresh-coordinator.js";

function deferred() {
  let resolve, reject;
  const promise = new Promise((accept, fail) => { resolve = accept; reject = fail; });
  return { promise, resolve, reject };
}

const snapshot = (revision, pendingDeep = false, pendingMedia = false) => ({
  revision, items: [{ aiDetection: { pendingDeep, pendingMedia } }],
});

function harness() {
  const timers = new Map(), requests = [], stages = [], metadata = [], applied = [], errors = [];
  let nextTimer = 0, context = "timeline:all", rendered = 0, allowed = true;
  let idle = () => Promise.resolve(), applyWork = () => {};
  const coordinator = createTimelineRefreshCoordinator({
    fetchSnapshot() { const result = deferred(); requests.push(result); return result.promise; },
    isUnchanged: value => value.revision === rendered,
    hasPendingDetection: value => value.items.some(item => item.aiDetection.pendingDeep || item.aiDetection.pendingMedia),
    onSnapshot: value => metadata.push(value),
    onStage: value => stages.push(value),
    applySnapshot: async value => { applied.push(value); await applyWork(value); rendered = value.revision; },
    getContextKey: () => context,
    canApply: () => allowed,
    waitForIdle: () => idle(),
    onError: error => errors.push(error),
    schedule(fn, delay) { const id = ++nextTimer; timers.set(id, { fn, delay }); return id; },
    cancel: id => timers.delete(id),
  });
  return {
    coordinator, requests, stages, metadata, applied, errors, timers,
    context(value) { context = value; },
    allow(value) { allowed = value; },
    idle(value) { idle = value; },
    applyWork(value) { applyWork = value; },
    async respond(value) { const result = coordinator.request(); requests.at(-1).resolve(value); await result; },
    poll() {
      assert.equal(timers.size, 1, "exactly one detection poll is armed");
      const [id, timer] = timers.entries().next().value;
      assert.equal(timer.delay, 5000);
      timers.delete(id);
      return timer.fn();
    },
  };
}

test("construction does not invoke host ports before their state is initialized", () => {
  const calls = [];
  const ports = Object.fromEntries([
    "fetchSnapshot", "isUnchanged", "applySnapshot", "hasPendingDetection",
    "onSnapshot", "onStage", "canApply", "waitForIdle", "getContextKey",
    "onError", "schedule", "cancel",
  ].map(name => [name, () => { calls.push(name); }]));
  const coordinator = createTimelineRefreshCoordinator(ports);
  assert.deepEqual(calls, []);
  assert.equal(coordinator.staged, null);
  assert.equal(coordinator.applying, false);
});

test("identical pending responses keep polling without application or stages", async () => {
  const h = harness();
  await h.respond(snapshot(0, true));
  for (let index = 0; index < 3; index++) {
    const poll = h.poll(); h.requests.at(-1).resolve(snapshot(0, true)); await poll;
    assert.equal(h.timers.size, 1);
  }
  assert.equal(h.requests.length, 4);
  assert.equal(h.metadata.length, 4);
  assert.deepEqual(h.stages, [null, null, null, null]);
  assert.equal(h.applied.length, 0);
});

test("new detection and pendingMedia results stage until explicit application", async () => {
  const h = harness(), value = snapshot(1, false, true);
  await h.respond(value);
  assert.equal(h.coordinator.staged, value);
  assert.equal(h.applied.length, 0);
  assert.equal(h.timers.size, 1);
  assert.equal(await h.coordinator.apply(), true);
  assert.deepEqual(h.applied, [value]);
  assert.equal(h.coordinator.staged, null);
  assert.equal(h.timers.size, 1, "application does not own detection polling");
});

test("concurrent requests coalesce into one serialized replay with the latest stage", async () => {
  const h = harness();
  const first = h.coordinator.request();
  const second = h.coordinator.request();
  const third = h.coordinator.request();
  assert.equal(first, second); assert.equal(second, third);
  assert.equal(h.requests.length, 1);
  h.requests[0].resolve(snapshot(1));
  await Promise.resolve(); await Promise.resolve();
  assert.equal(h.requests.length, 2);
  const latest = snapshot(2); h.requests[1].resolve(latest);
  await first;
  assert.equal(h.coordinator.staged, latest);
  assert.equal(h.applied.length, 0);
});

test("an identical result clears a previous changed stage", async () => {
  const h = harness();
  await h.respond(snapshot(1));
  await h.respond(snapshot(0));
  assert.equal(h.coordinator.staged, null);
  assert.equal(h.stages.at(-1), null);
  assert.equal(await h.coordinator.apply(), false);
});

test("foreground invalidation discards old responses without changing status or pending polls", async () => {
  const h = harness();
  await h.respond(snapshot(1, true));
  const request = h.coordinator.request();
  h.coordinator.invalidate();
  h.coordinator.setPending(true);
  h.requests.at(-1).resolve(snapshot(2));
  await request;
  assert.equal(h.metadata.length, 1);
  assert.equal(h.coordinator.staged, null);
  assert.equal(h.timers.size, 1, "foreground pending state survives stale completion");
  assert.equal(h.errors.length, 0);
});

test("stale errors after invalidation do not display or restart old polls", async () => {
  const h = harness();
  h.coordinator.setPending(true);
  const request = h.coordinator.request();
  h.coordinator.invalidate();
  h.requests.at(-1).reject(Error("old failure")); await request;
  assert.equal(h.errors.length, 0);
  assert.equal(h.timers.size, 0);
});

test("a request queued behind an invalidated flight fetches the current context", async () => {
  const h = harness();
  const request = h.coordinator.request();
  h.coordinator.invalidate();
  const replay = h.coordinator.request();
  h.requests[0].resolve(snapshot(1));
  await Promise.resolve(); await Promise.resolve();
  h.requests[1].resolve(snapshot(2));
  await Promise.all([request, replay]);
  assert.deepEqual(h.metadata.map(value => value.revision), [2]);
  assert.equal(h.coordinator.staged.revision, 2);
});

test("context changes discard old results and clear previously staged work", async () => {
  const h = harness();
  await h.respond(snapshot(1, true));
  const request = h.coordinator.request();
  h.context("library:saved");
  h.requests.at(-1).resolve(snapshot(2, true)); await request;
  assert.equal(h.coordinator.staged, null);
  assert.equal(h.metadata.length, 1);
  assert.equal(h.timers.size, 0);
  await h.respond(snapshot(3));
  assert.equal(h.coordinator.staged.revision, 3);
});

test("fetch proceeds during an idle wait and apply uses the latest same-context stage", async () => {
  const h = harness(), idle = deferred();
  await h.respond(snapshot(1));
  h.idle(() => idle.promise);
  const apply = h.coordinator.apply();
  assert.equal(h.coordinator.applying, true);
  const latest = snapshot(2);
  await h.respond(latest);
  assert.equal(h.requests.length, 2);
  assert.equal(h.applied.length, 0);
  idle.resolve(); assert.equal(await apply, true);
  assert.deepEqual(h.applied, [latest]);
  assert.equal(h.coordinator.applying, false);
});

test("apply rechecks playback and context after waiting without losing blocked stages", async () => {
  const h = harness(), idle = deferred();
  await h.respond(snapshot(1));
  h.idle(() => idle.promise);
  const apply = h.coordinator.apply();
  h.allow(false); idle.resolve();
  assert.equal(await apply, false);
  assert.equal(h.coordinator.staged.revision, 1);
  assert.equal(h.applied.length, 0);
  h.allow(true); h.context("timeline:source:x");
  assert.equal(await h.coordinator.apply(), false);
  assert.equal(h.coordinator.staged, null);
});

test("a context change during the idle wait prevents application", async () => {
  const h = harness(), idle = deferred();
  await h.respond(snapshot(1)); h.idle(() => idle.promise);
  const apply = h.coordinator.apply();
  h.context("timeline:source:other"); idle.resolve();
  assert.equal(await apply, false);
  assert.equal(h.applied.length, 0);
  assert.equal(h.coordinator.staged, null);
});

test("failed fetches retain pending work and rearm one poll", async () => {
  const h = harness();
  await h.respond(snapshot(1, true));
  const poll = h.poll();
  h.requests.at(-1).reject(Error("temporary network failure")); await poll;
  assert.equal(h.errors.length, 1);
  assert.equal(h.timers.size, 1);
  assert.equal(h.coordinator.staged.revision, 1);
  const retry = h.poll(); h.requests.at(-1).resolve(snapshot(2)); await retry;
  assert.equal(h.timers.size, 0);
  assert.equal(h.coordinator.staged.revision, 2);
});

test("failed applications retain their stage for an explicit retry", async () => {
  const h = harness(), value = snapshot(1);
  await h.respond(value);
  h.applyWork(() => { throw Error("presentation failed"); });
  assert.equal(await h.coordinator.apply(), false);
  assert.equal(h.coordinator.staged, value);
  assert.equal(h.errors.length, 1);
  assert.equal(h.coordinator.applying, false);
  h.applyWork(() => {});
  assert.equal(await h.coordinator.apply(), true);
  assert.equal(h.coordinator.staged, null);
});

test("simultaneous apply calls share one idle wait and one application", async () => {
  const h = harness(), idle = deferred(); let waits = 0;
  await h.respond(snapshot(1));
  h.idle(() => { waits++; return idle.promise; });
  const first = h.coordinator.apply(), second = h.coordinator.apply();
  assert.equal(first, second);
  idle.resolve(); await first;
  assert.equal(waits, 1); assert.equal(h.applied.length, 1);
});

test("an asynchronous apply does not clear a newer stage accepted during rendering", async () => {
  const h = harness(), rendering = deferred();
  await h.respond(snapshot(1)); h.applyWork(() => rendering.promise);
  const apply = h.coordinator.apply();
  await Promise.resolve(); await Promise.resolve();
  await h.respond(snapshot(2));
  rendering.resolve(); assert.equal(await apply, true);
  assert.equal(h.coordinator.staged.revision, 2);
});

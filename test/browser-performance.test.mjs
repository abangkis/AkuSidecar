import test from "node:test";
import assert from "node:assert/strict";
import { createBrowserPerformanceSampler } from "../internal/httpapi/web/diagnostics/browser-performance.js";

function fixture(supported = [], unavailable = []) {
  let now = 0, frame = null, timer = null, timerDelay, clockReads = 0;
  const observers = [], reports = [], cancellations = [];
  class Observer {
    static supportedEntryTypes = supported;
    constructor(callback) { this.callback = callback; this.pending = []; observers.push(this); }
    observe(options) {
      this.options = options;
      if (unavailable.includes(options.type)) throw new Error("unsupported");
    }
    takeRecords() { return this.pending.splice(0); }
    disconnect() { this.disconnected = true; }
    emit(entries) { this.callback({ getEntries: () => entries }); }
  }
  const sampler = createBrowserPerformanceSampler({
    stageNames: ["grid_layout", "panel_open"],
    stopReasons: { allowed: ["finish", "deadline", "background"], default: "finish", timeout: "deadline" },
    performance: { now: () => { clockReads++; return now; } }, Observer,
    requestFrame: callback => { frame = callback; return 1; },
    cancelFrame: id => { cancellations.push(id); frame = null; },
    setTimer: (callback, delay) => { timer = callback; timerDelay = delay; return 2; },
    clearTimer: id => { cancellations.push(id); timer = null; },
    onStop: report => reports.push(report),
  });
  return { sampler, observers, reports, cancellations, advance: value => { now = value; },
    tick: value => { now = value; frame(value); }, timeout: () => timer(),
    get frame() { return frame; }, get timer() { return timer; },
    get timerDelay() { return timerDelay; }, get clockReads() { return clockReads; } };
}

test("independent consumer stays inert until start and accepts only its approved stages and reasons", () => {
  const f = fixture();
  f.sampler.scroll(); f.sampler.mediaLoad();
  assert.equal(f.sampler.measure("grid_layout", () => 42), 42);
  assert.equal(f.sampler.stop(), null);
  assert.equal(f.clockReads, 0);
  assert.equal(f.frame, null);
  assert.equal(f.timer, null);
  assert.equal(f.observers.length, 0);
  f.advance(100);
  assert.equal(f.sampler.start(), true);
  assert.equal(f.sampler.start(), false);
  assert.equal(f.timerDelay, 30000);
  const reads = f.clockReads;
  f.sampler.measure("timeline_render", () => f.advance(110));
  assert.equal(f.clockReads, reads);
  assert.equal(f.sampler.measure("grid_layout", () => { f.advance(116); return 7; }), 7);
  assert.throws(() => f.sampler.measure("panel_open", () => { f.advance(120); throw new Error("work"); }), /work/);
  f.sampler.scroll(); f.sampler.mediaLoad();
  const report = f.sampler.stop("hidden");
  assert.equal(report.reason, "finish");
  assert.equal(report.durationMs, 20);
  assert.deepEqual(report.stages, [
    { name: "grid_layout", count: 1, totalMs: 6, maxMs: 6 },
    { name: "panel_open", count: 1, totalMs: 4, maxMs: 4 },
  ]);
  assert.deepEqual(report.events, [
    { kind: "stage", atMs: 10, name: "grid_layout", durationMs: 6 },
    { kind: "stage", atMs: 16, name: "panel_open", durationMs: 4 },
    { kind: "media_load", atMs: 20 },
  ]);
  assert.equal(report.scrollEvents, 1);
  assert.deepEqual(report.support, { longAnimationFrame: false, longTask: false, layoutShift: false });
  assert.deepEqual(f.cancellations, [1, 2]);
  assert.equal(f.frame, null);
  assert.equal(f.timer, null);
  assert.equal(f.sampler.stop(), null);
  assert.equal(f.reports.length, 1);
  f.advance(200); f.sampler.start();
  assert.equal(f.sampler.stop("background").reason, "background");
  assert.equal(f.reports[1].events.length, 0);
  assert.equal(f.reports[1].stages.length, 0);
  assert.equal(f.reports[1].scrollEvents, 0);
  f.sampler.start(90000);
  assert.equal(f.timerDelay, 60000);
  f.timeout();
  assert.equal(f.reports[2].reason, "deadline");
  assert.equal(f.sampler.active, false);
});

test("observer flush retains numeric projection and excludes private fields and historical records", () => {
  const f = fixture(["long-animation-frame", "longtask", "layout-shift"]);
  f.advance(100); f.sampler.start();
  f.observers[0].emit([{ startTime: 99, duration: 80 }, { startTime: 110, duration: 90,
    blockingDuration: 40, styleAndLayoutStart: 180, scripts: [{ duration: 60,
      forcedStyleAndLayoutDuration: 3, sourceURL: "SECRET", invoker: "SECRET" }] }]);
  f.observers[1].pending.push({ startTime: 111, duration: 60, attribution: [{ name: "SECRET" }] });
  f.observers[2].pending.push({ startTime: 112, value: 0.1, hadRecentInput: true, sources: [{ node: "SECRET" }] });
  f.advance(220);
  const report = f.sampler.stop();
  assert.deepEqual(report.support, { longAnimationFrame: true, longTask: true, layoutShift: true });
  assert.deepEqual(report.events, [
    { kind: "loaf", atMs: 10, durationMs: 90, blockingMs: 40, scriptMs: 60, forcedLayoutMs: 3, styleLayoutMs: 20 },
    { kind: "longtask", atMs: 11, durationMs: 60 },
    { kind: "layout_shift", atMs: 12, value: 0.1, hadRecentInput: true },
  ]);
  assert.equal(JSON.stringify(report).includes("SECRET"), false);
  assert.ok(f.observers.every(observer => observer.disconnected));
  assert.ok(f.observers.every(observer => !("buffered" in observer.options)));
  f.observers[0].emit([{ startTime: 230, duration: 100 }]);
  assert.equal(report.events.length, 3);
});

test("unsupported observers are explicit and custom consumer retains bounded frame and event sampling", () => {
  const f = fixture(["longtask"], ["longtask"]);
  f.sampler.start(0);
  assert.equal(f.timerDelay, 30000);
  for (let index = 0; index < 510; index++) f.sampler.mediaLoad();
  for (let index = 0; index < 6010; index++) f.tick(index * 40);
  const report = f.sampler.stop();
  assert.equal(report.events.length, 500);
  assert.equal(report.droppedEvents, 6019);
  assert.deepEqual(report.frameGaps, { count: 6009, maxMs: 40, p50Ms: 40, p95Ms: 40, over32Ms: 6009 });
  assert.equal(report.droppedFrameSamples, 9);
  assert.equal(report.durationMs, 60000);
  assert.deepEqual(report.support, { longAnimationFrame: false, longTask: false, layoutShift: false });
});

test("generic consumer keeps the byte cap and clamps non-finite observer timings", () => {
  const f = fixture(["long-animation-frame"]); f.sampler.start();
  f.observers[0].emit(Array.from({ length: 500 }, () => ({ startTime: 1.123456789012345,
    duration: 100.12345678901234, blockingDuration: 90.12345678901234,
    styleAndLayoutStart: 2.123456789012345,
    scripts: [{ duration: 80.12345678901234, forcedStyleAndLayoutDuration: 70.12345678901234 }] })));
  const report = f.sampler.stop();
  assert.ok(report.droppedEvents > 0);
  assert.ok(Buffer.byteLength(JSON.stringify(report)) < 65536);
  f.sampler.start();
  f.observers[1].emit([{ startTime: 1, duration: Infinity, blockingDuration: -1,
    scripts: [{ duration: NaN, forcedStyleAndLayoutDuration: Infinity }] }]);
  assert.deepEqual(f.sampler.stop().events[0], {
    kind: "loaf", atMs: 1, durationMs: 0, blockingMs: 0, scriptMs: 0, forcedLayoutMs: 0, styleLayoutMs: 0,
  });
});

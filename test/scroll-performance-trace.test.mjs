import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';
import { createScrollPerformanceTrace } from '../internal/httpapi/web/scroll-performance-trace.js';

function fixture(supported = []) {
  let now = 0, frame, timer;
  const observers = [], reports = [];
  class Observer {
    static supportedEntryTypes = supported;
    constructor(callback) { this.callback = callback; observers.push(this); }
    observe(options) { this.options = options; }
    takeRecords() { return []; }
    disconnect() { this.disconnected = true; }
    emit(entries) { this.callback({ getEntries: () => entries }); }
  }
  const trace = createScrollPerformanceTrace({ performance: { now: () => now }, Observer,
    requestFrame: callback => { frame = callback; return 1; }, cancelFrame: () => { frame = null; },
    setTimer: callback => { timer = callback; return 2; }, clearTimer: () => { timer = null; },
    onStop: report => reports.push(report) });
  return { trace, observers, reports, advance: value => { now = value; },
    tick: value => { now = value; frame(value); }, timeout: () => timer(), get frame() { return frame; } };
}

test('disabled trace is inert; active frame, stage and scroll timings stop and clean up', () => {
  const f = fixture();
  f.trace.scroll();
  assert.equal(f.trace.measure('timeline_render', () => 42), 42);
  assert.equal(f.trace.stop(), null);
  assert.equal(f.trace.start(), true);
  assert.equal(f.trace.start(), false);
  f.trace.scroll(); f.tick(10); f.tick(60);
  f.trace.measure('timeline_render', () => f.advance(70));
  const report = f.trace.stop();
  assert.equal(report.scrollEvents, 1);
  assert.equal(report.frameGaps.maxMs, 50);
  assert.equal(report.frameGaps.over32Ms, 1);
  assert.equal(report.stages[0].totalMs, 10);
  assert.deepEqual(report.support, { longAnimationFrame: false, longTask: false, layoutShift: false });
  assert.equal(f.frame, null);
  assert.equal(f.reports.length, 1);
  assert.equal(f.trace.stop(), null);
});

test('observer projection drops URLs, attribution, nodes and historical records', () => {
  const f = fixture(['long-animation-frame', 'longtask', 'layout-shift']);
  f.advance(100); f.trace.start();
  f.observers[0].emit([{ startTime: 99, duration: 80 }, { startTime: 110, duration: 90,
    blockingDuration: 40, styleAndLayoutStart: 180, scripts: [{ duration: 60,
      forcedStyleAndLayoutDuration: 3, sourceURL: 'SECRET', invoker: 'SECRET' }] }]);
  f.observers[1].emit([{ startTime: 111, duration: 60, attribution: [{ name: 'SECRET' }] }]);
  f.observers[2].emit([{ startTime: 112, value: 0.1, hadRecentInput: true, sources: [{ node: 'SECRET' }] }]);
  f.advance(220);
  const report = f.trace.stop('hidden');
  assert.equal(report.events.length, 3);
  assert.equal(report.events[0].styleLayoutMs, 20);
  assert.equal(report.events[0].forcedLayoutMs, 3);
  assert.equal(JSON.stringify(report).includes('SECRET'), false);
  assert.equal(report.reason, 'hidden');
  assert.ok(f.observers.every(observer => observer.disconnected));
  assert.ok(f.observers.every(observer => !('buffered' in observer.options)));
});

test('event and frame sample caps preserve aggregate counts; timeout stops recording', () => {
  const f = fixture(); f.trace.start();
  for (let index = 0; index < 510; index++) f.trace.mediaLoad();
  for (let index = 0; index < 6010; index++) f.tick(index);
  f.timeout();
  const report = f.reports[0];
  assert.equal(report.events.length, 500);
  assert.equal(report.droppedEvents, 10);
  assert.equal(report.frameGaps.count, 6009);
  assert.equal(report.droppedFrameSamples, 9);
  assert.equal(report.reason, 'timeout');
  assert.equal(f.trace.active, false);
});

test('measured throwing work preserves its exception and records its cost', () => {
  const f = fixture(); f.trace.start();
  assert.throws(() => f.trace.measure('side_pane', () => { f.advance(6); throw new Error('work'); }), /work/);
  assert.equal(f.trace.stop().stages[0].maxMs, 6);
});

test('long observer records stay below the endpoint byte limit', () => {
  const f = fixture(['long-animation-frame']); f.trace.start();
  f.observers[0].emit(Array.from({ length: 500 }, () => ({ startTime: 1.123456789012345,
    duration: 100.12345678901234, blockingDuration: 90.12345678901234,
    styleAndLayoutStart: 2.123456789012345,
    scripts: [{ duration: 80.12345678901234, forcedStyleAndLayoutDuration: 70.12345678901234 }] })));
  const report = f.trace.stop();
  assert.ok(report.droppedEvents > 0);
  assert.ok(Buffer.byteLength(JSON.stringify(report)) < 65536);
});

test('actual UI trace flow opens Timeline, uploads one object and recovers from save failure', async () => {
  const app = readFileSync(new URL('../internal/httpapi/web/app.js', import.meta.url), 'utf8');
  const source = app.slice(app.indexOf('let lastScrollPerformanceReport = null;'), app.indexOf('function exportDiagnostics()'));
  const elements = new Map();
  const element = selector => {
    if (!elements.has(selector)) elements.set(selector, { disabled: false, textContent: '',
      classList: { add() {}, remove() {} } });
    return elements.get(selector);
  };
  const calls = [];
  const context = vm.createContext({ $: element, setView: view => calls.push(view),
    scrollPerformanceTrace: { active: false, start: duration => calls.push(duration) },
    api: async (path, options) => { calls.push({ path, body: options.body }); } });
  vm.runInContext(source, context);
  vm.runInContext('startScrollPerformanceTrace()', context);
  assert.deepEqual(calls, ['timeline', 30000]);
  assert.equal(element('#start-scroll-trace').disabled, true);
  context.report = { version: 1 };
  await vm.runInContext('saveScrollPerformanceTrace(report)', context);
  assert.equal(calls[2].body, context.report);
  assert.equal(calls[2].path, '/api/diagnostics/ui-performance');
  assert.equal(element('#start-scroll-trace').disabled, false);
  context.api = async () => { throw new Error('offline'); };
  await vm.runInContext('saveScrollPerformanceTrace(report)', context);
  assert.match(element('#scroll-trace-status').textContent, /could not be saved/);
  assert.equal(element('#download-scroll-trace').disabled, false);
  assert.equal(element('#start-scroll-trace').disabled, false);
});

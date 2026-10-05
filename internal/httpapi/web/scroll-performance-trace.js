// Opt-in, bounded timing diagnostics. Never retain DOM nodes, resource URLs,
// script URLs, invokers, post content or native-reader capabilities.
export function createScrollPerformanceTrace({
  performance = globalThis.performance,
  requestFrame = callback => requestAnimationFrame(callback),
  cancelFrame = id => cancelAnimationFrame(id),
  setTimer = setTimeout, clearTimer = clearTimeout,
  Observer = globalThis.PerformanceObserver, onStop = () => {},
} = {}) {
  const names = new Set(["scroll_frame", "content_context_scroll", "back_to_top", "side_pane", "timeline_render"]);
  const ms = value => Math.min(60000, Math.max(0, Number.isFinite(value) ? value : 0));
  let active = false, started = 0, frame = null, timer = null, previousFrame = null;
  let report, observers = [], samples = [], eventBytes = 0;
  function event(kind, at, fields = {}) {
    if (!active || at < started) return;
    const record = { kind, atMs: ms(at - started), ...fields };
    const bytes = JSON.stringify(record).length + 1; // Numeric fields and enum names are ASCII.
    if (report.events.length >= 500 || eventBytes + bytes > 48000) { report.droppedEvents++; return; }
    eventBytes += bytes;
    report.events.push(record);
  }
  function entries(type, records) {
    for (const entry of records) {
      if (!active || entry.startTime < started) continue;
      if (type === "long-animation-frame") {
        const scripts = Array.isArray(entry.scripts) ? entry.scripts : [];
        const end = entry.startTime + entry.duration;
        event("loaf", entry.startTime, {
          durationMs: ms(entry.duration), blockingMs: ms(entry.blockingDuration),
          scriptMs: ms(scripts.reduce((sum, script) => sum + ms(script.duration), 0)),
          forcedLayoutMs: ms(scripts.reduce((sum, script) => sum + ms(script.forcedStyleAndLayoutDuration), 0)),
          styleLayoutMs: ms(entry.styleAndLayoutStart > 0 ? end - entry.styleAndLayoutStart : 0),
        });
      } else if (type === "longtask") {
        event("longtask", entry.startTime, { durationMs: ms(entry.duration) });
      } else {
        event("layout_shift", entry.startTime, { value: ms(entry.value), hadRecentInput: Boolean(entry.hadRecentInput) });
      }
    }
  }
  function tick(at) {
    if (!active) return;
    if (previousFrame !== null) {
      const gap = ms(at - previousFrame);
      report.frameGaps.count++;
      report.frameGaps.maxMs = Math.max(report.frameGaps.maxMs, gap);
      if (samples.length < 6000) samples.push(gap); else report.droppedFrameSamples++;
      if (gap > 32) { report.frameGaps.over32Ms++; event("frame_gap", at, { durationMs: gap }); }
    }
    previousFrame = at;
    frame = requestFrame(tick);
  }
  function start(durationMs = 30000) {
    if (active) return false;
    started = performance.now(); active = true; samples = []; observers = []; previousFrame = null; eventBytes = 0;
    report = { version: 1, durationMs: 0, reason: "manual", support: {},
      frameGaps: { count: 0, maxMs: 0, p50Ms: 0, p95Ms: 0, over32Ms: 0 },
      scrollEvents: 0, droppedEvents: 0, droppedFrameSamples: 0, stages: [], events: [] };
    for (const [type, key] of [["long-animation-frame", "longAnimationFrame"], ["longtask", "longTask"], ["layout-shift", "layoutShift"]]) {
      report.support[key] = false;
      if (!Observer?.supportedEntryTypes?.includes(type)) continue;
      try {
        const observer = new Observer(list => entries(type, list.getEntries()));
        observer.observe({ type }); // No historical buffered entries.
        observers.push({ observer, type }); report.support[key] = true;
      } catch { /* Unsupported APIs stay explicitly unavailable. */ }
    }
    frame = requestFrame(tick);
    timer = setTimer(() => stop("timeout"), Math.max(1, ms(durationMs) || 30000));
    return true;
  }
  function stop(reason = "manual") {
    if (!active) return null;
    for (const { observer, type } of observers) {
      entries(type, observer.takeRecords()); observer.disconnect();
    }
    active = false;
    cancelFrame(frame); clearTimer(timer); frame = timer = null;
    report.durationMs = ms(performance.now() - started);
    report.reason = ["manual", "timeout", "hidden"].includes(reason) ? reason : "manual";
    samples.sort((a, b) => a - b);
    report.frameGaps.p50Ms = samples.length ? samples[Math.floor((samples.length - 1) * 0.5)] : 0;
    report.frameGaps.p95Ms = samples.length ? samples[Math.floor((samples.length - 1) * 0.95)] : 0;
    onStop(report);
    return report;
  }
  function measure(name, work) {
    if (!active || !names.has(name)) return work();
    const begin = performance.now();
    try { return work(); }
    finally {
      const durationMs = ms(performance.now() - begin);
      let stage = report.stages.find(stage => stage.name === name);
      if (!stage) { stage = { name, count: 0, totalMs: 0, maxMs: 0 }; report.stages.push(stage); }
      stage.count++; stage.totalMs += durationMs; stage.maxMs = Math.max(stage.maxMs, durationMs);
      if (durationMs >= 4) event("stage", begin, { name, durationMs });
    }
  }
  return { start, stop, measure, get active() { return active; },
    scroll() { if (active) report.scrollEvents++; },
    mediaLoad() { if (active) event("media_load", performance.now()); },
  };
}

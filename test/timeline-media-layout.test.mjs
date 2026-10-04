import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";
import { reserveMediaDimensions, renderWithCurrentScroll } from "../internal/httpapi/web/timeline-media-layout.js";

test("known dimensions reserve intrinsic media size; unknown or invalid dimensions invent no ratio", () => {
  const known = {};
  assert.equal(reserveMediaDimensions(known, { width: 1200, height: 800 }), true);
  assert.deepEqual(known, { width: 1200, height: 800 });
  for (const media of [{}, { width: 100 }, { width: 0, height: 10 }, { width: -2, height: 1 },
    { width: Infinity, height: 2 }, { width: 2.5, height: 2 }, { width: 100001, height: 1 }]) {
    const unknown = {};
    assert.equal(reserveMediaDimensions(unknown, media), false);
    assert.deepEqual(unknown, {});
  }
});

test("scroll restoration uses current position in the render task and explicitly bypasses smooth CSS", () => {
  const calls = [];
  const viewport = { scrollY: 1800, scrollTo: value => calls.push(value) };
  renderWithCurrentScroll(() => { viewport.scrollY = 0; calls.push("render"); }, viewport);
  assert.deepEqual(calls, ["render", { top: 1800, behavior: "instant" }]);
});

test("actual refresh preserves user's movement during network acquisition, with no delayed restore callback", async () => {
  const app = readFileSync(new URL("../internal/httpapi/web/app.js", import.meta.url), "utf8");
  let respond; const restored = []; let rendered = 0;
  const window = { scrollY: 900, scrollTo: value => restored.push(value) };
  const context = { state: {}, window,
    api: () => new Promise(resolve => { respond = resolve; }),
    renderAutoUpdateStatus() {}, renderTimeline() { rendered++; window.scrollY = 0; },
    renderWithCurrentScroll: render => renderWithCurrentScroll(render, window),
    showError: error => assert.fail(error),
    requestAnimationFrame: () => assert.fail("no stale deferred callback"),
  };
  vm.createContext(context);
  vm.runInContext(app.slice(app.indexOf("async function refreshTimeline(options"), app.indexOf("function orderTimelineForRevealedBatch")), context);
  const update = context.refreshTimeline({ preserveScroll: true });
  window.scrollY = 1800; // Continued scrolling while the server responds.
  respond({ items: [], timelineBatches: [] }); await update;
  assert.equal(rendered, 1);
  assert.deepEqual(restored, [{ top: 1800, behavior: "instant" }]);
});

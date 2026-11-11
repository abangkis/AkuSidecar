import assert from "node:assert/strict";
import test from "node:test";
import { createTimelineVideoQuality } from "../internal/httpapi/web/timeline-video-quality.js";

class Element extends EventTarget {
  children = [];
  value = "";
  constructor(tag) { super(); this.tagName = tag; }
  append(...children) { this.children.push(...children); }
  replaceChildren(...children) { this.children = children; }
  setAttribute(name, value) { this[name] = value; }
  remove() { this.removed = true; }
}

test("quality selector keeps requested and playing quality distinct and returns to Auto", t => {
  const previous = globalThis.document;
  globalThis.document = { createElement: tag => new Element(tag) };
  t.after(() => { globalThis.document = previous; });
  const selections = [];
  const control = createTimelineVideoQuality(value => selections.push(value));
  const [label, status] = control.element.children;
  const select = label.children[1];
  assert.equal(select.disabled, true);
  assert.equal(select.value, "auto");
  const qualities = [{ id: "low", label: "360p", height: 360 }, { id: "high", label: "720p", height: 720 }];
  control.update({ qualities, selected: "auto", active: "low" });
  assert.equal(select.disabled, false);
  assert.equal(status.textContent, "Auto · 360p");
  select.value = "high";
  select.dispatchEvent(new Event("change"));
  assert.deepEqual(selections, ["high"]);
  control.update({ qualities, selected: "high", active: "low", pending: true });
  assert.equal(select.value, "high");
  assert.equal(status.textContent, "720p selected · Playing 360p");
  control.update({ qualities, selected: "high", active: "high" });
  assert.equal(status.textContent, "720p");
  select.value = "auto";
  select.dispatchEvent(new Event("change"));
  control.update({ qualities, selected: "auto", active: "high" });
  assert.equal(status.textContent, "Auto · 720p");
  assert.deepEqual(selections, ["high", "auto"]);
  control.dispose();
  assert.equal(control.element.removed, true);
  select.dispatchEvent(new Event("change"));
  assert.equal(selections.length, 2);
});

test("single-level video hides an unnecessary manual quality selector", t => {
  const previous = globalThis.document;
  globalThis.document = { createElement: tag => new Element(tag) };
  t.after(() => { globalThis.document = previous; });
  const control = createTimelineVideoQuality(() => assert.fail("unexpected selection"));
  control.update({ qualities: [{ id: "only", label: "480p" }], selected: "auto", active: "only" });
  assert.equal(control.element.hidden, true);
});

import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";
import { createFrameTaskQueue, setInlineStyle, setAttributeValue } from "../internal/httpapi/web/ui-frame.js";

test("a scroll burst coalesces tasks and defers requests made during a flush", () => {
  const frames = [], calls = [];
  const queue = createFrameTaskQueue({
    requestFrame: callback => frames.push(callback),
    run: tasks => {
      calls.push([...tasks]);
      if (calls.length === 1) queue.schedule("side-pane");
    },
  });
  for (let index = 0; index < 100; index++) queue.schedule("scroll");
  queue.schedule("back-to-top");
  assert.equal(frames.length, 1);
  assert.deepEqual(calls, []);
  frames.shift()();
  assert.deepEqual(calls, [["scroll", "back-to-top"]]);
  assert.equal(frames.length, 1);
  frames.shift()();
  assert.deepEqual(calls[1], ["side-pane"]);
  queue.schedule("scroll");
  assert.equal(frames.length, 1);
});

test("Related Context measures all cards before writing and skips other views", () => {
  const app = fs.readFileSync(new URL("../internal/httpapi/web/app.js", import.meta.url), "utf8");
  const start = app.indexOf("function measureTimelineContentContextTabs() {");
  const end = app.indexOf("\nfunction timelineContentContextFeedbackKey", start);
  assert.ok(start >= 0 && end > start);
  const operations = [], visibility = new Map();
  const tabs = ["first", "second"].map(id => ({
    dataset: { timelineContentContextId: id },
    closest: selector => selector === ".timeline-content-context-anchor" ? {
      getBoundingClientRect: () => { operations.push("read"); return { top: 100, bottom: 500, right: 500 }; },
    } : null,
    getBoundingClientRect: () => { operations.push("read"); return { width: 30 }; },
    classList: { toggle: (name, value) => { operations.push("write"); visibility.set(id + name, value); } },
  }));
  const state = { currentView: "timeline", timelineContentContextDrawerOpen: false };
  const context = vm.createContext({ state,
    document: { documentElement: { clientWidth: 1200 }, querySelectorAll: () => tabs },
    window: { innerWidth: 1200, innerHeight: 900 },
    selectContentContextViewportID: () => "second",
    contentContextTabFits: () => true,
    syncTimelineContentContextTab: () => operations.push("write"),
    CONTENT_CONTEXT_TAB_DEFAULT_WIDTH: 30,
  });
  vm.runInContext(app.slice(start, end), context);
  vm.runInContext("syncTimelineContentContextTabs()", context);
  assert.deepEqual(operations.slice(0, 4), ["read", "read", "read", "read"]);
  assert.ok(operations.slice(4).every(operation => operation === "write"));
  assert.equal(visibility.get("firstis-visible"), false);
  assert.equal(visibility.get("secondis-visible"), true);
  operations.length = 0;
  state.currentView = "settings";
  vm.runInContext("syncTimelineContentContextTabs()", context);
  assert.deepEqual(operations, []);
});

test("unchanged styles and attributes do not mutate the DOM", () => {
  const styles = new Map(), attributes = new Map(), writes = [];
  const element = {
    style: {
      getPropertyValue: name => styles.get(name) || "",
      setProperty: (name, value) => { styles.set(name, value); writes.push("style"); },
      removeProperty: name => { styles.delete(name); writes.push("remove"); },
    },
    getAttribute: name => attributes.get(name) ?? null,
    setAttribute: (name, value) => { attributes.set(name, value); writes.push("attribute"); },
  };
  setInlineStyle(element, "left", "12px");
  setAttributeValue(element, "aria-expanded", "false");
  for (let index = 0; index < 100; index++) {
    setInlineStyle(element, "left", "12px");
    setAttributeValue(element, "aria-expanded", "false");
  }
  assert.deepEqual(writes, ["style", "attribute"]);
  setInlineStyle(element, "left", "");
  setInlineStyle(element, "left", "");
  assert.deepEqual(writes, ["style", "attribute", "remove"]);
});

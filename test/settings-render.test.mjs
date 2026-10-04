import test from "node:test";
import assert from "node:assert/strict";
import { setSettingsText, setSettingsClass } from "../internal/httpapi/web/settings-render.js";

test("unchanged polling preserves text nodes and classes; changes still render", () => {
  let text = "ready", className = "source-session-status", writes = 0;
  const element = {
    get textContent() { return text; }, set textContent(value) { writes++; text = value; },
    get className() { return className; }, set className(value) { writes++; className = value; },
  };
  for (let i = 0; i < 10; i++) {
    setSettingsText(element, "ready");
    setSettingsClass(element, "source-session-status");
  }
  assert.equal(writes, 0);
  setSettingsText(element, "loading");
  setSettingsClass(element, "source-session-status source-session-warning");
  assert.equal(writes, 2);
});

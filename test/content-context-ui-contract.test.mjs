import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";

const app = fs.readFileSync(new URL("../internal/httpapi/web/app.js", import.meta.url), "utf8");
const state = fs.readFileSync(new URL("../internal/httpapi/web/timeline-content-context-state.js", import.meta.url), "utf8");
const styles = fs.readFileSync(new URL("../internal/httpapi/web/styles.css", import.meta.url), "utf8");

test("Timeline Content Context is explicit, lazy, bounded, and accessible", () => {
  for (const marker of [
    "timeline-content-context-state.js",
    "buildTimelineContentContextAction",
    "Find related context",
    "Searching local Personal Memory",
    "No related local context found.",
    "timeline-content-context-reason",
    "buildTimelineContentContextPath(entry.id)",
  ]) assert.match(app, new RegExp(marker.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")), `app missing ${marker}`);
  assert.match(state, /CONTENT_CONTEXT_DEFAULT_LIMIT = 3/);
  assert.match(state, /CONTENT_CONTEXT_MAX_LIMIT = 5/);
  assert.match(styles, /\.timeline-content-context/);
  assert.match(styles, /\.timeline-content-context-panel/);
  assert.doesNotMatch(app, /schedule.*ContentContext/i);
});

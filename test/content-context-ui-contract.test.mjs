import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";

const app = fs.readFileSync(new URL("../internal/httpapi/web/app.js", import.meta.url), "utf8");
const index = fs.readFileSync(new URL("../internal/httpapi/web/index.html", import.meta.url), "utf8");
const state = fs.readFileSync(new URL("../internal/httpapi/web/timeline-content-context-state.js", import.meta.url), "utf8");
const styles = fs.readFileSync(new URL("../internal/httpapi/web/styles.css", import.meta.url), "utf8");

test("Timeline Content Context is explicit, lazy, bounded, and accessible", () => {
  for (const marker of [
    "timeline-content-context-state.js",
    "timeline-content-context-tab",
    "Related context",
    "timeline-content-context-drawer",
    "timeline-content-context-close",
    "openTimelineContentContext",
    "closeTimelineContentContextDrawer",
    "handleTimelineContentContextScroll",
    "syncTimelineContentContextTab",
    "contentContextTabPlacement",
    "backToTopBoundaryBottom",
    "aria-controls",
    "Searching local Personal Memory",
    "No related local context found.",
    "timeline-content-context-reason",
    "buildTimelineContentContextPath(entry.id)",
  ]) assert.match(app, new RegExp(marker.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")), `app missing ${marker}`);
  assert.match(state, /CONTENT_CONTEXT_DEFAULT_LIMIT = 3/);
  assert.match(state, /CONTENT_CONTEXT_MAX_LIMIT = 5/);
  assert.match(state, /function backToTopBoundaryBottom/);
  assert.match(state, /function contentContextTabPlacement/);
  assert.match(index, /id="timeline-content-context-tab"[^>]*aria-controls="timeline-content-context-drawer"[^>]*aria-expanded="false"[^>]*>\s*Related context/);
  assert.doesNotMatch(index, /Find related context/);
  assert.match(styles, /\.timeline-content-context/);
  assert.match(styles, /\.timeline-content-context-tab/);
  assert.match(styles, /\.timeline-content-context-drawer/);
  assert.match(styles, /prefers-reduced-motion/);
  assert.doesNotMatch(styles, /\.timeline-content-context-drawer[^}]*transition:[^}]*\btop\b/i);
  assert.doesNotMatch(app, /timeline-content-context-panel/);
  assert.doesNotMatch(app, /buildTimelineContentContextAction|timeline-content-context-trigger|Find related context/);
  assert.match(app, /const active = candidates\.find\(\(\{ element \}\) => element\.dataset\.timelineId === state\.timelineContentContextActiveID\);[\s\S]*state\.timelineContentContextActiveID = "";/);
  assert.match(app, /if \(active\) return active\.element;/);
  assert.match(app, /return \(candidates\.find\(\(\{ rect \}\) => rect\.top >= 0\) \|\| candidates\[0\]\)\.element;/);
  assert.doesNotMatch(app, /if \(active\) return active;/);
  assert.match(app, /syncBackToTopPosition\(top\);\r?\n    syncTimelineContentContextTab\(\);/);
  assert.match(app, /container\.append\(rendered\);\r?\n    observeTimelineItem\(rendered, entry\.id\);\r?\n  \}\r?\n  syncTimelineContentContextTab\(\);/);
  assert.doesNotMatch(app, /function handleTimelineContentContextScroll\(\)[\s\S]*?scheduleTimelineContentContextPosition\(\)/);
  assert.doesNotMatch(app, /api\([^)]*content-context[^)]*,\s*\{\s*method:\s*["']POST/i);
});

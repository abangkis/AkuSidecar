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
    "syncTimelineContentContextTabs",
    "contentContextRailPlacement",
    "contentContextTabFits",
    "contentContextPostPassedReadingExitLine",
    "timelineContentContextOverlapsBackToTop",
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
  assert.match(state, /function contentContextTabFits/);
  assert.match(state, /CONTENT_CONTEXT_READING_EXIT_RATIO = 0\.2/);
  assert.match(state, /function contentContextPostPassedReadingExitLine/);
  assert.doesNotMatch(index, /id="timeline-content-context-tab"/);
  assert.match(app, /function buildTimelineContentContextAnchor/);
  assert.match(app, /const viewportWidth = document\.documentElement\.clientWidth \|\| window\.innerWidth;/);
  assert.match(app, /const safeRightBoundary = viewportWidth - 16;/);
  assert.match(app, /--timeline-content-context-document-left", `\$\{window\.scrollX \+ postRect\.right\}px`/);
  assert.match(app, /--timeline-content-context-document-top", `\$\{window\.scrollY \+ postRect\.top\}px`/);
  assert.match(app, /--timeline-content-context-width", `\$\{width\}px`/);
  assert.match(app, /tab\.dataset\.timelineContentContextId = entry\.id/);
  assert.match(app, /tab\.setAttribute\("aria-controls", "timeline-content-context-drawer"\)/);
  assert.doesNotMatch(index, /Find related context/);
  assert.match(styles, /\.timeline-content-context/);
  assert.match(styles, /\.timeline-content-context-tab/);
  assert.match(styles, /\.timeline-content-context-anchor \{[^}]*position: relative/);
  assert.match(styles, /\.timeline-content-context-tab \{[^}]*border-left: 0;[^}]*border-radius: 0 11px 11px 0;[^}]*writing-mode: vertical-rl/);
  assert.match(styles, /\.timeline-content-context-tab \{[^}]*top: 0;/);
  assert.doesNotMatch(styles, /\.timeline-content-context-tab(?:\.is-visible|\.is-retracting)? \{[^}]*rotate\(180deg\)/);
  assert.match(styles, /\.timeline-side-pane-toggle \{[^}]*rotate\(180deg\)/);
  assert.match(styles, /\.timeline-content-context-drawer\.is-rail \{[^}]*position: absolute;[^}]*right: auto;[^}]*bottom: auto;[^}]*left: var\(--timeline-content-context-document-left[^}]*border-left: 0;[^}]*border-radius: 0 16px 16px 0;/);
  assert.match(styles, /\.timeline-content-context-drawer/);
  assert.match(styles, /prefers-reduced-motion/);
  assert.doesNotMatch(styles, /\.timeline-content-context-drawer[^}]*transition:[^}]*\btop\b/i);
  assert.doesNotMatch(app, /timeline-content-context-panel/);
  assert.doesNotMatch(app, /Find related context/);
  assert.match(app, /const item = buildTimelineItem\(entry, \{ contentContext: false \}\)/);
  assert.match(app, /if \(expanded && state\.timelineContentContextActiveID === entry\.id\)[\s\S]*closeTimelineContentContextDrawer\(\{ clearActive: true, focusTrigger: false \}\)/);
  assert.match(app, /report\.classList\.toggle\("hidden", expanded\)/);
  assert.match(app, /function timelineContentContextOverlapsBackToTop\([\s\S]*timelineContentContextDrawerOverlapsBackToTop/);
  assert.match(app, /scheduleTimelineContentContextPosition\(\);\r?\n  scheduleBackToTop\(\);/);
  assert.match(app, /syncTimelineContentContextTabs\(\);\r?\n  scheduleBackToTop\(\);/);
  assert.match(app, /function syncBackToTopNow\(\)[\s\S]*syncBackToTopPosition\(top\);[\s\S]*syncTimelineContentContextTabs\(\);/);
  assert.match(app, /container\.append\(rendered\);\r?\n    observeTimelineItem\(rendered, entry\.id\);\r?\n  \}\r?\n  syncTimelineContentContextTabs\(\);/);
  assert.doesNotMatch(app, /function handleTimelineContentContextScroll\(\)[\s\S]*?scheduleTimelineContentContextPosition\(\)/);
  assert.match(app, /function handleTimelineContentContextScroll\(\)[\s\S]*?contentContextPostPassedReadingExitLine/);
  assert.match(app, /function handleTimelineContentContextScroll\(\)[\s\S]*?closeTimelineContentContextDrawer\(\{[\s\S]*?clearActive: true/);
  assert.doesNotMatch(app, /function handleTimelineContentContextScroll\(\)[\s\S]*?revealTimelineContentContextDrawer\(/);
  assert.doesNotMatch(app, /api\([^)]*content-context[^)]*,\s*\{\s*method:\s*["']POST/i);
});

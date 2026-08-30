import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const index = readFileSync(new URL("../internal/httpapi/web/index.html", import.meta.url), "utf8");
const app = readFileSync(new URL("../internal/httpapi/web/app.js", import.meta.url), "utf8");
const styles = readFileSync(new URL("../internal/httpapi/web/styles.css", import.meta.url), "utf8");

test("Living Topics thin slice is visibly manual and on demand", () => {
  assert.match(index, /id="topics-view-button"[^>]*>Topics<\/button>/);
  assert.match(index, /LIVING TOPICS · THIN SLICE/);
  assert.match(index, /Nothing is monitored in the background/);
  assert.match(index, /id="living-topic-create-form"/);
  assert.match(index, /id="living-topic-evidence-search-form"/);
  assert.match(index, /id="living-topic-snapshot-button"[^>]*>Create snapshot<\/button>/);
  assert.match(index, /Repeating with unchanged evidence records a truthful no-change snapshot without calling the provider/);
});

test("Living Topics UI exposes bounded membership and explicit mutations", () => {
  assert.match(app, /async function addLivingTopicMember\(memoryItemId\)/);
  assert.match(app, /async function removeLivingTopicMember\(memoryItemId\)/);
  assert.match(app, /async function createLivingTopicSnapshot\(\)/);
  assert.match(app, /detail\.members\.length >= 20/);
  assert.match(styles, /\.living-topics-layout \{[^}]*grid-template-columns/);
  assert.match(styles, /\.living-topic-snapshot-card \{/);
  assert.match(styles, /\.living-topics-layout \{ grid-template-columns: 1fr; \}/);
});

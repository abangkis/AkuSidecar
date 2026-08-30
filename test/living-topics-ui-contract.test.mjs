import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const index = readFileSync(new URL("../internal/httpapi/web/index.html", import.meta.url), "utf8");
const app = readFileSync(new URL("../internal/httpapi/web/app.js", import.meta.url), "utf8");
const styles = readFileSync(new URL("../internal/httpapi/web/styles.css", import.meta.url), "utf8");

test("Living Topics routes final posts while snapshots remain on demand", () => {
  assert.match(index, /id="topics-view-button"[^>]*>Living Topics<\/button>/);
  assert.match(index, /LIVING TOPICS · THIN SLICE/);
  assert.match(index, /Final, non-duplicate Timeline posts are sorted asynchronously/);
  assert.match(index, /id="living-topic-create-form"/);
  assert.match(index, /id="living-topic-evidence-search-form"/);
  assert.match(index, /id="living-topic-detail-tabs"[^>]*role="tablist"/);
  assert.match(index, /id="living-topic-snapshot-tab"[^>]*aria-selected="true"[^>]*>Snapshot<\/button>/);
  assert.match(index, /id="living-topic-evidence-tab"[^>]*>Manage evidence<\/button>/);
  assert.ok(index.indexOf('id="living-topic-snapshot-panel"') < index.indexOf('id="living-topic-evidence-panel"'));
  assert.match(index, /id="living-topic-snapshot-form"/);
  assert.match(index, /id="living-topic-snapshot-button"[^>]*type="submit"[^>]*>Create snapshot<\/button>/);
  assert.match(index, /id="living-topic-snapshot-status"[^>]*role="status"[^>]*aria-live="polite"/);
  assert.match(index, /Repeating with unchanged evidence records a truthful no-change snapshot without calling the provider/);
  assert.match(index, /id="living-topic-description"[^>]*maxlength="1200"/);
  assert.match(index, /Manual add\/remove actions become positive or negative examples/);
});

test("Living Topics UI exposes bounded membership and explicit mutations", () => {
  assert.match(app, /async function addLivingTopicMember\(memoryItemId\)/);
  assert.match(app, /async function removeLivingTopicMember\(memoryItemId\)/);
  assert.match(app, /async function createLivingTopicSnapshot\(event\)/);
  assert.match(app, /Snapshot could not be created:/);
  assert.match(app, /Snapshot \$\{snapshot\.version\} created\./);
  assert.match(app, /card\.dataset\.snapshotId = snapshot\.id/);
  assert.match(app, /detail\.members\.length >= 20/);
  assert.match(app, /membership\.origin === "automatic"/);
  assert.match(styles, /\.living-topics-layout \{[^}]*grid-template-columns/);
  assert.match(styles, /\.living-topic-snapshot-card \{/);
  assert.match(styles, /\.living-topics-layout \{ grid-template-columns: 1fr; \}/);
});

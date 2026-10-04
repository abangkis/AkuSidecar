import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const html = readFileSync(new URL("../internal/httpapi/web/index.html", import.meta.url), "utf8");
const app = readFileSync(new URL("../internal/httpapi/web/app.js", import.meta.url), "utf8");

test("Settings storage report separates retention status from database repair and cleanup actions", () => {
  assert.match(html, /<legend>Storage<\/legend>/);
  assert.match(html, /id="settings-storage-items"/);
  assert.match(html, /id="settings-storage-bytes"/);
  assert.match(html, /<details class="settings-storage-details">/);
  assert.match(html, /<legend>Fresh runtime boundary<\/legend>/);
  assert.match(app, /api\("\/api\/timeline\/storage"\)/);
  assert.match(app, /Preview only; no Timeline cards are removed/);
  assert.match(app, /Eligible cards are trimmed during retention maintenance/);
  assert.match(app, /usage\.active \? "Active retention" : "Preview only"/);
  assert.doesNotMatch(html, /id="settings-storage-(?:delete|cleanup|apply)"/);
});

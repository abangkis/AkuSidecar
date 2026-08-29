import test from "node:test";
import assert from "node:assert/strict";

import {
  LIBRARY_MAX_QUERY,
  LIBRARY_TAB_CLEANING,
  LIBRARY_TAB_LIBRARY,
  LIBRARY_TAB_SAVED,
  LIBRARY_STORAGE_MAX_LIMIT,
  buildLibraryForgetPath,
  buildLibraryDonePath,
  buildLibraryKeepPath,
  buildLibraryReleasePath,
  buildLibraryRemovePath,
  buildLibraryRequestPath,
  buildSavedLibraryRequestPath,
  buildLibraryStorageRequestPath,
  formatLibraryStorageBytes,
  formatLibraryTier,
  libraryFilterKey,
  libraryForgetConfirmation,
  libraryDoneConfirmation,
  libraryKeepConfirmation,
  libraryReleaseConfirmation,
  libraryRemoveConfirmation,
  libraryHasFullContent,
  mergeLibraryPage,
  normalizeLibraryStorageReport,
  normalizeLibraryTab,
  normalizeLibraryFilters,
} from "../internal/httpapi/web/library-state.js";

test("Library query state is bounded and encoded without leaking cursor state", () => {
  const filters = normalizeLibraryFilters({
    query: `  ${"memory ".repeat(LIBRARY_MAX_QUERY)}  `,
    source: "x",
    tier: "full_copy",
    publishedFrom: "2026-08-01",
    publishedTo: "2026-08-31",
    limit: 999,
  });
  assert.equal(filters.query.length, LIBRARY_MAX_QUERY);
  assert.equal(filters.limit, 50);
  assert.equal(filters.tier, "full_copy");

  const path = buildLibraryRequestPath(filters, "cursor+/=");
  assert.match(path, /^\/api\/library\/items\?/);
  assert.match(path, /query=/);
  assert.match(path, /source=x/);
  assert.match(path, /tier=full_copy/);
  assert.match(path, /cursor=cursor%2B%2F%3D/);
  assert.equal(libraryFilterKey(filters), libraryFilterKey({ ...filters }));
});

test("Library storage requests stay bounded and preserve an empty recommendation list", () => {
  assert.equal(buildLibraryStorageRequestPath(), "/api/library/storage");
  assert.equal(buildLibraryStorageRequestPath(12), "/api/library/storage?limit=12");
  assert.equal(buildLibraryStorageRequestPath(999), `/api/library/storage?limit=${LIBRARY_STORAGE_MAX_LIMIT}`);
  const report = normalizeLibraryStorageReport({ usage: { logicalBytes: 2048 }, recommendations: null });
  assert.deepEqual(report, { usage: { logicalBytes: 2048 }, recommendations: [] });
  assert.equal(formatLibraryStorageBytes(0), "0 B");
  assert.equal(formatLibraryStorageBytes(1024), "1.00 KB");
  assert.equal(formatLibraryStorageBytes(-1), "Unknown");
});

test("Library opens Saved first and keeps Spring Cleaning separate", () => {
  assert.equal(normalizeLibraryTab(), LIBRARY_TAB_SAVED);
  assert.equal(normalizeLibraryTab("unknown"), LIBRARY_TAB_SAVED);
  assert.equal(normalizeLibraryTab(LIBRARY_TAB_LIBRARY), LIBRARY_TAB_LIBRARY);
  assert.equal(normalizeLibraryTab(LIBRARY_TAB_CLEANING), LIBRARY_TAB_CLEANING);
});

test("Saved Library requests and actions expose bounded current membership", () => {
  const path = buildSavedLibraryRequestPath({ query: "bounded item", limit: 8 }, "cursor+/=");
  assert.match(path, /^\/api\/library\/items\?/);
  assert.match(path, /saved=true/);
  assert.match(path, /cursor=cursor%2B%2F%3D/);
  assert.equal(buildLibraryKeepPath("memory/one"), "/api/library/items/memory%2Fone/keep-in-library");
  assert.equal(buildLibraryDonePath("memory/one"), "/api/library/items/memory%2Fone/done");
  assert.equal(buildLibraryKeepPath(""), "");
  assert.equal(buildLibraryDonePath(""), "");
  const keepConfirmation = libraryKeepConfirmation({ title: "A bounded update" });
  assert.match(keepConfirmation, /A bounded update/);
  assert.match(keepConfirmation, /keeps the locally available full copy/);
  const doneConfirmation = libraryDoneConfirmation({ title: "A bounded update" });
  assert.match(doneConfirmation, /A bounded update/);
  assert.match(doneConfirmation, /leave Saved/);
});

test("Library page merging is append-idempotent and cursor-driven", () => {
  const first = mergeLibraryPage([], {
    items: [{ id: "one" }, { id: "two" }],
    nextCursor: "next",
  });
  assert.deepEqual(first.items.map((item) => item.id), ["one", "two"]);
  assert.equal(first.nextCursor, "next");

  const second = mergeLibraryPage(first.items, {
    items: [{ id: "two" }, { id: "three" }],
    nextCursor: "",
  }, true);
  assert.deepEqual(second.items.map((item) => item.id), ["one", "two", "three"]);
  assert.equal(second.nextCursor, "");
});

test("Library tier and detail helpers preserve the read-only payload boundary", () => {
  assert.equal(formatLibraryTier("recall"), "Recall stub");
  assert.equal(formatLibraryTier("full_copy"), "Full copy kept");
  assert.equal(libraryHasFullContent({ fullContent: "kept locally" }), true);
  assert.equal(libraryHasFullContent({ fullContent: "" }), false);
  assert.equal(libraryHasFullContent({}), false);
});

test("Library Remove and Forget use distinct narrow actions and confirmations", () => {
  assert.equal(buildLibraryRemovePath("memory/one"), "/api/library/items/memory%2Fone");
  assert.equal(buildLibraryForgetPath("memory/one"), "/api/library/items/memory%2Fone/forget-permanently");
  assert.equal(buildLibraryReleasePath("memory/one"), "/api/library/items/memory%2Fone/release-full-copy");
  assert.equal(buildLibraryRemovePath(""), "");
  assert.equal(buildLibraryForgetPath(""), "");
  assert.equal(buildLibraryReleasePath(""), "");
  const removeConfirmation = libraryRemoveConfirmation({ title: "A private marker" });
  assert.match(removeConfirmation, /A private marker/);
  assert.match(removeConfirmation, /local Library copy/);
  assert.match(removeConfirmation, /later More may add/);
  assert.doesNotMatch(removeConfirmation, /blocks automatic recapture/);
  const forgetConfirmation = libraryForgetConfirmation({ title: "A private marker" });
  assert.match(forgetConfirmation, /A private marker/);
  assert.match(forgetConfirmation, /permanently/);
  assert.match(forgetConfirmation, /blocks automatic recapture/);
  assert.match(forgetConfirmation, /cannot be undone/);
  assert.notEqual(removeConfirmation, forgetConfirmation);
  const releaseConfirmation = libraryReleaseConfirmation({ title: "A private marker" });
  assert.match(releaseConfirmation, /stored text/);
  assert.match(releaseConfirmation, /recall metadata/);
  assert.notEqual(releaseConfirmation, removeConfirmation);
});

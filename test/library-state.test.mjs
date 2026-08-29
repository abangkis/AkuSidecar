import test from "node:test";
import assert from "node:assert/strict";

import {
  LIBRARY_MAX_QUERY,
  buildLibraryRequestPath,
  formatLibraryTier,
  libraryFilterKey,
  libraryHasFullContent,
  mergeLibraryPage,
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

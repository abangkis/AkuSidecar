import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";
import {
  POST_FRESHNESS_CATEGORIES,
  classifyPostFreshness,
  compactPostAge,
  postHeaderContext,
  postFreshnessReferenceAt,
} from "../internal/httpapi/web/post-freshness.js";

const minute = 60 * 1000;
const hour = 60 * minute;
const day = 24 * hour;
const now = Date.parse("2026-08-17T12:00:00Z");

test("post freshness uses the agreed category boundaries", () => {
  assert.equal(classifyPostFreshness({ referenceAt: now - 9 * minute, now }).key, "current");
  assert.equal(classifyPostFreshness({ referenceAt: now - 10 * minute, now }).key, "fresh");
  assert.equal(classifyPostFreshness({ referenceAt: now - 59 * minute, now }).key, "fresh");
  assert.equal(classifyPostFreshness({ referenceAt: now - hour, now }).key, "recent");
  assert.equal(classifyPostFreshness({ referenceAt: now - 6 * hour, now }).key, "today");
  assert.equal(classifyPostFreshness({ referenceAt: now - day, now }).key, "today");
  assert.equal(classifyPostFreshness({ referenceAt: now - day - 1, now }).key, "older");
  assert.equal(classifyPostFreshness({ referenceAt: now - 3 * day + 1, now }).key, "older");
  assert.equal(classifyPostFreshness({ referenceAt: now - 3 * day, now }).key, "older_3d");
  assert.equal(classifyPostFreshness({ referenceAt: now - 6 * day + 1, now }).key, "older_3d");
  assert.equal(classifyPostFreshness({ referenceAt: now - 6 * day, now }).key, "older_6d");
  assert.deepEqual(POST_FRESHNESS_CATEGORIES.slice(-4), ["older", "older_3d", "older_6d", "unknown"]);
});

test("absolute publication time takes priority over relative source text", () => {
  const result = classifyPostFreshness({
    publishedAt: "2026-08-17T11:55:00Z",
    timestampText: "2d",
    now,
  });
  assert.equal(result.key, "current");
  assert.equal(result.referenceAt, Date.parse("2026-08-17T11:55:00Z"));
});

test("relative adapter timestamps support compact and verbose forms", () => {
  assert.equal(classifyPostFreshness({ timestampText: "@handle · 8m", now }).key, "current");
  assert.equal(classifyPostFreshness({ timestampText: "45 minutes ago", now }).key, "fresh");
  assert.equal(classifyPostFreshness({ timestampText: "3h • Edited", now }).key, "recent");
  assert.equal(classifyPostFreshness({ timestampText: "15 hours", now }).key, "today");
  assert.equal(classifyPostFreshness({ timestampText: "2d", now }).key, "older");
  assert.equal(classifyPostFreshness({ timestampText: "4d", now }).key, "older_3d");
  assert.equal(classifyPostFreshness({ timestampText: "7d", now }).key, "older_6d");
});

test("missing or implausibly future timestamps remain unknown", () => {
  assert.equal(classifyPostFreshness({ now }).key, "unknown");
  assert.equal(classifyPostFreshness({ publishedAt: "not-a-date", timestampText: "", now }).key, "unknown");
  assert.equal(postFreshnessReferenceAt({ publishedAt: "2026-08-17T12:06:00Z", now }), null);
});

test("small source clock skew is treated as current", () => {
  assert.equal(classifyPostFreshness({ publishedAt: "2026-08-17T12:04:00Z", now }).key, "current");
});

test("compact post ages use elapsed minutes, hours, days, weeks, months and years", () => {
  for (const [age, expected] of [[0, "now"], [30 * minute, "30m"], [4 * hour, "4h"], [2 * day, "2d"], [7 * day, "1w"], [60 * day, "2mo"], [365 * day, "1y"]]) {
    assert.equal(compactPostAge({ publishedAt: new Date(now - age).toISOString(), now }), expected);
  }
  assert.equal(compactPostAge({ now }), "");
  assert.equal(compactPostAge({ publishedAt: "invalid", timestampText: "30 minutes ago", now }), "30m");
  assert.equal(compactPostAge({ publishedAt: "invalid", timestampText: "6 months ago", now }), "6mo");
  assert.equal(compactPostAge({ publishedAt: new Date(now + hour).toISOString(), now }), "");
});

test("top-level platform headers share compact age while retaining identity context", () => {
  const examples = [
    { platform: "instagram", value: { publishedAt: new Date(now - 30 * minute).toISOString() }, expected: "30m" },
    { platform: "x", value: { secondary: "@example · 4h" }, expected: "@example · 4h" },
    { platform: "linkedin", value: { connectionDegree: "2nd", timestampText: "4 hours ago • Edited" }, expected: "2nd · 4h · Edited" },
    { platform: "facebook", value: { timestampText: "1 week ago" }, expected: "1w" },
    { platform: "facebook", value: { publishedAt: new Date(now - 2 * day).toISOString(), timestampText: "2h" }, expected: "2d" },
    { platform: "facebook", value: { publishedAt: new Date(now - 4 * hour).toISOString(), timestampText: "August 17 at 8:00 AM" }, expected: "4h" },
    { platform: "instagram", value: { publishedAt: new Date(now - 30 * minute).toISOString(), timestampText: "August 17, 2026" }, expected: "30m" },
  ];
  for (const { platform, value, expected } of examples) {
    assert.equal(postHeaderContext({ ...value, now }), expected, platform);
  }
});

test("unavailable header age retains source text without fabricating a time", () => {
  assert.equal(postHeaderContext({ timestampText: "Sponsored", now }), "Sponsored");
  assert.equal(postHeaderContext({ secondary: "@example", now }), "@example");
  assert.equal(postHeaderContext({ now }), "");
});

test("minute refresh advances mounted header ages and retains metadata", () => {
  const app = readFileSync(new URL("../internal/httpapi/web/app.js", import.meta.url), "utf8");
  const refresh = app.slice(app.indexOf("function refreshVisiblePostFreshness()"), app.indexOf("window.setInterval(refreshVisiblePostFreshness"));
  const headers = new WeakMap();
  const fixtures = [
    { publishedAt: new Date(now - 59 * minute).toISOString() },
    { secondary: "@example · 59m" },
    { connectionDegree: "2nd", timestampText: "59m • Edited" },
    { publishedAt: new Date(now - 59 * minute).toISOString(), timestampText: "2h" },
  ];
  const cards = fixtures.map((value) => {
    const card = { dataset: { freshnessReferenceAt: String(postFreshnessReferenceAt({ ...value, timestampText: value.timestampText || value.secondary, now })) } };
    headers.set(card, { context: { textContent: postHeaderContext({ ...value, now }) }, value });
    return card;
  });
  vm.runInNewContext(`${refresh}\nrefreshVisiblePostFreshness();`, {
    Date: { now: () => now + minute },
    document: { querySelectorAll: () => cards },
    postHeaderContexts: headers,
    postHeaderContext,
    applyPostFreshness: () => {},
  });
  assert.deepEqual(cards.map((card) => headers.get(card).context.textContent), ["1h", "@example · 1h", "2nd · 1h · Edited", "1h"]);
});

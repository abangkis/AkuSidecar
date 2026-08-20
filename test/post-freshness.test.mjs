import test from "node:test";
import assert from "node:assert/strict";
import {
  classifyPostFreshness,
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
});

test("missing or implausibly future timestamps remain unknown", () => {
  assert.equal(classifyPostFreshness({ now }).key, "unknown");
  assert.equal(classifyPostFreshness({ publishedAt: "not-a-date", timestampText: "", now }).key, "unknown");
  assert.equal(postFreshnessReferenceAt({ publishedAt: "2026-08-17T12:06:00Z", now }), null);
});

test("small source clock skew is treated as current", () => {
  assert.equal(classifyPostFreshness({ publishedAt: "2026-08-17T12:04:00Z", now }).key, "current");
});

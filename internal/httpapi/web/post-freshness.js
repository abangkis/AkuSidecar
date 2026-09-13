const MINUTE_MS = 60 * 1000;
const HOUR_MS = 60 * MINUTE_MS;
const DAY_MS = 24 * HOUR_MS;
const MONTH_MS = 30 * DAY_MS;
const MAX_FUTURE_SKEW_MS = 5 * MINUTE_MS;

export const POST_FRESHNESS_CATEGORIES = Object.freeze([
  "current",
  "fresh",
  "recent",
  "today",
  "older",
  "older_3d",
  "older_6d",
  "unknown",
]);

function relativeAgeMilliseconds(value) {
  const text = String(value || "").trim().toLowerCase();
  if (!text) return null;
  if (/\b(?:now|just now|moments? ago)\b/.test(text)) return 0;

  const compact = text.match(/(?:^|[\s·•])([0-9]{1,4})\s*(mo|[smhdwy])(?:$|[\s·•])/i);
  const verbose = text.match(/\b([0-9]{1,4})\s*(seconds?|secs?|minutes?|mins?|hours?|hrs?|days?|weeks?|months?|years?)\b/i);
  const amount = Number(compact?.[1] || verbose?.[1]);
  if (!Number.isFinite(amount)) return null;

  const unit = (compact?.[2] || verbose?.[2] || "").toLowerCase();
  if (unit === "s" || unit.startsWith("sec")) return amount * 1000;
  if (unit === "m" || unit.startsWith("min")) return amount * MINUTE_MS;
  if (unit === "h" || unit.startsWith("hour") || unit.startsWith("hr")) return amount * HOUR_MS;
  if (unit === "d" || unit.startsWith("day")) return amount * DAY_MS;
  if (unit === "w" || unit.startsWith("week")) return amount * 7 * DAY_MS;
  if (unit === "mo" || unit.startsWith("month")) return amount * MONTH_MS;
  if (unit === "y" || unit.startsWith("year")) return amount * 365 * DAY_MS;
  return null;
}

export function postFreshnessReferenceAt({ publishedAt, timestampText, now = Date.now() } = {}) {
  const nowMs = Number(now);
  const absoluteMs = Date.parse(String(publishedAt || ""));
  if (Number.isFinite(absoluteMs)) {
    if (absoluteMs - nowMs > MAX_FUTURE_SKEW_MS) return null;
    return Math.min(absoluteMs, nowMs);
  }

  const relativeAge = relativeAgeMilliseconds(timestampText);
  return relativeAge === null ? null : nowMs - relativeAge;
}

export function compactPostAge({ publishedAt, timestampText, now = Date.now(), referenceAt = postFreshnessReferenceAt({ publishedAt, timestampText, now }) } = {}) {
  if (!Number.isFinite(referenceAt)) return "";
  const ageMs = Math.max(0, Number(now) - referenceAt);
  if (!Number.isFinite(ageMs)) return "";
  for (const [unit, duration] of [["y", 365 * DAY_MS], ["mo", MONTH_MS], ["w", 7 * DAY_MS], ["d", DAY_MS], ["h", HOUR_MS], ["m", MINUTE_MS]]) {
    if (ageMs >= duration) return `${Math.floor(ageMs / duration)}${unit}`;
  }
  return "now";
}

export function postHeaderContext({ publishedAt, timestampText, connectionDegree, secondary, referenceAt, now = Date.now() } = {}) {
  const age = compactPostAge({ publishedAt, timestampText: timestampText || secondary, referenceAt, now });
  if (!age) return [connectionDegree, timestampText].filter(Boolean).join(" · ") || secondary || "";
  const handle = String(secondary || "").match(/^@[A-Za-z0-9_]+/)?.[0];
  const edited = /\bEdited\b/i.test(String(timestampText || secondary || "")) ? "Edited" : "";
  return [connectionDegree, handle, age, edited].filter(Boolean).join(" · ");
}

export function classifyPostFreshness({ publishedAt, timestampText, referenceAt, now = Date.now() } = {}) {
  const nowMs = Number(now);
  const suppliedReference = Number(referenceAt);
  const referenceMs = Number.isFinite(suppliedReference)
    ? suppliedReference
    : postFreshnessReferenceAt({ publishedAt, timestampText, now: nowMs });
  if (!Number.isFinite(referenceMs)) return { key: "unknown", referenceAt: null };

  const ageMs = Math.max(0, nowMs - referenceMs);
  let key = "older";
  if (ageMs < 10 * MINUTE_MS) key = "current";
  else if (ageMs < HOUR_MS) key = "fresh";
  else if (ageMs < 6 * HOUR_MS) key = "recent";
  else if (ageMs <= DAY_MS) key = "today";
  else if (ageMs < 3 * DAY_MS) key = "older";
  else if (ageMs < 6 * DAY_MS) key = "older_3d";
  else key = "older_6d";
  return { key, referenceAt: referenceMs };
}

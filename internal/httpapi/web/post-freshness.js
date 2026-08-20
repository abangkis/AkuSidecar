const MINUTE_MS = 60 * 1000;
const HOUR_MS = 60 * MINUTE_MS;
const DAY_MS = 24 * HOUR_MS;
const MAX_FUTURE_SKEW_MS = 5 * MINUTE_MS;

export const POST_FRESHNESS_CATEGORIES = Object.freeze([
  "current",
  "fresh",
  "recent",
  "today",
  "older",
  "unknown",
]);

function relativeAgeMilliseconds(value) {
  const text = String(value || "").trim().toLowerCase();
  if (!text) return null;
  if (/\b(?:now|just now|moments? ago)\b/.test(text)) return 0;

  const compact = text.match(/(?:^|[\s·•])([0-9]{1,4})\s*([smhdwy])(?:$|[\s·•])/i);
  const verbose = text.match(/\b([0-9]{1,4})\s*(seconds?|secs?|minutes?|mins?|hours?|hrs?|days?|weeks?|years?)\b/i);
  const amount = Number(compact?.[1] || verbose?.[1]);
  if (!Number.isFinite(amount)) return null;

  const unit = (compact?.[2] || verbose?.[2] || "").toLowerCase();
  if (unit === "s" || unit.startsWith("sec")) return amount * 1000;
  if (unit === "m" || unit.startsWith("min")) return amount * MINUTE_MS;
  if (unit === "h" || unit.startsWith("hour") || unit.startsWith("hr")) return amount * HOUR_MS;
  if (unit === "d" || unit.startsWith("day")) return amount * DAY_MS;
  if (unit === "w" || unit.startsWith("week")) return amount * 7 * DAY_MS;
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
  return { key, referenceAt: referenceMs };
}

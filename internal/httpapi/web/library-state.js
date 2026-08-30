export const LIBRARY_DEFAULT_LIMIT = 24;
export const LIBRARY_MAX_LIMIT = 50;
export const LIBRARY_MAX_QUERY = 200;
export const LIBRARY_STORAGE_DEFAULT_LIMIT = 6;
export const LIBRARY_STORAGE_MAX_LIMIT = 12;
export const LIBRARY_TAB_SAVED = "saved";
export const LIBRARY_TAB_LIBRARY = "library";
export const LIBRARY_TAB_CLEANING = "cleaning";

const allowedTiers = new Set(["recall", "full_copy"]);

export function normalizeLibraryTab(value) {
  if (value === LIBRARY_TAB_CLEANING) return LIBRARY_TAB_CLEANING;
  if (value === LIBRARY_TAB_LIBRARY) return LIBRARY_TAB_LIBRARY;
  return LIBRARY_TAB_SAVED;
}

export function normalizeLibraryFilters(input = {}) {
  const query = Array.from(String(input.query ?? "").trim()).slice(0, LIBRARY_MAX_QUERY).join("");
  const source = String(input.source ?? "").trim();
  const tier = String(input.tier ?? "").trim();
  const publishedFrom = String(input.publishedFrom ?? "").trim();
  const publishedTo = String(input.publishedTo ?? "").trim();
  const parsedLimit = Number.parseInt(input.limit, 10);
  const limit = Number.isFinite(parsedLimit)
    ? Math.min(LIBRARY_MAX_LIMIT, Math.max(1, parsedLimit))
    : LIBRARY_DEFAULT_LIMIT;
  return {
    query,
    source,
    tier: allowedTiers.has(tier) ? tier : "",
    publishedFrom,
    publishedTo,
    limit,
  };
}

export function buildLibraryRequestPath(filters = {}, cursor = "", savedOnly = false, includeTopicKnowledge = false) {
  const normalized = normalizeLibraryFilters(filters);
  const params = new URLSearchParams();
  if (normalized.query) params.set("query", normalized.query);
  if (normalized.source) params.set("source", normalized.source);
  if (normalized.tier) params.set("tier", normalized.tier);
  if (savedOnly) params.set("saved", "true");
  if (includeTopicKnowledge && !savedOnly && normalized.query) params.set("includeTopicKnowledge", "true");
  if (normalized.publishedFrom) params.set("publishedFrom", normalized.publishedFrom);
  if (normalized.publishedTo) params.set("publishedTo", normalized.publishedTo);
  params.set("limit", String(normalized.limit));
  const nextCursor = String(cursor ?? "").trim();
  if (nextCursor) params.set("cursor", nextCursor);
  return `/api/library/items?${params.toString()}`;
}

export function buildSavedLibraryRequestPath(filters = {}, cursor = "") {
  return buildLibraryRequestPath(filters, cursor, true);
}

export function buildLibraryStorageRequestPath(limit = LIBRARY_STORAGE_DEFAULT_LIMIT) {
  const parsedLimit = Number.parseInt(limit, 10);
  const normalized = Number.isFinite(parsedLimit)
    ? Math.min(LIBRARY_STORAGE_MAX_LIMIT, Math.max(1, parsedLimit))
    : LIBRARY_STORAGE_DEFAULT_LIMIT;
  return normalized === LIBRARY_STORAGE_DEFAULT_LIMIT
    ? "/api/library/storage"
    : `/api/library/storage?limit=${normalized}`;
}

export function normalizeLibraryStorageReport(input = {}) {
  return {
    usage: input?.usage && typeof input.usage === "object" ? input.usage : null,
    recommendations: Array.isArray(input?.recommendations) ? input.recommendations : [],
    savedPressure: input?.savedPressure && typeof input.savedPressure === "object" ? input.savedPressure : null,
    savedRecommendations: Array.isArray(input?.savedRecommendations) ? input.savedRecommendations : [],
  };
}

export function formatLibraryStorageBytes(value) {
  if (value === null || value === undefined || String(value).trim() === "") return "Unknown";
  const bytes = Number(value);
  if (!Number.isFinite(bytes) || bytes < 0) return "Unknown";
  if (bytes < 1024) return `${Math.round(bytes)} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let amount = bytes;
  let unit = "B";
  for (const nextUnit of units) {
    amount /= 1024;
    unit = nextUnit;
    if (amount < 1024 || nextUnit === units[units.length - 1]) break;
  }
  const precision = amount >= 100 ? 0 : amount >= 10 ? 1 : 2;
  return `${amount.toFixed(precision)} ${unit}`;
}

export function buildLibraryRemovePath(id) {
  const normalized = String(id ?? "").trim();
  return normalized ? `/api/library/items/${encodeURIComponent(normalized)}` : "";
}

export function buildLibraryForgetPath(id) {
  const normalized = String(id ?? "").trim();
  return normalized ? `/api/library/items/${encodeURIComponent(normalized)}/forget-permanently` : "";
}

export function buildLibraryReleasePath(id) {
  const normalized = String(id ?? "").trim();
  return normalized ? `/api/library/items/${encodeURIComponent(normalized)}/release-full-copy` : "";
}

export function buildLibraryKeepPath(id) {
  const normalized = String(id ?? "").trim();
  return normalized ? `/api/library/items/${encodeURIComponent(normalized)}/keep-in-library` : "";
}

export function buildLibraryDonePath(id) {
  const normalized = String(id ?? "").trim();
  return normalized ? `/api/library/items/${encodeURIComponent(normalized)}/done` : "";
}

export function libraryKeepConfirmation(item = {}) {
  const title = String(item?.title ?? "").trim() || "this memory";
  return `Keep “${title}” in Library? This keeps the locally available full copy after it leaves Saved.`;
}

export function libraryDoneConfirmation(item = {}) {
  const title = String(item?.title ?? "").trim() || "this memory";
  return `Done reading “${title}”? It will leave Saved; any permanent Library copy stays available.`;
}

export function libraryRemoveConfirmation(item = {}) {
  const title = String(item?.title ?? "").trim() || "this memory";
  return `Remove “${title}” from Personal Memory on this device? This deletes the local Library copy. A later More may add it again.`;
}

export function libraryForgetConfirmation(item = {}) {
  const title = String(item?.title ?? "").trim() || "this memory";
  return `Forget “${title}” permanently? This deletes the local Library copy and blocks automatic recapture of the same source item. This cannot be undone without a reset.`;
}

export function libraryReleaseConfirmation(item = {}) {
  const title = String(item?.title ?? "").trim() || "this memory";
  return `Release the full copy for “${title}”? This permanently removes the stored text from this device and keeps only its recall metadata.`;
}

export function libraryFilterKey(filters = {}) {
  const normalized = normalizeLibraryFilters(filters);
  return JSON.stringify(normalized);
}

export function mergeLibraryPage(existingItems = [], page = {}, append = false) {
  const incoming = Array.isArray(page?.items) ? page.items : [];
  const merged = append ? [...existingItems] : [];
  const seen = new Set(merged.map((item) => String(item?.id ?? "")).filter(Boolean));
  for (const item of incoming) {
    const id = String(item?.id ?? "").trim();
    if (!id || seen.has(id)) continue;
    seen.add(id);
    merged.push(item);
  }
  return {
    items: merged,
    nextCursor: typeof page?.nextCursor === "string" ? page.nextCursor : "",
  };
}

export function formatLibraryTier(tier) {
  return tier === "full_copy" ? "Full copy kept" : "Recall stub";
}

export function libraryHasFullContent(item) {
  return typeof item?.fullContent === "string" && item.fullContent.length > 0;
}

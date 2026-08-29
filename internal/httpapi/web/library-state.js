export const LIBRARY_DEFAULT_LIMIT = 24;
export const LIBRARY_MAX_LIMIT = 50;
export const LIBRARY_MAX_QUERY = 200;

const allowedTiers = new Set(["recall", "full_copy"]);

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

export function buildLibraryRequestPath(filters = {}, cursor = "") {
  const normalized = normalizeLibraryFilters(filters);
  const params = new URLSearchParams();
  if (normalized.query) params.set("query", normalized.query);
  if (normalized.source) params.set("source", normalized.source);
  if (normalized.tier) params.set("tier", normalized.tier);
  if (normalized.publishedFrom) params.set("publishedFrom", normalized.publishedFrom);
  if (normalized.publishedTo) params.set("publishedTo", normalized.publishedTo);
  params.set("limit", String(normalized.limit));
  const nextCursor = String(cursor ?? "").trim();
  if (nextCursor) params.set("cursor", nextCursor);
  return `/api/library/items?${params.toString()}`;
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

export const SOURCE_CATALOG = Object.freeze([
  Object.freeze({
    id: "x",
    label: "X",
    behavior: "stream",
    accessMode: "authenticated_browser",
    collectionPolicy: "user_triggered",
    canonicalUrl: "https://x.com/home",
  }),
  Object.freeze({
    id: "linkedin",
    label: "LinkedIn",
    behavior: "stream",
    accessMode: "authenticated_browser",
    collectionPolicy: "user_triggered",
    canonicalUrl: "https://www.linkedin.com/feed/",
  }),
]);

export const SOURCE_REGISTRY = buildSourceRegistry(["x", "linkedin"]);

export function buildSourceRegistry(activeSources = []) {
  const active = new Set(activeSources);
  return SOURCE_CATALOG.map((source) => Object.freeze({
    ...source,
    activationState: active.has(source.id) ? "active" : "inactive",
  }));
}

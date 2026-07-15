export const DEFAULT_BOUNDED_LOAD_PROFILE = "expanded_2x";

export const BOUNDED_LOAD_PROFILES = Object.freeze({
  standard_1x: Object.freeze({
    id: "standard_1x",
    scale: 1,
    maxItemsPerSource: 5,
    maxItemsTotal: 10,
    maxScrolls: 2,
    timelineCapacity: 12,
    qualityRetrySettleMs: 300,
  }),
  expanded_2x: Object.freeze({
    id: "expanded_2x",
    scale: 2,
    maxItemsPerSource: 10,
    maxItemsTotal: 20,
    maxScrolls: 4,
    timelineCapacity: 24,
    qualityRetrySettleMs: 1_000,
  }),
  stress_3x: Object.freeze({
    id: "stress_3x",
    scale: 3,
    maxItemsPerSource: 15,
    maxItemsTotal: 30,
    maxScrolls: 6,
    timelineCapacity: 36,
    qualityRetrySettleMs: 1_000,
  }),
});

export function getBoundedLoadProfile(id = DEFAULT_BOUNDED_LOAD_PROFILE) {
  return BOUNDED_LOAD_PROFILES[id] ?? null;
}

export function applyBoundedLoadProfile(config, id) {
  if (id === "custom") {
    config.limits.boundedLoadProfile = "custom";
    config.limits.boundedLoadScale = null;
    return true;
  }
  const profile = getBoundedLoadProfile(id);
  if (!profile) return false;
  config.limits.boundedLoadProfile = profile.id;
  config.limits.boundedLoadScale = profile.scale;
  config.limits.maxItems = profile.maxItemsPerSource;
  config.limits.maxItemsTotal = profile.maxItemsTotal;
  config.limits.maxScrolls = profile.maxScrolls;
  config.limits.defaultScrolls = profile.maxScrolls;
  config.limits.qualityRetrySettleMs = profile.qualityRetrySettleMs;
  config.presentation.timelineCapacity = profile.timelineCapacity;
  return true;
}

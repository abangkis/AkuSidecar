const knownCount = (value) => Number.isSafeInteger(value) && value >= 0 ? value : null;

export function settingsStorageView(report) {
  const usage = report?.timelineStorage;
  if (!usage || typeof usage !== "object") return null;
  const totalItems = knownCount(usage.totalItems);
  const logicalBytes = knownCount(usage.logicalBytes);
  const maxItems = knownCount(usage.policy?.maxItems);
  const maxLogicalBytes = knownCount(usage.policy?.maxLogicalBytes);
  if (totalItems === null || logicalBytes === null || !maxItems || !maxLogicalBytes) return null;
  const over = usage.overItemLimit === true || usage.overByteLimit === true;
  const eligible = knownCount(usage.eligibleItems);
  const status = over
    ? eligible === 0 ? "Above preview limit · no eligible cards" : eligible === null ? "Above preview limit · eligibility unknown" : "Above preview limit · candidates available"
    : "Within preview limits";
  return {
    ...usage,
    totalItems,
    logicalBytes,
    maxItems,
    maxLogicalBytes,
    status,
    pressure: over,
    eligible,
    itemPercent: Math.min(100, totalItems / maxItems * 100),
    bytePercent: Math.min(100, logicalBytes / maxLogicalBytes * 100),
  };
}

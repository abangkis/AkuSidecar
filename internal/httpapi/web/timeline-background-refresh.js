// Compare the last successfully rendered presentation, not mutable network or
// runtime state. Array order matters; JSON object property order does not.
function canonicalJSON(value) {
  return JSON.stringify(value, (_key, entry) => {
    if (!entry || typeof entry !== "object" || Array.isArray(entry)) return entry;
    return Object.fromEntries(Object.keys(entry).sort().map(key => [key, entry[key]]));
  });
}

export function createTimelineBackgroundRefresh() {
  let renderedPresentation = null;
  return {
    capture({ items, latestCheck, timelineBatches, settings, routing, preparedBatchCount, highlightSessionID, controls }) {
      // Complete payloads are intentionally conservative: new detection details,
      // evidence, order, and batch/check metadata must never be hidden by a no-op.
      return canonicalJSON({ items, latestCheck, timelineBatches, settings, routing, preparedBatchCount, highlightSessionID, controls });
    },
    isUnchanged(presentation) {
      return renderedPresentation !== null && presentation === renderedPresentation;
    },
    commit(presentation) {
      renderedPresentation = presentation;
    },
    invalidate() {
      renderedPresentation = null;
    },
  };
}

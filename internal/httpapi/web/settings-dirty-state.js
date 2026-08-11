export function createDirtyStateTracker({ readSnapshot, serialize = JSON.stringify, onChange = () => {} }) {
  if (typeof readSnapshot !== "function") throw new TypeError("readSnapshot must be a function");

  let baselineKey = null;
  let currentKey = null;
  let dirty = false;

  function setDirty(next) {
    if (dirty === next) return;
    dirty = next;
    onChange(dirty);
  }

  function setBaseline(snapshot = readSnapshot()) {
    baselineKey = serialize(snapshot);
    currentKey = baselineKey;
    setDirty(false);
    return dirty;
  }

  function refresh(snapshot = readSnapshot()) {
    currentKey = serialize(snapshot);
    setDirty(baselineKey !== null && currentKey !== baselineKey);
    return dirty;
  }

  return Object.freeze({
    refresh,
    setBaseline,
    isDirty: () => dirty,
    getBaselineKey: () => baselineKey,
    getCurrentKey: () => currentKey,
  });
}

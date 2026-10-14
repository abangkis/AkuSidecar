// Only the bound development provider advertises this contract. Readiness is
// advisory here; the server still verifies authority and owns admission.
export function developmentReaderPriority(runtime, source) {
  return runtime?.developmentSession === true && runtime.available === true
    && runtime.state === "ready" && runtime.requested === "headless" && runtime.effective === "headless"
    && runtime.pending === false && !runtime.batchPaused && !runtime.nativeReaderOnly
    && !runtime.collectionBorrowSource && Number.isInteger(runtime.generation) && runtime.generation > 0
    && ["x", "instagram"].includes(source) && Array.isArray(runtime.supportedSources)
    && runtime.supportedSources.includes(source);
}

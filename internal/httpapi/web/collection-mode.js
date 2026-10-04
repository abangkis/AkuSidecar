export function collectionModeState(runtime, bridgeCompatible = false) {
  if (!runtime?.available) return { canCollect: bridgeCompatible, label: "Browser", detail: "Browser collection is active.", canSelectHeadless: false };
  const requested = runtime.requested || "browser";
  const effective = runtime.effective || "unavailable";
  const facebookCollection = runtime.collectionBorrowSource === "facebook";
  const facebookDetail = runtime.failure
    ? "Facebook Browser collection is unavailable."
    : effective === "browser" && runtime.state === "ready"
      ? "Facebook is collecting through Browser. It returns to your selected mode after collection finishes."
      : "Preparing Facebook Browser collection; waiting for active capture to finish.";
  return {
    canCollect: runtime.state === "ready" && !runtime.pending && (effective === "headless" || bridgeCompatible),
    canSelectHeadless: runtime.headlessAvailable === true,
    label: effective === "headless" ? "Headless" : effective === "browser" ? "Browser" : "Unavailable",
    detail: `Requested: ${requested}. Active: ${effective}.${facebookCollection ? ` ${facebookDetail}` : runtime.pending ? " Waiting for active capture or open source windows to finish." : ""}${runtime.failure ? ` ${runtime.failure}` : ""}`,
  };
}

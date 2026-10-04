export function nativePostWaitReason(runtime, session, opening = false) {
  if (opening) return "Membuka native post…";
  const hybrid = runtime?.requested === "headless" || runtime?.effective === "headless";
  if (!hybrid || runtime?.nativeReaderOnly) return "";
  const active = session && !["completed", "partial", "failed", "cancelled"].includes(session.status);
  if (active || runtime?.collectionBorrowSource || runtime?.activeLeases > 0) {
    return "Menunggu koleksi selesai";
  }
  return "";
}

export function syncNativePostAvailability(link, reason) {
  if ((link.dataset.akuNativeWait || "") === reason) return;
  if (reason) {
    link.dataset.akuNativeWait = reason;
    link.setAttribute("aria-disabled", "true");
    link.setAttribute("aria-description", reason);
  } else {
    delete link.dataset.akuNativeWait;
    link.removeAttribute("aria-disabled");
    link.removeAttribute("aria-description");
  }
}

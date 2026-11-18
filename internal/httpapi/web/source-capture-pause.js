import { renderOperationalBanner } from "./operational-banner.js";

export function renderSourceCapturePause({ panel, detail }, pause) {
  const category = pause?.category === "cloudflare" ? "Cloudflare browser verification. "
    : pause?.category === "x_human_verification" ? "X requests verification through its phone app. " : "";
  renderOperationalBanner({ panel, detail }, { visible: Boolean(pause), detail: category + (pause?.message || "") });
}

export async function confirmSourceCaptureRecovery(api, pause, confirmed) {
  if (!confirmed || !pause?.id) return null;
  return api("/api/capture/x/confirm-recovery", {
    method: "POST", body: { pauseId: pause.id, confirmed: true },
  });
}

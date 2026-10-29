import {createDailySourceControls, xDailyView, xDailyCaptureNotice} from "./x-daily-controls.js";

export function instagramTrialView(value, context, consent, busy, needsRefresh) {
  const view = xDailyView(value, context, consent, busy, needsRefresh);
  view.visible = view.visible && value?.authorizationPolicy === "profile";
  view.startEnabled = view.visible && view.startEnabled;
  view.stopEnabled = view.visible && view.stopEnabled;
  view.detailText = value?.state === "retry-wait" && value?.cleanup === "verified"
    ? "Experimental IG capture is waiting after a temporary failure. Cleanup is verified; a limited retry can run on the next update after a cooldown, with fresh account/session verification. Saved permission is unchanged."
    : value?.cleanup === "uncertain" || value?.cleanup === "pending"
    ? "IG cleanup is not verified. New IG batches are blocked; your reader is preserved."
    : ["ready", "capturing"].includes(value?.state)
      ? "Experimental IG permission is saved for this browser profile across restarts, up to 60 seconds per batch. Each batch verifies the account/session. Your reader stays open."
      : value?.rememberedPermission
        ? "Experimental IG permission is saved for this browser profile. IG is unavailable in this process; saved permission does not replace account/session verification. Stop IG removes the saved choice."
        : "Experimental: this IG recipe is not yet qualified. Enable IG to save permission for this browser profile across restarts. Failures stop IG; other sources can continue.";
  return view;
}

export function createInstagramTrialControls(options) {
  return createDailySourceControls({...options,
    sourceLabel: "IG", statusField: "instagramCapture",
    startPath: "/api/development/instagram-trial/start", stopPath: "/api/development/instagram-trial/stop",
    startConsent: {acknowledgeLimitations: true, allowProcessSessionDerivation: true, acknowledgeUnqualifiedRecipe: true},
    deriveView: instagramTrialView,
    deriveNotice: (value, context, sources, stale) => {
      const notice = xDailyCaptureNotice(value, context, sources?.includes("instagram") ? ["x"] : [], stale);
      return {...notice, title: notice.title.replace(/\bX\b/g, "IG"), detail: notice.detail.replace(/\bX\b/g, "IG")};
    },
  });
}

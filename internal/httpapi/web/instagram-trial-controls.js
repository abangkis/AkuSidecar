import {createDailySourceControls, xDailyView, xDailyCaptureNotice} from "./x-daily-controls.js";

export function instagramTrialView(value, context, consent, busy, needsRefresh) {
  const view = xDailyView(value, context, consent, busy, needsRefresh);
  view.visible = view.visible && value?.authorizationPolicy === "process";
  view.startEnabled = view.visible && view.startEnabled;
  view.stopEnabled = view.visible && view.stopEnabled;
  view.detailText = value?.cleanup === "uncertain" || value?.cleanup === "pending"
    ? "IG cleanup is not verified. New IG batches are blocked; your reader is preserved."
    : ["ready", "capturing"].includes(value?.state)
      ? "Experimental IG capture is enabled for this process, up to 60 seconds per batch. Each batch verifies the account/session. Your reader stays open."
      : "Experimental: this IG recipe is not yet qualified. Explicit permission applies to this process only. Failures stop IG; other sources can continue.";
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

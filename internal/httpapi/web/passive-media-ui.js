import { browserCollectorProbeAllowed } from "./collection-mode.js";

export function createPassiveMediaUi({
  PASSIVE_MEDIA_LOOKUP_COOLDOWN_MS, PASSIVE_MEDIA_LOOKUP_TIMEOUT_MS, bridgeApi, endpoint, queueBackgroundTimelineRefresh, sourceDescriptor, state
}) {
  function schedulePassiveMediaEnrichment(items = state.timelineItems) {
    if (
      state.passiveMediaEnrichmentActive ||
      state.passiveMediaEnrichmentTimer ||
      state.session ||
      !browserCollectorProbeAllowed(state.bootstrap?.collectionRuntime) ||
      !state.bootstrap?.bridge?.compatible ||
      document.visibilityState === "hidden"
    ) return;
    if (!passiveMediaCandidates(items).length) return;
    state.passiveMediaEnrichmentTimer = window.setTimeout(() => {
      state.passiveMediaEnrichmentTimer = null;
      void enrichPassiveXMedia(items);
    }, 0);
  }

  function passiveMediaCandidates(items) {
    const now = Date.now();
    return (Array.isArray(items) ? items : []).filter((entry) => {
      if (sourceDescriptor(entry?.source)?.passiveMediaCapability !== "x_response" || !xMediaCandidateId(entry)) return false;
      if (Array.isArray(entry.evidence?.media) && entry.evidence.media.length > 0) return false;
      if (entry.evidence?.mediaRecovery?.outcome !== "unavailable") return false;
      const attemptedAt = state.passiveMediaEvidenceAttempts.get(entry.id) ?? 0;
      return now - attemptedAt >= PASSIVE_MEDIA_LOOKUP_COOLDOWN_MS;
    }).slice(0, 24);
  }

  async function enrichPassiveXMedia(items) {
    const candidates = passiveMediaCandidates(items);
    if (!candidates.length || state.passiveMediaEnrichmentActive) return;
    state.passiveMediaEnrichmentActive = true;
    const attemptedAt = Date.now();
    for (const entry of candidates) state.passiveMediaEvidenceAttempts.set(entry.id, attemptedAt);
    try {
      const candidateIds = [...new Set(candidates.map(xMediaCandidateId).filter(Boolean))];
      const evidence = await lookupPassiveXMediaEvidence(candidateIds);
      const byCandidate = new Map((evidence?.candidates ?? []).map((entry) => [entry.candidateId, entry.media]));
      let updated = false;
      for (const entry of candidates) {
        const candidateId = xMediaCandidateId(entry);
        const media = byCandidate.get(candidateId);
        if (!Array.isArray(media) || media.length === 0) continue;
        try {
          const result = await bridgeApi(
            `/api/bridge/timeline/${encodeURIComponent(entry.id)}/media-evidence`,
            {
              method: "POST",
              body: { candidateId, media, provenance: "passive_x_cache" },
            },
          );
          updated ||= result?.updated === true;
        } catch (error) {
          console.debug("Passive X media enrichment was not applied", error);
        }
      }
      if (updated) queueBackgroundTimelineRefresh();
    } catch (error) {
      console.debug("Passive X media evidence is not available yet", error);
    } finally {
      state.passiveMediaEnrichmentActive = false;
    }
  }

  function lookupPassiveXMediaEvidence(candidateIds) {
    const requestId = `x_media_${Date.now()}_${Math.random().toString(16).slice(2)}`;
    return new Promise((resolve, reject) => {
      const timeout = window.setTimeout(() => finish(
        reject,
        new Error("AkuBridge X media evidence lookup timed out."),
      ), PASSIVE_MEDIA_LOOKUP_TIMEOUT_MS);
      function finish(callback, value) {
        window.clearTimeout(timeout);
        window.removeEventListener("message", onResult);
        callback(value);
      }
      function onResult(event) {
        if (event.source !== window || event.origin !== endpoint || event.data?.requestId !== requestId) return;
        if (event.data.type === "AKU_BROWSER_X_MEDIA_EVIDENCE_RESULT") {
          finish(resolve, event.data.evidence ?? { candidates: [] });
        } else if (event.data.type === "AKU_BROWSER_X_MEDIA_EVIDENCE_FAILED") {
          finish(reject, new Error(event.data.message || "AkuBridge X media evidence lookup failed."));
        }
      }
      window.addEventListener("message", onResult);
      window.postMessage({
        type: "AKU_BROWSER_X_MEDIA_EVIDENCE_LOOKUP",
        requestId,
        candidateIds,
      }, endpoint);
    });
  }

  function xMediaCandidateId(entry) {
    if (sourceDescriptor(entry?.source)?.passiveMediaCapability !== "x_response") return null;
    const values = [
      entry.evidence?.platformId,
      entry.evidence?.permalink,
      entry.item?.sourceUrl,
      entry.evidenceKey,
    ];
    for (const value of values) {
      if (typeof value !== "string") continue;
      const direct = value.trim().match(/^(?:x:status:)?(\d{5,30})$/i);
      if (direct) return `x:status:${direct[1]}`;
      const status = value.match(/\/status\/(\d{5,30})(?:\b|\/|\?|#|$)/i);
      if (status) return `x:status:${status[1]}`;
    }
    return null;
  }

  return {
    schedulePassiveMediaEnrichment, passiveMediaCandidates, enrichPassiveXMedia, lookupPassiveXMediaEvidence, xMediaCandidateId
  };
}

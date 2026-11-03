import { buildTimelineReadLaterPath, timelineReadLaterConfirmation } from "./timeline-memory-state.js";
import { collectionModeState } from "./collection-mode.js";
import { mediaRecaptureTransport, waitForMediaRecapture } from "./media-recapture-transport.js";
import { videoPlaybackMissing } from "./video-recapture-policy.js";

export function createTimelineActions({
  $, api, clearNotice, configureNativePostLink, createNoticeDismissButton, endpoint, humanize, invalidateLibraryStorage, originalPostUrl, refreshTimeline, safePlaybackUrl, safeSourceUrl, setNoticeText, showError, sourceDescriptor, state, syncRunButtons, getTimelineFeedbackUI
}) {
  function buildActions(entry) {
    const actions = document.createElement("div");
    actions.className = "result-actions";
    const link = buildSourceLink(entry);
    const primary = document.createElement("div");
    primary.className = "timeline-primary-actions";
    if (link) primary.append(link);
    primary.append(buildTimelineReadLaterAction(entry));
    const feedback = getTimelineFeedbackUI().build(entry);
    actions.append(primary, feedback);
    if (entry.semanticEvent) actions.append(buildSemanticCorrectionActions(entry));
    return actions;
  }

  function buildTimelineReadLaterAction(entry) {
    const container = document.createElement("div");
    container.className = "timeline-memory-actions";
    const save = document.createElement("button");
    save.type = "button";
    save.className = "feedback-button memory-read-later-button";
    save.setAttribute("aria-label", "Read later");
    const status = document.createElement("small");
    status.className = "timeline-memory-action-status";
    status.setAttribute("role", "status");
    status.setAttribute("aria-live", "polite");
    let statusTimer = null;
    const hasText = Boolean(String(entry?.evidence?.text ?? "").trim());
    const render = () => {
      const saved = entry?.personalMemory?.saved === true;
      const busy = state.timelineReadLaterInFlight.has(entry.id);
      save.textContent = saved ? "Saved" : busy ? "Adding…" : "Read later";
      save.disabled = saved || busy;
      save.classList.toggle("selected", saved);
      save.classList.toggle("is-saved", saved);
      save.setAttribute("aria-pressed", String(saved));
      save.setAttribute("aria-label", saved ? "Saved" : "Read later");
      save.title = saved
        ? "This Timeline item is already Saved for later."
        : hasText
          ? "Keep the best locally available text in Saved."
          : "Keep a source reference in Saved; the source text is unavailable locally.";
    };
    render();
    save.addEventListener("click", async () => {
      if (!entry?.id || state.timelineReadLaterInFlight.has(entry.id) || entry?.personalMemory?.saved === true) return;
      if (!window.confirm(timelineReadLaterConfirmation(entry))) return;
      state.timelineReadLaterInFlight.add(entry.id);
      status.textContent = "";
      render();
      try {
        const response = await api(buildTimelineReadLaterPath(entry.id), { method: "POST" });
        if (response?.saved !== true) {
          throw new Error("AkuSidecar did not confirm the Saved action.");
        }
        entry.personalMemory = {
          retentionTier: response.retentionTier,
          saved: true,
          permanentKeep: response.permanentKeep === true,
        };
        invalidateLibraryStorage();
        status.textContent = response.alreadySaved === true ? "Already Saved for later." : "Saved for later.";
        window.clearTimeout(statusTimer);
        statusTimer = window.setTimeout(() => {
          status.textContent = "";
        }, 3500);
      } catch (error) {
        window.clearTimeout(statusTimer);
        status.textContent = "Could not add this item to Saved. Try again.";
        showError(error);
      } finally {
        state.timelineReadLaterInFlight.delete(entry.id);
        render();
      }
    });
    container.append(save, status);
    return container;
  }

  function buildSemanticCorrectionActions(entry) {
    const controls = document.createElement("div");
    controls.className = "semantic-correction-actions";
    const context = document.createElement("span");
    context.textContent = entry.semanticEvent.corrected
      ? "Event relationship corrected by you."
      : `${humanize(entry.semanticEvent.relation)} · ${Math.round((entry.semanticEvent.confidence || 0) * 100)}% confidence`;
    const button = document.createElement("button");
    button.type = "button";
    button.className = "event-correction-button";
    button.textContent = entry.semanticEvent.corrected
      ? "Undo correction"
      : "Review event…";
    button.disabled = entry.semanticEvent.corrected && !entry.semanticEvent.correctionId;
    button.addEventListener("click", async () => {
      button.disabled = true;
      try {
        if (entry.semanticEvent.corrected) {
          await api(`/api/event-corrections/${encodeURIComponent(entry.semanticEvent.correctionId)}/undo`, { method: "POST" });
          await refreshTimeline();
          clearNotice();
        } else {
          await showEventSuggestions(entry, controls, button);
        }
      } catch (error) {
        showError(error);
      } finally {
        button.disabled = false;
      }
    });
    controls.append(context, button);
    return controls;
  }

  async function showEventSuggestions(entry, controls, trigger) {
    const { suggestions = [] } = await api(`/api/timeline/${encodeURIComponent(entry.id)}/event-suggestions?limit=3`);
    controls.querySelector(".event-suggestion-editor")?.remove();
    const editor = document.createElement("span");
    editor.className = "event-suggestion-editor";
    const select = document.createElement("select");
    select.setAttribute("aria-label", "Choose the same semantic event");
    const available = [{
      eventId: entry.semanticEvent.eventId,
      canonicalClaim: entry.semanticEvent.canonicalClaim || entry.item?.whatChanged || "Current event",
      reportCount: entry.semanticEvent.reportCount || 1,
    }, ...suggestions.filter((suggestion) => suggestion.eventId !== entry.semanticEvent.eventId)];
    for (const suggestion of available) {
      const option = document.createElement("option");
      option.value = suggestion.eventId;
      option.textContent = `${suggestion.canonicalClaim} (${suggestion.reportCount} reports)`;
      select.append(option);
    }
    const novelty = document.createElement("select");
    novelty.setAttribute("aria-label", "Choose whether this report adds new information");
    novelty.append(
      new Option("Same event · repeated report", "duplicate_report"),
      new Option("Same event · meaningful update", "material_update"),
    );
    novelty.value = entry.semanticEvent.relation === "duplicate_report" ? "duplicate_report" : "material_update";
    const confirm = document.createElement("button");
    confirm.type = "button";
    confirm.textContent = "Apply";
    confirm.addEventListener("click", async () => {
      confirm.disabled = true;
      trigger.disabled = true;
      try {
        await applySemanticCorrection(entry.id, "same_event", select.value, novelty.value);
      } catch (error) {
        showError(error);
        confirm.disabled = false;
        trigger.disabled = false;
      }
    });
    const cancel = document.createElement("button");
    cancel.type = "button";
    cancel.textContent = "Cancel";
    cancel.addEventListener("click", () => editor.remove());
    const split = document.createElement("button");
    split.type = "button";
    split.textContent = "Not the same event";
    split.addEventListener("click", async () => {
      split.disabled = true;
      trigger.disabled = true;
      try {
        await applySemanticCorrection(entry.id, "not_same_event", "", "");
      } catch (error) {
        showError(error);
        split.disabled = false;
        trigger.disabled = false;
      }
    });
    editor.append(select, novelty, confirm, split, cancel);
    controls.append(editor);
  }

  async function applySemanticCorrection(timelineId, action, targetEventId, relation = "") {
    const { correction } = await api(`/api/timeline/${encodeURIComponent(timelineId)}/event-correction`, {
      method: "POST",
      body: { action, targetEventId, relation },
    });
    await refreshTimeline();
    showCorrectionNotice(correction);
  }

  function showCorrectionNotice(correction) {
    const notice = $("#provider-notice");
    notice.className = "notice notice-complete correction-notice";
    notice.setAttribute("role", "status");
    notice.replaceChildren();
    const message = document.createElement("span");
    message.textContent = correction.action === "not_same_event"
      ? "This report now starts a separate event."
      : correction.toRelation === "material_update"
        ? "Same event, meaningful update applied."
        : "Same event, repeated report applied.";
    const undo = document.createElement("button");
    undo.type = "button";
    undo.textContent = "Undo";
    undo.addEventListener("click", async () => {
      undo.disabled = true;
      try {
        await api(`/api/event-corrections/${encodeURIComponent(correction.id)}/undo`, { method: "POST" });
        await refreshTimeline();
        clearNotice();
      } catch (error) {
        showError(error);
      }
    });
    notice.append(message, undo, createNoticeDismissButton());
  }

  function buildSourceLink(entry) {
    const href = originalPostUrl(entry);
    if (!href) return null;
    const link = document.createElement("a");
    link.className = "source-link";
    configureNativePostLink(link, href, entry.source || entry.item?.source);
    link.textContent = entry.item?.sourceUrlKind === "native_post" ? "Open native post" : "Open source evidence";
    return link;
  }

  function mediaRecaptureReasonForEntry(entry) {
    const source = entry?.source || entry?.item?.source;
    if (sourceDescriptor(source)?.playbackRecoveryCapability === "native_post_recapture"
      && safeSourceUrl(entry?.item?.sourceUrl || entry?.evidence?.permalink, source)
      && videoPlaybackMissing(entry?.evidence, source, safePlaybackUrl)) return "unresolved_video";
    return "missing_media";
  }

  function buildMediaRecaptureButton(entry, reason = mediaRecaptureReasonForEntry(entry)) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "recapture-button";
    button.textContent = reason === "unresolved_video" ? "Recapture video" : "Recapture";
    button.disabled = Boolean(state.session) || state.mediaRecaptureActive || !collectionModeState(state.bootstrap?.collectionRuntime,state.bootstrap?.bridge?.compatible).canCollect;
    button.addEventListener("click", () => recaptureMedia(entry, button, "background", reason));
    return button;
  }

  function buildForegroundRecaptureOffer(entry) {
    const offer = document.createElement("div");
    offer.className = "foreground-recapture-offer";
    offer.setAttribute("aria-label", "Foreground media capture option");
    const copy = document.createElement("span");
    copy.textContent = "Still unavailable after a quiet recapture. Try a brief foreground capture? AkuBrowser will return here when it finishes.";
    const actions = document.createElement("span");
    actions.className = "foreground-recapture-actions";
    const accept = document.createElement("button");
    accept.type = "button";
    accept.className = "recapture-button foreground-recapture-button";
    accept.textContent = "Try in foreground";
    accept.disabled = Boolean(state.session) || state.mediaRecaptureActive || !state.bootstrap?.bridge?.compatible;
    accept.addEventListener("click", () => recaptureMedia(
      entry,
      accept,
      "foreground",
      state.foregroundRecaptureOffers.get(entry.id) || "missing_media",
    ));
    const dismiss = document.createElement("button");
    dismiss.type = "button";
    dismiss.className = "foreground-recapture-dismiss";
    dismiss.textContent = "Not now";
    dismiss.addEventListener("click", () => {
      state.foregroundRecaptureOffers.delete(entry.id);
      offer.replaceWith(buildMediaRecaptureButton(entry));
    });
    actions.append(accept, dismiss);
    offer.append(copy, actions);
    return offer;
  }

  async function recaptureMedia(entry, button, captureMode, reason = "missing_media") {
    if(captureMode==='foreground' && state.bootstrap?.collectionRuntime?.effective==='headless') return;
    if (state.session || state.mediaRecaptureActive || !collectionModeState(state.bootstrap?.collectionRuntime,state.bootstrap?.bridge?.compatible).canCollect) return;
    state.mediaRecaptureActive = true;
    if (button) {
      button.disabled = true;
      button.textContent = captureMode === "foreground" ? "Capturing in foreground..." : "Recapturing...";
    }
    syncRunButtons();
    clearNotice();
    try {
      const { recapture } = await api(`/api/timeline/${encodeURIComponent(entry.id)}/recapture`, {
        method: "POST",
        // The original missing-media request used body: { captureMode }.
        // A typed reason now reuses the same bounded recapture transport.
        body: { captureMode, reason },
      });
      const transport=mediaRecaptureTransport(recapture);
      const completed = transport==='sidecar' ? await waitForMediaRecapture(recapture.id,api) : await dispatchMediaRecapture(recapture.id);
      const bridgeCollector = transport === 'bridge' || recapture?.payload?.captureCollector?.backend === 'bridge';
      if (captureMode === "background" && completed?.outcome !== "recovered" && bridgeCollector) {
        state.foregroundRecaptureOffers.set(entry.id, reason);
        await refreshTimeline();
        return;
      }
      state.foregroundRecaptureOffers.delete(entry.id);
      await refreshTimeline();
      const notice = $("#provider-notice");
      notice.className = "notice notice-complete";
      notice.setAttribute("role", "status");
      setNoticeText(notice, completed?.outcome === "recovered"
        ? reason === "playback_error" || reason === "unresolved_video"
          ? "Playback refreshed from the native post. Select play again."
          : "Media recaptured from the native post."
        : transport==='sidecar' ? "Media is still unavailable after recapture." : "Media is still unavailable after the foreground capture.");
    } catch (error) {
      showError(error);
      if (button) {
        button.disabled = false;
        button.textContent = captureMode === "foreground" ? "Try in foreground" : "Recapture";
      }
    } finally {
      state.mediaRecaptureActive = false;
      syncRunButtons();
    }
  }

  function dispatchMediaRecapture(recaptureId) {
    return new Promise((resolve, reject) => {
      const timeout = window.setTimeout(() => {
        window.removeEventListener("message", onResult);
        reject(new Error("AkuBridge media recapture timed out."));
      }, 70_000);
      function finish(callback, value) {
        window.clearTimeout(timeout);
        window.removeEventListener("message", onResult);
        callback(value);
      }
      function onResult(event) {
        if (event.source !== window || event.origin !== endpoint || event.data?.recaptureId !== recaptureId) return;
        if (event.data.type === "AKU_BROWSER_MEDIA_RECAPTURE_COMPLETED") {
          finish(resolve, event.data.recapture);
        } else if (event.data.type === "AKU_BROWSER_MEDIA_RECAPTURE_FAILED") {
          finish(reject, new Error(event.data.message || "AkuBridge media recapture failed."));
        }
      }
      window.addEventListener("message", onResult);
      window.postMessage({
        type: "AKU_BROWSER_MEDIA_RECAPTURE",
        endpoint,
        token: state.bootstrap.bridgeToken,
        recaptureId,
      }, endpoint);
    });
  }

  return {
    buildActions, buildTimelineReadLaterAction, buildSemanticCorrectionActions, showEventSuggestions, applySemanticCorrection, showCorrectionNotice, buildSourceLink, mediaRecaptureReasonForEntry, buildMediaRecaptureButton, buildForegroundRecaptureOffer, recaptureMedia, dispatchMediaRecapture
  };
}

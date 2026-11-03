import { collectionModeState } from "./collection-mode.js";
import { isVerifiedSurfaceCleanupReceipt } from "./capture-surface-cleanup-receipt.js";
import { nativeReaderNotice, nativeReaderResumeMessage } from "./native-reader-notice.js";
import { releaseCompletedSourceSurfaces, runUsesBrowserCapture, sessionUsesBrowserCapture } from "./capture-surface-release-barrier.js";
import { setSettingsText } from "./settings-render.js";

export function createSessionUi({
  $, CAPTURE_CLEANUP_PENDING_DETAIL, CAPTURE_CLEANUP_PENDING_MESSAGE, api, clearNotice, defaultIntent, drainPlaybackRecovery, endpoint, humanize, getNativePostOpening, pollCollectionRuntime, refreshTimeline, renderAutoUpdateStatus, requestSourceSessionReadiness, retryBootstrap, schedulePassiveMediaEnrichment, setNoticeText, setPill, setView, shouldShowOnboardingLearning, showCalibration, showError, showNotice, sourceLabel, startPendingFirstCalibration, state, syncNativePostLinks, syncOnboardingLearning, terminalStatuses
}) {
  async function handleTimelinePrimaryAction() {
    if (state.bootstrapLoading || !state.bootstrap) {
      await retryBootstrap();
      return;
    }
    await startVisibleUpdate();
  }

  async function startVisibleUpdate() {
    if (state.captureCleanupPending || state.session || state.bootstrap?.calibration?.active || state.bootstrap?.onboarding?.status !== "completed" || !collectionModeState(state.bootstrap?.collectionRuntime,state.bootstrap?.bridge?.compatible).canCollect) return;
    hideFailure();
    clearNotice();
    setPill("#sidecar-status", "AkuSidecar ready", "ok");
    setView("timeline");
    try {
      const { session } = await api("/api/updates", {
        method: "POST",
        body: { intent: defaultIntent },
      });
      state.session = session;
      state.sessionProgress = { sessionId: session.id, value: 0 };
      state.dispatchRetryAfter.clear();
      renderSession();
      startPolling();
    } catch (error) {
      showError(error);
    }
  }

  async function cancelSession() {
    if (!state.session) return;
    const leaseId = state.session.id;
    try {
      const { session } = await api(`/api/sessions/${encodeURIComponent(state.session.id)}/cancel`, { method: "POST" });
      state.session = session;
      renderSession();
      stopPolling();
      state.dispatchKey = null;
      state.dispatchRetryAfter.clear();
      try {
        await releaseCaptureSurface(leaseId);
      } catch (error) {
        state.captureCleanupPending = leaseId;
        console.warn("Capture-surface cleanup after cancellation failed", error);
        renderSession();
        startPolling();
        return;
      }
      if (state.captureCleanupPending === leaseId) state.captureCleanupPending = null;
      state.session = null;
      renderSession();
    } catch (error) {
      showError(error);
    }
  }

  function startPolling() {
    if (state.poller) return;
    state.poller = setInterval(pollSession, 650);
    pollSession();
  }

  function stopPolling() {
    clearInterval(state.poller);
    state.poller = null;
  }

  async function pollSession() {
    if (!state.session || state.pollInFlight) return;
    const sessionId = state.session.id;
    state.pollInFlight = true;
    try {
      const { session } = await api(`/api/sessions/${encodeURIComponent(sessionId)}`);
      if (!state.session || state.session.id !== sessionId) return;
      state.session = session;
      const captureSurfaceBarrier = await releaseCompletedSourceSurfaces({
        session,
        releasedSources: state.releasedCaptureSources,
        releaseSource: releaseCaptureSurface,
      });
      if (!captureSurfaceBarrier.ready) {
        console.warn("Capture-surface cleanup deferred before the next source.", captureSurfaceBarrier.error);
      }
      renderSession();
      const captureActive = session.runs?.some((run) => run.status === "waiting_for_bridge" && run.bridgeCommandStatus === "claimed");
      const captureRun = session.runs?.find((run) => runUsesBrowserCapture(run) && run.status === "waiting_for_bridge" && (!run.bridgeCommandStatus || run.bridgeCommandStatus === "queued"));
      if (!captureActive && captureRun && captureSurfaceBarrier.ready) dispatch(captureRun);
      if (terminalStatuses.has(session.status)) {
        state.dispatchKey = null;
        state.dispatchRetryAfter.clear();
        try {
          await releaseCaptureSurface(session.id);
        } catch (error) {
          state.captureCleanupPending = session.id;
          console.warn("Capture-surface cleanup after session completion failed", error);
          renderSession();
          return;
        }
        if (state.captureCleanupPending === session.id) state.captureCleanupPending = null;
        stopPolling();
        if (session.delivery === "prepared") {
          const { autoUpdate } = await api("/api/auto-update/status");
          renderAutoUpdateStatus(autoUpdate);
        } else {
          await refreshTimeline();
          if (session.trigger === "user") {
            requestAnimationFrame(() => $("#timeline-heading")?.scrollIntoView({ block: "start", behavior: "smooth" }));
          }
        }
        showSessionOutcome(session);
        if (["failed", "partial"].includes(session.status)) showSessionFailure(session);
        state.session = null;
        renderSession();
        schedulePassiveMediaEnrichment();
        if (["completed", "partial"].includes(session.status)) {
          const calibration = await startPendingFirstCalibration(session);
          if (calibration) showCalibration(calibration);
        }
      }
    } catch (error) {
      if (/not found/i.test(error.message)) {
        try {
          await releaseCaptureSurface(sessionId);
        } catch (cleanupError) {
          state.captureCleanupPending = sessionId;
          console.warn("Capture-surface cleanup for a missing session is still pending.", cleanupError);
          renderSession();
          return;
        }
        if (state.captureCleanupPending === sessionId) state.captureCleanupPending = null;
        stopPolling();
        state.dispatchKey = null;
        state.dispatchRetryAfter.clear();
        state.session = null;
        renderSession();
        await refreshTimeline();
        return;
      }
      showError(error);
    } finally {
      state.pollInFlight = false;
    }
  }

  async function releaseCaptureSurface(leaseId, source = null) {
    if (state.session?.id === leaseId && !sessionUsesBrowserCapture(state.session)) return;
    let lastError = null;
    for (let attempt = 1; attempt <= 3; attempt += 1) {
      try {
        return await releaseCaptureSurfaceOnce(leaseId, source);
      } catch (error) {
        lastError = error;
        if (attempt < 3) await new Promise((resolve) => setTimeout(resolve, 250 * attempt));
      }
    }
    throw lastError ?? new Error("AkuBridge did not confirm capture-surface cleanup.");
  }

  function releaseCaptureSurfaceOnce(leaseId, source = null) {
    return new Promise((resolve, reject) => {
      const timeout = window.setTimeout(() => {
        window.removeEventListener("message", onReleaseResult);
        reject(new Error("AkuBridge capture-surface cleanup acknowledgement timed out."));
      }, 1500);
      function finish(callback, value) {
        window.clearTimeout(timeout);
        window.removeEventListener("message", onReleaseResult);
        callback(value);
      }
      function onReleaseResult(event) {
        if (event.source !== window || event.origin !== endpoint || event.data?.leaseId !== leaseId || (event.data?.source ?? null) !== source) return;
        if (event.data.type === "AKU_BROWSER_CAPTURE_SURFACE_RELEASED") {
          if (event.data.outcome?.reason === "lease_mismatch") {
            finish(reject, new Error("AkuBridge rejected cleanup because the capture lease changed."));
            return;
          }
          if (!isVerifiedSurfaceCleanupReceipt(event.data.outcome, leaseId, source)) {
            finish(reject, new Error("AkuBridge did not provide a verified capture-surface cleanup receipt."));
            return;
          }
          finish(resolve, event.data.outcome);
        } else if (event.data.type === "AKU_BROWSER_CAPTURE_SURFACE_RELEASE_FAILED") {
          finish(reject, new Error(event.data.message || "AkuBridge capture-surface cleanup failed."));
        }
      }
      window.addEventListener("message", onReleaseResult);
      window.postMessage({ type: "AKU_BROWSER_RELEASE_CAPTURE_SURFACE", leaseId, source }, endpoint);
    });
  }

  function dispatch(run) {
    if (!runUsesBrowserCapture(run)) return;
    const key = `${run.id}:${run.stage}`;
    if (state.dispatchKey === key) return;
    if ((state.dispatchRetryAfter.get(run.id) || 0) > Date.now()) return;
    state.dispatchRetryAfter.delete(run.id);
    state.dispatchKey = key;
    window.postMessage({
      type: "AKU_BROWSER_DISPATCH",
      endpoint,
      token: state.bootstrap.bridgeToken,
      runId: run.id,
    }, endpoint);
  }

  function renderSession() {
    syncNativePostLinks();
    const session = state.session;
    const cleanupPending = Boolean(state.captureCleanupPending);
    $("#processing-panel").classList.toggle("hidden", !cleanupPending && (!session || terminalStatuses.has(session.status)));
    const modeBadge = $("#processing-mode-badge");
    const badge = updatePolicyBadge(session);
    modeBadge.classList.toggle("hidden", !badge);
    modeBadge.textContent = badge?.text || "";
    modeBadge.setAttribute("aria-label", badge?.label || "");
    syncOnboardingLearning(shouldShowOnboardingLearning(session));
    syncRunButtons();
    if (cleanupPending) {
      $("#processing-title").textContent = "Capture cleanup pending";
      $("#processing-detail").textContent = CAPTURE_CLEANUP_PENDING_DETAIL;
      return;
    }
    if (!session || terminalStatuses.has(session.status)) return;
    const progress = describeSessionProgress(session);
    if (state.sessionProgress.sessionId !== session.id) {
      state.sessionProgress = { sessionId: session.id, value: 0 };
    }
    if (session.delivery === "prepared") {
      progress.detail = `Preparing a background batch · ${progress.detail}`;
    }
    state.sessionProgress.value = Math.max(state.sessionProgress.value, progress.value);
    const value = Math.min(97, state.sessionProgress.value);
    $("#progress-bar").style.width = `${value}%`;
    $("#progress-bar").parentElement.setAttribute("aria-valuenow", String(Math.round(value)));
    $("#processing-title").textContent = progress.title;
    $("#processing-detail").textContent = progress.detail;
  }

  function updatePolicyBadge(session) {
    if (!session) return null;
    if (session.trigger === "scheduler") return { text: "↻ AUTO", label: "Scheduled prepared update", className: "status-auto-update" };
    if (session.trigger === "onboarding") return { text: "FIRST RUN", label: "Onboarding update", className: "status-neutral" };
    if (session.delivery === "prepared") return { text: "PREPARE", label: "User-requested prepared batch", className: "status-neutral" };
    return { text: "UPDATE", label: "User-visible update", className: "status-neutral" };
  }

  function describeSessionProgress(session) {
    const pipelineStage = session.coverage?.pipelineStage;
    if (pipelineStage === "semantic_event_resolution" && firstRunCalibrationPending()) {
      return { value: 76, title: "Preparing calibration examples", detail: "Indexing the first clean sample locally - no semantic model turn" };
    }
    if (pipelineStage === "finalizing" && firstRunCalibrationPending()) {
      return { value: 97, title: "Finishing your first Timeline", detail: "Publishing calibration examples - AI Detection is skipped during onboarding" };
    }
    const pipelineStages = {
      semantic_event_resolution: { value: 76, title: "Resolving repeated events", detail: "Cross-source semantic event resolution" },
      timeline_composition: { value: 84, title: "Composing your Timeline", detail: "Applying global personalized order and finite capacity" },
      ai_fast_detection: { value: 91, title: "Checking AI-origin signals", detail: "AI Fast Detection · local and deterministic" },
      finalizing: { value: 97, title: "Finishing this update", detail: "Publishing Timeline · AI Deep Detection continues asynchronously" },
    };
    if (pipelineStages[pipelineStage]) return pipelineStages[pipelineStage];

    const runs = session.runs ?? [];
    const run = runs.find((candidate) => candidate.status === "waiting_for_bridge" && candidate.bridgeCommandStatus === "claimed")
      ?? runs.find((candidate) => candidate.status === "waiting_for_bridge")
      ?? runs.find((candidate) => candidate.status === "reasoning")
      ?? runs.find((candidate) => candidate.status === "queued");
    const runCount = Math.max(1, session.runs?.length ?? 1);
    const source = sourceLabel(run?.source);
    const stage = run?.stage ?? session.status;
    const stageProgressByName = {
      queued: 0.05,
      capture: 0.18,
      acquisition_planning: 0.38,
      follow_up_capture: 0.52,
      candidate_evaluation: 0.78,
      reasoning: 0.7,
      completed: 1,
      failed: 1,
      cancelled: 1,
    };
    const sourceProgress = runs.reduce((total, candidate) => {
      const candidateStage = candidate.stage ?? candidate.status;
      const progress = stageProgressByName[candidateStage] ?? (candidate.status === "reasoning" ? 0.7 : 0.18);
      return total + (candidate.status === "waiting_for_bridge" && candidate.bridgeCommandStatus === "queued" ? Math.min(progress, 0.12) : progress);
    }, 0) / runCount;
    const descriptions = {
      acquisition_planning: [`Planning ${source} follow-up`, "Deciding whether another bounded observation is useful"],
      follow_up_capture: [`Reading more ${source} evidence`, "Collecting the requested bounded follow-up"],
      candidate_evaluation: [`Evaluating ${source} evidence`, "Applying evidence and your personal preference profile"],
    };
    const queuedForCapture = run?.status === "waiting_for_bridge" && run.bridgeCommandStatus === "queued";
    const [title, stageDetail] = queuedForCapture
      ? [`Waiting for ${source} capture lane`, "Queued behind the active bounded browser capture"]
      : (descriptions[stage] ?? [`Reading ${source}`, humanize(stage)]);
    const activeStageDetails = runs
      .filter((candidate) => ["waiting_for_bridge", "reasoning"].includes(candidate.status))
      .slice(0, 3)
      .map((candidate) => {
        const labels = {
          capture: "capture",
          acquisition_planning: "planning",
          follow_up_capture: "follow-up capture",
          candidate_evaluation: "evaluation",
          reasoning: "reasoning",
        };
        const captureState = candidate.status === "waiting_for_bridge" && candidate.bridgeCommandStatus === "queued" ? "capture queued" : null;
        return `${sourceLabel(candidate.source)} ${captureState || labels[candidate.stage] || humanize(candidate.stage)}`;
      });
    const finished = runs.filter((candidate) => ["completed", "failed", "cancelled"].includes(candidate.status)).length;
    return {
      value: 4 + sourceProgress * 68,
      title,
      detail: `${activeStageDetails.join(" · ") || stageDetail} · ${finished} of ${runCount} sources finished`,
    };
  }

  function renderNativeReaderNotice(status) {
    const panel = $("#native-reader-update-notice");
    if (!panel) return;
    const notice = nativeReaderNotice(status, {
      busy: state.closingNativeReaders,
      feedback: state.nativeReaderResumeFeedback,
      runtimeFailure: state.bootstrap?.collectionRuntime?.failure,
    });
    panel.classList.toggle("hidden", !notice.visible);
    panel.setAttribute("aria-busy", String(state.closingNativeReaders));
    setSettingsText($("#native-reader-update-title"), notice.title);
    setSettingsText($("#native-reader-update-detail"), notice.detail);
    setSettingsText($("#native-reader-update-feedback"), notice.feedback);
    const manualAction = $("#native-reader-manual-action");
    setSettingsText(manualAction, notice.manualAction);
    manualAction.classList.toggle("hidden", !notice.manualAction);
    const button = $("#native-reader-close-resume");
    button.disabled = notice.disabled || getNativePostOpening();
    setSettingsText(button, notice.button);
  }

  async function closeNativeReadersAndResume() {
    if (state.closingNativeReaders || getNativePostOpening() || !state.bootstrap?.autoUpdate?.nativeReaderBlocked) return;
    state.closingNativeReaders = true;
    state.nativeReaderResumeFeedback = "";
    syncRunButtons();
    syncNativePostLinks();
    try {
      const response = await api("/api/collection/native-reader/close-and-resume", { method: "POST" });
      if (!response.autoUpdate && response.closed === true) {
        if (response.collectionRuntime) state.bootstrap.collectionRuntime = response.collectionRuntime;
        throw new Error("Jendela post ditutup, tetapi status update belum tersedia. Refresh Timeline untuk memeriksa pemulihan.");
      }
      if (response.closed === false && response.resumeOutcome === "no_reader" && !response.session) {
        state.bootstrap.autoUpdate = response.autoUpdate;
        if (response.collectionRuntime) state.bootstrap.collectionRuntime = response.collectionRuntime;
        renderAutoUpdateStatus(response.autoUpdate);
        await pollCollectionRuntime();
        showNotice("Jendela post sudah tidak terbuka. Status Auto Update diperbarui.");
        return;
      }
      if (response.closed !== true) throw new Error("Penutupan jendela post belum terverifikasi. Coba lagi.");
      state.bootstrap.autoUpdate = response.autoUpdate;
      if (response.collectionRuntime) state.bootstrap.collectionRuntime = response.collectionRuntime;
      if (response.session) {
        state.session = response.session;
        renderSession();
        startPolling();
      }
      renderAutoUpdateStatus(response.autoUpdate);
      await pollCollectionRuntime();
      showNotice(nativeReaderResumeMessage(response));
    } catch (error) {
      state.nativeReaderResumeFeedback = error.message || "Jendela post belum bisa ditutup. Coba lagi.";
    } finally {
      state.closingNativeReaders = false;
      syncRunButtons();
      syncNativePostLinks();
    }
  }

  function syncRunButtons() {
    drainPlaybackRecovery();
    renderNativeReaderNotice(state.bootstrap?.autoUpdate);
    const reason = state.captureCleanupPending ? CAPTURE_CLEANUP_PENDING_MESSAGE : runDisabledReason();
    const canRetryBootstrap = state.bootstrapLoading && Boolean(state.bootstrapError) && !state.captureCleanupPending;
    const disabled = Boolean(reason);
    const prepared = state.bootstrap?.autoUpdate?.preparedBatches?.length ?? 0;
    const preparedReason = state.captureCleanupPending ? CAPTURE_CLEANUP_PENDING_MESSAGE : preparedBatchDisabledReason(prepared);
    $("#timeline-runner-button").disabled = disabled && !canRetryBootstrap;
    $("#timeline-prepared-button").disabled = Boolean(preparedReason);
    $("#done-button").disabled = prepared > 0 ? Boolean(preparedReason) : disabled;
    setSettingsText($("#timeline-runner-button"), canRetryBootstrap ? "Retry connection" : "Update now");
    $("#timeline-runner-button").title = canRetryBootstrap ? "Retry restoring the Timeline and active check." : reason;
    $("#timeline-prepared-button").title = preparedReason;
    $("#done-button").title = prepared > 0 ? preparedReason : reason;
    const showCalibrationProgress = renderTimelineCalibrationProgress();
    const showGuidance = Boolean(reason) && !showCalibrationProgress && !Boolean(state.session && !terminalStatuses.has(state.session.status));
    setSettingsText($("#timeline-runner-status"), reason);
    $("#timeline-runner-guidance").classList.toggle("hidden", !showGuidance);
    $("#source-access-setup-button").classList.toggle("hidden", !showGuidance || state.bootstrap?.collectionRuntime?.nativeReaderOnly || !sourceAccessNeedsAttention());
    for (const button of document.querySelectorAll(".recapture-button")) button.disabled = disabled || (button.classList.contains("foreground-recapture-button") && state.bootstrap?.collectionRuntime?.effective === "headless");
    $("#open-reset-learning").disabled = Boolean(state.session);
    $("#open-full-reset").disabled = Boolean(state.session);
  }

  function renderTimelineCalibrationProgress() {
    const calibration = state.bootstrap?.calibration?.active;
    const panel = $("#timeline-calibration-progress");
    if (!calibration) {
      panel.classList.add("hidden");
      return false;
    }
    const samples = Array.isArray(calibration.samples) ? calibration.samples : [];
    const total = samples.length;
    const resolved = samples.filter((sample) => sample.label || sample.issueCode).length;
    const remaining = Math.max(0, total - resolved);
    const progress = total ? Math.round((resolved / total) * 100) : 0;
    const track = $("#timeline-calibration-progress-track");
    panel.classList.remove("hidden");
    panel.classList.toggle("is-preparing", total === 0);
    $("#timeline-calibration-progress-label").textContent = total
      ? `${resolved} of ${total} reviewed · ${remaining} left`
      : "Preparing calibration samples";
    $("#timeline-calibration-progress-bar").style.width = `${progress}%`;
    track.setAttribute("aria-valuenow", String(progress));
    track.setAttribute("aria-valuetext", total ? `${resolved} of ${total} calibration samples reviewed` : "Preparing calibration samples");
    return true;
  }

  function preparedBatchDisabledReason(prepared) {
    if (state.captureCleanupPending) return CAPTURE_CLEANUP_PENDING_MESSAGE;
    if (prepared < 1) return "No prepared batch is available.";
    if (state.bootstrapLoading || !state.bootstrap) return "Restoring prepared batches…";
    if (state.revealingPreparedBatch) return "Loading the prepared batch…";
    return "";
  }

  function runDisabledReason() {
    if (state.captureCleanupPending) return CAPTURE_CLEANUP_PENDING_MESSAGE;
    if (state.closingNativeReaders) return "Closing native post windows and restoring collection…";
    if (state.bootstrapLoading && state.bootstrapError) return "Timeline restore was interrupted. Retry now or wait for automatic recovery.";
    if (state.bootstrapLoading) return "Restoring your Timeline and active check…";
    if (state.session) {
      return terminalStatuses.has(state.session.status) ? "Finishing capture cleanup…" : "A check for updates is already running.";
    }
    if (state.mediaRecaptureActive) return "Finishing media recapture…";
    if (state.bootstrap?.calibration?.active) return "Finish calibration before starting another check.";
    if (state.bootstrap?.onboarding?.status !== "completed") return "Complete source setup before checking for updates.";
    const bridgeUnavailable = bridgeUnavailableReason(state.bootstrap?.bridge);
    if (bridgeUnavailable) return bridgeUnavailable;
    const grantedSources = state.bootstrap?.bridge?.actual?.sourceAccess?.grantedSources;
    if (state.bootstrap?.collectionRuntime?.effective === "headless") {
      const allowed = new Set(state.bootstrap.collectionRuntime.authorizedSources ?? []);
      return (state.bootstrap.settings.activeSources ?? []).some(source => allowed.has(source)) ? "" : "Source access is not confirmed. Open the source to grant access in Browser mode.";
    }
    if (!Array.isArray(grantedSources)) return "Waiting for AkuBridge source permission status…";
    if (sourceAccessNeedsAttention()) {
      return "No active source is ready. Open Settings, then use each source's Grant access or Sign in action before updating.";
    }
    return "";
  }

  function bridgeUnavailableReason(bridge) {
    const runtime = state.bootstrap?.collectionRuntime;
    if (runtime?.nativeReaderOnly) return "Close the native post windows to resume headless collection.";
    if (runtime?.available && runtime.pending) return "Waiting for the collection mode transition. Finish capture or close open source windows.";
    if (collectionModeState(runtime,bridge?.compatible).canCollect) return "";
    if (bridge?.compatible) return "";
    if (bridge?.state === "incompatible") {
      const reasons = Array.isArray(bridge.reasons)
        ? bridge.reasons.map((reason) => String(reason ?? "").trim()).filter(Boolean)
        : [];
      return reasons.length
        ? `AkuBridge update or reload required: ${reasons.join(", ")}.`
        : "AkuBridge update or reload required because its capabilities are incompatible.";
    }
    return "Waiting for AkuBridge to reconnect…";
  }

  function sourceAccessNeedsAttention() {
    const activeSources = state.bootstrap?.settings?.activeSources ?? [];
    const readySources = new Set(bridgeSourceReadiness().filter((source) => source.ready).map((source) => source.source));
    return readySources.size === 0 || !activeSources.some((source) => readySources.has(source));
  }

  function bridgeSourceReadiness() {
    const access = state.bootstrap?.bridge?.actual?.sourceAccess;
    if (Array.isArray(access?.sources) && access.sources.length) return access.sources;
    return [];
  }

  function openSourceAccessSettings() {
    setView("settings");
    requestSourceSessionReadiness();
  }

  function showSessionFailure(session) {
    const failedRuns = (session.runs ?? []).filter((run) => ["failed", "cancelled"].includes(run.status));
    const failedSources = failedRuns.map((run) => sourceLabel(run.source)).join(" and ");
    const sourcesUnavailable = failedRuns.length > 0 && failedRuns.every((run) => run.error?.code === "source_unavailable");
    const windowsSecurityBlocked = failedRuns.some(isWindowsSecurityRuntimeFailure);
    const panel = $("#failure-panel");
    const runtimeRecovery = $("#failure-runtime-recovery");
    $("#retry-button").dataset.action = "return";
    $("#retry-button").textContent = "Return to timeline";
    runtimeRecovery.classList.toggle("hidden", !windowsSecurityBlocked);
    if (windowsSecurityBlocked) {
      $("#failure-manual-bundle-download").href = matchingWindowsPortableBundleURL();
    }
    panel.classList.toggle("failure-panel-warning", sourcesUnavailable || windowsSecurityBlocked);
    panel.setAttribute("role", sourcesUnavailable ? "status" : "alert");
    $("#failure-label").textContent = windowsSecurityBlocked
      ? "WINDOWS SECURITY BLOCKED RUNTIME"
      : sourcesUnavailable ? "SOURCE UNAVAILABLE" : "RUN STOPPED";
    $("#failure-title").textContent = windowsSecurityBlocked
      ? "AkuSidecar could not start Codex with normal file access"
      : sourcesUnavailable
        ? `${failedSources || "One source"} is temporarily unavailable`
        : "The bounded snapshot could not finish";
    if (windowsSecurityBlocked) {
      $("#failure-message").textContent = "The local runtime is installed and healthy, but Avast or another Windows security product denied the Codex process access to its local state. Repeating Update now without restarting AkuSidecar will fail again.";
    } else if (sourcesUnavailable) {
      $("#failure-message").textContent = `${failedSources} reported a temporary service issue. AkuBrowser kept the validated results from every source that completed; retry after the source recovers.`;
    } else if (session.status === "partial") {
      $("#failure-message").textContent = `${failedSources || "One source"} could not finish. AkuBrowser retained and ordered the validated result from the source that completed.`;
    } else {
      $("#failure-message").textContent = session.error?.message
        || failedRuns[0]?.error?.message
        || "The session stopped before an active source could finish.";
    }
    panel.classList.remove("hidden");
  }

  function isWindowsSecurityRuntimeFailure(run) {
    if (run?.error?.code !== "reasoning_failed") return false;
    const message = String(run.error.message || "").toLowerCase();
    const accessDenied = message.includes("access is denied") || message.includes("os error 5") || message.includes("spawn eperm");
    const codexState = message.includes("codex app server") || message.includes("sqlite state runtime") || message.includes("state runtime");
    return accessDenied && codexState;
  }

  function matchingWindowsPortableBundleURL() {
    const version = String(state.bootstrap?.version || "").trim();
    if (!/^\d+\.\d+\.\d+$/.test(version)) {
      return "https://github.com/abangkis/AkuBrowser/releases/latest";
    }
    return `https://github.com/abangkis/AkuBrowser/releases/download/v${version}/AkuBrowser-${version}-windows-x64.zip`;
  }

  function firstRunCalibrationPending() {
    return state.bootstrap?.calibration?.firstRunStatus === "pending";
  }

  function showCalibrationRetry(session) {
    clearNotice();
    setPill("#sidecar-status", "AkuSidecar ready", "ok");
    const failedSources = (session?.runs ?? [])
      .filter((run) => run.status === "failed")
      .map((run) => sourceLabel(run.source));
    const panel = $("#failure-panel");
    $("#failure-runtime-recovery").classList.add("hidden");
    panel.classList.add("failure-panel-warning");
    panel.setAttribute("role", "status");
    $("#failure-label").textContent = "CALIBRATION WAITING";
    $("#failure-title").textContent = "Check once more to start calibration";
    $("#failure-message").textContent = failedSources.length
      ? `${failedSources.join(" and ")} did not produce a validated entry in this check. Choose Update now again; captured evidence remains available in Update Inbox.`
      : "This check did not produce a validated entry for calibration. Choose Update now again to collect another bounded sample.";
    $("#retry-button").dataset.action = "retry-update";
    $("#retry-button").textContent = "Update now again";
    panel.classList.remove("hidden");
  }

  function showSessionOutcome(session) {
    if (!["completed", "partial"].includes(session.status)) return;
    const additions = session.itemCount ?? session.items?.length ?? 0;
    if (session.delivery === "prepared") {
      const notice = $("#provider-notice");
      notice.className = "notice notice-complete";
      notice.setAttribute("role", "status");
      setNoticeText(notice, additions > 0
        ? `${session.trigger === "scheduler" ? "Auto Update" : "AkuBrowser"} prepared ${additions} item${additions === 1 ? "" : "s"}. Load it from the Timeline header or continue from the finish line when you are ready.`
        : "Batch preparation completed without a new prepared batch. The diagnostic trace remains in Update Inbox.");
      return;
    }
    if (additions > 0) {
      clearNotice();
      return;
    }
    const notice = $("#provider-notice");
    notice.className = "notice notice-complete";
    notice.setAttribute("role", "status");
    setNoticeText(notice, "Update complete: 0 additions. No captured candidate cleared the new, material, and trusted-evidence boundary.");
  }

  function hideFailure() {
    $("#failure-panel").classList.add("hidden");
    $("#failure-runtime-recovery").classList.add("hidden");
    $("#retry-button").dataset.action = "return";
    $("#retry-button").textContent = "Return to timeline";
  }

  return {
    handleTimelinePrimaryAction, startVisibleUpdate, cancelSession, startPolling, stopPolling, pollSession, releaseCaptureSurface, releaseCaptureSurfaceOnce, dispatch, renderSession, updatePolicyBadge, describeSessionProgress, renderNativeReaderNotice, closeNativeReadersAndResume, syncRunButtons, renderTimelineCalibrationProgress, preparedBatchDisabledReason, runDisabledReason, bridgeUnavailableReason, sourceAccessNeedsAttention, bridgeSourceReadiness, openSourceAccessSettings, showSessionFailure, isWindowsSecurityRuntimeFailure, matchingWindowsPortableBundleURL, firstRunCalibrationPending, showCalibrationRetry, showSessionOutcome, hideFailure
  };
}

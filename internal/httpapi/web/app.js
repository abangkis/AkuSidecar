const endpoint = location.origin;
const defaultIntent = "What materially changed since my last check?";
const terminalStatuses = new Set(["completed", "partial", "failed", "cancelled"]);
const BACK_TO_TOP_THRESHOLD_PX = 480;
const SOURCE_TEXT_COLLAPSE_CHARACTERS = 420;
const SOURCE_TEXT_COLLAPSE_LINES = 6;
const QUOTE_TEXT_COLLAPSE_CHARACTERS = 280;
const QUOTE_TEXT_COLLAPSE_LINES = 4;
const DEFAULT_TIMELINE_BATCH_GAP_PX = 36;
const LOAD_PROFILE_PRESETS = {
  standard: { timelineCapacity: 12, maxItemsPerSource: 5, maxItemsTotal: 10, maxScrolls: 2 },
  expanded: { timelineCapacity: 24, maxItemsPerSource: 10, maxItemsTotal: 20, maxScrolls: 4 },
  stress: { timelineCapacity: 36, maxItemsPerSource: 15, maxItemsTotal: 30, maxScrolls: 6 },
};
const state = {
  bootstrap: null,
  session: null,
  dispatchKey: null,
  poller: null,
  currentView: "timeline",
  media: [],
  mediaIndex: 0,
  onboardingEditing: false,
  calibration: null,
  calibrationOrdinal: 0,
  resetOperation: null,
  backToTopFrame: null,
  backToTopLastScrollY: 0,
  backToTopBoundary: null,
};
const $ = (selector) => document.querySelector(selector);

window.addEventListener("message", (event) => {
  if (event.source !== window || event.origin !== endpoint || !event.data) return;
  if (event.data.type === "AKU_BROWSER_BRIDGE_READY") {
    bridgeApi("/api/bridge/heartbeat", {
      method: "POST",
      body: { capabilities: event.data.capabilities ?? {} },
    }).then(({ bridge }) => renderBridge(bridge)).catch(showError);
  }
  if (event.data.type === "AKU_BROWSER_BRIDGE_ERROR") {
    showError(new Error(event.data.message));
  }
});

$("#session-view-button").addEventListener("click", () => setView("timeline"));
$("#inbox-view-button").addEventListener("click", () => setView("inbox"));
$("#settings-view-button").addEventListener("click", () => setView("settings"));
$("#timeline-refresh-button").addEventListener("click", refreshTimeline);
$("#inbox-refresh-button").addEventListener("click", loadInbox);
$("#timeline-runner-button").addEventListener("click", startSession);
$("#done-button").addEventListener("click", startSession);
$("#retry-button").addEventListener("click", () => {
  hideFailure();
  setView("timeline");
});
$("#cancel-button").addEventListener("click", cancelSession);
$("#runtime-settings-form").addEventListener("submit", saveSettings);
$("#bounded-load-profile").addEventListener("change", () => syncLoadProfileSettings(true));
$("#semantic-event-mode").addEventListener("change", syncSemanticEventSettings);
$("#stream-width").addEventListener("change", () => applyStreamWidth($("#stream-width").value));
$("#timeline-batch-gap").addEventListener("input", () => applyTimelineBatchGap($("#timeline-batch-gap").value));
$("#reset-timeline-batch-gap").addEventListener("click", resetTimelineBatchGap);
$("#edit-onboarding-profile").addEventListener("click", () => showOnboarding(true));
$("#onboarding-form").addEventListener("submit", saveOnboarding);
$("#onboarding-cancel").addEventListener("click", () => setView("settings"));
for (const input of document.querySelectorAll("#onboarding-form input[type='checkbox']")) {
  input.addEventListener("change", updateOnboardingSummary);
}
$("#calibration-previous").addEventListener("click", showPreviousCalibrationSample);
$("#calibration-less").addEventListener("click", () => decideCalibration({ label: "less_like_this" }));
$("#calibration-neutral").addEventListener("click", () => decideCalibration({ label: "neutral" }));
$("#calibration-more").addEventListener("click", () => decideCalibration({ label: "more_like_this" }));
for (const button of document.querySelectorAll("[data-calibration-issue]")) {
  button.addEventListener("click", () => decideCalibration({ issueCode: button.dataset.calibrationIssue }));
}
$("#open-reset-learning").addEventListener("click", () => openResetDialog("learning"));
$("#open-full-reset").addEventListener("click", () => openResetDialog("full"));
$("#reset-confirmation-cancel").addEventListener("click", closeResetDialog);
$("#reset-confirmation-input").addEventListener("input", syncResetConfirmation);
$("#reset-confirmation-submit").addEventListener("click", submitReset);
$("#back-to-top").addEventListener("click", returnToTop);
$("#media-viewer-close").addEventListener("click", () => $("#media-viewer").close());
$("#media-viewer-previous").addEventListener("click", () => moveMedia(-1));
$("#media-viewer-next").addEventListener("click", () => moveMedia(1));
window.addEventListener("scroll", scheduleBackToTop, { passive: true });
window.addEventListener("resize", scheduleBackToTop, { passive: true });

async function bootstrap() {
  try {
    clearNotice();
    state.bootstrap = await api("/api/bootstrap");
    state.session = state.bootstrap.activeSession;
    $("#runtime-version").textContent = `${state.bootstrap.version} · ${state.bootstrap.runtime}`;
    $("#bridge-contract").textContent = state.bootstrap.bridgeContractVersion;
    $("#provider-value").textContent = state.bootstrap.provider;
    $("#database-status").textContent = state.bootstrap.database?.status ?? "healthy";
    setPill("#sidecar-status", "AkuSidecar ready", "ok");
    setPill("#reasoning-status", state.bootstrap.provider, "neutral");
    renderBridge(state.bootstrap.bridge);
    renderSettings(state.bootstrap.settings);
    renderTimeline(state.bootstrap.timeline ?? [], state.bootstrap.latestCheck ?? null);
    renderSession();
    if (state.bootstrap.onboarding?.status !== "completed") {
      showOnboarding(false);
    } else if (state.bootstrap.calibration?.active) {
      showCalibration(state.bootstrap.calibration.active);
    } else {
      setView(state.currentView);
    }
    pingBridge();
    bridgeActionLoop();
    setInterval(pingBridge, 30_000);
    if (state.session) startPolling();
  } catch (error) {
    showError(error);
    setTimeout(bootstrap, 1500);
  }
}

function setView(view) {
  if (state.bootstrap?.onboarding?.status !== "completed") {
    showOnboarding(false);
    return;
  }
  if (state.bootstrap?.calibration?.active) {
    showCalibration(state.bootstrap.calibration.active);
    return;
  }
  state.currentView = view;
  state.onboardingEditing = false;
  const timeline = view === "timeline";
  const inbox = view === "inbox";
  const settings = view === "settings";
  $("#onboarding-panel").classList.add("hidden");
  $("#calibration-panel").classList.add("hidden");
  document.querySelector(".view-switch")?.classList.remove("hidden");
  $("#settings-panel").classList.toggle("hidden", !settings);
  $("#inbox-panel").classList.toggle("hidden", !inbox);
  $("#timeline-panel").classList.toggle("hidden", !timeline);
  $("#session-view-button").classList.toggle("selected", timeline);
  $("#inbox-view-button").classList.toggle("selected", inbox);
  $("#settings-view-button").classList.toggle("selected", settings);
  ({ timeline: $("#timeline-heading"), inbox: $("#inbox-heading"), settings: $("#settings-heading") }[view])?.focus?.();
  if (inbox) loadInbox();
  scheduleBackToTop();
}

function pingBridge() {
  window.postMessage({ type: "AKU_BROWSER_BRIDGE_PING" }, endpoint);
}

async function bridgeActionLoop() {
  while (true) {
    try {
      const response = await bridgeApi("/api/operations/bridge/actions/next?waitMs=25000");
      if (response?.action) {
        window.postMessage({
          type: "AKU_BROWSER_BRIDGE_RELOAD_SELF",
          actionId: response.action.id,
          endpoint,
          token: state.bootstrap.bridgeToken,
        }, endpoint);
      }
    } catch (error) {
      console.warn("Bridge action relay paused", error);
      await new Promise((resolve) => setTimeout(resolve, 1000));
    }
  }
}

function renderBridge(bridge) {
  if (state.bootstrap) state.bootstrap.bridge = bridge;
  if (bridge?.compatible) {
    setPill("#bridge-status", `AkuBridge ${bridge.actual?.extensionVersion} ready`, "ok");
  } else if (bridge?.state === "incompatible") {
    setPill("#bridge-status", bridge.reasons?.join(", ") || "Bridge incompatible", "danger");
  } else {
    setPill("#bridge-status", "AkuBridge reconnecting", "warning");
  }
  syncRunButtons();
}

function renderSettings(settings) {
  if (!settings) return;
  $("#bounded-load-profile").value = settings.loadProfile;
  $("#capture-visibility-policy").value = settings.captureVisibility;
  $("#preference-eligibility-mode").value = settings.preferenceEligibilityMode;
  $("#calibration-enabled").checked = settings.calibrationEnabled;
  $("#calibration-batch-size").value = settings.calibrationBatchSize;
  $("#open-missing-source").checked = settings.openMissingSource;
  $("#timeline-capacity").value = settings.timelineCapacity;
  $("#max-items-per-source").value = settings.maxItemsPerSource;
  $("#max-scrolls").value = settings.maxScrolls;
  $("#default-presentation").value = settings.defaultPresentation || "source";
  $("#stream-width").value = settings.streamWidth || "social";
  $("#timeline-batch-gap").value = settings.timelineBatchGapPx || DEFAULT_TIMELINE_BATCH_GAP_PX;
  $("#timeline-boundary-follow").checked = settings.timelineBoundaryCueMode !== "static";
  $("#semantic-event-mode").value = settings.semanticEventMode || "collapse";
  $("#semantic-event-shortlist").value = String(settings.semanticEventShortlist || 10);
  $("#knowledge-retention-days").value = String(settings.knowledgeRetentionDays || 30);
  $("#knowledge-storage-limit").value = String(settings.knowledgeStorageLimitMb || 100);
  $("#settings-source-x").checked = settings.activeSources?.includes("x") ?? false;
  $("#settings-source-linkedin").checked = settings.activeSources?.includes("linkedin") ?? false;
  applyStreamWidth(settings.streamWidth || "social");
  applyTimelineBatchGap(settings.timelineBatchGapPx || DEFAULT_TIMELINE_BATCH_GAP_PX);
  if (settings.timelineBoundaryCueMode === "static") releaseBackToTopBoundary();
  syncLoadProfileSettings(false);
  syncSemanticEventSettings();
}

async function saveSettings(event) {
  event.preventDefault();
  const current = state.bootstrap.settings;
  const activeSources = [
    $("#settings-source-x").checked ? "x" : null,
    $("#settings-source-linkedin").checked ? "linkedin" : null,
  ].filter(Boolean);
  if (!activeSources.length) {
    $("#runtime-settings-status").textContent = "Choose at least one active source.";
    return;
  }
  const loadProfile = $("#bounded-load-profile").value;
  const preset = LOAD_PROFILE_PRESETS[loadProfile];
  const perSource = Number.parseInt($("#max-items-per-source").value, 10);
  const settings = {
    ...current,
    loadProfile,
    captureVisibility: $("#capture-visibility-policy").value,
    preferenceEligibilityMode: $("#preference-eligibility-mode").value,
    calibrationEnabled: $("#calibration-enabled").checked,
    calibrationBatchSize: Number.parseInt($("#calibration-batch-size").value, 10),
    openMissingSource: $("#open-missing-source").checked,
    activeSources,
    timelineCapacity: Number.parseInt($("#timeline-capacity").value, 10),
    maxItemsPerSource: perSource,
    maxItemsTotal: preset?.maxItemsTotal ?? Math.min(30, Math.max(1, perSource * 2)),
    maxScrolls: Number.parseInt($("#max-scrolls").value, 10),
    defaultPresentation: $("#default-presentation").value,
    streamWidth: $("#stream-width").value,
    timelineBatchGapPx: Number.parseInt($("#timeline-batch-gap").value, 10),
    timelineBoundaryCueMode: $("#timeline-boundary-follow").checked ? "follow" : "static",
    semanticEventMode: $("#semantic-event-mode").value,
    semanticEventShortlist: Number.parseInt($("#semantic-event-shortlist").value, 10),
    knowledgeRetentionDays: Number.parseInt($("#knowledge-retention-days").value, 10),
    knowledgeStorageLimitMb: Number.parseInt($("#knowledge-storage-limit").value, 10),
  };
  const status = $("#runtime-settings-status");
  status.textContent = "Saving…";
  try {
    const response = await api("/api/settings", { method: "PUT", body: { settings } });
    state.bootstrap.settings = response.settings;
    renderSettings(response.settings);
    status.textContent = `Saved · ${response.settings.maxScrolls} scrolls · ${response.settings.maxItemsPerSource} items/source`;
    await refreshTimeline();
  } catch (error) {
    status.textContent = error.message;
    showError(error);
  }
}

function syncLoadProfileSettings(applyPreset) {
  const loadProfile = $("#bounded-load-profile").value;
  const preset = LOAD_PROFILE_PRESETS[loadProfile];
  if (applyPreset && preset) {
    $("#timeline-capacity").value = preset.timelineCapacity;
    $("#max-items-per-source").value = preset.maxItemsPerSource;
    $("#max-scrolls").value = preset.maxScrolls;
  }
  const custom = loadProfile === "custom";
  $("#timeline-capacity").disabled = !custom;
  $("#max-items-per-source").disabled = !custom;
  $("#max-scrolls").disabled = !custom;
}

function syncSemanticEventSettings() {
  const disabled = $("#semantic-event-mode").value === "show_all";
  $("#semantic-event-shortlist").disabled = disabled;
  $("#semantic-event-shortlist").closest(".settings-row")?.classList.toggle("settings-row-disabled", disabled);
}

function applyStreamWidth(value) {
  document.body.dataset.streamWidth = ["compact", "social", "comfortable", "wide"].includes(value) ? value : "social";
  scheduleBackToTop();
}

function applyTimelineBatchGap(value) {
  const parsed = Number.parseInt(value, 10);
  const bounded = Number.isFinite(parsed) ? Math.min(80, Math.max(16, parsed)) : DEFAULT_TIMELINE_BATCH_GAP_PX;
  document.documentElement.style.setProperty("--timeline-batch-gap", `${bounded}px`);
}

function resetTimelineBatchGap() {
  $("#timeline-batch-gap").value = DEFAULT_TIMELINE_BATCH_GAP_PX;
  applyTimelineBatchGap(DEFAULT_TIMELINE_BATCH_GAP_PX);
  $("#runtime-settings-status").textContent = "Default restored · save settings to keep it.";
}

function showOnboarding(editing) {
  state.onboardingEditing = editing;
  const sources = state.bootstrap?.settings?.activeSources ?? ["x", "linkedin"];
  $("#onboarding-source-x").checked = sources.includes("x");
  $("#onboarding-source-linkedin").checked = sources.includes("linkedin");
  $("#onboarding-cancel").classList.toggle("hidden", !editing);
  $("#onboarding-finish").textContent = editing ? "Save profile" : "Start calibrating";
  $("#onboarding-error").textContent = "";
  $("#settings-panel").classList.add("hidden");
  $("#inbox-panel").classList.add("hidden");
  $("#timeline-panel").classList.add("hidden");
  $("#calibration-panel").classList.add("hidden");
  $("#onboarding-panel").classList.remove("hidden");
  document.querySelector(".view-switch")?.classList.add("hidden");
  updateOnboardingSummary();
  $("#onboarding-heading").focus();
  window.scrollTo({ top: 0, behavior: editing ? "smooth" : "auto" });
  scheduleBackToTop();
}

function updateOnboardingSummary() {
  const count = document.querySelectorAll("#onboarding-form input[type='checkbox']:checked").length;
  $("#onboarding-summary").textContent = count
    ? `${count} source feed${count === 1 ? "" : "s"} selected · ready to calibrate your Timeline`
    : "Choose at least one source feed.";
}

async function saveOnboarding(event) {
  event.preventDefault();
  const activeSources = [...document.querySelectorAll("#onboarding-form input[type='checkbox']:checked")].map((input) => input.value);
  if (!activeSources.length) {
    $("#onboarding-error").textContent = "Choose at least one active source.";
    return;
  }
  const firstCompletion = state.bootstrap?.onboarding?.status !== "completed";
  const button = $("#onboarding-finish");
  button.disabled = true;
  $("#onboarding-error").textContent = "Saving source profile…";
  try {
    const response = await api("/api/onboarding", { method: "PUT", body: { activeSources } });
    state.bootstrap.onboarding = response.onboarding;
    state.bootstrap.settings = response.settings;
    state.bootstrap.calibration = response.calibration;
    renderSettings(response.settings);
    $("#onboarding-error").textContent = "";
    setView(firstCompletion ? "timeline" : "settings");
    if (firstCompletion) await startSession();
  } catch (error) {
    $("#onboarding-error").textContent = error.message;
  } finally {
    button.disabled = false;
  }
}

function openResetDialog(operation) {
  if (state.session) {
    $("#runtime-settings-status").textContent = "Reset is unavailable while an update is running.";
    return;
  }
  state.resetOperation = operation;
  const full = operation === "full";
  $("#reset-confirmation-title").textContent = full ? "Full reset and onboard again" : "Reset learning";
  $("#reset-confirmation-impact").textContent = full
    ? "AkuBrowser will first create and verify a local SQLite backup, then erase Timeline, runs, learning data, onboarding, and local settings. The live Bridge identity remains valid."
    : "AkuBrowser will erase calibration, More/Less feedback, and the fitted preference model. Timeline, source setup, and runtime settings remain.";
  $("#reset-confirmation-phrase").textContent = full ? "RESET AKUBROWSER" : "RESET LEARNING";
  $("#reset-confirmation-input").value = "";
  $("#reset-confirmation-status").textContent = "";
  $("#reset-confirmation-submit").disabled = true;
  $("#reset-confirmation-dialog").showModal();
  $("#reset-confirmation-input").focus();
}

function closeResetDialog() {
  state.resetOperation = null;
  $("#reset-confirmation-dialog").close();
}

function syncResetConfirmation() {
  const phrase = $("#reset-confirmation-phrase").textContent;
  $("#reset-confirmation-submit").disabled = $("#reset-confirmation-input").value !== phrase;
}

async function submitReset() {
  const operation = state.resetOperation;
  if (!operation) return;
  const phrase = $("#reset-confirmation-phrase").textContent;
  const button = $("#reset-confirmation-submit");
  button.disabled = true;
  $("#reset-confirmation-status").textContent = operation === "full" ? "Creating and verifying backup…" : "Resetting learning…";
  try {
    const path = operation === "full" ? "/api/operations/full-reset" : "/api/operations/reset-learning";
    const response = await api(path, { method: "POST", body: { confirmation: phrase } });
    if (operation === "full") {
      $("#reset-confirmation-status").textContent = `Verified backup ${response.reset?.backupFile || "created"}. Returning to onboarding…`;
      window.setTimeout(() => window.location.reload(), 350);
      return;
    }
    state.bootstrap.calibration = response.calibration;
    $("#reset-confirmation-status").textContent = "Learning reset complete.";
    window.setTimeout(() => {
      closeResetDialog();
      $("#runtime-settings-status").textContent = "Learning reset complete.";
    }, 350);
  } catch (error) {
    $("#reset-confirmation-status").textContent = error.message;
    syncResetConfirmation();
  }
}

function scheduleBackToTop() {
  if (state.backToTopFrame) return;
  state.backToTopFrame = requestAnimationFrame(() => {
    state.backToTopFrame = null;
    const top = document.scrollingElement?.scrollTop ?? window.scrollY ?? 0;
    $("#back-to-top").classList.toggle("hidden", top < BACK_TO_TOP_THRESHOLD_PX);
    syncBackToTopPosition(top);
  });
}

function syncBackToTopPosition(top) {
  const candidates = state.currentView === "timeline"
    ? [$("#result-panel"), document.querySelector(".timeline-heading-row")]
    : state.currentView === "inbox"
      ? [$("#inbox-panel")]
      : [$("#settings-panel")];
  const anchor = candidates.find((element) => {
    if (!element || element.classList.contains("hidden")) return false;
    const rect = element.getBoundingClientRect();
    return rect.width > 0 && rect.height > 0;
  });
  const anchorRect = anchor?.getBoundingClientRect();
  const buttonWidth = window.innerWidth <= 700 ? 44 : 48;
  const gap = 30;
  syncBackToTopBoundaryPosition(top, buttonWidth);
  if (anchorRect && window.innerWidth - anchorRect.right >= buttonWidth + gap * 2) {
    $("#back-to-top").style.left = `${Math.round(anchorRect.right + gap)}px`;
    $("#back-to-top").style.right = "auto";
    return;
  }
  $("#back-to-top").style.removeProperty("left");
  $("#back-to-top").style.removeProperty("right");
}

function syncBackToTopBoundaryPosition(top, buttonHeight) {
  const button = $("#back-to-top");
  const delta = top - state.backToTopLastScrollY;
  state.backToTopLastScrollY = top;
  const cueEnabled = state.bootstrap?.settings?.timelineBoundaryCueMode !== "static";
  if (!cueEnabled || state.currentView !== "timeline" || delta < -1 || top < BACK_TO_TOP_THRESHOLD_PX) {
    releaseBackToTopBoundary();
    return;
  }

  const restBottom = window.innerWidth <= 700 ? 14 : 20;
  const safeTop = 16 + buttonHeight / 2;
  const acquisitionBottom = window.innerHeight;
  const linePosition = (marker) => marker?.isConnected ? marker.getBoundingClientRect().bottom : Number.NaN;
  let marker = state.backToTopBoundary;
  let lineY = linePosition(marker);
  const markerVisible = Number.isFinite(lineY) && lineY >= safeTop && lineY <= acquisitionBottom;

  if (marker && !markerVisible) {
    releaseBackToTopBoundary();
    marker = null;
  }
  if (!marker && delta > 1) {
    marker = [...document.querySelectorAll(".timeline-older-batch-marker")].find((candidate) => {
      const candidateY = linePosition(candidate);
      return Number.isFinite(candidateY) && candidateY >= safeTop && candidateY <= acquisitionBottom;
    }) ?? null;
    lineY = linePosition(marker);
  }
  if (!marker || !Number.isFinite(lineY)) return;

  state.backToTopBoundary = marker;
  button.classList.add("is-following-boundary");
  const bottom = Math.max(restBottom, window.innerHeight - lineY - buttonHeight / 2);
  button.style.setProperty("--back-to-top-bottom", `${Math.round(bottom)}px`);
}

function releaseBackToTopBoundary() {
  state.backToTopBoundary = null;
  const button = $("#back-to-top");
  button.classList.remove("is-following-boundary");
  button.style.removeProperty("--back-to-top-bottom");
}

function returnToTop() {
  const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  window.scrollTo({ top: 0, behavior: reducedMotion ? "auto" : "smooth" });
  $("#app-heading").focus({ preventScroll: true });
}

async function startSession() {
  if (state.session || state.bootstrap?.calibration?.active || state.bootstrap?.onboarding?.status !== "completed" || !state.bootstrap?.bridge?.compatible) return;
  hideFailure();
  clearNotice();
  setPill("#sidecar-status", "AkuSidecar ready", "ok");
  setView("timeline");
  try {
    const { session } = await api("/api/sessions", {
      method: "POST",
      body: { intent: defaultIntent },
    });
    state.session = session;
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
    await releaseCaptureSurface(leaseId).catch((error) => {
      console.warn("Capture-surface cleanup after cancellation failed", error);
    });
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
  if (!state.session) return;
  try {
    const { session } = await api(`/api/sessions/${encodeURIComponent(state.session.id)}`);
    state.session = session;
    renderSession();
    const activeRun = session.runs?.find((run) => ["waiting_for_bridge", "reasoning"].includes(run.status));
    if (activeRun?.status === "waiting_for_bridge") dispatch(activeRun);
    if (terminalStatuses.has(session.status)) {
      stopPolling();
      state.dispatchKey = null;
      await releaseCaptureSurface(session.id).catch((error) => {
        console.warn("Capture-surface cleanup after session completion failed", error);
      });
      await refreshTimeline();
      showSessionOutcome(session);
      if (["failed", "partial"].includes(session.status)) showSessionFailure(session);
      state.session = null;
      renderSession();
      if (["completed", "partial"].includes(session.status)) {
        const calibration = await startPendingFirstCalibration(session);
        if (calibration) showCalibration(calibration);
      }
    }
  } catch (error) {
    if (/not found/i.test(error.message)) {
      stopPolling();
      state.session = null;
      renderSession();
      await refreshTimeline();
      return;
    }
    showError(error);
  }
}

async function releaseCaptureSurface(leaseId) {
  let lastError = null;
  for (let attempt = 1; attempt <= 3; attempt += 1) {
    try {
      return await releaseCaptureSurfaceOnce(leaseId);
    } catch (error) {
      lastError = error;
      if (attempt < 3) await new Promise((resolve) => setTimeout(resolve, 250 * attempt));
    }
  }
  throw lastError ?? new Error("AkuBridge did not confirm capture-surface cleanup.");
}

function releaseCaptureSurfaceOnce(leaseId) {
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
      if (event.source !== window || event.origin !== endpoint || event.data?.leaseId !== leaseId) return;
      if (event.data.type === "AKU_BROWSER_CAPTURE_SURFACE_RELEASED") {
        if (event.data.outcome?.reason === "lease_mismatch") {
          finish(reject, new Error("AkuBridge rejected cleanup because the capture lease changed."));
          return;
        }
        finish(resolve, event.data.outcome ?? null);
      } else if (event.data.type === "AKU_BROWSER_CAPTURE_SURFACE_RELEASE_FAILED") {
        finish(reject, new Error(event.data.message || "AkuBridge capture-surface cleanup failed."));
      }
    }
    window.addEventListener("message", onReleaseResult);
    window.postMessage({ type: "AKU_BROWSER_RELEASE_CAPTURE_SURFACE", leaseId }, endpoint);
  });
}

function dispatch(run) {
  const key = `${run.id}:${run.stage}`;
  if (state.dispatchKey === key) return;
  state.dispatchKey = key;
  window.postMessage({
    type: "AKU_BROWSER_DISPATCH",
    endpoint,
    token: state.bootstrap.bridgeToken,
    runId: run.id,
  }, endpoint);
}

function renderSession() {
  const session = state.session;
  $("#processing-panel").classList.toggle("hidden", !session || terminalStatuses.has(session.status));
  syncRunButtons();
  if (!session || terminalStatuses.has(session.status)) return;
  const run = session.runs?.find((candidate) => !["queued", "completed", "failed", "cancelled"].includes(candidate.status))
    ?? session.runs?.find((candidate) => candidate.status === "queued");
  const ordinal = run?.ordinal ?? 0;
  const reasoning = run?.status === "reasoning";
  const width = reasoning ? 42 + ordinal * 45 : 14 + ordinal * 45;
  const source = run?.source === "linkedin" ? "LinkedIn" : "X";
  $("#progress-bar").style.width = `${Math.min(96, width)}%`;
  $("#progress-bar").parentElement.setAttribute("aria-valuenow", String(Math.min(96, width)));
  $("#processing-title").textContent = reasoning ? `Evaluating ${source} evidence` : `Reading ${source}`;
  $("#processing-detail").textContent = `${ordinal + 1} of ${session.runs?.length ?? 2} sources · ${humanize(run?.stage ?? session.status)}`;
}

function syncRunButtons() {
  const disabled = Boolean(state.session) || Boolean(state.bootstrap?.calibration?.active) || state.bootstrap?.onboarding?.status !== "completed" || !state.bootstrap?.bridge?.compatible;
  $("#timeline-runner-button").disabled = disabled;
  $("#done-button").disabled = disabled;
  $("#open-reset-learning").disabled = Boolean(state.session);
  $("#open-full-reset").disabled = Boolean(state.session);
}

async function startPendingFirstCalibration(session) {
  if (state.bootstrap?.calibration?.firstRunStatus !== "pending") return null;
  try {
    const { calibration } = await api("/api/calibration/sessions", {
      method: "POST",
      body: { unifiedSessionId: session.id, triggerKind: "first_run" },
    });
    if (calibration.status === "completed") {
      state.bootstrap.calibration = { ...state.bootstrap.calibration, firstRunStatus: "completed", active: null };
      return null;
    }
    state.bootstrap.calibration.active = calibration;
    return calibration;
  } catch (error) {
    showError(new Error(`Calibration could not start: ${error.message}`));
    return null;
  }
}

function showCalibration(calibration) {
  clearNotice();
  setPill("#sidecar-status", "AkuSidecar ready", "ok");
  state.calibration = calibration;
  state.bootstrap.calibration.active = calibration;
  const unresolved = calibration.samples.findIndex((sample) => !sample.label && !sample.issueCode);
  state.calibrationOrdinal = unresolved >= 0 ? unresolved : Math.max(0, calibration.samples.length - 1);
  $("#onboarding-panel").classList.add("hidden");
  $("#settings-panel").classList.add("hidden");
  $("#inbox-panel").classList.add("hidden");
  $("#timeline-panel").classList.add("hidden");
  $("#calibration-panel").classList.remove("hidden");
  document.querySelector(".view-switch")?.classList.add("hidden");
  $("#calibration-heading").focus();
  window.scrollTo({ top: 0, behavior: "auto" });
  renderCalibration();
}

function renderCalibration() {
  const calibration = state.calibration;
  if (!calibration) return;
  const total = calibration.samples.length;
  const resolved = calibration.samples.filter((sample) => sample.label || sample.issueCode).length;
  const sample = calibration.samples[state.calibrationOrdinal];
  const progress = total ? Math.round((resolved / total) * 100) : 0;
  $("#calibration-progress").textContent = `${resolved} of ${total}`;
  $("#calibration-progress-bar").style.width = `${progress}%`;
  $("#calibration-progress-bar").parentElement?.setAttribute("aria-valuenow", String(progress));
  $("#calibration-previous").disabled = state.calibrationOrdinal === 0;
  const labels = { more_like_this: "More like this", neutral: "Neutral", less_like_this: "Less like this" };
  $("#calibration-status").textContent = sample?.label
    ? `Current decision: ${labels[sample.label] || humanize(sample.label)}`
    : sample?.issueCode
      ? `Reported capture problem: ${humanize(sample.issueCode)}`
      : "Choose Less, Neutral, or More for this real source entry.";
  $("#calibration-card").replaceChildren(buildCalibrationCard(sample));
}

function buildCalibrationCard(sample) {
  const card = document.createElement("article");
  card.className = "calibration-entry";
  if (!sample) {
    card.textContent = "No calibration entry is available.";
    return card;
  }
  const candidate = sample.candidate ?? {};
  const header = document.createElement("div");
  header.className = "calibration-entry-header";
  const source = document.createElement("strong");
  source.textContent = sourceLabel(sample.source);
  const position = document.createElement("span");
  position.textContent = `Source position ${candidate.feedPosition || state.calibrationOrdinal + 1}`;
  header.append(source, position);
  const sourceCard = buildSourceCard({
    source: sample.source,
    evidence: { ...candidate, permalink: candidate.sourceUrl },
    item: {
      source: sample.source,
      sourceUrl: candidate.sourceUrl,
      sourceUrlKind: "native_post",
      author: candidate.author,
      publishedAt: candidate.publishedAt,
      whatChanged: candidate.text || candidate.assessment?.rationale || "No readable text was captured.",
    },
  });
  card.append(header, sourceCard);
  if (candidate.sourceUrl) {
    const link = document.createElement("a");
    link.href = candidate.sourceUrl;
    link.target = "_blank";
    link.rel = "noopener noreferrer";
    link.textContent = "Open source entry";
    card.append(link);
  }
  return card;
}

function showPreviousCalibrationSample() {
  state.calibrationOrdinal = Math.max(0, state.calibrationOrdinal - 1);
  renderCalibration();
}

async function decideCalibration(decision) {
  if (!state.calibration) return;
  const buttons = [$("#calibration-less"), $("#calibration-neutral"), $("#calibration-more"), ...document.querySelectorAll("[data-calibration-issue]")];
  for (const button of buttons) button.disabled = true;
  try {
    const { calibration } = await api(
      `/api/calibration/sessions/${encodeURIComponent(state.calibration.id)}/samples/${state.calibrationOrdinal}`,
      { method: "PUT", body: decision },
    );
    state.calibration = calibration;
    if (calibration.status === "completed") {
      state.bootstrap.calibration = { ...state.bootstrap.calibration, firstRunStatus: "completed", active: null, liveInfluence: calibration.snapshot?.liveInfluence ?? false };
      state.calibration = null;
      $("#calibration-panel").classList.add("hidden");
      await refreshTimeline();
      setView("timeline");
      return;
    }
    state.bootstrap.calibration.active = calibration;
    const next = calibration.samples.findIndex((sample, index) => index > state.calibrationOrdinal && !sample.label && !sample.issueCode);
    state.calibrationOrdinal = next >= 0 ? next : Math.min(state.calibrationOrdinal + 1, calibration.samples.length - 1);
    renderCalibration();
  } catch (error) {
    $("#calibration-status").textContent = error.message;
  } finally {
    for (const button of buttons) button.disabled = false;
  }
}

async function refreshTimeline() {
  try {
    const limit = state.bootstrap?.settings?.timelineCapacity ?? 24;
    const { items, latestCheck } = await api(`/api/timeline?limit=${limit}&offset=0`);
    state.bootstrap.latestCheck = latestCheck ?? null;
    renderTimeline(items ?? [], latestCheck ?? null);
  } catch (error) {
    showError(error);
  }
}

async function loadInbox() {
  const meta = $("#inbox-meta");
  const refresh = $("#inbox-refresh-button");
  meta.textContent = "Loading recent checks...";
  refresh.disabled = true;
  try {
    const response = await api("/api/inbox?limit=12&offset=0");
    renderInbox(response.sessions ?? [], response.total ?? 0);
  } catch (error) {
    meta.textContent = error.message;
    $("#inbox-sessions").replaceChildren();
  } finally {
    refresh.disabled = false;
  }
}

function renderInbox(sessions, total) {
  const container = $("#inbox-sessions");
  $("#inbox-meta").textContent = total
    ? `${Math.min(sessions.length, total)} of ${total} recent checks \u00b7 newest first`
    : "No update checks have been recorded yet.";
  if (!sessions.length) {
    const empty = document.createElement("p");
    empty.className = "inbox-empty";
    empty.textContent = "Run Check for updates to create the first diagnostic entry.";
    container.replaceChildren(empty);
    return;
  }
  container.replaceChildren(...sessions.map((session, index) => buildInboxSession(session, index === 0)));
  scheduleBackToTop();
}

function buildInboxSession(session, expanded) {
  const details = document.createElement("details");
  details.className = "inbox-session";
  details.open = expanded;
  const summary = document.createElement("summary");
  const identity = document.createElement("div");
  identity.className = "inbox-session-identity";
  const title = document.createElement("strong");
  title.textContent = `Checked ${formatDate(session.createdAt)}`;
  const status = document.createElement("span");
  status.className = `status-pill status-${inboxStatusTone(session.status)}`;
  status.textContent = humanize(session.status);
  identity.append(title, status);
  const flow = document.createElement("span");
  flow.className = "inbox-flow-summary";
  flow.textContent = `${session.capturedCandidates} captured \u2192 ${session.evaluatedCandidates} evaluated \u2192 ${session.addedItems} unique${session.duplicateReports ? ` + ${session.duplicateReports} duplicate` : ""}`;
  summary.append(identity, flow);
  const body = document.createElement("div");
  body.className = "inbox-session-body";
  const duration = document.createElement("p");
  duration.className = "inbox-session-meta";
  duration.textContent = [formatDurationBetween(session.startedAt, session.completedAt), session.intent].filter(Boolean).join(" \u00b7 ");
  const runs = document.createElement("div");
  runs.className = "inbox-runs";
  runs.append(...(session.runs ?? []).map(buildInboxRun));
  body.append(duration);
  if (session.eventResolution) body.append(buildEventResolutionDiagnostic(session.eventResolution));
  body.append(runs);
  details.append(summary, body);
  return details;
}

function buildEventResolutionDiagnostic(value) {
  const diagnostic = document.createElement("section");
  diagnostic.className = `event-resolution-diagnostic event-resolution-${value.status}`;
  const title = document.createElement("strong");
  title.textContent = value.status === "failed" ? "Semantic event resolution degraded safely" : "Semantic event resolution";
  const detail = document.createElement("span");
  detail.textContent = value.status === "failed"
    ? `${value.error?.message || "Resolver unavailable"}. Reports remained unique.`
    : [
        `${value.uniqueItems} unique`,
        `${value.duplicateReports} duplicate reports`,
        `${value.shortlistCount} shortlisted threads`,
        value.provider === "local-index" ? "local index only" : `${value.provider}${value.durationMs ? ` \u00b7 ${formatDuration(value.durationMs)}` : ""}`,
        value.usage?.inputTokens != null ? `${value.usage.inputTokens} input / ${value.usage.outputTokens ?? 0} output tokens` : null,
      ].filter(Boolean).join(" \u00b7 ");
  const trigger = document.createElement("span");
  trigger.className = "event-resolution-trigger";
  const hasTriggerDiagnostics = Boolean(value.triggerReason);
  const triggerLabel = !hasTriggerDiagnostics ? "Legacy run" : value.resolverInvoked ? "Resolver invoked" : "Local fast path";
  const triggerTokens = value.triggerTokens?.length ? ` \u00b7 ${value.triggerTokens.join(", ")}` : "";
  const triggerReason = hasTriggerDiagnostics ? humanize(value.triggerReason) : "trigger diagnostics unavailable";
  trigger.textContent = `${triggerLabel}: ${triggerReason} \u00b7 ${value.historicalEventCount ?? 0} retained events \u00b7 strongest overlap ${value.strongestOverlap ?? 0}${triggerTokens}`;
  diagnostic.append(title, detail, trigger);
  return diagnostic;
}

function buildInboxRun(run) {
  const card = document.createElement("article");
  card.className = "inbox-run-card";
  const header = document.createElement("header");
  const source = document.createElement("strong");
  source.textContent = sourceLabel(run.source);
  const stage = document.createElement("span");
  stage.className = `status-pill status-${inboxStatusTone(run.status)}`;
  stage.textContent = run.status === "completed" ? "Completed" : `${humanize(run.status)} \u00b7 ${humanize(run.stage)}`;
  header.append(source, stage);
  const pipeline = document.createElement("div");
  pipeline.className = "inbox-pipeline";
  for (const [label, value] of [
    ["Captured", run.capturedCandidates],
    ["Evaluated", run.evaluatedCandidates],
    ["Selected", run.selectedCandidates],
    ["Added", run.addedItems],
  ]) {
    const metric = document.createElement("div");
    const number = document.createElement("strong");
    number.textContent = String(value ?? 0);
    const name = document.createElement("span");
    name.textContent = label;
    metric.append(number, name);
    pipeline.append(metric);
  }
  const mechanics = document.createElement("p");
  mechanics.className = "inbox-run-mechanics";
  mechanics.textContent = [
    `${run.acquisitionRounds ?? 0} capture round${run.acquisitionRounds === 1 ? "" : "s"}`,
    `${run.snapshotCount ?? 0} snapshots`,
    `${run.performedScrolls ?? 0} scrolls`,
    run.reasoningDurationMs ? `${formatDuration(run.reasoningDurationMs)} model time` : null,
  ].filter(Boolean).join(" \u00b7 ");
  card.append(header, pipeline, mechanics);
  if (run.error) {
    const failure = document.createElement("p");
    failure.className = "inbox-run-error";
    failure.textContent = `Stopped at ${humanize(run.error.stage || run.stage)}: ${run.error.message}`;
    card.append(failure);
  } else if (run.followUpFallback) {
    const fallback = document.createElement("p");
    fallback.className = "inbox-run-warning";
    fallback.textContent = `Completed from the initial capture after optional follow-up failed: ${run.followUpFallback.message}`;
    card.append(fallback);
  }
  if (!run.error && run.summary) {
    const summary = document.createElement("p");
    summary.className = "inbox-run-summary";
    summary.textContent = run.summary;
    card.append(summary);
  }
  return card;
}

function inboxStatusTone(status) {
  if (status === "completed") return "ok";
  if (["failed", "cancelled"].includes(status)) return "danger";
  if (status === "partial") return "warning";
  return "neutral";
}

function renderTimeline(items, latestCheck) {
  const container = $("#result-items");
  container.replaceChildren();
  if (latestCheck) {
    const unique = latestCheck.addedItems ?? 0;
    const duplicates = latestCheck.duplicateReports ?? 0;
    const parts = [unique ? `${unique} new item${unique === 1 ? "" : "s"}` : "No new items"];
    if (duplicates) parts.push(`${duplicates} duplicate report${duplicates === 1 ? "" : "s"}`);
    $("#timeline-meta").textContent = `${parts.join(" \u00b7 ")} from the latest check`;
  } else {
    $("#timeline-meta").textContent = "No completed check yet.";
  }
  $("#finish-stats").textContent = items.length
    ? `Shown: ${items.length} · bounded local evidence · personalized across sources`
    : "Check active sources to establish the finite timeline.";
  if (!items.length) {
    const empty = document.createElement("section");
    empty.className = "finish-line";
    const title = document.createElement("h3");
    title.textContent = "No retained updates yet";
    const detail = document.createElement("p");
    detail.textContent = "AkuBrowser will place evaluated, source-backed items here after the next bounded check.";
    empty.append(title, detail);
    container.append(empty);
    scheduleBackToTop();
    return;
  }

  const latestSession = latestCheck?.sessionId ?? items[0].sessionId;
  let previousSession = null;
  let historyBoundaryMarked = false;
  for (const entry of items) {
    if (entry.sessionId !== previousSession) {
      const marker = document.createElement("div");
      marker.className = "timeline-batch-marker";
      if (previousSession !== null) {
        marker.classList.add("timeline-older-batch-marker");
      }
      if (!historyBoundaryMarked && previousSession === latestSession && entry.sessionId !== latestSession) {
        marker.classList.add("timeline-history-boundary");
        marker.setAttribute("role", "separator");
        marker.setAttribute("aria-label", "Earlier retained updates");
        historyBoundaryMarked = true;
      }
      const checked = document.createElement("strong");
      const checkedAt = entry.sessionId === latestCheck?.sessionId ? latestCheck.completedAt : entry.createdAt;
      checked.textContent = `Checked ${formatDate(checkedAt)}`;
      const detail = document.createElement("span");
      detail.textContent = "Unified personalized order";
      marker.append(checked, detail);
      container.append(marker);
      previousSession = entry.sessionId;
    }
    container.append(buildTimelineItem(entry));
  }
  scheduleBackToTop();
}

function buildTimelineItem(entry) {
  if (entry.semanticEvent?.relation === "duplicate_report") return buildCollapsedDuplicate(entry);
  return buildExpandedTimelineItem(entry);
}

function buildCollapsedDuplicate(entry) {
  const container = document.createElement("article");
  container.className = "presentable-item semantic-duplicate-item";
  const summary = document.createElement("div");
  summary.className = "semantic-duplicate-summary";
  const copy = document.createElement("div");
  const label = document.createElement("span");
  label.className = "semantic-duplicate-label";
  label.textContent = "Cross-author semantic duplicate";
  const claim = document.createElement("strong");
  claim.textContent = entry.semanticEvent.canonicalClaim || entry.item?.whatChanged || "Same reported event";
  const provenance = document.createElement("small");
  provenance.textContent = [sourceLabel(entry.source), entry.item?.author, `${entry.semanticEvent.reportCount || 2} reports in this event`].filter(Boolean).join(" \u00b7 ");
  copy.append(label, claim, provenance);
  const toggle = document.createElement("button");
  toggle.type = "button";
  toggle.className = "secondary-button semantic-duplicate-toggle";
  toggle.textContent = "Show report";
  toggle.setAttribute("aria-expanded", "false");
  const report = buildExpandedTimelineItem(entry);
  report.classList.add("semantic-duplicate-report", "hidden");
  toggle.addEventListener("click", () => {
    const expanded = toggle.getAttribute("aria-expanded") === "true";
    toggle.setAttribute("aria-expanded", String(!expanded));
    toggle.textContent = expanded ? "Show report" : "Hide report";
    report.classList.toggle("hidden", expanded);
  });
  summary.append(copy, toggle);
  container.append(summary, report);
  return container;
}

function buildExpandedTimelineItem(entry) {
  const container = document.createElement("article");
  container.className = "presentable-item";
  const toolbar = document.createElement("div");
  toolbar.className = "item-presentation-toolbar";
  const label = document.createElement("span");
  const toggle = document.createElement("button");
  toggle.type = "button";
  toggle.className = "presentation-toggle";
  const brief = buildBrief(entry);
  const source = buildSourceCard(entry);
  const actions = buildActions(entry);
  let layout = entry.evidence && state.bootstrap?.settings?.defaultPresentation !== "brief" ? "source" : "brief";
  const render = () => {
    const sourceActive = layout === "source" && Boolean(entry.evidence);
    source.classList.toggle("hidden", !sourceActive);
    brief.classList.toggle("hidden", sourceActive);
    label.textContent = sourceActive ? "Source layout" : "Brief";
    toggle.textContent = sourceActive ? "Switch to brief" : "Switch to source layout";
    toggle.disabled = !entry.evidence;
  };
  toggle.addEventListener("click", () => {
    layout = layout === "source" ? "brief" : "source";
    render();
  });
  toolbar.append(label, toggle);
  container.append(toolbar, brief, source, actions);
  render();
  return container;
}

function buildBrief(entry) {
  const item = entry.item ?? {};
  const brief = document.createElement("div");
  brief.className = "item-layout-view";
  const badge = document.createElement("span");
  badge.className = "evidence-badge";
  badge.textContent = [humanize(item.evidenceState), `${Math.round((item.confidence ?? 0) * 100)}%`, humanize(item.knowledgeDelta)].filter(Boolean).join(" · ");
  const title = document.createElement("h3");
  title.textContent = item.whatChanged;
  const why = document.createElement("p");
  why.className = "why-it-matters";
  why.textContent = item.whyItMatters;
  const provenance = document.createElement("p");
  provenance.className = "provenance";
  provenance.textContent = [sourceLabel(entry.source), item.author, item.publishedAt ? formatDate(item.publishedAt) : null].filter(Boolean).join(" · ");
  brief.append(badge, title, why, provenance);
  return brief;
}

function buildSourceCard(entry) {
  const evidence = entry.evidence ?? {};
  const item = entry.item ?? {};
  const source = entry.source;
  const card = document.createElement("div");
  card.className = `source-layout-card source-${source}`;
  const header = document.createElement("header");
  const avatar = buildAvatar(evidence.avatarUrl, source, evidence.author || item.author);
  const identity = document.createElement("div");
  const author = document.createElement("strong");
  const parsed = sourceIdentity(evidence.author || item.author, source);
  author.textContent = parsed.displayName;
  const context = document.createElement("span");
  const presentation = evidence.presentation ?? {};
  context.textContent = source === "linkedin"
    ? [presentation.connectionDegree, presentation.timestampText].filter(Boolean).join(" · ") || formatDate(evidence.publishedAt || item.publishedAt)
    : parsed.secondary || formatDate(evidence.publishedAt || item.publishedAt);
  identity.append(author);
  if (presentation.headline) {
    const headline = document.createElement("span");
    headline.className = "source-layout-headline";
    headline.textContent = presentation.headline;
    identity.append(headline);
  }
  if (context.textContent) identity.append(context);
  header.append(avatar, identity);

  const content = document.createElement("div");
  content.className = `source-layout-content ${source === "linkedin" ? "linkedin-source-content" : "x-source-content"}`;
  if (presentation.socialContext) {
    const social = document.createElement("p");
    social.className = "x-social-context";
    social.textContent = presentation.socialContext;
    content.append(social);
  }
  content.append(buildExpandableText(evidence.text || item.whatChanged, {
    characterLimit: SOURCE_TEXT_COLLAPSE_CHARACTERS,
    lineLimit: SOURCE_TEXT_COLLAPSE_LINES,
    label: "post",
  }));
  const quote = buildQuotedPost(evidence.quotedPost, source);
  if (quote) content.append(quote);
  card.append(header, content);
  const media = buildMedia(evidence.media, source);
  if (media) card.append(media);
  if (evidence.mediaRecovery?.outcome === "unavailable") {
    const unavailable = document.createElement("div");
    unavailable.className = "source-layout-media-unavailable";
    const message = document.createElement("span");
    message.textContent = "Media was present at the source but unavailable in this captured view.";
    unavailable.append(message, buildSourceLink(entry));
    card.append(unavailable);
  }
  const engagement = buildEngagement(evidence.engagement, source);
  if (engagement) card.append(engagement);
  return card;
}

function buildAvatar(url, source, author) {
  if (safeMediaUrl(url)) {
    const image = document.createElement("img");
    image.className = "source-layout-avatar";
    image.src = url;
    image.alt = "";
    image.loading = "lazy";
    image.referrerPolicy = "no-referrer";
    image.addEventListener("error", () => image.replaceWith(buildAvatar(null, source, author)), { once: true });
    return image;
  }
  const fallback = document.createElement("span");
  fallback.className = "source-layout-badge";
  fallback.textContent = source === "x" ? "X" : initials(author);
  return fallback;
}

function buildQuotedPost(value, source) {
  if (!value?.text) return null;
  const quote = document.createElement("section");
  quote.className = "x-quote-card";
  const header = document.createElement("header");
  const avatar = buildAvatar(value.avatarUrl, source, value.author);
  const identity = document.createElement("div");
  const author = document.createElement("strong");
  author.textContent = value.author || "Quoted source";
  const timestamp = document.createElement("span");
  timestamp.textContent = formatDate(value.publishedAt);
  identity.append(author, timestamp);
  header.append(avatar, identity);
  quote.append(header, buildExpandableText(value.text, {
    characterLimit: QUOTE_TEXT_COLLAPSE_CHARACTERS,
    lineLimit: QUOTE_TEXT_COLLAPSE_LINES,
    label: "quoted post",
  }));
  return quote;
}

function buildExpandableText(value, { characterLimit, lineLimit, label }) {
  const wrapper = document.createElement("div");
  wrapper.className = "expandable-text";
  const text = document.createElement("p");
  text.className = "expandable-text-copy";
  text.textContent = value || "";
  wrapper.append(text);

  const logicalLines = text.textContent.split(/\r?\n/).length;
  if (text.textContent.length <= characterLimit && logicalLines <= lineLimit) return wrapper;

  text.classList.add("is-collapsed");
  text.style.setProperty("--collapse-lines", String(lineLimit));
  const toggle = document.createElement("button");
  toggle.type = "button";
  toggle.className = "content-expander";
  toggle.textContent = "Show more";
  toggle.setAttribute("aria-expanded", "false");
  toggle.setAttribute("aria-label", `Show more ${label} text`);
  toggle.addEventListener("click", () => {
    const expanded = toggle.getAttribute("aria-expanded") === "true";
    text.classList.toggle("is-collapsed", expanded);
    toggle.setAttribute("aria-expanded", String(!expanded));
    toggle.setAttribute("aria-label", `${expanded ? "Show more" : "Show less"} ${label} text`);
    toggle.textContent = expanded ? "Show more" : "Show less";
  });
  wrapper.append(toggle);
  return wrapper;
}

function buildMedia(values, source) {
  const media = (Array.isArray(values) ? values : [])
    .map((value) => ({ ...value, displayUrl: safeMediaUrl(value.posterUrl || value.url) }))
    .filter((value) => value.displayUrl)
    .slice(0, 4);
  if (!media.length) return null;
  const gallery = document.createElement("div");
  gallery.className = `source-layout-media media-count-${media.length}`;
  for (const value of media) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "source-layout-media-item";
    const image = document.createElement("img");
    image.src = value.displayUrl;
    image.alt = value.alt || `${sourceLabel(source)} post media`;
    image.loading = "lazy";
    image.referrerPolicy = "no-referrer";
    button.append(image);
    button.addEventListener("click", () => openMedia(media.map((entry) => entry.displayUrl), media.indexOf(value)));
    gallery.append(button);
  }
  return gallery;
}

function buildEngagement(value, source) {
  if (!value || typeof value !== "object") return null;
  const definitions = source === "x"
    ? [["reply", "○"], ["repost", "↻"], ["like", "♡"], ["view", "▥"]]
    : [["like", "👍"], ["comment", "💬"], ["repost", "↻"]];
  const available = definitions.filter(([key]) => value[key]);
  if (!available.length) return null;
  const footer = document.createElement("footer");
  footer.className = "source-layout-engagement";
  for (const [key, icon] of available) {
    const metric = document.createElement("span");
    metric.textContent = `${icon} ${value[key]}`;
    footer.append(metric);
  }
  return footer;
}

function buildActions(entry) {
  const actions = document.createElement("div");
  actions.className = "result-actions";
  const link = buildSourceLink(entry);
  const feedback = document.createElement("div");
  feedback.className = "feedback-actions";
  const more = feedbackButton("More like this");
  const less = feedbackButton("Less like this");
  const reason = document.createElement("select");
  reason.className = "feedback-reason hidden";
  reason.setAttribute("aria-label", "Less-like-this reason");
  for (const [value, label] of [["", "Optional reason"], ["not_interested", "Not interested"], ["already_knew", "Already knew"], ["old_info", "Old info"], ["duplicate", "Duplicate"]]) {
    const option = document.createElement("option");
    option.value = value;
    option.textContent = label;
    reason.append(option);
  }
  more.addEventListener("click", async () => {
    await sendFeedback(entry.id, "more", null);
    more.classList.add("selected");
    less.classList.remove("selected");
    reason.classList.add("hidden");
  });
  less.addEventListener("click", async () => {
    await sendFeedback(entry.id, "less", reason.value || null);
    less.classList.add("selected");
    more.classList.remove("selected");
    reason.classList.remove("hidden");
  });
  reason.addEventListener("change", () => {
    if (reason.value) sendFeedback(entry.id, "less", reason.value);
  });
  feedback.append(more, less, reason);
  actions.append(link, feedback);
  if (entry.semanticEvent) actions.append(buildSemanticCorrectionActions(entry));
  return actions;
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
    : entry.semanticEvent.relation === "duplicate_report" ? "Not the same event" : "Same event…";
  button.disabled = entry.semanticEvent.corrected && !entry.semanticEvent.correctionId;
  button.addEventListener("click", async () => {
    button.disabled = true;
    try {
      if (entry.semanticEvent.corrected) {
        await api(`/api/event-corrections/${encodeURIComponent(entry.semanticEvent.correctionId)}/undo`, { method: "POST" });
        await refreshTimeline();
        clearNotice();
      } else if (entry.semanticEvent.relation === "duplicate_report") {
        await applySemanticCorrection(entry.id, "not_same_event", "");
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
  if (!suggestions.length) {
    const message = document.createElement("span");
    message.className = "event-suggestion-editor";
    message.textContent = "No plausible retained event thread found.";
    controls.append(message);
    return;
  }
  const editor = document.createElement("span");
  editor.className = "event-suggestion-editor";
  const select = document.createElement("select");
  select.setAttribute("aria-label", "Choose the same semantic event");
  for (const suggestion of suggestions) {
    const option = document.createElement("option");
    option.value = suggestion.eventId;
    option.textContent = `${suggestion.canonicalClaim} (${suggestion.reportCount} reports)`;
    select.append(option);
  }
  const confirm = document.createElement("button");
  confirm.type = "button";
  confirm.textContent = "Confirm";
  confirm.addEventListener("click", async () => {
    confirm.disabled = true;
    trigger.disabled = true;
    try {
      await applySemanticCorrection(entry.id, "same_event", select.value);
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
  editor.append(select, confirm, cancel);
  controls.append(editor);
}

async function applySemanticCorrection(timelineId, action, targetEventId) {
  const { correction } = await api(`/api/timeline/${encodeURIComponent(timelineId)}/event-correction`, {
    method: "POST",
    body: { action, targetEventId },
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
  message.textContent = correction.action === "not_same_event" ? "Event split applied." : "Reports merged into the selected event.";
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
  notice.append(message, undo);
}

function buildSourceLink(entry) {
  const link = document.createElement("a");
  link.className = "source-link";
  link.href = entry.item?.sourceUrl || entry.evidence?.permalink || "#";
  link.target = "_blank";
  link.rel = "noopener noreferrer";
  link.textContent = entry.item?.sourceUrlKind === "native_post" ? "Open native post" : "Open source evidence";
  return link;
}

function feedbackButton(label) {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "feedback-button";
  button.textContent = label;
  return button;
}

async function sendFeedback(id, direction, reason) {
  try {
    await api(`/api/timeline/${encodeURIComponent(id)}/feedback`, {
      method: "POST",
      body: { direction, reason },
    });
  } catch (error) {
    showError(error);
  }
}

function openMedia(media, index) {
  state.media = media;
  state.mediaIndex = index;
  renderMedia();
  $("#media-viewer").showModal();
}

function moveMedia(delta) {
  if (!state.media.length) return;
  state.mediaIndex = (state.mediaIndex + delta + state.media.length) % state.media.length;
  renderMedia();
}

function renderMedia() {
  $("#media-viewer-image").src = state.media[state.mediaIndex] ?? "";
  $("#media-viewer-count").textContent = `${state.mediaIndex + 1} of ${state.media.length}`;
  $("#media-viewer-previous").disabled = state.media.length < 2;
  $("#media-viewer-next").disabled = state.media.length < 2;
}

function showSessionFailure(session) {
  const failedRuns = (session.runs ?? []).filter((run) => ["failed", "cancelled"].includes(run.status));
  const failedSources = failedRuns.map((run) => sourceLabel(run.source)).join(" and ");
  if (session.status === "partial") {
    $("#failure-message").textContent = `${failedSources || "One source"} could not finish. AkuBrowser retained and ordered the validated result from the source that completed.`;
  } else {
    $("#failure-message").textContent = session.error?.message
      || failedRuns[0]?.error?.message
      || "The session stopped before an active source could finish.";
  }
  $("#failure-panel").classList.remove("hidden");
}

function showSessionOutcome(session) {
  if (!["completed", "partial"].includes(session.status)) return;
  const additions = session.items?.length ?? 0;
  if (additions > 0) return;
  const notice = $("#provider-notice");
  notice.className = "notice notice-complete";
  notice.setAttribute("role", "status");
  notice.textContent = "Update complete: 0 additions. No captured candidate cleared the new, material, and trusted-evidence boundary.";
}

function hideFailure() {
  $("#failure-panel").classList.add("hidden");
}

function setPill(selector, text, tone) {
  const node = $(selector);
  node.textContent = text;
  node.className = `status-pill status-${tone}`;
}

function showError(error) {
  console.error(error);
  const notice = $("#provider-notice");
  notice.className = "notice notice-danger";
  notice.setAttribute("role", "alert");
  notice.textContent = error.message || String(error);
  setPill("#sidecar-status", "AkuSidecar attention", "danger");
}

function clearNotice() {
  const notice = $("#provider-notice");
  notice.className = "notice hidden";
  notice.setAttribute("role", "status");
  notice.textContent = "";
}

function sourceIdentity(value, source) {
  const text = String(value ?? "").trim();
  if (!text) return { displayName: sourceLabel(source), secondary: "" };
  if (source !== "x") return { displayName: text, secondary: "" };
  const match = text.match(/^(.+?)\s+(@[A-Za-z0-9_]+)(?:\s+[·•]\s+(.+))?$/);
  return {
    displayName: match?.[1]?.trim() || text,
    secondary: match ? [match[2], match[3]].filter(Boolean).join(" · ") : "",
  };
}

function initials(value) {
  const letters = String(value ?? "LI").trim().split(/\s+/).slice(0, 2).map((part) => part[0]?.toUpperCase()).join("");
  return letters || "LI";
}

function sourceLabel(source) {
  return source === "linkedin" ? "LinkedIn" : "X";
}

function humanize(value) {
  if (!value) return "";
  return String(value).replaceAll("_", " ").replace(/\b\w/g, (letter) => letter.toUpperCase());
}

function formatDate(value) {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.valueOf())) return "";
  return new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(date);
}

function formatDurationBetween(startedAt, completedAt) {
  if (!startedAt || !completedAt) return "";
  const duration = new Date(completedAt).valueOf() - new Date(startedAt).valueOf();
  return Number.isFinite(duration) && duration >= 0 ? formatDuration(duration) : "";
}

function formatDuration(milliseconds) {
  const seconds = Math.max(0, Math.round(Number(milliseconds) / 1000));
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const remainder = seconds % 60;
  return remainder ? `${minutes}m ${remainder}s` : `${minutes}m`;
}

function safeMediaUrl(value) {
  if (typeof value !== "string") return null;
  try {
    const url = new URL(value);
    return url.protocol === "https:" ? url.href : null;
  } catch {
    return null;
  }
}

async function api(path, options = {}) {
  const init = { method: options.method || "GET", cache: "no-store", headers: {} };
  if (options.body !== undefined) {
    init.headers["Content-Type"] = "application/json";
    init.body = JSON.stringify(options.body);
  }
  const response = await fetch(path, init);
  const payload = response.status === 204 ? null : await response.json();
  if (!response.ok) throw new Error(payload?.message || `HTTP ${response.status}`);
  return payload;
}

async function bridgeApi(path, options = {}) {
  const init = {
    method: options.method || "GET",
    cache: "no-store",
    headers: {
      "X-Aku-Bridge-Token": state.bootstrap.bridgeToken,
      "X-Aku-Bridge-Id": "aku-browser-page",
      "X-Aku-Bridge-Contract": state.bootstrap.bridgeContractVersion,
    },
  };
  if (options.body !== undefined) {
    init.headers["Content-Type"] = "application/json";
    init.body = JSON.stringify(options.body);
  }
  const response = await fetch(path, init);
  if (response.status === 204) return null;
  const payload = await response.json();
  if (!response.ok) throw new Error(payload?.message || `HTTP ${response.status}`);
  return payload;
}

bootstrap();

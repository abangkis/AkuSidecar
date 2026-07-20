const endpoint = location.origin;
const defaultIntent = "What materially changed since my last check?";
const terminalStatuses = new Set(["completed", "partial", "failed", "cancelled"]);
const BACK_TO_TOP_THRESHOLD_PX = 480;
const SOURCE_TEXT_COLLAPSE_CHARACTERS = 420;
const SOURCE_TEXT_COLLAPSE_LINES = 6;
const QUOTE_TEXT_COLLAPSE_CHARACTERS = 280;
const QUOTE_TEXT_COLLAPSE_LINES = 4;
const DEFAULT_TIMELINE_BATCH_GAP_PX = 36;
const DEFAULT_TIMELINE_BOUNDARY_RETURN_MS = 350;
const DEFAULT_SEMANTIC_EVENT_MERGE_THRESHOLD = 0.92;
const AI_HIDE_CONFIRMATION_PHRASE = "HIDE STRONG AI SIGNALS";
const AI_DEEP_POLL_INTERVAL_MS = 5000;
const ONBOARDING_LEARNING_INTERVAL_MS = 7000;
const PASSIVE_MEDIA_LOOKUP_TIMEOUT_MS = 2500;
const PASSIVE_MEDIA_LOOKUP_COOLDOWN_MS = 10000;
const BRIDGE_CONTEXT_RECOVERY_KEY = "akuBridgeContextRecoveryAt";
const BRIDGE_TOKEN_RECOVERY_KEY = "akuBridgeTokenRecoveryAt";
const BRIDGE_CONTEXT_RECOVERY_WINDOW_MS = 30000;
const LOAD_PROFILE_PRESETS = {
  standard: { timelineCapacity: 12, maxItemsPerSource: 5, maxItemsTotal: 10, maxScrolls: 2 },
  expanded: { timelineCapacity: 24, maxItemsPerSource: 10, maxItemsTotal: 20, maxScrolls: 4 },
  stress: { timelineCapacity: 36, maxItemsPerSource: 15, maxItemsTotal: 30, maxScrolls: 6 },
};
const RELEASE_REASONING_DEFAULTS = Object.freeze({
  acquisition_planning: "luna_high",
  candidate_evaluation: "luna_xhigh",
  semantic_event_resolution: "luna_high",
  ai_deep_detection: "luna_high",
});
const state = {
  bootstrap: null,
  session: null,
  dispatchKey: null,
  dispatchRetryAfter: new Map(),
  poller: null,
  pollInFlight: false,
  sessionProgress: { sessionId: null, value: 0 },
  currentView: "timeline",
  inboxSubView: "checks",
  modelUsageHelpSequence: 0,
  media: [],
  mediaIndex: 0,
  onboardingEditing: false,
  calibration: null,
  calibrationOrdinal: 0,
  onboardingLearningIndex: 0,
  onboardingLearningTimer: null,
  onboardingLearningPaused: false,
  onboardingLearningUserPaused: false,
  resetOperation: null,
  backToTopFrame: null,
  backToTopLastScrollY: 0,
  backToTopBoundary: null,
  mediaRecaptureActive: false,
  foregroundRecaptureOffers: new Set(),
  pendingSettings: null,
  seenTimelineItems: new Set(),
  aiDeepPoller: null,
  sidePaneItems: [],
  sidePaneFrame: null,
  timelineItems: [],
  passiveMediaEnrichmentTimer: null,
  passiveMediaEnrichmentActive: false,
  passiveMediaEvidenceAttempts: new Map(),
  expandedTimelineText: new Set(),
};
const $ = (selector) => document.querySelector(selector);

window.addEventListener("message", (event) => {
  if (event.source !== window || event.origin !== endpoint || !event.data) return;
  if (event.data.type === "AKU_BROWSER_BRIDGE_READY") {
    bridgeApi("/api/bridge/heartbeat", {
      method: "POST",
      body: { capabilities: event.data.capabilities ?? {} },
    }).then(({ bridge }) => {
      sessionStorage.removeItem(BRIDGE_CONTEXT_RECOVERY_KEY);
      sessionStorage.removeItem(BRIDGE_TOKEN_RECOVERY_KEY);
      renderBridge(bridge);
    }).catch(showError);
  }
  if (event.data.type === "AKU_BROWSER_BRIDGE_ERROR") {
    if (recoverInvalidatedBridgeContext(event.data.message)) return;
    showError(new Error(event.data.message));
  }
  if (event.data.type === "AKU_BROWSER_DISPATCH_FAILED") {
    const runId = String(event.data.runId || "");
    if (recoverInvalidatedBridgeContext(event.data.message)) return;
    const expectedLaneWait = /No queued browser command was available/i.test(event.data.message || "");
    if (state.dispatchKey?.startsWith(`${runId}:`)) state.dispatchKey = null;
    state.dispatchRetryAfter.set(runId, Date.now() + (expectedLaneWait ? 1500 : 500));
    if (!expectedLaneWait) {
      console.warn("AkuBridge dispatch stopped; waiting for the authoritative Sidecar run outcome.", {
        runId,
        message: event.data.message,
      });
    }
  }
});

function recoverInvalidatedBridgeContext(message) {
  if (!/Extension context invalidated/i.test(String(message ?? ""))) return false;
  const now = Date.now();
  const lastAttempt = Number(sessionStorage.getItem(BRIDGE_CONTEXT_RECOVERY_KEY) || 0);
  if (Number.isFinite(lastAttempt) && now - lastAttempt < BRIDGE_CONTEXT_RECOVERY_WINDOW_MS) {
    showError(new Error("AkuBridge reloaded, but this page could not reconnect automatically. Refresh AkuBrowser once to continue the queued update."));
    return true;
  }
  sessionStorage.setItem(BRIDGE_CONTEXT_RECOVERY_KEY, String(now));
  location.reload();
  return true;
}

function recoverInvalidBridgeToken(code) {
  if (code !== "invalid_bridge_token") return false;
  const now = Date.now();
  const lastAttempt = Number(sessionStorage.getItem(BRIDGE_TOKEN_RECOVERY_KEY) || 0);
  if (Number.isFinite(lastAttempt) && now - lastAttempt < BRIDGE_CONTEXT_RECOVERY_WINDOW_MS) return false;
  sessionStorage.setItem(BRIDGE_TOKEN_RECOVERY_KEY, String(now));
  location.reload();
  return true;
}

$("#session-view-button").addEventListener("click", () => setView("timeline"));
$("#inbox-view-button").addEventListener("click", () => setView("inbox"));
$("#settings-view-button").addEventListener("click", () => setView("settings"));
$("#timeline-refresh-button").addEventListener("click", refreshTimeline);
$("#inbox-refresh-button").addEventListener("click", loadInbox);
$("#model-usage-back").addEventListener("click", () => setInboxSubView("checks"));
$("#model-usage-refresh").addEventListener("click", loadAggregateModelUsage);
$("#model-usage-window").addEventListener("change", loadAggregateModelUsage);
$("#timeline-runner-button").addEventListener("click", startSession);
$("#done-button").addEventListener("click", startSession);
$("#retry-button").addEventListener("click", () => {
  const action = $("#retry-button").dataset.action;
  hideFailure();
  setView("timeline");
  if (action === "retry-update") startSession();
});
$("#cancel-button").addEventListener("click", cancelSession);
$("#processing-inbox-button").addEventListener("click", () => setView("inbox"));
$("#processing-settings-button").addEventListener("click", () => setView("settings"));
$("#onboarding-learning-previous").addEventListener("click", () => moveOnboardingLearning(-1, true));
$("#onboarding-learning-next").addEventListener("click", () => moveOnboardingLearning(1, true));
$("#onboarding-learning-toggle").addEventListener("click", toggleOnboardingLearningPlayback);
for (const button of document.querySelectorAll("[data-onboarding-slide]")) {
  button.addEventListener("click", () => showOnboardingLearningSlide(Number(button.dataset.onboardingSlide), true));
}
$("#onboarding-learning-panel").addEventListener("pointerenter", () => pauseOnboardingLearning(true));
$("#onboarding-learning-panel").addEventListener("pointerleave", () => pauseOnboardingLearning(false));
$("#onboarding-learning-panel").addEventListener("focusin", () => pauseOnboardingLearning(true));
$("#onboarding-learning-panel").addEventListener("focusout", () => {
  setTimeout(() => pauseOnboardingLearning($("#onboarding-learning-panel").contains(document.activeElement)), 0);
});
$("#runtime-settings-form").addEventListener("submit", saveSettings);
$("#detect-reasoning-executable").addEventListener("click", detectReasoningExecutable);
$("#bounded-load-profile").addEventListener("change", () => syncLoadProfileSettings(true));
$("#semantic-event-mode").addEventListener("change", syncSemanticEventSettings);
$("#ai-detection-enabled").addEventListener("change", syncAIDetectionSettings);
$("#ai-detection-presentation").addEventListener("change", syncAIDetectionSettings);
$("#reset-semantic-event-merge-threshold").addEventListener("click", resetSemanticEventMergeThreshold);
$("#stream-width").addEventListener("change", () => applyStreamWidth($("#stream-width").value));
$("#timeline-batch-gap").addEventListener("input", () => applyTimelineBatchGap($("#timeline-batch-gap").value));
$("#reset-timeline-batch-gap").addEventListener("click", resetTimelineBatchGap);
$("#timeline-boundary-return-ms").addEventListener("input", () => applyTimelineBoundaryReturnDuration($("#timeline-boundary-return-ms").value));
$("#reset-timeline-boundary-return").addEventListener("click", resetTimelineBoundaryReturnDuration);
$("#edit-onboarding-profile").addEventListener("click", () => showOnboarding(true));
$("#onboarding-form").addEventListener("submit", saveOnboarding);
$("#onboarding-cancel").addEventListener("click", () => setView("settings"));
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
$("#timeline-side-pane-toggle").addEventListener("click", openTimelineSidePane);
$("#timeline-side-pane-close").addEventListener("click", closeTimelineSidePane);
$("#back-to-top").addEventListener("click", returnToTop);
$("#media-viewer-close").addEventListener("click", () => $("#media-viewer").close());
$("#media-viewer-previous").addEventListener("click", () => moveMedia(-1));
$("#media-viewer-next").addEventListener("click", () => moveMedia(1));
window.addEventListener("scroll", () => {
  scheduleBackToTop();
  scheduleTimelineSidePanePosition();
}, { passive: true });
window.addEventListener("resize", () => {
  scheduleBackToTop();
  scheduleTimelineSidePanePosition();
}, { passive: true });
const timelineSidePaneLayoutObserver = new ResizeObserver(scheduleTimelineSidePanePosition);
for (const element of [$(".timeline-heading-row"), $("#processing-panel"), $("#result-items")]) {
  if (element) timelineSidePaneLayoutObserver.observe(element);
}
document.addEventListener("visibilitychange", () => {
  syncOnboardingLearningTimer();
  if (document.visibilityState === "visible") schedulePassiveMediaEnrichment();
});

async function bootstrap() {
  try {
    clearNotice();
    state.bootstrap = await api("/api/bootstrap");
    state.session = state.bootstrap.activeSession;
    renderSourceControls();
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
  if (!timeline) closeTimelineSidePane();
  syncTimelineSidePaneVisibility();
  $("#session-view-button").classList.toggle("selected", timeline);
  $("#inbox-view-button").classList.toggle("selected", inbox);
  $("#settings-view-button").classList.toggle("selected", settings);
  ({ timeline: $("#timeline-heading"), inbox: $("#inbox-heading"), settings: $("#settings-heading") }[view])?.focus?.();
  if (inbox) {
    syncInboxSubView();
    if (state.inboxSubView === "usage") loadAggregateModelUsage();
    else loadInbox();
  }
  scheduleBackToTop();
}

function syncInboxSubView() {
  const usage = state.inboxSubView === "usage";
  $("#inbox-ledger-view").classList.toggle("hidden", usage);
  $("#model-usage-view").classList.toggle("hidden", !usage);
}

function setInboxSubView(view) {
  state.inboxSubView = view === "usage" ? "usage" : "checks";
  syncInboxSubView();
  if (state.inboxSubView === "usage") {
    loadAggregateModelUsage();
    $("#model-usage-heading")?.focus();
  } else {
    loadInbox();
    $("#inbox-heading")?.focus();
  }
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
  if (bridge?.compatible) schedulePassiveMediaEnrichment();
}

function renderSettings(settings) {
  if (!settings) return;
  renderReasoningProcesses(state.bootstrap?.reasoningProcesses ?? []);
  const reasoningRuntime = state.bootstrap?.reasoningRuntime;
  $("#reasoning-executable-label").textContent = reasoningRuntime?.label || "Inference executable";
  $("#reasoning-executable-path").value = settings.reasoningExecutablePath || reasoningRuntime?.executablePath || "";
  $("#reasoning-executable-path").disabled = reasoningRuntime?.editable === false;
  $("#detect-reasoning-executable").disabled = reasoningRuntime?.editable === false;
  $("#bounded-load-profile").value = settings.loadProfile;
  $("#capture-visibility-policy").value = settings.captureVisibility;
  $("#source-wait-mode").value = settings.sourceWaitMode || "progressive_wait";
  $("#preference-eligibility-mode").value = settings.preferenceEligibilityMode;
  $("#calibration-enabled").checked = settings.calibrationEnabled;
  $("#calibration-batch-size").value = settings.calibrationBatchSize;
  $("#show-learning-panel").checked = settings.showLearningPanel === true;
  $("#open-missing-source").checked = settings.openMissingSource;
  $("#timeline-capacity").value = settings.timelineCapacity;
  $("#max-items-per-source").value = settings.maxItemsPerSource;
  $("#max-scrolls").value = settings.maxScrolls;
  $("#default-presentation").value = settings.defaultPresentation || "source";
  $("#stream-width").value = settings.streamWidth || "social";
  $("#timeline-batch-gap").value = settings.timelineBatchGapPx || DEFAULT_TIMELINE_BATCH_GAP_PX;
  $("#timeline-boundary-follow").checked = settings.timelineBoundaryCueMode !== "static";
  $("#timeline-boundary-return-ms").value = settings.timelineBoundaryReturnMs || DEFAULT_TIMELINE_BOUNDARY_RETURN_MS;
  $("#semantic-event-mode").value = settings.semanticEventMode || "collapse";
  $("#semantic-event-shortlist").value = String(settings.semanticEventShortlist || 10);
  $("#semantic-event-merge-threshold").value = Number(settings.semanticEventMergeThreshold || DEFAULT_SEMANTIC_EVENT_MERGE_THRESHOLD).toFixed(2);
  $("#knowledge-retention-days").value = String(settings.knowledgeRetentionDays || 30);
  $("#knowledge-storage-limit").value = String(settings.knowledgeStorageLimitMb || 100);
  $("#ai-detection-enabled").checked = settings.aiDetectionEnabled !== false;
  $("#ai-detection-presentation").value = settings.aiDetectionPresentation || "drawer";
  $("#resurface-mode").value = settings.resurfaceMode || "smart";
  $("#resurface-cooldown-days").value = String(settings.resurfaceCooldownDays || 7);
  renderSourceSettingsValues(settings);
  for (const input of document.querySelectorAll("#settings-source-options input[type='checkbox']")) {
    input.checked = settings.activeSources?.includes(input.value) ?? false;
  }
  applyStreamWidth(settings.streamWidth || "social");
  applyTimelineBatchGap(settings.timelineBatchGapPx || DEFAULT_TIMELINE_BATCH_GAP_PX);
  applyTimelineBoundaryReturnDuration(settings.timelineBoundaryReturnMs || DEFAULT_TIMELINE_BOUNDARY_RETURN_MS);
  if (settings.timelineBoundaryCueMode === "static") releaseBackToTopBoundary();
  syncLoadProfileSettings(false);
  syncSemanticEventSettings();
  syncAIDetectionSettings();
}

function renderSourceSettingsValues(settings) {
  for (const source of sourceDescriptors()) {
    const defaultMS = source.hydrationTimeoutDefaultMs;
    const input = document.querySelector(`[data-source-hydration="${source.id}"]`);
    if (input && defaultMS) {
      input.value = String((settings.sourceHydrationTimeoutMs?.[source.id] || defaultMS) / 1000);
    }
  }
}

function renderReasoningProcesses(processes) {
  const host = $("#reasoning-processes");
  if (!host) return;
  host.replaceChildren();
  for (const process of processes) {
    const row = document.createElement("div");
    row.className = "settings-row reasoning-process-profile";
    const copy = document.createElement("span");
    copy.className = "settings-copy";
    const label = document.createElement("strong");
    label.textContent = process.label;
    const description = document.createElement("small");
    description.textContent = process.description;
    copy.append(label, description);

    const route = document.createElement("span");
    route.className = "reasoning-process-route";
    const provider = document.createElement("small");
    provider.className = "reasoning-process-provider";
    provider.textContent = formatReasoningProvider(process.provider);
    route.append(provider);
    if (process.options?.length) {
      const select = document.createElement("select");
      select.id = `reasoning-profile-${process.id.replaceAll("_", "-")}`;
      select.className = "reasoning-profile-select";
      select.dataset.processId = process.id;
      select.setAttribute("aria-label", `${process.label} model profile`);
      for (const available of process.options) {
        const option = document.createElement("option");
        option.value = available.id;
        option.textContent = available.label;
        select.append(option);
      }
      select.value = process.profileId;
      route.append(select);
    } else {
      const model = document.createElement("strong");
      model.textContent = formatReasoningModel(process.model);
      route.append(model);
    }
    const detail = document.createElement("small");
    detail.textContent = `${process.execution === "async" ? "Async" : "In run"} · ${formatReasoningModel(process.model)} · ${formatReasoningEffort(process.effort)} thinking`;
    route.append(detail);
    row.append(copy, route);
    host.append(row);
  }
}

function formatReasoningProvider(value) {
  if (value === "codex-app-server") return "Codex App Server";
  if (value === "deterministic") return "Local deterministic";
  return String(value || "Custom backend").replaceAll("-", " ");
}

function formatReasoningModel(value) {
  if (value === "gpt-5.6-luna") return "GPT-5.6 Luna";
  if (value === "local-deterministic") return "No model";
  return value || "Backend default";
}

function formatReasoningEffort(value) {
  if (value === "xhigh") return "XHigh";
  if (value === "none") return "No";
  const text = String(value || "default");
  return text.charAt(0).toUpperCase() + text.slice(1);
}

async function saveSettings(event) {
  event.preventDefault();
  const current = state.bootstrap.settings;
  const activeSources = [...document.querySelectorAll("#settings-source-options input[type='checkbox']:checked")].map((input) => input.value);
  if (!activeSources.length) {
    $("#runtime-settings-status").textContent = "Choose at least one active source.";
    return;
  }
  const loadProfile = $("#bounded-load-profile").value;
  const preset = LOAD_PROFILE_PRESETS[loadProfile];
  const perSource = Number.parseInt($("#max-items-per-source").value, 10);
  const sourceHydrationTimeoutMs = Object.fromEntries(
    [...document.querySelectorAll("[data-source-hydration]")].map((input) => [
      input.dataset.sourceHydration,
      Number.parseInt(input.value, 10) * 1000,
    ]),
  );
  const settings = {
    ...current,
    loadProfile,
    captureVisibility: $("#capture-visibility-policy").value,
    sourceWaitMode: $("#source-wait-mode").value,
    preferenceEligibilityMode: $("#preference-eligibility-mode").value,
    calibrationEnabled: $("#calibration-enabled").checked,
    calibrationBatchSize: Number.parseInt($("#calibration-batch-size").value, 10),
    showLearningPanel: $("#show-learning-panel").checked,
    openMissingSource: $("#open-missing-source").checked,
    activeSources,
    sourceHydrationTimeoutMs,
    timelineCapacity: Number.parseInt($("#timeline-capacity").value, 10),
    maxItemsPerSource: perSource,
    maxItemsTotal: preset?.maxItemsTotal ?? Math.min(30, Math.max(1, perSource * activeSources.length)),
    maxScrolls: Number.parseInt($("#max-scrolls").value, 10),
    defaultPresentation: $("#default-presentation").value,
    streamWidth: $("#stream-width").value,
    timelineBatchGapPx: Number.parseInt($("#timeline-batch-gap").value, 10),
    timelineBoundaryCueMode: $("#timeline-boundary-follow").checked ? "follow" : "static",
    timelineBoundaryReturnMs: Number.parseInt($("#timeline-boundary-return-ms").value, 10),
    semanticEventMode: $("#semantic-event-mode").value,
    semanticEventShortlist: Number.parseInt($("#semantic-event-shortlist").value, 10),
    semanticEventMergeThreshold: Number.parseFloat($("#semantic-event-merge-threshold").value),
    knowledgeRetentionDays: Number.parseInt($("#knowledge-retention-days").value, 10),
    knowledgeStorageLimitMb: Number.parseInt($("#knowledge-storage-limit").value, 10),
    aiDetectionEnabled: $("#ai-detection-enabled").checked,
    aiDetectionPresentation: $("#ai-detection-presentation").value,
    resurfaceMode: $("#resurface-mode").value,
    resurfaceCooldownDays: Number.parseInt($("#resurface-cooldown-days").value, 10),
    reasoningExecutablePath: $("#reasoning-executable-path").value.trim(),
    reasoningAcquisitionProfile: reasoningProfileValue("acquisition_planning", current.reasoningAcquisitionProfile),
    reasoningEvaluationProfile: reasoningProfileValue("candidate_evaluation", current.reasoningEvaluationProfile),
    reasoningSemanticProfile: reasoningProfileValue("semantic_event_resolution", current.reasoningSemanticProfile),
    reasoningAiDeepProfile: reasoningProfileValue("ai_deep_detection", current.reasoningAiDeepProfile),
  };
  if (settings.aiDetectionEnabled && settings.aiDetectionPresentation === "hide" && current.aiDetectionPresentation !== "hide") {
    state.pendingSettings = settings;
    openResetDialog("ai-hide");
    return;
  }
  await persistSettings(settings);
}

function reasoningProfileValue(processId, fallback) {
  return document.querySelector(`[data-process-id="${processId}"]`)?.value
    || fallback
    || RELEASE_REASONING_DEFAULTS[processId]
    || "luna_high";
}

async function persistSettings(settings, confirmationPhrase = "") {
  const status = $("#runtime-settings-status");
  status.textContent = "Saving…";
  try {
    const response = await api("/api/settings", { method: "PUT", body: { settings, confirmationPhrase } });
    state.bootstrap.settings = response.settings;
    state.bootstrap.reasoningRuntime = response.reasoningRuntime ?? state.bootstrap.reasoningRuntime;
    state.bootstrap.reasoningProcesses = response.reasoningProcesses ?? state.bootstrap.reasoningProcesses;
    renderSettings(response.settings);
    syncOnboardingLearning(shouldShowOnboardingLearning(state.session));
    status.textContent = `Saved · ${response.settings.maxScrolls} scrolls · ${response.settings.maxItemsPerSource} items/source`;
    await refreshTimeline();
    return response.settings;
  } catch (error) {
    status.textContent = error.message;
    showError(error);
    return null;
  }
}

async function detectReasoningExecutable() {
  const status = $("#runtime-settings-status");
  status.textContent = "Detecting reasoning runtime…";
  try {
    const response = await api("/api/reasoning/runtime/discover", { method: "POST" });
    const runtime = response.reasoningRuntime;
    state.bootstrap.reasoningRuntime = runtime;
    $("#reasoning-executable-label").textContent = runtime.label || "Inference executable";
    $("#reasoning-executable-path").value = runtime.executablePath || "";
    status.textContent = "Detected · save settings to use this executable";
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
  $("#semantic-event-merge-threshold").disabled = disabled;
  $("#reset-semantic-event-merge-threshold").disabled = disabled;
  $("#semantic-event-merge-threshold").closest(".settings-row")?.classList.toggle("settings-row-disabled", disabled);
}

function syncAIDetectionSettings() {
  const enabled = $("#ai-detection-enabled").checked;
  $("#ai-detection-presentation").disabled = !enabled;
  $("#ai-detection-presentation").closest(".settings-row")?.classList.toggle("settings-row-disabled", !enabled);
  const deepProfile = document.querySelector('[data-process-id="ai_deep_detection"]');
  if (deepProfile) deepProfile.disabled = !enabled;
  const hide = enabled && $("#ai-detection-presentation").value === "hide";
  $("#ai-hide-warning").classList.toggle("hidden", !hide);
  const active = state.bootstrap?.settings?.aiDetectionPresentation === "hide";
  $("#ai-hide-status").textContent = active
    ? "Hide is active. Reviewable posts remain stored locally."
    : "Hide requires typed confirmation when saved.";
}

function resetSemanticEventMergeThreshold() {
  $("#semantic-event-merge-threshold").value = DEFAULT_SEMANTIC_EVENT_MERGE_THRESHOLD.toFixed(2);
  $("#runtime-settings-status").textContent = "Default restored · save settings to keep it.";
}

function applyStreamWidth(value) {
  document.body.dataset.streamWidth = ["compact", "social", "comfortable", "wide"].includes(value) ? value : "social";
  scheduleBackToTop();
  scheduleTimelineSidePanePosition();
}

function scheduleTimelineSidePanePosition() {
  if (state.sidePaneFrame !== null) return;
  state.sidePaneFrame = window.requestAnimationFrame(() => {
    state.sidePaneFrame = null;
    syncTimelineSidePanePosition();
  });
}

function syncTimelineSidePanePosition() {
  const stream = document.querySelector(".timeline-heading-row");
  if (!stream) return;
  const rect = stream.getBoundingClientRect();
  const firstTimelineItem = document.querySelector("#result-items > [data-timeline-id]");
  const verticalAnchor = firstTimelineItem?.getBoundingClientRect() ?? rect;
  const attachmentInset = firstTimelineItem
    ? Number.parseFloat(window.getComputedStyle(firstTimelineItem).borderTopLeftRadius) || 0
    : 0;
  const attachmentTop = verticalAnchor.top + attachmentInset;
  const viewportPadding = 16;
  const minimumTop = 18;
  const availableWidth = Math.max(0, rect.left - viewportPadding);
  const paneWidth = Math.min(420, Math.max(280, availableWidth));
  const paneLeft = Math.max(viewportPadding, rect.left - paneWidth);
  const paneTop = Math.max(minimumTop, attachmentTop);
  const toggleLeft = Math.max(12, rect.left - 56);
  const toggle = $("#timeline-side-pane-toggle");
  const toggleHeight = toggle?.getBoundingClientRect().height || 72;
  const toggleHalfHeight = toggleHeight / 2;
  const toggleTop = Math.min(
    window.innerHeight - minimumTop - toggleHalfHeight,
    Math.max(minimumTop + toggleHalfHeight, attachmentTop + toggleHalfHeight),
  );
  document.documentElement.style.setProperty("--timeline-side-pane-left", `${Math.round(paneLeft)}px`);
  document.documentElement.style.setProperty("--timeline-side-pane-width", `${Math.round(paneWidth)}px`);
  document.documentElement.style.setProperty("--timeline-side-pane-top", `${Math.round(paneTop)}px`);
  document.documentElement.style.setProperty("--timeline-side-pane-toggle-left", `${Math.round(toggleLeft)}px`);
  document.documentElement.style.setProperty("--timeline-side-pane-toggle-top", `${Math.round(toggleTop)}px`);
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

function applyTimelineBoundaryReturnDuration(value) {
  const parsed = Number.parseInt(value, 10);
  const bounded = Number.isFinite(parsed) ? Math.min(1000, Math.max(100, parsed)) : DEFAULT_TIMELINE_BOUNDARY_RETURN_MS;
  document.documentElement.style.setProperty("--back-to-top-return-duration", `${bounded}ms`);
}

function resetTimelineBoundaryReturnDuration() {
  $("#timeline-boundary-return-ms").value = DEFAULT_TIMELINE_BOUNDARY_RETURN_MS;
  applyTimelineBoundaryReturnDuration(DEFAULT_TIMELINE_BOUNDARY_RETURN_MS);
  $("#runtime-settings-status").textContent = "Default restored · save settings to keep it.";
}

function showOnboarding(editing) {
  state.onboardingEditing = editing;
  const completingFirstOnboarding = state.bootstrap?.onboarding?.status !== "completed";
  const sources = completingFirstOnboarding
    ? sourceDescriptors().filter((source) => source.defaultActive).map((source) => source.id)
    : (state.bootstrap?.settings?.activeSources ?? sourceDescriptors().filter((source) => source.defaultActive).map((source) => source.id));
  for (const input of document.querySelectorAll("#onboarding-source-options input[type='checkbox']")) {
    input.checked = sources.includes(input.value);
  }
  $("#onboarding-cancel").classList.toggle("hidden", !editing);
  $("#onboarding-finish").textContent = editing ? "Save profile" : "Start calibrating";
  $("#onboarding-error").textContent = "";
  $("#settings-panel").classList.add("hidden");
  $("#inbox-panel").classList.add("hidden");
  $("#timeline-panel").classList.add("hidden");
  $("#calibration-panel").classList.add("hidden");
  $("#onboarding-panel").classList.remove("hidden");
  document.querySelector(".view-switch")?.classList.add("hidden");
  syncTimelineSidePaneVisibility();
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
    $("#runtime-settings-status").textContent = operation === "ai-hide"
      ? "Hide cannot be activated while an update is running."
      : "Reset is unavailable while an update is running.";
    if (operation === "ai-hide" && state.bootstrap?.settings) {
      state.pendingSettings = null;
      $("#ai-detection-presentation").value = state.bootstrap.settings.aiDetectionPresentation || "drawer";
      syncAIDetectionSettings();
    }
    return;
  }
  state.resetOperation = operation;
  const full = operation === "full";
  const aiHide = operation === "ai-hide";
  $("#confirmation-operation-label").textContent = aiHide ? "HIGH-RISK PRESENTATION POLICY" : "DESTRUCTIVE OPERATION";
  $("#reset-confirmation-title").textContent = aiHide ? "Hide strong AI-origin signals" : full ? "Full reset and onboard again" : "Reset learning";
  $("#reset-confirmation-impact").textContent = aiHide
    ? "AI detection can be wrong. Only direct evidence and Deep-confirmed strong signals will be hidden. Posts remain stored locally and can be restored by disabling Hide."
    : full
      ? "AkuBrowser will first create and verify a local SQLite backup, then erase Timeline, runs, learning data, onboarding, and local settings. The live Bridge identity remains valid."
      : "AkuBrowser will erase calibration, More/Less feedback, and the fitted preference model. Timeline, source setup, and runtime settings remain.";
  $("#reset-confirmation-phrase").textContent = aiHide ? AI_HIDE_CONFIRMATION_PHRASE : full ? "RESET AKUBROWSER" : "RESET LEARNING";
  $("#reset-confirmation-submit").textContent = aiHide ? "Activate Hide" : full ? "Confirm full reset" : "Confirm reset";
  $("#reset-confirmation-input").value = "";
  $("#reset-confirmation-status").textContent = "";
  $("#reset-confirmation-submit").disabled = true;
  $("#reset-confirmation-dialog").showModal();
  $("#reset-confirmation-input").focus();
}

function closeResetDialog() {
  if (state.resetOperation === "ai-hide" && state.bootstrap?.settings) {
    $("#ai-detection-presentation").value = state.bootstrap.settings.aiDetectionPresentation || "drawer";
    syncAIDetectionSettings();
  }
  state.resetOperation = null;
  state.pendingSettings = null;
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
    if (operation === "ai-hide") {
      $("#reset-confirmation-status").textContent = "Activating Hide…";
      const pending = state.pendingSettings;
      if (!pending) throw new Error("Pending AI Detector settings are unavailable.");
      const saved = await persistSettings(pending, phrase);
      if (!saved) {
        syncResetConfirmation();
        return;
      }
      state.resetOperation = null;
      state.pendingSettings = null;
      $("#reset-confirmation-dialog").close();
      $("#runtime-settings-status").textContent = "Hide activated · direct and Deep-confirmed signals only.";
      return;
    }
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
  if (!state.session || state.pollInFlight) return;
  const sessionId = state.session.id;
  state.pollInFlight = true;
  try {
    const { session } = await api(`/api/sessions/${encodeURIComponent(sessionId)}`);
    if (!state.session || state.session.id !== sessionId) return;
    state.session = session;
    renderSession();
    const captureActive = session.runs?.some((run) => run.status === "waiting_for_bridge" && run.bridgeCommandStatus === "claimed");
    const captureRun = session.runs?.find((run) => run.status === "waiting_for_bridge" && (!run.bridgeCommandStatus || run.bridgeCommandStatus === "queued"));
    if (!captureActive && captureRun) dispatch(captureRun);
    if (terminalStatuses.has(session.status)) {
      stopPolling();
      state.dispatchKey = null;
      state.dispatchRetryAfter.clear();
      await releaseCaptureSurface(session.id).catch((error) => {
        console.warn("Capture-surface cleanup after session completion failed", error);
      });
      await refreshTimeline();
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
  const session = state.session;
  $("#processing-panel").classList.toggle("hidden", !session || terminalStatuses.has(session.status));
  syncOnboardingLearning(shouldShowOnboardingLearning(session));
  syncRunButtons();
  if (!session || terminalStatuses.has(session.status)) return;
  const progress = describeSessionProgress(session);
  if (state.sessionProgress.sessionId !== session.id) {
    state.sessionProgress = { sessionId: session.id, value: 0 };
  }
  state.sessionProgress.value = Math.max(state.sessionProgress.value, progress.value);
  const value = Math.min(97, state.sessionProgress.value);
  $("#progress-bar").style.width = `${value}%`;
  $("#progress-bar").parentElement.setAttribute("aria-valuenow", String(Math.round(value)));
  $("#processing-title").textContent = progress.title;
  $("#processing-detail").textContent = progress.detail;
}

function shouldShowOnboardingLearning(session) {
  if (firstRunCalibrationPending()) {
    return Boolean(session && !terminalStatuses.has(session.status));
  }
  return state.bootstrap?.settings?.showLearningPanel === true;
}

function syncOnboardingLearning(visible) {
  const panel = $("#onboarding-learning-panel");
  panel.classList.toggle("hidden", !visible);
  if (!visible) {
    stopOnboardingLearningTimer();
    return;
  }
  renderOnboardingLearningSlide();
  syncOnboardingLearningTimer();
}

function moveOnboardingLearning(delta, manual = false) {
  const slides = document.querySelectorAll(".onboarding-learning-slide");
  if (!slides.length) return;
  showOnboardingLearningSlide((state.onboardingLearningIndex + delta + slides.length) % slides.length, manual);
}

function showOnboardingLearningSlide(index, manual = false) {
  const slides = document.querySelectorAll(".onboarding-learning-slide");
  if (!slides.length) return;
  state.onboardingLearningIndex = Math.max(0, Math.min(index, slides.length - 1));
  renderOnboardingLearningSlide();
  if (manual) restartOnboardingLearningTimer();
}

function renderOnboardingLearningSlide() {
  const slides = document.querySelectorAll(".onboarding-learning-slide");
  if (!slides.length) return;
  $("#onboarding-learning-track").style.transform = `translateX(-${state.onboardingLearningIndex * 100}%)`;
  $("#onboarding-learning-count").textContent = `${state.onboardingLearningIndex + 1} of ${slides.length}`;
  for (const [index, slide] of [...slides].entries()) slide.setAttribute("aria-hidden", String(index !== state.onboardingLearningIndex));
  for (const dot of document.querySelectorAll("[data-onboarding-slide]")) {
    const active = Number(dot.dataset.onboardingSlide) === state.onboardingLearningIndex;
    if (active) dot.setAttribute("aria-current", "true");
    else dot.removeAttribute("aria-current");
  }
}

function pauseOnboardingLearning(paused) {
  state.onboardingLearningPaused = paused;
  syncOnboardingLearningTimer();
}

function toggleOnboardingLearningPlayback() {
  state.onboardingLearningUserPaused = !state.onboardingLearningUserPaused;
  const button = $("#onboarding-learning-toggle");
  button.textContent = state.onboardingLearningUserPaused ? "Play" : "Pause";
  button.setAttribute("aria-pressed", String(state.onboardingLearningUserPaused));
  syncOnboardingLearningTimer();
}

function syncOnboardingLearningTimer() {
  const panelVisible = !$("#onboarding-learning-panel").classList.contains("hidden");
  if (!panelVisible || state.onboardingLearningPaused || state.onboardingLearningUserPaused || document.visibilityState !== "visible") {
    stopOnboardingLearningTimer();
    return;
  }
  if (state.onboardingLearningTimer) return;
  state.onboardingLearningTimer = setInterval(() => moveOnboardingLearning(1), ONBOARDING_LEARNING_INTERVAL_MS);
}

function restartOnboardingLearningTimer() {
  stopOnboardingLearningTimer();
  syncOnboardingLearningTimer();
}

function stopOnboardingLearningTimer() {
  if (!state.onboardingLearningTimer) return;
  clearInterval(state.onboardingLearningTimer);
  state.onboardingLearningTimer = null;
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

function syncRunButtons() {
  const reason = runDisabledReason();
  const disabled = Boolean(reason);
  $("#timeline-runner-button").disabled = disabled;
  $("#done-button").disabled = disabled;
  $("#timeline-runner-button").title = reason;
  $("#done-button").title = reason;
  $("#timeline-runner-status").textContent = reason;
  $("#timeline-runner-status").classList.toggle("hidden", !reason || Boolean(state.session && !terminalStatuses.has(state.session.status)));
  for (const button of document.querySelectorAll(".recapture-button")) button.disabled = disabled;
  $("#open-reset-learning").disabled = Boolean(state.session);
  $("#open-full-reset").disabled = Boolean(state.session);
}

function runDisabledReason() {
  if (state.session) {
    return terminalStatuses.has(state.session.status) ? "Finishing capture cleanup…" : "A check for updates is already running.";
  }
  if (state.mediaRecaptureActive) return "Finishing media recapture…";
  if (state.bootstrap?.calibration?.active) return "Finish calibration before starting another check.";
  if (state.bootstrap?.onboarding?.status !== "completed") return "Complete source setup before checking for updates.";
  if (!state.bootstrap?.bridge?.compatible) return "Waiting for AkuBridge to reconnect…";
  return "";
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
    if (error.code === "calibration_sample_unavailable" || /no validated calibration entry|requires at least one validated candidate/i.test(error.message)) {
      showCalibrationRetry(session);
      return null;
    }
    showError(new Error(`Calibration could not start: ${error.message}`));
    return null;
  }
}

function showCalibration(calibration) {
  clearNotice();
  hideFailure();
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
  syncTimelineSidePaneVisibility();
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
  const calibrationSourceUrl = safeSourceUrl(candidate.sourceUrl, sample.source);
  if (calibrationSourceUrl) {
    const link = document.createElement("a");
    link.href = calibrationSourceUrl;
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
      if (calibration.triggerKind === "first_run") state.bootstrap.settings.showLearningPanel = false;
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
    if (state.bootstrap) state.bootstrap.latestCheck = latestCheck ?? null;
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
  flow.textContent = inboxSessionFlowText(session);
  summary.append(identity, flow);
  const body = document.createElement("div");
  body.className = "inbox-session-body";
  const duration = document.createElement("p");
  duration.className = "inbox-session-meta";
  duration.textContent = [formatDurationBetween(session.startedAt, session.completedAt), session.intent].filter(Boolean).join(" \u00b7 ");
  const runs = document.createElement("div");
  runs.className = "inbox-runs";
  runs.append(...(session.runs ?? []).map((run) => buildInboxRun(run, session.status)));
  body.append(duration, buildSessionModelUsage(session));
  if (session.eventResolution) body.append(buildEventResolutionDiagnostic(session.eventResolution));
  if (session.aiDetection) body.append(buildAIDetectionDiagnostic(session.aiDetection));
  if (session.preferenceDecisions?.length) {
    body.append(buildInboxPreferenceDecisions(session.preferenceDecisions));
  }
  body.append(runs);
  details.append(summary, body);
  return details;
}

function buildSessionModelUsage(session) {
  const details = document.createElement("details");
  details.className = "model-usage-section";
  const summary = document.createElement("summary");
  const label = document.createElement("strong");
  label.textContent = "Model usage";
  const value = document.createElement("span");
  value.textContent = "Open to inspect this check";
  summary.append(label, value);
  const body = document.createElement("div");
  body.className = "model-usage-body";
  details.append(summary, body);
  let loaded = false;
  let loading = false;
  details.addEventListener("toggle", async () => {
    if (!details.open || loaded || loading) return;
    loading = true;
    body.replaceChildren(modelUsageMessage("Loading model usage..."));
    try {
      const response = await api(`/api/inbox/sessions/${encodeURIComponent(session.id)}/model-usage`);
      renderModelUsageReport(body, response.usage, true);
      value.textContent = modelUsageSummary(response.usage);
      loaded = true;
    } catch (error) {
      body.replaceChildren(modelUsageMessage(error.message, "error"));
    } finally {
      loading = false;
    }
  });
  return details;
}

function modelUsageMessage(message, tone = "") {
  const element = document.createElement("p");
  element.className = `model-usage-message${tone ? ` model-usage-message-${tone}` : ""}`;
  element.textContent = message;
  return element;
}

function buildModelUsageHelp() {
  const wrapper = document.createElement("span");
  wrapper.className = "model-usage-help";
  const button = document.createElement("button");
  button.type = "button";
  button.className = "model-usage-help-button";
  button.textContent = "?";
  button.title = "How model usage is counted";
  button.setAttribute("aria-label", "How model usage is counted");
  button.setAttribute("aria-expanded", "false");
  const popover = document.createElement("span");
  popover.className = "model-usage-help-popover";
  popover.id = `model-usage-help-${++state.modelUsageHelpSequence}`;
  popover.setAttribute("role", "tooltip");
  popover.textContent = "Input already includes cached input, so cached input is shown as a breakout and is not added again. Reasoning output is also a breakout. Failed invocations may still use tokens. Unavailable means the provider did not report usage, not zero. AI Deep Detection may update after the Timeline is published.";
  button.setAttribute("aria-controls", popover.id);
  button.setAttribute("aria-describedby", popover.id);
  button.addEventListener("click", () => {
    const open = !wrapper.classList.contains("is-open");
    wrapper.classList.toggle("is-open", open);
    button.setAttribute("aria-expanded", String(open));
  });
  wrapper.append(button, popover);
  return wrapper;
}

function renderModelUsageReport(container, report, showTotalLink) {
  const header = document.createElement("header");
  header.className = "model-usage-heading";
  const copy = document.createElement("div");
  const title = document.createElement("strong");
  title.textContent = report.scope === "aggregate" ? "Locally recorded usage" : "Usage for this check";
  const detail = document.createElement("span");
  detail.textContent = modelUsageSummary(report);
  copy.append(title, detail);
  header.append(copy, buildModelUsageHelp());

  const totals = document.createElement("div");
  totals.className = "model-usage-totals";
  for (const [label, value, note] of [
    ["Input", report.usage?.inputTokens, "Cached input is included"],
    ["Cached input", report.usage?.cachedInputTokens, "Breakout only"],
    ["Output", report.usage?.outputTokens, "Primary output counter"],
    ["Reasoning output", report.usage?.reasoningOutputTokens, "Breakout only"],
    ["Model time", report.durationMs, `${modelUsageInvocationCount(report)} invocation${modelUsageInvocationCount(report) === 1 ? "" : "s"}`],
  ]) {
    const metric = document.createElement("div");
    const number = document.createElement("strong");
    number.textContent = label === "Model time" ? formatDuration(value ?? 0) : formatTokenCount(value);
    const name = document.createElement("span");
    name.textContent = label;
    const context = document.createElement("small");
    context.textContent = note;
    metric.append(number, name, context);
    totals.append(metric);
  }

  const categories = document.createElement("div");
  categories.className = "model-usage-categories";
  categories.append(...(report.categories ?? []).map(buildModelUsageCategory));
  container.replaceChildren(header, totals, categories);

  if (report.usageCoverage === "partial" || report.usageCoverage === "unavailable") {
    const coverage = modelUsageMessage(report.usageCoverage === "partial"
      ? "Some invocations did not report every token counter. Displayed totals include only reported values."
      : "The provider did not report token counters for the recorded invocations.");
    coverage.classList.add("model-usage-coverage-note");
    container.append(coverage);
  }
  if (report.scope === "aggregate") {
    const retention = modelUsageMessage("This is local AkuBrowser history, not account-wide Codex usage. Database reset and retention or storage trimming can reduce the available history.");
    retention.classList.add("model-usage-retention-note");
    container.append(retention);
  }
  if (showTotalLink) {
    const link = document.createElement("button");
    link.type = "button";
    link.className = "text-button model-usage-total-link";
    link.textContent = "View total model usage →";
    link.addEventListener("click", () => setInboxSubView("usage"));
    container.append(link);
  }
}

function buildModelUsageCategory(category) {
  const details = document.createElement("details");
  details.className = "model-usage-category";
  const summary = document.createElement("summary");
  const identity = document.createElement("span");
  const label = document.createElement("strong");
  label.textContent = category.label;
  const execution = document.createElement("small");
  execution.textContent = category.execution === "async" ? "Async" : "In run";
  identity.append(label, execution);
  const rollup = document.createElement("span");
  rollup.textContent = category.invocationCount
    ? `${formatTokenCount(category.usage?.inputTokens)} input · ${formatTokenCount(category.usage?.outputTokens)} output · ${formatDuration(category.durationMs)}`
    : humanize(category.status);
  summary.append(identity, rollup);
  const body = document.createElement("div");
  body.className = "model-usage-category-body";
  if (category.note) body.append(modelUsageMessage(category.note));
  if (category.entries?.length) {
    const entries = document.createElement("div");
    entries.className = "model-usage-entries";
    entries.append(...category.entries.map(buildModelUsageEntry));
    body.append(entries);
  }
  details.append(summary, body);
  return details;
}

function buildModelUsageEntry(entry) {
  const row = document.createElement("article");
  row.className = "model-usage-entry";
  const identity = document.createElement("div");
  const title = document.createElement("strong");
  title.textContent = entry.source ? sourceLabel(entry.source) : "All sources";
  const runtime = document.createElement("span");
  runtime.textContent = [formatReasoningProvider(entry.provider), formatReasoningModel(entry.model), formatReasoningEffort(entry.effort), `${entry.invocationCount} invocation${entry.invocationCount === 1 ? "" : "s"}`].filter(Boolean).join(" · ");
  identity.append(title, runtime);
  const metrics = document.createElement("div");
  metrics.className = "model-usage-entry-metrics";
  for (const text of [
    `${formatTokenCount(entry.usage?.inputTokens)} in`,
    `${formatTokenCount(entry.usage?.cachedInputTokens)} cached`,
    `${formatTokenCount(entry.usage?.outputTokens)} out`,
    `${formatTokenCount(entry.usage?.reasoningOutputTokens)} reasoning`,
    formatDuration(entry.durationMs),
  ]) {
    const metric = document.createElement("span");
    metric.textContent = text;
    metrics.append(metric);
  }
  const status = document.createElement("span");
  status.className = `model-usage-entry-status status-${inboxStatusTone(entry.status)}`;
  status.textContent = entry.usageCoverage === "unavailable" ? `${humanize(entry.status)} · usage unavailable` : humanize(entry.status);
  row.append(identity, metrics, status);
  return row;
}

function modelUsageInvocationCount(report) {
  return (report.categories ?? []).reduce((total, category) => total + Number(category.invocationCount || 0), 0);
}

function modelUsageSummary(report) {
  const invocations = modelUsageInvocationCount(report);
  if (!invocations) return "No model invocation required";
  const summary = `${formatTokenCount(report.usage?.inputTokens)} input · ${formatTokenCount(report.usage?.outputTokens)} output · ${formatDuration(report.durationMs)} model time`;
  return report.usageCoverage === "complete" ? summary : `${summary} · ${humanize(report.usageCoverage)} telemetry`;
}

function formatTokenCount(value) {
  if (value == null || !Number.isFinite(Number(value))) return "Unavailable";
  return new Intl.NumberFormat(undefined, { notation: "compact", maximumFractionDigits: 1 }).format(Number(value));
}

async function loadAggregateModelUsage() {
  const meta = $("#model-usage-meta");
  const container = $("#model-usage-total");
  const refresh = $("#model-usage-refresh");
  const windowDays = Number($("#model-usage-window").value || 30);
  meta.textContent = `Loading ${windowDays} days of locally recorded usage...`;
  refresh.disabled = true;
  container.replaceChildren(modelUsageMessage("Loading model usage..."));
  try {
    const response = await api(`/api/model-usage?windowDays=${windowDays}`);
    const report = response.usage;
    meta.textContent = `${report.sessionCount} check${report.sessionCount === 1 ? "" : "s"} recorded in the last ${windowDays} days · generated ${formatDate(report.generatedAt)}`;
    renderModelUsageReport(container, report, false);
  } catch (error) {
    meta.textContent = error.message;
    container.replaceChildren(modelUsageMessage(error.message, "error"));
  } finally {
    refresh.disabled = false;
  }
}

function buildInboxPreferenceDecisions(decisions) {
  const section = document.createElement("details");
  section.className = "inbox-preference-decisions";
  const summary = document.createElement("summary");
  const title = document.createElement("strong");
  title.textContent = "Personalization decisions";
  const guidance = document.createElement("span");
  guidance.textContent = "Change an earlier choice. The latest More or Less decision is authoritative.";
  summary.append(title, guidance);
  const list = document.createElement("div");
  list.className = "inbox-preference-list";
  list.append(...decisions.map(buildInboxPreferenceDecision));
  section.append(summary, list);
  return section;
}

function buildInboxPreferenceDecision(decision) {
  const row = document.createElement("article");
  row.className = "inbox-preference-decision";
  const copy = document.createElement("div");
  copy.className = "inbox-preference-copy";
  const summary = document.createElement("strong");
  summary.textContent = decision.summary || "Previously rated update";
  const context = document.createElement("span");
  const renderContext = () => {
    const origin = decision.origin === "calibration" ? "from calibration" : null;
    context.textContent = [sourceLabel(decision.source), decision.author, origin, `updated ${formatDate(decision.updatedAt)}`]
      .filter(Boolean)
      .join(" \u00b7 ");
  };
  renderContext();
  copy.append(summary, context);
  const sourceUrl = safeSourceUrl(decision.sourceUrl, decision.source);
  if (sourceUrl) {
    const link = document.createElement("a");
    link.href = sourceUrl;
    link.target = "_blank";
    link.rel = "noopener noreferrer";
    link.textContent = "Open source";
    copy.append(link);
  }

  const actions = document.createElement("div");
  actions.className = "inbox-preference-actions";
  const more = feedbackButton("More");
  const less = feedbackButton("Less");
  const buttons = { more, less };
  const renderDirection = () => {
    for (const [direction, button] of Object.entries(buttons)) {
      const selected = decision.direction === direction;
      button.classList.toggle("selected", selected);
      button.setAttribute("aria-pressed", String(selected));
    }
  };
  for (const [direction, button] of Object.entries(buttons)) {
    button.addEventListener("click", async () => {
      if (decision.direction === direction) return;
      more.disabled = true;
      less.disabled = true;
      try {
        await api(`/api/timeline/${encodeURIComponent(decision.timelineId)}/feedback`, {
          method: "POST",
          body: { direction, reason: direction === "less" ? "not_interested" : null },
        });
        decision.direction = direction;
        decision.origin = "routine";
        decision.updatedAt = new Date().toISOString();
        renderContext();
        renderDirection();
      } catch (error) {
        showError(error);
      } finally {
        more.disabled = false;
        less.disabled = false;
      }
    });
  }
  renderDirection();
  actions.append(more, less);
  row.append(copy, actions);
  return row;
}

function buildAIDetectionDiagnostic(value) {
  const diagnostic = document.createElement("section");
  diagnostic.className = `ai-detection-diagnostic ai-detection-${value.status}`;
  const title = document.createElement("strong");
  title.textContent = value.status === "failed" ? "AI Deep Detection degraded safely" : "AI Deep Detection";
  const detail = document.createElement("span");
  detail.textContent = value.status === "failed"
    ? `${value.error || "Deep Detection unavailable"}. Timeline kept the local Fast Detection result.`
    : [
        humanize(value.status),
        `${value.candidateCount ?? 0} reviewed posts`,
        value.durationMs ? formatDuration(value.durationMs) : null,
      ].filter(Boolean).join(" \u00b7 ");
  diagnostic.append(title, detail);
  return diagnostic;
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
        value.userSplitCorrections ? `${value.userSplitCorrections} user split${value.userSplitCorrections === 1 ? "" : "s"}` : null,
        value.userMergeCorrections ? `${value.userMergeCorrections} user merge${value.userMergeCorrections === 1 ? "" : "s"}` : null,
        value.provider === "local-index" ? "local index only" : `${value.provider}${value.durationMs ? ` \u00b7 ${formatDuration(value.durationMs)}` : ""}`,
      ].filter(Boolean).join(" \u00b7 ");
  const trigger = document.createElement("span");
  trigger.className = "event-resolution-trigger";
  const triggerLabel = value.resolverInvoked ? "Resolver invoked" : "Local fast path";
  const triggerTokens = value.triggerTokens?.length ? ` \u00b7 ${value.triggerTokens.join(", ")}` : "";
  const triggerReason = humanize(value.triggerReason);
  trigger.textContent = `${triggerLabel}: ${triggerReason} \u00b7 ${value.historicalEventCount ?? 0} retained events \u00b7 strongest overlap ${value.strongestOverlap ?? 0}${triggerTokens}`;
  diagnostic.append(title, detail, trigger);
  return diagnostic;
}

function buildInboxRun(run, sessionStatus = "") {
  const card = document.createElement("article");
  card.className = "inbox-run-card";
  const header = document.createElement("header");
  const source = document.createElement("strong");
  source.textContent = sourceLabel(run.source);
  const stage = document.createElement("span");
  const sourceUnavailable = run.error?.code === "source_unavailable";
  stage.className = `status-pill status-${sourceUnavailable ? "warning" : inboxStatusTone(run.status)}`;
  stage.textContent = run.status === "completed"
    ? "Completed"
    : sourceUnavailable
      ? "Unavailable \u00b7 Source"
      : `${humanize(run.status)} \u00b7 ${humanize(run.stage)}`;
  header.append(source, stage);
  const pipeline = document.createElement("div");
  pipeline.className = "inbox-pipeline";
  const metricNumbers = {};
  for (const [label, value] of [
    ["Captured", run.capturedCandidates],
    ["Evaluated", run.evaluatedCandidates],
    ["Selected", run.selectedCandidates],
    ["Added", run.addedItems],
  ]) {
    const metric = document.createElement("div");
    const number = document.createElement("strong");
    number.textContent = inboxRunMetricText(run, sessionStatus, label.toLowerCase(), value);
    metricNumbers[label.toLowerCase()] = number;
    const name = document.createElement("span");
    name.textContent = label;
    const stageDuration = run.stageDurationsMs?.[label.toLowerCase()];
    const timing = document.createElement("small");
    timing.className = "inbox-pipeline-duration";
    timing.textContent = Number.isFinite(stageDuration) ? formatDuration(stageDuration) : "";
    metric.append(number, name, timing);
    pipeline.append(metric);
  }
  const mechanics = document.createElement("p");
  mechanics.className = "inbox-run-mechanics";
  mechanics.textContent = [
    `${run.acquisitionRounds ?? 0} capture round${run.acquisitionRounds === 1 ? "" : "s"}`,
    `${run.snapshotCount ?? 0} snapshots`,
    `${run.performedScrolls ?? 0} scrolls`,
    run.totalDurationMs ? `Total ${formatDuration(run.totalDurationMs)}` : null,
    run.reasoningDurationMs ? `${formatDuration(run.reasoningDurationMs)} model time` : null,
    run.resurfacedItems ? `${run.resurfacedItems} resurfaced${run.skippedResurfaces ? ` · ${run.skippedResurfaces} skipped` : ""}` : null,
  ].filter(Boolean).join(" \u00b7 ");
  card.append(header, pipeline, mechanics);
  if (run.error) {
    const failure = document.createElement("p");
    failure.className = sourceUnavailable ? "inbox-run-warning" : "inbox-run-error";
    failure.textContent = sourceUnavailable
      ? run.error.message
      : `Stopped at ${humanize(run.error.stage || run.stage)}: ${run.error.message}`;
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
  card.append(buildInboxFlowInspector(run, (counts) => {
    for (const [stage, number] of Object.entries(metricNumbers)) {
      number.textContent = inboxRunMetricText(run, sessionStatus, stage, counts?.[stage]);
    }
  }));
  return card;
}

function inboxSessionFlowText(session) {
  const terminal = ["completed", "partial", "failed", "cancelled"].includes(session.status);
  if (!terminal) {
    const evaluation = (session.capturedCandidates ?? 0) > 0 && (session.evaluatedCandidates ?? 0) === 0
      ? "evaluating candidates"
      : `${session.evaluatedCandidates ?? 0} evaluated`;
    return `${session.capturedCandidates ?? 0} captured \u2192 ${evaluation} \u2192 composition pending`;
  }
  return `${session.capturedCandidates} captured \u2192 ${session.evaluatedCandidates} evaluated \u2192 ${session.addedItems} unique${session.duplicateReports ? ` + ${session.duplicateReports} duplicate` : ""}`;
}

function inboxRunMetricText(run, sessionStatus, stage, value) {
  const runTerminal = ["completed", "failed", "cancelled"].includes(run.status);
  const sessionTerminal = ["completed", "partial", "failed", "cancelled"].includes(sessionStatus);
  if (runTerminal && sessionTerminal) return String(value ?? 0);
  if (stage === "captured") return String(value ?? 0);
  if (stage === "evaluated") {
    if (runTerminal) return String(value ?? 0);
    if ((value ?? 0) > 0) return String(value);
    return run.status === "reasoning" ? "Evaluating\u2026" : "Waiting";
  }
  if (stage === "selected") {
    if (runTerminal) return String(value ?? 0);
    if ((value ?? 0) > 0) return String(value);
    return run.status === "reasoning" ? "Pending" : "Waiting";
  }
  if (stage === "added") {
    if (["failed", "cancelled"].includes(run.status)) return String(value ?? 0);
    if ((value ?? 0) > 0) return String(value);
    return sessionTerminal ? String(value ?? 0) : "Pending";
  }
  return String(value ?? 0);
}

function buildInboxFlowInspector(run, onCounts) {
  const inspector = document.createElement("section");
  inspector.className = "inbox-flow-inspector";
  const toggle = document.createElement("button");
  toggle.type = "button";
  toggle.className = "inbox-flow-toggle";
  toggle.textContent = "Inspect flow";
  toggle.setAttribute("aria-expanded", "false");
  const panel = document.createElement("div");
  panel.className = "inbox-flow-panel";
  panel.hidden = true;
  const filters = document.createElement("div");
  filters.className = "inbox-flow-filters";
  const list = document.createElement("div");
  list.className = "inbox-flow-list";
  const footer = document.createElement("div");
  footer.className = "inbox-flow-footer";
  const meta = document.createElement("span");
  const more = document.createElement("button");
  more.type = "button";
  more.textContent = "Show more";
  more.hidden = true;
  footer.append(meta);
  if (run.error && (run.capturedCandidates ?? 0) > 0) {
    const retry = document.createElement("button");
    retry.type = "button";
    retry.className = "inbox-flow-retry";
    retry.textContent = "Re-evaluate run";
    retry.title = "Retry reasoning from the already captured canonical evidence. No new browser capture is performed.";
    retry.addEventListener("click", async () => {
      retry.disabled = true;
      retry.textContent = "Starting…";
      try {
        await api(`/api/inbox/runs/${encodeURIComponent(run.id)}/re-evaluate`, { method: "POST" });
        await loadInbox();
      } catch (error) {
        showError(error);
        retry.disabled = false;
        retry.textContent = "Re-evaluate run";
      }
    });
    footer.append(retry);
  }
  footer.append(more);
  panel.append(filters, list, footer);
  inspector.append(toggle, panel);

  let activeStage = "captured";
  let offset = 0;
  let total = 0;
  let loading = false;
  const stageButtons = {};
  for (const stage of ["captured", "evaluated", "selected", "added"]) {
    const button = document.createElement("button");
    button.type = "button";
    button.dataset.stage = stage;
    button.textContent = humanize(stage);
    button.addEventListener("click", () => {
      if (activeStage === stage || loading) return;
      activeStage = stage;
      loadTrace(true);
    });
    stageButtons[stage] = button;
    filters.append(button);
  }

  const renderFilterState = (counts = {}) => {
    for (const [stage, button] of Object.entries(stageButtons)) {
      const count = counts[stage] ?? run[`${stage}Candidates`] ?? (stage === "added" ? run.addedItems : 0);
      button.textContent = `${humanize(stage)} ${count ?? 0}`;
      button.classList.toggle("selected", stage === activeStage);
      button.setAttribute("aria-pressed", String(stage === activeStage));
    }
  };

  const loadTrace = async (reset) => {
    if (loading) return;
    loading = true;
    if (reset) {
      offset = 0;
      list.replaceChildren();
    }
    meta.textContent = "Loading…";
    more.disabled = true;
    for (const button of Object.values(stageButtons)) button.disabled = true;
    try {
      const response = await api(`/api/inbox/runs/${encodeURIComponent(run.id)}/trace?stage=${activeStage}&limit=10&offset=${offset}`);
      const trace = response.trace;
      total = trace.total ?? 0;
      const rows = (trace.items ?? []).map((item) => buildInboxFlowItem(item, trace.source, run.id, async () => {
        await loadTrace(true);
        await refreshTimeline();
      }));
      if (reset) list.replaceChildren(...rows);
      else list.append(...rows);
      offset += rows.length;
      renderFilterState(trace.counts);
      onCounts?.(trace.counts);
      if (!list.children.length) {
        const empty = document.createElement("p");
        empty.className = "inbox-flow-empty";
        empty.textContent = `No ${humanize(activeStage).toLowerCase()} candidates in this source run.`;
        list.append(empty);
      }
      meta.textContent = total ? `${Math.min(offset, total)} of ${total}` : "No matching candidates";
      more.hidden = offset >= total;
    } catch (error) {
      meta.textContent = error.message;
      more.hidden = true;
    } finally {
      loading = false;
      more.disabled = false;
      for (const button of Object.values(stageButtons)) button.disabled = false;
    }
  };

  toggle.addEventListener("click", () => {
    const opening = panel.hidden;
    panel.hidden = !opening;
    toggle.textContent = opening ? "Hide flow" : "Inspect flow";
    toggle.setAttribute("aria-expanded", String(opening));
    if (opening && !list.children.length) loadTrace(true);
  });
  more.addEventListener("click", () => loadTrace(false));
  renderFilterState();
  return inspector;
}

function buildInboxFlowItem(item, source, runId, onChanged) {
  const row = document.createElement("article");
  row.className = "inbox-flow-item";
  const heading = document.createElement("div");
  heading.className = "inbox-flow-item-heading";
  const author = document.createElement("strong");
  author.textContent = item.author || "Captured source item";
  const outcome = document.createElement("span");
  outcome.className = `inbox-flow-outcome inbox-flow-outcome-${item.outcome}`;
  outcome.textContent = inboxFlowOutcomeLabel(item.outcome);
  heading.append(author);
  if (item.continuityStatus) {
    const continuity = document.createElement("span");
    continuity.className = `inbox-flow-outcome inbox-flow-continuity-${item.continuityStatus}`;
    continuity.textContent = inboxContinuityLabel(item.continuityStatus);
    continuity.title = item.continuityDetail || "";
    heading.append(continuity);
  }
  heading.append(outcome);
  const excerpt = document.createElement("p");
  excerpt.className = "inbox-flow-excerpt";
  excerpt.textContent = item.excerpt || "No textual excerpt was captured.";
  const reason = document.createElement("p");
  reason.className = "inbox-flow-reason";
  reason.textContent = item.reason || "No additional rationale recorded.";
  row.append(heading, excerpt, reason);
  const actions = document.createElement("div");
  actions.className = "inbox-flow-item-actions";
  const sourceUrl = safeSourceUrl(item.sourceUrl, source);
  if (sourceUrl) {
    const link = document.createElement("a");
    link.href = sourceUrl;
    link.target = "_blank";
    link.rel = "noopener noreferrer";
    link.textContent = "Open source";
    actions.append(link);
  }
  if (item.correction) {
    const state = document.createElement("span");
    state.textContent = "Selected by you";
    const undo = document.createElement("button");
    undo.type = "button";
    undo.textContent = "Undo";
    undo.addEventListener("click", async () => {
      undo.disabled = true;
      undo.textContent = "Undoing…";
      try {
        await api(`/api/selection-corrections/${encodeURIComponent(item.correction.id)}/undo`, { method: "POST" });
        await onChanged();
      } catch (error) {
        showError(error);
        undo.disabled = false;
        undo.textContent = "Undo";
      }
    });
    actions.append(state, undo);
  } else if (item.outcome === "not_selected" && item.candidateRef) {
    const select = document.createElement("button");
    select.type = "button";
    select.className = "inbox-selection-correction-button";
    const overlap = item.continuityStatus === "prior_knowledge_overlap";
    const resurfaced = item.continuityStatus?.startsWith("resurfaced_");
    select.textContent = overlap ? "Select despite overlap" : resurfaced ? "Show this resurface" : "Should have selected";
    select.title = overlap
      ? "Add this update despite its overlap with retained knowledge and record the explicit user correction."
      : resurfaced
        ? "Add this resurfaced update to the Timeline as an explicit user decision."
        : "Add this update to the Timeline and teach AkuBrowser that this kind of item matters.";
    select.addEventListener("click", async () => {
      select.disabled = true;
      select.textContent = "Selecting…";
      try {
        await api(`/api/inbox/runs/${encodeURIComponent(runId)}/selection-corrections`, {
          method: "POST",
          body: { candidateRef: item.candidateRef },
        });
        await onChanged();
      } catch (error) {
        showError(error);
        select.disabled = false;
        select.textContent = overlap ? "Select despite overlap" : resurfaced ? "Show this resurface" : "Should have selected";
      }
    });
    actions.append(select);
  }
  if (actions.children.length) row.append(actions);
  return row;
}

function inboxFlowOutcomeLabel(outcome) {
  if (outcome === "captured_only") return "Captured only";
  if (outcome === "not_selected") return "Not selected";
  if (outcome === "exact_replay") return "Already captured";
  if (outcome === "resurfaced_unchanged") return "Skipped before reasoning";
  if (outcome === "collapsed_duplicate") return "Semantic duplicate";
  if (outcome === "user_selected") return "Selected by you";
  return humanize(outcome || "captured");
}

function inboxContinuityLabel(status) {
  if (status === "resurfaced_unchanged") return "Resurfaced · unchanged";
  if (status === "resurfaced_changed") return "Resurfaced · new signal";
  if (status === "resurfaced_after_cooldown") return "Resurfaced · cooldown passed";
  if (status === "prior_knowledge_overlap") return "Prior knowledge overlap";
  return humanize(status);
}

function inboxStatusTone(status) {
  if (status === "completed") return "ok";
  if (["failed", "cancelled"].includes(status)) return "danger";
  if (status === "partial") return "warning";
  return "neutral";
}

function routeAIDetectedItems(items) {
  if (state.bootstrap?.settings?.aiDetectionEnabled === false) {
    return { inline: [...items], drawer: [], hidden: [], pending: false };
  }
  const mode = state.bootstrap?.settings?.aiDetectionPresentation || "drawer";
  const result = { inline: [], drawer: [], hidden: [], pending: false };
  for (const entry of items) {
    const detection = entry.aiDetection;
    result.pending ||= Boolean(detection?.pendingDeep);
    const seen = state.seenTimelineItems.has(entry.id);
    if (mode === "drawer" && detection?.routeToSignals && !seen) result.drawer.push(entry);
    else if (mode === "hide" && detection?.hideEligible && !seen) result.hidden.push(entry);
    else result.inline.push(entry);
  }
  return result;
}

function renderTimeline(items, latestCheck) {
  const allItems = Array.isArray(items) ? items : [];
  state.timelineItems = allItems;
  const retainedIDs = new Set(allItems.map((entry) => entry.id));
  for (const timelineID of state.passiveMediaEvidenceAttempts.keys()) {
    if (!retainedIDs.has(timelineID)) state.passiveMediaEvidenceAttempts.delete(timelineID);
  }
  for (const key of state.expandedTimelineText) {
    if (!retainedIDs.has(key.split("|")[0])) state.expandedTimelineText.delete(key);
  }
  schedulePassiveMediaEnrichment(allItems);
  const routed = routeAIDetectedItems(items);
  items = routed.inline;
  renderTimelineSidePane(routed.drawer, routed.pending);
  scheduleAIDeepRefresh(routed.pending);
  const container = $("#result-items");
  container.replaceChildren();
  if (latestCheck) {
    const unique = latestCheck.addedItems ?? 0;
    const duplicates = latestCheck.duplicateReports ?? 0;
    const parts = [unique ? `${unique} new item${unique === 1 ? "" : "s"}` : "No new items"];
    if (duplicates) parts.push(`${duplicates} duplicate report${duplicates === 1 ? "" : "s"}`);
    if (routed.drawer.length) parts.push(`${routed.drawer.length} in AI Signals`);
    if (routed.hidden.length) parts.push(`${routed.hidden.length} AI-signal post${routed.hidden.length === 1 ? "" : "s"} hidden`);
    $("#timeline-meta").textContent = `${parts.join(" \u00b7 ")} from the latest check`;
  } else {
    $("#timeline-meta").textContent = "No completed check yet.";
  }
  $("#finish-stats").textContent = items.length
    ? `Shown: ${items.length} · bounded local evidence · personalized across sources`
    : "Check active sources to establish the finite timeline.";
  if (items.length || routed.drawer.length || routed.hidden.length) {
    $("#finish-stats").textContent = `Shown: ${items.length} · ${routed.drawer.length} in side pane · ${routed.hidden.length} hidden · bounded local evidence`;
  }
  if (!items.length) {
    const empty = document.createElement("section");
    empty.className = "finish-line";
    const title = document.createElement("h3");
    title.textContent = routed.drawer.length || routed.hidden.length ? "No inline updates in this view" : "No retained updates yet";
    const detail = document.createElement("p");
    detail.textContent = routed.drawer.length
      ? "Strong AI-origin signals are available in the AI Signals side pane."
      : routed.hidden.length
        ? "The active high-risk Hide policy removed Deep-confirmed or directly labeled posts from this view."
        : "AkuBrowser will place evaluated, source-backed items here after the next bounded check.";
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
    const rendered = buildTimelineItem(entry);
    rendered.dataset.timelineId = entry.id;
    container.append(rendered);
    observeTimelineItem(rendered, entry.id);
  }
  scheduleBackToTop();
}

function schedulePassiveMediaEnrichment(items = state.timelineItems) {
  if (
    state.passiveMediaEnrichmentActive ||
    state.passiveMediaEnrichmentTimer ||
    state.session ||
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
    if (updated) await refreshTimeline();
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

function observeTimelineItem(element, timelineId) {
  if (!("IntersectionObserver" in window)) return;
  const observer = new IntersectionObserver((entries) => {
    if (entries.some((entry) => entry.isIntersecting)) {
      state.seenTimelineItems.add(timelineId);
      observer.disconnect();
    }
  }, { threshold: 0.35 });
  observer.observe(element);
}

function scheduleAIDeepRefresh(pending) {
  if (!pending) {
    if (state.aiDeepPoller) clearTimeout(state.aiDeepPoller);
    state.aiDeepPoller = null;
    return;
  }
  if (state.aiDeepPoller) return;
  state.aiDeepPoller = window.setTimeout(async () => {
    state.aiDeepPoller = null;
    await refreshTimeline();
  }, AI_DEEP_POLL_INTERVAL_MS);
}

function renderTimelineSidePane(items, pending) {
  scheduleTimelineSidePanePosition();
  state.sidePaneItems = items;
  const available = syncTimelineSidePaneVisibility();
  $("#timeline-side-pane-count").textContent = String(items.length);
  $("#timeline-side-pane-detail").textContent = pending
    ? `${items.length} routed post${items.length === 1 ? "" : "s"} · Deep Detection is still reviewing this Timeline.`
    : `${items.length} strong-signal post${items.length === 1 ? "" : "s"} routed from the current finite Timeline.`;
  if (!available) closeTimelineSidePane();
  if (!$("#timeline-side-pane").classList.contains("hidden")) populateTimelineSidePane();
}

function timelineSidePaneAvailable() {
  return state.bootstrap?.settings?.aiDetectionEnabled !== false
    && state.bootstrap?.settings?.aiDetectionPresentation === "drawer"
    && state.currentView === "timeline"
    && state.bootstrap?.onboarding?.status === "completed"
    && !state.onboardingEditing
    && !state.bootstrap?.calibration?.active
    && Boolean(state.bootstrap?.latestCheck)
    && state.timelineItems.length > 0;
}

function syncTimelineSidePaneVisibility() {
  const available = timelineSidePaneAvailable();
  const toggle = $("#timeline-side-pane-toggle");
  toggle.classList.toggle("hidden", !available);
  toggle.classList.toggle("view-hidden", !available);
  if (!available) closeTimelineSidePane();
  return available;
}

function openTimelineSidePane() {
  if (!timelineSidePaneAvailable()) return;
  $("#timeline-side-pane").classList.remove("hidden");
  $("#timeline-side-pane-toggle").setAttribute("aria-expanded", "true");
  populateTimelineSidePane();
}

function closeTimelineSidePane() {
  $("#timeline-side-pane").classList.add("hidden");
  $("#timeline-side-pane-toggle").setAttribute("aria-expanded", "false");
}

function populateTimelineSidePane() {
  const container = $("#timeline-side-pane-items");
  container.replaceChildren();
  if (!state.sidePaneItems.length) {
    const empty = document.createElement("p");
    empty.className = "timeline-side-pane-empty";
    empty.textContent = "No strong AI-origin signals are routed here yet.";
    container.append(empty);
    return;
  }
  for (const entry of state.sidePaneItems) {
    const item = buildTimelineItem(entry);
    item.classList.add("timeline-side-pane-card");
    container.append(item);
  }
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
  const signalSlot = document.createElement("span");
  signalSlot.className = "item-signal-slot";
  const ai = buildAIDetectionControls(entry);
  signalSlot.append(buildSourceIcon(entry.source), ai.badge);
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
  toolbar.append(signalSlot, label, toggle);
  container.append(toolbar);
  container.append(ai.details);
  container.append(brief, source, actions);
  render();
  return container;
}

function buildAIDetectionControls(entry) {
  if (state.bootstrap?.settings?.aiDetectionEnabled === false || !entry.aiDetection) {
    const badge = document.createElement("span");
    badge.className = "hidden";
    const details = document.createElement("div");
    details.className = "hidden";
    return { badge, details };
  }
  const detection = entry.aiDetection ?? null;
  const hasAssessmentLabel = Boolean(detection?.badgeLabel);
  const badgeLabel = detection?.badgeLabel || (detection?.pendingDeep
    ? "AI signal · Checking"
    : "AI signal · Neutral");
  const badgeTone = hasAssessmentLabel
    ? detection.corrected || detection.status === "conflicting_evidence"
      ? "corrected"
      : detection.userOverride
        ? "user"
        : detection.stage || "fast"
    : detection?.pendingDeep
      ? "pending"
      : "neutral";
  const badge = document.createElement("button");
  badge.type = "button";
  badge.className = `ai-origin-badge ai-origin-${badgeTone}`;
  badge.textContent = badgeLabel;
  badge.title = "Review AI signal status and corrections";
  badge.setAttribute("aria-expanded", "false");
  const details = document.createElement("div");
  details.className = "ai-assessment-detail hidden";
  const summary = document.createElement("p");
  summary.textContent = detection?.detail || (detection?.pendingDeep
    ? "Deep Detection is still reviewing this post. You remain in control of its personal classification."
    : "No strong AI-origin signal is currently recorded for this post.");
  const meta = document.createElement("small");
  const evidence = (detection?.evidenceCodes || []).map(humanize).join(", ");
  meta.textContent = detection
    ? [humanize(detection.stage), humanize(detection.confidenceBand), evidence, `${detection.historyCount || 0} assessment${detection.historyCount === 1 ? "" : "s"}`].filter(Boolean).join(" · ")
    : "No detector assessment yet";
  const actions = document.createElement("div");
  actions.className = "ai-assessment-actions";
  if (detection?.userOverride && detection.correctionId) {
    const undo = document.createElement("button");
    undo.type = "button";
    undo.textContent = "Clear my correction";
    undo.addEventListener("click", async () => {
      undo.disabled = true;
      try {
        await api(`/api/ai-corrections/${encodeURIComponent(detection.correctionId)}/undo`, { method: "POST" });
        await refreshTimeline();
      } catch (error) {
        showError(error);
        undo.disabled = false;
      }
    });
    actions.append(undo);
  } else {
    const notAI = document.createElement("button");
    notAI.type = "button";
    notAI.textContent = "Mark as not AI-generated";
    notAI.addEventListener("click", () => applyAICorrection(entry.id, "not_ai", notAI));
    const isAI = document.createElement("button");
    isAI.type = "button";
    isAI.textContent = "Mark as AI-generated";
    isAI.addEventListener("click", () => applyAICorrection(entry.id, "ai", isAI));
    actions.append(notAI, isAI);
  }
  details.append(summary, meta, actions);
  badge.addEventListener("click", () => {
    const expanded = badge.getAttribute("aria-expanded") === "true";
    badge.setAttribute("aria-expanded", String(!expanded));
    details.classList.toggle("hidden", expanded);
  });
  return { badge, details };
}

async function applyAICorrection(timelineId, verdict, trigger) {
  trigger.disabled = true;
  try {
    await api(`/api/timeline/${encodeURIComponent(timelineId)}/ai-correction`, { method: "POST", body: { verdict } });
    await refreshTimeline();
  } catch (error) {
    showError(error);
    trigger.disabled = false;
  }
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
  const descriptor = sourceDescriptor(source) || {};
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
  context.textContent = [presentation.connectionDegree, presentation.timestampText].filter(Boolean).join(" · ")
    || parsed.secondary
    || formatDate(evidence.publishedAt || item.publishedAt);
  identity.append(author);
  if (presentation.headline) {
    const headline = document.createElement("span");
    headline.className = "source-layout-headline";
    headline.textContent = presentation.headline;
    identity.append(headline);
  }
  if (context.textContent) identity.append(context);
  header.append(avatar, identity);

  if (descriptor.socialContextPlacement === "above" && presentation.socialContext) {
    const social = document.createElement("div");
    social.className = "source-social-context-above";
    const socialAvatarUrl = safeMediaUrl(presentation.socialContextAvatarUrl);
    if (socialAvatarUrl) {
      const socialAvatar = document.createElement("img");
      socialAvatar.src = socialAvatarUrl;
      socialAvatar.alt = "";
      socialAvatar.loading = "lazy";
      socialAvatar.referrerPolicy = "no-referrer";
      social.append(socialAvatar);
    }
    const socialCopy = document.createElement("span");
    socialCopy.textContent = presentation.socialContext;
    social.append(socialCopy);
    card.append(social);
  }
  card.append(header);

  const content = document.createElement("div");
  content.className = `source-layout-content source-${descriptor.presentationStyle || "social"}-content`;
  if (descriptor.socialContextPlacement !== "above" && presentation.socialContext) {
    const social = document.createElement("p");
    social.className = "source-social-context-content";
    social.textContent = presentation.socialContext;
    content.append(social);
  }
  content.append(buildExpandableText(evidence.text || item.whatChanged, {
    characterLimit: SOURCE_TEXT_COLLAPSE_CHARACTERS,
    lineLimit: SOURCE_TEXT_COLLAPSE_LINES,
    label: "post",
    expansionKey: entry.id ? `${entry.id}|post` : null,
  }));
  const quote = buildQuotedPost(evidence.quotedPost, source, entry.id ? `${entry.id}|quote` : null);
  if (quote) content.append(quote);
  card.append(content);
  const attachments = buildAttachments(evidence.attachments, source);
  if (attachments) card.append(attachments);
  const media = buildMedia(evidence.media, source, evidence.contentKind);
  if (media) card.append(media);
  if (evidence.mediaRecovery?.outcome === "unavailable") {
    const unavailable = document.createElement("div");
    unavailable.className = "source-layout-media-unavailable";
    const message = document.createElement("span");
    message.textContent = "Media was present at the source but unavailable in this captured view.";
    unavailable.append(message);
    unavailable.append(state.foregroundRecaptureOffers.has(entry.id)
      ? buildForegroundRecaptureOffer(entry)
      : buildMediaRecaptureButton(entry));
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
  const descriptor = sourceDescriptor(source);
  fallback.textContent = descriptor?.avatarFallback === "source_icon"
    ? (descriptor.iconText || sourceLabel(source).slice(0, 1))
    : initials(author);
  return fallback;
}

function buildQuotedPost(value, source, expansionKey = null) {
  if (!value?.text) return null;
  const quote = document.createElement("section");
  quote.className = "source-quote-card";
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
    expansionKey,
  }));
  return quote;
}

function buildExpandableText(value, { characterLimit, lineLimit, label, expansionKey = null }) {
  const wrapper = document.createElement("div");
  wrapper.className = "expandable-text";
  const text = document.createElement("p");
  text.className = "expandable-text-copy";
  text.textContent = value || "";
  wrapper.append(text);

  const logicalLines = text.textContent.split(/\r?\n/).length;
  if (text.textContent.length <= characterLimit && logicalLines <= lineLimit) return wrapper;

  const initiallyExpanded = expansionKey && state.expandedTimelineText.has(expansionKey);
  text.classList.toggle("is-collapsed", !initiallyExpanded);
  text.style.setProperty("--collapse-lines", String(lineLimit));
  const toggle = document.createElement("button");
  toggle.type = "button";
  toggle.className = "content-expander";
  toggle.textContent = initiallyExpanded ? "Show less" : "Show more";
  toggle.setAttribute("aria-expanded", String(Boolean(initiallyExpanded)));
  toggle.setAttribute("aria-label", `${initiallyExpanded ? "Show less" : "Show more"} ${label} text`);
  toggle.addEventListener("click", () => {
    const expanded = toggle.getAttribute("aria-expanded") === "true";
    text.classList.toggle("is-collapsed", expanded);
    toggle.setAttribute("aria-expanded", String(!expanded));
    toggle.setAttribute("aria-label", `${expanded ? "Show more" : "Show less"} ${label} text`);
    toggle.textContent = expanded ? "Show more" : "Show less";
    if (expansionKey) {
      if (expanded) state.expandedTimelineText.delete(expansionKey);
      else state.expandedTimelineText.add(expansionKey);
    }
  });
  wrapper.append(toggle);
  return wrapper;
}

function buildMedia(values, source, contentKind = "") {
  const media = (Array.isArray(values) ? values : [])
    .map((value) => ({ ...value, displayUrl: safeMediaUrl(value.posterUrl || value.url) }))
    .filter((value) => value.displayUrl)
    .slice(0, 4);
  if (!media.length) return null;
  const gallery = document.createElement("div");
  gallery.className = `source-layout-media media-count-${media.length}`;
  for (const value of media) {
    const isVideo = value.kind === "video" || (contentKind === "video" && media.length === 1);
    const button = document.createElement("button");
    button.type = "button";
    button.className = "source-layout-media-item";
    const image = document.createElement("img");
    image.src = value.displayUrl;
    image.alt = value.alt || `${sourceLabel(source)} post media`;
    image.loading = "lazy";
    image.referrerPolicy = "no-referrer";
    button.append(image);
    if (isVideo) {
      button.classList.add("is-video-poster");
      const cue = document.createElement("span");
      cue.className = "source-layout-video-cue";
      cue.setAttribute("aria-hidden", "true");
      cue.textContent = "▶";
      const label = document.createElement("span");
      label.className = "source-layout-video-label";
      label.textContent = "Video preview";
      button.append(cue, label);
      button.setAttribute("aria-label", `Open ${sourceLabel(source)} video preview`);
    }
    button.addEventListener("click", () => openMedia(media.map((entry) => entry.displayUrl), media.indexOf(value)));
    gallery.append(button);
  }
  return gallery;
}

function buildAttachments(values, source) {
  const attachments = (Array.isArray(values) ? values : [])
    .map((value) => ({ ...value, safeUrl: safeMediaUrl(value.url) }))
    .filter((value) => value.safeUrl && value.title)
    .slice(0, 3);
  if (!attachments.length) return null;
  const list = document.createElement("div");
  list.className = "source-layout-attachments";
  for (const value of attachments) {
    const link = document.createElement("a");
    link.className = "source-layout-attachment";
    link.href = value.safeUrl;
    link.target = "_blank";
    link.rel = "noopener noreferrer";
    const imageUrl = safeMediaUrl(value.imageUrl);
    if (imageUrl) {
      const image = document.createElement("img");
      image.src = imageUrl;
      image.alt = "";
      image.loading = "lazy";
      image.referrerPolicy = "no-referrer";
      link.append(image);
    } else {
      const fallback = document.createElement("span");
      fallback.className = "source-layout-attachment-fallback";
      fallback.textContent = sourceDescriptor(source)?.iconText || sourceLabel(source).slice(0, 1);
      link.append(fallback);
    }
    const copy = document.createElement("span");
    copy.className = "source-layout-attachment-copy";
    const title = document.createElement("strong");
    title.textContent = value.title;
    copy.append(title);
    const subtitle = [value.subtitle, value.detail].filter(Boolean).join(" · ");
    if (subtitle) {
      const detail = document.createElement("span");
      detail.textContent = subtitle;
      copy.append(detail);
    }
    const domain = document.createElement("small");
    domain.textContent = value.domain || value.actionLabel || "Open attachment";
    copy.append(domain);
    link.append(copy);
    list.append(link);
  }
  return list;
}

function buildEngagement(value, source) {
  if (!value || typeof value !== "object") return null;
  const definitions = (sourceDescriptor(source)?.engagementMetrics ?? [])
    .map((metric) => [metric.key, metric.icon]);
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
  const renderDirection = () => {
    const direction = entry.feedback?.direction;
    const calibrationChoice = entry.feedback?.origin === "calibration" ? "Chosen during onboarding calibration" : "";
    more.classList.toggle("selected", direction === "more");
    less.classList.toggle("selected", direction === "less");
    more.setAttribute("aria-pressed", String(direction === "more"));
    less.setAttribute("aria-pressed", String(direction === "less"));
    more.title = direction === "more" ? calibrationChoice : "";
    less.title = direction === "less" ? calibrationChoice : "";
  };
  renderDirection();
  more.addEventListener("click", async () => {
    const feedback = await sendFeedback(entry.id, "more", null);
    if (!feedback) return;
    entry.feedback = feedback;
    renderDirection();
  });
  less.addEventListener("click", async () => {
    const feedback = await sendFeedback(entry.id, "less", "not_interested");
    if (!feedback) return;
    entry.feedback = feedback;
    renderDirection();
  });
  feedback.append(more, less);
  if (link) actions.append(link);
  actions.append(feedback);
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
  const href = safeSourceUrl(entry.item?.sourceUrl || entry.evidence?.permalink, entry.source || entry.item?.source);
  if (!href) return null;
  const link = document.createElement("a");
  link.className = "source-link";
  link.href = href;
  link.target = "_blank";
  link.rel = "noopener noreferrer";
  link.textContent = entry.item?.sourceUrlKind === "native_post" ? "Open native post" : "Open source evidence";
  return link;
}

function safeSourceUrl(value, source) {
  const href = safeMediaUrl(value);
  if (!href) return null;
  const url = new URL(href);
  const descriptor = sourceDescriptor(source);
  if (!descriptor) return null;
  const trustedHost = (descriptor.nativeHosts ?? []).includes(url.hostname);
  const nativePath = (descriptor.nativePathTokens ?? []).some((token) => url.pathname.includes(token));
  return trustedHost && nativePath ? url.href : null;
}

function buildMediaRecaptureButton(entry) {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "recapture-button";
  button.textContent = "Recapture";
  button.disabled = Boolean(state.session) || state.mediaRecaptureActive || !state.bootstrap?.bridge?.compatible;
  button.addEventListener("click", () => recaptureMedia(entry, button, "background"));
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
  accept.addEventListener("click", () => recaptureMedia(entry, accept, "foreground"));
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

async function recaptureMedia(entry, button, captureMode) {
  if (state.session || state.mediaRecaptureActive || !state.bootstrap?.bridge?.compatible) return;
  state.mediaRecaptureActive = true;
  button.disabled = true;
  button.textContent = captureMode === "foreground" ? "Capturing in foreground..." : "Recapturing...";
  syncRunButtons();
  clearNotice();
  try {
    const { recapture } = await api(`/api/timeline/${encodeURIComponent(entry.id)}/recapture`, {
      method: "POST",
      body: { captureMode },
    });
    const completed = await dispatchMediaRecapture(recapture.id);
    if (captureMode === "background" && completed?.outcome !== "recovered") {
      state.foregroundRecaptureOffers.add(entry.id);
      await refreshTimeline();
      return;
    }
    state.foregroundRecaptureOffers.delete(entry.id);
    await refreshTimeline();
    const notice = $("#provider-notice");
    notice.className = "notice notice-complete";
    notice.setAttribute("role", "status");
    notice.textContent = completed?.outcome === "recovered"
      ? "Media recaptured from the native post."
      : "Media is still unavailable after the foreground capture.";
  } catch (error) {
    showError(error);
    button.disabled = false;
    button.textContent = captureMode === "foreground" ? "Try in foreground" : "Recapture";
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

function feedbackButton(label) {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "feedback-button";
  button.textContent = label;
  return button;
}

async function sendFeedback(id, direction, reason) {
  try {
    const response = await api(`/api/timeline/${encodeURIComponent(id)}/feedback`, {
      method: "POST",
      body: { direction, reason },
    });
    return response.feedback;
  } catch (error) {
    showError(error);
    return null;
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
  const sourcesUnavailable = failedRuns.length > 0 && failedRuns.every((run) => run.error?.code === "source_unavailable");
  const panel = $("#failure-panel");
  $("#retry-button").dataset.action = "return";
  $("#retry-button").textContent = "Return to timeline";
  panel.classList.toggle("failure-panel-warning", sourcesUnavailable);
  panel.setAttribute("role", sourcesUnavailable ? "status" : "alert");
  $("#failure-label").textContent = sourcesUnavailable ? "SOURCE UNAVAILABLE" : "RUN STOPPED";
  $("#failure-title").textContent = sourcesUnavailable
    ? `${failedSources || "One source"} is temporarily unavailable`
    : "The bounded snapshot could not finish";
  if (sourcesUnavailable) {
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
  panel.classList.add("failure-panel-warning");
  panel.setAttribute("role", "status");
  $("#failure-label").textContent = "CALIBRATION WAITING";
  $("#failure-title").textContent = "Check once more to start calibration";
  $("#failure-message").textContent = failedSources.length
    ? `${failedSources.join(" and ")} did not produce a validated entry in this check. Choose Check for updates again; captured evidence remains available in Update Inbox.`
    : "This check did not produce a validated entry for calibration. Choose Check for updates again to collect another bounded sample.";
  $("#retry-button").dataset.action = "retry-update";
  $("#retry-button").textContent = "Check for updates again";
  panel.classList.remove("hidden");
}

function showSessionOutcome(session) {
  if (!["completed", "partial"].includes(session.status)) return;
  const additions = session.items?.length ?? 0;
  if (additions > 0) {
    clearNotice();
    return;
  }
  const notice = $("#provider-notice");
  notice.className = "notice notice-complete";
  notice.setAttribute("role", "status");
  notice.textContent = "Update complete: 0 additions. No captured candidate cleared the new, material, and trusted-evidence boundary.";
}

function hideFailure() {
  $("#failure-panel").classList.add("hidden");
  $("#retry-button").dataset.action = "return";
  $("#retry-button").textContent = "Return to timeline";
}

function setPill(selector, text, tone) {
  const node = $(selector);
  node.textContent = text;
  node.className = `status-pill status-${tone}`;
}

function showError(error) {
  if (error?.recoveryInitiated) return;
  console.error(error);
  const notice = $("#provider-notice");
  notice.className = "notice notice-danger";
  notice.setAttribute("role", "alert");
  notice.textContent = error.message || String(error);
  setPill(
    "#sidecar-status",
    error.code === "sidecar_unavailable" ? "AkuSidecar offline" : "AkuSidecar attention",
    "danger",
  );
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
  if (sourceDescriptor(source)?.identityFormat !== "display_handle") return { displayName: text, secondary: "" };
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
  return sourceDescriptor(source)?.displayName || String(source || "Unknown source");
}

function buildSourceIcon(source) {
  const descriptor = sourceDescriptor(source);
  const icon = document.createElement("span");
  icon.className = `timeline-source-icon timeline-source-icon-${source}`;
  icon.title = sourceLabel(source);
  icon.setAttribute("aria-label", `Source: ${sourceLabel(source)}`);
  icon.style.background = descriptor?.iconBackground || "var(--panel-strong)";
  icon.style.color = descriptor?.iconForeground || "var(--text)";
  icon.textContent = descriptor?.iconText || sourceLabel(source).slice(0, 1);
  return icon;
}

function sourceDescriptors() {
  return Array.isArray(state.bootstrap?.sources) ? state.bootstrap.sources : [];
}

function sourceDescriptor(source) {
  return sourceDescriptors().find((descriptor) => descriptor.id === source);
}

function renderSourceControls() {
  const onboarding = $("#onboarding-source-options");
  const settings = $("#settings-source-options");
  onboarding.replaceChildren();
  settings.replaceChildren();
  for (const descriptor of sourceDescriptors()) {
    const label = document.createElement("label");
    label.className = "onboarding-source-option";
    const input = document.createElement("input");
    input.type = "checkbox";
    input.value = descriptor.id;
    input.checked = Boolean(descriptor.defaultActive);
    input.addEventListener("change", updateOnboardingSummary);
    const card = document.createElement("span");
    card.className = "onboarding-source-card";
    const icon = document.createElement("span");
    icon.className = "onboarding-source-icon";
    icon.setAttribute("aria-hidden", "true");
    icon.style.background = descriptor.iconBackground;
    icon.style.color = descriptor.iconForeground;
    icon.textContent = descriptor.iconText;
    const copy = document.createElement("span");
    copy.className = "onboarding-source-copy";
    const strong = document.createElement("strong");
    strong.textContent = descriptor.displayName;
    const small = document.createElement("small");
    small.textContent = descriptor.onboardingDescription;
    copy.append(strong, small);
    const check = document.createElement("span");
    check.className = "onboarding-source-check";
    check.setAttribute("aria-hidden", "true");
    check.textContent = "✓";
    card.append(icon, copy, check);
    label.append(input, card);
    onboarding.append(label);

    const settingsLabel = document.createElement("div");
    settingsLabel.className = "settings-row source-settings-row";
    const settingsCopy = document.createElement("span");
    settingsCopy.className = "settings-copy";
    const settingsStrong = document.createElement("strong");
    settingsStrong.textContent = descriptor.displayName;
    const settingsSmall = document.createElement("small");
    settingsSmall.textContent = `Use the signed-in ${descriptor.displayName} feed during the next update.`;
    settingsCopy.append(settingsStrong, settingsSmall);
    const controls = document.createElement("span");
    controls.className = "source-settings-control";
    const activeLabel = document.createElement("label");
    activeLabel.className = "source-active-control";
    const activeText = document.createElement("span");
    activeText.textContent = "Active";
    const settingsInput = document.createElement("input");
    settingsInput.type = "checkbox";
    settingsInput.value = descriptor.id;
    settingsInput.setAttribute("aria-label", `Use ${descriptor.displayName} during updates`);
    activeLabel.append(settingsInput, activeText);

    const hydrationLabel = document.createElement("label");
    hydrationLabel.className = "source-hydration-input";
    const hydrationText = document.createElement("span");
    hydrationText.textContent = "Hydration";
    const hydrationField = document.createElement("span");
    const hydrationInput = document.createElement("input");
    hydrationInput.type = "number";
    hydrationInput.min = String(descriptor.hydrationTimeoutMinMs / 1000);
    hydrationInput.max = String(descriptor.hydrationTimeoutMaxMs / 1000);
    hydrationInput.step = "1";
    hydrationInput.dataset.sourceHydration = descriptor.id;
    hydrationInput.setAttribute("aria-label", `${descriptor.displayName} hydration wait in seconds`);
    const hydrationUnit = document.createElement("span");
    hydrationUnit.textContent = "s";
    hydrationField.append(hydrationInput, hydrationUnit);
    hydrationLabel.append(hydrationText, hydrationField);

    const reset = document.createElement("button");
    reset.type = "button";
    reset.textContent = "Reset to default";
    reset.addEventListener("click", () => {
      hydrationInput.value = String(descriptor.hydrationTimeoutDefaultMs / 1000);
    });
    controls.append(activeLabel, hydrationLabel, reset);
    settingsLabel.append(settingsCopy, controls);
    settings.append(settingsLabel);
  }
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
  const response = await fetchFromSidecar(path, init);
  const payload = response.status === 204 ? null : await response.json();
  if (!response.ok) {
    const error = new Error(payload?.message || `HTTP ${response.status}`);
    error.code = payload?.error || `http_${response.status}`;
    error.details = payload?.details;
    throw error;
  }
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
  const response = await fetchFromSidecar(path, init);
  if (response.status === 204) return null;
  const payload = await response.json();
  if (!response.ok) {
    const error = new Error(payload?.message || `HTTP ${response.status}`);
    error.code = payload?.error || `http_${response.status}`;
    if (recoverInvalidBridgeToken(error.code)) {
      error.recoveryInitiated = true;
      throw error;
    }
    if (error.code === "invalid_bridge_token") {
      error.message = "AkuSidecar keeps changing its Bridge token. Check whether another AkuSidecar instance or security software is restarting the runtime, then refresh AkuBrowser once.";
    }
    throw error;
  }
  return payload;
}

async function fetchFromSidecar(path, init) {
  try {
    return await fetch(path, init);
  } catch (error) {
    if (error instanceof TypeError) {
      const unavailable = new Error(
        "AkuSidecar is offline or unreachable. Start it through AkuSupervisor; AkuBrowser will reconnect automatically.",
      );
      unavailable.name = "SidecarUnavailableError";
      unavailable.code = "sidecar_unavailable";
      unavailable.cause = error;
      throw unavailable;
    }
    throw error;
  }
}

syncRunButtons();
bootstrap();

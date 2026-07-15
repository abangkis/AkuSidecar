const endpoint = location.origin;
const defaultIntent = "What materially changed since my last check?";
const terminalStatuses = new Set(["completed", "partial", "failed", "cancelled"]);
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
$("#settings-view-button").addEventListener("click", () => setView("settings"));
$("#timeline-refresh-button").addEventListener("click", refreshTimeline);
$("#timeline-runner-button").addEventListener("click", startSession);
$("#done-button").addEventListener("click", startSession);
$("#retry-button").addEventListener("click", () => {
  hideFailure();
  setView("timeline");
});
$("#cancel-button").addEventListener("click", cancelSession);
$("#runtime-settings-form").addEventListener("submit", saveSettings);
$("#bounded-load-profile").addEventListener("change", syncCustomSettings);
$("#stream-width").addEventListener("change", () => applyStreamWidth($("#stream-width").value));
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
$("#back-to-top").addEventListener("click", () => window.scrollTo({ top: 0, behavior: "smooth" }));
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
    renderTimeline(state.bootstrap.timeline ?? []);
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
  const settings = view === "settings";
  $("#onboarding-panel").classList.add("hidden");
  $("#calibration-panel").classList.add("hidden");
  document.querySelector(".view-switch")?.classList.remove("hidden");
  $("#settings-panel").classList.toggle("hidden", !settings);
  $("#timeline-panel").classList.toggle("hidden", settings);
  $("#session-view-button").classList.toggle("selected", !settings);
  $("#settings-view-button").classList.toggle("selected", settings);
  (settings ? $("#settings-heading") : $("#timeline-heading")).focus?.();
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
  $("#settings-source-x").checked = settings.activeSources?.includes("x") ?? false;
  $("#settings-source-linkedin").checked = settings.activeSources?.includes("linkedin") ?? false;
  applyStreamWidth(settings.streamWidth || "social");
  syncCustomSettings();
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
    maxItemsTotal: loadProfile === "custom" ? Math.min(30, Math.max(1, perSource * 2)) : current.maxItemsTotal,
    maxScrolls: Number.parseInt($("#max-scrolls").value, 10),
    defaultPresentation: $("#default-presentation").value,
    streamWidth: $("#stream-width").value,
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

function syncCustomSettings() {
  const custom = $("#bounded-load-profile").value === "custom";
  $("#timeline-capacity").disabled = !custom;
  $("#max-items-per-source").disabled = !custom;
  $("#max-scrolls").disabled = !custom;
}

function applyStreamWidth(value) {
  document.body.dataset.streamWidth = ["compact", "social", "comfortable", "wide"].includes(value) ? value : "social";
  scheduleBackToTop();
}

function showOnboarding(editing) {
  state.onboardingEditing = editing;
  const sources = state.bootstrap?.settings?.activeSources ?? ["x", "linkedin"];
  $("#onboarding-source-x").checked = sources.includes("x");
  $("#onboarding-source-linkedin").checked = sources.includes("linkedin");
  $("#onboarding-cancel").classList.toggle("hidden", !editing);
  $("#onboarding-finish").textContent = editing ? "Save profile" : "Start first update";
  $("#onboarding-error").textContent = "";
  $("#settings-panel").classList.add("hidden");
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
    ? `${count} source feed${count === 1 ? "" : "s"} selected · first update and calibration start automatically`
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
    $("#back-to-top").classList.toggle("hidden", top < 320);
  });
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
  try {
    const { session } = await api(`/api/sessions/${encodeURIComponent(state.session.id)}/cancel`, { method: "POST" });
    state.session = session;
    renderSession();
    stopPolling();
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
      await refreshTimeline();
      window.postMessage({ type: "AKU_BROWSER_RELEASE_CAPTURE_SURFACE", leaseId: session.id }, endpoint);
      if (session.status === "failed") showSessionFailure(session);
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
    const { items } = await api(`/api/timeline?limit=${limit}&offset=0`);
    renderTimeline(items ?? []);
  } catch (error) {
    showError(error);
  }
}

function renderTimeline(items) {
  const container = $("#result-items");
  container.replaceChildren();
  $("#timeline-meta").textContent = items.length
    ? `${items.length} retained update${items.length === 1 ? "" : "s"} · newest first`
    : "No completed update has been retained yet.";
  $("#finish-stats").textContent = items.length
    ? `Shown: ${items.length} · bounded local evidence · newest first`
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

  let previousSession = null;
  for (const entry of items) {
    if (entry.sessionId !== previousSession) {
      const marker = document.createElement("div");
      marker.className = "timeline-batch-marker";
      const checked = document.createElement("strong");
      checked.textContent = `Checked ${formatDate(entry.createdAt)}`;
      const detail = document.createElement("span");
      detail.textContent = "Unified X + LinkedIn";
      marker.append(checked, detail);
      container.append(marker);
      previousSession = entry.sessionId;
    }
    container.append(buildTimelineItem(entry));
  }
  scheduleBackToTop();
}

function buildTimelineItem(entry) {
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
  const text = document.createElement("p");
  text.textContent = evidence.text || item.whatChanged;
  content.append(text);
  const quote = buildQuotedPost(evidence.quotedPost, source);
  if (quote) content.append(quote);
  card.append(header, content);
  const media = buildMedia(evidence.media, source);
  if (media) card.append(media);
  if (evidence.mediaRecovery?.outcome === "unavailable") {
    const unavailable = document.createElement("div");
    unavailable.className = "source-layout-media-unavailable";
    unavailable.textContent = "Media was present at the source but unavailable in this captured view.";
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
  const text = document.createElement("p");
  text.textContent = value.text;
  quote.append(header, text);
  return quote;
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
  const link = document.createElement("a");
  link.className = "source-link";
  link.href = entry.item?.sourceUrl || entry.evidence?.permalink || "#";
  link.target = "_blank";
  link.rel = "noopener noreferrer";
  link.textContent = entry.item?.sourceUrlKind === "native_post" ? "Open native post" : "Open source evidence";
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
  return actions;
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
  $("#failure-message").textContent = session.error?.message || "The session stopped before both sources could finish.";
  $("#failure-panel").classList.remove("hidden");
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
  notice.textContent = error.message || String(error);
  notice.classList.remove("hidden");
  setPill("#sidecar-status", "AkuSidecar attention", "danger");
}

function clearNotice() {
  $("#provider-notice").classList.add("hidden");
  $("#provider-notice").textContent = "";
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

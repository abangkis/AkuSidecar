import { parseXSourceText } from "./x-source-format.js";

const state = {
  bootstrap: null,
  bridgeReady: false,
  currentRun: null,
  currentSession: null,
  pollTimer: null,
  externalSessionDiscoveryTimer: null,
  externalSessionDiscoveryInFlight: false,
  dispatchedRounds: new Set(),
  currentView: "timeline",
  timelineCapacity: 12,
  timelineFeed: null,
  defaultPresentation: "source",
  streamWidth: "social",
  telemetryBehavior: "flow",
  runtimeConfiguration: null,
  reviewPage: 0,
  reviewLoading: false,
  reviewHasNext: false,
  onboardingStep: 1,
};

const REVIEW_PAGE_SIZE = 10;
const REVIEW_MAX_RUNS = 50;

const elements = {
  sidecarStatus: document.querySelector("#sidecar-status"),
  bridgeStatus: document.querySelector("#bridge-status"),
  reasoningStatus: document.querySelector("#reasoning-status"),
  providerNotice: document.querySelector("#provider-notice"),
  sessionViewButton: document.querySelector("#session-view-button"),
  reviewViewButton: document.querySelector("#review-view-button"),
  settingsViewButton: document.querySelector("#settings-view-button"),
  onboardingPanel: document.querySelector("#onboarding-panel"),
  onboardingForm: document.querySelector("#onboarding-form"),
  onboardingStepLabel: document.querySelector("#onboarding-step-label"),
  onboardingInterests: document.querySelector("#onboarding-interests"),
  onboardingRefinement: document.querySelector("#onboarding-refinement"),
  onboardingContentTypes: document.querySelector("#onboarding-content-types"),
  onboardingSources: document.querySelector("#onboarding-sources"),
  onboardingSummary: document.querySelector("#onboarding-summary"),
  onboardingError: document.querySelector("#onboarding-error"),
  onboardingBack: document.querySelector("#onboarding-back"),
  onboardingNext: document.querySelector("#onboarding-next"),
  onboardingFinish: document.querySelector("#onboarding-finish"),
  editOnboardingProfile: document.querySelector("#edit-onboarding-profile"),
  settingsPanel: document.querySelector("#settings-panel"),
  timelineCapacity: document.querySelector("#timeline-capacity"),
  runtimeSettingsForm: document.querySelector("#runtime-settings-form"),
  missingSourceTabPolicy: document.querySelector("#missing-source-tab-policy"),
  defaultPresentation: document.querySelector("#default-presentation"),
  streamWidth: document.querySelector("#stream-width"),
  telemetryBehavior: document.querySelector("#telemetry-behavior"),
  maxItemsPerSource: document.querySelector("#max-items-per-source"),
  maxScrolls: document.querySelector("#max-scrolls"),
  maxAcquisitionRounds: document.querySelector("#max-acquisition-rounds"),
  maxKnowledgeContextEvents: document.querySelector("#max-knowledge-context-events"),
  fixedEngineConstraints: document.querySelector("#fixed-engine-constraints"),
  missingSourceTabDetail: document.querySelector("#missing-source-tab-detail"),
  reasoningProvider: document.querySelector("#reasoning-provider"),
  planningPolicy: document.querySelector("#planning-policy"),
  evaluationModel: document.querySelector("#evaluation-model"),
  evaluationEffort: document.querySelector("#evaluation-effort"),
  planningModel: document.querySelector("#planning-model"),
  planningEffort: document.querySelector("#planning-effort"),
  reasoningTimeout: document.querySelector("#reasoning-timeout"),
  startupSettingsDetail: document.querySelector("#startup-settings-detail"),
  runtimeSettingsStatus: document.querySelector("#runtime-settings-status"),
  saveRuntimeSettings: document.querySelector("#save-runtime-settings"),
  reviewPanel: document.querySelector("#review-panel"),
  overviewSummary: document.querySelector("#overview-summary"),
  overviewSources: document.querySelector("#overview-sources"),
  timelinePanel: document.querySelector("#timeline-panel"),
  timelineMeta: document.querySelector("#timeline-meta"),
  timelineRefreshButton: document.querySelector("#timeline-refresh-button"),
  timelineRunnerButton: document.querySelector("#timeline-runner-button"),
  reviewRefreshButton: document.querySelector("#review-refresh-button"),
  reviewMetrics: document.querySelector("#review-metrics"),
  sourceHealthStatus: document.querySelector("#source-health-status"),
  sourceHealthDetails: document.querySelector("#source-health-details"),
  reviewTokenUsage: document.querySelector("#review-token-usage"),
  preferenceReadinessStatus: document.querySelector("#preference-readiness-status"),
  preferenceReadinessGates: document.querySelector("#preference-readiness-gates"),
  preferenceReadinessDetail: document.querySelector("#preference-readiness-detail"),
  preferenceExperimentStatus: document.querySelector("#preference-experiment-status"),
  preferenceExperimentDetail: document.querySelector("#preference-experiment-detail"),
  shadowComparisonDetail: document.querySelector("#shadow-comparison-detail"),
  shadowCandidateList: document.querySelector("#shadow-candidate-list"),
  fitPreferenceExperiment: document.querySelector("#fit-preference-experiment"),
  reviewSourceFilter: document.querySelector("#review-source-filter"),
  reviewVerdictFilter: document.querySelector("#review-verdict-filter"),
  reviewMeta: document.querySelector("#review-meta"),
  reviewRuns: document.querySelector("#review-runs"),
  reviewScrollSentinel: document.querySelector("#review-scroll-sentinel"),
  processingPanel: document.querySelector("#processing-panel"),
  processingTitle: document.querySelector("#processing-title"),
  processingDetail: document.querySelector("#processing-detail"),
  progressBar: document.querySelector("#progress-bar"),
  cancelButton: document.querySelector("#cancel-button"),
  resultPanel: document.querySelector("#result-panel"),
  resultTitle: document.querySelector("#result-title"),
  resultMeta: document.querySelector("#result-meta"),
  coverageBadge: document.querySelector("#coverage-badge"),
  coverageContent: document.querySelector("#coverage-content"),
  resultSummary: document.querySelector("#result-summary"),
  resultItems: document.querySelector("#result-items"),
  finishTitle: document.querySelector("#finish-title"),
  finishStats: document.querySelector("#finish-stats"),
  doneButton: document.querySelector("#done-button"),
  failurePanel: document.querySelector("#failure-panel"),
  failureTitle: document.querySelector("#failure-title"),
  failureMessage: document.querySelector("#failure-message"),
  retryButton: document.querySelector("#retry-button"),
};

window.addEventListener("message", (event) => {
  if (event.source !== window || !event.data || typeof event.data !== "object") return;
  if (event.data.type === "AKU_BROWSER_BRIDGE_READY") {
    state.bridgeReady = true;
    setStatus(elements.bridgeStatus, "AkuBridge ready", "ok");
    api("/api/operations/bridge/heartbeat", {
      method: "POST",
      body: JSON.stringify({ capabilities: event.data.capabilities ?? {} }),
    }).catch(() => {
      setStatus(elements.bridgeStatus, "AkuBridge diagnostics pending", "warning");
    });
  }
  if (event.data.type === "AKU_BROWSER_BRIDGE_ERROR") {
    if (state.currentSession && !isUnifiedTerminal(state.currentSession.status)) {
      const activeChild = state.currentSession.children.find(
        (child) => child.run && !isTerminal(child.run.status),
      );
      if (activeChild?.run) {
        state.dispatchedRounds.delete(
          `${activeChild.run.id}:${activeChild.run.observations?.length ?? 0}`,
        );
      }
      setStatus(elements.bridgeStatus, "AkuBridge recovery pending", "warning");
      elements.processingDetail.textContent = event.data.message
        ? `AkuBridge dispatch failed: ${event.data.message}`
        : "A source action could not be verified. AkuSidecar may retry once with bounded detect-only capture.";
      return;
    }
    setStatus(elements.bridgeStatus, "AkuBridge error", "error");
    if (
      (state.currentRun && !isTerminal(state.currentRun.status)) ||
      (state.currentSession && !isUnifiedTerminal(state.currentSession.status))
    ) {
      reportRunFailure({
        stage: "browser_bridge",
        message: event.data.message || "AkuBridge could not dispatch the run.",
      });
    }
  }
});

elements.cancelButton.addEventListener("click", cancelCurrentRun);
elements.doneButton.addEventListener("click", startRun);
elements.retryButton.addEventListener("click", startRun);
elements.sessionViewButton.addEventListener("click", showSessionView);
elements.reviewViewButton.addEventListener("click", showReviewView);
elements.settingsViewButton.addEventListener("click", showSettingsView);
elements.runtimeSettingsForm.addEventListener("submit", saveRuntimeSettings);
elements.reviewRefreshButton.addEventListener("click", loadPilotReview);
elements.fitPreferenceExperiment.addEventListener("click", fitPreferenceExperiment);
elements.reviewSourceFilter.addEventListener("change", resetPilotReviewPage);
elements.reviewVerdictFilter.addEventListener("change", resetPilotReviewPage);
elements.timelineRunnerButton.addEventListener("click", startRun);
elements.timelineRefreshButton.addEventListener("click", refreshTimeline);
elements.onboardingBack.addEventListener("click", () => setOnboardingStep(state.onboardingStep - 1));
elements.onboardingNext.addEventListener("click", advanceOnboarding);
elements.onboardingForm.addEventListener("submit", saveOnboarding);
elements.editOnboardingProfile.addEventListener("click", () => showOnboarding(true));
await bootstrap();
observePilotReviewScroll();

async function bootstrap() {
  try {
    state.bootstrap = await api("/api/bootstrap");
    populateOnboarding(state.bootstrap.onboarding?.profile);
    state.defaultPresentation = state.bootstrap.presentation?.defaultLayout ?? "source";
    state.timelineCapacity = state.bootstrap.presentation?.timelineCapacity ?? 12;
    applyStreamWidth(state.bootstrap.presentation?.streamWidth ?? "social");
    applyTelemetryBehavior(state.bootstrap.presentation?.telemetryBehavior ?? "flow");
    setStatus(elements.sidecarStatus, "AkuSidecar ready", "ok");
    const reasoning = state.bootstrap.reasoning ?? {};
    setStatus(
      elements.reasoningStatus,
      `${friendlyModel(reasoning.evaluationModel || reasoning.model)} · eval ${reasoning.evaluationEffort || "default"}`,
      "neutral",
    );
    if (state.bootstrap.provider !== "codex-sdk") {
      elements.providerNotice.textContent =
        "Development fallback active. This run can verify plumbing, but its ranking must not be used to evaluate product quality.";
      elements.providerNotice.classList.remove("hidden");
    }
    pingBridge();
    setInterval(pingBridge, 30_000);
    if (state.bootstrap.onboarding?.status !== "completed") {
      showOnboarding(false);
      return;
    }
    const [{ session }, { timeline }] = await Promise.all([
      api("/api/sessions/active"),
      api(`/api/timeline?limit=${encodeURIComponent(state.timelineCapacity)}&offset=0`),
    ]);
    state.timelineFeed = timeline;
    if (session) {
      state.currentSession = session;
      showUnifiedProcessing(session);
      dispatchUnifiedSession(session);
      schedulePoll();
    } else {
      await loadTimelineFeed();
      showSessionView();
    }
    startExternalSessionDiscovery();
    setTimeout(() => {
      if (!state.bridgeReady) {
        setStatus(elements.bridgeStatus, "AkuBridge not detected", "warning");
      }
    }, 1_200);
  } catch (error) {
    setStatus(elements.sidecarStatus, "AkuSidecar unavailable", "error");
    setUpdateButtonsDisabled(true);
    elements.providerNotice.textContent = error.message;
    elements.providerNotice.classList.remove("hidden");
  }
}

function showOnboarding(editing) {
  state.onboardingStep = 1;
  setOnboardingStep(1);
  hide(elements.timelinePanel, elements.reviewPanel, elements.settingsPanel);
  show(elements.onboardingPanel);
  document.querySelector(".view-switch")?.classList.add("hidden");
  if (editing) populateOnboarding(state.bootstrap?.onboarding?.profile);
}

function populateOnboarding(profile) {
  if (!profile) {
    for (const input of elements.onboardingInterests.querySelectorAll("input")) input.checked = false;
    elements.onboardingRefinement.value = "";
    for (const input of elements.onboardingContentTypes.querySelectorAll("input")) {
      input.checked = ["announcement", "tutorial", "research", "discovery"].includes(input.value);
    }
    for (const input of elements.onboardingSources.querySelectorAll("input")) input.checked = true;
    return;
  }
  for (const input of elements.onboardingInterests.querySelectorAll("input")) {
    input.checked = profile.selectedInterests.includes(input.value);
  }
  elements.onboardingRefinement.value = profile.interestRefinement;
  for (const input of elements.onboardingContentTypes.querySelectorAll("input")) {
    input.checked = profile.preferredContentTypes.includes(input.value);
  }
  for (const input of elements.onboardingSources.querySelectorAll("input")) {
    input.checked = profile.activeSources.includes(input.value);
  }
}

function setOnboardingStep(step) {
  state.onboardingStep = Math.max(1, Math.min(4, step));
  for (const panel of document.querySelectorAll("[data-onboarding-step]")) {
    panel.classList.toggle("hidden", Number(panel.dataset.onboardingStep) !== state.onboardingStep);
  }
  elements.onboardingStepLabel.textContent = `Step ${state.onboardingStep} of 4`;
  elements.onboardingBack.classList.toggle("hidden", state.onboardingStep === 1);
  elements.onboardingNext.classList.toggle("hidden", state.onboardingStep === 4);
  elements.onboardingFinish.classList.toggle("hidden", state.onboardingStep !== 4);
  elements.onboardingError.textContent = "";
  if (state.onboardingStep === 4) {
    const profile = readOnboardingForm();
    elements.onboardingSummary.textContent = `${profile.selectedInterests.length} interest(s) · ${profile.interestRefinement ? "refinement added" : "broad interests only"} · ${profile.preferredContentTypes.length} content form(s) · ${profile.activeSources.length} source(s)`;
  }
}

function advanceOnboarding() {
  const profile = readOnboardingForm();
  if (state.onboardingStep === 1 && profile.selectedInterests.length === 0) {
    elements.onboardingError.textContent = "Choose at least one interest.";
    return;
  }
  if (state.onboardingStep === 3 && profile.preferredContentTypes.length === 0) {
    elements.onboardingError.textContent = "Choose at least one content form.";
    return;
  }
  setOnboardingStep(state.onboardingStep + 1);
}

function readOnboardingForm() {
  return {
    selectedInterests: [...elements.onboardingInterests.querySelectorAll("input:checked")].map((input) => input.value),
    interestRefinement: elements.onboardingRefinement.value.trim(),
    preferredContentTypes: [...elements.onboardingContentTypes.querySelectorAll("input:checked")].map((input) => input.value),
    activeSources: [...elements.onboardingSources.querySelectorAll("input:checked")].map((input) => input.value),
  };
}

async function saveOnboarding(event) {
  event.preventDefault();
  try {
    const { onboarding } = await api("/api/onboarding", {
      method: "PUT",
      body: JSON.stringify(readOnboardingForm()),
    });
    state.bootstrap.onboarding = onboarding;
    hide(elements.onboardingPanel);
    document.querySelector(".view-switch")?.classList.remove("hidden");
    await loadTimelineFeed();
    showSessionView();
    startExternalSessionDiscovery();
  } catch (error) {
    elements.onboardingError.textContent = error.message;
  }
}

function pingBridge() {
  window.postMessage(
    {
      type: "AKU_BROWSER_BRIDGE_PING",
      endpoint: window.location.origin,
    },
    window.location.origin,
  );
}

async function startRun() {
  if (
    (state.currentSession && !isUnifiedTerminal(state.currentSession.status)) ||
    (state.currentRun && !isTerminal(state.currentRun.status))
  ) return;
  state.currentView = "timeline";
  selectViewButton(elements.sessionViewButton);
  hide(elements.reviewPanel, elements.settingsPanel);
  show(elements.timelinePanel);
  clearPoll();
  state.dispatchedRounds.clear();
  hide(elements.failurePanel);
  setUpdateButtonsDisabled(true);
  try {
    const { session } = await api("/api/sessions", {
      method: "POST",
      body: JSON.stringify({}),
    });
    state.currentRun = null;
    state.currentSession = session;
    showUnifiedProcessing(session);
    dispatchUnifiedSession(session);
    schedulePoll();
  } catch (error) {
    setUpdateButtonsDisabled(false);
    showFailure({ stage: "create_session", message: error.message });
  }
}

function setUpdateButtonsDisabled(disabled) {
  elements.timelineRunnerButton.disabled = disabled;
  elements.doneButton.disabled = disabled;
  elements.retryButton.disabled = disabled;
}

function dispatchUnifiedSession(session) {
  const activeChild = session.children.find(
    (child) => child.run && !isTerminal(child.run.status),
  );
  if (activeChild?.run.status === "waiting_for_bridge") {
    dispatchToBridge(activeChild.run);
  }
}

function dispatchToBridge(run) {
  if (!state.bootstrap) return;
  const dispatchKey = `${run.id}:${run.observations?.length ?? 0}`;
  if (state.dispatchedRounds.has(dispatchKey)) return;
  state.dispatchedRounds.add(dispatchKey);
  window.postMessage(
    {
      type: "AKU_BROWSER_DISPATCH",
      endpoint: window.location.origin,
      token: state.bootstrap.bridgeToken,
      runId: run.id,
    },
    window.location.origin,
  );
}

function schedulePoll() {
  clearPoll();
  state.pollTimer = setTimeout(pollCurrent, 700);
}

function startExternalSessionDiscovery() {
  if (state.externalSessionDiscoveryTimer) return;
  state.externalSessionDiscoveryTimer = setInterval(async () => {
    if (
      state.externalSessionDiscoveryInFlight ||
      (state.currentRun && !isTerminal(state.currentRun.status)) ||
      (state.currentSession && !isUnifiedTerminal(state.currentSession.status))
    ) return;
    state.externalSessionDiscoveryInFlight = true;
    try {
      const { session } = await api("/api/sessions/active");
      if (!session) return;
      state.dispatchedRounds.clear();
      state.currentSession = session;
      if (state.currentView === "timeline") showUnifiedProcessing(session);
      dispatchUnifiedSession(session);
      schedulePoll();
    } catch {
      // Sidecar availability is reported by the normal status surface.
    } finally {
      state.externalSessionDiscoveryInFlight = false;
    }
  }, 1_000);
}

async function pollCurrent() {
  if (state.currentSession) await pollUnifiedSession();
  else await pollRun();
}

async function pollRun() {
  if (!state.currentRun) return;
  try {
    const { run } = await api(`/api/runs/${encodeURIComponent(state.currentRun.id)}`);
    state.currentRun = run;
    if (run.status === "waiting_for_bridge") dispatchToBridge(run);
    if (run.status === "completed") {
      clearPoll();
      setUpdateButtonsDisabled(false);
      if (state.currentView === "review") await loadPilotReview();
      else if (state.currentView === "timeline") showResult(run);
      return;
    }
    if (["failed", "cancelled"].includes(run.status)) {
      clearPoll();
      setUpdateButtonsDisabled(false);
      if (state.currentView === "review") await loadPilotReview();
      else if (state.currentView === "timeline") {
        showFailure(run.error ?? {
          stage: run.status,
          message: run.status === "cancelled" ? "The run was cancelled." : "The run failed.",
        });
      }
      return;
    }
    if (state.currentView === "timeline") showProcessing(run);
    schedulePoll();
  } catch (error) {
    clearPoll();
    reportRunFailure({ stage: "status", message: error.message });
  }
}

async function pollUnifiedSession() {
  if (!state.currentSession) return;
  try {
    const { session } = await api(
      `/api/sessions/${encodeURIComponent(state.currentSession.id)}`,
    );
    state.currentSession = session;
    dispatchUnifiedSession(session);
    if (isUnifiedTerminal(session.status)) {
      clearPoll();
      setUpdateButtonsDisabled(false);
      if (state.currentView === "review") await loadPilotReview();
      else if (
        state.currentView === "timeline" &&
        (session.status === "failed" || session.status === "cancelled")
      ) {
        setUpdateButtonsDisabled(false);
        showFailure({
          stage: session.status,
          message:
            session.status === "cancelled"
              ? "The bounded unified session was cancelled."
              : unifiedFailureMessage(session),
        });
      } else if (state.currentView === "timeline") {
        await loadTimelineFeed();
        showSessionView();
      }
      return;
    }
    if (state.currentView === "timeline") showUnifiedProcessing(session);
    schedulePoll();
  } catch (error) {
    clearPoll();
    reportRunFailure({ stage: "session_status", message: error.message });
  }
}

async function cancelCurrentRun() {
  if (!state.currentRun && !state.currentSession) return;
  elements.cancelButton.disabled = true;
  try {
    if (state.currentSession) {
      const { session } = await api(
        `/api/sessions/${encodeURIComponent(state.currentSession.id)}/cancel`,
        { method: "POST" },
      );
      state.currentSession = session;
      clearPoll();
      setUpdateButtonsDisabled(false);
      if (session.status === "partial") {
        await loadTimelineFeed();
        showSessionView();
      } else showFailure({ stage: "cancelled", message: "The bounded unified session was cancelled." });
      return;
    }
    const { run } = await api(`/api/runs/${encodeURIComponent(state.currentRun.id)}/cancel`, {
      method: "POST",
    });
    state.currentRun = run;
    clearPoll();
    showFailure({ stage: "cancelled", message: "The bounded run was cancelled." });
  } catch (error) {
    showFailure({ stage: "cancel", message: error.message });
  } finally {
    elements.cancelButton.disabled = false;
  }
}

function showProcessing(run) {
  showTimelineDuringProcessing();
  renderProcessingStep({ ...sourceStepState(run, 0), total: 7 });
  syncTimelineChrome();
}

function showUnifiedProcessing(session) {
  showTimelineDuringProcessing();
  const activeChild = session.children.find(
    (child) => child.run && !isTerminal(child.run.status),
  );
  const queuedIndex = session.children.findIndex((child) => child.status === "queued");
  const activeIndex = activeChild
    ? session.children.findIndex((child) => child === activeChild)
    : Math.max(0, queuedIndex);
  const totalSteps = session.children.length * 5 + 2;
  renderProcessingStep(activeChild?.run
    ? { ...sourceStepState(activeChild.run, activeIndex), total: totalSteps }
    : {
        step: activeIndex === 0 ? 1 : 6,
        label: `Preparing ${sourceLabel(session.children[activeIndex]?.source ?? "x")} source`,
        total: totalSteps,
      });
  syncTimelineChrome();
}

function showTimelineDuringProcessing() {
  show(elements.timelinePanel);
  hide(elements.failurePanel);
  if (elements.resultPanel.classList.contains("hidden") && state.timelineFeed) {
    renderTimelineFeed(state.timelineFeed);
  }
  show(elements.processingPanel);
  setUpdateButtonsDisabled(true);
}

function sourceStepState(run, sourceIndex) {
  const offset = sourceIndex === 0 ? 0 : 5;
  const label = sourceLabel(run.source);
  return {
    queued: { step: 1 + offset, label: `Preparing ${label} source` },
    waiting_for_bridge: { step: 2 + offset, label: `Opening ${label} source` },
    capturing: { step: 3 + offset, label: `Reading ${label} source` },
    reasoning: { step: 5 + offset, label: `Evaluating ${label} updates` },
  }[run.status] ?? { step: 1 + offset, label: `Processing ${label} source` };
}

function renderProcessingStep({ step, label, total = 12 }) {
  const safeTotal = Math.max(2, total);
  const safeStep = Math.max(1, Math.min(safeTotal, step));
  const progress = Math.round((safeStep / safeTotal) * 100);
  elements.processingTitle.textContent = label;
  elements.processingDetail.textContent = `${safeStep}/${safeTotal} steps`;
  elements.progressBar.style.width = `${progress}%`;
  elements.processingPanel
    .querySelector(".progress-track")
    ?.setAttribute("aria-valuenow", String(progress));
}

function showResult(run) {
  elements.resultPanel.classList.remove("timeline-feed-mode");
  show(elements.timelinePanel);
  hide(elements.processingPanel, elements.failurePanel);
  show(elements.resultPanel);
  elements.resultTitle.textContent = `${humanize(run.mode)} snapshot complete`;
  elements.resultMeta.textContent = `${run.source === "x" ? "X" : "LinkedIn"} · as of ${formatDate(run.completedAt)}`;

  const coverage = run.coverage ?? {};
  const partial = coverage.status !== "complete_within_scope";
  elements.coverageBadge.textContent = partial ? "Coverage partial" : "Complete within scope";
  elements.coverageBadge.classList.toggle("status-ok", !partial);
  elements.coverageContent.replaceChildren(buildCoverageList(coverage));
  elements.resultSummary.textContent = run.result?.summary || "No summary was returned.";
  elements.resultItems.replaceChildren();
  for (const item of run.result?.items ?? []) {
    elements.resultItems.append(buildResultItem(run, item));
  }
  if ((run.result?.items?.length ?? 0) === 0) {
    const empty = document.createElement("p");
    empty.className = "result-summary";
    empty.textContent = "No visible item was promoted in this bounded sample.";
    elements.resultItems.append(empty, buildEmptyResultFeedback(run));
  }
  elements.finishTitle.textContent = partial
    ? "Brief complete—coverage partial"
    : "Brief complete within selected scope";
  elements.finishStats.textContent = [
    `Shown: ${run.result?.items?.length ?? 0}`,
    `Repeated claims collapsed: ${run.result?.repeatedClaimsCollapsed ?? 0}`,
    `Deferred by budget: ${run.result?.deferredByBudget ?? 0}`,
  ].join(" · ");
  syncTimelineChrome();
}

function showUnifiedResult(session) {
  elements.resultPanel.classList.remove("timeline-feed-mode");
  show(elements.timelinePanel);
  hide(elements.processingPanel, elements.failurePanel);
  show(elements.resultPanel);
  setStatus(elements.bridgeStatus, "AkuBridge ready", "ok");
  elements.resultTitle.textContent = `${humanize(session.mode)} unified brief`;
  elements.resultMeta.textContent = `X + LinkedIn · ${session.result?.items?.length ?? 0} item(s) · as of ${formatDate(session.completedAt)}`;
  const partial = session.status !== "completed";
  elements.coverageBadge.textContent = partial
    ? "Coverage partial"
    : "Both source runs completed";
  elements.coverageBadge.classList.toggle("status-ok", !partial);
  elements.coverageContent.replaceChildren(buildUnifiedCoverage(session));
  elements.resultSummary.textContent = session.result?.summary || "No summary was returned.";
  elements.resultItems.replaceChildren();

  for (const entry of session.result?.items ?? []) {
    const child = session.children.find((candidate) => candidate.runId === entry.runId);
    if (child?.run) elements.resultItems.append(buildResultItem(child.run, entry.item));
  }
  for (const child of session.children.filter(
    (candidate) =>
      candidate.run?.status === "completed" &&
      (candidate.run.result?.items?.length ?? 0) === 0,
  )) {
    const outcome = document.createElement("section");
    outcome.className = "source-outcome";
    const title = document.createElement("h3");
    title.textContent = `${sourceLabel(child.source)} · no item promoted`;
    const summary = document.createElement("p");
    summary.textContent = child.run.result?.summary ?? "No material item was promoted.";
    outcome.append(title, summary, buildEmptyResultFeedback(child.run));
    elements.resultItems.append(outcome);
  }

  elements.finishTitle.textContent =
    session.mode === "catch_up" ? "End of catch-up" : "End of live brief";
  elements.finishStats.textContent = [
    `Shown: ${session.result?.items?.length ?? 0}`,
    `X: ${session.coverage?.resultCountBySource?.x ?? 0}`,
    `LinkedIn: ${session.coverage?.resultCountBySource?.linkedin ?? 0}`,
    partial ? "One or more sources incomplete" : "Both source runs completed",
  ].join(" · ");
  syncTimelineChrome();
}

function renderTimelineFeed(timeline) {
  elements.resultPanel.classList.add("timeline-feed-mode");
  hide(elements.processingPanel, elements.failurePanel);
  show(elements.resultPanel);
  setUpdateButtonsDisabled(false);
  elements.resultTitle.textContent = "Bounded knowledge timeline";
  elements.resultMeta.textContent = `${timeline.entries.length} retained update(s) · capacity ${timeline.capacity}`;
  elements.coverageBadge.textContent = "Newest first";
  elements.coverageBadge.classList.add("status-ok");

  const coverage = document.createElement("ul");
  for (const value of [
    `Rolling capacity: ${timeline.capacity}`,
    `Retained updates: ${timeline.summary.retained}`,
    `Sessions scanned: ${timeline.summary.sessionsScanned}`,
    `X retained: ${timeline.summary.sources?.x ?? 0}`,
    `LinkedIn retained: ${timeline.summary.sources?.linkedin ?? 0}`,
    "New evaluated updates enter first; the oldest retained items leave when capacity is full.",
  ]) {
    const item = document.createElement("li");
    item.textContent = value;
    coverage.append(item);
  }
  elements.coverageContent.replaceChildren(coverage);
  elements.resultSummary.textContent = timeline.entries.length > 0
    ? "The latest evaluated updates across completed bounded checks."
    : "No update has been retained yet. Check active sources to establish the timeline.";
  elements.resultItems.replaceChildren();

  let previousSessionId = null;
  for (const entry of timeline.entries) {
    if (entry.sessionId !== previousSessionId) {
      const marker = document.createElement("div");
      marker.className = "timeline-batch-marker";
      const label = document.createElement("strong");
      label.textContent = `Checked ${formatDate(entry.sessionCompletedAt)}`;
      const detail = document.createElement("span");
      detail.textContent = "Unified X + LinkedIn";
      marker.append(label, detail);
      elements.resultItems.append(marker);
      previousSessionId = entry.sessionId;
    }
    const timelineItem = buildResultItem(
      entry.run,
      entry.item,
      loadTimelineFeed,
      { preferenceOnly: true },
    );
    if (entry.isLatestAddition) timelineItem.classList.add("timeline-new-item");
    elements.resultItems.append(timelineItem);
  }

  elements.finishTitle.textContent = "You’re caught up within this timeline";
  elements.finishStats.textContent = [
    formatLatestAdditions(timeline.summary.latestAdditions),
  ].join("");
  syncTimelineChrome();
}

function buildUnifiedCoverage(session) {
  const container = document.createElement("div");
  const summary = document.createElement("ul");
  for (const child of session.children) {
    const item = document.createElement("li");
    item.textContent = `${sourceLabel(child.source)}: ${humanize(child.status)} · ${child.run?.result?.items?.length ?? 0} shown`;
    summary.append(item);
  }
  const scope = document.createElement("p");
  scope.textContent = session.coverage?.scopeStatement ?? "Bounded source samples.";
  container.append(summary, scope);
  for (const child of session.children.filter((candidate) => candidate.run?.coverage)) {
    const details = document.createElement("details");
    const label = document.createElement("summary");
    label.textContent = `${sourceLabel(child.source)} acquisition details`;
    details.append(label, buildCoverageList(child.run.coverage));
    container.append(details);
  }
  return container;
}

function buildEmptyResultFeedback(run, onSaved = () => {}) {
  const container = document.createElement("div");
  container.className = "empty-result-feedback";

  if (
    run.coverage?.status === "unavailable" ||
    !runHasDisplayEvidence(run)
  ) {
    const unavailable = document.createElement("p");
    unavailable.textContent =
      "This source captured no visible evidence, so the empty result cannot be rated.";
    container.append(unavailable);
    return container;
  }

  const prompt = document.createElement("p");
  prompt.textContent = "Was this empty result correct?";

  const actions = document.createElement("div");
  actions.className = "feedback-actions";
  const previous = new Set(
    (run.feedback ?? [])
      .filter((entry) => !entry.itemId)
      .map((entry) => entry.kind),
  );

  const verdict = (run.feedback ?? []).find(
    (entry) => !entry.itemId && ["correct_empty", "missed"].includes(entry.kind),
  );
  for (const [kind, label] of [["correct_empty", "Correctly empty"]]) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "feedback-button";
    button.textContent = label;
    if (previous.has(kind)) {
      button.classList.add("selected");
    }
    button.disabled = Boolean(verdict);
    button.addEventListener("click", async () => {
      const response = await api(`/api/runs/${encodeURIComponent(run.id)}/feedback`, {
        method: "POST",
        body: JSON.stringify({ kind }),
      });
      run.feedback = response.run.feedback;
      for (const candidate of actions.querySelectorAll("button")) {
        candidate.disabled = true;
      }
      button.classList.add("selected");
      await onSaved();
    });
    actions.append(button);
  }

  const missedButton = document.createElement("button");
  missedButton.type = "button";
  missedButton.className = "feedback-button";
  missedButton.textContent = "Missed something";
  if (previous.has("missed")) missedButton.classList.add("selected");
  missedButton.disabled = Boolean(verdict);
  actions.append(missedButton);

  const missedForm = document.createElement("form");
  missedForm.className = "missed-feedback-form hidden";
  const noteLabel = document.createElement("label");
  noteLabel.textContent = "What should have appeared? Description or post URL";
  const note = document.createElement("textarea");
  note.rows = 2;
  note.maxLength = 500;
  note.required = true;
  note.placeholder = "What important information was missed?";
  noteLabel.append(note);
  const missedError = document.createElement("p");
  missedError.className = "feedback-error hidden";
  const formActions = document.createElement("div");
  formActions.className = "feedback-actions";
  const submit = document.createElement("button");
  submit.type = "submit";
  submit.className = "secondary-button";
  submit.textContent = "Save missed note";
  const cancel = document.createElement("button");
  cancel.type = "button";
  cancel.className = "text-button";
  cancel.textContent = "Cancel";
  formActions.append(cancel, submit);
  missedForm.append(noteLabel, missedError, formActions);
  missedButton.addEventListener("click", () => {
    missedForm.classList.remove("hidden");
    note.focus();
  });
  cancel.addEventListener("click", () => {
    missedForm.classList.add("hidden");
    missedError.classList.add("hidden");
    note.value = "";
  });
  missedForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const value = note.value.trim();
    if (!value) {
      missedError.textContent = "Describe the important information that was missed.";
      missedError.classList.remove("hidden");
      return;
    }
    submit.disabled = true;
    try {
      const response = await api(`/api/runs/${encodeURIComponent(run.id)}/feedback`, {
        method: "POST",
        body: JSON.stringify({ kind: "missed", note: value }),
      });
      run.feedback = response.run.feedback;
      missedButton.classList.add("selected");
      for (const candidate of actions.querySelectorAll("button")) candidate.disabled = true;
      missedForm.classList.add("hidden");
      await onSaved();
    } catch (error) {
      missedError.textContent = error.message;
      missedError.classList.remove("hidden");
    } finally {
      submit.disabled = false;
    }
  });

  container.append(prompt, actions, missedForm);

  if (verdict?.note) {
    const savedNote = document.createElement("p");
    savedNote.className = "saved-feedback-note";
    savedNote.textContent = verdict.note;
    container.append(savedNote);
  }
  return container;
}

function buildCoverageList(coverage) {
  const list = document.createElement("ul");
  const values = [
    `Scope: ${coverage.scopeStatement ?? "Bounded browser sample."}`,
    Number.isInteger(coverage.acquisitionRounds)
      ? `Acquisition rounds: ${coverage.acquisitionRounds} of ${state.bootstrap?.limits?.maxAcquisitionRounds ?? coverage.acquisitionRounds}`
      : null,
    coverage.providerFollowUpRequested
      ? `Provider follow-up: ${coverage.providerFollowUpExecuted ? "executed" : "requested but not executable"}${coverage.providerFollowUpReason ? ` — ${coverage.providerFollowUpReason}` : ""}`
      : "Provider follow-up: not requested",
    coverage.previousCheckpointAt
      ? `Previous checkpoint: ${formatDate(coverage.previousCheckpointAt)}`
      : "Previous checkpoint: none; this run establishes the frontier",
    Number.isInteger(coverage.exactDuplicatesSuppressed)
      ? `Exact evidence suppressed: ${coverage.exactDuplicatesSuppressed}`
      : null,
    Number.isInteger(coverage.deliveredEvidenceSuppressed)
      ? `Previously delivered evidence suppressed: ${coverage.deliveredEvidenceSuppressed}`
      : null,
    Number.isInteger(coverage.confirmedExcludedSuppressed)
      ? `User-confirmed exclusions suppressed for this intent: ${coverage.confirmedExcludedSuppressed}`
      : null,
    Number.isInteger(coverage.unseenEvidenceCount)
      ? `Unseen evidence at reasoning: ${coverage.unseenEvidenceCount}`
      : null,
    Number.isInteger(coverage.knowledgeContextEvents)
      ? `Prior frontier events supplied: ${coverage.knowledgeContextEvents}`
      : null,
    coverage.checkpointAdvanced ? "Checkpoint: advanced after this completed run" : null,
    `Snapshots: ${coverage.snapshotCount ?? 0}`,
    `Visible candidates observed: ${coverage.candidateCount ?? 0}`,
    coverage.browserAdapter ? `Browser adapter: ${coverage.browserAdapter}` : null,
    coverage.scrollContainer ? `Scroll container: ${coverage.scrollContainer}` : null,
    Number.isInteger(coverage.requestedScrolls)
      ? `Native scrolls: ${coverage.performedScrolls ?? 0} of ${coverage.requestedScrolls}`
      : null,
    coverage.scrollStopReason ? `Scroll stop reason: ${humanize(coverage.scrollStopReason)}` : null,
    coverage.restoreAttempted
      ? `Starting position restored: ${coverage.restored ? "yes" : "no"}`
      : null,
    coverage.browserAdapter
      ? `Computer Use fallback: ${coverage.fallbackUsed ? "used" : "not used"}`
      : null,
    coverage.pendingNewContentAction
      ? coverage.pendingNewContent
        ? `Pending new content: ${coverage.pendingNewContentLabel || "detected"} (${humanize(coverage.pendingNewContentAction)})`
        : "Pending new content: not detected"
      : null,
    coverage.pendingContentPolicy
      ? `Pending-content policy: ${humanize(coverage.pendingContentPolicy)}`
      : null,
    coverage.sourceReadinessState
      ? `Source readiness: ${humanize(coverage.sourceReadinessState)} after ${formatDuration(coverage.sourceReadinessWaitMs)} · ${coverage.sourceVisibleSelectorCandidateCount ?? 0}/${coverage.sourceSelectorCandidateCount ?? 0} visible selectors`
      : null,
    coverage.sourceTabOpened ? "Source tab: opened by AkuBridge" : null,
    coverage.sourceTabActivatedForReadiness
      ? "Source tab: temporarily activated for readiness"
      : null,
    coverage.sourceTabBackgroundAtDispatch ? "Source tab started in background" : null,
    coverage.sourceTabRecoveryCount > 0
      ? `Stale source-tab recoveries: ${coverage.sourceTabRecoveryCount}`
      : null,
    Number.isInteger(coverage.sourceReadinessRetryCount)
      ? `Source readiness retries: ${coverage.sourceReadinessRetryCount}`
      : null,
    coverage.pendingContentRecovery
      ? `Pending-content recovery: ${humanize(coverage.pendingContentRecovery)}`
      : null,
    coverage.pendingContentActivationEvidence
      ? `Fresh-content activation evidence: ${humanize(coverage.pendingContentActivationEvidence)}`
      : null,
    coverage.feedMutation
      ? `Feed mutation: same source tab changed; pre-action position ${coverage.preActionScrollY}, capture baseline ${coverage.originalScrollY}, final ${coverage.finalScrollY}`
      : coverage.pendingContentPolicy
        ? "Feed mutation: none"
        : null,
    coverage.restorationScope
      ? `Restoration scope: ${humanize(coverage.restorationScope)}`
      : null,
    `Checked through: ${formatDate(coverage.checkedThrough)}`,
    `Reasoning provider: ${coverage.provider ?? "unknown"}`,
    ...(coverage.notes ?? []),
  ].filter(Boolean);
  for (const value of values) {
    const item = document.createElement("li");
    item.textContent = value;
    list.append(item);
  }
  return list;
}

function buildResultItem(run, item, onSaved = () => {}, options = {}) {
  const brief = document.createElement("div");
  brief.className = "result-item item-layout-view";

  const header = document.createElement("div");
  header.className = "result-item-header";
  const evidence = document.createElement("span");
  evidence.className = "evidence-badge";
  evidence.textContent = [
    humanize(item.evidenceState),
    `${Math.round(item.confidence * 100)}%`,
    item.knowledgeDelta ? humanize(item.knowledgeDelta) : null,
  ]
    .filter(Boolean)
    .join(" · ");
  header.append(evidence);

  const title = document.createElement("h3");
  title.textContent = item.whatChanged;
  const why = document.createElement("p");
  why.className = "why-it-matters";
  why.textContent = item.whyItMatters;
  const provenance = document.createElement("p");
  provenance.className = "provenance";
  provenance.textContent = [
    item.source === "x" ? "X" : "LinkedIn",
    item.author,
    item.publishedAt ? formatDate(item.publishedAt) : "publication time unavailable",
  ]
    .filter(Boolean)
    .join(" · ");

  const candidate = (run.candidateEvaluations ?? []).find(
    (entry) => entry.evidenceKey === item.evidenceKey,
  );
  const actions = buildResultItemActions(run, item, candidate, onSaved, options);
  brief.append(header, title, why, provenance);
  return buildItemPresentation({
    brief,
    source: buildSourceLayoutCard(run, item, candidate),
    actions,
    className: "promoted-item-presentation",
  });
}

function buildResultItemActions(run, item, candidate, onSaved, options = {}) {
  const actions = document.createElement("div");
  actions.className = "result-actions";
  const link = document.createElement("a");
  link.className = "source-link";
  link.href = item.sourceUrl;
  link.target = "_blank";
  link.rel = "noreferrer noopener";
  link.textContent = provenanceLinkLabel(item.sourceUrlKind);

  const feedback = document.createElement("div");
  feedback.className = "feedback-actions";
  const previous = new Set(
    (run.feedback ?? []).filter((entry) => entry.itemId === item.id).map((entry) => entry.kind),
  );
  for (const [kind, label] of options.preferenceOnly ? [] : [
    ["useful", "Useful"],
    ["correct_lane", "Correct lane"],
    ["wrong_lane", "Wrong lane"],
    ["duplicate", "Duplicate"],
  ]) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "feedback-button";
    button.textContent = label;
    if (previous.has(kind)) {
      button.classList.add("selected");
      button.disabled = true;
    }
    button.addEventListener("click", async () => {
      const response = await api(`/api/runs/${encodeURIComponent(run.id)}/feedback`, {
        method: "POST",
        body: JSON.stringify({ kind, itemId: item.id }),
      });
      run.feedback = response.run.feedback;
      button.classList.add("selected");
      button.disabled = true;
      await onSaved();
    });
    feedback.append(button);
  }
  if (candidate) {
    feedback.append(
      buildPreferenceButton(run, item.evidenceKey, "more_like_this", "More like this", onSaved),
      buildPreferenceButton(run, item.evidenceKey, "less_like_this", "Less like this", onSaved),
    );
  }
  actions.append(link, feedback);
  return actions;
}

function buildPreferenceButton(run, evidenceKey, kind, label, onSaved = () => {}) {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "feedback-button";
  button.textContent = label;
  button.dataset.preferenceEvidence = evidenceKey;
  button.dataset.preferenceKind = kind;
  if (effectivePreferenceKind(run, evidenceKey) === kind) {
    button.classList.add("selected");
    button.disabled = true;
  }
  button.addEventListener("click", async () => {
    const response = await api(`/api/runs/${encodeURIComponent(run.id)}/preference-feedback`, {
      method: "POST",
      body: JSON.stringify({ kind, evidenceKey, reasonCode: null, note: "" }),
    });
    run.preferenceFeedback = response.run.preferenceFeedback;
    syncPreferenceButtons(run, evidenceKey);
    await onSaved();
  });
  return button;
}

function effectivePreferenceKind(run, evidenceKey) {
  const latest = (run.preferenceFeedback ?? [])
    .filter((entry) => entry.evidenceKey === evidenceKey)
    .at(-1)?.kind;
  return latest;
}

function syncPreferenceButtons(run, evidenceKey) {
  const effective = effectivePreferenceKind(run, evidenceKey);
  for (const button of document.querySelectorAll("[data-preference-evidence]")) {
    if (button.dataset.preferenceEvidence !== evidenceKey) continue;
    const selected = button.dataset.preferenceKind === effective;
    button.classList.toggle("selected", selected);
    button.disabled = selected;
  }
}

function buildItemPresentation({ brief, source, actions, className = "" }) {
  const container = document.createElement("article");
  container.className = `presentable-item ${className}`.trim();
  const toolbar = document.createElement("div");
  toolbar.className = "item-presentation-toolbar";
  const note = document.createElement("span");
  note.textContent = "Captured evidence · not a live source copy";
  const toggle = document.createElement("button");
  toggle.type = "button";
  toggle.className = "presentation-toggle";
  let layout = state.defaultPresentation;
  const render = () => {
    const sourceActive = layout === "source";
    brief.classList.toggle("hidden", sourceActive);
    source.classList.toggle("hidden", !sourceActive);
    toggle.textContent = sourceActive ? "Switch to brief" : "Switch to source layout";
    toggle.setAttribute("aria-pressed", String(sourceActive));
  };
  toggle.addEventListener("click", () => {
    layout = layout === "source" ? "brief" : "source";
    render();
  });
  toolbar.append(note, toggle);
  container.append(toolbar, brief, source, actions);
  render();
  return container;
}

function buildSourceLayoutCard(run, item, candidate) {
  const source = item.source || candidate?.source || run.source;
  const article = document.createElement("div");
  article.className = `source-layout-card source-${source}`;

  const header = document.createElement("header");
  const identity = document.createElement("div");
  const author = document.createElement("strong");
  author.textContent = candidate?.author || item.author || sourceLabel(source);
  const context = document.createElement("span");
  context.textContent = [
    sourceLabel(source),
    item.publishedAt ? formatDate(item.publishedAt) : "Captured in this run",
  ].join(" · ");
  identity.append(author, context);
  const sourceBadge = document.createElement("span");
  sourceBadge.className = "source-layout-badge";
  sourceBadge.textContent = source === "x" ? "X" : "in";
  header.append(sourceBadge, identity);

  const content = candidate?.text
    ? source === "x"
      ? buildXSourceLayoutContent(candidate)
      : buildCandidateContent({ ...candidate, source }, { includeIdentity: false })
    : document.createElement("div");
  content.classList.add("source-layout-content");
  if (!candidate?.text) {
    const paragraph = document.createElement("p");
    paragraph.textContent = item.whatChanged;
    content.append(paragraph);
  }

  const media = buildSourceLayoutMedia(candidate?.media ?? [], source);
  article.append(header, content);
  if (media) {
    const quote = content.querySelector(".x-quote-card");
    (quote ?? article).append(media);
  }
  return article;
}

function buildXSourceLayoutContent(candidate) {
  const parsed = parseXSourceText(candidate);
  const content = document.createElement("div");
  content.className = "candidate-content source-layout-content x-source-content";
  if (parsed.socialContext) {
    const context = document.createElement("span");
    context.className = "candidate-context x-social-context";
    context.textContent = parsed.socialContext;
    content.append(context);
  }
  const body = document.createElement("p");
  body.textContent = parsed.body;
  content.append(body);
  if (parsed.quote) {
    const quote = document.createElement("section");
    quote.className = "x-quote-card";
    const identity = document.createElement("strong");
    identity.textContent = parsed.quote.identity;
    const quoteBody = document.createElement("p");
    quoteBody.textContent = parsed.quote.body;
    quote.append(identity, quoteBody);
    content.append(quote);
  }
  return content;
}

function buildSourceLayoutMedia(entries, source) {
  const media = [];
  for (const entry of entries.slice(0, 4)) {
    const url = safePresentationMediaUrl(entry?.url, source);
    if (!url) continue;
    const figure = document.createElement("figure");
    figure.className = "source-layout-media-item";
    const image = document.createElement("img");
    image.src = url;
    image.alt = entry.alt || "Captured post image";
    image.loading = "lazy";
    image.decoding = "async";
    image.referrerPolicy = "no-referrer";
    image.addEventListener("error", () => figure.remove(), { once: true });
    figure.append(image);
    media.push(figure);
  }
  if (media.length === 0) return null;
  const gallery = document.createElement("div");
  gallery.className = `source-layout-media media-count-${media.length}`;
  gallery.append(...media);
  return gallery;
}

function safePresentationMediaUrl(value, source) {
  if (typeof value !== "string") return null;
  try {
    const url = new URL(value);
    if (url.protocol !== "https:") return null;
    const host = url.hostname.toLowerCase();
    if (source === "x" && !["pbs.twimg.com", "video.twimg.com"].includes(host)) return null;
    if (source === "linkedin" && host !== "licdn.com" && !host.endsWith(".licdn.com")) return null;
    return url.href;
  } catch {
    return null;
  }
}

function showSessionView() {
  state.currentView = "timeline";
  selectViewButton(elements.sessionViewButton);
  hide(elements.reviewPanel, elements.settingsPanel);
  show(elements.timelinePanel);
  if (state.currentSession && isUnifiedTerminal(state.currentSession.status)) {
    if (!["completed", "partial"].includes(state.currentSession.status)) {
      showFailure({
        stage: state.currentSession.status,
        message: unifiedFailureMessage(state.currentSession),
      });
    } else if (state.timelineFeed) renderTimelineFeed(state.timelineFeed);
  } else if (state.currentSession) {
    showUnifiedProcessing(state.currentSession);
  } else if (state.currentRun?.status === "completed") {
    showResult(state.currentRun);
  } else if (state.currentRun && !isTerminal(state.currentRun.status)) {
    showProcessing(state.currentRun);
  } else if (state.currentRun?.status === "failed") {
    showFailure(state.currentRun.error ?? { stage: "run", message: "The run failed." });
  } else if (state.timelineFeed) {
    renderTimelineFeed(state.timelineFeed);
  } else {
    renderTimelineFeed({
      capacity: state.timelineCapacity,
      entries: [],
      summary: { retained: 0, sessionsScanned: 0, newestSessionAt: null, sources: {} },
    });
  }
  syncTimelineChrome();
}

async function showReviewView() {
  state.currentView = "review";
  selectViewButton(elements.reviewViewButton);
  hide(elements.timelinePanel, elements.settingsPanel);
  show(elements.reviewPanel);
  await loadPilotReview();
}

async function showSettingsView() {
  state.currentView = "settings";
  selectViewButton(elements.settingsViewButton);
  hide(elements.timelinePanel, elements.reviewPanel);
  show(elements.settingsPanel);
  await Promise.all([loadRuntimeSettings(), loadOverview()]);
}

function selectViewButton(selected) {
  for (const button of [
    elements.sessionViewButton,
    elements.reviewViewButton,
    elements.settingsViewButton,
  ]) {
    button.classList.toggle("selected", button === selected);
  }
}

async function loadTimelineFeed() {
  const { timeline } = await api(
    `/api/timeline?limit=${encodeURIComponent(state.timelineCapacity)}&offset=0`,
  );
  state.timelineFeed = timeline;
  state.currentSession = null;
  state.currentRun = null;
  return timeline;
}

async function refreshTimeline() {
  elements.timelineRefreshButton.disabled = true;
  elements.timelineMeta.textContent = "Refreshing the bounded timeline…";
  try {
    await loadTimelineFeed();
    showSessionView();
  } catch (error) {
    elements.timelineMeta.textContent = error.message;
  } finally {
    elements.timelineRefreshButton.disabled = false;
  }
}

function syncTimelineChrome() {
  elements.timelineRunnerButton.textContent = "Check for updates";
  if (state.currentSession && !isUnifiedTerminal(state.currentSession.status)) {
    elements.timelineMeta.textContent = "Checking active sources within the bounded acquisition policy.";
  } else if (state.timelineFeed) {
    elements.timelineMeta.textContent = state.timelineFeed.summary.newestSessionAt
      ? `${formatLatestAdditions(state.timelineFeed.summary.latestAdditions)} · checked ${formatDate(state.timelineFeed.summary.newestSessionAt)}`
      : "No update has run yet.";
  } else if (state.currentRun?.status === "completed") {
    elements.timelineMeta.textContent = `${sourceLabel(state.currentRun.source)} · completed ${formatDate(state.currentRun.completedAt)}`;
  } else if (!state.currentSession && !state.currentRun) {
    elements.timelineMeta.textContent = "No retained updates yet. Run a bounded check to establish the timeline.";
  } else {
    elements.timelineMeta.textContent = "A bounded source check is in progress.";
  }
}

function formatLatestAdditions(value) {
  const count = Number.isInteger(value) && value >= 0 ? value : 0;
  return `${count} ${count === 1 ? "addition" : "additions"}`;
}

async function loadOverview() {
  elements.overviewSummary.textContent = "Loading source state…";
  try {
    const [{ bridge }, timeline] = await Promise.all([
      api("/api/operations/bridge/health"),
      api("/api/sessions?limit=1&offset=0"),
    ]);
    const latest = timeline.sessions[0] ?? null;
    renderOverviewSummary(latest, bridge);
    renderOverviewSources(state.bootstrap?.sourceRegistry ?? [], bridge?.sources ?? {});
  } catch (error) {
    elements.overviewSummary.textContent = error.message;
    elements.overviewSources.replaceChildren();
  }
}

function renderOverviewSummary(latest, bridge) {
  const activeCount = (state.bootstrap?.sourceRegistry ?? [])
    .filter((source) => source.activationState === "active").length;
  const values = [
    ["Active sources", String(activeCount)],
    ["Available adapters", String(state.bootstrap?.sourceRegistry?.length ?? 0)],
    ["Latest update", latest ? formatDate(latest.completedAt) : "Not run yet"],
  ];
  elements.overviewSummary.replaceChildren(...values.map(([label, value]) => {
    const card = document.createElement("article");
    const term = document.createElement("span");
    term.textContent = label;
    const detail = document.createElement("strong");
    detail.textContent = value;
    card.append(term, detail);
    return card;
  }));
}

function renderOverviewSources(registry, healthBySource) {
  const cards = registry.map((source) => {
    const health = healthBySource[source.id] ?? {};
    const card = document.createElement("article");
    card.className = "overview-source-card";
    const header = document.createElement("div");
    const title = document.createElement("h3");
    title.textContent = source.label;
    const status = document.createElement("span");
    status.className = `status-pill ${health.status === "healthy" ? "status-ok" : "status-neutral"}`;
    status.textContent = humanize(health.status ?? "not observed");
    const toggleLabel = document.createElement("label");
    toggleLabel.className = "source-toggle";
    const toggle = document.createElement("input");
    toggle.type = "checkbox";
    toggle.value = source.id;
    toggle.dataset.sourceToggle = "true";
    toggle.checked = source.activationState === "active";
    const toggleText = document.createElement("span");
    toggleText.textContent = toggle.checked ? "Included" : "Excluded";
    toggle.addEventListener("change", () => {
      toggleText.textContent = toggle.checked ? "Included" : "Excluded";
    });
    toggleLabel.append(toggle, toggleText);
    header.append(title, status, toggleLabel);
    const description = document.createElement("p");
    description.textContent = `${humanize(source.behavior)} source · ${humanize(source.accessMode)}`;
    const facts = document.createElement("dl");
    for (const [label, value] of [
      ["Collection", humanize(source.collectionPolicy)],
      ["Last checked", health.lastObservedAt ? formatDate(health.lastObservedAt) : "Not observed"],
      ["Tab", health.lifecycle?.opened ? humanize(health.lifecycle.ownership ?? "open") : "Not currently reported"],
    ]) {
      const row = document.createElement("div");
      const term = document.createElement("dt");
      term.textContent = label;
      const detail = document.createElement("dd");
      detail.textContent = value;
      row.append(term, detail);
      facts.append(row);
    }
    card.append(header, description, facts);
    return card;
  });
  elements.overviewSources.replaceChildren(...cards);
}

async function loadRuntimeSettings() {
  elements.runtimeSettingsStatus.textContent = "Loading…";
  try {
    const { configuration } = await api("/api/configuration/runtime");
    renderRuntimeSettings(configuration);
    elements.runtimeSettingsStatus.textContent = "";
  } catch (error) {
    elements.runtimeSettingsStatus.textContent = error.message;
  }
}

function renderRuntimeSettings(configuration) {
  state.runtimeConfiguration = configuration;
  const timelineCapacity = configuration.timelineCapacity;
  state.timelineCapacity = timelineCapacity.effectiveValue;
  elements.timelineCapacity.value =
    timelineCapacity.persistedValue ?? timelineCapacity.effectiveValue;
  elements.timelineCapacity.disabled = timelineCapacity.source === "environment";
  const presentation = configuration.defaultPresentation;
  state.defaultPresentation = presentation.effectiveValue;
  elements.defaultPresentation.value =
    presentation.persistedValue ?? presentation.effectiveValue;
  elements.defaultPresentation.disabled = presentation.source === "environment";
  const streamWidth = configuration.streamWidth;
  applyStreamWidth(streamWidth.effectiveValue);
  elements.streamWidth.value = streamWidth.persistedValue ?? streamWidth.effectiveValue;
  elements.streamWidth.disabled = streamWidth.source === "environment";
  const telemetryBehavior = configuration.telemetryBehavior;
  applyTelemetryBehavior(telemetryBehavior.effectiveValue);
  elements.telemetryBehavior.value =
    telemetryBehavior.persistedValue ?? telemetryBehavior.effectiveValue;
  elements.telemetryBehavior.disabled = telemetryBehavior.source === "environment";
  for (const [name, control] of Object.entries({
    maxItemsPerSource: elements.maxItemsPerSource,
    maxScrolls: elements.maxScrolls,
    maxAcquisitionRounds: elements.maxAcquisitionRounds,
    maxKnowledgeContextEvents: elements.maxKnowledgeContextEvents,
  })) {
    const entry = configuration[name];
    control.value = entry.persistedValue ?? entry.effectiveValue;
    control.disabled = entry.source === "environment";
  }
  renderFixedEngineConstraints();
  const setting = configuration.missingSourceTabPolicy;
  elements.missingSourceTabPolicy.value = setting.persistedValue ?? setting.effectiveValue;
  const overridden = setting.source === "environment";
  elements.missingSourceTabPolicy.disabled = overridden;
  elements.missingSourceTabDetail.textContent = overridden
    ? `Effective: ${humanize(setting.effectiveValue)} · environment override; dashboard editing is disabled.`
    : `Effective: ${humanize(setting.effectiveValue)} · source: ${setting.source} · applies to the next run.`;
  const controls = {
    reasoningProvider: elements.reasoningProvider,
    planningPolicy: elements.planningPolicy,
    evaluationModel: elements.evaluationModel,
    evaluationEffort: elements.evaluationEffort,
    planningModel: elements.planningModel,
    planningEffort: elements.planningEffort,
    timeoutMs: elements.reasoningTimeout,
  };
  for (const [name, control] of Object.entries(controls)) {
    const entry = configuration[name];
    control.value = entry.persistedValue ?? entry.effectiveValue ?? "";
    control.disabled = entry.source === "environment";
  }
  elements.saveRuntimeSettings.disabled = Object.values(configuration)
    .every((entry) => entry.source === "environment");
  const restartNames = Object.entries(configuration)
    .filter(([, entry]) => entry.restartRequired)
    .map(([name]) => humanize(name));
  const lockedNames = Object.entries(configuration)
    .filter(([, entry]) => entry.source === "environment" && entry.applyMode === "restart")
    .map(([name]) => humanize(name));
  elements.startupSettingsDetail.textContent = [
    restartNames.length ? `Restart required: ${restartNames.join(", ")}.` : "No pending restart changes.",
    lockedNames.length ? `Environment override: ${lockedNames.join(", ")}.` : null,
  ].filter(Boolean).join(" ");
}

async function saveRuntimeSettings(event) {
  event.preventDefault();
  elements.saveRuntimeSettings.disabled = true;
  elements.runtimeSettingsStatus.textContent = "Saving…";
  try {
    const values = {
      activeSources: [...elements.overviewSources.querySelectorAll("[data-source-toggle]:checked")]
        .map((control) => control.value),
      maxItemsPerSource: Number(elements.maxItemsPerSource.value),
      maxScrolls: Number(elements.maxScrolls.value),
      maxAcquisitionRounds: Number(elements.maxAcquisitionRounds.value),
      maxKnowledgeContextEvents: Number(elements.maxKnowledgeContextEvents.value),
      timelineCapacity: Number(elements.timelineCapacity.value),
      defaultPresentation: elements.defaultPresentation.value,
      streamWidth: elements.streamWidth.value,
      telemetryBehavior: elements.telemetryBehavior.value,
      missingSourceTabPolicy: elements.missingSourceTabPolicy.value,
      reasoningProvider: elements.reasoningProvider.value,
      planningPolicy: elements.planningPolicy.value,
      evaluationModel: elements.evaluationModel.value,
      evaluationEffort: elements.evaluationEffort.value,
      planningModel: elements.planningModel.value,
      planningEffort: elements.planningEffort.value,
      timeoutMs: Number(elements.reasoningTimeout.value),
    };
    if (values.activeSources.length === 0) {
      throw new Error("Keep at least one installed source active.");
    }
    const update = Object.fromEntries(
      Object.entries(values).filter(([name]) =>
        state.runtimeConfiguration?.[name]?.source !== "environment"),
    );
    const { configuration } = await api("/api/configuration/runtime", {
      method: "PUT",
      body: JSON.stringify(update),
    });
    state.bootstrap.limits.missingSourceTabPolicy =
      configuration.missingSourceTabPolicy.effectiveValue;
    state.bootstrap.presentation.defaultLayout =
      configuration.defaultPresentation.effectiveValue;
    state.bootstrap.sourceRegistry = state.bootstrap.sourceRegistry.map((source) => ({
      ...source,
      activationState: configuration.activeSources.effectiveValue.includes(source.id)
        ? "active"
        : "inactive",
    }));
    state.bootstrap.limits.maxItems = configuration.maxItemsPerSource.effectiveValue;
    state.bootstrap.limits.maxScrolls = configuration.maxScrolls.effectiveValue;
    state.bootstrap.limits.defaultScrolls = configuration.maxScrolls.effectiveValue;
    state.bootstrap.limits.maxAcquisitionRounds = configuration.maxAcquisitionRounds.effectiveValue;
    state.bootstrap.limits.maxKnowledgeContextEvents =
      configuration.maxKnowledgeContextEvents.effectiveValue;
    state.bootstrap.presentation.timelineCapacity =
      configuration.timelineCapacity.effectiveValue;
    state.bootstrap.presentation.streamWidth = configuration.streamWidth.effectiveValue;
    state.bootstrap.presentation.telemetryBehavior =
      configuration.telemetryBehavior.effectiveValue;
    renderRuntimeSettings(configuration);
    await loadTimelineFeed();
    await loadOverview();
    elements.runtimeSettingsStatus.textContent =
      "Saved. Live settings are applied; startup changes still require a visible restart.";
  } catch (error) {
    elements.runtimeSettingsStatus.textContent = error.message;
    elements.saveRuntimeSettings.disabled = false;
  }
}

function renderFixedEngineConstraints() {
  const limits = state.bootstrap?.limits ?? {};
  const facts = [
    ["Unified output", "10 items maximum"],
    ["Anchored follow-up", `${limits.followUpScrolls ?? 1} scroll`],
    ["Continuation anchors", String(limits.maxContinuationAnchors ?? 3)],
    ["Evidence blocks", `${limits.maxBlocksPerSnapshot ?? 20} per snapshot`],
    ["Block size", `${limits.maxBlockCharacters ?? 4_000} characters`],
    ["Media", `${limits.maxMediaPerBlock ?? 4} per block`],
    ["Capture timeout", `${Math.round((limits.captureTimeoutMs ?? 45_000) / 1_000)}s`],
    ["Fresh-content wait", `${Math.round((limits.pendingContentTimeoutMs ?? 5_000) / 1_000)}s`],
  ];
  elements.fixedEngineConstraints.replaceChildren(...facts.map(([label, value]) => {
    const card = document.createElement("article");
    const term = document.createElement("span");
    term.textContent = label;
    const detail = document.createElement("strong");
    detail.textContent = value;
    card.append(term, detail);
    return card;
  }));
}

function applyStreamWidth(value) {
  const allowed = new Set(["compact", "social", "comfortable", "wide"]);
  state.streamWidth = allowed.has(value) ? value : "social";
  document.body.dataset.streamWidth = state.streamWidth;
}

function applyTelemetryBehavior(value) {
  const allowed = new Set(["flow", "sticky"]);
  state.telemetryBehavior = allowed.has(value) ? value : "flow";
  document.body.dataset.telemetryBehavior = state.telemetryBehavior;
}

async function loadPilotReview({ append = false } = {}) {
  if (state.reviewLoading) return;
  state.reviewLoading = true;
  const requestedPage = append ? state.reviewPage + 1 : 0;
  elements.reviewRefreshButton.disabled = true;
  if (append) elements.reviewScrollSentinel.textContent = "Loading 10 more runs…";
  else elements.reviewMeta.textContent = "Loading pilot evidence…";
  try {
    const params = new URLSearchParams({
      limit: String(REVIEW_PAGE_SIZE),
      offset: String(requestedPage * REVIEW_PAGE_SIZE),
      source: elements.reviewSourceFilter.value,
      verdict: elements.reviewVerdictFilter.value,
    });
    const [{ review }, { profile }, { replay }, { experiment }, { comparison }] = await Promise.all([
      api(`/api/pilot/review?${params}`),
      api("/api/preferences/profile"),
      api("/api/preferences/replay"),
      api("/api/preferences/experiment"),
      api("/api/preferences/shadow-comparison?limit=5&offset=0"),
    ]);
    if (
      review.runs.length === 0 &&
      review.pagination.available > 0 &&
      review.pagination.offset >= review.pagination.available
    ) {
      state.reviewPage = Math.ceil(review.pagination.available / REVIEW_PAGE_SIZE) - 1;
      state.reviewLoading = false;
      return loadPilotReview();
    }
    state.reviewPage = requestedPage;
    state.reviewHasNext = review.pagination.hasNext;
    if (!append) {
      renderPilotMetrics(review.summary);
      renderSourceHealth(review.summary.sourceHealth);
      renderReasoningEconomics(review.summary.tokenUsage, review.runs);
      renderPreferenceReadiness(replay);
      renderPreferenceExperiment(experiment);
      renderShadowComparison(comparison);
    }
    const shown = Math.min(review.pagination.offset + review.runs.length, REVIEW_MAX_RUNS);
    elements.reviewMeta.textContent = [
      `${review.totalMatching} matching run(s)`,
      shown ? `${shown} loaded` : "no runs shown",
      review.window?.pilotStartedAt
        ? `pilot cohort since ${formatDate(review.window.pilotStartedAt)}`
        : null,
      review.window?.truncated ? "metrics use the latest 500 runs" : null,
      `preference ${profile.status} · ${profile.feedbackEventCount} signal(s)`,
    ]
      .filter(Boolean)
      .join(" · ");
    if (append) appendPilotRunGroups(review.runs);
    else elements.reviewRuns.replaceChildren(...buildPilotRunGroups(review.runs, true));
    renderPilotReviewScrollStatus(review.pagination, review.runs.length);
    if (review.runs.length === 0) {
      const empty = document.createElement("p");
      empty.className = "review-empty";
      empty.textContent = "No runs match these filters.";
      elements.reviewRuns.append(empty);
    }
  } catch (error) {
    if (append) elements.reviewScrollSentinel.textContent = `Could not load more: ${error.message}`;
    else elements.reviewMeta.textContent = error.message;
  } finally {
    state.reviewLoading = false;
    elements.reviewRefreshButton.disabled = false;
  }
}

async function fitPreferenceExperiment() {
  elements.fitPreferenceExperiment.disabled = true;
  elements.preferenceExperimentDetail.textContent = "Fitting shadow-only snapshot…";
  try {
    const { experiment } = await api("/api/preferences/experiment/fit", { method: "POST" });
    renderPreferenceExperiment(experiment);
  } catch (error) {
    elements.preferenceExperimentDetail.textContent = error.message;
  }
}

function renderPreferenceExperiment(experiment) {
  const labels = {
    blocked: "Blocked",
    ready_to_fit: "Ready to fit",
    fitted: "Shadow fitted",
  };
  const fitted = experiment.status === "fitted";
  const ready = experiment.status === "ready_to_fit";
  setStatus(
    elements.preferenceExperimentStatus,
    labels[experiment.status] ?? humanize(experiment.status),
    fitted ? "ok" : ready ? "warning" : "neutral",
  );
  elements.fitPreferenceExperiment.disabled = !ready;
  const snapshot = experiment.currentSnapshot;
  elements.preferenceExperimentDetail.textContent = snapshot
    ? [
        `Snapshot v${snapshot.version}`,
        `${snapshot.evaluation.holdoutSignals} holdout signal(s)`,
        `agreement ${formatPercent(snapshot.evaluation.agreement)}`,
        `balanced ${formatPercent(snapshot.evaluation.balancedAccuracy)}`,
        `${snapshot.shadow.scoredCandidates} shadow-scored candidates`,
        "live influence off",
      ].join(" · ")
    : [
        `${experiment.readiness.passedGates}/${experiment.readiness.totalGates} readiness gates passed`,
        experiment.latestSnapshot ? "latest snapshot is stale" : "no snapshot persisted",
        "live influence off",
      ].join(" · ");
}

function renderShadowComparison(comparison) {
  if (!comparison?.available) {
    elements.shadowComparisonDetail.textContent =
      "Waiting for a current fitted snapshot · live influence off.";
    elements.shadowCandidateList.replaceChildren();
    return;
  }
  const summary = comparison.summary;
  elements.shadowComparisonDetail.textContent = [
    `${summary.scoredCandidates} scored`,
    `${summary.wouldMoveUp} would move up`,
    `${summary.wouldMoveDown} would move down`,
    `${summary.unchanged} unchanged`,
    `${summary.insufficientEvidence} insufficient`,
    summary.duplicateCandidatesCollapsed
      ? `${summary.duplicateCandidatesCollapsed} repeat appearances collapsed`
      : null,
    "live influence off",
  ].filter(Boolean).join(" · ");
  renderShadowCandidates(comparison.candidates ?? [], summary.wouldMoveUp);
}

function renderShadowCandidates(candidates, totalPromotions) {
  const promotions = candidates
    .filter((candidate) => candidate.movement === "would_move_up")
    .slice(0, 5);
  if (promotions.length === 0) {
    elements.shadowCandidateList.replaceChildren();
    return;
  }

  const heading = document.createElement("p");
  heading.className = "shadow-candidate-heading";
  heading.textContent = `Reviewing ${promotions.length} of ${totalPromotions} shadow promotion(s)`;
  const cards = promotions.map((candidate) => {
    const card = document.createElement("article");
    card.className = "shadow-candidate-card";

    const meta = document.createElement("div");
    meta.className = "shadow-candidate-meta";
    const identity = document.createElement("strong");
    identity.textContent = `${sourceLabel(candidate.source)} · ${candidate.author || "Unknown author"}`;
    const score = document.createElement("span");
    score.textContent = `Preference match ${formatPercent(candidate.probability)}`;
    meta.append(identity, score);

    const text = document.createElement("p");
    text.className = "shadow-candidate-text";
    text.textContent = candidate.text || "Captured candidate text is unavailable.";

    const topics = document.createElement("p");
    topics.className = "shadow-candidate-topics";
    topics.textContent = (candidate.topicTags ?? []).slice(0, 4).join(" · ") || candidate.contentType;

    card.append(meta, text, topics);
    if (candidate.sourceUrl) {
      const link = document.createElement("a");
      link.className = "shadow-candidate-source";
      link.href = candidate.sourceUrl;
      link.target = "_blank";
      link.rel = "noopener noreferrer";
      link.textContent = "Open source";
      card.append(link);
    }
    return card;
  });
  elements.shadowCandidateList.replaceChildren(heading, ...cards);
}

function resetPilotReviewPage() {
  state.reviewPage = 0;
  state.reviewHasNext = false;
  loadPilotReview();
}

function renderPilotReviewScrollStatus(pagination, receivedCount) {
  const available = Math.min(pagination.available, REVIEW_MAX_RUNS);
  const loaded = Math.min(pagination.offset + receivedCount, available);
  elements.reviewScrollSentinel.textContent = pagination.hasNext
    ? `Scroll to load more · ${loaded} of ${available}`
    : available > 0
      ? `End of review window · ${available} run(s)`
      : "No review history";
}

function observePilotReviewScroll() {
  const observer = new IntersectionObserver(
    (entries) => {
      if (entries.some((entry) => entry.isIntersecting) && state.reviewHasNext) {
        loadPilotReview({ append: true });
      }
    },
    { rootMargin: "240px 0px" },
  );
  observer.observe(elements.reviewScrollSentinel);
}

function renderPreferenceReadiness(replay) {
  const ready = replay.readiness.status === "ready_for_offline_fit";
  setStatus(
    elements.preferenceReadinessStatus,
    ready ? "Ready for offline fit" : "Collecting",
    ready ? "ok" : "neutral",
  );
  elements.preferenceReadinessGates.replaceChildren(
    ...replay.readiness.gates.map((gate) => {
      const card = document.createElement("div");
      card.className = gate.passed ? "passed" : "pending";
      const label = document.createElement("span");
      label.textContent = humanize(gate.id);
      const value = document.createElement("strong");
      value.textContent = `${gate.observed} / ${gate.required}`;
      card.append(label, value);
      return card;
    }),
  );
  elements.preferenceReadinessDetail.textContent = [
    `${replay.dataset.evaluatedCandidates} evaluated candidates`,
    `${replay.dataset.assessedCandidates} assessed`,
    `${replay.dataset.assessedFeedback} feedback signals have structured assessment`,
    "Live ranking influence remains disabled",
  ].join(" · ");
}

function renderPilotMetrics(summary) {
  const metrics = [
    ["Completed", `${summary.completedRuns} / ${summary.totalRuns}`],
    [
      "Review coverage",
      `${formatPercent(summary.reviewCoverage)} (${summary.reviewedRuns}/${summary.reviewableRuns ?? summary.completedRuns})`,
    ],
    [
      "Empty-result trust",
      `${formatPercent(summary.emptyTrustRate)} (${summary.correctlyEmptyRuns}/${summary.correctlyEmptyRuns + summary.missedRuns})`,
    ],
    [
      "Positive reviewed items",
      `${formatPercent(summary.positiveItemRate)} (${summary.positiveReviewedItems}/${summary.reviewedItems})`,
    ],
    ["Median duration", formatDuration(summary.medianDurationMs)],
    ["Failed runs", String(summary.failedRuns)],
    ["More like this", String(summary.moreLikeThisSignals ?? 0)],
    ["Less like this", String(summary.lessLikeThisSignals ?? 0)],
  ];
  elements.reviewMetrics.replaceChildren(
    ...metrics.map(([label, value]) => {
      const card = document.createElement("div");
      const term = document.createElement("span");
      term.textContent = label;
      const metric = document.createElement("strong");
      metric.textContent = value;
      card.append(term, metric);
      return card;
    }),
  );
}

function renderSourceHealth(health) {
  elements.sourceHealthDetails.replaceChildren();
  if (!health) {
    setStatus(elements.sourceHealthStatus, "Unavailable", "neutral");
    return;
  }
  const tone = health.status === "healthy"
    ? "ok"
    : health.status === "degraded" || health.status === "insufficient"
      ? "warning"
      : "error";
  setStatus(elements.sourceHealthStatus, humanize(health.status), tone);
  const rows = [
    ["Window", `${health.totalRuns} of ${health.windowSize} latest source runs`],
    ["Completion", `${formatPercent(health.completionRate)} (${health.completedRuns}/${health.totalRuns})`],
    ["X", healthSummary(health.sources?.x)],
    ["LinkedIn", healthSummary(health.sources?.linkedin)],
    ["Restoration failures", String(health.restorationFailures ?? 0)],
    ["Stale-tab recoveries", String(health.staleTabRecoveries ?? 0)],
  ];
  for (const [label, value] of rows) {
    const row = document.createElement("div");
    const term = document.createElement("span");
    term.textContent = label;
    const detail = document.createElement("strong");
    detail.textContent = value;
    row.append(term, detail);
    elements.sourceHealthDetails.append(row);
  }
  if (health.failureCategories?.length) {
    const failures = document.createElement("p");
    failures.textContent = `Recent failures: ${health.failureCategories
      .map((entry) => `${humanize(entry.category)} ${entry.count}`)
      .join(" · ")}`;
    elements.sourceHealthDetails.append(failures);
  }
}

function healthSummary(value) {
  if (!value?.totalRuns) return "No recent run";
  return `${formatPercent(value.completionRate)} · ${value.completedRuns}/${value.totalRuns}`;
}

function renderReasoningEconomics(tokenUsage, runs) {
  const invocations = runs.flatMap((run) => run.reasoningInvocations ?? []);
  const reasoning = state.bootstrap?.reasoning ?? {};
  const phases = [
    ["Candidate Evaluation", "candidate_evaluation", reasoning.evaluationModel, reasoning.evaluationEffort, tokenUsage?.byPhase?.candidateEvaluation],
    ["Acquisition Planning", "acquisition_planning", reasoning.planningModel, reasoning.planningEffort, tokenUsage?.byPhase?.acquisitionPlanning],
  ];
  elements.reviewTokenUsage.replaceChildren(...phases.map(([label, phase, configuredModel, configuredEffort, usage]) => {
    const latest = invocations.find((entry) => entry.phase === phase && entry.model);
    const card = document.createElement("article");
    const heading = document.createElement("div");
    const title = document.createElement("h4");
    title.textContent = label;
    const setup = document.createElement("span");
    const model = latest?.model || configuredModel;
    const effort = latest?.reasoningEffort || configuredEffort;
    setup.textContent = `${friendlyModel(model)} · ${effort || "default"}`;
    heading.append(title, setup);
    const stats = document.createElement("dl");
    for (const [term, value] of [
      ["Invocations", usage?.invocations ?? 0],
      ["Input", formatTokenCount(usage?.inputTokens)],
      ["Cached input", formatTokenCount(usage?.cachedInputTokens)],
      ["Output", formatTokenCount(usage?.outputTokens)],
      ["Reasoning output", formatTokenCount(usage?.reasoningOutputTokens)],
    ]) {
      const dt = document.createElement("dt");
      dt.textContent = term;
      const dd = document.createElement("dd");
      dd.textContent = String(value);
      stats.append(dt, dd);
    }
    card.append(heading, stats);
    return card;
  }));
}

function formatTokenCount(value) {
  return Number.isFinite(value) ? value.toLocaleString() : "Not reported";
}

function buildPilotRunGroups(runs, expandFirst = false) {
  const groups = [];
  const byKey = new Map();
  for (const run of runs) {
    const key = run.unifiedSessionId || `single:${run.id}`;
    if (!byKey.has(key)) {
      const group = { key, unified: Boolean(run.unifiedSessionId), runs: [] };
      byKey.set(key, group);
      groups.push(group);
    }
    byKey.get(key).runs.push(run);
  }
  for (const group of groups) {
    if (group.unified) {
      group.runs.sort((left, right) => sourceReviewOrder(left.source) - sourceReviewOrder(right.source));
    }
  }
  return groups.map((group, groupIndex) => {
    const section = document.createElement("section");
    section.className = `unified-review-group${group.unified ? " is-unified" : ""}`;
    section.dataset.reviewGroupKey = group.key;
    const heading = document.createElement("div");
    heading.className = "unified-review-heading";
    const title = document.createElement("h3");
    title.textContent = group.unified ? "Unified session · X + LinkedIn" : "Legacy single-source run";
    const meta = document.createElement("p");
    const reference = group.runs[0];
    meta.textContent = [
      formatDate(reference.unifiedSessionCreatedAt ?? reference.createdAt),
      group.unified ? `${group.runs.length}/2 source runs` : null,
    ].filter(Boolean).join(" · ");
    heading.append(title, meta);
    section.append(heading);
    for (const run of group.runs) {
      section.append(buildPilotRunCard(run, expandFirst && groupIndex === 0));
    }
    return section;
  });
}

function sourceReviewOrder(source) {
  return source === "x" ? 0 : source === "linkedin" ? 1 : 2;
}

function appendPilotRunGroups(runs) {
  const groups = buildPilotRunGroups(runs, false);
  const lastExisting = elements.reviewRuns.lastElementChild;
  const firstNew = groups[0];
  if (
    lastExisting?.dataset.reviewGroupKey &&
    firstNew?.dataset.reviewGroupKey === lastExisting.dataset.reviewGroupKey
  ) {
    for (const card of [...firstNew.querySelectorAll(":scope > .pilot-run-card")]) {
      lastExisting.append(card);
    }
    sortReviewGroupCards(lastExisting);
    groups.shift();
  }
  elements.reviewRuns.append(...groups);
}

function sortReviewGroupCards(group) {
  const cards = [...group.querySelectorAll(":scope > .pilot-run-card")];
  cards.sort(
    (left, right) =>
      sourceReviewOrder(left.dataset.reviewSource) -
      sourceReviewOrder(right.dataset.reviewSource),
  );
  group.append(...cards);
}

function buildPilotRunCard(run, expanded = false) {
  const card = document.createElement("details");
  card.className = "pilot-run-card";
  card.dataset.reviewSource = run.source;
  card.open = expanded;
  const cardSummary = document.createElement("summary");
  const header = document.createElement("div");
  header.className = "pilot-run-header";
  const identity = document.createElement("div");
  const title = document.createElement("h3");
  title.textContent = `${run.source === "x" ? "X" : "LinkedIn"} · ${humanize(run.mode)}`;
  const timestamp = document.createElement("p");
  timestamp.textContent = `${formatDate(run.completedAt ?? run.createdAt)} · ${formatDuration(run.durationMs)}`;
  identity.append(title, timestamp);
  const status = document.createElement("span");
  status.className = `status-pill ${run.status === "completed" ? "status-ok" : "status-error"}`;
  status.textContent = humanize(run.status);
  const badges = document.createElement("div");
  badges.className = "pilot-run-badges";
  const evaluationInvocation = [...(run.reasoningInvocations ?? [])]
    .reverse()
    .find((entry) => entry.phase === "candidate_evaluation");
  if (evaluationInvocation) {
    const reasoning = document.createElement("span");
    reasoning.className = "status-pill status-neutral";
    reasoning.textContent = `${friendlyModel(evaluationInvocation.model)} · ${evaluationInvocation.reasoningEffort || "default"}`;
    badges.append(reasoning);
  }
  badges.append(status);
  header.append(identity, badges);

  cardSummary.append(header, buildRunPhaseUsage(run));
  card.append(cardSummary);
  const syncBody = (event) => {
    if (event && event.target !== card) return;
    if (card.open) mountPilotRunBody(card, run);
    else unmountPilotRunBody(card);
  };
  card.addEventListener("toggle", syncBody);
  if (expanded) mountPilotRunBody(card, run);
  return card;
}

function mountPilotRunBody(card, run) {
  if (card.querySelector(".pilot-run-body")) return;
  const body = document.createElement("div");
  body.className = "pilot-run-body";
  const intent = document.createElement("p");
  intent.className = "pilot-run-intent";
  intent.textContent = run.intent;
  const summary = document.createElement("p");
  summary.className = "result-summary";
  summary.textContent = run.result?.summary ?? run.error?.message ?? "No result summary.";

  const stats = document.createElement("p");
  stats.className = "pilot-run-stats";
  stats.textContent = [
    `Shown ${run.result?.items?.length ?? 0}`,
    `Candidates ${run.coverage?.candidateCount ?? 0}`,
    `Delivered suppressed ${run.coverage?.deliveredEvidenceSuppressed ?? 0}`,
    `Confirmed excluded ${run.coverage?.confirmedExcludedSuppressed ?? 0}`,
    `Rounds ${run.coverage?.acquisitionRounds ?? 0}`,
    run.coverage?.providerFollowUpExecuted ? "Follow-up yes" : "Follow-up no",
    run.coverage?.restoreAttempted
      ? `Restored ${run.coverage?.restored ? "yes" : "no"}`
      : null,
  ]
    .filter(Boolean)
    .join(" · ");
  body.append(intent, summary, stats);

  if (run.status === "completed" && (run.result?.items?.length ?? 0) === 0) {
    body.append(buildEmptyResultFeedback(run, loadPilotReview));
  }
  if ((run.result?.items?.length ?? 0) > 0) {
    const details = document.createElement("details");
    const label = document.createElement("summary");
    label.textContent = `Review ${run.result.items.length} promoted item(s)`;
    const items = document.createElement("div");
    items.className = "result-items review-result-items";
    for (const item of run.result.items) {
      items.append(buildResultItem(run, item, loadPilotReview));
    }
    details.append(label, items);
    body.append(details);
  }
  if ((run.candidateEvaluations?.length ?? 0) > 0) {
    const candidates = document.createElement("section");
    candidates.className = "candidate-review-list";
    const heading = document.createElement("h4");
    heading.textContent = `${run.candidateEvaluations.length} evaluated candidate(s)`;
    candidates.append(heading);
    for (const candidate of run.candidateEvaluations) {
      candidates.append(buildCandidateReview(run, candidate));
    }
    body.append(candidates);
  } else if (run.status === "completed") {
    const unavailable = document.createElement("p");
    unavailable.className = "review-empty";
    unavailable.textContent = run.reasoningInvocations?.length
      ? "No new candidate required model evaluation in this bounded run."
      : "Candidate decision history is unavailable for runs created before Learning Loop v0.";
    body.append(unavailable);
  }
  card.append(body);
}

function unmountPilotRunBody(card) {
  card.querySelector(".pilot-run-body")?.remove();
}

function buildRunPhaseUsage(run) {
  const container = document.createElement("div");
  container.className = "run-phase-usage";
  const configured = state.bootstrap?.reasoning ?? {};
  const phases = [
    ["Candidate evaluation", "candidate_evaluation", configured.evaluationModel, configured.evaluationEffort],
    ["Acquisition planning", "acquisition_planning", configured.planningModel, configured.planningEffort],
  ];
  for (const [label, phase, configuredModel, configuredEffort] of phases) {
    const invocations = (run.reasoningInvocations ?? []).filter((entry) => entry.phase === phase);
    const latest = invocations.at(-1);
    const article = document.createElement("article");
    const heading = document.createElement("strong");
    heading.textContent = label;
    const setup = document.createElement("span");
    setup.textContent = `${friendlyModel(latest?.model || configuredModel)} · ${latest?.reasoningEffort || configuredEffort || "default"}`;
    const usage = document.createElement("span");
    usage.textContent = invocations.length > 0
      ? [
          `${sumInvocationTokens(invocations, "inputTokens")} input`,
          `${sumInvocationTokens(invocations, "cachedInputTokens")} cached`,
          `${sumInvocationTokens(invocations, "outputTokens")} output`,
          `${sumInvocationTokens(invocations, "reasoningOutputTokens")} reasoning`,
        ].join(" · ")
      : "Not invoked · 0 tokens";
    article.append(heading, setup, usage);
    container.append(article);
  }
  return container;
}

function sumInvocationTokens(invocations, field) {
  const values = invocations.map((entry) => entry[field]).filter(Number.isFinite);
  return values.length > 0
    ? values.reduce((sum, value) => sum + value, 0).toLocaleString()
    : "not reported";
}

function buildCandidateReview(run, candidate) {
  const brief = document.createElement("div");
  brief.className = "candidate-review-brief item-layout-view";
  const meta = document.createElement("p");
  meta.className = "pilot-run-stats";
  meta.textContent = [
    candidate.decision,
    candidate.reasonCode,
    candidate.author,
    Number.isInteger(candidate.feedPosition) ? `feed #${candidate.feedPosition}` : null,
  ].filter(Boolean).join(" · ");
  const content = buildCandidateContent(candidate);
  const assessment = candidate.assessment ? buildCandidateAssessment(candidate.assessment) : null;
  const actions = document.createElement("div");
  actions.className = "result-actions";
  const link = document.createElement("a");
  link.className = "source-link";
  link.href = candidate.sourceUrl;
  link.target = "_blank";
  link.rel = "noreferrer noopener";
  link.textContent = "Open source";
  actions.append(link, buildPreferenceButton(
    run, candidate.evidenceKey, "more_like_this", "More like this",
  ));
  actions.append(buildPreferenceButton(
    run, candidate.evidenceKey, "less_like_this", "Less like this",
  ));
  brief.append(meta, content);
  if (assessment) brief.append(assessment);
  const sourceItem = {
    source: candidate.source || run.source,
    author: candidate.author,
    publishedAt: candidate.publishedAt,
    whatChanged: candidate.text,
    sourceUrl: candidate.sourceUrl,
    sourceUrlKind: candidate.sourceUrlKind,
    evidenceKey: candidate.evidenceKey,
  };
  return buildItemPresentation({
    brief,
    source: buildSourceLayoutCard(run, sourceItem, candidate),
    actions,
    className: "candidate-review-item",
  });
}

function buildCandidateAssessment(assessment) {
  const section = document.createElement("section");
  section.className = "candidate-assessment";
  const badges = document.createElement("div");
  badges.className = "assessment-badges";
  for (const label of [
    humanize(assessment.contentType),
    ...(assessment.topicTags ?? []),
  ].filter(Boolean)) {
    const badge = document.createElement("span");
    badge.textContent = label;
    badges.append(badge);
  }
  const scores = document.createElement("p");
  scores.className = "assessment-scores";
  scores.textContent = [
    ["novelty", assessment.novelty],
    ["urgency", assessment.urgency],
    ["actionability", assessment.actionability],
  ].map(([label, value]) => `${label} ${formatPercent(value)}`).join(" · ");
  const rationale = document.createElement("p");
  rationale.textContent = assessment.rationale;
  section.append(badges, scores, rationale);
  return section;
}

function buildCandidateContent(candidate, options = {}) {
  const container = document.createElement("div");
  container.className = "candidate-content";
  let text = String(candidate.text ?? "").replace(/^Feed post\s+/i, "").trim();
  if (candidate.source === "linkedin") {
    const authorEnd = text.indexOf(" • ");
    if (authorEnd > 0) {
      const author = document.createElement("strong");
      author.textContent = text.slice(0, authorEnd).trim();
      if (options.includeIdentity !== false) container.append(author);
    }
    const bodyMatch = text.match(/(?:\b\d+[mhdw]\b|Promoted by [^•]+)\s*•\s*/i);
    if (bodyMatch?.index !== undefined) {
      const headerEnd = bodyMatch.index + bodyMatch[0].length;
      const context = text.slice(authorEnd > 0 ? authorEnd + 3 : 0, headerEnd).trim();
      if (context) {
        const contextLine = document.createElement("span");
        contextLine.className = "candidate-context";
        contextLine.textContent = context;
        container.append(contextLine);
      }
      text = text.slice(headerEnd).trim();
    }
  } else if (candidate.author && text.startsWith(candidate.author)) {
    text = text.slice(candidate.author.length).trim();
  }
  const paragraph = document.createElement("p");
  paragraph.textContent = text;
  container.append(paragraph);
  return container;
}

function formatTokenUsage(usage) {
  if (!usage || usage.inputTokens === null) return "Not reported";
  return `${usage.inputTokens.toLocaleString()} in · ${(usage.outputTokens ?? 0).toLocaleString()} out`;
}

function friendlyModel(model) {
  return {
    "gpt-5.6-sol": "GPT-5.6 Sol",
    "gpt-5.6-terra": "GPT-5.6 Terra",
    "gpt-5.6-luna": "GPT-5.6 Luna",
  }[model] || model || "Codex default";
}

function provenanceLinkLabel(sourceUrlKind) {
  return {
    native_post: "Open native post",
    source_page: "Open source feed",
    external_reference: "Open referenced page",
  }[sourceUrlKind] ?? "Open source";
}

function showFailure(error) {
  state.currentView = "timeline";
  selectViewButton(elements.sessionViewButton);
  hide(elements.processingPanel, elements.resultPanel, elements.reviewPanel, elements.settingsPanel);
  show(elements.timelinePanel);
  show(elements.failurePanel);
  setUpdateButtonsDisabled(false);
  elements.failureTitle.textContent = `Stopped at ${humanize(error.stage || "unknown stage")}`;
  elements.failureMessage.textContent = error.message || "The run did not complete.";
  syncTimelineChrome();
}

function unifiedFailureMessage(session) {
  const failures = (session.children ?? [])
    .filter((child) => child.run?.error?.message)
    .map((child) => `${sourceLabel(child.source)}: ${child.run.error.message}`);
  return failures.length > 0
    ? failures.join(" | ")
    : "Neither requested source completed.";
}

function reportRunFailure(error) {
  if (state.currentView === "review") {
    elements.reviewMeta.textContent = `Active run status: ${error.message || "unavailable"}`;
    return;
  }
  if (state.currentView === "settings") {
    elements.runtimeSettingsStatus.textContent = `Active run status: ${error.message || "unavailable"}`;
    return;
  }
  showFailure(error);
}

async function api(path, options = {}) {
  const response = await fetch(path, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...(options.headers ?? {}),
    },
  });
  if (!response.ok) {
    const payload = await response.json().catch(() => ({}));
    throw new Error(payload.message || `Request failed with status ${response.status}`);
  }
  return response.json();
}

function setStatus(element, text, kind) {
  element.textContent = text;
  element.className = `status-pill status-${kind}`;
}

function show(...nodes) {
  for (const node of nodes) node.classList.remove("hidden");
}

function hide(...nodes) {
  for (const node of nodes) node.classList.add("hidden");
}

function clearPoll() {
  if (state.pollTimer) clearTimeout(state.pollTimer);
  state.pollTimer = null;
}

function isTerminal(status) {
  return ["completed", "failed", "cancelled"].includes(status);
}

function isUnifiedTerminal(status) {
  return ["completed", "partial", "failed", "cancelled"].includes(status);
}

function sourceLabel(source) {
  return source === "x" ? "X" : "LinkedIn";
}

function runHasDisplayEvidence(run) {
  if ((run.observations?.length ?? 0) > 0) {
    return run.observations.some((observation) =>
      (observation.payload?.snapshots ?? []).some(
        (snapshot) => (snapshot.blocks?.length ?? 0) > 0,
      ),
    );
  }
  return run.coverage?.observedBlockCount !== 0;
}

function humanize(value) {
  return String(value ?? "").replaceAll("_", " ").replace(/\b\w/g, (letter) => letter.toUpperCase());
}

function formatDate(value) {
  if (!value) return "unavailable";
  const date = new Date(value);
  return Number.isNaN(date.valueOf())
    ? "unavailable"
    : new Intl.DateTimeFormat(undefined, {
        dateStyle: "medium",
        timeStyle: "short",
      }).format(date);
}

function formatPercent(value) {
  return Number.isFinite(value) ? `${Math.round(value * 100)}%` : "Not rated";
}

function formatDuration(value) {
  if (!Number.isFinite(value)) return "Unavailable";
  return value < 10_000 ? `${(value / 1_000).toFixed(1)}s` : `${Math.round(value / 1_000)}s`;
}

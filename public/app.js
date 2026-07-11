const state = {
  bootstrap: null,
  bridgeReady: false,
  currentRun: null,
  currentSession: null,
  pollTimer: null,
  dispatchedRounds: new Set(),
  currentView: "session",
};

const elements = {
  sidecarStatus: document.querySelector("#sidecar-status"),
  bridgeStatus: document.querySelector("#bridge-status"),
  reasoningStatus: document.querySelector("#reasoning-status"),
  providerNotice: document.querySelector("#provider-notice"),
  sessionViewButton: document.querySelector("#session-view-button"),
  reviewViewButton: document.querySelector("#review-view-button"),
  reviewPanel: document.querySelector("#review-panel"),
  reviewRefreshButton: document.querySelector("#review-refresh-button"),
  reviewMetrics: document.querySelector("#review-metrics"),
  reviewTokenUsage: document.querySelector("#review-token-usage"),
  reviewSourceFilter: document.querySelector("#review-source-filter"),
  reviewVerdictFilter: document.querySelector("#review-verdict-filter"),
  reviewMeta: document.querySelector("#review-meta"),
  reviewRuns: document.querySelector("#review-runs"),
  controlPanel: document.querySelector(".control-panel"),
  runForm: document.querySelector("#run-form"),
  runButton: document.querySelector("#run-button"),
  singleSourceField: document.querySelector("#single-source-field"),
  preflightCopy: document.querySelector("#preflight-copy"),
  processingPanel: document.querySelector("#processing-panel"),
  processingTitle: document.querySelector("#processing-title"),
  processingDetail: document.querySelector("#processing-detail"),
  sourceProgress: document.querySelector("#source-progress"),
  progressBar: document.querySelector("#progress-bar"),
  contractMode: document.querySelector("#contract-mode"),
  contractSource: document.querySelector("#contract-source"),
  contractAttention: document.querySelector("#contract-attention"),
  contractScrolls: document.querySelector("#contract-scrolls"),
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
      elements.processingDetail.textContent =
        "A source action could not be verified. AkuSidecar may retry once with bounded detect-only capture.";
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

elements.runForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  await startRun();
});

elements.cancelButton.addEventListener("click", cancelCurrentRun);
elements.doneButton.addEventListener("click", resetToSetup);
elements.retryButton.addEventListener("click", resetToSetup);
elements.sessionViewButton.addEventListener("click", showSessionView);
elements.reviewViewButton.addEventListener("click", showReviewView);
elements.reviewRefreshButton.addEventListener("click", loadPilotReview);
elements.reviewSourceFilter.addEventListener("change", loadPilotReview);
elements.reviewVerdictFilter.addEventListener("change", loadPilotReview);
for (const input of elements.runForm.querySelectorAll('input[name="scope"]')) {
  input.addEventListener("change", updateScopeControls);
}

await bootstrap();
updateScopeControls();

async function bootstrap() {
  try {
    state.bootstrap = await api("/api/bootstrap");
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
    const { session } = await api("/api/sessions/active");
    if (session) {
      state.currentSession = session;
      showUnifiedProcessing(session);
      dispatchUnifiedSession(session);
      schedulePoll();
    } else {
      await showReviewView();
    }
    setTimeout(() => {
      if (!state.bridgeReady) {
        setStatus(elements.bridgeStatus, "AkuBridge not detected", "warning");
      }
    }, 1_200);
  } catch (error) {
    setStatus(elements.sidecarStatus, "AkuSidecar unavailable", "error");
    elements.runButton.disabled = true;
    elements.providerNotice.textContent = error.message;
    elements.providerNotice.classList.remove("hidden");
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
  state.currentView = "session";
  clearPoll();
  state.dispatchedRounds.clear();
  hide(elements.resultPanel, elements.failurePanel);
  const form = new FormData(elements.runForm);
  elements.runButton.disabled = true;
  try {
    if (form.get("scope") === "unified") {
      const { session } = await api("/api/sessions", {
        method: "POST",
        body: JSON.stringify({
          mode: form.get("mode"),
          intent: form.get("intent"),
          maxItemsPerSource: state.bootstrap?.unifiedSession?.maxItemsPerSource ?? 5,
        }),
      });
      state.currentRun = null;
      state.currentSession = session;
      showUnifiedProcessing(session);
      dispatchUnifiedSession(session);
    } else {
      const { run } = await api("/api/runs", {
        method: "POST",
        body: JSON.stringify({
          mode: form.get("mode"),
          source: form.get("source"),
          intent: form.get("intent"),
          maxItems: state.bootstrap?.limits?.maxItems ?? 5,
          scrolls: Math.min(
            state.bootstrap?.limits?.defaultScrolls ?? 2,
            state.bootstrap?.limits?.maxScrolls ?? 2,
          ),
        }),
      });
      state.currentSession = null;
      state.currentRun = run;
      showProcessing(run);
      dispatchToBridge(run);
    }
    schedulePoll();
  } catch (error) {
    elements.runButton.disabled = false;
    showFailure({ stage: "create_session", message: error.message });
  }
}

function updateScopeControls() {
  const form = new FormData(elements.runForm);
  const unified = form.get("scope") === "unified";
  elements.singleSourceField.classList.toggle("hidden", unified);
  elements.runButton.textContent = unified ? "Run unified brief" : "Run advanced source";
  elements.preflightCopy.textContent = unified
    ? "Keep signed-in X and LinkedIn feed tabs open. AkuBrowser checks them sequentially and never likes, replies, follows, or posts."
    : "Advanced mode keeps one source run visible for adapter tracing and controlled pilot work.";
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
      if (state.currentView === "review") await loadPilotReview();
      else showResult(run);
      return;
    }
    if (["failed", "cancelled"].includes(run.status)) {
      clearPoll();
      if (state.currentView === "review") await loadPilotReview();
      else {
        showFailure(run.error ?? {
          stage: run.status,
          message: run.status === "cancelled" ? "The run was cancelled." : "The run failed.",
        });
      }
      return;
    }
    if (state.currentView === "session") showProcessing(run);
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
      if (state.currentView === "review") await loadPilotReview();
      else if (session.status === "failed" || session.status === "cancelled") {
        showFailure({
          stage: session.status,
          message:
            session.status === "cancelled"
              ? "The bounded unified session was cancelled."
              : unifiedFailureMessage(session),
        });
      } else {
        showUnifiedResult(session);
      }
      return;
    }
    if (state.currentView === "session") showUnifiedProcessing(session);
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
      if (session.status === "partial") showUnifiedResult(session);
      else showFailure({ stage: "cancelled", message: "The bounded unified session was cancelled." });
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
  hide(elements.controlPanel, elements.resultPanel, elements.failurePanel);
  show(elements.processingPanel);
  elements.sourceProgress.replaceChildren();
  elements.contractMode.textContent = humanize(run.mode);
  elements.contractSource.textContent = run.source === "x" ? "X" : "LinkedIn";
  elements.contractAttention.textContent = `Up to ${run.maxItems} result(s)`;
  elements.contractScrolls.textContent =
    run.scrolls === 0
      ? "Reveal pending content if present; no scrolling"
      : `Reveal pending content if present; up to ${run.scrolls} native scroll(s)`;

  const followUp = (run.observations?.length ?? 0) > 0;
  const copy = {
    waiting_for_bridge: followUp
      ? [
          "Waiting for bounded follow-up",
          "The provider requested one policy-controlled adjacent observation.",
          58,
        ]
      : ["Waiting for AkuBridge", "Sending one bounded capture request.", 18],
    capturing: [
      "Observing the source tab",
      `Revealing pending fresh content when present, then capturing up to ${run.scrolls + 1} viewport(s) and restoring the resulting feed baseline.`,
      48,
    ],
    reasoning: ["Evaluating the observation", "Applying the provider-neutral result contract.", 76],
  }[run.status] ?? ["Processing", "The bounded run is still active.", 32];
  elements.processingTitle.textContent = copy[0];
  elements.processingDetail.textContent = copy[1];
  elements.progressBar.style.width = `${copy[2]}%`;
}

function showUnifiedProcessing(session) {
  hide(elements.controlPanel, elements.resultPanel, elements.failurePanel);
  show(elements.processingPanel);
  const activeChild = session.children.find(
    (child) => child.run && !isTerminal(child.run.status),
  );
  const activeLabel = activeChild ? sourceLabel(activeChild.source) : "next source";
  elements.processingTitle.textContent = `Checking ${activeLabel}`;
  elements.processingDetail.textContent =
    activeChild?.run.status === "reasoning"
      ? `Evaluating the bounded ${activeLabel} observation before moving to the next source.`
      : `Running the bounded ${activeLabel} capture. Sources execute sequentially.`;
  elements.contractMode.textContent = humanize(session.mode);
  elements.contractSource.textContent = "X then LinkedIn";
  elements.contractAttention.textContent = `Up to ${session.maxItemsPerSource} per source · ${session.maxItemsTotal} total`;
  elements.contractScrolls.textContent = "Existing bounded policy per source";
  const terminalCount = session.children.filter((child) =>
    ["completed", "failed", "cancelled"].includes(child.status),
  ).length;
  const activeProgress = activeChild?.run.status === "reasoning" ? 32 : activeChild ? 14 : 0;
  elements.progressBar.style.width = `${Math.min(94, terminalCount * 50 + activeProgress)}%`;
  elements.sourceProgress.replaceChildren(
    ...session.children.map((child) => {
      const card = document.createElement("div");
      const label = document.createElement("strong");
      label.textContent = sourceLabel(child.source);
      const status = document.createElement("span");
      status.className = `status-pill ${child.status === "completed" ? "status-ok" : child.status === "failed" ? "status-error" : "status-neutral"}`;
      status.textContent = humanize(child.status);
      card.append(label, status);
      return card;
    }),
  );
}

function showResult(run) {
  hide(elements.controlPanel, elements.processingPanel, elements.failurePanel);
  show(elements.resultPanel);
  elements.runButton.disabled = false;
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
}

function showUnifiedResult(session) {
  hide(elements.controlPanel, elements.processingPanel, elements.failurePanel);
  show(elements.resultPanel);
  setStatus(elements.bridgeStatus, "AkuBridge ready", "ok");
  elements.runButton.disabled = false;
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

function buildResultItem(run, item, onSaved = () => {}) {
  const article = document.createElement("article");
  article.className = "result-item";

  const header = document.createElement("div");
  header.className = "result-item-header";
  const priority = document.createElement("span");
  priority.className = "priority-badge";
  priority.dataset.priority = item.priority;
  priority.textContent = item.priority;
  const evidence = document.createElement("span");
  evidence.className = "evidence-badge";
  evidence.textContent = [
    humanize(item.evidenceState),
    `${Math.round(item.confidence * 100)}%`,
    item.knowledgeDelta ? humanize(item.knowledgeDelta) : null,
  ]
    .filter(Boolean)
    .join(" · ");
  header.append(priority, evidence);

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
  for (const [kind, label] of [
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
  const candidate = (run.candidateEvaluations ?? []).find(
    (entry) => entry.evidenceKey === item.evidenceKey,
  );
  if (candidate) {
    feedback.append(
      buildPreferenceButton(run, item.evidenceKey, "more_like_this", "More like this", onSaved),
      buildPreferenceButton(run, item.evidenceKey, "should_not_show", "Should not show", onSaved),
    );
  }
  actions.append(link, feedback);
  article.append(header, title, why, provenance, actions);
  return article;
}

function buildPreferenceButton(run, evidenceKey, kind, label, onSaved = () => {}) {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "feedback-button";
  button.textContent = label;
  const matchesKind = (entry) => entry.kind === kind
    || (kind === "more_like_this" && entry.kind === "should_show");
  if ((run.preferenceFeedback ?? []).some(
    (entry) => entry.evidenceKey === evidenceKey && matchesKind(entry),
  )) {
    button.classList.add("selected");
    button.disabled = true;
  }
  button.addEventListener("click", async () => {
    const response = await api(`/api/runs/${encodeURIComponent(run.id)}/preference-feedback`, {
      method: "POST",
      body: JSON.stringify({ kind, evidenceKey, reasonCode: null, note: "" }),
    });
    run.preferenceFeedback = response.run.preferenceFeedback;
    button.classList.add("selected");
    button.disabled = true;
    await onSaved();
  });
  return button;
}

function showSessionView() {
  state.currentView = "session";
  elements.sessionViewButton.classList.add("selected");
  elements.reviewViewButton.classList.remove("selected");
  hide(elements.reviewPanel);
  if (state.currentSession && isUnifiedTerminal(state.currentSession.status)) {
    if (["completed", "partial"].includes(state.currentSession.status)) {
      showUnifiedResult(state.currentSession);
    } else {
      showFailure({
        stage: state.currentSession.status,
        message: unifiedFailureMessage(state.currentSession),
      });
    }
  } else if (state.currentSession) {
    showUnifiedProcessing(state.currentSession);
  } else if (state.currentRun?.status === "completed") {
    showResult(state.currentRun);
  } else if (state.currentRun && !isTerminal(state.currentRun.status)) {
    showProcessing(state.currentRun);
  } else if (state.currentRun?.status === "failed") {
    showFailure(state.currentRun.error ?? { stage: "run", message: "The run failed." });
  } else {
    hide(elements.processingPanel, elements.resultPanel, elements.failurePanel);
    show(elements.controlPanel);
  }
}

async function showReviewView() {
  state.currentView = "review";
  elements.reviewViewButton.classList.add("selected");
  elements.sessionViewButton.classList.remove("selected");
  hide(
    elements.controlPanel,
    elements.processingPanel,
    elements.resultPanel,
    elements.failurePanel,
  );
  show(elements.reviewPanel);
  await loadPilotReview();
}

async function loadPilotReview() {
  elements.reviewRefreshButton.disabled = true;
  elements.reviewMeta.textContent = "Loading pilot evidence…";
  try {
    const params = new URLSearchParams({
      limit: "50",
      source: elements.reviewSourceFilter.value,
      verdict: elements.reviewVerdictFilter.value,
    });
    const [{ review }, { profile }] = await Promise.all([
      api(`/api/pilot/review?${params}`),
      api("/api/preferences/profile"),
    ]);
    renderPilotMetrics(review.summary);
    renderReasoningEconomics(review.summary.tokenUsage, review.runs);
    elements.reviewMeta.textContent = [
      `${review.totalMatching} matching run(s)`,
      `latest ${review.runs.length} shown`,
      review.window?.pilotStartedAt
        ? `pilot cohort since ${formatDate(review.window.pilotStartedAt)}`
        : null,
      review.window?.truncated ? "metrics use the latest 500 runs" : null,
      `preference ${profile.status} · ${profile.feedbackEventCount} signal(s)`,
    ]
      .filter(Boolean)
      .join(" · ");
    elements.reviewRuns.replaceChildren(...buildPilotRunGroups(review.runs));
    if (review.runs.length === 0) {
      const empty = document.createElement("p");
      empty.className = "review-empty";
      empty.textContent = "No runs match these filters.";
      elements.reviewRuns.append(empty);
    }
  } catch (error) {
    elements.reviewMeta.textContent = error.message;
  } finally {
    elements.reviewRefreshButton.disabled = false;
  }
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
    ["Should not show", String(summary.shouldNotShowSignals ?? 0)],
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

function buildPilotRunGroups(runs) {
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
  return groups.map((group, groupIndex) => {
    const section = document.createElement("section");
    section.className = `unified-review-group${group.unified ? " is-unified" : ""}`;
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
      section.append(buildPilotRunCard(run, groupIndex === 0));
    }
    return section;
  });
}

function buildPilotRunCard(run, expanded = false) {
  const card = document.createElement("details");
  card.className = "pilot-run-card";
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
  cardSummary.append(header);
  card.append(cardSummary, intent, summary, stats);

  if (run.status === "completed" && (run.result?.items?.length ?? 0) === 0) {
    card.append(buildEmptyResultFeedback(run, loadPilotReview));
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
    card.append(details);
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
    card.append(candidates);
  } else if (run.status === "completed") {
    const unavailable = document.createElement("p");
    unavailable.className = "review-empty";
    unavailable.textContent = run.reasoningInvocations?.length
      ? "No new candidate required model evaluation in this bounded run."
      : "Candidate decision history is unavailable for runs created before Learning Loop v0.";
    card.append(unavailable);
  }
  if ((run.reasoningInvocations?.length ?? 0) > 0) {
    const telemetry = document.createElement("p");
    telemetry.className = "pilot-run-stats";
    telemetry.textContent = summarizeRunTelemetry(run.reasoningInvocations);
    card.append(telemetry);
  }
  return card;
}

function buildCandidateReview(run, candidate) {
  const article = document.createElement("article");
  article.className = "candidate-review-item";
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
  if (candidate.decision === "selected") {
    actions.append(buildPreferenceButton(
      run, candidate.evidenceKey, "should_not_show", "Should not show",
    ));
  }
  article.append(meta, content);
  if (assessment) article.append(assessment);
  article.append(actions);
  return article;
}

function buildCandidateAssessment(assessment) {
  const section = document.createElement("section");
  section.className = "candidate-assessment";
  const badges = document.createElement("div");
  badges.className = "assessment-badges";
  for (const label of [
    assessment.recommendedPriority,
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
    ["relevance", assessment.intentRelevance],
    ["novelty", assessment.novelty],
    ["urgency", assessment.urgency],
    ["actionability", assessment.actionability],
  ].map(([label, value]) => `${label} ${formatPercent(value)}`).join(" · ");
  const rationale = document.createElement("p");
  rationale.textContent = assessment.rationale;
  section.append(badges, scores, rationale);
  return section;
}

function buildCandidateContent(candidate) {
  const container = document.createElement("div");
  container.className = "candidate-content";
  let text = candidate.text.replace(/^Feed post\s+/i, "").trim();
  if (candidate.source === "linkedin") {
    const authorEnd = text.indexOf(" • ");
    if (authorEnd > 0) {
      const author = document.createElement("strong");
      author.textContent = text.slice(0, authorEnd).trim();
      container.append(author);
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

function summarizeRunTelemetry(invocations) {
  const input = invocations.map((entry) => entry.inputTokens).filter(Number.isFinite);
  const output = invocations.map((entry) => entry.outputTokens).filter(Number.isFinite);
  const models = [...new Set(invocations.map((entry) => friendlyModel(entry.model)))];
  const efforts = [...new Set(invocations.map((entry) => entry.reasoningEffort).filter(Boolean))];
  return [
    `Reasoning ${models.join(", ")} · ${efforts.join(", ") || "default effort"}`,
    input.length ? `${input.reduce((sum, value) => sum + value, 0).toLocaleString()} input tokens` : "usage unavailable",
    output.length ? `${output.reduce((sum, value) => sum + value, 0).toLocaleString()} output tokens` : null,
  ].filter(Boolean).join(" · ");
}

function provenanceLinkLabel(sourceUrlKind) {
  return {
    native_post: "Open native post",
    source_page: "Open source feed",
    external_reference: "Open referenced page",
  }[sourceUrlKind] ?? "Open source";
}

function showFailure(error) {
  state.currentView = "session";
  elements.sessionViewButton.classList.add("selected");
  elements.reviewViewButton.classList.remove("selected");
  hide(elements.controlPanel, elements.processingPanel, elements.resultPanel, elements.reviewPanel);
  show(elements.failurePanel);
  elements.runButton.disabled = false;
  elements.failureTitle.textContent = `Stopped at ${humanize(error.stage || "unknown stage")}`;
  elements.failureMessage.textContent = error.message || "The run did not complete.";
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
  showFailure(error);
}

function resetToSetup() {
  clearPoll();
  state.currentRun = null;
  state.currentSession = null;
  state.currentView = "session";
  elements.sessionViewButton.classList.add("selected");
  elements.reviewViewButton.classList.remove("selected");
  hide(elements.processingPanel, elements.resultPanel, elements.failurePanel, elements.reviewPanel);
  show(elements.controlPanel);
  elements.runButton.disabled = false;
  pingBridge();
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

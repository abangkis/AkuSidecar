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
  providerNotice: document.querySelector("#provider-notice"),
  sessionViewButton: document.querySelector("#session-view-button"),
  reviewViewButton: document.querySelector("#review-view-button"),
  reviewPanel: document.querySelector("#review-panel"),
  reviewRefreshButton: document.querySelector("#review-refresh-button"),
  reviewMetrics: document.querySelector("#review-metrics"),
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
              : "Neither requested source completed.",
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
  actions.append(link, feedback);
  article.append(header, title, why, provenance, actions);
  return article;
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
        message: "The unified session did not produce a completed source.",
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
    const { review } = await api(`/api/pilot/review?${params}`);
    renderPilotMetrics(review.summary);
    elements.reviewMeta.textContent = [
      `${review.totalMatching} matching run(s)`,
      `latest ${review.runs.length} shown`,
      review.window?.pilotStartedAt
        ? `pilot cohort since ${formatDate(review.window.pilotStartedAt)}`
        : null,
      review.window?.truncated ? "metrics use the latest 500 runs" : null,
    ]
      .filter(Boolean)
      .join(" · ");
    elements.reviewRuns.replaceChildren(
      ...review.runs.map((run) => buildPilotRunCard(run)),
    );
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

function buildPilotRunCard(run) {
  const card = document.createElement("article");
  card.className = "pilot-run-card";
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
  header.append(identity, status);

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
  card.append(header, intent, summary, stats);

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
  return card;
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

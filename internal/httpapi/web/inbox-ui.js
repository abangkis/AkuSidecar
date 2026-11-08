

export function createInboxUi({
  $, api, configureNativePostLink, feedbackButton, formatDate, formatDuration, formatDurationBetween, formatReasoningEffort, formatReasoningModel, formatReasoningProvider, humanize, invalidateLibraryStorage, refreshTimeline, safeSourceUrl, scheduleBackToTop, showError, sourceLabel, state, updatePolicyBadge
}) {
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
      empty.textContent = "Run Update now, or let Auto Update prepare a batch, to create the first diagnostic entry.";
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
    const labels = {
      onboarding: "Onboarding check",
      scheduler: "Checked",
    };
    title.textContent = `${labels[session.trigger] || (session.delivery === "prepared" ? "Prepared check" : "User update")} ${formatDate(session.createdAt)}`;
    const policyBadge = updatePolicyBadge(session);
    if (policyBadge) {
      const badge = document.createElement("span");
      badge.className = `status-pill ${policyBadge.className}`;
      badge.textContent = policyBadge.text;
      badge.setAttribute("aria-label", policyBadge.label);
      identity.append(badge);
    }
    const status = document.createElement("span");
    status.className = `status-pill status-${inboxStatusTone(session.status)}`;
    status.textContent = humanize(session.status);
    identity.append(title, status);
    if (session.delivery === "prepared" && session.deliveryState) {
      const delivery = document.createElement("span");
      delivery.className = "status-pill status-neutral";
      delivery.textContent = humanize(session.deliveryState);
      identity.append(delivery);
    }
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
    if (session.aiDetectorYield) body.append(buildAIDetectorYield(session.aiDetectorYield));
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
    popover.textContent = "Input already includes cached input, so cached input is shown as a breakout and is not added again. Reasoning output is also a breakout. Failed invocations may still use tokens. Unavailable means the provider did not report usage, not zero. AI Deep Detection and Living Topic routing or understanding may update asynchronously. Library search, Memory storage, and deterministic related context do not invoke a provider.";
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
    execution.textContent = [
      category.execution === "async" ? "Async" : "In run",
      modelUsageCategoryProfile(category),
    ].filter(Boolean).join(" · ");
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

  function modelUsageCategoryProfile(category) {
    const process = (state.bootstrap?.reasoningProcesses ?? []).find((candidate) => candidate.id === category.id);
    const actual = new Map();
    for (const entry of category.entries ?? []) {
      if (!(entry.invocationCount > 0) || !entry.model) continue;
      actual.set(`${entry.model}\u0000${entry.effort || ""}`, { model: entry.model, effort: entry.effort || "" });
    }
    if (actual.size > 1) return "Mixed models";
    if (actual.size === 1) {
      const used = [...actual.values()][0];
      return process?.options?.find((option) => option.model === used.model && option.effort === used.effort)?.label
        || `${formatReasoningModel(used.model)} ${formatReasoningEffort(used.effort)}`;
    }
    if (category.status === "pending" || category.status === "running") {
      const configured = process?.options?.find((option) => option.id === process.profileId)?.label;
      return configured ? `${configured} configured` : "Configured model pending";
    }
    return "No model used";
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
      configureNativePostLink(link, sourceUrl, decision.source);
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
          invalidateLibraryStorage();
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

  function buildAIDetectorYield(value) {
    const details = document.createElement("details");
    details.className = "ai-detector-yield";
    const summary = document.createElement("summary");
    const title = document.createElement("strong");
    title.textContent = "AI Detector yield";
    const rollup = document.createElement("span");
    rollup.textContent = `${value.fastReviewed || 0} local · ${value.deepReviewed || 0} Deep · ${value.c2paInspected || 0} C2PA`;
    summary.append(title, rollup);
    const metrics = document.createElement("div");
    metrics.className = "ai-detector-yield-metrics";
    for (const [label, number, detail] of [
      ["Fast Detection", value.fastReviewed, `${value.fastStrong || 0} preliminary · ${value.fastNoSignal || 0} neutral · ${value.fastInsufficient || 0} insufficient`],
      ["Deep Detection", value.deepReviewed, `${value.deepEligible || 0} eligible · ${value.deepSkipped || 0} skipped before model use`],
      ["Platform labels", value.platformAiSignals, "High-authority platform AI labels"],
      ["C2PA images", value.c2paInspected, `${value.c2paAiOrigin || 0} AI origin · ${value.c2paWithManifest || 0} manifest · ${value.c2paNoManifest || 0} neutral · ${value.c2paFailed || 0} failed`],
    ]) {
      const row = document.createElement("article");
      const copy = document.createElement("span");
      const name = document.createElement("strong");
      name.textContent = label;
      const context = document.createElement("small");
      context.textContent = detail;
      copy.append(name, context);
      const count = document.createElement("strong");
      count.textContent = String(number || 0);
      row.append(copy, count);
      metrics.append(row);
    }
    details.append(summary, metrics);
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
    for (const [label, stage, value] of [
      ["Captured", "captured", run.capturedCandidates],
      ["Skipped unchanged", "skipped", run.skippedResurfaces],
      ["Evaluated", "evaluated", run.evaluatedCandidates],
      ["Selected", "selected", run.selectedCandidates],
      ["Added", "added", run.addedItems],
    ]) {
      const metric = document.createElement("div");
      const number = document.createElement("strong");
      number.textContent = inboxRunMetricText(run, sessionStatus, stage, value);
      metricNumbers[stage] = number;
      const name = document.createElement("span");
      name.textContent = label;
      const stageDuration = run.stageDurationsMs?.[stage];
      const timing = document.createElement("small");
      timing.className = "inbox-pipeline-duration";
      timing.textContent = Number.isFinite(stageDuration) ? formatDuration(stageDuration) : "";
      metric.append(number, name, timing);
      pipeline.append(metric);
    }
    const mechanics = document.createElement("p");
    mechanics.className = "inbox-run-mechanics";
    mechanics.textContent = [
      run.capturePerformance?.outcome ? `Capture ${humanize(run.capturePerformance.outcome)}` : null,
      `${run.acquisitionRounds ?? 0} capture round${run.acquisitionRounds === 1 ? "" : "s"}`,
      `${run.snapshotCount ?? 0} snapshots`,
      `${run.performedScrolls ?? 0} scrolls`,
      run.totalDurationMs ? `Total ${formatDuration(run.totalDurationMs)}` : null,
      run.reasoningDurationMs ? `${formatDuration(run.reasoningDurationMs)} model time` : null,
      run.resurfacedItems ? `${run.resurfacedItems} resurfaced${run.skippedResurfaces ? ` · ${run.skippedResurfaces} unchanged skipped` : ""}` : null,
      (run.evaluatedCandidates ?? 0) > 0 ? `${Math.round(((run.selectedCandidates ?? 0) / run.evaluatedCandidates) * 100)}% selection yield` : null,
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
    const captureReliabilityNotice = inboxRunCaptureReliabilityNotice(run);
    if (captureReliabilityNotice) {
      const notice = document.createElement("p");
      notice.className = captureReliabilityNotice.tone === "danger" ? "inbox-run-error" : "inbox-run-warning";
      notice.textContent = captureReliabilityNotice.text;
      card.append(notice);
    }
    if (!run.error && run.summary) {
      const summary = document.createElement("p");
      summary.className = "inbox-run-summary";
      summary.textContent = run.summary;
      card.append(summary);
    }
    if (run.candidateDiagnostics?.length) {
      card.append(buildCaptureCandidateTelemetry(run.candidateDiagnostics));
    }
    if (run.mediaAcquisition) {
      card.append(buildMediaAcquisitionTelemetry(run.mediaAcquisition));
    }
    if (run.visionEvaluation) {
      card.append(buildVisionEvaluationTelemetry(run.visionEvaluation));
    }
    if (run.captureSurface?.length) {
      card.append(buildCaptureSurfaceTelemetry(run.captureSurface));
    }
    if (run.acquisitionPlanning || run.identityResolution) {
      card.append(buildAcquisitionIdentityTelemetry(run.acquisitionPlanning, run.identityResolution));
    }
    card.append(buildInboxFlowInspector(run, (counts) => {
      for (const [stage, number] of Object.entries(metricNumbers)) {
        number.textContent = inboxRunMetricText(run, sessionStatus, stage, counts?.[stage]);
      }
    }));
    return card;
  }

  function buildVisionEvaluationTelemetry(value) {
    const details = document.createElement("details");
    details.className = "vision-evaluation-telemetry";
    const summary = document.createElement("summary");
    const title = document.createElement("strong");
    title.textContent = "Instagram media-only review";
    const availability = document.createElement("span");
    availability.textContent = `${value.pending || 0} awaiting review · ${value.deferred || 0} deferred`;
    summary.append(title, availability);
    const body = document.createElement("div");
    body.className = "vision-evaluation-body";
    const note = document.createElement("p");
    note.className = "capture-candidate-note";
    note.textContent = value.availability || "Labels are generated locally. No image is sent to an AI provider.";
    body.append(note);
    const list = document.createElement("ol");
    list.className = "vision-evaluation-jobs";
    for (const job of value.jobs || []) {
      const row = document.createElement("li");
      const label = document.createElement("strong");
      label.textContent = `${job.manualLabel || "Manual review · media-only post"} · ${job.canonicalIdentity || job.evidenceKey}`;
      const meta = document.createElement("span");
      meta.textContent = [job.attemptCount ? `${job.attemptCount} attempt${job.attemptCount === 1 ? "" : "s"}` : null, job.lastError || job.reason].filter(Boolean).join(" · ");
      row.append(label, meta);
      list.append(row);
    }
    if (list.children.length) body.append(list);
    details.append(summary, body);
    return details;
  }

  function inboxRunCaptureReliabilityNotice(run) {
    const captured = Number(run.capturedCandidates) || 0;
    const terminal = ["completed", "partial", "failed", "cancelled"].includes(run.status);
    if (captured > 0 || !terminal) return null;
    if (run.error) {
      return { tone: "danger", text: "Capture reliability alert: no candidate reached the evaluation pipeline." };
    }
    return { tone: "warning", text: "Capture completed with no candidates; evaluation was not attempted." };
  }

  function buildCaptureCandidateTelemetry(snapshots) {
    const details = document.createElement("details");
    details.className = "capture-candidate-telemetry";
    const summary = document.createElement("summary");
    const title = document.createElement("strong");
    title.textContent = "Capture candidates";
    const rollup = document.createElement("span");
    const maximum = (field) => Math.max(0, ...snapshots.map((snapshot) => Number(snapshot[field]) || 0));
    rollup.textContent = [
      `${snapshots.length} observed snapshot${snapshots.length === 1 ? "" : "s"}`,
      `max ${maximum("structuralCandidates")} structural`,
      `max ${maximum("eligibleCandidates")} eligible`,
      `max ${maximum("visibleEligibleCandidates")} visible`,
    ].join(" \u00b7 ");
    summary.append(title, rollup);

    const list = document.createElement("ol");
    list.className = "capture-candidate-snapshots";
    const formatReasons = (reasons) => Object.entries(reasons || {})
      .filter(([, count]) => Number(count) > 0)
      .map(([reason, count]) => `${humanize(reason)} ${count}`)
      .join(", ");
    for (const snapshot of snapshots) {
      const row = document.createElement("li");
      const heading = document.createElement("strong");
      heading.textContent = `Round ${snapshot.round} \u00b7 Snapshot ${snapshot.snapshot}`;
      const counts = document.createElement("span");
      counts.textContent = `${snapshot.structuralCandidates ?? 0} structural \u2192 ${snapshot.eligibleCandidates ?? 0} eligible \u2192 ${snapshot.visibleEligibleCandidates ?? 0} visible \u2192 ${snapshot.newCandidates ?? 0} new`;
      const context = document.createElement("small");
      context.textContent = [
        snapshot.strategy ? humanize(snapshot.strategy) : null,
        Number.isFinite(snapshot.scrollY) ? `scroll ${snapshot.scrollY}px` : null,
        Number.isFinite(snapshot.viewportHeight) ? `viewport ${snapshot.viewportHeight}px` : null,
        snapshot.actionAnchoredCandidates ? `${snapshot.actionAnchoredCandidates} action anchored` : null,
        formatReasons(snapshot.admittedReasons) ? `admitted: ${formatReasons(snapshot.admittedReasons)}` : null,
        formatReasons(snapshot.rejectedReasons) ? `rejected: ${formatReasons(snapshot.rejectedReasons)}` : null,
      ].filter(Boolean).join(" \u00b7 ");
      row.append(heading, counts, context);
      list.append(row);
    }
    const note = document.createElement("p");
    note.className = "capture-candidate-note";
    note.textContent = "Counts are per snapshot observations; repeated DOM candidates are not unique posts.";
    details.append(summary, list, note);
    return details;
  }

  function buildMediaAcquisitionTelemetry(value) {
    const details = document.createElement("details");
    details.className = "media-acquisition-telemetry";
    const summary = document.createElement("summary");
    const title = document.createElement("strong");
    title.textContent = "Media evidence";
    const rollup = document.createElement("span");
    rollup.textContent = [
      `${value.outcomes?.primary_complete || 0} primary`,
      `${value.outcomes?.recovered || 0} recovered`,
      `${value.outcomes?.unavailable || 0} unavailable`,
    ].join(" \u00b7 ");
    summary.append(title, rollup);

    const body = document.createElement("div");
    body.className = "media-acquisition-body";
    const kinds = Object.entries(value.expectedKindCounts || {})
      .filter(([, count]) => Number(count) > 0)
      .map(([kind, count]) => `${count} ${humanize(kind)}`)
      .join(", ");
    const methods = (value.methods || []).map(humanize).join(", ");
    const outcome = document.createElement("p");
    outcome.textContent = [
      `${value.candidateCount || 0} bounded candidate check${value.candidateCount === 1 ? "" : "s"}`,
      kinds ? `expected media: ${kinds}` : "no rendered media root declared",
      `${value.attempts || 0} recovery attempt${value.attempts === 1 ? "" : "s"}`,
      value.foregroundRequiredCount ? `${value.foregroundRequiredCount} foreground-required` : null,
      methods ? `methods: ${methods}` : null,
    ].filter(Boolean).join(" \u00b7 ");
    const note = document.createElement("p");
    note.className = "capture-candidate-note";
    note.textContent = "This telemetry reuses local capture evidence and does not invoke a model.";
    body.append(outcome, note);
    details.append(summary, body);
    return details;
  }

  function buildCaptureSurfaceTelemetry(events) {
    const details = document.createElement("details");
    details.className = "capture-surface-telemetry";
    const summary = document.createElement("summary");
    const title = document.createElement("strong");
    title.textContent = "Capture surface";
    const rollup = document.createElement("span");
    const counts = new Map();
    for (const event of events) {
      counts.set(event.event, (counts.get(event.event) || 0) + 1);
    }
    const cleanup = events.filter((event) => event.detail?.cleanup?.schema === "aku.surface-cleanup.v1")
      .at(-1)?.detail.cleanup;
    const cleanupSummary = cleanup && [cleanup.expected, cleanup.closed, cleanup.retained, cleanup.pending]
      .every((value) => Number.isInteger(value) && value >= 0 && value <= 4096)
      ? `${cleanup.closed} closed / ${cleanup.expected} tracked · ${cleanup.retained} lease-retained · ${cleanup.pending} pending`
      : null;
    rollup.textContent = [
      cleanupSummary,
      ...new Set(events.map((event) => event.detail?.focusPolicyRevision).filter(Boolean)),
      counts.get("created") ? `${counts.get("created")} created` : null,
      counts.get("reused") ? `${counts.get("reused")} reused` : null,
      counts.get("release_requested") ? `${counts.get("release_requested")} release requested` : null,
      counts.get("released") ? `${counts.get("released")} released` : null,
      counts.get("preserved_user_owned") ? `${counts.get("preserved_user_owned")} user-owned preserved` : null,
      counts.get("focus_intervention") ? `${counts.get("focus_intervention")} focus intervention` : null,
    ].filter(Boolean).join(" \u00b7 ");
    summary.append(title, rollup);
    const list = document.createElement("ol");
    list.className = "capture-surface-events";
    const labels = {
      created: "Managed surface created",
      reused: "Managed surface reused",
      release_requested: "Cleanup requested",
      released: "Surface released",
      preserved_user_owned: "User-owned surface preserved",
      focus_intervention: "Focus intervention",
      reconciled: "Stale surface reconciled",
    };
    for (const event of events) {
      const row = document.createElement("li");
      const copy = document.createElement("span");
      const label = document.createElement("strong");
      label.textContent = event.detail?.cleanup ? "Capture cleanup receipt" : labels[event.event] || humanize(event.event);
      const context = document.createElement("small");
      context.textContent = [
        humanize(event.outcome),
        event.detail?.captureSurfacePolicy?.fingerprint
          ? `policy ${String(event.detail.captureSurfacePolicy.fingerprint).slice(0, 12)}` : null,
        event.detail?.isolation ? humanize(event.detail.isolation) : null,
        event.detail?.restored === true ? "working focus restored" : null,
        event.detail?.containmentApplied === true ? "capture minimized; rendering may be throttled" : null,
        event.detail?.focusedWriteAttempted === true ? "explicit foreground requested" : null,
        event.detail?.preservedUserTabs ? `${event.detail.preservedUserTabs} user tab preserved` : null,
      ].filter(Boolean).join(" \u00b7 ");
      copy.append(label, context);
      const time = document.createElement("time");
      time.dateTime = event.occurredAt || "";
      time.textContent = event.occurredAt ? formatDate(event.occurredAt) : "";
      row.append(copy, time);
      list.append(row);
    }
    details.append(summary, list);
    return details;
  }

  function buildAcquisitionIdentityTelemetry(acquisitionPlanning, identityResolution) {
    const details = document.createElement("details");
    details.className = "acquisition-identity-telemetry";
    const summary = document.createElement("summary");
    const title = document.createElement("strong");
    title.textContent = "Acquisition & identity";
    const rollup = document.createElement("span");
    const rollupParts = [];
    if (acquisitionPlanning) {
      const mode = humanize(acquisitionPlanning.mode);
      const decision = humanize(acquisitionPlanning.decision);
      if (mode || decision) {
        rollupParts.push([mode, decision].filter(Boolean).join(" \u00b7 "));
      }
      if (acquisitionPlanning.followUpQueued) {
        rollupParts.push("follow-up queued");
      }
      if (Number(acquisitionPlanning.followUpNewCandidates) > 0) {
        rollupParts.push(`${acquisitionPlanning.followUpNewCandidates} follow-up new`);
      }
    }
    if (identityResolution) {
      rollupParts.push(`${identityResolution.nativePresent || 0} native \u00b7 ${identityResolution.nativeMissing || 0} fallback`);
      if (Number(identityResolution.aliasesReused) > 0) {
        rollupParts.push(`${identityResolution.aliasesReused} alias reused`);
      }
      if (Number(identityResolution.fallbacksPromoted) > 0) {
        rollupParts.push(`${identityResolution.fallbacksPromoted} promoted`);
      }
      if (Number(identityResolution.nativeConflicts) > 0) {
        rollupParts.push(`${identityResolution.nativeConflicts} conflict`);
      }
      if (Number(identityResolution.ambiguousFallbacks) > 0) {
        rollupParts.push(`${identityResolution.ambiguousFallbacks} ambiguous`);
      }
    }
    rollup.textContent = rollupParts.join(" \u00b7 ");
    summary.append(title, rollup);

    const body = document.createElement("div");
    body.className = "acquisition-identity-body";
    if (acquisitionPlanning) {
      const planning = document.createElement("p");
      const planningLabel = document.createElement("strong");
      planningLabel.textContent = "Acquisition planning";
      const planningDetail = document.createElement("span");
      const followUpCount = Number(acquisitionPlanning.followUpNewCandidates);
      planningDetail.textContent = [
        acquisitionPlanning.mode ? `mode ${humanize(acquisitionPlanning.mode)}` : null,
        acquisitionPlanning.decision ? `decision ${humanize(acquisitionPlanning.decision)}` : null,
        acquisitionPlanning.followUpQueued ? "follow-up queued" : "no follow-up queued",
        Number.isFinite(followUpCount)
          ? `${followUpCount} new follow-up candidate${followUpCount === 1 ? "" : "s"}`
          : null,
        acquisitionPlanning.reason ? String(acquisitionPlanning.reason) : null,
      ].filter(Boolean).join(" \u00b7 ");
      planning.append(planningLabel, planningDetail);
      body.append(planning);
    }
    if (identityResolution) {
      const identity = document.createElement("p");
      const identityLabel = document.createElement("strong");
      identityLabel.textContent = "Identity resolution";
      const identityDetail = document.createElement("span");
      identityDetail.textContent = [
        `${identityResolution.nativePresent || 0} native present`,
        `${identityResolution.nativeMissing || 0} native missing`,
        `${identityResolution.aliasesReused || 0} aliases reused`,
        `${identityResolution.fallbacksPromoted || 0} fallbacks promoted`,
        `${identityResolution.nativeConflicts || 0} native conflicts`,
        `${identityResolution.ambiguousFallbacks || 0} ambiguous fallbacks`,
      ].join(" \u00b7 ");
      identity.append(identityLabel, identityDetail);
      body.append(identity);
    }
    const note = document.createElement("p");
    note.className = "capture-candidate-note";
    note.textContent = "This panel reuses local acquisition and identity receipts and does not invoke a model.";
    body.append(note);
    details.append(summary, body);
    return details;
  }

  function inboxSessionFlowText(session) {
    const terminal = ["completed", "partial", "failed", "cancelled"].includes(session.status);
    const skipped = Number(session.skippedResurfaces) || 0;
    const continuity = skipped ? ` · ${skipped} skipped unchanged` : "";
    if (!terminal) {
      const evaluation = (session.capturedCandidates ?? 0) > 0 && (session.evaluatedCandidates ?? 0) === 0
        ? "evaluating candidates"
        : `${session.evaluatedCandidates ?? 0} evaluated`;
      return `${session.capturedCandidates ?? 0} captured${continuity} \u2192 ${evaluation} \u2192 composition pending`;
    }
    return `${session.capturedCandidates} captured${continuity} \u2192 ${session.evaluatedCandidates} evaluated \u2192 ${session.addedItems} unique${session.duplicateReports ? ` + ${session.duplicateReports} duplicate` : ""}`;
  }

  function inboxRunMetricText(run, sessionStatus, stage, value) {
    const runTerminal = ["completed", "failed", "cancelled"].includes(run.status);
    const sessionTerminal = ["completed", "partial", "failed", "cancelled"].includes(sessionStatus);
    if (runTerminal && sessionTerminal) return String(value ?? 0);
    if (stage === "captured") return String(value ?? 0);
    if (stage === "skipped") return String(value ?? 0);
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
    for (const stage of ["captured", "skipped", "evaluated", "selected", "added"]) {
      const button = document.createElement("button");
      button.type = "button";
      button.dataset.stage = stage;
      button.textContent = stage === "skipped" ? "Skipped unchanged" : humanize(stage);
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
        const fallback = stage === "skipped"
          ? run.skippedResurfaces
          : stage === "added"
            ? run.addedItems
            : run[`${stage}Candidates`];
        const count = counts[stage] ?? fallback ?? 0;
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
          const stageLabel = activeStage === "skipped" ? "skipped unchanged" : humanize(activeStage).toLowerCase();
          empty.textContent = `No ${stageLabel} candidates in this source run.`;
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
    row.append(heading, excerpt);
    const selectionReason = inboxSelectionReasonLabel(item.selectionDecision?.reason);
    if (selectionReason) {
      const selection = document.createElement("p");
      selection.className = "inbox-flow-reason";
      selection.textContent = `Selection policy · ${selectionReason}`;
      row.append(selection);
    }
    row.append(reason);
    const actions = document.createElement("div");
    actions.className = "inbox-flow-item-actions";
    const sourceUrl = safeSourceUrl(item.sourceUrl, source);
    if (sourceUrl) {
      const link = document.createElement("a");
      configureNativePostLink(link, sourceUrl, source);
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

  function inboxSelectionReasonLabel(reason) {
    const labels = {
      baseline: "Met the standard selection threshold.",
      preference_promotion: "Promoted by your preferences.",
      protected: "Qualified for protected priority.",
      discovery: "Selected for discovery.",
      previously_delivered: "This post had already been delivered.",
      insufficient_evidence: "Evidence was below the minimum required.",
      below_admission: "Did not meet the selection threshold.",
      preference_suppressed: "Suppressed by your preferences.",
      budget_exhausted: "Qualified, but this source had no selection slots left.",
      displaced_by_protection: "A protected candidate took this slot.",
      displaced_by_discovery: "A discovery candidate took this slot.",
    };
    return Object.hasOwn(labels, reason) ? labels[reason] : "";
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

  return {
    syncInboxSubView, setInboxSubView, loadInbox, renderInbox, buildInboxSession, buildSessionModelUsage, modelUsageMessage, buildModelUsageHelp, renderModelUsageReport, buildModelUsageCategory, modelUsageCategoryProfile, buildModelUsageEntry, modelUsageInvocationCount, modelUsageSummary, formatTokenCount, loadAggregateModelUsage, buildInboxPreferenceDecisions, buildInboxPreferenceDecision, buildAIDetectionDiagnostic, buildAIDetectorYield, buildEventResolutionDiagnostic, buildInboxRun, buildVisionEvaluationTelemetry, inboxRunCaptureReliabilityNotice, buildCaptureCandidateTelemetry, buildMediaAcquisitionTelemetry, buildCaptureSurfaceTelemetry, buildAcquisitionIdentityTelemetry, inboxSessionFlowText, inboxRunMetricText, buildInboxFlowInspector, buildInboxFlowItem, inboxFlowOutcomeLabel, inboxContinuityLabel, inboxStatusTone
  };
}

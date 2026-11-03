import { captureBehaviorDiagnostics } from "./capture-behavior-status.js";
import { confirmSourceCaptureRecovery, renderSourceCapturePause } from "./source-capture-pause.js";

export function createAutoUpdateUi({
  $, api, formatDate, formatSchedulerMoment, formatTokenCount, humanize, renderNativeReaderNotice, renderSession, renderTimelineActions, setNoticeText, setView, showError, sourceLabel, startPolling, state
}) {
  function renderAutoUpdateStatus(status) {
    const capturePause = status?.sourceCapturePause;
    renderSourceCapturePause({ panel: $("#x-capture-pause"), detail: $("#x-capture-pause-detail") }, capturePause);
    renderNativeReaderNotice(status);
    const detail = $("#auto-update-status");
    const queue = $("#auto-update-queue-status");
    const modeBadge = $("#auto-update-mode-badge");
    const modeNotice = $("#auto-update-mode-notice");
    const stateBadge = $("#auto-update-state-badge");
    const preparedMetric = $("#auto-update-prepared-metric");
    const runwayLabel = $("#auto-update-runway-label");
    const runwayMetric = $("#auto-update-runway-metric");
    const resultMetric = $("#auto-update-result-metric");
    const pressureLabel = $("#auto-update-pressure-label");
    const pressureMetric = $("#auto-update-pressure-metric");
    const allowanceLabel = $("#auto-update-allowance-label");
    const allowanceMetric = $("#auto-update-allowance-metric");
    const nextMetric = $("#auto-update-next-metric");
    const budget = $("#auto-update-budget-status");
    const budgetDetail = $("#auto-update-budget-detail");
    const reset = $("#reset-auto-update-budget");
    const confirmRestored = $("#confirm-codex-usage-restored");
    const runNow = $("#prepare-batch-now");
    const timeline = $("#auto-update-timeline-status");
    const timelineAction = $("#auto-update-timeline-settings");
    if (!detail || !queue || !modeBadge || !modeNotice || !stateBadge || !preparedMetric || !runwayLabel || !runwayMetric || !resultMetric || !pressureLabel || !pressureMetric || !allowanceLabel || !allowanceMetric || !nextMetric || !budget || !budgetDetail || !timeline) return;
    if (state.bootstrap && status) state.bootstrap.autoUpdate = status;
    if (!status) {
      detail.textContent = "Local scheduler telemetry is not available yet.";
      queue.textContent = "—";
      modeBadge.textContent = "Unknown";
      modeNotice.textContent = "";
      modeNotice.classList.add("hidden");
      stateBadge.textContent = "Unavailable";
      stateBadge.className = "auto-update-state-pill auto-update-state-neutral";
      for (const metric of [preparedMetric, runwayMetric, resultMetric, pressureMetric, allowanceMetric, nextMetric]) metric.textContent = "—";
      budget.textContent = "Daily token telemetry is not available yet.";
      budgetDetail.textContent = "—";
      timeline.classList.add("hidden");
      renderTimelineActions();
      return;
    }
    const used = status.dailyTokensUsed || 0;
    const quotaUsed = status.quotaTokensUsed ?? used;
    const dailyBudget = status.dailyTokenBudget || 0;
    const dailyRemaining = status.dailyTokensRemaining || 0;
    const automaticRemaining = status.automaticTokensRemaining || 0;
    const reserve = status.manualReserveTokens || 0;
    const estimate = status.estimatedNextRunTokens || 0;
    const adaptive = status.mode === "adaptive";
    const liveMode = adaptive ? "adaptive" : "fixed";
    const configuredMode = state.bootstrap?.settings?.autoUpdateMode || liveMode;
    const selectedMode = $("#auto-update-mode")?.value || configuredMode;
    const unsavedModeChange = selectedMode !== configuredMode;
    const applyingModeChange = !unsavedModeChange && configuredMode !== liveMode;
    modeBadge.textContent = autoUpdateModeLabel(liveMode);
    modeNotice.classList.toggle("hidden", !unsavedModeChange && !applyingModeChange);
    if (unsavedModeChange) {
      modeNotice.textContent = `Unsaved change: ${autoUpdateModeDescription(selectedMode)}. Status below still reflects the current ${autoUpdateModeDescription(liveMode)} scheduler.`;
    } else if (applyingModeChange) {
      modeNotice.textContent = `Applying ${autoUpdateModeDescription(configuredMode)}. Status below will switch after the scheduler confirms the new policy.`;
    } else {
      modeNotice.textContent = "";
    }
    const stateTone = status.state === "running"
      ? "active"
      : ["paused", "budget_paused"].includes(status.state)
        ? "warning"
        : status.state === "usage_limit_paused" ? "danger" : "neutral";
    stateBadge.textContent = humanize(status.state || "idle");
    stateBadge.className = `auto-update-state-pill auto-update-state-${stateTone}`;
    detail.textContent = status.reason || (status.enabled ? "Automatic checks are ready." : "Automatic checks are off.");

    const prepared = status.preparedBatches?.length || 0;
    const limit = status.preparedBatchLimit || prepared;
    const available = status.availablePreparedSlots ?? Math.max(0, limit - prepared);
    preparedMetric.textContent = `${prepared} / ${limit} batches`;
    runwayLabel.textContent = adaptive ? "Ready posts" : "Open slots";
    runwayMetric.textContent = adaptive
      ? `${status.adaptiveReadyItems || 0} / ${status.adaptiveReadyItemTarget || 1} posts`
      : `${available} available`;

    const latestReceipt = status.schedulerReceipts?.[0];
    const outcomeSources = (status.lastAdaptiveOutcomeCompletedSources || 0)
      + (status.lastAdaptiveOutcomeFailedSources || 0)
      + (status.lastAdaptiveOutcomeCancelledSources || 0);
    const failedOutcomeSources = status.lastAdaptiveOutcomeFailedSources || 0;
    const cancelledOutcomeSources = status.lastAdaptiveOutcomeCancelledSources || 0;
    const outcomeSourceDetail = outcomeSources > 0
      ? `${status.lastAdaptiveOutcomeCompletedSources || 0}/${outcomeSources} source${outcomeSources === 1 ? "" : "s"} completed${failedOutcomeSources ? `, ${failedOutcomeSources} failed` : ""}${cancelledOutcomeSources ? `, ${cancelledOutcomeSources} cancelled` : ""}`
      : "source counts unavailable";
    resultMetric.textContent = status.lastAdaptiveOutcome
      ? `${humanize(status.lastAdaptiveOutcome)} · ${status.lastAdaptiveOutcomeItems || 0} post${status.lastAdaptiveOutcomeItems === 1 ? "" : "s"}`
      : latestReceipt ? humanize(latestReceipt.outcome) : "No result yet";
    pressureLabel.textContent = adaptive ? "Pressure" : "Cadence";
    pressureMetric.textContent = adaptive
      ? `${humanize(status.replenishmentPressureTier || "low")} · ${status.replenishmentPressure || 0}/100`
      : `${status.cadenceMinutes || status.refillIntervalMinutes || 0} min`;
    allowanceLabel.textContent = adaptive ? `Attempts (${status.generationWindowMinutes || 30}m)` : "Policy";
    allowanceMetric.textContent = adaptive
      ? `${status.generationAllowanceUsed || 0} / ${status.generationAllowanceLimit || limit}`
      : "Continuous";
    nextMetric.textContent = status.state === "usage_limit_paused"
      ? "Needs confirmation"
      : status.state === "budget_paused"
        ? "Waiting for budget"
        : status.state === "disabled"
          ? "Off"
          : status.nextCheckAt ? formatSchedulerMoment(status.nextCheckAt) : "Ready when needed";

    const diagnostics = [];
    if (adaptive) {
      const baseTarget = status.adaptiveBaseTargetBatches || status.adaptiveTargetBatches || 1;
      diagnostics.push(`Target ${status.adaptiveTargetBatches || 1}${baseTarget > status.adaptiveTargetBatches ? ` of learned ${baseTarget}` : ""} batch${baseTarget === 1 ? "" : "es"}`);
      diagnostics.push(status.consumptionPaceMinutes ? `Pace ~${status.consumptionPaceMinutes}m/batch` : "Learning consumption pace");
      if (status.pressureAdditionalDelayMinutes) diagnostics.push(`Pressure adds ${status.pressureAdditionalDelayMinutes}m spacing`);
    } else {
      diagnostics.push(`${humanize(status.cadenceTier || "continuous")} cadence ${status.cadenceMinutes || status.refillIntervalMinutes || 0}m`);
    }
    if (latestReceipt) diagnostics.push(`Last tick ${humanize(latestReceipt.outcome)}${latestReceipt.reason ? `: ${latestReceipt.reason}` : ""}`);
    if (status.lastAdaptiveOutcome) diagnostics.push(`Last outcome: ${outcomeSourceDetail}`);
    diagnostics.push(`Next estimate ${formatTokenCount(estimate)}`);
    if (status.pressureRefillNotBefore && Date.parse(status.pressureRefillNotBefore) > Date.now()) diagnostics.push(`Pressure spacing until ${formatDate(status.pressureRefillNotBefore)}`);
    if (adaptive && status.adaptiveReadingGraceUntil && Date.parse(status.adaptiveReadingGraceUntil) > Date.now()) diagnostics.push(`Reading grace until ${formatDate(status.adaptiveReadingGraceUntil)}`);
    if (status.supplyBackoffUntil && Date.parse(status.supplyBackoffUntil) > Date.now()) diagnostics.push(`Supply cooldown until ${formatDate(status.supplyBackoffUntil)}`);
    if (status.technicalBackoffUntil && Date.parse(status.technicalBackoffUntil) > Date.now()) diagnostics.push(`Retry cooldown until ${formatDate(status.technicalBackoffUntil)}`);
    diagnostics.push(...captureBehaviorDiagnostics(status.captureBehavior, { sourceLabel, formatMoment: formatDate }));
    queue.textContent = diagnostics.join(" · ");
    const actual = quotaUsed === used ? "" : ` · ${formatTokenCount(used)} actual today`;
    const manualReset = status.lastManualBudgetResetAt ? ` · quota reset ${formatDate(status.lastManualBudgetResetAt)}` : "";
    budget.textContent = `${formatTokenCount(quotaUsed)} of ${formatTokenCount(dailyBudget)} quota used${actual}${manualReset} · ${formatTokenCount(dailyRemaining)} remaining · resets ${formatDate(status.budgetResetAt)}`;
    budgetDetail.textContent = `${formatTokenCount(automaticRemaining)} automatic · ${formatTokenCount(reserve)} user reserve`;
    const usageLimitPaused = status.state === "usage_limit_paused";
    if (timelineAction) timelineAction.textContent = usageLimitPaused ? "Confirm usage restored" : "Review Auto Update";
    if (reset) reset.disabled = Boolean(state.session);
    if (confirmRestored) {
      confirmRestored.classList.toggle("hidden", !usageLimitPaused);
      confirmRestored.disabled = Boolean(state.session);
    }
    if (runNow) runNow.disabled = Boolean(state.session) || !status.enabled || status.state === "running" || usageLimitPaused;
    renderTimelineActions();
    const paused = status.enabled && ["paused", "budget_paused", "usage_limit_paused"].includes(status.state);
    timeline.classList.toggle("hidden", !paused);
    if (paused) {
      $("#auto-update-timeline-title").textContent = usageLimitPaused
        ? "Auto Update paused by Codex usage limit"
        : status.state === "budget_paused" ? "Auto Update paused by today’s budget" : "Auto Update paused";
      $("#auto-update-timeline-detail").textContent = usageLimitPaused
        ? `Codex reported a usage limit${status.usageLimitPausedAt ? ` at ${formatDate(status.usageLimitPausedAt)}` : ""}. Automatic checks will not retry until you confirm in Settings that usage is restored.`
        : status.state === "budget_paused"
          ? `${formatTokenCount(dailyRemaining)} quota remains; the next run is estimated at ${formatTokenCount(estimate)}. Increase or reset today’s local quota in Settings, or wait until ${formatDate(status.budgetResetAt)}.`
          : status.reason || "Automatic work is waiting for an available boundary.";
    }
  }

  function autoUpdateModeLabel(mode) {
    return mode === "fixed" ? "Continuous" : "Adaptive";
  }

  function autoUpdateModeDescription(mode) {
    return mode === "fixed" ? "Continuous background" : "Adaptive demand";
  }

  function syncAutoUpdateModeSettings() {
    const intervalRow = $("#continuous-background-interval-row");
    if (!intervalRow) return;
    intervalRow.classList.toggle("hidden", $("#auto-update-mode").value !== "fixed");
    renderAutoUpdateStatus(state.bootstrap?.autoUpdate);
  }

  async function prepareBatchNow() {
    if (state.session) {
      showError(new Error("Finish or cancel the active check before starting another update."));
      return;
    }
    const button = $("#prepare-batch-now");
    button.disabled = true;
    try {
      const { session } = await api("/api/auto-update/prepare", { method: "POST" });
      state.session = session;
      setView("timeline");
      renderSession();
      startPolling();
    } catch (error) {
      showError(error);
      button.disabled = false;
    }
  }

  function openAutoUpdateSettings() {
    setView("settings");
    requestAnimationFrame(() => document.querySelector("#auto-update-enabled")?.closest("fieldset")?.scrollIntoView({ block: "start", behavior: "smooth" }));
  }

  function handleAutoUpdateTimelineAction() {
    if (state.bootstrap?.autoUpdate?.state === "usage_limit_paused") {
      confirmCodexUsageRestored();
      return;
    }
    openAutoUpdateSettings();
  }

  async function resetAutoUpdateBudget() {
    if (state.session) {
      showError(new Error("Finish or cancel the active check before resetting today’s Auto Update quota."));
      return;
    }
    if (!window.confirm("Reset today’s local Auto Update quota? Usage history remains intact, but automatic work may start again and spend more model tokens today.")) return;
    const button = $("#reset-auto-update-budget");
    button.disabled = true;
    try {
      const { autoUpdate } = await api("/api/auto-update/budget/reset", { method: "POST" });
      if (state.bootstrap) state.bootstrap.autoUpdate = autoUpdate;
      renderAutoUpdateStatus(autoUpdate);
      const notice = $("#provider-notice");
      notice.className = "notice notice-complete";
      notice.setAttribute("role", "status");
      const providerPause = autoUpdate.state === "usage_limit_paused" ? " The Codex provider pause is unchanged; confirm restored usage separately." : "";
      setNoticeText(notice, `Today’s local Auto Update quota was reset. Recorded model usage was preserved.${providerPause}`);
    } catch (error) {
      showError(error);
    } finally {
      button.disabled = Boolean(state.session);
    }
  }

  async function confirmXCaptureRecovery() {
    const pause = state.bootstrap?.autoUpdate?.sourceCapturePause;
    if (!pause) return;
    if (!window.confirm("Have you completed verification manually in X? This allows another capture attempt; it does not verify X automatically or turn Auto Update on.")) return;
    const button = $("#confirm-x-capture-recovery");
    button.disabled = true;
    try {
      const { autoUpdate } = await confirmSourceCaptureRecovery(api, pause, true);
      state.bootstrap.autoUpdate = autoUpdate;
      renderAutoUpdateStatus(autoUpdate);
    } catch (error) { showError(error); }
    finally { button.disabled = false; }
  }

  async function confirmCodexUsageRestored() {
    if (state.session) {
      showError(new Error("Finish or cancel the active check before confirming restored Codex usage."));
      return;
    }
    if (!window.confirm("Confirm that your Codex usage limit has been restored? Auto Update may resume immediately.")) return;
    const buttons = [$("#confirm-codex-usage-restored"), $("#auto-update-timeline-settings")].filter(Boolean);
    for (const button of buttons) button.disabled = true;
    try {
      const { autoUpdate } = await api("/api/auto-update/usage-limit/restore", { method: "POST" });
      if (state.bootstrap) state.bootstrap.autoUpdate = autoUpdate;
      renderAutoUpdateStatus(autoUpdate);
      const notice = $("#provider-notice");
      notice.className = "notice notice-complete";
      notice.setAttribute("role", "status");
      setNoticeText(notice, "Codex usage restoration confirmed. Auto Update may resume at the next eligible scheduler boundary.");
    } catch (error) {
      showError(error);
    } finally {
      for (const button of buttons) button.disabled = Boolean(state.session);
    }
  }

  return {
    renderAutoUpdateStatus, autoUpdateModeLabel, autoUpdateModeDescription, syncAutoUpdateModeSettings, prepareBatchNow, openAutoUpdateSettings, handleAutoUpdateTimelineAction, resetAutoUpdateBudget, confirmXCaptureRecovery, confirmCodexUsageRestored
  };
}

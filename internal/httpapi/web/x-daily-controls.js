import { setSettingsClass, setSettingsText } from "./settings-render.js";

const X_DAILY_START = "/api/development/x-daily/start";
const X_DAILY_STOP = "/api/development/x-daily/stop";
const X_RUNTIME = "/api/collection/runtime";
const START_CONSENT = Object.freeze({
  acknowledgeLimitations: true,
  allowProcessSessionDerivation: true,
});
const VALID_STATES = new Set([
  "selected", "not-ready", "disabled", "ready", "capturing", "starting", "startup-in-progress", "stopping", "stopped", "failed",
]);
const VALID_CLEANUP = new Set(["", "verified", "pending", "uncertain"]);

function unwrapStatus(value) {
  if (value && typeof value === "object" && Object.hasOwn(value, "xCapture")) return value.xCapture;
  return value;
}

function normalizeStatus(value) {
  const status = unwrapStatus(value);
  if (!status || typeof status !== "object" || Array.isArray(status)) return null;
  if (
    typeof status.selectedMode !== "string" ||
    typeof status.authorizationPolicy !== "string" ||
    !Number.isInteger(status.maxBatchMs) ||
    typeof status.state !== "string" ||
    !VALID_STATES.has(status.state) ||
    typeof status.reason !== "string" ||
    typeof status.derivationAuthorized !== "boolean"
  ) return null;
  const cleanup = status.cleanup ?? "";
  if (typeof cleanup !== "string" || !VALID_CLEANUP.has(cleanup)) return null;
  if (status.rememberedPermission !== undefined && typeof status.rememberedPermission !== "boolean") return null;
  return {
    selectedMode: status.selectedMode,
    authorizationPolicy: status.authorizationPolicy,
    maxBatchMs: status.maxBatchMs,
    state: status.state,
    reason: status.reason,
    derivationAuthorized: status.derivationAuthorized,
    cleanup,
    rememberedPermission: status.rememberedPermission === true,
  };
}

function readContextSafely(readContext) {
  try {
    const context = readContext?.();
    return {
      epoch: typeof context?.epoch === "string" && context.epoch.trim() ? context.epoch : "",
      development: context?.development === true,
    };
  } catch {
    return { epoch: "", development: false };
  }
}

function setHidden(element, hidden) {
  if (element && element.hidden !== hidden) element.hidden = hidden;
}

function setDisabled(element, disabled) {
  if (element && element.disabled !== disabled) element.disabled = disabled;
}

function setChecked(element, checked) {
  if (element && element.checked !== checked) element.checked = checked;
}

function statusLabel(status) {
  if (!status) return "Checking status";
  if (status.state === "capturing") return "Capturing";
  if (status.state === "starting" || status.state === "startup-in-progress") return "Starting";
  if (status.state === "stopping") return "Stopping";
  if (status.cleanup === "uncertain") return "Cleanup uncertain";
  if (status.state === "stopped" && ["reader-start-failed", "batch-failed"].includes(status.reason)) {
    return "Failed — restart required";
  }
  if (status.cleanup === "pending") return "Cleanup pending";
  if (status.state === "disabled" && status.reason === "process-consent-required") return "Consent required";
  if (status.state === "ready") return "Ready";
  if (status.state === "failed") return "Failed — restart required";
  if (status.state === "stopped") {
    return "Stopped — restart required";
  }
  if (status.state === "not-ready") return "Unavailable";
  return "Not ready";
}

function statusDetail(status) {
  if (!status) return "Daily X controls are available only for a development concurrent-mode session.";
  if (status.authorizationPolicy === "profile") {
    if (status.cleanup === "pending" || status.cleanup === "uncertain") {
      return "X cleanup is not yet verified. Your saved permission is unchanged; no new batch can start until cleanup completes.";
    }
    if (status.state === "ready" || status.state === "capturing") {
      return "X permission is saved for this browser profile. Each batch verifies the account and session again. Temporary Home and collector targets close after the update; your reader stays open.";
    }
    if (status.state === "disabled") {
      return "Enable X to save permission for this browser profile across restarts. Each batch is limited to 60 seconds and requires fresh account and session verification.";
    }
    if (status.state === "stopped" && !status.rememberedPermission) {
      return "X is off and saved permission has been removed. Restart AkuBrowser before enabling X again.";
    }
    if (status.state === "failed" || status.state === "stopped") {
      return "X is unavailable in this process. Saved permission is not a successful account or session verification; inspect the startup status before retrying.";
    }
  }
  if (status.cleanup === "uncertain") return "Cleanup could not be confirmed. Stop can request cleanup again; starting remains disabled.";
  if (status.state === "stopped" && ["reader-start-failed", "batch-failed"].includes(status.reason)) {
    return status.cleanup === "pending"
      ? "Daily X failed and cleanup is pending. Wait for a confirmed status; restart AkuSidecar before requesting permission again."
      : "Daily X failed. Restart AkuSidecar before requesting process permission again.";
  }
  if (status.state === "capturing") {
    return "Process permission is active. Stop ends it, and a restart is required before permission can be granted again.";
  }
  if (status.state === "starting" || status.state === "startup-in-progress" || status.state === "stopping") {
    return "Daily X is starting or stopping. Wait for a confirmed status before taking another action.";
  }
  if (status.cleanup === "pending") return "Daily X cleanup is pending. Wait for a confirmed status before taking another action.";
  if (status.state === "disabled" && status.reason === "process-consent-required") {
    return "Use your existing X login for batches up to 60 seconds. Stop ends permission until AkuBrowser restarts.";
  }
  if (status.state === "ready") {
    return "Process permission is active. Collector windows close after each batch; the X Home bootstrap remains until Stop X. A restart is required before permission can be granted again.";
  }
  if (status.state === "failed") return "Daily X failed. Restart AkuSidecar before requesting process permission again.";
  if (status.state === "stopped") {
    return "X capture is off. Restart AkuBrowser before granting permission again.";
  }
  if (status.state === "not-ready") return "Daily X is not ready in this session.";
  return "Daily X is not ready for process permission.";
}

export function xDailyView(statusValue, contextValue, consentChecked, busy = false, needsRefresh = false) {
  const status = normalizeStatus(statusValue);
  const context = contextValue ?? { epoch: "", development: false };
  const visible = context.development === true && typeof context.epoch === "string" && context.epoch.trim() !== "" &&
    status?.selectedMode === "concurrent" && ["process", "profile"].includes(status.authorizationPolicy) && status.maxBatchMs === 60000;
  const cleanupUnresolved = status?.cleanup === "pending" || status?.cleanup === "uncertain";
  const terminal = !cleanupUnresolved && (status?.state === "stopped" || status?.state === "failed" || status?.reason === "opted-out" ||
    status?.reason === "process-authorization-ended");
  const canStart = visible && !busy && !needsRefresh && consentChecked === true && status.state === "disabled" &&
    ["process-consent-required", "profile-consent-required"].includes(status.reason) && status.derivationAuthorized === false && !cleanupUnresolved;
  const canStop = visible && !busy && (status.state === "ready" || status.state === "capturing" ||
    status.state === "starting" || status.state === "startup-in-progress" || status.state === "stopping" || cleanupUnresolved ||
    status.rememberedPermission);
  return {
    visible,
    startEnabled: canStart,
    stopEnabled: canStop && (!terminal || status.rememberedPermission),
    statusText: statusLabel(status),
    detailText: statusDetail(status),
    terminal,
  };
}

export function createXDailyControls({
  panel,
  status,
  detail,
  consent,
  startButton,
  stopButton,
  feedback,
  api,
  readContext,
  onStatus = () => {},
}) {
  let latestStatus = null;
  let destroyed = false;
  let busy = false;
  let busyAction = "";
  let needsRefresh = true;
  let revision = 0;
  let operationId = 0;
  let observedContext = readContextSafely(readContext);

  function sameContext(left, right) {
    return left.epoch === right.epoch && left.development === right.development;
  }

  function syncContext() {
    const current = readContextSafely(readContext);
    if (!sameContext(current, observedContext)) {
      observedContext = current;
      revision++;
      operationId++;
      busy = false;
      busyAction = "";
      latestStatus = null;
      needsRefresh = true;
      setChecked(consent, false);
      showFeedback("");
      render();
    }
    return current;
  }

  function publishStatus(nextStatus) {
    latestStatus = nextStatus;
    if (nextStatus.authorizationPolicy === "profile") setChecked(consent, nextStatus.rememberedPermission);
    needsRefresh = false;
    try {
      onStatus(nextStatus);
    } catch {
      // UI observers must not interfere with consent or lifecycle controls.
    }
  }

  function render() {
    if (destroyed) return;
    const context = syncContext();
    const view = xDailyView(latestStatus, context, consent?.checked === true, busy, needsRefresh);
    setHidden(panel, !view.visible);
    setDisabled(consent, !view.visible || busy || view.terminal || !latestStatus ||
      latestStatus.state !== "disabled" || !["process-consent-required", "profile-consent-required"].includes(latestStatus.reason) ||
      latestStatus.derivationAuthorized !== false || latestStatus.cleanup === "pending" ||
      latestStatus.cleanup === "uncertain" || needsRefresh);
    setDisabled(startButton, !view.startEnabled);
    setDisabled(stopButton, !view.stopEnabled);
    const displayedStatus = busy
      ? (busyAction === "stop" ? "Stopping" : "Starting")
      : (needsRefresh && latestStatus ? "Status unavailable" : view.statusText);
    const displayedDetail = busy
      ? "Daily X is starting or stopping. Wait for a confirmed status before taking another action."
      : (needsRefresh && latestStatus
        ? "The latest Daily X status could not be confirmed. Stop remains available if permission was previously active; starting is disabled."
        : view.detailText);
    setSettingsText(status, displayedStatus);
    setSettingsClass(status, `x-daily-status${view.visible ? "" : " x-daily-status-hidden"}`);
    setSettingsText(detail, displayedDetail);
  }

  function beginRead() {
    if (destroyed) return { epoch: "", revision: -1 };
    const context = syncContext();
    revision++;
    return { epoch: context.epoch, revision };
  }

  function applyRead(ticket, value) {
    if (destroyed) return false;
    const context = syncContext();
    if (!ticket || busy || ticket.epoch !== context.epoch || ticket.revision !== revision) return false;
    const nextStatus = normalizeStatus(value);
    if (!nextStatus) {
      needsRefresh = true;
      setChecked(consent, false);
      render();
      return false;
    }
    publishStatus(nextStatus);
    render();
    return true;
  }

  function rejectRead(ticket) {
    if (destroyed) return false;
    const context = syncContext();
    if (!ticket || busy || ticket.epoch !== context.epoch || ticket.revision !== revision) return false;
    needsRefresh = true;
    setChecked(consent, latestStatus?.authorizationPolicy === "profile" && latestStatus.rememberedPermission);
    render();
    return true;
  }

  function reset(value) {
    if (destroyed) return;
    const context = syncContext();
    revision++;
    operationId++;
    busy = false;
    busyAction = "";
    setChecked(consent, false);
    showFeedback("");
    const nextStatus = normalizeStatus(value);
    if (nextStatus && context.development && context.epoch) publishStatus(nextStatus);
    else {
      latestStatus = null;
      needsRefresh = true;
    }
    render();
  }

  function showFeedback(message) {
    setSettingsText(feedback, message);
  }

  function adoptMutationStatus(value) {
    const nextStatus = normalizeStatus(value);
    if (!nextStatus) return false;
    publishStatus(nextStatus);
    return true;
  }

  async function reconcileAfterError(epoch, token) {
    const current = syncContext();
    if (destroyed || token !== operationId || current.epoch !== epoch || !current.development || !epoch) return;
    const ticket = beginRead();
    try {
      const result = await api(X_RUNTIME, { instanceEpoch: epoch });
      const latest = syncContext();
      if (destroyed || token !== operationId || latest.epoch !== epoch || !latest.development) return;
      if (applyRead(ticket, result)) {
        showFeedback("Current Daily X status was refreshed.");
      } else {
        showFeedback("Current status could not be confirmed. Starting remains disabled.");
      }
    } catch {
      const latest = syncContext();
      if (!destroyed && token === operationId && latest.epoch === epoch && latest.development) {
        showFeedback("Current status could not be confirmed. Starting remains disabled.");
        render();
      }
    }
  }

  async function mutate(kind) {
    if (destroyed || busy) return;
    const context = syncContext();
    const view = xDailyView(latestStatus, context, consent?.checked === true, busy, needsRefresh);
    if (!context.development || !context.epoch || (kind === "start" ? !view.startEnabled : !view.stopEnabled)) return;

    const epoch = context.epoch;
    const token = ++operationId;
    revision++;
    busy = true;
    busyAction = kind;
    needsRefresh = true;
    setChecked(consent, false);
    showFeedback(kind === "start" ? "Starting daily X…" : "Stopping daily X…");
    render();

    let result = null;
    let failed = false;
    try {
      result = await api(kind === "start" ? X_DAILY_START : X_DAILY_STOP, {
        method: "POST",
        body: kind === "start" ? { ...START_CONSENT } : {},
        instanceEpoch: epoch,
      });
    } catch {
      failed = true;
    }

    const current = syncContext();
    if (destroyed || token !== operationId || current.epoch !== epoch || !current.development) return;
    busy = false;
    busyAction = "";
    revision++;

    if (!failed && adoptMutationStatus(result)) {
      if (kind === "stop" && latestStatus.state === "stopped" &&
          latestStatus.cleanup !== "pending" && latestStatus.cleanup !== "uncertain") {
        showFeedback(latestStatus.authorizationPolicy === "profile" && !latestStatus.rememberedPermission
          ? "X is stopped and saved permission has been removed. Restart AkuBrowser before enabling X again."
          : ["reader-start-failed", "batch-failed"].includes(latestStatus.reason)
          ? "Daily X failed. Restart AkuSidecar before requesting permission again."
          : "Daily X stopped. Restart AkuSidecar before requesting permission again.");
      } else if (kind === "start") {
        showFeedback(latestStatus.state === "capturing" ? "Daily X is capturing." : "Daily X is ready.");
      } else {
        showFeedback("Daily X status updated.");
      }
      render();
      return;
    }

    showFeedback("Daily X action could not be confirmed. Checking current status.");
    render();
    await reconcileAfterError(epoch, token);
  }

  const onConsentChange = () => render();
  const onStartClick = (event) => {
    event?.preventDefault?.();
    void mutate("start");
  };
  const onStopClick = (event) => {
    event?.preventDefault?.();
    void mutate("stop");
  };
  consent?.addEventListener?.("change", onConsentChange);
  startButton?.addEventListener?.("click", onStartClick);
  stopButton?.addEventListener?.("click", onStopClick);
  render();

  return {
    reset,
    beginRead,
    applyRead,
    rejectRead,
    destroy() {
      if (destroyed) return;
      destroyed = true;
      revision++;
      operationId++;
      busy = false;
      consent?.removeEventListener?.("change", onConsentChange);
      startButton?.removeEventListener?.("click", onStartClick);
      stopButton?.removeEventListener?.("click", onStopClick);
    },
  };
}

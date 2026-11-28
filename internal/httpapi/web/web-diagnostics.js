// The page owns console delivery. Durable native-post records keep their tab-local schema.
(() => {
  const recorders = new Map();
  const issues = Object.freeze({
    user_activity: "warn",
    auto_update_refresh: "warn",
    database_health_refresh: "warn",
    dispatch_stopped: "warn",
    action_relay: "warn",
    notification_refresh: "warn",
    session_cancel_cleanup: "warn",
    next_source_cleanup: "warn",
    session_complete_cleanup: "warn",
    missing_session_cleanup: "warn",
    media_enrichment: "debug",
    media_evidence: "debug",
    video_refresh_queue: "warn",
    show_error: "error",
  });
  const reasons = new Set(["sidecar_unavailable", "sidecar_epoch_changed", "invalid_bridge_token",
    "invalid_bridge_contract", "capture_instance_mismatch", "timeout", "cleanup_pending"]);
  const feedbackPhases = new Set(["click", "request_start", "request_end", "rendered"]);
  const uuid = /^[\da-f]{8}-[\da-f]{4}-[\da-f]{4}-[\da-f]{4}-[\da-f]{12}$/i;

  function emit(operation, phase, marker, level, details) {
    try {
      if (!recorders.has(operation)) recorders.set(operation, globalThis.AkuDiagnosticRecorder.create(operation));
      const recorder = recorders.get(operation);
      recorder.advance(phase);
      const { location, sequence } = recorder.snapshot();
      console[level](marker, { ...details, ...location, sequence, level });
    } catch { /* A console sink must never interrupt a UI action. */ }
  }

  function issue(phase, error, runId) {
    if (!Object.hasOwn(issues, phase)) return;
    const code = error?.code;
    const message = typeof error === "string" ? error : error?.message;
    let reason = "unavailable";
    if (reasons.has(code) || typeof code === "string" && /^http_[1-5]\d\d$/.test(code)) reason = code;
    else if (error?.name === "AbortError") reason = "aborted";
    else if (error?.name === "TimeoutError") reason = "timeout";
    else if (message === "AkuBridge did not provide a verified capture-surface cleanup receipt.") reason = "cleanup_unverified";
    else if (message === "AkuBridge rejected cleanup because the capture lease changed.") reason = "lease_mismatch";
    else if (window.akuNativePostDiagnostics) {
      const kind = window.akuNativePostDiagnostics.errorKind(typeof message === "string" ? message : "");
      if (kind !== "other") reason = kind;
    }
    const details = { reason };
    if (typeof runId === "string" && /^run_[a-f0-9]{32}$/.test(runId)) details.runId = runId;
    emit("web_ui", phase, "ui_activity", issues[phase], details);
  }

  function feedback(trace, phase, details = {}) {
    if (!feedbackPhases.has(phase)) return;
    const row = { trace: typeof trace === "string" && uuid.test(trace) ? trace : "invalid" };
    if (["more", "less"].includes(details.direction)) row.direction = details.direction;
    if (["ok", "error"].includes(details.outcome)) row.outcome = details.outcome;
    if (Number.isSafeInteger(details.elapsedMs) && details.elapsedMs >= 0) row.elapsedMs = details.elapsedMs;
    emit("timeline_feedback", phase, "timeline_feedback_trace", "info", row);
  }

  globalThis.AkuWebDiagnostics = Object.freeze({
    issue, feedback,
    nativePost(entry) { emit("native_post", entry.phase, "native_post_trace", "info", entry); },
    recovered(events) {
      emit("native_post", "recovered", "native_post_trace_recovered", "info", { count: events.length, events });
    },
  });
})();

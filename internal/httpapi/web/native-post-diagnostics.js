// A small, tab-local trace survives a UI reload so pre-queue failures remain inspectable.
// Only allowlisted diagnostic fields are stored; native URLs and bridge tokens never are.
(() => {
  const key = "akuNativePostDiagnostics.v1";
  const limit = 64;
  const maxAgeMs = 24 * 60 * 60 * 1000;
  const tracePattern = /^(?:broker_[a-f0-9]{32}|pointer_[a-f0-9]{32}|native_post_\d+_[a-f0-9]+)$/;
  const phases = new Set([
    "broker_ready", "broker_unavailable",
    "pointerdown", "click_ignored", "click", "dispatch", "terminal",
    "relay_received", "relay_bootstrap_done", "relay_request_start",
    "relay_request_end", "relay_error",
  ]);
  const outcomes = new Set(["opened", "rejected", "timeout", "bridge_unavailable", "ignored"]);
  const errorKinds = new Set([
    "broker_not_ready", "broker_click_missing", "ui_not_foreground", "broker_busy", "broker_identity",
    "extension_unavailable", "native_host_unavailable", "reader_activation",
    "epoch_mismatch", "bootstrap_failed", "transport_busy", "network",
    "timeout", "other",
  ]);

  function errorKind(error) {
    const message = String(error?.message ?? error ?? "");
    if (/did not receive a trusted click/i.test(message)) return "broker_click_missing";
    if (/UI reader broker is not ready/i.test(message)) return "broker_not_ready";
    if (/UI must be active|UI must remain foreground|UI foreground changed/i.test(message)) return "ui_not_foreground";
    if (/Reader broker is unavailable or busy/i.test(message)) return "broker_busy";
    if (/Reader (?:helper|broker server) identity rejected/i.test(message)) return "broker_identity";
    if (/Extension context invalidated|Receiving end does not exist|Could not establish connection/i.test(message)) return "extension_unavailable";
    if (/native messaging host|Native host has exited/i.test(message)) return "native_host_unavailable";
    if (/Windows rejected reader activation|reader helper activation rejected|Native reader foreground was not verified/i.test(message)) return "reader_activation";
    if (/AkuBrowser restarted|capture_epoch_mismatch/i.test(message)) return "epoch_mismatch";
    if (/Capture transport bootstrap failed/i.test(message)) return "bootstrap_failed";
    if (/Capture transport is unavailable or busy/i.test(message)) return "transport_busy";
    if (/timed out|timeout/i.test(message)) return "timeout";
    if (/Failed to fetch|NetworkError/i.test(message)) return "network";
    return "other";
  }

  function safeEntry(value) {
    if (!value || typeof value !== "object" || !phases.has(value.phase)) return null;
    const at = Date.parse(value.at);
    if (!Number.isFinite(at) || at > Date.now() + 60_000 || Date.now() - at > maxAgeMs) return null;
    const entry = {
      at: new Date(at).toISOString(),
      trace: tracePattern.test(value.trace ?? "") ? value.trace : "invalid",
      phase: value.phase,
    };
    if (tracePattern.test(value.gesture ?? "") && value.gesture.startsWith("pointer_")) entry.gesture = value.gesture;
    if (outcomes.has(value.outcome)) entry.outcome = value.outcome;
    if (errorKinds.has(value.errorKind)) entry.errorKind = value.errorKind;
    if (Number.isInteger(value.status) && value.status >= 100 && value.status <= 599) entry.status = value.status;
    if (Number.isInteger(value.elapsedMs) && value.elapsedMs >= 0 && value.elapsedMs <= 600_000) entry.elapsedMs = value.elapsedMs;
    if (typeof value.brokerReady === "boolean") entry.brokerReady = value.brokerReady;
    return entry;
  }

  function read() {
    try {
      const value = JSON.parse(sessionStorage.getItem(key) || "[]");
      return Array.isArray(value) ? value.map(safeEntry).filter(Boolean).slice(-limit) : [];
    } catch {
      return [];
    }
  }

  function record(trace, phase, details = {}) {
    const entry = safeEntry({ at: new Date().toISOString(), trace, phase, ...details });
    if (!entry) return;
    try {
      sessionStorage.setItem(key, JSON.stringify([...read(), entry].slice(-limit)));
    } catch {
      // Storage is diagnostic only; denied or full storage must not block a click.
    }
    console.info("native_post_trace", entry);
  }

  const recovered = read();
  if (recovered.length) console.info("native_post_trace_recovered", { count: recovered.length, events: recovered });
  window.akuNativePostDiagnostics = Object.freeze({ record, read, errorKind });
})();

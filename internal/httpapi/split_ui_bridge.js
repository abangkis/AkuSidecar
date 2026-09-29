// Windows-only server-injected adapter. The UI profile has no AkuBridge or
// source permissions; ordinary browser actions remain typed requests to the
// capture process. The separate UI reader broker handles only trusted clicks.
(() => {
  const origin = location.origin;
  // Retain the launch URL only in this closure: the page watchdog strips its
  // acknowledgement fragment. Never persist or log that capability.
  const startupReloadURL = /^#aku-startup=[a-f0-9]{64}$/.test(location.hash) ? location.href : null;
  const startupReloadKey = "akuReaderBrokerStartupReload.v1";
  let startupEdited = false;
  window.addEventListener("input", () => { startupEdited = true; }, true);
  window.addEventListener("change", () => { startupEdited = true; }, true);
  const pendingNativeTraces = [];
  const flushNativeTraces = () => {
    if (!window.akuNativePostDiagnostics) return;
    for (const entry of pendingNativeTraces.splice(0)) window.akuNativePostDiagnostics.record(...entry);
  };
  window.addEventListener("aku-native-post-diagnostics-ready", flushNativeTraces);
  const nativePostTrace = (requestId, phase, details = {}) => {
    if (window.akuNativePostDiagnostics) {
      flushNativeTraces();
      window.akuNativePostDiagnostics.record(requestId, phase, details);
    } else {
      if (pendingNativeTraces.length < 16) pendingNativeTraces.push([requestId, phase, details]);
      console.info("native_post_trace", { trace: requestId, phase, ...details });
    }
  };
  let readerBrokerReady = false;
  window.akuReaderBrokerStatus = "pending";
  const brokerStatus = (status) => {
    if (window.akuReaderBrokerStatus === status) return;
    window.akuReaderBrokerStatus = status;
    nativePostTrace("invalid", status === "ready" ? "broker_ready" : "broker_unavailable");
    window.dispatchEvent(new CustomEvent("aku-reader-broker-status", { detail: status }));
  };
  const readerBrokerDeadline = Date.now() + 5000;
  const recoverReaderBroker = () => {
    brokerStatus("unavailable");
    if (!startupReloadURL || startupEdited || readerBrokerReady) return;
    try {
      if (sessionStorage.getItem(startupReloadKey) === "attempted") return;
      sessionStorage.setItem(startupReloadKey, "attempted");
      // Verify storage before navigating: failed persistence must not loop.
      if (sessionStorage.getItem(startupReloadKey) !== "attempted") return;
    } catch { return; }
    nativePostTrace("invalid", "broker_reload_attempt");
    // Same tab/window/profile; no native action or click is replayed.
    try {
      // Restore the acknowledgement fragment without navigating, then explicitly
      // reload. location.replace with only a fragment change keeps this document
      // alive and cannot retry extension content-script injection.
      history.replaceState(history.state, "", startupReloadURL);
      location.reload();
    }
    catch { nativePostTrace("invalid", "broker_reload_failed"); }
  };
  // Bounded availability handshake; this grants no native activation authority.
  const probeReaderBroker = () => {
    if (readerBrokerReady) return;
    window.postMessage({ type: "AKU_BROWSER_READER_BROKER_PROBE" }, origin);
    if (Date.now() < readerBrokerDeadline) setTimeout(probeReaderBroker, 250);
    else recoverReaderBroker();
  };
  let bootstrapPromise;
  const bootstrap = () => bootstrapPromise ??= fetch("/api/bootstrap", { cache: "no-store" })
    .then(async (r) => { if (!r.ok) throw new Error("Capture transport bootstrap failed."); return r.json(); });
  const operations = {
    AKU_BROWSER_BRIDGE_PING: ["ping", "AKU_BROWSER_BRIDGE_READY", "AKU_BROWSER_BRIDGE_ERROR"],
    AKU_BROWSER_PROBE_SOURCE_SESSIONS: ["probe_source_sessions", "AKU_BROWSER_SOURCE_SESSIONS_RESULT", "AKU_BROWSER_SOURCE_SESSIONS_FAILED"],
    AKU_BROWSER_OPEN_SOURCE: ["open_source", "AKU_BROWSER_SOURCE_OPENED", "AKU_BROWSER_SOURCE_OPEN_FAILED"],
    AKU_BROWSER_OPEN_NATIVE_POST: ["open_native_post", "AKU_BROWSER_NATIVE_POST_OPENED", "AKU_BROWSER_NATIVE_POST_OPEN_FAILED"],
    AKU_BROWSER_BRIDGE_RELOAD_SELF: ["reload_self", null, "AKU_BROWSER_BRIDGE_ERROR"],
    AKU_BROWSER_RELEASE_CAPTURE_SURFACE: ["release", "AKU_BROWSER_CAPTURE_SURFACE_RELEASED", "AKU_BROWSER_CAPTURE_SURFACE_RELEASE_FAILED"],
    AKU_BROWSER_MEDIA_RECAPTURE: ["media_recapture", "AKU_BROWSER_MEDIA_RECAPTURE_COMPLETED", "AKU_BROWSER_MEDIA_RECAPTURE_FAILED"],
    AKU_BROWSER_X_MEDIA_EVIDENCE_LOOKUP: ["media_evidence", "AKU_BROWSER_X_MEDIA_EVIDENCE_RESULT", "AKU_BROWSER_X_MEDIA_EVIDENCE_FAILED"],
    AKU_BROWSER_CONFIGURE_BACKGROUND_DISPATCH: ["configure_background", null, "AKU_BROWSER_BRIDGE_ERROR"],
    AKU_BROWSER_REVOKE_SOURCE_ACCESS: ["revoke_source_access", "AKU_BROWSER_REVOKE_SOURCE_ACCESS_RESULT", "AKU_BROWSER_REVOKE_SOURCE_ACCESS_FAILED"],
    AKU_BROWSER_DISPATCH: ["dispatch", null, "AKU_BROWSER_DISPATCH_FAILED"],
  };
  window.addEventListener("message", async (event) => {
    if (event.source !== window || event.origin !== origin || !event.data) return;
    const message = event.data;
    if (message.type === "AKU_BROWSER_READER_BROKER_DIAGNOSTIC") {
      if (["broker_listener_ready", "broker_startup_error"].includes(message.phase) &&
          message.brokerRevision === "listener-first-v1") {
        nativePostTrace("invalid", message.phase, { brokerRevision: message.brokerRevision });
      }
      return;
    }
    if (message.type === "AKU_BROWSER_READER_BROKER_READY") {
      readerBrokerReady = true;
      brokerStatus("ready");
      return;
    }
    const operation = Object.hasOwn(operations, message.type) ? operations[message.type] : null;
    if (!operation) return;
    const nativeStarted = operation[0] === "open_native_post" ? performance.now() : null;
    const nativeTraceId = /^(?:broker_[0-9a-f]{32}|native_post_\d+_[0-9a-f]+)$/.test(message.requestId ?? "")
      ? message.requestId : "invalid";
    if (nativeStarted !== null) nativePostTrace(nativeTraceId, "relay_received", { brokerReady: readerBrokerReady });
    const correlation = {};
    for (const key of ["requestId", "source", "leaseId", "runId", "recaptureId"]) {
      if (typeof message[key] === "string") correlation[key] = message[key];
    }
    try {
      if (operation[0] === "open_native_post" && !readerBrokerReady) {
        throw new Error("Open native post is unavailable: the UI reader broker is not ready. Reload the interface to retry.");
      }
      if (operation[0] === "open_native_post" && !/^broker_[0-9a-f]{32}$/.test(message.requestId ?? "")) {
        throw new Error("Open native post is unavailable: the UI reader broker did not receive a trusted click. Reload the interface and click the post again.");
      }
      const config = await bootstrap();
      if (nativeStarted !== null) nativePostTrace(nativeTraceId, "relay_bootstrap_done", { elapsedMs: Math.round(performance.now() - nativeStarted) });
      const body = { type: operation[0] };
      for (const key of ["source", "url", "runId", "leaseId", "recaptureId", "actionId"]) {
        if (typeof message[key] === "string") body[key] = message[key];
      }
      if (operation[0] === "open_native_post" && typeof message.requestId === "string") body.requestId = message.requestId;
      if (Array.isArray(message.candidateIds)) body.candidateIds = message.candidateIds;
      if (nativeStarted !== null) nativePostTrace(nativeTraceId, "relay_request_start", { elapsedMs: Math.round(performance.now() - nativeStarted) });
      const response = await fetch("/api/split-capture/actions", {
        method: "POST", cache: "no-store", headers: {
          "Content-Type": "application/json",
          "X-Aku-Bridge-Token": config.bridgeToken,
          "X-Aku-Bridge-Contract": config.bridgeContractVersion,
          "X-Aku-Split-Epoch": config.instanceEpoch,
        }, body: JSON.stringify(body),
      });
      const reply = await response.json();
      if (nativeStarted !== null) nativePostTrace(nativeTraceId, "relay_request_end", { status: response.status, elapsedMs: Math.round(performance.now() - nativeStarted) });
      if (!response.ok || !reply.ok) throw new Error(reply.message || "Capture process action failed.");
      if (operation[1]) window.postMessage({
        ...correlation, ...(reply.result ?? {}),
        type: operation[0] === "open_source" && reply.result?.state === "permission_required"
          ? "AKU_BROWSER_SOURCE_PERMISSION_REQUIRED" : operation[1],
      }, origin);
    } catch (error) {
      if (nativeStarted !== null) nativePostTrace(nativeTraceId, "relay_error", {
        elapsedMs: Math.round(performance.now() - nativeStarted),
        errorKind: window.akuNativePostDiagnostics?.errorKind(error) ?? "other",
      });
      bootstrapPromise = undefined;
      window.postMessage({ ...correlation, type: operation[2], message: String(error?.message ?? error) }, origin);
    }
  });
  probeReaderBroker();
})();

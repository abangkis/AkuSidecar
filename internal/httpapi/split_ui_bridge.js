// Windows-only server-injected adapter. No extension is installed in the UI
// profile; all browser actions are typed requests to the capture process.
(() => {
  const origin = location.origin;
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
    const operation = Object.hasOwn(operations, message.type) ? operations[message.type] : null;
    if (!operation) return;
    const correlation = {};
    for (const key of ["requestId", "source", "leaseId", "runId", "recaptureId"]) {
      if (typeof message[key] === "string") correlation[key] = message[key];
    }
    try {
      const config = await bootstrap();
      const body = { type: operation[0] };
      for (const key of ["source", "url", "runId", "leaseId", "recaptureId", "actionId"]) {
        if (typeof message[key] === "string") body[key] = message[key];
      }
      if (Array.isArray(message.candidateIds)) body.candidateIds = message.candidateIds;
      const response = await fetch("/api/split-capture/actions", {
        method: "POST", cache: "no-store", headers: {
          "Content-Type": "application/json",
          "X-Aku-Bridge-Token": config.bridgeToken,
          "X-Aku-Bridge-Contract": config.bridgeContractVersion,
          "X-Aku-Split-Epoch": config.instanceEpoch,
        }, body: JSON.stringify(body),
      });
      const reply = await response.json();
      if (!response.ok || !reply.ok) throw new Error(reply.message || "Capture process action failed.");
      if (operation[1]) window.postMessage({
        ...correlation, ...(reply.result ?? {}),
        type: operation[0] === "open_source" && reply.result?.state === "permission_required"
          ? "AKU_BROWSER_SOURCE_PERMISSION_REQUIRED" : operation[1],
      }, origin);
    } catch (error) {
      bootstrapPromise = undefined;
      window.postMessage({ ...correlation, type: operation[2], message: String(error?.message ?? error) }, origin);
    }
  });
})();

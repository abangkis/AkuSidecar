// This isolated-world capture listener is the only native-host trigger.
// No window message or synthetic click can manufacture user activation.
if (window === window.top && location.pathname === "/") {
  // The native startup capability is acknowledgement-only. Capture the exact
  // launch URL before the page watchdog removes its fragment, so this same tab
  // can perform one quiet navigation retry without creating or foregrounding
  // another window. The service worker never exposes or logs this value.
  const startupURL = /^#aku-startup=[a-f0-9]{64}$/.test(location.hash) ? location.href : "";
  // Call immediately inside the trusted event; convert synchronous runtime
  // exceptions to rejections without deferring user activation to another task.
  const sendMessage = (message) => {
    try { return Promise.resolve(chrome.runtime.sendMessage(message)); }
    catch (error) { return Promise.reject(error); }
  };
  const reportStage = (phase) => window.postMessage({
    type: "AKU_BROWSER_READER_BROKER_DIAGNOSTIC", phase,
    brokerRevision: "listener-first-v1",
  }, location.origin);
  // Availability only, never authorization: no message can launch the host.
  const announceReady = () => {
    reportStage("broker_listener_ready");
    window.postMessage({ type: "AKU_BROWSER_READER_BROKER_READY" }, location.origin);
  };
  window.addEventListener("message", (event) => {
    if (event.source === window && event.origin === location.origin && event.data?.type === "AKU_BROWSER_READER_BROKER_PROBE") announceReady();
  });
  document.addEventListener("click", (event) => {
    if (!event.isTrusted || event.button !== 0 || document.visibilityState !== "visible") return;
    const link = event.target?.closest?.("a[data-aku-native-post]");
    if (!link) return;
    const source = link.dataset.akuNativePost;
    const requestId = "broker_" + crypto.randomUUID().replaceAll("-", "");
    // The existing bubble handler reads/clears this before enqueueing exactly
    // the same source/URL/request ID. Starting the helper now preserves the
    // direct-child-of-foreground-UI eligibility window.
    link.dataset.akuReaderRequest = requestId;
    const request = { requestId, source, url: link.href };
    void sendMessage(request).then((reply) => {
      if (!reply?.ok) throw new Error(reply?.message || "Native reader broker failed.");
    }).catch((error) => {
      window.postMessage({ type: "AKU_BROWSER_NATIVE_POST_OPEN_FAILED", requestId, source, message: String(error?.message ?? error) }, location.origin);
    });
  }, true);
  announceReady();
  if (startupURL) {
    let applicationReady = false;
    void sendMessage({ type: "AKU_BROWSER_UI_STARTUP_WATCH", startupUrl: startupURL }).catch(() => reportStage("broker_startup_error"));
    window.addEventListener("aku-startup-stage", (event) => {
      if (event.detail !== "ready" || applicationReady) return;
      applicationReady = true;
      void sendMessage({ type: "AKU_BROWSER_UI_STARTUP_READY" }).catch(() => reportStage("broker_startup_error"));
    });
    setTimeout(() => {
      if (applicationReady) return;
      void sendMessage({ type: "AKU_BROWSER_UI_STARTUP_RECOVER", startupUrl: startupURL }).catch(() => reportStage("broker_startup_error"));
    }, 8_000);
  }

}

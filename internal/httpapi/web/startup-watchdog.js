// Independent of the app's module graph: a failed import must leave recovery
// instructions even when none of app.js can execute. Diagnostics are fixed
// stage labels, never exception messages, URLs, tokens, or source content.
(() => {
  // An acknowledgement-only capability, never the privileged runtime control token.
  // Keep it in this closure and remove it from the URL before modules execute.
  const startupMatch = /^#aku-startup=([a-f0-9]{64})$/.exec(location.hash);
  let startupToken = startupMatch?.[1] || "";
  if (startupMatch) history.replaceState(history.state, "", location.pathname + location.search);
  function acknowledgeNativeStartup() {
    if (!startupToken) return;
    const token = startupToken;
    startupToken = "";
    // Rendering opportunities are evidence of initialization, not proof that
    // the OS compositor displayed pixels. Backend health never dismisses this.
    requestAnimationFrame(() => requestAnimationFrame(() => {
      void fetch("/api/app-shell/startup-ready", {
        method: "POST", headers: { "X-Aku-Startup-Token": token },
        credentials: "same-origin", cache: "no-store", redirect: "error",
      }).catch(() => {});
    }));
  }
  const panel = document.getElementById("startup-recovery");
  const heading = document.getElementById("startup-recovery-heading");
  const detail = document.getElementById("startup-recovery-detail");
  if (!panel || !heading || !detail) return;
  let stage = "interface assets";
  let ready = false;
  function report(reason) {
    if (ready) return;
    panel.hidden = false;
    heading.textContent = "AkuBrowser could not finish opening";
    detail.textContent = `Startup stage: ${stage}. ${reason} Reload the interface to retry. You do not need to download Chrome. If this repeats, report this startup stage to support.`;
  }
  const timer = setTimeout(() => report("No ready confirmation arrived within 60 seconds."), 60_000);
  function onStage(event) {
    if (event.detail === "ready") {
      acknowledgeNativeStartup();
      ready = true;
      clearTimeout(timer);
      panel.hidden = true;
      window.removeEventListener("aku-startup-stage", onStage);
      window.removeEventListener("error", onError, true);
      window.removeEventListener("unhandledrejection", onRejection);
    } else if (event.detail === "restoring") {
      stage = "restoring saved state";
    } else if (event.detail === "rendering") {
      stage = "rendering the interface";
    }
  }
  function onError(event) {
    if (event.target?.tagName === "SCRIPT") {
      report("An interface script could not load.");
    } else if (event.target === window) {
      report("An interface script stopped unexpectedly.");
    }
  }
  function onRejection() {
    report("An interface operation could not finish.");
  }
  window.addEventListener("aku-startup-stage", onStage);
  window.addEventListener("error", onError, true);
  window.addEventListener("unhandledrejection", onRejection);
})();

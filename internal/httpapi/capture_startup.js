// Independent of AkuBridge. A loaded host can report missing markers even
// when the extension content script never ran on this document.
(() => {
  if (!["http://127.0.0.1:11122", "http://localhost:11122"].includes(location.origin) || location.pathname !== "/split-capture-host") return;
  const status = document.getElementById("split-capture-status");
  const doc = Number(document.currentScript?.dataset.document);
  const key = location.hash.slice(1);
  if (!status || !Number.isInteger(doc) || doc < 1 || !/^[a-f0-9]{64}$/.test(key)) return;
  const report = (phase) => {
    const reload = performance.getEntriesByType("navigation")[0]?.type === "reload";
    void fetch("/api/bridge/split-capture/startup-sample", {
      method: "POST", headers: { "X-Aku-Capture-Instance": key, "Content-Type": "application/json" },
      body: JSON.stringify({ doc, phase, reload, scriptStarted: status.dataset.akuScriptStarted === "1", workerReceived: status.dataset.akuWorkerReceived === "1" }),
      redirect: "error", referrerPolicy: "no-referrer", cache: "no-store",
    }).catch(() => {});
  };
  window.addEventListener("load", () => {
    report("early");
    const timer = setTimeout(() => report("late"), 12_000);
    window.addEventListener("pagehide", () => clearTimeout(timer), { once: true });
  }, { once: true });
})();

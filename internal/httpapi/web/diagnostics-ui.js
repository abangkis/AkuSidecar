

export function createDiagnosticsUi({
  $, api, getScrollPerformanceTrace, setView
}) {
  let lastScrollPerformanceReport = null;

  function startScrollPerformanceTrace() {
    if (getScrollPerformanceTrace().active) return;
    setView("timeline");
    $("#start-scroll-trace").disabled = true;
    $("#scroll-trace-status").textContent = "Recording timing for 30 seconds. Scroll the Timeline now.";
    $("#scroll-trace-banner").classList.remove("hidden");
    getScrollPerformanceTrace().start(30000);
  }

  async function saveScrollPerformanceTrace(report) {
    lastScrollPerformanceReport = report;
    $("#scroll-trace-banner").classList.add("hidden");
    $("#download-scroll-trace").disabled = false;
    $("#scroll-trace-status").textContent = "Saving timing trace…";
    try {
      await api("/api/diagnostics/ui-performance", { method: "POST", body: report });
      $("#scroll-trace-status").textContent = "Trace saved in runtime memory. You can download it or ask Codex to inspect it.";
    } catch {
      $("#scroll-trace-status").textContent = "Trace could not be saved to the runtime. Download last trace to keep the timing report.";
    } finally {
      $("#start-scroll-trace").disabled = false;
    }
  }

  function downloadScrollPerformanceTrace() {
    if (!lastScrollPerformanceReport) return;
    const url = URL.createObjectURL(new Blob([JSON.stringify(lastScrollPerformanceReport, null, 2)], { type: "application/json" }));
    const link = document.createElement("a");
    link.href = url;
    link.download = "aku-scroll-trace.json";
    document.body.appendChild(link);
    link.click();
    link.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }

  function exportDiagnostics() {
    const since = $("#diagnostics-since").value ? new Date($("#diagnostics-since").value).toISOString() : "";
    const until = $("#diagnostics-until").value ? new Date($("#diagnostics-until").value).toISOString() : "";
    const button = $("#export-diagnostics");
    const originalText = button.textContent;
    button.disabled = true;
    button.textContent = "Exporting…";
    const params = new URLSearchParams();
    if (since) params.set("since", since);
    if (until) params.set("until", until);
    api("/api/diagnostics/export?" + params.toString(), { method: "GET", responseType: "blob" })
      .then((blob) => {
        const url = window.URL.createObjectURL(blob);
        const a = document.createElement("a");
        a.href = url;
        a.download = "aku-diagnostics-" + new Date().toISOString().slice(0, 19).replace(/[:-]/g, "") + ".json";
        document.body.appendChild(a);
        a.click();
        window.URL.revokeObjectURL(url);
        a.remove();
      })
      .catch((err) => {
        $("#runtime-settings-status").textContent = "Export failed: " + (err.message || err);
      })
      .finally(() => {
        button.disabled = false;
        button.textContent = "Export diagnostics…";
      });
  }

  return {
    startScrollPerformanceTrace, saveScrollPerformanceTrace, downloadScrollPerformanceTrace, exportDiagnostics
  };
}

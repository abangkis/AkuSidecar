import { browserCollectorProbeAllowed } from "./collection-mode.js";
import { renderOperationalBanner } from "./operational-banner.js";
import { setSettingsText } from "./settings-render.js";

export function requestCaptureJournal(window, origin, journalOperation, previewToken) {
  const requestId = globalThis.crypto.randomUUID();
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => finish(reject, new Error("The capture journal check timed out. Review again to refresh its status.")), 20_000);
    function finish(callback, value) {
      clearTimeout(timer);
      window.removeEventListener("message", receive);
      callback(value);
    }
    function receive(event) {
      const result = event.data;
      if (event.source !== window || event.origin !== origin || result?.requestId !== requestId) return;
      if (result.type === "AKU_BROWSER_CAPTURE_JOURNAL_FAILED") finish(reject, new Error(result.message));
      if (result.type === "AKU_BROWSER_CAPTURE_JOURNAL_RESULT") {
        const journal = result.journal;
        const valid = typeof journal?.capacityBlocked === "boolean" &&
          [journal.recordsUsed, journal.capacity, journal.protectedRecords].every(count => Number.isSafeInteger(count) && count >= 0) &&
          (journal.previewToken === undefined || (typeof journal.previewToken === "string" && /^[a-f0-9]{64}$/.test(journal.previewToken) &&
            Number.isSafeInteger(journal.eligibleRecords) && journal.eligibleRecords >= 0)) &&
          (journal.outcome !== "archived" || (Number.isSafeInteger(journal.archivedRecords) && journal.archivedRecords > 0));
        if (!valid) finish(reject, new Error("AkuBridge returned an unavailable journal status."));
        else finish(resolve, result.journal);
      }
    }
    window.addEventListener("message", receive);
    window.postMessage({ type: "AKU_BROWSER_CAPTURE_JOURNAL", requestId, journalOperation, previewToken }, origin);
  });
}

export function createCaptureJournalUI({ $, getRuntime, request }) {
  const modal = $("#capture-journal-dialog");
  const back = $("#capture-journal-cancel");
  const submit = $("#capture-journal-submit");
  const review = $("#capture-journal-review");
  const banner = {
    panel: $("#capture-journal-banner"), title: $("#capture-journal-banner-title"),
    detail: $("#capture-journal-banner-detail"), action: review,
  };
  const format = count => count.toLocaleString("en-US");
  let status = null;
  let preview = null;
  let busy = false;
  let poll = false;
  let revision = 0;
  const canReview = () => {
    const runtime = getRuntime();
    return !runtime?.pending && browserCollectorProbeAllowed(runtime);
  };

  function renderBanner() {
    renderOperationalBanner(banner, canReview() && status?.capacityBlocked ? {
      title: "Capture journal full", detail: "Browser capture is paused.",
      actionLabel: "Review capture journal", actionDisabled: busy,
    } : null);
  }

  async function refresh() {
    renderBanner();
    if (busy || poll || !canReview()) return;
    poll = true;
    const requestedRevision = revision;
    try {
      const current = await request("status");
      if (requestedRevision === revision) status = current;
    } catch { /* Preserve the last verified condition until the owner responds. */ }
    finally { poll = false; renderBanner(); }
  }

  function renderDialog({ result = null, note, failure = false } = {}) {
    const ready = result && !result.capacityBlocked;
    const archived = result?.outcome === "archived";
    const view = result ?? preview;
    setSettingsText($("#capture-journal-title"), ready ? "Capture journal ready" : "Review capture journal");
    setSettingsText($("#capture-journal-summary"), archived ? "Backup and archive verified" : "Read-only preview");
    $("#capture-journal-counts").hidden = !view;
    if (view) {
      setSettingsText($("#capture-journal-used"), `${format(view.recordsUsed)} / ${format(view.capacity)}`);
      setSettingsText($("#capture-journal-eligible-label"), archived ? "Archived records" : "Eligible closed records");
      setSettingsText($("#capture-journal-eligible"), format(archived ? view.archivedRecords : view.eligibleRecords ?? 0));
      $("#capture-journal-protected-row").classList.toggle("hidden", Boolean(ready));
      if (!ready) setSettingsText($("#capture-journal-protected"), format(view.protectedRecords));
    }
    const protectedSummary = result?.preserved;
    const completion = archived && protectedSummary
      ? `Latest backup: ${format(result.backupRecords)} records. Preserved: ${protectedSummary.active} active, ${protectedSummary.pending} pending, ${protectedSummary.retained} retained.`
      : "The journal has room for new capture records.";
    setSettingsText($("#capture-journal-feedback"), note ?? (ready
      ? completion
      : preview?.eligibleRecords ? "Only verified closed leases can be archived." : "No closed lease is currently safe to archive."));
    $("#capture-journal-feedback").setAttribute("role", failure ? "alert" : "status");
    back.disabled = busy;
    setSettingsText(back, ready ? "Back to Timeline" : "Cancel");
    submit.hidden = Boolean(ready);
    submit.disabled = busy || Boolean(preview && !preview.eligibleRecords);
    setSettingsText(submit, preview?.eligibleRecords ? `Archive ${format(preview.eligibleRecords)} closed records` : "Review again");
  }

  async function run(operation) {
    if (busy) return;
    const token = preview?.previewToken;
    if (!canReview() || (operation === "archive" && (!token || preview.backupPolicy !== "latest_verified"))) return;
    revision++;
    busy = true;
    renderBanner();
    renderDialog({ note: operation === "preview" ? "Checking capture journal…" : "Verifying the backup and archiving eligible records…" });
    let display = {};
    try {
      const result = await request(operation, operation === "archive" ? token : undefined);
      status = result;
      preview = result.previewToken ? result : null;
      if (preview && result.capacityBlocked && preview.backupPolicy !== "latest_verified") {
        preview = null;
        display = { note: "The active AkuBridge must support a verified journal backup. Reload it before archiving.", failure: true };
      } else if (result.outcome === "review_required" && result.capacityBlocked) {
        display = { note: "The preview changed. Review the current counts before confirming again." };
      } else if (result.outcome === "archived" && result.capacityBlocked) {
        display = { result, note: "The archive was verified. The journal still needs room for new capture records." };
      } else if (!result.capacityBlocked) {
        display = { result };
      }
    } catch (error) {
      preview = null;
      display = { note: error.message, failure: true };
    } finally {
      busy = false;
      renderBanner();
      renderDialog(display);
    }
  }

  review.addEventListener("click", () => {
    if (busy || modal.open || !status?.capacityBlocked || !canReview()) return;
    preview = null;
    modal.showModal();
    void run("preview");
  });
  back.addEventListener("click", () => modal.close());
  submit.addEventListener("click", () => void run(preview ? "archive" : "preview"));
  modal.addEventListener("cancel", event => { if (busy) event.preventDefault(); });
  return { refresh, renderBanner };
}

import { formatLibraryStorageBytes } from "./library-state.js";
import { settingsStorageView } from "./settings-storage-state.js";

export function createSettingsStorageUi({
  $, api, state
}) {
  function settingsStorageDetail(label, value) {
    const row = document.createElement("div");
    const term = document.createElement("dt");
    const definition = document.createElement("dd");
    term.textContent = label;
    definition.textContent = value;
    row.append(term, definition);
    return row;
  }

  function renderSettingsStorage() {
    const { usage, loading, error } = state.settingsStorage;
    const status = $("#settings-storage-status");
    const database = $("#settings-storage-database");
    const protection = $("#settings-storage-protection");
    const details = $("#settings-storage-detail-list");
    const setMeter = (name, percent, label) => {
      const meter = $(`#settings-storage-${name}-meter`);
      meter.querySelector("span").style.width = `${percent ?? 0}%`;
      if (percent === null) meter.removeAttribute("aria-valuenow");
      else meter.setAttribute("aria-valuenow", String(Math.round(percent)));
      meter.setAttribute("aria-valuetext", label);
    };
    if (!usage) {
      $("#settings-storage-mode").textContent = loading ? "Loading…" : "Unavailable";
      status.textContent = loading ? "Loading storage usage…" : "Storage usage unavailable";
      status.classList.remove("settings-storage-pressure");
      database.textContent = "Database size: unavailable";
      $("#settings-storage-items").textContent = "—";
      $("#settings-storage-bytes").textContent = "—";
      setMeter("items", null, "Unavailable");
      setMeter("bytes", null, "Unavailable");
      protection.textContent = error ? "Could not load storage usage." : "Storage usage is unavailable.";
      details.replaceChildren();
      return;
    }
    status.textContent = error ? `${usage.status} · last known; refresh failed` : loading ? `${usage.status} · refreshing…` : usage.status;
    status.classList.toggle("settings-storage-pressure", usage.pressure);
    database.textContent = `Database used: ${formatLibraryStorageBytes(usage.databaseEffectiveBytes)} (whole SQLite database)`;
    $("#settings-storage-items").textContent = `${usage.totalItems.toLocaleString()} / ${usage.maxItems.toLocaleString()}`;
    $("#settings-storage-bytes").textContent = `${formatLibraryStorageBytes(usage.logicalBytes)} / ${formatLibraryStorageBytes(usage.maxLogicalBytes)}`;
    setMeter("items", usage.itemPercent, `${usage.totalItems} of ${usage.maxItems} cards`);
    setMeter("bytes", usage.bytePercent, `${formatLibraryStorageBytes(usage.logicalBytes)} of ${formatLibraryStorageBytes(usage.maxLogicalBytes)}`);
    const eligible = usage.eligible === null ? "unknown" : usage.eligible.toLocaleString();
    const count = (value) => Number.isSafeInteger(value) && value >= 0 ? value.toLocaleString() : "Unknown";
    protection.textContent = usage.active
      ? `${count(usage.protectedItems)} protected · ${eligible} eligible. Eligible cards are trimmed during retention maintenance.`
      : `${count(usage.protectedItems)} protected · ${eligible} eligible. Preview only; no Timeline cards are removed.`;
    $("#settings-storage-mode").textContent = usage.active ? "Active retention" : "Preview only";
    const evaluated = usage.evaluatedAt ? new Date(usage.evaluatedAt) : null;
    details.replaceChildren(
      settingsStorageDetail("Policy", `${usage.policy?.protectionDays ?? "Unknown"} days protected · ${usage.policy?.routineExpiryDays ?? "Unknown"} days routine expiry`),
      settingsStorageDetail("Routine-expired", count(usage.routineExpiredItems)),
      settingsStorageDetail("Hidden", count(usage.hiddenItems)),
      settingsStorageDetail("Missing presentation time", count(usage.missingPresentationItems)),
      settingsStorageDetail("Processing cards protected", count(usage.processingProtectedItems)),
      settingsStorageDetail("Needs attention", usage.needsAttention === true ? "Yes" : usage.needsAttention === false ? "No" : "Unknown"),
      settingsStorageDetail("Potential reclaim", `${count(usage.reclaimableItems)} cards · ${formatLibraryStorageBytes(usage.reclaimableBytes)}`),
      settingsStorageDetail("Database allocated", formatLibraryStorageBytes(usage.databaseAllocatedBytes)),
      settingsStorageDetail("Evaluated", evaluated && !Number.isNaN(evaluated.getTime()) ? evaluated.toLocaleString() : "Unknown"),
    );
  }

  async function loadSettingsStorage() {
    if (state.settingsStorage.loading) return;
    state.settingsStorage.loading = true;
    state.settingsStorage.error = null;
    renderSettingsStorage();
    try {
      const usage = settingsStorageView(await api("/api/timeline/storage"));
      if (!usage) throw new Error("Incomplete storage usage response");
      state.settingsStorage.usage = usage;
    } catch (error) {
      state.settingsStorage.error = error;
    } finally {
      state.settingsStorage.loading = false;
      renderSettingsStorage();
    }
  }

  return {
    settingsStorageDetail, renderSettingsStorage, loadSettingsStorage
  };
}

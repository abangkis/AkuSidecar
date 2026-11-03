

export function createAppLifecycle({
  $, BOOTSTRAP_RETRY_MS, BOOTSTRAP_TIMEOUT_MS, api, bridgeActionLoop, clearNotice, getInstagramTrialControls, onboardingRequiresSetup, pingBridge, refreshLivingTopicNotificationProjection, renderActiveReasoningProvider, renderAutoUpdateStatus, renderBridge, renderCollectionRuntime, renderLibrary, renderLibrarySourceOptions, renderSession, renderSettings, renderSourceControls, renderSourceSessionReadiness, renderTimeline, setPill, setView, showCalibration, showError, showNotice, showOnboarding, startPolling, state, syncRunButtons, getXDailyControls
}) {
  const shownDatabaseIssues = new Set();
  let databaseCleanupBusy = false;
  let databaseHealthPoller = null;
  let databaseHealthPollInFlight = false;

  function installUIActivityTracking() {
    if (state.uiActivityTrackingInstalled) return;
    state.uiActivityTrackingInstalled = true;
    for (const eventName of ["pointerdown", "keydown", "touchstart", "wheel"]) {
      window.addEventListener(eventName, () => recordUIActivity(false), { passive: true });
    }
  }

  function recordUIActivity(force) {
    if (!state.bootstrap || document.visibilityState !== "visible") return;
    const now = Date.now();
    if (!force && now - state.lastUIActivitySentAt < 60_000) return;
    state.lastUIActivitySentAt = now;
    api("/api/ui/activity", { method: "POST" })
      .then(({ autoUpdate }) => {
        if (!autoUpdate || !state.bootstrap) return;
        state.bootstrap.autoUpdate = autoUpdate;
        renderAutoUpdateStatus(autoUpdate);
      })
      .catch((error) => {
        console.warn("Recent user activity could not be recorded.", error);
      });
  }

  async function bootstrap(options = {}) {
    window.dispatchEvent(new CustomEvent("aku-startup-stage", { detail: "restoring" }));
    clearTimeout(state.bootstrapRetryTimer);
    state.bootstrapRetryTimer = null;
    state.bootstrapController?.abort();
    const attempt = state.bootstrapAttempt + 1;
    const controller = new AbortController();
    state.bootstrapAttempt = attempt;
    state.bootstrapController = controller;
    state.bootstrapLoading = true;
    if (options.userInitiated) state.bootstrapError = null;
    syncRunButtons();
    const timeout = setTimeout(() => controller.abort(), BOOTSTRAP_TIMEOUT_MS);
    try {
      const restored = await api("/api/bootstrap", { signal: controller.signal });
      if (attempt !== state.bootstrapAttempt) return;
      if (!["not_started", "completed"].includes(restored?.onboarding?.status)) {
        throw new Error("AkuBrowser could not restore the authoritative onboarding state. Waiting for AkuSidecar to recover.");
      }
      clearNotice();
      window.dispatchEvent(new CustomEvent("aku-startup-stage", { detail: "rendering" }));
      state.bootstrap = restored;
      getXDailyControls().reset(restored.xCapture);
      getInstagramTrialControls().reset(restored.instagramCapture);
      state.bootstrapError = null;
      state.releasedCaptureSources.clear();
      state.lastUIActivitySentAt = 0;
      installUIActivityTracking();
      recordUIActivity(true);
      state.session = state.bootstrap.activeSession;
      state.bootstrapLoading = false;
      renderSourceControls();
      renderLibrarySourceOptions();
      renderLibrary();
      $("#runtime-version").textContent = `${state.bootstrap.version} · ${state.bootstrap.runtime}`;
      $("#bridge-contract").textContent = state.bootstrap.bridgeContractVersion;
      renderActiveReasoningProvider();
      $("#database-status").textContent = state.bootstrap.database?.status ?? "healthy";
      renderDatabaseHealth(state.bootstrap.databaseHealth);
      renderDeployment(state.bootstrap.deployment);
      setPill("#sidecar-status", "AkuSidecar ready", "ok");
      renderBridge(state.bootstrap.bridge);
      renderSettings(state.bootstrap.settings);
      state.timelineBatches = state.bootstrap.timelineBatches ?? [];
      renderTimeline(state.bootstrap.timeline ?? [], state.bootstrap.latestCheck ?? null, state.timelineBatches);
      renderSession();
      if (onboardingRequiresSetup()) {
        showOnboarding(false);
      } else if (state.bootstrap.calibration?.active) {
        showCalibration(state.bootstrap.calibration.active);
      } else {
        setView(state.currentView);
      }
      $("#startup-recovery").hidden = true;
      window.dispatchEvent(new CustomEvent("aku-startup-stage", { detail: "ready" }));
      pingBridge();
      bridgeActionLoop();
      setInterval(pingBridge, 30_000);
      state.collectionPoller ??= setInterval(pollCollectionRuntime, 2000);
      setInterval(pollAutoUpdate, 15_000);
      databaseHealthPoller ??= setInterval(pollDatabaseHealth, 60_000);
      void refreshLivingTopicNotificationProjection();
      if (state.session) startPolling();
    } catch (error) {
      if (attempt !== state.bootstrapAttempt) return;
      const failure = error?.name === "AbortError"
        ? new Error("AkuSidecar did not finish restoring within 45 seconds. Retry the connection while the active check continues in the background.")
        : error;
      state.bootstrapError = failure;
      $("#startup-recovery-heading").textContent = "AkuBrowser is still connecting";
      $("#startup-recovery-detail").textContent = "The interface is retrying automatically. You can reload this page if it remains here; you do not need to download Chrome.";
      showError(failure);
      syncRunButtons();
      state.bootstrapRetryTimer = setTimeout(() => bootstrap(), BOOTSTRAP_RETRY_MS);
    } finally {
      clearTimeout(timeout);
      if (attempt === state.bootstrapAttempt) state.bootstrapController = null;
    }
  }

  function renderDeployment(deployment) {
    const mode = deployment?.mode ?? "unknown";
    const installKind = deployment?.runtimeInstallKind;
    const labels = {
      development: "Development",
      acceptance: "Stable candidate · 3B",
      "production-store": "Production · Web Store",
      "production-offline": "Production · Offline bundle",
      "production-installed-app": "Production · Installed app",
      unknown: "Mode unknown",
    };
    const suffix = installKind && !["workspace", "installed"].includes(installKind)
      ? ` · ${installKind}`
      : "";
    setPill("#deployment-status", `${labels[mode] ?? "Mode invalid"}${suffix}`, mode === "unknown" ? "warning" : "neutral");
    $("#scroll-trace-controls").classList.toggle("hidden", mode !== "development");
  }

  async function pollAutoUpdate() {
    if (!state.bootstrap || state.pollInFlight) return;
    try {
      const [{ autoUpdate }, { session }] = await Promise.all([
        api("/api/auto-update/status"),
        api("/api/sessions/active"),
      ]);
      state.bootstrap.autoUpdate = autoUpdate;
      renderAutoUpdateStatus(autoUpdate);
      if (!state.session && session) {
        state.session = session;
        renderSession();
        startPolling();
      }
    } catch (error) {
      console.warn("Auto Update status refresh deferred.", error);
    }
    void refreshLivingTopicNotificationProjection();
  }

  async function pollDatabaseHealth() {
    if (!state.bootstrap || document.visibilityState !== "visible" || databaseCleanupBusy || databaseHealthPollInFlight) return;
    databaseHealthPollInFlight = true;
    try {
      const { databaseHealth } = await api("/api/database/health");
      renderDatabaseHealth(databaseHealth);
    } catch (error) {
      console.warn("Database health refresh deferred.", error);
    } finally {
      databaseHealthPollInFlight = false;
    }
  }

  function renderDatabaseHealth(health, force = false) {
    if (!health || !state.bootstrap) return;
    state.bootstrap.databaseHealth = health;
    const attention = health.status !== "healthy";
    const status = $("#database-status");
    status.textContent = attention ? "Cleanup required · review" : health.storagePressure ? "Storage pressure · young history retained" : "healthy";
    status.title = attention ? "Review local database cleanup" : health.storagePressure ? "Database exceeds its storage boundary. Only expired history may be trimmed." : "Local database is healthy";
    const dialog = $("#database-maintenance-dialog");
    if (!attention) { if (dialog.open) dialog.close(); return; }
    if (databaseCleanupBusy) return;
    $("#database-maintenance-summary").textContent = `${health.orphanRows} disconnected rows across ${health.affectedTables.length} affected relationships.`;
    $("#database-maintenance-status").textContent = health.repairable ? "Choose whether to keep a verified local backup before cleanup." : "This issue needs manual repair. Automatic cleanup is unavailable; your data has been left in place.";
    $("#database-maintenance-clean").disabled = !health.repairable;
    $("#database-maintenance-backup").disabled = !health.repairable;
    const key = `aku-database-maintenance:${health.fingerprint}`;
    let seen = shownDatabaseIssues.has(key);
    try { seen ||= localStorage.getItem(key) === "seen"; } catch { /* In-memory deduplication remains available. */ }
    if (force || !seen) {
      shownDatabaseIssues.add(key);
      try { localStorage.setItem(key, "seen"); } catch { /* Storage can be unavailable. */ }
      if (!dialog.open) dialog.showModal();
    }
  }

  async function cleanDatabase(backup) {
    const health = state.bootstrap?.databaseHealth;
    if (databaseCleanupBusy || !health?.repairable) return;
    databaseCleanupBusy = true;
    for (const id of ["clean", "backup", "later"]) $(`#database-maintenance-${id}`).disabled = true;
    $("#database-maintenance-status").textContent = backup ? "Verifying backup and cleaning…" : "Cleaning disconnected data…";
    try {
      const result = await api("/api/database/cleanup", { method: "POST", body: { confirmed: true, fingerprint: health.fingerprint, backup } });
      $("#database-maintenance-dialog").close();
      await bootstrap();
      if (result.backupPath) showNotice(`Cleanup complete. Backup: ${result.backupPath}`);
      else showNotice(`Cleanup complete. Removed ${result.removedRows} rows without a backup.`);
    } catch (error) {
      $("#database-maintenance-status").textContent = error.message;
    } finally {
      databaseCleanupBusy = false;
      for (const id of ["clean", "backup", "later"]) $(`#database-maintenance-${id}`).disabled = false;
    }
  }

  async function pollCollectionRuntime() {
    if (!state.bootstrap || state.collectionPollInFlight) return;
    state.collectionPollInFlight = true;
    const xDailyRead = getXDailyControls().beginRead();
    const instagramRead = getInstagramTrialControls().beginRead();
    try {
      const response = await api("/api/collection/runtime");
      state.bootstrap.collectionRuntime = response.collectionRuntime;
      getXDailyControls().applyRead(xDailyRead, response.xCapture);
      getInstagramTrialControls().applyRead(instagramRead, response.instagramCapture);
      renderCollectionRuntime();
      renderSourceSessionReadiness();
      syncRunButtons();
      if (response.collectionRuntime.effective === "headless") setPill("#bridge-status", "Headless collection ready", "ok");
    } catch {
      getXDailyControls().rejectRead(xDailyRead);
      getInstagramTrialControls().rejectRead(instagramRead);
      // Existing connection recovery owns transient failures.
    }
    finally { state.collectionPollInFlight = false; }
  }

  async function retryBootstrap() {
    await bootstrap({ userInitiated: true });
  }

  return {
    installUIActivityTracking, recordUIActivity, bootstrap, renderDeployment, pollAutoUpdate, pollDatabaseHealth, renderDatabaseHealth, cleanDatabase, pollCollectionRuntime, retryBootstrap
  };
}

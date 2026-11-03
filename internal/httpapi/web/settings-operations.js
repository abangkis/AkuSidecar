

export function createSettingsOperations({
  $, AI_HIDE_CONFIRMATION_PHRASE, getREVOKE_SOURCE_ACCESS_TIMEOUT_MS, api, persistSettings, refreshSettingsDirtyState, state, syncAIDetectionSettings
}) {
  async function revokeSourceAccessViaBridge() {
    if (state.bootstrap?.collectionRuntime?.effective === "headless") {
      await api("/api/collection/interactive", { method:"POST", body:{} });
    }
    return new Promise((resolve) => {
      const requestId = `revoke-${Date.now()}`;
      let settled = false;
      const finish = (value) => {
        if (settled) return;
        settled = true;
        window.removeEventListener("message", listener);
        window.clearTimeout(timer);
        resolve(value);
      };
      const listener = (event) => {
        if (event.source !== window) return;
        const data = event.data;
        if (data?.type === "AKU_BROWSER_REVOKE_SOURCE_ACCESS_RESULT" && data.requestId === requestId) return finish(true);
        if (data?.type === "AKU_BROWSER_REVOKE_SOURCE_ACCESS_FAILED" && data.requestId === requestId) return finish(false);
      };
      const timer = window.setTimeout(() => finish(false), getREVOKE_SOURCE_ACCESS_TIMEOUT_MS());
      window.addEventListener("message", listener);
      window.postMessage({ type: "AKU_BROWSER_REVOKE_SOURCE_ACCESS", requestId }, window.location.origin);
    });
  }

  function openResetDialog(operation) {
    if (state.session) {
      $("#runtime-settings-status").textContent = operation === "ai-hide"
        ? "Hide cannot be activated while an update is running."
        : "Reset is unavailable while an update is running.";
      if (operation === "ai-hide" && state.bootstrap?.settings) {
        state.pendingSettings = null;
        $("#ai-detection-presentation").value = state.bootstrap.settings.aiDetectionPresentation || "drawer";
        syncAIDetectionSettings();
        refreshSettingsDirtyState();
      }
      return;
    }
    state.resetOperation = operation;
    const full = operation === "full";
    const aiHide = operation === "ai-hide";
    $("#confirmation-operation-label").textContent = aiHide ? "HIGH-RISK PRESENTATION POLICY" : "DESTRUCTIVE OPERATION";
    $("#reset-confirmation-title").textContent = aiHide ? "Hide strong AI-origin signals" : full ? "Full reset and onboard again" : "Reset learning";
    $("#reset-confirmation-impact").textContent = aiHide
      ? "AI detection can be wrong. Only direct evidence and Deep-confirmed strong signals will be hidden. Posts remain stored locally and can be restored by disabling Hide."
      : full
        ? "AkuBrowser will first revoke all optional source permissions, then create and verify a local SQLite backup before erasing Timeline, runs, learning data, onboarding, and local settings. The browser profile, AkuBridge installation, and existing website sign-ins are preserved."
        : "AkuBrowser will erase calibration, More/Less feedback, and the fitted preference model. Timeline, source setup, and runtime settings remain.";
    $("#reset-confirmation-phrase").textContent = aiHide ? AI_HIDE_CONFIRMATION_PHRASE : full ? "RESET AKUBROWSER" : "RESET LEARNING";
    $("#reset-confirmation-submit").textContent = aiHide ? "Activate Hide" : full ? "Confirm full reset" : "Confirm reset";
    $("#reset-confirmation-input").value = "";
    $("#reset-confirmation-status").textContent = "";
    $("#reset-confirmation-submit").disabled = true;
    $("#reset-confirmation-dialog").showModal();
    $("#reset-confirmation-input").focus();
  }

  function closeResetDialog() {
    if (state.resetOperation === "ai-hide" && state.bootstrap?.settings) {
      $("#ai-detection-presentation").value = state.bootstrap.settings.aiDetectionPresentation || "drawer";
      syncAIDetectionSettings();
      refreshSettingsDirtyState();
    }
    state.resetOperation = null;
    state.pendingSettings = null;
    $("#reset-confirmation-dialog").close();
  }

  function syncResetConfirmation() {
    const phrase = $("#reset-confirmation-phrase").textContent;
    $("#reset-confirmation-submit").disabled = $("#reset-confirmation-input").value !== phrase;
  }

  async function submitReset() {
    const operation = state.resetOperation;
    if (!operation) return;
    const phrase = $("#reset-confirmation-phrase").textContent;
    const button = $("#reset-confirmation-submit");
    button.disabled = true;
    $("#reset-confirmation-status").textContent = operation === "full" ? "Creating and verifying backup…" : "Resetting learning…";
    try {
      if (operation === "ai-hide") {
        $("#reset-confirmation-status").textContent = "Activating Hide…";
        const pending = state.pendingSettings;
        if (!pending) throw new Error("Pending AI Detector settings are unavailable.");
        const saved = await persistSettings(pending, phrase);
        if (!saved) {
          syncResetConfirmation();
          return;
        }
        state.resetOperation = null;
        state.pendingSettings = null;
        $("#reset-confirmation-dialog").close();
        $("#runtime-settings-status").textContent = "Hide activated · direct and Deep-confirmed signals only.";
        return;
      }
      const path = operation === "full" ? "/api/operations/full-reset" : "/api/operations/reset-learning";
      let sourceAccessRevoked = false;
      if (operation === "full") {
        $("#reset-confirmation-status").textContent = "Revoking source access…";
        sourceAccessRevoked = await revokeSourceAccessViaBridge();
        if (!sourceAccessRevoked) {
          throw new Error("Full reset stopped because AkuBridge did not confirm source-access revocation. The browser profile and application data were left unchanged.");
        }
      }
      const response = await api(path, { method: "POST", body: { confirmation: phrase } });
      if (operation === "full") {
        $("#reset-confirmation-status").textContent = `Verified backup ${response.reset?.backupFile || "created"}. Source access revoked; browser profile preserved. Returning to onboarding…`;
        state.settingsUnloadBypass = true;
        window.setTimeout(() => window.location.reload(), 350);
        return;
      }
      state.bootstrap.calibration = response.calibration;
      $("#reset-confirmation-status").textContent = "Learning reset complete.";
      window.setTimeout(() => {
        closeResetDialog();
        $("#runtime-settings-status").textContent = "Learning reset complete.";
      }, 350);
    } catch (error) {
      $("#reset-confirmation-status").textContent = error.message;
      syncResetConfirmation();
    }
  }

  return {
    revokeSourceAccessViaBridge, openResetDialog, closeResetDialog, syncResetConfirmation, submitReset
  };
}

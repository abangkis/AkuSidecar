import { applyReasoningRuntimeResponse } from "./reasoning-runtime-state.js";
import { providerCanActivate, providerReadinessFeedback, providerRequiresSecureCredential } from "./onboarding-provider-credential.js";

export function createProviderSetupUi({
  $, getONBOARDING_PROVIDER_COPY, api, completeOnboarding, renderActiveReasoningProvider, renderReasoningProcesses, renderReasoningProviderControl, getSettingsDirty, state
}) {
  function providerCopy(entry) {
    const copy = getONBOARDING_PROVIDER_COPY();
    return copy[entry?.name] || (entry?.name?.startsWith("gemini-") ? copy["gemini-flash-lite"] : null);
  }

  function onboardingProviderEntries() {
    const order = { "codex-app-server": 0, "gemini-flash-lite": 1, "ollama-nemotron": 2, "ollama-qwen": 3 };
    const rank = (entry) => order[entry.name] ?? (entry.name.startsWith("gemini-") ? 1 : 9);
    return (state.bootstrap?.reasoningProviders || []).slice().sort((a, b) => rank(a) - rank(b));
  }

  function renderOnboardingProviderOptions() {
    const container = $("#onboarding-provider-options");
    const selected = state.onboardingProviderChoice;
    container.replaceChildren(...onboardingProviderEntries().map((entry) => {
      const copy = providerCopy(entry) || { tag: entry.runtimeKind, description: "" };
      const label = document.createElement("label");
      label.className = "onboarding-provider-option";
      const input = document.createElement("input");
      input.type = "radio";
      input.name = "onboarding-reasoning-provider";
      input.value = entry.name;
      input.checked = entry.name === selected;
      input.addEventListener("change", () => {
        $("#onboarding-provider-secret").value = "";
        state.onboardingProviderChoice = entry.name;
        renderOnboardingProviderOptions();
      });
      const card = document.createElement("span");
      card.className = "onboarding-provider-card";
      const heading = document.createElement("span");
      heading.className = "onboarding-provider-heading";
      const strong = document.createElement("strong");
      strong.textContent = entry.label;
      const tag = document.createElement("small");
      tag.textContent = copy.tag || "";
      heading.append(strong, tag);
      const description = document.createElement("small");
      description.className = "onboarding-provider-description";
      description.textContent = copy.description || "";
      const status = document.createElement("small");
      status.className = "onboarding-provider-status";
      if (entry.configurationStatus === "development_fallback") {
        status.textContent = "Secure setup required — development fallback detected";
      } else if (entry.configured === false) {
        status.textContent = "Setup required — " + (entry.credentialName || "credential") + " missing";
      } else if (entry.availabilityRequired && !entry.availabilityChecked) {
        status.textContent = "Availability check required";
      } else if (entry.availabilityRequired && !entry.available) {
        status.textContent = `Unavailable — ${entry.availabilityMessage || "local runtime not ready"}`;
      } else if (entry.availabilityRequired) {
        status.textContent = entry.availabilityMessage || "Available";
      } else if (entry.credentialName) {
        status.textContent = "Key saved securely";
      } else {
        status.textContent = "Ready";
      }
      card.append(heading, description, status);
      label.append(input, card);
      return label;
    }));
    const choice = onboardingProviderEntries().find((entry) => entry.name === selected);
    const setup = $("#onboarding-provider-setup");
    const credentialSetup = $("#onboarding-provider-credential");
    const requiresSecureCredential = providerRequiresSecureCredential(choice);
    const showCredentialSetup = Boolean(choice) && (choice.configured === false || requiresSecureCredential);
    const copy = providerCopy(choice) || {};
    const settingsMode = state.providerDialogMode === "settings";
    $("#onboarding-provider-dialog").classList.toggle("has-provider-context", Boolean(choice));
    $("#onboarding-provider-dialog").classList.toggle("provider-switch-mode", settingsMode);
    if (choice) {
      const panelLabel = showCredentialSetup
        ? "SECURE KEY SETUP"
        : choice.credentialName
          ? "SECURE KEY READY"
          : copy.panelLabel || "PROVIDER DETAILS";
      $("#onboarding-provider-panel-label").textContent = panelLabel;
      let panelText = choice.configurationStatus === "development_fallback"
        ? "A development-only key was detected, but it is not stored securely for AkuBrowser. Paste the key below to save it in this operating system's credential store."
        : showCredentialSetup
          ? copy.setup || "Configure the provider credential, then refresh its status."
          : copy.ready || copy.setup || "This provider is ready to use.";
      $("#onboarding-provider-setup-text").textContent = panelText;
      setup.classList.remove("hidden");
      credentialSetup.classList.toggle("hidden", !showCredentialSetup || !choice.credentialName);
      $("#onboarding-provider-credential-label").textContent = `${choice.label} API key`;
      const credentialLink = $("#onboarding-provider-credential-link");
      const credentialURL = copy.credentialURL;
      credentialLink.classList.toggle("hidden", !showCredentialSetup || !credentialURL);
      if (credentialURL) credentialLink.href = credentialURL;
      const quotaLink = $("#onboarding-provider-quota-link");
      const quotaURL = copy.quotaURL;
      quotaLink.classList.toggle("hidden", !choice || !quotaURL);
      if (quotaURL) quotaLink.href = quotaURL;
      const pricingLink = $("#onboarding-provider-pricing-link");
      const pricingURL = copy.pricingURL;
      pricingLink.classList.toggle("hidden", !choice || !pricingURL);
      if (pricingURL) pricingLink.href = pricingURL;
      const recheck = $("#onboarding-provider-recheck");
      recheck.classList.toggle("hidden", !showCredentialSetup && !choice.availabilityRequired);
      syncProviderReadinessFeedback(choice);
    } else {
      setup.classList.add("hidden");
      credentialSetup.classList.add("hidden");
      $("#onboarding-provider-quota-link").classList.add("hidden");
      $("#onboarding-provider-pricing-link").classList.add("hidden");
      $("#onboarding-provider-secret").value = "";
    }
    const confirm = $("#onboarding-provider-confirm");
    confirm.textContent = settingsMode && choice ? `Switch to ${choice.label}` : "Use selected provider";
    confirm.disabled = !providerCanActivate(choice);
    const codex = onboardingProviderEntries().find((entry) => entry.name === "codex-app-server");
    const skip = $("#onboarding-provider-skip");
    skip.textContent = settingsMode ? "Cancel" : "Keep Codex App Server";
    skip.disabled = settingsMode ? false : !providerCanActivate(codex);
  }

  async function refreshReasoningProviderReadiness() {
    if (!state.bootstrap) return false;
    if (state.providerReadinessInFlight) {
      syncProviderReadinessFeedback();
      return false;
    }
    state.providerReadinessInFlight = true;
    state.providerReadinessCheckError = "";
    syncProviderReadinessFeedback();
    try {
      const response = await api("/api/reasoning/providers/readiness");
      state.bootstrap.reasoningProviders = response.reasoningProviders ?? state.bootstrap.reasoningProviders;
      state.providerReadinessCheckedAt = new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
      renderReasoningProviderControl(state.bootstrap.settings);
      if ($("#onboarding-provider-dialog")?.open) renderOnboardingProviderOptions();
      return true;
    } catch (error) {
      state.providerReadinessCheckError = error.message;
      if ($("#onboarding-provider-dialog")?.open) {
        $("#onboarding-provider-error").textContent = error.message;
      } else {
        $("#runtime-settings-status").textContent = error.message;
      }
      return false;
    } finally {
      state.providerReadinessInFlight = false;
      syncProviderReadinessFeedback();
    }
  }

  function syncProviderReadinessFeedback(choice) {
    const selected = choice || onboardingProviderEntries().find((entry) => entry.name === state.onboardingProviderChoice);
    const onboardingButton = $("#onboarding-provider-recheck");
    if (onboardingButton) {
      onboardingButton.disabled = state.providerReadinessInFlight;
      onboardingButton.textContent = state.providerReadinessInFlight
        ? "Checking…"
        : selected?.availabilityRequired
          ? "Check availability"
          : "Refresh credential status";
    }
    const status = $("#onboarding-provider-check-status");
    if (!status) return;
    status.classList.remove("is-ready", "is-unavailable", "is-checking", "is-error");
    status.removeAttribute("title");
    if (state.providerReadinessInFlight) {
      status.textContent = "Checking";
      status.classList.add("is-checking");
    } else if (state.providerReadinessCheckError) {
      status.textContent = "Check failed";
      status.title = state.providerReadinessCheckError;
      status.classList.add("is-error");
    } else {
      const checkedSuffix = state.providerReadinessCheckedAt ? ` · ${state.providerReadinessCheckedAt}` : "";
      const feedback = providerReadinessFeedback(selected);
      status.textContent = `${feedback.label}${checkedSuffix}`;
      if (feedback.className) status.classList.add(feedback.className);
    }
  }

  function openOnboardingProviderDialog() {
    state.providerDialogMode = "onboarding";
    state.onboardingProviderChoice = state.bootstrap?.settings?.reasoningProvider || state.bootstrap?.provider || "codex-app-server";
    $("#onboarding-provider-title").textContent = "Choose how AkuBrowser reasons";
    $("#onboarding-provider-explanation").textContent = "Updates, semantic comparisons, and AI review all use this backend. You can change it later in Settings; the switch applies when no update is running.";
    renderOnboardingProviderOptions();
    $("#onboarding-provider-error").textContent = "";
    $("#onboarding-provider-dialog").showModal();
    $("#onboarding-provider-title").focus();
    void recheckOnboardingProviders();
  }

  function openSettingsProviderDialog(target) {
    const choice = onboardingProviderEntries().find((entry) => entry.name === target);
    if (!choice) return;
    state.providerDialogMode = "settings";
    state.onboardingProviderChoice = target;
    state.providerReadinessCheckError = "";
    $("#onboarding-provider-title").textContent = `Switch to ${choice.label}`;
    $("#onboarding-provider-explanation").textContent = "Your current provider remains active until this setup is complete. AkuBrowser checks any stored credential before asking for a new key.";
    $("#onboarding-provider-error").textContent = "";
    renderOnboardingProviderOptions();
    $("#onboarding-provider-dialog").showModal();
    $("#onboarding-provider-title").focus();
    void recheckOnboardingProviders();
  }

  async function recheckOnboardingProviders() {
    await refreshReasoningProviderReadiness();
    renderOnboardingProviderOptions();
  }

  async function saveOnboardingProviderCredential() {
    const choice = onboardingProviderEntries().find((entry) => entry.name === state.onboardingProviderChoice);
    const input = $("#onboarding-provider-secret");
    const button = $("#onboarding-provider-save-credential");
    const secret = input.value.trim();
    if (!choice?.credentialName || !secret) {
      $("#onboarding-provider-error").textContent = "Paste the API key before saving.";
      return;
    }
    button.disabled = true;
    button.textContent = "Saving…";
    $("#onboarding-provider-error").textContent = "Saving the key securely on this device…";
    try {
      const response = await api("/api/reasoning/credentials", {
        method: "PUT",
        body: { provider: choice.name, secret },
      });
      state.bootstrap.reasoningProviders = response.reasoningProviders ?? state.bootstrap.reasoningProviders;
      const validation = response.validation;
      $("#onboarding-provider-error").textContent = validation?.status === "valid"
        ? `Gemini accepted the key for ${validation.model || choice.label} metadata; it is saved securely. Actual generation and quota are checked when used.`
        : "API key saved securely. Check provider availability before continuing.";
      state.providerReadinessCheckedAt = new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
      renderOnboardingProviderOptions();
    } catch (error) {
      $("#onboarding-provider-error").textContent = error.message;
    } finally {
      input.value = "";
      button.disabled = false;
      button.textContent = "Save key";
    }
  }

  async function confirmOnboardingProvider() {
    const choice = state.onboardingProviderChoice;
    if (!choice) return;
    const sources = state.pendingOnboardingSources;
    if (state.providerDialogMode === "settings") {
      await activateSettingsReasoningProvider(choice);
      return;
    }
    if (!sources) {
      $("#onboarding-provider-dialog").close();
      return;
    }
    const button = $("#onboarding-provider-confirm");
    const activeProvider = state.bootstrap?.settings?.reasoningProvider || state.bootstrap?.provider;
    if (choice !== activeProvider) {
      button.disabled = true;
      $("#onboarding-provider-error").textContent = "Activating the selected provider…";
      try {
        const response = await api("/api/settings", { method: "PUT", body: { settings: { ...state.bootstrap.settings, reasoningProvider: choice } } });
        applyReasoningRuntimeResponse(state.bootstrap, response);
        renderActiveReasoningProvider();
      } catch (error) {
        $("#onboarding-provider-error").textContent = error.message;
        button.disabled = false;
        return;
      }
    }
    state.pendingOnboardingSources = null;
    $("#onboarding-provider-dialog").close();
    await completeOnboarding(sources, true);
  }

  async function activateSettingsReasoningProvider(choice) {
    const button = $("#onboarding-provider-confirm");
    const provider = onboardingProviderEntries().find((entry) => entry.name === choice);
    if (!providerCanActivate(provider)) return;
    button.disabled = true;
    $("#onboarding-provider-error").textContent = "Activating the selected provider…";
    try {
      const response = await api("/api/settings", {
        method: "PUT",
        body: { settings: { ...state.bootstrap.settings, reasoningProvider: choice } },
      });
      applyReasoningRuntimeResponse(state.bootstrap, response);
      renderActiveReasoningProvider();
      renderReasoningProviderControl(response.settings);
      renderReasoningProcesses(response.reasoningProcesses ?? []);
      const baseline = JSON.parse(getSettingsDirty().getBaselineKey() || "{}");
      getSettingsDirty().setBaseline({ ...baseline, reasoningProvider: choice });
      getSettingsDirty().refresh();
      $("#runtime-settings-status").textContent = `Provider changed · ${provider.label} is now active.`;
      $("#onboarding-provider-dialog").close();
    } catch (error) {
      $("#onboarding-provider-error").textContent = error.message;
      button.disabled = false;
    }
  }

  function skipOnboardingProvider() {
    if (state.providerDialogMode === "settings") {
      $("#onboarding-provider-dialog").close();
      return;
    }
    const sources = state.pendingOnboardingSources;
    const codex = onboardingProviderEntries().find((entry) => entry.name === "codex-app-server");
    if (!providerCanActivate(codex)) {
      $("#onboarding-provider-error").textContent = codex?.availabilityMessage || "Codex App Server is not ready. Check its availability or choose another provider.";
      return;
    }
    state.pendingOnboardingSources = null;
    $("#onboarding-provider-secret").value = "";
    $("#onboarding-provider-dialog").close();
    void completeOnboarding(sources || [], true);
  }

  return {
    onboardingProviderEntries, renderOnboardingProviderOptions, refreshReasoningProviderReadiness, syncProviderReadinessFeedback, openOnboardingProviderDialog, openSettingsProviderDialog, recheckOnboardingProviders, saveOnboardingProviderCredential, confirmOnboardingProvider, activateSettingsReasoningProvider, skipOnboardingProvider
  };
}

import { applyReasoningRuntimeResponse } from "./reasoning-runtime-state.js";
import { sourceAccessReadinessState } from "./onboarding-source-readiness.js";

export function createSettingsFeature({
  $, DEFAULT_SEMANTIC_EVENT_MERGE_THRESHOLD, DEFAULT_TIMELINE_BATCH_GAP_PX, DEFAULT_TIMELINE_BOUNDARY_RETURN_MS, LOAD_PROFILE_PRESETS, RELEASE_REASONING_DEFAULTS, api, applyPostFreshnessStyle, applySingleImageFit, applyStreamWidth, applyTimelineBatchGap, applyTimelineBoundaryReturnDuration, bridgeSourceReadiness, openResetDialog, openSettingsProviderDialog, refreshTimeline, releaseBackToTopBoundary, renderActiveReasoningProvider, renderAutoUpdateStatus, renderCollectionRuntime, getSettingsDirty, shouldShowOnboardingLearning, showError, sourceDescriptors, state, syncAutoUpdateModeSettings, syncOnboardingLearning
}) {
  function renderSettings(settings) {
    if (!settings) return;
    renderReasoningProcesses(state.bootstrap?.reasoningProcesses ?? []);
    const reasoningRuntime = state.bootstrap?.reasoningRuntime;
    renderReasoningProviderControl(settings);
    $("#reasoning-executable-label").textContent = reasoningRuntime?.label || "Inference executable";
    $("#reasoning-executable-path").value = reasoningRuntime?.executablePath || settings.reasoningExecutablePath || "";
    $("#reasoning-executable-path").disabled = reasoningRuntime?.editable === false;
    $("#detect-reasoning-executable").disabled = reasoningRuntime?.editable === false;
    $("#bounded-load-profile").value = settings.loadProfile;
    $("#capture-visibility-policy").value = settings.captureVisibility;
    $("#collection-mode").value = settings.collectionMode || "browser";
    renderCollectionRuntime();
    $("#source-wait-mode").value = settings.sourceWaitMode || "progressive_wait";
    $("#preference-eligibility-mode").value = settings.preferenceEligibilityMode;
    $("#calibration-enabled").checked = settings.calibrationEnabled;
    $("#calibration-batch-size").value = settings.calibrationBatchSize;
    $("#show-learning-panel").checked = settings.showLearningPanel === true;
    $("#open-missing-source").checked = settings.openMissingSource;
    $("#timeline-capacity").value = settings.timelineCapacity;
    $("#max-items-per-source").value = settings.maxItemsPerSource;
    $("#max-scrolls").value = settings.maxScrolls;
    $("#default-presentation").value = settings.defaultPresentation || "source";
    $("#stream-width").value = settings.streamWidth || "social";
    $("#single-image-fit").value = settings.singleImageFit || "cover";
    $("#post-freshness-style").value = settings.postFreshnessStyle || "header_shade";
    $("#content-context-up-scroll-mode").value = settings.contentContextUpScrollMode || "close_offscreen";
    $("#timeline-batch-gap").value = settings.timelineBatchGapPx || DEFAULT_TIMELINE_BATCH_GAP_PX;
    $("#timeline-boundary-follow").checked = settings.timelineBoundaryCueMode !== "static";
    $("#timeline-boundary-return-ms").value = settings.timelineBoundaryReturnMs || DEFAULT_TIMELINE_BOUNDARY_RETURN_MS;
    $("#semantic-event-mode").value = settings.semanticEventMode || "collapse";
    $("#semantic-event-shortlist").value = String(settings.semanticEventShortlist || 10);
    $("#semantic-event-merge-threshold").value = Number(settings.semanticEventMergeThreshold || DEFAULT_SEMANTIC_EVENT_MERGE_THRESHOLD).toFixed(2);
    $("#knowledge-retention-days").value = String(settings.knowledgeRetentionDays || 30);
    $("#knowledge-storage-limit").value = String(settings.knowledgeStorageLimitMb || 100);
    $("#ai-detection-enabled").checked = settings.aiDetectionEnabled !== false;
    $("#ai-detection-presentation").value = settings.aiDetectionPresentation || "drawer";
    const mediaRuntime = state.bootstrap?.mediaProvenanceRuntime;
    $("#c2pa-runtime-status").textContent = mediaRuntime?.available
      ? `${mediaRuntime.provider} ready · ${mediaRuntime.version}`
      : `${mediaRuntime?.provider || "c2patool"} unavailable`;
    $("#resurface-mode").value = settings.resurfaceMode || "smart";
    $("#resurface-cooldown-days").value = String(settings.resurfaceCooldownDays || 7);
    $("#auto-update-enabled").checked = settings.autoUpdateEnabled !== false;
    $("#auto-update-mode").value = settings.autoUpdateMode || "adaptive";
    $("#auto-update-refill").value = String(settings.autoUpdateRefillMinutes || 15);
    syncAutoUpdateModeSettings();
    $("#prepared-batch-limit").value = String(settings.preparedBatchLimit || 2);
    $("#auto-update-token-budget").value = String(settings.autoUpdateDailyTokenBudget || 2000000);
    $("#auto-update-manual-reserve").value = String(settings.autoUpdateManualReservePct || 25);
    $("#prepared-batch-max-age").value = String(settings.preparedBatchMaxAgeHours || 24);
    $("#next-batch-behavior").value = settings.nextBatchBehavior || "require_action";
    renderSourceSettingsValues(settings);
    for (const input of document.querySelectorAll("#settings-source-options input[type='checkbox']")) {
      input.checked = settings.activeSources?.includes(input.value) ?? false;
    }
    applyStreamWidth(settings.streamWidth || "social");
    applySingleImageFit(settings.singleImageFit || "cover");
    applyPostFreshnessStyle(settings.postFreshnessStyle || "header_shade");
    applyTimelineBatchGap(settings.timelineBatchGapPx || DEFAULT_TIMELINE_BATCH_GAP_PX);
    applyTimelineBoundaryReturnDuration(settings.timelineBoundaryReturnMs || DEFAULT_TIMELINE_BOUNDARY_RETURN_MS);
    if (settings.timelineBoundaryCueMode === "static") releaseBackToTopBoundary();
    syncLoadProfileSettings(false);
    syncSemanticEventSettings();
    syncAIDetectionSettings();
    renderAutoUpdateStatus(state.bootstrap?.autoUpdate);
    const now = new Date();
    const since = new Date(now.getTime() - 24 * 60 * 60 * 1000);
    const until = new Date();
    $("#diagnostics-since").value = since.toISOString().slice(0, 16);
    $("#diagnostics-until").value = until.toISOString().slice(0, 16);
    getSettingsDirty().setBaseline(readSettingsDraft(settings));
  }

  function syncReasoningProviderSelection() {
    const active = state.bootstrap?.settings?.reasoningProvider || state.bootstrap?.provider;
    const provider = (state.bootstrap?.reasoningProviders || []).find((entry) => entry.name === active);
    const status = $("#reasoning-provider-status");
    const executableRow = document.querySelector(".reasoning-executable-row");
    if (executableRow) executableRow.hidden = provider?.runtimeKind !== "executable";
    if (!status) return;
    status.classList.remove("is-warning");
    if (!provider) {
      status.textContent = "No active reasoning provider is available.";
    } else {
      status.textContent = `Active · ${provider.label}`;
    }
  }

  function beginSettingsProviderSelection() {
    const select = $("#reasoning-provider");
    const target = select?.value;
    const active = state.bootstrap?.settings?.reasoningProvider || state.bootstrap?.provider;
    if (select && active) select.value = active;
    syncReasoningProviderSelection();
    if (!target || target === active) return;
    if (state.session) {
      $("#runtime-settings-status").textContent = "Reasoning provider cannot be changed while an update is running.";
      return;
    }
    openSettingsProviderDialog(target);
  }

  function renderSourceSettingsValues(settings) {
    if (!settings) return;
    const readiness = new Map(bridgeSourceReadiness().map((source) => [source.source, source]));
    for (const source of sourceDescriptors()) {
      const defaultMS = source.hydrationTimeoutDefaultMs;
      const input = document.querySelector(`[data-source-hydration="${source.id}"]`);
      if (input && defaultMS) {
        input.value = String((settings.sourceHydrationTimeoutMs?.[source.id] || defaultMS) / 1000);
      }
      const status = document.querySelector(`[data-source-readiness="${source.id}"]`);
      if (status) {
        const current = readiness.get(source.id);
        status.className = "source-readiness-status";
        const accessState = sourceAccessReadinessState(current);
        if (accessState === "checking") {
          status.textContent = "Checking access";
        } else if (accessState === "ready") {
          status.textContent = "Ready";
          status.classList.add("source-readiness-ready");
        } else if (accessState === "permission_not_granted") {
          status.textContent = "Chrome permission needed";
          status.classList.add("source-readiness-warning");
        } else if (accessState === "registration_missing") {
          status.textContent = "Capture registration missing";
          status.classList.add("source-readiness-warning");
        } else {
          status.textContent = "Capture script not ready";
          status.classList.add("source-readiness-warning");
        }
      }
    }
  }

  function renderReasoningProcesses(processes) {
    const host = $("#reasoning-processes");
    if (!host) return;
    host.replaceChildren();
    for (const process of processes) {
      const row = document.createElement("div");
      row.className = "settings-row reasoning-process-profile";
      const copy = document.createElement("span");
      copy.className = "settings-copy";
      const label = document.createElement("strong");
      label.textContent = process.label;
      const description = document.createElement("small");
      description.textContent = process.description;
      copy.append(label, description);

      const route = document.createElement("span");
      route.className = "reasoning-process-route";
      const provider = document.createElement("small");
      provider.className = "reasoning-process-provider";
      provider.textContent = formatReasoningProvider(process.provider);
      route.append(provider);
      if (process.options?.length) {
        const select = document.createElement("select");
        select.id = `reasoning-profile-${process.id.replaceAll("_", "-")}`;
        select.className = "reasoning-profile-select";
        select.dataset.processId = process.id;
        select.setAttribute("aria-label", `${process.label} model profile`);
        for (const available of process.options) {
          const option = document.createElement("option");
          option.value = available.id;
          option.textContent = available.label;
          select.append(option);
        }
        select.value = process.profileId;
        route.append(select);
      } else {
        const model = document.createElement("strong");
        model.textContent = formatReasoningModel(process.model);
        route.append(model);
      }
      const detail = document.createElement("small");
      detail.textContent = `${process.execution === "async" ? "Async" : "In run"} · ${formatReasoningModel(process.model)} · ${formatReasoningEffort(process.effort)} thinking`;
      route.append(detail);
      row.append(copy, route);
      host.append(row);
    }
  }

  function formatReasoningProvider(value) {
    const label = state.bootstrap?.reasoningProviders?.find((entry) => entry.name === value)?.label;
    if (label) return label;
    if (value === "codex-app-server") return "Codex App Server";
    if (value === "deterministic") return "Local deterministic";
    if (value === "ollama-nemotron") return "Ollama · Nemotron 3.5 Lightning";
    if (value === "ollama-qwen") return "Ollama · Qwen 3.8 27B";
    if (String(value).startsWith("ollama-")) return `Ollama · ${String(value).replace("ollama-", "")}`;
    return String(value || "Custom backend").replaceAll("-", " ");
  }

  function formatReasoningModel(value) {
    if (value === "gpt-5.6-luna") return "GPT-5.6 Luna";
    if (value === "local-deterministic") return "No model";
    return value || "Backend default";
  }

  function formatReasoningEffort(value) {
    if (value === "xhigh") return "XHigh";
    if (value === "none") return "No";
    const text = String(value || "default");
    return text.charAt(0).toUpperCase() + text.slice(1);
  }

  function readSettingsDraft(current = state.bootstrap?.settings ?? {}) {
    const activeSources = [...document.querySelectorAll("#settings-source-options input[type='checkbox']:checked")]
      .map((input) => input.value)
      .sort();
    const loadProfile = $("#bounded-load-profile").value;
    const preset = LOAD_PROFILE_PRESETS[loadProfile];
    const perSource = Number.parseInt($("#max-items-per-source").value, 10);
    const sourceHydrationTimeoutMs = Object.fromEntries(
      [...document.querySelectorAll("[data-source-hydration]")]
        .map((input) => [input.dataset.sourceHydration, Number.parseInt(input.value, 10) * 1000])
        .sort(([left], [right]) => left.localeCompare(right)),
    );
    return {
      loadProfile,
      captureVisibility: $("#capture-visibility-policy").value,
      collectionMode: $("#collection-mode").value,
      sourceWaitMode: $("#source-wait-mode").value,
      preferenceEligibilityMode: $("#preference-eligibility-mode").value,
      calibrationEnabled: $("#calibration-enabled").checked,
      calibrationBatchSize: Number.parseInt($("#calibration-batch-size").value, 10),
      showLearningPanel: $("#show-learning-panel").checked,
      openMissingSource: $("#open-missing-source").checked,
      activeSources,
      sourceHydrationTimeoutMs,
      timelineCapacity: Number.parseInt($("#timeline-capacity").value, 10),
      maxItemsPerSource: perSource,
      maxItemsTotal: preset?.maxItemsTotal ?? Math.min(30, Math.max(1, perSource * activeSources.length)),
      maxScrolls: Number.parseInt($("#max-scrolls").value, 10),
      defaultPresentation: $("#default-presentation").value,
      streamWidth: $("#stream-width").value,
      singleImageFit: $("#single-image-fit").value,
      postFreshnessStyle: $("#post-freshness-style").value,
      contentContextUpScrollMode: $("#content-context-up-scroll-mode").value || "close_offscreen",
      timelineBatchGapPx: Number.parseInt($("#timeline-batch-gap").value, 10),
      timelineBoundaryCueMode: $("#timeline-boundary-follow").checked ? "follow" : "static",
      timelineBoundaryReturnMs: Number.parseInt($("#timeline-boundary-return-ms").value, 10),
      semanticEventMode: $("#semantic-event-mode").value,
      semanticEventShortlist: Number.parseInt($("#semantic-event-shortlist").value, 10),
      semanticEventMergeThreshold: Number.parseFloat($("#semantic-event-merge-threshold").value),
      knowledgeRetentionDays: Number.parseInt($("#knowledge-retention-days").value, 10),
      knowledgeStorageLimitMb: Number.parseInt($("#knowledge-storage-limit").value, 10),
      aiDetectionEnabled: $("#ai-detection-enabled").checked,
      aiDetectionPresentation: $("#ai-detection-presentation").value,
      resurfaceMode: $("#resurface-mode").value,
      resurfaceCooldownDays: Number.parseInt($("#resurface-cooldown-days").value, 10),
      autoUpdateEnabled: $("#auto-update-enabled").checked,
      autoUpdateMode: $("#auto-update-mode").value,
      autoUpdateRefillMinutes: Number.parseInt($("#auto-update-refill").value, 10),
      preparedBatchLimit: Number.parseInt($("#prepared-batch-limit").value, 10),
      autoUpdateDailyTokenBudget: Number.parseInt($("#auto-update-token-budget").value, 10),
      autoUpdateManualReservePct: Number.parseInt($("#auto-update-manual-reserve").value, 10),
      preparedBatchMaxAgeHours: Number.parseInt($("#prepared-batch-max-age").value, 10),
      nextBatchBehavior: $("#next-batch-behavior").value,
      reasoningExecutablePath: $("#reasoning-executable-path").value.trim(),
      reasoningProvider: $("#reasoning-provider")?.value || current.reasoningProvider,
      reasoningAcquisitionProfile: reasoningProfileValue("acquisition_planning", current.reasoningAcquisitionProfile),
      reasoningEvaluationProfile: reasoningProfileValue("candidate_evaluation", current.reasoningEvaluationProfile),
      reasoningSemanticProfile: reasoningProfileValue("semantic_event_resolution", current.reasoningSemanticProfile),
      reasoningAiDeepProfile: reasoningProfileValue("ai_deep_detection", current.reasoningAiDeepProfile),
    };
  }

  function refreshSettingsDirtyState() {
    getSettingsDirty().refresh();
  }

  function renderSettingsDirtyState(dirty) {
    const indicator = $("#settings-dirty-indicator");
    const save = $("#save-runtime-settings");
    if (!indicator || !save) return;
    indicator.classList.toggle("hidden", !dirty);
    save.disabled = !dirty;
    save.setAttribute("aria-disabled", String(!dirty));
  }

  async function saveSettings(event) {
    event.preventDefault();
    const current = state.bootstrap.settings;
    const draft = readSettingsDraft(current);
    if (!draft.activeSources.length) {
      $("#runtime-settings-status").textContent = "Choose at least one active source.";
      return;
    }
    const settings = { ...current, ...draft };
    if (settings.aiDetectionEnabled && settings.aiDetectionPresentation === "hide" && current.aiDetectionPresentation !== "hide") {
      state.pendingSettings = settings;
      openResetDialog("ai-hide");
      return;
    }
    await persistSettings(settings);
  }

  function reasoningProfileValue(processId, fallback) {
    return document.querySelector(`[data-process-id="${processId}"]`)?.value
      || fallback
      || RELEASE_REASONING_DEFAULTS[processId]
      || "luna_high";
  }

  async function persistSettings(settings, confirmationPhrase = "") {
    const status = $("#runtime-settings-status");
    const saveButton = $("#save-runtime-settings");
    status.textContent = "Saving…";
    if (saveButton) saveButton.disabled = true;
    try {
      const previousProvider = state.bootstrap.settings?.reasoningProvider || state.bootstrap.provider;
      const response = await api("/api/settings", { method: "PUT", body: { settings, confirmationPhrase } });
      state.bootstrap.collectionRuntime = response.collectionRuntime ?? state.bootstrap.collectionRuntime;
      applyReasoningRuntimeResponse(state.bootstrap, response);
      renderActiveReasoningProvider();
      renderSettings(response.settings);
      syncOnboardingLearning(shouldShowOnboardingLearning(state.session));
      status.textContent = response.settings.reasoningProvider !== previousProvider
        ? "Saved · the selected reasoning provider is now active."
        : `Saved · ${response.settings.maxScrolls} scrolls · ${response.settings.maxItemsPerSource} items/source`;
      await refreshTimeline();
      return response.settings;
    } catch (error) {
      status.textContent = error.message;
      showError(error);
      return null;
    } finally {
      renderSettingsDirtyState(getSettingsDirty().isDirty());
    }
  }

  function renderReasoningProviderControl(settings) {
    const reasoningProviders = state.bootstrap?.reasoningProviders || [];
    const reasoningProviderSelect = $("#reasoning-provider");
    if (!reasoningProviderSelect) return;
    const activeProvider = settings?.reasoningProvider || state.bootstrap?.provider;
    reasoningProviderSelect.replaceChildren(...reasoningProviders.map((entry) => {
      const option = document.createElement("option");
      option.value = entry.name;
      if (entry.configured === false) {
        option.textContent = `${entry.label} · credential missing`;
      } else if (entry.availabilityRequired && entry.availabilityChecked && !entry.available) {
        option.textContent = `${entry.label} · unavailable`;
      } else {
        option.textContent = entry.label;
      }
      return option;
    }));
    if (reasoningProviders.some((entry) => entry.name === activeProvider)) reasoningProviderSelect.value = activeProvider;
    syncReasoningProviderSelection();
  }

  async function detectReasoningExecutable() {
    const status = $("#runtime-settings-status");
    status.textContent = "Detecting reasoning runtime…";
    try {
      const response = await api("/api/reasoning/runtime/discover", { method: "POST" });
      const runtime = response.reasoningRuntime;
      state.bootstrap.reasoningRuntime = runtime;
      if (runtime.autoRepaired && state.bootstrap.settings) {
        state.bootstrap.settings.reasoningExecutablePath = runtime.executablePath;
      }
      $("#reasoning-executable-label").textContent = runtime.label || "Inference executable";
      $("#reasoning-executable-path").value = runtime.executablePath || "";
      refreshSettingsDirtyState();
      status.textContent = runtime.autoRepaired
        ? "Detected and refreshed · ready to use"
        : "Detected · save settings to use this executable";
    } catch (error) {
      status.textContent = error.message;
      showError(error);
    }
  }

  function syncLoadProfileSettings(applyPreset) {
    const loadProfile = $("#bounded-load-profile").value;
    const preset = LOAD_PROFILE_PRESETS[loadProfile];
    if (applyPreset && preset) {
      $("#timeline-capacity").value = preset.timelineCapacity;
      $("#max-items-per-source").value = preset.maxItemsPerSource;
      $("#max-scrolls").value = preset.maxScrolls;
    }
    const custom = loadProfile === "custom";
    $("#timeline-capacity").disabled = !custom;
    $("#max-items-per-source").disabled = !custom;
    $("#max-scrolls").disabled = !custom;
  }

  function syncSemanticEventSettings() {
    const disabled = $("#semantic-event-mode").value === "show_all";
    $("#semantic-event-shortlist").disabled = disabled;
    $("#semantic-event-shortlist").closest(".settings-row")?.classList.toggle("settings-row-disabled", disabled);
    $("#semantic-event-merge-threshold").disabled = disabled;
    $("#reset-semantic-event-merge-threshold").disabled = disabled;
    $("#semantic-event-merge-threshold").closest(".settings-row")?.classList.toggle("settings-row-disabled", disabled);
  }

  function syncAIDetectionSettings() {
    const enabled = $("#ai-detection-enabled").checked;
    $("#ai-detection-presentation").disabled = !enabled;
    $("#ai-detection-presentation").closest(".settings-row")?.classList.toggle("settings-row-disabled", !enabled);
    const deepProfile = document.querySelector('[data-process-id="ai_deep_detection"]');
    if (deepProfile) deepProfile.disabled = !enabled;
    const hide = enabled && $("#ai-detection-presentation").value === "hide";
    $("#ai-hide-warning").classList.toggle("hidden", !hide);
    const active = state.bootstrap?.settings?.aiDetectionPresentation === "hide";
    $("#ai-hide-status").textContent = active
      ? "Hide is active. Reviewable posts remain stored locally."
      : "Hide requires typed confirmation when saved.";
  }

  function resetSemanticEventMergeThreshold() {
    $("#semantic-event-merge-threshold").value = DEFAULT_SEMANTIC_EVENT_MERGE_THRESHOLD.toFixed(2);
    $("#runtime-settings-status").textContent = "Default restored · save settings to keep it.";
  }

  return {
    renderSettings, syncReasoningProviderSelection, beginSettingsProviderSelection, renderSourceSettingsValues, renderReasoningProcesses, formatReasoningProvider, formatReasoningModel, formatReasoningEffort, readSettingsDraft, refreshSettingsDirtyState, renderSettingsDirtyState, saveSettings, reasoningProfileValue, persistSettings, renderReasoningProviderControl, detectReasoningExecutable, syncLoadProfileSettings, syncSemanticEventSettings, syncAIDetectionSettings, resetSemanticEventMergeThreshold
  };
}

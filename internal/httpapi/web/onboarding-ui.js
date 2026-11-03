import { renderSetupPage } from "./page-navigation.js";
import { sourcePermissionReadyForOnboarding } from "./onboarding-source-readiness.js";

export function createOnboardingUi({
  $, ONBOARDING_LEARNING_INTERVAL_MS, api, bridgeSourceReadiness, buildSourceCard, clearNotice, configureNativePostLink, firstRunCalibrationPending, hideFailure, humanize, onboardingRequiresSetup, openOnboardingProviderDialog, openSourceFromSettings, refreshTimeline, renderBrowserConnection, renderOnboardingSourceReadiness, renderSettings, renderSourceSessionReadiness, requestSourceSessionReadiness, safeSourceUrl, scheduleBackToTop, setPill, setView, showCalibrationRetry, showError, sourceDescriptors, sourceLabel, startVisibleUpdate, state, syncTimelineSidePaneVisibility, terminalStatuses
}) {
  function showOnboarding(editing) {
    state.onboardingEditing = editing;
    const completingFirstOnboarding = onboardingRequiresSetup();
    const sources = completingFirstOnboarding
      ? sourceDescriptors().filter((source) => source.defaultActive).map((source) => source.id)
      : (state.bootstrap?.settings?.activeSources ?? sourceDescriptors().filter((source) => source.defaultActive).map((source) => source.id));
    for (const input of document.querySelectorAll("#onboarding-source-options input[type='checkbox']")) {
      input.checked = sources.includes(input.value);
    }
    $("#onboarding-cancel").classList.toggle("hidden", !editing);
    $("#onboarding-finish").textContent = editing ? "Save profile" : "Continue when ready";
    $("#onboarding-error").textContent = "";
    renderSetupPage(document, "onboarding");
    syncTimelineSidePaneVisibility();
    renderBrowserConnection();
    updateOnboardingSummary();
    if (state.bootstrap?.bridge?.compatible) requestSourceSessionReadiness();
    (state.bootstrap?.bridge?.compatible ? $("#onboarding-heading") : $("#onboarding-browser-heading")).focus();
    window.scrollTo({ top: 0, behavior: editing ? "smooth" : "auto" });
    scheduleBackToTop();
  }

  function updateOnboardingSummary() {
    const activeSources = [...document.querySelectorAll("#onboarding-form input[type='checkbox']:checked")].map((input) => input.value);
    if (!activeSources.length) {
      $("#onboarding-summary").textContent = "Choose at least one source feed, then make one selected source ready.";
      return;
    }
    const readyCount = activeSources.filter((source) => sourceReadyForOnboarding(source)).length;
    const selectedLabel = activeSources.length + " source feed" + (activeSources.length === 1 ? "" : "s") + " selected";
    $("#onboarding-summary").textContent = readyCount
      ? selectedLabel + " · " + readyCount + " access-ready · continue to your first Timeline"
      : selectedLabel + " · grant access to at least one selected source; sign-in is checked when the update opens it";
  }

  function renderActiveReasoningProvider() {
    const provider = state.bootstrap?.provider || state.bootstrap?.settings?.reasoningProvider || "unknown";
    $("#provider-value").textContent = provider;
    setPill("#reasoning-status", provider, "neutral");
  }

  function sourceReadyForOnboarding(source) {
    const access = bridgeSourceReadiness().find((entry) => entry.source === source);
    const session = state.sourceSessionReadiness?.[source];
    return sourcePermissionReadyForOnboarding({ access, session });
  }

  function onboardingReadinessGate(activeSources) {
    if (!state.bootstrap?.bridge?.compatible) {
      return { ready: false, message: "AkuBridge is still connecting. Keep this page open while source access is checked." };
    }
    if (!bridgeSourceReadiness().length) {
      return { ready: false, message: "AkuBrowser is still checking per-source access. Try Continue when ready again in a moment." };
    }
    if (activeSources.some((source) => sourceReadyForOnboarding(source))) return { ready: true };
    const selected = activeSources.map((source) => {
      const access = bridgeSourceReadiness().find((entry) => entry.source === source);
      if (!access || !access.permissionGranted) return sourceLabel(source) + " needs access permission";
      if (!access.scriptRegistered || !access.ready) return sourceLabel(source) + " capture is not ready";
      return sourceLabel(source) + " access is ready";
    });
    return { ready: false, message: "Grant access to at least one selected source before continuing: " + selected.join("; ") + ". Sign-in will be checked when the update opens the source." };
  }

  async function saveOnboarding(event) {
    event.preventDefault();
    const activeSources = [...document.querySelectorAll("#onboarding-form input[type='checkbox']:checked")].map((input) => input.value);
    if (!activeSources.length) {
      $("#onboarding-error").textContent = "Choose at least one active source.";
      return;
    }
    const firstCompletion = onboardingRequiresSetup();
    if (firstCompletion) {
      const gate = onboardingReadinessGate(activeSources);
      if (!gate.ready) {
        $("#onboarding-error").textContent = gate.message;
        requestSourceSessionReadiness();
        renderSourceSessionReadiness();
        return;
      }
      state.pendingOnboardingSources = activeSources;
      openOnboardingProviderDialog();
      return;
    }
    await completeOnboarding(activeSources, false);
  }

  async function completeOnboarding(activeSources, firstCompletion) {
    const button = $("#onboarding-finish");
    button.disabled = true;
    $("#onboarding-error").textContent = "Saving source profile…";
    try {
      const response = await api("/api/onboarding", { method: "PUT", body: { activeSources } });
      state.bootstrap.onboarding = response.onboarding;
      state.bootstrap.settings = response.settings;
      state.bootstrap.calibration = response.calibration;
      renderSettings(response.settings);
      $("#onboarding-error").textContent = "";
      state.pendingOnboardingSources = null;
      setView(firstCompletion ? "timeline" : "settings");
      if (firstCompletion) await startVisibleUpdate();
    } catch (error) {
      $("#onboarding-error").textContent = error.message;
    } finally {
      button.disabled = false;
    }
  }

  function shouldShowOnboardingLearning(session) {
    if (firstRunCalibrationPending()) {
      return Boolean(session && !terminalStatuses.has(session.status));
    }
    return state.bootstrap?.settings?.showLearningPanel === true;
  }

  function syncOnboardingLearning(visible) {
    const panel = $("#onboarding-learning-panel");
    panel.classList.toggle("hidden", !visible);
    if (!visible) {
      stopOnboardingLearningTimer();
      return;
    }
    renderOnboardingLearningSlide();
    syncOnboardingLearningTimer();
  }

  function moveOnboardingLearning(delta, manual = false) {
    const slides = document.querySelectorAll(".onboarding-learning-slide");
    if (!slides.length) return;
    showOnboardingLearningSlide((state.onboardingLearningIndex + delta + slides.length) % slides.length, manual);
  }

  function showOnboardingLearningSlide(index, manual = false) {
    const slides = document.querySelectorAll(".onboarding-learning-slide");
    if (!slides.length) return;
    state.onboardingLearningIndex = Math.max(0, Math.min(index, slides.length - 1));
    renderOnboardingLearningSlide();
    if (manual) restartOnboardingLearningTimer();
  }

  function renderOnboardingLearningSlide() {
    const slides = document.querySelectorAll(".onboarding-learning-slide");
    if (!slides.length) return;
    $("#onboarding-learning-track").style.transform = `translateX(-${state.onboardingLearningIndex * 100}%)`;
    $("#onboarding-learning-count").textContent = `${state.onboardingLearningIndex + 1} of ${slides.length}`;
    for (const [index, slide] of [...slides].entries()) slide.setAttribute("aria-hidden", String(index !== state.onboardingLearningIndex));
    for (const dot of document.querySelectorAll("[data-onboarding-slide]")) {
      const active = Number(dot.dataset.onboardingSlide) === state.onboardingLearningIndex;
      if (active) dot.setAttribute("aria-current", "true");
      else dot.removeAttribute("aria-current");
    }
  }

  function pauseOnboardingLearning(paused) {
    state.onboardingLearningPaused = paused;
    syncOnboardingLearningTimer();
  }

  function toggleOnboardingLearningPlayback() {
    state.onboardingLearningUserPaused = !state.onboardingLearningUserPaused;
    const button = $("#onboarding-learning-toggle");
    button.textContent = state.onboardingLearningUserPaused ? "Play" : "Pause";
    button.setAttribute("aria-pressed", String(state.onboardingLearningUserPaused));
    syncOnboardingLearningTimer();
  }

  function syncOnboardingLearningTimer() {
    const panelVisible = !$("#onboarding-learning-panel").classList.contains("hidden");
    if (!panelVisible || state.onboardingLearningPaused || state.onboardingLearningUserPaused || document.visibilityState !== "visible") {
      stopOnboardingLearningTimer();
      return;
    }
    if (state.onboardingLearningTimer) return;
    state.onboardingLearningTimer = setInterval(() => moveOnboardingLearning(1), ONBOARDING_LEARNING_INTERVAL_MS);
  }

  function restartOnboardingLearningTimer() {
    stopOnboardingLearningTimer();
    syncOnboardingLearningTimer();
  }

  function stopOnboardingLearningTimer() {
    if (!state.onboardingLearningTimer) return;
    clearInterval(state.onboardingLearningTimer);
    state.onboardingLearningTimer = null;
  }

  async function startPendingFirstCalibration(session) {
    if (state.bootstrap?.calibration?.firstRunStatus !== "pending") return null;
    try {
      const { calibration } = await api("/api/calibration/sessions", {
        method: "POST",
        body: { unifiedSessionId: session.id, triggerKind: "first_run" },
      });
      if (calibration.status === "completed") {
        state.bootstrap.calibration = { ...state.bootstrap.calibration, firstRunStatus: "completed", active: null };
        return null;
      }
      state.bootstrap.calibration.active = calibration;
      return calibration;
    } catch (error) {
      if (error.code === "calibration_sample_unavailable" || /no validated calibration entry|requires at least one validated candidate/i.test(error.message)) {
        showCalibrationRetry(session);
        return null;
      }
      showError(new Error(`Calibration could not start: ${error.message}`));
      return null;
    }
  }

  function showCalibration(calibration) {
    clearNotice();
    hideFailure();
    setPill("#sidecar-status", "AkuSidecar ready", "ok");
    state.calibration = calibration;
    state.bootstrap.calibration.active = calibration;
    const unresolved = calibration.samples.findIndex((sample) => !sample.label && !sample.issueCode);
    state.calibrationOrdinal = unresolved >= 0 ? unresolved : Math.max(0, calibration.samples.length - 1);
    renderSetupPage(document, "calibration");
    syncTimelineSidePaneVisibility();
    $("#calibration-heading").focus();
    window.scrollTo({ top: 0, behavior: "auto" });
    renderCalibration();
  }

  function renderCalibration() {
    const calibration = state.calibration;
    if (!calibration) return;
    const total = calibration.samples.length;
    const resolved = calibration.samples.filter((sample) => sample.label || sample.issueCode).length;
    const sample = calibration.samples[state.calibrationOrdinal];
    const progress = total ? Math.round((resolved / total) * 100) : 0;
    $("#calibration-progress").textContent = `${resolved} of ${total}`;
    $("#calibration-progress-bar").style.width = `${progress}%`;
    $("#calibration-progress-bar").parentElement?.setAttribute("aria-valuenow", String(progress));
    $("#calibration-previous").disabled = state.calibrationOrdinal === 0;
    const labels = { more_like_this: "More like this", neutral: "Neutral", less_like_this: "Less like this" };
    $("#calibration-status").textContent = sample?.label
      ? `Current decision: ${labels[sample.label] || humanize(sample.label)}`
      : sample?.issueCode
        ? `Reported capture problem: ${humanize(sample.issueCode)}`
        : "Choose Less, Neutral, or More for this real source entry.";
    $("#calibration-card").replaceChildren(buildCalibrationCard(sample));
  }

  function buildCalibrationCard(sample) {
    const card = document.createElement("article");
    card.className = "calibration-entry";
    if (!sample) {
      card.textContent = "No calibration entry is available.";
      return card;
    }
    const candidate = sample.candidate ?? {};
    const header = document.createElement("div");
    header.className = "calibration-entry-header";
    const source = document.createElement("strong");
    source.textContent = sourceLabel(sample.source);
    const position = document.createElement("span");
    position.textContent = `Source position ${candidate.feedPosition || state.calibrationOrdinal + 1}`;
    header.append(source, position);
    const sourceCard = buildSourceCard({
      source: sample.source,
      evidence: { ...candidate, permalink: candidate.sourceUrl },
      item: {
        source: sample.source,
        sourceUrl: candidate.sourceUrl,
        sourceUrlKind: "native_post",
        author: candidate.author,
        publishedAt: candidate.publishedAt,
        whatChanged: candidate.text || candidate.assessment?.rationale || "No readable text was captured.",
      },
    });
    card.append(header, sourceCard);
    const calibrationSourceUrl = safeSourceUrl(candidate.sourceUrl, sample.source);
    if (calibrationSourceUrl) {
      const link = document.createElement("a");
      configureNativePostLink(link, calibrationSourceUrl, sample.source);
      link.textContent = "Open source entry";
      card.append(link);
    }
    return card;
  }

  function showPreviousCalibrationSample() {
    state.calibrationOrdinal = Math.max(0, state.calibrationOrdinal - 1);
    renderCalibration();
  }

  async function decideCalibration(decision) {
    if (!state.calibration) return;
    const buttons = [$("#calibration-less"), $("#calibration-neutral"), $("#calibration-more"), ...document.querySelectorAll("[data-calibration-issue]")];
    for (const button of buttons) button.disabled = true;
    try {
      const { calibration } = await api(
        `/api/calibration/sessions/${encodeURIComponent(state.calibration.id)}/samples/${state.calibrationOrdinal}`,
        { method: "PUT", body: decision },
      );
      state.calibration = calibration;
      if (calibration.status === "completed") {
        state.bootstrap.calibration = { ...state.bootstrap.calibration, firstRunStatus: "completed", active: null, liveInfluence: calibration.snapshot?.liveInfluence ?? false };
        if (calibration.triggerKind === "first_run") state.bootstrap.settings.showLearningPanel = false;
        state.calibration = null;
        $("#calibration-panel").classList.add("hidden");
        await refreshTimeline();
        setView("timeline");
        return;
      }
      state.bootstrap.calibration.active = calibration;
      const next = calibration.samples.findIndex((sample, index) => index > state.calibrationOrdinal && !sample.label && !sample.issueCode);
      state.calibrationOrdinal = next >= 0 ? next : Math.min(state.calibrationOrdinal + 1, calibration.samples.length - 1);
      renderCalibration();
    } catch (error) {
      $("#calibration-status").textContent = error.message;
    } finally {
      for (const button of buttons) button.disabled = false;
    }
  }

  function renderSourceControls() {
    const onboarding = $("#onboarding-source-options");
    const settings = $("#settings-source-options");
    onboarding.replaceChildren();
    settings.replaceChildren();
    for (const descriptor of sourceDescriptors()) {
      const label = document.createElement("label");
      label.className = "onboarding-source-option";
      const input = document.createElement("input");
      input.type = "checkbox";
      input.value = descriptor.id;
      input.checked = Boolean(descriptor.defaultActive);
      input.addEventListener("change", updateOnboardingSummary);
      const card = document.createElement("span");
      card.className = "onboarding-source-card";
      const icon = document.createElement("span");
      icon.className = "onboarding-source-icon";
      icon.setAttribute("aria-hidden", "true");
      icon.style.background = descriptor.iconBackground;
      icon.style.color = descriptor.iconForeground;
      icon.textContent = descriptor.iconText;
      const copy = document.createElement("span");
      copy.className = "onboarding-source-copy";
      const strong = document.createElement("strong");
      strong.textContent = descriptor.displayName;
      const small = document.createElement("small");
      small.textContent = descriptor.onboardingDescription;
      copy.append(strong, small);
      const check = document.createElement("span");
      check.className = "onboarding-source-check";
      check.setAttribute("aria-hidden", "true");
      check.textContent = "✓";
      card.append(icon, copy, check);
      label.append(input, card);
      const onboardingAccess = document.createElement("div");
      onboardingAccess.className = "onboarding-source-access";
      const onboardingStatuses = document.createElement("span");
      onboardingStatuses.className = "onboarding-source-statuses";
      const onboardingReadiness = document.createElement("small");
      onboardingReadiness.dataset.onboardingSourceReadiness = descriptor.id;
      onboardingReadiness.className = "source-readiness-status";
      onboardingReadiness.textContent = "Access: checking";
      const onboardingSession = document.createElement("small");
      onboardingSession.dataset.onboardingSourceSessionReadiness = descriptor.id;
      onboardingSession.className = "source-session-status";
      onboardingSession.textContent = "Session: checking existing tabs";
      onboardingStatuses.append(onboardingReadiness, onboardingSession);
      const onboardingOpen = document.createElement("button");
      onboardingOpen.type = "button";
      onboardingOpen.className = "secondary-button source-open-button";
      onboardingOpen.dataset.onboardingSourceOpen = descriptor.id;
      onboardingOpen.setAttribute("aria-label", `Open ${descriptor.displayName} source`);
      onboardingOpen.textContent = "Open source";
      onboardingOpen.addEventListener("click", () => openSourceFromSettings(descriptor.id));
      onboardingAccess.append(onboardingStatuses, onboardingOpen);
      const onboardingGroup = document.createElement("div");
      onboardingGroup.className = "onboarding-source-group";
      onboardingGroup.append(label, onboardingAccess);
      onboarding.append(onboardingGroup);

      const settingsLabel = document.createElement("div");
      settingsLabel.className = "settings-row source-settings-row";
      const settingsCopy = document.createElement("span");
      settingsCopy.className = "settings-copy";
      const settingsStrong = document.createElement("strong");
      settingsStrong.textContent = descriptor.displayName;
      const settingsSmall = document.createElement("small");
      settingsSmall.textContent = `Use the ${descriptor.displayName} feed during the next update; sign-in is checked separately.`;
      const readiness = document.createElement("small");
      readiness.dataset.sourceReadiness = descriptor.id;
      readiness.className = "source-readiness-status";
      readiness.textContent = "Checking access";
      const sessionStatus = document.createElement("small");
      sessionStatus.dataset.sourceSessionReadiness = descriptor.id;
      sessionStatus.className = "source-session-status";
      sessionStatus.textContent = "Session: checking existing tabs";
      settingsCopy.append(settingsStrong, settingsSmall, readiness);
      settingsCopy.append(sessionStatus);
      const controls = document.createElement("span");
      controls.className = "source-settings-control";
      const activeLabel = document.createElement("label");
      activeLabel.className = "source-active-control";
      const activeText = document.createElement("span");
      activeText.textContent = "Active";
      const settingsInput = document.createElement("input");
      settingsInput.type = "checkbox";
      settingsInput.value = descriptor.id;
      settingsInput.setAttribute("aria-label", `Use ${descriptor.displayName} during updates`);
      activeLabel.append(settingsInput, activeText);

      const hydrationLabel = document.createElement("label");
      hydrationLabel.className = "source-hydration-input";
      const hydrationText = document.createElement("span");
      hydrationText.textContent = "Hydration";
      const hydrationField = document.createElement("span");
      const hydrationInput = document.createElement("input");
      hydrationInput.type = "number";
      hydrationInput.min = String(descriptor.hydrationTimeoutMinMs / 1000);
      hydrationInput.max = String(descriptor.hydrationTimeoutMaxMs / 1000);
      hydrationInput.step = "1";
      hydrationInput.dataset.sourceHydration = descriptor.id;
      hydrationInput.setAttribute("aria-label", `${descriptor.displayName} hydration wait in seconds`);
      const hydrationUnit = document.createElement("span");
      hydrationUnit.textContent = "s";
      hydrationField.append(hydrationInput, hydrationUnit);
      hydrationLabel.append(hydrationText, hydrationField);

      const reset = document.createElement("button");
      reset.type = "button";
      reset.textContent = "Reset to default";
      reset.addEventListener("click", () => {
        hydrationInput.value = String(descriptor.hydrationTimeoutDefaultMs / 1000);
      });
      const openSource = document.createElement("button");
      openSource.type = "button";
      openSource.className = "secondary-button source-open-button";
      openSource.dataset.sourceOpen = descriptor.id;
      openSource.textContent = "Open source";
      openSource.addEventListener("click", () => openSourceFromSettings(descriptor.id));
      controls.append(activeLabel, hydrationLabel, openSource, reset);
      settingsLabel.append(settingsCopy, controls);
      settings.append(settingsLabel);
    }
    renderSourceSessionReadiness();
    renderOnboardingSourceReadiness();
  }

  return {
    showOnboarding, updateOnboardingSummary, renderActiveReasoningProvider, sourceReadyForOnboarding, onboardingReadinessGate, saveOnboarding, completeOnboarding, shouldShowOnboardingLearning, syncOnboardingLearning, moveOnboardingLearning, showOnboardingLearningSlide, renderOnboardingLearningSlide, pauseOnboardingLearning, toggleOnboardingLearningPlayback, syncOnboardingLearningTimer, restartOnboardingLearningTimer, stopOnboardingLearningTimer, startPendingFirstCalibration, showCalibration, renderCalibration, buildCalibrationCard, showPreviousCalibrationSample, decideCalibration, renderSourceControls
  };
}

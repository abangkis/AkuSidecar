import { bridgeCaptureBusy, bridgeRecoveryState, bridgeReloadVerified } from "./bridge-recovery-state.js";
import { browserCollectorProbeAllowed, collectionModeState } from "./collection-mode.js";
import { failedSourceSessionObservations, sourceAccessReadinessState } from "./onboarding-source-readiness.js";
import { nativePostWaitReason, syncNativePostAvailability } from "./native-post-availability.js";
import { setSettingsClass, setSettingsText } from "./settings-render.js";

export function createCaptureUi({
  $, BRIDGE_CONTEXT_RECOVERY_KEY, BRIDGE_CONTEXT_RECOVERY_WINDOW_MS, BRIDGE_TOKEN_RECOVERY_KEY, NATIVE_POST_OPEN_TIMEOUT_MS, SOURCE_SESSION_PROBE_TIMEOUT_MESSAGE, api, bridgeApi, bridgeSourceReadiness, endpoint, logNativePostTrace, getNativePointerTraces, getNativePostOpening, renderSourceSettingsValues, getRouteNativePost, schedulePassiveMediaEnrichment, setPill, showError, showNotice, sourceDescriptor, sourceDescriptors, state, syncRunButtons, updateOnboardingSummary, setNativePostOpening
}) {
  let lastBridgeReturnRefreshAt = 0;

  function recoverInvalidatedBridgeContext(message) {
    if (!/Extension context invalidated/i.test(String(message ?? ""))) return false;
    const now = Date.now();
    const lastAttempt = Number(sessionStorage.getItem(BRIDGE_CONTEXT_RECOVERY_KEY) || 0);
    if (Number.isFinite(lastAttempt) && now - lastAttempt < BRIDGE_CONTEXT_RECOVERY_WINDOW_MS) {
      showError(new Error("AkuBridge reloaded, but this page could not reconnect automatically. Refresh AkuBrowser once to continue the queued update."));
      return true;
    }
    sessionStorage.setItem(BRIDGE_CONTEXT_RECOVERY_KEY, String(now));
    state.settingsUnloadBypass = true;
    location.reload();
    return true;
  }

  function recoverInvalidBridgeToken(code) {
    if (code !== "invalid_bridge_token") return false;
    const now = Date.now();
    const lastAttempt = Number(sessionStorage.getItem(BRIDGE_TOKEN_RECOVERY_KEY) || 0);
    if (Number.isFinite(lastAttempt) && now - lastAttempt < BRIDGE_CONTEXT_RECOVERY_WINDOW_MS) return false;
    sessionStorage.setItem(BRIDGE_TOKEN_RECOVERY_KEY, String(now));
    state.settingsUnloadBypass = true;
    location.reload();
    return true;
  }

  function pingBridge() {
    if (!browserCollectorProbeAllowed(state.bootstrap?.collectionRuntime)) return;
    window.postMessage({
      type: "AKU_BROWSER_BRIDGE_PING",
      protocolMajor: 2,
      protocolMinor: 0,
    }, endpoint);
  }

  function refreshBridgeAfterUserReturn() {
    if (!state.bootstrap?.bridgeToken || document.visibilityState !== "visible") return;
    const now = Date.now();
    if (now - lastBridgeReturnRefreshAt < 1_000) return;
    lastBridgeReturnRefreshAt = now;
    pingBridge();
    requestSourceSessionReadiness();
  }

  function requestSourceSessionReadiness() {
    if (!browserCollectorProbeAllowed(state.bootstrap?.collectionRuntime)) return;
    if (!state.bootstrap?.bridge?.compatible || state.sourceSessionProbeInFlight) return;
    state.sourceSessionProbeInFlight = true;
    state.sourceSessionProbeTimer = window.setTimeout(() => {
      state.sourceSessionProbeInFlight = false;
      state.sourceSessionProbeTimer = null;
      if (!$("#onboarding-panel").classList.contains("hidden")) $("#onboarding-error").textContent = SOURCE_SESSION_PROBE_TIMEOUT_MESSAGE;
      renderSourceSessionError(SOURCE_SESSION_PROBE_TIMEOUT_MESSAGE);
      updateOnboardingSummary();
    }, 8_000);
    renderSourceSessionReadiness();
    window.postMessage({
      type: "AKU_BROWSER_PROBE_SOURCE_SESSIONS",
    }, endpoint);
  }

  function openSourceFromSettings(source) {
    const descriptor = sourceDescriptor(source);
    if (!descriptor || (!state.bootstrap?.bridge?.compatible && !state.bootstrap?.collectionRuntime?.available)) return;
    setSourceSessionStatus(source, {
      source,
      state: "loading",
      observedAt: new Date().toISOString(),
      tabCount: 1,
    });
    renderSourceSessionReadiness();
    window.postMessage({
      type: "AKU_BROWSER_OPEN_SOURCE",
      source,
    }, endpoint);
  }

  function syncNativePostLinks() {
    for (const link of document.querySelectorAll("[data-aku-native-post]")) syncNativePostAvailability(link,
      nativePostWaitReason(state.bootstrap?.collectionRuntime, state.session, getNativePostOpening() || state.closingNativeReaders, link.dataset.akuNativePost));
  }

  function configureNativePostLink(link, href, source) {
    link.href = href;
    link.dataset.akuNativePost = source;
    link.rel = "noopener noreferrer";
    syncNativePostAvailability(link, nativePostWaitReason(state.bootstrap?.collectionRuntime, state.session, getNativePostOpening() || state.closingNativeReaders, source));
    link.addEventListener("click", (event) => {
      const pointer = getNativePointerTraces().get(link);
      getNativePointerTraces().delete(link);
      const gesture = pointer && Date.now() - pointer.at <= 5000 ? pointer.trace : null;
      if (!event.isTrusted || event.defaultPrevented || event.button !== 0) {
        if (gesture) logNativePostTrace(gesture, "click_ignored", { outcome: "ignored" });
        return;
      }
      event.preventDefault();
      const waitReason = nativePostWaitReason(state.bootstrap?.collectionRuntime, state.session, getNativePostOpening() || state.closingNativeReaders, source);
      syncNativePostAvailability(link, waitReason);
      if (waitReason) return;
      const brokerRequestId = link.dataset.akuReaderRequest;
      if (/^broker_[a-f0-9]{32}$/.test(brokerRequestId ?? "")) {
        const consumed = (link.dataset.akuReaderRequestsConsumed ?? "").split(",")
          .filter((id) => /^broker_[a-f0-9]{32}$/.test(id));
        consumed.push(brokerRequestId);
        link.dataset.akuReaderRequestsConsumed = consumed.join(",");
      }
      delete link.dataset.akuReaderRequest;
      setNativePostOpening(true);
      syncNativePostLinks();
      openNativePostInReaderWindow(href, source, brokerRequestId, gesture).catch(showError).finally(() => {
        setNativePostOpening(false);
        syncNativePostLinks();
      });
    });
  }

  function openNativePostInReaderWindow(url, source, brokerRequestId = null, gesture = null) {
    const requestId = brokerRequestId || `native_post_${Date.now()}_${Math.random().toString(16).slice(2)}`;
    const started = performance.now();
    logNativePostTrace(requestId, "click", { gesture });
    return new Promise((resolve, reject) => {
      let settled = false;
      const routing = new AbortController();
      const timeout = window.setTimeout(() => finish(
        reject,
        new Error("AkuBridge timed out while opening the native post."),
        "timeout",
      ), NATIVE_POST_OPEN_TIMEOUT_MS);
      function finish(callback, value, outcome) {
        if (settled) return;
        settled = true;
        routing.abort();
        window.clearTimeout(timeout);
        window.removeEventListener("message", onResult);
        if (callback === reject && brokerRequestId) {
          cancelNativePostBroker(brokerRequestId, source, url);
        }
        logNativePostTrace(requestId, "terminal", {
          outcome,
          elapsedMs: Math.round(performance.now() - started),
          ...(outcome === "rejected" ? { errorKind: window.akuNativePostDiagnostics?.errorKind(value) ?? "other" } : {}),
        });
        callback(value);
      }
      function onResult(event) {
        if (event.source !== window || event.origin !== endpoint || event.data?.requestId !== requestId) return;
        if (event.data.type === "AKU_BROWSER_NATIVE_POST_OPENED") {
          if (event.data.foreground === "manual_required") {
            showNotice(event.data.message || "The post opened in your Native Reader tab, but Windows did not bring that window to the foreground. Switch to the Native Reader window manually.");
          }
          finish(resolve, event.data, "opened");
        } else if (event.data.type === "AKU_BROWSER_NATIVE_POST_OPEN_FAILED") {
          finish(reject, new Error(event.data.message || "AkuBridge could not open the native post."), "rejected");
        }
      }
      window.addEventListener("message", onResult);
      // Subscribe before reading mode so an early broker rejection is not lost.
      getRouteNativePost()({
        type: "AKU_BROWSER_OPEN_NATIVE_POST",
        requestId,
        source,
        url,
      }, { signal: routing.signal }).then(() => {
        if (!settled) {
          logNativePostTrace(requestId, "dispatch");
        }
      }).catch((error) => finish(reject, error, "rejected"));
    });
  }

  function cancelNativePostBroker(requestId, source, url) {
    if (!/^broker_[a-f0-9]{32}$/.test(requestId || "")) return;
    window.postMessage({
      type: "AKU_BROWSER_CANCEL_NATIVE_POST_BROKER",
      requestId,
      source,
      url,
    }, endpoint);
  }

  function setSourceSessionStatus(source, observation) {
    state.sourceSessionReadiness = {
      ...(state.sourceSessionReadiness ?? {}),
      [source]: observation,
    };
  }

  function renderSourceSessionError(message, source = null) {
    const detail = String(message || "Source session status is unavailable.").slice(0, 160);
    const observedAt = new Date().toISOString();
    if (source) {
      setSourceSessionStatus(source, {
        source,
        state: "unknown",
        observedAt,
        detail,
      });
    } else {
      state.sourceSessionReadiness = failedSourceSessionObservations(
        sourceDescriptors().map((descriptor) => descriptor.id), detail, observedAt,
      );
    }
    renderSourceSessionReadiness();
  }

  function renderSourceSessionReadiness() {
    for (const descriptor of sourceDescriptors()) {
      const observation = state.sourceSessionReadiness?.[descriptor.id] ?? null;
      const status = document.querySelector(`[data-source-session-readiness="${descriptor.id}"]`);
      const button = document.querySelector(`[data-source-open="${descriptor.id}"]`);
      if (!status || !button) continue;
      setSettingsClass(status, "source-session-status" +
        (observation?.state === "ready" ? " source-session-ready" :
         ["login_required", "permission_required", "challenge_required"].includes(observation?.state) ? " source-session-warning" : ""));
      if (state.sourceSessionProbeInFlight && !observation) {
        setSettingsText(status, "Session: checking existing tabs");
        setSettingsText(button, "Open source");
        button.disabled = false;
        continue;
      }
      const stateValue = observation?.state ?? "unknown";
      const labels = {
        ready: "Session: feed available",
        login_required: "Session: sign-in required",
        challenge_required: "Session: human verification required",
        permission_required: "Access: permission required",
        not_observed: "Session: source not open",
        loading: "Session: loading",
        unavailable: "Session: temporarily unavailable",
        unknown: "Session: not confirmed",
      };
      setSettingsText(status, labels[stateValue] ?? labels.unknown);
      status.title = observation?.detail || "";
      setSettingsText(button, stateValue === "permission_required"
        ? "Grant access"
        : stateValue === "login_required" ? "Sign in" : "Open source");
      button.disabled = !state.bootstrap?.bridge?.compatible && !state.bootstrap?.collectionRuntime?.available;
    }
    renderOnboardingSourceReadiness();
  }

  function renderOnboardingSourceReadiness() {
    const accessBySource = new Map(bridgeSourceReadiness().map((entry) => [entry.source, entry]));
    for (const descriptor of sourceDescriptors()) {
      const accessStatus = document.querySelector(`[data-onboarding-source-readiness="${descriptor.id}"]`);
      const sessionStatus = document.querySelector(`[data-onboarding-source-session-readiness="${descriptor.id}"]`);
      const button = document.querySelector(`[data-onboarding-source-open="${descriptor.id}"]`);
      if (!accessStatus || !sessionStatus || !button) continue;
      const access = accessBySource.get(descriptor.id);
      const accessState = sourceAccessReadinessState(access);
      setSettingsClass(accessStatus, "source-readiness-status" +
        (accessState === "ready" ? " source-readiness-ready" :
         ["permission_not_granted", "registration_missing", "capture_not_ready"].includes(accessState) ? " source-readiness-warning" : ""));
      if (accessState === "checking") {
        setSettingsText(accessStatus, "Access: checking");
      } else if (accessState === "permission_not_granted") {
        setSettingsText(accessStatus, "Access: permission required");
      } else if (accessState === "registration_missing") {
        setSettingsText(accessStatus, "Access: capture registration missing");
      } else if (accessState === "capture_not_ready") {
        setSettingsText(accessStatus, "Access: capture not ready");
      } else {
        setSettingsText(accessStatus, "Access: ready");
      }
      const observation = state.sourceSessionReadiness?.[descriptor.id] ?? null;
      const stateValue = observation?.state ?? "unknown";
      const labels = {
        ready: "Session: feed available",
        login_required: "Session: sign-in required",
        challenge_required: "Session: human verification required",
        permission_required: "Session: permission required",
        not_observed: "Session: source not open",
        loading: "Session: loading",
        unavailable: "Session: temporarily unavailable",
        unknown: "Session: not confirmed",
      };
      setSettingsClass(sessionStatus, "source-session-status" +
        (stateValue === "ready" ? " source-session-ready" :
         ["login_required", "permission_required", "challenge_required"].includes(stateValue) ? " source-session-warning" : ""));
      setSettingsText(sessionStatus, labels[stateValue] ?? labels.unknown);
      sessionStatus.title = observation?.detail || "";
      setSettingsText(button, !access?.permissionGranted || stateValue === "permission_required"
        ? "Grant access"
        : stateValue === "login_required" ? "Sign in" : "Open source");
      button.disabled = !state.bootstrap?.bridge?.compatible && !state.bootstrap?.collectionRuntime?.available;
    }
  }

  async function bridgeActionLoop() {
    while (true) {
      try {
        const response = await bridgeApi("/api/operations/bridge/actions/next?waitMs=25000");
        if (response?.action) {
          window.postMessage({
            type: "AKU_BROWSER_BRIDGE_RELOAD_SELF",
            actionId: response.action.id,
            endpoint,
            token: state.bootstrap.bridgeToken,
          }, endpoint);
        }
      } catch (error) {
        console.warn("Bridge action relay paused", error);
        await new Promise((resolve) => setTimeout(resolve, 1000));
      }
    }
  }

  function renderBridge(bridge) {
    if (state.bootstrap) state.bootstrap.bridge = bridge;
    if (state.bootstrap?.collectionRuntime?.effective === "headless") {
      setPill("#bridge-status", "Headless collection ready", "ok");
      $("#bridge-reload").classList.add("hidden");
      syncRunButtons();
      return;
    }
    const bridgeRecoveryButton = $("#bridge-reload");
    const development = state.bootstrap?.deployment?.mode === "development";
    const recovery = bridgeRecoveryState(bridge, development);
    bridgeRecoveryButton.classList.toggle("hidden", !recovery.showReload);
    bridgeRecoveryButton.title = recovery.detail || "Reload AkuBridge";
    bridgeRecoveryButton.disabled = state.bridgeReloadInFlight === true || bridgeCaptureBusy(state.session);
    if (bridgeCaptureBusy(state.session)) bridgeRecoveryButton.title = "Wait for the active capture to finish before reloading AkuBridge.";
    if (bridge?.compatible) {
      const readiness = bridgeSourceReadiness();
      const activeSources = state.bootstrap?.settings?.activeSources ?? [];
      const readySources = new Set(readiness.filter((source) => source.ready).map((source) => source.source));
      const readyActiveCount = activeSources.filter((source) => readySources.has(source)).length;
      if (!readiness.length) {
        setPill("#bridge-status", "AkuBridge source access checking", "warning");
      } else if (readyActiveCount === 0) {
        setPill("#bridge-status", "AkuBridge source access needed", "warning");
      } else if (readyActiveCount < activeSources.length) {
        setPill("#bridge-status", `AkuBridge partial · ${readyActiveCount}/${activeSources.length} sources ready`, "warning");
      } else {
        setPill("#bridge-status", `AkuBridge ${bridge.actual?.extensionVersion} ready`, "ok");
      }
      configureBackgroundBridge();
    } else if (bridge?.state === "incompatible") {
      const reasons = bridge.reasons?.join(", ");
      setPill("#bridge-status", reasons ? `AkuBridge incompatible · ${reasons}` : "AkuBridge incompatible", "danger");
    } else {
      setPill("#bridge-status", "AkuBridge reconnecting", "warning");
    }
    if (recovery.label) setPill("#bridge-status", recovery.label, "warning");
    $("#bridge-status").title = recovery.detail;
    renderBrowserConnection();
    syncRunButtons();
    renderSourceSettingsValues(state.bootstrap?.settings);
    updateOnboardingSummary();
    if (bridge?.compatible) schedulePassiveMediaEnrichment();
  }

  function configureBackgroundBridge() {
    if (!state.bootstrap?.bridgeToken) return;
    window.postMessage({
      type: "AKU_BROWSER_CONFIGURE_BACKGROUND_DISPATCH",
      endpoint,
      token: state.bootstrap.bridgeToken,
    }, endpoint);
  }

  function renderCollectionRuntime() {
    syncNativePostLinks();
    const runtime = state.bootstrap?.collectionRuntime;
    const view = collectionModeState(runtime,state.bootstrap?.bridge?.compatible);
    const select = $("#collection-mode");
    if (!select) return;
    select.querySelector('[value="headless"]').disabled = !view.canSelectHeadless;
    const visibility = $("#capture-visibility-policy");
    // Facebook's Browser exception still follows the selected visibility policy.
    visibility.disabled = false;
    const hybrid = select.value === "headless";
    const hiddenQuiet = !hybrid && runtime?.quietAvailable === true;
    setSettingsText(visibility.querySelector('[value="quiet"]'), hybrid
      ? "Facebook: single window" : hiddenQuiet
      ? "Quiet capture — hidden X/Facebook — recommended" : "Quiet capture — single window — recommended");
    setSettingsText(visibility.querySelector('[value="quiet_multi_window"]'), hybrid
      ? "Facebook: multiple windows" : hiddenQuiet
      ? "Quiet capture — hidden X/Facebook, separate windows for other sources" : "Quiet capture — multiple windows (trial)");
    setSettingsText($("#capture-visibility-description"), hybrid
      ? "Visibility applies to Facebook Browser collection. X, Instagram and LinkedIn use headless. If Facebook content is unavailable, try Adaptive fidelity; its collection window can become visible." : hiddenQuiet
      ? "Quiet keeps X and Facebook collection hidden. Login and native posts still open an interactive window. Other sources retain the selected background-window policy; Adaptive uses a normal Chrome tab."
      : "Single-window Quiet shares one background window; multi-window Quiet remains available for trial; Adaptive uses a canonical tab in your normal Chrome window.");
    setSettingsText($("#collection-runtime-status"), view.detail);
  }

  function renderBrowserConnection() {
    const connection = $("#onboarding-browser-connection");
    const sourceSetup = $("#onboarding-source-setup");
    if (!connection || !sourceSetup) return;
    const connected = state.bootstrap?.bridge?.compatible === true;
    connection.classList.toggle("hidden", connected);
    sourceSetup.classList.toggle("hidden", !connected);
    $("#onboarding-panel").setAttribute("aria-labelledby", connected ? "onboarding-heading" : "onboarding-browser-heading");
    if (connected) {
      $("#onboarding-browser-status").textContent = "";
      return;
    }
    const development = state.bootstrap?.deployment?.mode === "development";
    $("#onboarding-browser-development-actions").classList.toggle("hidden", !development);
    $("#onboarding-browser-copy").textContent = development
      ? "AkuBridge is not connected to this dedicated development profile. Open Chrome Extensions and load it once before choosing source access."
      : "AkuBridge is missing or incompatible. This installed AkuBrowser runtime needs repair before source setup can continue.";
    $("#onboarding-browser-status").textContent = development
      ? "Waiting for AkuBridge to connect…"
      : "Close AkuBrowser and run the installer repair flow.";
  }

  async function openChromeExtensions() {
    const button = $("#open-chrome-extensions");
    const status = $("#onboarding-browser-status");
    button.disabled = true;
    status.textContent = "Opening Chrome Extensions in the AkuBrowser development profile…";
    try {
      await api("/api/app-shell/open-extensions", { method: "POST" });
      status.textContent = "Chrome Extensions opened. Load AkuBridge there; this page will continue automatically when it connects.";
    } catch (error) {
      status.textContent = error.message;
    } finally {
      button.disabled = false;
    }
  }

  async function reloadIncompatibleBridge() {
    if (state.bridgeReloadInFlight) return;
    state.bridgeReloadInFlight = true;
    const button = $("#bridge-reload");
    const idleLabel = "Reload AkuBridge";
    button.disabled = true;
    button.title = "Reload AkuBridge and verify the expected runtime revision";
    button.textContent = "Reloading…";
    try {
      if (bridgeCaptureBusy(state.session)) throw new Error("Wait for the active capture to finish before reloading AkuBridge.");
      const request = await bridgeApi("/api/operations/bridge/actions/reload-self", {
        method: "POST",
        body: {
          requestId: `bridge_recovery_${Date.now()}_${Math.random().toString(16).slice(2)}`,
          actor: { actorType: "user", actorId: "bridge-recovery-ui" },
          reason: "user requested reload of mismatched AkuBridge from the contextual recovery control",
        },
      });
      const actionId = request?.action?.id;
      if (!actionId) throw new Error("AkuSidecar did not return a Bridge reload action.");
      const deadline = Date.now() + 20_000;
      while (Date.now() < deadline) {
        await new Promise((resolve) => setTimeout(resolve, 500));
        const result = await bridgeApi(`/api/operations/bridge/actions/${encodeURIComponent(actionId)}`);
        if (result?.action?.status === "failed") {
          throw new Error(result.action.message || "AkuBridge reload failed.");
        }
        if (result?.action?.status !== "completed") continue;
        const health = await api("/api/bridge/health");
        renderBridge(health.bridge);
        if (!bridgeReloadVerified(health.bridge, result.action)) {
          throw new Error("AkuBridge reloaded without the expected compatible runtime revision.");
        }
        button.textContent = "AkuBridge reloaded";
        return;
      }
      throw new Error("AkuBridge reload timed out. Keep this AkuBrowser page open and try again.");
    } catch (error) {
      button.classList.remove("hidden");
      button.textContent = "Reload failed";
      button.title = error.message;
    } finally {
      window.setTimeout(() => {
        state.bridgeReloadInFlight = false;
        button.disabled = bridgeCaptureBusy(state.session);
        button.textContent = idleLabel;
      }, 1800);
    }
  }

  return {
    recoverInvalidatedBridgeContext, recoverInvalidBridgeToken, pingBridge, refreshBridgeAfterUserReturn, requestSourceSessionReadiness, openSourceFromSettings, syncNativePostLinks, configureNativePostLink, openNativePostInReaderWindow, cancelNativePostBroker, setSourceSessionStatus, renderSourceSessionError, renderSourceSessionReadiness, renderOnboardingSourceReadiness, bridgeActionLoop, renderBridge, configureBackgroundBridge, renderCollectionRuntime, renderBrowserConnection, openChromeExtensions, reloadIncompatibleBridge
  };
}

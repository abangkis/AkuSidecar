import { backToTopBoundaryBottom } from "./timeline-content-context-state.js";
import { backToTopHorizontalPosition } from "./timeline-scroll-layout.js";
import { decorateTimelineConversation, planTimelineConversations } from "./timeline-conversations.js";
import { formatTimelineCheckSummary } from "./timeline-check-summary.js";
import { renderWithCurrentScroll } from "./timeline-media-layout.js";
import { setInlineStyle } from "./ui-frame.js";
import { createTimelineBackgroundRefresh } from "./timeline-background-refresh.js";

export function createTimelineRendering({
  $, AI_DEEP_POLL_INTERVAL_MS, BACK_TO_TOP_THRESHOLD_PX, DEFAULT_TIMELINE_BATCH_GAP_PX, DEFAULT_TIMELINE_BOUNDARY_RETURN_MS, api, buildTimelineItem, closeTimelineContentContextDrawer, formatDate, measureTimelineContentContextTabs, renderAutoUpdateStatus, schedulePassiveMediaEnrichment, scheduleTimelineContentContextPosition, getScrollPerformanceTrace, getScrollUIFrames, showError, showNotice, sourceIdentity, startVisibleUpdate, state, stopObservingInlineVideoVisibilityWithin, syncRunButtons, syncTimelineContentContextTabs, getTimelineScrollIdle
}) {
  const backgroundRefresh = createTimelineBackgroundRefresh();

  function applyStreamWidth(value) {
    document.body.dataset.streamWidth = ["compact", "social", "comfortable", "wide"].includes(value) ? value : "social";
    scheduleBackToTop();
    scheduleTimelineSidePanePosition();
  }

  function applySingleImageFit(value) {
    document.body.dataset.singleImageFit = value === "contain" ? "contain" : "cover";
  }

  function applyPostFreshnessStyle(value) {
    document.body.dataset.postFreshnessStyle = ["header_shade", "border_shade", "off"].includes(value) ? value : "header_shade";
  }

  function scheduleTimelineSidePanePosition() {
    if (state.currentView !== "timeline") return;
    getScrollUIFrames().schedule("side-pane");
  }

  function syncTimelineSidePanePosition() {
    if (state.currentView !== "timeline") return;
    const stream = document.querySelector(".timeline-heading-row");
    if (!stream) return;
    const rect = stream.getBoundingClientRect();
    const firstTimelineItem = document.querySelector("#result-items > [data-timeline-id]");
    const verticalAnchor = firstTimelineItem?.getBoundingClientRect() ?? rect;
    const attachmentInset = firstTimelineItem
      ? Number.parseFloat(window.getComputedStyle(firstTimelineItem).borderTopLeftRadius) || 0
      : 0;
    const attachmentTop = verticalAnchor.top + attachmentInset;
    const viewportPadding = 16;
    const minimumTop = 18;
    const availableWidth = Math.max(0, rect.left - viewportPadding);
    const paneWidth = Math.min(420, Math.max(280, availableWidth));
    const paneLeft = Math.max(viewportPadding, rect.left - paneWidth);
    const paneTop = Math.max(minimumTop, attachmentTop);
    const toggleLeft = Math.max(12, rect.left - 56);
    const toggle = $("#timeline-side-pane-toggle");
    const toggleHeight = toggle?.getBoundingClientRect().height || 72;
    const toggleHalfHeight = toggleHeight / 2;
    const toggleTop = Math.min(
      window.innerHeight - minimumTop - toggleHalfHeight,
      Math.max(minimumTop + toggleHalfHeight, attachmentTop + toggleHalfHeight),
    );
    const pane = $("#timeline-side-pane");
    setInlineStyle(pane, "--timeline-side-pane-left", `${Math.round(paneLeft)}px`);
    setInlineStyle(pane, "--timeline-side-pane-width", `${Math.round(paneWidth)}px`);
    setInlineStyle(pane, "--timeline-side-pane-top", `${Math.round(paneTop)}px`);
    setInlineStyle(toggle, "--timeline-side-pane-toggle-left", `${Math.round(toggleLeft)}px`);
    setInlineStyle(toggle, "--timeline-side-pane-toggle-top", `${Math.round(toggleTop)}px`);
  }

  function applyTimelineBatchGap(value) {
    const parsed = Number.parseInt(value, 10);
    const bounded = Number.isFinite(parsed) ? Math.min(80, Math.max(16, parsed)) : DEFAULT_TIMELINE_BATCH_GAP_PX;
    document.documentElement.style.setProperty("--timeline-batch-gap", `${bounded}px`);
  }

  function resetTimelineBatchGap() {
    $("#timeline-batch-gap").value = DEFAULT_TIMELINE_BATCH_GAP_PX;
    applyTimelineBatchGap(DEFAULT_TIMELINE_BATCH_GAP_PX);
    $("#runtime-settings-status").textContent = "Default restored · save settings to keep it.";
  }

  function applyTimelineBoundaryReturnDuration(value) {
    const parsed = Number.parseInt(value, 10);
    const bounded = Number.isFinite(parsed) ? Math.min(1000, Math.max(100, parsed)) : DEFAULT_TIMELINE_BOUNDARY_RETURN_MS;
    document.documentElement.style.setProperty("--back-to-top-return-duration", `${bounded}ms`);
  }

  function resetTimelineBoundaryReturnDuration() {
    $("#timeline-boundary-return-ms").value = DEFAULT_TIMELINE_BOUNDARY_RETURN_MS;
    applyTimelineBoundaryReturnDuration(DEFAULT_TIMELINE_BOUNDARY_RETURN_MS);
    $("#runtime-settings-status").textContent = "Default restored · save settings to keep it.";
  }

  function scheduleBackToTop() {
    getScrollUIFrames().schedule("back-to-top");
  }

  function syncBackToTopNow() {
    const top = document.scrollingElement?.scrollTop ?? window.scrollY ?? 0;
    const measurements = state.currentView === "timeline" ? measureTimelineContentContextTabs() : [];
    $("#back-to-top").classList.toggle("hidden", top < BACK_TO_TOP_THRESHOLD_PX);
    if (top >= BACK_TO_TOP_THRESHOLD_PX) syncBackToTopPosition(top, measurements);
    syncTimelineContentContextTabs(measurements);
  }

  function timelineContentContextObstacles(measurements) {
    if (state.currentView !== "timeline") return [];
    const obstacles = [];
    for (const { rect: anchorRect, width, height, eligible } of measurements) {
      if (!eligible) continue;
      if (!anchorRect || anchorRect.bottom < 0 || anchorRect.top > window.innerHeight) continue;
      if (width <= 0 || height <= 0) continue;
      const left = anchorRect.right - 1;
      obstacles.push({ left, right: left + width, top: anchorRect.top, bottom: anchorRect.top + height });
    }
    const drawer = state.timelineContentContextDrawerOpen && $("#timeline-content-context-drawer");
    if (drawer && !drawer.classList.contains("hidden")) obstacles.push(drawer.getBoundingClientRect());
    return obstacles;
  }

  function syncBackToTopPosition(top, measurements) {
    const anchorsByView = {
      timeline: [$("#result-panel"), document.querySelector(".timeline-heading-row")],
      inbox: [$("#inbox-panel")],
      library: [$("#library-panel")],
      topics: [$("#topics-panel")],
      settings: [$("#settings-panel")],
    };
    const candidates = anchorsByView[state.currentView] ?? [$("#timeline-panel")];
    let anchorRect;
    for (const element of candidates) {
      if (!element || element.classList.contains("hidden")) continue;
      const rect = element.getBoundingClientRect();
      if (rect.width > 0 && rect.height > 0) { anchorRect = rect; break; }
    }
    const button = $("#back-to-top");
    const measured = button.getBoundingClientRect();
    const obstacles = timelineContentContextObstacles(measurements);
    const bottom = syncBackToTopBoundaryPosition(top, measured.height || 48);
    const buttonRect = Number.isFinite(bottom)
      ? { width: measured.width, top: window.innerHeight - bottom - measured.height, bottom: window.innerHeight - bottom }
      : measured;
    const position = backToTopHorizontalPosition({ anchor: anchorRect, button: buttonRect, viewportWidth: window.innerWidth, obstacles });
    setInlineStyle(button, "left", position.left);
    setInlineStyle(button, "right", position.right);
  }

  function syncBackToTopBoundaryPosition(top, buttonHeight) {
    const button = $("#back-to-top");
    const delta = top - state.backToTopLastScrollY;
    state.backToTopLastScrollY = top;
    const cueEnabled = state.bootstrap?.settings?.timelineBoundaryCueMode !== "static";
    if (!cueEnabled || state.currentView !== "timeline" || delta < -1 || top < BACK_TO_TOP_THRESHOLD_PX) {
      releaseBackToTopBoundary();
      return;
    }

    const restBottom = window.innerWidth <= 700 ? 14 : 20;
    const safeTop = 16 + buttonHeight / 2;
    const acquisitionBottom = window.innerHeight;
    const linePosition = (marker) => marker?.isConnected ? marker.getBoundingClientRect().bottom : Number.NaN;
    let marker = state.backToTopBoundary;
    let lineY = linePosition(marker);
    const markerVisible = Number.isFinite(lineY) && lineY >= safeTop && lineY <= acquisitionBottom;

    if (marker && !markerVisible) {
      marker = null;
    }
    if (!marker && delta > 1) {
      marker = [...document.querySelectorAll(".timeline-older-batch-marker")].find((candidate) => {
        const candidateY = linePosition(candidate);
        return Number.isFinite(candidateY) && candidateY >= safeTop && candidateY <= acquisitionBottom;
      }) ?? null;
      lineY = linePosition(marker);
    }
    if (!marker || !Number.isFinite(lineY)) { releaseBackToTopBoundary(); return; }

    state.backToTopBoundary = marker;
    button.classList.add("is-following-boundary");
    const bottom = backToTopBoundaryBottom({ lineY, viewportHeight: window.innerHeight, restBottom });
    setInlineStyle(button, "--back-to-top-bottom", `${Math.round(bottom)}px`);
    return Math.round(bottom);
  }

  function releaseBackToTopBoundary() {
    state.backToTopBoundary = null;
    const button = $("#back-to-top");
    button.classList.remove("is-following-boundary");
    setInlineStyle(button, "--back-to-top-bottom", "");
  }

  function returnToTop() {
    const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    window.scrollTo({ top: 0, behavior: reducedMotion ? "auto" : "smooth" });
    $("#app-heading").focus({ preventScroll: true });
  }

  async function refreshTimeline(options = {}) {
    try {
      const configuredLimit = state.bootstrap?.settings?.timelineCapacity ?? 24;
      const limit = Math.min(50, configuredLimit + Math.max(0, options.extraItems || 0));
      const { items, timelineBatches, latestCheck, autoUpdate } = await api(`/api/timeline?limit=${limit}&offset=0`);
      if (options.background) {
        // Scrolling may have started while the request was in flight.
        await getTimelineScrollIdle().wait();
        if (timelineInteractionActive()) { state.backgroundTimelineRefreshPending = true; return; }
      }
      if (state.bootstrap) state.bootstrap.latestCheck = latestCheck ?? null;
      if (state.bootstrap) state.bootstrap.timelineBatches = timelineBatches ?? state.bootstrap.timelineBatches ?? [];
      if (state.bootstrap) state.bootstrap.autoUpdate = autoUpdate ?? state.bootstrap.autoUpdate;
      state.timelineBatches = timelineBatches ?? state.timelineBatches;
      renderAutoUpdateStatus(autoUpdate);
      const timelineItems = options.revealedSessionID
        ? orderTimelineForRevealedBatch(
            items ?? [],
            options.previousItems ?? [],
            options.revealedSessionID,
            options.revealPlacement ?? "append",
          )
        : items ?? [];
      if (options.background && backgroundRefresh.isUnchanged(
        timelinePresentationSnapshot(timelineItems, latestCheck ?? null, state.timelineBatches, options.highlightSessionID || ""),
      )) {
        // Polling must follow the fresh response even when existing cards stay
        // mounted. Existing event handlers retain their rendered entry objects.
        scheduleAIDeepRefresh(routeAIDetectedItems(timelineItems).pending);
        schedulePassiveMediaEnrichment(timelineItems);
        return;
      }
      const render = () => renderTimeline(timelineItems, latestCheck ?? null, state.timelineBatches, options.highlightSessionID || "");
      if (options.preserveScroll || options.background) renderWithCurrentScroll(render);
      else render();
    } catch (error) {
      showError(error);
    }
  }

  function orderTimelineForRevealedBatch(items, previousItems, revealedSessionID, placement = "append") {
    const byID = new Map(items.map((entry) => [entry.id, entry]));
    const previousIDs = new Set(previousItems.map((entry) => entry.id));
    const previous = previousItems.map((entry) => byID.get(entry.id)).filter(Boolean);
    const revealed = items.filter((entry) => entry.sessionId === revealedSessionID && !previousIDs.has(entry.id));
    const included = new Set([...previous, ...revealed].map((entry) => entry.id));
    const remaining = items.filter((entry) => !included.has(entry.id));
    if (placement === "prepend") return [...revealed, ...previous, ...remaining];
    return [...previous, ...revealed, ...remaining];
  }

  async function handleFinishLineAction() {
    const revealed = await revealPreparedBatch("continue");
    if (!revealed) await startVisibleUpdate();
  }

  async function revealPreparedBatch(presentation) {
    if (state.revealingPreparedBatch) return false;
    state.revealingPreparedBatch = true;
    syncRunButtons();
    let sessionID = "";
    try {
      const { autoUpdate } = await api("/api/auto-update/status");
      if (state.bootstrap) state.bootstrap.autoUpdate = autoUpdate;
      renderAutoUpdateStatus(autoUpdate);
      sessionID = autoUpdate?.preparedBatches?.[0]?.sessionId || "";
      if (!sessionID) return false;
      const previousItems = [...state.timelineItems];
      const revealPlacement = presentation === "latest" ? "prepend" : "append";
      const { batch } = await api(`/api/auto-update/batches/${encodeURIComponent(sessionID)}/reveal`, {
        method: "POST",
        body: { presentation: revealPlacement },
      });
      if (presentation === "latest") {
        await refreshTimeline({
          revealedSessionID: sessionID,
          revealPlacement,
          previousItems,
          extraItems: batch?.itemCount || 0,
        });
        showNotice(`Loaded ${batch?.itemCount || 0} item${batch?.itemCount === 1 ? "" : "s"} from the next prepared batch.`);
        requestAnimationFrame(() => {
          $("#timeline-heading")?.scrollIntoView({ block: "start", behavior: "smooth" });
          $("#timeline-heading")?.focus({ preventScroll: true });
        });
      } else {
        await refreshTimeline({
          revealedSessionID: sessionID,
          revealPlacement,
          previousItems,
          extraItems: batch?.itemCount || 0,
          preserveScroll: true,
          highlightSessionID: sessionID,
        });
      }
      return true;
    } catch (error) {
      showError(error);
      return false;
    } finally {
      state.revealingPreparedBatch = false;
      syncRunButtons();
    }
  }

  function routeAIDetectedItems(items) {
    if (state.bootstrap?.settings?.aiDetectionEnabled === false) {
      return { inline: [...items], drawer: [], hidden: [], pending: false };
    }
    const mode = state.bootstrap?.settings?.aiDetectionPresentation || "drawer";
    const result = { inline: [], drawer: [], hidden: [], pending: false };
    for (const entry of items) {
      const detection = entry.aiDetection;
      result.pending ||= Boolean(detection?.pendingDeep || detection?.pendingMedia);
      const seen = state.seenTimelineItems.has(entry.id);
      const explicitAIOverride = detection?.personalPolicy?.applied
        && detection.personalPolicy.verdict === "ai";
      const keepPreviouslySeenInline = seen
        && !detection?.directOriginEvidence
        && !explicitAIOverride;
      if (mode === "drawer" && detection?.routeToSignals && !keepPreviouslySeenInline) result.drawer.push(entry);
      else if (mode === "hide" && detection?.hideEligible && !keepPreviouslySeenInline) result.hidden.push(entry);
      else result.inline.push(entry);
    }
    return result;
  }

  function renderTimeline(items, latestCheck, timelineBatches = null, highlightSessionID = "") {
    const presentation = timelinePresentationSnapshot(items, latestCheck, timelineBatches, highlightSessionID);
    // A failed render may already have torn down the old DOM. It cannot leave
    // the previous successful checkpoint eligible for a later no-op.
    backgroundRefresh.invalidate();
    const rendered = getScrollPerformanceTrace().measure("timeline_render", () => renderTimelineNow(items, latestCheck, timelineBatches, highlightSessionID));
    backgroundRefresh.commit(presentation);
    return rendered;
  }

  function timelinePresentationSnapshot(items, latestCheck, timelineBatches, highlightSessionID) {
    const routed = routeAIDetectedItems(items ?? []);
    return backgroundRefresh.capture({
      items: items ?? [],
      latestCheck: latestCheck ?? null,
      timelineBatches: Array.isArray(timelineBatches)
        ? timelineBatches
        : (state.bootstrap?.timelineBatches ?? state.timelineBatches ?? []),
      settings: state.bootstrap?.settings ?? {},
      routing: Object.fromEntries(["inline", "drawer", "hidden"].map(key => [key, routed[key].map(entry => entry.id)])),
      preparedBatchCount: state.bootstrap?.autoUpdate?.preparedBatches?.length ?? 0,
      highlightSessionID: highlightSessionID || "",
      controls: {
        sessionActive: Boolean(state.session),
        mediaRecaptureActive: Boolean(state.mediaRecaptureActive),
        collectionMode: state.bootstrap?.collectionRuntime?.effective,
        bridgeCompatible: state.bootstrap?.bridge?.compatible,
        onboardingStatus: state.bootstrap?.onboarding?.status,
        onboardingEditing: Boolean(state.onboardingEditing),
        calibrationActive: Boolean(state.bootstrap?.calibration?.active),
      },
    });
  }

  function renderTimelineNow(items, latestCheck, timelineBatches = null, highlightSessionID = "") {
    $("#finish-line").classList.remove("hidden");
    const allItems = Array.isArray(items) ? items : [];
    const batchMetadata = Array.isArray(timelineBatches)
      ? timelineBatches
      : (state.bootstrap?.timelineBatches ?? state.timelineBatches ?? []);
    state.timelineBatches = batchMetadata;
    state.timelineItems = allItems;
    const retainedIDs = new Set(allItems.map((entry) => entry.id));
    for (const timelineID of state.passiveMediaEvidenceAttempts.keys()) {
      if (!retainedIDs.has(timelineID)) state.passiveMediaEvidenceAttempts.delete(timelineID);
    }
    for (const key of state.expandedTimelineText) {
      if (!retainedIDs.has(key.split("|")[0])) state.expandedTimelineText.delete(key);
    }
    for (const timelineID of state.expandedAIDetails) {
      if (!retainedIDs.has(timelineID)) state.expandedAIDetails.delete(timelineID);
    }
    for (const timelineID of state.expandedAIFeedbackOptions) {
      if (!retainedIDs.has(timelineID)) state.expandedAIFeedbackOptions.delete(timelineID);
    }
    for (const timelineID of state.timelineContentContext.keys()) {
      if (!retainedIDs.has(timelineID)) state.timelineContentContext.delete(timelineID);
    }
    if (state.timelineContentContextActiveID && !retainedIDs.has(state.timelineContentContextActiveID)) {
      closeTimelineContentContextDrawer({ clearActive: true, focusTrigger: false });
    }
    schedulePassiveMediaEnrichment(allItems);
    const routed = routeAIDetectedItems(items);
    items = routed.inline;
    renderTimelineSidePane(routed.drawer, routed.pending);
    scheduleAIDeepRefresh(routed.pending);
    const container = $("#result-items");
    stopObservingInlineVideoVisibilityWithin(container);
    container.replaceChildren();
    if (latestCheck) {
      const duplicates = latestCheck.duplicateReports ?? 0;
      const parts = [formatTimelineCheckSummary(latestCheck)];
      if (duplicates) parts.push(`${duplicates} duplicate report${duplicates === 1 ? "" : "s"}`);
      if (routed.drawer.length) parts.push(`${routed.drawer.length} in AI Signals`);
      if (routed.hidden.length) parts.push(`${routed.hidden.length} AI-signal post${routed.hidden.length === 1 ? "" : "s"} hidden`);
      $("#timeline-meta").textContent = `${parts.join(" \u00b7 ")} from the latest check`;
    } else {
      $("#timeline-meta").textContent = "No completed check yet.";
    }
    $("#finish-stats").textContent = items.length
      ? `Shown: ${items.length} · bounded local evidence · personalized across sources`
      : "Check active sources to establish the finite timeline.";
    if (items.length || routed.drawer.length || routed.hidden.length) {
      $("#finish-stats").textContent = `Shown: ${items.length} · ${routed.drawer.length} in side pane · ${routed.hidden.length} hidden · bounded local evidence`;
    }
    const prepared = state.bootstrap?.autoUpdate?.preparedBatches?.length ?? 0;
    renderTimelineActions();
    if (prepared > 0) {
      $("#finish-stats").textContent += ` · ${prepared} prepared`;
    }
    if (!items.length) {
      const empty = document.createElement("section");
      empty.className = "finish-line";
      const title = document.createElement("h3");
      title.textContent = routed.drawer.length || routed.hidden.length ? "No inline updates in this view" : "No retained updates yet";
      const detail = document.createElement("p");
      detail.textContent = routed.drawer.length
        ? "Strong AI-origin signals are available in the AI Signals side pane."
        : routed.hidden.length
          ? "The active high-risk Hide policy removed Deep-confirmed or directly labeled posts from this view."
          : "AkuBrowser will place evaluated, source-backed items here after the next bounded check.";
      empty.append(title, detail);
      container.append(empty);
      syncTimelineContentContextTabs();
      scheduleBackToTop();
      scheduleTimelineContentContextPosition();
      return;
    }

    const latestSession = latestCheck?.sessionId ?? items[0].sessionId;
    const batchBySession = new Map(batchMetadata.map((batch) => [batch.sessionId, batch]));
    let previousSession = null;
    let historyBoundaryMarked = false;
    const conversations = planTimelineConversations(items);
    for (const group of conversations) {
      const entry = group.anchor;
      if (entry.sessionId !== previousSession) {
        const marker = document.createElement("div");
        marker.className = "timeline-batch-marker";
        if (previousSession !== null) {
          marker.classList.add("timeline-older-batch-marker");
        }
        if (!historyBoundaryMarked && previousSession === latestSession && entry.sessionId !== latestSession) {
          marker.classList.add("timeline-history-boundary");
          marker.setAttribute("role", "separator");
          marker.setAttribute("aria-label", "Earlier retained updates");
          historyBoundaryMarked = true;
        }
        const checked = document.createElement("strong");
        const metadata = batchBySession.get(entry.sessionId);
        const checkedAt = metadata?.completedAt || (entry.sessionId === latestCheck?.sessionId ? latestCheck.completedAt : entry.createdAt);
        const detail = document.createElement("span");
        const isPrepared = metadata?.delivery === "prepared" || metadata?.trigger === "scheduler";
        if (isPrepared) marker.classList.add("timeline-auto-batch-marker");
        if (entry.sessionId === highlightSessionID) {
          marker.classList.add("timeline-revealed-batch-marker");
          marker.setAttribute("aria-label", "New prepared batch");
          checked.textContent = "New prepared batch";
          detail.textContent = `Checked ${formatDate(checkedAt)} · continue scrolling`;
        } else {
          checked.textContent = isPrepared ? "Auto check" : `Checked ${formatDate(checkedAt)}`;
          detail.textContent = isPrepared
            ? `Checked ${formatDate(checkedAt)} · ${metadata?.itemCount ?? 0} item${metadata?.itemCount === 1 ? "" : "s"} in this batch`
            : "Unified personalized order";
        }
        marker.append(checked, detail);
        container.append(marker);
        previousSession = entry.sessionId;
      }
      for (const member of group.members) {
        const rendered = buildTimelineItem(member.entry);
        rendered.dataset.timelineId = member.entry.id;
        rendered.setAttribute("tabindex", "-1");
        decorateTimelineConversation(rendered, member, group, {
          documentRef: document,
          formatDate,
          formatAuthor: (author, source) => sourceIdentity(author, source).displayName,
          checkedAt: value => batchBySession.get(value.sessionId)?.completedAt || value.createdAt,
          navigateToParent: timelineID => {
            const parent = [...container.querySelectorAll(":scope > [data-timeline-id]")]
              .find(card => card.dataset.timelineId === timelineID);
            parent?.scrollIntoView({ behavior: window.matchMedia("(prefers-reduced-motion: reduce)").matches ? "instant" : "smooth", block: "start" });
            parent?.focus({ preventScroll: true });
            if (parent) {
              parent.classList.add("timeline-conversation-target");
              window.setTimeout(() => parent.classList.remove("timeline-conversation-target"), 1600);
            }
          },
        });
        container.append(rendered);
        observeTimelineItem(rendered, member.entry.id);
      }
    }
    syncTimelineContentContextTabs();
    scheduleBackToTop();
    scheduleTimelineContentContextPosition();
  }

  function renderTimelineActions() {
    const prepared = state.bootstrap?.autoUpdate?.preparedBatches?.length ?? 0;
    const primary = $("#timeline-runner-button");
    const preparedButton = $("#timeline-prepared-button");
    const finish = $("#done-button");
    primary.textContent = "Update now";
    primary.classList.remove("hidden");
    if (prepared > 0) {
      preparedButton.textContent = `Load next batch (${prepared})`;
      preparedButton.classList.remove("hidden");
      finish.textContent = `Continue with next batch (${prepared})`;
      finish.classList.remove("hidden");
      return;
    }
    preparedButton.classList.add("hidden");
    finish.textContent = "Update now";
    finish.classList.remove("hidden");
  }

  function observeTimelineItem(element, timelineId) {
    if (!("IntersectionObserver" in window)) return;
    const observer = new IntersectionObserver((entries) => {
      if (entries.some((entry) => entry.isIntersecting)) {
        state.seenTimelineItems.add(timelineId);
        observer.disconnect();
      }
    }, { threshold: 0.35 });
    observer.observe(element);
  }

  function scheduleAIDeepRefresh(pending) {
    if (!pending) {
      if (state.aiDeepPoller) clearTimeout(state.aiDeepPoller);
      state.aiDeepPoller = null;
      return;
    }
    if (state.aiDeepPoller) return;
    state.aiDeepPoller = window.setTimeout(() => {
      state.aiDeepPoller = null;
      queueBackgroundTimelineRefresh();
    }, AI_DEEP_POLL_INTERVAL_MS);
  }

  function timelineInteractionActive() {
    const timeline = $("#result-items");
    if (!timeline) return false;
    if (timeline.querySelector(".event-suggestion-editor")) return true;
    if (timeline.querySelector(".ai-feedback-options[open]")) return true;
    const active = document.activeElement;
    return Boolean(
      active
      && timeline.contains(active)
      && active.matches("button, select, summary, input, textarea, a[href]"),
    );
  }

  function queueBackgroundTimelineRefresh() {
    state.backgroundTimelineRefreshPending = true;
    void flushBackgroundTimelineRefresh();
  }

  async function flushBackgroundTimelineRefresh() {
    if (
      !state.backgroundTimelineRefreshPending
      || state.backgroundTimelineRefreshInFlight
      || timelineInteractionActive()
    ) return;
    state.backgroundTimelineRefreshInFlight = true;
    try {
      await getTimelineScrollIdle().wait();
      if (timelineInteractionActive()) return;
      state.backgroundTimelineRefreshPending = false;
      await refreshTimeline({ background: true });
    } finally {
      state.backgroundTimelineRefreshInFlight = false;
    }
  }

  function renderTimelineSidePane(items, pending) {
    scheduleTimelineSidePanePosition();
    state.sidePaneItems = items;
    const available = syncTimelineSidePaneVisibility();
    $("#timeline-side-pane-count").textContent = String(items.length);
    $("#timeline-side-pane-detail").textContent = pending
      ? `${items.length} routed post${items.length === 1 ? "" : "s"} · Deep Detection is still reviewing this Timeline.`
      : `${items.length} strong-signal post${items.length === 1 ? "" : "s"} routed from the current finite Timeline.`;
    if (!available) closeTimelineSidePane();
    if (!$("#timeline-side-pane").classList.contains("hidden")) populateTimelineSidePane();
  }

  function timelineSidePaneAvailable() {
    return state.bootstrap?.settings?.aiDetectionEnabled !== false
      && state.bootstrap?.settings?.aiDetectionPresentation === "drawer"
      && state.currentView === "timeline"
      && state.bootstrap?.onboarding?.status === "completed"
      && !state.onboardingEditing
      && !state.bootstrap?.calibration?.active
      && Boolean(state.bootstrap?.latestCheck)
      && state.timelineItems.length > 0;
  }

  function syncTimelineSidePaneVisibility() {
    const available = timelineSidePaneAvailable();
    const toggle = $("#timeline-side-pane-toggle");
    toggle.classList.toggle("hidden", !available);
    toggle.classList.toggle("view-hidden", !available);
    if (!available) closeTimelineSidePane();
    return available;
  }

  function openTimelineSidePane() {
    if (!timelineSidePaneAvailable()) return;
    $("#timeline-side-pane").classList.remove("hidden");
    $("#timeline-side-pane-toggle").setAttribute("aria-expanded", "true");
    populateTimelineSidePane();
  }

  function closeTimelineSidePane() {
    $("#timeline-side-pane").classList.add("hidden");
    $("#timeline-side-pane-toggle").setAttribute("aria-expanded", "false");
  }

  function populateTimelineSidePane() {
    const container = $("#timeline-side-pane-items");
    container.replaceChildren();
    if (!state.sidePaneItems.length) {
      const empty = document.createElement("p");
      empty.className = "timeline-side-pane-empty";
      empty.textContent = "No strong AI-origin signals are routed here yet.";
      container.append(empty);
      return;
    }
    for (const entry of state.sidePaneItems) {
      const item = buildTimelineItem(entry, { contentContext: false });
      item.classList.add("timeline-side-pane-card");
      item.dataset.timelineId = entry.id;
      container.append(item);
    }
  }

  return {
    applyStreamWidth, applySingleImageFit, applyPostFreshnessStyle, scheduleTimelineSidePanePosition, syncTimelineSidePanePosition, applyTimelineBatchGap, resetTimelineBatchGap, applyTimelineBoundaryReturnDuration, resetTimelineBoundaryReturnDuration, scheduleBackToTop, syncBackToTopNow, timelineContentContextObstacles, syncBackToTopPosition, syncBackToTopBoundaryPosition, releaseBackToTopBoundary, returnToTop, refreshTimeline, orderTimelineForRevealedBatch, handleFinishLineAction, revealPreparedBatch, routeAIDetectedItems, renderTimeline, renderTimelineNow, renderTimelineActions, observeTimelineItem, scheduleAIDeepRefresh, timelineInteractionActive, queueBackgroundTimelineRefresh, flushBackgroundTimelineRefresh, renderTimelineSidePane, timelineSidePaneAvailable, syncTimelineSidePaneVisibility, openTimelineSidePane, closeTimelineSidePane, populateTimelineSidePane
  };
}

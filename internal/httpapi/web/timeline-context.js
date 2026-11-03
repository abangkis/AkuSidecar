import { CONTENT_CONTEXT_MAX_LIMIT, CONTENT_CONTEXT_TAB_DEFAULT_WIDTH, CONTENT_CONTEXT_UP_SCROLL_MODE_DEFAULT, beginTimelineContentContextLookup, buildContentContextFeedbackUndoPath, buildTimelineContentContextFeedbackPath, buildTimelineContentContextPath, contentContextCanExpand, contentContextDrawerHeightLimits, contentContextDrawerMode, contentContextPostPassedReadingExitLine, contentContextRailPlacement, contentContextShouldCloseOnScroll, contentContextTabFits, selectContentContextViewportID, settleTimelineContentContextLookup } from "./timeline-content-context-state.js";
import { buildTimelineContentContextTab, renderTimelineDirectContext, syncTimelineContentContextTab } from "./timeline-content-context-ui.js";

export function createTimelineContext({
  $, api, configureNativePostLink, formatDate, safeSourceUrl, scheduleBackToTop, showError, sourceLabel, state, syncBackToTopNow, getTimelineContentContextAnchorResizeObserver
}) {
  function timelineContentContextAnchor(id) {
    const trigger = timelineContentContextTrigger(id);
    if (trigger && !trigger.closest(".semantic-duplicate-report.hidden")) {
      return trigger.closest(".timeline-content-context-anchor");
    }
    const candidates = [...document.querySelectorAll("#result-items > [data-timeline-id]")]
      .filter((element) => element.dataset.timelineId === String(id));
    return candidates[0] || null;
  }

  function timelineContentContextTrigger(id) {
    return [...document.querySelectorAll("#result-items [data-timeline-content-context-id]")]
      .find((element) => element.dataset.timelineContentContextId === String(id)) || null;
  }

  function measureTimelineContentContextTabs() {
    if (state.currentView !== "timeline") return [];
    const tabs = [...document.querySelectorAll("#result-items [data-timeline-content-context-id]")];
    // Finish every geometry read before any tab attribute/class writes.
    return tabs.map((tab) => {
      const anchor = tab.closest(".timeline-content-context-anchor");
      const rect = anchor?.getBoundingClientRect();
      const tabRect = tab.getBoundingClientRect();
      return {
        tab,
        id: tab.dataset.timelineContentContextId || "",
        rect,
        width: tabRect.width,
        height: tabRect.height,
        top: rect?.top,
        bottom: rect?.bottom,
        eligible: state.currentView === "timeline" && !tab.closest(".semantic-duplicate-report.hidden"),
      };
    });
  }

  function syncTimelineContentContextTabs(measured = measureTimelineContentContextTabs()) {
    if (state.currentView !== "timeline") return;
    const viewportWidth = document.documentElement.clientWidth || window.innerWidth;
    const visibleID = state.timelineContentContextDrawerOpen
      ? state.timelineContentContextActiveID
      : selectContentContextViewportID({
        candidates: measured,
        viewportHeight: window.innerHeight,
        previousID: state.timelineContentContextViewportID,
      });
    state.timelineContentContextViewportID = visibleID;
    for (const measurement of measured) {
      const { tab, id, rect: postRect, width, eligible } = measurement;
      const active = state.timelineContentContextDrawerOpen && state.timelineContentContextActiveID === id;
      const fits = state.currentView === "timeline"
        && id === visibleID
        && eligible
        && postRect
        && contentContextTabFits({
          postRight: postRect.right,
          // The back-to-top control yields this horizontal gutter to the tabs
          // when its content-side placement would collide with one. Keeping the
          // fit boundary at the viewport edge prevents an older post's control
          // from making the next readable post lose its trigger.
          boundaryLeft: viewportWidth,
          viewportWidth,
          tabWidth: width || CONTENT_CONTEXT_TAB_DEFAULT_WIDTH,
        });
      syncTimelineContentContextTab(tab, { active });
      tab.classList.toggle("is-visible", Boolean(fits));
      tab.classList.toggle("is-retracting", !fits);
    }
  }

  function timelineContentContextFeedbackKey(timelineID, memoryItemID) {
    return `${String(timelineID || "")}\n${String(memoryItemID || "")}`;
  }

  async function submitTimelineContentContextFeedback(timelineID, match, verdict) {
    const memoryItemID = String(match?.item?.id || "").trim();
    const current = state.timelineContentContext.get(timelineID);
    if (!timelineID || !memoryItemID || !current) return;
    const key = timelineContentContextFeedbackKey(timelineID, memoryItemID);
    if (state.timelineContentContextFeedbackInFlight.has(key)) return;
    state.timelineContentContextFeedbackInFlight.add(key);
    current.feedbackDirty = true;
    renderTimelineContentContextDrawer();
    try {
      const payload = verdict === "clear"
        ? await api(buildContentContextFeedbackUndoPath(match?.feedback?.id), { method: "POST" })
        : await api(buildTimelineContentContextFeedbackPath(timelineID), {
          method: "POST",
          body: { memoryItemId: memoryItemID, verdict },
        });
      const feedback = payload?.feedback;
      match.feedback = feedback?.verdict === "clear"
        ? null
        : { id: String(feedback?.id || ""), verdict: String(feedback?.verdict || "") };
    } catch (error) {
      showError(error);
    } finally {
      state.timelineContentContextFeedbackInFlight.delete(key);
      if (state.timelineContentContextActiveID === timelineID) renderTimelineContentContextDrawer();
    }
  }

  function renderTimelineContentContextMatch(match, timelineID) {
    const item = match?.item || {};
    const article = document.createElement("article");
    article.className = "timeline-content-context-match";
    const title = document.createElement("strong");
    const titleText = String(item.title || item.summary || item.canonicalPermalink || "Untitled local memory").trim();
    const sourceURL = safeSourceUrl(item.canonicalPermalink, item.source);
    if (sourceURL) {
      const link = document.createElement("a");
      link.href = sourceURL;
      link.target = "_blank";
      link.rel = "noopener noreferrer";
      link.textContent = titleText;
      title.append(link);
    } else {
      title.textContent = titleText;
    }
    const meta = document.createElement("small");
    const date = item.publishedAt || item.updatedAt;
    meta.textContent = [sourceLabel(item.source), date ? formatDate(date) : ""].filter(Boolean).join(" · ");
    const reason = document.createElement("span");
    reason.className = "timeline-content-context-reason";
    reason.textContent = String(match?.matchReason || "Matches a local lexical memory field");
    const feedback = document.createElement("div");
    feedback.className = "timeline-content-context-feedback";
    const verdict = String(match?.feedback?.verdict || "");
    article.classList.toggle("is-feedback-not-relevant", verdict === "not_relevant");
    article.classList.toggle("is-feedback-relevant", verdict === "relevant");
    const key = timelineContentContextFeedbackKey(timelineID, item.id);
    const busy = state.timelineContentContextFeedbackInFlight.has(key);
    if (verdict === "relevant" || verdict === "not_relevant") {
      const status = document.createElement("span");
      status.className = "timeline-content-context-feedback-status";
      status.setAttribute("role", "status");
      status.textContent = verdict === "relevant" ? "Marked relevant" : "Marked not relevant";
      const undo = document.createElement("button");
      undo.type = "button";
      undo.className = "text-button timeline-content-context-feedback-undo";
      undo.textContent = busy ? "Undoing…" : "Undo";
      undo.disabled = busy || !match?.feedback?.id;
      undo.addEventListener("click", () => void submitTimelineContentContextFeedback(timelineID, match, "clear"));
      feedback.append(status, undo);
    } else {
      for (const [label, value] of [["Relevant", "relevant"], ["Not relevant", "not_relevant"]]) {
        const button = document.createElement("button");
        button.type = "button";
        button.className = "feedback-button timeline-content-context-feedback-button";
        button.textContent = label;
        button.disabled = busy;
        button.setAttribute("aria-label", `Mark this related context as ${label.toLowerCase()}`);
        button.addEventListener("click", () => void submitTimelineContentContextFeedback(timelineID, match, value));
        feedback.append(button);
      }
    }
    article.append(title, meta, reason, feedback);
    return article;
  }

  function renderTimelineContentContextTopicInsight(insight) {
    const article = document.createElement("article");
    article.className = "timeline-content-context-topic-insight";
    const title = document.createElement("strong");
    title.textContent = insight?.topicName || "Living Topic";
    const overview = document.createElement("p");
    overview.textContent = insight?.overview || "No current overview.";
    const claims = document.createElement("ul");
    claims.className = "timeline-content-context-topic-claims";
    for (const claim of (insight?.claims || [])
      .filter((claim) => claim?.assessment === "supported" && claim?.temporalStatus === "current")
      .slice(0, 3)) {
      const item = document.createElement("li");
      item.textContent = claim.text;
      claims.append(item);
    }
    const meta = document.createElement("small");
    meta.textContent = [
      "Current supported understanding",
      `${insight?.evidenceCount || 0} active evidence`,
      `v${insight?.snapshotVersion || 1}`,
      insight?.evidenceAsOf ? `evidence through ${formatDate(insight.evidenceAsOf)}` : "evidence date unknown",
      insight?.updatedAt ? `regenerated ${formatDate(insight.updatedAt)}` : null,
    ].filter(Boolean).join(" · ");
    const reason = document.createElement("span");
    reason.className = "timeline-content-context-reason";
    reason.textContent = String(insight?.matchReason || "Matches a current Living Topic understanding");
    article.append(title, overview);
    if (claims.childElementCount) article.append(claims);
    article.append(meta, reason);
    return article;
  }

  function renderDirectContext(relation, source) {
    return renderTimelineDirectContext(relation, source, {
      documentRef: document, safeSourceUrl, configureNativePostLink, formatDate,
    });
  }

  function renderTimelineContentContextDrawer() {
    const body = $("#timeline-content-context-body");
    if (!body) return;
    const current = state.timelineContentContext.get(state.timelineContentContextActiveID)
      || { status: "idle", matches: [] };
    body.replaceChildren();
    body.setAttribute("role", current.status === "error" ? "alert" : "status");
    if (current.status === "loading") {
      body.textContent = "Loading captured and local context…";
      return;
    }
    if (current.status === "error") {
      body.textContent = current.message || "Related local context could not be loaded.";
      return;
    }
    const matches = Array.isArray(current.matches) ? current.matches.slice(0, CONTENT_CONTEXT_MAX_LIMIT) : [];
    const topicInsights = Array.isArray(current.topicInsights) ? current.topicInsights.slice(0, 2) : [];
    const directContext = Array.isArray(current.directContext) ? current.directContext.slice(0, 8) : [];
    if (!directContext.length && !matches.length && !topicInsights.length) {
      body.textContent = "No related local context found.";
      return;
    }
    if (directContext.length) {
      const heading = document.createElement("strong"); heading.textContent = "Conversation and interaction context";
      const list = document.createElement("div"); list.className = "timeline-content-context-list";
      list.append(...directContext.map((relation) => renderDirectContext(relation, current.source)));
      body.append(heading, list);
    }
    if (topicInsights.length) {
      const topicHeading = document.createElement("strong");
      topicHeading.textContent = "Current Living Topic understanding";
      const topicList = document.createElement("div");
      topicList.className = "timeline-content-context-topic-list";
      topicList.append(...topicInsights.map(renderTimelineContentContextTopicInsight));
      body.append(topicHeading, topicList);
    }
    if (matches.length) {
      const heading = document.createElement("strong");
      heading.textContent = "Related local context";
      const list = document.createElement("div");
      list.className = "timeline-content-context-list";
      list.append(...matches.map((match) => renderTimelineContentContextMatch(match, state.timelineContentContextActiveID)));
      body.append(heading, list);
    }
  }

  function contentContextDrawerMotionDuration() {
    return window.matchMedia("(prefers-reduced-motion: reduce)").matches ? 0 : 220;
  }

  function revealTimelineContentContextDrawer({ focus = true } = {}) {
    const drawer = $("#timeline-content-context-drawer");
    if (!drawer || !state.timelineContentContextActiveID) return;
    if (state.timelineContentContextCloseTimer !== null) {
      window.clearTimeout(state.timelineContentContextCloseTimer);
      state.timelineContentContextCloseTimer = null;
    }
    state.timelineContentContextDrawerOpen = true;
    drawer.classList.remove("hidden", "is-closing", "is-retracting");
    drawer.setAttribute("aria-hidden", "false");
    renderTimelineContentContextDrawer();
    syncTimelineContentContextTabs();
    scheduleTimelineContentContextPosition();
    scheduleBackToTop();
    window.requestAnimationFrame(() => {
      if (!state.timelineContentContextDrawerOpen) return;
      drawer.classList.add("is-open");
    });
    if (focus) $("#timeline-content-context-close")?.focus({ preventScroll: true });
  }

  function closeTimelineContentContextDrawer({ clearActive = true, focusTrigger = true } = {}) {
    const drawer = $("#timeline-content-context-drawer");
    const activeID = state.timelineContentContextActiveID;
    const activeContext = state.timelineContentContext.get(activeID);
    if (activeContext?.feedbackDirty) state.timelineContentContext.delete(activeID);
    state.timelineContentContextDrawerOpen = false;
    observeTimelineContentContextAnchor(null);
    state.timelineContentContextExpanded = false;
    $("#timeline-content-context-expand-actions")?.classList.add("hidden");
    $("#timeline-content-context-expand")?.setAttribute("aria-expanded", "false");
    if (clearActive) state.timelineContentContextActiveID = "";
    syncTimelineContentContextTabs();
    if (!drawer) {
      syncBackToTopNow();
      return;
    }
    drawer.setAttribute("aria-hidden", "true");
    drawer.classList.remove("is-open");
    drawer.classList.add("is-closing", "is-retracting");
    syncBackToTopNow();
    if (state.timelineContentContextCloseTimer !== null) {
      window.clearTimeout(state.timelineContentContextCloseTimer);
    }
    const finish = () => {
      state.timelineContentContextCloseTimer = null;
      if (state.timelineContentContextDrawerOpen) return;
      drawer.classList.add("hidden");
      drawer.classList.remove("is-closing", "is-retracting", "is-rail", "is-overlay", "is-sheet");
    };
    const duration = contentContextDrawerMotionDuration();
    if (duration === 0) finish();
    else state.timelineContentContextCloseTimer = window.setTimeout(finish, duration);
    if (focusTrigger) {
      const trigger = timelineContentContextTrigger(activeID);
      if (trigger && trigger.classList.contains("is-visible")) trigger.focus({ preventScroll: true });
      else $("#timeline-heading")?.focus({ preventScroll: true });
    }
  }

  function timelineContentContextFocusIsInsideDrawer() {
    const drawer = $("#timeline-content-context-drawer");
    return Boolean(drawer && drawer.contains(document.activeElement));
  }

  function openTimelineContentContext(entry, { focus = true } = {}) {
    if (!entry?.id || state.currentView !== "timeline") return;
    const previousID = state.timelineContentContextActiveID;
    if (previousID !== entry.id) state.timelineContentContextExpanded = false;
    if (previousID && previousID !== entry.id && state.timelineContentContext.get(previousID)?.feedbackDirty) {
      state.timelineContentContext.delete(previousID);
    }
    state.timelineContentContextActiveID = entry.id;
    const pending = beginTimelineContentContextLookup(state.timelineContentContext, entry.id);
    revealTimelineContentContextDrawer({ focus });
    void api(buildTimelineContentContextPath(entry.id)).then((payload) => {
      if (!settleTimelineContentContextLookup(state.timelineContentContext, entry.id, pending, {
        status: "success",
        matches: Array.isArray(payload?.matches) ? payload.matches : [],
        topicInsights: Array.isArray(payload?.topicInsights) ? payload.topicInsights : [],
        directContext: Array.isArray(payload?.directContext) ? payload.directContext : [],
        source: entry.source || entry.item?.source,
        feedbackDirty: false,
      })) return;
      if (state.timelineContentContextActiveID === entry.id) {
        renderTimelineContentContextDrawer();
        scheduleTimelineContentContextPosition();
      }
    }).catch((error) => {
      if (!settleTimelineContentContextLookup(state.timelineContentContext, entry.id, pending, {
        status: "error",
        message: error.message || String(error),
        matches: [],
        topicInsights: [],
        feedbackDirty: false,
      })) return;
      if (state.timelineContentContextActiveID === entry.id) {
        renderTimelineContentContextDrawer();
        scheduleTimelineContentContextPosition();
      }
    });
  }

  function toggleTimelineContentContext(entry) {
    if (state.timelineContentContextActiveID === entry?.id && state.timelineContentContextDrawerOpen) {
      closeTimelineContentContextDrawer();
      return;
    }
    openTimelineContentContext(entry);
  }

  function observeTimelineContentContextAnchor(anchor) {
    if (state.timelineContentContextObservedAnchor === anchor) return;
    getTimelineContentContextAnchorResizeObserver().disconnect();
    state.timelineContentContextObservedAnchor = anchor || null;
    if (anchor) getTimelineContentContextAnchorResizeObserver().observe(anchor);
  }

  function syncTimelineContentContextExpandAction({ defaultHeight, expandedHeight, hasExpandRoom }) {
    const drawer = $("#timeline-content-context-drawer");
    const actions = $("#timeline-content-context-expand-actions");
    const button = $("#timeline-content-context-expand");
    if (!drawer || !actions || !button) return;
    const actionHeight = actions.classList.contains("hidden") ? 0 : actions.offsetHeight;
    const contentHeight = Math.max(0, drawer.scrollHeight - actionHeight);
    const hasOverflow = contentHeight > defaultHeight + 1;
    const available = hasExpandRoom && hasOverflow && contentContextCanExpand({
      contentHeight,
      defaultHeight,
      expandedHeight,
    });
    if (!available) state.timelineContentContextExpanded = false;
    actions.classList.toggle("hidden", !available);
    button.textContent = state.timelineContentContextExpanded ? "Collapse context" : "Expand context";
    button.setAttribute("aria-expanded", String(state.timelineContentContextExpanded));
    drawer.style.setProperty(
      "--timeline-content-context-rail-height",
      `${Math.round(state.timelineContentContextExpanded && available ? expandedHeight : defaultHeight)}px`,
    );
  }

  function syncTimelineContentContextPosition() {
    const drawer = $("#timeline-content-context-drawer");
    const anchor = timelineContentContextAnchor(state.timelineContentContextActiveID);
    if (!drawer || !state.timelineContentContextDrawerOpen || state.currentView !== "timeline") return;
    if (!anchor) {
      closeTimelineContentContextDrawer({ clearActive: true, focusTrigger: false });
      return;
    }
    observeTimelineContentContextAnchor(anchor);
    const postRect = anchor.getBoundingClientRect();
    const viewportWidth = document.documentElement.clientWidth || window.innerWidth;
    const backToTop = $("#back-to-top");
    const backRect = backToTop && !backToTop.classList.contains("hidden") ? backToTop.getBoundingClientRect() : null;
    const viewportBottom = backRect ? Math.max(72, backRect.top - 16) : window.innerHeight - 16;
    const top = Math.max(16, Math.min(postRect.top, viewportBottom - 120));
    const safeRightBoundary = viewportWidth - 16;
    const rightGutter = Math.max(0, safeRightBoundary - postRect.right);
    const mode = contentContextDrawerMode({
      viewportWidth,
      rightGutter,
    });
    drawer.classList.remove("is-rail", "is-overlay", "is-sheet");
    drawer.classList.add(`is-${mode}`);
    if (mode !== "rail") {
      state.timelineContentContextExpanded = false;
      $("#timeline-content-context-expand-actions")?.classList.add("hidden");
      $("#timeline-content-context-expand")?.setAttribute("aria-expanded", "false");
    }
    drawer.style.removeProperty("--timeline-content-context-document-left");
    drawer.style.removeProperty("--timeline-content-context-document-top");
    drawer.style.removeProperty("--timeline-content-context-rail-height");
    drawer.style.setProperty("--timeline-content-context-top", `${Math.round(top)}px`);
    drawer.style.setProperty("--timeline-content-context-max-height", `${Math.max(80, Math.round(viewportBottom - top))}px`);
    if (mode === "sheet") {
      drawer.style.removeProperty("--timeline-content-context-right");
      drawer.style.removeProperty("--timeline-content-context-width");
      if (backRect) {
        drawer.style.setProperty(
          "--timeline-content-context-sheet-bottom",
          `${Math.round(Math.max(68, window.innerHeight - backRect.top + 16))}px`,
        );
      } else {
        drawer.style.removeProperty("--timeline-content-context-sheet-bottom");
      }
      syncBackToTopNow();
      return;
    }
    drawer.style.removeProperty("--timeline-content-context-sheet-bottom");
    const railPlacement = mode === "rail"
      ? contentContextRailPlacement({
        postRight: postRect.right,
        safeBoundary: safeRightBoundary,
        viewportWidth,
      })
      : null;
    const width = railPlacement?.width
      ?? Math.min(360, Math.max(240, viewportWidth - 32));
    const right = railPlacement?.right ?? 16;
    if (mode === "rail") {
      drawer.style.removeProperty("--timeline-content-context-right");
      drawer.style.setProperty("--timeline-content-context-document-left", `${window.scrollX + postRect.right}px`);
      drawer.style.setProperty("--timeline-content-context-document-top", `${window.scrollY + postRect.top}px`);
      const heightLimits = contentContextDrawerHeightLimits({
        postHeight: postRect.height,
        viewportAvailable: viewportBottom - postRect.top,
      });
      const targetHeight = state.timelineContentContextExpanded
        ? heightLimits?.expandedHeight
        : heightLimits?.defaultHeight;
      drawer.style.setProperty("--timeline-content-context-rail-height", `${Math.round(targetHeight || 120)}px`);
      drawer.style.setProperty("--timeline-content-context-width", `${width}px`);
      window.requestAnimationFrame(() => {
        if (!state.timelineContentContextDrawerOpen || !heightLimits) return;
        syncTimelineContentContextExpandAction(heightLimits);
      });
      syncBackToTopNow();
      return;
    }
    drawer.style.setProperty("--timeline-content-context-right", `${right}px`);
    drawer.style.setProperty("--timeline-content-context-width", `${width}px`);
    syncBackToTopNow();
  }

  function scheduleTimelineContentContextPosition() {
    if (state.timelineContentContextPositionFrame !== null) return;
    state.timelineContentContextPositionFrame = window.requestAnimationFrame(() => {
      state.timelineContentContextPositionFrame = null;
      syncTimelineContentContextPosition();
    });
  }

  function handleTimelineContentContextScroll() {
    const scrollY = window.scrollY;
    const previousScrollY = state.timelineContentContextLastScrollY;
    state.timelineContentContextLastScrollY = scrollY;
    if (state.timelineContentContextDrawerOpen && state.timelineContentContextActiveID) {
      const anchor = timelineContentContextAnchor(state.timelineContentContextActiveID);
      const postRect = anchor?.getBoundingClientRect();
      const passedReadingExitLine = postRect && contentContextPostPassedReadingExitLine({
        postBottom: postRect.bottom,
        viewportHeight: window.innerHeight,
      });
      if (!postRect || contentContextShouldCloseOnScroll({
        previousScrollY,
        scrollY,
        postTop: postRect.top,
        postBottom: postRect.bottom,
        viewportHeight: window.innerHeight,
        upScrollMode: state.bootstrap?.settings?.contentContextUpScrollMode || CONTENT_CONTEXT_UP_SCROLL_MODE_DEFAULT,
      }) || (scrollY > previousScrollY && passedReadingExitLine)) {
        closeTimelineContentContextDrawer({
          clearActive: true,
          focusTrigger: timelineContentContextFocusIsInsideDrawer(),
        });
        return;
      }
    }
    // The shared frame callback synchronizes tabs once via syncBackToTopNow.
  }

  function buildTimelineContentContextAnchor(entry, item) {
    const { anchor, tab } = buildTimelineContentContextTab(entry, () => toggleTimelineContentContext(entry), document);
    tab.dataset.timelineContentContextId = entry.id;
    anchor.prepend(item);
    return anchor;
  }

  return {
    timelineContentContextAnchor, timelineContentContextTrigger, measureTimelineContentContextTabs, syncTimelineContentContextTabs, timelineContentContextFeedbackKey, submitTimelineContentContextFeedback, renderTimelineContentContextMatch, renderTimelineContentContextTopicInsight, renderDirectContext, renderTimelineContentContextDrawer, contentContextDrawerMotionDuration, revealTimelineContentContextDrawer, closeTimelineContentContextDrawer, timelineContentContextFocusIsInsideDrawer, openTimelineContentContext, toggleTimelineContentContext, observeTimelineContentContextAnchor, syncTimelineContentContextExpandAction, syncTimelineContentContextPosition, scheduleTimelineContentContextPosition, handleTimelineContentContextScroll, buildTimelineContentContextAnchor
  };
}

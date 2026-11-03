import { LIBRARY_TAB_CLEANING, LIBRARY_TAB_LIBRARY, LIBRARY_TAB_SAVED, buildLibraryDonePath, buildLibraryForgetPath, buildLibraryKeepPath, buildLibraryReleasePath, buildLibraryRemovePath, buildLibraryRequestPath, buildLibraryStorageRequestPath, buildSavedLibraryRequestPath, formatLibraryStorageBytes, formatLibraryTier, libraryDoneConfirmation, libraryForgetConfirmation, libraryHasFullContent, libraryKeepConfirmation, libraryReleaseConfirmation, libraryRemoveConfirmation, mergeLibraryPage, normalizeLibraryFilters, normalizeLibraryStorageReport, normalizeLibraryTab } from "./library-state.js";

export function createLibraryUi({
  $, api, buildSourceIcon, configureNativePostLink, formatDate, safeMediaUrl, safeSourceUrl, sourceDescriptors, sourceLabel, state
}) {
  function setLibraryTab(tab, focus = false) {
    state.library.activeTab = normalizeLibraryTab(tab);
    if (state.currentView === "library") {
      resetLibraryList();
      void loadLibrary();
    }
    renderLibraryTabs();
    maybeLoadLibraryStorage();
    if (focus) {
      const activeTab = state.library.activeTab === LIBRARY_TAB_CLEANING
        ? $("#library-cleaning-tab")
        : state.library.activeTab === LIBRARY_TAB_LIBRARY
          ? $("#library-library-tab")
          : $("#library-saved-tab");
      activeTab?.focus();
    }
  }

  function renderLibraryTabs() {
    const activeTab = normalizeLibraryTab(state.library.activeTab);
    state.library.activeTab = activeTab;
    const cleaning = activeTab === LIBRARY_TAB_CLEANING;
    const tabs = [
      [$("#library-saved-tab"), activeTab === LIBRARY_TAB_SAVED],
      [$("#library-library-tab"), activeTab === LIBRARY_TAB_LIBRARY],
      [$("#library-cleaning-tab"), cleaning],
    ];
    for (const [tab, selected] of tabs) {
      if (!tab) continue;
      tab.setAttribute("aria-selected", String(selected));
      tab.tabIndex = selected ? 0 : -1;
      tab.classList.toggle("selected", selected);
    }
    const panels = [
      [$("#library-saved-view"), !cleaning],
      [$("#library-cleaning-view"), cleaning],
    ];
    for (const [panel, selected] of panels) {
      if (!panel) continue;
      panel.classList.toggle("hidden", !selected);
      panel.hidden = !selected;
    }
    $("#library-saved-view")?.setAttribute(
      "aria-labelledby",
      activeTab === LIBRARY_TAB_LIBRARY ? "library-library-tab" : "library-saved-tab",
    );
  }

  function maybeLoadLibraryStorage() {
    if (state.currentView !== "library" || state.library.activeTab !== LIBRARY_TAB_CLEANING) return;
    const storage = state.library.storage;
    if (!storage.loading && !storage.usage) void loadLibraryStorage();
  }

  function handleLibraryTabKeydown(event) {
    const keys = ["ArrowRight", "ArrowLeft", "Home", "End"];
    if (!keys.includes(event.key)) return;
    const tabs = [...document.querySelectorAll("#library-tabs [role=\"tab\"]")];
    const currentIndex = tabs.indexOf(event.currentTarget);
    if (currentIndex < 0 || !tabs.length) return;
    event.preventDefault();
    const nextIndex = event.key === "Home"
      ? 0
      : event.key === "End"
        ? tabs.length - 1
        : (currentIndex + (event.key === "ArrowRight" ? 1 : -1) + tabs.length) % tabs.length;
    const next = tabs[nextIndex];
    setLibraryTab(next.id === "library-cleaning-tab"
      ? LIBRARY_TAB_CLEANING
      : next.id === "library-library-tab" ? LIBRARY_TAB_LIBRARY : LIBRARY_TAB_SAVED, true);
  }

  function renderLibrarySourceOptions() {
    const select = $("#library-source");
    if (!select) return;
    const current = state.library.filters.source || select.value;
    select.replaceChildren(new Option("All sources", ""));
    for (const descriptor of sourceDescriptors()) {
      const option = new Option(descriptor.displayName, descriptor.id);
      select.append(option);
    }
    select.value = [...select.options].some((option) => option.value === current) ? current : "";
    syncLibraryFilterToggle();
  }

  function toggleLibraryFilters() {
    const toggle = $("#library-filters-toggle");
    const fields = $("#library-filter-fields");
    if (!toggle || !fields) return;
    const expanded = toggle.getAttribute("aria-expanded") !== "true";
    fields.classList.toggle("hidden", !expanded);
    toggle.setAttribute("aria-expanded", String(expanded));
    syncLibraryFilterToggle();
  }

  function syncLibraryFilterToggle() {
    const toggle = $("#library-filters-toggle");
    const fields = $("#library-filter-fields");
    if (!toggle || !fields) return;
    const activeCount = [
      $("#library-source")?.value,
      $("#library-tier")?.value,
      $("#library-published-from")?.value,
      $("#library-published-to")?.value,
    ].filter(Boolean).length;
    toggle.textContent = activeCount ? `Filters (${activeCount})` : "Filters";
    toggle.classList.toggle("has-active-filters", activeCount > 0);
    toggle.setAttribute("aria-expanded", String(!fields.classList.contains("hidden")));
  }

  function readLibraryFilters() {
    return normalizeLibraryFilters({
      query: $("#library-query").value,
      source: $("#library-source").value,
      tier: $("#library-tier").value,
      publishedFrom: $("#library-published-from").value,
      publishedTo: $("#library-published-to").value,
    });
  }

  function resetLibraryList() {
    state.library.controller?.abort();
    state.library.controller = null;
    state.library.requestID += 1;
    state.library.loading = false;
    state.library.loadingMore = false;
    state.library.items = [];
    state.library.topicKnowledge = [];
    state.library.nextCursor = "";
    state.library.error = null;
    state.library.detailRequestID += 1;
    state.library.detailLoading = false;
    state.library.detailError = null;
    state.library.mutationLoading = false;
    state.library.mutationError = null;
    state.library.mutationAction = "";
    state.library.mutationErrorAction = "";
    state.library.notice = "";
    state.library.selectedId = "";
    state.library.selectedItem = null;
    state.library.detailFocusTarget = null;
    renderLibraryDetail();
  }

  function resetLibraryStorage() {
    const storage = state.library.storage;
    storage.controller?.abort();
    storage.controller = null;
    storage.requestID += 1;
    storage.loading = false;
    storage.error = null;
    storage.usage = null;
    storage.recommendations = [];
    storage.savedPressure = null;
    storage.savedRecommendations = [];
  }

  function invalidateLibraryStorage() {
    resetLibraryStorage();
    maybeLoadLibraryStorage();
  }

  function reloadLibraryStorage() {
    invalidateLibraryStorage();
  }

  function submitLibrarySearch(event) {
    event?.preventDefault();
    state.library.filters = readLibraryFilters();
    resetLibraryList();
    void loadLibrary();
  }

  function clearLibrarySearch() {
    $("#library-query").value = "";
    $("#library-source").value = "";
    $("#library-tier").value = "";
    $("#library-published-from").value = "";
    $("#library-published-to").value = "";
    syncLibraryFilterToggle();
    submitLibrarySearch();
  }

  function refreshLibrary() {
    resetLibraryList();
    reloadLibraryStorage();
    void loadLibrary();
  }

  function loadMoreLibrary() {
    if (state.library.loading || !state.library.nextCursor) return;
    void loadLibrary({ append: true });
  }

  async function loadLibrary({ append = false } = {}) {
    if (append && (state.library.loading || !state.library.nextCursor)) return;
    if (!append && state.library.loading) state.library.controller?.abort();
    const requestID = state.library.requestID + 1;
    state.library.requestID = requestID;
    const controller = new AbortController();
    state.library.controller = controller;
    state.library.loading = true;
    state.library.loadingMore = append;
    state.library.error = null;
    renderLibrary();
    try {
      const requestPath = state.library.activeTab === LIBRARY_TAB_SAVED
        ? buildSavedLibraryRequestPath(state.library.filters, append ? state.library.nextCursor : "")
        : buildLibraryRequestPath(
          state.library.filters,
          append ? state.library.nextCursor : "",
          false,
          true,
        );
      const response = await api(requestPath, { signal: controller.signal });
      if (requestID !== state.library.requestID) return;
      const merged = mergeLibraryPage(state.library.items, response, append);
      state.library.items = merged.items;
      state.library.nextCursor = merged.nextCursor;
      if (!append) {
        state.library.topicKnowledge = state.library.activeTab === LIBRARY_TAB_LIBRARY && Array.isArray(response?.topicKnowledge)
          ? response.topicKnowledge.slice(0, 3)
          : [];
      }
      if (state.library.selectedId) {
        const selected = state.library.items.find((item) => item.id === state.library.selectedId);
        if (!selected) closeLibraryDetail();
      }
    } catch (error) {
      if (requestID !== state.library.requestID || error?.name === "AbortError") return;
      state.library.error = error;
    } finally {
      if (requestID !== state.library.requestID) return;
      state.library.loading = false;
      state.library.loadingMore = false;
      state.library.controller = null;
      renderLibrary();
    }
  }

  async function loadLibraryStorage() {
    const storage = state.library.storage;
    if (storage.loading) return;
    const requestID = storage.requestID + 1;
    storage.requestID = requestID;
    const controller = new AbortController();
    storage.controller = controller;
    storage.loading = true;
    storage.error = null;
    renderLibraryStorage();
    try {
      const response = await api(buildLibraryStorageRequestPath(), { signal: controller.signal });
      if (requestID !== storage.requestID) return;
      const report = normalizeLibraryStorageReport(response);
      storage.usage = report.usage;
      storage.recommendations = report.recommendations;
      storage.savedPressure = report.savedPressure;
      storage.savedRecommendations = report.savedRecommendations;
    } catch (error) {
      if (requestID !== storage.requestID || error?.name === "AbortError") return;
      storage.error = error;
    } finally {
      if (requestID !== storage.requestID) return;
      storage.loading = false;
      storage.controller = null;
      renderLibraryStorage();
    }
  }

  function libraryErrorMessage(error) {
    if (error?.code === "sidecar_unavailable" || error?.name === "SidecarUnavailableError") {
      return "AkuSidecar is offline. Start the local runtime, then refresh Library.";
    }
    if (error?.code === "invalid_memory_library_query") {
      return "Those Library filters are not valid. Clear the filters and try again.";
    }
      return state.library.activeTab === LIBRARY_TAB_SAVED
        ? "Saved could not be loaded. Refresh and try again."
        : "Library could not be loaded. Refresh and try again.";
  }

  function libraryStorageErrorMessage(error) {
    if (error?.code === "sidecar_unavailable" || error?.name === "SidecarUnavailableError") {
      return "Local storage summary is unavailable while AkuSidecar is offline.";
    }
    return "Local storage summary could not be loaded. Library search still works; refresh to try again.";
  }

  function libraryMutationErrorMessage(error, action) {
    if (error?.code === "sidecar_unavailable" || error?.name === "SidecarUnavailableError") {
      return "AkuSidecar is offline. Start the local runtime, then try this Library action again.";
    }
    if (action === "forget") return "Library could not forget this memory permanently. Try again.";
    if (action === "release") return "Library could not release the full copy. Try again.";
    if (action === "keep") return "This item could not be kept in Library. A local full copy may be unavailable.";
    if (action === "done") return "This Saved item could not be marked Done. Try again.";
    return "Library could not remove this local memory. Try again.";
  }

  function renderLibrary() {
    renderLibraryTabs();
    renderLibraryStorage();
    const results = $("#library-results");
    const loadMore = $("#library-load-more");
    const status = $("#library-status");
    const meta = $("#library-meta");
    const layout = $("#library-layout");
    if (!results || !loadMore || !status || !meta) return;
    renderLibraryTopicKnowledge();

    layout?.classList.toggle("has-selection", Boolean(state.library.selectedId));

    results.replaceChildren();
    status.className = "library-status";
    if (state.library.loading && !state.library.loadingMore) {
      status.textContent = "Loading local memories…";
    } else if (state.library.loadingMore) {
      status.textContent = "Loading more local memories…";
    } else if (state.library.error) {
      status.classList.add("library-status-error");
      status.textContent = libraryErrorMessage(state.library.error);
    } else if (state.library.notice) {
      status.textContent = state.library.notice;
    } else if (state.library.items.length || state.library.topicKnowledge.length) {
      const parts = [];
      if (state.library.topicKnowledge.length) parts.push(`${state.library.topicKnowledge.length} topic understanding${state.library.topicKnowledge.length === 1 ? "" : "s"}`);
      if (state.library.items.length) parts.push(`${state.library.items.length} memor${state.library.items.length === 1 ? "y" : "ies"}${state.library.nextCursor ? " · more available" : ""}`);
      status.textContent = parts.join(" · ");
    } else {
      status.textContent = "";
    }
    meta.textContent = state.library.error && !state.library.items.length
      ? "This local view is read-only. No provider was contacted."
      : state.library.activeTab === LIBRARY_TAB_SAVED
        ? "Items currently Saved for later. Saved reads never call a provider."
        : "Search local memories and current Living Topic knowledge. Library reads never call a provider.";

    if (state.library.loading && !state.library.items.length) {
      results.append(libraryStateMessage("Loading local memories…"));
    } else if (state.library.error && !state.library.items.length) {
      results.append(libraryStateMessage(libraryErrorMessage(state.library.error), true));
    } else if (!state.library.items.length) {
      results.append(libraryStateMessage(state.library.activeTab === LIBRARY_TAB_SAVED
        ? "Nothing is currently Saved for later. Choose Read later on a Timeline item to add one."
        : state.library.topicKnowledge.length
          ? "No individual Memory items match these filters. Current Living Topic knowledge is shown above."
          : "No local memories or current topic knowledge match these filters. Try a broader search or run a Timeline check first."));
    } else {
      results.append(...state.library.items.map(buildLibraryCard));
    }
    loadMore.classList.toggle("hidden", !state.library.nextCursor);
    loadMore.disabled = state.library.loading;
    loadMore.textContent = state.library.loadingMore ? "Loading…" : "Load more";
  }

  function renderLibraryTopicKnowledge() {
    const section = $("#library-topic-knowledge");
    const results = $("#library-topic-knowledge-results");
    if (!section || !results) return;
    const visible = state.library.activeTab === LIBRARY_TAB_LIBRARY
      && Boolean(state.library.filters.query)
      && state.library.topicKnowledge.length > 0;
    section.classList.toggle("hidden", !visible);
    if (!visible) {
      results.replaceChildren();
      return;
    }
    results.replaceChildren(...state.library.topicKnowledge.map(buildLibraryTopicKnowledgeCard));
  }

  function buildLibraryTopicKnowledgeCard(knowledge) {
    const card = document.createElement("article");
    card.className = "library-topic-knowledge-card";
    card.setAttribute("role", "listitem");
    const header = document.createElement("header");
    const title = document.createElement("strong");
    title.textContent = knowledge?.topicName || "Living Topic";
    const current = document.createElement("span");
    current.textContent = "Current supported";
    header.append(title, current);
    const overview = document.createElement("p");
    overview.textContent = knowledge?.overview || "Current supported understanding.";
    const claims = document.createElement("ul");
    const topicClaims = (Array.isArray(knowledge?.claims) ? knowledge.claims : [])
      .filter((claim) => claim?.assessment === "supported" && claim?.temporalStatus === "current")
      .slice(0, 3);
    for (const claim of topicClaims) {
      const item = document.createElement("li");
      item.textContent = claim?.text || "";
      if (item.textContent) claims.append(item);
    }
    const meta = document.createElement("small");
    meta.className = "library-topic-knowledge-meta";
    meta.textContent = [
      `${Number(knowledge?.evidenceCount || 0)} active evidence`,
      knowledge?.snapshotVersion ? `snapshot ${knowledge.snapshotVersion}` : null,
      knowledge?.evidenceAsOf ? `evidence through ${formatDate(knowledge.evidenceAsOf)}` : "evidence date unknown",
      knowledge?.updatedAt ? `regenerated ${formatDate(knowledge.updatedAt)}` : null,
    ].filter(Boolean).join(" · ");
    const reason = document.createElement("small");
    reason.className = "library-topic-knowledge-reason";
    reason.textContent = knowledge?.matchReason || "Matched current Living Topic understanding.";
    card.append(header, overview);
    if (claims.children.length) card.append(claims);
    card.append(meta, reason);
    return card;
  }

  function renderLibraryStorage() {
    const summary = $("#library-storage-summary");
    const recommendations = $("#library-storage-recommendations");
    if (!summary || !recommendations) return;
    const savedSummary = $("#library-saved-pressure-summary");
    const savedRecommendations = $("#library-saved-recommendations");
    const storage = state.library.storage;
    summary.replaceChildren();
    recommendations.replaceChildren();
    savedSummary?.replaceChildren();
    savedRecommendations?.replaceChildren();
    if (storage.loading && !storage.usage) {
      summary.append(libraryStateMessage("Loading local storage summary…"));
    } else if (storage.error && !storage.usage) {
      const error = libraryStateMessage(libraryStorageErrorMessage(storage.error), true);
      summary.append(error);
    } else if (storage.usage) {
      summary.append(buildLibraryStorageUsage(storage.usage));
    } else {
      summary.append(libraryStateMessage("Local storage summary is not available yet."));
    }
    if (storage.error && storage.usage) {
      const error = document.createElement("p");
      error.className = "library-storage-error";
      error.setAttribute("role", "alert");
      error.textContent = libraryStorageErrorMessage(storage.error);
      summary.append(error);
    }
    if (storage.loading && storage.usage) {
      const loading = document.createElement("p");
      loading.className = "library-storage-loading";
      loading.textContent = "Refreshing local storage summary…";
      summary.append(loading);
    }
    if (!storage.recommendations.length) {
      const empty = document.createElement("p");
      empty.className = "library-storage-empty";
      empty.textContent = storage.usage
        ? "No full copies with positive content bytes are queued for review."
        : "Recommendations will appear when the local storage summary loads.";
      recommendations.append(empty);
    } else {
      recommendations.append(...storage.recommendations.map(buildLibraryStorageRecommendation));
    }
    if (savedSummary && savedRecommendations) {
      if (storage.loading && !storage.savedPressure) {
        savedSummary.append(libraryStateMessage("Loading Saved backlog snapshot…"));
      } else if (storage.error && !storage.savedPressure) {
        savedSummary.append(libraryStateMessage("Saved backlog snapshot is unavailable while storage summary is offline.", true));
      } else if (storage.savedPressure) {
        savedSummary.append(buildLibrarySavedPressure(storage.savedPressure));
      } else {
        savedSummary.append(libraryStateMessage("Saved backlog snapshot is not available yet."));
      }
      if (!storage.savedRecommendations.length) {
        const empty = document.createElement("p");
        empty.className = "library-storage-empty";
        empty.textContent = storage.savedPressure
          ? "No active Saved items need review."
          : "Saved review recommendations will appear when the backlog snapshot loads.";
        savedRecommendations.append(empty);
      } else {
        savedRecommendations.append(...storage.savedRecommendations.map(buildLibrarySavedRecommendation));
      }
    }
  }

  function buildLibrarySavedPressure(pressure) {
    const fragment = document.createDocumentFragment();
    const metrics = document.createElement("div");
    metrics.className = "library-saved-pressure-breakdown";
    for (const [label, value] of [
      ["Saved items", formatLibraryStorageCount(pressure.activeItems)],
      ["Local-copy Saved", formatLibraryStorageCount(pressure.localCopyItems)],
      ["Source-dependent Saved", formatLibraryStorageCount(pressure.sourceDependentItems)],
      ["Saved content", formatLibraryStorageBytes(pressure.contentBytes)],
    ]) {
      metrics.append(libraryStorageMetric(label, value));
    }
    const oldest = document.createElement("p");
    oldest.className = "library-storage-counts";
    const oldestAt = String(pressure.oldestClaimedAt ?? "").trim();
    oldest.textContent = oldestAt
      ? `Oldest Saved: ${formatDate(oldestAt) || oldestAt}`
      : "Oldest Saved: none";
    fragment.append(metrics, oldest);
    return fragment;
  }

  function buildLibraryStorageUsage(usage) {
    const fragment = document.createDocumentFragment();
    const total = document.createElement("div");
    total.className = "library-storage-total";
    const totalLabel = document.createElement("span");
    totalLabel.textContent = "Logical storage";
    const totalValue = document.createElement("strong");
    totalValue.textContent = formatLibraryStorageBytes(usage.logicalBytes);
    total.append(totalLabel, totalValue);
    const note = document.createElement("p");
    note.className = "library-storage-note";
    note.textContent = "Estimate of local content and metadata; this is not the physical SQLite or WAL file size. Nothing is removed automatically.";
    const breakdown = document.createElement("div");
    breakdown.className = "library-storage-breakdown";
    for (const [label, value] of [
      ["Content", usage.contentBytes],
      ["Metadata", usage.metadataBytes],
      ["Provenance", usage.provenanceBytes],
      ["Actions", usage.actionBytes],
    ]) {
      breakdown.append(libraryStorageMetric(label, formatLibraryStorageBytes(value)));
    }
    const counts = document.createElement("p");
    counts.className = "library-storage-counts";
    counts.textContent = `Items: ${formatLibraryStorageCount(usage.activeItems)} active · ${formatLibraryStorageCount(usage.recallItems)} recall stubs · ${formatLibraryStorageCount(usage.fullCopyItems)} full copies · ${formatLibraryStorageCount(usage.tombstones)} tombstones`;
    fragment.append(total, note, breakdown, counts);
    return fragment;
  }

  function formatLibraryStorageCount(value) {
    const count = Number(value);
    return Number.isInteger(count) && count >= 0 ? String(count) : "unknown";
  }

  function libraryStorageMetric(label, value) {
    const metric = document.createElement("span");
    metric.className = "library-storage-metric";
    const metricLabel = document.createElement("small");
    metricLabel.textContent = label;
    const metricValue = document.createElement("strong");
    metricValue.textContent = value;
    metric.append(metricLabel, metricValue);
    return metric;
  }

  function buildLibraryStorageRecommendation(recommendation) {
    const article = document.createElement("article");
    article.className = "library-storage-recommendation";
    article.setAttribute("role", "listitem");
    const identity = document.createElement("div");
    identity.className = "library-storage-recommendation-identity";
    const title = document.createElement("strong");
    title.textContent = recommendation.title || "Untitled memory";
    const source = document.createElement("span");
    source.textContent = sourceLabel(recommendation.source);
    identity.append(title, source);
    const detail = document.createElement("p");
    detail.textContent = `Full copy · ${formatLibraryStorageBytes(recommendation.reclaimableBytes)} reclaimable content · updated ${formatDate(recommendation.updatedAt) || recommendation.updatedAt || "unknown"}`;
    const reason = document.createElement("p");
    reason.className = "library-storage-recommendation-reason";
    reason.textContent = "Largest retained full-copy payload. Review it before deciding what to keep.";
    const review = document.createElement("button");
    review.type = "button";
    review.className = "secondary-button library-storage-review";
    review.dataset.reviewAction = recommendation.reviewAction || "review_full_copy";
    review.textContent = "Review";
    review.addEventListener("click", () => reviewLibraryStorageRecommendation(recommendation));
    article.append(identity, detail, reason, review);
    return article;
  }

  function reviewLibraryStorageRecommendation(recommendation) {
    setLibraryTab(LIBRARY_TAB_LIBRARY);
    selectLibraryItem(recommendation.id, {
      id: recommendation.id,
      source: recommendation.source,
      title: recommendation.title,
      author: recommendation.author,
      retentionTier: "full_copy",
      updatedAt: recommendation.updatedAt,
    });
    window.requestAnimationFrame(() => $("#library-detail-heading")?.focus());
  }

  function buildLibrarySavedRecommendation(recommendation) {
    const article = document.createElement("article");
    article.className = "library-saved-recommendation";
    article.setAttribute("role", "listitem");
    const identity = document.createElement("div");
    identity.className = "library-storage-recommendation-identity";
    const title = document.createElement("strong");
    title.textContent = recommendation.title || "Untitled memory";
    const source = document.createElement("span");
    source.textContent = sourceLabel(recommendation.source);
    identity.append(title, source);
    const detail = document.createElement("p");
    const savedAt = String(recommendation.savedAt ?? "").trim();
    const tier = recommendation.sourceDependent ? "Source-dependent Saved" : "Local-copy Saved";
    detail.textContent = `${tier} · Saved ${formatDate(savedAt) || savedAt || "unknown"} · ${formatLibraryStorageBytes(recommendation.contentBytes)} retained`;
    const reason = document.createElement("p");
    reason.className = "library-storage-recommendation-reason";
    reason.textContent = "Oldest active Saved item. Review it in Saved.";
    const review = document.createElement("button");
    review.type = "button";
    review.className = "secondary-button library-storage-review";
    review.dataset.reviewAction = recommendation.reviewAction || "review_saved";
    review.textContent = "Review";
    review.addEventListener("click", () => reviewSavedRecommendation(recommendation));
    article.append(identity, detail, reason, review);
    return article;
  }

  function reviewSavedRecommendation(recommendation) {
    setLibraryTab(LIBRARY_TAB_SAVED);
    selectLibraryItem(recommendation.id, {
      id: recommendation.id,
      source: recommendation.source,
      title: recommendation.title,
      author: recommendation.author,
      saved: true,
      retentionTier: recommendation.retentionTier,
      permanentKeep: false,
      updatedAt: recommendation.savedAt,
    });
    window.requestAnimationFrame(() => $("#library-detail-heading")?.focus());
  }

  function libraryStateMessage(message, error = false) {
    const element = document.createElement("p");
    element.className = `library-state${error ? " library-state-error" : ""}`;
    if (error) element.setAttribute("role", "alert");
    element.textContent = message;
    return element;
  }

  function buildLibraryCard(item) {
    const article = document.createElement("article");
    article.className = "library-card-shell";
    article.setAttribute("role", "listitem");
    const button = document.createElement("button");
    button.type = "button";
    button.className = "library-card";
    button.classList.toggle("selected", state.library.selectedId === item.id);
    button.setAttribute("aria-pressed", String(state.library.selectedId === item.id));
    button.setAttribute("aria-label", `Open memory: ${item.title || "Untitled memory"}`);
    button.addEventListener("click", () => selectLibraryItem(item.id));

    const header = document.createElement("div");
    header.className = "library-card-header";
    header.append(buildSourceIcon(item.source));
    const identity = document.createElement("span");
    identity.className = "library-card-identity";
    const title = document.createElement("strong");
    title.textContent = item.title || "Untitled memory";
    const source = document.createElement("span");
    source.textContent = sourceLabel(item.source);
    identity.append(title, source);
    const tier = document.createElement("span");
    tier.className = "library-tier-badge";
    tier.textContent = formatLibraryTier(item.retentionTier);
    header.append(identity, tier);

    const summary = document.createElement("p");
    summary.className = "library-card-summary";
    summary.textContent = item.summary || "No summary was captured for this recall stub.";

    const metadata = document.createElement("div");
    metadata.className = "library-card-metadata";
    if (item.author) metadata.append(libraryMetaChip("Author", item.author));
    const date = formatDate(item.publishedAt || item.updatedAt);
    if (date) metadata.append(libraryMetaChip("Published", date));
    if (Array.isArray(item.tags)) {
      for (const tag of item.tags.slice(0, 6)) metadata.append(libraryMetaChip("Tag", tag));
    }
    button.append(header, summary, metadata);
    article.append(button);
    return article;
  }

  function libraryMetaChip(label, value) {
    const chip = document.createElement("span");
    chip.className = "library-meta-chip";
    chip.textContent = `${label}: ${value}`;
    return chip;
  }

  function selectLibraryItem(id, fallbackItem = null) {
    const item = state.library.items.find((entry) => entry.id === id)
      || (fallbackItem?.id === id ? fallbackItem : null);
    if (!item) return;
    if (document.activeElement?.closest?.(".library-card")) state.library.detailFocusTarget = document.activeElement;
    state.library.selectedId = id;
    state.library.selectedItem = item;
    state.library.detailRequestID += 1;
    const detailRequestID = state.library.detailRequestID;
    state.library.detailLoading = true;
    state.library.detailError = null;
    renderLibrary();
    renderLibraryDetail();
    void (async () => {
      try {
        const response = await api(`/api/library/items/${encodeURIComponent(id)}`);
        if (detailRequestID !== state.library.detailRequestID || state.library.selectedId !== id) return;
        if (!response?.item || response.item.id !== id) throw new Error("Library detail was unavailable.");
        state.library.selectedItem = response.item;
      } catch (error) {
        if (detailRequestID !== state.library.detailRequestID || state.library.selectedId !== id) return;
        state.library.detailError = error;
      } finally {
        if (detailRequestID !== state.library.detailRequestID || state.library.selectedId !== id) return;
        state.library.detailLoading = false;
        renderLibraryDetail();
      }
    })();
  }

  function closeLibraryDetail() {
    const target = state.library.detailFocusTarget;
    state.library.detailRequestID += 1;
    state.library.selectedId = "";
    state.library.selectedItem = null;
    state.library.detailLoading = false;
    state.library.detailError = null;
    state.library.detailFocusTarget = null;
    renderLibraryDetail();
    target?.focus?.();
    renderLibrary();
  }

  function renderLibraryDetail() {
    const detail = $("#library-detail");
    const heading = $("#library-detail-heading");
    const content = $("#library-detail-content");
    if (!detail || !heading || !content) return;
    const item = state.library.selectedItem;
    detail.classList.toggle("hidden", !item || !state.library.selectedId);
    detail.setAttribute("aria-busy", String(Boolean(state.library.mutationLoading)));
    if (!item || !state.library.selectedId) {
      heading.textContent = "Choose a memory";
      content.replaceChildren();
      return;
    }
    heading.textContent = item.title || "Untitled memory";
    content.replaceChildren();
    if (state.library.detailLoading) {
      content.append(libraryStateMessage("Loading memory details…"));
      return;
    }
    if (state.library.detailError) {
      content.append(libraryStateMessage("This memory detail could not be loaded. Try selecting it again.", true));
      const retry = document.createElement("button");
      retry.type = "button";
      retry.className = "secondary-button library-detail-retry";
      retry.textContent = "Retry details";
      retry.addEventListener("click", () => selectLibraryItem(item.id));
      content.append(retry);
      return;
    }
    content.append(buildLibraryDetail(item));
  }

  function buildLibraryDetail(item) {
    const fragment = document.createDocumentFragment();
    const metadata = document.createElement("dl");
    metadata.className = "library-detail-metadata";
    appendLibraryDetailField(metadata, "Source", sourceLabel(item.source));
    appendLibraryDetailField(metadata, "Tier", formatLibraryTier(item.retentionTier));
    if (item.author) appendLibraryDetailField(metadata, "Author", item.author);
    if (item.publishedAt) appendLibraryDetailField(metadata, "Published", formatDate(item.publishedAt) || item.publishedAt);
    if (item.updatedAt) appendLibraryDetailField(metadata, "Updated", formatDate(item.updatedAt) || item.updatedAt);
    fragment.append(metadata);

    if (item.summary) {
      const summary = document.createElement("p");
      summary.className = "library-detail-summary";
      summary.textContent = item.summary;
      fragment.append(summary);
    }

    const sourceURL = safeSourceUrl(item.canonicalPermalink, item.source);
    if (sourceURL) {
      const source = document.createElement("p");
      source.className = "library-detail-source";
      const link = document.createElement("a");
      link.className = "source-link";
      configureNativePostLink(link, sourceURL, item.source);
      link.textContent = "Open source entry";
      source.append(link);
      fragment.append(source);
    }

    appendLibraryLabels(fragment, "Tags", item.tags);
    appendLibraryLabels(fragment, "Facets", item.facets);
    appendLibraryMediaMetadata(fragment, item.media);

    if (item.saved) {
      const savedActions = document.createElement("section");
      savedActions.className = "library-saved-actions";
      const savedHeading = document.createElement("h4");
      savedHeading.textContent = "Saved for later";
      const savedNote = document.createElement("p");
      savedNote.className = "library-detail-action-note";
      savedNote.textContent = "Choose what happens after reading: keep the local copy in Library, or mark it Done.";
      const keep = document.createElement("button");
      keep.type = "button";
      keep.className = "primary-button library-detail-keep";
      keep.textContent = state.library.mutationLoading && state.library.mutationAction === "keep"
        ? "Keeping…" : "Keep in Library";
      keep.disabled = state.library.mutationLoading;
      keep.addEventListener("click", () => keepSavedItem(item));
      const done = document.createElement("button");
      done.type = "button";
      done.className = "secondary-button library-detail-done";
      done.textContent = state.library.mutationLoading && state.library.mutationAction === "done"
        ? "Finishing…" : "Done";
      done.disabled = state.library.mutationLoading;
      done.addEventListener("click", () => doneSavedItem(item));
      savedActions.append(savedHeading, savedNote, keep, done);
      fragment.append(savedActions);
    }

    const fullCopy = document.createElement("section");
    fullCopy.className = "library-detail-copy";
    const copyHeading = document.createElement("h4");
    copyHeading.textContent = libraryHasFullContent(item) ? "Kept full text" : "Recall stub only";
    fullCopy.append(copyHeading);
    const copy = document.createElement("pre");
    copy.className = "library-detail-full-content";
    copy.textContent = libraryHasFullContent(item)
      ? item.fullContent
      : "Only the bounded recall metadata is stored for this item.";
    fullCopy.append(copy);
    fragment.append(fullCopy);

    const actions = document.createElement("section");
    actions.className = "library-detail-actions";
    const actionHeading = document.createElement("h4");
    actionHeading.textContent = "Memory actions";
    const releaseNote = document.createElement("p");
    releaseNote.className = "library-detail-action-note";
    releaseNote.id = "library-release-note";
    releaseNote.textContent = item.saved
      ? "Choose Keep in Library or Done before changing the stored copy for this Saved item."
      : "Release removes only the stored full text. The searchable recall metadata and identity stay in Library.";
    const release = document.createElement("button");
    release.type = "button";
    release.className = "secondary-button library-detail-release";
    release.textContent = state.library.mutationLoading && state.library.mutationAction === "release" ? "Releasing…" : "Release full copy";
    release.disabled = state.library.mutationLoading || item.retentionTier !== "full_copy" || item.saved;
    release.setAttribute("aria-describedby", releaseNote.id);
    release.addEventListener("click", () => releaseLibraryItem(item));
    const removeNote = document.createElement("p");
    removeNote.className = "library-detail-action-note";
    removeNote.id = "library-remove-note";
    removeNote.textContent = "Remove deletes only this local Library copy. A later More may add the source item again.";
    const remove = document.createElement("button");
    remove.type = "button";
    remove.className = "secondary-button";
    remove.textContent = state.library.mutationLoading && state.library.mutationAction === "remove" ? "Removing…" : "Remove from Library";
    remove.disabled = state.library.mutationLoading;
    remove.setAttribute("aria-describedby", removeNote.id);
    remove.addEventListener("click", () => removeLibraryItem(item));
    const forgetNote = document.createElement("p");
    forgetNote.className = "library-detail-action-note library-detail-action-note-danger";
    forgetNote.id = "library-forget-note";
    forgetNote.textContent = "Forget permanently also blocks automatic recapture of the same source item until a reset.";
    const forget = document.createElement("button");
    forget.type = "button";
    forget.className = "danger-button library-detail-forget";
    forget.textContent = state.library.mutationLoading && state.library.mutationAction === "forget" ? "Forgetting…" : "Forget permanently";
    forget.disabled = state.library.mutationLoading;
    forget.setAttribute("aria-describedby", forgetNote.id);
    forget.addEventListener("click", () => forgetLibraryItem(item));
    actions.append(actionHeading);
    if (item.retentionTier === "full_copy") actions.append(releaseNote, release);
    actions.append(removeNote, remove, forgetNote, forget);
    if (state.library.mutationError) {
      const error = document.createElement("p");
      error.className = "library-detail-action-error";
      error.setAttribute("role", "alert");
      error.textContent = libraryMutationErrorMessage(state.library.mutationError, state.library.mutationErrorAction);
      actions.append(error);
    }
    fragment.append(actions);
    return fragment;
  }

  function removeLibraryItem(item) {
    return mutateLibraryItem(item, "remove");
  }

  function releaseLibraryItem(item) {
    return mutateLibraryItem(item, "release");
  }

  function forgetLibraryItem(item) {
    return mutateLibraryItem(item, "forget");
  }

  function keepSavedItem(item) {
    return mutateLibraryItem(item, "keep");
  }

  function doneSavedItem(item) {
    return mutateLibraryItem(item, "done");
  }

  async function mutateLibraryItem(item, action) {
    if (!item?.id || state.library.mutationLoading) return;
    const config = action === "keep"
      ? {
        method: "POST",
        path: buildLibraryKeepPath,
        confirm: libraryKeepConfirmation,
        acknowledged: (response) => response?.kept === true && response?.saved === false && response?.permanentKeep === true,
        failure: "Library did not confirm keeping this item.",
        notice: "Kept in Library. It is no longer in Saved.",
      }
      : action === "done"
        ? {
          method: "POST",
          path: buildLibraryDonePath,
          confirm: libraryDoneConfirmation,
          acknowledged: (response) => response?.done === true && response?.saved === false,
          failure: "Library did not confirm that this item is Done.",
          notice: "Done. The item is no longer in Saved.",
        }
      : action === "forget"
      ? {
        method: "POST",
        path: buildLibraryForgetPath,
        confirm: libraryForgetConfirmation,
        acknowledged: (response) => response?.forgotten === true,
        failure: "Library did not confirm the permanent forget.",
        notice: "Memory forgotten permanently. Automatic recapture for this source item is suppressed.",
      }
      : action === "release"
        ? {
          method: "POST",
          path: buildLibraryReleasePath,
          confirm: libraryReleaseConfirmation,
          acknowledged: (response) => response?.released === true && response?.retentionTier === "recall",
          failure: "Library did not confirm the full-copy release.",
          notice: "Full copy released. Recall metadata remains in Library.",
        }
      : {
        method: "DELETE",
        path: buildLibraryRemovePath,
        confirm: libraryRemoveConfirmation,
        acknowledged: (response) => response?.removed === true,
        failure: "Library did not confirm the local removal.",
        notice: "Memory removed locally. A future More can add it again.",
      };
    if (config.confirm && !window.confirm(config.confirm(item))) return;
    state.library.mutationLoading = true;
    state.library.mutationAction = action;
    state.library.mutationError = null;
    state.library.mutationErrorAction = action;
    state.library.notice = "";
    renderLibraryDetail();
    try {
      const response = await api(config.path(item.id), { method: config.method });
      if (!config.acknowledged(response) || response.id !== item.id) throw new Error(config.failure);
      state.library.notice = config.notice;
      if (action === "release") {
        state.library.items = state.library.items.map((entry) => entry.id === item.id
          ? { ...entry, retentionTier: response.retentionTier, permanentKeep: false }
          : entry);
        if (state.library.selectedItem?.id === item.id) {
          const { fullContent: _releasedContent, ...withoutFullContent } = state.library.selectedItem;
          state.library.selectedItem = { ...withoutFullContent, retentionTier: response.retentionTier, permanentKeep: false };
        }
      } else if (action === "keep" || action === "done") {
        if (state.library.activeTab === LIBRARY_TAB_SAVED) {
          state.library.items = state.library.items.filter((entry) => entry.id !== item.id);
          state.library.selectedId = "";
          state.library.selectedItem = null;
          state.library.detailRequestID += 1;
          state.library.detailLoading = false;
          state.library.detailError = null;
        } else {
          state.library.items = state.library.items.map((entry) => entry.id === item.id
            ? { ...entry, saved: false, permanentKeep: response.permanentKeep, retentionTier: response.retentionTier }
            : entry);
          if (state.library.selectedItem?.id === item.id) {
            const { fullContent: _staleContent, ...withoutFullContent } = state.library.selectedItem;
            state.library.selectedItem = {
              ...withoutFullContent,
              saved: false,
              permanentKeep: response.permanentKeep,
              retentionTier: response.retentionTier,
            };
            if (action === "keep" && libraryHasFullContent(item)) state.library.selectedItem.fullContent = item.fullContent;
          }
        }
      } else {
        state.library.items = state.library.items.filter((entry) => entry.id !== item.id);
        state.library.selectedId = "";
        state.library.selectedItem = null;
        state.library.detailRequestID += 1;
        state.library.detailLoading = false;
        state.library.detailError = null;
      }
      reloadLibraryStorage();
    } catch (error) {
      state.library.mutationError = error;
    } finally {
      state.library.mutationLoading = false;
      state.library.mutationAction = "";
      renderLibraryDetail();
      renderLibrary();
    }
  }

  function appendLibraryDetailField(list, label, value) {
    const wrapper = document.createElement("div");
    const term = document.createElement("dt");
    term.textContent = label;
    const description = document.createElement("dd");
    description.textContent = value;
    wrapper.append(term, description);
    list.append(wrapper);
  }

  function appendLibraryLabels(fragment, label, values) {
    if (!Array.isArray(values) || !values.length) return;
    const section = document.createElement("section");
    section.className = "library-detail-labels";
    const heading = document.createElement("h4");
    heading.textContent = label;
    const list = document.createElement("div");
    list.className = "library-label-list";
    for (const value of values.slice(0, 12)) {
      const chip = document.createElement("span");
      chip.textContent = String(value);
      list.append(chip);
    }
    section.append(heading, list);
    fragment.append(section);
  }

  function appendLibraryMediaMetadata(fragment, values) {
    if (!Array.isArray(values) || !values.length) return;
    const section = document.createElement("section");
    section.className = "library-detail-media";
    const heading = document.createElement("h4");
    heading.textContent = "Media metadata";
    const note = document.createElement("p");
    note.textContent = "Metadata references only; no binary media is stored or loaded here.";
    const list = document.createElement("ul");
    for (const value of values.slice(0, 16)) {
      const entry = document.createElement("li");
      const kind = document.createElement("strong");
      kind.textContent = value.kind || "Media";
      entry.append(kind);
      const copy = [value.title, value.altText].filter(Boolean).join(" · ");
      if (copy) entry.append(document.createTextNode(` · ${copy}`));
      const href = safeMediaUrl(value.url);
      if (href) {
        const link = document.createElement("a");
        link.href = href;
        link.target = "_blank";
        link.rel = "noopener noreferrer";
        link.textContent = "Open reference";
        link.setAttribute("aria-label", `Open ${value.kind || "media"} metadata reference`);
        entry.append(document.createTextNode(" · "), link);
      }
      list.append(entry);
    }
    section.append(heading, note, list);
    fragment.append(section);
  }

  return {
    setLibraryTab, renderLibraryTabs, maybeLoadLibraryStorage, handleLibraryTabKeydown, renderLibrarySourceOptions, toggleLibraryFilters, syncLibraryFilterToggle, readLibraryFilters, resetLibraryList, resetLibraryStorage, invalidateLibraryStorage, reloadLibraryStorage, submitLibrarySearch, clearLibrarySearch, refreshLibrary, loadMoreLibrary, loadLibrary, loadLibraryStorage, libraryErrorMessage, libraryStorageErrorMessage, libraryMutationErrorMessage, renderLibrary, renderLibraryTopicKnowledge, buildLibraryTopicKnowledgeCard, renderLibraryStorage, buildLibrarySavedPressure, buildLibraryStorageUsage, formatLibraryStorageCount, libraryStorageMetric, buildLibraryStorageRecommendation, reviewLibraryStorageRecommendation, buildLibrarySavedRecommendation, reviewSavedRecommendation, libraryStateMessage, buildLibraryCard, libraryMetaChip, selectLibraryItem, closeLibraryDetail, renderLibraryDetail, buildLibraryDetail, removeLibraryItem, releaseLibraryItem, forgetLibraryItem, keepSavedItem, doneSavedItem, mutateLibraryItem, appendLibraryDetailField, appendLibraryLabels, appendLibraryMediaMetadata
  };
}

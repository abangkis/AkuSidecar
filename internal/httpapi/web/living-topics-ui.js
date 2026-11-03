import { LIVING_TOPIC_MAX_MEMBERS, LIVING_TOPIC_TAB_EVIDENCE, LIVING_TOPIC_TAB_SNAPSHOT, buildLivingTopicActivationPath, buildLivingTopicCandidateActionPath, buildLivingTopicMemberMovePath, buildLivingTopicMemberPath, buildLivingTopicMembersPath, buildLivingTopicMoveUndoPath, buildLivingTopicNotificationsPath, buildLivingTopicPath, buildLivingTopicSeenPath, buildLivingTopicSnapshotsPath, buildLivingTopicsPath, livingTopicClaimGroups, livingTopicStatementLabel, livingTopicStatusLabel, livingTopicUnderstandingLabel, normalizeLivingTopicName, normalizeLivingTopicNotifications, normalizeLivingTopicTab } from "./living-topics-state.js";
import { buildLibraryRequestPath } from "./library-state.js";

export function createLivingTopicsUi({
  $, api, formatDate, humanize, sourceLabel, state
}) {
  async function loadLivingTopics(force = false) {
    const topicsState = state.livingTopics;
    if (topicsState.loading || (!force && topicsState.topics.length && topicsState.detail)) return;
    topicsState.loading = true;
    topicsState.error = null;
    renderLivingTopics();
    try {
      const response = await api(buildLivingTopicsPath());
      topicsState.topics = Array.isArray(response?.topics) ? response.topics : [];
      updateLivingTopicNotificationFromTopics(topicsState.topics);
      if (topicsState.selectedId && !topicsState.topics.some((topic) => topic.id === topicsState.selectedId)) {
        topicsState.selectedId = "";
        topicsState.detail = null;
        resetLivingTopicSnapshotStatus();
      }
      const target = topicsState.selectedId || topicsState.topics[0]?.id || "";
      if (target) await loadLivingTopicDetail(target, { skipListRender: true });
    } catch (error) {
      topicsState.error = error;
    } finally {
      topicsState.loading = false;
      renderLivingTopics();
    }
  }

  async function refreshLivingTopicNotificationProjection() {
    const topicsState = state.livingTopics;
    if (!state.bootstrap || topicsState.notificationLoading) return;
    topicsState.notificationLoading = true;
    try {
      const response = await api(buildLivingTopicNotificationsPath());
      state.livingTopicNotifications = normalizeLivingTopicNotifications(response?.notifications);
      renderLivingTopicNotificationBadge();
    } catch (error) {
      console.warn("Living Topics notification refresh deferred.", error);
    } finally {
      topicsState.notificationLoading = false;
    }
  }

  function updateLivingTopicNotificationFromTopics(topics) {
    const values = Array.isArray(topics) ? topics : [];
    const active = values.filter((topic) => Number(topic?.newEvidenceCount) > 0);
    state.livingTopicNotifications = normalizeLivingTopicNotifications({
      newEvidenceCount: active.reduce((total, topic) => total + Number(topic.newEvidenceCount || 0), 0),
      topicsWithNewEvidence: active.length,
      latestEvidenceAt: active.map((topic) => topic.newEvidenceAt || "").sort().at(-1) || "",
    });
    renderLivingTopicNotificationBadge();
  }

  function renderLivingTopicNotificationBadge() {
    const summary = normalizeLivingTopicNotifications(state.livingTopicNotifications);
    const badge = $("#living-topics-nav-badge");
    const button = $("#topics-view-button");
    const count = summary.newEvidenceCount;
    badge.classList.toggle("hidden", count === 0);
    badge.textContent = count > 99 ? "99+" : String(count);
    badge.setAttribute("aria-label", count ? `${count} new Living Topics evidence item${count === 1 ? "" : "s"}` : "No new Living Topics evidence");
    button.setAttribute("aria-label", count ? `Living Topics, ${count} new evidence item${count === 1 ? "" : "s"}` : "Living Topics");
  }

  async function selectLivingTopic(id) {
    const topicsState = state.livingTopics;
    await loadLivingTopicDetail(id);
    const topic = topicsState.topics.find((value) => value.id === id);
    if (topicsState.error || !topic?.newEvidenceCount || topicsState.detail?.topic?.id !== id || topicsState.acknowledgingTopicId) return;
    topicsState.acknowledgingTopicId = id;
    try {
      const response = await api(buildLivingTopicSeenPath(id), { method: "POST", body: { seenThrough: topic.newEvidenceAt } });
      if (!response?.topic?.id) throw new Error("AkuSidecar returned an incomplete Living Topic acknowledgment.");
      topicsState.topics = topicsState.topics.map((value) => value.id === id ? response.topic : value);
      if (topicsState.detail?.topic?.id === id) topicsState.detail.topic = response.topic;
      updateLivingTopicNotificationFromTopics(topicsState.topics);
    } catch (error) {
      topicsState.error = error;
    } finally {
      topicsState.acknowledgingTopicId = "";
      renderLivingTopics();
    }
  }

  async function loadLivingTopicDetail(id, options = {}) {
    const topicsState = state.livingTopics;
    const path = buildLivingTopicPath(id);
    if (!path || topicsState.detailLoading) return;
    if (topicsState.selectedId !== id) {
      resetLivingTopicSnapshotStatus();
      topicsState.activeTab = LIVING_TOPIC_TAB_SNAPSHOT;
      topicsState.candidates = [];
      topicsState.candidatesLoaded = false;
    }
    topicsState.selectedId = id;
    topicsState.detailLoading = true;
    topicsState.error = null;
    renderLivingTopics();
    try {
      topicsState.detail = await api(path);
      if (["pending", "running"].includes(topicsState.detail?.topic?.understandingStatus)) {
        scheduleLivingTopicUnderstandingPoll(id);
      }
      if (["pending", "running"].includes(topicsState.detail?.topic?.routingStatus)) {
        scheduleLivingTopicActivationPoll(id);
      }
      if (topicsState.activeTab === LIVING_TOPIC_TAB_EVIDENCE) {
        await loadLivingTopicCandidates($("#living-topic-evidence-query")?.value || "");
      }
    } catch (error) {
      topicsState.error = error;
    } finally {
      topicsState.detailLoading = false;
      if (!options.skipListRender) renderLivingTopics();
    }
  }

  async function createLivingTopic(event) {
    event.preventDefault();
    const topicsState = state.livingTopics;
    const name = normalizeLivingTopicName($("#living-topic-create-name").value);
    if (!name || topicsState.mutationLoading) return;
    topicsState.mutationLoading = true;
    topicsState.error = null;
    renderLivingTopics();
    try {
      const response = await api(buildLivingTopicsPath(), { method: "POST", body: { name } });
      $("#living-topic-create-name").value = "";
      resetLivingTopicSnapshotStatus();
      topicsState.activeTab = LIVING_TOPIC_TAB_SNAPSHOT;
      topicsState.candidates = [];
      topicsState.candidatesLoaded = false;
      topicsState.selectedId = response.topic.id;
      topicsState.detail = null;
      await loadLivingTopics(true);
    } catch (error) {
      topicsState.error = error;
    } finally {
      topicsState.mutationLoading = false;
      renderLivingTopics();
    }
  }

  async function renameLivingTopic(event) {
    event.preventDefault();
    const topicsState = state.livingTopics;
    const name = normalizeLivingTopicName($("#living-topic-rename-name").value);
    const description = $("#living-topic-description").value.trim();
    const aliases = $("#living-topic-aliases").value.split(",").map((value) => value.trim()).filter(Boolean);
    const includeCriteria = $("#living-topic-includes").value.trim();
    const excludeCriteria = $("#living-topic-excludes").value.trim();
    const path = buildLivingTopicPath(topicsState.selectedId);
    if (!name || !path || topicsState.mutationLoading) return;
    topicsState.mutationLoading = true;
    topicsState.error = null;
    renderLivingTopics();
    try {
      await api(path, { method: "PATCH", body: { name, description, aliases, includeCriteria, excludeCriteria } });
      await loadLivingTopics(true);
    } catch (error) {
      topicsState.error = error;
    } finally {
      topicsState.mutationLoading = false;
      renderLivingTopics();
    }
  }

  async function searchLivingTopicEvidence(event) {
    event?.preventDefault?.();
    await loadLivingTopicCandidates($("#living-topic-evidence-query")?.value || "");
  }

  async function loadLivingTopicCandidates(query = "") {
    const topicsState = state.livingTopics;
    if (!topicsState.selectedId || topicsState.candidatesLoading) return;
    topicsState.candidatesLoading = true;
    renderLivingTopics();
    try {
      const path = buildLibraryRequestPath({ query, limit: 12 });
      const response = await api(path);
      topicsState.candidates = Array.isArray(response?.items) ? response.items : [];
      topicsState.candidatesLoaded = true;
    } catch (error) {
      topicsState.error = error;
    } finally {
      topicsState.candidatesLoading = false;
      renderLivingTopics();
    }
  }

  async function addLivingTopicMember(memoryItemId) {
    const topicsState = state.livingTopics;
    const path = buildLivingTopicMembersPath(topicsState.selectedId);
    if (!path || topicsState.mutationLoading) return;
    topicsState.mutationLoading = true;
    topicsState.error = null;
    resetLivingTopicSnapshotStatus();
    renderLivingTopics();
    try {
      topicsState.detail = await api(path, { method: "POST", body: { memoryItemId } });
      await refreshLivingTopicListProjection();
      if (["pending", "running"].includes(topicsState.detail?.topic?.understandingStatus)) scheduleLivingTopicUnderstandingPoll(topicsState.selectedId);
    } catch (error) {
      topicsState.error = error;
    } finally {
      topicsState.mutationLoading = false;
      renderLivingTopics();
    }
  }

  async function removeLivingTopicMember(memoryItemId) {
    const topicsState = state.livingTopics;
    const path = buildLivingTopicMemberPath(topicsState.selectedId, memoryItemId);
    if (!path || topicsState.mutationLoading) return;
    topicsState.mutationLoading = true;
    topicsState.error = null;
    resetLivingTopicSnapshotStatus();
    renderLivingTopics();
    try {
      topicsState.detail = await api(path, { method: "DELETE" });
      await refreshLivingTopicListProjection();
      if (["pending", "running"].includes(topicsState.detail?.topic?.understandingStatus)) scheduleLivingTopicUnderstandingPoll(topicsState.selectedId);
    } catch (error) {
      topicsState.error = error;
    } finally {
      topicsState.mutationLoading = false;
      renderLivingTopics();
    }
  }

  async function moveLivingTopicMember(memoryItemId, toTopicId) {
    const topicsState = state.livingTopics;
    const fromTopicId = topicsState.selectedId;
    const path = buildLivingTopicMemberMovePath(fromTopicId, memoryItemId);
    if (!path || !toTopicId || topicsState.mutationLoading) return;
    const fromName = topicsState.topics.find((topic) => topic.id === fromTopicId)?.name || "the source topic";
    const toName = topicsState.topics.find((topic) => topic.id === toTopicId)?.name || "the destination topic";
    topicsState.mutationLoading = true;
    topicsState.error = null;
    resetLivingTopicSnapshotStatus();
    renderLivingTopics();
    try {
      const response = await api(path, { method: "POST", body: { toTopicId } });
      topicsState.lastMove = { ...response.move, fromName, toName };
      topicsState.detail = await api(buildLivingTopicPath(fromTopicId));
      await refreshLivingTopicListProjection();
      scheduleLivingTopicUnderstandingPoll(fromTopicId);
    } catch (error) {
      topicsState.error = error;
    } finally {
      topicsState.mutationLoading = false;
      renderLivingTopics();
    }
  }

  async function undoLivingTopicMemberMove() {
    const topicsState = state.livingTopics;
    const move = topicsState.lastMove;
    const path = buildLivingTopicMoveUndoPath(move?.id);
    if (!path || topicsState.mutationLoading) return;
    topicsState.mutationLoading = true;
    topicsState.error = null;
    renderLivingTopics();
    try {
      await api(path, { method: "POST" });
      topicsState.lastMove = null;
      topicsState.detail = await api(buildLivingTopicPath(topicsState.selectedId));
      await refreshLivingTopicListProjection();
      scheduleLivingTopicUnderstandingPoll(topicsState.selectedId);
    } catch (error) {
      topicsState.error = error;
    } finally {
      topicsState.mutationLoading = false;
      renderLivingTopics();
    }
  }

  async function requestLivingTopicActivation() {
    const topicsState = state.livingTopics;
    const path = buildLivingTopicActivationPath(topicsState.selectedId);
    if (!path || topicsState.mutationLoading) return;
    topicsState.mutationLoading = true;
    topicsState.error = null;
    renderLivingTopics();
    try {
      const response = await api(path, { method: "POST" });
      topicsState.detail = response.detail;
      await refreshLivingTopicListProjection();
      scheduleLivingTopicActivationPoll(topicsState.selectedId);
    } catch (error) {
      topicsState.error = error;
    } finally {
      topicsState.mutationLoading = false;
      renderLivingTopics();
    }
  }

  async function reviewLivingTopicCandidate(memoryItemId, action) {
    const topicsState = state.livingTopics;
    const path = buildLivingTopicCandidateActionPath(topicsState.selectedId, memoryItemId, action);
    if (!path || topicsState.mutationLoading) return;
    topicsState.mutationLoading = true;
    topicsState.error = null;
    resetLivingTopicSnapshotStatus();
    renderLivingTopics();
    try {
      topicsState.detail = await api(path, { method: "POST" });
      await refreshLivingTopicListProjection();
      if (["pending", "running"].includes(topicsState.detail?.topic?.understandingStatus)) scheduleLivingTopicUnderstandingPoll(topicsState.selectedId);
    } catch (error) {
      topicsState.error = error;
    } finally {
      topicsState.mutationLoading = false;
      renderLivingTopics();
    }
  }

  function resetLivingTopicSnapshotStatus() {
    state.livingTopics.snapshotStatus = { kind: "idle", message: "", snapshotId: "" };
  }

  async function createLivingTopicSnapshot(event) {
    event?.preventDefault?.();
    const topicsState = state.livingTopics;
    const path = buildLivingTopicSnapshotsPath(topicsState.selectedId);
    if (topicsState.mutationLoading) return;
    if (!path) {
      topicsState.snapshotStatus = { kind: "error", message: "Select a topic before refreshing its understanding.", snapshotId: "" };
      renderLivingTopics();
      return;
    }
    topicsState.mutationLoading = true;
    topicsState.error = null;
    topicsState.snapshotStatus = { kind: "working", message: "Queuing a source-backed refresh…", snapshotId: "" };
    renderLivingTopics();
    try {
      const response = await api(path, { method: "POST" });
      if (!response?.detail?.topic?.id) throw new Error("AkuSidecar returned an incomplete refresh result.");
      topicsState.detail = response.detail;
      await refreshLivingTopicListProjection();
      topicsState.snapshotStatus = {
        kind: "working",
        message: "Refreshing current understanding in the background…",
        snapshotId: "",
      };
      scheduleLivingTopicUnderstandingPoll(topicsState.selectedId, true);
    } catch (error) {
      topicsState.snapshotStatus = {
        kind: "error",
        message: `Understanding could not be refreshed: ${error?.message || "Unknown error"}`,
        snapshotId: "",
      };
    } finally {
      topicsState.mutationLoading = false;
      renderLivingTopics();
    }
  }

  function scheduleLivingTopicUnderstandingPoll(topicId, announce = false) {
    const topicsState = state.livingTopics;
    if (!topicId || topicsState.understandingPollTopicId === topicId) return;
    topicsState.understandingPollTopicId = topicId;
    let attempts = 0;
    const poll = async () => {
      if (topicsState.selectedId !== topicId || attempts >= 60) {
        topicsState.understandingPollTopicId = "";
        return;
      }
      attempts += 1;
      try {
        const detail = await api(buildLivingTopicPath(topicId));
        if (topicsState.selectedId !== topicId) return;
        topicsState.detail = detail;
        await refreshLivingTopicListProjection();
        const understanding = detail?.topic?.understandingStatus || "idle";
        if (!["pending", "running"].includes(understanding)) {
          topicsState.understandingPollTopicId = "";
          if (announce) {
            topicsState.snapshotStatus = understanding === "failed"
              ? { kind: "error", message: detail.topic.understandingLastError || "Automatic understanding refresh failed.", snapshotId: "" }
              : { kind: "success", message: understanding === "insufficient_evidence" ? "More evidence is needed to build a useful understanding." : "Available topic evidence has been evaluated.", snapshotId: detail.topic.latestSnapshot?.id || "" };
          }
          renderLivingTopics();
          return;
        }
        renderLivingTopics();
      } catch (error) {
        topicsState.understandingPollTopicId = "";
        if (announce) topicsState.snapshotStatus = { kind: "error", message: error?.message || "Understanding refresh status could not be loaded.", snapshotId: "" };
        renderLivingTopics();
        return;
      }
      window.setTimeout(poll, 1000);
    };
    window.setTimeout(poll, 500);
  }

  function scheduleLivingTopicActivationPoll(topicId) {
    const topicsState = state.livingTopics;
    if (!topicId || topicsState.activationPollTopicId === topicId) return;
    topicsState.activationPollTopicId = topicId;
    let attempts = 0;
    const poll = async () => {
      if (topicsState.selectedId !== topicId || attempts >= 120) {
        topicsState.activationPollTopicId = "";
        return;
      }
      attempts += 1;
      try {
        const detail = await api(buildLivingTopicPath(topicId));
        if (topicsState.selectedId !== topicId) return;
        topicsState.detail = detail;
        await refreshLivingTopicListProjection();
        if (!["pending", "running"].includes(detail?.topic?.routingStatus)) {
          topicsState.activationPollTopicId = "";
          renderLivingTopics();
          return;
        }
        renderLivingTopics();
      } catch (error) {
        topicsState.activationPollTopicId = "";
        topicsState.error = error;
        renderLivingTopics();
        return;
      }
      window.setTimeout(poll, 1000);
    };
    window.setTimeout(poll, 500);
  }

  async function refreshLivingTopicListProjection() {
    const response = await api(buildLivingTopicsPath());
    state.livingTopics.topics = Array.isArray(response?.topics) ? response.topics : [];
    updateLivingTopicNotificationFromTopics(state.livingTopics.topics);
  }

  function renderLivingTopics() {
    const topicsState = state.livingTopics;
    const status = $("#living-topics-status");
    if (topicsState.error) status.textContent = topicsState.error.message || "Living Topics could not be loaded.";
    else if (topicsState.loading) status.textContent = "Loading local topics…";
    else if (topicsState.detailLoading) status.textContent = "Loading selected topic…";
    else if (topicsState.mutationLoading) status.textContent = "Applying explicit topic action…";
    else status.textContent = topicsState.topics.length ? `${topicsState.topics.length} local topic${topicsState.topics.length === 1 ? "" : "s"}` : "No topics yet. Create one to begin.";

    $("#living-topic-create-form").querySelector("button").disabled = topicsState.mutationLoading;
    const list = $("#living-topics-list");
    if (!topicsState.topics.length) {
      const empty = document.createElement("p");
      empty.className = "library-state";
      empty.textContent = "Create a Living Topic and describe what belongs. New final Timeline posts can then be routed automatically.";
      list.replaceChildren(empty);
    } else {
      list.replaceChildren(...topicsState.topics.map((topic) => {
        const button = document.createElement("button");
        button.type = "button";
        button.className = "living-topic-list-item";
        button.classList.toggle("selected", topic.id === topicsState.selectedId);
        button.setAttribute("aria-current", topic.id === topicsState.selectedId ? "true" : "false");
        const name = document.createElement("strong");
        name.textContent = topic.name;
        const fresh = document.createElement("span");
        fresh.className = "living-topic-new-badge";
        fresh.textContent = topic.newEvidenceCount > 1 ? `New ${topic.newEvidenceCount}` : "New";
        fresh.classList.toggle("hidden", !topic.newEvidenceCount);
        const meta = document.createElement("small");
        meta.textContent = `${topic.memberCount || 0} evidence${topic.suggestedCount ? ` · ${topic.suggestedCount} suggested` : ""} · ${livingTopicUnderstandingLabel(topic.understandingStatus)}`;
        button.append(name, fresh, meta);
        button.addEventListener("click", () => selectLivingTopic(topic.id));
        return button;
      }));
    }
    renderLivingTopicDetail();
  }

  function renderLivingTopicDetail() {
    const topicsState = state.livingTopics;
    const panel = $("#living-topic-detail");
    const detail = topicsState.detail;
    panel.classList.toggle("hidden", !detail);
    if (!detail) return;
    const topic = detail.topic;
    renderLivingTopicDetailTabs();
    $("#living-topic-detail-heading").textContent = topic.name;
    $("#living-topic-rename-name").value = topic.name;
    $("#living-topic-description").value = topic.description || "";
    $("#living-topic-aliases").value = (topic.aliases || []).join(", ");
    $("#living-topic-includes").value = topic.includeCriteria || "";
    $("#living-topic-excludes").value = topic.excludeCriteria || "";
    $("#living-topic-rename-form").querySelector("button").disabled = topicsState.mutationLoading;
    $("#living-topic-member-count").textContent = `${detail.members.length} / ${LIVING_TOPIC_MAX_MEMBERS}`;
    const snapshotButton = $("#living-topic-snapshot-button");
    const understandingBusy = ["pending", "running"].includes(topic.understandingStatus);
    snapshotButton.disabled = topicsState.mutationLoading || understandingBusy;
    snapshotButton.textContent = understandingBusy ? "Refreshing…" : "Refresh now";
    snapshotButton.setAttribute("aria-busy", understandingBusy ? "true" : "false");
    const snapshotStatus = $("#living-topic-snapshot-status");
    snapshotStatus.textContent = topicsState.snapshotStatus.message || livingTopicUnderstandingSummary(topic);
    snapshotStatus.dataset.kind = topicsState.snapshotStatus.kind !== "idle" ? topicsState.snapshotStatus.kind : (topic.understandingStatus === "failed" ? "error" : understandingBusy ? "working" : "success");

    const activationButton = $("#living-topic-activation-button");
    const activationBusy = ["pending", "running"].includes(topic.routingStatus);
    activationButton.disabled = topicsState.mutationLoading || activationBusy;
    activationButton.textContent = activationBusy ? "Scanning…" : "Scan again";
    activationButton.setAttribute("aria-busy", activationBusy ? "true" : "false");
    const routingStatus = $("#living-topic-routing-status");
    routingStatus.textContent = livingTopicRoutingSummary(topic);
    routingStatus.dataset.kind = topic.routingStatus === "failed" ? "error" : activationBusy ? "working" : "success";
    const suggestions = $("#living-topic-suggestions");
    const memberIDs = new Set(detail.members.map((item) => item.id));
    const suggested = (detail.candidates || []).filter((candidate) => candidate.status === "suggested" && !memberIDs.has(candidate.memoryItemId));
    const reviewed = (detail.candidates || []).filter((candidate) => candidate.status === "accepted" || candidate.status === "rejected");
    if (activationBusy && !suggested.length && !reviewed.length) {
      const loading = document.createElement("p");
      loading.className = "library-state";
      loading.textContent = "Scanning recent local evidence without changing topic membership…";
      suggestions.replaceChildren(loading);
    } else if (!suggested.length && !reviewed.length) {
      const empty = document.createElement("p");
      empty.className = "library-state";
      empty.textContent = "No local suggestions for the current criteria revision.";
      suggestions.replaceChildren(empty);
    } else {
      suggestions.replaceChildren(...suggested.map(buildLivingTopicSuggestionCard), ...reviewed.map(buildLivingTopicReviewedCandidateCard));
    }

    const members = $("#living-topic-members");
    if (!detail.members.length) {
      const empty = document.createElement("p");
      empty.className = "library-state";
      empty.textContent = "No evidence attached. Search the local Library below.";
      members.replaceChildren(empty);
    } else {
      const membershipByID = new Map((detail.memberships || []).map((membership) => [membership.memoryItemId, membership]));
      members.replaceChildren(...detail.members.map((item) => buildLivingTopicEvidenceCard(item, "Remove", () => removeLivingTopicMember(item.id), false, membershipByID.get(item.id))));
    }
    renderLivingTopicMoveStatus();

    const available = topicsState.candidates.filter((item) => !memberIDs.has(item.id));
    const candidates = $("#living-topic-candidates");
    if (topicsState.candidatesLoading) {
      const loading = document.createElement("p");
      loading.className = "library-state";
      loading.textContent = "Searching local Memory…";
      candidates.replaceChildren(loading);
    } else if (!available.length) {
      const empty = document.createElement("p");
      empty.className = "library-state";
      empty.textContent = "No additional local Memory evidence matches this search.";
      candidates.replaceChildren(empty);
    } else {
      candidates.replaceChildren(...available.map((item) => buildLivingTopicEvidenceCard(item, "Add", () => addLivingTopicMember(item.id), detail.members.length >= LIVING_TOPIC_MAX_MEMBERS)));
    }
    renderLivingTopicSnapshots(detail);
  }

  function livingTopicUnderstandingSummary(topic) {
    if (topic.understandingStatus === "pending") return "Evidence changed. An automatic understanding refresh is queued.";
    if (topic.understandingStatus === "running") return "Evaluating current evidence without blocking the Timeline…";
    if (topic.understandingStatus === "failed") return topic.understandingLastError || "The last automatic refresh failed. Refresh now to retry.";
    if (topic.understandingStatus === "insufficient_evidence") return "More evidence is needed before a useful understanding can be published.";
    if (topic.understandingStatus === "current" && topic.understandingCheckedAt) return `Evidence last evaluated ${formatDate(topic.understandingCheckedAt)}.`;
    return "Understanding will build automatically when evidence enters this topic.";
  }

  function livingTopicRoutingSummary(topic) {
    if (topic.routingStatus === "pending") return `Criteria revision ${topic.criteriaRevision || 1} is queued for a bounded local scan.`;
    if (topic.routingStatus === "running") return "Evaluating a bounded shortlist of recent local evidence…";
    if (topic.routingStatus === "failed") return topic.routingLastError || "The last local activation scan failed. Scan again to retry.";
    if (topic.routingStatus === "current" && topic.routingCheckedAt) return `Local evidence last evaluated ${formatDate(topic.routingCheckedAt)} · criteria revision ${topic.criteriaRevision || 1}.`;
    return "A bounded local scan starts when the topic is created or its criteria change.";
  }

  function setLivingTopicDetailTab(tab, focus = false) {
    const topicsState = state.livingTopics;
    topicsState.activeTab = normalizeLivingTopicTab(tab);
    renderLivingTopicDetailTabs();
    if (topicsState.activeTab === LIVING_TOPIC_TAB_EVIDENCE && !topicsState.candidatesLoaded && !topicsState.candidatesLoading) {
      void loadLivingTopicCandidates($("#living-topic-evidence-query")?.value || "");
    }
    if (focus) {
      (topicsState.activeTab === LIVING_TOPIC_TAB_EVIDENCE ? $("#living-topic-evidence-tab") : $("#living-topic-snapshot-tab"))?.focus();
    }
  }

  function renderLivingTopicDetailTabs() {
    const activeTab = normalizeLivingTopicTab(state.livingTopics.activeTab);
    state.livingTopics.activeTab = activeTab;
    const evidence = activeTab === LIVING_TOPIC_TAB_EVIDENCE;
    const tabs = [
      [$("#living-topic-snapshot-tab"), !evidence],
      [$("#living-topic-evidence-tab"), evidence],
    ];
    for (const [tab, selected] of tabs) {
      tab.setAttribute("aria-selected", String(selected));
      tab.tabIndex = selected ? 0 : -1;
    }
    const panels = [
      [$("#living-topic-snapshot-panel"), !evidence],
      [$("#living-topic-evidence-panel"), evidence],
    ];
    for (const [panel, selected] of panels) {
      panel.classList.toggle("hidden", !selected);
      panel.hidden = !selected;
    }
  }

  function handleLivingTopicDetailTabKeydown(event) {
    if (!["ArrowRight", "ArrowLeft", "Home", "End"].includes(event.key)) return;
    event.preventDefault();
    const evidence = event.key === "End"
      || ((event.key === "ArrowRight" || event.key === "ArrowLeft") && event.currentTarget.id === "living-topic-snapshot-tab");
    setLivingTopicDetailTab(evidence ? LIVING_TOPIC_TAB_EVIDENCE : LIVING_TOPIC_TAB_SNAPSHOT, true);
  }

  function buildLivingTopicEvidenceCard(item, actionLabel, action, disabled = false, membership = null) {
    const card = document.createElement("article");
    card.className = "living-topic-evidence-card";
    card.setAttribute("role", "listitem");
    const copy = document.createElement("div");
    const title = document.createElement("strong");
    title.textContent = item.title || item.summary || "Untitled Memory";
    const meta = document.createElement("small");
    meta.textContent = [sourceLabel(item.source), item.author, item.publishedAt ? formatDate(item.publishedAt) : null].filter(Boolean).join(" · ");
    copy.append(title, meta);
    if (membership) {
      const routing = document.createElement("small");
      routing.className = "living-topic-routing-meta";
      routing.textContent = membership.origin === "automatic"
        ? `Auto · ${Math.round((membership.confidence || 0) * 100)}% · ${membership.matchMode || "hybrid"}${membership.reason ? ` · ${membership.reason}` : ""}`
        : "Added by you · positive routing example";
      copy.append(routing);
    }
    const actions = document.createElement("div");
    actions.className = "living-topic-evidence-actions";
    if (membership) {
      const destinations = state.livingTopics.topics.filter((topic) => topic.id !== state.livingTopics.selectedId);
      const select = document.createElement("select");
      select.className = "living-topic-move-select";
      select.setAttribute("aria-label", `Move ${title.textContent} to another Living Topic`);
      const placeholder = document.createElement("option");
      placeholder.value = "";
      placeholder.textContent = destinations.length ? "Move to…" : "No other topic";
      select.append(placeholder, ...destinations.map((topic) => {
        const option = document.createElement("option");
        option.value = topic.id;
        option.textContent = topic.name;
        return option;
      }));
      select.disabled = disabled || state.livingTopics.mutationLoading || !destinations.length;
      const move = document.createElement("button");
      move.type = "button";
      move.className = "secondary-button living-topic-move-button";
      move.textContent = "Move";
      move.disabled = true;
      select.addEventListener("change", () => { move.disabled = !select.value || state.livingTopics.mutationLoading; });
      move.addEventListener("click", () => void moveLivingTopicMember(item.id, select.value));
      actions.append(select, move);
    }
    const button = document.createElement("button");
    button.type = "button";
    button.className = "text-button";
    button.textContent = actionLabel;
    button.disabled = disabled || state.livingTopics.mutationLoading;
    button.addEventListener("click", action);
    actions.append(button);
    card.append(copy, actions);
    return card;
  }

  function renderLivingTopicMoveStatus() {
    const status = $("#living-topic-move-status");
    const move = state.livingTopics.lastMove;
    if (!status) return;
    const appliesToSelectedTopic = move
      && [move.fromTopicId, move.toTopicId].includes(state.livingTopics.selectedId);
    status.replaceChildren();
    status.classList.toggle("hidden", !appliesToSelectedTopic);
    if (!appliesToSelectedTopic) return;
    const copy = document.createElement("span");
    copy.textContent = `Moved evidence from ${move.fromName} to ${move.toName}. Both topic understandings are refreshing.`;
    const undo = document.createElement("button");
    undo.type = "button";
    undo.className = "text-button";
    undo.textContent = state.livingTopics.mutationLoading ? "Undoing…" : "Undo";
    undo.disabled = state.livingTopics.mutationLoading;
    undo.addEventListener("click", () => void undoLivingTopicMemberMove());
    status.append(copy, undo);
  }

  function buildLivingTopicSuggestionCard(candidate) {
    const card = document.createElement("article");
    card.className = "living-topic-evidence-card living-topic-suggestion-card";
    card.setAttribute("role", "listitem");
    const copy = document.createElement("div");
    const title = document.createElement("strong");
    title.textContent = candidate.item?.title || candidate.item?.summary || "Untitled Memory";
    const meta = document.createElement("small");
    meta.textContent = [sourceLabel(candidate.item?.source), candidate.item?.author, candidate.item?.publishedAt ? formatDate(candidate.item.publishedAt) : null].filter(Boolean).join(" · ");
    const reason = document.createElement("small");
    reason.className = "living-topic-routing-meta";
    reason.textContent = `${candidate.matchMode === "llm" ? "Semantic" : "Local"} suggestion · ${Math.round((candidate.confidence || 0) * 100)}%${candidate.reason ? ` · ${candidate.reason}` : ""}`;
    copy.append(title, meta, reason);
    const actions = document.createElement("div");
    actions.className = "living-topic-candidate-actions";
    for (const [label, action] of [["Accept", "accept"], ["Reject", "reject"]]) {
      const button = document.createElement("button");
      button.type = "button";
      button.className = action === "accept" ? "secondary-button" : "text-button";
      button.textContent = label;
      button.disabled = state.livingTopics.mutationLoading;
      button.addEventListener("click", () => reviewLivingTopicCandidate(candidate.memoryItemId, action));
      actions.append(button);
    }
    card.append(copy, actions);
    return card;
  }

  function buildLivingTopicReviewedCandidateCard(candidate) {
    const card = document.createElement("article");
    card.className = "living-topic-evidence-card living-topic-suggestion-card reviewed";
    card.setAttribute("role", "listitem");
    const copy = document.createElement("div");
    const title = document.createElement("strong");
    title.textContent = candidate.item?.title || candidate.item?.summary || "Untitled Memory";
    const meta = document.createElement("small");
    meta.textContent = candidate.status === "accepted" ? "Accepted into this topic" : "Rejected from this topic";
    copy.append(title, meta);
    const undo = document.createElement("button");
    undo.type = "button";
    undo.className = "text-button";
    undo.textContent = "Undo";
    undo.disabled = state.livingTopics.mutationLoading;
    undo.addEventListener("click", () => reviewLivingTopicCandidate(candidate.memoryItemId, "undo"));
    card.append(copy, undo);
    return card;
  }

  function renderLivingTopicSnapshots(detail) {
    const container = $("#living-topic-snapshots");
    const evidenceByID = new Map(detail.members.map((item) => [item.id, item]));
    if (!detail.snapshots.length) {
      const empty = document.createElement("p");
      empty.className = "library-state";
      empty.textContent = detail.members.length
        ? "Current evidence is waiting for its first automatic understanding refresh."
        : "Add or route evidence to begin building this topic understanding.";
      container.replaceChildren(empty);
      return;
    }
    const current = detail.snapshots[0];
    if (!current?.isCurrent) {
      const empty = document.createElement("p");
      empty.className = "library-state";
      if (["pending", "running"].includes(detail.topic?.understandingStatus)) {
        empty.textContent = "Current evidence changed. A new supported understanding is being prepared; earlier versions are historical only.";
      } else if (detail.topic?.understandingStatus === "insufficient_evidence") {
        empty.textContent = "There is not enough current evidence to publish an understanding. Earlier versions remain available for audit only.";
      } else {
        empty.textContent = "No snapshot is fully supported by the current topic evidence. Earlier versions remain available for audit only.";
      }
      container.replaceChildren(empty, buildLivingTopicUnderstandingHistory(detail.snapshots, evidenceByID, "Historical understanding"));
      return;
    }
    const nodes = [buildCurrentLivingTopicUnderstanding(current, detail, evidenceByID)];
    if (detail.snapshots.length > 1) {
      nodes.push(buildLivingTopicUnderstandingHistory(detail.snapshots.slice(1), evidenceByID, "Understanding history"));
    }
    container.replaceChildren(...nodes);
  }

  function buildLivingTopicUnderstandingHistory(snapshots, evidenceByID, label) {
    const history = document.createElement("details");
    history.className = "living-topic-understanding-history";
    const summary = document.createElement("summary");
    summary.textContent = `${label} · ${snapshots.length} version${snapshots.length === 1 ? "" : "s"}`;
    const body = document.createElement("div");
    body.replaceChildren(...snapshots.map((snapshot) => buildLivingTopicHistoryEntry(snapshot, evidenceByID)));
    history.append(summary, body);
    return history;
  }

  function buildCurrentLivingTopicUnderstanding(snapshot, detail, evidenceByID) {
    const card = document.createElement("article");
    card.className = "living-topic-snapshot-card living-topic-current-understanding";
    card.dataset.snapshotId = snapshot.id;
    const header = document.createElement("header");
    const identity = document.createElement("div");
    const label = document.createElement("strong");
    label.textContent = "Latest understanding";
    const meta = document.createElement("small");
    const independentSources = Number(snapshot.independentSourceCount) || new Set(detail.members.map(livingTopicSourceOrigin).filter(Boolean)).size;
    const coverage = snapshot.coverageState ? `${humanize(snapshot.coverageState)} topic coverage` : "Topic coverage unknown";
    const diversity = snapshot.sourceDiversityState ? humanize(snapshot.sourceDiversityState) : "source diversity unknown";
    const platformDetail = snapshot.sourceDiversityState === "single_platform" ? ` · ${snapshot.sourcePlatformCount || 1} platform only` : "";
    meta.textContent = `${detail.members.length} evidence · ${independentSources} independent source${independentSources === 1 ? "" : "s"} · ${coverage} · ${diversity}${platformDetail}`;
    identity.append(label, meta);
    const badge = document.createElement("span");
    badge.className = "living-topic-understanding-badge";
    badge.textContent = livingTopicStatusLabel(snapshot.status);
    header.append(identity, badge);
    const overview = document.createElement("p");
    overview.className = "living-topic-understanding-overview";
    overview.textContent = snapshot.overview || "No overview was produced.";
    card.append(header, overview);

    const timing = document.createElement("p");
    timing.className = "library-state";
    timing.textContent = `Evaluated ${formatDate(snapshot.createdAt)} · Latest dated source in this snapshot: ${snapshot.evidenceAsOf ? formatDate(snapshot.evidenceAsOf) : "unknown"}. Newer developments may be missing from local evidence.`;
    card.append(timing);

    const { current: supported, secondary, uncertain, historical } = livingTopicClaimGroups(snapshot.claims);
    if (supported.length) {
      const heading = document.createElement("h5");
      heading.textContent = "Latest known state";
      card.append(heading, buildLivingTopicStatementList(supported, evidenceByID, "assessment"));
    }
    if (secondary.length) {
      const heading = document.createElement("h5");
      heading.textContent = "Supporting and peripheral observations";
      card.append(heading, buildLivingTopicStatementList(secondary, evidenceByID, "centrality"));
    }
    if (uncertain.length) {
      const heading = document.createElement("h5");
      heading.textContent = "Uncertainty, conflicts, and unknown applicability";
      card.append(heading, buildLivingTopicStatementList(uncertain, evidenceByID, "assessment"));
    }
    if (snapshot.previousSnapshotId && snapshot.deltas?.length) {
      const heading = document.createElement("h5");
      heading.textContent = "What changed since the previous understanding";
      card.append(heading, buildLivingTopicStatementList(snapshot.deltas, evidenceByID, "kind"));
    }
    if (historical.length) {
      const heading = document.createElement("h5");
      heading.textContent = "Historical context";
      card.append(heading, buildLivingTopicStatementList(historical, evidenceByID, "assessment"));
    }
    const technical = document.createElement("details");
    technical.className = "living-topic-generation-details";
    const technicalSummary = document.createElement("summary");
    technicalSummary.textContent = "Generation details";
    const technicalMeta = document.createElement("small");
    technicalMeta.textContent = [snapshot.provider || "provider unavailable", snapshot.model || "model unavailable", snapshot.effort || null].filter(Boolean).join(" · ");
    technical.append(technicalSummary, technicalMeta);
    card.append(technical);
    return card;
  }

  function buildLivingTopicHistoryEntry(snapshot, evidenceByID) {
    const entry = document.createElement("details");
    entry.className = "living-topic-history-entry";
    const summary = document.createElement("summary");
    const label = document.createElement("strong");
    label.textContent = `Understanding from ${formatDate(snapshot.createdAt)}`;
    const delta = document.createElement("small");
    const availability = snapshot.evidenceAvailability === "available"
      ? `${snapshot.activeEvidenceCount || 0} evidence still active`
      : snapshot.evidenceAvailability === "partial"
        ? `${snapshot.activeEvidenceCount || 0} evidence still active · partial`
        : "source evidence no longer active in this topic";
    delta.textContent = `${snapshot.deltas?.length || 0} material change${snapshot.deltas?.length === 1 ? "" : "s"} · ${availability}`;
    summary.append(label, delta);
    const body = document.createElement("div");
    const overview = document.createElement("p");
    overview.textContent = snapshot.overview || "No overview was produced.";
    body.append(overview);
    if (snapshot.claims?.length) body.append(buildLivingTopicStatementList(snapshot.claims, evidenceByID, "assessment"));
    entry.append(summary, body);
    return entry;
  }

  function buildLivingTopicStatementList(values, evidenceByID, labelField) {
    const list = document.createElement("ul");
    list.className = "living-topic-statements";
    for (const value of values) {
      const item = document.createElement("li");
      const label = document.createElement("span");
      label.className = "living-topic-statement-label";
      label.textContent = livingTopicStatementLabel(value, labelField);
      const text = document.createElement("p");
      text.textContent = value.text;
      const evidence = (value.evidenceIds || []).map((id) => evidenceByID.get(id)).filter(Boolean);
      const sourceCount = new Set(evidence.map(livingTopicSourceOrigin).filter(Boolean)).size;
      const corroborated = evidence.map((entry) => entry.publishedAt).filter(Boolean).sort().at(-1);
      const evidenceHealth = document.createElement("small");
      evidenceHealth.className = "living-topic-evidence-health";
      evidenceHealth.textContent = [
        `${value.evidenceIds?.length || 0} evidence`,
        sourceCount ? `${sourceCount} source${sourceCount === 1 ? "" : "s"}` : null,
        corroborated ? `latest ${formatDate(corroborated)}` : null,
      ].filter(Boolean).join(" · ");
      const citations = document.createElement("details");
      citations.className = "living-topic-citations";
      const citationsSummary = document.createElement("summary");
      citationsSummary.textContent = "View supporting evidence";
      const citationList = document.createElement("ul");
      for (const id of value.evidenceIds || []) {
        const cited = evidenceByID.get(id);
        const citation = document.createElement("li");
        citation.textContent = cited ? [cited.title || cited.summary || "Untitled evidence", sourceLabel(cited.source)].filter(Boolean).join(" · ") : "Evidence no longer belongs to the current topic";
        citationList.append(citation);
      }
      citations.append(citationsSummary, citationList);
      item.append(label, text, evidenceHealth, citations);
      list.append(item);
    }
    return list;
  }

  function livingTopicSourceOrigin(item) {
    if (!item) return "";
    return [item.source, item.author || item.id].filter(Boolean).join(":");
  }

  return {
    loadLivingTopics, refreshLivingTopicNotificationProjection, updateLivingTopicNotificationFromTopics, renderLivingTopicNotificationBadge, selectLivingTopic, loadLivingTopicDetail, createLivingTopic, renameLivingTopic, searchLivingTopicEvidence, loadLivingTopicCandidates, addLivingTopicMember, removeLivingTopicMember, moveLivingTopicMember, undoLivingTopicMemberMove, requestLivingTopicActivation, reviewLivingTopicCandidate, resetLivingTopicSnapshotStatus, createLivingTopicSnapshot, scheduleLivingTopicUnderstandingPoll, scheduleLivingTopicActivationPoll, refreshLivingTopicListProjection, renderLivingTopics, renderLivingTopicDetail, livingTopicUnderstandingSummary, livingTopicRoutingSummary, setLivingTopicDetailTab, renderLivingTopicDetailTabs, handleLivingTopicDetailTabKeydown, buildLivingTopicEvidenceCard, renderLivingTopicMoveStatus, buildLivingTopicSuggestionCard, buildLivingTopicReviewedCandidateCard, renderLivingTopicSnapshots, buildLivingTopicUnderstandingHistory, buildCurrentLivingTopicUnderstanding, buildLivingTopicHistoryEntry, buildLivingTopicStatementList, livingTopicSourceOrigin
  };
}

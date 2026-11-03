

export function createTimelineAiUi({
  api, formatDate, humanize, refreshTimeline, showError, sourceLabel, state
}) {
  function buildAIDetectionControls(entry) {
    if (state.bootstrap?.settings?.aiDetectionEnabled === false) {
      const badge = document.createElement("span");
      badge.className = "hidden";
      const details = document.createElement("div");
      details.className = "hidden";
      return { badge, details };
    }
    const detection = entry.aiDetection ?? null;
    const hasAssessmentLabel = Boolean(detection?.badgeLabel);
    const badgeLabel = detection?.badgeLabel || (detection?.pendingDeep || detection?.pendingMedia
      ? "AI signal · Checking"
      : "AI signal · Neutral");
    const policy = detection?.personalPolicy;
    const personalTone = policy?.reviewRequested
      ? "review"
      : policy?.applied && policy.verdict === "ai"
        ? "user-ai"
        : policy?.applied && policy.verdict === "not_ai"
          ? "user-not-ai"
          : "";
    const badgeTone = personalTone || (hasAssessmentLabel
      ? detection.corrected || detection.status === "conflicting_evidence"
        ? "corrected"
        : detection.userOverride
          ? "user"
          : detection.stage || "fast"
      : detection?.pendingDeep || detection?.pendingMedia
        ? "pending"
        : "neutral");
    const badge = document.createElement("button");
    badge.type = "button";
    badge.className = `ai-origin-badge ai-origin-${badgeTone}`;
    badge.textContent = badgeLabel;
    badge.title = "Open AI signal details and personal corrections";
    badge.setAttribute("aria-label", `${badgeLabel}. Open AI signal details and personal corrections.`);
    const initiallyExpanded = state.expandedAIDetails.has(entry.id);
    badge.setAttribute("aria-expanded", String(initiallyExpanded));
    const details = document.createElement("div");
    details.className = `ai-assessment-detail${initiallyExpanded ? "" : " hidden"}`;
    const summary = document.createElement("p");
    summary.textContent = detection?.detail || (detection?.pendingDeep || detection?.pendingMedia
      ? detection?.pendingMedia
        ? "Local C2PA inspection is checking attached image provenance. This does not assess authorship of the post text."
        : "Deep Detection is still reviewing this post. You remain in control of its personal classification."
      : "No strong AI-origin signal is currently recorded for this post.");
    const meta = document.createElement("small");
    const evidence = (detection?.evidenceCodes || []).map(humanize).join(", ");
    const mediaSignals = (detection?.mediaSignals || []).map((signal) =>
      [signal.label, humanize(signal.origin), humanize(signal.trustState)].filter(Boolean).join(" / ")
    ).join("; ");
    const platformSignals = (detection?.platformSignals || []).map((signal) =>
      [signal.label, humanize(signal.scope), sourceLabel(signal.source)].filter(Boolean).join(" / ")
    ).join("; ");
    const combinedEvidence = [evidence, platformSignals, mediaSignals].filter(Boolean).join("; ");
    const policyMeta = policy?.applied
      ? [policy.source === "account_rule" ? "Explicit account rule" : "Your item feedback", humanize(policy.signalScope), humanize(policy.reason)].filter(Boolean).join(" / ")
      : "";
    meta.textContent = detection
      ? [humanize(detection.stage), humanize(detection.confidenceBand), combinedEvidence, policyMeta, `${detection.historyCount || 0} detector assessment${detection.historyCount === 1 ? "" : "s"}`].filter(Boolean).join(" · ")
      : "No detector assessment yet";
    const actions = document.createElement("div");
    actions.className = "ai-assessment-actions";
    if (policy?.applied && detection.correctionId) {
      const undo = document.createElement("button");
      undo.type = "button";
      undo.textContent = policy.reviewRequested ? "Clear review request" : "Clear my AI feedback";
      undo.addEventListener("click", async () => {
        undo.disabled = true;
        try {
          await api(`/api/ai-feedback/${encodeURIComponent(detection.correctionId)}/undo`, { method: "POST" });
          await refreshTimeline();
        } catch (error) {
          showError(error);
          undo.disabled = false;
        }
      });
      actions.append(undo);
    }
    const fields = document.createElement("div");
    fields.className = "ai-feedback-fields";
    const target = document.createElement("select");
    target.setAttribute("aria-label", "AI feedback scope");
    [
      ["post", "social_post", "This post text", true],
      ["media", "attached_media", "Attached media", Boolean(entry.evidence?.attachments?.length || entry.evidence?.media?.length)],
      ["quote", "quoted_post", "Quoted content", Boolean(entry.evidence?.quotedPost && Object.keys(entry.evidence.quotedPost).length)],
      ["account", "author_account", "This captured account", Boolean(entry.evidence?.author || entry.item?.author)],
    ].filter((option) => option[3]).forEach(([value, scope, label]) => {
      const option = document.createElement("option");
      option.value = value;
      option.dataset.scope = scope;
      option.textContent = label;
      target.append(option);
    });
    if (policy?.targetType && [...target.options].some((option) => option.value === policy.targetType)) {
      target.value = policy.targetType;
    }
    const reason = document.createElement("select");
    reason.setAttribute("aria-label", "Optional AI feedback reason");
    [
      ["", "Optional reason"],
      ["author_disclosed_ai", "Author disclosed AI use"],
      ["platform_label", "Platform AI label"],
      ["content_credentials", "Content Credentials / C2PA"],
      ["account_identifies_as_agent", "Account identifies as an agent"],
      ["repeated_automated_pattern", "Repeated automated pattern"],
      ["signal_applies_to_another_object", "Signal applies to another object"],
      ["known_human_authored", "Known human-authored"],
      ["insufficient_evidence", "Insufficient evidence"],
    ].forEach(([value, label]) => {
      const option = document.createElement("option");
      option.value = value;
      option.textContent = label;
      reason.append(option);
    });
    if (policy?.reason) reason.value = policy.reason;
    const scopeNotice = document.createElement("small");
    scopeNotice.className = "ai-feedback-scope-notice";
    const syncScopeNotice = () => {
      scopeNotice.textContent = target.value === "account"
        ? "This is an explicit rule for the same captured account identity. It is never inferred from one post automatically."
        : "This changes AI presentation only. It never changes Timeline ranking or preference learning.";
    };
    target.addEventListener("change", syncScopeNotice);
    syncScopeNotice();
    fields.append(target, reason, scopeNotice);
    const notAI = document.createElement("button");
    notAI.type = "button";
    notAI.textContent = "Mark as not AI-generated";
    notAI.addEventListener("click", () => applyAIFeedback(entry.id, "not_ai", target, reason, notAI));
    const unsure = document.createElement("button");
    unsure.type = "button";
    unsure.textContent = "Unsure · Review more deeply";
    unsure.addEventListener("click", () => applyAIFeedback(entry.id, "unsure", target, reason, unsure));
    const isAI = document.createElement("button");
    isAI.type = "button";
    isAI.textContent = "Mark as AI-generated";
    isAI.addEventListener("click", () => applyAIFeedback(entry.id, "ai", target, reason, isAI));
    const decisions = document.createElement("div");
    decisions.className = "ai-feedback-decisions";
    decisions.append(notAI, unsure, isAI);
    const options = document.createElement("details");
    options.className = "ai-feedback-options";
    options.open = state.expandedAIFeedbackOptions.has(entry.id);
    const optionsSummary = document.createElement("summary");
    optionsSummary.textContent = "Change scope or add a reason";
    const optionsHint = document.createElement("small");
    optionsHint.textContent = "Optional. The default action applies to this post text.";
    options.append(optionsSummary, optionsHint, fields);
    options.addEventListener("toggle", () => {
      if (options.open) state.expandedAIFeedbackOptions.add(entry.id);
      else state.expandedAIFeedbackOptions.delete(entry.id);
    });
    actions.append(decisions, options);
    if ((detection?.feedbackHistoryCount || 0) > 0) {
      const history = document.createElement("button");
      history.type = "button";
      history.className = "ai-feedback-history-button";
      history.textContent = `View feedback history (${detection.feedbackHistoryCount})`;
      history.addEventListener("click", () => loadAIFeedbackHistory(entry.id, history, details));
      actions.append(history);
    }
    details.append(summary, meta, actions);
    badge.addEventListener("click", () => {
      const expanded = badge.getAttribute("aria-expanded") === "true";
      badge.setAttribute("aria-expanded", String(!expanded));
      details.classList.toggle("hidden", expanded);
      if (expanded) state.expandedAIDetails.delete(entry.id);
      else state.expandedAIDetails.add(entry.id);
    });
    return { badge, details };
  }

  async function applyAIFeedback(timelineId, verdict, target, reason, trigger) {
    if (target.value === "account" && !window.confirm(
      "Apply this as an explicit account-level AI rule? It will affect posts from the same captured account identity, but never Timeline ranking or preference learning."
    )) {
      return;
    }
    trigger.disabled = true;
    try {
      const selected = target.options[target.selectedIndex];
      await api(`/api/timeline/${encodeURIComponent(timelineId)}/ai-feedback`, {
        method: "POST",
        body: {
          verdict,
          targetType: target.value,
          signalScope: selected.dataset.scope,
          reason: reason.value,
        },
      });
      await refreshTimeline();
    } catch (error) {
      showError(error);
      trigger.disabled = false;
    }
  }

  async function loadAIFeedbackHistory(timelineId, trigger, container) {
    trigger.disabled = true;
    try {
      const payload = await api(`/api/timeline/${encodeURIComponent(timelineId)}/ai-feedback`);
      container.querySelector(".ai-feedback-history")?.remove();
      const history = document.createElement("ol");
      history.className = "ai-feedback-history";
      (payload.feedback || []).forEach((event) => {
        const row = document.createElement("li");
        const decision = event.verdict === "clear" ? "Cleared" : humanize(event.verdict);
        row.textContent = [
          decision,
          humanize(event.targetType),
          humanize(event.reason),
          event.createdAt ? formatDate(event.createdAt) : "",
        ].filter(Boolean).join(" · ");
        history.append(row);
      });
      container.append(history);
      trigger.textContent = "Feedback history loaded";
    } catch (error) {
      showError(error);
      trigger.disabled = false;
    }
  }

  return {
    buildAIDetectionControls, applyAIFeedback, loadAIFeedbackHistory
  };
}

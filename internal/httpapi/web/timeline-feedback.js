export function createTimelineFeedbackUI({
  api, feedbackButton, invalidateLibraryStorage, showError, logInteractionTrace,
  document = globalThis.document, crypto = globalThis.crypto, performance = globalThis.performance,
}) {
  function build(entry) {
    const feedback = document.createElement("div");
    feedback.className = "feedback-actions";
    const more = feedbackButton("More like this");
    const less = feedbackButton("Less like this");
    const renderDirection = () => {
      const direction = entry.feedback?.direction;
      const calibrationChoice = entry.feedback?.origin === "calibration" ? "Chosen during onboarding calibration" : "";
      more.classList.toggle("selected", direction === "more");
      less.classList.toggle("selected", direction === "less");
      more.setAttribute("aria-pressed", String(direction === "more"));
      less.setAttribute("aria-pressed", String(direction === "less"));
      more.title = direction === "more" ? calibrationChoice : "";
      less.title = direction === "less" ? calibrationChoice : "";
    };
    renderDirection();
    more.addEventListener("click", async () => {
      const traceId = crypto.randomUUID();
      const started = performance.now();
      logInteractionTrace("timeline_feedback", traceId, "click", { direction: "more" });
      const feedback = await sendFeedback(entry.id, "more", null, traceId);
      if (!feedback) return;
      entry.feedback = feedback;
      renderDirection();
      logInteractionTrace("timeline_feedback", traceId, "rendered", { elapsedMs: Math.round(performance.now() - started) });
    });
    less.addEventListener("click", async () => {
      const traceId = crypto.randomUUID();
      const started = performance.now();
      logInteractionTrace("timeline_feedback", traceId, "click", { direction: "less" });
      const feedback = await sendFeedback(entry.id, "less", "not_interested", traceId);
      if (!feedback) return;
      entry.feedback = feedback;
      renderDirection();
      logInteractionTrace("timeline_feedback", traceId, "rendered", { elapsedMs: Math.round(performance.now() - started) });
    });
    feedback.append(more, less);
    return feedback;
  }

  async function sendFeedback(id, direction, reason, traceId) {
    const started = performance.now();
    let outcome = "error";
    logInteractionTrace("timeline_feedback", traceId, "request_start");
    try {
      const response = await api(`/api/timeline/${encodeURIComponent(id)}/feedback`, {
        method: "POST",
        body: { direction, reason },
        feedbackTraceId: traceId,
      });
      invalidateLibraryStorage();
      outcome = "ok";
      return response.feedback;
    } catch (error) {
      showError(error);
      return null;
    } finally {
      logInteractionTrace("timeline_feedback", traceId, "request_end", { outcome, elapsedMs: Math.round(performance.now() - started) });
    }
  }

  return { build };
}

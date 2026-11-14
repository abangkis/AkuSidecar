// Reading protection and the update notice are UI concerns. Fetch ordering and
// pending snapshots belong to the refresh coordinator, not this component.
export function createTimelineRefreshUi({ document, viewport, onApply, isVisible = () => true, isApplying = () => false }) {
  let staged = null;
  let listening = false;
  const roots = () => ["result-items", "timeline-side-pane"].map(id => document?.getElementById?.(id))
    .filter(root => root && !root.classList?.contains("hidden"));

  function blockedReason() {
    if (!isVisible() || document?.visibilityState === "hidden") return "Return to Timeline to apply updates.";
    const surfaces = roots();
    if (!surfaces.length) return "";
    if ([...(document.querySelectorAll?.("#result-items video, #timeline-side-pane video") ?? [])]
      .some(video => !video.paused && !video.ended)) return "Pause the video to apply updates.";
    const selection = viewport?.getSelection?.();
    if (selection && !selection.isCollapsed) {
      for (let index = 0; index < selection.rangeCount; index++) {
        if (surfaces.some(surface => selection.getRangeAt(index).intersectsNode(surface))) return "Finish selecting text to apply updates.";
      }
    }
    if (surfaces.some(surface => surface.querySelector(".event-suggestion-editor, .ai-feedback-options[open]"))) return "Finish editing to apply updates.";
    const active = document.activeElement;
    if (active && surfaces.some(surface => surface.contains(active)) && active.matches("button, select, summary, input, textarea, a[href], video")) {
      return "Finish interacting with the post to apply updates.";
    }
    return "";
  }

  function sync() {
    const notice = document?.getElementById?.("timeline-refresh-notice");
    if (!notice) return;
    notice.classList.toggle("hidden", !staged || !isVisible());
    const button = document.getElementById("timeline-refresh-apply");
    const detail = document.getElementById("timeline-refresh-detail");
    const reason = blockedReason();
    button.disabled = !staged || isApplying() || Boolean(reason);
    detail.textContent = reason || "Updated post details are ready. Apply when you are ready.";
  }

  function show(snapshot) {
    staged = snapshot;
    const button = document?.getElementById?.("timeline-refresh-apply");
    if (button && !listening) {
      listening = true;
      button.addEventListener("click", async () => {
        button.disabled = true;
        try { await onApply(); } finally { sync(); }
      });
      for (const event of ["play", "pause", "ended", "selectionchange", "focusin", "focusout", "input", "toggle", "visibilitychange"]) {
        document.addEventListener(event, sync, true);
      }
    }
    sync();
  }

  return { show, sync, canApply: () => !blockedReason(), interactionActive: () => Boolean(blockedReason()) };
}

// Anchor restoration shares the synchronous render task. Prefer the first
// surviving visible card; use the current scroll position if all anchors vanish.
export function renderWithReadingAnchor(render, documentRef = document, viewport = window) {
  const top = viewport.scrollY;
  const cards = () => [...(documentRef.querySelectorAll?.("#result-items > [data-timeline-id]") ?? [])];
  const anchors = cards().map(card => ({ id: card.dataset.timelineId, rect: card.getBoundingClientRect() }))
    .filter(({ rect }) => rect.bottom > 0 && rect.top < viewport.innerHeight);
  render();
  const after = new Map(cards().map(card => [card.dataset.timelineId, card]));
  const anchor = anchors.find(candidate => after.has(candidate.id));
  const delta = anchor ? after.get(anchor.id).getBoundingClientRect().top - anchor.rect.top : 0;
  viewport.scrollTo({ top: Math.max(0, top + delta), behavior: "instant" });
}

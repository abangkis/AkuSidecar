export function rectanglesOverlap(a, b) {
  return a.left < b.right && a.right > b.left && a.top < b.bottom && a.bottom > b.top;
}

export function createScrollIdleGate({ now = () => performance.now(), schedule = setTimeout, cancel = clearTimeout, delay = 350 } = {}) {
  let lastScroll = -Infinity, timer = null;
  const waiting = [];
  function arm() {
    if (timer !== null) cancel(timer);
    timer = schedule(() => {
      timer = null;
      if (now() - lastScroll < delay) { arm(); return; }
      for (const resolve of waiting.splice(0)) resolve();
    }, Math.max(0, delay - (now() - lastScroll)));
  }
  return {
    noteScroll() { lastScroll = now(); if (waiting.length) arm(); },
    wait() {
      if (now() - lastScroll >= delay) return Promise.resolve();
      return new Promise(resolve => { waiting.push(resolve); arm(); });
    },
  };
}

// Test candidate positions in memory; never move the DOM just to measure it.
export function backToTopHorizontalPosition({ anchor, button, viewportWidth, obstacles = [] }) {
  const width = button.width;
  const gap = 30;
  const fallbackLeft = viewportWidth - (viewportWidth <= 700 ? 14 : Math.min(36, Math.max(16, viewportWidth * 0.03))) - width;
  const candidates = [];
  if (anchor && viewportWidth - anchor.right >= width + gap * 2) candidates.push({ left: Math.round(anchor.right + gap), inline: true });
  candidates.push({ left: fallbackLeft, inline: false });
  if (anchor && anchor.left >= width + gap * 2) candidates.push({ left: Math.round(anchor.left - width - gap), inline: true });
  const selected = candidates.find(candidate => !obstacles.some(obstacle => rectanglesOverlap({
    ...button, left: candidate.left, right: candidate.left + width,
  }, obstacle))) || candidates.find(candidate => !candidate.inline);
  return { left: selected.inline ? `${selected.left}px` : "", right: selected.inline ? "auto" : "" };
}

import { createBrowserPerformanceSampler } from "./diagnostics/browser-performance.js";

// Timeline adapter: retain the existing public API and approved report vocabulary.
export function createScrollPerformanceTrace({
  performance, requestFrame, cancelFrame, setTimer, clearTimer, Observer, onStop,
} = {}) {
  return createBrowserPerformanceSampler({
    performance, requestFrame, cancelFrame, setTimer, clearTimer, Observer, onStop,
    stageNames: ["scroll_frame", "content_context_scroll", "back_to_top", "side_pane", "timeline_render"],
    stopReasons: { allowed: ["manual", "timeout", "hidden"], default: "manual", timeout: "timeout" },
  });
}

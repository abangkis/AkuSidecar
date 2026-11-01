import { performance } from 'node:perf_hooks';
import { startupFailure } from './x-manual-startup-diagnostic.mjs';
import { MANUAL_STARTUP_MAX_MS } from './x-manual-startup-limits.mjs';

// Private wait hint only. Go retains the authoritative joint deadline across
// transport, evidence validation and the final ownership inspection.
export function createStartupWaitBudget(milliseconds, { role, signal, now = () => performance.now(), setTimer = setTimeout, clearTimer = clearTimeout } = {}) {
  if (!Number.isSafeInteger(milliseconds) || milliseconds < 1 || milliseconds > MANUAL_STARTUP_MAX_MS ||
      !['reader', 'collector'].includes(role)) throw new Error('manual_startup_budget_invalid');
  const start = now();
  if (!Number.isFinite(start) || start < 0) throw new Error('manual_startup_budget_invalid');
  let last = start;
  const remaining = () => {
    const at = now();
    if (!Number.isFinite(at) || at < last) throw startupFailure(role, 'wire_decode', 'time_invalid');
    last = at;
    if (signal?.aborted) throw startupFailure(role, 'joint', 'canceled');
    const left = Math.floor(milliseconds - (at - start));
    if (left <= 0) throw startupFailure(role, 'settings', 'deadline');
    return left;
  };
  return Object.freeze({
    async wait(promise) {
      let timer, onAbort;
      try {
        const left = remaining();
        const result = await Promise.race([promise, new Promise((resolve, reject) => {
          onAbort = () => reject(startupFailure(role, 'joint', 'canceled'));
          signal?.addEventListener('abort', onAbort, { once: true });
          timer = setTimer(() => reject(startupFailure(role, 'settings', 'deadline')), left);
        })]);
        remaining();
        return result;
      } finally {
        if (timer !== undefined) clearTimer(timer);
        if (onAbort) signal?.removeEventListener('abort', onAbort);
      }
    },
  });
}

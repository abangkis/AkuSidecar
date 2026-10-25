import {performance} from 'node:perf_hooks';
import {startupDiagnostic, startupRPCCode} from './x-manual-startup-diagnostic.mjs';

const OPS = new Set(['inspect_rpc','inventory_rpc','acquire_rpc','identity','rendezvous','settings_wait','cleanup','root_restore','complete']);
// One pending RPC, at most 32 closed progress records. No pins, request IDs,
// account data or exception text enter the diagnostic payload. It has no authority.
export function createStartupTiming(report, now = () => performance.now()) {
  let operation = 'acquire_rpc', started = now(), count = 0, closed = false;
  const publish = cause => {
    if (closed || count >= 32) return;
    const elapsed = now() - started;
    if (!Number.isFinite(elapsed) || elapsed < 0) return;
    count++;
    try { report({operation, elapsedMS: Math.min(60000, Math.floor(elapsed)), cause}); } catch { /* Diagnostics never alter admission. */ }
  };
  return Object.freeze({
    step(next) {
      if (closed || !OPS.has(next)) return;
      operation = next; started = now(); publish('');
    },
    failure(error) { publish(startupDiagnostic(error) ? startupRPCCode(error) : ''); closed = true; },
    finish() { publish(''); closed = true; },
  });
}

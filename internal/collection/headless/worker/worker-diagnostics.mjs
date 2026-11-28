import { Console } from 'node:console';
import './vendor/diagnostic-state-core.js';
import './vendor/diagnostic-recorder.js';

import reasonCatalog from './diagnostic-reasons.json' with { type: 'json' };

const reasons = new Set(reasonCatalog);

export function createWorkerDiagnostics(errorOutput) {
  let recorder;
  let sink;
  return (phase, error) => {
    try {
      if (!errorOutput || errorOutput.destroyed || errorOutput.writableNeedDrain) return;
      recorder ??= globalThis.AkuDiagnosticRecorder.create('headless_worker');
      recorder.advance(phase);
      const { location, sequence } = recorder.snapshot();
      const reason = reasons.has(error?.code) ? error.code : 'worker_error';
      sink ??= new Console({ stdout: errorOutput, stderr: errorOutput, ignoreErrors: true });
      sink.error(JSON.stringify({ ...location, sequence, level: 'error', reason }));
    } catch { /* Diagnostics are best-effort and must never enter the RPC stream. */ }
  };
}

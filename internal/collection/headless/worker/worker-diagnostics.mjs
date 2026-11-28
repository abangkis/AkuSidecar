import { Console } from 'node:console';
import './vendor/diagnostic-state-core.js';
import './vendor/diagnostic-recorder.js';

const reasons = new Set([
  'already_initialized', 'browser_unavailable', 'capture_timeout', 'challenge_required',
  'chrome_start_failed', 'empty_unverified', 'evidence_too_large', 'existing_surface',
  'freshness_unavailable', 'invalid_init', 'invalid_json', 'invalid_observation',
  'invalid_page_url', 'invalid_payload', 'invalid_request', 'login_required', 'navigation_failed',
  'not_initialized', 'owned_chrome_cleanup_failed', 'owned_chrome_cleanup_unverified',
  'photo_parent_unverified', 'profile_in_use', 'profile_owner_check_failed', 'request_too_large',
  'response_too_large', 'source_frontier_unavailable', 'source_route_changed', 'source_unavailable',
  'target_unavailable', 'temporary_target_cleanup_failed', 'unsupported_continuation',
  'unsupported_source', 'windowed_identity_override_failed', 'windowed_identity_unavailable',
  'worker_not_ready', 'worker_shutting_down',
]);

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

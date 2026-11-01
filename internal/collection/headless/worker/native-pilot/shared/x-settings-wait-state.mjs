// Closed diagnostics only: no request references, URLs, headers, body or identity.
export const SETTINGS_WAIT_STATES = Object.freeze([
  'unknown', 'request_pending', 'request_before_identity', 'request_frame',
  'request_document', 'request_history', 'response_pending', 'completion_pending',
  'extra_pending', 'metadata_complete', 'network_failed', 'candidate_rejected',
  'scope_lost', 'body_pending', 'body_complete', 'body_failed',
]);

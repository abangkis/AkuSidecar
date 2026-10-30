// Internal acquisition primitive only. No browser capability is created here.
// The caller must own a response-stage Fetch stream through a sealed scope;
// scope.call must enforce its timeout and retain late-resource obligations.
import {INSTAGRAM_CORROBORATION_POLICY as policy} from './instagram-bootstrap-corroboration-policy.mjs';
const MAX_BODY_BYTES = policy.maxBodyBytes;
const CHUNK_BYTES = policy.chunkBytes;
const failure = () => new Error('instagram_bootstrap_stream_rejected');

export async function readInstagramBootstrapBodyStream(scope, session, stream, {now = Date.now, deadlineAt, maxBytes = MAX_BODY_BYTES} = {}) {
  if (!scope || typeof scope.call !== 'function' || typeof session !== 'string' || !session ||
      typeof stream !== 'string' || !stream || !Number.isSafeInteger(deadlineAt) ||
      !Number.isSafeInteger(maxBytes) || maxBytes < 1 || maxBytes > MAX_BODY_BYTES) throw failure();
  const chunks = [];
  let size = 0, reason = 'none', cleanup = 'uncertain', body;
  let previous = now();
  try {
    for (;;) {
      const time = now();
      if (!Number.isSafeInteger(time) || time < previous || time >= deadlineAt) { reason = 'body_deadline'; break; }
      previous = time;
      // The extra byte detects overflow at an exact cap without trusting headers.
      const requested = Math.min(CHUNK_BYTES, maxBytes - size + 1);
      const reply = await scope.call('IO.read', {handle: stream, size: requested}, session, Math.min(1000, deadlineAt - time));
      if (!reply || typeof reply.data !== 'string' || typeof reply.eof !== 'boolean' ||
          reply.base64Encoded !== undefined && typeof reply.base64Encoded !== 'boolean' ||
          Object.keys(reply).some(key => !['data', 'eof', 'base64Encoded'].includes(key))) { reason = 'body_read'; break; }
      // Bound decoding allocation, including a provider that violates IO.read size.
      if (reply.data.length > (reply.base64Encoded ? 4 * Math.ceil(requested / 3) : requested)) { reason = 'body_bounds'; break; }
      if (reply.base64Encoded && !/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(reply.data)) { reason = 'body_encoding'; break; }
      const chunk = Buffer.from(reply.data, reply.base64Encoded ? 'base64' : 'utf8');
      chunks.push(chunk);
      if (chunk.length > requested || size + chunk.length > maxBytes) { reason = 'body_bounds'; break; }
      size += chunk.length;
      if (reply.eof) {
        const finished = now();
        if (!Number.isSafeInteger(finished) || finished < previous || finished >= deadlineAt) { reason = 'body_deadline'; break; }
        body = Buffer.concat(chunks, size);
        break;
      }
      if (!chunk.length) { reason = 'body_read'; break; }
    }
  } catch { reason = 'body_read'; }
  finally {
    for (const chunk of chunks) chunk.fill(0);
    try { await scope.call('IO.close', {handle: stream}, session, 1000); cleanup = 'verified'; } catch {}
  }
  if (cleanup !== 'verified' || reason !== 'none') { body?.fill(0); body = undefined; }
  // Private bytes must be erased by the caller after parsing/replay. This is
  // deliberately not a public diagnostic projection or authentication proof.
  return {reason, cleanup, body};
}

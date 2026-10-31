import {igFailureDetail} from './instagram-diagnostic-trace.mjs';
import {cdpCommandFailureDetail} from './cdp-command-failure.mjs';
// Internal acquisition primitive only. No browser capability is created here.
// The caller must own a response-stage Fetch stream through a sealed scope;
// scope.call must enforce its timeout and retain late-resource obligations.
import {INSTAGRAM_CORROBORATION_POLICY as policy} from './instagram-bootstrap-corroboration-policy.mjs';
const MAX_BODY_BYTES = policy.maxBodyBytes;
const CHUNK_BYTES = policy.chunkBytes;
const failure = () => new Error('instagram_bootstrap_stream_rejected');

function validReadReply(reply){
  try{
    if(!reply||![Object.prototype,null].includes(Object.getPrototypeOf(reply)))return false;
    const keys=Reflect.ownKeys(reply);
    if(keys.some(key=>!['data','eof','base64Encoded'].includes(key)))return false;
    const data=Object.getOwnPropertyDescriptor(reply,'data'),eof=Object.getOwnPropertyDescriptor(reply,'eof');
    const encoded=Object.getOwnPropertyDescriptor(reply,'base64Encoded');
    return !!data&&Object.hasOwn(data,'value')&&typeof data.value==='string'&&
      !!eof&&Object.hasOwn(eof,'value')&&typeof eof.value==='boolean'&&
      (!encoded||Object.hasOwn(encoded,'value')&&typeof encoded.value==='boolean');
  }catch{return false;}
}

function readCommandCondition(error){
  switch(cdpCommandFailureDetail(error)?.kind){
    case 'timeout':return 'read_command_timeout';
    case 'transport_closed':return 'read_transport_closed';
    case 'command_rejected':return 'read_cdp_rejected';
    case 'reply_invalid':return 'read_reply_invalid';
    default:return 'read_command_failed';
  }
}

export async function readInstagramBootstrapBodyStream(scope, session, stream, {now = Date.now, deadlineAt, maxBytes = MAX_BODY_BYTES, trace} = {}) {
  if (!scope || typeof scope.call !== 'function' || typeof session !== 'string' || !session ||
      typeof stream !== 'string' || !stream || !Number.isFinite(deadlineAt) ||
      !Number.isSafeInteger(maxBytes) || maxBytes < 1 || maxBytes > MAX_BODY_BYTES) throw failure();
  const chunks = [];
  let size = 0, reason = 'none', cleanup = 'uncertain', body, condition;
  let previous = now();
  try {
    for (;;) {
      const time = now();
      if (!Number.isFinite(time) || time < previous || time >= deadlineAt) { reason = 'body_deadline'; break; }
      previous = time;
      // The extra byte detects overflow at an exact cap without trusting headers.
      const requested = Math.min(CHUNK_BYTES, maxBytes - size + 1);
      const reply = await scope.call('IO.read', {handle: stream, size: requested}, session, Math.min(1000, deadlineAt - time));
      if (!validReadReply(reply)) { reason = 'body_read'; condition = 'read_reply_invalid'; break; }
      // Bound decoding allocation, including a provider that violates IO.read size.
      if (reply.data.length > (reply.base64Encoded ? 4 * Math.ceil(requested / 3) : requested)) { reason = 'body_bounds'; break; }
      if (reply.base64Encoded && !/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(reply.data)) { reason = 'body_encoding'; break; }
      const chunk = Buffer.from(reply.data, reply.base64Encoded ? 'base64' : 'utf8');
      chunks.push(chunk);
      if (chunk.length > requested || size + chunk.length > maxBytes) { reason = 'body_bounds'; break; }
      size += chunk.length;
      if (reply.eof) {
        const finished = now();
        if (!Number.isFinite(finished) || finished < previous || finished >= deadlineAt) { reason = 'body_deadline'; break; }
        body = Buffer.concat(chunks, size);
        break;
      }
      if (!chunk.length) { reason = 'body_read'; condition = 'read_empty_nonfinal'; break; }
    }
  } catch(error) { reason = 'body_read';const detail=igFailureDetail(error);if(detail)trace?.fail(detail.stage,detail.condition);else condition=readCommandCondition(error); }
  finally {
    if(reason!=='none')trace?.fail('body_read',condition??reason);
    for (const chunk of chunks) chunk.fill(0);
    trace?.note('stream_close');
    try { await scope.call('IO.close', {handle: stream}, session, 1000); cleanup = 'verified';trace?.done('stream_close'); } catch { trace?.cleanup('uncertain','stream_close_failed'); }
  }
  if (cleanup !== 'verified' || reason !== 'none') { body?.fill(0); body = undefined; }
  // Private bytes must be erased by the caller after parsing/replay. This is
  // deliberately not a public diagnostic projection or authentication proof.
  return {reason, cleanup, body};
}

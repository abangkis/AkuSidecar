import {performance} from 'node:perf_hooks';
import {createIGTrace,igAllowed} from './instagram-diagnostic-trace.mjs';
// Sealed, reader-only diagnostic. Importing this module performs no I/O.
export const INSTAGRAM_DIAGNOSTIC_SCHEMA = 'aku.instagram-reader-diagnostic.v1';
export const INSTAGRAM_DIAGNOSTIC_HOME = 'https://www.instagram.com/';
const endpoint = 'https://www.instagram.com/api/v1/accounts/current_user/?edit=true';
const noncePattern = /^[a-f0-9]{48}$/u;
export const DIAGNOSTIC_PRIMARY_CAUSES = Object.freeze(['none', 'invalid_request', 'scope_unavailable',
  'setup_failed', 'navigation_failed', 'context_unavailable', 'document_changed', 'evaluation_failed',
  'projection_invalid', 'fetch_timeout', 'fetch_failed', 'http_rejected', 'unexpected_success',
  'response_scope', 'body_oversize', 'body_read_failed', 'body_invalid', 'wire_unmatched',
  'cleanup_failed', 'page_scope', 'login_visible']);
const mimes = ['missing', 'json', 'html', 'other'];
const bodies = ['not_read', 'json_object', 'json_nonobject', 'invalid_json', 'invalid_utf8', 'oversize', 'read_failed'];
const errors = ['none', 'login_required', 'challenge_required', 'checkpoint_required', 'feedback_required', 'sentry_block', 'csrf_failed', 'invalid_app_id', 'unknown'];
const cleanupStates = ['not_started', 'verified', 'uncertain'];
const resultKeys = ['schema', 'primaryCause', 'status', 'mime', 'bodyClass', 'errorCategory', 'documentBound', 'wireMatched', 'cleanup'];
const pageKeys = ['nonce', 'primaryCause', 'status', 'mime', 'bodyClass', 'errorCategory', 'homeAfter'];
const failure = () => new Error('instagram_reader_diagnostic_rejected');
const reference = value => typeof value === 'string' && value.length > 0 && value.length <= 512 && !/[\u0000-\u001f\u007f]/u.test(value);
function record(value, keys) {
  if (!value || ![Object.prototype, null].includes(Object.getPrototypeOf(value))) return false;
  const descriptors = Object.getOwnPropertyDescriptors(value);
  return Reflect.ownKeys(descriptors).length === keys.length && keys.every(key => Object.hasOwn(descriptors[key] ?? {}, 'value'));
}
function fieldsValid(value) {
  return DIAGNOSTIC_PRIMARY_CAUSES.includes(value.primaryCause) && Number.isInteger(value.status) &&
    (value.status === 0 || value.status >= 100 && value.status <= 599) && mimes.includes(value.mime) &&
    bodies.includes(value.bodyClass) && errors.includes(value.errorCategory);
}
export function validateInstagramReaderDiagnostic(value) {
  if (!record(value, resultKeys) || value.schema !== INSTAGRAM_DIAGNOSTIC_SCHEMA || !fieldsValid(value) ||
      typeof value.documentBound !== 'boolean' || typeof value.wireMatched !== 'boolean' ||
      !cleanupStates.includes(value.cleanup) || Buffer.byteLength(JSON.stringify(value), 'utf8') > 1024) throw failure();
  return Object.freeze(Object.fromEntries(resultKeys.map(key => [key, value[key]])));
}

// The allowlist labels are observations, not proof of a missing header. Unknown
// server messages and error types never leave this realm as arbitrary strings.
export function instagramReaderDiagnosticExpression(nonce) {
  if (typeof nonce !== 'string' || !noncePattern.test(nonce)) throw failure();
  return `(async()=>{
    const out={nonce:${JSON.stringify(nonce)},primaryCause:'none',status:0,mime:'missing',bodyClass:'not_read',errorCategory:'none',homeAfter:false};
    const home=()=>location.origin==='https://www.instagram.com'&&location.pathname==='/';
    const finish=()=>({...out,homeAfter:home()});
    if(!home()){out.primaryCause='page_scope';return finish();}
    if(document.querySelector('input[type="password"],form[action*="login"]')){out.primaryCause='login_visible';return finish();}
    const controller=new AbortController(),timer=setTimeout(()=>controller.abort(),4000);
    let reader,phase='fetch',chunks=[],bytes,text='',value;
    try{
      const response=await fetch('/api/v1/accounts/current_user/?edit=true',{method:'GET',credentials:'same-origin',cache:'no-store',redirect:'error',signal:controller.signal,headers:{Accept:'application/json'}});
      const status=response.status;
      if(!Number.isInteger(status)||status<100||status>599){out.primaryCause='projection_invalid';return finish();}
      out.status=status;
      if(status===200){out.primaryCause='unexpected_success';return finish();}
      out.primaryCause='http_rejected';
      const url=new URL(response.url);
      if(url.origin!=='https://www.instagram.com'||url.pathname!=='/api/v1/accounts/current_user/'||url.search!=='?edit=true'){out.primaryCause='response_scope';return finish();}
      const mime=response.headers.get('content-type')||'';
      out.mime=!mime?'missing':/^application\\/json(?:;|$)/i.test(mime)?'json':/^text\\/html(?:;|$)/i.test(mime)?'html':'other';
      if(out.mime!=='json')return finish();
      if(Number(response.headers.get('content-length'))>4096){out.bodyClass='oversize';return finish();}
      if(!response.body){out.bodyClass='read_failed';return finish();}
      phase='body';reader=response.body.getReader();let size=0;
      while(true){const part=await reader.read();if(part.done)break;size+=part.value.byteLength;if(size>4096){out.bodyClass='oversize';return finish();}chunks.push(part.value);}
      bytes=new Uint8Array(size);let offset=0;for(const chunk of chunks){bytes.set(chunk,offset);offset+=chunk.byteLength;}
      try{text=new TextDecoder('utf-8',{fatal:true}).decode(bytes);}catch{out.bodyClass='invalid_utf8';return finish();}
      try{value=JSON.parse(text);}catch{out.bodyClass='invalid_json';return finish();}
      if(!value||typeof value!=='object'||Array.isArray(value)){out.bodyClass='json_nonobject';return finish();}
      out.bodyClass='json_object';
      const allowed=['login_required','challenge_required','checkpoint_required','feedback_required','sentry_block','csrf_failed','invalid_app_id'];
      out.errorCategory=allowed.includes(value.error_type)?value.error_type:allowed.includes(value.message)?value.message:'unknown';
      return finish();
    }catch{
      if(out.primaryCause==='none')out.primaryCause=controller.signal.aborted?'fetch_timeout':'fetch_failed';
      if(phase==='body')out.bodyClass='read_failed';
      return finish();
    }finally{
      clearTimeout(timer);bytes?.fill(0);for(const chunk of chunks)chunk.fill(0);chunks=[];text='';value=undefined;
      try{const canceled=reader?.cancel();if(canceled&&typeof canceled.catch==='function')void canceled.catch(()=>{});}catch{}
    }
  })()`;
}

const pause = ms => new Promise(resolve => setTimeout(resolve, ms));
export async function runInstagramReaderDiagnostic(transport, request, {trace=createIGTrace(),budgetMS=30000}={}) {
  const now=()=>Math.floor(performance.now()),acquisitionUntil=now()+Math.max(0,budgetMS-5000);trace.note('scope');
  const result = {schema: INSTAGRAM_DIAGNOSTIC_SCHEMA, primaryCause: 'none', status: 0, mime: 'missing',
    bodyClass: 'not_read', errorCategory: 'none', documentBound: false, wireMatched: false, cleanup: 'not_started'};
  const cause = value => { if (result.primaryCause === 'none') result.primaryCause = value;if(value!=='none')trace.fail(trace.snapshot().stage,igAllowed('conditions',value)?value:'unknown'); };
  if (!record(request, ['nonce']) || typeof request.nonce !== 'string' || !noncePattern.test(request.nonce)) {
    cause('invalid_request'); return validateInstagramReaderDiagnostic(result);
  }
  let scope, targetId, sessionId, unsubscribe, pinned, attemptedTarget = false, invalidated = false, phase = 'setup_failed', setupRevision = 0;
  let requestID, requestCount = 0, wireStatus, expectedFrame, expectedLoader;
  const contexts = new Map();
  const setupUntil = Math.min(now()+10000,acquisitionUntil);
  const invalidate = () => { if (pinned) { invalidated = true; cause('document_changed'); } };
  const timeout = () => { const ms = setupUntil - now(); if (ms < 1) throw failure(); return Math.min(8000, ms); };
  const send = (method, params = {}, ms = timeout()) => scope.call(method, params, sessionId ?? '', ms);
  const frame = async (ms, settling = false) => {
    const raw = await send('Page.getFrameTree', {}, ms);
    const f = raw?.frameTree?.frame;
    if (!f || !reference(f.id) || typeof f.loaderId !== 'string' || !f.loaderId && !(settling && f.url === 'about:blank') ||
        f.url !== INSTAGRAM_DIAGNOSTIC_HOME && !(settling && f.url === 'about:blank') || f.parentId !== undefined) throw failure();
    return {id: f.id, loaderId: f.loaderId, url: f.url};
  };
  try {
    if (typeof transport?.createDiagnosticScope !== 'function') { cause('scope_unavailable'); return validateInstagramReaderDiagnostic(result); }
    scope = transport.createDiagnosticScope();
    if (!scope || !['call', 'on', 'reserveTarget', 'reserveSession', 'releaseTarget', 'releaseSession', 'alive', 'cleanupVerified'].every(k => typeof scope[k] === 'function')) throw failure();
    unsubscribe = scope.on(event => {
      // Event handlers must not throw into the shared transport listener loop.
      try {
        if (!sessionId || event.sessionId !== sessionId) return;
        const p = event.params ?? {};
        if (event.method === 'Runtime.executionContextCreated') {
          const c = p.context;
          if (c?.auxData?.isDefault === true && reference(c.auxData.frameId)) {
            contexts.delete(c.auxData.frameId);
            if (c.auxData.frameId === expectedFrame) setupRevision++;
            if (pinned && c.auxData.frameId === pinned.id && c.uniqueId !== pinned.uniqueId) invalidate();
          }
          if (c?.auxData?.isDefault === true && reference(c.auxData.frameId) && Number.isSafeInteger(c.id) && c.id > 0 &&
              reference(c.uniqueId) && c.origin === 'https://www.instagram.com') {
            contexts.set(c.auxData.frameId, {id: c.id, uniqueId: c.uniqueId});
          }
        } else if (event.method === 'Runtime.executionContextsCleared') { setupRevision++;contexts.clear(); invalidate(); }
        else if (event.method === 'Runtime.executionContextDestroyed') {
          for (const [key, c] of contexts) if (c.id === p.executionContextId || c.uniqueId === p.executionContextUniqueId) {
            contexts.delete(key);if (key === expectedFrame) setupRevision++;
          }
          if (pinned && (p.executionContextId === pinned.contextID || p.executionContextUniqueId === pinned.uniqueId)) invalidate();
        } else if (event.method === 'Page.frameNavigated' && p.frame?.id === expectedFrame) { setupRevision++;invalidate(); }
        else if (event.method === 'Page.frameDetached' && p.frameId === expectedFrame) { setupRevision++;invalidate(); }
        else if (event.method === 'Page.navigatedWithinDocument' && p.frameId === expectedFrame) { setupRevision++;invalidate(); }
        else if (event.method === 'Network.requestWillBeSent' && pinned && p.frameId === pinned.id && p.loaderId === pinned.loaderId && p.request?.url === endpoint) {
          requestCount++;requestID=reference(p.requestId)?p.requestId:undefined;
          if (p.request.method !== 'GET' || p.redirectResponse) requestCount++;
        } else if (event.method === 'Network.responseReceived' && pinned && requestID && p.requestId === requestID && p.frameId === pinned.id && p.loaderId === pinned.loaderId && p.response?.url === endpoint) {
          if (Number.isInteger(p.response.status)) wireStatus=p.response.status;
        }
      } catch { invalidate(); }
    });
    trace.cleanup('pending');attemptedTarget = true;
    const created = await scope.call('Target.createTarget', {url: 'about:blank', background: true, forTab: false}, '', timeout());
    if (typeof created?.targetId !== 'string' || !created.targetId) throw failure();
    targetId = created.targetId;scope.reserveTarget(targetId);
    const attached = await scope.call('Target.attachToTarget', {targetId, flatten: true}, '', timeout());
    if (typeof attached?.sessionId !== 'string' || !attached.sessionId) throw failure();
    sessionId = attached.sessionId;scope.reserveSession(sessionId);
    for (const method of ['Page.enable', 'Runtime.enable', 'Network.enable']) await send(method);
    phase = 'navigation_failed';trace.note('navigation');
    const navigation = await send('Page.navigate', {url: INSTAGRAM_DIAGNOSTIC_HOME});
    if (navigation?.errorText || typeof navigation?.frameId !== 'string' || typeof navigation?.loaderId !== 'string' || !navigation.frameId || !navigation.loaderId) throw failure();
    expectedFrame = navigation.frameId;expectedLoader = navigation.loaderId;
    phase = 'context_unavailable';trace.note('document');
    // Page.navigate acknowledges a navigation; its expected document may not
    // have committed yet. Observe candidates until that exact loader is ready.
    // The evidence pin is immutable only after both frame checks agree.
    while (now() < setupUntil) {
      const before = await frame(timeout(), true), currentContext = contexts.get(before.id);
      if (before.id !== expectedFrame) { cause('document_changed'); throw failure(); }
      if (before.url === INSTAGRAM_DIAGNOSTIC_HOME && before.loaderId === expectedLoader && currentContext) {
        const candidate = {...before, uniqueId: currentContext.uniqueId, contextID: currentContext.id}, revision = setupRevision;
        let ready;
        try {
          ready = await send('Runtime.evaluate', {expression: "location.origin==='https://www.instagram.com'&&location.pathname==='/'&&['interactive','complete'].includes(document.readyState)", uniqueContextId: candidate.uniqueId, returnByValue: true, awaitPromise: true, userGesture: false});
        } catch (error) {
          if (setupRevision === revision && contexts.get(before.id)?.uniqueId === candidate.uniqueId) throw error;
        }
        if (ready?.exceptionDetails && setupRevision === revision && contexts.get(before.id)?.uniqueId === candidate.uniqueId) throw failure();
        if (ready?.result?.value === true) {
          const confirmed = await frame(timeout(), true), context = contexts.get(confirmed.id);
          if (confirmed.id !== expectedFrame) { cause('document_changed'); throw failure(); }
          if (setupRevision === revision && confirmed.url === INSTAGRAM_DIAGNOSTIC_HOME && confirmed.loaderId === expectedLoader && context?.uniqueId === candidate.uniqueId) {
            pinned = candidate;break;
          }
        }
      }
      await pause(25);
    }
    if (!pinned || invalidated) throw failure();
    for (const [method, params] of [['Network.setCacheDisabled', {cacheDisabled: true}], ['Network.setBypassServiceWorker', {bypass: true}]]) await send(method, params);
    // Instrumentation awaits can observe a new document. Never dispatch the
    // endpoint expression after the frozen frame/context has been invalidated.
    const admitted = await frame(timeout()), admittedContext = contexts.get(admitted.id);
    if (invalidated || admitted.id !== pinned.id || admitted.loaderId !== pinned.loaderId || admittedContext?.uniqueId !== pinned.uniqueId) {
      invalidate();throw failure();
    }
    phase = 'evaluation_failed';trace.note('identity');
    let evaluation;
    try {
      evaluation = await send('Runtime.evaluate', {expression: instagramReaderDiagnosticExpression(request.nonce), uniqueContextId: pinned.uniqueId, returnByValue: true, awaitPromise: true, userGesture: false}, Math.max(1,Math.min(6000,acquisitionUntil-now())));
      if (evaluation?.exceptionDetails) throw failure();
      const value = evaluation?.result?.value;
      if (!record(value, pageKeys) || value.nonce !== request.nonce || !fieldsValid(value) || typeof value.homeAfter !== 'boolean' || Buffer.byteLength(JSON.stringify(value), 'utf8') > 1024) {
        cause('projection_invalid');throw failure();
      }
      for (const key of ['status', 'mime', 'bodyClass', 'errorCategory']) result[key]=value[key];
      cause(value.primaryCause);
      if (!value.homeAfter) invalidate();
    } catch (error) { cause('evaluation_failed');throw error; }
    finally {
      // Even HTTP rejection, malformed projection and evaluation failure must
      // attempt a final document check before retiring the owned target.
      try {
        const after = await frame(1000), context = contexts.get(after.id);
        result.documentBound = !invalidated && after.id === pinned.id && after.loaderId === pinned.loaderId && context?.uniqueId === pinned.uniqueId;
        if (!result.documentBound) invalidate();
      } catch { invalidate(); }
    }
    result.wireMatched=requestCount===1&&wireStatus===result.status&&result.status!==0;
    if (!result.wireMatched) cause('wire_unmatched');
  } catch { cause(phase); }
  finally {
    trace.note('cleanup');
    // Cleanup has its own budget and never replaces an earlier primary cause.
    if (attemptedTarget && !targetId) result.cleanup='uncertain';
    if (targetId) {
      result.cleanup='uncertain';const until=now()+3000;
      try {
        try { await scope.call('Target.closeTarget', {targetId}, '', 1000); } catch {}
        while (now()<until) {
          const reply=await scope.call('Target.getTargets', {}, '', Math.max(1,until-now()));
          if (!Array.isArray(reply?.targetInfos)) throw failure();
          if (!reply.targetInfos.some(t=>t?.targetId===targetId)) {
            if (sessionId) scope.releaseSession(sessionId);scope.releaseTarget(targetId);result.cleanup='verified';break;
          }
          await pause(25);
        }
      } catch { /* The target pin remains retained when absence is unverified. */ }
      if (result.cleanup!=='verified') cause('cleanup_failed');
    }
    if (attemptedTarget) {
      try { if (scope.cleanupVerified() !== true) result.cleanup='uncertain'; }
      catch { result.cleanup='uncertain'; }
      if (result.cleanup==='uncertain') cause('cleanup_failed');
    }
    try { unsubscribe?.(); } catch {}
    try { scope?.dispose?.(); } catch {}
  }
  trace.cleanup(result.cleanup,result.cleanup==='uncertain'?'cleanup_failed':'none');
  return validateInstagramReaderDiagnostic(result);
}

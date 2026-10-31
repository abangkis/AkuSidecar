import {createIGTrace,attachIGTrace} from './instagram-diagnostic-trace.mjs';
import {createHmac} from 'node:crypto';
import {nativeSourceFailure} from './native-source-kernel.mjs';
import {markSourceTransient} from './source-retry-policy.mjs';
import {acquireOwnedInstagramIdentity} from './instagram-bootstrap-corroboration.mjs';
import {INSTAGRAM_CORROBORATION_POLICY,INSTAGRAM_CORROBORATION_READY} from './instagram-bootstrap-corroboration-policy.mjs';
import {createInstagramDocumentWatch,instagramDocumentFrameCondition} from './instagram-document-watch.mjs';

export const INSTAGRAM_NATIVE_RECIPE = Object.freeze({source: 'instagram', recipe: 'instagram-native-v1', nativeQualified: false,
  home: 'https://www.instagram.com/', evidenceMaxAgeMS: 6000, responseMaxBytes: INSTAGRAM_CORROBORATION_POLICY.maxBodyBytes, projectionMaxBytes: 4096,
  requiredCookies: Object.freeze(['csrftoken', 'ds_user_id', 'sessionid']), optionalCookies: Object.freeze([])});

// Finite HTTP contract: integers 100..599 except 200. Preserve familiar auth
// aliases; every other rejected status has an exact, bounded diagnostic.
// Keep this contract synchronized with source_native_failure.go.
export const INSTAGRAM_HTTP_REJECTIONS = Object.freeze(Array.from({length: 500}, (_, index) => index + 100)
  .filter(status => ![200, 401, 403, 429].includes(status)).map(status => `http_status_${status}`));

// Closed, value-free reasons only. Neither native exception text nor server
// messages, URLs, response fields or account values are diagnostic material.
export const INSTAGRAM_IDENTITY_REJECTIONS = Object.freeze([
  ...INSTAGRAM_HTTP_REJECTIONS,
  'page_scope', 'login_visible', 'http_unauthorized', 'http_forbidden', 'http_rate_limited',
  'http_server', 'http_status', 'response_origin', 'response_mime', 'body_size', 'body_missing',
  'body_read', 'body_encoding', 'body_json', 'response_shape', 'account_missing',
  'account_anonymous', 'account_type', 'account_format', 'page_changed', 'fetch_timeout', 'fetch_failed',
]);
export function instagramIdentityFailure(role, reason) {
  const owner = ['reader', 'collector'].includes(role) ? role : 'unknown';
  const category = INSTAGRAM_IDENTITY_REJECTIONS.includes(reason) ? reason : 'projection';
  const error = nativeSourceFailure('evidence', `${owner}_identity_${category}`);
  // Fetch TypeError and generic server rejection can also hide login redirects
  // or challenges. They remain terminal until a more precise source exists.
  return markSourceTransient(error, category === 'fetch_timeout' ? 'timeout' : '');
}

// Legacy endpoint diagnostic/fixture helper, never the production identity
// acquisition. Read one bounded same-origin response and project account/login.
// The body and its account data never leave the page as a raw response.
export function instagramAccountExpression(nonce) {
  if (!/^[a-f0-9]{48}$/u.test(nonce)) throw nativeSourceFailure('evidence', 'nonce');
  return `(async()=>{
    const out={nonce:${JSON.stringify(nonce)},authenticated:false,accountId:'',rejection:'none'};
    const reject=rejection=>({...out,rejection});
    if(location.origin!=='https://www.instagram.com'||location.pathname!=='/')return reject('page_scope');
    if(document.querySelector('input[type="password"],form[action*="login"]'))return reject('login_visible');
    const controller=new AbortController(),timer=setTimeout(()=>controller.abort(),4000);
    let reader,phase='fetch';
    try{
      const response=await fetch('/api/v1/accounts/current_user/?edit=true',{method:'GET',credentials:'same-origin',cache:'no-store',redirect:'error',signal:controller.signal,headers:{Accept:'application/json'}});
      const status=response.status;
      if(status!==200)return reject(status===401?'http_unauthorized':status===403?'http_forbidden':status===429?'http_rate_limited':Number.isInteger(status)&&status>=100&&status<=599?'http_status_'+status:'http_status');
      if(new URL(response.url).origin!==location.origin)return reject('response_origin');
      if(!/^application\\/json(?:;|$)/i.test(response.headers.get('content-type')||''))return reject('response_mime');
      if(Number(response.headers.get('content-length'))>65536)return reject('body_size');
      if(!response.body)return reject('body_missing');
      phase='body';
      reader=response.body.getReader();const chunks=[];let size=0;
      while(true){const part=await reader.read();if(part.done)break;size+=part.value.byteLength;if(size>65536)return reject('body_size');chunks.push(part.value);}
      const bytes=new Uint8Array(size);let offset=0;for(const chunk of chunks){bytes.set(chunk,offset);offset+=chunk.byteLength;}
      let text,value;
      try{text=new TextDecoder('utf-8',{fatal:true}).decode(bytes);}catch{return reject('body_encoding');}finally{bytes.fill(0);}
      try{value=JSON.parse(text);}catch{return reject('body_json');}finally{text='';}
      if(value?.status!=='ok')return reject('response_shape');
      if(!value.user||typeof value.user!=='object'||Array.isArray(value.user))return reject('account_missing');
      if(value.user.is_anonymous===true)return reject('account_anonymous');
      const pk=value.user.pk;if(!(typeof pk==='string'||Number.isSafeInteger(pk)))return reject('account_type');
      const id=String(pk);if(!/^[1-9][0-9]{0,29}$/.test(id))return reject('account_format');
      if(location.origin!=='https://www.instagram.com'||location.pathname!=='/'||document.querySelector('input[type="password"],form[action*="login"]'))return reject('page_changed');
      return {...out,authenticated:true,accountId:id};
    }catch{return reject(controller.signal.aborted?'fetch_timeout':phase==='body'?'body_read':'fetch_failed');}finally{clearTimeout(timer);try{await reader?.cancel();}catch{}}
  })()`;
}

export function projectInstagramCookies(reply, nowMS) {
  if (!reply || !Array.isArray(reply.cookies) || reply.cookies.length > 128) throw nativeSourceFailure('session', 'projection');
  const selected = new Map();
  for (const cookie of reply.cookies) {
    if (!cookie || typeof cookie !== 'object') throw nativeSourceFailure('session', 'projection');
    // Ordinary preferences and analytics are not required authentication data
    // and confer no transfer authority. Known auth names on another scope fail.
    if (!INSTAGRAM_NATIVE_RECIPE.requiredCookies.includes(cookie.name)) continue;
    if (selected.has(cookie.name) || !['.instagram.com', 'instagram.com', 'www.instagram.com'].includes(cookie.domain) ||
        cookie.path !== '/' || cookie.secure !== true || typeof cookie.httpOnly !== 'boolean' ||
        cookie.name === 'sessionid' && cookie.httpOnly !== true ||
        typeof cookie.value !== 'string' || !cookie.value || Buffer.byteLength(cookie.value, 'utf8') > 4096 || /[\u0000-\u0020\u007f]/u.test(cookie.value) ||
        cookie.partitionKey !== undefined || cookie.partitionKeyOpaque === true ||
        !['Strict', 'Lax', 'None', undefined].includes(cookie.sameSite) ||
        typeof cookie.expires !== 'number' || !Number.isFinite(cookie.expires) || cookie.expires !== -1 && cookie.expires * 1000 <= nowMS) throw nativeSourceFailure('session', 'scope');
    selected.set(cookie.name, {name: cookie.name, value: cookie.value, domain: cookie.domain, path: '/', secure: true, httpOnly: cookie.httpOnly,
      ...(cookie.sameSite === undefined ? {} : {sameSite: cookie.sameSite}), ...(cookie.expires === -1 ? {} : {expires: cookie.expires})});
  }
  if (selected.size !== INSTAGRAM_NATIVE_RECIPE.requiredCookies.length || !/^[1-9][0-9]{0,29}$/u.test(selected.get('ds_user_id').value)) throw nativeSourceFailure('session', 'required');
  return INSTAGRAM_NATIVE_RECIPE.requiredCookies.map(name => selected.get(name));
}

export async function readInstagramEvidence(page, attempt, role, ownerID, profileID, secret, now = Date.now, nonce = attempt.binding.nonce) {
  let cookies;const trace=createIGTrace();trace.disposition('started');trace.note('verifier');
  try {
    const rejectDocument=condition=>{trace.fail('post_check',condition);throw nativeSourceFailure('evidence','document_changed');};
    const frame = async () => {
      let value;
      try { value = await attempt.run('evidence', ms => page.send('Page.getFrameTree', {}, ms)); }
      catch(error) { trace.fail('post_check','document_frame_request_failed');throw error; }
      const root = value?.frameTree?.frame;
      if (!root || typeof root.id !== 'string' || !root.id || typeof root.loaderId !== 'string' || !root.loaderId ||
          root.parentId !== undefined) {trace.fail('post_check','document_frame_invalid');throw nativeSourceFailure('evidence', 'document');}
      if(root.url!==INSTAGRAM_NATIVE_RECIPE.home){trace.fail('post_check','document_url_mismatch');throw nativeSourceFailure('evidence','document');}
      return {id: root.id, loaderId: root.loaderId, url: root.url};
    };
    const documentWatch=createInstagramDocumentWatch(page,condition=>{try{trace.fail(trace.currentStage(),condition);}catch{try{trace.incomplete();}catch{}}});
    let projection,readFailure;
    try {
      // Install the verifier observer at binding, before acquisition cleanup or
      // any promise handoff. The original server time remains the proof time.
      projection = await attempt.run('evidence', ms => acquireOwnedInstagramIdentity(page,nonce,{trace,now,budgetMS:Math.min(30000,ms-3000),onDocumentBound:documentWatch.bind}));
      if(projection.diagnostic.reason!=='corroborated'||projection.diagnostic.cleanup!=='verified'){
        const owner=['reader','collector'].includes(role)?role:'unknown';
        throw nativeSourceFailure('evidence',owner+'_bootstrap_'+projection.diagnostic.reason);
      }
      const before=projection.documentBinding;
      const checkDocument=()=>{const condition=documentWatch.condition();if(condition)rejectDocument(condition);};
      trace.note('post_check');checkDocument();
      trace.note('cookie');cookies = projectInstagramCookies(await attempt.run('session', ms => page.send('Network.getCookies', {urls: [INSTAGRAM_NATIVE_RECIPE.home]}, ms)), now());
      trace.note('post_check');const after = await frame();
      checkDocument();const frameCondition=instagramDocumentFrameCondition(after,before);if(frameCondition)rejectDocument(frameCondition);
      let context;
      try { context=await attempt.run('evidence',ms=>page.send('Runtime.evaluate',{expression:INSTAGRAM_CORROBORATION_READY,uniqueContextId:before.uniqueContextId,returnByValue:true,awaitPromise:true,userGesture:false},ms)); }
      catch(error) {trace.fail('post_check','document_evaluation_request_failed');throw error;}
      checkDocument();
      if(context?.exceptionDetails)rejectDocument('document_evaluation_exception');
      if(context?.result?.value!==true)rejectDocument('document_not_ready');
      if (cookies.find(cookie => cookie.name === 'ds_user_id').value !== projection.accountId) throw nativeSourceFailure('evidence', 'account_mismatch');
      const digest = value => createHmac('sha256', secret).update(value).digest('hex');
      const observedAtMS = projection.observedAtMS;
      const age=now()-observedAtMS;
      if(!Number.isSafeInteger(observedAtMS)||age<0||age>INSTAGRAM_NATIVE_RECIPE.evidenceMaxAgeMS)throw nativeSourceFailure('evidence','stale');
      const proof = Object.freeze({source: 'instagram', domain: 'native-browser', recipe: INSTAGRAM_NATIVE_RECIPE.recipe,
        nonce, profileScope: attempt.binding.profileScope, generation: attempt.binding.generation,
        role, ownerID, profileID, accountKey: digest('account\0' + projection.accountId),
        sessionRevision: digest('session\0' + JSON.stringify(cookies)), authenticated: true, observedAtMS});
      trace.done('verifier');trace.disposition('completed');return {proof, cookies};
    } catch(error) { readFailure=error;throw error; }
    finally {
      if(projection)projection.accountId='';
      try { documentWatch.dispose(); } catch {
        trace.cleanup('uncertain','dispose_failed');
        if(!readFailure)throw nativeSourceFailure('cleanup','unverified');
      }
    }
  } catch (error) { if (cookies) eraseInstagramCookies(cookies);try{trace.fail(trace.currentStage(),'request_failed');}catch{try{trace.incomplete();}catch{}}throw attachIGTrace(error,trace); }
}
export function eraseInstagramCookies(cookies) { for (const cookie of cookies ?? []) cookie.value = ''; if (Array.isArray(cookies)) cookies.length = 0; }

export function verifyInstagramPair(reader, collector, binding, nowMS) {
  for (const [role, proof] of [['reader', reader], ['collector', collector]]) {
    if (!proof || proof.source !== 'instagram' || proof.domain !== 'native-browser' || proof.recipe !== INSTAGRAM_NATIVE_RECIPE.recipe ||
        proof.role !== role || proof.nonce !== binding.nonce || proof.generation !== binding.generation || proof.profileScope !== binding.profileScope ||
        proof.authenticated !== true || !/^[a-f0-9]{64}$/u.test(proof.accountKey) || !/^[a-f0-9]{64}$/u.test(proof.sessionRevision) ||
        !Number.isSafeInteger(proof.observedAtMS) || proof.observedAtMS > nowMS || nowMS - proof.observedAtMS > 6000) throw nativeSourceFailure('evidence', 'stale');
  }
  if (reader.ownerID === collector.ownerID || reader.profileID === collector.profileID || reader.accountKey !== collector.accountKey || reader.sessionRevision !== collector.sessionRevision) throw nativeSourceFailure('evidence', 'pair');
  return Object.freeze({reader, collector});
}

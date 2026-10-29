import {createHmac} from 'node:crypto';
import {nativeSourceFailure, nativeRecord} from './native-source-kernel.mjs';
import {markSourceTransient} from './source-retry-policy.mjs';

export const INSTAGRAM_NATIVE_RECIPE = Object.freeze({source: 'instagram', recipe: 'instagram-native-v1', nativeQualified: false,
  home: 'https://www.instagram.com/', evidenceMaxAgeMS: 6000, responseMaxBytes: 65536, projectionMaxBytes: 4096,
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

// Candidate endpoint, not a claim of a stable/documented Instagram API. Read
// one bounded same-origin response and return only the account/login projection.
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
  let cookies;
  try {
    for (const [method, values] of [['Network.setCacheDisabled', {cacheDisabled: true}], ['Network.setBypassServiceWorker', {bypass: true}]]) {
      await attempt.run('evidence', ms => page.send(method, values, ms));
    }
    const frame = async () => {
      const value = await attempt.run('evidence', ms => page.send('Page.getFrameTree', {}, ms));
      const root = value?.frameTree?.frame;
      if (!root || typeof root.id !== 'string' || !root.id || typeof root.loaderId !== 'string' || !root.loaderId ||
          root.url !== INSTAGRAM_NATIVE_RECIPE.home || root.parentId !== undefined) throw nativeSourceFailure('evidence', 'document');
      return {id: root.id, loaderId: root.loaderId, url: root.url};
    };
    const before = await frame();
    const projection = await attempt.run('evidence', ms => page.evaluate(instagramAccountExpression(nonce), ms));
    if (!nativeRecord(projection, ['nonce', 'authenticated', 'accountId', 'rejection']) || projection.nonce !== nonce ||
        Buffer.byteLength(JSON.stringify(projection), 'utf8') > INSTAGRAM_NATIVE_RECIPE.projectionMaxBytes) throw instagramIdentityFailure(role, 'projection');
    if (projection.authenticated === false && projection.accountId === '' && INSTAGRAM_IDENTITY_REJECTIONS.includes(projection.rejection)) throw instagramIdentityFailure(role, projection.rejection);
    if (projection.authenticated !== true || projection.rejection !== 'none' || typeof projection.accountId !== 'string' ||
        !/^[1-9][0-9]{0,29}$/u.test(projection.accountId)) throw instagramIdentityFailure(role, 'projection');
    cookies = projectInstagramCookies(await attempt.run('session', ms => page.send('Network.getCookies', {urls: [INSTAGRAM_NATIVE_RECIPE.home]}, ms)), now());
    const after = await frame();
    if (before.id !== after.id || before.loaderId !== after.loaderId || before.url !== after.url) throw nativeSourceFailure('evidence', 'document_changed');
    if (cookies.find(cookie => cookie.name === 'ds_user_id').value !== projection.accountId) throw nativeSourceFailure('evidence', 'account_mismatch');
    const digest = value => createHmac('sha256', secret).update(value).digest('hex');
    const observedAtMS = now();
    const proof = Object.freeze({source: 'instagram', domain: 'native-browser', recipe: INSTAGRAM_NATIVE_RECIPE.recipe,
      nonce, profileScope: attempt.binding.profileScope, generation: attempt.binding.generation,
      role, ownerID, profileID, accountKey: digest('account\0' + projection.accountId),
      sessionRevision: digest('session\0' + JSON.stringify(cookies)), authenticated: true, observedAtMS});
    return {proof, cookies};
  } catch (error) { if (cookies) eraseInstagramCookies(cookies); throw error; }
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

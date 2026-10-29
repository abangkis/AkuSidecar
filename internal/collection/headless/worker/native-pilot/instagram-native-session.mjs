import {createHmac} from 'node:crypto';
import {nativeSourceFailure, nativeRecord} from './native-source-kernel.mjs';

export const INSTAGRAM_NATIVE_RECIPE = Object.freeze({source: 'instagram', recipe: 'instagram-native-v1', nativeQualified: false,
  home: 'https://www.instagram.com/', evidenceMaxAgeMS: 6000, responseMaxBytes: 65536, projectionMaxBytes: 4096,
  requiredCookies: Object.freeze(['csrftoken', 'ds_user_id', 'sessionid']), optionalCookies: Object.freeze([])});

// Candidate endpoint, not a claim of a stable/documented Instagram API. Read
// one bounded same-origin response and return only the account/login projection.
// The body and its account data never leave the page as a raw response.
export function instagramAccountExpression(nonce) {
  if (!/^[a-f0-9]{48}$/u.test(nonce)) throw nativeSourceFailure('evidence', 'nonce');
  return `(async()=>{
    const out={nonce:${JSON.stringify(nonce)},authenticated:false,accountId:''};
    if(location.origin!=='https://www.instagram.com'||location.pathname!=='/'||document.querySelector('input[type="password"],form[action*="login"]'))return out;
    const controller=new AbortController(),timer=setTimeout(()=>controller.abort(),4000);
    let reader;
    try{
      const response=await fetch('/api/v1/accounts/current_user/?edit=true',{method:'GET',credentials:'same-origin',cache:'no-store',redirect:'error',signal:controller.signal,headers:{Accept:'application/json'}});
      if(response.status!==200||new URL(response.url).origin!==location.origin||!/^application\\/json(?:;|$)/i.test(response.headers.get('content-type')||''))return out;
      if(Number(response.headers.get('content-length'))>65536||!response.body)return out;
      reader=response.body.getReader();const chunks=[];let size=0;
      while(true){const part=await reader.read();if(part.done)break;size+=part.value.byteLength;if(size>65536)return out;chunks.push(part.value);}
      const bytes=new Uint8Array(size);let offset=0;for(const chunk of chunks){bytes.set(chunk,offset);offset+=chunk.byteLength;}
      const value=JSON.parse(new TextDecoder('utf-8',{fatal:true}).decode(bytes));bytes.fill(0);
      if(value?.status!=='ok'||!value.user||value.user.is_anonymous===true)return out;
      const pk=value.user.pk;if(!(typeof pk==='string'||Number.isSafeInteger(pk)))return out;
      const id=String(pk);if(!/^[1-9][0-9]{0,29}$/.test(id))return out;
      if(location.origin!=='https://www.instagram.com'||location.pathname!=='/'||document.querySelector('input[type="password"],form[action*="login"]'))return out;
      return {nonce:out.nonce,authenticated:true,accountId:id};
    }catch{return out;}finally{clearTimeout(timer);try{await reader?.cancel();}catch{}}
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
    if (!nativeRecord(projection, ['nonce', 'authenticated', 'accountId']) || projection.nonce !== nonce ||
        projection.authenticated !== true || !/^[1-9][0-9]{0,29}$/u.test(projection.accountId) ||
        Buffer.byteLength(JSON.stringify(projection), 'utf8') > INSTAGRAM_NATIVE_RECIPE.projectionMaxBytes) throw nativeSourceFailure('evidence', 'identity');
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

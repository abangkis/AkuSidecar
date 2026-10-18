import {createContext, runInContext} from 'node:vm';
import {canonicalSourceURL} from './observation.mjs';
import {resolveStructuredMedia} from './structured-media.mjs';

const installations = new WeakMap();
const stores = new WeakMap();
const MAX_RESPONSE_BYTES = 256 * 1024;
const responsePath = 'AkuBridge/x-response-evidence-adapter.js';
const runtimePath = 'AkuBridge/x-media-evidence-runtime.js';

function storeFor(assets) {
  if (stores.has(assets)) return stores.get(assets);
  const runtime = assets.find(asset => asset.relative === runtimePath);
  if (typeof runtime?.content !== 'string') return null;
  const context = createContext({URL});
  runInContext(runtime.content, context, {timeout: 1000, filename: runtimePath});
  const api = context.AkuXMediaEvidence;
  if (!api?.validResponseEvidenceEnvelope) return null;
  const store = {api, avatars: api.createAvatarCache(), media: api.createCache()};
  stores.set(assets, store);
  return store;
}

// Registration is scoped to an already-owned source/temporary target. Borrowed
// pages retain their own extension and lifecycle; never install new-document
// hooks or a second evidence bridge on those pages.
export async function prepareXPassiveEvidence({browser, page, assets, deadlineAt}) {
  if (browser?.backend !== 'headless_worker' || typeof page?.send !== 'function') return 'not_applicable';
  const scripts = [responsePath, runtimePath].map(relative => assets?.find(asset => asset.relative === relative));
  if (scripts.some(asset => typeof asset?.content !== 'string')) return 'unavailable';
  const previous = installations.get(page);
  if (previous) return previous.identifier ? 'registered' : 'unavailable';
  const left = deadlineAt - Date.now();
  if (left <= 0) return 'deadline';
  const record = {identifier: null};
  installations.set(page, record);
  let timer;
  try {
    const timeoutMs = Math.min(left, 1500);
    const operation = Promise.resolve(page.send('Page.addScriptToEvaluateOnNewDocument',
      {source: scripts.map(asset => asset.content).join('\n;\n')}, timeoutMs))
      .then(result => { if (typeof result?.identifier === 'string' && result.identifier) record.identifier = result.identifier; return result; });
    const result = await Promise.race([operation,
      new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('Observer registration deadline')), timeoutMs); })]);
    if (typeof result?.identifier !== 'string' || !result.identifier) return 'unavailable';
    return 'registered';
  } catch { return 'unavailable'; }
  finally { clearTimeout(timer); }
}

// Only sanitized shared-runtime evidence for requested native owners crosses
// CDP. No response body, DOM text, cookies, or author profile is returned.
export function readXPassiveEvidenceInPage(owners) {
  if (globalThis.location?.origin !== 'https://x.com') return null;
  const runtime = globalThis.AkuXMediaEvidenceRuntime;
  if (!runtime?.lookup) return null;
  const candidates = [];
  for (const owner of owners.slice(0, 16)) {
    const media = (runtime.lookup(owner.candidateId) || [])
      .filter(item => item.provenance === 'x_response_graphql').slice(0, 4)
      .map(({kind, url, posterUrl, playbackUrl, width, height, provenance}) =>
        ({kind, url, posterUrl, playbackUrl, width, height, provenance}));
    const avatarUrl = owner.keys.map(key => runtime.lookupAvatar?.(key)).find(Boolean);
    const replyToId = runtime.lookupReplyTo?.(owner.candidateId);
    candidates.push({candidateId: owner.candidateId, media,
      ...(avatarUrl ? {avatarUrl} : {}), ...(replyToId ? {replyToId} : {})});
  }
  return {type: 'AKU_X_RESPONSE_MEDIA_EVIDENCE', runtimeRevision: 'x-response-evidence-v2', candidates};
}

export function mergeXReplyDirectContext(post, incomingRelations) {
  const incoming = (Array.isArray(incomingRelations) ? incomingRelations : [])
    .filter(relation => relation?.kind === 'replies_to');
  if (!incoming.length) return post;
  const ownerId = statusIdForPost(post);
  if (!ownerId) return post;
  const directContext = Array.isArray(post.directContext) ? post.directContext : [];
  const existing = directContext.filter(relation => relation?.kind === 'replies_to');
  const retained = directContext.filter(relation => relation?.kind !== 'replies_to');
  if (incoming.length !== 1) {
    const validIncomingIds = incoming.map(relation => exactReplyIdentity(relation, ownerId)).filter(Boolean);
    const existingId = existing.length === 1 ? exactReplyIdentity(existing[0], ownerId) : null;
    const hasMalformed = validIncomingIds.length < incoming.length;
    if (existingId && (!validIncomingIds.length
        || (hasMalformed && validIncomingIds.length === 1 && validIncomingIds[0] === existingId))) return post;
    return markReplyContextAmbiguous(post, retained);
  }
  const incomingId = exactReplyIdentity(incoming[0], ownerId);
  if (!incomingId) {
    const existingId = existing.length === 1 ? exactReplyIdentity(existing[0], ownerId) : null;
    return existingId ? post : markReplyContextAmbiguous(post, retained);
  }
  if (conflictingReplyParentPermalink(post, incomingId)) return markReplyContextAmbiguous(post, retained);
  if (existing.length > 1) return markReplyContextAmbiguous(post, retained);
  if (existing.length === 1) {
    const existingId = exactReplyIdentity(existing[0], ownerId);
    if (existingId === incomingId) return withResolvedReplySemantics(post, incoming[0]);
    if (!existingId && isUnresolvedDomReply(existing[0])) {
      const relation = {...incoming[0],
        ...(typeof existing[0].observedText === 'string' && existing[0].observedText.length <= 500
          ? {observedText: existing[0].observedText} : {}),
      };
      return withResolvedReplySemantics({...post, directContext: [...retained, relation]}, relation);
    }
    if (existingId) return markReplyContextAmbiguous(post, retained);
    return withResolvedReplySemantics({...post, directContext: [...retained, incoming[0]]}, incoming[0]);
  }
  if (directContext.length >= 8) return post;
  return withResolvedReplySemantics({...post, directContext: [...directContext, incoming[0]]}, incoming[0]);
}

export function markXReplyContextAmbiguous(post) {
  const directContext = Array.isArray(post?.directContext) ? post.directContext : [];
  return markReplyContextAmbiguous(post, directContext.filter(relation => relation?.kind !== 'replies_to'));
}

function responseReplyRelation(ownerId, replyToId) {
  if (!/^\d{5,30}$/.test(replyToId || '') || replyToId === ownerId) return null;
  return {kind: 'replies_to', provenance: 'observed_response', capturedAt: new Date().toISOString(),
    target: {kind: 'post', id: replyToId, permalink: `https://x.com/i/status/${replyToId}`,
      text: '', author: '', availability: 'reference_only'}};
}

function markReplyContextAmbiguous(post, retained) {
  const clearReplyParent = post?.relationshipType === 'reply' && !isQuotePost(post);
  return {...post, directContext: retained,
    presentation: {...(post.presentation && typeof post.presentation === 'object' ? post.presentation : {}),
      directContextStatus: 'ambiguous'},
    ...(clearReplyParent ? {parentPermalink: ''} : {})};
}

function withResolvedReplySemantics(post, relation) {
  if (isQuotePost(post)) return post;
  return {...post, relationshipType: 'reply', parentPermalink: relation.target.permalink};
}

function isQuotePost(post) {
  return post?.relationshipType === 'quote' || post?.quotedPostObserved === true
    || Boolean(post?.quotedPost) || (Array.isArray(post?.directContext)
      && post.directContext.some(relation => relation?.kind === 'quotes'));
}

function statusIdForPost(post) {
  const id = /^(?:x:status:)?(\d{5,30})$/.exec(post?.id || '')?.[1];
  const permalink = canonicalSourceURL('x', post?.permalink);
  const permalinkId = permalink && new URL(permalink).pathname.match(/\/status\/(\d+)$/)?.[1];
  return id && id === permalinkId ? id : null;
}

function conflictingReplyParentPermalink(post, replyToId) {
  if (post?.relationshipType !== 'reply' || !post.parentPermalink) return false;
  const permalink = canonicalSourceURL('x', post.parentPermalink);
  const permalinkId = permalink && new URL(permalink).pathname.match(/\/status\/(\d+)$/)?.[1];
  return permalinkId !== replyToId;
}

function exactReplyIdentity(relation, ownerId) {
  if (!relation || relation.kind !== 'replies_to'
      || !['observed_dom', 'observed_response'].includes(relation.provenance)
      || !isCanonicalCaptureTime(relation.capturedAt)) return null;
  const target = relation.target;
  if (!target || target.kind !== 'post' || typeof target.id !== 'string'
      || !/^\d{5,30}$/.test(target.id) || target.id === ownerId
      || typeof target.permalink !== 'string') return null;
  const permalink = canonicalSourceURL('x', target.permalink);
  const permalinkId = permalink && new URL(permalink).pathname.match(/\/status\/(\d+)$/)?.[1];
  if (!permalinkId || permalinkId !== target.id) return null;
  if (!['captured', 'reference_only'].includes(target.availability)
      || typeof target.text !== 'string' || target.text.length > 4000
      || typeof target.author !== 'string' || target.author.length > 300) return null;
  const hasMedia = target.hasMedia === true;
  if (target.availability === 'reference_only' && (target.text || hasMedia)) return null;
  // The adapter can know the exact reply edge from a response while getting the
  // bounded parent body from the one matching DOM article. Response-created
  // relations themselves stay reference-only with an empty body.
  if (target.availability === 'captured' && (!target.text && !hasMedia)) return null;
  if (typeof relation.observedText === 'string' && relation.observedText.length > 500) return null;
  return target.id;
}

function isUnresolvedDomReply(relation) {
  const target = relation?.target;
  return relation?.kind === 'replies_to' && relation.provenance === 'observed_dom'
    && isCanonicalCaptureTime(relation.capturedAt) && target?.kind === 'post'
    && target.id === '' && target.permalink === '' && target.availability === 'reference_only'
    && typeof target.text === 'string' && !target.text && typeof target.author === 'string' && !target.author
    && (!relation.observedText || typeof relation.observedText === 'string' && relation.observedText.length <= 500);
}

function isCanonicalCaptureTime(value) {
  if (typeof value !== 'string') return false;
  const timestamp = Date.parse(value);
  return Number.isFinite(timestamp) && new Date(timestamp).toISOString() === value;
}

function ownerFor(post, api) {
  const id = /^(?:x:status:)?(\d{5,30})$/.exec(post?.id || '')?.[1];
  const permalink = canonicalSourceURL('x', post?.permalink);
  if (!id || !permalink || new URL(permalink).pathname.match(/\/status\/(\d+)$/)?.[1] !== id) return null;
  const candidateId = `x:status:${id}`;
  const keys = [candidateId, ...(Array.isArray(post.avatarEvidenceKeys) ? post.avatarEvidenceKeys : [])]
    .map(key => api.normalizeAvatarKey(key))
    .filter(key => key === candidateId || key?.startsWith('x:user:'));
  return {post, candidateId, keys: [...new Set(keys)].slice(0, 2)};
}

export async function resolveXPassiveEvidence({browser, page, assets, posts, deadlineAt}) {
  if (browser?.backend !== 'headless_worker' || !Array.isArray(assets) || !installations.get(page)?.identifier) return posts;
  let store;
  try { store = storeFor(assets); } catch { return posts; }
  if (!store) return posts;
  const bindings = (Array.isArray(posts) ? posts : []).map(post => ownerFor(post, store.api)).filter(Boolean);
  const owners = bindings.map(({candidateId, keys}) => ({candidateId, keys})).slice(0, 16);
  const requested = new Set(owners.map(owner => owner.candidateId));
  const seen = new Set();
  const ambiguous = new Set();
  const ambiguousReplyEvidence = new Set();
  for (const {candidateId} of bindings) {
    if (seen.has(candidateId)) ambiguous.add(candidateId);
    seen.add(candidateId);
  }
  const observed = new Set();
  const replyEvidence = new Map();
  if (owners.length && deadlineAt > Date.now()) {
    try {
      const result = await page.evaluate(`(${readXPassiveEvidenceInPage.toString()})(${JSON.stringify(owners)})`,
        Math.min(900, deadlineAt - Date.now()));
      if (result && Buffer.byteLength(JSON.stringify(result)) <= MAX_RESPONSE_BYTES
          && store.api.validResponseEvidenceEnvelope(result)) {
        const counts = new Map();
        for (const candidate of result.candidates) counts.set(candidate.candidateId, (counts.get(candidate.candidateId) || 0) + 1);
        for (const candidate of result.candidates) {
          if (!requested.has(candidate.candidateId) || counts.get(candidate.candidateId) < 2) continue;
          const parents = new Set(result.candidates.filter(value => value.candidateId === candidate.candidateId)
            .map(value => value.replyToId).filter(Boolean));
          if (parents.size > 1) ambiguousReplyEvidence.add(candidate.candidateId);
        }
        for (const candidate of result.candidates) {
          if (!requested.has(candidate.candidateId) || ambiguous.has(candidate.candidateId) || counts.get(candidate.candidateId) !== 1) continue;
          if (candidate.media.length || candidate.avatarUrl || candidate.replyToId) observed.add(candidate.candidateId);
          if (candidate.replyToId) replyEvidence.set(candidate.candidateId, candidate.replyToId);
          store.media.put(candidate.candidateId, candidate.media, 'x_response_graphql');
          const owner = owners.find(value => value.candidateId === candidate.candidateId);
          if (candidate.avatarUrl) for (const key of owner.keys) store.avatars.put(key, candidate.avatarUrl);
        }
      }
    } catch { /* Optional evidence cannot turn a valid DOM capture into failure. */ }
  }
  const candidates = bindings.filter(owner => !ambiguous.has(owner.candidateId))
    .map(owner => ({candidateId: owner.candidateId, media: store.media.get(owner.candidateId).map(item => ({...item,
      // The shared response parser constructs this poster/playback tuple from
      // one media entity. Preserve that proof when using its bounded cache;
      // never derive a video asset ID from a generic /media/ image path.
      ...(item.kind === 'video' && item.provenance === 'x_response_graphql'
        ? {pairing: 'same_video_media_entity'} : {}),
    }))}));
  const pending = bindings.filter(({post, candidateId}) => !ambiguous.has(candidateId)
    && post.media?.some(item => item.kind === 'video_poster')).map(({post}) => post);
  // Reuse the existing exact-post/DOM-poster/MP4 admission. Cached media never
  // becomes a new attachment and cannot supply a quote's or foreign post's URL.
  const resolution = await resolveStructuredMedia({source: 'x', posts: pending, deadlineAt,
    resolver: {available: true, runtimeRevision: 'x-main-world-media-resolver-v1', functionSource: '()=>{}'},
    page: {evaluate: async () => ({runtimeRevision: 'x-main-world-media-resolver-v1', candidates})}});
  const replacement = new Map(pending.map((post, index) => [post, resolution.posts[index]]));
  return posts.map(post => {
    const owner = bindings.find(value => value.post === post);
    if (!owner || ambiguous.has(owner.candidateId)) return post;
    let next = replacement.get(post) || post;
    const recovered = next.media?.some((item, index) => post.media?.[index]?.kind === 'video_poster'
      && item.kind === 'video' && item.playbackUrl);
    if (!recovered) next = post;
    if (ambiguousReplyEvidence.has(owner.candidateId)) next = markXReplyContextAmbiguous(next);
    const replyToId = replyEvidence.get(owner.candidateId);
    const reply = replyToId ? responseReplyRelation(owner.candidateId.slice('x:status:'.length), replyToId) : null;
    if (reply) next = mergeXReplyDirectContext(next, [reply]);
    const currentAvatar = store.api.safeXAvatarUrl(post.avatar);
    if (currentAvatar) for (const key of owner.keys) store.avatars.put(key, currentAvatar);
    const cachedAvatar = owner.keys.map(key => store.avatars.get(key)).find(Boolean);
    const avatar = currentAvatar || cachedAvatar || post.avatar;
    if (!observed.has(owner.candidateId) && !(!currentAvatar && cachedAvatar) && !recovered) return next;
    if (recovered) next = {...next, media: next.media.map((item, index) => post.media?.[index]?.kind === 'video_poster' && item.playbackUrl
      ? {...item, provenance: observed.has(owner.candidateId) ? 'x_response_graphql' : 'x_worker_session_cache'} : item)};
    return {...next, avatar, mediaEvidence: {...next.mediaEvidence, passiveResponse: {
      observerRegistered: true, evidenceSource: observed.has(owner.candidateId) ? 'observed_response' : 'worker_session_cache',
      avatarRecovered: !currentAvatar && Boolean(cachedAvatar), playbackUrlRecovered: Boolean(recovered),
    }}};
  });
}

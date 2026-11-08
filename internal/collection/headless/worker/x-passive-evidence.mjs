import {createContext, runInContext} from 'node:vm';
import {capturePrimitivesFor} from './capture-primitives.mjs';
import {MAX_SINGLE_POST_BYTES} from './observation.mjs';
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
  const observer = runtime.responseDiagnostics?.();
  const diagnostics = observer && {
    observerReady: observer.observerReady,
    fetchHookActive: observer.fetchHookActive,
    xhrHookActive: observer.xhrHookActive,
    graphqlResponseCount: observer.graphqlResponseCount,
    filteredOperationCount: observer.filteredOperationCount,
    unsuccessfulResponseCount: observer.unsuccessfulResponseCount,
    observedResponseCount: observer.observedResponseCount,
    parsedResponseCount: observer.parsedResponseCount,
    rejectedResponseCount: observer.rejectedResponseCount,
  };
  return {type: 'AKU_X_RESPONSE_MEDIA_EVIDENCE', runtimeRevision: 'x-response-evidence-v2', candidates, diagnostics};
}

function ownerFor(post, api, primitives) {
  const id = primitives.xReplyOwnerId(post);
  if (!id) return null;
  const candidateId = `x:status:${id}`;
  const keys = [candidateId, ...(Array.isArray(post.avatarEvidenceKeys) ? post.avatarEvidenceKeys : [])]
    .map(key => api.normalizeAvatarKey(key))
    .filter(key => key === candidateId || key?.startsWith('x:user:'));
  return {post, candidateId, keys: [...new Set(keys)].slice(0, 2)};
}

// Only the requested owners' parent IDs cross CDP. Use one bounded read after
// scroll/detail recovery so every retained viewport sees the same cache state.
function readFinalXReplyIdsInPage(ids) {
  if (globalThis.location?.origin !== 'https://x.com') return null;
  const runtime = globalThis.AkuXMediaEvidenceRuntime;
  if (typeof runtime?.lookupReplyTo !== 'function') return null;
  const replies = [];
  for (const id of ids) {
    const parent = runtime.lookupReplyTo('x:status:' + id);
    if (typeof parent === 'string' && /^\d{5,30}$/.test(parent)) replies.push([id, parent]);
  }
  return replies;
}

export async function finalizeXReplyEvidence({browser, page, assets, snapshots, deadlineAt, remainingBytes}) {
  if (browser?.backend !== 'headless_worker' || !installations.get(page)?.identifier || deadlineAt <= Date.now()) return;
  const primitives = capturePrimitivesFor(assets);
  const owners = snapshots.map(snapshot => primitives.xReplySnapshotOwners(snapshot.posts));
  const ids = [...new Set(owners.flatMap(indices => [...indices.keys()]))];
  if (!ids.length) return;
  let parents;
  try {
    const timeoutMs = Math.min(900, deadlineAt - Date.now());
    if (timeoutMs <= 0) return;
    parents = await page.evaluate(`(${readFinalXReplyIdsInPage.toString()})(${JSON.stringify(ids)})`, timeoutMs);
  } catch { return; }
  if (!Array.isArray(parents) || parents.length > ids.length || deadlineAt <= Date.now()) return;
  const requested = new Set(ids);
  const byId = new Map();
  for (const pair of parents) {
    if (!Array.isArray(pair) || pair.length !== 2 || !requested.has(pair[0])) return;
    const [id, parent] = pair;
    byId.set(id, byId.has(id) ? null : parent);
  }
  for (const [snapshotIndex, indices] of owners.entries()) {
    const {posts} = snapshots[snapshotIndex];
    for (const [id, index] of indices) {
      const post = posts[index];
      const merged = primitives.mergeXResponseReply(post, byId.get(id));
      if (merged === post) continue;
      const mergedBytes = Buffer.byteLength(JSON.stringify(merged));
      if (mergedBytes > MAX_SINGLE_POST_BYTES) continue;
      const delta = mergedBytes - Buffer.byteLength(JSON.stringify(post));
      if (delta > remainingBytes) continue;
      posts[index] = merged;
      remainingBytes -= delta;
    }
  }
}

export async function resolveXPassiveEvidence({browser, page, assets, posts, deadlineAt, onDiagnostics}) {
  if (browser?.backend !== 'headless_worker' || !Array.isArray(assets) || !installations.get(page)?.identifier) return posts;
  let store;
  try { store = storeFor(assets); } catch { return posts; }
  if (!store) return posts;
  const primitives = capturePrimitivesFor(assets);
  const bindings = (Array.isArray(posts) ? posts : []).map(post => ownerFor(post, store.api, primitives)).filter(Boolean);
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
        if (result.diagnostics) onDiagnostics?.(result.diagnostics);
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
    if (ambiguousReplyEvidence.has(owner.candidateId)) next = primitives.markXReplyContextAmbiguous(next);
    const replyToId = replyEvidence.get(owner.candidateId);
    if (replyToId) next = primitives.mergeXResponseReply(next, replyToId);
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

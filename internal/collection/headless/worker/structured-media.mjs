import { canonicalSourceURL } from './observation.mjs';

const MAX_CANDIDATES = 16;
const MAX_RESOLVER_RESULT_BYTES = 256 * 1024;
const MAX_DIAGNOSTIC_COUNT = 1_000_000;

export function structuredMediaRequest(source, posts) {
  const eligible = [];
  for (const post of Array.isArray(posts) ? posts : []) {
    if (!expectsVideo(post) && !nativeVideoPermalink(source, post)) continue;
    const candidateId = nativeCandidateId(source, post);
    if (candidateId) eligible.push({ post, candidateId });
  }
  const candidateIds = [...new Set(eligible.map(entry => entry.candidateId))];
  const request = source === 'x'
    ? { candidateIds: candidateIds.slice(0, MAX_CANDIDATES), maxCandidates: MAX_CANDIDATES,
        maxMediaPerCandidate: 8, maxTraversalNodes: 1500, maxDepth: 16 }
    : { candidateIds: candidateIds.slice(0, MAX_CANDIDATES), maxCandidates: MAX_CANDIDATES,
        maxScripts: 12, maxScriptBytes: 131072, maxTotalBytes: 524288,
        maxTraversalNodes: 6000, maxDepth: 24 };
  return {
    eligible,
    requestedIds: new Set(request.candidateIds),
    request,
    bounded: candidateIds.length > MAX_CANDIDATES,
  };
}

export async function resolveStructuredMedia({ page, source, posts, resolver, deadlineAt }) {
  const originalPosts = (Array.isArray(posts) ? posts : []).map(post => {
    if (!nativeVideoPermalink(source, post)) return post;
    return { ...post,
      mediaExpected: [...new Set([...(Array.isArray(post.mediaExpected) ? post.mediaExpected : []), 'video'])],
      mediaEvidence: { ...(plainObject(post.mediaEvidence) ? post.mediaEvidence : {}), nativeVideoPermalink: true },
    };
  });
  const { eligible, requestedIds, request, bounded } = structuredMediaRequest(source, originalPosts);
  const domCounts = countDomMedia(eligible);
  const statusFor = new Map();
  for (const entry of eligible) {
    statusFor.set(entry.post, requestedIds.has(entry.candidateId) ? 'no_match' : 'bounded');
  }
  const noWorkSummary = status => makeSummary({
    available: status !== 'unavailable', status, request, eligible, bounded, domCounts,
  });

  if (!eligible.length) return { posts: originalPosts, summary: noWorkSummary('not_needed') };
  if (!resolver?.available || typeof resolver.functionSource !== 'string' || resolver.functionSource.length > 128 * 1024) {
    return attachUnavailable(originalPosts, eligible, 'unavailable', request, bounded);
  }
  if (!request.candidateIds.length) return attachUnavailable(originalPosts, eligible, 'bounded', request, true);

  let result;
  try {
    const remainingMs = deadlineAt - Date.now();
    if (remainingMs <= 0) return attachUnavailable(originalPosts, eligible, 'unavailable', request, bounded);
    const timeoutMs = Math.min(remainingMs, source === 'facebook' ? 1400 : 900);
    // This expression is derived from the imported self-contained Bridge export. Its bytes are
    // never labeled with the SHA-256 of the original Bridge module; provenance hashes that file.
    const expression = `(${resolver.functionSource})(${JSON.stringify(request)})`;
    result = await page.evaluate(expression, timeoutMs);
    if (!result || typeof result !== 'object' || Array.isArray(result)
        || result.runtimeRevision !== resolver.runtimeRevision || !Array.isArray(result.candidates)) {
      return attachUnavailable(originalPosts, eligible, 'unavailable', request, bounded);
    }
    if (Buffer.byteLength(JSON.stringify(result), 'utf8') > MAX_RESOLVER_RESULT_BYTES) {
      return attachUnavailable(originalPosts, eligible, 'bounded', request, true);
    }
  } catch {
    return attachUnavailable(originalPosts, eligible, 'unavailable', request, bounded);
  }

  const byId = new Map();
  for (const candidate of result.candidates.slice(0, MAX_CANDIDATES * 2)) {
    if (!candidate || typeof candidate !== 'object' || !requestedIds.has(candidate.candidateId)) continue;
    const candidates = byId.get(candidate.candidateId) || [];
    candidates.push(candidate);
    byId.set(candidate.candidateId, candidates);
  }
  const entriesById = new Map();
  for (const entry of eligible) {
    const entries = entriesById.get(entry.candidateId) || [];
    entries.push(entry);
    entriesById.set(entry.candidateId, entries);
  }

  const replacements = new Map();
  let resolvedPosts = 0;
  const diagnostics = {
    returnedCandidates: diagnosticCount(result.candidates.length),
    resolverCandidateCount: diagnosticCount(result.diagnostics?.candidateCount),
    returnedExactCandidateCount: byId.size,
    noExactReturnedCandidateCount: [...requestedIds].filter(candidateId => !byId.has(candidateId)).length,
    resolverNoSafePairCandidateCount: 0,
    resolverDomVideoPosterPathMismatchCount: domCounts.videoPosterCount === null ? null : 0,
    resolverDomImagePathNoMatchCount: domCounts.imageCount === null ? null : 0,
    resolverPairWithoutDomPosterCount: domCounts.videoPosterCount === null
      || source === 'facebook' && domCounts.imageCount === null ? null : 0,
    resolverAmbiguousCandidateCount: 0,
    resolverTraversedNodeCount: diagnosticCount(result.diagnostics?.traversedNodeCount),
    resolverMatchedStructuredNodeCount: diagnosticCount(result.diagnostics?.matchedStructuredNodeCount),
    resolverMatchedMediaObjectCount: diagnosticCount(result.diagnostics?.matchedMediaObjectCount),
    resolverBounded: typeof result.diagnostics?.bounded === 'boolean' ? result.diagnostics.bounded : null,
    ownSafePairCount: 0,
    domVideoPosterCount: domCounts.videoPosterCount,
    domImageCount: domCounts.imageCount,
    matchedPosterPathCount: domCounts.videoPosterCount === null || source === 'facebook' && domCounts.imageCount === null ? null : 0,
    enrichedVideoCount: domCounts.videoPosterCount === null || source === 'facebook' && domCounts.imageCount === null ? null : 0,
    unmatchedVideoPosterCount: domCounts.videoPosterCount === null ? null : 0,
  };
  for (const [candidateId, entries] of entriesById) {
    if (!requestedIds.has(candidateId)) continue;
    if (entries.length !== 1) {
      diagnostics.resolverAmbiguousCandidateCount++;
      for (const entry of entries) statusFor.set(entry.post, 'ambiguous');
      continue;
    }
    const candidates = byId.get(candidateId) || [];
    if (candidates.length > 1) {
      diagnostics.resolverAmbiguousCandidateCount++;
      statusFor.set(entries[0].post, 'ambiguous');
      continue;
    }
    const candidate = candidates[0];
    if (!candidate) continue;
    const pairs = verifiedVideoPairs(source, candidate.media);
    diagnostics.ownSafePairCount = diagnosticCount((diagnostics.ownSafePairCount || 0) + pairs.byPosterPath.size);
    if (pairs.ambiguous) {
      diagnostics.resolverAmbiguousCandidateCount++;
      statusFor.set(entries[0].post, 'ambiguous');
      continue;
    }
    if (!pairs.byPosterPath.size) {
      diagnostics.resolverNoSafePairCandidateCount++;
      continue;
    }
    const post = entries[0].post;
    const originalMedia = Array.isArray(post.media) ? post.media : [];
    const domCandidatePosterCount = Array.isArray(post.media)
      ? originalMedia.filter(item => item?.kind === 'video_poster'
        || source === 'facebook' && item?.kind === 'image').length
      : null;
    if (domCandidatePosterCount === 0 && diagnostics.resolverPairWithoutDomPosterCount !== null) {
      diagnostics.resolverPairWithoutDomPosterCount++;
    }
    const nextMedia = [];
    let ownVideoPosterCount = 0;
    let accountedPosterCount = 0;
    let verifiedVideoCount = 0;
    let matchedFacebookImage = false;
    let unmatchedVideoPostersForCandidate = 0;
    let matchedFacebookImagesForCandidate = 0;
    const facebookImageCount = source === 'facebook'
      ? originalMedia.filter(item => item?.kind === 'image').length : 0;
    for (const item of originalMedia) {
      if (!item || typeof item !== 'object' || Array.isArray(item)) {
        nextMedia.push(item);
        continue;
      }
      const isVideoPoster = item.kind === 'video_poster';
      const itemKey = posterPathKey(item.url);
      let pair = itemKey ? pairs.byPosterPath.get(itemKey) : null;
      // Facebook's DOM extractor can label an image poster as an image even inside the
      // exact native video post. Upgrade it only after native ID and poster path both match.
      const isMatchedFacebookImage = source === 'facebook' && item.kind === 'image' && Boolean(pair);
      if (isVideoPoster) ownVideoPosterCount++;
      if (isMatchedFacebookImage) {
        ownVideoPosterCount++;
        matchedFacebookImage = true;
      }
      if (!pair || (!isVideoPoster && !isMatchedFacebookImage)) {
        if (isVideoPoster) {
          unmatchedVideoPostersForCandidate++;
          if (diagnostics.unmatchedVideoPosterCount !== null) diagnostics.unmatchedVideoPosterCount++;
        }
        nextMedia.push(item);
        continue;
      }
      if (diagnostics.matchedPosterPathCount !== null) diagnostics.matchedPosterPathCount++;
      if (diagnostics.enrichedVideoCount !== null) diagnostics.enrichedVideoCount++;
      if (isMatchedFacebookImage) matchedFacebookImagesForCandidate++;
      accountedPosterCount++;
      verifiedVideoCount++;
      nextMedia.push({
        ...item,
        domKind: item.kind,
        kind: 'video',
        posterUrl: pair.posterUrl,
        playbackUrl: pair.playbackUrl,
        playbackMode: 'inline',
        ...(pair.width ? { width: pair.width } : {}),
        ...(pair.height ? { height: pair.height } : {}),
        provenance: source === 'facebook' ? 'facebook_structured_json' : 'x_main_structured_state',
      });
    }
    const fullyResolved = ownVideoPosterCount > 0
      && accountedPosterCount === ownVideoPosterCount
      && verifiedVideoCount === ownVideoPosterCount;
    const status = fullyResolved ? 'resolved'
      : (accountedPosterCount > 0 || ownVideoPosterCount > 0 || matchedFacebookImage ? 'partial' : 'no_match');
    if (unmatchedVideoPostersForCandidate > 0 && diagnostics.resolverDomVideoPosterPathMismatchCount !== null) {
      diagnostics.resolverDomVideoPosterPathMismatchCount++;
    }
    if (facebookImageCount > 0 && matchedFacebookImagesForCandidate === 0
        && diagnostics.resolverDomImagePathNoMatchCount !== null) diagnostics.resolverDomImagePathNoMatchCount++;
    statusFor.set(post, status);
    if (fullyResolved) resolvedPosts++;
    replacements.set(post, {
      media: nextMedia,
      marker: {
        status,
        resolverVersion: typeof result.resolverVersion === 'string' ? result.resolverVersion.slice(0, 64) : 'unknown',
        ownVideoPosterCount,
        accountedPosterCount,
        verifiedVideoCount,
      },
    });
  }

  const enriched = originalPosts.map(post => {
    const replacement = replacements.get(post);
    if (!replacement) {
      const status = statusFor.get(post);
      return status ? attachMarker(post, {
        status,
        resolverVersion: resolverVersion(result.resolverVersion),
        ownVideoPosterCount: countVideoPosters(post),
        accountedPosterCount: 0,
        verifiedVideoCount: 0,
      }) : post;
    }
    const mediaEvidence = plainObject(post.mediaEvidence) ? { ...post.mediaEvidence } : {};
    mediaEvidence.structuredMediaResolution = replacement.marker;
    const limitations = Array.isArray(post.limitations) ? post.limitations : [];
    const nextLimitations = replacement.marker.status === 'resolved'
      ? limitations.filter(value => value !== 'video_stream_not_resolved' && value !== 'no_structured_media_resolver')
      : limitations;
    return { ...post, media: replacement.media, mediaEvidence, limitations: nextLimitations };
  });
  const summary = makeSummary({
    available: true,
    status: bounded ? 'bounded' : (resolvedPosts ? 'observed' : 'unresolved'),
    request, eligible, bounded: bounded || result.diagnostics?.bounded === true,
    domCounts, resolvedPosts, diagnostics,
  });
  return { posts: enriched, summary };
}

function attachUnavailable(posts, eligible, status, request, bounded) {
  const eligibleSet = new Set(eligible.map(entry => entry.post));
  const result = posts.map(post => eligibleSet.has(post)
    ? attachMarker(post, {
      status: bounded && !request.candidateIds.includes(nativeCandidateIdFromEntry(eligible, post)) ? 'bounded' : status,
      resolverVersion: 'unavailable',
      ownVideoPosterCount: countVideoPosters(post),
      accountedPosterCount: 0,
      verifiedVideoCount: 0,
    })
    : post);
  return {
    posts: result,
    summary: makeSummary({
      available: status !== 'unavailable', status: bounded ? 'bounded' : status,
      request, eligible, bounded, domCounts: countDomMedia(eligible),
    }),
  };
}

function attachMarker(post, marker) {
  const mediaEvidence = plainObject(post.mediaEvidence) ? { ...post.mediaEvidence } : {};
  mediaEvidence.structuredMediaResolution = marker;
  return { ...post, mediaEvidence };
}

function nativeCandidateId(source, post) {
  if (!post || typeof post.id !== 'string') return null;
  const canonical = canonicalSourceURL(source, post.permalink);
  if (!canonical) return null;
  if (source === 'x') {
    const statusId = new URL(canonical).pathname.match(/^\/[^/]+\/status\/(\d+)$/)?.[1];
    const postId = /^x:status:(\d+)$/.exec(post.id)?.[1] || (/^\d+$/.test(post.id) ? post.id : null);
    return statusId && postId === statusId ? `x:status:${statusId}` : null;
  }
  const nativeId = facebookNativeId(canonical);
  const postId = /^facebook:post:(pfbid[A-Za-z0-9]+|\d{1,32})$/i.exec(post.id)?.[1];
  return nativeId && postId && nativeId === postId
    ? `facebook:post:${postId}` : null;
}

function nativeCandidateIdFromEntry(eligible, post) {
  return eligible.find(entry => entry.post === post)?.candidateId || null;
}

function facebookNativeId(canonical) {
  try {
    const url = new URL(canonical);
    if (url.pathname.toLowerCase() === '/watch/') return url.searchParams.get('v');
    const pathId = url.pathname.match(/\/(?:posts|permalink|videos|reel)\/(pfbid[A-Za-z0-9]+|\d{1,32})(?:\/|$)/i)?.[1];
    if (pathId) return pathId;
    return ['story_fbid', 'fbid', 'photo_id'].map(key => url.searchParams.get(key))
      .find(value => /^(?:pfbid[A-Za-z0-9]+|\d{1,32})$/i.test(value || '')) || null;
  } catch { return null; }
}

function nativeVideoPermalink(source, post) {
  if (source !== 'facebook' || !nativeCandidateId(source, post)) return false;
  const canonical = canonicalSourceURL(source, post.permalink);
  const path = new URL(canonical).pathname;
  return path.toLowerCase() === '/watch/' || /\/(?:videos|reel)\/(?:pfbid[A-Za-z0-9]+|\d+)(?:\/|$)/i.test(path);
}

function expectsVideo(post) {
  return Array.isArray(post?.mediaExpected) && post.mediaExpected.includes('video')
    || Array.isArray(post?.media) && post.media.some(item => item?.kind === 'video_poster');
}

function verifiedVideoPairs(source, media) {
  const byPosterPath = new Map();
  let ambiguous = false;
  for (const item of Array.isArray(media) ? media.slice(0, 64) : []) {
    if (!item || item.kind !== 'video') continue;
    const posterUrl = safePoster(source, item.posterUrl || item.url);
    const playbackUrl = safePlayback(source, item.playbackUrl);
    const key = posterPathKey(posterUrl);
    if (!posterUrl || !playbackUrl || !key) continue;
    const pair = {
      posterUrl,
      playbackUrl,
      width: boundedDimension(item.width),
      height: boundedDimension(item.height),
    };
    const previous = byPosterPath.get(key);
    if (previous && previous.playbackUrl !== pair.playbackUrl) {
      ambiguous = true;
    } else if (!previous) byPosterPath.set(key, pair);
  }
  return { byPosterPath, ambiguous };
}

function safePoster(source, value) {
  const url = safeMediaUrl(value);
  if (!url) return null;
  const parsed = new URL(url);
  if (source === 'x') {
    return parsed.hostname === 'pbs.twimg.com'
      && /^\/(?:ext_tw_video_thumb|amplify_video_thumb|tweet_video_thumb)\//i.test(parsed.pathname)
      && /\.(?:avif|gif|jpe?g|png|webp)$/i.test(parsed.pathname) ? url : null;
  }
  return facebookMediaHost(parsed.hostname) && /\.(?:avif|gif|jpe?g|png|webp)$/i.test(parsed.pathname) ? url : null;
}

function safePlayback(source, value) {
  const url = safeMediaUrl(value);
  if (!url) return null;
  const parsed = new URL(url);
  if (source === 'x') {
    return parsed.hostname === 'video.twimg.com'
      && /^\/(?:amplify_video|ext_tw_video|tweet_video)\//i.test(parsed.pathname)
      && /\.mp4$/i.test(parsed.pathname) ? url : null;
  }
  return facebookMediaHost(parsed.hostname) && /\.mp4$/i.test(parsed.pathname) ? url : null;
}

function safeMediaUrl(value) {
  if (typeof value !== 'string' || value.length > 4096) return null;
  try {
    const url = new URL(value);
    if (url.protocol !== 'https:' || url.username || url.password || url.port) return null;
    url.hash = '';
    return url.href;
  } catch { return null; }
}

function facebookMediaHost(host) {
  return ['fbcdn.net', 'fbsbx.com'].some(suffix => host === suffix || host.endsWith(`.${suffix}`));
}

function posterPathKey(value) {
  const url = safeMediaUrl(value);
  if (!url) return '';
  const parsed = new URL(url);
  return `${parsed.hostname.toLowerCase()}${parsed.pathname}`;
}

function boundedDimension(value) {
  return Number.isSafeInteger(value) && value > 0 && value <= 8192 ? value : 0;
}

function countVideoPosters(post) {
  return Array.isArray(post?.media) ? post.media.filter(item => item?.kind === 'video_poster').length : 0;
}

function makeSummary({ available, status, request, eligible, bounded, domCounts, resolvedPosts = 0, diagnostics = null }) {
  const unresolvedPosts = Math.max(0, eligible.length - resolvedPosts);
  const base = {
    available,
    status,
    requestedCandidates: diagnosticCount(request?.candidateIds?.length),
    eligibleCandidates: diagnosticCount(eligible?.length),
    returnedCandidates: diagnostics?.returnedCandidates ?? null,
    resolvedPosts: diagnosticCount(resolvedPosts),
    unresolvedPosts: diagnosticCount(unresolvedPosts),
    bounded: typeof bounded === 'boolean' ? bounded : null,
    domVideoPosterCount: domCounts?.videoPosterCount ?? null,
    domImageCount: domCounts?.imageCount ?? null,
    ...nullResolverDiagnostics(),
    ...(diagnostics || {}),
  };
  return base;
}

function nullResolverDiagnostics() {
  return {
    resolverCandidateCount: null,
    returnedExactCandidateCount: null,
    noExactReturnedCandidateCount: null,
    resolverNoSafePairCandidateCount: null,
    resolverDomVideoPosterPathMismatchCount: null,
    resolverDomImagePathNoMatchCount: null,
    resolverPairWithoutDomPosterCount: null,
    resolverAmbiguousCandidateCount: null,
    resolverTraversedNodeCount: null,
    resolverMatchedStructuredNodeCount: null,
    resolverMatchedMediaObjectCount: null,
    resolverBounded: null,
    ownSafePairCount: null,
    matchedPosterPathCount: null,
    enrichedVideoCount: null,
    unmatchedVideoPosterCount: null,
  };
}

function countDomMedia(eligible) {
  if (!eligible.length) return { videoPosterCount: null, imageCount: null };
  let videoPosterCount = 0;
  let imageCount = 0;
  let complete = true;
  for (const { post } of eligible) {
    if (!Array.isArray(post?.media)) {
      complete = false;
      continue;
    }
    videoPosterCount += post.media.filter(item => item?.kind === 'video_poster').length;
    imageCount += post.media.filter(item => item?.kind === 'image').length;
  }
  return complete ? { videoPosterCount, imageCount } : { videoPosterCount: null, imageCount: null };
}

function diagnosticCount(value) {
  return Number.isSafeInteger(value) && value >= 0 && value <= MAX_DIAGNOSTIC_COUNT ? value : null;
}

function resolverVersion(value) {
  return typeof value === 'string' ? value.slice(0, 64) : 'unknown';
}

function plainObject(value) {
  return Boolean(value && typeof value === 'object' && !Array.isArray(value));
}

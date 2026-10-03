import { createHash } from 'node:crypto';

const X_POST = /^\/([^/]+)\/status\/(\d+)(?:\/.*)?$/;
const MAX_STRUCTURED_MEDIA_DIAGNOSTIC_SNAPSHOTS = 8;
const STRUCTURED_MEDIA_DIAGNOSTIC_COUNTS = [
  'requestedCandidates', 'eligibleCandidates', 'returnedCandidates',
  'resolverCandidateCount', 'returnedExactCandidateCount', 'noExactReturnedCandidateCount',
  'resolverNoSafePairCandidateCount', 'resolverDomVideoPosterPathMismatchCount', 'resolverDomImagePathNoMatchCount',
  'resolverPairWithoutDomPosterCount',
  'resolverAmbiguousCandidateCount', 'resolverTraversedNodeCount', 'resolverMatchedStructuredNodeCount',
  'resolverMatchedMediaObjectCount', 'ownSafePairCount', 'domVideoPosterCount', 'domImageCount',
  'matchedPosterPathCount', 'enrichedVideoCount', 'unmatchedVideoPosterCount', 'resolvedPosts', 'unresolvedPosts',
];

export function canonicalSourceURL(source, raw) {
  if (typeof raw !== 'string' || raw.length > 4096) return null;
  let url;
  try { url = new URL(raw.trim()); } catch { return null; }
  if (url.protocol !== 'https:' || url.username || url.password || url.port) return null;
  const host = url.hostname.toLowerCase();
  if (source === 'x' && host === 'x.com') {
    const match = url.pathname.match(X_POST);
    if (!match) return null;
    url.pathname = `/${match[1]}/status/${match[2]}`;
    url.search = '';
    url.hash = '';
    return url.href;
  }
  if (source === 'instagram' && ['www.instagram.com','instagram.com'].includes(host)) {
    const match=url.pathname.match(/^\/(p|reel|tv)\/([A-Za-z0-9_-]+)\/?$/);
    if (!match) return null;
    return `https://www.instagram.com/${match[1]}/${match[2]}/`;
  }
  if (source === 'linkedin' && host === 'www.linkedin.com') {
    const match=url.pathname.match(/^\/feed\/update\/urn:li:(activity|ugcPost|share):(\d{5,30})\/?$/i)
      || url.pathname.match(/^\/posts\/[^/]*-(activity|ugcpost|share)-(\d{5,30})(?:-[A-Za-z0-9_-]+)?\/?$/i);
    if (!match) return null;
    const kind=match[1].toLowerCase()==='ugcpost'?'ugcPost':match[1].toLowerCase();
    return `https://www.linkedin.com/feed/update/urn:li:${kind}:${match[2]}/`;
  }
  if (source !== 'facebook' || !['www.facebook.com', 'facebook.com', 'm.facebook.com'].includes(host)) return null;
  const path = url.pathname.toLowerCase();
  if (/^\/(?:watch\/|video\.php)$/.test(path)) {
    const videoId = url.searchParams.get('v');
    if (url.searchParams.getAll('v').length !== 1 || !/^\d{1,32}$/.test(videoId || '')) return null;
    return `https://www.facebook.com/watch/?v=${videoId}`;
  }
  const native = path.includes('/posts/') || path.includes('/permalink/')
    || (path.includes('/story.php') && Boolean(url.searchParams.get('story_fbid')))
    || (path.includes('/photo') && Boolean(url.searchParams.get('fbid') || url.searchParams.get('photo_id')))
    || path.includes('/videos/') || /^\/reel\/\d+\/?$/.test(path);
  return native ? url.href : null;
}

export function evidenceKey(source, platformId, permalink, author, text) {
  let identity = typeof platformId === 'string' ? platformId.trim() : '';
  if (!identity) identity = typeof permalink === 'string' ? permalink.trim() : '';
  if (!identity) identity = `${String(author || '').trim().toLowerCase().replace(/\s+/g, ' ')}\0${String(text || '').trim().toLowerCase().replace(/\s+/g, ' ')}`;
  if (!identity || identity === '\0') return '';
  const digest = createHash('sha256').update(`${source}\0${identity}`, 'utf8').digest('hex').slice(0, 24);
  return `${source}:${digest}`;
}

function preserveMap(value) {
  return value && typeof value === 'object' && !Array.isArray(value) ? structuredClone(value) : {};
}

function validatePost(source, post) {
  if (!post || typeof post !== 'object' || Array.isArray(post)) throw captureError('invalid_observation', 'extractor returned an invalid post record');
  if (typeof post.id !== 'string' || !post.id || post.id.length > 128) throw captureError('invalid_observation', 'post identity is missing or too long');
  if (typeof post.permalink !== 'string' || !canonicalSourceURL(source, post.permalink)) {
    throw captureError('invalid_observation', 'post permalink is outside the native source URL contract');
  }
  let platformId = post.id;
  if (source === 'x') {
    const statusId = new URL(post.permalink).pathname.match(X_POST)?.[2];
    const observedId = /^x:status:(\d+)$/.exec(post.id)?.[1] || post.id;
    if (!statusId || !/^\d+$/.test(observedId) || statusId !== observedId) {
      throw captureError('invalid_observation', 'X post identity does not match its native permalink');
    }
    platformId = `x:status:${statusId}`;
  } else if (source === 'instagram' || source === 'linkedin') {
    const parsed=new URL(canonicalSourceURL(source,post.permalink));
    const match=source==='instagram'?parsed.pathname.match(/^\/(p|reel|tv)\/([A-Za-z0-9_-]+)\/$/)
      :parsed.pathname.match(/urn:li:(activity|ugcPost|share):(\d{5,30})/i);
    const expected=`${source}:${match[1].toLowerCase()}:${match[2]}`;
    if (post.id !== expected) throw captureError('invalid_observation','post identity does not match its native permalink');
    platformId=expected;
  } else if (!/^facebook:post:(?:pfbid[A-Za-z0-9]+|\d+)$/i.test(post.id)) {
    throw captureError('invalid_observation', 'Facebook post identity is not canonical');
  } else {
    const canonical = canonicalSourceURL(source, post.permalink);
    const parsed = canonical ? new URL(canonical) : null;
    if (parsed?.pathname.toLowerCase() === '/watch/') {
      const videoId = parsed.searchParams.get('v');
      if (post.id !== `facebook:post:${videoId}`) {
        throw captureError('invalid_observation', 'Facebook video identity does not match its native permalink');
      }
    }
  }
  if (typeof post.author !== 'string' || post.author.length > 1200 || typeof post.text !== 'string') {
    throw captureError('invalid_observation', 'post author or text has an invalid shape');
  }
  if (Array.from(post.text).length > 4000) throw captureError('evidence_too_large', 'post text exceeds the 4000 character observation limit');
  const encodedSize = Buffer.byteLength(JSON.stringify(post), 'utf8');
  if (encodedSize > 96 * 1024) throw captureError('evidence_too_large', 'post evidence exceeds the 96 KiB observation limit');
  return platformId;
}

function toBlock(source, post, feedPosition, captureMode = 'headless_worker') {
  const platformId = validatePost(source, post);
  const media = Array.isArray(post.media) ? structuredClone(post.media.slice(0, 20)) : [];
  const expected = Array.isArray(post.mediaExpected) ? post.mediaExpected.slice(0, 12) : [];
  const limitations = Array.isArray(post.limitations) ? post.limitations.slice(0, 24) : [];
  const expectsVideo = expected.includes('video') || media.some(item => item?.kind === 'video_poster');
  const structuredResolution = post.mediaEvidence?.structuredMediaResolution;
  const verifiedStructuredVideos = media.filter(item => isVerifiedStructuredVideo(source, item));
  const structuredResolved = expectsVideo
    && structuredResolution?.status === 'resolved'
    && structuredResolution.resolverVersion === (source === 'x' ? 'x-main-world-structured-v1' : 'facebook-structured-video-v1')
    && Number.isSafeInteger(structuredResolution.ownVideoPosterCount)
    && structuredResolution.ownVideoPosterCount > 0
    && structuredResolution.accountedPosterCount === structuredResolution.ownVideoPosterCount
    && structuredResolution.verifiedVideoCount === structuredResolution.ownVideoPosterCount
    && verifiedStructuredVideos.length >= structuredResolution.verifiedVideoCount
    && !media.some(item => item?.kind === 'video_poster');
  const unresolvedVideo = expectsVideo && !structuredResolved;
  const additionalOwnedVideo=['instagram','linkedin'].includes(source)
    && post.mediaEvidence?.additionalStructuredMedia?.status==='observed_owned_urls'
    && media.some(item=>item?.kind==='video' && typeof item.playbackUrl==='string');
  const recovery = {
    status: structuredResolved ? 'resolved' : unresolvedVideo ? 'partial' : post.mediaEvidence?.status || 'unverified',
    outcome: structuredResolved ? 'verified_owned_playback' : additionalOwnedVideo ? 'owned_playback_url_observed' : unresolvedVideo ? 'unresolved'
      : source==='instagram' && post.evidenceMode==='headless_native_target_structured_observation' ? 'observed_native_target_structured' : 'observed_dom_only',
    expected,
    evidence: preserveMap(post.mediaEvidence),
    ...(unresolvedVideo ? additionalOwnedVideo
      ? {unknownVideo:'playback_unverified',limitation:'video_playback_unverified'}
      : { unknownVideo: 'unresolved', limitation: 'video_stream_not_resolved' } : {}),
  };
  const presentation = preserveMap(post.presentation);
  if (post.timestampSource) presentation.timestampSource = post.timestampSource;
  if (typeof post.timestampEstimated === 'boolean') presentation.timestampEstimated = post.timestampEstimated;
  if (typeof post.timestampText === 'string' && post.timestampText) presentation.timestampText = post.timestampText;
  if (post.timestampEvidence && typeof post.timestampEvidence === 'object') presentation.timestampEvidence = structuredClone(post.timestampEvidence);
  const quality = {
    status: 'unverified',
    mode: source==='instagram' && post.evidenceMode==='headless_native_target_structured_observation' ? post.evidenceMode
      : captureMode === 'browser_quiet_hidden' ? 'quiet_dom_observation' : 'headless_dom_observation',
    headlessSourceId: post.id,
    limitations,
    textStatus: typeof post.textStatus === 'string' ? post.textStatus : 'unverified',
    ...(post.permalinkProvenance ? { permalinkProvenance: post.permalinkProvenance } : {}),
  };
  return {
    evidenceKey: evidenceKey(source, platformId, post.permalink, post.author, post.text),
    author: post.author,
    avatarUrl: typeof post.avatar === 'string' ? post.avatar : '',
    text: post.text,
    permalink: post.permalink,
    publishedAt: typeof post.publishedAt === 'string' && post.publishedAt ? post.publishedAt : null,
    platformId,
    contentKind: typeof post.contentKind === 'string' ? post.contentKind : 'post',
    relationshipType: typeof post.relationshipType === 'string' ? post.relationshipType : 'original',
    parentPermalink: typeof post.parentPermalink === 'string' ? post.parentPermalink : '',
    quotedPost: preserveMap(post.quotedPost),
    ...(['instagram','linkedin'].includes(source) && Array.isArray(post.directContext)
      ? {directContext:structuredClone(post.directContext.slice(0,12))} : {}),
    engagement: preserveMap(post.engagement),
    presentation,
    attachments: ['instagram','linkedin'].includes(source) && Array.isArray(post.attachments) ? structuredClone(post.attachments.slice(0,20)) : [],
    media,
    links: ['instagram','linkedin'].includes(source) && Array.isArray(post.links) ? structuredClone(post.links.slice(0,20)) : [],
    mediaRecovery: recovery,
    captureQuality: quality,
    feedPosition,
  };
}

function isVerifiedStructuredVideo(source, item) {
  if (!item || item.kind !== 'video' || typeof item.posterUrl !== 'string' || typeof item.playbackUrl !== 'string') return false;
  try {
    const poster = new URL(item.posterUrl);
    const playback = new URL(item.playbackUrl);
    if ([poster, playback].some(url => url.protocol !== 'https:' || url.username || url.password || url.port)) return false;
    if (source === 'x') {
      return poster.hostname === 'pbs.twimg.com'
        && /^\/(?:ext_tw_video_thumb|amplify_video_thumb|tweet_video_thumb)\//i.test(poster.pathname)
        && /\.(?:avif|gif|jpe?g|png|webp)$/i.test(poster.pathname)
        && playback.hostname === 'video.twimg.com'
        && /^\/(?:amplify_video|ext_tw_video|tweet_video)\//i.test(playback.pathname)
        && /\.mp4$/i.test(playback.pathname);
    }
    const allowed = host => ['fbcdn.net', 'fbsbx.com'].some(suffix => host === suffix || host.endsWith(`.${suffix}`));
    return allowed(poster.hostname.toLowerCase())
      && /\.(?:avif|gif|jpe?g|png|webp)$/i.test(poster.pathname)
      && allowed(playback.hostname.toLowerCase())
      && /\.mp4$/i.test(playback.pathname);
  } catch { return false; }
}

export function toObservation({ source, requestedUrl, snapshots, provenance, capturedAt, stopReason, frontier, freshness, captureMode = 'headless_worker' }) {
  const first = snapshots[0] || {};
  const hasPosts = snapshots.some(snapshot => (snapshot.posts || []).length > 0);
  const domainSnapshots = snapshots.map((snapshot, index) => {
    const previousIds = new Set(snapshots.slice(0, index).flatMap(value => (value.posts || []).map(post => post.id)));
    const posts = Array.isArray(snapshot.posts) ? snapshot.posts.slice(0, 20) : [];
    const uniqueIds = new Set(posts.map(post => post.id));
    const counts = { candidates: Number.isInteger(snapshot.candidateCount) ? snapshot.candidateCount : posts.length };
    const candidateDiagnostics = snapshot.candidateDiagnostics && typeof snapshot.candidateDiagnostics === 'object'
      ? structuredClone(snapshot.candidateDiagnostics) : undefined;
    const qualityReports = [{ status: 'unverified', mode: captureMode === 'browser_quiet_hidden' ? 'quiet_dom_observation' : 'headless_dom_observation', candidateCount: counts.candidates,
      observedPostCount: posts.length, rejected: Number.isInteger(snapshot.rejected) ? snapshot.rejected : 0,
      rejectionReasons: preserveMap(snapshot.rejectionReasons) }];
    return {
      index,
      adapterVersion: typeof snapshot.adapterVersion === 'string' ? snapshot.adapterVersion : 'unknown',
      selectorStrategy: typeof snapshot.discoveryStrategy === 'string' ? snapshot.discoveryStrategy : 'headless_visible_dom',
      selectorCounts: counts,
      selectorCandidateCount: counts.candidates,
      structuralCandidateCount: candidateDiagnostics?.structuralCandidates || 0,
      visibleContainerCount: posts.length,
      ...(candidateDiagnostics ? { candidateDiagnostics } : {}),
      capturedAt: snapshot.capturedAt || capturedAt,
      scrollY: Number.isFinite(snapshot.scroll?.y) ? Math.trunc(snapshot.scroll.y) : 0,
      viewportHeight: Number.isFinite(snapshot.scroll?.viewportHeight) ? Math.trunc(snapshot.scroll.viewportHeight) : 0,
      newCandidateCount: [...uniqueIds].filter(id => !previousIds.has(id)).length,
      blocks: posts.map((post, position) => toBlock(source, post, position, captureMode)),
      qualityReports,
    };
  });
  const last = snapshots.at(-1) || first;
  const status = hasPosts ? 'partial' : 'unverified';
  const structuredMediaResolution = {
    schema: 'aku.headless-structured-media-diagnostics.v1',
    snapshotCount: boundedDiagnosticCount(snapshots.length),
    includedSnapshotCount: Math.min(snapshots.length, MAX_STRUCTURED_MEDIA_DIAGNOSTIC_SNAPSHOTS),
    truncated: snapshots.length > MAX_STRUCTURED_MEDIA_DIAGNOSTIC_SNAPSHOTS,
    snapshots: snapshots.slice(0, MAX_STRUCTURED_MEDIA_DIAGNOSTIC_SNAPSHOTS)
      .map((snapshot, index) => structuredMediaDiagnosticSnapshot(snapshot?.structuredMediaResolution, index)),
  };
  return {
    source,
    pageUrl: requestedUrl,
    pageTitle: typeof last.title === 'string' ? last.title.slice(0, 1000) : '',
    capturedAt,
    snapshots: domainSnapshots,
    coverage: {
      status,
      captureMode,
      captureStatus: 'captured_partial',
      stopReason,
      observedBlockCount: domainSnapshots.reduce((sum, snapshot) => sum + snapshot.blocks.length, 0),
      observedUniqueIdCount: new Set(snapshots.flatMap(snapshot => (snapshot.posts || []).map(post => post.id))).size,
      loginRequired: last.loginRequired === true,
      challengeDetected: last.challengeDetected === true,
      sourceUnavailable: last.sourceUnavailable === true,
      authenticatedUiObserved: last.authenticatedUiObserved === true,
      documentReady: last.documentReady === true,
      structuredMediaResolution,
      ...(first.quoteIdentityProbe ? { quoteIdentityProbe: structuredClone(first.quoteIdentityProbe) } : {}),
      ...(frontier ? { frontier: structuredClone(frontier) } : {}),
      ...(freshness ? { freshness: structuredClone(freshness) } : {}),
      provenance,
    },
  };
}

function structuredMediaDiagnosticSnapshot(summary, index) {
  const value = summary && typeof summary === 'object' && !Array.isArray(summary) ? summary : {};
  const allowedStatuses = new Set(['not_needed', 'unavailable', 'bounded', 'observed', 'unresolved']);
  return {
    snapshotIndex: index,
    status: allowedStatuses.has(value.status) ? value.status : 'not_recorded',
    available: typeof value.available === 'boolean' ? value.available : null,
    bounded: typeof value.bounded === 'boolean' ? value.bounded : null,
    resolverBounded: typeof value.resolverBounded === 'boolean' ? value.resolverBounded : null,
    ...Object.fromEntries(STRUCTURED_MEDIA_DIAGNOSTIC_COUNTS.map(key => [key, boundedDiagnosticCount(value[key])])),
  };
}

function boundedDiagnosticCount(value) {
  return Number.isSafeInteger(value) && value >= 0 && value <= 1_000_000 ? value : null;
}

export function captureError(code, message) {
  return Object.assign(new Error(message), { code });
}

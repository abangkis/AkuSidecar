import { createHash } from 'node:crypto';

const X_POST = /^\/([^/]+)\/status\/(\d+)(?:\/.*)?$/;

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
  if (source !== 'facebook' || !['www.facebook.com', 'facebook.com', 'm.facebook.com'].includes(host)) return null;
  const path = url.pathname.toLowerCase();
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
  } else if (!/^facebook:post:(?:pfbid[A-Za-z0-9]+|\d+)$/i.test(post.id)) {
    throw captureError('invalid_observation', 'Facebook post identity is not canonical');
  }
  if (typeof post.author !== 'string' || post.author.length > 1200 || typeof post.text !== 'string') {
    throw captureError('invalid_observation', 'post author or text has an invalid shape');
  }
  if (Array.from(post.text).length > 4000) throw captureError('evidence_too_large', 'post text exceeds the 4000 character observation limit');
  const encodedSize = Buffer.byteLength(JSON.stringify(post), 'utf8');
  if (encodedSize > 96 * 1024) throw captureError('evidence_too_large', 'post evidence exceeds the 96 KiB observation limit');
  return platformId;
}

function toBlock(source, post, feedPosition) {
  const platformId = validatePost(source, post);
  const media = Array.isArray(post.media) ? structuredClone(post.media.slice(0, 20)) : [];
  const expected = Array.isArray(post.mediaExpected) ? post.mediaExpected.slice(0, 12) : [];
  const limitations = Array.isArray(post.limitations) ? post.limitations.slice(0, 24) : [];
  const unresolvedVideo = expected.includes('video') || media.some(item => item?.kind === 'video_poster');
  const recovery = {
    status: unresolvedVideo ? 'partial' : post.mediaEvidence?.status || 'unverified',
    outcome: unresolvedVideo ? 'unresolved' : 'observed_dom_only',
    expected,
    evidence: preserveMap(post.mediaEvidence),
    ...(unresolvedVideo ? { unknownVideo: 'unresolved', limitation: 'video_stream_not_resolved' } : {}),
  };
  const presentation = preserveMap(post.presentation);
  if (post.timestampSource) presentation.timestampSource = post.timestampSource;
  if (typeof post.timestampEstimated === 'boolean') presentation.timestampEstimated = post.timestampEstimated;
  if (typeof post.timestampText === 'string' && post.timestampText) presentation.timestampText = post.timestampText;
  if (post.timestampEvidence && typeof post.timestampEvidence === 'object') presentation.timestampEvidence = structuredClone(post.timestampEvidence);
  const quality = {
    status: 'unverified',
    mode: 'headless_dom_observation',
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
    engagement: preserveMap(post.engagement),
    presentation,
    attachments: [],
    media,
    links: [],
    mediaRecovery: recovery,
    captureQuality: quality,
    feedPosition,
  };
}

export function toObservation({ source, requestedUrl, snapshots, provenance, capturedAt, stopReason }) {
  const first = snapshots[0] || {};
  const hasPosts = snapshots.some(snapshot => (snapshot.posts || []).length > 0);
  const domainSnapshots = snapshots.map((snapshot, index) => {
    const previousIds = new Set(snapshots.slice(0, index).flatMap(value => (value.posts || []).map(post => post.id)));
    const posts = Array.isArray(snapshot.posts) ? snapshot.posts.slice(0, 20) : [];
    const uniqueIds = new Set(posts.map(post => post.id));
    const counts = { candidates: Number.isInteger(snapshot.candidateCount) ? snapshot.candidateCount : posts.length };
    const candidateDiagnostics = snapshot.candidateDiagnostics && typeof snapshot.candidateDiagnostics === 'object'
      ? structuredClone(snapshot.candidateDiagnostics) : undefined;
    const qualityReports = [{ status: 'unverified', mode: 'headless_dom_observation', candidateCount: counts.candidates,
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
      blocks: posts.map((post, position) => toBlock(source, post, position)),
      qualityReports,
    };
  });
  const last = snapshots.at(-1) || first;
  const status = hasPosts ? 'partial' : 'unverified';
  return {
    source,
    pageUrl: requestedUrl,
    pageTitle: typeof last.title === 'string' ? last.title.slice(0, 1000) : '',
    capturedAt,
    snapshots: domainSnapshots,
    coverage: {
      status,
      captureMode: 'headless_worker',
      captureStatus: 'captured_partial',
      stopReason,
      observedBlockCount: domainSnapshots.reduce((sum, snapshot) => sum + snapshot.blocks.length, 0),
      observedUniqueIdCount: new Set(snapshots.flatMap(snapshot => (snapshot.posts || []).map(post => post.id))).size,
      loginRequired: last.loginRequired === true,
      challengeDetected: last.challengeDetected === true,
      sourceUnavailable: last.sourceUnavailable === true,
      authenticatedUiObserved: last.authenticatedUiObserved === true,
      documentReady: last.documentReady === true,
      ...(first.quoteIdentityProbe ? { quoteIdentityProbe: structuredClone(first.quoteIdentityProbe) } : {}),
      provenance,
    },
  };
}

export function captureError(code, message) {
  return Object.assign(new Error(message), { code });
}

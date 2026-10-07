import { applyQuoteRecovery, probeQuoteNavigation } from './quote-navigation.mjs';
import { canonicalSourceURL, captureError, toObservation } from './observation.mjs';
import { sourceProvenance } from './provenance.mjs';
import { resolveStructuredMedia } from './structured-media.mjs';
import { resolveAdditionalSourceMedia, resolveInstagramNativeTarget } from './additional-source-media.mjs';
import { photoRecaptureObservation } from './photo-recapture.mjs';
import { createXTextRecovery } from './x-text-recovery.mjs';
import { recoverHeadlessFreshness } from './headless-freshness.mjs';
import { sourceFreshnessContractFor } from './source-freshness-contract.mjs';

const MAX_CAPTURE_MS = 90000;
const MAX_SNAPSHOT_BYTES = 4 * 1024 * 1024;
const MAX_POST_EVIDENCE_BYTES = 6 * 1024 * 1024;
const sourceFrontiers = new Map();
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));

export function validateCapture(source, payload = {}) {
  if (!['x','facebook','instagram','linkedin'].includes(source)) throw captureError('unsupported_source', 'no headless collector for this source');
  if (!payload || typeof payload !== 'object' || Array.isArray(payload)) throw captureError('invalid_payload', 'payload must be a JSON object');
  const acquisitionRound = boundedInteger(payload.acquisitionRound ?? payload.round, 1, 1, 2, 'acquisitionRound');
  const home = {x:'https://x.com/home',facebook:'https://www.facebook.com/',instagram:'https://www.instagram.com/',linkedin:'https://www.linkedin.com/feed/'}[source];
  const rawPageUrl = payload.pageUrl;
  let pageUrl = null;
  if (rawPageUrl !== undefined && rawPageUrl !== null && rawPageUrl !== '') {
    if (typeof rawPageUrl !== 'string' || rawPageUrl.length > 2048) throw captureError('invalid_page_url', 'pageUrl must be a native source URL of at most 2048 characters');
    pageUrl = canonicalSourceURL(source, rawPageUrl);
    if (!pageUrl) throw captureError('invalid_page_url', 'pageUrl must be a canonical native X or Facebook post URL');
  }
  const scrolls = boundedInteger(payload.scrolls ?? payload.maxScrolls, 4, 0, 6, 'scrolls');
  const hydrationMs = boundedInteger(payload.sourceHydrationTimeoutMs ?? payload.waitMs, 12000, 1000, 45000, 'sourceHydrationTimeoutMs');
  const captureTimeoutMs = boundedInteger(payload.captureTimeoutMs, 45000, 1000, MAX_CAPTURE_MS, 'captureTimeoutMs');
  const maxBlocksPerSnapshot = boundedInteger(payload.maxBlocksPerSnapshot ?? payload.maxPosts, 20, 1, 20, 'maxBlocksPerSnapshot');
  const scrollFraction = boundedNumber(payload.scrollFraction, 0.75, 0.25, 1, 'scrollFraction');
  const scrollSettleMs = boundedInteger(payload.scrollSettleMs, 900, 100, 5000, 'scrollSettleMs');
  const pendingContentPolicy = payload.pendingContentPolicy ?? (acquisitionRound === 1 ? 'reveal_if_present' : 'detect_only');
  if (!['reveal_if_present', 'detect_only'].includes(pendingContentPolicy)) throw captureError('invalid_payload', 'pendingContentPolicy is unsupported');
  const sourceFreshnessPolicy = payload.sourceFreshnessPolicy ?? (acquisitionRound === 1 ? 'wake_and_reveal' : 'preserve_frontier');
  if (!['wake_and_reveal', 'preserve_frontier', 'preserve_target'].includes(sourceFreshnessPolicy)) throw captureError('invalid_payload', 'sourceFreshnessPolicy is unsupported');
  const sameTabMutationAllowed = payload.sameTabMutationAllowed ?? acquisitionRound === 1;
  if (typeof sameTabMutationAllowed !== 'boolean') throw captureError('invalid_payload', 'sameTabMutationAllowed must be boolean');
  if (payload.restoreScroll !== undefined && typeof payload.restoreScroll !== 'boolean') throw captureError('invalid_payload', 'restoreScroll must be boolean');
  const continuation = payload.continuation ?? null;
  if (acquisitionRound === 2 && (!continuation || typeof continuation !== 'object' || Array.isArray(continuation))) {
    throw captureError('unsupported_continuation', 'round 2 requires a source continuation object');
  }
  if (acquisitionRound === 1 && continuation !== null) throw captureError('unsupported_continuation', 'round 1 cannot consume a continuation');
  if (acquisitionRound === 2 && (!Number.isSafeInteger(continuation.startScrollY) || continuation.startScrollY < 0
      || !Number.isSafeInteger(continuation.settleMs) || continuation.settleMs < 0
      || !Array.isArray(continuation.anchorKeys) || continuation.anchorKeys.length < 1 || continuation.anchorKeys.length > 3
      || continuation.anchorKeys.some(anchor => typeof anchor !== 'string' || !anchor || anchor.length > 512))) {
    throw captureError('unsupported_continuation', 'round 2 continuation must include bounded startScrollY, settleMs, and anchorKeys');
  }
  return {
    source, pageUrl: pageUrl || home, explicitPageUrl: Boolean(pageUrl), acquisitionRound,
    scrolls, hydrationMs, captureTimeoutMs, maxBlocksPerSnapshot, scrollFraction, scrollSettleMs,
    pendingContentPolicy, sourceFreshnessPolicy, sameTabMutationAllowed,
    restoreScroll: payload.restoreScroll ?? true, continuation,
  };
}

function boundedInteger(value, fallback, min, max, name) {
  if (value === undefined || value === null) return fallback;
  if (!Number.isSafeInteger(value) || value < min || value > max) throw captureError('invalid_payload', `${name} must be an integer from ${min} to ${max}`);
  return value;
}

function boundedNumber(value, fallback, min, max, name) {
  if (value === undefined || value === null) return fallback;
  if (typeof value !== 'number' || !Number.isFinite(value) || value < min || value > max) throw captureError('invalid_payload', `${name} must be a number from ${min} to ${max}`);
  return value;
}

export function continuationMatches(state, source, requestedUrl, continuation) {
  if (!state || state.source !== source || state.pageUrl !== pageKey(source, requestedUrl)) return false;
  const start = continuation?.startScrollY;
  const settle = continuation?.settleMs;
  const anchors = continuation?.anchorKeys;
  if (!Number.isSafeInteger(start) || start < 0 || !Number.isSafeInteger(settle) || settle < 0
      || !Array.isArray(anchors) || anchors.length < 1 || anchors.length > 3
      || anchors.some(anchor => typeof anchor !== 'string' || !anchor || anchor.length > 512)) return false;
  return start === state.frontier.scrollY && sameStrings(anchors, state.frontier.anchorKeys);
}

function sameStrings(left, right) {
  return left.length === right.length && left.every((value, index) => value === right[index]);
}

function pageKey(source, url) {
  const canonical = canonicalSourceURL(source, url);
  if (canonical) return canonical;
  try {
    const parsed = new URL(url);
    if (parsed.protocol !== 'https:' || parsed.username || parsed.password || parsed.port) return '';
    const expected = {x:['x.com'],facebook:['www.facebook.com','facebook.com','m.facebook.com'],instagram:['www.instagram.com','instagram.com'],linkedin:['www.linkedin.com']}[source] || [];
    if (!expected.includes(parsed.hostname.toLowerCase())) return '';
    return `${source}://${parsed.hostname.toLowerCase()}${parsed.pathname.replace(/\/$/, '') || '/'}${parsed.search}`;
  } catch { return ''; }
}

export async function capture(browser, assetsBySource, source, payload) {
  const options = validateCapture(source, payload);
  const deadline = Date.now() + options.captureTimeoutMs;
  const page = typeof browser.forSource === 'function' ? await browser.forSource(source) : browser;
  const assets = assetsBySource?.[source];
  if (!Array.isArray(assets) || assets.length < 3) throw captureError('worker_not_ready', 'source assets are unavailable');
  const provenance = sourceProvenance(assets);
  const startedAt = Date.now();
  const capturedAt = new Date(startedAt).toISOString();
  let requestedUrl = options.pageUrl;
  let resumeScrollY = 0;
  let frontierUrlMismatch = false;

  if (options.acquisitionRound === 1) {
    // Any fresh navigation invalidates the prior source checkpoint, including failed captures.
    sourceFrontiers.delete(source);
    const navigation = await page.navigate(requestedUrl, timeLeft(deadline));
    if (navigation.errorText) throw captureError('navigation_failed', 'Chrome could not navigate to the requested source page');
  } else {
    const state = sourceFrontiers.get(source);
    if (!state) throw captureError('source_frontier_unavailable', 'the source tab has no retained round-1 frontier');
    if (options.explicitPageUrl && pageKey(source, options.pageUrl) !== state.pageUrl) {
      throw captureError('source_frontier_unavailable', 'round-2 page URL does not match the retained source frontier');
    }
    requestedUrl = state.requestedUrl;
    if (!continuationMatches(state, source, requestedUrl, options.continuation)) {
      throw captureError('unsupported_continuation', 'continuation does not match the retained source page frontier');
    }
    const actualUrl = await page.evaluate('location.href', timeLeft(deadline));
    frontierUrlMismatch = pageKey(source, actualUrl) !== state.pageUrl;
    resumeScrollY = options.continuation.startScrollY;
    await page.evaluate(`globalThis.XHeadlessPoC?.scrollSourceTo ? globalThis.XHeadlessPoC.scrollSourceTo(${resumeScrollY}) : window.scrollTo({top:${resumeScrollY},behavior:'instant'})`, timeLeft(deadline));
    await sleep(Math.min(options.continuation.settleMs, timeLeft(deadline)));
  }

  await page.evaluate(`globalThis.AkuHeadlessCapturePolicy={allowContentExpansion:${Boolean(options.acquisitionRound === 1 && options.pendingContentPolicy === 'reveal_if_present' && options.sameTabMutationAllowed)},deadlineAt:${deadline}}`, timeLeft(deadline));
  let readinessDeadline = Math.min(deadline, Date.now() + options.hydrationMs);
  let photoResolution = null;
  let mediaSettleDeadline = null;
  let snapshot = null;
  while (Date.now() < readinessDeadline) {
    ensureTime(deadline);
    if (options.acquisitionRound === 1 || snapshot?.posts.length) await sleep(Math.min(300, Math.max(25, readinessDeadline - Date.now())));
    try {
      await inject(page, assets, deadline);
      snapshot = await collect(page, source, deadline);
    } catch (error) {
      if (isTransientContextError(error)) continue;
      throw error;
    }
    if (snapshot.loginRequired || snapshot.challengeDetected || snapshot.sourceUnavailable) break;
    if (payload?.mode === 'recapture_media' && payload.reason === 'missing_media'
        && source === 'facebook' && options.acquisitionRound === 1
        && options.explicitPageUrl && browser.backend !== 'browser_quiet_hidden') {
      const actualUrl=await page.evaluate('location.href',timeLeft(deadline));
      const photo=photoRecaptureObservation(snapshot.photoEvidence,requestedUrl,actualUrl,capturedAt,provenance);
      if(photo)return photo;
    }
    if (!photoResolution && source === 'facebook' && options.acquisitionRound === 1
        && options.explicitPageUrl && browser.backend !== 'browser_quiet_hidden') {
      const parent=verifiedPhotoParent(snapshot.photoEvidence, requestedUrl);
      if(parent){
        photoResolution={...parent,photoUrl:requestedUrl};
        requestedUrl=parent.url;
        const navigation=await page.navigate(requestedUrl,timeLeft(deadline));
        if(navigation.errorText)throw captureError('navigation_failed','Chrome could not navigate to the verified parent');
        snapshot=null;
        readinessDeadline=Math.min(deadline,Date.now()+options.hydrationMs);
        continue;
      }
    }
    if (snapshot.posts.length) {
      // Post text can render before its attachment shell hydrates. Re-sample
      // only missing expected URLs, within both source readiness and a 3s bound.
      const pendingMedia = ['x','instagram','linkedin'].includes(source) && snapshot.posts.some(post =>
        Array.isArray(post.mediaEvidence?.expectedWithoutUrl) && post.mediaEvidence.expectedWithoutUrl.length > 0);
      if (!pendingMedia) break;
      mediaSettleDeadline ??= Math.min(readinessDeadline, Date.now() + 3000);
      if (Date.now() >= mediaSettleDeadline) break;
    }
  }
  if (!snapshot) throw captureError('empty_unverified', 'source page produced no verifiable snapshot before the readiness deadline');
  if(source==='instagram' && options.explicitPageUrl && snapshot.posts.length===0) {
    snapshot=await resolveInstagramNativeTarget({page,requestedUrl,snapshot,resolver:assets?.structuredFeedResolver,deadlineAt:deadline});
  }
  const stateError = sourceStateError(snapshot);
  if (stateError) throw stateError;
  if (source === 'facebook' && options.explicitPageUrl && snapshot.posts.length === 0) {
    // A saved feed post can later open an unavailable native page. Keep this
    // distinct from an empty extractor and from an account-wide source outage.
    const actualUrl = await page.evaluate('location.href', timeLeft(deadline));
    if (canonicalSourceURL(source, actualUrl) === requestedUrl
        && await page.evaluate(`(${facebookTargetUnavailable.toString()})()`, timeLeft(deadline))) {
      throw captureError('target_unavailable', 'Facebook reports that this post is unavailable in the current session');
    }
  }
  if (frontierUrlMismatch) throw captureError('source_frontier_unavailable', 'the retained source tab navigated away from its frontier');

  if (options.acquisitionRound === 2) {
    const visible = new Set(snapshot.posts.map(post => canonicalPostId(source, post)));
    if (!options.continuation.anchorKeys.some(anchor => visible.has(anchor))) {
      throw captureError('source_frontier_unavailable', 'retained source page did not show a continuation anchor after restoration');
    }
  }

  let freshness;
  {
    const recovered = await recoverHeadlessFreshness({source, contract: sourceFreshnessContractFor(assets, source),
      page, snapshot, options, deadlineAt: deadline,
      backend: browser.backend, collectSnapshot: async (phaseDeadline) => {
        await inject(page, assets, phaseDeadline);
        return collect(page, source, phaseDeadline);
      }});
    snapshot = recovered.snapshot;
    freshness = recovered.freshness;
    const freshnessStateError = sourceStateError(snapshot);
    if (freshnessStateError) throw freshnessStateError;
  }

  let quoteRecovery = null;
  let quoteIdentityProbe = null;
  if (source === 'x' && options.acquisitionRound === 1) {
    const resolution = await probeQuoteNavigation(page, snapshot, { url: requestedUrl, deadlineAt: deadline }, async () => {
      ensureTime(deadline);
      await inject(page, assets, deadline);
      return collect(page, source, deadline);
    });
    quoteRecovery = resolution?.recovery || null;
    quoteIdentityProbe = resolution?.probe || null;
  }
  snapshot = applyQuoteRecovery(snapshot, quoteRecovery);
  snapshot = await resolveSnapshotStructuredMedia(page, source, snapshot, assets, deadline);

  if(photoResolution) snapshot=await bindPhotoParentSnapshot(page,snapshot,photoResolution,deadline);

  const snapshots = [];
  const seenIds = new Set();
  let postEvidenceBytes = 0;
  let lastNewCandidateCount = 0;
  let unchangedRounds = 0;
  let stopReason = options.scrolls === 0 ? 'scrolls_zero' : 'scroll_limit';
  const originalScrollY = options.acquisitionRound === 1 ? 0 : resumeScrollY;
  const textRecovery = source === 'x' ? createXTextRecovery({ browser, assets, deadlineAt: deadline }) : null;
  try {
    for (let scroll = 0; scroll <= options.scrolls; scroll++) {
      ensureTime(deadline);
      if (scroll > 0) {
        await page.evaluate(`globalThis.XHeadlessPoC?.scrollSourceBy ? globalThis.XHeadlessPoC.scrollSourceBy(${options.scrollFraction}) : window.scrollBy(0,Math.round(innerHeight*${options.scrollFraction}))`, timeLeft(deadline));
        await sleep(Math.min(options.scrollSettleMs, timeLeft(deadline)));
        snapshot = await collect(page, source, deadline);
        snapshot = applyQuoteRecovery(snapshot, quoteRecovery);
        const nextError = sourceStateError(snapshot);
        if (nextError) throw nextError;
        snapshot = await resolveSnapshotStructuredMedia(page, source, snapshot, assets, deadline);
        if(photoResolution) snapshot=await bindPhotoParentSnapshot(page,snapshot,photoResolution,deadline);
      }
      if (textRecovery) snapshot = await textRecovery.recoverSnapshot(snapshot);
      const previousCount = seenIds.size;
      const posts = [];
      let evidenceLimitReached = false;
      for (const post of snapshot.posts.slice(0, options.maxBlocksPerSnapshot)) {
        const bytes = Buffer.byteLength(JSON.stringify(post), 'utf8');
        if (postEvidenceBytes + bytes > MAX_POST_EVIDENCE_BYTES) { evidenceLimitReached = true; break; }
        posts.push(post);
        postEvidenceBytes += bytes;
      }
      if (evidenceLimitReached && posts.length === 0) { stopReason = 'evidence_size_limit'; break; }
      for (const post of posts) seenIds.add(post.id);
      lastNewCandidateCount = seenIds.size - previousCount;
      snapshots.push({ ...snapshot, posts, capturedAt: new Date().toISOString(), ...(scroll === 0 && quoteIdentityProbe ? { quoteIdentityProbe } : {}) });
      unchangedRounds = seenIds.size === previousCount ? unchangedRounds + 1 : 0;
      if (evidenceLimitReached) { stopReason = 'evidence_size_limit'; break; }
      if (unchangedRounds >= 3) { stopReason = 'three_rounds_without_new_identity'; break; }
    }
  } finally {
    await textRecovery?.close();
  }
  if (!seenIds.size) throw Object.assign(captureError('empty_unverified', 'no source post evidence was captured'),
    { diagnostics: emptyCaptureDiagnostics(snapshots) });
  const last = snapshots.at(-1);
  const anchorKeys = [...new Set(last.posts.map(post => canonicalPostId(source, post)).filter(Boolean))].slice(0, 3);
  const scrollY = Number.isFinite(last.scroll?.y) ? Math.max(0, Math.trunc(last.scroll.y)) : resumeScrollY;
  const height = Number.isFinite(last.scroll?.height) ? Math.trunc(last.scroll.height) : 0;
  const viewport = Number.isFinite(last.scroll?.viewportHeight) ? Math.trunc(last.scroll.viewportHeight) : 0;
  const frontier = { scrollY, anchorKeys, newCandidateCount: lastNewCandidateCount,
    hasMoreCandidateSignal: height > scrollY + viewport };
  sourceFrontiers.set(source, { source, requestedUrl, pageUrl: pageKey(source, requestedUrl), frontier });
  if (options.restoreScroll) await page.evaluate(`globalThis.XHeadlessPoC?.scrollSourceTo ? globalThis.XHeadlessPoC.scrollSourceTo(${originalScrollY}) : window.scrollTo({top:${originalScrollY},behavior:'instant'})`, timeLeft(deadline)).catch(() => {});
  try {
    const observation=toObservation({
      source, requestedUrl, snapshots, provenance, capturedAt, stopReason,
      captureMode: browser.backend === 'browser_quiet_hidden' ? 'browser_quiet_hidden' : 'headless_worker',
      frontier,
      freshness,
    });
    if(photoResolution) observation.coverage.photoParentResolution={status:'verified',photoId:photoResolution.photoId,
      parentPlatformId:photoResolution.nativeId,provenance:'structured_photo_parent_and_matching_native_post'};
    return observation;
  } catch (error) {
    if (error?.code === 'invalid_observation') error.diagnostics = emptyCaptureDiagnostics(snapshots);
    throw error;
  }
}

export function verifiedPhotoParent(evidence, requestedUrl) {
  if(evidence?.status!=='verified_photo_media'||evidence.identityKind!=='photo'||!evidence.parent)return null;
  try{
    const photo=new URL(requestedUrl),p=evidence.parent,u=new URL(p.url);
    const ids=[...photo.searchParams.getAll('fbid'),...photo.searchParams.getAll('photo_id')];
    if(photo.origin!=='https://www.facebook.com'||!/^\/photo(?:\.php|\/)?$/.test(photo.pathname)
      ||!ids.length||!ids.every(id=>/^\d+$/.test(id)&&id===evidence.photoId))return null;
    const native=u.pathname.match(/^\/[^/]+\/posts\/(pfbid[A-Za-z0-9]+|\d+)\/?$/)?.[1];
    if(u.origin!=='https://www.facebook.com'||u.username||u.password||u.search||u.hash||!native
      ||p.nativeId!==`facebook:post:${native}`||typeof p.author!=='string'||!p.author.trim()||p.author.length>1200
      ||typeof p.text!=='string'||p.text.length>4000)return null;
    const media=new URL(evidence.media?.url);
    if(media.protocol!=='https:'||media.username||media.password||media.port||!/(^|\.)fbcdn\.net$/.test(media.hostname))return null;
    return {...p,photoId:evidence.photoId,mediaPath:media.origin+media.pathname};
  }catch{return null;}
}

async function bindPhotoParentSnapshot(page,snapshot,parent,deadline) {
  const actual=await page.evaluate('location.href',timeLeft(deadline));
  const norm=s=>String(s||'').replace(/\s+/g,' ').trim();
  const posts=snapshot.posts.filter(p=>p.id===parent.nativeId);
  const matches=p=>norm(p.author)===norm(parent.author)&&norm(p.text)===norm(parent.text)
    &&p.media?.some(m=>{try{const u=new URL(m.url);return m.kind==='image'&&u.origin+u.pathname===parent.mediaPath;}catch{return false;}});
  const routeMatches=canonicalSourceURL('facebook',actual)===parent.url;
  if(!routeMatches||!posts.length||posts.some(p=>!matches(p)))
    throw Object.assign(captureError('photo_parent_unverified','Parent post did not corroborate the photo metadata'),{
      diagnostics:{stage:'photo_parent_corroboration',routeMatches,
        candidateCount:Math.min(snapshot.posts.length,1_000_000),
        nativeIdentityCandidateCount:Math.min(posts.length,1_000_000),
        authorMatchCount:Math.min(posts.filter(p=>norm(p.author)===norm(parent.author)).length,1_000_000),
        textMatchCount:Math.min(posts.filter(p=>norm(p.text)===norm(parent.text)).length,1_000_000),
        imagePathMatchCount:Math.min(posts.filter(p=>p.media?.some(m=>{try{const u=new URL(m.url);return m.kind==='image'&&u.origin+u.pathname===parent.mediaPath;}catch{return false;}})).length,1_000_000),
        corroboratedCandidateCount:Math.min(posts.filter(matches).length,1_000_000)}
    });
  return {...snapshot,posts};
}

// Error diagnostics contain bounded structural counts, never page text, URLs or IDs.
// Runs in the page. Only recognize a standalone native-page notice, never
// matching words inside a feed post, quote, or caption. No page text is returned.
export function facebookTargetUnavailable() {
  if (document.readyState !== 'complete') return false;
  if (document.querySelector('[role="article"], [role="feed"], [aria-posinset], [data-ad-preview="message"], [data-ad-comet-preview="message"]')) return false;
  const root = document.querySelector('[role="main"], main');
  if (!root) return false;
  const text = String(root.innerText || '');
  if (text.length > 8000) return false;
  return text.split(/\r?\n/).some(line => /^(?:this content (?:isn['’]t|is not) available(?: right now)?|content not found|konten ini tidak tersedia)[.!]?$/i.test(line.trim()));
}

export function emptyCaptureDiagnostics(snapshots) {
  const count = value => Number.isSafeInteger(value) && value >= 0 && value <= 1_000_000 ? value : null;
  const flag = value => typeof value === 'boolean' ? value : null;
  const records = value => Array.isArray(value) ? value.filter(item=>item && typeof item==='object' && !Array.isArray(item)) : [];
  return {sampleCount:snapshots.length,samples:records(snapshots).slice(-2).map(snapshot=>({
    candidateCount:count(snapshot.candidateCount), rejected:count(snapshot.rejected),
    photoEvidence:{status:['not_photo_route','evidence_limit','photo_metadata_missing','conflicting_photo_binding','photo_owner_missing','photo_image_not_visible','verified_photo_media'].includes(snapshot.photoEvidence?.status)?snapshot.photoEvidence.status:null,
      postBinding:snapshot.photoEvidence?.postBinding==='unverified'?'unverified':null},
    dialogScope:{status:['page','ambiguous_dialog','dialog_pending_identity','native_post_dialog'].includes(snapshot.dialogScope?.status)?snapshot.dialogScope.status:null,
      candidateCount:count(snapshot.dialogScope?.candidateCount)},
    discovery:{
      structuralCandidates:count(snapshot.candidateDiagnostics?.structuralCandidates),
      eligibleCandidates:count(snapshot.candidateDiagnostics?.eligibleCandidates),
      actionAnchoredCandidates:count(snapshot.candidateDiagnostics?.actionAnchoredCandidates),
    },
    authenticatedUiObserved:flag(snapshot.authenticatedUiObserved),documentReady:flag(snapshot.documentReady),
    loginRequired:flag(snapshot.loginRequired),challengeDetected:flag(snapshot.challengeDetected),
    scrollY:count(snapshot.scroll?.y),
    rejectionReasons:Object.fromEntries(Object.entries(snapshot.rejectionReasons || {}).slice(0,16)
      .filter(([key,value])=>/^[a-z_]{1,64}$/.test(key) && count(value) !== null)),
    boundaries:records(snapshot.boundaryDiagnostics).slice(0,6).map(boundary=>({
      positionWrapper:flag(boundary.positionWrapper),narrowed:flag(boundary.narrowed),
      nestedPostBoundaries:count(boundary.nestedPostBoundaries),
      allActions:count(boundary.allActions),ownActions:count(boundary.ownActions),
      allBodies:count(boundary.allBodies),ownBodies:count(boundary.ownBodies),
      allAnchors:count(boundary.allAnchors),ownAnchors:count(boundary.ownAnchors),
    })),
    hover:records(snapshot.identityDiagnostics).slice(0,4).map(identity=>({
      eligibleHoverAnchors:count(identity.eligibleHoverAnchors),
      attempted:flag(identity.hovered?.attempted),
      permalinkRecovered:flag(identity.hovered?.permalinkRecoveredByHover),
    })),
  }))};
}

function canonicalPostId(source, post) {
  if (!post || typeof post.id !== 'string') return '';
  if (source === 'x' && /^\d+$/.test(post.id)) return `x:status:${post.id}`;
  return post.id;
}

async function inject(page, assets, deadline) {
  for (const asset of assets) if (asset.execute) await page.evaluate(asset.content, timeLeft(deadline));
}

async function collect(page, source, deadline) {
  ensureTime(deadline);
  const targets = await page.evaluate('globalThis.XHeadlessPoC?.prepareEvidenceTargets?.() || []', timeLeft(deadline));
  if (Array.isArray(targets)) {
    for (const target of targets.slice(0, 4)) {
      ensureTime(deadline);
      if (!target || !Number.isFinite(target.x) || !Number.isFinite(target.y) || typeof target.token !== 'string') continue;
      await page.send('Input.dispatchMouseEvent', { type: 'mouseMoved', x: Math.trunc(target.x), y: Math.trunc(target.y) }, timeLeft(deadline));
      await sleep(Math.min(650, timeLeft(deadline)));
      await page.evaluate(`globalThis.XHeadlessPoC.recordHoverEvidence(${JSON.stringify(target.token)})`, timeLeft(deadline));
    }
    if (targets.length) await page.send('Input.dispatchMouseEvent', { type: 'mouseMoved', x: 0, y: 0 }, timeLeft(deadline));
  }
  ensureTime(deadline);
  const encoded = await page.evaluate('(async () => JSON.stringify(await globalThis.XHeadlessPoC.collect()))()', timeLeft(deadline));
  if (typeof encoded !== 'string' || Buffer.byteLength(encoded, 'utf8') > MAX_SNAPSHOT_BYTES) throw captureError('evidence_too_large', 'page snapshot exceeds the 4 MiB limit');
  let snapshot;
  try { snapshot = JSON.parse(encoded); } catch { throw captureError('invalid_observation', 'source extractor returned invalid JSON'); }
  if (!snapshot || !Array.isArray(snapshot.posts)) throw captureError('invalid_observation', 'source extractor returned no posts array');
  return snapshot;
}

async function resolveSnapshotStructuredMedia(page, source, snapshot, assets, deadline) {
  const resolveMedia=source==='instagram' || source==='linkedin' ? resolveAdditionalSourceMedia : resolveStructuredMedia;
  const resolution = await resolveMedia({
    page,
    source,
    posts: snapshot.posts,
    resolver: assets?.structuredMediaResolver,
    feedResolver: assets?.structuredFeedResolver,
    deadlineAt: deadline,
  });
  return { ...snapshot, posts: resolution.posts, structuredMediaResolution: resolution.summary };
}

function sourceStateError(snapshot) {
  if (snapshot.loginRequired === true) return captureError('login_required', 'source requires an authenticated login');
  if (snapshot.challengeDetected === true) return captureError('challenge_required', 'source presented an access challenge');
  if (snapshot.sourceUnavailable) return captureError('source_unavailable', 'source reports that content is unavailable');
  return null;
}

function ensureTime(deadline) {
  if (Date.now() >= deadline) throw captureError('capture_timeout', 'capture exceeded its configured worker limit');
}

function timeLeft(deadline) {
  ensureTime(deadline);
  return Math.max(1, deadline - Date.now());
}

function isTransientContextError(error) {
  return /context.*destroyed|cannot find context/i.test(String(error?.message || error));
}

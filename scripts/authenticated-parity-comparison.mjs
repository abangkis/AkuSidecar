import { canonicalSourceURL } from '../internal/collection/headless/worker/observation.mjs';

const normalized = value => String(value || '').replace(/\s+/g, ' ').trim();
const urlNormalizedProse = value => normalized(value).replace(/https?:\/\/[^\s<>"']+/giu, '[URL]');
const blocksOf = result => (result?.snapshots || []).flatMap(snapshot => snapshot.blocks || []);
const mediaKey = item => JSON.stringify([item?.kind || null, item?.url || null, item?.posterUrl || null,
  item?.playbackUrl || null, item?.playbackMode || null]);
const urlPath = value => {try {const url=new URL(value);return `${url.origin}${url.pathname}`;}catch{return value || null;}};
const mediaPathKey = item => JSON.stringify([item?.kind || null,urlPath(item?.url),urlPath(item?.posterUrl),
  urlPath(item?.playbackUrl),item?.playbackMode || null]);
export function nativeIdentity(source, permalink) {
  const canonical = canonicalSourceURL(source, permalink);
  if (!canonical) return null;
  const url = new URL(canonical);
  if (source === 'x') return `x:status:${url.pathname.match(/\/status\/(\d+)/)?.[1]}`;
  const pathId = url.pathname.match(/\/(?:posts|permalink|videos)\/(pfbid[A-Za-z0-9]+|\d+)(?:\/|$)/i)?.[1];
  const storyId = /\/(?:story|permalink)\.php$/i.test(url.pathname) ? url.searchParams.get('story_fbid') : null;
  const watchId = /^\/watch\/$/i.test(url.pathname) ? url.searchParams.get('v') : null;
  if (watchId !== null) return /^\d{1,32}$/.test(watchId) ? `facebook:post:${watchId}` : null;
  const id = pathId || storyId;
  return id && /^(?:pfbid[A-Za-z0-9]+|\d+)$/.test(id) ? `facebook:post:${id}` : null;
}

export function comparisonScope(baseline, quiet = false) {
  const driver = quiet ? 'quiet' : 'headless';
  if (baseline?.scope === 'saved_browser_timeline_vs_live_headless_sequential') return `saved_timeline_vs_live_${driver}_sequential`;
  if (baseline?.scope === 'previous_headless_observed_video_target_not_legacy_parity') return `previous_headless_vs_live_${driver}_sequential`;
  return `unverified_baseline_vs_live_${driver}_sequential`;
}

// ID/URL agreement only checks internal consistency. In particular, a media
// parent's numeric ID plus an inferred author path is not an observed post link.
export function permalinkEvidence(target) {
  if (target?.source !== 'facebook') return 'not_applicable';
  const source = target.presentation?.permalinkSource;
  if (source === 'media_parent_id') return 'inferred_media_parent';
  if (['direct_anchor', 'post_anchor', 'story_anchor', 'video_anchor', 'embedded_video_anchor'].includes(source)) return 'observed_anchor';
  return 'unknown';
}

export function compareReport(report) {
  const captures = Array.isArray(report.captures) ? report.captures : [];
  const targets = report.baseline?.targets || [];
  const cases = targets.map(target => {
    const evidence = captures.filter(capture => capture.source === target.source && capture.ok)
      .flatMap(capture => blocksOf(capture.result)).filter(block => block.platformId === target.platformId);
    const native = canonicalSourceURL(target.source, target.permalink);
    const signatures = new Set(evidence.map(block => JSON.stringify([normalized(block.author), normalized(block.text)])));
    const result = {source:target.source, platformId:target.platformId, observedCopies:evidence.length,
      baselinePermalinkEvidence:permalinkEvidence(target),
      baselineObservedAt:target.observedAt || null, status:'not_observed'};
    if (!native) return {...result,status:'invalid_baseline'};
    if (nativeIdentity(target.source,native) !== target.platformId) return {...result,status:'baseline_native_id_url_unverified'};
    if (!evidence.length) return result;
    if (signatures.size > 1) return {...result,status:'ambiguous_identity_binding'};
    const block = evidence[0];
    const permalink = canonicalSourceURL(target.source, block.permalink);
    if (!permalink || permalink !== native) return {...result,status:'native_permalink_mismatch'};
    if (nativeIdentity(target.source,permalink) !== block.platformId) return {...result,status:'observed_native_id_url_unverified'};
    if (normalized(block.author) !== normalized(target.author)) return {...result,status:'author_binding_mismatch'};
    const before = normalized(target.text), after = normalized(block.text);
    const proseEqualWithUrlTokensReplaced = urlNormalizedProse(before) === urlNormalizedProse(after);
    const beforeMedia = Array.isArray(target.media) ? target.media : null;
    const afterMedia = Array.isArray(block.media) ? block.media : null;
    const a = beforeMedia ? [...new Set(beforeMedia.map(mediaKey))].sort() : null;
    const b = afterMedia ? [...new Set(afterMedia.map(mediaKey))].sort() : null;
    const pathA = beforeMedia ? [...new Set(beforeMedia.map(mediaPathKey))].sort() : null;
    const pathB = afterMedia ? [...new Set(afterMedia.map(mediaPathKey))].sort() : null;
    return {...result,status:'native_identity_and_author_match',
      textEqual:before === after, textPrefixCompatible:Boolean(before && after && (before.startsWith(after) || after.startsWith(before))),
      proseEqualWithUrlTokensReplaced,
      baselineTextCharacters:Array.from(before).length, observedTextCharacters:Array.from(after).length,
      baselineMediaCount:beforeMedia?.length ?? null, observedMediaCount:afterMedia?.length ?? null,
      exactMediaSetsEqual:a && b ? JSON.stringify(a) === JSON.stringify(b) : null,
      sameHostPathMediaSets:pathA && pathB ? JSON.stringify(pathA) === JSON.stringify(pathB) : null,
      publishedAtObserved:block.publishedAt || null, timestampSource:block.presentation?.timestampSource || null,
      relationshipObserved:block.relationshipType || null,
      mediaRecoveryStatus:block.mediaRecovery?.status || 'unknown',
      unknownVideo:block.mediaRecovery?.unknownVideo || null,
      textStatus:block.captureQuality?.textStatus || 'unknown',
      qualityStatus:block.captureQuality?.status || 'unknown',
      limitations:block.captureQuality?.limitations || []};
  });
  return {schema:'aku.authenticated-parity-summary.v1',
    scope:comparisonScope(report?.baseline, report?.execution?.workerMode === 'production_quiet_driver_packaged_worker'
      || report?.execution?.scope?.endsWith('_quiet_sequential') === true),
    fullParityVerified:false,
    limitations:['saved baseline is not a simultaneous headed capture','coverage is bounded',
      'missing identity is not proof of source absence','exact media URL mismatch may require CDN evidence',
      'ID/URL consistency does not establish that an inferred permalink opens the saved post',
      'URL tokens replaced for prose comparison do not establish displayed URL or destination equality',
      'unknown quality and video resolution remain unknown'],
    sources:['x','facebook'].map(source => {
      const sourceCaptures = captures.filter(capture => capture.source === source);
      const blocks = sourceCaptures.filter(capture => capture.ok).flatMap(capture => blocksOf(capture.result));
      return {source,captureCount:sourceCaptures.length,
        successfulCaptures:sourceCaptures.filter(capture => capture.ok).length,
        failureCodes:sourceCaptures.filter(capture => !capture.ok).map(capture => capture.error?.code || 'unknown'),
        uniqueNativeIds:new Set(blocks.map(block => block.platformId).filter(Boolean)).size,
        observedBlocks:blocks.length, mediaItems:blocks.reduce((total,block)=>total+(block.media?.length || 0),0),
        captures:sourceCaptures.map(capture=>({kind:capture.kind,ok:capture.ok,
          snapshots:capture.result?.snapshots?.length ?? null,
          authenticatedUiObserved:capture.result?.coverage?.authenticatedUiObserved ?? null,
          loginRequired:capture.result?.coverage?.loginRequired ?? null,
          challengeDetected:capture.result?.coverage?.challengeDetected ?? null,
          stopReason:capture.result?.coverage?.stopReason ?? null,
          frontierAnchorCount:capture.result?.coverage?.frontier?.anchorKeys?.length ?? null,
          withinSnapshotDuplicateIds:capture.result?.snapshots ? capture.result.snapshots.reduce((total,snapshot)=>{
            const ids=(snapshot.blocks || []).map(block=>block.platformId).filter(Boolean);
            return total+ids.length-new Set(ids).size;
          },0) : null}))};
    }),cases};
}

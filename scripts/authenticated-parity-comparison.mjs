import { canonicalSourceURL } from '../internal/collection/headless/worker/observation.mjs';

const normalized = value => String(value || '').replace(/\s+/g, ' ').trim();
const urlNormalizedProse = value => normalized(value).replace(/https?:\/\/[^\s<>"']+/giu, '[URL]');
const blocksOf = result => (result?.snapshots || []).flatMap(snapshot => snapshot.blocks || []);
const mediaKey = item => JSON.stringify([item?.kind || null, item?.url || null, item?.posterUrl || null,
  item?.playbackUrl || null, item?.playbackMode || null]);
const urlPath = value => {try {const url=new URL(value);return `${url.origin}${url.pathname}`;}catch{return value || null;}};
const mediaPathKey = item => JSON.stringify([item?.kind || null,urlPath(item?.url),urlPath(item?.posterUrl),
  urlPath(item?.playbackUrl),item?.playbackMode || null]);
const xAuthorKey = (value, permalink) => {
  const match = normalized(value).match(/^(.*?)\s+@([A-Za-z0-9_]{1,15})(?:\s+[·•]\s+\d+[smhd])?$/u);
  if (!match || !match[1]) return null;
  try {
    const handle = new URL(permalink).pathname.split('/')[1];
    return handle?.toLowerCase() === match[2].toLowerCase()
      ? JSON.stringify([match[1], match[2].toLowerCase()]) : null;
  } catch { return null; }
};
const mediaAssetKey = item => {
  if (item?.kind !== 'image') return mediaPathKey(item);
  try {
    const url = new URL(item.url);
    const match = url.pathname.match(/^\/media\/([A-Za-z0-9_-]+)(?:\.(png|jpe?g|webp))?$/i);
    if (url.origin !== 'https://pbs.twimg.com' || !match) return mediaKey(item);
    const normalizeFormat = value => value === 'jpeg' ? 'jpg' : value;
    const extension = normalizeFormat((match[2] || '').toLowerCase());
    const formats = url.searchParams.getAll('format').map(v => normalizeFormat(v.toLowerCase()));
    const format = extension || formats[0];
    if (!['png','jpg','webp'].includes(format) || formats.some(v => v !== format)) return mediaKey(item);
    return JSON.stringify(['x_image_asset_format', match[1], format,
      item.posterUrl || null, item.playbackUrl || null, item.playbackMode || null]);
  } catch { return mediaKey(item); }
};
export function nativeIdentity(source, permalink) {
  const canonical = canonicalSourceURL(source, permalink);
  if (!canonical) return null;
  const url = new URL(canonical);
  if (source === 'x') return `x:status:${url.pathname.match(/\/status\/(\d+)/)?.[1]}`;
  if (source === 'instagram') {
    const match=url.pathname.match(/^\/(p|reel|tv)\/([A-Za-z0-9_-]+)\/$/);
    return match ? `instagram:${match[1]}:${match[2]}` : null;
  }
  if (source === 'linkedin') {
    const match=url.pathname.match(/urn:li:(activity|ugcPost|share):(\d{5,30})/i);
    return match ? `linkedin:${match[1].toLowerCase()}:${match[2]}` : null;
  }
  const pathId = url.pathname.match(/\/(?:posts|permalink|videos|reel)\/(pfbid[A-Za-z0-9]+|\d+)(?:\/|$)/i)?.[1];
  const storyId = /\/(?:story|permalink)\.php$/i.test(url.pathname) ? url.searchParams.get('story_fbid') : null;
  const watchId = /^\/watch\/$/i.test(url.pathname) ? url.searchParams.get('v') : null;
  if (watchId !== null) return /^\d{1,32}$/.test(watchId) ? `facebook:post:${watchId}` : null;
  const id = pathId || storyId;
  return id && /^(?:pfbid[A-Za-z0-9]+|\d+)$/.test(id) ? `facebook:post:${id}` : null;
}

export function comparisonScope(baseline, quiet = false) {
  const driver = quiet ? 'quiet' : 'headless';
  if (baseline?.scope === 'new_source_headless_qualification_baseline') return `new_source_browser_targets_vs_live_${driver}_sequential`;
  if (baseline?.scope === 'fresh_browser_feed_acquisition_baseline') return `fresh_browser_feed_targets_vs_live_${driver}_sequential`;
  if (baseline?.scope === 'native_bridge_media_recapture_foreground_baseline') return `native_bridge_media_recapture_vs_live_${driver}_sequential`;
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
    let evidence = captures.filter(capture => capture.source === target.source && capture.ok)
      .flatMap(capture => blocksOf(capture.result)).filter(block => block.platformId === target.platformId);
    const native = canonicalSourceURL(target.source, target.permalink);
    let result = {source:target.source, platformId:target.platformId, observedCopies:evidence.length,
      baselinePermalinkEvidence:permalinkEvidence(target),
      baselineObservedAt:target.observedAt || null, status:'not_observed'};
    if (!native) return {...result,status:'invalid_baseline'};
    let photoParent = null;
    const baselineUrl = new URL(native);
    if (target.source === 'facebook' && /^\/photo(?:\/|\.php)$/i.test(baselineUrl.pathname)) {
      const ids = [...baselineUrl.searchParams.getAll('fbid'),...baselineUrl.searchParams.getAll('photo_id')];
      const photoId = ids[0];
      if (!photoId || !/^\d{1,32}$/.test(photoId) || ids.some(id => id !== photoId)
          || ![`facebook:post:${photoId}`,`facebook:photo:${photoId}`].includes(target.platformId)) {
        return {...result,status:'baseline_photo_identity_unverified'};
      }
      const mappings = captures.filter(c => c.source === 'facebook' && c.ok).flatMap(c => {
        const proof = c.result?.coverage?.photoParentResolution;
        if (proof?.status !== 'verified' || proof.photoId !== photoId
            || proof.provenance !== 'structured_photo_parent_and_matching_native_post'
            || nativeIdentity('facebook', c.result.pageUrl) !== proof.parentPlatformId) return [];
        return blocksOf(c.result).filter(b => b.platformId === proof.parentPlatformId
          && nativeIdentity('facebook',b.permalink) === proof.parentPlatformId).map(block => ({block,proof}));
      });
      const parents = new Set(mappings.map(v => v.proof.parentPlatformId));
      if (parents.size !== 1) return {...result,status:parents.size ? 'ambiguous_photo_parent_binding' : 'photo_parent_binding_unverified'};
      evidence = mappings.map(v => v.block);
      photoParent = mappings[0].proof;
      result = {...result,observedCopies:evidence.length,photoId,parentPlatformId:photoParent.parentPlatformId,
        photoParentProvenance:photoParent.provenance};
    } else if (nativeIdentity(target.source,native) !== target.platformId) return {...result,status:'baseline_native_id_url_unverified'};
    if (!evidence.length) return result;
    const signatures = new Set(evidence.map(block => JSON.stringify([normalized(block.author), normalized(block.text)])));
    if (signatures.size > 1) return {...result,status:'ambiguous_identity_binding'};
    const block = evidence[0];
    const permalink = canonicalSourceURL(target.source, block.permalink);
    if (!permalink || !photoParent && permalink !== native) return {...result,status:'native_permalink_mismatch'};
    if (nativeIdentity(target.source,permalink) !== block.platformId) return {...result,status:'observed_native_id_url_unverified'};
    const authorRawEqual = normalized(block.author) === normalized(target.author);
    const authorIdentityNormalizedMatch = target.source === 'x' && Boolean(xAuthorKey(target.author,native))
      && xAuthorKey(target.author,native) === xAuthorKey(block.author,permalink);
    if (!authorRawEqual && !authorIdentityNormalizedMatch) return {...result,status:'author_binding_mismatch'};
    const before = normalized(target.text), after = normalized(block.text);
    const proseEqualWithUrlTokensReplaced = urlNormalizedProse(before) === urlNormalizedProse(after);
    const beforeMedia = Array.isArray(target.media) ? target.media : null;
    const afterMedia = Array.isArray(block.media) ? block.media : null;
    const a = beforeMedia ? [...new Set(beforeMedia.map(mediaKey))].sort() : null;
    const b = afterMedia ? [...new Set(afterMedia.map(mediaKey))].sort() : null;
    const pathA = beforeMedia ? [...new Set(beforeMedia.map(mediaPathKey))].sort() : null;
    const pathB = afterMedia ? [...new Set(afterMedia.map(mediaPathKey))].sort() : null;
    const assetA = beforeMedia ? [...new Set(beforeMedia.map(mediaAssetKey))].sort() : null;
    const assetB = afterMedia ? [...new Set(afterMedia.map(mediaAssetKey))].sort() : null;
    return {...result,status:photoParent ? 'verified_photo_parent_and_author_match' : 'native_identity_and_author_match',
      authorRawEqual,authorIdentityNormalizedMatch,
      textEqual:before === after, textPrefixCompatible:Boolean(before && after && (before.startsWith(after) || after.startsWith(before))),
      proseEqualWithUrlTokensReplaced,
      baselineTextCharacters:Array.from(before).length, observedTextCharacters:Array.from(after).length,
      baselineMediaCount:beforeMedia?.length ?? null, observedMediaCount:afterMedia?.length ?? null,
      exactMediaSetsEqual:a && b ? JSON.stringify(a) === JSON.stringify(b) : null,
      sameHostPathMediaSets:pathA && pathB ? JSON.stringify(pathA) === JSON.stringify(pathB) : null,
      sameMediaAssetFormatSets:assetA && assetB ? JSON.stringify(assetA) === JSON.stringify(assetB) : null,
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
    sources:['x','facebook','instagram','linkedin'].map(source => {
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

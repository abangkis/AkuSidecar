import {canonicalSourceURL} from '../internal/collection/headless/worker/observation.mjs';
import {nativeIdentity} from './authenticated-parity-comparison.mjs';

// Project observed acquisition rows only; never fill missing identity, author,
// text or media from the requested URL or target metadata.
export function nativeBridgeRecaptureBaseline(observation, targetUrl) {
  const canonical=canonicalSourceURL('facebook',targetUrl);
  if(!canonical||!nativeIdentity('facebook',canonical))throw Error('Invalid native target');
  if(observation?.source!=='facebook'||observation.coverage?.browserAdapter!=='aku-bridge'
      ||canonicalSourceURL('facebook',observation.pageUrl)!==canonical
      ||typeof observation.capturedAt!=='string'||!Number.isFinite(Date.parse(observation.capturedAt))
      ||!Array.isArray(observation.snapshots))throw Error('Unverified native Bridge observation');
  const expectedId=nativeIdentity('facebook',canonical);
  const rows=observation.snapshots.flatMap(snapshot=>Array.isArray(snapshot.blocks)?snapshot.blocks:[]);
  const targets=rows.filter(row=>row.platformId===expectedId
    &&nativeIdentity('facebook',row.permalink)===expectedId
    &&canonicalSourceURL('facebook',row.permalink)===canonical
    &&typeof row.author==='string'&&typeof row.text==='string').map(row=>({
      ...row,source:'facebook',observedAt:observation.capturedAt,
    }));
  return {schema:'aku.native-bridge-recapture-baseline.v1',
    scope:'native_bridge_media_recapture_foreground_baseline',
    observedAt:observation.capturedAt,targetUrl:canonical,
    captureStatus:targets.length?'exact_target_observed':'exact_target_not_observed',
    observedBlockCount:rows.length,rejectedBlockCount:rows.length-targets.length,
    targets,fullParityVerified:false};
}

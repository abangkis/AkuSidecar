import {compareReport} from './authenticated-parity-comparison.mjs';

const blocksOf=capture=>(capture.result?.snapshots||[]).flatMap(snapshot=>snapshot.blocks||[]);
const signature=block=>JSON.stringify([block.author,block.text,block.permalink,block.media]);
const sourceHashes=capture=>JSON.stringify((capture.result?.coverage?.provenance?.sources||[])
  .map(({path,sha256})=>[path,sha256]).sort((a,b)=>a[0].localeCompare(b[0])));

// Read-only receipt comparison. It compares the requested native target only;
// feed differences from sequential navigation remain outside this contract.
export function compareQuietHeadless(quiet,headless) {
  const modes=[quiet?.execution?.workerMode,headless?.execution?.workerMode];
  const lifecycle=[quiet,headless].every(report=>report?.schema==='aku.authenticated-parity-report.v1'
    &&report.execution?.runtimeControl?.restored===true
    &&report.execution.runtimeControl.profileReleasedBeforeRestore===true
    &&report.execution.runtimeControl.workerExitConfirmed===true);
  if(!lifecycle||modes[0]!=='production_quiet_driver_packaged_worker'||modes[1]!=='packaged_worker') {
    throw new Error('official_source_lifecycle_receipts_required');
  }
  const sameChrome=quiet.workerIdentity?.chromeVersion?.product===headless.workerIdentity?.chromeVersion?.product
    &&Boolean(quiet.workerIdentity?.chromeVersion?.product);
  const cases=['x','facebook'].map(source=>{
    const a=quiet.captures.filter(c=>c.source===source&&c.kind==='target');
    const b=headless.captures.filter(c=>c.source===source&&c.kind==='target');
    if(a.length!==1||b.length!==1||!a[0].ok||!b[0].ok)return {source,status:'target_capture_unavailable'};
    const before=a[0],after=b[0];
    if(!before.targetPlatformId||before.targetPlatformId!==after.targetPlatformId)return {source,status:'requested_target_mismatch'};
    const own=blocksOf(before).filter(block=>block.platformId===before.targetPlatformId);
    if(!own.length)return {source,status:'quiet_target_not_observed'};
    if(new Set(own.map(signature)).size!==1)return {source,status:'ambiguous_quiet_target'};
    const provenanceMatch=sourceHashes(before)===sourceHashes(after)
      &&(before.result.coverage?.provenance?.sources?.length||0)>0;
    const target={...own[0],source,observedAt:before.result.capturedAt};
    const comparison=compareReport({baseline:{targets:[target]},captures:[after]}).cases[0];
    const {platformId,...publicCase}=comparison;
    return {...publicCase,sameChrome,sourceAssetHashesEqual:provenanceMatch,
      quietMediaRecoveryStatus:own[0].mediaRecovery?.status||'unknown',
      quietUnknownVideo:own[0].mediaRecovery?.unknownVideo||null,
      quietCapturedAt:before.result.capturedAt||null,headlessCapturedAt:after.result.capturedAt||null};
  });
  return {schema:'aku.quiet-headless-target-comparison.v1',scope:'native_quiet_vs_headless_targets_sequential',
    fullParityVerified:false,sameChrome,cases,
    limitations:['source captures are sequential, not simultaneous','requested native targets only; feed population may change',
      'source asset equality verifies extraction inputs, not all browser or Bridge behavior',
      'timestamp estimates, unknown media, aliases and overall source quality remain qualified']};
}

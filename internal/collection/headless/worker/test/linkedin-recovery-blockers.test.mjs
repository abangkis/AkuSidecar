import test from 'node:test';
import assert from 'node:assert/strict';
import {projectLinkedInRecoveryBlockers, projectLinkedInPermalinkRecoveryFailures,
  projectLinkedInAnchorMismatchPolls, toObservation} from '../observation.mjs';
import {emptyCaptureDiagnostics} from '../capture.mjs';

const sample = {kind:'dialog',selector:'role_dialog',aria:{labelledBy:'absent',controls:'absent',owns:'absent'},guardVisible:true,
  hasArea:true,intersectsViewport:true,displayed:true,cssVisible:true,opacityPositive:true,hiddenAttribute:false,ariaHidden:false,inert:false,
  ancestors:{inspected:0,complete:true,displayNone:false,visibilityHidden:false,opacityZero:false,hiddenAttribute:false,ariaHidden:false,inert:false}};
const diagnostic = {code:'existing_surface',phase:'pre_recovery_guard',snapshotIndex:0,candidateIndex:0,menuCount:0,dialogCount:1,samples:[sample],complete:true};
const snapshot = extra => ({source:'linkedin',posts:[],linkedinRecoveryBlockers:[diagnostic],...extra});

test('anchor poll diagnostics retain resolved mismatches and exclude invalid counts',()=>{
  const key='linkedinAnchorMismatchPolls';
  const entry={snapshotIndex:0,candidateIndex:2,mismatchPolls:3,horizontalPolls:3,verticalPolls:1,persistence:'resolved'};
  const valid=snapshot({[key]:[{...entry,left:120,author:'Private'}]});
  assert.deepEqual(projectLinkedInAnchorMismatchPolls(valid),{[key]:[entry]});
  const withBreakdown={...entry,verticalGapPolls:0,verticalOverlapPolls:1};
  assert.deepEqual(projectLinkedInAnchorMismatchPolls(snapshot({[key]:[withBreakdown]})),{[key]:[withBreakdown]});
  for(const extra of [{verticalGapPolls:1},{verticalGapPolls:1,verticalOverlapPolls:1}]){
    assert.deepEqual(projectLinkedInAnchorMismatchPolls(snapshot({[key]:[{...entry,...extra}]})),{});
  }
  for(const extra of [{horizontalPolls:0},{verticalPolls:4},{persistence:'single_sample'},{persistence:'unknown'}]){
    assert.deepEqual(projectLinkedInAnchorMismatchPolls(snapshot({[key]:[{...entry,...extra}]})),{});
  }
  assert.deepEqual(projectLinkedInAnchorMismatchPolls({...valid,source:'instagram'}),{});
  assert.deepEqual(projectLinkedInAnchorMismatchPolls(snapshot({[key]:Array(9).fill(entry)})),{});
  assert.deepEqual(emptyCaptureDiagnostics([valid]).samples[0][key],[entry]);
  const observation=toObservation({source:'linkedin',requestedUrl:'https://www.linkedin.com/feed/',
    snapshots:[valid,snapshot()],capturedAt:'2026-11-17T00:00:00Z'});
  assert.deepEqual(observation.coverage[key],[entry]);
  assert.ok(observation.snapshots.every(value=>!Object.hasOwn(value,key)));
});

test('permalink failure coverage is bounded and independent of blocker evidence and admission',()=>{
  const failure={snapshotIndex:0,candidateIndex:2,stage:'ownership',reason:'anchor_mismatch',viewport:'visible',scrollRoot:'partial'};
  const key='linkedinPermalinkRecoveryFailures';
  const failed=snapshot({[key]:[{...failure,privateUrl:'https://private.invalid'}]});
  const invalid=snapshot({[key]:[{...failure,stage:'menu'}]});
  assert.deepEqual(projectLinkedInPermalinkRecoveryFailures(failed),{[key]:[failure]});
  assert.deepEqual(projectLinkedInPermalinkRecoveryFailures(invalid),{});
  assert.deepEqual(projectLinkedInPermalinkRecoveryFailures({...failed,source:'x'}),{});
  assert.deepEqual(projectLinkedInPermalinkRecoveryFailures(snapshot({[key]:Array(9).fill(failure)})),{});
  assert.deepEqual(emptyCaptureDiagnostics([invalid,failed]).samples[1][key],[{...failure,snapshotIndex:1}]);
  const observation=toObservation({source:'linkedin',requestedUrl:'https://www.linkedin.com/feed/',
    snapshots:[invalid,failed],capturedAt:'2026-11-17T00:00:00Z'});
  assert.deepEqual(observation.coverage[key],[{...failure,snapshotIndex:1}]);
  assert.equal(observation.coverage.linkedinRecoveryBlockers.length,2);
  assert.ok(observation.snapshots.every(value=>value.blocks.length===0 && !Object.hasOwn(value,key)));
});

test('native projection preserves blocker evidence for rejected posts and the empty-capture error path',()=>{
  const value=snapshot();
  assert.deepEqual(projectLinkedInRecoveryBlockers(value),{linkedinRecoveryBlockers:[diagnostic]});
  assert.deepEqual(emptyCaptureDiagnostics([value]).samples[0].linkedinRecoveryBlockers,[diagnostic]);
  const observed=toObservation({source:'linkedin',requestedUrl:'https://www.linkedin.com/feed/',snapshots:[value],capturedAt:'2026-11-17T00:00:00Z'});
  assert.deepEqual(observed.coverage.linkedinRecoveryBlockers,[diagnostic]);
  assert.equal(observed.snapshots[0].blocks.length,0);
});

test('native projection strips private extras without reading getters or granting identity',()=>{
  const value={...diagnostic,samples:[{...sample,privateURL:'https://private.invalid'}],privateText:'private'};
  Object.defineProperty(value,'privateGetter',{get(){throw Error('Must not execute');}});
  assert.deepEqual(projectLinkedInRecoveryBlockers(snapshot({linkedinRecoveryBlockers:[value]})),{linkedinRecoveryBlockers:[diagnostic]});
  assert.deepEqual(projectLinkedInRecoveryBlockers(snapshot({source:'x'})),{});
  assert.deepEqual(projectLinkedInRecoveryBlockers({source:'linkedin'}),{});
});

test('native projection preserves unknown short-circuited dialogs and incomplete visibility evidence',()=>{
  const value={...diagnostic,menuCount:1,dialogCount:null,samples:[{...sample,kind:'menu',selector:'role_menu',hasArea:null}],complete:false};
  assert.deepEqual(projectLinkedInRecoveryBlockers(snapshot({linkedinRecoveryBlockers:[value]})),{linkedinRecoveryBlockers:[value]});
});

test('native projection retains allowlisted menu evidence and tolerates an invalid optional extension',()=>{
  const menu={...sample,kind:'menu',selector:'role_menu'};
  const base={...diagnostic,menuCount:1,dialogCount:null,samples:[menu],complete:false};
  const menuEvidence={component:'popover',scope:'navigation',ariaOwnerRelation:'other',triggerBinding:'target_reference',triggerRelation:'other',
    triggerExpanded:false,popoverOpen:false,dropdownOpen:null,cssCheck:true,hitTest:{hit:0,covered:5,miss:0}};
  const value=snapshot({linkedinRecoveryBlockers:[{...base,samples:[{...menu,menuEvidence:{...menuEvidence,privateId:'Private ID'}}]}]});
  const expected=[{...base,samples:[{...menu,menuEvidence}]}];
  assert.deepEqual(projectLinkedInRecoveryBlockers(value).linkedinRecoveryBlockers,expected);
  assert.deepEqual(emptyCaptureDiagnostics([value]).samples[0].linkedinRecoveryBlockers,expected);
  const observed=toObservation({source:'linkedin',requestedUrl:'https://www.linkedin.com/feed/',snapshots:[value],capturedAt:'2026-11-17T00:00:00Z'});
  assert.deepEqual(observed.coverage.linkedinRecoveryBlockers,expected);
  assert.equal(observed.snapshots[0].blocks.length,0);

  value.linkedinRecoveryBlockers[0].samples[0].menuEvidence.hitTest.hit=1;
  assert.deepEqual(projectLinkedInRecoveryBlockers(value).linkedinRecoveryBlockers,[base]);
});

test('native coverage retains earlier blocker evidence when the last snapshot has none, capped at eight records',()=>{
  const convert=snapshots=>toObservation({source:'linkedin',requestedUrl:'https://www.linkedin.com/feed/',snapshots,capturedAt:'2026-11-17T00:00:00Z'});
  const value=snapshot(),empty={source:'linkedin',posts:[]};
  assert.deepEqual(convert([value,empty]).coverage.linkedinRecoveryBlockers,[diagnostic]);
  assert.equal(convert([...Array(10).fill(value),empty]).coverage.linkedinRecoveryBlockers.length,8);
  assert.deepEqual(convert([value,{...value},empty]).coverage.linkedinRecoveryBlockers.map(value=>value.snapshotIndex),[0,1]);
  assert.deepEqual(emptyCaptureDiagnostics([value,null,value,value]).samples.map(sample=>sample.linkedinRecoveryBlockers[0].snapshotIndex),[2,3]);
  const baseline=convert([empty]);
  assert.deepEqual(convert([snapshot({linkedinRecoveryBlockers:[{...diagnostic,menuCount:-1}]})]),baseline);
});

test('invalid, oversized and contradictory blocker telemetry is omitted independently of capture',()=>{
  const invalid=[{...diagnostic,menuCount:33},{...diagnostic,dialogCount:0},
    {...diagnostic,samples:Array(5).fill(sample)}, {...diagnostic,samples:[]},
    {...diagnostic,samples:[{...sample,selector:'Private label'}]},
    {...diagnostic,samples:[{...sample,ariaHidden:'private'}]},
    {...diagnostic,samples:[{...sample,hasArea:null}]},
    {...diagnostic,dialogCount:null},{...diagnostic,phase:'private'},
    {...diagnostic,candidateIndex:1_000_001},{...diagnostic,snapshotIndex:-1},
    {...diagnostic,samples:[{...sample,ancestors:{...sample.ancestors,inspected:17}}]},
    {...diagnostic,complete:false,samples:[{...sample,ancestors:{...sample.ancestors,complete:false}}]},
    {...diagnostic,samples:[{...sample,aria:{...sample.aria,controls:'current_control'}}]}];
  for(const value of invalid)assert.deepEqual(projectLinkedInRecoveryBlockers(snapshot({linkedinRecoveryBlockers:[value]})),{});
  assert.deepEqual(projectLinkedInRecoveryBlockers(snapshot({linkedinRecoveryBlockers:Array(9).fill(diagnostic)})),{});
  const getter={...diagnostic};Object.defineProperty(getter,'menuCount',{get(){throw Error('Private getter');}});
  assert.deepEqual(projectLinkedInRecoveryBlockers(snapshot({linkedinRecoveryBlockers:[getter]})),{});
});

test('ARIA conflicts, distinct semantic flags and unknown ancestor tails survive native projection without admission',()=>{
  const ancestors={inspected:16,complete:false,displayNone:null,visibilityHidden:null,opacityZero:true,hiddenAttribute:null,ariaHidden:true,inert:null};
  const value={...diagnostic,snapshotIndex:null,samples:[{...sample,
    aria:{labelledBy:'matches',controls:'different',owns:'unknown'},hiddenAttribute:true,ancestors}],complete:false};
  const result=projectLinkedInRecoveryBlockers(snapshot({linkedinRecoveryBlockers:[value]}),3);
  assert.deepEqual(result.linkedinRecoveryBlockers,[{...value,snapshotIndex:3}]);
  assert.equal(toObservation({source:'linkedin',requestedUrl:'https://www.linkedin.com/feed/',snapshots:[snapshot({linkedinRecoveryBlockers:[value]})],capturedAt:'2026-11-17T00:00:00Z'}).snapshots[0].blocks.length,0);
  const aria={...sample.aria};Object.defineProperty(aria,'owns',{get(){throw Error('Private getter');}});
  assert.deepEqual(projectLinkedInRecoveryBlockers(snapshot({linkedinRecoveryBlockers:[{...diagnostic,samples:[{...sample,aria}]}]})),{});
});

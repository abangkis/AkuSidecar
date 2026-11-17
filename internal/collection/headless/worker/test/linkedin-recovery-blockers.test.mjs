import test from 'node:test';
import assert from 'node:assert/strict';
import {projectLinkedInRecoveryBlockers, toObservation} from '../observation.mjs';
import {emptyCaptureDiagnostics} from '../capture.mjs';

const sample = {kind:'dialog',selector:'role_dialog',aria:{labelledBy:'absent',controls:'absent',owns:'absent'},guardVisible:true,
  hasArea:true,intersectsViewport:true,displayed:true,cssVisible:true,opacityPositive:true,hiddenAttribute:false,ariaHidden:false,inert:false,
  ancestors:{inspected:0,complete:true,displayNone:false,visibilityHidden:false,opacityZero:false,hiddenAttribute:false,ariaHidden:false,inert:false}};
const diagnostic = {code:'existing_surface',phase:'pre_recovery_guard',snapshotIndex:0,candidateIndex:0,menuCount:0,dialogCount:1,samples:[sample],complete:true};
const snapshot = extra => ({source:'linkedin',posts:[],linkedinRecoveryBlockers:[diagnostic],...extra});

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

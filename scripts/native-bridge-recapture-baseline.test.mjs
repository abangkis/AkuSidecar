import test from 'node:test';
import assert from 'node:assert/strict';
import {nativeBridgeRecaptureBaseline} from './native-bridge-recapture-baseline.mjs';
const url='https://www.facebook.com/reel/123/';
const row={platformId:'facebook:post:123',permalink:url,author:'Author',text:'Observed',media:[]};
const observation=blocks=>({source:'facebook',pageUrl:url,capturedAt:'2026-10-03T11:00:00Z',coverage:{browserAdapter:'aku-bridge'},snapshots:[{blocks}]});
test('retains observed rows and their acquisition time without inventing missing media',()=>{
 const value=nativeBridgeRecaptureBaseline(observation([row,{...row,media:undefined}]),url);
 assert.equal(value.targets.length,2);
 assert.equal(value.targets[0].observedAt,'2026-10-03T11:00:00Z');
 assert.deepEqual(value.targets[0].media,[]);
 assert.equal(value.targets[1].media,undefined);
 assert.equal(value.fullParityVerified,false);
});
test('does not manufacture a baseline row for empty, neighboring or inconsistent evidence',()=>{
 for(const blocks of [[],[{...row,platformId:'facebook:post:456'}],[{...row,permalink:'https://www.facebook.com/reel/456/'}],[{...row,author:undefined}]]){
  const value=nativeBridgeRecaptureBaseline(observation(blocks),url);
  assert.equal(value.captureStatus,'exact_target_not_observed');
  assert.deepEqual(value.targets,[]);
 }
});
test('rejects wrong transport, page and unobserved capture times',()=>{
 for(const delta of [{coverage:{browserAdapter:'aku-headless-worker'}},{pageUrl:'https://www.facebook.com/reel/456/'},{capturedAt:undefined},{source:'x'}]){
  assert.throws(()=>nativeBridgeRecaptureBaseline({...observation([row]),...delta},url),/Unverified native Bridge observation/);
 }
});

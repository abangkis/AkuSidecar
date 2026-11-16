import test from 'node:test';
import assert from 'node:assert/strict';
import {runInstagramShadowParity} from '../../../../../scripts/shared-dom-instagram-shadow.mjs';
import {projectDOMShadow,toObservation} from '../observation.mjs';

test('Instagram shadow agrees across actual offline Windowed/Headless paths and changes no capture decision',
  {skip:!process.env.AKU_OFFLINE_CHROME&&'Set AKU_OFFLINE_CHROME for isolated real-DOM shadow parity.'},async()=>{
    const result=await runInstagramShadowParity(process.env.AKU_OFFLINE_CHROME);
    assert.equal(result.fixtures,23);
  });

test('Instagram projection adds only allowlisted diagnostic counts and cannot grant admission',()=>{
  const diagnostic={code:'ready',counts:{candidates:1,visibleStructural:1,accepted:1,rejected:0,offscreen:0},rejections:{},privateText:'must not escape'};
  const snapshot={source:'instagram',sharedDOMShadow:diagnostic,posts:[{id:'instagram:p:Fixture123',permalink:'https://www.instagram.com/p/Fixture123/',author:'Fixture',text:'Body',media:[]}]};
  const convert=value=>toObservation({source:'instagram',requestedUrl:'https://www.instagram.com/',snapshots:[value],capturedAt:'2026-11-16T00:00:00Z'});
  const observed=convert(snapshot),baseline=convert({...snapshot,sharedDOMShadow:undefined});
  assert.deepEqual(observed.coverage.sharedDOMShadow,projectDOMShadow(snapshot).sharedDOMShadow);
  const {sharedDOMShadow,...coverage}=observed.coverage;
  assert.deepEqual({...observed,coverage},baseline);
  assert.equal(observed.snapshots[0].blocks[0].captureQuality.status,'unverified');
  assert.equal(JSON.stringify(sharedDOMShadow).includes('privateText'),false);
});

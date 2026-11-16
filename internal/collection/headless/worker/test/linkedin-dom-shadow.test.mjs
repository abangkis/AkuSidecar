import test from 'node:test';
import assert from 'node:assert/strict';
import {runLinkedInShadowParity} from '../../../../../scripts/shared-dom-linkedin-shadow.mjs';
import {projectDOMShadow,toObservation} from '../observation.mjs';

test('LinkedIn shadow agrees on actual offline Windowed/Headless DOM without capture changes',
  {skip:!process.env.AKU_OFFLINE_CHROME&&'Set AKU_OFFLINE_CHROME for isolated real-DOM parity.'},async()=>{
    const report=await runLinkedInShadowParity(process.env.AKU_OFFLINE_CHROME);assert.equal(report.fixtures,38);
  });
test('LinkedIn final projection adds only allowlisted shadow diagnostics',()=>{
  const snapshot={source:'linkedin',sharedDOMShadow:{code:'ready',counts:{candidates:1,visibleStructural:1,accepted:1,rejected:0,offscreen:0},rejections:{},privateText:'Private source'},posts:[{id:'linkedin:activity:1234567890123456789',permalink:'https://www.linkedin.com/feed/update/urn:li:activity:1234567890123456789/',author:'Fixture',text:'Body',media:[]}]};
  const convert=s=>toObservation({source:'linkedin',requestedUrl:'https://www.linkedin.com/feed/',snapshots:[s],capturedAt:'2026-11-16T00:00:00Z'});
  const observed=convert(snapshot),baseline=convert({...snapshot,sharedDOMShadow:undefined});
  const {sharedDOMShadow,...coverage}=observed.coverage;
  assert.deepEqual(sharedDOMShadow,projectDOMShadow(snapshot).sharedDOMShadow);
  assert.deepEqual({...observed,coverage},baseline);
  assert.equal(observed.snapshots[0].blocks[0].captureQuality.status,'unverified');
  assert.equal(JSON.stringify(sharedDOMShadow).includes('privateText'),false);
});

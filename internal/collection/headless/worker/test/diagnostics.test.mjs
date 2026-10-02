import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import { emptyCaptureDiagnostics, facebookTargetUnavailable } from '../capture.mjs';

test('native Facebook unavailable notice excludes live posts, loading shells and quoted prose',()=>{
  const probe=(text,{post=false,ready='complete',main=true}={})=>vm.runInNewContext(`(${facebookTargetUnavailable.toString()})()`,{
    document:{readyState:ready,querySelector:selector=>selector==='[role="main"], main'?(main?{innerText:text}:null):(post?{}:null)},
  });
  assert.equal(probe("This content isn't available\nGo back"),true);
  assert.equal(probe('This content isn’t available.'),true);
  assert.equal(probe('Konten ini tidak tersedia'),true);
  assert.equal(probe("This content isn't available",{post:true}),false);
  assert.equal(probe("This content isn't available",{ready:'loading'}),false);
  assert.equal(probe("This content isn't available",{main:false}),false);
  assert.equal(probe("A post about why this content isn't available"),false);
  assert.equal(probe(''),false);
  assert.equal(probe('x'.repeat(8001)+"\nThis content isn't available"),false);
});

test('empty capture diagnostics retain unknown states and exclude private DOM evidence',()=>{
  const value = emptyCaptureDiagnostics([{candidateCount:12,rejected:3,rejectionReasons:{missing_identity:3},
    text:'private body',url:'https://private.invalid/secret',posts:[{id:'private-id'}],
    authenticatedUiObserved:true,documentReady:true,
    boundaryDiagnostics:[{ownBodies:2,ownAnchors:0,author:'private author'}],
    identityDiagnostics:[{eligibleHoverAnchors:1,hovered:{attempted:true},anchors:[{url:'private'}]}]}]);
  assert.equal(value.samples[0].candidateCount,12);
  assert.equal(value.samples[0].loginRequired,null);
  assert.deepEqual(value.samples[0].rejectionReasons,{missing_identity:3});
  assert.equal(value.samples[0].boundaries[0].ownAnchors,0);
  assert.equal(value.samples[0].hover[0].attempted,true);
  assert.equal(JSON.stringify(value).includes('private'),false);
  assert.deepEqual(emptyCaptureDiagnostics([{boundaryDiagnostics:'untrusted',identityDiagnostics:[null]}]).samples[0].boundaries,[]);
});

test('distinguishes no discovered post from rejected candidates without copying page labels',()=>{
  const value=emptyCaptureDiagnostics([
    {candidateCount:0,candidateDiagnostics:{structuralCandidates:0,eligibleCandidates:0,actionAnchoredCandidates:0}},
    {candidateCount:0,candidateDiagnostics:{structuralCandidates:2,eligibleCandidates:0,actionAnchoredCandidates:-1,
      admittedReasons:{private_author:1},rejectedReasons:{private_text:2}}},
  ]);
  assert.equal(value.samples[0].discovery.structuralCandidates,0);
  assert.equal(value.samples[1].discovery.structuralCandidates,2);
  assert.equal(value.samples[1].discovery.eligibleCandidates,0);
  assert.equal(value.samples[1].discovery.actionAnchoredCandidates,null);
  assert.equal(JSON.stringify(value).includes('private'),false);
  assert.equal(emptyCaptureDiagnostics([{}]).samples[0].discovery.structuralCandidates,null);
});

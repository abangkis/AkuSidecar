import test from 'node:test';
import assert from 'node:assert/strict';
import { emptyCaptureDiagnostics } from '../capture.mjs';

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

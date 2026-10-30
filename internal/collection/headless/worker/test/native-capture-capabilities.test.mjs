import test from 'node:test';
import assert from 'node:assert/strict';
import {captureIsolated,captureCapabilities,NATIVE_CAPTURE_CAPABILITIES,emptyCaptureDiagnostics} from '../capture.mjs';
import {toObservation} from '../observation.mjs';
import {projectCaptureWorkerDiagnostic} from '../../../../../scripts/auth-session-fixture/x-capture-worker-diagnostic.mjs';
import {validateManualCaptureRequest} from '../../../../../scripts/auth-session-fixture/x-native-manual-capture.mjs';

test('native capability declarations require both owner support and an actual temporary page primitive',()=>{
  const createTemporaryPage=()=>{};
  assert.deepEqual(captureCapabilities({captureCapabilities:NATIVE_CAPTURE_CAPABILITIES.x,createTemporaryPage}),
    {contentExpansion:false,freshnessMutation:false,quoteNavigation:false,temporaryDetailPage:false});
  assert.equal(captureCapabilities({captureCapabilities:NATIVE_CAPTURE_CAPABILITIES.instagram}).temporaryDetailPage,false);
  assert.equal(captureCapabilities({captureCapabilities:NATIVE_CAPTURE_CAPABILITIES.instagram,createTemporaryPage}).temporaryDetailPage,true);
});

test('guarded X capture never clicks an unresolved quote or enables mutation despite permissive payload and transport',async()=>{
  const expressions=[];
  const url='https://x.com/fixture/status/123';
  const page={async navigate(){return {};},async send(){return {};},async evaluate(expression){
    expressions.push(expression);
    if(expression.includes('clickQuote'))throw Error('guarded quote navigation attempted');
    if(expression.includes('prepareEvidenceTargets'))return [];
    if(expression.includes('XHeadlessPoC.collect()'))return JSON.stringify({documentReady:true,posts:[{
      id:'123',permalink:url,author:'Fixture',text:'Primary text',media:[],
      quotedPost:{permalink:null,text:'Unresolved quoted text',identityStatus:'unknown',media:[]},
    }]});
    return undefined;
  }};
  const assets={x:['runtime','adapter','extractor'].map((relative,index)=>({relative,execute:false,sha256:String(index).repeat(64)}))};
  const result=await captureIsolated({backend:'headless_worker',captureCapabilities:NATIVE_CAPTURE_CAPABILITIES.x,
    forSource:async()=>page,createTemporaryPage:async()=>{throw Error('unowned target attempted');}},assets,'x',{
    pageUrl:url,scrolls:0,sourceHydrationTimeoutMs:1000,captureTimeoutMs:3000,
    pendingContentPolicy:'reveal_if_present',sourceFreshnessPolicy:'wake_and_reveal',sameTabMutationAllowed:true,
  });
  assert.equal(result.snapshots[0].blocks[0].quotedPost.permalink,null);
  assert.equal(result.coverage.quoteIdentityProbe.reason,'owner_quote_navigation_unsupported');
  assert.ok(expressions.some(value=>value.includes('allowContentExpansion:false')));
  assert.ok(expressions.every(value=>!value.includes('clickQuote')));
});

test('native X request still rejects caller capability and mutation overrides',()=>{
  const pins=Object.fromEntries(['ownerId','profileId','contextId','targetId','frameId','documentId'].map(key=>[key,key]));
  for(const payload of [{sameTabMutationAllowed:true},{captureCapabilities:NATIVE_CAPTURE_CAPABILITIES.instagram}]) {
    assert.throws(()=>validateManualCaptureRequest({correlation:'test',pins,deadlineMS:Date.now()+5000,payload}));
  }
});


test('readiness projection is closed, observational and absent-compatible on success and empty diagnostics',()=>{
  const snapshot={posts:[{id:'123',permalink:'https://x.com/fixture/status/123',author:'Fixture',text:'Observed post',media:[]}],documentReady:true};
  const observe=value=>toObservation({source:'x',requestedUrl:'https://x.com/home',snapshots:[value],capturedAt:'2026-10-30T00:00:00.000Z'});
  const baseline=JSON.stringify(observe(snapshot));
  const diagnosticBaseline=JSON.stringify(emptyCaptureDiagnostics([snapshot]));
  for(const state of ['source_unavailable','login_required','feed_ready','feed_empty','loading','feed_not_visible','selector_mismatch','page_shell']) {
    const sample={...snapshot,sourceReadiness:state};
    const result=observe(sample);
    assert.equal(result.coverage.sourceReadinessState,state);
    assert.equal(result.coverage.authenticatedUiObserved,false);
    assert.equal(result.coverage.status,'partial');
    assert.equal(Object.hasOwn(result.coverage,'sourceReadiness'),false);
    const diagnostics=emptyCaptureDiagnostics([sample]);
    assert.equal(diagnostics.samples[0].sourceReadinessState,state);
    // Native X's private error wire has a separate closed Go schema. Do not
    // smuggle new fields into it or let telemetry invalidate its whole record.
    const wire=projectCaptureWorkerDiagnostic({code:'empty_unverified',diagnostics},{hydrationMS:1000,elapsedMS:1000});
    assert.equal(Object.hasOwn(wire.samples[0],'sourceReadinessState'),false);
  }
  for(const state of [undefined,null,'PRIVATE page text',{state:'feed_ready'},true]) {
    const sample={...snapshot,sourceReadiness:state};
    assert.equal(JSON.stringify(observe(sample)),baseline);
    assert.equal(JSON.stringify(emptyCaptureDiagnostics([sample])),diagnosticBaseline);
  }
  const getter={...snapshot};
  Object.defineProperty(getter,'sourceReadiness',{get(){throw Error('private getter invoked');}});
  assert.equal(JSON.stringify(observe(getter)),baseline);
  assert.equal(JSON.stringify(emptyCaptureDiagnostics([getter])),diagnosticBaseline);
});

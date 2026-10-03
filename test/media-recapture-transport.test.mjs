import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFile} from 'node:fs/promises';
import {collectionModeState} from '../internal/httpapi/web/collection-mode.js';
import {mediaRecaptureTransport,waitForMediaRecapture} from '../internal/httpapi/web/media-recapture-transport.js';
test('durable collector decides transport, not a later UI mode change',()=>{
 for(const backend of ['headless','browser_quiet_hidden','bridge'])assert.equal(mediaRecaptureTransport({payload:{captureCollector:{version:1,backend}}}),backend==='bridge'?'bridge':'sidecar');
 assert.equal(mediaRecaptureTransport({payload:{captureRuntime:{driver:'headless'}}}),'sidecar');
 assert.equal(mediaRecaptureTransport({}),'bridge');
 assert.throws(()=>mediaRecaptureTransport({payload:{captureCollector:{version:2,backend:'headless'}}}));
});
test('actual UI recapture function completes headless jobs without Bridge and refreshes timeline',async()=>{
 const app=await readFile(new URL('../internal/httpapi/web/app.js',import.meta.url),'utf8');
 const fn=app.slice(app.indexOf('async function recaptureMedia('),app.indexOf('\nfunction dispatchMediaRecapture('));
 for(const outcome of ['recovered','unavailable']){
  const calls=[],notices=[],errors=[];let refreshed=0;
  const state={session:null,mediaRecaptureActive:false,bootstrap:{bridge:{compatible:false},collectionRuntime:{available:true,state:'ready',effective:'headless'}},foregroundRecaptureOffers:new Map()};
  const context=vm.createContext({state,collectionModeState,mediaRecaptureTransport,waitForMediaRecapture,
   api:async(path,opts)=>{calls.push({path,opts});return {recapture:opts?.method==='POST'?{id:'job',payload:{captureCollector:{version:1,backend:'headless'}}}:{id:'job',status:'completed',outcome}};},
   dispatchMediaRecapture:()=>{throw Error('Bridge dispatch must not run');},syncRunButtons(){},clearNotice(){},
   refreshTimeline:async()=>{refreshed++;},$:()=>({setAttribute(){}}),setNoticeText:(_,text)=>notices.push(text),showError:e=>errors.push(e.message)});
  vm.runInContext(fn,context);await context.recaptureMedia({id:'item'},null,'background');
  assert.equal(calls[0].path,'/api/timeline/item/recapture');assert.equal(calls[1].path,'/api/media-recaptures/job');
  assert.equal(refreshed,1);assert.deepEqual(errors,[]);assert.equal(state.mediaRecaptureActive,false);
  assert.equal(state.foregroundRecaptureOffers.size,0);assert.equal(notices.length,1);
 }
});
test('polls queued and claimed until completion without extension messages',async()=>{
 let clock=0,index=0;const paths=[];
 const result=await waitForMediaRecapture('job/a',async path=>{paths.push(path);return {recapture:{id:'job/a',status:['queued','claimed','completed'][index++],outcome:'recovered'}};},{now:()=>clock,sleep:async ms=>{clock+=ms;}});
 assert.equal(result.outcome,'recovered');assert.equal(index,3);assert.equal(paths[0],'/api/media-recaptures/job%2Fa');
});
test('reports failure, mismatched jobs, unknown states and bounded timeout',async()=>{
 for(const recapture of [{id:'id',status:'failed',error:{message:'Capture unavailable'}},{id:'other',status:'completed'},{id:'id',status:'unknown'}])await assert.rejects(waitForMediaRecapture('id',async()=>({recapture})));
 let clock=0;await assert.rejects(waitForMediaRecapture('id',async()=>({recapture:{id:'id',status:'claimed'}}),{now:()=>clock,sleep:async ms=>{clock+=ms;},timeoutMs:1000}),/still running/);
});

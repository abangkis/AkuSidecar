import test from 'node:test';
import assert from 'node:assert/strict';
import {createManualV2BoundObservation} from './x-qa-bound-observation.mjs';
import {inspectBootstrapCurrentSelf} from './x-bootstrap-current-self.mjs';
import {QA_AB_URL} from './x-qa-direct-ab.mjs';
import {SETTINGS_WAIT_STATES} from './x-settings-wait-state.mjs';
import {createStartupWaitBudget} from './x-manual-startup-wait-budget.mjs';
import {startupDiagnostic} from './x-manual-startup-diagnostic.mjs';
import {startupFailure} from './x-manual-startup-diagnostic.mjs';
import {createStartupTiming} from './x-manual-startup-timing.mjs';

function fixture({bodyGate, bodyFailure=false, onFailure}={}) {
  const scope={owner:1,profileId:'synthetic-profile',sessionId:'synthetic-session',targetId:'synthetic-target'};
  let receipts=0,reads=0,first;
  const b=createManualV2BoundObservation(scope,'frame',async id=>{
    reads++;assert.equal(b.capture.consumeRead(id),true);await bodyGate;
    if(bodyFailure)throw Error('PRIVATE_BODY_TOKEN');
    return{schema:'aku.x.qa-direct-body.v1',category:'settings_match',settingsMatch:true};
  },()=>100,undefined,{deferCookieMutation:false,onPrivateResult:()=>{},onPrivateStartup:()=>{receipts++;}},
  (reason,trace,state)=>{first??={reason,trace};onFailure?.(reason,trace,state);});
  const emit=(method,params)=>b.observe({method,params,sessionId:scope.sessionId},scope);
  emit('Page.frameNavigated',{frame:{id:'frame',loaderId:'document',url:'https://x.com/home'}});
  emit('Runtime.executionContextCreated',{context:{id:7,origin:'https://x.com',auxData:{isDefault:true,frameId:'frame'}}});
  const identity=()=>b.acceptIdentity('12345',inspectBootstrapCurrentSelf({location:{origin:'https://x.com',pathname:'/home'},__INITIAL_STATE__:{session:{user_id:'12345'},entities:{users:{entities:{'12345':{id_str:'12345'}}}}}}));
  const request=(patch={},requestId='settings')=>emit('Network.requestWillBeSent',{requestId,frameId:'frame',loaderId:'document',request:{method:'GET',url:QA_AB_URL},...patch});
  const response=(extra=true,requestId='settings')=>emit('Network.responseReceived',{requestId,frameId:'frame',loaderId:'document',hasExtraInfo:extra,response:{url:QA_AB_URL,status:200,mimeType:'application/json',fromDiskCache:false,fromServiceWorker:false,fromPrefetchCache:false}});
  const extra=(statusCode=200,requestId='settings')=>emit('Network.responseReceivedExtraInfo',{requestId,statusCode,headers:{}});
  const finish=(requestId='settings')=>emit('Network.loadingFinished',{requestId,encodedDataLength:512});
  return{b,emit,identity,request,response,extra,finish,receipts:()=>receipts,reads:()=>reads,first:()=>first};
}

test('both completion layers report selected loadingFailed immediately without issuing a receipt',async()=>{
  const f=fixture();f.identity();f.request();
  f.emit('Network.loadingFailed',{requestId:'settings',errorText:'PRIVATE_NETWORK_URL',canceled:true});
  for(const result of await Promise.all([f.b.capture.done,f.b.done]))assert.deepEqual(result,{status:'failed',reason:'network_failed'});
  assert.equal(f.b.settingsWaitState(),'network_failed');assert.equal(f.first().reason,'source_unavailable');
  assert.equal(f.first().trace.firstCause,'settings_capture');
  f.b.invalidate('scope_lost','context_lost');
  assert.equal(f.first().trace.firstCause,'settings_capture');assert.equal(f.b.settingsWaitState(),'network_failed');
  await f.b.finish(false);assert.equal(f.receipts(),0);assert.equal(f.reads(),0);
  assert.equal(JSON.stringify(f.first()).includes('PRIVATE'),false);
});

test('diagnostics distinguish selection, response, completion, extra and body prerequisites',async()=>{
  const f=fixture(),states=[];const stop=f.b.onSettingsWait(state=>states.push(state));
  assert.equal(states.at(-1),'request_pending');
  f.request();assert.equal(states.at(-1),'request_before_identity');
  f.identity();f.request({frameId:'child'},'child');assert.equal(states.at(-1),'request_frame');
  f.request({loaderId:'old'},'old');assert.equal(states.at(-1),'request_document');
  f.request({},'valid');assert.equal(states.at(-1),'response_pending');
  f.response(true,'valid');assert.equal(states.at(-1),'completion_pending');
  f.finish('valid');assert.equal(states.at(-1),'extra_pending');
  let settled=false;f.b.done.then(()=>{settled=true;});await Promise.resolve();assert.equal(settled,false);
  f.extra(200,'valid');assert.equal((await f.b.done).status,'complete');
  await f.b.finish(true);assert.equal(f.receipts(),1);stop();
  assert.ok(states.every(state=>SETTINGS_WAIT_STATES.includes(state)));
  assert.equal(JSON.stringify(states).includes('valid'),false);
});

test('a body still in flight cannot delay metadata completion or become premature account authority',async()=>{
  let release;const bodyGate=new Promise(resolve=>{release=resolve;}),f=fixture({bodyGate});
  f.identity();f.request();f.response(false);f.finish();
  assert.deepEqual(await f.b.done,{status:'complete',reason:'metadata_complete'});
  assert.equal(f.b.settingsWaitState(),'body_pending');assert.equal(f.receipts(),0);
  let finished=false;const pending=f.b.finish(true).then(report=>{finished=true;return report;});
  await Promise.resolve();assert.equal(finished,false);assert.equal(f.receipts(),0);
  release();assert.equal((await pending).diagnostic,'bound');assert.equal(f.receipts(),1);
});

test('candidate contradictions, scope loss and body failure remain rejected',async()=>{
  for(const fault of ['duplicate','extra','scope','body']){
    const f=fixture({bodyFailure:fault==='body'});f.identity();f.request();
    if(fault==='duplicate')f.request();
    if(fault==='scope')f.emit('Runtime.executionContextsCleared',{});
    f.response();f.extra(fault==='extra'?401:200);f.finish();
    if(fault!=='body')assert.equal((await f.b.done).status,'failed');
    const report=await f.b.finish(true);assert.notEqual(report.diagnostic,'bound');assert.equal(f.receipts(),0);
  }
});

test('an ineligible or missing request stays pending until the existing wait budget expires',async()=>{
  for(const mode of ['missing','before_identity','response','completion','extra']){
    const f=fixture();
    if(mode==='before_identity')f.request();f.identity();
    if(['response','completion','extra'].includes(mode))f.request();
    if(['completion','extra'].includes(mode))f.response();
    if(mode==='extra')f.finish();
    const budget=createStartupWaitBudget(10,{role:'collector'});
    await assert.rejects(budget.wait(f.b.done),error=>startupDiagnostic(error).reason==='deadline');
    await f.b.finish(false);assert.equal(f.receipts(),0);
  }
});

test('diagnostic subscribers cannot affect evidence or leak native errors',async()=>{
  const f=fixture();f.b.onSettingsWait(()=>{throw Error('PRIVATE_LISTENER');});
  f.identity();f.request();f.response(false);f.finish();
  assert.equal((await f.b.done).status,'complete');assert.equal((await f.b.finish(true)).diagnostic,'bound');
  assert.equal(f.receipts(),1);
});

test('explicit finish also retires incomplete waiters without promoting incomplete metadata',async()=>{
  for(const selected of [false,true]){
    const f=fixture();f.identity();if(selected){f.request();f.response(false);}
    const report=await f.b.finish(false);
    for(const result of await Promise.all([f.b.done,f.b.capture.done]))assert.equal(result.status,'failed');
    assert.equal(report.source.diagnostic,selected?'incomplete':'not_observed');
    assert.notEqual(report.diagnostic,'bound');assert.equal(f.receipts(),0);
  }
});

test('the first failure carries its terminal prerequisite in the same frozen progress record',async()=>{
  const output=[],timing=createStartupTiming(row=>output.push(row),()=>100);
  const f=fixture({onFailure:(reason,trace,state)=>timing.failure(startupFailure('collector','settings',reason,false,undefined,trace),state)});
  f.b.onSettingsWait(state=>timing.settings(state));timing.step('settings_wait');
  f.identity();f.request();f.emit('Network.loadingFailed',{requestId:'settings'});
  assert.equal(output.at(-1).settingsWait,'network_failed');assert.match(output.at(-1).cause,/^manual_initial_v9_/);
  const first=JSON.stringify(output.at(-1));f.b.invalidate('scope_lost','context_lost');
  assert.equal(JSON.stringify(output.at(-1)),first);
  assert.ok(output.every(row=>Buffer.byteLength(JSON.stringify(row))<=512));
  await f.b.finish(false);
});

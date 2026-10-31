import test from 'node:test';
import assert from 'node:assert/strict';
import {PassThrough} from 'node:stream';
import {performance} from 'node:perf_hooks';
import {fixture} from './instagram-bootstrap-fixture.mjs';
import {createDiagnosticTrace} from './diagnostic-trace.mjs';
import {createIGTrace,validateIGTrace,attachIGTrace,igErrorTrace} from './instagram-diagnostic-trace.mjs';
import {createInstagramDocumentWatch} from './instagram-document-watch.mjs';
import {acquireOwnedInstagramIdentity} from './instagram-bootstrap-corroboration.mjs';
import {readInstagramEvidence} from './instagram-native-session.mjs';
import {createNativeOwnedPage,nativeSourceDiagnostic} from './native-source-kernel.mjs';
import {createMainCDP,serveMainBrowserTransport,MAIN_TRANSPORT_SCHEMA} from './main-browser-transport-protocol.mjs';

const nonce='a'.repeat(48),binding={nonce,generation:1,profileScope:'scope'};
const attempt={binding,run:async(_stage,fn)=>fn(60000)};
const duplicate=JSON.stringify({sessionId:'owned-session',method:'Network.requestWillBeSent',params:{frameId:'root-frame',type:'Document',requestId:'repeat',loaderId:'repeat',request:{url:'https://www.instagram.com/',method:'GET'}}})+'\0';
const invalidation=JSON.stringify({sessionId:'owned-session',method:'Page.navigatedWithinDocument',params:{frameId:'root-frame'}})+'\0';

function wireFixture(f,input,output,{storm=false,disableFails=false,onDisable=()=>{}}={}){
 const methods=[];
 const off=f.raw.subscribe(event=>{if(!input.destroyed)input.write(JSON.stringify(event)+'\0');});
 output.on('data',wire=>{
  const q=JSON.parse(wire.toString().slice(0,-1));methods.push(q.method);
  const respond=result=>{if(!input.destroyed)input.write(JSON.stringify({id:q.id,...(q.sessionId?{sessionId:q.sessionId}:{}),result})+'\0');};
  if(q.method==='Browser.getVersion'){respond({product:'synthetic'});return;}
  f.raw.call(q.method,q.params,q.sessionId,1000).then(result=>{
   if(q.method==='Fetch.disable'){
    onDisable();
    input.write((storm?duplicate.repeat(10020):'')+invalidation);
    if(disableFails){if(!input.destroyed)input.write(JSON.stringify({id:q.id,sessionId:q.sessionId,error:{message:'PRIVATE_FAILURE'}})+'\0');return;}
   }
   respond(result);
  }).catch(()=>{});
 });
 return {off,methods};
}
function framedFixture(t,options={}){
 const f=fixture(),input=new PassThrough(),output=new PassThrough(),failures=[];
 const transport=createMainCDP({input,output,onFailure:failure=>failures.push(failure)});
 const {off,methods}=wireFixture(f,input,output,options);
 t.after(()=>{off();transport.close();});
 return {transport,failures,methods};
}

test('saturated trace remains bounded and preserves the final first cause and cleanup',()=>{
 let lastProgress=0,count=0,validProgress=true,observedIncomplete=false;
 const trace=createIGTrace(value=>{if(!validateIGTrace(value)||value.sequence<=lastProgress)validProgress=false;if(!value.complete)observedIncomplete=true;lastProgress=value.sequence;count++;});
 trace.note('cookie');trace.fail('cookie','document_within_navigation');
 for(let i=0;i<10020;i++)trace.note('cleanup');
 trace.cleanup('verified');const final=trace.snapshot();
 assert.ok(validateIGTrace(final));assert.equal(final.complete,false);
 assert.equal(validProgress,true);assert.equal(observedIncomplete,true);assert.ok(final.sequence>lastProgress);assert.ok(final.sequence<=10000);assert.ok(count<10020);
 assert.deepEqual(final.cause,{stage:'cookie',condition:'document_within_navigation'});
 assert.deepEqual(final.cleanup,{state:'verified',condition:'none'});
 for(let i=0;i<5;i++)assert.ok(validateIGTrace(trace.snapshot()));
});

test('saturated public trace flushes final cleanup once with a fresh sequence',()=>{
 let latest,count=0;
 const trace=createIGTrace(value=>{latest=value;count++;});
 trace.note('root_delivery');trace.fail('root_delivery','root_request_duplicate');
 for(let i=0;i<10020;i++)trace.note('cleanup');
 assert.equal(latest.sequence,9999);assert.equal(latest.complete,false);
 trace.cleanup('verified');trace.disposition('completed');trace.flush();
 assert.ok(validateIGTrace(latest));assert.equal(latest.sequence,10000);assert.equal(latest.complete,false);
 assert.equal(latest.cause.condition,'root_request_duplicate');assert.equal(latest.cleanup.state,'verified');
 assert.equal(latest.disposition,'completed');const flushed=count;trace.flush();assert.equal(count,flushed);
});

test('out-of-range elapsed time is incomplete and unknown without inventing a budget',()=>{
 let now=0;
 const trace=createDiagnosticTrace({schema:'aku.ig-trace.v1',now:()=>now,validate:validateIGTrace,sequenceLimit:10000,elapsedLimitMS:60000});
 trace.budget(30000);trace.note('cookie');now=60001;
 const value=trace.snapshot();assert.ok(validateIGTrace(value));assert.equal(value.complete,false);
 assert.equal(value.elapsedMS,null);assert.equal(value.budgetMS,null);assert.equal(value.remainingMS,null);
});

test('failing progress delivery keeps a bounded incomplete trace without private errors',()=>{
 const trace=createIGTrace(()=>{throw Error('PRIVATE_PROGRESS_FAILURE');});
 trace.note('cookie');trace.fail('cookie','document_within_navigation');
 for(let i=0;i<10020;i++)trace.note('cleanup');
 trace.cleanup('uncertain','cleanup_failed');assert.doesNotThrow(()=>trace.flush());
 const value=trace.snapshot();assert.ok(validateIGTrace(value));assert.equal(value.complete,false);
 assert.equal(value.cause.condition,'document_within_navigation');assert.equal(value.cleanup.state,'uncertain');
 assert.ok(!JSON.stringify(value).includes('PRIVATE'));
});

test('invalid trace attachment never replaces the original failure',()=>{
 const error=new Error('synthetic');
 assert.equal(attachIGTrace(error,{snapshot(){throw Error('PRIVATE_FAILURE');}}),error);
 assert.equal(igErrorTrace(error),null);
});

test('document watcher contains callback failures and fails closed on malformed events',()=>{
 let receive;const page={on(listener){receive=listener;return()=>{};}};
 const binding={id:'root-frame',loaderId:'root-loader',url:'https://www.instagram.com/',uniqueContextId:'root-context',contextId:7};
 const watch=createInstagramDocumentWatch(page,()=>{throw Error('PRIVATE_FAILURE');});watch.bind(binding);
 assert.doesNotThrow(()=>receive({method:'Page.navigatedWithinDocument',params:{frameId:'root-frame'}}));
 assert.equal(watch.condition(),'document_within_navigation');watch.dispose();
 const uncertain=createInstagramDocumentWatch(page);uncertain.bind(binding);
 assert.doesNotThrow(()=>receive({method:'Page.frameDetached',get params(){throw Error('PRIVATE_EVENT');}}));
 assert.equal(uncertain.condition(),'unavailable');uncertain.dispose();
});

for(const disableFails of [false,true])test(`saturated IG event storm preserves shared CDP and cleanup ${disableFails?'uncertain':'verified'}`,async t=>{
 const f=framedFixture(t,{storm:true,disableFails}),page=await createNativeOwnedPage(f.transport,1000);
 await assert.rejects(readInstagramEvidence(page,attempt,'reader','PRIVATE_OWNER','PRIVATE_PROFILE','PRIVATE_SECRET'),error=>{
  assert.equal(nativeSourceDiagnostic(error)?.reason,'reader_bootstrap_wire_unmatched');
  const trace=igErrorTrace(error);assert.ok(validateIGTrace(trace));assert.equal(trace.complete,false);
  assert.deepEqual(trace.cause,{stage:'root_delivery',condition:'root_request_duplicate'});
  assert.equal(trace.cleanup.state,disableFails?'uncertain':'verified');
  assert.ok(!JSON.stringify(trace).includes('PRIVATE'));return true;
 });
 assert.equal(f.transport.alive(),true);assert.equal(f.failures.length,0);
 assert.equal((await f.transport.call('Browser.getVersion',{},'',1000)).product,'synthetic');
 await page.close();assert.ok(f.methods.includes('Target.closeTarget'));
});

test('already saturated trace rejects invalidated identity without closing shared CDP',async t=>{
 const f=framedFixture(t),page=await createNativeOwnedPage(f.transport,1000),trace=createIGTrace();
 for(let i=0;i<10020;i++)trace.note('document');
 const result=await acquireOwnedInstagramIdentity(page,nonce,{trace});
 assert.equal(result.diagnostic.reason,'document_changed');assert.equal(result.diagnostic.cleanup,'verified');
 assert.equal(result.accountId,'');assert.equal(result.observedAtMS,undefined);
 const observed=trace.snapshot();assert.ok(validateIGTrace(observed));assert.equal(observed.complete,false);
 assert.equal(observed.cause.condition,'document_within_navigation');
 assert.equal(f.transport.alive(),true);assert.equal(f.failures.length,0);
 assert.equal((await f.transport.call('Browser.getVersion',{},'',1000)).product,'synthetic');
 await page.close();
});

test('out-of-range trace clock leaves shared CDP alive and rejects invalidated evidence',async t=>{
 const descriptor=Object.getOwnPropertyDescriptor(performance,'now');let now=0;
 Object.defineProperty(performance,'now',{configurable:true,value:()=>now});
 t.after(()=>{if(descriptor)Object.defineProperty(performance,'now',descriptor);else delete performance.now;});
 const f=framedFixture(t,{onDisable:()=>{now=60001;}}),page=await createNativeOwnedPage(f.transport,1000);
 await assert.rejects(readInstagramEvidence(page,attempt,'reader','1','p','s'),error=>{
  assert.equal(nativeSourceDiagnostic(error)?.reason,'reader_bootstrap_document_changed');
  const trace=igErrorTrace(error);assert.ok(validateIGTrace(trace));assert.equal(trace.complete,false);
  assert.equal(trace.elapsedMS,null);assert.equal(trace.cause.condition,'document_within_navigation');return true;
 });
 assert.equal(f.transport.alive(),true);assert.equal(f.failures.length,0);
 assert.equal((await f.transport.call('Browser.getVersion',{},'',1000)).product,'synthetic');
 await page.close();
});

test('trace write or stage failure cannot hide invalidation or escape into shared CDP',async t=>{
 for(const mode of ['write','stage'])await t.test(mode,async()=>{
  const f=framedFixture(t),page=await createNativeOwnedPage(f.transport,1000),base=createIGTrace();
  const fail=()=>{throw Error('PRIVATE_TRACE_FAILURE');};
  const trace=mode==='write'?{...base,fail}:{...base,currentStage:fail};
  const result=await acquireOwnedInstagramIdentity(page,nonce,{trace});
  assert.equal(result.diagnostic.reason,'document_changed');assert.equal(result.diagnostic.cleanup,'verified');
  assert.equal(result.accountId,'');assert.equal(result.observedAtMS,undefined);
  const observed=base.snapshot();assert.ok(validateIGTrace(observed));assert.equal(observed.complete,false);
  if(mode==='write')assert.equal(observed.cause,null);
  else assert.deepEqual(observed.cause,{stage:'none',condition:'document_within_navigation'});
  assert.ok(!JSON.stringify(observed).includes('PRIVATE'));
  assert.equal(f.transport.alive(),true);assert.equal(f.failures.length,0);
  assert.equal((await f.transport.call('Browser.getVersion',{},'',1000)).product,'synthetic');
  await page.close();
 });
});

test('the existing X control lane remains usable after a saturated IG source failure',async t=>{
 const input=new PassThrough(),output=new PassThrough(),rawInput=new PassThrough(),rawOutput=new PassThrough();
 const f=fixture(),replies=[];let buffer='';
 const {off}=wireFixture(f,rawInput,rawOutput,{storm:true});
 output.on('data',chunk=>{
  buffer+=chunk.toString('utf8');let at;
  while((at=buffer.indexOf('\n'))>=0){replies.push(JSON.parse(buffer.slice(0,at)));buffer=buffer.slice(at+1);}
 });
 const server=serveMainBrowserTransport({input,output,connect:async()=>({input:rawInput,output:rawOutput}),
  createX:async(_params,transport)=>({execute:async()=>transport.call('Browser.getVersion',{},'',1000),close:async()=>{}}),
  sourceFactories:{instagram:async(_params,transport)=>({
   async execute(){const page=await createNativeOwnedPage(transport,1000);try{return await readInstagramEvidence(page,attempt,'reader','PRIVATE_OWNER','PRIVATE_PROFILE','PRIVATE_SECRET');}finally{await page.close();}},
   close:async()=>{},
  })},
 });
 t.after(()=>{off();server.close();});
 const send=value=>input.write(JSON.stringify(value)+'\n');
 const wait=async predicate=>{for(let i=0;i<1000;i++){const value=predicate();if(value)return value;await new Promise(resolve=>setTimeout(resolve,10));}assert.fail('synthetic control reply did not arrive');};
 send({op:'main_transport_bootstrap',schema:MAIN_TRANSPORT_SCHEMA,nonce,transport:'inherited-stdio'});
 await wait(()=>replies.find(reply=>reply.ready));
 send({id:1,op:'main_x',operation:'daily_owner_bootstrap',params:{}});assert.equal((await wait(()=>replies.find(reply=>reply.id===1))).ok,true);
 send({id:2,op:'main_source',source:'instagram',operation:'source_bootstrap',params:{}});assert.equal((await wait(()=>replies.find(reply=>reply.id===2))).ok,true);
 send({id:3,op:'main_source',source:'instagram',operation:'source_check',params:{}});
 const failure=await wait(()=>replies.find(reply=>reply.id===3));
 assert.equal(failure.ok,false);assert.equal(failure.error.code,'native_source_evidence_reader_bootstrap_wire_unmatched');
 assert.equal(failure.error.diagnostic.complete,false);assert.equal(failure.error.diagnostic.cleanup.state,'verified');
 assert.equal(failure.error.diagnostic.cause.condition,'root_request_duplicate');
 assert.ok(!JSON.stringify(failure).includes('PRIVATE'));assert.equal(replies.some(reply=>reply.event==='main_transport_lifecycle'),false);
 send({id:4,op:'main_x',operation:'daily_reader_check',params:{}});
 const x=await wait(()=>replies.find(reply=>reply.id===4));assert.equal(x.ok,true);assert.equal(x.result.product,'synthetic');
});

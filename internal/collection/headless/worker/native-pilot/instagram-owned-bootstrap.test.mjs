import test from 'node:test';
import assert from 'node:assert/strict';
import {fixture,tuple} from './instagram-bootstrap-fixture.mjs';
import {acquireOwnedInstagramIdentity} from './instagram-bootstrap-corroboration.mjs';
import {readInstagramEvidence,verifyInstagramPair} from './instagram-native-session.mjs';
import {nativeSourceDiagnostic} from './native-source-kernel.mjs';
import {createIGTrace,igErrorTrace,validateIGTrace} from './instagram-diagnostic-trace.mjs';
import {PassThrough} from 'node:stream';
import {createMainCDP,serveMainBrowserTransport,MAIN_TRANSPORT_SCHEMA} from './main-browser-transport-protocol.mjs';
import {createNativeOwnedPage,nativeSourceFailure} from './native-source-kernel.mjs';

const nonce='a'.repeat(48),binding={nonce,generation:1,profileScope:'scope'};
function owned(options={}){
 const f=fixture(options),calls=[];
 const cookies=['csrftoken','ds_user_id','sessionid'].map(name=>({name,value:name==='ds_user_id'?'123':'PRIVATE_'+name,domain:'.instagram.com',path:'/',secure:true,httpOnly:name==='sessionid',expires:-1}));
 const page={on:f.raw.subscribe,async send(method,params,ms){
  calls.push(method);
  if(method==='Network.getCookies')return {cookies:structuredClone(cookies)};
  if(method==='Fetch.disable'&&options.disableFails)throw Error('PRIVATE');
  return f.raw.call(method,params,'owned-session',ms);
 }};
 return {page,f,calls,cookies};
}
const attempt={binding,run:async(stage,fn)=>fn(60000)};

test('private acquisition preserves live owned targets and original delivery time',async()=>{
 const f=owned();let time=10000;
 const result=await acquireOwnedInstagramIdentity(f.page,nonce,{now:()=>time,budgetMS:30000});
 assert.equal(result.accountId,'123');assert.equal(result.observedAtMS,10000);
 assert.equal(result.diagnostic.qualified,false);assert.equal(result.diagnostic.cleanup,'verified');
 assert.ok(!JSON.stringify(result.diagnostic).includes('123'));
 assert.equal(f.calls.filter(x=>x==='Fetch.disable').length,1);
 assert.ok(!f.calls.some(x=>x.startsWith('Target.')));
 assert.equal(f.f.listeners.size,0);
 time+=1;
 const again=await acquireOwnedInstagramIdentity(f.page,nonce,{now:()=>time});
 assert.equal(again.accountId,'123');assert.equal(again.observedAtMS,10001);
});

test('both roles produce unchanged bound HMAC proofs without endpoint requests',async()=>{
 const reader=owned(),collector=owned();
 const results=await Promise.all([readInstagramEvidence(reader.page,attempt,'reader','1','reader','secret'),readInstagramEvidence(collector.page,attempt,'collector','2','collector','secret')]);
 const pair=verifyInstagramPair(results[0].proof,results[1].proof,binding,Date.now());
 assert.equal(pair.reader.accountKey,pair.collector.accountKey);
 assert.ok(!JSON.stringify(pair).includes('PRIVATE'));assert.match(pair.reader.accountKey,/^[a-f0-9]{64}$/);
 assert.ok([...reader.f.calls,...collector.f.calls].every(c=>!JSON.stringify(c.params).includes('current_user')));
 for(const mutate of [p=>p.nonce='b'.repeat(48),p=>p.generation++,p=>p.role='collector',p=>p.profileScope='other',p=>p.observedAtMS-=6001]){
  const proof={...pair.reader};mutate(proof);assert.throws(()=>verifyInstagramPair(proof,pair.collector,binding,Date.now()));
 }
});

test('private acquisition rejects incomplete, duplicate, conflicting, cached and changed documents',async()=>{
 for(const [options,reason] of [[{domScripts:[tuple('123'),tuple('123')]},'bootstrap_rejected'],[{bodyScripts:[tuple('123'),tuple('456')]},'bootstrap_rejected'],
  [{domScripts:Array(4097).fill(tuple('123'))},'bootstrap_rejected'],
  [{bodyScripts:['{PRIVATE']},'bootstrap_rejected'],[{cached:true},'delivery_unproven'],[{changeContext:true},'document_changed'],
  [{cookieID:'456'},'cookie_mismatch'],[{domID:'456'},'id_mismatch'],[{forgedReply:true},'projection_invalid'],[{disableFails:true},'cleanup_failed']]){
  const f=owned(options);const result=await acquireOwnedInstagramIdentity(f.page,nonce);
  assert.equal(result.diagnostic.reason,reason);assert.equal(result.accountId,'');assert.equal(f.f.listeners.size,0);
 }
});

test('insufficient remaining acquisition budget performs no navigation or session read',async()=>{
 const f=owned(),result=await acquireOwnedInstagramIdentity(f.page,nonce,{budgetMS:2000});
 assert.equal(result.accountId,'');assert.equal(result.diagnostic.reason,'setup_failed');
 assert.ok(!f.calls.includes('Page.navigate'));assert.ok(!f.calls.includes('Network.getCookies'));
 assert.equal(f.f.listeners.size,0);
});

test('stale delivery and cookie mismatch never acquire a new proof timestamp',async()=>{
 const f=owned();let now=10000;
 const send=f.page.send;f.page.send=async(method,...args)=>{const v=await send(method,...args);if(method==='Network.getCookies')now+=6001;return v;};
 await assert.rejects(readInstagramEvidence(f.page,attempt,'reader','1','p','s',()=>now),e=>nativeSourceDiagnostic(e).reason==='stale');
 const g=owned();g.cookies.find(c=>c.name==='ds_user_id').value='456';
 await assert.rejects(readInstagramEvidence(g.page,attempt,'reader','1','p','s'),e=>nativeSourceDiagnostic(e).reason==='account_mismatch');
});

test('private errors retain role and first cause separately from failed cleanup',async()=>{
 for(const role of ['reader','collector']){
  const f=owned({readFailure:true,disableFails:true});
  await assert.rejects(readInstagramEvidence(f.page,attempt,role,'PRIVATE','PRIVATE','PRIVATE'),error=>{
   assert.equal(nativeSourceDiagnostic(error).reason,role+'_bootstrap_body_read');
   assert.equal(igErrorTrace(error).cleanup.state,'uncertain');
   assert.ok(!JSON.stringify(igErrorTrace(error)).includes('PRIVATE'));return true;
  });
 }
});

test('context replacement between acquisition and cookie read cannot publish a proof',async()=>{
 for(const event of [
  {method:'Runtime.executionContextsCleared',params:{}},
  {method:'Runtime.executionContextDestroyed',params:{executionContextId:7}},
  {method:'Runtime.executionContextCreated',params:{context:{id:8,uniqueId:'replacement',auxData:{isDefault:true,frameId:'root-frame'}}}},
 ]){
  const f=owned(),send=f.page.send;
  f.page.send=async(method,...args)=>{const value=await send(method,...args);if(method==='Network.getCookies')f.f.emit(event.method,event.params);return value;};
  await assert.rejects(readInstagramEvidence(f.page,attempt,'reader','1','p','s'),e=>nativeSourceDiagnostic(e).reason==='document_changed');
  assert.equal(f.f.listeners.size,0);
 }
});

const invalidations=[
 ['document_context_cleared','Runtime.executionContextsCleared',{}],
 ['document_context_destroyed','Runtime.executionContextDestroyed',{executionContextId:7}],
 ['document_context_destroyed','Runtime.executionContextDestroyed',{executionContextUniqueId:'root-context'}],
 ['document_context_replaced','Runtime.executionContextCreated',{context:{id:8,uniqueId:'PRIVATE_REPLACEMENT',auxData:{isDefault:true,frameId:'root-frame'}}}],
 ['document_frame_navigated','Page.frameNavigated',{frame:{id:'root-frame',loaderId:'root-loader',url:'https://www.instagram.com/'}}],
 ['document_frame_detached','Page.frameDetached',{frameId:'root-frame'}],
 ['document_within_navigation','Page.navigatedWithinDocument',{frameId:'root-frame',url:'https://www.instagram.com/'}],
];
const assertDocumentFailure=(error,condition,reason='document_changed',cleanup='verified',stage='post_check')=>{
 assert.equal(nativeSourceDiagnostic(error).reason,reason);
 const trace=igErrorTrace(error);assert.ok(validateIGTrace(trace));
 assert.deepEqual(trace.cause,{stage,condition});assert.equal(trace.cleanup.state,cleanup);
 assert.ok(!JSON.stringify(trace).includes('PRIVATE'));assert.ok(Buffer.byteLength(JSON.stringify(trace))<=512);return true;
};

test('each invalidation blocks both roles during cleanup, handoff, cookies and final readiness',async t=>{
 for(const role of ['reader','collector'])for(const phase of ['cleanup','handoff','cookie','readiness'])for(const [condition,method,params] of invalidations){
  await t.test(`${role} ${phase} ${condition} ${JSON.stringify(params)}`,async()=>{
   const f=owned(),send=f.page.send;
   f.page.send=async(command,...args)=>{
    const result=await send(command,...args);
    if(phase==='cleanup'&&command==='Fetch.disable'||phase==='cookie'&&command==='Network.getCookies'||
       phase==='readiness'&&command==='Runtime.evaluate'&&f.calls.includes('Network.getCookies'))f.f.emit(method,params);
    return result;
   };
   const localAttempt={binding,async run(stage,fn){const result=await fn(60000);if(phase==='handoff'&&result?.diagnostic)f.f.emit(method,params);return result;}};
   await assert.rejects(readInstagramEvidence(f.page,localAttempt,role,'PRIVATE_OWNER','PRIVATE_PROFILE','PRIVATE_SECRET'),
    error=>assertDocumentFailure(error,condition,phase==='cleanup'?role+'_bootstrap_document_changed':'document_changed','verified',phase==='readiness'?'post_check':phase));
   if(['cleanup','handoff'].includes(phase))assert.ok(!f.calls.includes('Network.getCookies'));
   assert.equal(f.f.listeners.size,0);assert.ok(!f.calls.some(x=>x.startsWith('Target.')));
  });
 }
});

test('standalone owned acquisition rechecks document validity after awaited cleanup',async()=>{
 const f=owned(),trace=createIGTrace(),send=f.page.send;
 f.page.send=async(method,...args)=>{const result=await send(method,...args);if(method==='Fetch.disable')f.f.emit('Page.navigatedWithinDocument',{frameId:'root-frame'});return result;};
 const result=await acquireOwnedInstagramIdentity(f.page,nonce,{trace});
 assert.equal(result.diagnostic.reason,'document_changed');assert.equal(result.diagnostic.documentBound,false);
 assert.equal(result.diagnostic.cleanup,'verified');assert.equal(result.accountId,'');assert.equal(result.observedAtMS,undefined);assert.equal(f.f.listeners.size,0);
 assert.deepEqual(trace.snapshot().cause,{stage:'cleanup',condition:'document_within_navigation'});
});

test('document invalidation during page inspection records the observed parsing stage',async()=>{
 for(const role of ['reader','collector']){
  let f;f=owned({onCookie:()=>f.f.emit('Page.navigatedWithinDocument',{frameId:'root-frame'})});
  await assert.rejects(readInstagramEvidence(f.page,attempt,role,'PRIVATE_OWNER','PRIVATE_PROFILE','PRIVATE_SECRET'),
   error=>assertDocumentFailure(error,'document_within_navigation',`${role}_bootstrap_document_changed`,'verified','parsing'));
  assert.ok(!f.calls.includes('Network.getCookies'));assert.equal(f.f.listeners.size,0);
 }
});

test('cleanup rejects late delivery failures for both owned roles',async t=>{
 const cases=[
  ['wire_unmatched','root_request_duplicate','Network.requestWillBeSent',{frameId:'root-frame',type:'Document',requestId:'late-request',loaderId:'late-loader',request:{url:'https://www.instagram.com/',method:'GET'}}],
  ['wire_unmatched','pause_duplicate','Fetch.requestPaused',{requestId:'late-fetch'}],
  ['wire_unmatched','response_duplicate','Network.responseReceived',{requestId:'root-request',frameId:'root-frame',loaderId:'root-loader',type:'Document',response:{url:'https://www.instagram.com/',status:200}}],
  ['delivery_unproven','extra_duplicate','Network.responseReceivedExtraInfo',{requestId:'root-request',statusCode:200}],
  ['delivery_unproven','cache_observed','Network.requestServedFromCache',{requestId:'root-request'}],
  ['navigation_failed','navigation_failed','Network.loadingFailed',{requestId:'root-request'}],
 ];
 for(const role of ['reader','collector'])for(const [reason,condition,event,params] of cases)await t.test(`${role} ${condition}`,async()=>{
  const f=owned(),send=f.page.send;
  f.page.send=async(method,...args)=>{const result=await send(method,...args);if(method==='Fetch.disable')f.f.emit(event,params);return result;};
  await assert.rejects(readInstagramEvidence(f.page,attempt,role,'PRIVATE_OWNER','PRIVATE_PROFILE','PRIVATE_SECRET'),error=>{
   assert.equal(nativeSourceDiagnostic(error).reason,`${role}_bootstrap_${reason}`);
   const trace=igErrorTrace(error);assert.ok(validateIGTrace(trace));assert.equal(trace.cause.condition,condition);
   assert.equal(trace.cleanup.state,'verified');assert.ok(!JSON.stringify(trace).includes('PRIVATE'));return true;
  });
  assert.ok(!f.calls.includes('Network.getCookies'));assert.equal(f.f.listeners.size,0);
 });
});

test('cleanup preserves first delivery failure and independent cleanup state',async()=>{
 for(const cleanupFails of [false,true]){
  const f=owned(),send=f.page.send;
  f.page.send=async(method,...args)=>{
   if(method==='Fetch.disable'){
    f.f.emit('Network.requestWillBeSent',{frameId:'root-frame',type:'Document',requestId:'late-request',loaderId:'late-loader',request:{url:'https://www.instagram.com/',method:'GET'}});
    f.f.emit('Network.requestServedFromCache',{requestId:'root-request'});
    if(cleanupFails)throw Error('PRIVATE_CLEANUP');
   }
   return send(method,...args);
  };
  await assert.rejects(readInstagramEvidence(f.page,attempt,'reader','1','p','s'),error=>{
   assert.equal(nativeSourceDiagnostic(error).reason,'reader_bootstrap_wire_unmatched');
   const trace=igErrorTrace(error);assert.equal(trace.cause.condition,'root_request_duplicate');
   assert.equal(trace.cleanup.state,cleanupFails?'uncertain':'verified');return true;
  });
  assert.equal(f.f.listeners.size,0);assert.ok(!f.calls.includes('Network.getCookies'));
 }
 const f=owned({readFailure:true}),send=f.page.send;
 f.page.send=async(method,...args)=>{if(method==='Fetch.disable')f.f.emit('Network.requestServedFromCache',{requestId:'root-request'});return send(method,...args);};
 await assert.rejects(readInstagramEvidence(f.page,attempt,'reader','1','p','s'),error=>{
  assert.equal(nativeSourceDiagnostic(error).reason,'reader_bootstrap_body_read');
  assert.equal(igErrorTrace(error).cause.condition,'read_command_failed');return true;
 });
 assert.equal(f.f.listeners.size,0);
 const earlier=owned({cookieID:'456'}),earlierSend=earlier.page.send;
 earlier.page.send=async(method,...args)=>{if(method==='Fetch.disable')earlier.f.emit('Network.requestServedFromCache',{requestId:'root-request'});return earlierSend(method,...args);};
 await assert.rejects(readInstagramEvidence(earlier.page,attempt,'reader','1','p','s'),error=>{
  assert.equal(nativeSourceDiagnostic(error).reason,'reader_bootstrap_cookie_mismatch');
  assert.equal(igErrorTrace(error).cause.condition,'cookie_mismatch');return true;
 });
 assert.equal(earlier.f.listeners.size,0);
});

test('standalone owned acquisition clears private identity on late delivery failure',async()=>{
 const f=owned(),trace=createIGTrace(),send=f.page.send;
 f.page.send=async(method,...args)=>{
  const result=await send(method,...args);
  if(method==='Fetch.disable')f.f.emit('Network.requestWillBeSent',{frameId:'root-frame',type:'Document',requestId:'late-request',loaderId:'late-loader',request:{url:'https://www.instagram.com/',method:'GET'}});
  return result;
 };
 const result=await acquireOwnedInstagramIdentity(f.page,nonce,{trace});
 assert.equal(result.diagnostic.reason,'wire_unmatched');assert.equal(result.diagnostic.cleanup,'verified');
 assert.equal(result.accountId,'');assert.equal(result.observedAtMS,undefined);
 assert.equal(trace.snapshot().cause.condition,'root_request_duplicate');assert.equal(f.f.listeners.size,0);
});

test('unrelated delivery events during cleanup do not invalidate the owned root',async()=>{
 for(const role of ['reader','collector']){
  const f=owned(),send=f.page.send;
  f.page.send=async(method,...args)=>{
   const result=await send(method,...args);
   if(method==='Fetch.disable'){
    f.f.emit('Network.requestWillBeSent',{frameId:'other-frame',type:'Document',requestId:'other-request',loaderId:'other-loader',request:{url:'https://other.invalid/',method:'GET'}});
    f.f.emit('Network.responseReceivedExtraInfo',{requestId:'other-request',statusCode:403});
    f.f.emit('Network.requestServedFromCache',{requestId:'other-request'});
    f.f.emit('Network.loadingFailed',{requestId:'other-request'});
   }
   return result;
  };
  const result=await readInstagramEvidence(f.page,attempt,role,'1','p','s');
  assert.equal(result.proof.authenticated,true);assert.ok(f.calls.includes('Network.getCookies'));
  assert.equal(f.f.listeners.size,0);
 }
});

test('cleanup first-event ordering preserves document and delivery causes',async()=>{
 for(const documentFirst of [true,false]){
  const f=owned(),send=f.page.send;
  f.page.send=async(method,...args)=>{
   if(method==='Fetch.disable'){
    const document=()=>f.f.emit('Page.navigatedWithinDocument',{frameId:'root-frame'});
    const delivery=()=>f.f.emit('Network.requestWillBeSent',{frameId:'root-frame',type:'Document',requestId:'late-request',loaderId:'late-loader',request:{url:'https://www.instagram.com/',method:'GET'}});
    if(documentFirst){document();delivery();}else{delivery();document();}
   }
   return send(method,...args);
  };
  await assert.rejects(readInstagramEvidence(f.page,attempt,'reader','1','p','s'),error=>{
   assert.equal(nativeSourceDiagnostic(error).reason,documentFirst?'reader_bootstrap_document_changed':'reader_bootstrap_wire_unmatched');
   assert.deepEqual(igErrorTrace(error).cause,{stage:documentFirst?'cleanup':'root_delivery',condition:documentFirst?'document_within_navigation':'root_request_duplicate'});return true;
  });
  assert.equal(f.f.listeners.size,0);assert.ok(!f.calls.includes('Network.getCookies'));
  const standalone=owned(),trace=createIGTrace(),standaloneSend=standalone.page.send;
  standalone.page.send=async(method,...args)=>{
   if(method==='Fetch.disable'){
    const document=()=>standalone.f.emit('Page.navigatedWithinDocument',{frameId:'root-frame'});
    const delivery=()=>standalone.f.emit('Network.requestWillBeSent',{frameId:'root-frame',type:'Document',requestId:'late-request',loaderId:'late-loader',request:{url:'https://www.instagram.com/',method:'GET'}});
    if(documentFirst){document();delivery();}else{delivery();document();}
   }
   return standaloneSend(method,...args);
  };
  const result=await acquireOwnedInstagramIdentity(standalone.page,nonce,{trace});
  assert.equal(result.diagnostic.reason,documentFirst?'document_changed':'wire_unmatched');
  assert.deepEqual(trace.snapshot().cause,{stage:documentFirst?'cleanup':'root_delivery',condition:documentFirst?'document_within_navigation':'root_request_duplicate'});
  assert.equal(result.accountId,'');assert.equal(standalone.f.listeners.size,0);
 }
});

test('document invalidation and uncertain cleanup remain independent observations',async()=>{
 const f=owned(),send=f.page.send;
 f.page.send=async(method,...args)=>{if(method==='Fetch.disable'){f.f.emit('Page.navigatedWithinDocument',{frameId:'root-frame'});throw Error('PRIVATE_CLEANUP');}return send(method,...args);};
 await assert.rejects(readInstagramEvidence(f.page,attempt,'reader','1','p','s'),error=>assertDocumentFailure(error,'document_within_navigation','reader_bootstrap_document_changed','uncertain','cleanup'));
 assert.equal(f.f.listeners.size,0);assert.ok(!f.calls.includes('Network.getCookies'));
});

test('an invalidation observed before a command failure retains its diagnostic cause',async()=>{
 const f=owned(),send=f.page.send;
 f.page.send=async(method,...args)=>{
  if(method==='Network.getCookies'){
   f.f.emit('Page.navigatedWithinDocument',{frameId:'root-frame'});
   throw nativeSourceFailure('session','unavailable');
  }
  return send(method,...args);
 };
 await assert.rejects(readInstagramEvidence(f.page,attempt,'reader','1','p','s'),error=>assertDocumentFailure(error,'document_within_navigation','unavailable','verified','cookie'));
 assert.equal(f.f.listeners.size,0);
});

test('final frame and evaluation failures have distinct bounded conditions',async t=>{
 const cases=[
  ['document_frame_invalid','Page.getFrameTree',()=>({frameTree:{frame:null}}),'document'],
  ['document_frame_mismatch','Page.getFrameTree',()=>({frameTree:{frame:{id:'PRIVATE_FRAME',loaderId:'root-loader',url:'https://www.instagram.com/'}}})],
  ['document_loader_mismatch','Page.getFrameTree',()=>({frameTree:{frame:{id:'root-frame',loaderId:'PRIVATE_LOADER',url:'https://www.instagram.com/'}}})],
  ['document_url_mismatch','Page.getFrameTree',()=>({frameTree:{frame:{id:'root-frame',loaderId:'root-loader',url:'https://PRIVATE_URL/'}}}),'document'],
  ['document_frame_request_failed','Page.getFrameTree',()=>{throw nativeSourceFailure('evidence','unavailable');},'unavailable'],
  ['document_evaluation_request_failed','Runtime.evaluate',()=>{throw nativeSourceFailure('evidence','unavailable');},'unavailable'],
  ['document_evaluation_exception','Runtime.evaluate',()=>({exceptionDetails:{text:'PRIVATE_EXCEPTION'},result:{value:true}})],
  ['document_not_ready','Runtime.evaluate',()=>({result:{value:false}})],
  ['document_not_ready','Runtime.evaluate',()=>({})],
 ];
 for(const [condition,command,reply,reason='document_changed'] of cases)await t.test(condition,async()=>{
  const f=owned(),send=f.page.send;
  f.page.send=async(method,...args)=>method===command&&f.calls.includes('Network.getCookies')?reply():send(method,...args);
  await assert.rejects(readInstagramEvidence(f.page,attempt,'reader','PRIVATE_OWNER','PRIVATE_PROFILE','PRIVATE_SECRET'),error=>assertDocumentFailure(error,condition,reason));
  assert.equal(f.f.listeners.size,0);
 });
});

test('unrelated subframe and non-default contexts preserve proofs without listener growth',async()=>{
 const f=owned(),send=f.page.send;
 f.page.send=async(method,...args)=>{
  const result=await send(method,...args);
  if(['Fetch.disable','Network.getCookies'].includes(method)){
   f.f.emit('Page.navigatedWithinDocument',{frameId:'foreign-frame'});
   f.f.emit('Page.frameDetached',{frameId:'foreign-frame'});
   f.f.emit('Runtime.executionContextCreated',{context:{id:9,uniqueId:'isolated-context',auxData:{isDefault:false,frameId:'root-frame'}}});
   f.f.emit('Runtime.executionContextDestroyed',{executionContextId:9,executionContextUniqueId:'isolated-context'});
  }
  return result;
 };
 for(let i=0;i<3;i++){
  const result=await readInstagramEvidence(f.page,attempt,'reader','1','p','s');assert.equal(result.proof.authenticated,true);assert.equal(f.f.listeners.size,0);
 }
});

test('cancellation before binding cannot install a late verifier observer',async()=>{
 const f=owned(),send=f.page.send;
 let readyEntered,releaseReady,cancel,pending;
 const entered=new Promise(resolve=>{readyEntered=resolve;});
 const ready=new Promise(resolve=>{releaseReady=resolve;});
 const canceled=new Promise((_,reject)=>{cancel=()=>reject(nativeSourceFailure('evidence','canceled'));});
 f.page.send=async(method,...args)=>{if(method==='Runtime.evaluate'){readyEntered();await ready;}return send(method,...args);};
 const localAttempt={binding,run:(_stage,fn)=>{pending=fn(60000);return Promise.race([pending,canceled]);}};
 const reading=readInstagramEvidence(f.page,localAttempt,'reader','1','p','s');
 await entered;cancel();
 try{await assert.rejects(reading,error=>nativeSourceDiagnostic(error).reason==='canceled');}
 finally{releaseReady();}
 const late=await pending;
 assert.notEqual(late.diagnostic.reason,'corroborated');assert.equal(late.accountId,'');
 assert.equal(f.f.listeners.size,0);assert.ok(!f.calls.includes('Network.getCookies'));
});

test('watcher disposal failure blocks success and preserves an earlier document failure',async()=>{
 for(const invalidate of [false,true]){
  const f=owned(),on=f.page.on,send=f.page.send;let subscriptions=0;
  f.page.on=listener=>{const off=on(listener);const verifier=++subscriptions===3;return()=>{off();if(verifier)throw Error('PRIVATE_DISPOSE');};};
  f.page.send=async(method,...args)=>{const result=await send(method,...args);if(invalidate&&method==='Network.getCookies')f.f.emit('Page.navigatedWithinDocument',{frameId:'root-frame'});return result;};
  await assert.rejects(readInstagramEvidence(f.page,attempt,'reader','1','p','s'),error=>{
   assert.equal(nativeSourceDiagnostic(error).reason,invalidate?'document_changed':'unverified');
   assert.equal(igErrorTrace(error).cleanup.condition,'dispose_failed');
   if(invalidate)assert.equal(igErrorTrace(error).cause.condition,'document_within_navigation');
   return true;
  });
  assert.equal(f.f.listeners.size,0);
 }
});

test('real main CDP framing binds acquisition to the batch-owned target until batch retirement',async t=>{
 const f=fixture(),input=new PassThrough(),output=new PassThrough(),requests=[];
 const transport=createMainCDP({input,output});t.after(()=>transport.close());
 const off=f.raw.subscribe(event=>input.write(JSON.stringify(event)+'\0'));t.after(off);
 output.on('data',wire=>{
  const q=JSON.parse(wire.toString().slice(0,-1));requests.push(q);
  f.raw.call(q.method,q.params,q.sessionId,1000).then(result=>input.write(JSON.stringify({id:q.id,...(q.sessionId?{sessionId:q.sessionId}:{}),result})+'\0'));
 });
 const page=await createNativeOwnedPage(transport,1000);
 const result=await acquireOwnedInstagramIdentity(page,nonce);
 assert.equal(result.accountId,'123');assert.equal(result.diagnostic.cleanup,'verified');
 assert.equal(transport.privateTarget('owned-target'),true);
 assert.equal(requests.some(q=>q.method==='Target.closeTarget'),false);
 await page.close();assert.equal(transport.privateTarget('owned-target'),false);
 assert.equal(transport.privateSession('owned-session'),false);
 assert.equal(requests.filter(q=>q.method==='Target.closeTarget').length,1);
});

test('bootstrap rejection remains a closed role-specific code across the source control wire',async t=>{
 const input=new PassThrough(),output=new PassThrough(),rawInput=new PassThrough(),rawOutput=new PassThrough(),replies=[];
 output.on('data',wire=>replies.push(JSON.parse(wire)));
 const server=serveMainBrowserTransport({input,output,connect:async()=>({input:rawInput,output:rawOutput}),
  sourceFactories:{instagram:async()=>({execute:async()=>{throw nativeSourceFailure('evidence','collector_bootstrap_stale_delivery');},close:async()=>{}})}});
 t.after(()=>server.close());const tick=()=>new Promise(r=>setImmediate(r));
 input.write(JSON.stringify({op:'main_transport_bootstrap',schema:MAIN_TRANSPORT_SCHEMA,nonce,transport:'inherited-stdio'})+'\n');await tick();
 input.write(JSON.stringify({id:1,op:'main_source',source:'instagram',operation:'source_bootstrap',params:{}})+'\n');await tick();
 input.write(JSON.stringify({id:2,op:'main_source',source:'instagram',operation:'source_check',params:{}})+'\n');await tick();
 assert.equal(replies.find(v=>v.id===2&&!v.event).error.code,'native_source_evidence_collector_bootstrap_stale_delivery');
});

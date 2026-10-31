import test from 'node:test';
import assert from 'node:assert/strict';
import {fixture,tuple} from './instagram-bootstrap-fixture.mjs';
import {acquireOwnedInstagramIdentity} from './instagram-bootstrap-corroboration.mjs';
import {readInstagramEvidence,verifyInstagramPair} from './instagram-native-session.mjs';
import {nativeSourceDiagnostic} from './native-source-kernel.mjs';
import {igErrorTrace} from './instagram-diagnostic-trace.mjs';
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

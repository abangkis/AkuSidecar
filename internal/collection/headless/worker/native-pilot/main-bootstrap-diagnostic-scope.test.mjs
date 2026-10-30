import test from 'node:test';
import assert from 'node:assert/strict';
import {PassThrough} from 'node:stream';
import {createMainBootstrapDiagnosticScope} from './main-bootstrap-diagnostic-scope.mjs';
import {diagnoseInstagramSelfBootstrap} from './instagram-self-bootstrap-probe.mjs';
import {serveMainBrowserTransport,MAIN_TRANSPORT_SCHEMA} from './main-browser-transport-protocol.mjs';
const result=()=>({schema:'aku.instagram-self-bootstrap-probe.v1',reason:'missing',modulePresent:false,idString:false,idSafeNumber:false,usernameString:false,documentBound:true,serverFresh:false,qualified:false,scanComplete:true,budgetExpanded:false,failureStage:'none',failureCondition:'none',cleanup:'verified',cleanupStage:'none',cleanupCondition:'none'});
function fixture(options={}){
 const calls=[],sessions=new Set();let listener;
 const raw={alive:()=>true,privateTarget:()=>false,privateSession:id=>sessions.has(id),reserveSession:id=>sessions.add(id),releaseSession:id=>sessions.delete(id),subscribe:fn=>{listener=fn;return()=>{listener=null;};},
  async call(method,params,session){calls.push(method);
   if(method==='Target.getTargets')return{targetInfos:Array.from({length:options.targets??1},(_,i)=>({targetId:'user-'+i,type:'page',url:'https://www.instagram.com/'}))};
   if(method==='Target.attachToTarget')return{sessionId:'diagnostic'};
   if(method==='Runtime.enable'){
    listener({sessionId:'diagnostic',method:'Runtime.executionContextCreated',params:{context:{id:1,uniqueId:'unique',auxData:{isDefault:true,frameId:'root'}}}});
    if(options.iframe)for(const event of [
     {method:'Runtime.executionContextCreated',params:{context:{id:2,uniqueId:'child',auxData:{isDefault:true,frameId:'child'}}}},
     {method:'Page.frameNavigated',params:{frame:{id:'child',loaderId:'child-loader'}}},
     {method:'Runtime.executionContextDestroyed',params:{executionContextId:2}},
     {method:'Page.frameDetached',params:{frameId:'child'}}])listener({sessionId:'diagnostic',...event});
    if(options.mainChanged)listener({sessionId:'diagnostic',method:'Runtime.executionContextCreated',params:{context:{id:3,uniqueId:'replacement',auxData:{isDefault:true,frameId:'root'}}}});
    if(options.mainEvent)listener({sessionId:'diagnostic',...options.mainEvent});
   }
   if(method==='Page.getFrameTree')return{frameTree:{frame:{id:'root',loaderId:'loader',url:'https://www.instagram.com/'}}};
   if(method==='Runtime.evaluate') {assert.equal(params.uniqueContextId,'unique');if(options.evaluateFails)throw Error('PRIVATE');const {cleanup,failureStage,failureCondition,cleanupStage,cleanupCondition,...value}=result();return{result:{value}};}
   if(method==='Target.detachFromTarget'&&options.detachFails)throw Error('PRIVATE');
   return{};
  }};
 return{raw,calls,sessions};
}
test('bootstrap runner uses existing target, pins context and detaches only its session',async()=>{
 const f=fixture();const r=await diagnoseInstagramSelfBootstrap({createBootstrapDiagnosticScope:()=>createMainBootstrapDiagnosticScope(f.raw)});
 assert.deepEqual(r,result());assert.equal(f.sessions.size,0);
 assert(!f.calls.some(m=>['Target.createTarget','Target.closeTarget','Page.navigate','Network.getCookies','Network.setCookies'].includes(m)));
});
test('absent or ambiguous Home never attaches; failed detach stays uncertain',async()=>{
 for(const targets of [0,2]){const f=fixture({targets});const r=await diagnoseInstagramSelfBootstrap({createBootstrapDiagnosticScope:()=>createMainBootstrapDiagnosticScope(f.raw)});assert.equal(r.failureStage,'target_selection');assert.equal(r.failureCondition,targets===0?'missing_home':'ambiguous_home');assert.equal(r.cleanup,'verified');assert(!f.calls.includes('Target.attachToTarget'));}
 const f=fixture({detachFails:true});const r=await diagnoseInstagramSelfBootstrap({createBootstrapDiagnosticScope:()=>createMainBootstrapDiagnosticScope(f.raw)});assert.equal(r.failureStage,'none');assert.equal(r.cleanup,'uncertain');assert.equal(r.cleanupStage,'detach');assert.equal(f.sessions.size,1);assert(!f.calls.includes('Target.closeTarget'));
});
test('restricted page rejects arbitrary methods and expressions',async()=>{
 const f=fixture(),scope=createMainBootstrapDiagnosticScope(f.raw),page=await scope.attach();
 await assert.rejects(page.send('Network.getCookies',{}));await assert.rejects(page.evaluate('PRIVATE'));
 await scope.close();
});
test('late attachment is detached after local timeout without closing its user target',async()=>{
 const f=fixture();let late;const original=f.raw.call;
 f.raw.call=async(method,params,session,ms,onLate)=>{
  if(method==='Target.attachToTarget'){late=onLate;throw Error('local timeout');}
  return original(method,params,session,ms,onLate);
 };
 const r=await diagnoseInstagramSelfBootstrap({createBootstrapDiagnosticScope:()=>createMainBootstrapDiagnosticScope(f.raw)});
 assert.equal(r.failureStage,'attachment');assert.equal(r.cleanupStage,'attachment');assert.equal(r.cleanupCondition,'attachment_pending');
 await late({sessionId:'late-attachment'});
 assert.equal(f.sessions.size,0);assert(f.calls.includes('Target.detachFromTarget'));assert(!f.calls.includes('Target.closeTarget'));
});
test('iframe contexts and events do not replace or invalidate the pinned main context',async()=>{
 const f=fixture({iframe:true});const r=await diagnoseInstagramSelfBootstrap({createBootstrapDiagnosticScope:()=>createMainBootstrapDiagnosticScope(f.raw)});assert.deepEqual(r,result());
 const changed=fixture({mainChanged:true});const rejected=await diagnoseInstagramSelfBootstrap({createBootstrapDiagnosticScope:()=>createMainBootstrapDiagnosticScope(changed.raw)});assert.equal(rejected.failureStage,'frame_before');assert.equal(rejected.failureCondition,'document_changed');assert(!changed.calls.includes('Runtime.evaluate'));
});
test('evaluation first cause survives independent detach failure without private error text',async()=>{
 const f=fixture({evaluateFails:true,detachFails:true});const r=await diagnoseInstagramSelfBootstrap({createBootstrapDiagnosticScope:()=>createMainBootstrapDiagnosticScope(f.raw)});
 assert.equal(r.failureStage,'evaluation');assert.equal(r.failureCondition,'request_failed');assert.equal(r.cleanup,'uncertain');assert.equal(r.cleanupStage,'detach');assert.equal(r.cleanupCondition,'request_failed');assert(!JSON.stringify(r).includes('PRIVATE'));
});
test('main frame navigation, main context destruction and global clear remain rejecting',async()=>{
 for(const mainEvent of [
  {method:'Page.frameNavigated',params:{frame:{id:'root',loaderId:'new-loader',url:'https://www.instagram.com/'}}},
  {method:'Runtime.executionContextDestroyed',params:{executionContextId:1}},
  {method:'Runtime.executionContextsCleared',params:{}},
 ]){
  const f=fixture({mainEvent});const r=await diagnoseInstagramSelfBootstrap({createBootstrapDiagnosticScope:()=>createMainBootstrapDiagnosticScope(f.raw)});
  assert.equal(r.failureStage,'frame_before');assert.equal(r.failureCondition,'document_changed');assert.equal(r.cleanup,'verified');assert(!f.calls.includes('Runtime.evaluate'));
 }
});
test('bootstrap wire requires sealed factory/nonce, rejects arbitrary fields and private output',async t=>{
 const input=new PassThrough(),output=new PassThrough(),rawInput=new PassThrough(),rawOutput=new PassThrough(),replies=[];let calls=0;
 output.on('data',bytes=>replies.push(JSON.parse(bytes)));
 const server=serveMainBrowserTransport({input,output,connect:async()=>({input:rawInput,output:rawOutput}),diagnoseInstagramBootstrap:async()=>{calls++;return calls===1?result():{...result(),accountId:'PRIVATE'};}});
 t.after(()=>server.close());
 const tick=()=>new Promise(r=>setImmediate(r));const send=q=>input.write(JSON.stringify(q)+'\n');
 send({op:'main_transport_bootstrap',schema:MAIN_TRANSPORT_SCHEMA,nonce:'a'.repeat(48),transport:'inherited-stdio'});await tick();
 send({id:1,op:'main_instagram_bootstrap_diagnostic',nonce:'a'.repeat(48),targetId:'user'});await tick();assert.equal(calls,0);
 send({id:2,op:'main_instagram_bootstrap_diagnostic',nonce:'a'.repeat(48)});await tick();assert.equal(replies.at(-1).ok,true);
 send({id:3,op:'main_instagram_bootstrap_diagnostic',nonce:'b'.repeat(48)});await tick();assert.equal(replies.at(-1).ok,false);assert(!JSON.stringify(replies).includes('PRIVATE'));
});

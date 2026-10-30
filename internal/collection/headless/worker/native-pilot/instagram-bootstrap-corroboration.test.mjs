import test from 'node:test';
import assert from 'node:assert/strict';
import {runInNewContext} from 'node:vm';
import {PassThrough} from 'node:stream';
import {serveMainBrowserTransport,MAIN_TRANSPORT_SCHEMA} from './main-browser-transport-protocol.mjs';
import {runInstagramBootstrapCorroboration,validateInstagramBootstrapCorroboration} from './instagram-bootstrap-corroboration.mjs';
import {createMainBootstrapCorroborationScope} from './main-bootstrap-corroboration-scope.mjs';
import {instagramBootstrapCorroborationExpression} from './instagram-bootstrap-corroboration-expression.mjs';
import {INSTAGRAM_CORROBORATION_POLICY as policy} from './instagram-bootstrap-corroboration-policy.mjs';
const nonce='a'.repeat(48),home='https://www.instagram.com/';
const tuple=id=>JSON.stringify(['PolarisViewer',[],{id},1]);
const htmlFor=id=>`<!doctype html><html><head><script type="application/json">${tuple(id)}</script></head><body></body></html>`;

// Synthetic DOM fixture only; the production path uses the browser's inert
// HTML template parser. The strict JSON scanner/expression run unmodified.
function realm(options={}){
 const calls=[],bodyID=options.bodyID??'123',domID=options.domID??'123';
 const doc=scripts=>({querySelector:selector=>options.login&&selector.includes('password')?{}:options.challenge&&selector.includes('captcha')?{}:null,
  querySelectorAll:()=>scripts.map(textContent=>({textContent}))});
 const actual=doc(options.domScripts??[tuple(domID)]);
 actual.readyState='complete';
 actual.createElement=name=>{assert.equal(name,'template');return {content:doc(options.bodyScripts??[tuple(bodyID)]),set innerHTML(value){if(value)assert.equal(value,htmlFor(bodyID));}};};
 Object.defineProperty(actual,'cookie',{get(){throw Error('whole cookie jar forbidden');}});
 const sandbox={document:actual,location:{origin:'https://www.instagram.com',pathname:'/'},performance,TextEncoder,TextDecoder,Uint8Array,atob,
  cookieStore:options.unsupported?undefined:{async getAll(value){calls.push(value);options.onCookie?.();if(options.cookieFailure)throw Error('private cookie failure');return options.cookies??[{name:'ds_user_id',value:options.cookieID??'123'}];}}};
 return {sandbox,calls,body:Buffer.from(htmlFor(bodyID))};
}
async function expression(options={}){const f=realm(options);return {value:JSON.parse(JSON.stringify(await runInNewContext(instagramBootstrapCorroborationExpression(f.body,nonce),f.sandbox))),calls:f.calls};}

function fixture(options={}){
 const listeners=new Set(),targets=new Set(),sessions=new Set(),calls=[],f=realm(options);
 let committed=false,closed=false,offset=0,resolveNavigation,disposed=false;
 const emit=(method,params)=>{for(const listener of listeners)listener({sessionId:'owned-session',method,params});};
 const response=()=>emit('Network.responseReceived',{requestId:'root-request',frameId:'root-frame',loaderId:'root-loader',type:'Document',response:{url:home,status:200,...options.responseFlags}});
 const extra=()=>emit('Network.responseReceivedExtraInfo',{requestId:'root-request',statusCode:options.extraStatus??200,headers:{'set-cookie':'PRIVATE_UNREAD'}});
 const paused=()=>emit('Fetch.requestPaused',{requestId:'fetch-request',networkId:options.wrongRequest?'foreign':'root-request',frameId:'root-frame',resourceType:'Document',request:{url:home,method:'GET'},responseStatusCode:options.status??200,
  responseHeaders:[{name:'Content-Type',value:'text/html; charset=utf-8'},{name:'Content-Length',value:'1'},{name:'Content-Encoding',value:'gzip'}]});
 const raw={alive:()=>true,privateTarget:id=>targets.has(id),privateSession:id=>sessions.has(id),reserveTarget:id=>targets.add(id),reserveSession:id=>sessions.add(id),releaseTarget:id=>targets.delete(id),releaseSession:id=>sessions.delete(id),
  subscribe(listener){listeners.add(listener);return()=>listeners.delete(listener);},
  async call(method,params,session,timeoutMS){
   calls.push({method,params,session,timeoutMS});
   if(method==='Target.createTarget')return {targetId:'owned-target'};
   if(method==='Target.attachToTarget')return {sessionId:'owned-session'};
   if(method==='Page.getFrameTree')return {frameTree:{frame:{id:'root-frame',loaderId:committed?'root-loader':'initial-loader',url:committed?home:'about:blank'}}};
   if(method==='Page.navigate'){
    queueMicrotask(()=>{
     emit('Network.requestWillBeSent',{requestId:'root-request',frameId:'root-frame',loaderId:'root-loader',type:'Document',request:{url:home,method:'GET'}});
     if(options.order==='response-first')response();
     if(options.order==='pause-first'){paused();extra();}else{extra();paused();}
     if(options.cached)emit('Network.requestServedFromCache',{requestId:'root-request'});
    });
    return new Promise(resolve=>{resolveNavigation=resolve;});
   }
   if(method==='Fetch.takeResponseBodyAsStream')return {stream:'owned-stream'};
   if(method==='IO.read'){
    options.onRead?.();
    if(options.readFailure)throw Error('PRIVATE_BODY_ERROR');
    const source=options.oversize?Buffer.alloc(policy.maxBodyBytes+1,65):f.body;
    const bytes=source.subarray(offset,offset+params.size);offset+=bytes.length;
    return {data:bytes.toString('base64'),base64Encoded:true,eof:offset===source.length};
   }
   if(method==='IO.close'){if(options.closeStreamFails)throw Error('PRIVATE_CLOSE_ERROR');return {};}
   if(method==='Fetch.fulfillRequest'){
    assert.deepEqual(Buffer.from(params.body,'base64'),f.body);
    assert.ok(!params.responseHeaders.some(h=>['content-encoding','transfer-encoding'].includes(h.name.toLowerCase())));
    assert.equal(params.responseHeaders.find(h=>h.name.toLowerCase()==='content-length').value,String(f.body.length));
    committed=true;
    emit('Page.frameNavigated',{frame:{id:'root-frame',loaderId:'root-loader',url:home}});
    emit('Runtime.executionContextCreated',{context:{id:7,uniqueId:'root-context',origin:'https://www.instagram.com',auxData:{isDefault:true,frameId:'root-frame'}}});
    if(options.order!=='response-first')response();
    emit('Network.loadingFinished',{requestId:'root-request'});
    resolveNavigation({frameId:'root-frame',loaderId:'root-loader'});return {};
   }
   if(method==='Runtime.evaluate'){
    let value=await runInNewContext(params.expression,f.sandbox);
    if(options.forgedReply&&params.expression.includes('cookieStore'))value={nonce,reason:'corroborated',scanComplete:false,idMatched:false,cookieMatched:false};
    if(options.changeContext&&params.expression.includes('cookieStore'))emit('Runtime.executionContextsCleared',{});
    return {result:{value:JSON.parse(JSON.stringify(value))}};
   }
   if(method==='Target.closeTarget'){options.onCloseTarget?.();closed=true;resolveNavigation?.({errorText:'closed'});return {success:true};}
   if(method==='Target.getTargets'){
    if(options.cleanupFails)throw Error('PRIVATE_INVENTORY_ERROR');
    return {targetInfos:closed?[{targetId:'user-owned-reader'}]:[{targetId:'owned-target'},{targetId:'user-owned-reader'}]};
   }
   if(['Page.enable','Runtime.enable','Network.enable','Network.setCacheDisabled','Network.setBypassServiceWorker','Fetch.enable'].includes(method))return {};
   throw Error('forbidden operation '+method);
  }};
 return {calls,raw,emit,realm:f,transport:{createBootstrapCorroborationScope:()=>createMainBootstrapCorroborationScope(raw)}};
}

test('combined real adapter and sealed scope corroborate only booleans with response-order permutations',async()=>{
 for(const order of ['extra-first','pause-first','response-first']){
  const f=fixture({order});const result=await runInstagramBootstrapCorroboration(f.transport,{nonce});
  assert.equal(result.reason,'corroborated');assert.equal(result.qualified,false);assert.equal(result.cleanup,'verified');
  assert.equal(result.scanComplete,true);assert.ok(Buffer.byteLength(JSON.stringify(result))<1024);
  assert.deepEqual(f.realm.calls.map(v=>JSON.parse(JSON.stringify(v))),[{name:'ds_user_id'}]);
  assert.ok(!JSON.stringify(result).includes('123'));assert.ok(!f.calls.some(c=>/Cookies|cookie|ResponseBody$/.test(c.method)));
  assert.deepEqual(f.calls.filter(c=>c.method==='Target.closeTarget').map(c=>c.params.targetId),['owned-target']);
 }
});
test('body and DOM identity mismatch reject before any cookie accessor',async()=>{
 const f=fixture({domID:'456'});const result=await runInstagramBootstrapCorroboration(f.transport,{nonce});
 assert.equal(result.reason,'id_mismatch');assert.equal(result.idMatched,false);assert.equal(result.serverFresh,true);assert.equal(f.realm.calls.length,0);
});
test('name-selective cookie access has no fallback and rejects missing/ambiguous/invalid/mismatched cookies',async()=>{
 for(const [options,reason] of [[{unsupported:true},'cookie_unsupported'],[{cookies:[]},'cookie_missing'],[{cookies:[{},{}]},'cookie_ambiguous'],[{cookies:[{name:'sessionid',value:'123'}]},'cookie_invalid'],
  ...[{domain:'foreign.invalid'},{path:'/other'},{secure:false},{partitioned:true}].map(extra=>[{cookies:[{name:'ds_user_id',value:'123',...extra}]},'cookie_invalid']),
  [{cookieID:'456'},'cookie_mismatch'],[{cookieFailure:true},'cookie_failed']]){
  const {value}=await expression(options);assert.equal(value.reason,reason);assert.equal(value.cookieMatched,false);
 }
});
test('actual strict scanner rejects malformed, duplicate, conflicting and incomplete candidates',async()=>{
 for(const scripts of [[tuple('123'),tuple('123')],[tuple('123'),tuple('456')],['["PolarisViewer",[],{},1]'],['["PolarisViewer"']]){
  const {value,calls}=await expression({bodyScripts:scripts});assert.equal(value.reason,'bootstrap_rejected');assert.equal(value.idMatched,false);assert.equal(calls.length,0);
 }
});
test('body cap is enforced despite lying Content-Length and no replay occurs',async()=>{
 const f=fixture({oversize:true});const result=await runInstagramBootstrapCorroboration(f.transport,{nonce});
 assert.equal(result.reason,'body_bounds');assert.equal(result.bodyBound,false);assert.equal(result.cleanup,'verified');
 assert.ok(!f.calls.some(c=>c.method==='Fetch.fulfillRequest'));assert.ok(f.calls.filter(c=>c.method==='IO.read').every(c=>c.params.size<=policy.chunkBytes));
});
test('root mismatch, rejected status and cached/malformed cache delivery fail closed',async()=>{
 for(const [options,reason]of [[{wrongRequest:true},'wire_unmatched'],[{status:403},'http_rejected'],[{extraStatus:304},'delivery_unproven'],[{cached:true},'delivery_unproven'],[{responseFlags:{fromServiceWorker:true}},'delivery_unproven'],[{responseFlags:{fromDiskCache:'false'}},'delivery_unproven']]){
  const result=await runInstagramBootstrapCorroboration(fixture(options).transport,{nonce});assert.equal(result.reason,reason);assert.equal(result.qualified,false);
 }
});
test('first cause survives stream and target cleanup failures; context change cannot corroborate',async()=>{
 const failed=await runInstagramBootstrapCorroboration(fixture({readFailure:true,closeStreamFails:true,cleanupFails:true}).transport,{nonce});
 assert.equal(failed.reason,'body_read');assert.equal(failed.cleanup,'uncertain');
 const changed=await runInstagramBootstrapCorroboration(fixture({changeContext:true}).transport,{nonce});
 assert.equal(changed.reason,'document_changed');assert.equal(changed.documentBound,false);
 const cleanup=await runInstagramBootstrapCorroboration(fixture({cleanupFails:true}).transport,{nonce});
 assert.equal(cleanup.reason,'cleanup_failed');assert.equal(cleanup.cleanup,'uncertain');
 const stream=await runInstagramBootstrapCorroboration(fixture({closeStreamFails:true}).transport,{nonce});
 assert.equal(stream.reason,'cleanup_failed');assert.equal(stream.cleanup,'uncertain');
 const forged=await runInstagramBootstrapCorroboration(fixture({forgedReply:true}).transport,{nonce});
 assert.equal(forged.reason,'projection_invalid');assert.equal(forged.cleanup,'verified');
});
test('invalid requests and missing factory do not create a scope',async()=>{
 let calls=0;const transport={createBootstrapCorroborationScope(){calls++;throw Error();}};
 assert.equal((await runInstagramBootstrapCorroboration(transport,{nonce,extra:true})).reason,'invalid_request');assert.equal(calls,0);
 assert.equal((await runInstagramBootstrapCorroboration({}, {nonce})).reason,'scope_unavailable');
});
test('closed projection rejects values, extra keys, forged authority and partial success',()=>{
 const base={schema:'aku.instagram-bootstrap-corroboration.v1',reason:'bootstrap_rejected',bodyBound:false,documentBound:false,serverFresh:false,idMatched:false,cookieMatched:false,scanComplete:false,qualified:false,cleanup:'verified'};
 assert.doesNotThrow(()=>validateInstagramBootstrapCorroboration(base));
 for(const altered of [{...base,id:'123'},{...base,qualified:true},{...base,reason:'corroborated'},{...base,cookieMatched:'123'}])assert.throws(()=>validateInstagramBootstrapCorroboration(altered));
});
test('separate scope rejects arbitrary evaluation, jar APIs, foreign stream and resource ownership',async()=>{
 const f=fixture(),scope=createMainBootstrapCorroborationScope(f.raw);
 await scope.call('Target.createTarget',{url:'about:blank',background:true,forTab:false},'',1000);
 await scope.call('Target.attachToTarget',{targetId:'owned-target',flatten:true},'',1000);
 for(const [method,params]of [['Network.getCookies',{}],['Runtime.evaluate',{expression:'document.cookie'}],['Network.getResponseBody',{requestId:'root-request'}],['IO.read',{handle:'foreign',size:1}],['Fetch.fulfillRequest',{requestId:'foreign'}]])await assert.rejects(scope.call(method,params,'owned-session',1000));
 await assert.rejects(scope.call('Target.closeTarget',{targetId:'user-owned-reader'},'',1000));scope.dispose();
});

test('stale delivery cannot acquire a fresh timestamp from successful page evaluation',async t=>{
 let now=100000;
 const f=fixture({onCookie:()=>{now+=policy.freshMS+1;}});
 const result=await runInstagramBootstrapCorroboration(f.transport,{nonce},{now:()=>now});
 assert.equal(result.reason,'stale_delivery');assert.equal(result.serverFresh,false);assert.equal(result.qualified,false);assert.equal(result.cleanup,'verified');
});
test('late stream allocation stays uncertain until the exact late handle closes',async()=>{
 const f=fixture(),original=f.raw.call;let late;
 f.raw.call=async(method,params,session,ms,onLate)=>{if(method==='Fetch.takeResponseBodyAsStream'){late=onLate;throw Error('timeout');}return original(method,params,session,ms);};
 const scope=createMainBootstrapCorroborationScope(f.raw);
 await scope.call('Target.createTarget',{url:'about:blank',background:true,forTab:false},'',1000);
 await scope.call('Target.attachToTarget',{targetId:'owned-target',flatten:true},'',1000);
 await scope.call('Fetch.enable',{patterns:[{urlPattern:home,resourceType:'Document',requestStage:'Response'}]},'owned-session',1000);
 f.emit('Fetch.requestPaused',{requestId:'fetch-request'});
 await assert.rejects(scope.call('Fetch.takeResponseBodyAsStream',{requestId:'fetch-request'},'owned-session',1000));
 await scope.call('Target.closeTarget',{targetId:'owned-target'},'',1000);
 await scope.call('Target.getTargets',{},'',1000);scope.releaseSession('owned-session');scope.releaseTarget('owned-target');
 assert.equal(scope.cleanupVerified(),false);await late({stream:'late-owned-stream'});assert.equal(scope.cleanupVerified(),true);
 assert.deepEqual(f.calls.filter(c=>c.method==='IO.close').map(c=>c.params.handle),['late-owned-stream']);scope.dispose();
});
test('maximum body expression fits existing outbound transport without raising its limit',()=>{
 const body=Buffer.alloc(policy.maxBodyBytes);
 const expr=instagramBootstrapCorroborationExpression(body,nonce);assert.ok(Buffer.byteLength(expr)<7*1024*1024);
 assert.throws(()=>instagramBootstrapCorroborationExpression(Buffer.alloc(policy.maxBodyBytes+1),nonce));body.fill(0);
});
test('acquisition expiry leaves cleanup and drain time inside the unchanged Go deadline',async t=>{
 const start=100000;let now=start;
 const f=fixture({onRead:()=>{now=start+policy.acquisitionMS;},onCloseTarget:()=>{now+=1000;}});
 const result=await runInstagramBootstrapCorroboration(f.transport,{nonce},{now:()=>now});
 assert.equal(result.reason,'body_deadline');assert.equal(result.cleanup,'verified');
 assert.ok(now-start<policy.totalMS);assert.ok(policy.acquisitionMS+1000+policy.cleanupMS<=policy.totalMS-2000);
 assert.ok(f.calls.filter(c=>c.method==='Target.getTargets').every(c=>c.timeoutMS<=policy.cleanupMS));
});
test('new wire operation is sealed, factory-gated, exclusive and rejects private output',async t=>{
 const tick=()=>new Promise(resolve=>setImmediate(resolve));
 function control(factory){
  const input=new PassThrough(),output=new PassThrough(),cdpInput=new PassThrough(),cdpOutput=new PassThrough(),replies=[];
  output.on('data',wire=>replies.push(JSON.parse(wire.toString())));
  const service=serveMainBrowserTransport({input,output,connect:async()=>({input:cdpInput,output:cdpOutput}),diagnoseInstagramBootstrapCorroboration:factory});
  t.after(()=>{service.close();input.destroy();output.destroy();cdpInput.destroy();cdpOutput.destroy();});
  const send=q=>input.write(JSON.stringify(q)+'\n');send({op:'main_transport_bootstrap',schema:MAIN_TRANSPORT_SCHEMA,nonce,transport:'inherited-stdio'});
  return {send,replies};
 }
 let calls=0,resolve;
 const f=control(async(_transport,request)=>{calls++;assert.deepEqual(request,{nonce});return new Promise(r=>{resolve=r;});});await tick();
 f.send({id:1,op:'main_instagram_bootstrap_corroboration',nonce,url:home});await tick();assert.equal(calls,0);
 f.send({id:2,op:'main_instagram_bootstrap_corroboration',nonce});await tick();assert.equal(calls,1);
 f.send({id:3,op:'main_instagram_bootstrap_corroboration',nonce});await tick();assert.equal(f.replies.find(v=>!v.event&&v.id===3).ok,false);
 resolve({schema:'aku.instagram-bootstrap-corroboration.v1',reason:'bootstrap_rejected',bodyBound:false,documentBound:false,serverFresh:false,idMatched:false,cookieMatched:false,scanComplete:false,qualified:false,cleanup:'verified',privateID:'SECRET'});await tick();
 assert.equal(f.replies.find(v=>!v.event&&v.id===2).ok,false);assert.ok(!JSON.stringify(f.replies).includes('SECRET'));
 const missing=control();await tick();missing.send({id:1,op:'main_instagram_bootstrap_corroboration',nonce});await tick();assert.equal(missing.replies.find(v=>!v.event&&v.id===1).ok,false);
});

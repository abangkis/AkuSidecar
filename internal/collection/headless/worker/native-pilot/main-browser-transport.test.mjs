import test from 'node:test';
import assert from 'node:assert/strict';
import {PassThrough} from 'node:stream';
import {mkdir,mkdtemp,writeFile,rm} from 'node:fs/promises';
import {resolve,join,dirname} from 'node:path';
import {fileURLToPath} from 'node:url';
import {createApplicationDailyOwner} from '../../../../../scripts/auth-session-fixture/x-native-metadata-probe-driver.mjs';
import {createMainCDP,createMainPublicFacade,serveMainBrowserTransport,MAIN_TRANSPORT_SCHEMA,mainOwnerFailure} from './main-browser-transport-protocol.mjs';
import {createBorrowedDailyControl} from './main-browser-transport.mjs';
import {ownerErrorReply} from './owner-protocol.mjs';
import {manualCaptureFailure} from '../../../../../scripts/auth-session-fixture/x-native-manual-capture.mjs';

const tick=()=>new Promise(resolve=>setImmediate(resolve));
function cdpFixture(t){
  const input=new PassThrough(),output=new PassThrough(),requests=[];
  output.on('data',bytes=>requests.push(JSON.parse(bytes.subarray(0,-1).toString())));
  const transport=createMainCDP({input,output});t.after(()=>transport.close());
  return{transport,requests,input,answer:(id,result)=>{const sessionId=requests.find(q=>q.id===id)?.sessionId;input.write(JSON.stringify({id,result,...(sessionId?{sessionId}:{})})+'\0');}};
}
test('raw replies and header events are Node-local; public structural responses project fields',async t=>{
  const f=cdpFixture(t),events=[];f.transport.subscribe(event=>events.push(event));
  f.input.write(JSON.stringify({method:'Network.requestWillBeSentExtraInfo',params:{headers:{Cookie:'synthetic-secret'}}})+'\0');
  assert.equal(events[0].params.headers.Cookie,'synthetic-secret');
  const call=createMainPublicFacade(f.transport)({method:'Browser.getVersion',params:{},sessionId:'',timeoutMS:1000});
  f.answer(f.requests[0].id,{product:'Chrome/synthetic',headers:{Cookie:'synthetic-secret'},cookies:['synthetic-secret']});
  assert.deepEqual(await call,{product:'Chrome/synthetic'});
});
test('public caller cannot request cookies, arbitrary evaluator or browser shutdown',async t=>{
  const f=cdpFixture(t),call=createMainPublicFacade(f.transport);
  for(const method of ['Network.getCookies','Network.getResponseBody','Runtime.evaluate','Browser.close','Target.setAutoAttach'])
    await assert.rejects(call({method,params:{},sessionId:'',timeoutMS:1000}),/main_transport_rejected/);
  assert.equal(f.requests.length,0);
});
test('sealed recipe requires exact expression and projects its result inside Node',async t=>{
  const f=cdpFixture(t),expression='Boolean(document.hasFocus())';
  const call=createMainPublicFacade(f.transport,{recipes:[{expression,project:r=>({result:{type:'boolean',value:r.result?.value===true}})}]});
  const attached=call({method:'Target.attachToBrowserTarget',params:{},sessionId:'',timeoutMS:1000});f.answer(f.requests.at(-1).id,{sessionId:'s'});await attached;
  await assert.rejects(call({method:'Runtime.evaluate',params:{expression:expression+' ',returnByValue:true},sessionId:'s',timeoutMS:1000}));
  const result=call({method:'Runtime.evaluate',params:{expression,returnByValue:true},sessionId:'s',timeoutMS:1000});
  f.answer(f.requests.at(-1).id,{result:{value:true,headers:{Cookie:'synthetic-secret'}}});
  assert.deepEqual(await result,{result:{type:'boolean',value:true}});
});

test('public target inventory excludes private X, unknown sessions fail, navigation errors stay closed',async t=>{
  const f=cdpFixture(t),call=createMainPublicFacade(f.transport);
  f.transport.reserveTarget('private-x');
  const inventory=call({method:'Target.getTargets',params:{},sessionId:'',timeoutMS:1000});
  f.answer(f.requests.at(-1).id,{targetInfos:[{targetId:'main-host',type:'page',url:'http://127.0.0.1:11122/'},{targetId:'private-x',url:'https://x.com/home'}]});
  assert.deepEqual((await inventory).targetInfos.map(t=>t.targetId),['main-host']);
  for(const targetId of ['private-x','unknown','main-host'])await assert.rejects(call({method:'Target.closeTarget',params:{targetId},sessionId:'',timeoutMS:1000}));
  const attached=call({method:'Target.attachToTarget',params:{targetId:'main-host',flatten:true},sessionId:'',timeoutMS:1000});
  f.answer(f.requests.at(-1).id,{sessionId:'public-session'});await attached;
  await assert.rejects(call({method:'Page.navigate',params:{url:'https://example.com/'},sessionId:'private-session',timeoutMS:1000}));
  const navigated=call({method:'Page.navigate',params:{url:'https://example.com/'},sessionId:'public-session',timeoutMS:1000});
  f.answer(f.requests.at(-1).id,{frameId:'f',errorText:'synthetic-secret'});
  assert.deepEqual(await navigated,{frameId:'f',errorText:'navigation_rejected'});
});
test('private CDP failure rejects pending work without raw exception text',async t=>{
  const f=cdpFixture(t),p=f.transport.call('Network.getCookies',{},'s',1000);
  f.input.write(JSON.stringify({id:f.requests[0].id,error:{message:'synthetic-secret'}})+'\0');
  await assert.rejects(p,error=>error.message==='main_transport_rejected');
});
function controlFixture(t,createX){
  const input=new PassThrough(),output=new PassThrough(),rawIn=new PassThrough(),rawOut=new PassThrough(),replies=[];
  output.on('data',b=>replies.push(JSON.parse(b.toString())));
  const server=serveMainBrowserTransport({input,output,createX,connect:async()=>({input:rawIn,output:rawOut})});
  t.after(()=>server.close());
  const send=q=>input.write(JSON.stringify(q)+'\n');
  send({op:'main_transport_bootstrap',schema:MAIN_TRANSPORT_SCHEMA,nonce:'a'.repeat(48),transport:'inherited-stdio'});
  return{input,replies,rawOut,rawIn,send};
}
test('control protocol rejects replay and never forwards private CDP events',async t=>{
  const f=controlFixture(t);await tick();assert.equal(f.replies[0].ready,true);
  f.rawIn.write(JSON.stringify({method:'Network.responseReceivedExtraInfo',params:{headers:{'set-cookie':'synthetic-secret'}}})+'\0');
  f.send({id:1,op:'main_call',method:'Browser.close',params:{},sessionId:'',timeoutMS:1000});await tick();
  f.send({id:1,op:'main_call',method:'Browser.getVersion',params:{},sessionId:'',timeoutMS:1000});await tick();
  assert.equal(f.replies.length,3);assert.equal(f.replies[2].ok,false);
  assert.ok(!JSON.stringify(f.replies).includes('synthetic-secret'));
});
test('seed phase excludes public operations while capture can coexist with post preparation',async t=>{
  let release;const owner={execute:async op=>op==='daily_batch_seed'?new Promise(r=>{release=r;}):{phase:op},close:async()=>{}};
  const f=controlFixture(t,async()=>owner);await tick();
  f.send({id:1,op:'main_x',operation:'daily_owner_bootstrap',params:{}});await tick();
  f.send({id:2,op:'main_x',operation:'daily_batch_seed',params:{}});await tick();
  f.send({id:3,op:'main_call',method:'Browser.getVersion',params:{},sessionId:'',timeoutMS:1000});await tick();
  assert.equal(f.replies.find(r=>r.id===3).ok,false);release({seeded:true});await tick();
  f.send({id:4,op:'main_x',operation:'manual_prepare_post',params:{}});await tick();
  assert.equal(f.replies.find(r=>r.id===4).ok,true);
});
test('main X preserves normalized first-phase diagnostics but never arbitrary exception properties',async t=>{
  const normalized=ownerErrorReply({id:1,op:'manual_capture'},manualCaptureFailure(new Error('synthetic-secret'),'navigation','navigation_failed'));
  const f=controlFixture(t,async()=>({close:async()=>{},execute:async op=>{
    if(op==='manual_capture')throw mainOwnerFailure(normalized);
    throw Object.assign(new Error('synthetic-secret'),{code:'synthetic_secret',diagnostic:{headers:'synthetic-secret'}});
  }}));await tick();
  f.send({id:1,op:'main_x',operation:'daily_owner_bootstrap',params:{}});await tick();
  f.send({id:2,op:'main_x',operation:'manual_capture',params:{}});await tick();
  const result=f.replies.find(r=>r.id===2);
  assert.equal(result.error.code,normalized.code);assert.deepEqual(result.error.diagnostic,normalized.diagnostic);
  f.send({id:3,op:'main_x',operation:'daily_reader_check',params:{}});await tick();
  assert.deepEqual(f.replies.find(r=>r.id===3).error,{code:'main_transport_rejected'});
  assert.ok(!JSON.stringify(f.replies).includes('synthetic-secret'));
});
test('borrowed daily control requires process consent and keeps generation/seed replay checks',async()=>{
  const params={chromePath:'C:\\synthetic\\chrome.exe',readerProfile:'C:\\synthetic\\profile',readerPID:123,
    profileDirectory:'Profile 2',acknowledgeLimitations:true,allowProcessSessionDerivation:true};
  const transport={alive:()=>true};let scope,closed=0;
  const createOwner=async s=>{scope=s;return{initialize:async()=>({owner:1,readerPID:123}),begin:async()=>({owner:2}),
    seed:async()=>({seeded:true}),end:async()=>({closed:true}),close:async()=>{closed++;}};};
  await assert.rejects(createBorrowedDailyControl({...params,allowProcessSessionDerivation:false},transport,{createOwner}));
  const c=await createBorrowedDailyControl(params,transport,{createOwner,loadAssets:async()=>[]});
  assert.equal(scope.borrowedReader.pid,123);assert.equal(scope.borrowedReader.profileDirectory,'Profile 2');
  await c.execute('daily_reader_initialize',{});
  await c.execute('daily_batch_begin',{generation:1,collectorProfile:'C:\\synthetic\\collector'});
  await c.execute('daily_batch_seed',{generation:1,owner:2});
  await assert.rejects(c.execute('daily_batch_seed',{generation:1,owner:2}));
  await c.execute('daily_owner_close',{});assert.ok(closed>=1);
});

test('actual borrowed factory attaches only its new target and never launches or closes main Chrome',async t=>{
  const project=resolve(dirname(fileURLToPath(import.meta.url)),'../../../../..');
  const parent=join(project,'build');await mkdir(parent,{recursive:true});
  const root=await mkdtemp(join(parent,'test-main-borrowed-'));
  t.after(async()=>{assert.equal(dirname(root),parent);await rm(root,{recursive:true,force:true});});
  const chromePath=join(root,'not-an-executable.exe'),readerProfile=join(root,'synthetic-profile');
  await writeFile(chromePath,'synthetic; never execute');await mkdir(readerProfile);
  const calls=[];let unsubscribed=false;
  const transport={alive:()=>true,subscribe:()=>()=>{unsubscribed=true;},call:async(method,params)=>{
    calls.push({method,params});
    switch(method){case 'Target.createTarget':return{targetId:'synthetic-owned-target'};
      case 'Target.attachToTarget':return{sessionId:'synthetic-owned-session'};
      case 'Runtime.evaluate':return{result:{type:'boolean',value:true}};
      case 'Target.closeTarget':return{success:true};
      default:return{};}
  }};
  const owner=await createApplicationDailyOwner({chromePath,readerProfile,loadAssets:async()=>[],
    borrowedReader:{pid:12345,profileDirectory:'Profile 2',transport}});
  const result=await owner.initialize();assert.equal(result.readerPID,12345);
  await owner.close();
  assert.equal(calls.filter(c=>c.method==='Target.createTarget').length,1);
  assert.equal(calls.filter(c=>c.method==='Target.closeTarget').length,1);
  assert.equal(calls.find(c=>c.method==='Target.closeTarget').params.targetId,'synthetic-owned-target');
  assert.ok(!calls.some(c=>['Browser.close','Target.getTargets'].includes(c.method)));
  assert.equal(unsubscribed,true);
});

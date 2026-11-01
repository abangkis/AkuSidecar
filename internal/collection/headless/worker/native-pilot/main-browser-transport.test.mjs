import test from 'node:test';
import assert from 'node:assert/strict';
import {PassThrough} from 'node:stream';
import {mkdir,mkdtemp,writeFile,rm} from 'node:fs/promises';
import {resolve,join,dirname} from 'node:path';
import {fileURLToPath} from 'node:url';
import {createApplicationDailyOwner} from './x-application-owner-factories.mjs';
import {createMainCDP,createMainPublicFacade,serveMainBrowserTransport,MAIN_TRANSPORT_SCHEMA,mainOwnerFailure,mainStartupProgress,mainStartupProgressForOwnerOperation} from './main-browser-transport-protocol.mjs';
import {createBorrowedDailyControl,createBorrowedInstagramControl} from './main-browser-transport.mjs';
import {ownerErrorReply} from './owner-protocol.mjs';
import {manualCaptureFailure} from './shared/x-native-manual-capture.mjs';
import {nativeSourceFailure,annotateNativeSourceCleanup} from './native-source-kernel.mjs';
import {INSTAGRAM_HTTP_REJECTIONS} from './instagram-native-session.mjs';

const tick=()=>new Promise(resolve=>setImmediate(resolve));
test('startup progress preserves bounded v10 phase ages and legacy v9 without raw clock data',()=>{
  const parts=Array(28).fill('0');parts[23]='4';parts.splice(24,4,'11201','101','11001','101');
  const value={operation:'identity',elapsedMS:100,cause:'manual_initial_v10_'+parts.join('_')};
  assert.deepEqual(mainStartupProgress(value),value);
  const legacy={...value,cause:'manual_initial_v9_'+parts.slice(0,23).join('_')};
  assert.deepEqual(mainStartupProgress(legacy),legacy);
  for(const age of ['-1','60002','00','1700000000000','private-token']){
    const bad=[...parts];bad[24]=age;
    assert.equal(mainStartupProgress({...value,cause:'manual_initial_v10_'+bad.join('_')}),null);
  }
  assert.equal(mainStartupProgress({...value,cause:value.cause+'_0'}),null);
});
async function waitFor(predicate){for(let i=0;i<100;i++){if(predicate())return;await tick();}assert.fail('synthetic transport did not settle');}
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
function controlFixture(t,createX,sourceFactories={},diagnostics={}){
  const input=new PassThrough(),output=new PassThrough(),rawIn=new PassThrough(),rawOut=new PassThrough(),replies=[];
  output.on('data',b=>replies.push(JSON.parse(b.toString())));
  const server=serveMainBrowserTransport({input,output,createX,sourceFactories,...diagnostics,connect:async()=>({input:rawIn,output:rawOut})});
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
test('startup progress accepts only the closed bounded diagnostic DTO',()=>{
  const valid={operation:'settings_wait',elapsedMS:12,cause:'manual_initial_reader_settings_source_unavailable_primary',settingsWait:'response_pending'};
  assert.deepEqual(mainStartupProgress(valid),valid);
  assert.deepEqual(mainStartupProgressForOwnerOperation('manual_acquire',valid),valid);
  assert.equal(mainStartupProgressForOwnerOperation('manual_capture',valid),null);
  for(const value of [
    'synthetic-secret',
    {...valid,operation:'synthetic-secret'},
    {...valid,cause:'synthetic-secret'},
    {...valid,settingsWait:'synthetic-secret'},
    {...valid,extra:'synthetic-secret'},
    {...valid,elapsedMS:60001},
    {operation:'settings_wait',elapsedMS:12},
    {operation:'settings_wait',cause:''},
    {...valid,cause:null},
    {...valid,settingsWait:null},
  ])assert.equal(mainStartupProgress(value),null);
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

test('borrowed measured startup RPC progress stays correlated, capped and private',async t=>{
  const params={chromePath:'C:\\synthetic\\chrome.exe',readerProfile:'C:\\synthetic\\profile',readerPID:123,
    profileDirectory:'Profile 2',acknowledgeLimitations:true,allowProcessSessionDerivation:true};
  const expectedProgress={manual_acquire:['acquire_rpc','identity','rendezvous','settings_wait','cleanup','root_restore','complete'],
    manual_inspect:['acquire_rpc','inspect_rpc','complete'],manual_inventory:['acquire_rpc','inventory_rpc','complete']};
  let releaseAcquire,lateProgress;
  const f=controlFixture(t,async(p,transport)=>createBorrowedDailyControl(p,transport,{
    loadAssets:async()=>[],
    createOwner:async scope=>({
      initialize:async()=>({owner:1,readerPID:123}),
      begin:async()=>({owner:2}),
      seed:async()=>({seeded:true}),
      check:async()=>({checked:true}),
      execute:async q=>{
        const stages=expectedProgress[q.op];assert.ok(stages);
        if(q.op==='manual_acquire'){
          const valid={operation:'acquire_rpc',elapsedMS:0,cause:''};
          scope.emitProgress('synthetic-secret');
          scope.emitProgress({event:'manual_startup_progress',id:q.id,result:{...valid,cause:'synthetic-secret'}});
          scope.emitProgress({event:'manual_startup_progress',id:q.id,result:{...valid,extra:'synthetic-secret'}});
          scope.emitProgress({event:'manual_startup_progress',id:q.id,result:valid,secret:'synthetic-secret'});
          scope.emitProgress({event:'unknown_private_event',id:q.id,result:'synthetic-secret'});
          for(let i=0;i<40;i++){
            const operation=stages[i%stages.length],result={operation,elapsedMS:i,cause:''};
            if(operation==='settings_wait')result.settingsWait='response_pending';
            scope.emitProgress({event:'manual_startup_progress',id:q.id,result});
          }
          lateProgress=()=>scope.emitProgress({event:'manual_startup_progress',id:q.id,result:{...valid,elapsedMS:40}});
          await new Promise(resolve=>{releaseAcquire=resolve;});
          setImmediate(lateProgress);
        }else{
          for(let i=0;i<3;i++)scope.emitProgress({event:'manual_startup_progress',id:q.id,result:{operation:stages[i],elapsedMS:i,cause:''}});
        }
        return{executed:q.op};
      },
      end:async()=>({closed:true}),post:async()=>({ok:true}),close:async()=>{},
    }),
  }));
  await waitFor(()=>f.replies.some(r=>r.ready===true));
  f.send({id:1,op:'main_x',operation:'daily_owner_bootstrap',params});
  await waitFor(()=>f.replies.some(r=>r.id===1));
  f.send({id:2,op:'main_x',operation:'daily_reader_initialize',params:{}});
  await waitFor(()=>f.replies.some(r=>r.id===2));
  f.send({id:3,op:'main_x',operation:'daily_batch_begin',params:{generation:1,collectorProfile:'C:\\synthetic\\collector'}});
  await waitFor(()=>f.replies.some(r=>r.id===3));
  f.send({id:4,op:'main_x',operation:'daily_batch_seed',params:{generation:1,owner:2}});
  await waitFor(()=>f.replies.some(r=>r.id===4));
  const request={challenge:'synthetic-challenge',role:'reader',ownerId:'synthetic-owner',profileId:'synthetic-profile',
    contextId:'synthetic-context',targetId:'synthetic-target',frameId:'synthetic-frame',documentId:'synthetic-document'};
  f.send({id:5,op:'main_x',operation:'manual_acquire',params:{generation:1,owner:2,role:'reader',handle:'synthetic-handle',request,startupBudgetMS:1000}});
  await waitFor(()=>typeof releaseAcquire==='function');
  f.send({id:6,op:'main_x',operation:'daily_reader_check',params:{}});
  await waitFor(()=>f.replies.some(r=>r.id===6));
  const progressEvents=f.replies.filter(r=>r.event==='main_x_progress');
  const acquire=progressEvents.filter(r=>r.id===5);
  assert.equal(acquire.length,32);
  assert.ok(acquire.every(r=>expectedProgress.manual_acquire.includes(r.result.operation)&&r.result.elapsedMS>=0&&r.result.elapsedMS<40&&r.result.cause===''));
  assert.ok(acquire.some(r=>r.result.operation==='settings_wait'&&r.result.settingsWait==='response_pending'));
  assert.ok(expectedProgress.manual_acquire.every(operation=>acquire.some(r=>r.result.operation===operation)));
  assert.ok(f.replies.some(r=>r.id===6&&r.ok&&r.result.checked===true));
  assert.ok(!JSON.stringify(f.replies).includes('synthetic-secret'));
  releaseAcquire();
  await waitFor(()=>f.replies.some(r=>r.id===5&&Object.hasOwn(r,'ok')));
  await tick();
  assert.equal(f.replies.filter(r=>r.event==='main_x_progress').length,32);
  assert.ok(f.replies.find(r=>r.id===5&&Object.hasOwn(r,'ok'))?.ok,JSON.stringify(f.replies));
  for(const [id,operation] of [[7,'manual_inspect'],[8,'manual_inventory']]){
    f.send({id,op:'main_x',operation,params:{generation:1,owner:2,role:'reader'}});
    await waitFor(()=>f.replies.some(r=>r.id===id&&Object.hasOwn(r,'ok')));
    const entries=f.replies.filter(r=>r.event==='main_x_progress'&&r.id===id);
    assert.equal(entries.length,3);
    assert.ok(entries.every((r,i)=>r.result.operation===expectedProgress[operation][i]&&r.result.elapsedMS===i));
  }
  assert.ok(!f.replies.some(r=>r.event==='main_x_progress'&&[3,4,6].includes(r.id)));
  assert.ok(!JSON.stringify(f.replies).includes('synthetic-secret'));
});

test('actual borrowed factory initializes process ownership without creating Home or closing main Chrome',async t=>{
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
  assert.deepEqual(calls,[]);
  assert.equal(unsubscribed,true);
});

test('sealed IG lane is source-bound and preserves the existing X lane',async t=>{
  const calls=[];
  const f=controlFixture(t,async()=>({execute:async()=>({legacyX:true}),close:async()=>{}}),{
    instagram:async()=>({execute:async(op,params)=>{calls.push([op,params]);return{source:'instagram'};},close:async()=>{}}),
  });await tick();
  f.send({id:1,op:'main_source',source:'instagram',operation:'source_bootstrap',params:{}});await tick();
  f.send({id:2,op:'main_source',source:'instagram',operation:'source_initialize',params:{}});await tick();
  assert.equal(f.replies.find(r=>r.id===2).result.source,'instagram');
  f.send({id:3,op:'main_source',source:'x',operation:'source_bootstrap',params:{}});await tick();
  assert.equal(f.replies.find(r=>r.id===3).ok,false);
  f.send({id:4,op:'main_x',operation:'daily_owner_bootstrap',params:{}});await tick();
  f.send({id:5,op:'main_x',operation:'daily_reader_check',params:{}});await tick();
  assert.equal(f.replies.find(r=>r.id===5).result.legacyX,true);
  assert.equal(calls.length,1);
});

test('source seed excludes public and X mutation, then ordinary reader calls resume',async t=>{
  let release;
  const f=controlFixture(t,undefined,{instagram:async()=>({
    execute:async op=>op==='source_seed'?new Promise(done=>{release=done;}):{},close:async()=>{},
  })});await tick();
  f.send({id:1,op:'main_source',source:'instagram',operation:'source_bootstrap',params:{}});await tick();
  f.send({id:2,op:'main_source',source:'instagram',operation:'source_seed',params:{}});await tick();
  f.send({id:3,op:'main_call',method:'Browser.getVersion',params:{},sessionId:'',timeoutMS:1000});await tick();
  f.send({id:4,op:'main_x',operation:'daily_batch_begin',params:{}});await tick();
  assert.equal(f.replies.find(r=>r.id===3).ok,false);assert.equal(f.replies.find(r=>r.id===4).ok,false);
  release({seeded:true});await tick();
  f.send({id:5,op:'main_call',method:'Browser.getVersion',params:{},sessionId:'',timeoutMS:1000});await tick();
  f.rawIn.write(JSON.stringify({id:1,result:{product:'Chrome/synthetic'}})+'\0');await tick();
  assert.equal(f.replies.find(r=>r.id===5).result.product,'Chrome/synthetic');
});

test('source errors forward only private diagnostic categories, never arbitrary exception fields',async t=>{
  const f=controlFixture(t,undefined,{instagram:async()=>({execute:async op=>{
    if(op==='source_check')throw nativeSourceFailure('evidence','collector_identity_http_forbidden');
    const error=new Error('private-token');Object.defineProperty(error,'code',{get(){throw Error('getter invoked');}});throw error;
  },close:async()=>{}})});await tick();
  f.send({id:1,op:'main_source',source:'instagram',operation:'source_bootstrap',params:{}});await tick();
  f.send({id:2,op:'main_source',source:'instagram',operation:'source_check',params:{}});await tick();
  assert.equal(f.replies.find(r=>r.id===2).error.code,'native_source_evidence_collector_identity_http_forbidden');
  f.send({id:3,op:'main_source',source:'instagram',operation:'source_proof',params:{}});await tick();
  assert.equal(f.replies.find(r=>r.id===3).error.code,'main_transport_rejected');
  assert.ok(!JSON.stringify(f.replies).includes('private-token'));
});

test('explicit source retry is source-local and preserves ordinary and X transport', async t => {
  let finishRetry, sourceClosed = false;
  const operations = [];
  const f = controlFixture(t, async () => ({execute: async () => ({legacyX: true}), close: async () => {}}), {
    instagram: async () => ({execute: async (operation, params) => {
      operations.push([operation, params]);
      return new Promise(resolve => { finishRetry = resolve; });
    }, close: async () => { sourceClosed = true; }}),
    x: async () => ({execute: async () => { assert.fail('source_retry must be IG-only'); }, close: async () => {}}),
  }); await tick();
  const send = async (id, source, operation, params = {}) => { f.send({id, op: 'main_source', source, operation, params}); await tick(); };
  await send(1, 'instagram', 'source_bootstrap');
  await send(2, 'x', 'source_bootstrap');
  await send(3, 'x', 'source_retry');
  await send(4, 'instagram', 'source_retry', {diagnostic: {reason: 'document_changed', cleanup: 'verified'}});
  assert.equal(f.replies.find(r => r.id === 3).ok, false);
  assert.equal(f.replies.find(r => r.id === 4).ok, false);
  assert.equal(operations.length, 0);
  await send(5, 'instagram', 'source_retry');
  f.send({id: 6, op: 'main_call', method: 'Browser.getVersion', params: {}, sessionId: '', timeoutMS: 1000}); await tick();
  f.send({id: 7, op: 'main_x', operation: 'daily_owner_bootstrap', params: {}}); await tick();
  await send(8, 'instagram', 'source_retry');
  assert.equal(f.replies.find(r => r.id === 7).ok, false);
  assert.equal(f.replies.find(r => r.id === 8).error.code, 'native_source_busy');
  f.rawIn.write(JSON.stringify({id: 1, result: {product: 'Chrome/synthetic'}}) + '\0'); await tick();
  assert.equal(f.replies.find(r => r.id === 6).result.product, 'Chrome/synthetic');
  finishRetry({rearmed: true, retryCount: 1, retryLimit: 2}); await tick();
  assert.deepEqual(f.replies.find(r => r.id === 5).result, {rearmed: true, retryCount: 1, retryLimit: 2});
  f.send({id: 9, op: 'main_x', operation: 'daily_owner_bootstrap', params: {}}); await tick();
  f.send({id: 10, op: 'main_x', operation: 'daily_reader_check', params: {}}); await tick();
  assert.deepEqual(f.replies.find(r => r.id === 10).result, {legacyX: true});
  f.send({id: 11, op: 'main_call', method: 'Browser.getVersion', params: {}, sessionId: '', timeoutMS: 1000}); await tick();
  f.rawIn.write(JSON.stringify({id: 2, result: {product: 'Chrome/synthetic'}}) + '\0'); await tick();
  assert.equal(f.replies.find(r => r.id === 11).result.product, 'Chrome/synthetic');
  assert.equal(sourceClosed, false);
});

test('IG retry and X batch begin coexist in both coalesced wire orders',async t=>{
  for(const retryFirst of [true,false]){
    let finishX,finishRetry;
    const f=controlFixture(t,async()=>({execute:()=>new Promise(resolve=>{finishX=resolve;}),close:async()=>{}}),{
      instagram:async()=>({execute:()=>new Promise(resolve=>{finishRetry=resolve;}),close:async()=>{}})});await tick();
    f.send({id:1,op:'main_x',operation:'daily_owner_bootstrap',params:{}});await tick();
    f.send({id:2,op:'main_source',source:'instagram',operation:'source_bootstrap',params:{}});await tick();
    const retry={op:'main_source',source:'instagram',operation:'source_retry',params:{}},x={op:'main_x',operation:'daily_batch_begin',params:{}};
    const pair=retryFirst?[retry,x]:[x,retry];
    f.input.write(pair.map((q,i)=>JSON.stringify({id:i+3,...q})).join('\n')+'\n');await tick();
    assert.equal(typeof finishX,'function');assert.equal(typeof finishRetry,'function');
    finishRetry({rearmed:true,retryCount:1,retryLimit:2,descriptor:{source:'instagram'}});finishX({begun:true});await tick();
    for(const id of [3,4])assert.equal(f.replies.find(r=>r.id===id).ok,true);
  }
});

test('IG retry accepts a pending public call and diagnostic fences both orderings',async t=>{
  let finishRetry,finishDiagnostic,dispatches=0;
  const f=controlFixture(t,undefined,{instagram:async()=>({execute:()=>{dispatches++;return new Promise(resolve=>{finishRetry=resolve;});},close:async()=>{}})},
    {diagnoseInstagramReader:()=>new Promise(resolve=>{finishDiagnostic=resolve;})});await tick();
  f.send({id:1,op:'main_source',source:'instagram',operation:'source_bootstrap',params:{}});await tick();
  f.send({id:2,op:'main_call',method:'Browser.getVersion',params:{},sessionId:'',timeoutMS:1000});
  f.send({id:3,op:'main_source',source:'instagram',operation:'source_retry',params:{}});await tick();assert.equal(dispatches,1);
  f.rawIn.write(JSON.stringify({id:1,result:{product:'Chrome/synthetic'}})+'\0');await tick();
  f.send({id:4,op:'main_instagram_reader_diagnostic',nonce:'a'.repeat(48)});await tick();
  assert.equal(f.replies.find(r=>r.id===4&&r.ok===false).error.code,'instagram_diagnostic_rejected');assert.equal(finishDiagnostic,undefined);
  finishRetry({rearmed:true});await tick();
  f.send({id:5,op:'main_instagram_reader_diagnostic',nonce:'a'.repeat(48)});await tick();assert.equal(typeof finishDiagnostic,'function');
  f.send({id:6,op:'main_source',source:'instagram',operation:'source_retry',params:{}});await tick();
  assert.deepEqual(f.replies.find(r=>r.id===6),{id:6,ok:false,error:{code:'main_ig_retry_not_dispatched'}});assert.equal(dispatches,1);
  f.send({id:7,op:'main_source',source:'instagram',operation:'source_retry',params:{extra:true}});await tick();
  assert.equal(f.replies.find(r=>r.id===7).error.code,'main_transport_rejected');
  finishDiagnostic({});await tick();
});

test('only validated IG retry receives typed pre-registry saturation rejection',async t=>{
  let dispatches=0;
  const f=controlFixture(t,undefined,{instagram:async()=>({execute:async()=>{dispatches++;return {};},close:async()=>{}})});await tick();
  f.send({id:1,op:'main_source',source:'instagram',operation:'source_bootstrap',params:{}});await tick();
  for(let id=2;id<34;id++)f.send({id,op:'main_call',method:'Browser.getVersion',params:{},sessionId:'',timeoutMS:1000});
  const variants=[{source:'instagram',operation:'source_retry',params:{}},{source:'x',operation:'source_retry',params:{}},{source:'instagram',operation:'source_check',params:{}},{source:'instagram',operation:'source_retry',params:{extra:true}},{source:'instagram',operation:'source_retry',params:[]},{source:'instagram',operation:'source_retry',params:{},extra:true}];
  variants.forEach((q,i)=>f.send({id:34+i,op:'main_source',...q}));await tick();
  variants.forEach((q,i)=>assert.equal(f.replies.find(r=>r.id===34+i).error.code,i===0?'main_ig_retry_not_dispatched':'main_transport_rejected'));
  assert.equal(dispatches,0);
  for(let id=1;id<=32;id++)f.rawIn.write(JSON.stringify({id,result:{product:'Chrome/synthetic'}})+'\0');await tick();
});

test('source wire preserves every finite IG HTTP identity reason for each role',async t=>{
  let failure;
  const f=controlFixture(t,undefined,{instagram:async()=>({execute:async()=>{throw failure;},close:async()=>{}})});await tick();
  f.send({id:1,op:'main_source',source:'instagram',operation:'source_bootstrap',params:{}});await tick();
  let id=1;
  for(const role of ['reader','collector','unknown'])for(const status of INSTAGRAM_HTTP_REJECTIONS){
    const reason=`${role}_identity_${status}`;
    failure=nativeSourceFailure('evidence',reason);
    annotateNativeSourceCleanup(failure,'verified');
    f.send({id:++id,op:'main_source',source:'instagram',operation:'source_check',params:{}});await tick();
    assert.deepEqual(f.replies.at(-1),{id,ok:false,error:{code:`native_source_evidence_${reason}`}});
  }
});

test('source wire rejects invented numeric identity reasons and numeric stage or cleanup text',async t=>{
  let failure;
  const f=controlFixture(t,undefined,{instagram:async()=>({execute:async()=>{throw failure;},close:async()=>{}})});await tick();
  f.send({id:1,op:'main_source',source:'instagram',operation:'source_bootstrap',params:{}});await tick();
  const reasons=['99','600','200','401','403','429','0400','4000','400_private_token','400\n','private-token']
    .map(suffix=>`reader_identity_http_status_${suffix}`);
  const cases=[...reasons.map(reason=>['evidence',reason,'verified']),
    ['evidence','outsider_identity_http_status_400','verified'],
    ['capture','reader_identity_http_status_400','verified'],
    ['evidence1','reader_identity_http_status_400','verified'],
    ['evidence','reader_identity_http_status_400','verified1']];
  let id=1;
  for(const [stage,reason,cleanup] of cases){
    failure=nativeSourceFailure(stage,reason);annotateNativeSourceCleanup(failure,cleanup);
    f.send({id:++id,op:'main_source',source:'instagram',operation:'source_check',params:{}});await tick();
    assert.deepEqual(f.replies.at(-1),{id,ok:false,error:{code:'main_transport_rejected'}});
  }
});

test('source identity and X operation failures release their lanes without closing the peer transport',async t=>{
  let sourceClosed=false,xClosed=false;
  const f=controlFixture(t,async()=>({execute:async op=>{
    if(op==='daily_batch_begin')throw new Error('synthetic-x-failure');
    return {legacyX:true};
  },close:async()=>{xClosed=true;}}),{instagram:async()=>({execute:async op=>{
    if(op==='source_seed')throw nativeSourceFailure('evidence','reader_identity_http_status_400');
    if(op==='source_end')return {closed:true};
    return {source:'instagram'};
  },close:async()=>{sourceClosed=true;}})});await tick();
  const send=async(id,op,operation)=>{
    f.send({id,op,...(op==='main_source'?{source:'instagram'}:{}),operation,params:{}});await tick();
    return f.replies.find(reply=>reply.id===id);
  };
  await send(1,'main_source','source_bootstrap');
  assert.equal((await send(2,'main_source','source_seed')).error.code,'native_source_evidence_reader_identity_http_status_400');
  assert.deepEqual((await send(3,'main_source','source_end')).result,{closed:true});
  await send(4,'main_x','daily_owner_bootstrap');
  assert.deepEqual((await send(5,'main_x','daily_reader_check')).result,{legacyX:true});
  assert.equal((await send(6,'main_x','daily_batch_begin')).ok,false);
  assert.deepEqual((await send(7,'main_source','source_check')).result,{source:'instagram'});
  assert.equal(sourceClosed,false);assert.equal(xClosed,false);
});

test('unqualified native IG cannot be enabled by a request or read a profile during bootstrap',async t=>{
  let reads=0;
  const transport={get call(){reads++;throw new Error('must not read');}};
  await assert.rejects(createBorrowedInstagramControl({nativeQualified:true},transport));
  assert.equal(reads,0);
  const f=controlFixture(t,undefined,{instagram:createBorrowedInstagramControl});await tick();
  f.send({id:1,op:'main_source',source:'instagram',operation:'source_bootstrap',params:{nativeQualified:true}});await tick();
  assert.equal(f.replies.find(r=>r.id===1).error.code,'native_source_qualification_required');
});

test('terminal source close can cancel pending seed without publishing its late success',async t=>{
  let finishSeed,finishClose;
  const f=controlFixture(t,undefined,{instagram:async()=>({
    execute:async op=>op==='source_seed'?new Promise(done=>{finishSeed=done;}):{},
    close:async()=>{finishSeed({seeded:true});await new Promise(done=>{finishClose=done;});},
  })});await tick();
  f.send({id:1,op:'main_source',source:'instagram',operation:'source_bootstrap',params:{}});await tick();
  f.send({id:2,op:'main_source',source:'instagram',operation:'source_seed',params:{}});await tick();
  f.send({id:3,op:'main_source',source:'instagram',operation:'source_close',params:{}});await tick();
  assert.equal(f.replies.find(r=>r.id===2).ok,false);
  // The seed holder has finished, but cleanup still owns the exclusive lane.
  f.send({id:4,op:'main_call',method:'Browser.getVersion',params:{},sessionId:'',timeoutMS:1000});await tick();
  assert.equal(f.replies.find(r=>r.id===4).ok,false);
  finishClose();await tick();
  assert.deepEqual(f.replies.find(r=>r.id===3).result,{closed:true});
  assert.ok(!f.replies.some(r=>r.id===2&&r.ok));
});

test('explicit trial acknowledgement permits owner binding without qualifying the recipe or reading session data',async()=>{
  const transport={alive:()=>true,call:async()=>{throw Error('no session read');},reserveTarget(){},reserveSession(){},releaseTarget(){},releaseSession(){}};
  const owner=await createBorrowedInstagramControl({chromePath:resolve('offline/chrome.exe'),readerProfile:resolve('offline/reader'),readerPID:101,
    profileDirectory:'Default',acknowledgeLimitations:true,allowProcessSessionDerivation:true,acknowledgeUnqualifiedRecipe:true},transport);
  const result=await owner.execute('source_initialize',{});assert.equal(result.source,'instagram');
  assert.equal(owner.capabilities.nativeQualified,false);await owner.close();
});

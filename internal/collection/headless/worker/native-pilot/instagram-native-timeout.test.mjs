import test from 'node:test';
import assert from 'node:assert/strict';
import {PassThrough} from 'node:stream';
import {createMainCDP,serveMainBrowserTransport,MAIN_TRANSPORT_SCHEMA} from './main-browser-transport-protocol.mjs';
import {createNativeOwnedPage,createNativeSourceAttempt,nativeSourceDiagnostic,nativeSourceFailure} from './native-source-kernel.mjs';
import {createOwnedInstagramBootstrapScope} from './main-bootstrap-corroboration-scope.mjs';
import {createIGTrace} from './instagram-diagnostic-trace.mjs';
import {cdpCommandFailureDetail} from './cdp-command-failure.mjs';

const tick=()=>new Promise(resolve=>setImmediate(resolve));
async function waitFor(predicate){for(let i=0;i<100;i++){if(predicate())return;await tick();}assert.fail('synthetic resource did not retire');}
function peer(input,output){
  const requests=[],held=[],targets=new Set(['peer-target']);
  let hold;
  const answer=(q,result)=>input.write(JSON.stringify({id:q.id,...(q.sessionId?{sessionId:q.sessionId}:{}),result})+'\0');
  output.on('data',wire=>{
    const q=JSON.parse(wire.toString().slice(0,-1));requests.push(q);
    if(q.method===hold){held.push(q);return;}
    let result={};
    if(q.method==='Target.createTarget'){targets.add('owned-target');result={targetId:'owned-target'};}
    if(q.method==='Target.attachToTarget')result={sessionId:'owned-session'};
    if(q.method==='Target.closeTarget'){targets.delete(q.params.targetId);result={success:true};}
    if(q.method==='Target.getTargets')result={targetInfos:[...targets].map(targetId=>({targetId}))};
    if(q.method==='Fetch.takeResponseBodyAsStream')result={stream:'owned-stream'};
    if(q.method==='Browser.getVersion')result={product:'Chrome/synthetic'};
    answer(q,result);
  });
  return {requests,held,targets,answer,set hold(method){hold=method;}};
}
function fixture(t){
  const input=new PassThrough(),output=new PassThrough(),f=peer(input,output),failures=[];
  const transport=createMainCDP({input,output,onFailure:cause=>failures.push(cause)});t.after(()=>transport.close());
  return {...f,input,output,transport,failures,peer:f};
}
async function bootstrapScope(f,page){
  const scope=createOwnedInstagramBootstrapScope(page,createIGTrace());
  await scope.call('Fetch.enable',{patterns:[{urlPattern:'https://www.instagram.com/',resourceType:'Document',requestStage:'Response'}]},'owned',1000);
  f.input.write(JSON.stringify({sessionId:'owned-session',method:'Fetch.requestPaused',params:{requestId:'owned-request'}})+'\0');
  return scope;
}

test('native reader body timeout is local and the owned stream can still retire',async t=>{
  const f=fixture(t),page=await createNativeOwnedPage(f.transport,1000),scope=await bootstrapScope(f,page);
  await scope.call('Fetch.takeResponseBodyAsStream',{requestId:'owned-request'},'owned',1000);
  f.peer.hold='IO.read';
  await assert.rejects(scope.call('IO.read',{handle:'owned-stream',size:1},'owned',10),error=>cdpCommandFailureDetail(error)?.kind==='timeout');
  assert.equal(f.transport.alive(),true);assert.equal(f.failures.length,0);
  await scope.call('IO.close',{handle:'owned-stream'},'owned',1000);
  assert.equal(await scope.retire(1000),true);scope.dispose();await page.close();
  assert.deepEqual([...f.targets],['peer-target']);
});

for(const method of ['Target.createTarget','Target.attachToTarget'])test('native '+method+' timeout retains the exact late cleanup obligation',async t=>{
  const f=fixture(t);f.peer.hold=method;
  const attempt=createNativeSourceAttempt({source:'instagram',generation:1,nonce:'a'.repeat(48),profileScope:'synthetic',deadlineMS:Date.now()+1000,cleanupMS:20});
  const pending=attempt.own('target',()=>createNativeOwnedPage(f.transport,20),page=>page.close());
  await assert.rejects(pending,error=>{assert.equal(nativeSourceDiagnostic(error)?.cleanup,'uncertain');return true;});
  assert.equal((await attempt.close()).cleanup,'uncertain');assert.equal(f.transport.alive(),true);
  const q=f.held[0];
  if(method==='Target.createTarget'){f.targets.add('owned-target');f.answer(q,{targetId:'owned-target'});}
  else f.answer(q,{sessionId:'owned-session'});
  await waitFor(()=>!f.targets.has('owned-target')&&!f.transport.privateTarget('owned-target')&&!f.transport.privateSession('owned-session'));
  assert.deepEqual([...f.targets],['peer-target']);assert.equal(attempt.status().cleanup,'uncertain');
  assert.equal(f.requests.filter(q=>q.method==='Target.createTarget').length,1);
});

test('native late Fetch stream forwards its disposer after bootstrap disposal',async t=>{
  const f=fixture(t),page=await createNativeOwnedPage(f.transport,1000),scope=await bootstrapScope(f,page);
  f.peer.hold='Fetch.takeResponseBodyAsStream';
  await assert.rejects(scope.call('Fetch.takeResponseBodyAsStream',{requestId:'owned-request'},'owned',10));
  assert.equal(await scope.retire(1000),false);scope.dispose();
  f.answer(f.held[0],{stream:'late-owned-stream'});
  await waitFor(()=>f.requests.some(q=>q.method==='IO.close'&&q.params.handle==='late-owned-stream'));
  await page.close();assert.equal(f.transport.alive(),true);assert.deepEqual([...f.targets],['peer-target']);
  assert.equal(f.requests.filter(q=>q.method==='IO.close'&&q.params.handle==='late-owned-stream').length,1);
});

test('a stream reply after page retirement cannot turn uncertain cleanup into verified cleanup or proof',async t=>{
  const f=fixture(t),page=await createNativeOwnedPage(f.transport,1000),scope=await bootstrapScope(f,page);
  f.peer.hold='Fetch.takeResponseBodyAsStream';
  await assert.rejects(scope.call('Fetch.takeResponseBodyAsStream',{requestId:'owned-request'},'owned',10));
  scope.dispose();await assert.rejects(page.close());
  assert.equal(f.transport.privateTarget('owned-target'),false);assert.equal(f.transport.privateSession('owned-session'),false);
  f.answer(f.held[0],{stream:'after-retirement-stream'});await tick();await tick();
  await assert.rejects(page.close());await assert.rejects(scope.inspect(Buffer.from('PRIVATE'),'a'.repeat(48),'owned-context',10));
  assert.equal(f.transport.alive(),true);assert.deepEqual([...f.targets],['peer-target']);
});

test('private local capability rejects peer targets, sessions and stream handles',async t=>{
  const f=fixture(t),owned=f.transport.createInstagramOwnedScope();
  for(const [method,params,session]of [['Target.closeTarget',{targetId:'peer-target'},''],['Page.enable',{},'peer-session'],['IO.close',{handle:'peer-stream'},'peer-session']])await assert.rejects(owned.call(method,params,session,10));
  assert.equal(f.requests.length,0);assert.equal(f.transport.alive(),true);owned.dispose();
});

test('actual source server delivers one IG terminal result after local timeout and still serves ordinary calls',async t=>{
  const input=new PassThrough(),output=new PassThrough(),rawInput=new PassThrough(),rawOutput=new PassThrough(),f=peer(rawInput,rawOutput),replies=[];
  output.on('data',wire=>replies.push(JSON.parse(wire)));
  let page,serverClosed=0;
  const server=serveMainBrowserTransport({input,output,connect:async()=>({input:rawInput,output:rawOutput}),onClosed:()=>serverClosed++,sourceFactories:{instagram:async(_params,transport)=>({
    execute:async operation=>{
      if(operation!=='source_seed')return {};
      page=await createNativeOwnedPage(transport,1000);const scope=await bootstrapScope({input:rawInput},page);
      await scope.call('Fetch.takeResponseBodyAsStream',{requestId:'owned-request'},'owned',1000);f.hold='IO.read';
      try{await scope.call('IO.read',{handle:'owned-stream',size:1},'owned',10);assert.fail('timeout expected');}
      catch(error){assert.equal(cdpCommandFailureDetail(error)?.kind,'timeout');throw nativeSourceFailure('evidence','reader_bootstrap_body_read');}
      finally{await scope.call('IO.close',{handle:'owned-stream'},'owned',1000);await scope.retire(1000);scope.dispose();await page.close();}
    },close:async()=>{},
  })}});t.after(()=>server.close());
  const send=q=>input.write(JSON.stringify(q)+'\n');
  send({op:'main_transport_bootstrap',schema:MAIN_TRANSPORT_SCHEMA,nonce:'a'.repeat(48),transport:'inherited-stdio'});await waitFor(()=>replies.some(q=>q.ready));
  send({id:1,op:'main_source',source:'instagram',operation:'source_bootstrap',params:{}});await waitFor(()=>replies.some(q=>q.id===1));
  send({id:2,op:'main_source',source:'instagram',operation:'source_seed',params:{}});await waitFor(()=>f.held.length>0);
  await new Promise(resolve=>setTimeout(resolve,20));await waitFor(()=>replies.some(q=>q.id===2));
  assert.equal(replies.filter(q=>q.id===2).length,1);assert.equal(replies.find(q=>q.id===2).error.code,'native_source_evidence_reader_bootstrap_body_read');
  send({id:3,op:'main_call',method:'Browser.getVersion',params:{},sessionId:'',timeoutMS:1000});await waitFor(()=>replies.some(q=>q.id===3));
  assert.equal(replies.find(q=>q.id===3).ok,true);assert.equal(serverClosed,0);assert.equal(replies.some(q=>q.event==='main_transport_lifecycle'),false);
});

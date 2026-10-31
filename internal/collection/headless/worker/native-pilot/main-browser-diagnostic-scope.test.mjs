import test from 'node:test';
import assert from 'node:assert/strict';
import {PassThrough} from 'node:stream';
import {createMainCDP,serveMainBrowserTransport,MAIN_TRANSPORT_SCHEMA} from './main-browser-transport-protocol.mjs';
import {runInstagramReaderDiagnostic} from './instagram-reader-diagnostic.mjs';

const tick=()=>new Promise(resolve=>setImmediate(resolve));
const result=()=>({schema:'aku.instagram-reader-diagnostic.v1',primaryCause:'http_rejected',status:400,mime:'json',
  bodyClass:'json_object',errorCategory:'unknown',documentBound:true,wireMatched:true,cleanup:'verified'});
function fixture(t){
  const input=new PassThrough(),output=new PassThrough(),requests=[];
  output.on('data',wire=>requests.push(JSON.parse(wire.subarray(0,-1).toString())));
  const transport=createMainCDP({input,output});t.after(()=>transport.close());
  return{transport,requests,input,answer(request,value){input.write(JSON.stringify({id:request.id,result:value,...(request.sessionId?{sessionId:request.sessionId}:{})})+'\0');}};
}
async function own(f){
  const scope=f.transport.createDiagnosticScope();
  const created=scope.call('Target.createTarget',{url:'about:blank',background:true,forTab:false},'',1000);
  f.answer(f.requests.at(-1),{targetId:'owned'});await created;
  const attached=scope.call('Target.attachToTarget',{targetId:'owned',flatten:true},'',1000);
  f.answer(f.requests.at(-1),{sessionId:'owned-session'});await attached;
  return scope;
}

test('diagnostic evaluation timeout and late reply leave the ordinary CDP connection alive',async t=>{
  const f=fixture(t),scope=await own(f);
  const timeout=scope.call('Runtime.evaluate',{expression:'fixed',uniqueContextId:'context',returnByValue:true,awaitPromise:true,userGesture:false},'owned-session',10);
  const expired=f.requests.at(-1);await assert.rejects(timeout,/diagnostic_request_timeout/);
  assert.equal(f.transport.alive(),true);
  const peer=f.transport.call('Browser.getVersion',{},'',1000),peerRequest=f.requests.at(-1);
  f.answer(expired,{result:{value:'PRIVATE_LATE_BODY'}});f.answer(peerRequest,{product:'Synthetic'});
  assert.equal((await peer).product,'Synthetic');
  const close=scope.call('Target.closeTarget',{targetId:'owned'},'',1000);f.answer(f.requests.at(-1),{success:true});await close;
  const inventory=scope.call('Target.getTargets',{},'',1000);f.answer(f.requests.at(-1),{targetInfos:[{targetId:'user-reader'}]});await inventory;
  scope.releaseSession('owned-session');scope.releaseTarget('owned');scope.dispose();
  assert.equal(scope.cleanupVerified(),true);
});

test('ordinary CDP retains its existing fail-closed timeout behavior',async t=>{
  const f=fixture(t);
  await assert.rejects(f.transport.call('Browser.getVersion',{},'',10),/main_transport_rejected/);
  assert.equal(f.transport.alive(),false);
});

test('diagnostic scope rejects cookie APIs, foreign resources and caller navigation changes',async t=>{
  const f=fixture(t),scope=await own(f),before=f.requests.length;
  for(const [method,params,session]of [
    ['Network.getCookies',{},'owned-session'],['Network.setCookies',{cookies:[]},'owned-session'],
    ['Network.getResponseBody',{requestId:'private'},'owned-session'],['Browser.close',{},''],
    ['Target.closeTarget',{targetId:'user-reader'},''],['Target.attachToTarget',{targetId:'user-reader',flatten:true},''],
    ['Page.navigate',{url:'https://outside.invalid/'},'owned-session'],['Page.getFrameTree',{},'foreign'],
    ['Runtime.evaluate',{expression:'x',returnByValue:true,awaitPromise:true,userGesture:false},'owned-session'],
  ])await assert.rejects(scope.call(method,params,session,1000),/diagnostic_scope_rejected/);
  assert.throws(()=>scope.reserveTarget('user-reader'),/diagnostic_scope_rejected/);
  assert.throws(()=>scope.reserveSession('foreign'),/diagnostic_scope_rejected/);
  assert.throws(()=>scope.releaseTarget('owned'),/diagnostic_scope_rejected/);
  assert.throws(()=>scope.releaseSession('owned-session'),/diagnostic_scope_rejected/);
  assert.equal(f.requests.length,before);
  scope.dispose();assert.equal(scope.cleanupVerified(),false);assert.equal(f.transport.alive(),true);
});

test('a timed-out target allocation is uncertain and a late reply retires only its own target',async t=>{
  const f=fixture(t),scope=f.transport.createDiagnosticScope();
  const allocation=scope.call('Target.createTarget',{url:'about:blank',background:true,forTab:false},'',10);
  const request=f.requests.at(-1);await assert.rejects(allocation,/diagnostic_request_timeout/);
  scope.dispose();assert.equal(scope.cleanupVerified(),false);assert.equal(f.transport.alive(),true);
  f.answer(request,{targetId:'late-owned'});await tick();
  assert.deepEqual(f.requests.at(-1).params,{targetId:'late-owned'});
  f.answer(f.requests.at(-1),{success:true});await tick();
  assert.equal(f.requests.at(-1).method,'Target.getTargets');
  f.answer(f.requests.at(-1),{targetInfos:[{targetId:'user-reader'}]});await tick();
  assert.equal(scope.cleanupVerified(),true);assert.equal(f.transport.privateTarget('late-owned'),false);
  assert.ok(!f.requests.some(q=>q.method==='Target.closeTarget'&&q.params.targetId==='user-reader'));
});

test('a late attachment cannot leave a private session pin after verified target retirement',async t=>{
  const f=fixture(t),scope=f.transport.createDiagnosticScope();
  const create=scope.call('Target.createTarget',{url:'about:blank',background:true,forTab:false},'',1000);
  f.answer(f.requests.at(-1),{targetId:'owned'});await create;
  const attach=scope.call('Target.attachToTarget',{targetId:'owned',flatten:true},'',10),late=f.requests.at(-1);
  await assert.rejects(attach,/diagnostic_request_timeout/);
  const close=scope.call('Target.closeTarget',{targetId:'owned'},'',1000);f.answer(f.requests.at(-1),{success:true});await close;
  const inventory=scope.call('Target.getTargets',{},'',1000);f.answer(f.requests.at(-1),{targetInfos:[]});await inventory;
  scope.releaseTarget('owned');scope.dispose();assert.equal(scope.cleanupVerified(),false);
  f.answer(late,{sessionId:'late-session'});await tick();
  assert.equal(f.transport.privateSession('late-session'),false);assert.equal(scope.cleanupVerified(),true);
});

test('only owned diagnostic events are visible and listener errors cannot close peers',async t=>{
  const f=fixture(t),scope=await own(f),seen=[];
  scope.on(event=>{seen.push(event.method);throw Error('PRIVATE');});
  for(const event of [
    {method:'Runtime.executionContextsCleared',sessionId:'foreign'},
    {method:'Network.requestWillBeSentExtraInfo',sessionId:'owned-session',params:{headers:{Cookie:'PRIVATE'}}},
    {method:'Runtime.executionContextsCleared',sessionId:'owned-session'},
  ])f.input.write(JSON.stringify(event)+'\0');
  assert.deepEqual(seen,['Runtime.executionContextsCleared']);assert.equal(f.transport.alive(),true);scope.dispose();
});

test('malformed inventory cannot prove absence or release diagnostic ownership',async t=>{
  const f=fixture(t),scope=await own(f);
  for(const targetInfos of [[{}],[null],[{targetId:''}],[{targetId:'user-reader'},{targetId:'user-reader'}]]){
    const pending=scope.call('Target.getTargets',{},'',1000);
    f.answer(f.requests.at(-1),{targetInfos});await assert.rejects(pending,/diagnostic_scope_rejected/);
    assert.throws(()=>scope.releaseTarget('owned'),/diagnostic_scope_rejected/);
    assert.throws(()=>scope.releaseSession('owned-session'),/diagnostic_scope_rejected/);
    assert.equal(scope.cleanupVerified(),false);assert.equal(f.transport.privateTarget('owned'),true);
  }
  assert.equal(f.transport.alive(),true);scope.dispose();
});

function control(t,diagnoseInstagramReader,createX,sourceFactories={}){
  const input=new PassThrough(),output=new PassThrough(),rawIn=new PassThrough(),rawOut=new PassThrough(),replies=[];
  output.on('data',wire=>replies.push(JSON.parse(wire.toString())));
  const server=serveMainBrowserTransport({input,output,createX,sourceFactories,diagnoseInstagramReader,connect:async()=>({input:rawIn,output:rawOut})});
  t.after(()=>server.close());
  const send=q=>input.write(JSON.stringify(q)+'\n');
  send({op:'main_transport_bootstrap',schema:MAIN_TRANSPORT_SCHEMA,nonce:'a'.repeat(48),transport:'inherited-stdio'});
  return{send,replies};
}
test('diagnostic op is sealed, unavailable without a factory and rejects arbitrary caller fields',async t=>{
  let calls=0;
  const f=control(t,async(_transport,request)=>{calls++;assert.deepEqual(request,{nonce:'a'.repeat(48)});return result();});await tick();
  for(const [id,extra]of [[1,{url:'https://outside.invalid/'}],[2,{nonce:'PRIVATE'}],[3,{params:{expression:'PRIVATE'}}]]){
    f.send({id,op:'main_instagram_reader_diagnostic',nonce:'a'.repeat(48),...extra});await tick();
    assert.equal(f.replies.find(reply=>!reply.event&&reply.id===id).ok,false);
  }
  assert.equal(calls,0);
  f.send({id:4,op:'main_instagram_reader_diagnostic',nonce:'a'.repeat(48)});await tick();
  assert.deepEqual(f.replies.find(reply=>!reply.event&&reply.id===4).result,result());assert.equal(calls,1);
  const disabled=control(t);await tick();disabled.send({id:1,op:'main_instagram_reader_diagnostic',nonce:'a'.repeat(48)});await tick();
  assert.equal(disabled.replies.find(reply=>!reply.event&&reply.id===1).ok,false);
});

test('diagnostic lane stays exclusive and malformed output never reaches the client',async t=>{
  let release;
  const f=control(t,async()=>new Promise(resolve=>{release=resolve;}),async()=>({execute:async()=>({peer:true}),close:async()=>{}}));await tick();
  f.send({id:1,op:'main_x',operation:'daily_owner_bootstrap',params:{}});await tick();
  f.send({id:2,op:'main_instagram_reader_diagnostic',nonce:'a'.repeat(48)});await tick();
  f.send({id:3,op:'main_x',operation:'daily_batch_begin',params:{}});await tick();
  assert.equal(f.replies.find(reply=>!reply.event&&reply.id===3).ok,false);
  f.send({id:4,op:'main_x',operation:'daily_reader_check',params:{}});await tick();
  assert.equal(f.replies.find(reply=>!reply.event&&reply.id===4).ok,false);
  release({...result(),accountId:'PRIVATE_ACCOUNT'});await tick();
  assert.equal(f.replies.find(reply=>!reply.event&&reply.id===2).ok,false);assert.ok(!JSON.stringify(f.replies).includes('PRIVATE'));
  f.send({id:5,op:'main_x',operation:'daily_batch_begin',params:{}});await tick();
  assert.deepEqual(f.replies.find(reply=>!reply.event&&reply.id===5).result,{peer:true});
});

test('diagnostic admission waits for in-flight nonexclusive source or X owner checks',async t=>{
  for(const kind of ['x','instagram']){
    let release,calls=0;
    const owner={execute:async()=>new Promise(resolve=>{release=resolve;}),close:async()=>{}};
    const f=control(t,async()=>{calls++;return result();},async()=>owner,{instagram:async()=>owner});await tick();
    const message=(id,operation)=>({id,op:kind==='x'?'main_x':'main_source',...(kind==='instagram'?{source:kind}:{}),operation,params:{}});
    f.send(message(1,kind==='x'?'daily_owner_bootstrap':'source_bootstrap'));await tick();
    f.send(message(2,kind==='x'?'daily_reader_check':'source_check'));await tick();
    f.send({id:3,op:'main_instagram_reader_diagnostic',nonce:'a'.repeat(48)});await tick();
    assert.equal(f.replies.find(reply=>!reply.event&&reply.id===3).ok,false);assert.equal(calls,0);
    release({peer:true});await tick();
    f.send({id:4,op:'main_instagram_reader_diagnostic',nonce:'a'.repeat(48)});await tick();
    assert.equal(f.replies.find(reply=>!reply.event&&reply.id===4).ok,true);assert.equal(calls,1);
  }
});

test('uncertain diagnostic cleanup blocks another probe without disabling the X lane',async t=>{
  let calls=0;
  const f=control(t,async()=>{calls++;return{...result(),cleanup:'uncertain'};},async()=>({execute:async()=>({peer:true}),close:async()=>{}}));await tick();
  f.send({id:1,op:'main_instagram_reader_diagnostic',nonce:'a'.repeat(48)});await tick();
  f.send({id:2,op:'main_instagram_reader_diagnostic',nonce:'b'.repeat(48)});await tick();
  assert.equal(f.replies.find(reply=>!reply.event&&reply.id===2).ok,false);assert.equal(calls,1);
  f.send({id:3,op:'main_x',operation:'daily_owner_bootstrap',params:{}});await tick();
  f.send({id:4,op:'main_x',operation:'daily_reader_check',params:{}});await tick();
  assert.deepEqual(f.replies.find(reply=>!reply.event&&reply.id===4).result,{peer:true});
});

test('reader runner binds and retires its target through the actual restricted CDP scope',async t=>{
  const f=fixture(t),nonce='a'.repeat(48);let done=false,closed=false;
  const run=runInstagramReaderDiagnostic(f.transport,{nonce}).finally(()=>{done=true;});
  let next=0;
  for(let cycle=0;!done&&cycle<100;cycle++){
    await tick();
    while(next<f.requests.length){
      const q=f.requests[next++];let reply={};
      const event=(method,params)=>f.input.write(JSON.stringify({method,sessionId:'owned-session',params})+'\0');
      switch(q.method){
        case 'Target.createTarget':reply={targetId:'owned'};break;
        case 'Target.attachToTarget':reply={sessionId:'owned-session'};break;
        case 'Page.navigate':
          event('Runtime.executionContextCreated',{context:{id:7,uniqueId:'context',origin:'https://www.instagram.com',auxData:{isDefault:true,frameId:'frame'}}});
          reply={frameId:'frame',loaderId:'loader'};break;
        case 'Page.getFrameTree':reply={frameTree:{frame:{id:'frame',loaderId:'loader',url:'https://www.instagram.com/'}}};break;
        case 'Runtime.evaluate':
          if(!q.params.expression.includes('api/v1/accounts'))reply={result:{value:true}};
          else{
            const url='https://www.instagram.com/api/v1/accounts/current_user/?edit=true';
            event('Network.requestWillBeSent',{frameId:'frame',loaderId:'loader',requestId:'request',request:{url,method:'GET'}});
            event('Network.responseReceived',{frameId:'frame',loaderId:'loader',requestId:'request',response:{url,status:400}});
            reply={result:{value:{nonce,primaryCause:'http_rejected',status:400,mime:'json',bodyClass:'json_object',errorCategory:'unknown',homeAfter:true}}};
          }break;
        case 'Target.closeTarget':closed=true;reply={success:true};break;
        case 'Target.getTargets':reply={targetInfos:[{targetId:'user-reader'},...(!closed?[{targetId:'owned'}]:[])]};break;
      }
      f.answer(q,reply);
    }
  }
  assert.equal(done,true,'synthetic runner did not settle');
  assert.deepEqual(await run,result());assert.equal(f.transport.alive(),true);
  assert.equal(f.transport.privateTarget('owned'),false);assert.equal(f.transport.privateSession('owned-session'),false);
  assert.ok(!f.requests.some(q=>/getCookies|setCookies|createBrowserContext|Browser.close/.test(q.method)));
  assert.equal(f.requests.filter(q=>q.method==='Runtime.evaluate'&&q.params.expression.includes('api/v1/accounts')).length,1);
});

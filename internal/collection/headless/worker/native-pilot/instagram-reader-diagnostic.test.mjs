import test from 'node:test';
import assert from 'node:assert/strict';
import {runInNewContext} from 'node:vm';
import {instagramReaderDiagnosticExpression, runInstagramReaderDiagnostic, validateInstagramReaderDiagnostic,
  INSTAGRAM_DIAGNOSTIC_SCHEMA} from './instagram-reader-diagnostic.mjs';

const nonce='a'.repeat(48), home='https://www.instagram.com/', endpoint=home+'api/v1/accounts/current_user/?edit=true';
const plain=value=>JSON.parse(JSON.stringify(value));
function pageFixture(options={}) {
  const counters={fetch:0,body:0,cancel:0,headers:0};let request,timeoutMS;
  const sandbox={location:{origin:'https://www.instagram.com',pathname:'/'},document:{querySelector:()=>options.login?{}:null},
    URL,TextDecoder,Uint8Array,AbortController,clearTimeout,
    setTimeout(callback,ms){timeoutMS=ms;if(options.abort){queueMicrotask(callback);return 1;}return setTimeout(callback,ms);},
    fetch:async(url,init)=>{
      counters.fetch++;request={url,init};
      if(options.abort){await Promise.resolve();throw Error('PRIVATE_TOKEN');}
      if(options.network)throw Error('PRIVATE_TOKEN');
      const data=options.bytes??new TextEncoder().encode(options.body??JSON.stringify({error_type:'login_required',user:{pk:'PRIVATE_ACCOUNT'},message:'PRIVATE_SECRET'}));
      let sent=false,index=0;
      const response={status:options.status??400,
        get url(){if(options.poison)throw Error('PRIVATE_URL');return options.url??endpoint;},
        get headers(){counters.headers++;if(options.poison)throw Error('PRIVATE_HEADERS');return new Headers({'content-type':options.mime??'application/json',...(options.length?{'content-length':String(options.length)}:{})});},
        get body(){counters.body++;if(options.poison)throw Error('PRIVATE_BODY');return{getReader:()=>({
          async read(){if(options.readFailure)throw Error('PRIVATE_READ');if(options.chunks){return index<options.chunks.length?{done:false,value:options.chunks[index++]}:{done:true};}if(sent)return{done:true};sent=true;return{done:false,value:data};},
          cancel(){counters.cancel++;return options.cancelNever?new Promise(()=>{}):Promise.resolve();},
        })};},
      };return response;
    }};
  if(options.path)sandbox.location.pathname=options.path;
  return{async run(){return plain(await runInNewContext(instagramReaderDiagnosticExpression(nonce),sandbox));},counters,
    get request(){return request;},get timeoutMS(){return timeoutMS;}};
}

test('one unchanged request; error body exports only exact finite classification',async()=>{
  const f=pageFixture(),p=await f.run();
  assert.equal(f.counters.fetch,1);assert.equal(f.timeoutMS,4000);
  assert.equal(f.request.url,'/api/v1/accounts/current_user/?edit=true');
  const {signal,...options}=f.request.init;
  assert.deepEqual(plain(options),{method:'GET',credentials:'same-origin',cache:'no-store',redirect:'error',headers:{Accept:'application/json'}});
  assert.ok(signal instanceof AbortSignal);
  assert.deepEqual(p,{nonce,primaryCause:'http_rejected',status:400,mime:'json',bodyClass:'json_object',errorCategory:'login_required',homeAfter:true});
  assert.ok(!JSON.stringify(p).includes('PRIVATE'));assert.equal(f.counters.cancel,1);
});
test('200 never reads response URL, headers, body, account or cookies',async()=>{
  const f=pageFixture({status:200,poison:true}),p=await f.run();
  assert.equal(p.primaryCause,'unexpected_success');assert.equal(p.bodyClass,'not_read');
  assert.equal(f.counters.headers,0);assert.equal(f.counters.body,0);
});
test('stream and declared size enforce the 4096-byte boundary',async()=>{
  for(const [size,bodyClass] of [[4096,'json_object'],[4097,'oversize']]){
    const f=pageFixture({body:'{"message":"'+ 'x'.repeat(size-14)+'"}'});
    assert.equal((await f.run()).bodyClass,bodyClass);
  }
  const f=pageFixture({length:4097});assert.equal((await f.run()).bodyClass,'oversize');assert.equal(f.counters.body,0);
  const chunked=pageFixture({chunks:[new Uint8Array(2048),new Uint8Array(2049)]});
  assert.equal((await chunked.run()).bodyClass,'oversize');assert.equal(chunked.counters.cancel,1);
});
test('invalid, non-object, unknown and private JSON never becomes arbitrary diagnostics',async()=>{
  for(const [options,bodyClass] of [[{body:'PRIVATE'},'invalid_json'],[{bytes:new Uint8Array([255])},'invalid_utf8'],[{body:'[]'},'json_nonobject'],
    [{body:'null'},'json_nonobject'],[{readFailure:true},'read_failed']]){
    const p=await pageFixture(options).run();assert.equal(p.bodyClass,bodyClass);assert.ok(!JSON.stringify(p).includes('PRIVATE'));
  }
  for(const value of [{error_type:'PRIVATE_TOKEN',message:'PRIVATE_MESSAGE'},{error_type:'invalid_app_id',user:{pk:'PRIVATE_ACCOUNT'}},
    {message:'csrf token missing PRIVATE_SECRET'},{error_type:{value:'login_required'}}]){
    const p=await pageFixture({body:JSON.stringify(value)}).run();assert.equal(p.errorCategory,'unknown');assert.ok(!JSON.stringify(p).includes('PRIVATE'));
  }
  for(const error_type of ['login_required','challenge_required','checkpoint_required','feedback_required','sentry_block','csrf_failed']){
    assert.equal((await pageFixture({body:JSON.stringify({error_type})}).run()).errorCategory,error_type);
  }
});
test('scope/MIME gates and preflight reject without reading a body',async()=>{
  for(const options of [{url:'https://outside.invalid/PRIVATE'},{mime:'text/html'},{mime:'text/plain'}]){
    const f=pageFixture(options);await f.run();assert.equal(f.counters.body,0);
  }
  for(const options of [{login:true},{path:'/accounts/login/'}]){
    const f=pageFixture(options);await f.run();assert.equal(f.counters.fetch,0);
  }
});
test('fetch timeout is finite; cancellation cannot hold result indefinitely',async()=>{
  assert.equal((await pageFixture({abort:true}).run()).primaryCause,'fetch_timeout');
  assert.equal((await pageFixture({network:true}).run()).primaryCause,'fetch_failed');
  assert.equal((await pageFixture({cancelNever:true}).run()).primaryCause,'http_rejected');
});

function runnerFixture(options={}) {
  const calls=[],pins=new Set(),sessions=new Set();let listener,alive=true,exists=false,disposed=false,frameReads=0;
  const emit=(method,params)=>listener?.({sessionId:'session-owned',method,params});
  const context=()=>emit('Runtime.executionContextCreated',{context:{id:7,uniqueId:'unique-owned',origin:'https://www.instagram.com',auxData:{isDefault:true,frameId:'frame-owned'}}});
  const scope={alive:()=>alive,cleanupVerified:()=>!options.unresolvedAllocation&&pins.size===0&&sessions.size===0,on(fn){listener=fn;return()=>{listener=undefined;};},
    reserveTarget:id=>pins.add(id),reserveSession:id=>sessions.add(id),releaseTarget:id=>pins.delete(id),releaseSession:id=>sessions.delete(id),
    dispose(){disposed=true;},async call(method,params,sessionId,ms){
      calls.push({method,params,sessionId,ms});
      if(options.failMethod===method)throw Error('PRIVATE_TRANSPORT');
      if(method==='Target.createTarget'){exists=true;return{targetId:'target-owned'};}
      if(method==='Target.attachToTarget')return{sessionId:'session-owned'};
      if(method==='Page.navigate'){context();return{frameId:'frame-owned',loaderId:'loader-owned'};}
      if(method==='Page.getFrameTree'){
        frameReads++;return{frameTree:{frame:{id:'frame-owned',loaderId:options.changeLoader&&frameReads>1?'changed':'loader-owned',url:home}}};
      }
      if(method==='Runtime.evaluate'){
        assert.equal(params.uniqueContextId,'unique-owned');
        if(!params.expression.includes('api/v1/accounts'))return{result:{value:true}};
        if(options.evalFailure){if(options.destroyContext)emit('Runtime.executionContextDestroyed',{executionContextId:7,executionContextUniqueId:'unique-owned'});throw Error('PRIVATE_EVALUATION');}
        if(options.exception)return{exceptionDetails:{text:'PRIVATE_EXCEPTION'}};
        if(!options.noWire){
          emit('Network.requestWillBeSent',{frameId:'frame-owned',loaderId:'loader-owned',requestId:'request-owned',request:{url:endpoint,method:'GET',headers:{PRIVATE:'TOKEN'}}});
          if(options.duplicateWire)emit('Network.requestWillBeSent',{frameId:'frame-owned',loaderId:'loader-owned',requestId:'request-owned',request:{url:endpoint,method:'GET'}});
          emit('Network.responseReceived',{frameId:'frame-owned',loaderId:'loader-owned',requestId:'request-owned',response:{url:endpoint,status:options.status??400,headers:{PRIVATE:'TOKEN'}}});
        }
        if(options.destroyContext)emit('Runtime.executionContextDestroyed',{executionContextId:7,executionContextUniqueId:'unique-owned'});
        if(options.sameDocumentMove)emit('Page.navigatedWithinDocument',{frameId:'frame-owned'});
        return{result:{value:{nonce,primaryCause:options.status===200?'unexpected_success':'http_rejected',status:options.status??400,mime:'json',bodyClass:options.status===200?'not_read':'json_object',errorCategory:'unknown',homeAfter:true,...options.projection}}};
      }
      if(method==='Target.closeTarget'){exists=false;return{success:true};}
      if(method==='Target.getTargets'){if(options.inventoryFailure)throw Error('PRIVATE_INVENTORY');return{targetInfos:exists?[{targetId:'target-owned',url:'PRIVATE_URL'}]:[{targetId:'user-tab',url:'PRIVATE_URL'}]};}
      return{};
    }};
  const transport={createDiagnosticScope:()=>scope};
  return{transport,calls,pins,sessions,scope,emit,get frameReads(){return frameReads;},get disposed(){return disposed;},get alive(){return alive;}};
}
const forbidden=/getCookies|setCookies|Storage\.|createBrowserContext|source_|collector|Browser.close/;
function assertReaderOnly(f){
  assert.ok(f.calls.every(c=>!forbidden.test(c.method)));
  assert.equal(f.calls.filter(c=>c.method==='Target.createTarget').length,1);
  assert.deepEqual(f.calls.find(c=>c.method==='Target.createTarget').params,{url:'about:blank',background:true,forTab:false});
  assert.ok(f.calls.filter(c=>c.method==='Target.closeTarget').every(c=>c.params.targetId==='target-owned'));
  assert.ok(f.calls.filter(c=>c.method==='Page.navigate').every(c=>c.params.url===home));
}
test('sealed runner binds reader request before and after HTTP400; cleanup is independent',async()=>{
  const f=runnerFixture(),r=await runInstagramReaderDiagnostic(f.transport,{nonce});
  assert.deepEqual(r,{schema:INSTAGRAM_DIAGNOSTIC_SCHEMA,primaryCause:'http_rejected',status:400,mime:'json',bodyClass:'json_object',errorCategory:'unknown',documentBound:true,wireMatched:true,cleanup:'verified'});
  assert.equal(f.frameReads,2);assert.equal(f.pins.size,0);assert.equal(f.sessions.size,0);assert.ok(f.disposed);assert.ok(f.alive);
  assertReaderOnly(f);assert.ok(!JSON.stringify(r).includes('PRIVATE'));assert.ok(Buffer.byteLength(JSON.stringify(r))<=1024);
});
test('200 and HTTP failures all remain reader-only',async()=>{
  for(const status of [200,400,401,403,429,500]){
    const f=runnerFixture({status}),r=await runInstagramReaderDiagnostic(f.transport,{nonce});
    assert.equal(r.status,status);assert.equal(r.cleanup,'verified');assertReaderOnly(f);
  }
});
test('rejected responses cannot retain document attribution after context or loader changes',async()=>{
  for(const options of [{destroyContext:true},{changeLoader:true},{sameDocumentMove:true}]){
    const f=runnerFixture(options),r=await runInstagramReaderDiagnostic(f.transport,{nonce});
    assert.equal(r.status,400);assert.equal(r.documentBound,false);assert.equal(r.cleanup,'verified');assertReaderOnly(f);
  }
});
test('wire mismatch is reported independently without altering the HTTP first cause',async()=>{
  for(const options of [{noWire:true},{duplicateWire:true}]){
    const r=await runInstagramReaderDiagnostic(runnerFixture(options).transport,{nonce});
    assert.equal(r.wireMatched,false);assert.equal(r.primaryCause,'http_rejected');
  }
});
test('HTTP cause survives later document mismatch and cleanup failure together',async()=>{
  const f=runnerFixture({changeLoader:true,inventoryFailure:true}),r=await runInstagramReaderDiagnostic(f.transport,{nonce});
  assert.equal(r.primaryCause,'http_rejected');assert.equal(r.documentBound,false);assert.equal(r.cleanup,'uncertain');assert.equal(r.status,400);
});
test('projection forgery and evaluation failure still perform after-binding and cleanup',async()=>{
  for(const [options,cause] of [[{projection:{nonce:'b'.repeat(48)}},'projection_invalid'],[{projection:{accountId:'PRIVATE'}},'projection_invalid'],
    [{projection:{errorCategory:'PRIVATE'}},'projection_invalid'],[{evalFailure:true},'evaluation_failed'],[{exception:true},'evaluation_failed']]){
    const f=runnerFixture(options),r=await runInstagramReaderDiagnostic(f.transport,{nonce});
    assert.equal(r.primaryCause,cause);assert.equal(f.frameReads,2);assert.equal(r.cleanup,'verified');assertReaderOnly(f);
    assert.ok(!JSON.stringify(r).includes('PRIVATE'));
  }
});
test('uncertain cleanup retains pins and first cause without closing shared transport',async()=>{
  const f=runnerFixture({inventoryFailure:true}),r=await runInstagramReaderDiagnostic(f.transport,{nonce});
  assert.equal(r.cleanup,'uncertain');assert.equal(r.primaryCause,'http_rejected');assert.equal(f.pins.size,1);assert.equal(f.sessions.size,1);assert.ok(f.alive);
});
test('scope allocation uncertainty overrides apparent target cleanup success',async()=>{
  const f=runnerFixture({unresolvedAllocation:true}),r=await runInstagramReaderDiagnostic(f.transport,{nonce});
  assert.equal(r.cleanup,'uncertain');assert.equal(r.primaryCause,'http_rejected');assert.ok(f.disposed);
});
test('setup failure and missing create acknowledgement never authorize another target',async()=>{
  for(const method of ['Target.createTarget','Target.attachToTarget','Page.enable','Page.navigate']){
    const f=runnerFixture({failMethod:method}),r=await runInstagramReaderDiagnostic(f.transport,{nonce});
    assert.notEqual(r.primaryCause,'none');assert.equal(r.cleanup,method==='Target.createTarget'?'uncertain':'verified');
    assert.ok(f.calls.filter(c=>c.method==='Target.closeTarget').every(c=>c.params.targetId==='target-owned'));assert.ok(f.alive);
  }
});
test('strict request/output validators reject extra fields, getters, arbitrary labels and status coercion',async()=>{
  const valid=await runInstagramReaderDiagnostic(runnerFixture().transport,{nonce});
  for(const patch of [{status:99},{status:'400'},{status:600},{primaryCause:'PRIVATE'}, {errorCategory:'PRIVATE'}, {cleanup:'PRIVATE'}, {extra:'PRIVATE'}, {documentBound:1}]){
    assert.throws(()=>validateInstagramReaderDiagnostic({...valid,...patch}));
  }
  assert.throws(()=>validateInstagramReaderDiagnostic(Object.defineProperty({...valid},'status',{get(){throw Error('PRIVATE');}})));
  for(const request of [{nonce,extra:true},{nonce:'PRIVATE'},{nonce:1},null]){
    let scopes=0;const r=await runInstagramReaderDiagnostic({createDiagnosticScope(){scopes++;} },request);
    assert.equal(r.primaryCause,'invalid_request');assert.equal(scopes,0);
  }
});

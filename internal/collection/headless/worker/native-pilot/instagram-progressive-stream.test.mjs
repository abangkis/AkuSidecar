import test from 'node:test';
import assert from 'node:assert/strict';
import {PassThrough} from 'node:stream';
import {createMainCDP} from './main-browser-transport-protocol.mjs';
import {createOwnedInstagramBootstrapScope} from './main-bootstrap-corroboration-scope.mjs';
import {createIGTrace} from './instagram-diagnostic-trace.mjs';
import {readInstagramBootstrapBodyStream} from './instagram-bootstrap-body-stream.mjs';
import {fixture as bootstrapFixture} from './instagram-bootstrap-fixture.mjs';
import {acquireOwnedInstagramIdentity} from './instagram-bootstrap-corroboration.mjs';
import {createNativeOwnedPage,createNativeSourceAttempt,nativeSourceDiagnostic} from './native-source-kernel.mjs';
import {readInstagramEvidence} from './instagram-native-session.mjs';
import {createTemporaryPage} from '../chrome.mjs';
import {createChromeCommandSender,settleChromeCommandReply} from '../chrome-command-queue.mjs';

const HOME='https://www.instagram.com/';
const SESSION='synthetic-session';
const TARGET='synthetic-owned-target';
const HANDLE='synthetic-owned-stream';
const REQUEST='synthetic-paused-request';
const CHUNK_BYTES=65536;
const wait=ms=>new Promise(resolve=>setTimeout(resolve,ms));
async function waitFor(predicate,label,timeoutMS=2500){
  const deadline=Date.now()+timeoutMS;
  while(!predicate()){
    if(Date.now()>=deadline)throw new Error(`timed out waiting for ${label}`);
    await wait(2);
  }
}

// This peer is synthetic. It models a single pending IO.read that completes
// when its requested size is buffered or EOF is signaled. On EOF, buffered
// bytes arrive first with eof:false; a following read receives empty eof:true.
// It does not claim to observe or prove Chromium's behavior.
function syntheticPeer(t){
  const input=new PassThrough(),output=new PassThrough(),requests=[];
  const pendingReads=[];
  let buffered=Buffer.alloc(0),eof=false,streamClosed=false,closedTarget=false,lateReadReplies=0;
  const transport=createMainCDP({input,output});
  t.after(()=>transport.close());
  const writeReply=(request,result)=>input.write(JSON.stringify({id:request.id,...(request.sessionId?{sessionId:request.sessionId}:{}),result})+'\0');
  const sendReadReply=(request,data,isEOF)=>{
    if(streamClosed)lateReadReplies++;
    writeReply(request,{data:data.toString('base64'),base64Encoded:true,eof:isEOF});
  };
  const drainReads=()=>{
    while(pendingReads.length){
      const request=pendingReads[0],size=request.params.size;
      if(buffered.length>=size){
        const data=Buffer.from(buffered.subarray(0,size));
        buffered=Buffer.from(buffered.subarray(size));
        pendingReads.shift();sendReadReply(request,data,false);data.fill(0);
      }else if(eof){
        pendingReads.shift();
        if(buffered.length){
          const data=buffered;buffered=Buffer.alloc(0);sendReadReply(request,data,false);data.fill(0);
        }else sendReadReply(request,Buffer.alloc(0),true);
      }else break;
    }
  };
  output.on('data',wire=>{
    const request=JSON.parse(wire.toString().slice(0,-1));requests.push(request);
    switch(request.method){
      case 'Target.createTarget':writeReply(request,{targetId:TARGET});break;
      case 'Target.attachToTarget':writeReply(request,{sessionId:SESSION});break;
      case 'Fetch.enable':case 'Fetch.disable':writeReply(request,{});break;
      case 'Fetch.takeResponseBodyAsStream':writeReply(request,{stream:HANDLE});break;
      case 'IO.read':pendingReads.push(request);drainReads();break;
      case 'IO.close':streamClosed=true;writeReply(request,{});break;
      case 'Target.closeTarget':closedTarget=true;writeReply(request,{success:true});break;
      case 'Target.getTargets':writeReply(request,{targetInfos:closedTarget?[]:[{targetId:TARGET,type:'page'}]});break;
      case 'Browser.getVersion':writeReply(request,{protocolVersion:'1.3',product:'synthetic-peer'});break;
      default:writeReply(request,{});
    }
  });
  return {
    transport,requests,
    emitPaused(){input.write(JSON.stringify({method:'Fetch.requestPaused',sessionId:SESSION,params:{requestId:REQUEST,responseStatusCode:200,responseHeaders:[]}})+'\0');},
    enqueue(data){const next=Buffer.from(data);const previous=buffered;buffered=Buffer.concat([previous,next]);previous.fill(0);next.fill(0);drainReads();},
    signalEOF(){eof=true;drainReads();},
    get pendingReadCount(){return pendingReads.length;},
    get lateReadReplies(){return lateReadReplies;},
  };
}

async function prepare(f){
  const owned=f.transport.createInstagramOwnedScope();
  const target=await owned.call('Target.createTarget',{url:'about:blank',background:true,forTab:false},'',1000);
  const attachment=await owned.call('Target.attachToTarget',{targetId:target.targetId,flatten:true},'',1000);
  const readTimeouts=[];
  const page={
    on:listener=>owned.on(listener),
    send:(method,params,timeoutMS,late)=>{
      if(method==='IO.read')readTimeouts.push(timeoutMS);
      return owned.call(method,params,attachment.sessionId,timeoutMS,late);
    },
  };
  const trace=createIGTrace();
  const scope=createOwnedInstagramBootstrapScope(page,trace);
  await scope.call('Fetch.enable',{patterns:[{urlPattern:HOME,resourceType:'Document',requestStage:'Response'}]},'owned',1000);
  f.emitPaused();
  const stream=await scope.call('Fetch.takeResponseBodyAsStream',{requestId:REQUEST},'owned',1000);
  assert.equal(stream.stream,HANDLE);
  return {owned,scope,trace,target:target.targetId,session:attachment.sessionId,readTimeouts};
}

async function retireOwned(f,prepared){
  // These assertions cover the production scope's client-side ownership
  // ledger against synthetic acknowledgements, not Chrome's internal state.
  const retired=await prepared.scope.retire(1000);
  assert.equal(retired,false,'an unreplayed stream must await exact target retirement');
  assert.equal(f.requests.some(item=>item.method==='Fetch.disable'),false);
  assert.equal(prepared.scope.cleanupVerified(),true,'the sealed scope ledger should have no pending stream');
  await prepared.owned.call('Target.closeTarget',{targetId:prepared.target},'',1000);
  const inventory=await prepared.owned.call('Target.getTargets',{},'',1000);
  assert.deepEqual(inventory.targetInfos,[]);
  prepared.owned.releaseSession(prepared.session);
  prepared.owned.releaseTarget(prepared.target);
  assert.equal(prepared.owned.cleanupVerified(),true,'the owned-scope ledger should have no target, session, or stream');
  prepared.scope.dispose();prepared.owned.dispose();
}

function readCount(f){return f.requests.filter(request=>request.method==='IO.read').length;}
function closeCount(f){return f.requests.filter(request=>request.method==='IO.close').length;}

test('progressive bytes below the requested size time out at the 3s cap without leaking or replaying',async t=>{
  const f=syntheticPeer(t),prepared=await prepare(f),partial=Buffer.from('buffered-before-timeout');
  const started=Date.now();prepared.trace.note('body_read');
  const reading=readInstagramBootstrapBodyStream(prepared.scope,'owned',HANDLE,{deadlineAt:Date.now()+10000,trace:prepared.trace});
  await waitFor(()=>readCount(f)>=1,'first IO.read request');
  const request=f.requests.find(item=>item.method==='IO.read');
  assert.equal(request.params.size,CHUNK_BYTES);
  f.enqueue(partial);
  assert.equal(f.pendingReadCount,1,'a partial buffer must leave the synthetic peer read pending');
  assert.equal(f.requests.filter(item=>item.method==='IO.read').length,1);

  const result=await reading;
  assert.ok(Date.now()-started>=2900,'the production 3000ms local cap must expire the pending read');
  assert.deepEqual(prepared.readTimeouts,[3000],'the long overall deadline must still use the production 3s IO.read cap');
  assert.equal(result.reason,'body_read');
  assert.equal(result.cleanup,'verified');
  assert.equal(result.body,undefined,'timed-out partial bytes must not escape the reader');
  assert.equal(prepared.trace.snapshot().cause.condition,'read_command_timeout');
  assert.equal(closeCount(f),1,'the acquired stream should receive one IO.close');
  assert.equal(f.requests.some(item=>item.method==='Fetch.fulfillRequest'),false,'this reader-only test must not issue a replay command');

  // The synthetic peer deliberately returns the old partial read after close.
  // The transport has retired that local request, so the late frame is ignored.
  f.signalEOF();
  assert.equal(f.lateReadReplies,1);
  assert.equal(result.body,undefined);
  assert.equal(f.transport.alive(),true);
  await retireOwned(f,prepared);
  const control=await f.transport.call('Browser.getVersion',{},'',1000);
  assert.equal(control.protocolVersion,'1.3','a normal shared-transport control call should still succeed');
  assert.equal(f.transport.alive(),true);
  partial.fill(0);
});

test('a complete first read does not leak when the partial final read times out before EOF',async t=>{
  const f=syntheticPeer(t),prepared=await prepare(f);
  const complete=Buffer.alloc(CHUNK_BYTES,0x62),tail=Buffer.from('tail-after-complete-chunk');
  const started=Date.now();prepared.trace.note('body_read');
  const reading=readInstagramBootstrapBodyStream(prepared.scope,'owned',HANDLE,{deadlineAt:Date.now()+10000,trace:prepared.trace});
  await waitFor(()=>readCount(f)>=1,'first IO.read request');
  f.enqueue(complete);
  await waitFor(()=>readCount(f)>=2,'second IO.read request');
  f.enqueue(tail);
  assert.equal(f.pendingReadCount,1,'the second read should remain pending with only a partial tail and no EOF');

  const result=await reading;
  assert.ok(Date.now()-started>=2900,'the partial second read must reach the production 3s timeout');
  assert.deepEqual(prepared.readTimeouts,[3000,3000]);
  assert.equal(result.reason,'body_read');
  assert.equal(result.cleanup,'verified');
  assert.equal(result.body,undefined,'neither the complete first chunk nor the partial tail may escape after a later read fails');
  assert.equal(prepared.trace.snapshot().cause.condition,'read_command_timeout');
  assert.equal(closeCount(f),1);
  assert.equal(f.requests.some(item=>item.method==='Fetch.fulfillRequest'),false,'this reader-only test must not issue a replay command');

  // The synthetic peer returns the buffered tail only after IO.close. The
  // timed-out request is already retired from the shared transport ledger.
  f.signalEOF();
  assert.equal(f.lateReadReplies,1);
  assert.equal(result.body,undefined);
  assert.equal(f.transport.alive(),true);
  await retireOwned(f,prepared);
  const control=await f.transport.call('Browser.getVersion',{},'',1000);
  assert.equal(control.protocolVersion,'1.3');
  assert.equal(f.transport.alive(),true);
  complete.fill(0);tail.fill(0);
});

test('a full chunk followed by a delayed partial EOF reconstructs the exact body',async t=>{
  const f=syntheticPeer(t),prepared=await prepare(f);
  const firstPrefix=Buffer.from('progressive-prefix:');
  const firstChunk=Buffer.concat([firstPrefix,Buffer.alloc(CHUNK_BYTES-firstPrefix.length,0x61)]);
  const tail=Buffer.from('final-partial-tail');
  prepared.trace.note('body_read');
  const reading=readInstagramBootstrapBodyStream(prepared.scope,'owned',HANDLE,{deadlineAt:Date.now()+5000,trace:prepared.trace});
  await waitFor(()=>readCount(f)>=1,'first IO.read request');
  f.enqueue(firstPrefix);
  assert.equal(f.pendingReadCount,1,'bytes below the requested size must not complete an open read');
  f.enqueue(firstChunk.subarray(firstPrefix.length));
  await waitFor(()=>readCount(f)>=2,'second IO.read request');
  f.enqueue(tail);
  assert.equal(f.pendingReadCount,1,'a final partial chunk must wait until EOF');
  await wait(40);
  assert.equal(readCount(f),2,'the peer should not answer a partial read before EOF');
  f.signalEOF();

  const result=await reading;
  assert.deepEqual(prepared.readTimeouts,[3000,3000,3000]);
  assert.equal(result.reason,'none');
  assert.equal(result.cleanup,'verified');
  assert.deepEqual(result.body,Buffer.concat([firstChunk,tail]),'buffered bytes and final partial tail should be reconstructed exactly');
  assert.equal(readCount(f),3,'EOF after buffered bytes should be observed by a subsequent empty read');
  assert.deepEqual(f.requests.filter(item=>item.method==='IO.read').map(item=>item.params.size),[CHUNK_BYTES,CHUNK_BYTES,CHUNK_BYTES]);
  assert.equal(closeCount(f),1);
  assert.equal(f.requests.some(item=>item.method==='Fetch.fulfillRequest'),false,'this reader-only test must not issue a replay command');
  assert.equal(f.transport.alive(),true);
  result.body.fill(0);firstChunk.fill(0);tail.fill(0);firstPrefix.fill(0);
  await retireOwned(f,prepared);
  const control=await f.transport.call('Browser.getVersion',{},'',1000);
  assert.equal(control.protocolVersion,'1.3');
});

// Run the canonical bootstrap DOM/wire fixture through the production CDP
// framing, local timeouts, owned page and parent attempt cleanup.
async function acquisitionPeer(t,{respond,now=Date.now,budgetMS=30000,beforeEvent=()=>{}}={}){
  const canonical=bootstrapFixture(),input=new PassThrough(),output=new PassThrough(),requests=[],readTimeouts=[];
  const transport=createMainCDP({input,output});t.after(()=>transport.close());
  canonical.raw.subscribe(event=>{beforeEvent(event);input.write(JSON.stringify(event)+'\0');});
  const answer=(q,result)=>input.write(JSON.stringify({id:q.id,...(q.sessionId?{sessionId:q.sessionId}:{}),result})+'\0');
  output.on('data',wire=>{
    const q=JSON.parse(wire.toString().slice(0,-1));requests.push(q);
    const normal=()=>q.method==='Browser.getVersion'?{product:'synthetic-control'}:
      q.method==='Network.getCookies'?{cookies:['csrftoken','ds_user_id','sessionid'].map(name=>({name,value:name==='ds_user_id'?'123':'synthetic',domain:'.instagram.com',path:'/',secure:true,httpOnly:name==='sessionid',expires:-1}))}:
      canonical.raw.call(q.method,q.params,q.sessionId,1000);
    void Promise.resolve().then(()=>respond?respond(q,normal):normal()).then(result=>answer(q,result),()=>{
      input.write(JSON.stringify({id:q.id,...(q.sessionId?{sessionId:q.sessionId}:{}),error:{code:-32000,message:'synthetic failure'}})+'\0');
    });
  });
  const nonce='a'.repeat(48),attempt=createNativeSourceAttempt({source:'instagram',generation:1,nonce,profileScope:'synthetic',deadlineMS:Date.now()+60000});
  const nativePage=await attempt.own('page',()=>createNativeOwnedPage(transport,1000),page=>page.close());
  const page={...nativePage,send:(method,params,ms,late)=>{
    if(method==='IO.read')readTimeouts.push(ms);
    return nativePage.send(method,params,ms,late);
  }};
  return {canonical,transport,requests,readTimeouts,attempt,page,nonce,
    acquire:()=>acquireOwnedInstagramIdentity(page,nonce,{now,budgetMS}),
    async retire(expected='verified'){
      assert.equal((await attempt.close()).cleanup,expected);
      assert.deepEqual(requests.filter(q=>q.method==='Target.closeTarget').map(q=>q.params.targetId),['owned-target']);
      assert.ok(requests.some(q=>q.method==='Target.getTargets'));
      assert.equal(transport.privateTarget('owned-target'),false);
      assert.equal(transport.privateSession('owned-session'),false);
      assert.equal((await transport.call('Browser.getVersion',{},'',1000)).product,'synthetic-control');
      assert.equal(transport.alive(),true);
    },
  };
}

test('full reader and collector acquisitions accept 1-3s delayed EOF and preserve original delivery time',async t=>{
  await Promise.all([['reader',1200],['collector',2400]].map(async([role,delay])=>{
    const deliveredAt=10100;let clock=10000;
    const f=await acquisitionPeer(t,{
      beforeEvent:event=>{if(event.method==='Network.responseReceivedExtraInfo')clock=deliveredAt;else if(event.method==='Fetch.requestPaused')clock=deliveredAt+100;},
      respond:async(q,normal)=>{if(q.method==='IO.read'){await wait(delay);clock+=delay;}return normal();},
    });
    const result=await readInstagramEvidence(f.page,f.attempt,role,role,role,'synthetic',()=>clock);
    assert.equal(result.proof.role,role);
    assert.equal(result.proof.observedAtMS,deliveredAt);
    assert.equal(clock-result.proof.observedAtMS,100+delay);
    assert.deepEqual(f.readTimeouts,[3000]);
    assert.equal(f.requests.filter(q=>q.method==='Fetch.fulfillRequest').length,1);
    assert.equal(f.requests.filter(q=>q.method==='Fetch.disable').length,1);
    await f.retire();
  }));
});

test('full acquisition keeps a fixed freshness-reserved deadline across chunks',async t=>{
  let time=10000,reads=0;
  const f=await acquisitionPeer(t,{now:()=>time,respond:async(q,normal)=>{
    if(q.method==='IO.read'){
      reads++;time+=reads===1?2500:1500;
      return reads===1?{data:'partial',eof:false}:{data:'tail',eof:true};
    }
    return normal();
  }});
  const result=await f.acquire();
  assert.equal(result.diagnostic.reason,'body_deadline');assert.equal(result.diagnostic.cleanup,'uncertain');assert.equal(result.accountId,'');
  assert.deepEqual(f.readTimeouts,[3000,1500],'later chunks use the same delivery+4000 deadline');
  assert.equal(f.requests.some(q=>['Fetch.fulfillRequest','Fetch.disable'].includes(q.method)),false);
  await f.retire();
});

test('existing acquisition deadline can be earlier than the freshness reserve',async t=>{
  let time=10000;
  const f=await acquisitionPeer(t,{now:()=>time,budgetMS:7000,respond:async(q,normal)=>{
    if(q.method==='IO.read'){time+=1000;return {data:'complete',eof:true};}return normal();
  }});
  const result=await f.acquire();
  assert.equal(result.diagnostic.reason,'body_deadline');assert.deepEqual(f.readTimeouts,[1000]);
  assert.equal(f.requests.some(q=>q.method==='Fetch.disable'),false);await f.retire();
});

for(const clock of ['exhausted','fractional','backward','invalid'])test('full acquisition fails closed before stream allocation on '+clock+' body clock',async t=>{
  let time=10000;
  const f=await acquisitionPeer(t,{now:()=>time});
  // The delivery observer runs after acquisition's trusted timestamp capture.
  f.canonical.raw.subscribe(event=>{if(event.method==='Network.responseReceivedExtraInfo')time=clock==='exhausted'?14000:clock==='fractional'?10000.5:clock==='backward'?9999:NaN;});
  const result=await f.acquire();assert.equal(result.diagnostic.reason,'body_deadline');assert.equal(result.diagnostic.cleanup,'uncertain');
  assert.equal(f.requests.some(q=>['Fetch.takeResponseBodyAsStream','Fetch.fulfillRequest','Fetch.disable'].includes(q.method)),false);
  await f.retire();
});

test('full acquisition timeout has no retry, replay or Fetch.disable and late reply leaves transport usable',async t=>{
  let release,readStarted;
  const started=new Promise(resolve=>{readStarted=resolve;});
  const f=await acquisitionPeer(t,{respond:async(q,normal)=>{
    if(q.method==='IO.read'){readStarted();await new Promise(resolve=>{release=resolve;});}return normal();
  }});
  const acquiring=f.acquire();await started;
  const result=await acquiring;
  assert.equal(result.diagnostic.reason,'body_read');assert.equal(result.diagnostic.cleanup,'uncertain');assert.equal(result.accountId,'');
  assert.deepEqual(f.readTimeouts,[3000]);
  assert.equal(f.requests.filter(q=>q.method==='IO.read').length,1);
  assert.equal(f.requests.some(q=>['Fetch.fulfillRequest','Fetch.disable'].includes(q.method)),false);
  release();await wait(10);await f.retire();
});

test('full acquisition uncertain fulfillment never disables Fetch or produces proof',async t=>{
  const f=await acquisitionPeer(t,{respond:async(q,normal)=>{
    if(q.method==='Fetch.fulfillRequest'){await normal();throw Error('synthetic lost acknowledgement');}return normal();
  }});
  const result=await f.acquire();
  assert.equal(result.accountId,'');assert.equal(result.diagnostic.cleanup,'uncertain');
  assert.equal(f.requests.filter(q=>q.method==='Fetch.fulfillRequest').length,1);
  assert.equal(f.requests.some(q=>q.method==='Fetch.disable'),false);await f.retire();
});

test('a late stream after full acquisition timeout is closed before independent target retirement',async t=>{
  let release;
  const f=await acquisitionPeer(t,{respond:async(q,normal)=>{
    if(q.method==='Fetch.takeResponseBodyAsStream')await new Promise(resolve=>{release=resolve;});return normal();
  }});
  const result=await f.acquire();
  assert.equal(result.diagnostic.cleanup,'uncertain');assert.equal(result.accountId,'');
  assert.equal(f.requests.some(q=>q.method==='Fetch.disable'),false);
  release();await waitFor(()=>f.requests.some(q=>q.method==='IO.close'),'late stream close');
  await wait(10);
  assert.equal(result.diagnostic.cleanup,'uncertain','late close cannot rewrite a published acquisition result');
  assert.equal(f.requests.some(q=>['IO.read','Fetch.fulfillRequest','Fetch.disable'].includes(q.method)),false);
  await f.retire();
});

test('full acquisition does not disable an unreplayed navigation rejected before take',async t=>{
  const f=await acquisitionPeer(t);
  f.canonical.raw.subscribe(event=>{if(event.method==='Network.requestWillBeSent')f.canonical.emit('Network.requestServedFromCache',{requestId:'root-request'});});
  const result=await f.acquire();assert.equal(result.diagnostic.reason,'delivery_unproven');assert.equal(result.diagnostic.cleanup,'uncertain');
  assert.equal(f.requests.some(q=>['Fetch.takeResponseBodyAsStream','Fetch.disable'].includes(q.method)),false);await f.retire();
});

test('cancelled full acquisition retains pending late stream cleanup and never upgrades uncertain state',async t=>{
  let release,takeStarted;
  const started=new Promise(resolve=>{takeStarted=resolve;});
  const f=await acquisitionPeer(t,{respond:async(q,normal)=>{
    if(q.method==='Fetch.takeResponseBodyAsStream'){takeStarted();await new Promise(resolve=>{release=resolve;});}return normal();
  }});
  const reading=readInstagramEvidence(f.page,f.attempt,'reader','reader','reader','synthetic');
  await started;
  const closing=f.attempt.close();
  await assert.rejects(reading,error=>nativeSourceDiagnostic(error)?.reason==='canceled');
  assert.equal((await closing).cleanup,'uncertain');
  assert.equal(f.requests.some(q=>q.method==='Fetch.disable'),false);
  release();await wait(20);
  // After target retirement the page capability is closed. A late allocation
  // cannot be treated as acknowledged stream cleanup or make the attempt live.
  assert.equal(f.attempt.status().cleanup,'uncertain');
  assert.equal(f.requests.some(q=>q.method==='Fetch.fulfillRequest'),false);
  assert.equal(f.requests.some(q=>q.method==='Fetch.disable'),false);
  await f.retire('uncertain');
});

test('Fetch can be safely disabled before navigation or stream acquisition',async t=>{
  const f=syntheticPeer(t),owned=f.transport.createInstagramOwnedScope();
  const {targetId}=await owned.call('Target.createTarget',{url:'about:blank',background:true,forTab:false},'',1000);
  const {sessionId}=await owned.call('Target.attachToTarget',{targetId,flatten:true},'',1000);
  const scope=createOwnedInstagramBootstrapScope({on:fn=>owned.on(fn),send:(method,params,ms,late)=>owned.call(method,params,sessionId,ms,late)},createIGTrace());
  await scope.call('Fetch.enable',{patterns:[{urlPattern:HOME,resourceType:'Document',requestStage:'Response'}]},'owned',1000);
  assert.equal(await scope.retire(1000),true);
  assert.equal(f.requests.filter(q=>q.method==='Fetch.disable').length,1);
  scope.dispose();await owned.call('Target.closeTarget',{targetId},'',1000);
  assert.deepEqual((await owned.call('Target.getTargets',{},'',1000)).targetInfos,[]);
  owned.releaseSession(sessionId);owned.releaseTarget(targetId);owned.dispose();
});

async function collectorAcquisitionPeer(t,respond){
  const canonical=bootstrapFixture(),requests=[],readTimeouts=[],pending=new Map(),closed=[];
  const normal=q=>['Page.addScriptToEvaluateOnNewDocument','Emulation.setFocusEmulationEnabled','Emulation.setDeviceMetricsOverride'].includes(q.method)?{}:
    canonical.raw.call(q.method,q.params,q.sessionId,1000);
  const sender=createChromeCommandSender({pending,retainLateResources:true,isClosed:()=>false,write:(wire,callback)=>{
    const q=JSON.parse(wire.slice(0,-1));requests.push(q);callback();
    void Promise.resolve().then(()=>respond?respond(q,()=>normal(q)):normal(q)).then(result=>settleChromeCommandReply(pending,{id:q.id,result}),()=>settleChromeCommandReply(pending,{id:q.id,error:{message:'synthetic collector failure'}}));
  }});
  const send=(method,params,session,ms,late)=>{if(method==='IO.read')readTimeouts.push(ms);return sender(method,params,session,ms,late);};
  t.after(()=>{for(const entry of pending.values())clearTimeout(entry.timer);pending.clear();});
  const attempt=createNativeSourceAttempt({source:'instagram',generation:1,nonce:'a'.repeat(48),profileScope:'synthetic-collector',deadlineMS:Date.now()+60000});
  const page=await attempt.own('collector-page',()=>createTemporaryPage({send,subscribe:canonical.raw.subscribe,browserSessionId:'browser',timeoutMs:1000,retainLateResources:true,
    closeTargetAndWait:async targetId=>{
      closed.push(targetId);await send('Target.closeTarget',{targetId},'browser',1000);
      const inventory=await send('Target.getTargets',{},'browser',1000);
      assert.deepEqual(inventory.targetInfos,[{targetId:'user-owned-reader'}]);
    }}),page=>page.close());
  const trace=createIGTrace();
  return {requests,readTimeouts,pending,page,attempt,closed,trace,
    acquire:()=>acquireOwnedInstagramIdentity(page,'a'.repeat(48),{trace,now:Date.now}),
    async retire(expected='verified'){
      assert.equal((await attempt.close()).cleanup,expected);
      assert.deepEqual(closed,['owned-target']);
      assert.equal(requests.some(q=>q.method==='Target.getTargets'),true);
    },
  };
}

test('actual collector temporary page accepts delayed EOF with confirmed replay and verified retirement',async t=>{
  const f=await collectorAcquisitionPeer(t,async(q,normal)=>{if(q.method==='IO.read')await wait(1200);return normal();});
  const result=await f.acquire();assert.equal(result.accountId,'123');assert.equal(result.diagnostic.cleanup,'verified');
  assert.ok(Date.now()-result.observedAtMS>=1180);assert.deepEqual(f.readTimeouts,[3000]);
  assert.equal(f.requests.filter(q=>q.method==='Fetch.fulfillRequest').length,1);
  assert.equal(f.requests.filter(q=>q.method==='Fetch.disable').length,1);await f.retire();
});

test('actual collector read timeout preserves generic provenance and avoids retry, replay and disable',async t=>{
  let release;
  const f=await collectorAcquisitionPeer(t,async(q,normal)=>{if(q.method==='IO.read')await new Promise(resolve=>{release=resolve;});return normal();});
  const result=await f.acquire();assert.equal(result.accountId,'');assert.equal(result.diagnostic.reason,'body_read');assert.equal(result.diagnostic.cleanup,'uncertain');
  assert.deepEqual(f.readTimeouts,[3000]);assert.equal(f.trace.snapshot().cause.condition,'read_command_failed');
  assert.equal(f.requests.filter(q=>q.method==='IO.read').length,1);
  assert.equal(f.requests.some(q=>['Fetch.fulfillRequest','Fetch.disable'].includes(q.method)),false);
  release();await wait(10);await f.retire();
});

test('actual collector pending stream closes late without falsely verifying acquisition cleanup',async t=>{
  let release;
  const f=await collectorAcquisitionPeer(t,async(q,normal)=>{if(q.method==='Fetch.takeResponseBodyAsStream')await new Promise(resolve=>{release=resolve;});return normal();});
  const result=await f.acquire();assert.equal(result.accountId,'');assert.equal(result.diagnostic.cleanup,'uncertain');
  assert.equal(f.requests.some(q=>q.method==='Fetch.disable'),false);
  release();await waitFor(()=>f.requests.some(q=>q.method==='IO.close'),'collector late stream close');await wait(10);
  assert.equal(result.diagnostic.cleanup,'uncertain');
  assert.deepEqual(f.requests.filter(q=>q.method==='IO.close').map(q=>q.params.handle),['owned-stream']);
  await f.retire();
});

test('actual collector cancellation retires exact target and retains uncertainty after late stream close',async t=>{
  let release,takeStarted;
  const started=new Promise(resolve=>{takeStarted=resolve;});
  const f=await collectorAcquisitionPeer(t,async(q,normal)=>{if(q.method==='Fetch.takeResponseBodyAsStream'){takeStarted();await new Promise(resolve=>{release=resolve;});}return normal();});
  const acquiring=f.attempt.run('evidence',()=>f.acquire());
  await started;const closing=f.attempt.close();
  await assert.rejects(acquiring,error=>nativeSourceDiagnostic(error)?.reason==='canceled');
  assert.equal((await closing).cleanup,'uncertain');
  release();await waitFor(()=>f.requests.some(q=>q.method==='IO.close'),'retired collector late stream close');await wait(10);
  assert.equal(f.requests.some(q=>['IO.read','Fetch.fulfillRequest','Fetch.disable'].includes(q.method)),false);
  await f.retire('uncertain');
});

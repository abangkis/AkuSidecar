import test from 'node:test';
import assert from 'node:assert/strict';
import {fileURLToPath} from 'node:url';
import {captureNativeXTarget,validateManualCaptureRequest,manualCaptureRPCCode,manualCaptureRPCDiagnostic,manualCaptureDiagnostic,manualCaptureFailure,MANUAL_CAPTURE_COMMAND_TIMEOUT_MS} from './x-native-manual-capture.mjs';
import {executeManualReply} from './x-native-metadata-probe-driver.mjs';
import {syntheticCaptureTransport,syntheticCapturePort} from './x-native-manual-capture-fixture.mjs';
import {MANUAL_TIMING} from './x-manual-timing-policy.mjs';
import {CaptureTrafficDiagnostics} from './x-capture-traffic-diagnostics.mjs';
import {HeaderSafetyGuard} from './x-manual-header-safety.mjs';
import {projectCaptureWorkerDiagnostic} from './x-capture-worker-diagnostic.mjs';
import {performance} from 'node:perf_hooks';
const bridgePath=fileURLToPath(new URL('../../../AkuBridge',import.meta.url));
const pins={ownerId:'4102',profileId:'synthetic-collector',contextId:'proof-context',targetId:'proof-target',frameId:'proof-frame',documentId:'proof-document'};
const request=()=>({correlation:'synthetic-capture',pins,deadlineMS:Date.now()+10000,payload:{scrolls:0,maxBlocksPerSnapshot:2}});

test('native hydration uses the shared policy and cannot extend the original authorization',()=>{
  const now=Date.now(),q=request();q.deadlineMS=now+2000;
  assert.equal(validateManualCaptureRequest(request()).sourceHydrationTimeoutMs,MANUAL_TIMING.captureHydrationMS);
  const payload=validateManualCaptureRequest(q,now);
  assert.equal(payload.sourceHydrationTimeoutMs,2000);assert.equal(payload.captureTimeoutMs,2000);
  q.payload.sourceHydrationTimeoutMs=9000;
  assert.equal(validateManualCaptureRequest(q,now).sourceHydrationTimeoutMs,2000);
  q.deadlineMS=now+999;
  assert.throws(()=>validateManualCaptureRequest(q,now));
  q.deadlineMS=now+10000;q.payload.sourceHydrationTimeoutMs='3000';
  assert.throws(()=>validateManualCaptureRequest(q,now));
});

test('a delayed synthetic post fails the former one-second wait and passes the configured wait',{timeout:12000},async()=>{
  for(const hydration of [1000,undefined]){
    const transport=syntheticCaptureTransport(),query=transport.dom.context.document.querySelectorAll;
    const command=transport.command.bind(transport);let navigated=Infinity;
    transport.dom.context.document.querySelectorAll=(...args)=>performance.now()-navigated<2300?[]:query(...args);
    const send=async(method,params,...rest)=>{if(method==='Page.navigate')navigated=performance.now();return command(method,params,...rest);};
    const q=request();if(hydration)q.payload.sourceHydrationTimeoutMs=hydration;
    const capturing=captureNativeXTarget({request:q,bridgePath,command:send,subscribe:transport.subscribe.bind(transport),browserContextId:'owned'});
    if(hydration)await assert.rejects(capturing,error=>{
      const d=manualCaptureRPCDiagnostic(error);
      assert.equal(d.reason,'empty_unverified');assert.equal(d.worker.phase,'no_posts');
      assert.equal(d.worker.samples[0].candidateCount,0);return true;
    });
    else assert.equal(JSON.parse((await capturing).observationJSON).snapshots[0].blocks.length,1);
    assert.equal(transport.closed,true);
  }
});

test('worker candidate rejections reach the private reply despite later cleanup failure',{timeout:5000},async()=>{
  const transport=syntheticCaptureTransport(),query=transport.dom.context.document.querySelectorAll;
  for(const article of query('article[data-testid="tweet"]')){
    const own=article.querySelectorAll;article.querySelectorAll=s=>s==='time'?[]:own(s);
  }
  const q=request();q.payload.sourceHydrationTimeoutMs=1000;
  const command=async(method,...args)=>{const value=await transport.command(method,...args);if(method==='Target.closeTarget')throw Error('PRIVATE_CLEANUP');return value;};
  let reply;
  await executeManualReply({id:77,op:'manual_capture'},()=>captureNativeXTarget({request:q,bridgePath,command,subscribe:transport.subscribe.bind(transport),browserContextId:'owned'}),(...args)=>reply=args);
  const d=reply[4];assert.equal(reply[2],'manual_capture_rejected');assert.equal(d.stage,'worker');
  assert.equal(d.reason,'empty_unverified');assert.equal(d.cleanup,'failed');assert.equal(d.worker.phase,'no_posts');
  assert.equal(d.worker.samples[0].rejections.missing_identity,1);
  assert.equal(d.worker.samples[0].authenticatedUiObserved,true);
  assert.ok(Buffer.byteLength(JSON.stringify(d))<=2048);assert.equal(JSON.stringify(reply).includes('PRIVATE'),false);
});

test('worker telemetry projects closed keys without invoking getters or retaining private evidence',()=>{
  let reads=0;
  const sample={candidateCount:2,rejected:1,documentReady:false,authenticatedUiObserved:true,
    rejectionReasons:{missing_identity:1,PRIVATE_TOKEN:99},url:'PRIVATE_URL',body:'PRIVATE_BODY'};
  Object.defineProperty(sample,'loginRequired',{get(){reads++;return true;}});
  const error=Object.assign(Error('PRIVATE'),{code:'empty_unverified',diagnostics:{sampleCount:1,samples:[sample]}});
  const d=projectCaptureWorkerDiagnostic(error,{hydrationMS:3000,elapsedMS:1001});
  assert.equal(reads,0);assert.equal(d.samples[0].loginRequired,null);assert.equal(JSON.stringify(d).includes('PRIVATE'),false);
  assert.equal(Object.isFrozen(d.samples[0].rejections),true);
  assert.equal(projectCaptureWorkerDiagnostic(Error('PRIVATE'),{hydrationMS:3000,elapsedMS:1}),undefined);
  const empty=Object.assign(Error(),{code:'empty_unverified',diagnostics:{sampleCount:0,samples:[]}});
  assert.equal(projectCaptureWorkerDiagnostic(empty,{hydrationMS:3000,elapsedMS:3000}).phase,'snapshot_unavailable');
});

test('a worker with no verifiable snapshot reports a different phase from rejected candidates',{timeout:5000},async()=>{
  const transport=syntheticCaptureTransport(),command=transport.command.bind(transport);
  const q=request();q.payload.sourceHydrationTimeoutMs=1000;
  const send=async(method,params,...rest)=>{
    if(method==='Runtime.evaluate'&&params.expression.includes('XHeadlessPoC.collect()'))throw Error('Execution context was destroyed');
    return command(method,params,...rest);
  };
  await assert.rejects(captureNativeXTarget({request:q,bridgePath,command:send,subscribe:transport.subscribe.bind(transport),browserContextId:'owned'}),error=>{
    const d=manualCaptureRPCDiagnostic(error);assert.equal(d.reason,'empty_unverified');
    assert.equal(d.worker.phase,'snapshot_unavailable');assert.equal(d.worker.sampleCount,0);assert.deepEqual(d.worker.samples,[]);return true;
  });
  assert.equal(transport.closed,true);
});

for(const mode of ['orphan-response','orphan-extra','missing-flag','invalid-flag','extra-pending','unexpected-extra','response-pending','completion-pending','network-failed','redirect-reuse','capacity'])test(`ordinary ${mode} cannot revoke the capture owner`,async()=>{
  const transport=syntheticCaptureTransport({pendingExtra:mode==='extra-pending'});let invalidations=0,summary;
  const subscribe=listener=>transport.subscribe(raw=>{
    if(raw.method==='Network.requestWillBeSent'){
      if(mode==='orphan-response')listener({sessionId:raw.sessionId,method:'Network.responseReceived',params:{requestId:'PRIVATE_ORPHAN',hasExtraInfo:false,response:{headers:{}}}});
      if(mode==='orphan-extra'||mode==='unexpected-extra')listener({sessionId:raw.sessionId,method:'Network.responseReceivedExtraInfo',params:{requestId:mode==='orphan-extra'?'PRIVATE_ORPHAN':raw.params.requestId,headers:{}}});
      if(mode==='capacity')for(let i=0;i<1300;i++)listener({...raw,params:{...raw.params,requestId:'PRIVATE_'+i}});
    }
    if(mode==='response-pending'&&raw.method==='Network.responseReceived'||mode==='completion-pending'&&raw.method==='Network.loadingFinished')return;
    if(raw.method==='Network.responseReceived'&&['missing-flag','invalid-flag'].includes(mode)){const params={...raw.params};if(mode==='missing-flag')delete params.hasExtraInfo;else params.hasExtraInfo='PRIVATE_VALUE';listener({...raw,params});return;}
    listener(mode==='network-failed'&&raw.method==='Network.loadingFinished'?{...raw,method:'Network.loadingFailed'}:raw);
    if(mode==='redirect-reuse'&&raw.method==='Network.requestWillBeSent'){listener({...raw,params:{...raw.params,redirectResponse:{headers:{}}}});listener(raw);}
  });
  const result=await captureNativeXTarget({request:request(),bridgePath,command:transport.command.bind(transport),subscribe,browserContextId:'owned',onUnsafe:()=>{invalidations++;},onTrafficDiagnostic:value=>{summary=value;}});
  assert.equal(invalidations,0);assert.equal(transport.closed,true);assert.equal(summary.coverage,'partial');assert.equal(result.traffic,summary);
  assert.equal(JSON.stringify(summary).includes('PRIVATE'),false);assert.ok(JSON.parse(result.observationJSON).snapshots[0].blocks.length>0);
});

for(const kind of ['candidate','ambiguous','malformed','blocked'])test(`strict ${kind} header rejection survives ordinary diagnostic saturation and orphan correlation`,async()=>{
  const transport=syntheticCaptureTransport();let invalidations=0;
  const subscribe=listener=>transport.subscribe(raw=>{
    listener(raw);
    if(raw.method==='Network.requestWillBeSent'){
      for(let i=0;i<17000;i++)listener({...raw,params:{...raw.params,requestId:'PRIVATE_'+i}});
      const params={requestId:'PRIVATE_UNTRACKED',statusCode:200,headers:{}};
      if(kind==='candidate')params.headers={'set-cookie':'auth_token=synthetic; Path=/'};
      if(kind==='ambiguous')params.headers={'set-cookie':'auth_token=synthetic, ct0=synthetic'};
      if(kind==='malformed')params.headers={'set-cookie':17};
      if(kind==='blocked')params.blockedCookies=[{cookieLine:'ct0=synthetic; Path=/',blockedReasons:[]}];
      listener({sessionId:raw.sessionId,method:'Network.responseReceivedExtraInfo',params});
    }
  });
  await assert.rejects(captureNativeXTarget({request:request(),bridgePath,command:transport.command.bind(transport),subscribe,browserContextId:'owned',onUnsafe:()=>{invalidations++;}}),error=>{
    const d=manualCaptureDiagnostic(error);assert.equal(d.stage,'coverage');assert.ok(['cookie_candidate','cookie_ambiguous','header_guard'].includes(d.reason));if(['candidate','blocked'].includes(kind))assert.equal(d.reason,'cookie_candidate');assert.equal(d.cleanup,'completed');return true;
  });
  assert.ok(invalidations>0);assert.equal(transport.closed,true);
});

test('diagnostic observer, finalizer and sink failures cannot turn into capture rejection',async()=>{
  for(const method of ['observe','finish']){
    const original=CaptureTrafficDiagnostics.prototype[method];
    CaptureTrafficDiagnostics.prototype[method]=()=>{throw Error('PRIVATE_DIAGNOSTIC');};
    try{
      const transport=syntheticCaptureTransport();let invalidations=0;
      await captureNativeXTarget({request:request(),bridgePath,command:transport.command.bind(transport),subscribe:transport.subscribe.bind(transport),browserContextId:'owned',onUnsafe:()=>{invalidations++;},onTrafficDiagnostic:()=>{throw Error('PRIVATE_SINK');}});
      assert.equal(invalidations,0);
    }finally{CaptureTrafficDiagnostics.prototype[method]=original;}
  }
});

test('lifecycle loss, failed header inspection and unsafe-sink errors remain strict',async()=>{
  for(const [mode,reason]of [['detached','target_lost'],['inspection','header_guard'],['unsafe-sink','cookie_candidate']]){
    const original=HeaderSafetyGuard.prototype.observe,transport=syntheticCaptureTransport({cookie:mode==='unsafe-sink'});
    if(mode==='inspection')HeaderSafetyGuard.prototype.observe=()=>{throw Error('PRIVATE_INSPECTION');};
    try{
      const subscribe=listener=>transport.subscribe(raw=>{listener(raw);if(mode==='detached'&&raw.method==='Network.requestWillBeSent')listener({sessionId:raw.sessionId,method:'Inspector.detached',params:{}});});
      await assert.rejects(captureNativeXTarget({request:request(),bridgePath,command:transport.command.bind(transport),subscribe,browserContextId:'owned',onUnsafe:()=>{if(mode==='unsafe-sink')throw Error('PRIVATE_SINK');}}),error=>{
        assert.deepEqual(manualCaptureDiagnostic(error),{stage:'coverage',reason,cleanup:'completed'});return true;
      });
      assert.equal(transport.closed,true);
    }finally{HeaderSafetyGuard.prototype.observe=original;}
  }
});

test('ordinary partial traffic survives the actual owner port and leaves inspection usable',async()=>{
  const transport=syntheticCaptureTransport(),subscribe=transport.subscribe.bind(transport);
  transport.subscribe=listener=>subscribe(raw=>{
    if(raw.method==='Network.requestWillBeSent')listener({sessionId:raw.sessionId,method:'Network.responseReceivedExtraInfo',params:{requestId:'PRIVATE_ORPHAN',headers:{}}});
    listener(raw);
  });
  const port=await syntheticCapturePort(pins,transport,bridgePath);
  const result=await port.execute({id:30,op:'manual_capture',owner:1,role:'collector',handle:'collector-handle',request:request()});
  assert.equal(result.traffic.coverage,'partial');assert.ok(result.traffic.conditions.includes('orphan_extra'));
  const inspected=await port.execute({id:31,op:'manual_inspect',owner:1,role:'collector'});assert.equal(inspected.invalid,false);
});

test('capture authorization remains capped at one minute',()=>{
  assert.equal(MANUAL_CAPTURE_COMMAND_TIMEOUT_MS,MANUAL_TIMING.captureCommandMS);
  const now=100000,valid={...request(),deadlineMS:now+MANUAL_TIMING.attemptAuthorizationMaxMS};
  assert.doesNotThrow(()=>validateManualCaptureRequest(valid,now));
  assert.throws(()=>validateManualCaptureRequest({...valid,deadlineMS:now+MANUAL_TIMING.attemptAuthorizationMaxMS+1},now));
});

test('capture wire retains only closed challenge categories and a separate bounded result capacity',async()=>{
  for(const category of ['cloudflare','x_human_verification','PRIVATE_COOKIE']){
    const error=Object.assign(new Error('PRIVATE_BODY'),{code:'challenge_required',challengeCategory:category});
    let reply;await executeManualReply({id:1,op:'manual_capture'},async()=>{throw error;},(...args)=>reply=args);
    assert.equal(reply[2],`manual_capture_challenge_${category==='PRIVATE_COOKIE'?'unknown':category}`);
    assert.equal(JSON.stringify(reply).includes('PRIVATE'),false);
  }
  assert.equal(manualCaptureRPCCode(new Error('manual_capture_challenge_cloudflare')),'manual_capture_rejected');
  for(const op of ['manual_capture','manual_inspect']){
    let reply;await executeManualReply({id:1,op},async()=>({ok:true}),(...args)=>reply=args);
    assert.equal(reply[3],op==='manual_capture'?800*1024:65536);
  }
  const payload=validateManualCaptureRequest({...request(),payload:{scrollFraction:.5,scrollSettleMs:1400}});
  assert.equal(payload.scrollFraction,.5);assert.equal(payload.scrollSettleMs,1400);
});
test('production X worker and vendored parser execute on synthetic DOM',async()=>{
  const transport=syntheticCaptureTransport({websocket:true});const port=await syntheticCapturePort(pins,transport,bridgePath);
  const result=await port.execute({id:9,op:'manual_capture',owner:1,role:'collector',handle:'collector-handle',request:request()});
  const observation=JSON.parse(result.observationJSON),block=observation.snapshots[0].blocks[0];
  assert.equal(block.text,'Synthetic native worker parser evidence.');assert.equal(block.platformId,'x:status:1908123456789012346');assert.equal(observation.snapshots[0].adapterVersion,'x-dom-v23');assert.equal(transport.closed,true);
  await assert.rejects(port.execute({id:12,op:'manual_capture',owner:1,role:'collector',handle:'collector-handle',request:request()}));
  assert.equal(transport.events.filter(method=>method==='Target.createTarget').length,1);
});
test('owner inspection remains responsive during separate capture lane',async()=>{
  let release;const hold=new Promise(r=>release=r),transport=syntheticCaptureTransport({hold});const port=await syntheticCapturePort(pins,transport,bridgePath);
  const capturing=port.execute({id:10,op:'manual_capture',owner:1,role:'collector',handle:'collector-handle',request:request()});
  await new Promise(r=>setTimeout(r,20));const inspected=await port.execute({id:11,op:'manual_inspect',owner:1,role:'collector'});assert.equal(inspected.pins.targetId,pins.targetId);release();await capturing;
});
test('wrong scope, arbitrary scripts, expired deadline, parser failure, and cookie write fail closed',async()=>{
  for(const payload of [{script:'danger'},{pageUrl:'http://127.0.0.1/'},{source:'facebook'}])assert.throws(()=>validateManualCaptureRequest({...request(),payload}));
  assert.throws(()=>validateManualCaptureRequest({...request(),deadlineMS:Date.now()-1}));
  for(const mode of [{parserThrow:true},{cookie:true},{lateCookie:true}]){
    const transport=syntheticCaptureTransport(mode);
    await assert.rejects(captureNativeXTarget({request:request(),bridgePath,command:transport.command.bind(transport),subscribe:transport.subscribe.bind(transport),browserContextId:'owned'}));assert.equal(transport.closed,true);
  }
});
test('cancellation closes only the capture target and cannot deliver a late result',{timeout:5000},async()=>{
  let release;const hold=new Promise(r=>release=r),transport=syntheticCaptureTransport({hold});const port=await syntheticCapturePort(pins,transport,bridgePath);
  const capturing=port.execute({id:20,op:'manual_capture',owner:1,role:'collector',handle:'collector-handle',request:request()});
  await transport.targetCreated;await port.execute({id:21,op:'manual_capture_cancel',owner:1,role:'collector',handle:'collector-handle'});
  release();await assert.rejects(capturing);assert.equal(transport.closed,true);assert.equal(transport.events.includes('Browser.close'),false);
});

test('safe capture diagnostics distinguish setup, navigation, coverage and cleanup without raw errors',async()=>{
  for(const [mode,wantStage,wantReason,wantCleanup] of [
    ['create','target_create','unavailable','unknown'],
    ['navigation','navigation','unavailable','completed'],
    ['late-cookie','coverage','cookie_candidate','completed'],
    ['cleanup','cleanup','close_failed','failed'],
    ['navigation-cleanup','navigation','unavailable','failed'],
  ]){
    const transport=syntheticCaptureTransport({pendingExtra:mode==='extra',lateCookie:mode==='late-cookie'});
    const command=async(method,...args)=>{
      if(mode==='create'&&method==='Target.createTarget'||mode.startsWith('navigation')&&method==='Page.navigate')throw Error('PRIVATE_URL_COOKIE_BODY');
      const result=await transport.command(method,...args);
      if(mode.includes('cleanup')&&method==='Target.closeTarget')throw Error('PRIVATE_CLEANUP_PATH');
      return result;
    };
    await assert.rejects(captureNativeXTarget({request:request(),bridgePath,command,subscribe:transport.subscribe.bind(transport),browserContextId:'owned'}),error=>{
      const d=manualCaptureDiagnostic(error);
      assert.deepEqual(d,{stage:wantStage,reason:wantReason,cleanup:wantCleanup},mode);
      assert.equal(JSON.stringify(d).includes('PRIVATE'),false);
      assert.equal(error.message.includes('PRIVATE'),false);
      assert.equal(manualCaptureRPCCode(error),'manual_capture_rejected');
      assert.deepEqual(manualCaptureRPCDiagnostic(error),{schema:'aku.capture-diagnostic.v1',...d});
      return true;
    });
    if(mode!=='create')assert.equal(transport.events.filter(m=>m==='Target.closeTarget').length,1);
  }
});

test('worker challenge remains the first cause when cleanup and coverage also fail',async()=>{
  const transport=syntheticCaptureTransport();
  let caught;
  const command=async(method,...args)=>{
    if(method==='Page.navigate')throw Object.assign(Error('PRIVATE_CHALLENGE_BODY'),{code:'challenge_required',challengeCategory:'cloudflare'});
    const result=await transport.command(method,...args);
    if(method==='Target.closeTarget')throw Error('PRIVATE_CLEANUP');
    return result;
  };
  await assert.rejects(captureNativeXTarget({request:request(),bridgePath,command,subscribe:transport.subscribe.bind(transport),browserContextId:'owned'}),error=>{
    caught=error;
    assert.deepEqual(manualCaptureDiagnostic(error),{stage:'worker',reason:'challenge_cloudflare',cleanup:'failed'});
    assert.equal(manualCaptureRPCCode(error),'manual_capture_challenge_cloudflare');
    return true;
  });
  let reply;await executeManualReply({id:1,op:'manual_capture'},async()=>{throw caught;},(...args)=>reply=args);
  assert.equal(reply[2],'manual_capture_challenge_cloudflare');
  assert.deepEqual(reply[4],{schema:'aku.capture-diagnostic.v1',stage:'worker',reason:'challenge_cloudflare',cleanup:'failed'});
  assert.equal(JSON.stringify(reply).includes('PRIVATE'),false);
});

test('private diagnostic codes cannot be forged through strings, properties or getters',async()=>{
  let reads=0;
  const fake=Object.defineProperty(new Error('manual_capture_detail_worker__empty_unverified__completed'),'code',{get(){reads++;return 'challenge_required';}});
  assert.equal(manualCaptureRPCCode(fake),'manual_capture_rejected');assert.equal(reads,0);
  assert.equal(manualCaptureRPCDiagnostic(fake),undefined);
  const first=manualCaptureFailure(Error('PRIVATE'),'worker','empty_unverified');
  assert.equal(manualCaptureFailure(first,'cleanup','close_failed'),first);
  assert.deepEqual(manualCaptureDiagnostic(manualCaptureFailure(null,'PRIVATE','PRIVATE')),{stage:'unknown',reason:'unknown',cleanup:'unknown'});
  let reply;await executeManualReply({id:1,op:'manual_capture'},async()=>{throw first;},(...args)=>reply=args);
  assert.equal(reply[2],'manual_capture_rejected');
  assert.deepEqual(reply[4],{schema:'aku.capture-diagnostic.v1',stage:'worker',reason:'empty_unverified',cleanup:'unknown'});
});

test('inspection failure is unknown ownership while a confirmed pinned mismatch is owner_changed',async()=>{
  for(const mode of ['unavailable','mismatch']){
    let fault=false;
    const transport=syntheticCaptureTransport();
    const port=await syntheticCapturePort(pins,transport,bridgePath,{inspect:value=>{
      if(fault&&value.headless){if(mode==='unavailable')throw Error('PRIVATE transport error');return{...value,contextId:'different-context'};}
      return value;
    }});
    fault=true;
    await assert.rejects(port.execute({id:40,op:'manual_capture',owner:1,role:'collector',handle:'collector-handle',request:request()}),error=>{
      assert.deepEqual(manualCaptureDiagnostic(error),{stage:'owner_before',reason:mode==='unavailable'?'unavailable':'owner_changed',cleanup:'unknown'});return true;
    });
    assert.equal(transport.events.includes('Target.createTarget'),false);
  }
});

test('diagnostics preserve the production workers existing transient context recovery',async()=>{
  const transport=syntheticCaptureTransport();let injected=false;
  const command=async(method,params,...args)=>{
    if(method==='Runtime.evaluate'&&params.expression.includes('XHeadlessPoC.collect')&&!injected){injected=true;throw Error('Execution context was destroyed');}
    return transport.command(method,params,...args);
  };
  const result=await captureNativeXTarget({request:request(),bridgePath,command,subscribe:transport.subscribe.bind(transport),browserContextId:'owned'});
  assert.equal(injected,true);assert.equal(JSON.parse(result.observationJSON).snapshots[0].blocks.length,1);
});

test('production worker login rejection receives a closed category without page content',async()=>{
  const transport=syntheticCaptureTransport();
  const command=async(method,params,...args)=>{
    const result=await transport.command(method,params,...args);
    if(method==='Runtime.evaluate'&&params.expression.includes('XHeadlessPoC.collect')){
      const snapshot=JSON.parse(result.result.value);snapshot.loginRequired=true;result.result.value=JSON.stringify(snapshot);
    }
    return result;
  };
  await assert.rejects(captureNativeXTarget({request:request(),bridgePath,command,subscribe:transport.subscribe.bind(transport),browserContextId:'owned'}),error=>{
    assert.deepEqual(manualCaptureDiagnostic(error),{stage:'worker',reason:'login_required',cleanup:'completed'});return true;
  });
});

import test from 'node:test';
import assert from 'node:assert/strict';
import {fileURLToPath} from 'node:url';
import {captureNativeXTarget,validateManualCaptureRequest,manualCaptureRPCCode,manualCaptureRPCDiagnostic,manualCaptureDiagnostic,manualCaptureFailure} from './x-native-manual-capture.mjs';
import {executeManualReply} from './x-native-metadata-probe-driver.mjs';
import {syntheticCaptureTransport,syntheticCapturePort} from './x-native-manual-capture-fixture.mjs';
const bridgePath=fileURLToPath(new URL('../../../AkuBridge',import.meta.url));
const pins={ownerId:'4102',profileId:'synthetic-collector',contextId:'proof-context',targetId:'proof-target',frameId:'proof-frame',documentId:'proof-document'};
const request=()=>({correlation:'synthetic-capture',pins,deadlineMS:Date.now()+10000,payload:{scrolls:0,maxBlocksPerSnapshot:2}});

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
  for(const mode of [{parserThrow:true},{cookie:true},{pendingExtra:true},{lateCookie:true}]){
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
    ['extra','coverage','coverage_extra_pending','completed'],
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

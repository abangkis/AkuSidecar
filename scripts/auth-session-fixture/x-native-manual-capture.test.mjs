import test from 'node:test';
import assert from 'node:assert/strict';
import {fileURLToPath} from 'node:url';
import {captureNativeXTarget,validateManualCaptureRequest,manualCaptureRPCCode} from './x-native-manual-capture.mjs';
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

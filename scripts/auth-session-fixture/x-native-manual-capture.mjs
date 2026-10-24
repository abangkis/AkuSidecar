import {createHash} from 'node:crypto';
import {captureIsolated,validateCapture} from '../../internal/collection/headless/worker/capture.mjs';
import {sourceAssets} from '../../internal/collection/headless/worker/worker.mjs';
import {HeaderSafetyGuard} from './x-manual-header-safety.mjs';

const fail=()=>new Error('xnp_manual_capture_rejected');
const pinsKeys=['ownerId','profileId','contextId','targetId','frameId','documentId'];
const ref=v=>typeof v==='string'&&v.length>0&&v.length<=512&&!/[\u0000-\u001f\u007f]/u.test(v);
const exact=(v,keys)=>{if(!v||![Object.prototype,null].includes(Object.getPrototypeOf(v))||Reflect.ownKeys(v).length!==keys.length||Reflect.ownKeys(v).some(k=>!keys.includes(k)||!Object.hasOwn(Object.getOwnPropertyDescriptor(v,k),'value')))throw fail();};
export function validateManualCaptureRequest(request,now=Date.now()) {
  exact(request,['correlation','pins','deadlineMS','payload']);exact(request.pins,pinsKeys);
  if(!ref(request.correlation)||!pinsKeys.every(k=>ref(request.pins[k]))||!Number.isSafeInteger(request.deadlineMS)||request.deadlineMS<=now||request.deadlineMS>now+60000)throw fail();
  const allowed=['pageUrl','scrolls','maxBlocksPerSnapshot','sourceHydrationTimeoutMs'];
  if(!request.payload||![Object.prototype,null].includes(Object.getPrototypeOf(request.payload))||Object.keys(request.payload).some(k=>!allowed.includes(k)))throw fail();
  const left=request.deadlineMS-now;
  if(left<1000)throw fail();
  const payload={...request.payload,acquisitionRound:1,captureTimeoutMs:Math.floor(left),scrolls:request.payload.scrolls??0,
    sourceHydrationTimeoutMs:Math.min(request.payload.sourceHydrationTimeoutMs??1000,Math.floor(left)),
    pendingContentPolicy:'detect_only',sourceFreshnessPolicy:'preserve_frontier',sameTabMutationAllowed:false,restoreScroll:false};
  validateCapture('x',payload);
  return payload;
}

// A private capability on the existing owned collector pipe. No launcher, cookie
// operation, startup navigation, or raw RPC URL/script reaches this boundary.
export async function captureNativeXTarget({request,bridgePath,command,subscribe,browserContextId,proofSession,signal,onUnsafe=()=>{}}) {
  const payload=validateManualCaptureRequest(request);
  if(typeof command!=='function'||typeof subscribe!=='function'||!ref(browserContextId))throw fail();
  const assets=await sourceAssets(bridgePath,'x');
  let target,session,unsubscribe,guard,unsafe=false,closed=false;
  const requests=new Map();
  const expired=()=>signal?.aborted||Date.now()>=request.deadlineMS||unsafe;
  const close=async()=>{if(target&&!closed){closed=true;await command('Target.closeTarget',{targetId:target});}};
  const abort=()=>{void close().catch(()=>{});};
  const timer=setTimeout(abort,Math.max(1,request.deadlineMS-Date.now()));
  signal?.addEventListener('abort',abort,{once:true});
  try {
    if(expired())throw fail();
    const created=await command('Target.createTarget',{url:'about:blank',browserContextId});
    if(!ref(created.targetId)||created.targetId===request.pins.targetId)throw fail();
    target=created.targetId;
    session=(await command('Target.attachToTarget',{targetId:target,flatten:true})).sessionId;
    if(!ref(session)||session===proofSession)throw fail();
    await command('Page.enable',{},session);
    const frame=(await command('Page.getFrameTree',{},session)).frameTree?.frame?.id;
    if(!ref(frame))throw fail();
    const reject=()=>{unsafe=true;onUnsafe();abort();};
    guard=new HeaderSafetyGuard(frame,session,{onRejection:reject});guard.claim(frame,session);
    unsubscribe=subscribe(raw=>{
      if(raw.sessionId===session){
        if(['Inspector.detached','Inspector.targetCrashed'].includes(raw.method))reject();
        const id=raw.params?.requestId,known=requests.has(id),first=raw.method==='Network.requestWillBeSent'&&!known&&!raw.params?.redirectResponse;
        if(typeof id==='string'&&(known||raw.method==='Network.requestWillBeSent')){
          if(requests.size>=1280&&!known){reject();return;}
          if(!known)requests.set(id,{response:false,extra:false,finished:false,extraNeeded:null});
          const record=requests.get(id);
          if(raw.method==='Network.responseReceived'){record.response=true;record.extraNeeded=raw.params.hasExtraInfo;if(typeof record.extraNeeded!=='boolean')reject();}
          if(raw.method==='Network.responseReceivedExtraInfo')record.extra=true;
          if(raw.method==='Network.loadingFinished'||raw.method==='Network.loadingFailed')record.finished=true;
        }
        if(typeof id==='string'&&!known&&['Network.responseReceived','Network.responseReceivedExtraInfo'].includes(raw.method))reject();
        guard.observe(raw,{known,first});
      }
      if(raw.method==='Target.targetDestroyed'&&raw.params?.targetId===target&&!closed)reject();
    });
    await command('Network.enable',{maxTotalBufferSize:1000000,maxResourceBufferSize:100000,maxPostDataSize:0},session);
    const send=async(method,params={},timeoutMs=1500)=>{
      if(expired()||closed||!['Page.navigate','Runtime.evaluate','Page.addScriptToEvaluateOnNewDocument','Input.dispatchMouseEvent'].includes(method))throw fail();
      if(method==='Page.navigate'&&params.url!==validateCapture('x',payload).pageUrl)throw fail();
      if(method==='Runtime.evaluate'&&(typeof params.expression!=='string'||params.expression.length>1024*1024))throw fail();
      // Trusted worker assets generate expressions; none comes from the wire.
      const result=await command(method,params,session,Math.min(timeoutMs,request.deadlineMS-Date.now()));
      if(expired())throw fail();return result;
    };
    const page={send,navigate:(url,ms)=>send('Page.navigate',{url},ms),evaluate:async(expression,ms)=>{
      const result=await send('Runtime.evaluate',{expression,returnByValue:true,awaitPromise:true},ms);
      if(result.exceptionDetails)throw fail();return result.result?.value;
    }};
    const observation=await captureIsolated({backend:'headless_worker',forSource:async source=>{if(source!=='x')throw fail();return page;}},{x:assets},'x',payload);
    await command('Runtime.evaluate',{expression:'void 0',returnByValue:true},session);
    if(expired()||guard.rejection()!=='unknown')throw fail();
    const observationJSON=JSON.stringify(observation);
    if(Buffer.byteLength(observationJSON)>512*1024)throw fail();
    const wire={schema:'aku.x.manual-capture.v1',correlation:request.correlation,pins:request.pins,captureTarget:target,observationJSON,digest:createHash('sha256').update(observationJSON).digest('hex')};
    if(Buffer.byteLength(JSON.stringify(wire))>768*1024)throw fail();
    return wire;
  } finally {
    clearTimeout(timer);signal?.removeEventListener('abort',abort);
    try{await close();}finally{unsubscribe?.();}
    // A round trip is not evidence that ExtraInfo has arrived. Observed pending
    // response/header coverage closes the result even after target retirement.
    if(guard&&(unsafe||guard.rejection()!=='unknown'||[...requests.values()].some(r=>!r.finished||!r.response||r.extraNeeded===null||r.extraNeeded&&!r.extra||r.extra&&!r.extraNeeded)))throw fail();
  }
}

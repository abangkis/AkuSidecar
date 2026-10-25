import {createHash} from 'node:crypto';
import {captureIsolated,validateCapture} from '../../internal/collection/headless/worker/capture.mjs';
import {sourceAssets} from '../../internal/collection/headless/worker/worker.mjs';
import {HeaderSafetyGuard} from './x-manual-header-safety.mjs';

const fail=()=>new Error('xnp_manual_capture_rejected');
export const CAPTURE_STAGES=Object.freeze(['unknown','admission','behavior','coordinator','owner_before','payload','transport','assets','target_create','target_attach','observer','navigation','evaluation','worker','coverage','serialization','wire','observation','owner_after','cleanup']);
export const CAPTURE_REASONS=Object.freeze(['unknown','unavailable','invalid_request','deadline','canceled','owner_changed','scope_lost','request_budget','header_guard','coverage_pending','coverage_shape','extra_mismatch','network_failed','target_lost','method_rejected','evaluation_exception','invalid_payload','navigation_failed','empty_unverified','invalid_observation','worker_not_ready','target_unavailable','worker_other','envelope_invalid','digest_mismatch','observation_invalid','close_failed','source_paused','challenge_cloudflare','challenge_x_human_verification','challenge_unknown','replay','invalidated','coverage_response_pending','coverage_completion_pending','coverage_extra_pending','cookie_candidate','cookie_ambiguous','login_required','source_unavailable']);
export const CAPTURE_CLEANUP=Object.freeze(['unknown','completed','failed']);
const captureDetails=new WeakMap();
// Keep transient command errors intact inside the existing worker so its normal
// context-recovery classification is unchanged. Only a terminal error is copied
// to a sanitized outward error; the private weak hint contains closed enums.
const commandDetails=new WeakMap();
const data=(value,key)=>value&&(typeof value==='object'||typeof value==='function')?Object.getOwnPropertyDescriptor(value,key)?.value:undefined;
export const manualCaptureDiagnostic=error=>error&&(typeof error==='object'||typeof error==='function')?captureDetails.get(error)??null:null;
// Branded local errors only. A page/transport error string cannot mint a code.
export function manualCaptureFailure(error,stage,reason) {
  if(manualCaptureDiagnostic(error))return error;
  const hint=error&&(typeof error==='object'||typeof error==='function')?commandDetails.get(error):null;
  if(hint){stage=hint.stage;reason=hint.reason;}
  const challenge=data(error,'code')==='challenge_required';
  const category=['cloudflare','x_human_verification'].includes(data(error,'challengeCategory'))?data(error,'challengeCategory'):'unknown';
  if(challenge){stage='worker';reason='challenge_'+category;}
  const result=fail();
  if(challenge)Object.assign(result,{code:'challenge_required',challengeCategory:category});
  captureDetails.set(result,Object.freeze({stage:CAPTURE_STAGES.includes(stage)?stage:'unknown',reason:CAPTURE_REASONS.includes(reason)?reason:'unknown',cleanup:'unknown'}));
  return result;
}
// The private wire carries a closed reason, never page text or raw exceptions.
export function manualCaptureRPCCode(error) {
  if(data(error,'code')==='challenge_required') {
    return data(error,'challengeCategory')==='cloudflare'?'manual_capture_challenge_cloudflare'
      :data(error,'challengeCategory')==='x_human_verification'?'manual_capture_challenge_x_human_verification'
      :'manual_capture_challenge_unknown';
  }
  return 'manual_capture_rejected';
}
// Observational metadata is independent of the operational failure code. It
// cannot select challenge handling, admission, retry, or another authority.
export function manualCaptureRPCDiagnostic(error) {
  const d=manualCaptureDiagnostic(error);
  return d?Object.freeze({schema:'aku.capture-diagnostic.v1',...d}):undefined;
}
const pinsKeys=['ownerId','profileId','contextId','targetId','frameId','documentId'];
const ref=v=>typeof v==='string'&&v.length>0&&v.length<=512&&!/[\u0000-\u001f\u007f]/u.test(v);
const exact=(v,keys)=>{if(!v||![Object.prototype,null].includes(Object.getPrototypeOf(v))||Reflect.ownKeys(v).length!==keys.length||Reflect.ownKeys(v).some(k=>!keys.includes(k)||!Object.hasOwn(Object.getOwnPropertyDescriptor(v,k),'value')))throw fail();};
export function validateManualCaptureRequest(request,now=Date.now()) {
  try {
  exact(request,['correlation','pins','deadlineMS','payload']);exact(request.pins,pinsKeys);
  if(!ref(request.correlation)||!pinsKeys.every(k=>ref(request.pins[k]))||!Number.isSafeInteger(request.deadlineMS)||request.deadlineMS<=now||request.deadlineMS>now+60000)throw fail();
  const allowed=['pageUrl','scrolls','maxBlocksPerSnapshot','sourceHydrationTimeoutMs','scrollFraction','scrollSettleMs'];
  if(!request.payload||![Object.prototype,null].includes(Object.getPrototypeOf(request.payload))||Object.keys(request.payload).some(k=>!allowed.includes(k)))throw fail();
  const left=request.deadlineMS-now;
  if(left<1000)throw fail();
  const payload={...request.payload,acquisitionRound:1,captureTimeoutMs:Math.floor(left),scrolls:request.payload.scrolls??0,
    sourceHydrationTimeoutMs:Math.min(request.payload.sourceHydrationTimeoutMs??1000,Math.floor(left)),
    pendingContentPolicy:'detect_only',sourceFreshnessPolicy:'preserve_frontier',sameTabMutationAllowed:false,restoreScroll:false};
  validateCapture('x',payload);
  return payload;
  }catch(error){throw manualCaptureFailure(error,'payload','invalid_request');}
}

// A private capability on the existing owned collector pipe. No launcher, cookie
// operation, startup navigation, or raw RPC URL/script reaches this boundary.
export async function captureNativeXTarget({request,bridgePath,command,subscribe,browserContextId,proofSession,signal,onUnsafe=()=>{}}) {
  const payload=validateManualCaptureRequest(request);
  if(typeof command!=='function'||typeof subscribe!=='function'||!ref(browserContextId))throw manualCaptureFailure(null,'admission','invalid_request');
  let target,session,unsubscribe,guard,unsafe=false,closed=false,first=null,stage='assets',wire;
  const record=(error,at,reason)=>first??=(manualCaptureFailure(error,at,reason));
  const requests=new Map();
  const expired=()=>signal?.aborted||Date.now()>=request.deadlineMS||unsafe;
  let closing;
  const close=async()=>{if(target&&!closed){closed=true;closing=command('Target.closeTarget',{targetId:target});}await closing;};
  const abort=()=>{void close().catch(()=>{});};
  const timer=setTimeout(abort,Math.max(1,request.deadlineMS-Date.now()));
  signal?.addEventListener('abort',abort,{once:true});
  try {
    const assets=await sourceAssets(bridgePath,'x');
    if(expired())throw record(null,'admission',signal?.aborted?'canceled':'deadline');
    stage='target_create';
    const created=await command('Target.createTarget',{url:'about:blank',browserContextId});
    if(!ref(created.targetId)||created.targetId===request.pins.targetId)throw fail();
    target=created.targetId;
    stage='target_attach';
    session=(await command('Target.attachToTarget',{targetId:target,flatten:true})).sessionId;
    if(!ref(session)||session===proofSession)throw fail();
    stage='observer';
    await command('Page.enable',{},session);
    const frame=(await command('Page.getFrameTree',{},session)).frameTree?.frame?.id;
    if(!ref(frame))throw fail();
    const reject=(reason='scope_lost')=>{record(null,'coverage',reason);unsafe=true;onUnsafe();abort();};
    guard=new HeaderSafetyGuard(frame,session,{onRejection:()=>reject(['cookie_candidate','cookie_ambiguous'].includes(guard.rejection())?guard.rejection():'header_guard')});guard.claim(frame,session);
    unsubscribe=subscribe(raw=>{
      if(raw.sessionId===session){
        if(['Inspector.detached','Inspector.targetCrashed'].includes(raw.method))reject('target_lost');
        const id=raw.params?.requestId,known=requests.has(id),first=raw.method==='Network.requestWillBeSent'&&!known&&!raw.params?.redirectResponse;
        if(typeof id==='string'&&(known||raw.method==='Network.requestWillBeSent')){
          if(requests.size>=1280&&!known){reject('request_budget');return;}
          if(!known)requests.set(id,{response:false,extra:false,finished:false,extraNeeded:null});
          const record=requests.get(id);
          if(raw.method==='Network.responseReceived'){record.response=true;record.extraNeeded=raw.params.hasExtraInfo;if(typeof record.extraNeeded!=='boolean')reject('coverage_shape');}
          if(raw.method==='Network.responseReceivedExtraInfo')record.extra=true;
          if(raw.method==='Network.loadingFinished'||raw.method==='Network.loadingFailed')record.finished=true;
        }
        if(typeof id==='string'&&!known&&['Network.responseReceived','Network.responseReceivedExtraInfo'].includes(raw.method))reject('coverage_shape');
        guard.observe(raw,{known,first});
      }
      if(raw.method==='Target.targetDestroyed'&&raw.params?.targetId===target&&!closed)reject('target_lost');
    });
    await command('Network.enable',{maxTotalBufferSize:1000000,maxResourceBufferSize:100000,maxPostDataSize:0},session);
    const send=async(method,params={},timeoutMs=1500)=>{
      const at=method==='Page.navigate'?'navigation':'evaluation';
      if(expired()||closed)throw first??manualCaptureFailure(null,at,signal?.aborted?'canceled':unsafe?'scope_lost':'deadline');
      if(!['Page.navigate','Runtime.evaluate','Page.addScriptToEvaluateOnNewDocument','Input.dispatchMouseEvent'].includes(method))throw manualCaptureFailure(null,at,'method_rejected');
      if(method==='Page.navigate'&&params.url!==validateCapture('x',payload).pageUrl)throw manualCaptureFailure(null,at,'invalid_request');
      if(method==='Runtime.evaluate'&&(typeof params.expression!=='string'||params.expression.length>1024*1024))throw manualCaptureFailure(null,at,'invalid_request');
      // Trusted worker assets generate expressions; none comes from the wire.
      let result;
      try{result=await command(method,params,session,Math.min(timeoutMs,request.deadlineMS-Date.now()));}
      catch(error){
        if(error&&(typeof error==='object'||typeof error==='function')&&!commandDetails.has(error))commandDetails.set(error,{stage:at,reason:signal?.aborted?'canceled':Date.now()>=request.deadlineMS?'deadline':'unavailable'});
        throw error;
      }
      if(expired())throw first??manualCaptureFailure(null,at,signal?.aborted?'canceled':unsafe?'scope_lost':'deadline');return result;
    };
    const page={send,navigate:(url,ms)=>send('Page.navigate',{url},ms),evaluate:async(expression,ms)=>{
      const result=await send('Runtime.evaluate',{expression,returnByValue:true,awaitPromise:true},ms);
      if(result.exceptionDetails)throw manualCaptureFailure(null,'evaluation','evaluation_exception');return result.result?.value;
    }};
    stage='worker';
    const observation=await captureIsolated({backend:'headless_worker',forSource:async source=>{if(source!=='x')throw fail();return page;}},{x:assets},'x',payload);
    stage='coverage';
    await command('Runtime.evaluate',{expression:'void 0',returnByValue:true},session);
    if(expired()||guard.rejection()!=='unknown')throw record(null,'coverage',signal?.aborted?'canceled':unsafe?'header_guard':'deadline');
    stage='serialization';
    const observationJSON=JSON.stringify(observation);
    if(Buffer.byteLength(observationJSON)>512*1024)throw fail();
    wire={schema:'aku.x.manual-capture.v1',correlation:request.correlation,pins:request.pins,captureTarget:target,observationJSON,digest:createHash('sha256').update(observationJSON).digest('hex')};
    if(Buffer.byteLength(JSON.stringify(wire))>768*1024)throw fail();
  } catch(error) {
    const code=data(error,'code');
    record(error,stage,signal?.aborted?'canceled':Date.now()>=request.deadlineMS||code==='capture_timeout'?'deadline':stage==='worker'?(['invalid_payload','navigation_failed','empty_unverified','invalid_observation','worker_not_ready','target_unavailable','login_required','source_unavailable'].includes(code)?code:'worker_other'):'unavailable');
  } finally {
    clearTimeout(timer);signal?.removeEventListener('abort',abort);
    let cleanup='unknown';
    try{await close();if(target)cleanup='completed';}
    catch(error){cleanup='failed';record(error,'cleanup','close_failed');}
    finally{try{unsubscribe?.();}catch(error){cleanup='failed';record(error,'cleanup','close_failed');}}
    // A round trip is not evidence that ExtraInfo has arrived. Observed pending
    // response/header coverage closes the result even after target retirement.
    if(guard){
      if(unsafe||guard.rejection()!=='unknown')record(null,'coverage','header_guard');
      for(const r of requests.values()){
        if(!r.response)record(null,'coverage','coverage_response_pending');
        else if(!r.finished)record(null,'coverage','coverage_completion_pending');
        else if(r.extraNeeded===null)record(null,'coverage','coverage_shape');
        else if(r.extraNeeded&&!r.extra)record(null,'coverage','coverage_extra_pending');
        else if(r.extra&&!r.extraNeeded)record(null,'coverage','extra_mismatch');
      }
    }
    if(first){const d=manualCaptureDiagnostic(first);captureDetails.set(first,Object.freeze({...d,cleanup}));}
  }
  if(first)throw first;
  return wire;
}

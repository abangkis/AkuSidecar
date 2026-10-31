import {createIGTrace,igFailure,recordIGError,igErrorTrace} from './instagram-diagnostic-trace.mjs';
import {cdpCommandFailure} from './cdp-command-failure.mjs';
import {createHash} from 'node:crypto';
import {parseStrictJSON} from '../../../../../scripts/auth-session-fixture/x-evidence-ingress.mjs';
import {MANUAL_TIMING} from '../../../../../scripts/auth-session-fixture/x-manual-timing-policy.mjs';
import {STARTUP_PHASES,STARTUP_REASONS,STARTUP_ROLES,STARTUP_TRACE} from '../../../../../scripts/auth-session-fixture/x-manual-startup-diagnostic.mjs';
import {SETTINGS_WAIT_STATES} from '../../../../../scripts/auth-session-fixture/x-settings-wait-state.mjs';
import {createSourceOwnerRegistry,sourceOwnerRegistryFailure} from './source-owner-registry.mjs';
import {nativeSourceDiagnostic} from './native-source-kernel.mjs';
import {INSTAGRAM_HTTP_REJECTIONS} from './instagram-native-session.mjs';
import {createMainDiagnosticScope,createMainInstagramOwnedScope} from './main-browser-diagnostic-scope.mjs';
import {mainTransportLifecycle} from './main-transport-lifecycle.mjs';
import {validateInstagramReaderDiagnostic} from './instagram-reader-diagnostic.mjs';
import {createMainBootstrapDiagnosticScope} from './main-bootstrap-diagnostic-scope.mjs';
import {validateInstagramBootstrapDiagnostic} from './instagram-self-bootstrap-probe.mjs';
import {createMainBootstrapCorroborationScope} from './main-bootstrap-corroboration-scope.mjs';
import {validateInstagramBootstrapCorroboration} from './instagram-bootstrap-corroboration.mjs';

export const MAIN_TRANSPORT_SCHEMA='aku.main-browser-transport.v1';
const rejected=()=>new Error('main_transport_rejected');
// Numeric diagnostics are admitted only from the same finite identity catalog
// used by the IG projection, never by widening arbitrary diagnostic text.
const instagramHTTPReasons=new Set(['reader','collector','unknown'].flatMap(role=>INSTAGRAM_HTTP_REJECTIONS.map(reason=>`${role}_identity_${reason}`)));
const diagnosticWord=value=>typeof value==='string'&&/^[a-z_]{1,64}$/u.test(value);
const sourceDiagnosticAllowed=value=>value&&diagnosticWord(value.stage)&&diagnosticWord(value.cleanup)&&
  (diagnosticWord(value.reason)||value.stage==='evidence'&&instagramHTTPReasons.has(value.reason));
const object=v=>v!==null&&typeof v==='object'&&!Array.isArray(v);
const exact=(v,keys)=>{if(!object(v)||Object.keys(v).length!==keys.length||keys.some(k=>!Object.hasOwn(v,k)))throw rejected();};
const bounded=(v,n=4096)=>typeof v==='string'&&v.length<=n&&!/[\u0000-\u001f\u007f]/u.test(v);
const hash=s=>createHash('sha256').update(s).digest('hex');
const parse=b=>parseStrictJSON(new TextDecoder('utf-8',{fatal:true}).decode(b),{maxJSONDepth:64,maxJSONTokens:500000});
const startupProgressOperations=new Set(['inspect_rpc','inventory_rpc','acquire_rpc','identity','rendezvous','settings_wait','cleanup','root_restore','complete']);
const startupProgressOwnerOperations=new Set(['manual_acquire','manual_inspect','manual_inventory']);
const startupCauseCatalogs=[STARTUP_ROLES,STARTUP_PHASES,STARTUP_REASONS,['primary','peer'],...Object.values(STARTUP_TRACE)];
function startupCause(value){
  if(value==='')return true;
  for(const role of STARTUP_ROLES)for(const phase of STARTUP_PHASES)for(const reason of STARTUP_REASONS)for(const kind of ['primary','peer'])
    if(value===`manual_initial_${role}_${phase}_${reason}_${kind}`)return true;
  const current=value.startsWith('manual_initial_v10_'),prefix=current?'manual_initial_v10_':'manual_initial_v9_';
  if(!value.startsWith(prefix)||value.length>(current?128:80))return false;
  const parts=value.slice(prefix.length).split('_');
  const enums=current?startupCauseCatalogs.length:startupCauseCatalogs.length-1;
  return parts.length===enums+(current?4:0)&&parts.every((part,index)=>/^(?:0|[1-9][0-9]*)$/u.test(part)&&
    (index<enums?Number(part)<startupCauseCatalogs[index].length:Number.isSafeInteger(Number(part))&&Number(part)<=60001));
}
// Progress is diagnostic-only. Normalize it against the current closed catalogs
// before it crosses the Node/Go control channel.
export function mainStartupProgress(value){
  try{
    if(!object(value))return null;
    const keys=Object.keys(value),required=['operation','elapsedMS','cause'];
    if((keys.length!==3&&keys.length!==4)||required.some(k=>!Object.hasOwn(value,k))||keys.some(k=>!['operation','elapsedMS','cause','settingsWait'].includes(k)))return null;
    if(!startupProgressOperations.has(value.operation)||!Number.isSafeInteger(value.elapsedMS)||value.elapsedMS<0||value.elapsedMS>MANUAL_TIMING.attemptAuthorizationMaxMS||
      typeof value.cause!=='string'||!startupCause(value.cause))return null;
    const out={operation:value.operation,elapsedMS:value.elapsedMS,cause:value.cause};
    if(Object.hasOwn(value,'settingsWait')){
      if(typeof value.settingsWait!=='string'||!SETTINGS_WAIT_STATES.includes(value.settingsWait))return null;
      out.settingsWait=value.settingsWait;
    }
    return Buffer.byteLength(JSON.stringify(out),'utf8')<=512?out:null;
  }catch{return null;}
}
export function mainStartupProgressForOwnerOperation(operation,value){
  if(!startupProgressOwnerOperations.has(operation))return null;
  return mainStartupProgress(value);
}
const ownerFailures=new WeakMap();
// Called only for replies produced by the in-process daily owner validator,
// whose code/diagnostic originates in the existing closed diagnostic catalogs.
// Arbitrary exceptions (even ones with code/diagnostic properties) are untrusted.
export function mainOwnerFailure(reply){
  const error=rejected();
  const code=typeof reply.code==='string'&&/^[a-z0-9_]{1,1024}$/u.test(reply.code)?reply.code:'application_owner_failed';
  const diagnostic=reply.diagnostic;
  ownerFailures.set(error,{code,...(object(diagnostic)&&diagnostic.schema==='aku.capture-diagnostic.v1'&&JSON.stringify(diagnostic).length<=16384?{diagnostic:structuredClone(diagnostic)}:{})});
  return error;
}

// This object never leaves Node. Raw results/events are available only to the
// sealed X verifier; public callers receive method-specific projections below.
export function createMainCDP({input,output,onFailure=()=>{}}){
  let closed=false,bytes=Buffer.alloc(0),sequence=0;
  const pending=new Map(),listeners=new Set(),privateTargets=new Set(),privateSessions=new Set();
  const close=(cause={reason:'explicit_close',domain:'cdp',method:'none',role:'unknown',stage:'shutdown'})=>{if(closed)return;closed=true;bytes.fill(0);bytes=Buffer.alloc(0);
    for(const p of pending.values()){clearTimeout(p.timer);p.reject(cdpCommandFailure('transport_closed'));}pending.clear();listeners.clear();input.destroy();output.destroy();onFailure(mainTransportLifecycle(cause));};
  const fail=(reason,stage='framing',method='none',role='unknown')=>close({reason,domain:'cdp',method,role,stage});
  input.on('data',chunk=>{
    if(closed)return;
    if(bytes.length+chunk.length>16*1024*1024){fail('inbound_limit');return;}
    const previous=bytes;bytes=Buffer.concat([previous,chunk]);previous.fill(0);
    let end;
    while(!closed&&(end=bytes.indexOf(0))>=0){
      const frame=bytes.subarray(0,end);let q;
      try{q=parse(frame);}catch{fail('invalid_frame');return;}
      const rest=Buffer.from(bytes.subarray(end+1));bytes.fill(0);bytes=rest;
      if(!object(q)){fail('invalid_frame');return;}
      if(Object.hasOwn(q,'id')){
        const p=pending.get(q.id);if(!p)continue;pending.delete(q.id);clearTimeout(p.timer);
        if((q.sessionId??'')!==p.sessionId){p.reject(cdpCommandFailure('reply_invalid'));fail('reply_scope','command',p.method,p.role);return;}
        if(p.expired){if(!q.error&&object(q.result))Promise.resolve().then(()=>p.late(q.result)).catch(()=>{});}
        else if(q.error)p.reject(cdpCommandFailure('command_rejected'));
        else if(!object(q.result))p.reject(cdpCommandFailure('reply_invalid'));else p.resolve(q.result);
      }else if(typeof q.method==='string'){
        for(const listener of listeners){try{listener(q);}catch{fail('listener_failure');break;}}
      }else{fail('invalid_frame');return;}
    }
  });
  input.on('end',()=>fail('input_end','stream'));input.on('error',()=>fail('input_error','stream'));output.on('error',()=>fail('output_error','stream'));
  function call(method,params={},sessionId='',timeoutMS=15000,local=false,late,role='ordinary'){
    if(closed)return Promise.reject(cdpCommandFailure('transport_closed'));
    if(pending.size>=32||!bounded(method,128)||!bounded(sessionId,512)||!Number.isInteger(timeoutMS)||timeoutMS<1||timeoutMS>60000)return Promise.reject(cdpCommandFailure('request_rejected'));
    const id=++sequence;if(!Number.isSafeInteger(id)){fail('sequence_limit','command',method,role);return Promise.reject(cdpCommandFailure('request_rejected'));}
    const wire=Buffer.from(JSON.stringify({id,method,params,...(sessionId?{sessionId}:{})})+'\0');
    if(wire.length>8*1024*1024){wire.fill(0);return Promise.reject(cdpCommandFailure('request_rejected'));}
    return new Promise((resolve,reject)=>{
      const entry={resolve,reject,sessionId,late,method,role,expired:false};
      entry.timer=setTimeout(()=>{
        if(!local){fail('command_timeout','command',method,role);return;}
        if(typeof late==='function')entry.expired=true;else pending.delete(id);
        reject(cdpCommandFailure('timeout',true));
      },timeoutMS);
      pending.set(id,entry);
      try{output.write(wire,error=>{wire.fill(0);if(error)fail('output_error','command',method,role);});}catch{wire.fill(0);fail('output_error','command',method,role);}
    });
  }
  const diagnosticRaw={
    alive:()=>!closed,
    privateTarget:id=>privateTargets.has(id),privateSession:id=>privateSessions.has(id),
    reserveTarget(id){if(!bounded(id,512)||!id||privateTargets.size>=2048)throw rejected();privateTargets.add(id);},
    reserveSession(id){if(!bounded(id,512)||!id||privateSessions.size>=2048)throw rejected();privateSessions.add(id);},
    releaseTarget:id=>privateTargets.delete(id),releaseSession:id=>privateSessions.delete(id),
    subscribe(fn){if(closed||typeof fn!=='function'||listeners.size>=32)throw rejected();listeners.add(fn);return()=>listeners.delete(fn);},
    call:(method,params,sessionId,timeoutMS,late)=>call(method,params,sessionId,timeoutMS,true,late,'diagnostic'),
  };
  return Object.freeze({
    alive:()=>!closed,
    privateTarget:id=>privateTargets.has(id),privateSession:id=>privateSessions.has(id),
    reserveTarget(id){if(!bounded(id,512)||!id||privateTargets.size>=2048)throw rejected();privateTargets.add(id);},
    reserveSession(id){if(!bounded(id,512)||!id||privateSessions.size>=2048)throw rejected();privateSessions.add(id);},
    releaseTarget:id=>privateTargets.delete(id),releaseSession:id=>privateSessions.delete(id),
    subscribe(fn){if(closed||typeof fn!=='function'||listeners.size>=32)throw rejected();listeners.add(fn);return()=>listeners.delete(fn);},
    call:(method,params,sessionId,timeoutMS)=>call(method,params,sessionId,timeoutMS),
    createDiagnosticScope:()=>createMainDiagnosticScope(diagnosticRaw),
    createInstagramOwnedScope:()=>createMainInstagramOwnedScope({...diagnosticRaw,call:(method,params,sessionId,timeoutMS,late)=>call(method,params,sessionId,timeoutMS,true,late,'ig_reader')}),
    createBootstrapCorroborationScope:trace=>createMainBootstrapCorroborationScope(diagnosticRaw,trace),
    createBootstrapDiagnosticScope:options=>createMainBootstrapDiagnosticScope(diagnosticRaw,options),close,
  });
}

const fields={
  'Browser.getVersion':[], 'Browser.getWindowForTarget':['targetId'], 'Browser.getWindowBounds':['windowId'],
  'Browser.setWindowBounds':['windowId','bounds'], 'Target.attachToBrowserTarget':[], 'Target.getTargets':[],
  'Target.createTarget':['url','newWindow','background','browserContextId','width','height'],
  'Target.attachToTarget':['targetId','flatten'], 'Target.detachFromTarget':['sessionId'],
  'Target.closeTarget':['targetId'], 'Page.navigate':['url'], 'Page.enable':[], 'Runtime.enable':[],
  'Input.dispatchMouseEvent':['type','x','y','button','buttons','clickCount','deltaX','deltaY','modifiers','pointerType'],
  'Emulation.setDeviceMetricsOverride':['width','height','deviceScaleFactor','mobile'], 'Network.enable':[],
};
const pick=(v,keys)=>Object.fromEntries(keys.filter(k=>Object.hasOwn(v,k)).map(k=>[k,v[k]]));
const geometry=v=>pick(v??{},['left','top','width','height','windowState']);
function projection(method,result){
  switch(method){
    case 'Browser.getVersion':return pick(result,['protocolVersion','product','revision','userAgent','jsVersion']);
    case 'Browser.getWindowForTarget':return{windowId:result.windowId,bounds:geometry(result.bounds)};
    case 'Browser.getWindowBounds':return{bounds:geometry(result.bounds)};
    case 'Target.getTargets':
      if(!Array.isArray(result.targetInfos)||result.targetInfos.length>2048)throw rejected();
      return{targetInfos:result.targetInfos.map(t=>pick(t,['targetId','type','title','url','attached','browserContextId']))};
    case 'Target.createTarget':return{targetId:result.targetId};
    case 'Target.attachToBrowserTarget':case 'Target.attachToTarget':return{sessionId:result.sessionId};
    case 'Target.closeTarget':return{success:result.success===true};
    case 'Page.navigate':return{...pick(result,['frameId','loaderId']),...(result.errorText?{errorText:'navigation_rejected'}:{})};
    default:return{};
  }
}

// Catalog entries must be supplied by sealed Node code, never by a Go message.
// An exact expression hash selects both a recipe and its result projection.
export function createMainPublicFacade(transport,{recipes=[],busy=()=>false}={}){
  const catalog=new Map(),sessions=new Set(),targets=new Set(),createdTargets=new Set();
  for(const r of recipes){if(typeof r.expression!=='string'||typeof r.project!=='function'||catalog.has(hash(r.expression)))throw rejected();catalog.set(hash(r.expression),r);}
  return async q=>{
    exact(q,['method','params','sessionId','timeoutMS']);
    if(busy()||!bounded(q.sessionId,512)||!object(q.params)||q.sessionId&&(!sessions.has(q.sessionId)||transport.privateSession(q.sessionId)))throw rejected();
    if(q.params.targetId&&transport.privateTarget(q.params.targetId)||q.params.sessionId&&(!sessions.has(q.params.sessionId)||transport.privateSession(q.params.sessionId)))throw rejected();
    if(q.params.targetId&&!targets.has(q.params.targetId)||q.method==='Target.closeTarget'&&!createdTargets.has(q.params.targetId))throw rejected();
    if(q.method==='Runtime.evaluate'){
      const p=q.params,recipe=typeof p.expression==='string'?catalog.get(hash(p.expression)):null;
      if(!recipe||Object.keys(p).some(k=>!['expression','returnByValue','awaitPromise','userGesture'].includes(k))||p.returnByValue!==true||p.userGesture===true)throw rejected();
      return recipe.project(await transport.call(q.method,p,q.sessionId,q.timeoutMS));
    }
    const keys=fields[q.method];if(!keys||Object.keys(q.params).some(k=>!keys.includes(k)))throw rejected();
    if(q.method==='Browser.setWindowBounds'&&(!object(q.params.bounds)||Object.keys(q.params.bounds).some(k=>!['left','top','width','height','windowState'].includes(k))))throw rejected();
    if(['Target.createTarget','Page.navigate'].includes(q.method)){
      const url=q.params.url;
      if(!bounded(url)||!(/^(?:https?:\/\/|about:blank$)/u.test(url)))throw rejected();
    }
    if(q.method==='Target.attachToTarget'&&q.params.flatten!==true)throw rejected();
    const raw=await transport.call(q.method,q.params,q.sessionId,q.timeoutMS);
    if(['Target.attachToBrowserTarget','Target.attachToTarget'].includes(q.method)){
      if(!bounded(raw.sessionId,512)||!raw.sessionId||sessions.size>=2048)throw rejected();sessions.add(raw.sessionId);
    }
    if(q.method==='Target.detachFromTarget')sessions.delete(q.params.sessionId);
    if(q.method==='Target.createTarget'){
      if(!bounded(raw.targetId,512)||!raw.targetId||targets.size>=2048)throw rejected();targets.add(raw.targetId);createdTargets.add(raw.targetId);
    }
    if(q.method==='Target.closeTarget'&&raw.success===true){targets.delete(q.params.targetId);createdTargets.delete(q.params.targetId);}
    if(q.method==='Target.getTargets'){
      if(!Array.isArray(raw.targetInfos))throw rejected();
      const visible=raw.targetInfos.filter(t=>!transport.privateTarget(t.targetId));
      if(visible.length>2048)throw rejected();targets.clear();for(const t of visible){if(!bounded(t.targetId,512)||!t.targetId)throw rejected();targets.add(t.targetId);}
      return projection(q.method,{targetInfos:visible});
    }
    return projection(q.method,raw);
  };
}

// The entry supplies connection and sealed source factories. Neither a control request nor
// an error can return raw CDP frames, headers, cookies or protocol exceptions.
export function serveMainBrowserTransport({input,output,connect,createX,sourceFactories={},diagnoseInstagramReader,diagnoseInstagramBootstrap,diagnoseInstagramBootstrapCorroboration,recipes=[],expectedNonce,onClosed=()=>{}}){
  let bytes=Buffer.alloc(0),closed=false,booting=false,transport,publicCall,x,xAttempted=false,busy=null,lastID=0,pending=0,publicPending=0,ownerPending=0,diagnosticBlocked=false;
  let firstLifecycle,lifecycleWritten=Promise.resolve();
  const sources=createSourceOwnerRegistry(sourceFactories);
  const acquire=(lane,terminal=false)=>{
    if(busy&&(!terminal||busy.lane!==lane))throw rejected();
    const lock=busy??={lane,holders:0};lock.holders++;
    return()=>{if(--lock.holders===0&&busy===lock)busy=null;};
  };
  const seen=new Set();
  const reportLifecycle=cause=>{
    if(firstLifecycle)return;firstLifecycle=mainTransportLifecycle(cause);
    const wire=Buffer.from(JSON.stringify({event:'main_transport_lifecycle',result:firstLifecycle})+'\n');
    // Diagnostics can be lost on an already broken output. Never retry them or
    // substitute exception text; the Go observer retains unknown in that case.
    lifecycleWritten=new Promise(resolve=>{try{output.write(wire,()=>{wire.fill(0);resolve();});}catch{wire.fill(0);resolve();}});
  };
  const flushLifecycle=async()=>{let timer;try{await Promise.race([lifecycleWritten,new Promise(resolve=>{timer=setTimeout(resolve,100);})]);}finally{clearTimeout(timer);}};
  const controlCause=(reason,stage='stream')=>({reason,domain:'control',method:'none',role:'unknown',stage});
  const emit=q=>{if(closed)return;const wire=Buffer.from(JSON.stringify(q)+'\n');if(wire.length>8*1024*1024){wire.fill(0);close(controlCause('control_output_limit','framing'));return;}
    try{output.write(wire,error=>{wire.fill(0);if(error)close(controlCause('output_error'));});}catch{wire.fill(0);close(controlCause('output_error'));}};
  const close=(cause={reason:'explicit_close',domain:'owner',method:'none',role:'unknown',stage:'shutdown'})=>{
    if(closed)return;reportLifecycle(cause);closed=true;bytes.fill(0);input.pause();
    void Promise.allSettled([Promise.resolve().then(()=>x?.close()),sources.close()]).finally(async()=>{transport?.close();await flushLifecycle();onClosed(firstLifecycle);});
  };
  async function handle(line){
    let q,igTrace;try{
      q=parse(line);
      if(!transport){
        if(booting)throw rejected();booting=true;
        exact(q,['op','schema','nonce','transport']);
        if(q.op!=='main_transport_bootstrap'||q.schema!==MAIN_TRANSPORT_SCHEMA||!/^[a-f0-9]{48}$/u.test(q.nonce)||
          expectedNonce!==undefined&&q.nonce!==expectedNonce||q.transport!=='inherited-stdio')throw rejected();
        const streams=await connect(q);if(closed){streams.input.destroy();streams.output.destroy();return;}
        transport=createMainCDP({...streams,onFailure:close});publicCall=createMainPublicFacade(transport,{recipes,busy:()=>busy});
        emit({ready:true,schema:MAIN_TRANSPORT_SCHEMA,nonce:q.nonce});return;
      }
      if(!Number.isSafeInteger(q.id)||q.id<1||q.id<=lastID-1024||seen.has(q.id)||pending>=32)throw rejected();
      lastID=Math.max(lastID,q.id);seen.add(q.id);for(const id of seen)if(id<=lastID-1024)seen.delete(id);pending++;
      try{
        let result;
        if(q.op==='main_call'){exact(q,['id','op','method','params','sessionId','timeoutMS']);const {id,op,...call}=q;publicPending++;try{result=await publicCall(call);}finally{publicPending--;}}
        else if(q.op==='main_x'){
          exact(q,['id','op','operation','params']);if(typeof createX!=='function'||busy&&(busy.lane==='diagnostic:instagram'||q.operation!=='daily_reader_check'))throw rejected();
          const exclusive=['daily_owner_bootstrap','daily_reader_initialize','daily_batch_begin','daily_batch_seed','daily_batch_end','daily_owner_close'].includes(q.operation);
          if(exclusive&&publicPending)throw rejected();const release=exclusive?acquire('x'):()=>{};ownerPending++;
          try{if(!x){if(q.operation!=='daily_owner_bootstrap'||xAttempted)throw rejected();xAttempted=true;x=await createX(q.params,transport);result={bound:true};}
            else{
              let terminal=false,progressCount=0;
              const report=value=>{
                if(terminal||progressCount>=32)return;
                const progress=mainStartupProgressForOwnerOperation(q.operation,value);if(!progress)return;
                progressCount++;emit({event:'main_x_progress',id:q.id,result:progress});
              };
              try{result=await x.execute(q.operation,q.params,q.id,report);}finally{terminal=true;}
            }
          }finally{ownerPending--;release();}
        }else if(q.op==='main_source'){
          exact(q,['id','op','source','operation','params']);
          const terminal=['source_end','source_close'].includes(q.operation),lane='source:'+q.source;
          if(typeof q.source!=='string'||typeof q.operation!=='string'||busy&&(!terminal||busy.lane!==lane))throw rejected();
          const exclusive=['source_bootstrap','source_initialize','source_begin','source_seed','source_end','source_close'].includes(q.operation);
          if(exclusive&&publicPending&&!terminal)throw rejected();const release=exclusive?acquire(lane,terminal):()=>{};ownerPending++;
          try{result=await sources.execute(q.source,q.operation,q.params,transport);}
          finally{ownerPending--;release();}
        }else if(['main_instagram_reader_diagnostic','main_instagram_bootstrap_diagnostic','main_instagram_bootstrap_corroboration'].includes(q.op)){
          igTrace=createIGTrace(value=>emit({event:'main_ig_progress',id:q.id,result:value}));igTrace.note('dispatch');
          try{exact(q,Object.hasOwn(q,'budgetMS')?['id','op','nonce','budgetMS']:['id','op','nonce']);}catch{throw igFailure('dispatch','invalid_request');}
          if(q.budgetMS!==undefined&&(!Number.isInteger(q.budgetMS)||q.budgetMS<1||q.budgetMS>30000))throw igFailure('dispatch','invalid_request');
          igTrace.budget(q.budgetMS??30000);
          const bootstrap=q.op==='main_instagram_bootstrap_diagnostic',corroboration=q.op==='main_instagram_bootstrap_corroboration';
          const diagnose=corroboration?diagnoseInstagramBootstrapCorroboration:bootstrap?diagnoseInstagramBootstrap:diagnoseInstagramReader;
          if(typeof diagnose!=='function')throw igFailure('dispatch','factory_missing');
          if(typeof q.nonce!=='string'||!/^[a-f0-9]{48}$/u.test(q.nonce))throw igFailure('dispatch','nonce_invalid');
          if(busy)throw igFailure('dispatch','lane_busy');
          if(publicPending)throw igFailure('dispatch','public_pending');
          if(ownerPending)throw igFailure('dispatch','owner_pending');
          if(diagnosticBlocked)throw igFailure('dispatch','cleanup_blocked');
          const release=acquire('diagnostic:instagram');
          try{igTrace.disposition('started');result=(corroboration?validateInstagramBootstrapCorroboration:bootstrap?validateInstagramBootstrapDiagnostic:validateInstagramReaderDiagnostic)(await diagnose(transport,{nonce:q.nonce},{trace:igTrace,budgetMS:q.budgetMS??30000}));diagnosticBlocked=result.cleanup!=='verified';
            igTrace.cleanup(result.cleanup,result.cleanup==='uncertain'?'cleanup_failed':'none');
            const reason=result.reason??result.primaryCause;if(reason&&!['none','corroborated','observed','cleanup_failed'].includes(reason)){try{igTrace.fail('projection',reason);}catch{igTrace.incomplete();}}
            igTrace.done('projection');igTrace.disposition('completed');}
          catch(error){diagnosticBlocked=true;throw error;}
          finally{release();}
        }else throw rejected();
        emit({id:q.id,ok:true,result});
      }finally{pending--;}
    }catch(error){
      let failure={code:'main_transport_rejected'};
      if(igTrace){recordIGError(igTrace,error,igTrace.snapshot().stage);failure={code:'instagram_diagnostic_rejected',diagnostic:igTrace.snapshot()};}
      if(q?.op==='main_x'&&ownerFailures.has(error))failure=ownerFailures.get(error);
      if(q?.op==='main_source'){
        const reason=sourceOwnerRegistryFailure(error),diagnostic=nativeSourceDiagnostic(error);
        if(reason)failure={code:'native_'+reason};
        else if(sourceDiagnosticAllowed(diagnostic))
          failure={code:`native_source_${diagnostic.stage}_${diagnostic.reason}`};
      }
      const igSideband=igErrorTrace(error);if(igSideband)failure={...failure,diagnostic:igSideband};
      if(q&&Number.isSafeInteger(q.id))emit({id:q.id,ok:false,error:failure});else close(controlCause('invalid_frame','framing'));
    }
    finally{line.fill(0);}
  }
  input.on('data',chunk=>{if(closed)return;if(bytes.length+chunk.length>8*1024*1024){close(controlCause('control_input_limit','framing'));return;}const old=bytes;bytes=Buffer.concat([old,chunk]);old.fill(0);let at;
    while(!closed&&(at=bytes.indexOf(10))>=0){const line=Buffer.from(bytes.subarray(0,at)),rest=Buffer.from(bytes.subarray(at+1));bytes.fill(0);bytes=rest;void handle(line);}});
  input.on('end',()=>close(controlCause('control_end')));input.on('error',()=>close(controlCause('control_error')));output.on('error',()=>close(controlCause('output_error')));
  return{close,reportLifecycle};
}

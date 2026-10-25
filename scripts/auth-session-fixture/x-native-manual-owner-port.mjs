// Private, default-off QA transport. OS provenance belongs to the compiled Go
// owner backend; this module cannot grant it through a request or JSON flag.
import {startupFailureLatch,startupInspectionFailure} from './x-manual-startup-diagnostic.mjs';
import {validateManualCaptureRequest} from './x-native-manual-capture.mjs';
import {MANUAL_STARTUP_MAX_MS} from './x-manual-startup-limits.mjs';
const roles=['reader','collector'];
const pinKeys=['ownerId','profileId','contextId','targetId','frameId','documentId'];
const requestKeys=['challenge','role',...pinKeys];
const failure=()=>new Error('xnp_manual_owner_unavailable');
const ref=v=>typeof v==='string'&&v.length>0&&Buffer.byteLength(v)<=512&&!/[\u0000-\u001f\u007f]/u.test(v);
function exact(value, keys) {
  if(!value||![Object.prototype,null].includes(Object.getPrototypeOf(value)))throw failure();
  const descriptors=Object.getOwnPropertyDescriptors(value), names=Reflect.ownKeys(descriptors);
  if(names.length!==keys.length||names.some(k=>!keys.includes(k)||!Object.hasOwn(descriptors[k],'value')||!descriptors[k].enumerable))throw failure();
}
export function manualOwnerOperation(value) {
  const operations={manual_prepare:['id','op','owner','role'],manual_acquire:['id','op','owner','role','handle','request','startupBudgetMS'],
    manual_inspect:['id','op','owner','role'],manual_inventory:['id','op','owner','role'],
    manual_abort:['id','op','owner','role','handle','preserveReader'],manual_capture:['id','op','owner','role','handle','request'],manual_capture_cancel:['id','op','owner','role','handle']};
  const op=value&&Object.getOwnPropertyDescriptor(value,'op')?.value;
  const keys=typeof op==='string'&&Object.hasOwn(operations,op)?operations[op]:null;
  if(!keys)throw failure();exact(value,keys);
  if(!Number.isSafeInteger(value.id)||value.id<1||!Number.isSafeInteger(value.owner)||value.owner<1||!roles.includes(value.role))throw failure();
  if('handle' in value&&!(value.op==='manual_abort'&&value.handle===null)&&!ref(value.handle))throw failure();
  if(value.op==='manual_abort'&&(typeof value.preserveReader!=='boolean'||value.role==='collector'&&value.preserveReader))throw failure();
  if(value.op==='manual_acquire'){
    if(!Number.isSafeInteger(value.startupBudgetMS)||value.startupBudgetMS<1||value.startupBudgetMS>MANUAL_STARTUP_MAX_MS)throw failure();
    exact(value.request,requestKeys);
    if(!requestKeys.every(k=>ref(value.request[k]))||value.request.role!==value.role)throw failure();
  }
  if(value.op==='manual_capture'){if(value.role!=='collector')throw failure();validateManualCaptureRequest(value.request);}
  if(value.op==='manual_capture_cancel'&&value.role!=='collector')throw failure();
  return value;
}
export function createXManualOwnerPort({enabled=false,owner,createOwners,snapshot,retire,capture,diagnostics=startupFailureLatch()}={}) {
  if(enabled!==true||!Number.isSafeInteger(owner)||owner<1||![createOwners,snapshot,retire].every(v=>typeof v==='function'))throw failure();
  const actors=createOwners();
  if(!actors||!roles.every(role=>['prepare','acquire','abort','cancel'].every(k=>typeof actors[role]?.[k]==='function')))throw failure();
  const state=Object.fromEntries(roles.map(role=>[role,{actor:actors[role],pins:null,handle:null,preparing:false,acquired:false,invalid:false,closed:false,session:null}]));
  let failed=false;
  const invalidate=role=>{diagnostics.record(null,role,'joint','scope_lost','xnp_manual_owner_unavailable');state[role].invalid=true;failed=true;};
  const checked=async(role,document=false,phase='prepare')=>{try{
    const s=state[role];let value;
    try{value=await snapshot(role,document,s.pins,phase);}
    catch(error){throw startupInspectionFailure(error,role,phase,'node_snapshot','command_failed');}
    const reject=condition=>{throw startupInspectionFailure(null,role,phase,'node_validate',condition);};
    if(!value)reject('snapshot_shape');
    if(value.alive!==true)reject('owner_dead');
    if(!['ownerId','profileId','targetId','sessionId'].every(k=>ref(value[k])))reject('reference_invalid');
    if(value.headless!==(role==='collector'))reject('headless_mismatch');
    if(!Number.isSafeInteger(value.windowCount)||value.windowCount<0||value.windowCount>128)reject('window_count');
    if(!Number.isSafeInteger(value.bootstrapCount)||value.bootstrapCount<0||value.bootstrapCount>value.windowCount)reject('bootstrap_count');
    if(role==='reader'&&value.windowCount<1)reject('reader_windows');
    if(role==='reader'&&value.bootstrapCount!==1)reject('reader_bootstrap');
    if(role==='collector'&&(value.windowCount!==0||value.bootstrapCount!==0))reject('collector_windows');
    if(s.pins){
      for(const [key,condition]of [['ownerId','owner_mismatch'],['profileId','profile_mismatch'],['targetId','target_mismatch']])if(s.pins[key]!==value[key])reject(condition);
      if(s.session!==value.sessionId)reject('session_mismatch');
      if(document){
        for(const [key,condition]of [['frameId','frame_mismatch'],['documentId','document_mismatch'],['contextId','context_mismatch']])if(s.pins[key]!==value[key])reject(condition);
        if(value.lifecycleReady!==true)reject('lifecycle_unready');
      }
    }
    return value;
  }catch(error){const cause=diagnostics.record(error,role,phase,'unavailable','xnp_manual_owner_unavailable');invalidate(role);throw cause;}};
  return Object.freeze({
    // Called from the owned CDP dispatcher, including after capture cleanup.
    observe(role,event){
      if(!roles.includes(role))return;
      const s=state[role];if(!s.pins||s.closed)return;
      const p=event.params??{};
      if(event.method==='Target.targetDestroyed'&&p.targetId===s.pins.targetId||
        event.method==='Target.detachedFromTarget'&&p.sessionId===s.session){invalidate(role);return;}
      if(event.sessionId!==s.session)return;
      if(['Inspector.detached','Inspector.targetCrashed','Runtime.executionContextsCleared'].includes(event.method)||
        event.method==='Runtime.executionContextDestroyed'&&String(p.executionContextId)===s.pins.contextId||
        event.method==='Runtime.executionContextCreated'&&p.context?.auxData?.isDefault&&p.context.auxData.frameId===s.pins.frameId&&String(p.context.id)!==s.pins.contextId||
        event.method==='Page.frameDetached'&&p.frameId===s.pins.frameId||
        event.method==='Page.frameNavigated'&&!p.frame?.parentId)invalidate(role);
    },
    invalidate(){diagnostics.record(null,'pair','joint','unavailable','xnp_manual_owner_unavailable');for(const role of roles)invalidate(role);},
    async execute(raw){
      const message=manualOwnerOperation(raw), role=message.role,s=state[role];
      if(message.owner!==owner)throw failure();
      if(message.op==='manual_capture_cancel'){
        if(!s.handle||message.handle!==s.handle)throw failure();s.captureCancel?.abort();return{closed:true};
      }
      if(message.op==='manual_abort'){
        if(message.handle!==null&&(!s.handle||message.handle!==s.handle))throw failure();
        if(s.cleanup){
          if(s.preserved&&!message.preserveReader){
            s.preserved=false;
            s.cleanup=s.cleanup.then(async()=>{await retire(role,false);return {closed:true};});
          }
          return s.cleanup;
        }
        s.captureCancel?.abort();s.closed=true;failed=true;
        s.preserved=message.preserveReader;
        s.cleanup=(async()=>{
          if(message.handle===null)await s.actor.cancel();else await s.actor.abort(s.handle);
          await retire(role,message.preserveReader);
          return {closed:true};
        })();
        return s.cleanup;
      }
      if(failed||s.closed||s.invalid)throw diagnostics.record(null,role,message.op==='manual_prepare'?'prepare':'acquire_rpc','rejected','xnp_manual_owner_unavailable');
      if(message.op==='manual_inventory'){
        const value=await checked(role);
        return {windowCount:value.windowCount,bootstrapCount:value.bootstrapCount};
      }
      if(message.op==='manual_prepare'){
        if(s.preparing||s.pins)throw failure();s.preparing=true;
        try{
          const ready=await s.actor.prepare();exact(ready,['handle','pins']);exact(ready.pins,pinKeys);
          if(!ref(ready.handle)||!pinKeys.every(k=>ref(ready.pins[k])))throw failure();
          s.pins=Object.freeze({...ready.pins});s.handle=ready.handle;
          const value=await snapshot(role,true,s.pins);s.session=value?.sessionId;
          await checked(role,true);
          if(failed||s.closed)throw failure();
          const other=state[role==='reader'?'collector':'reader'];
          if(other.pins&&(other.pins.ownerId===s.pins.ownerId||other.pins.profileId===s.pins.profileId))throw failure();
          return {handle:s.handle,pins:s.pins};
        }catch(error){const cause=diagnostics.record(error,role,'prepare','unavailable','xnp_manual_owner_unavailable');invalidate(role);if(s.handle)await s.actor.abort(s.handle).catch(()=>{});throw cause;}
      }
      if(!s.pins)throw failure();
      if(message.op==='manual_capture'){
        if(typeof capture!=='function'||s.captureCancel||!roles.every(r=>state[r].acquired)||message.handle!==s.handle||pinKeys.some(k=>message.request.pins[k]!==s.pins[k]))throw failure();
        s.captureSeen??=new Set();
        if(s.captureSeen.size>=32||s.captureSeen.has(message.request.correlation)||s.captureDeadline&&message.request.deadlineMS>s.captureDeadline)throw failure();
        s.captureSeen.add(message.request.correlation);s.captureDeadline=message.request.deadlineMS;
        s.captureCancel=new AbortController();
        try{
          await checked(role,true,'inspect_after');
          const result=await capture(message.request,{signal:s.captureCancel.signal,proofSession:s.session,onUnsafe:()=>invalidate(role)});
          await checked(role,true,'inspect_after');
          if(failed||s.invalid||s.closed||s.captureCancel.signal.aborted)throw failure();
          return result;
        }catch(error){invalidate(role);throw error;}
        finally{s.captureCancel=null;}
      }
      if(message.op==='manual_inspect'){
        const phase=s.acquired?'inspect_after':'inspect_before';
        const value=await checked(role,true,phase);
        if(s.invalid||failed)throw diagnostics.record(null,role,phase,'scope_lost','xnp_manual_owner_unavailable');
        return {pins:s.pins,windowCount:value.windowCount,bootstrapCount:value.bootstrapCount,paused:!s.acquired,invalid:false,headless:value.headless};
      }
      if(message.op==='manual_acquire'){
        if(!roles.every(r=>state[r].pins)||s.acquired||message.handle!==s.handle||pinKeys.some(k=>message.request[k]!==s.pins[k]))throw failure();
        s.acquired=true;
        try{
          const wire=await s.actor.acquire(s.handle,message.request,undefined,message.startupBudgetMS);
          if(failed||s.invalid){wire.fill(0);throw failure();}
          return wire;
        }catch(error){const cause=diagnostics.record(error,role,'acquire_rpc','unavailable','xnp_manual_owner_unavailable');invalidate(role);throw cause;}
      }
      throw failure();
    },
  });
}

// Private, default-off QA transport. OS provenance belongs to the compiled Go
// owner backend; this module cannot grant it through a request or JSON flag.
import {startupFailureLatch} from './x-manual-startup-diagnostic.mjs';
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
  const operations={manual_prepare:['id','op','owner','role'],manual_acquire:['id','op','owner','role','handle','request'],
    manual_inspect:['id','op','owner','role'],manual_inventory:['id','op','owner','role'],
    manual_abort:['id','op','owner','role','handle','preserveReader']};
  const op=value&&Object.getOwnPropertyDescriptor(value,'op')?.value;
  const keys=typeof op==='string'&&Object.hasOwn(operations,op)?operations[op]:null;
  if(!keys)throw failure();exact(value,keys);
  if(!Number.isSafeInteger(value.id)||value.id<1||!Number.isSafeInteger(value.owner)||value.owner<1||!roles.includes(value.role))throw failure();
  if('handle' in value&&!(value.op==='manual_abort'&&value.handle===null)&&!ref(value.handle))throw failure();
  if(value.op==='manual_abort'&&(typeof value.preserveReader!=='boolean'||value.role==='collector'&&value.preserveReader))throw failure();
  if(value.op==='manual_acquire'){
    exact(value.request,requestKeys);
    if(!requestKeys.every(k=>ref(value.request[k]))||value.request.role!==value.role)throw failure();
  }
  return value;
}
export function createXManualOwnerPort({enabled=false,owner,createOwners,snapshot,retire,diagnostics=startupFailureLatch()}={}) {
  if(enabled!==true||!Number.isSafeInteger(owner)||owner<1||![createOwners,snapshot,retire].every(v=>typeof v==='function'))throw failure();
  const actors=createOwners();
  if(!actors||!roles.every(role=>['prepare','acquire','abort','cancel'].every(k=>typeof actors[role]?.[k]==='function')))throw failure();
  const state=Object.fromEntries(roles.map(role=>[role,{actor:actors[role],pins:null,handle:null,preparing:false,acquired:false,invalid:false,closed:false,session:null}]));
  let failed=false;
  const invalidate=role=>{diagnostics.record(null,role,'joint','scope_lost','xnp_manual_owner_unavailable');state[role].invalid=true;failed=true;};
  const checked=async(role,document=false,phase='prepare')=>{try{
    const s=state[role], value=await snapshot(role,document,s.pins);
    if(!value||value.alive!==true||!['ownerId','profileId','targetId','sessionId'].every(k=>ref(value[k]))||
      value.headless!==(role==='collector')||!Number.isSafeInteger(value.windowCount)||value.windowCount<0||value.windowCount>128||
      !Number.isSafeInteger(value.bootstrapCount)||value.bootstrapCount<0||value.bootstrapCount>value.windowCount||
      role==='reader'&&(value.windowCount<1||value.bootstrapCount!==1)||role==='collector'&&(value.windowCount!==0||value.bootstrapCount!==0))throw failure();
    if(s.pins&&(['ownerId','profileId','targetId'].some(k=>s.pins[k]!==value[k])||s.session!==value.sessionId||
      document&&(s.pins.frameId!==value.frameId||s.pins.documentId!==value.documentId||s.pins.contextId!==value.contextId||value.lifecycleReady!==true)))throw failure();
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
      if(message.op==='manual_abort'){
        if(message.handle!==null&&(!s.handle||message.handle!==s.handle))throw failure();
        if(s.cleanup){
          if(s.preserved&&!message.preserveReader){
            s.preserved=false;
            s.cleanup=s.cleanup.then(async()=>{await retire(role,false);return {closed:true};});
          }
          return s.cleanup;
        }
        s.closed=true;failed=true;
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
      if(message.op==='manual_inspect'){
        const value=await checked(role,true,s.acquired?'inspect_after':'inspect_before');
        if(s.invalid||failed)throw failure();
        return {pins:s.pins,windowCount:value.windowCount,bootstrapCount:value.bootstrapCount,paused:!s.acquired,invalid:false,headless:value.headless};
      }
      if(message.op==='manual_acquire'){
        if(!roles.every(r=>state[r].pins)||s.acquired||message.handle!==s.handle||pinKeys.some(k=>message.request[k]!==s.pins[k]))throw failure();
        s.acquired=true;
        try{
          const wire=await s.actor.acquire(s.handle,message.request);
          if(failed||s.invalid){wire.fill(0);throw failure();}
          return wire;
        }catch(error){const cause=diagnostics.record(error,role,'acquire_rpc','unavailable','xnp_manual_owner_unavailable');invalidate(role);throw cause;}
      }
      throw failure();
    },
  });
}

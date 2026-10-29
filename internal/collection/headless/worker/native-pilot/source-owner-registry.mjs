const failures=new WeakMap();
const operations=new Set(['source_initialize','source_check','source_begin','source_seed','source_proof',
  'source_capture','source_end','source_close']);
const record=value=>value&&[Object.prototype,null].includes(Object.getPrototypeOf(value));
const reject=reason=>{const error=new Error('native_source_control_rejected');failures.set(error,reason);return error;};

// Factory selection is supplied by sealed composition, never by a control message.
// A slot owns one process-lifetime provider. Terminal calls may cancel pending work;
// a factory that finishes after shutdown still retains its cleanup obligation.
export function createSourceOwnerRegistry(factories={}){
  if(!record(factories))throw reject('source_unavailable');
  const catalog=new Map();
  for(const source of Object.keys(factories)){
    const descriptor=Object.getOwnPropertyDescriptor(factories,source);
    if(!/^[a-z][a-z0-9_]{0,31}$/u.test(source)||typeof descriptor?.value!=='function')throw reject('source_unavailable');
    catalog.set(source,descriptor.value);
  }
  const slots=new Map();let closed=false,closePromise;
  const retire=slot=>{
    slot.closing??=slot.creation.then(owner=>owner.close());
    return slot.closing;
  };
  const execute=async(source,operation,params,transport)=>{
    if(closed)throw reject('source_closed');
    if(!catalog.has(source)||!record(params)||Array.isArray(params))throw reject('source_unavailable');
    if(operation==='source_bootstrap'){
      if(slots.has(source))throw reject('source_replay');
      const slot={active:false,closing:null,owner:null,creation:null};
      slots.set(source,slot);
      slot.creation=Promise.resolve().then(()=>catalog.get(source)(params,transport)).then(owner=>{
        if(!owner||typeof owner.execute!=='function'||typeof owner.close!=='function')throw reject('source_unavailable');
        slot.owner=owner;return owner;
      });
      slot.creation.catch(()=>{});
      await slot.creation;
      if(closed||slot.closing){await retire(slot);throw reject('source_closed');}
      return{bound:true};
    }
    if(!operations.has(operation))throw reject('source_operation');
    const slot=slots.get(source);
    if(!slot||slot.closing)throw reject('source_unavailable');
    if(operation==='source_close'){
      await retire(slot);return{closed:true};
    }
    if(!slot.owner)throw reject('source_unavailable');
    if(slot.active&&(operation!=='source_end'||slot.active.operation==='source_end'))throw reject('source_busy');
    const token={operation};slot.active=token;
    try{
      const result=await slot.owner.execute(operation,params);
      if(closed||slot.closing||slot.active!==token)throw reject('source_closed');
      return result;
    }finally{if(slot.active===token)slot.active=false;}
  };
  const close=()=>{
    closed=true;
    closePromise??=(async()=>{
      const results=await Promise.allSettled([...slots.values()].map(retire));
      if(results.some(result=>result.status==='rejected'))throw reject('source_cleanup_uncertain');
    })();
    return closePromise;
  };
  return Object.freeze({execute,close});
}

export function sourceOwnerRegistryFailure(error){return failures.get(error)??null;}

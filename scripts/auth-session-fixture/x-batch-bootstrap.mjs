import {performance} from 'node:perf_hooks';
import {MANUAL_TIMING} from './x-manual-timing-policy.mjs';

const failures=new WeakMap();
const fail=(stage='state',condition='pending')=>{
  const error=new Error(`batch_bootstrap_${stage}_${condition}`);
  failures.set(error,Object.freeze({stage,condition}));
  return error;
};
export const batchBootstrapDiagnostic=error=>failures.get(error)??Object.freeze({stage:'unknown',condition:'unavailable'});
const valid=id=>typeof id==='string'&&id.length>0&&id.length<=512;

// Holds only the target created for this batch; never adopts a user target.
export function createBatchBootstrap({call,reserveTarget,releaseTarget,
  timeoutMS=MANUAL_TIMING.captureCleanupMS,pollMS=Math.min(50,timeoutMS),now=()=>performance.now(),sleep=ms=>new Promise(resolve=>setTimeout(resolve,ms))}){
  if(typeof call!=='function'||typeof now!=='function'||typeof sleep!=='function'||
    !Number.isSafeInteger(timeoutMS)||timeoutMS<1||timeoutMS>60000||!Number.isSafeInteger(pollMS)||pollMS<1||pollMS>timeoutMS)throw fail('state','invalid_reply');
  let target=null,pending=false,closing=null,closeRequested=false;
  const inventory=async(invoke=(method,params)=>call(method,params),stage='inventory_before')=>{
    let result;
    try{result=await invoke('Target.getTargets',{},stage);}catch(error){throw failures.has(error)?error:fail(stage,'rpc');}
    if(!Array.isArray(result?.targetInfos)||result.targetInfos.length>128||result.targetInfos.some(row=>!valid(row?.targetId))||
      new Set(result.targetInfos.map(row=>row.targetId)).size!==result.targetInfos.length)throw fail(stage,'invalid_reply');
    return result.targetInfos;
  };
  const close=async()=>{
    if(pending)throw fail();
    if(!target)return;
    const owned=target,start=now();let last=start;
    if(!Number.isFinite(start))throw fail('state','clock_invalid');
    const remaining=stage=>{
      const time=now();
      if(!Number.isFinite(time)||time<last)throw fail(stage,'clock_invalid');
      last=time;
      const left=Math.floor(start+timeoutMS-time);
      if(left<=0)throw fail(stage,'timeout');
      return left;
    };
    // Bound RPCs and waits by one monotonic deadline. A late reply cannot
    // release ownership, and a second close request is never sent implicitly.
    const within=async(stage,operation)=>{
      const left=remaining(stage);let timer;
      try{
        const result=await Promise.race([
          Promise.resolve().then(()=>operation(remaining(stage))).catch(error=>{throw failures.has(error)?error:fail(stage,'rpc');}),
          new Promise((_,reject)=>{timer=setTimeout(()=>reject(fail(stage,'timeout')),left);}),
        ]);
        remaining(stage);
        return result;
      }finally{clearTimeout(timer);}
    };
    const invoke=(method,params,stage)=>within(stage,left=>call(method,params,left));
    if((await inventory(invoke)).some(row=>row.targetId===owned)){
      if(!closeRequested){
        closeRequested=true;
        const result=await invoke('Target.closeTarget',{targetId:owned},'close');
        if(result?.success!==true)throw fail('close','invalid_reply');
      }
      while((await inventory(invoke,'inventory_after')).some(row=>row.targetId===owned)){
        await within('absence',left=>sleep(Math.min(pollMS,left)));
      }
    }
    remaining('release');
    try{releaseTarget?.(owned);}catch{throw fail('release','rpc');}
    target=null;closeRequested=false;
  };
  return Object.freeze({
    async begin(){
      if(target||pending||closing)throw fail();
      pending=true;
      let before;
      try{before=new Set((await inventory()).map(row=>row.targetId));}
      catch(error){pending=false;throw error;}
      const created=await call('Target.createTarget',{url:'https://x.com/home',background:true});
      if(!valid(created?.targetId)||before.has(created.targetId))throw fail();
      target=created.targetId;reserveTarget?.(target);pending=false;
      return target;
    },
    close(){
      if(!closing)closing=close().finally(()=>{closing=null;});
      return closing;
    },
  });
}

// Private application composition only. This one-use grant is never decoded
// from JSON, exported as a receipt, or installed through QA environment flags.
import {performance} from 'node:perf_hooks';
import {MANUAL_TIMING} from './x-manual-timing-policy.mjs';
import {manualSeedFailure} from './x-manual-retained-setup.mjs';
import {createNativeOperationScope} from '../../internal/collection/headless/worker/native-pilot/native-source-kernel.mjs';

export function createApplicationSeedGrant({runSeed,now=()=>performance.now(),wallNow=()=>Date.now(),budgetMS=MANUAL_TIMING.attemptAuthorizationMaxMS}={}){
  if(typeof runSeed!=='function'||typeof now!=='function'||typeof wallNow!=='function'||!Number.isSafeInteger(budgetMS)||budgetMS<1||budgetMS>MANUAL_TIMING.attemptAuthorizationMaxMS)throw manualSeedFailure('preconditions','disabled');
  let used=false,state='idle',deadline,scope,first;
  const fail=(_stage,reason)=>{
    state='failed';
    first??=manualSeedFailure('preconditions',reason==='clock'?'clock_regressed':reason==='deadline'?'deadline':'rejected','unknown');
    return first;
  };
  const current=()=>{if(!scope)throw fail('lifetime','closed');scope.remaining();};
  const permitted=kind=>{
    if(kind==='transfer'?state!=='running':!['running','complete'].includes(state))return false;
    try{current();return true;}catch{return false;}
  };
  return Object.freeze({
    transferAllowed:()=>permitted('transfer'),
    startupAllowed:()=>permitted('startup'),
    assertCurrent(){if(!['running','complete'].includes(state))throw manualSeedFailure('preconditions','rejected','unknown');current();},
    assertCaptureDeadline(value){current();if(state!=='complete'||!Number.isSafeInteger(value)||value>deadline)throw manualSeedFailure('preconditions','deadline','unknown');},
    cancel(){used=true;state='failed';scope?.cancel();},
    async run(message){
      if(used)throw manualSeedFailure('preconditions','replay','unknown');used=true;
      const started=now(),wall=wallNow();
      if(!Number.isFinite(started)||!Number.isSafeInteger(wall)){state='failed';throw manualSeedFailure('preconditions','clock_regressed');}
      deadline=wall+budgetMS;state='running';
      scope=createNativeOperationScope({now,admitted:started,deadlineMS:started+budgetMS,validTime:Number.isFinite,fail,
        rejected(_stage,error){state='failed';first??=error;return first;}});
      try{
        const result=await scope.run('seed',()=>runSeed(message));
        current();if(state!=='running')throw manualSeedFailure('preconditions','rejected','unknown');state='complete';return result;
      }catch(error){state='failed';throw error;}
    },
  });
}

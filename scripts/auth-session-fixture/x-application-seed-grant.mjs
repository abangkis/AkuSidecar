// Private application composition only. This one-use grant is never decoded
// from JSON, exported as a receipt, or installed through QA environment flags.
import {performance} from 'node:perf_hooks';
import {MANUAL_TIMING} from './x-manual-timing-policy.mjs';
import {manualSeedFailure} from './x-manual-retained-setup.mjs';

export function createApplicationSeedGrant({runSeed,now=()=>performance.now(),wallNow=()=>Date.now(),budgetMS=MANUAL_TIMING.attemptAuthorizationMaxMS}={}){
  if(typeof runSeed!=='function'||typeof now!=='function'||typeof wallNow!=='function'||!Number.isSafeInteger(budgetMS)||budgetMS<1||budgetMS>MANUAL_TIMING.attemptAuthorizationMaxMS)throw manualSeedFailure('preconditions','disabled');
  let used=false,state='idle',started,last,deadline;
  const current=()=>{
    const time=now();
    if(!Number.isFinite(time)||time<last){state='failed';throw manualSeedFailure('preconditions','clock_regressed','unknown');}
    last=time;
    if(time-started>=budgetMS){state='failed';throw manualSeedFailure('preconditions','deadline','unknown');}
  };
  const permitted=kind=>{
    if(kind==='transfer'?state!=='running':!['running','complete'].includes(state))return false;
    try{current();return true;}catch{return false;}
  };
  return Object.freeze({
    transferAllowed:()=>permitted('transfer'),
    startupAllowed:()=>permitted('startup'),
    assertCurrent(){if(!['running','complete'].includes(state))throw manualSeedFailure('preconditions','rejected','unknown');current();},
    assertCaptureDeadline(value){current();if(state!=='complete'||!Number.isSafeInteger(value)||value>deadline)throw manualSeedFailure('preconditions','deadline','unknown');},
    async run(message){
      if(used)throw manualSeedFailure('preconditions','replay','unknown');used=true;
      started=last=now();const wall=wallNow();
      if(!Number.isFinite(started)||!Number.isSafeInteger(wall)){state='failed';throw manualSeedFailure('preconditions','clock_regressed');}
      deadline=wall+budgetMS;state='running';let timer;
      try{
        const result=await Promise.race([runSeed(message),new Promise((_,reject)=>{timer=setTimeout(()=>{state='failed';reject(manualSeedFailure('preconditions','deadline','unknown'));},budgetMS);})]);
        current();if(state!=='running')throw manualSeedFailure('preconditions','rejected','unknown');state='complete';return result;
      }catch(error){state='failed';throw error;}
      finally{clearTimeout(timer);}
    },
  });
}

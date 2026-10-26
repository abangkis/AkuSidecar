// QA sequencing only. Status cannot grant account/session, Store or reader authority.
// Keep this function self-contained: fresh controllers embed these exact tested bytes.
export function createCaptureStartGate({readStatus,openReader,deadlineMS,signal,now=Date.now,pause,readTimeoutMS=1000,pollMS=100}) {
  if(typeof readStatus!=='function'||typeof openReader!=='function'||typeof now!=='function'||
    pause!==undefined&&typeof pause!=='function'||!Number.isSafeInteger(deadlineMS)||
    !Number.isSafeInteger(readTimeoutMS)||readTimeoutMS<1||readTimeoutMS>1000||
    !Number.isSafeInteger(pollMS)||pollMS<1||pollMS>1000)throw new Error('capture_gate_configuration');
  const sleep=pause??(ms=>new Promise(resolve=>{
    const done=()=>{clearTimeout(timer);signal?.removeEventListener('abort',done);resolve();};
    const timer=setTimeout(done,ms);signal?.addEventListener('abort',done,{once:true});if(signal?.aborted)done();
  }));
  let attempt;
  async function run(){
    for(;;){
      if(signal?.aborted)return{opened:false,reason:'canceled'};
      const left=deadlineMS-now();if(left<=0)return{opened:false,reason:'capture_start_deadline'};
      const controller=new AbortController();let timedOut=false,timer,onCancel;
      let report;
      try{
        const canceled=new Promise((_,reject)=>{
          onCancel=()=>{controller.abort();reject(new Error('capture_gate_canceled'));};
          signal?.addEventListener('abort',onCancel,{once:true});if(signal?.aborted)onCancel();
        });
        const timeout=new Promise((_,reject)=>{timer=setTimeout(()=>{timedOut=true;controller.abort();reject(new Error('capture_gate_timeout'));},Math.min(left,readTimeoutMS));});
        report=await Promise.race([Promise.resolve().then(()=>readStatus({signal:controller.signal})),canceled,timeout]);
      }catch{
        return{opened:false,reason:signal?.aborted?'canceled':timedOut?'status_timeout':'status_unavailable'};
      }finally{clearTimeout(timer);signal?.removeEventListener('abort',onCancel);controller.abort();}
      if(signal?.aborted)return{opened:false,reason:'canceled'};
      if(now()>=deadlineMS)return{opened:false,reason:'capture_start_deadline'};
      const status=report?.status,evidence=report?.evidence;
      if(evidence?.firstFailure)return{opened:false,reason:'attempt_failed'};
      if(status?.source!=='x'||status.state!=='active'||!Number.isSafeInteger(status.remainingMs)||status.remainingMs<=0||status.remainingMs>60000)
        return{opened:false,reason:'attempt_unavailable'};
      const stages=evidence?.stages;
      if(evidence?.dropped!==false||!Array.isArray(stages)||stages.length>96||stages.some(s=>!s||typeof s.stage!=='string'||!Number.isSafeInteger(s.elapsedMS)||s.elapsedMS<0))
        return{opened:false,reason:'evidence_unavailable'};
      const entered=stages.filter(s=>s.stage==='capture_entered'),returned=stages.filter(s=>s.stage==='capture_returned');
      if(returned.length)return{opened:false,reason:'capture_window_missed'};
      if(entered.length>1)return{opened:false,reason:'evidence_unavailable'};
      if(entered.length===1){
        // This sends one ordinary reader request; the backend still validates
        // ownership, capability lifetime and the original attempt deadline.
        if(signal?.aborted)return{opened:false,reason:'canceled'};
        try{await openReader({captureEnteredMS:entered[0].elapsedMS});}
        catch{return{opened:false,reason:'reader_dispatch_failed'};}
        return{opened:true,reason:'capture_active',captureEnteredMS:entered[0].elapsedMS};
      }
      try{await sleep(Math.min(pollMS,Math.max(0,deadlineMS-now())));}
      catch{return{opened:false,reason:signal?.aborted?'canceled':'wait_failed'};}
    }
  }
  // Repeated start events share this one attempt, including every failure.
  return()=>attempt??=(run());
}

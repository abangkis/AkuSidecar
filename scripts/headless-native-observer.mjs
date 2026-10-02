import {spawn} from 'node:child_process';
import {createInterface} from 'node:readline';

export async function startNativeObserver(executable) {
  const child=spawn(executable,[],{windowsHide:true,stdio:['pipe','pipe','pipe']});
  const records=[],rootRecords=[];
  let closed=false,exitCode=null;
  const input=createInterface({input:child.stdout,crlfDelay:Infinity});
  let readyResolve,readyReject,rootReadyResolve;
  const ready=new Promise((resolve,reject)=>{readyResolve=resolve;readyReject=reject;});
  child.on('error',error=>readyReject(error));
  child.stdin.on('error',()=>{});
  child.on('close',code=>{closed=true;exitCode=code;input.close();readyReject(new Error('native observer exited before ready'));});
  input.on('line',line=>{
    if(line.length>65536)return;
    let value;try{value=JSON.parse(line);}catch{return;}
    if(value.type==='ready')readyResolve(value);
    else if(value.type==='sample'&&records.length<256)records.push(value.sample);
    else if(value.type==='root_sample'&&rootRecords.length<256)rootRecords.push(value.sample);
    else if(value.type==='root_ready')rootReadyResolve?.(value);
  });
  const timer=setTimeout(()=>readyReject(new Error('native observer readiness timeout')),7000);
  let metadata;
  try{metadata=await ready;}catch(error){child.kill();throw error;}finally{clearTimeout(timer);}
  return {async bind(pid){
    if(!metadata.rootBindingSupported)return {supported:false};
    if(!Number.isSafeInteger(pid)||pid<1)throw new Error('exact root PID required');
    let bindTimer;
    try{
      const ack=new Promise((resolve,reject)=>{rootReadyResolve=resolve;bindTimer=setTimeout(()=>reject(new Error('root observer readiness timeout')),5000);});
      child.stdin.write(JSON.stringify({type:'bind_root',pid})+'\n');
      const value=await ack;
      if(value.pid!==pid)throw new Error('root observer identity mismatch');
      return {supported:true,exactRootPid:pid};
    }finally{clearTimeout(bindTimer);rootReadyResolve=null;}
  },async close(){
    child.stdin.end('stop\n');
    const deadline=Date.now()+5000;
    while(!closed&&Date.now()<deadline)await new Promise(resolve=>setTimeout(resolve,50));
    if(!closed)child.kill();
    return {closed,exitCode,records,rootRecords};
  }};
}

export function summarizeNativeVisibility(trace, pid, mode = 'headless') {
  const records=trace?.records||[];
  const start=records.find(s=>s.trigger==='trace_start');
  const end=records.find(s=>s.trigger==='trace_end');
  const complete=Boolean(trace?.closed&&trace.exitCode===0&&start&&end&&Number.isSafeInteger(pid)&&pid>0
    &&records.every(s=>s.status==='available'&&!s.windowsTruncated));
  return {scope:mode === 'quiet' ? 'passive_windows_quiet_probe_lifetime' : 'passive_windows_headless_worker_lifetime',status:complete?'bounded_observation_complete':'partial',
    sampleCount:records.length,startAt:start?.at||null,endAt:end?.at||null,
    exactRootPid:pid||null,visibleRootSamples:records.filter(s=>(s.windows||[]).some(w=>w.pid===pid&&w.visible)).length,
    visibleNonMinimizedRootSamples:records.filter(s=>(s.windows||[]).some(w=>w.pid===pid&&w.visible&&!w.minimized)).length,
    rootForegroundEvents:records.filter(s=>s.trigger==='foreground_event'&&s.eventPid===pid).length,
    zeroBlinkingGuaranteed:false,
    afterInitRootVisibility:summarizeRootVisibility(trace,pid),
    limitations:['window state sampled at 100ms; brief visibility between polls can be missed',
      'foreground hook supplements polling; pixels and DWM occlusion are not observed',
      mode === 'quiet' ? 'Quiet probe with a minimized local host; application Settings, Bridge and interactive journeys require separate evidence'
        : 'standalone headless source capture only; Quiet and interactive journeys require separate evidence']};
}

function summarizeRootVisibility(trace,pid) {
  const records=trace?.rootRecords||[];
  const start=records.find(s=>s.trigger==='root_start'),end=records.find(s=>s.trigger==='root_end');
  const identityKnown=Number.isSafeInteger(pid)&&pid>0&&records.length>0&&records.every(s=>s.pid===pid);
  const complete=Boolean(trace?.closed&&trace.exitCode===0&&identityKnown&&start&&end
    &&records.every(s=>s.pid===pid&&s.status==='available'&&s.scanComplete===true));
  return {scope:'exact_root_after_init_before_restore',status:complete?'bounded_observation_complete':'partial',
    sampleCount:records.length,startAt:start?.at||null,endAt:end?.at||null,
    visibleRootSamples:identityKnown?records.filter(s=>s.visibleRootCount>0).length:null,
    visibleNonMinimizedRootSamples:identityKnown?records.filter(s=>s.visibleNonMinimizedRootCount>0).length:null,
    rootForegroundSamples:identityKnown?records.filter(s=>s.rootForeground).length:null,
    limitations:['starts after worker init acknowledgement; launch/startup is covered separately',
      'exact root windows only; scans at 100ms and cannot guarantee absence of brief visibility between polls']};
}

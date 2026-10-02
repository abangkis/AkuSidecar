import {spawn} from 'node:child_process';
import {createInterface} from 'node:readline';

export async function startNativeObserver(executable) {
  const child=spawn(executable,[],{windowsHide:true,stdio:['pipe','pipe','pipe']});
  const records=[];
  let closed=false,exitCode=null;
  const input=createInterface({input:child.stdout,crlfDelay:Infinity});
  let readyResolve,readyReject;
  const ready=new Promise((resolve,reject)=>{readyResolve=resolve;readyReject=reject;});
  child.on('error',error=>readyReject(error));
  child.stdin.on('error',()=>{});
  child.on('close',code=>{closed=true;exitCode=code;input.close();readyReject(new Error('native observer exited before ready'));});
  input.on('line',line=>{
    if(line.length>65536)return;
    let value;try{value=JSON.parse(line);}catch{return;}
    if(value.type==='ready')readyResolve(value);
    else if(value.type==='sample'&&records.length<256)records.push(value.sample);
  });
  const timer=setTimeout(()=>readyReject(new Error('native observer readiness timeout')),7000);
  try{await ready;}catch(error){child.kill();throw error;}finally{clearTimeout(timer);}
  return {async close(){
    child.stdin.end('stop\n');
    const deadline=Date.now()+5000;
    while(!closed&&Date.now()<deadline)await new Promise(resolve=>setTimeout(resolve,50));
    if(!closed)child.kill();
    return {closed,exitCode,records};
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
    limitations:['window state sampled at 100ms; brief visibility between polls can be missed',
      'foreground hook supplements polling; pixels and DWM occlusion are not observed',
      mode === 'quiet' ? 'Quiet probe with a minimized local host; application Settings, Bridge and interactive journeys require separate evidence'
        : 'standalone headless source capture only; Quiet and interactive journeys require separate evidence']};
}

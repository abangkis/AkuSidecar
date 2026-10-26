import test from 'node:test';
import assert from 'node:assert/strict';
import {createCaptureStartGate} from './x-qa-capture-start-gate.mjs';

const report=(stages=[],extra={})=>({status:{source:'x',state:'active',remainingMs:5000},
  evidence:{stages:stages.map((stage,i)=>({stage,elapsedMS:i+1})),dropped:false},...extra});
const active=()=>report(['startup_accepted','capture_entered']);

test('active status cannot open reader; verified backend entry opens exactly once',async()=>{
  const steps=[report(['startup_accepted']),report(['startup_accepted']),active()];
  const order=[];let clock=0;
  const start=createCaptureStartGate({deadlineMS:5000,now:()=>clock,pause:async ms=>{clock+=ms;},
    readStatus:async()=>{const next=steps.shift();order.push(next.evidence.stages.some(s=>s.stage==='capture_entered')?'backend_entered':'waiting');return next;},
    openReader:({captureEnteredMS})=>{assert.equal(captureEnteredMS,2);order.push('open_post');}});
  const first=start(),second=start();assert.equal(first,second);
  const result=await first;assert.equal(result.opened,true);assert.deepEqual(order,['waiting','waiting','backend_entered','open_post']);
  await start();assert.equal(order.filter(v=>v==='open_post').length,1);
});

for(const [name,value,want] of [
  ['already_complete',report(['capture_entered','capture_returned']),'capture_window_missed'],
  ['completion_without_entry',report(['capture_returned']),'capture_window_missed'],
  ['duplicate_entry',report(['capture_entered','capture_entered']),'evidence_unavailable'],
  ['dropped_evidence',{...active(),evidence:{...active().evidence,dropped:true}},'evidence_unavailable'],
  ['missing_evidence',{...active(),evidence:null},'evidence_unavailable'],
  ['capture_failure',{...active(),evidence:{...active().evidence,firstFailure:{stage:'capture',detail:'PRIVATE'}}},'attempt_failed'],
  ['stopped',{...active(),status:{source:'x',state:'stopped',remainingMs:1000}},'attempt_unavailable'],
  ['expired',{...active(),status:{source:'x',state:'active',remainingMs:0}},'attempt_unavailable'],
  ['wrong_source',{...active(),status:{source:'linkedin',state:'active',remainingMs:1000}},'attempt_unavailable'],
])test(`reader remains closed for ${name}`,async()=>{
  let opens=0;
  const start=createCaptureStartGate({deadlineMS:5000,now:()=>0,readStatus:async()=>value,openReader:()=>opens++});
  const result=await start();assert.equal(result.reason,want);assert.equal(result.opened,false);
  await start();assert.equal(opens,0);assert.equal(JSON.stringify(result).includes('PRIVATE'),false);
});

test('a backend that never enters consumes only the existing wait budget',async()=>{
  let clock=0,reads=0;
  const start=createCaptureStartGate({deadlineMS:300,now:()=>clock,pause:async ms=>{clock+=ms;},
    readStatus:async()=>{reads++;return report();},openReader:()=>assert.fail('stalled capture opened reader')});
  const result=await start();assert.equal(result.reason,'capture_start_deadline');assert.equal(clock,300);assert.equal(reads,3);
});

test('a stalled status call is bounded and aborted without reader dispatch',async()=>{
  let observedSignal;
  const start=createCaptureStartGate({deadlineMS:Date.now()+1000,readTimeoutMS:20,
    readStatus:({signal})=>{observedSignal=signal;return new Promise(()=>{});},openReader:()=>assert.fail('timed-out status opened reader')});
  assert.equal((await start()).reason,'status_timeout');assert.equal(observedSignal.aborted,true);
});

test('read errors are sanitized, and a failed dispatch is never retried',async()=>{
  let opens=0;
  for(const phase of ['status','dispatch']){
    const start=createCaptureStartGate({deadlineMS:Date.now()+1000,readStatus:async()=>{if(phase==='status')throw Error('PRIVATE');return active();},
      openReader:()=>{opens++;throw Error('PRIVATE');}});
    const result=await start();assert.equal(result.reason,phase==='status'?'status_unavailable':'reader_dispatch_failed');
    assert.equal(JSON.stringify(result).includes('PRIVATE'),false);await start();
  }
  assert.equal(opens,1);
});

test('closing the attempt cancels status waiting and ignores a late positive response',async()=>{
  const stop=new AbortController();let release;
  const waiting=new Promise(resolve=>release=resolve);
  const start=createCaptureStartGate({deadlineMS:Date.now()+5000,signal:stop.signal,
    readStatus:()=>waiting,openReader:()=>assert.fail('late response reopened reader')});
  const result=start();stop.abort();assert.equal((await result).reason,'canceled');release(active());
  await Promise.resolve();assert.equal((await start()).opened,false);
});

test('capture finishing between samples cannot create a false overlap request',async()=>{
  const samples=[report(),report(['capture_entered','capture_returned'])];
  const start=createCaptureStartGate({deadlineMS:Date.now()+1000,pause:async()=>{},readStatus:async()=>samples.shift(),openReader:()=>assert.fail('missed interval reopened capture')});
  assert.equal((await start()).reason,'capture_window_missed');
});

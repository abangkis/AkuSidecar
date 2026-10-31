import test from 'node:test';
import assert from 'node:assert/strict';
import {PassThrough} from 'node:stream';
import {spawnSync} from 'node:child_process';
import {EventEmitter} from 'node:events';
import {createMainCDP,serveMainBrowserTransport,MAIN_TRANSPORT_SCHEMA} from './main-browser-transport-protocol.mjs';
import {mainTransportLifecycle,monitorMainTransportProcess} from './main-transport-lifecycle.mjs';

const tick=()=>new Promise(resolve=>setImmediate(resolve));
async function waitFor(predicate){for(let i=0;i<100;i++){if(predicate())return;await tick();}assert.fail('synthetic lifecycle did not settle');}
function cdp(t){
  const input=new PassThrough(),output=new PassThrough(),requests=[],causes=[];
  output.on('data',wire=>requests.push(JSON.parse(wire.toString().slice(0,-1))));
  const transport=createMainCDP({input,output,onFailure:cause=>causes.push(cause)});t.after(()=>transport.close());
  return {input,output,requests,causes,transport};
}
function assertClosedShape(value){
  assert.deepEqual(Object.keys(value).sort(),['schema','reason','domain','method','role','stage'].sort());
  assert.equal(value.schema,'aku.main-transport-lifecycle.v1');assert.ok(Buffer.byteLength(JSON.stringify(value))<=512);
  assert.ok(!JSON.stringify(value).includes('PRIVATE'));
}

test('ordinary timeout remains globally fatal and sanitizes a noncatalog method',async t=>{
  const f=cdp(t);
  await assert.rejects(f.transport.call('PRIVATE.method',{cookie:'PRIVATE'},'',10));
  assert.equal(f.transport.alive(),false);assert.equal(f.causes.length,1);
  assert.deepEqual(f.causes[0],mainTransportLifecycle({reason:'command_timeout',domain:'cdp',method:'other',role:'ordinary',stage:'command'}));
  assertClosedShape(f.causes[0]);f.transport.close();assert.equal(f.causes.length,1);
});

for(const [kind,reason]of [['end','input_end'],['input_error','input_error'],['output_error','output_error'],['malformed','invalid_frame'],['array','invalid_frame'],['listener','listener_failure'],['limit','inbound_limit']])test(kind+' remains fatal with a closed first cause',async t=>{
  const f=cdp(t);
  if(kind==='end')f.input.end();
  if(kind==='input_error')f.input.emit('error',new Error('PRIVATE'));
  if(kind==='output_error')f.output.emit('error',new Error('PRIVATE'));
  if(kind==='malformed')f.input.write('PRIVATE\0');
  if(kind==='array')f.input.write('[]\0');
  if(kind==='listener'){f.transport.subscribe(()=>{throw Error('PRIVATE');});f.input.write(JSON.stringify({method:'PRIVATE.event'})+'\0');}
  if(kind==='limit')f.input.write(Buffer.alloc(16*1024*1024+1));
  await waitFor(()=>f.causes.length===1);assert.equal(f.transport.alive(),false);assert.equal(f.causes[0].reason,reason);assertClosedShape(f.causes[0]);
});

test('a mismatched reply session remains globally fatal even for the IG local capability',async t=>{
  const f=cdp(t),scope=f.transport.createInstagramOwnedScope();
  const pending=scope.call('Target.createTarget',{url:'about:blank',background:true,forTab:false},'',1000);
  f.input.write(JSON.stringify({id:f.requests[0].id,sessionId:'PRIVATE-peer-session',result:{targetId:'PRIVATE-target'}})+'\0');
  await assert.rejects(pending);assert.equal(f.transport.alive(),false);assert.equal(f.causes[0].reason,'reply_scope');
  assert.equal(f.causes[0].role,'ig_reader');assert.equal(f.causes[0].method,'Target.createTarget');assertClosedShape(f.causes[0]);
});

test('server writes exactly one lifecycle frame before invoking close and dropping terminal work',async t=>{
  const input=new PassThrough(),output=new PassThrough(),rawInput=new PassThrough(),rawOutput=new PassThrough(),frames=[],order=[];
  output.on('data',wire=>{frames.push(JSON.parse(wire));order.push('frame');});
  let final;
  const server=serveMainBrowserTransport({input,output,connect:async()=>({input:rawInput,output:rawOutput}),onClosed:cause=>{final=cause;order.push('closed');}});t.after(()=>server.close());
  input.write(JSON.stringify({op:'main_transport_bootstrap',schema:MAIN_TRANSPORT_SCHEMA,nonce:'a'.repeat(48),transport:'inherited-stdio'})+'\n');await waitFor(()=>frames.some(q=>q.ready));
  input.write(JSON.stringify({id:1,op:'main_call',method:'Browser.getVersion',params:{},sessionId:'',timeoutMS:10})+'\n');
  await new Promise(resolve=>setTimeout(resolve,20));await waitFor(()=>final);
  const events=frames.filter(q=>q.event==='main_transport_lifecycle');assert.equal(events.length,1);
  assert.deepEqual(Object.keys(events[0]).sort(),['event','result']);assertClosedShape(events[0].result);
  assert.equal(events[0].result.reason,'command_timeout');assert.deepEqual(final,events[0].result);assert.equal(order.at(-1),'closed');
  assert.equal(frames.some(q=>q.id===1),false);server.close();assert.equal(frames.filter(q=>q.event==='main_transport_lifecycle').length,1);
});

test('process monitoring installs no exception/rejection recovery and records no error text',()=>{
  const emitter=new EventEmitter(),causes=[];
  const stop=monitorMainTransportProcess(cause=>causes.push(mainTransportLifecycle(cause)),emitter);
  assert.equal(emitter.listenerCount('uncaughtException'),0);assert.equal(emitter.listenerCount('unhandledRejection'),0);
  emitter.emit('uncaughtExceptionMonitor',new Error('PRIVATE'),'unhandledRejection');
  assert.equal(causes[0].reason,'unhandled_rejection');assertClosedShape(causes[0]);stop();assert.equal(emitter.listenerCount('uncaughtExceptionMonitor'),0);
});

for(const reason of ['uncaught_exception','unhandled_rejection'])test(reason+' still exits the real Node process unsuccessfully',()=>{
  const url=new URL('./main-transport-lifecycle.mjs',import.meta.url).href;
  const trigger=reason==='uncaught_exception'?"setImmediate(()=>{throw Error('PRIVATE')})":"Promise.reject(Error('PRIVATE'))";
  const script=`import {monitorMainTransportProcess,mainTransportLifecycle} from ${JSON.stringify(url)};monitorMainTransportProcess(cause=>process.stdout.write(JSON.stringify(mainTransportLifecycle(cause))+'\\n'));${trigger};`;
  const child=spawnSync(process.execPath,['--unhandled-rejections=throw','--input-type=module','--eval',script],{encoding:'utf8',windowsHide:true});
  assert.equal(child.status,1);const cause=JSON.parse(child.stdout);assert.equal(cause.reason,reason);assertClosedShape(cause);
});

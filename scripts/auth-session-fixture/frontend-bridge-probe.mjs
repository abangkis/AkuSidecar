// Non-browser contract probe: the real relay executes against the real isolated
// HTTP server. Manufactured messages here never count as trusted UI input.
import vm from 'node:vm';
import assert from 'node:assert/strict';
const origin=process.argv[2];
const parsed=new URL(origin);
if(process.argv.length!==3||parsed.protocol!=='http:'||parsed.hostname!=='127.0.0.1'||!parsed.port||parsed.pathname!=='/')throw new Error('private origin required');
const relayResponse=await fetch(origin+'/split-ui-bridge.js');assert.equal(relayResponse.status,200);
const relay=await relayResponse.text();
const listeners=new Map(),messages=[],traces=[],timers=new Set(),calls=[];
const window={
 addEventListener:(name,fn)=>listeners.set(name,fn),
 postMessage:(message)=>messages.push(message),dispatchEvent(){},
 akuNativePostDiagnostics:{record:(trace,phase,details)=>traces.push({trace,phase,...details}),errorKind:()=> 'other'},
};
const context={window,location:{origin,hash:'',href:origin+'/'},console:{info(){}},Date,performance,AbortController,CustomEvent:class{},
 sessionStorage:{getItem:()=>null,setItem(){}},history:{},
 setTimeout:(fn,ms)=>{const t=setTimeout(fn,ms);t.unref();timers.add(t);return t;},clearTimeout,
 fetch:async(path,options)=>{const reply=await fetch(origin+path,{...options,headers:{...options?.headers,Origin:origin}});calls.push({path,status:reply.status});return reply;},
};
vm.runInNewContext(relay,context);
const send=async(data)=>await listeners.get('message')({source:window,origin,data});
await send({type:'AKU_BROWSER_READER_BROKER_READY'});
for(const type of ['AKU_BROWSER_BRIDGE_PING','AKU_BROWSER_PROBE_SOURCE_SESSIONS']){
 await send({type});assert.equal(messages.at(-1).type,'AKU_BROWSER_PASSIVE_PROBE_SKIPPED');
}
await send({type:'AKU_BROWSER_CONFIGURE_BACKGROUND_DISPATCH'});
assert.ok(calls.some(c=>c.path==='/api/split-capture/actions'&&c.status===200));
const requestId='broker_'+ 'b'.repeat(32);
await send({type:'AKU_BROWSER_OPEN_NATIVE_POST',requestId,source:'x',url:'https://x.com/aku_fixture_only/status/1'});
assert.equal(messages.at(-1).type,'AKU_BROWSER_NATIVE_POST_OPEN_FAILED');
assert.equal(messages.at(-1).requestId,requestId);
assert.match(messages.at(-1).message,/QA frontend: native activation is disabled/);
assert.ok(calls.some(c=>c.path==='/api/split-capture/reader-broker/cancel'&&c.status===200));
assert.ok(traces.some(t=>t.trace===requestId&&t.phase==='relay_request_end'&&t.status===409));
for(const t of timers)clearTimeout(t);
process.stdout.write(JSON.stringify({realRelayAsset:true,realBootstrap:true,passiveHeadlessSkipped:2,typedBackgroundSuccess:true,nativeDenied:true,correlatedCancel:true,trustedUIInput:false,nativeActivation:false})+'\n');

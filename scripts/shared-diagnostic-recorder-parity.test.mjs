// Workspace integration fixtures: both tracked consumers, no build or browser.
import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {readFile} from 'node:fs/promises';
import {createHash} from 'node:crypto';
import vm from 'node:vm';
import {createDiagnosticTrace} from '../internal/collection/headless/worker/native-pilot/diagnostic-trace.mjs';
import {validateIGTrace} from '../internal/collection/headless/worker/native-pilot/instagram-diagnostic-trace.mjs';
import {verifySharedDiagnosticCore} from './build-x-native-owner.mjs';

const bridge=new URL('../../AkuBridge/diagnostics/',import.meta.url);
const sources=['state-core.js','recorder.js','bridge-contract.js','chrome-transport.js'].map(name=>readFileSync(new URL(name,bridge),'utf8'));
const plain=value=>JSON.parse(JSON.stringify(value));
function pair({sequenceLimit=10000,throwProgress=false}={}){
 const realm=vm.createContext({});for(const source of sources)vm.runInContext(source,realm);
 const clock={time:1000.5,throwNow:false},frames=[];
 const native=createDiagnosticTrace({schema:'aku.ig-trace.v1',sequenceLimit,elapsedLimitMS:60000,
  now:()=>{if(clock.throwNow)throw Error('synthetic clock');return clock.time;},
  onProgress:value=>{if(throwProgress)throw Error('synthetic receiver');frames.push(value);},validate:validateIGTrace});
 return{realm,clock,frames,native,bridge:realm.AkuDiagnosticRecorder.create('capture')};
}

test('native runtime core is byte-identical and a packaging mismatch fails closed',async()=>{
 const canonical=readFileSync(new URL('state-core.js',bridge));
 const runtime=readFileSync(new URL('../internal/collection/headless/worker/vendor/diagnostic-state-core.js',import.meta.url));
 assert.deepEqual(runtime,canonical);
 assert.equal(await verifySharedDiagnosticCore(),createHash('sha256').update(canonical).digest('hex'));
 await assert.rejects(verifySharedDiagnosticCore(async file=>{
  const value=await readFile(file);return file.endsWith('diagnostic-state-core.js')?Buffer.concat([value,Buffer.from(' drift')]):value;
 }),/diagnostic_core_source_mismatch/u);
 await assert.rejects(verifySharedDiagnosticCore(async()=>{throw Object.assign(Error('missing source'),{code:'ENOENT'});}),{code:'ENOENT'});
});

test('the same snapshot reads preserve the distinct sequence and knownness contracts',()=>{
 const p=pair();
 assert.deepEqual([p.bridge.snapshot().sequence,p.bridge.snapshot().sequence],[0,0]);
 assert.deepEqual([p.native.snapshot().sequence,p.native.snapshot().sequence],[1,2]);
 assert.deepEqual(plain(p.bridge.snapshot().elapsed),{known:false});
 assert.equal(p.native.snapshot().elapsedMS,0);
});

for(const c of [
 {name:'zero',elapsed:0,native:0,bridgeKnown:true},
 {name:'fractional',elapsed:12.25,native:13,bridgeKnown:true},
 {name:'backward',elapsed:-1,native:null,bridgeKnown:true},
 {name:'not finite',elapsed:NaN,native:null,bridgeKnown:false},
 {name:'infinite',elapsed:Infinity,native:null,bridgeKnown:false},
 {name:'over native limit',elapsed:60001,native:null,bridgeKnown:true},
 {name:'clock throws',elapsed:undefined,native:null,bridgeKnown:false,throws:true},
])test(`same clock fixture: ${c.name}`,()=>{
 const p=pair();p.native.budget(30000);p.clock.time+=c.elapsed;p.clock.throwNow=c.throws===true;
 p.bridge.advance('body_read',{known:true,value:c.elapsed});p.native.note('body_read');
 const b=plain(p.bridge.snapshot()),n=p.frames.at(-1);
 assert.deepEqual(b.elapsed,c.bridgeKnown?{known:true,value:c.elapsed}:{known:false});
 assert.equal(b.complete,true);assert.equal(Object.hasOwn(b,'budgetMS'),false);
 assert.equal(n.elapsedMS,c.native);assert.equal(n.complete,c.native!==null);
 assert.equal(n.budgetMS,c.native===null?null:30000);
 assert.equal(n.remainingMS,c.native===null?null:Math.max(0,30000-c.native));
 assert.equal(validateIGTrace(n),true);
});

test('remaining budget reaching zero does not gain cancellation authority',()=>{
 const p=pair();p.native.budget(30);p.clock.time+=31;
 p.bridge.advance('body_read');p.native.note('body_read');p.bridge.advance('handoff');p.native.note('handoff');
 assert.equal(p.frames.at(-1).remainingMS,0);assert.equal(p.frames.at(-1).stage,'handoff');
 assert.equal(p.bridge.snapshot().location.phase,'handoff');assert.equal(p.frames.at(-1).complete,true);
});

test('native budget validation and wire schema remain independent from Bridge',()=>{
 const p=pair();p.native.budget(0);p.native.note('body_read');
 assert.throws(()=>p.native.snapshot(),/diagnostic_projection_invalid/u);
 assert.equal(validateIGTrace(plain(p.bridge.snapshot())),false);
 const valid=pair();valid.native.note('body_read');assert.equal(validateIGTrace(valid.native.snapshot()),true);
});

test('same events retain each consumer sequence, first cause and cleanup policy',()=>{
 const p=pair();p.bridge.advance('body_read');p.native.note('body_read');
 p.bridge.completed('body_read');p.native.done('body_read');
 for(const recorder of [p.bridge,p.native]){
  recorder.disposition('started');recorder.fail('body_read','body_encoding');recorder.fail('cleanup','cleanup_failed');
  recorder.cleanup('uncertain','stream_close_failed');recorder.cleanup('uncertain','cleanup_failed');
 }
 const b=plain(p.bridge.snapshot()),n=p.native.snapshot();
 assert.equal(b.sequence,1);assert.equal(n.sequence,8);
 assert.equal(b.cause.condition,'body_encoding');assert.equal(n.cause.condition,'body_encoding');
 assert.equal(b.cleanup.condition,'cleanup_failed');assert.equal(n.cleanup.condition,'stream_close_failed');
 assert.equal(b.complete,true);assert.equal(n.complete,true);
});

test('native saturation reserves one incomplete marker and final cleanup flush',()=>{
 const p=pair({sequenceLimit:6});
 for(let i=0;i<12;i++){p.bridge.advance('body_read');p.native.note('body_read');}
 assert.deepEqual(p.frames.map(v=>v.sequence),[1,2,3,4,5]);
 p.native.fail('body_read','body_encoding');p.native.cleanup('uncertain','stream_close_failed');
 p.native.flush();p.native.flush();
 assert.deepEqual(p.frames.map(v=>v.sequence),[1,2,3,4,5,6]);
 const n=p.frames.at(-1);assert.equal(n.complete,false);assert.equal(n.cause.condition,'body_encoding');
 assert.equal(n.cleanup.condition,'stream_close_failed');
 assert.equal(p.bridge.snapshot().sequence,12);assert.equal(p.bridge.snapshot().complete,true);
});

test('failed progress delivery remains diagnostic-only in both adapters',()=>{
 const p=pair({throwProgress:true});p.native.note('body_read');assert.equal(p.native.snapshot().complete,false);
 const r=p.realm,progress=r.AkuBridgeDiagnostics.createProgress({runtimeRevision:'source-adapters-v131',adapterRuntimeRevision:'source-adapters-v131',clock:()=> '2026-11-16T00:00:00.000Z'});
 const publisher=r.AkuChromeDiagnostics.createPublisher({attemptId:'00000000-0000-4000-8000-000000000001',validAttempt:r.AkuBridgeDiagnostics.validAttempt,
  project:r.AkuBridgeDiagnostics.project,send:()=>{throw Error('synthetic receiver');},dropped:()=>progress.incomplete()});
 const sample=progress.update('capture_started',{source:'instagram'});
 publisher.publish(progress.snapshot().sequence,sample);assert.equal(progress.snapshot().complete,false);publisher.dispose();
});

test('Bridge wall-clock projection does not become native elapsed time',()=>{
 const p=pair(),r=p.realm;let at='2026-11-16T00:00:00.000Z';
 const progress=r.AkuBridgeDiagnostics.createProgress({runtimeRevision:'source-adapters-v131',adapterRuntimeRevision:'source-adapters-v131',clock:()=>at});
 progress.update('capture_started',{source:'instagram'});at='2026-11-16T00:00:00.013Z';
 assert.equal(progress.update('extracting_block',{source:'instagram'}).updatedAt,at);
 assert.deepEqual(plain(progress.snapshot().elapsed),{known:false});
});

test('snapshot isolation and sticky incompleteness survive both projections',()=>{
 const p=pair();
 for(const recorder of [p.bridge,p.native]){recorder.fail('body_read','body_read');recorder.incomplete();}
 const b=p.bridge.snapshot(),n=p.native.snapshot();b.cause.condition='changed';n.cause.condition='changed';
 b.cleanup.state='changed';n.cleanup.state='changed';
 assert.equal(p.bridge.snapshot().cause.condition,'body_read');assert.equal(p.native.snapshot().cause.condition,'body_read');
 assert.equal(p.bridge.snapshot().cleanup.state,'not_started');assert.equal(p.native.snapshot().cleanup.state,'not_started');
 assert.equal(p.bridge.snapshot().complete,false);assert.equal(p.native.snapshot().complete,false);
});

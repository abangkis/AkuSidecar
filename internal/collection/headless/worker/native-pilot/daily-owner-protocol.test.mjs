import test from 'node:test';
import assert from 'node:assert/strict';
import {PassThrough} from 'node:stream';
import {serveDailyOwnerPipe,DAILY_OWNER_SCHEMA} from './daily-owner-protocol.mjs';
import {applicationPostOperation} from '../../../../../scripts/auth-session-fixture/x-application-reader-post.mjs';

async function fixture(t,overrides={}){
  const input=new PassThrough(),output=new PassThrough(),events=[],waiting=[];let buffer='',id=0;
  const calls={initialize:0,begin:[],seed:[],end:[],close:0};
  const owner={initialize:async()=>{calls.initialize++;return{owner:1,readerPID:100};},check:async()=>({readerPID:100,readerProfile:'reader'}),
    begin:async q=>{calls.begin.push(q.generation);return{owner:q.generation+1,readerPID:100,collectorPID:200+q.generation};},
    seed:async q=>{calls.seed.push(q.generation);return{seeded:true};},end:async q=>{calls.end.push(q.generation);return{closed:true};},
    execute:async()=>({accepted:true}),post:async()=>({accepted:true}),close:async()=>{calls.close++;},...overrides};
  const protocol=serveDailyOwnerPipe({input,output,createOwner:async()=>owner});t.after(()=>protocol.close());
  output.on('data',chunk=>{buffer+=chunk;let end;while((end=buffer.indexOf('\n'))>=0){const value=JSON.parse(buffer.slice(0,end));buffer=buffer.slice(end+1);const resolve=waiting.shift();if(resolve)resolve(value);else events.push(value);}});
  const next=()=>events.length?Promise.resolve(events.shift()):new Promise(resolve=>waiting.push(resolve));
  const request=async(op,fields={})=>{const response=next();input.write(JSON.stringify({id:++id,op,...fields})+'\n');return response;};
  input.write(JSON.stringify({op:'daily_owner_bootstrap',schema:DAILY_OWNER_SCHEMA,nonce:'a'.repeat(48),chromePath:'synthetic',readerProfile:'reader'})+'\n');
  assert.equal((await next()).ready,true);assert.equal((await request('daily_reader_initialize')).ok,true);
  return{input,output,request,next,calls,protocol};
}
test('daily reader survives more than the pilot call quota with fresh batch generations',async t=>{
 const f=await fixture(t);
 for(let generation=1;generation<=300;generation++){
   const begin=await f.request('daily_batch_begin',{generation,collectorProfile:'collector-'+generation});assert.equal(begin.ok,true);assert.equal(begin.result.readerPID,100);
   assert.equal((await f.request('daily_batch_seed',{generation,owner:generation+1})).ok,true);
   assert.equal((await f.request('daily_batch_end',{generation})).ok,true);
 }
 assert.equal(f.calls.initialize,1);assert.equal(f.calls.seed.length,300);assert.equal(f.calls.close,0);
});
test('overlapping batch and seed replay fail closed',async t=>{
 const f=await fixture(t);await f.request('daily_batch_begin',{generation:1,collectorProfile:'c1'});await f.request('daily_batch_seed',{generation:1,owner:2});
 assert.equal((await f.request('daily_batch_seed',{generation:1,owner:2})).ok,false);
 assert.equal((await f.request('daily_batch_end',{generation:1})).ok,true);
 assert.equal((await f.request('daily_batch_begin',{generation:2,collectorProfile:'c2'})).ok,false);
 assert.deepEqual(f.calls.seed,[1]);
});
test('stale generation cannot cancel or execute against next collector',async t=>{
 const f=await fixture(t);for(const generation of [1,2]){await f.request('daily_batch_begin',{generation,collectorProfile:'c'+generation});await f.request('daily_batch_seed',{generation,owner:generation+1});if(generation===1)await f.request('daily_batch_end',{generation});}
 assert.equal((await f.request('daily_batch_end',{generation:1})).ok,false);assert.deepEqual(f.calls.end,[1]);
});
test('cleanup uncertainty prevents reuse and EOF closes process owner',async t=>{
 const f=await fixture(t,{end:async()=>{throw new Error('synthetic');}});await f.request('daily_batch_begin',{generation:1,collectorProfile:'c'});
 assert.equal((await f.request('daily_batch_end',{generation:1})).ok,false);
 assert.equal((await f.request('daily_batch_begin',{generation:2,collectorProfile:'c2'})).ok,false);
 f.input.end();await new Promise(resolve=>setImmediate(resolve));assert.equal(f.calls.close,1);
});
test('reader check remains independent of retired batch',async t=>{
 const f=await fixture(t);await f.request('daily_batch_begin',{generation:1,collectorProfile:'c'});await f.request('daily_batch_seed',{generation:1,owner:2});await f.request('daily_batch_end',{generation:1});
 assert.equal((await f.request('daily_reader_check')).ok,true);
});
test('reader RPC may overlap batch creation without consuming batch authority',async t=>{
 let release;const f=await fixture(t,{check:()=>new Promise(r=>{release=()=>r({readerPID:100,readerProfile:'reader'});})});
 const check=f.request('daily_reader_check');
 // Responses are intentionally out of request order; the fixture queue reads
 // them in wire order, as the Go client correlates them by ID.
 const begin=f.request('daily_batch_begin',{generation:1,collectorProfile:'c'});
 const beginReply=await check;assert.equal(beginReply.ok,true);assert.equal(beginReply.result.collectorPID,201);
 release();assert.equal((await begin).ok,true);assert.deepEqual(f.calls.begin,[1]);
});
test('main origin is admitted only through trusted daily post policy',()=>{
 const q={id:1,op:'manual_prepare_post',owner:1,request:{requestId:'broker_'+'a'.repeat(32),source:'x',url:'https://x.com/synthetic/status/1'},marker:'http://127.0.0.1:11122/split-reader-intent?id=split_'+'b'.repeat(32)};
 assert.throws(()=>applicationPostOperation(q));assert.equal(applicationPostOperation(q,{allowMainOrigin:true}),q);
});

import test from 'node:test';
import assert from 'node:assert/strict';
import {readInstagramBootstrapBodyStream} from './instagram-bootstrap-body-stream.mjs';
import {createIGTrace,igFailure,validateIGTrace} from './instagram-diagnostic-trace.mjs';
import {cdpCommandFailure} from './cdp-command-failure.mjs';

async function run(replies, options = {}) {
  const calls = [];
  const scope = {async call(method, params) {
    calls.push({method, params});
    if (method === 'IO.close') { if (options.closeFails) throw Error('private failure'); return {}; }
    const reply = replies.shift(); if (reply instanceof Error) throw reply; return reply;
  }};
  const result = await readInstagramBootstrapBodyStream(scope, 'owned-session', 'owned-stream', {now: () => 100, deadlineAt: 200, maxBytes: 4, ...options});
  return {result, calls};
}
test('bounded stream accepts exact cap with EOF and closes only owned handle', async () => {
  const {result, calls} = await run([{data:'YWJjZA==', base64Encoded:true, eof:true}]);
  assert.equal(result.body.toString(), 'abcd'); result.body.fill(0);
  assert.equal(result.reason, 'none'); assert.equal(result.cleanup, 'verified');
  assert.deepEqual(calls, [{method:'IO.read',params:{handle:'owned-stream',size:5}}, {method:'IO.close',params:{handle:'owned-stream'}}]);
});
test('extra byte and dishonest read size reject without returning body', async () => {
  for (const data of ['abcde', 'abcdef']) {
    const {result} = await run([{data,eof:true}]);
    assert.equal(result.reason,'body_bounds'); assert.equal(result.body,undefined); assert.equal(result.cleanup,'verified');
  }
});
test('empty nonterminal chunk and malformed base64 fail closed', async () => {
  for (const reply of [{data:'',eof:false},{data:'!!!!',eof:true,base64Encoded:true}]) {
    const {result} = await run([reply]); assert.notEqual(result.reason,'none'); assert.equal(result.body,undefined);
  }
});
test('cleanup uncertainty discards bytes and preserves primary failure', async () => {
  const good = await run([{data:'ok',eof:true}],{closeFails:true});
  assert.equal(good.result.reason,'none'); assert.equal(good.result.cleanup,'uncertain'); assert.equal(good.result.body,undefined);
  const bad = await run([new Error('secret')],{closeFails:true});
  assert.equal(bad.result.reason,'body_read'); assert.equal(bad.result.cleanup,'uncertain');
});
test('expired deadline reads no bytes but still closes stream', async () => {
  const {result,calls} = await run([],{deadlineAt:100});
  assert.equal(result.reason,'body_deadline'); assert.deepEqual(calls.map(call=>call.method),['IO.close']);
});

test('read commands, invalid replies and empty non-final chunks have distinct safe causes',async()=>{
  const cases=[
    [Error('PRIVATE_COMMAND'),'read_command_failed'],
    [cdpCommandFailure('timeout',true),'read_command_timeout'],
    [cdpCommandFailure('transport_closed'),'read_transport_closed'],
    [cdpCommandFailure('command_rejected'),'read_cdp_rejected'],
    [cdpCommandFailure('reply_invalid'),'read_reply_invalid'],
    [{data:'ok'},'read_reply_invalid'],
    [{data:1,eof:true},'read_reply_invalid'],
    [{data:'ok',eof:'true'},'read_reply_invalid'],
    [{data:'ok',eof:true,base64Encoded:undefined},'read_reply_invalid'],
    [{data:'ok',eof:true,private:'PRIVATE_BODY'},'read_reply_invalid'],
    [{data:'',eof:false},'read_empty_nonfinal']
  ];
  for(const [reply,condition] of cases){
    const trace=createIGTrace();trace.note('body_read');
    const {result,calls}=await run([reply],{trace});
    const snapshot=trace.snapshot();assert.equal(result.reason,'body_read');assert.equal(result.body,undefined);
    assert.deepEqual(snapshot.cause,{stage:'body_read',condition});assert.equal(validateIGTrace(snapshot),true);
    assert.equal(calls.filter(c=>c.method==='IO.read').length,1);assert.equal(result.cleanup,'verified');
    assert.ok(!JSON.stringify(snapshot).includes('PRIVATE'));
  }
});

test('arbitrary timeout messages and error properties cannot impersonate trusted transport errors',async()=>{
  const forged=Object.assign(Error('diagnostic_request_timeout'),{kind:'timeout',diagnostic:{condition:'read_command_timeout'}});
  Object.defineProperty(forged,'code',{get(){throw Error('PRIVATE_GETTER');}});
  const trace=createIGTrace();trace.note('body_read');
  await run([forged],{trace});assert.equal(trace.snapshot().cause.condition,'read_command_failed');
});

test('reply getters and non-records are rejected as reply format without evaluating private data',async()=>{
  const getter={eof:true};Object.defineProperty(getter,'data',{enumerable:true,get(){throw Error('PRIVATE_GETTER');}});
  for(const reply of [null,[],getter]){
    const trace=createIGTrace();trace.note('body_read');await run([reply],{trace});
    assert.equal(trace.snapshot().cause.condition,'read_reply_invalid');
  }
});

test('first observed cause survives later read and cleanup failures',async()=>{
  for(const reply of [Error('PRIVATE_READ'),{data:'ok'},{data:'',eof:false}]){
    const trace=createIGTrace();trace.note('body_read');trace.fail('root_delivery','extra_status');
    await run([reply],{trace,closeFails:true});
    assert.deepEqual(trace.snapshot().cause,{stage:'root_delivery',condition:'extra_status'});
    assert.deepEqual(trace.snapshot().cleanup,{state:'uncertain',condition:'stream_close_failed'});
  }
  const trace=createIGTrace();trace.note('body_read');await run([igFailure('scope','ownership')],{trace,closeFails:true});
  assert.deepEqual(trace.snapshot().cause,{stage:'scope',condition:'ownership'});
});

test('final empty chunk stays valid and neither EOF nor cleanup creates a read failure',async()=>{
  const trace=createIGTrace();trace.note('body_read');
  const {result}=await run([{data:'',eof:true}],{trace});
  assert.equal(result.reason,'none');assert.equal(result.body.length,0);result.body.fill(0);
  assert.equal(trace.snapshot().cause,null);
  const failedClose=createIGTrace();failedClose.note('body_read');
  const closed=await run([{data:'ok',eof:true}],{trace:failedClose,closeFails:true});
  assert.equal(closed.result.reason,'none');assert.equal(failedClose.snapshot().cause,null);
  assert.equal(failedClose.snapshot().cleanup.condition,'stream_close_failed');
});

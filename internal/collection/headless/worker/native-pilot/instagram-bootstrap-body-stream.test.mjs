import test from 'node:test';
import assert from 'node:assert/strict';
import {readInstagramBootstrapBodyStream} from './instagram-bootstrap-body-stream.mjs';

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

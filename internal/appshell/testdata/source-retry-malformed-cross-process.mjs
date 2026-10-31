// Offline fault-injection peer. It does not launch or connect to a browser.
import assert from 'node:assert/strict';
import {createInterface} from 'node:readline';

const code = 'main_ig_retry_not_dispatched';
const errors = [
  `{"code":"${code}"}`,
  ` { "code" : "${code}" } `,
  `{"code":"native_source_admission_retry","code":"${code}"}`,
  `{"code":"${code}","code":"${code}"}`,
  `{"code":"${code}","co\\u0064e":"${code}"}`,
  `{"Code":"${code}"}`,
  `{"code":"${code}","extra":true}`,
  `{"code":"${code}","diagnostic":null}`,
  `{"code":"${code}","diagnostic":{"private":"fixture"}}`,
  '{"code":"native_source_admission_retry","diagnostic":{"stage":"fixture"}}',
];
let ready = false;
for await (const line of createInterface({input: process.stdin, crlfDelay: Infinity})) {
  const request = JSON.parse(line);
  if (!ready) {
    assert.equal(request.op, 'main_transport_bootstrap');
    process.stdout.write(JSON.stringify({ready: true, schema: request.schema, nonce: request.nonce}) + '\n');
    ready = true;
    continue;
  }
  if (request.op === 'main_source') {
    assert.equal(request.source, 'instagram');
    assert.equal(request.operation, 'source_retry');
    assert.deepEqual(request.params, {});
    const error = errors.shift();
    assert.ok(error);
    process.stdout.write(`{"id":${request.id},"ok":false,"error":${error}}\n`);
  } else if (request.op === 'main_x') {
    assert.equal(request.operation, 'daily_reader_check');
    process.stdout.write(JSON.stringify({id: request.id, ok: true, result: {source: 'x', usable: true}}) + '\n');
  } else {
    assert.equal(request.op, 'main_call');
    assert.equal(request.method, 'Browser.getVersion');
    process.stdout.write(JSON.stringify({id: request.id, ok: true, result: {product: 'Chrome/offline-fault-fixture'}}) + '\n');
  }
}
assert.equal(errors.length, 0);

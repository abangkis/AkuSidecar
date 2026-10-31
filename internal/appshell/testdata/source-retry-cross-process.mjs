// Test-only stdio peer. The production server and registry frame/dispatch every
// request; synthetic owners and CDP streams grant no browser or account access.
import assert from 'node:assert/strict';
import {PassThrough} from 'node:stream';
import {serveMainBrowserTransport} from '../../collection/headless/worker/native-pilot/main-browser-transport-protocol.mjs';
import {nativeRecord, nativeSourceFailure} from '../../collection/headless/worker/native-pilot/native-source-kernel.mjs';

const rawInput = new PassThrough(), rawOutput = new PassThrough();
let bytes = Buffer.alloc(0), sourceClosed = false, xClosed = false;
rawOutput.on('data', chunk => {
  bytes = Buffer.concat([bytes, chunk]);
  let end;
  while ((end = bytes.indexOf(0)) >= 0) {
    const request = JSON.parse(bytes.subarray(0, end).toString('utf8'));
    bytes = Buffer.from(bytes.subarray(end + 1));
    assert.equal(request.method, 'Browser.getVersion');
    rawInput.write(JSON.stringify({id: request.id, result: {product: 'Chrome/offline-source-retry-fixture'}}) + '\0');
  }
});

const syntheticInstagram = async () => {
  let documentFailure = false, ended = false, rearmed = false;
  return {
    async execute(operation, params) {
      assert.equal(nativeRecord(params, []), true, 'fixture operations must have exact empty params');
      if (sourceClosed) throw nativeSourceFailure('lifetime', 'owner');
      switch (operation) {
        case 'source_initialize': return {source: 'instagram'};
        case 'source_seed':
          documentFailure = true;
          throw nativeSourceFailure('evidence', 'document_changed');
        case 'source_end':
          ended = documentFailure;
          return {closed: true};
        case 'source_retry':
          if (!documentFailure || !ended || rearmed) throw nativeSourceFailure('admission', 'retry');
          rearmed = true;
          return {rearmed: true, retryCount: 1, retryLimit: 2, descriptor: {source: 'instagram', recipe: 'instagram-native-v1', readerOwnerID: '123', readerProfileID: 'synthetic-profile'}};
        case 'source_check': return {source: 'instagram', usable: rearmed};
        default: throw nativeSourceFailure('admission', 'operation');
      }
    },
    async close() { sourceClosed = true; },
  };
};

serveMainBrowserTransport({
  input: process.stdin,
  output: process.stdout,
  connect: async () => ({input: rawInput, output: rawOutput}),
  sourceFactories: {
    instagram: syntheticInstagram,
    x: async () => ({
      execute: async () => { throw Error('wrong-source operation reached a source owner'); },
      close: async () => {},
    }),
  },
  createX: async () => ({
    execute: async operation => {
      assert.equal(operation, 'daily_reader_check');
      assert.equal(sourceClosed, false);
      assert.equal(xClosed, false);
      return {source: 'x', usable: true};
    },
    close: async () => { xClosed = true; },
  }),
});

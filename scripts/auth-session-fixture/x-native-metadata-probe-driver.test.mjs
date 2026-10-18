import assert from 'node:assert/strict';
import test from 'node:test';
import {
  CDPFrameDecoder,
  LoginObservationGate,
  MetadataWindow,
  classifyXOperation,
  projectNetworkEvent,
} from './x-native-metadata-probe-driver.mjs';

test('CDP decoder accepts fragmented NUL frames and clears framed bytes', () => {
  const decoder = new CDPFrameDecoder();
  const seen = [];
  const frame = Buffer.from('{"method":"Page.loadEventFired","params":{}}\0');
  decoder.push(frame.subarray(0, 20), value => seen.push(value));
  decoder.push(frame.subarray(20), value => seen.push(value));
  decoder.finish();
  assert.deepEqual(JSON.parse(JSON.stringify(seen)), [{ method: 'Page.loadEventFired', params: {} }]);
  assert.equal(decoder.length, 0);
  assert.ok(decoder.buffer.subarray(0, frame.length - 1).every(byte => byte === 0));
});

test('CDP decoder rejects oversized, invalid UTF-8, duplicate-key and empty frames', () => {
  const small = new CDPFrameDecoder({ maxFrameBytes: 8, maxInputBytes: 128 });
  assert.throws(() => small.push(Buffer.from('{"value":123}\0'), () => {}), /xnp_cdp_frame_too_large/u);

  const invalidUtf8 = new CDPFrameDecoder();
  assert.throws(() => invalidUtf8.push(Buffer.from([0xff, 0]), () => {}), /xnp_cdp_utf8_invalid/u);

  const duplicate = new CDPFrameDecoder();
  assert.throws(() => duplicate.push(Buffer.from('{"id":1,"id":2}\0'), () => {}), /xnp_cdp_json_invalid/u);

  const empty = new CDPFrameDecoder();
  assert.throws(() => empty.push(Buffer.from([0]), () => {}), /xnp_cdp_frame_empty/u);
});

test('X GraphQL operation projection uses only a fixed allowlist', () => {
  const known = 'https://x.com/i/api/graphql/hash123/HomeTimeline?account=SECRET&token=PRIVATE#fragment';
  const unknown = 'https://x.com/i/api/graphql/hash456/SecretAccountName123?user=PRIVATE';
  assert.equal(classifyXOperation(known), 'graphql:HomeTimeline');
  assert.equal(classifyXOperation(unknown), 'graphql:other');
  assert.equal(classifyXOperation('https://x.com/i/api/1.1/account/settings.json?token=PRIVATE'), 'rest:account-settings');
  assert.equal(classifyXOperation('https://api.x.com/1.1/account/verify_credentials.json?token=PRIVATE'), 'rest:verify-credentials');
  assert.equal(classifyXOperation('https://api.x.com/i/api/1.1/account/multi/list.json?account=SECRET'), 'rest:account-multi-list');
  assert.equal(classifyXOperation('https://api.x.com/1.1/users/SECRET/lists.json?token=PRIVATE'), 'rest:other');
  assert.equal(classifyXOperation('https://example.com/i/api/graphql/id/HomeTimeline'), null);
});

test('network metadata projection strips URL, query, request id, headers, body and server strings', () => {
  const requestKey = 'request-id-with-account-secret';
  const request = projectNetworkEvent('Network.requestWillBeSent', {
    requestId: requestKey,
    timestamp: 10,
    request: {
      url: 'https://x.com/i/api/graphql/opaque/Viewer?screen_name=SECRET&token=PRIVATE#private',
      method: 'post',
      headers: { authorization: 'Bearer PRIVATE' },
      postData: 'ACCOUNT BODY SECRET',
    },
  });
  const response = projectNetworkEvent('Network.responseReceived', {
    requestId: requestKey,
    timestamp: 10.25,
    hasExtraInfo: true,
    response: {
      status: 200,
      url: 'https://x.com/i/api/graphql/opaque/Viewer?private=SECRET',
      headers: { 'set-cookie': 'PRIVATE' },
      fromDiskCache: true,
      fromServiceWorker: false,
      fromPrefetchCache: false,
      remoteIPAddress: '192.0.2.2',
    },
  });
  const finished = projectNetworkEvent('Network.loadingFinished', {
    requestId: requestKey,
    timestamp: 10.5,
    encodedDataLength: 1234,
  });
  assert.equal(request.operation, 'graphql:Viewer');
  assert.equal(response.hasExtraInfo, true);
  assert.equal(Object.hasOwn(response, 'statusAmbiguous'), false);

  const window = new MetadataWindow();
  window.observe(request);
  window.observe(response);
  window.observe(finished);
  const projected = window.finish();
  const safe = JSON.stringify(projected);
  for (const secret of [requestKey, 'SECRET', 'PRIVATE', 'authorization', 'set-cookie', '192.0.2.2', 'opaque']) {
    assert.equal(safe.includes(secret), false, `projection contained ${secret}`);
  }
  assert.equal(projected.candidates.length, 1);
  assert.deepEqual(projected.candidates[0], {
    alias: 'r1',
    operation: 'graphql:Viewer',
    method: 'POST',
    status: 200,
    ambiguous: false,
    hasExtraInfo: true,
    cache: {
      servedFromCache: null,
      fromDiskCache: true,
      fromServiceWorker: false,
      fromPrefetchCache: false,
    },
    completion: 'complete',
    bytes: 1234,
    timingMs: { response: 250, completion: 500 },
  });
});

test('redirects and reused request IDs are flagged ambiguous with no status proof', () => {
  const window = new MetadataWindow();
  const first = projectNetworkEvent('Network.requestWillBeSent', {
    requestId: 'reuse-me', timestamp: 1,
    request: { url: 'https://x.com/i/api/graphql/id/HomeTimeline', method: 'GET' },
  });
  const reused = projectNetworkEvent('Network.requestWillBeSent', {
    requestId: 'reuse-me', timestamp: 1.1,
    redirectResponse: { status: 302, headers: { location: 'PRIVATE' } },
    request: { url: 'https://x.com/i/api/graphql/id/Viewer?account=SECRET', method: 'GET' },
  });
  const response = projectNetworkEvent('Network.responseReceived', {
    requestId: 'reuse-me', timestamp: 1.2,
    hasExtraInfo: false,
    response: { status: 200 },
  });
  window.observe(first);
  window.observe(reused);
  window.observe(response);
  const candidate = window.finish().candidates[0];
  assert.equal(candidate.ambiguous, true);
  assert.equal(candidate.status, null);
  assert.equal(candidate.hasExtraInfo, false);
  assert.equal(candidate.operation, 'graphql:HomeTimeline');
});

test('candidate collection is capped at 64', () => {
  const window = new MetadataWindow();
  for (let i = 1; i <= 65; i++) {
    window.observe({
      type: 'request', requestKey: `request-${i}`, operation: 'graphql:other',
      method: 'GET', timestamp: i, redirected: false,
    });
  }
  const result = window.finish();
  assert.equal(result.candidates.length, 64);
  assert.equal(result.dropped, 1);
  assert.equal(result.candidates.at(-1).alias, 'r64');
});

test('Network collection gate stays closed before manual login and permits one reload', () => {
  const gate = new LoginObservationGate();
  assert.throws(() => gate.beginObservation(), /xnp_observation_gate_closed/u);
  assert.throws(() => gate.markNetworkEnabled(), /xnp_network_enable_order_invalid/u);
  assert.equal(gate.networkEnabled, false);
  assert.equal(gate.reloadCount, 0);

  gate.setLoginVisible(true);
  gate.beginObservation();
  gate.markNetworkEnabled();
  gate.markReloaded();
  assert.equal(gate.networkEnabled, true);
  assert.equal(gate.reloadCount, 1);
  assert.throws(() => gate.beginObservation(), /xnp_observation_gate_closed/u);
  assert.throws(() => gate.markReloaded(), /xnp_reload_order_invalid/u);
});

import test from 'node:test';
import assert from 'node:assert/strict';
import { createFixtureXEvidenceIngress } from './x-evidence-ingress.mjs';

const origin = 'http://127.0.0.1:43127';
const sessionId = 'synthetic-session';
const targetId = 'synthetic-target';

function createIngress({ limits, maxWindowMs, maxAgeMs } = {}) {
  let now = 10_000;
  const age = maxAgeMs ?? 30_000;
  const windowMs = maxWindowMs ?? age;
  const window = {
    id: 'local-window-reference',
    startTimestamp: 100,
    endTimestamp: 100 + windowMs / 1000,
    openedAtMs: now,
    expiresAtMs: now + windowMs,
  };
  const ingress = createFixtureXEvidenceIngress({
    scope: {
      ownerId: 'synthetic-owner', contextId: 'synthetic-context',
      sessionId, targetId, origin,
    },
    window,
    clock: () => now,
    limits: { ...(maxAgeMs ? { maxAgeMs } : {}), ...(maxWindowMs ? { maxWindowMs } : {}), ...limits },
  });
  return { ingress, setNow(value) { now = value; }, window };
}

function event(method, params, { sid = sessionId } = {}) {
  return JSON.stringify({ method, sessionId: sid, params });
}

function frame(value) {
  return Buffer.concat([Buffer.from(typeof value === 'string' ? value : JSON.stringify(value), 'utf8'), Buffer.from([0])]);
}

function request(id = 'request-1', { timestamp = 100.1, url = `${origin}/x-fixture-operation`, method = 'POST', extra = {} } = {}) {
  return event('Network.requestWillBeSent', {
    requestId: id,
    timestamp,
    request: { url, method, headers: {}, ...extra },
  });
}

function response(id = 'request-1', { timestamp = 100.2, status = 200, hasExtraInfo = false, extra = {} } = {}) {
  return event('Network.responseReceived', {
    requestId: id,
    timestamp,
    hasExtraInfo,
    response: { url: `${origin}/x-fixture-operation`, status, ...extra },
  });
}

function extraInfo(id = 'request-1', statusCode = 200) {
  return event('Network.responseReceivedExtraInfo', {
    requestId: id, statusCode, headers: { 'set-cookie': 'fixture_secret_cookie=hidden' },
  });
}

function terminal(id = 'request-1', timestamp = 100.3, method = 'Network.loadingFinished') {
  return event(method, { requestId: id, timestamp, errorText: 'fixture_secret_raw_error' });
}

function pushEvents(ingress, ...events) {
  return ingress.push(Buffer.concat(events.map(item => frame(item))));
}

function complete(ingress, id = 'request-1', { cached = false, responseStatus = 200, extraStatus = 200 } = {}) {
  const items = [request(id)];
  if (cached) items.push(event('Network.requestServedFromCache', { requestId: id }));
  items.push(response(id, { hasExtraInfo: true, status: responseStatus }));
  items.push(extraInfo(id, extraStatus));
  items.push(terminal(id));
  return pushEvents(ingress, ...items);
}

test('fragmented UTF-8 and coalesced CDP frames project immutable safe metadata', () => {
  const { ingress } = createIngress();
  const rawRequest = event('Network.requestWillBeSent', {
    requestId: 'request-unicode', timestamp: 100.1,
    request: {
      url: `${origin}/x-fixture-operation`, method: 'POST',
      headers: { 'x-private': 'fixture_secret_header_令牌' },
      postData: 'fixture_secret_post_body',
    },
  });
  const requestFrame = frame(rawRequest);
  const splitAt = requestFrame.indexOf(Buffer.from('令')) + 1;
  const firstPart = Buffer.from(requestFrame.subarray(0, splitAt));
  ingress.push(firstPart);
  firstPart.fill(0);
  ingress.push(Buffer.from(requestFrame.subarray(splitAt)));
  pushEvents(ingress,
    response('request-unicode', { hasExtraInfo: true, extra: {
      headers: { authorization: 'fixture_secret_authorization' },
      location: `${origin}/x-fixture-operation?fixture_secret_query`,
      body: 'fixture_secret_body',
    } }),
    extraInfo('request-unicode', 200),
    terminal('request-unicode'),
    JSON.stringify({ id: 7, error: { message: 'fixture_secret_raw_error' } }));

  const records = ingress.drain();
  assert.equal(records.length, 1);
  assert.equal(records[0].operationId, 'x-fixture-operation-v1');
  assert.equal(records[0].phase, 'completed');
  assert.equal(records[0].statusCode, 200);
  assert.equal(records[0].authorizing, false);
  assert.equal(Object.isFrozen(records), true);
  assert.equal(Object.isFrozen(records[0]), true);
  assert.throws(() => { records[0].statusCode = 401; }, TypeError);
  assert.throws(() => { ingress.status().pending = 9; }, TypeError);
  const safe = JSON.stringify({ records, status: ingress.status() });
  for (const secret of [
    'fixture_secret_header', 'fixture_secret_post_body', 'fixture_secret_query',
    'fixture_secret_authorization', 'fixture_secret_body', 'fixture_secret_cookie',
    'fixture_secret_raw_error', '令牌',
  ]) assert.equal(safe.includes(secret), false, secret);
  assert.equal(ingress.drain().length, 0);
});

test('early and late ExtraInfo are correlated from status-only projections', () => {
  for (const order of ['early', 'late']) {
    const { ingress } = createIngress();
    if (order === 'early') {
      pushEvents(ingress, extraInfo('early-request', 200));
      pushEvents(ingress, request('early-request'), response('early-request', { hasExtraInfo: true }),
        terminal('early-request'));
    } else {
      pushEvents(ingress, request('early-request'), response('early-request', { hasExtraInfo: true }));
      pushEvents(ingress, terminal('early-request'));
      assert.equal(ingress.status().unavailable, false);
      pushEvents(ingress, extraInfo('early-request', 200));
    }
    const record = ingress.drain()[0];
    assert.equal(record.extraInfoExpected, true);
    assert.equal(record.extraInfoReceived, true);
    assert.equal(record.extraInfoStatusCode, 200);
    assert.equal(ingress.status().unavailable, false);
  }
});

test('cache metadata permits the synthetic 200 response and 304 ExtraInfo pair', () => {
  const { ingress } = createIngress();
  complete(ingress, 'cached-request', { cached: true, responseStatus: 200, extraStatus: 304 });
  const [record] = ingress.drain();
  assert.equal(record.statusCode, 200);
  assert.equal(record.extraInfoStatusCode, 304);
  assert.equal(record.cache, 'served_from_cache');
  assert.equal(record.authorizing, false);
  assert.equal(ingress.status().unavailable, false);

  const unflagged = createIngress().ingress;
  complete(unflagged, 'revalidated-request', { responseStatus: 200, extraStatus: 304 });
  assert.equal(unflagged.drain()[0].cache, 'revalidated');
  assert.equal(unflagged.status().unavailable, false);

  const prefetch = createIngress().ingress;
  pushEvents(prefetch, request('prefetch'), response('prefetch', {
    hasExtraInfo: false, extra: { fromPrefetchCache: true },
  }), terminal('prefetch'));
  assert.equal(prefetch.drain()[0].cache, 'served_from_prefetch_cache');

  const cacheConflict = createIngress().ingress;
  complete(cacheConflict, 'cache-conflict', { cached: true, responseStatus: 200, extraStatus: 500 });
  assert.equal(cacheConflict.status().reason, 'response_status_ambiguous');
});

test('expected missing, unexpected, and duplicate ExtraInfo fail closed', () => {
  const missing = createIngress().ingress;
  pushEvents(missing, request(), response('request-1', { hasExtraInfo: true }), terminal());
  assert.equal(missing.status().unavailable, false);
  assert.equal(missing.finish().reason, 'extra_info_missing');

  const unexpected = createIngress().ingress;
  pushEvents(unexpected, request(), response(), extraInfo());
  assert.equal(unexpected.status().reason, 'extra_info_unexpected');

  const duplicate = createIngress().ingress;
  pushEvents(duplicate, request(), extraInfo(), extraInfo());
  assert.equal(duplicate.status().reason, 'extra_info_duplicate');

  const finishMissing = createIngress().ingress;
  pushEvents(finishMissing, request(), response('request-1', { hasExtraInfo: true }));
  assert.equal(finishMissing.finish().reason, 'extra_info_missing');

  const unresolvedOrphan = createIngress().ingress;
  pushEvents(unresolvedOrphan, extraInfo('orphan-only'));
  assert.equal(unresolvedOrphan.finish().reason, 'extra_info_ambiguous');
});

test('requests that begin before the window cannot be revived by a fresh response', () => {
  const { ingress } = createIngress();
  pushEvents(ingress,
    request('stale-request', { timestamp: 99.9 }),
    response('stale-request', { timestamp: 100.2 }),
    terminal('stale-request'));
  assert.equal(ingress.status().reason, 'request_outside_window');
  assert.equal(ingress.drain().length, 0);
});

test('redirect reuse, request ID collisions, and cross-session events fail closed', () => {
  const redirect = createIngress().ingress;
  pushEvents(redirect,
    request('reused-request'),
    event('Network.requestWillBeSent', {
      requestId: 'reused-request', timestamp: 100.15,
      redirectResponse: { headers: { cookie: 'fixture_secret_redirect' } },
      request: { url: `${origin}/elsewhere`, method: 'GET' },
    }));
  assert.equal(redirect.status().reason, 'redirect_unsupported');

  const collision = createIngress().ingress;
  pushEvents(collision, request('same-request'), request('same-request'));
  assert.equal(collision.status().reason, 'request_id_collision');

  const crossSession = createIngress().ingress;
  crossSession.push(frame(event('Network.requestWillBeSent', {
    requestId: 'same-id', timestamp: 100.1,
    request: { url: `${origin}/x-fixture-operation`, method: 'POST' },
  }, { sid: 'different-session' })));
  assert.equal(crossSession.status().reason, 'scope_mismatch');
});

test('response and terminal timestamps remain bounded by the initiating request', () => {
  const urlMismatch = createIngress().ingress;
  pushEvents(urlMismatch, request('url-mismatch'), response('url-mismatch', {
    extra: { url: `${origin}/x-fixture-operation?fixture_secret_query` },
  }));
  assert.equal(urlMismatch.status().reason, 'response_url_unsupported');
  assert.equal(JSON.stringify(urlMismatch.status()).includes('fixture_secret_query'), false);

  const responseBeforeRequest = createIngress().ingress;
  pushEvents(responseBeforeRequest,
    request('ordered-response', { timestamp: 100.2 }),
    response('ordered-response', { timestamp: 100.1 }));
  assert.equal(responseBeforeRequest.status().reason, 'response_before_request');

  const earlyNetworkFinish = createIngress().ingress;
  pushEvents(earlyNetworkFinish,
    request('early-network-finish', { timestamp: 100.1 }),
    response('early-network-finish', { timestamp: 100.25, hasExtraInfo: true }),
    terminal('early-network-finish', 100.2),
    extraInfo('early-network-finish'));
  assert.equal(earlyNetworkFinish.status().unavailable, false);
  const [earlyRecord] = earlyNetworkFinish.drain();
  assert.equal(earlyRecord.phase, 'completed');
  assert.equal(earlyRecord.requestOffsetMs, (100.1 - 100) * 1000);
  assert.equal(earlyRecord.completionOffsetMs, (100.2 - 100) * 1000);
  assert.equal(earlyRecord.responseOffsetMs, (100.25 - 100) * 1000);
  const ticket = earlyNetworkFinish.claimCompletedBodyRequest('early-network-finish');
  assert.ok(ticket);
  assert.equal(earlyNetworkFinish.withBodyRequest(ticket, requestId => requestId), 'early-network-finish');

  const finishedBeforeRequest = createIngress().ingress;
  pushEvents(finishedBeforeRequest,
    request('finish-before-request', { timestamp: 100.2 }),
    response('finish-before-request', { timestamp: 100.3 }),
    terminal('finish-before-request', 100.1));
  assert.equal(finishedBeforeRequest.status().reason, 'loading_finished_before_request');

  const missingResponse = createIngress().ingress;
  pushEvents(missingResponse, request('missing-response'), terminal('missing-response', 100.2));
  assert.equal(missingResponse.status().reason, 'response_missing');

  const failedBeforeResponse = createIngress().ingress;
  pushEvents(failedBeforeResponse,
    request('failed-before-response'),
    response('failed-before-response', { timestamp: 100.25 }),
    terminal('failed-before-response', 100.2, 'Network.loadingFailed'));
  assert.equal(failedBeforeResponse.status().reason, 'loading_failed_before_response');

  const conflictingCacheFlags = createIngress().ingress;
  pushEvents(conflictingCacheFlags,
    request('cache-flags'),
    response('cache-flags', { extra: { fromDiskCache: false } }),
    response('cache-flags', { extra: { fromDiskCache: true } }));
  assert.equal(conflictingCacheFlags.status().reason, 'response_conflict');

  const precise = createIngress().ingress;
  pushEvents(precise,
    request('precise', { timestamp: 100.1001 }),
    response('precise', { timestamp: 100.1002 }),
    terminal('precise', 100.1003));
  const [record] = precise.drain();
  assert.ok(record.requestOffsetMs < record.responseOffsetMs);
  assert.ok(record.responseOffsetMs < record.completionOffsetMs);
});

test('body access tickets exist only for completed qualified requests and are single use', () => {
  const { ingress } = createIngress();
  pushEvents(ingress, request('body-request'), response('body-request', { hasExtraInfo: true }));
  assert.equal(ingress.claimCompletedBodyRequest('body-request'), null);
  pushEvents(ingress, terminal('body-request'));
  assert.equal(ingress.claimCompletedBodyRequest('body-request'), null);
  pushEvents(ingress, extraInfo('body-request'));
  const ticket = ingress.claimCompletedBodyRequest('body-request');
  assert.ok(ticket);
  assert.deepEqual(Object.keys(ticket), []);
  assert.equal(ingress.withBodyRequest(ticket, requestId => requestId), 'body-request');
  assert.throws(() => ingress.withBodyRequest(ticket, () => 'replay'), /fixture_body_ticket_invalid/u);
  assert.equal(ingress.claimCompletedBodyRequest('unrelated-request'), null);

  const ignored = createIngress().ingress;
  pushEvents(ignored, request('other-request', { url: `${origin}/elsewhere` }),
    response('other-request', { extra: { url: `${origin}/elsewhere` } }), terminal('other-request'));
  assert.equal(ignored.claimCompletedBodyRequest('other-request'), null);
});

test('body access tickets are denied for cache-backed responses and after finish', () => {
  const cached = createIngress().ingress;
  complete(cached, 'cached-body-request', { cached: true });
  assert.equal(cached.claimCompletedBodyRequest('cached-body-request'), null);

  const revalidated = createIngress().ingress;
  complete(revalidated, 'revalidated-body-request', { responseStatus: 200, extraStatus: 304 });
  assert.equal(revalidated.drain()[0].cache, 'revalidated');
  assert.equal(revalidated.claimCompletedBodyRequest('revalidated-body-request'), null);

  const finished = createIngress().ingress;
  complete(finished, 'finished-body-request');
  assert.equal(finished.finish().state, 'finished');
  assert.equal(finished.claimCompletedBodyRequest('finished-body-request'), null);
});

test('body access tickets expire with the phase scope and are rechecked after async body reads', async () => {
  const expired = createIngress({ maxWindowMs: 1_000, maxAgeMs: 1_000 });
  complete(expired.ingress, 'expired-body-request');
  const expiredTicket = expired.ingress.claimCompletedBodyRequest('expired-body-request');
  assert.ok(expiredTicket);
  expired.setNow(11_000);
  assert.throws(() => expired.ingress.withBodyRequest(expiredTicket, () => 'late-body'),
    /fixture_body_ticket_expired/u);

  const lost = createIngress();
  complete(lost.ingress, 'lost-body-request');
  const lostTicket = lost.ingress.claimCompletedBodyRequest('lost-body-request');
  const deferred = createDeferred();
  const read = lost.ingress.withBodyRequest(lostTicket, () => deferred.promise);
  lost.ingress.loss();
  deferred.resolve('late-body');
  await assert.rejects(read, /fixture_body_ticket_expired/u);

  const expiring = createIngress({ maxWindowMs: 1_000, maxAgeMs: 1_000 });
  complete(expiring.ingress, 'expiring-body-request');
  const expiringTicket = expiring.ingress.claimCompletedBodyRequest('expiring-body-request');
  const expiringBody = createDeferred();
  const expiryRead = expiring.ingress.withBodyRequest(expiringTicket, () => expiringBody.promise);
  expiring.setNow(11_000);
  expiringBody.resolve('late-body');
  await assert.rejects(expiryRead, /fixture_body_ticket_expired/u);

  const closed = createIngress();
  complete(closed.ingress, 'closed-body-request');
  const closedTicket = closed.ingress.claimCompletedBodyRequest('closed-body-request');
  const pendingBody = createDeferred();
  const closeRead = closed.ingress.withBodyRequest(closedTicket, () => pendingBody.promise);
  closed.ingress.close();
  pendingBody.resolve('late-body');
  await assert.rejects(closeRead, /fixture_body_ticket_expired/u);
});

function createDeferred() {
  let resolve;
  const promise = new Promise(ok => { resolve = ok; });
  return { promise, resolve };
}

test('owner change, target loss, detach, and source loss remain sticky through drain', () => {
  const cases = [
    [() => { const value = createIngress().ingress; value.changeOwner('new-owner'); return value; }, 'owner_changed'],
    [() => { const value = createIngress().ingress; pushEvents(value, event('Target.targetDestroyed', { targetId })); return value; }, 'target_lost'],
    [() => { const value = createIngress().ingress; pushEvents(value, event('Target.detachedFromTarget', { sessionId })); return value; }, 'source_detached'],
    [() => { const value = createIngress().ingress; value.loss(); return value; }, 'source_lost'],
  ];
  for (const [create, reason] of cases) {
    const ingress = create();
    assert.equal(ingress.status().reason, reason);
    ingress.drain();
    assert.equal(ingress.status().reason, reason);
    assert.equal(ingress.status().authorizing, false);
  }
});

test('pending, orphan, alias, and output limits stop further correlation work', () => {
  const pending = createIngress({ limits: { maxPending: 1 } }).ingress;
  pushEvents(pending, request('one'), request('two'));
  assert.equal(pending.status().reason, 'pending_limit');

  const orphan = createIngress({ limits: { maxOrphans: 1 } }).ingress;
  pushEvents(orphan, extraInfo('one'), extraInfo('two'));
  assert.equal(orphan.status().reason, 'orphan_limit');

  const aliases = createIngress({ limits: { maxAliases: 6 } }).ingress;
  pushEvents(aliases, request('one'), request('two'));
  assert.equal(aliases.status().reason, 'alias_limit');

  const output = createIngress({ limits: { maxOutputRecords: 1 } }).ingress;
  complete(output, 'one');
  complete(output, 'two');
  assert.equal(output.status().reason, 'output_limit');
});

test('expiration and end-of-stream preserve fail-closed state', () => {
  const expired = createIngress({ maxAgeMs: 1_000, maxWindowMs: 5_000 });
  pushEvents(expired.ingress, request('expires-soon'));
  expired.setNow(11_001);
  assert.equal(expired.ingress.expire().reason, 'correlation_expired');

  const unterminated = createIngress().ingress;
  unterminated.push(Buffer.from('{"method":"Network.requestWillBeSent"}', 'utf8'));
  assert.equal(unterminated.finish().reason, 'unterminated_frame');

  const closed = createIngress().ingress;
  closed.close();
  assert.equal(closed.status().reason, 'source_closed');
  assert.equal(closed.status().unavailable, true);
});

test('finish rechecks expiry and does not convert an expired window into success', () => {
  const expired = createIngress({ maxAgeMs: 1_000, maxWindowMs: 5_000 });
  pushEvents(expired.ingress, request('finish-expired'));
  expired.setNow(15_000);
  assert.equal(expired.ingress.finish().reason, 'window_expired');
  assert.equal(expired.ingress.status().authorizing, false);
});

test('invalid UTF-8, duplicate keys, excessive depth, and invalid JSON never parse as evidence', () => {
  const cases = [
    [Buffer.from([0xff, 0]), {}, 'invalid_utf8'],
    [frame('{"method":"Network.loadingFinished","method":"Network.loadingFailed"}'), {}, 'malformed_json'],
    [frame('{"a":{"b":{"c":{"d":1}}}}'), { maxJSONDepth: 3 }, 'malformed_json'],
    [frame('{"method":"Unknown.fixtureEvent","params":[1,2,3]}'), { maxJSONTokens: 4 }, 'malformed_json'],
    [frame('{not-json}'), {}, 'malformed_json'],
    [frame('null'), {}, 'malformed_json'],
    [frame('[]'), {}, 'malformed_json'],
    [frame(JSON.stringify({ method: 'Network.requestWillBeSent', sessionId, params: null })), {}, 'malformed_json'],
    [Buffer.concat([Buffer.from([0xef, 0xbb, 0xbf]), frame('{}')]), {}, 'malformed_json'],
    [frame(JSON.stringify({ method: 'Target.targetDestroyed', params: {} })), {}, 'malformed_json'],
    [frame(JSON.stringify({ method: 'Target.detachedFromTarget', params: {} })), {}, 'malformed_json'],
  ];
  for (const [bytes, limits, reason] of cases) {
    const ingress = createIngress({ limits }).ingress;
    ingress.push(bytes);
    assert.equal(ingress.status().reason, reason);
    assert.equal(ingress.drain().length, 0);
  }
});

test('frame, body-bearing input, coalesced chunk, and frame-count caps are enforced', () => {
  const oversizedFrame = createIngress({ limits: { maxFrameBytes: 64 } }).ingress;
  oversizedFrame.push(frame({ id: 1, result: { body: 'fixture_secret_'.repeat(20) } }));
  assert.equal(oversizedFrame.status().reason, 'frame_too_large');

  const bodyFrame = createIngress({ limits: { maxFrameBytes: 128 } }).ingress;
  bodyFrame.push(frame({ id: 2, result: { body: 'fixture_secret_'.repeat(20) } }));
  assert.equal(bodyFrame.status().reason, 'frame_too_large');

  const hugeChunk = createIngress().ingress;
  hugeChunk.push(new Uint8Array(1_048_577));
  assert.equal(hugeChunk.status().reason, 'input_chunk_too_large');

  const manyFrames = createIngress({ limits: { maxFramesPerPush: 2 } }).ingress;
  const ignored = JSON.stringify({ method: 'Unknown.fixtureEvent' });
  manyFrames.push(Buffer.concat([frame(ignored), frame(ignored), frame(ignored)]));
  assert.equal(manyFrames.status().reason, 'too_many_frames');
});

test('a status mismatch without cache evidence is unavailable and outside fixture requests are ignored', () => {
  const mismatch = createIngress().ingress;
  complete(mismatch, 'mismatch', { responseStatus: 401, extraStatus: 304 });
  assert.equal(mismatch.status().reason, 'response_status_ambiguous');

  const cachedConflict = createIngress().ingress;
  complete(cachedConflict, 'cached-conflict', { cached: true, responseStatus: 200, extraStatus: 500 });
  assert.equal(cachedConflict.status().reason, 'response_status_ambiguous');

  const outside = createIngress().ingress;
  pushEvents(outside,
    request('unrelated', { url: 'https://x.example.test/anything', method: 'POST' }),
    event('Network.responseReceivedExtraInfo', { requestId: 'unrelated', statusCode: 200,
      headers: { cookie: 'fixture_secret_cookie' } }));
  assert.equal(outside.status().unavailable, false);
  assert.equal(outside.drain().length, 0);

  const queryRequest = createIngress().ingress;
  pushEvents(queryRequest, request('query-request', {
    url: `${origin}/x-fixture-operation?fixture_secret_query`,
  }));
  assert.equal(queryRequest.status().reason, 'unsupported_request');
  assert.equal(JSON.stringify(queryRequest.status()).includes('fixture_secret_query'), false);
});

test('explicit invalid configuration limits are rejected with a fixed error', () => {
  for (const value of [null, 0, -1, 1.5, NaN, Infinity, 1_048_577]) {
    assert.throws(() => createIngress({ limits: { maxFrameBytes: value } }), {
      name: 'TypeError', message: 'invalid_fixture_x_ingress_configuration',
    });
  }
});

test('clock rollback loses the observation instead of extending its lifetime', () => {
  const { ingress, setNow } = createIngress();
  setNow(10_100);
  ingress.expire();
  setNow(10_050);
  pushEvents(ingress, request());
  assert.equal(ingress.status().reason, 'invalid_clock');
  ingress.drain();
  assert.equal(ingress.status().reason, 'invalid_clock');
  assert.equal(ingress.status().authorizing, false);
});

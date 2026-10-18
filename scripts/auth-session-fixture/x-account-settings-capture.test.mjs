import test from 'node:test';
import assert from 'node:assert/strict';
import { AccountSettingsCapture, accountSettingsBodyMode } from './x-account-settings-capture.mjs';

const request = (id = 'private-request', extra = {}) => ({ requestId: id, request: { method: 'GET', url: 'https://x.com/i/api/1.1/account/settings.json?private=SECRET' }, ...extra });
const response = (extra = {}) => ({ requestId: 'private-request', response: { url: 'https://x.com/i/api/1.1/account/settings.json', status: 200, mimeType: 'application/json', fromDiskCache: false, fromServiceWorker: false, fromPrefetchCache: false, ...extra } });
const finished = { requestId: 'private-request', encodedDataLength: 1115 };
function choose(capture, extra = {}) { capture.observe('Network.requestWillBeSent', request()); capture.observe('Network.responseReceived', response(extra)); }

test('body mode is fixed and a complete eligible response consumes one bound read', async () => {
  assert.equal(accountSettingsBodyMode('two_phase'), false);
  assert.equal(accountSettingsBodyMode('single_phase'), false);
  assert.equal(accountSettingsBodyMode('two_phase_account_settings'), true);
  assert.equal(accountSettingsBodyMode('retained_account_settings'), true);
  assert.equal(accountSettingsBodyMode('retained'), false);
  let count = 0;
  let capture;
  capture = new AccountSettingsCapture({ readBody: id => {
    count++;
    assert.equal(capture.consumeRead('wrong'), false);
    assert.equal(capture.consumeRead(id), true);
    assert.equal(capture.consumeRead(id), false);
    return { schema: 'aku.x.account-settings-shape.v1', diagnostic: 'projected', canAuthorize: false };
  }});
  choose(capture);
  capture.observe('Network.loadingFinished', finished);
  capture.observe('Network.loadingFinished', finished);
  const report = await capture.finish();
  assert.equal(count, 1);
  assert.equal(report.bodyReadAttempts, 1);
  assert.equal(report.diagnostic, 'projected');
  assert.doesNotMatch(JSON.stringify(report), /SECRET|private-request|x\.com/u);
});

test('cache, redirects, missing facts, wrong status/MIME and oversized transfers deny body reads', async () => {
  for (const invalid of [{status:401},{mimeType:'text/html'},{fromDiskCache:true},{fromServiceWorker:true},{fromPrefetchCache:null},{url:'https://other.invalid/'}]) {
    const capture = new AccountSettingsCapture({readBody:()=>{throw new Error('must not read');}});
    choose(capture,invalid);capture.observe('Network.loadingFinished',finished);
    assert.equal((await capture.finish()).bodyReadAttempts,0);
  }
  for (const variant of ['cache','redirect','unfinished','oversized','failed']) {
    let reads=0;const capture=new AccountSettingsCapture({readBody:()=>{reads++;}});
    capture.observe('Network.requestWillBeSent',request('private-request',variant==='redirect'?{redirectResponse:{}}:{}));
    capture.observe('Network.responseReceived',response());
    if(variant==='cache')capture.observe('Network.requestServedFromCache',{requestId:'private-request'});
    if(variant==='failed')capture.observe('Network.loadingFailed',{requestId:'private-request'});
    if(variant!=='unfinished')capture.observe('Network.loadingFinished',{...finished,encodedDataLength:variant==='oversized'?16385:1115});
    assert.equal((await capture.finish()).bodyReadAttempts,0);assert.equal(reads,0);
  }
});

test('reused IDs and multiple matching requests are ambiguous and never selected again', async () => {
  for(const duplicate of [request(),request('second-private-request')]) {
    let reads=0;const capture=new AccountSettingsCapture({readBody:()=>{reads++;}});
    choose(capture);capture.observe('Network.requestWillBeSent',duplicate);capture.observe('Network.loadingFinished',finished);
    const report=await capture.finish();assert.equal(reads,0);assert.equal(report.diagnostic,'ambiguous');assert.equal(report.projection,null);
  }
});

test('unknown routes are ignored; read errors and invalid clocks produce fixed diagnostics',async()=>{
  const ignored=new AccountSettingsCapture({readBody:()=>{throw new Error('must not read');}});
  ignored.observe('Network.requestWillBeSent',{requestId:'secret',request:{method:'GET',url:'https://x.com/i/api/graphql/hash/Viewer'}});
  assert.equal((await ignored.finish()).diagnostic,'not_observed');
  let clock=10;let capture;
  capture=new AccountSettingsCapture({now:()=>clock,readBody:id=>{clock=9;assert.equal(capture.consumeRead(id),false);return Promise.reject(new Error('private server details'));}});
  choose(capture);capture.observe('Network.loadingFinished',finished);
  const report=await capture.finish();assert.equal(report.diagnostic,'candidate_rejected');assert.equal(report.projection,null);
  assert.doesNotMatch(JSON.stringify(report),/private server details/u);
});

test('expired tickets and later ambiguity/coverage loss cannot publish a successful projection',async()=>{
  let clock=0;let expired;
  expired=new AccountSettingsCapture({now:()=>clock,readBody:id=>{
    clock=4000;assert.equal(expired.consumeRead(id),false);
    return {schema:'aku.x.account-settings-shape.v1',diagnostic:'projected',canAuthorize:false};
  }});
  choose(expired);expired.observe('Network.loadingFinished',finished);
  assert.equal((await expired.finish()).diagnostic,'read_failed');
  for(const later of ['multiple','overflow']) {
    let capture;
    capture=new AccountSettingsCapture({readBody:id=>{
      assert.equal(capture.consumeRead(id),true);
      return {schema:'aku.x.account-settings-shape.v1',diagnostic:'projected',canAuthorize:false};
    }});
    choose(capture);capture.observe('Network.loadingFinished',finished);
    if(later==='multiple')capture.observe('Network.requestWillBeSent',request('later-private'));
    else capture.invalidate();
    const result=await capture.finish();assert.equal(result.bodyReadAttempts,1);assert.equal(result.projection,null);
    assert.equal(result.diagnostic,later==='multiple'?'ambiguous':'candidate_rejected');
  }
});

test('separate OPTIONS preflights do not compete with GET, but selected-ID reuse still denies capture',async()=>{
  const preflight={requestId:'preflight-private',request:{...request().request,method:'OPTIONS'}};
  for(const order of ['before','after']) {
    let capture;
    capture=new AccountSettingsCapture({readBody:id=>{
      assert.equal(capture.consumeRead(id),true);
      return {schema:'aku.x.account-settings-shape.v1',diagnostic:'projected',canAuthorize:false};
    }});
    if(order==='before')capture.observe('Network.requestWillBeSent',preflight);
    choose(capture);
    if(order==='after')capture.observe('Network.requestWillBeSent',preflight);
    capture.observe('Network.responseReceived',{requestId:preflight.requestId,response:{status:200}});
    capture.observe('Network.loadingFinished',{requestId:preflight.requestId,encodedDataLength:0});
    capture.observe('Network.loadingFinished',finished);
    const result=await capture.finish();assert.equal(result.diagnostic,'projected');assert.equal(result.matchingRequests,1);assert.equal(result.bodyReadAttempts,1);
  }
  const reused=new AccountSettingsCapture({readBody:()=>{throw new Error('must not read');}});
  choose(reused);
  reused.observe('Network.requestWillBeSent',{...preflight,requestId:'private-request'});
  reused.observe('Network.loadingFinished',finished);
  assert.equal((await reused.finish()).diagnostic,'ambiguous');
});

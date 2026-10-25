import test from 'node:test';
import assert from 'node:assert/strict';
import {CaptureTrafficDiagnostics} from './x-capture-traffic-diagnostics.mjs';
const event=(method,params={})=>({method:'Network.'+method,params:{requestId:'PRIVATE_REQUEST',...params}});
test('ordinary correlation variations remain closed diagnostic categories',()=>{
  const cases=[
    ['orphan_response',[event('responseReceived',{hasExtraInfo:false})]],
    ['orphan_extra',[event('responseReceivedExtraInfo')]],
    ['extra_flag_missing',[event('requestWillBeSent'),event('responseReceived')]],
    ['extra_flag_invalid',[event('requestWillBeSent'),event('responseReceived',{hasExtraInfo:'PRIVATE'})]],
    ['request_reused',[event('requestWillBeSent'),event('requestWillBeSent')]],
    ['redirect',[event('requestWillBeSent',{redirectResponse:{}})]],
    ['response_pending',[event('requestWillBeSent')]],
    ['completion_pending',[event('requestWillBeSent'),event('responseReceived',{hasExtraInfo:false})]],
    ['extra_pending',[event('requestWillBeSent'),event('responseReceived',{hasExtraInfo:true}),event('loadingFinished')]],
    ['unexpected_extra',[event('requestWillBeSent'),event('responseReceived',{hasExtraInfo:false}),event('responseReceivedExtraInfo'),event('loadingFinished')]],
    ['network_failed',[event('requestWillBeSent'),event('loadingFailed')]],
    ['metadata_shape',[{method:'Network.responseReceived',params:{requestId:17}}]],
  ];
  for(const [condition,events]of cases){
    const traffic=new CaptureTrafficDiagnostics();for(const raw of events)traffic.observe(raw);
    const report=traffic.finish();assert.equal(report.coverage,'partial');assert.ok(report.conditions.includes(condition),condition);
    assert.equal(JSON.stringify(report).includes('PRIVATE'),false);assert.equal(Object.hasOwn(report,'canAuthorize'),false);
    assert.equal(traffic.finish(),report);assert.ok(Object.isFrozen(report.conditions));
  }
});
test('known ExtraInfo can precede response; completed correlation is diagnostic only',()=>{
  const traffic=new CaptureTrafficDiagnostics();
  for(const raw of [event('requestWillBeSent'),event('responseReceivedExtraInfo'),event('responseReceived',{hasExtraInfo:true}),event('loadingFinished')])traffic.observe(raw);
  assert.deepEqual(traffic.finish(),{schema:'aku.capture-traffic.v1',coverage:'complete',conditions:[],records:1,dropped:0});
});
test('record and event saturation only mark diagnostics partial without retaining raw data',()=>{
  const traffic=new CaptureTrafficDiagnostics();
  for(let i=0;i<17000;i++)traffic.observe(event('requestWillBeSent',{requestId:'PRIVATE_'+i}));
  const report=traffic.finish();assert.equal(report.records,256);assert.ok(report.conditions.includes('record_capacity'));assert.ok(report.conditions.includes('event_capacity'));
  assert.ok(report.dropped>0);assert.equal(JSON.stringify(report).includes('PRIVATE'),false);
});

import test from 'node:test';
import assert from 'node:assert/strict';
import {summarizeNativeVisibility} from './headless-native-observer.mjs';
const sample=(trigger,windows=[],other={})=>({trigger,at:'2026-10-02T00:00:00Z',status:'available',windowsTruncated:false,windows,...other});
test('attributes visible windows and activation only to the exact worker Chrome PID',()=>{
  const result=summarizeNativeVisibility({closed:true,exitCode:0,records:[sample('trace_start',[{pid:22,visible:true}]),
    sample('foreground_event',[{pid:11,visible:true}],{eventPid:11}),sample('trace_end')]},11);
  assert.equal(result.status,'bounded_observation_complete');
  assert.equal(result.visibleRootSamples,1);
  assert.equal(result.rootForegroundEvents,1);
  assert.equal(result.zeroBlinkingGuaranteed,false);
});
test('missing end, unknown PID, truncated or degraded observation cannot claim complete coverage',()=>{
  for(const [records,pid] of [[[],11],[[sample('trace_start'),sample('trace_end')],null],
    [[sample('trace_start',[],{windowsTruncated:true}),sample('trace_end')],11],
    [[sample('trace_start'),sample('trace_end',[],{status:'foreground_hook_unavailable_polling'})],11]]){
    assert.equal(summarizeNativeVisibility({closed:true,exitCode:0,records},pid).status,'partial');
  }
});
test('Quiet separates minimized visible host from an exposed root window',()=>{
  const result=summarizeNativeVisibility({closed:true,exitCode:0,records:[
    sample('trace_start',[{pid:11,visible:true,minimized:true}]),
    sample('state_poll',[{pid:11,visible:true,minimized:false}]),sample('trace_end')]},11,'quiet');
  assert.equal(result.scope,'passive_windows_quiet_probe_lifetime');
  assert.equal(result.visibleRootSamples,2);
  assert.equal(result.visibleNonMinimizedRootSamples,1);
  assert.match(result.limitations.at(-1),/Settings, Bridge and interactive/);
});

test('exact root observation stays separately qualified when the global inventory truncates',()=>{
  const root=trigger=>({trigger,pid:11,at:'2026-10-02T00:00:00Z',status:'available',scanComplete:true,
    visibleRootCount:1,visibleNonMinimizedRootCount:0,rootForeground:false});
  const trace={closed:true,exitCode:0,records:[sample('trace_start',[],{windowsTruncated:true}),sample('trace_end')],
    rootRecords:[root('root_start'),root('root_end')]};
  const result=summarizeNativeVisibility(trace,11,'quiet');
  assert.equal(result.status,'partial');
  assert.equal(result.afterInitRootVisibility.status,'bounded_observation_complete');
  assert.equal(result.afterInitRootVisibility.visibleNonMinimizedRootSamples,0);
  assert.equal(summarizeNativeVisibility(trace,12).afterInitRootVisibility.visibleNonMinimizedRootSamples,null);
  trace.rootRecords[1].scanComplete=false;
  assert.equal(summarizeNativeVisibility(trace,11).afterInitRootVisibility.status,'partial');
});

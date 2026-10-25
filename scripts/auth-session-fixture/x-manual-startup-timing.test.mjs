import test from 'node:test';
import assert from 'node:assert/strict';
import {createStartupTiming} from './x-manual-startup-timing.mjs';
import {startupFailure} from './x-manual-startup-diagnostic.mjs';

test('role-local progress reports only closed operations and monotonic durations',()=>{
  let now=100;const reader=[],collector=[];
  const r=createStartupTiming(v=>reader.push(v),()=>now),c=createStartupTiming(v=>collector.push(v),()=>now);
  r.step('identity');c.step('rendezvous');now+=37;
  r.step('settings_wait');c.finish();now+=80;r.failure(startupFailure('reader','settings','source_unavailable'));
  r.failure(startupFailure('pair','joint','deadline'));r.step('cleanup');
  assert.deepEqual(reader.at(-1),{operation:'settings_wait',elapsedMS:80,cause:'manual_initial_reader_settings_source_unavailable_primary'});
  assert.deepEqual(collector.at(-1),{operation:'rendezvous',elapsedMS:37,cause:''});
  assert.equal(reader.length,3);
});
test('untrusted strings, clock faults and excess events cannot leak or affect capture',()=>{
  let now=0;const output=[];const t=createStartupTiming(v=>output.push(v),()=>now);
  t.step('PRIVATE_TOKEN');assert.equal(output.length,0);
  t.step('identity');now=-1;t.finish();assert.equal(output.length,1);
  const many=createStartupTiming(v=>output.push(v),()=>0);
  for(let i=0;i<100;i++)many.step('cleanup');
  assert.equal(output.length,33);
  const errors=[];createStartupTiming(v=>errors.push(v),()=>0).failure(new Error('PRIVATE_TOKEN'));
  assert.equal(JSON.stringify(errors).includes('PRIVATE'),false);
  assert.doesNotThrow(()=>createStartupTiming(()=>{throw Error('sink failed')}).step('identity'));
});

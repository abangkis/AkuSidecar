import test from 'node:test';
import assert from 'node:assert/strict';
import {createStartupTiming} from './x-manual-startup-timing.mjs';
import {startupFailure} from './x-manual-startup-diagnostic.mjs';
import {SETTINGS_WAIT_STATES} from './x-settings-wait-state.mjs';

test('settings prerequisites use closed categories without resetting phase age',()=>{
  let now=100;const output=[],t=createStartupTiming(value=>output.push(value),()=>now);
  t.step('settings_wait','response_pending');now+=20;t.settings('completion_pending');now+=30;t.settings('extra_pending');
  assert.equal(output.at(-1).elapsedMS,50);assert.equal(output.at(-1).settingsWait,'extra_pending');
  const count=output.length;t.settings('PRIVATE_COOKIE_TOKEN');t.settings({get value(){throw Error('PRIVATE');}});
  assert.equal(output.length,count);
  t.failure(startupFailure('collector','settings','source_unavailable'));t.settings('body_complete');
  assert.equal(output.at(-1).settingsWait,'extra_pending');
  assert.ok(output.every(value=>value.settingsWait===undefined||SETTINGS_WAIT_STATES.includes(value.settingsWait)));
  assert.equal(JSON.stringify(output).includes('PRIVATE'),false);
});

test('role-local progress reports only closed operations and monotonic durations',()=>{
  let now=100;const reader=[],collector=[];
  const r=createStartupTiming(v=>reader.push(v),()=>now),c=createStartupTiming(v=>collector.push(v),()=>now);
  r.step('identity');c.step('rendezvous');now+=37;
  r.step('settings_wait');c.finish();now+=80;r.failure(startupFailure('reader','settings','source_unavailable'));
  r.failure(startupFailure('pair','joint','deadline'));r.step('cleanup');
  assert.deepEqual(reader.at(-1),{operation:'settings_wait',elapsedMS:80,cause:'manual_initial_reader_settings_source_unavailable_primary'});
  assert.deepEqual(reader.slice(0,4),[
    {operation:'acquire_rpc',elapsedMS:0,cause:''},
    {operation:'identity',elapsedMS:0,cause:''},
    {operation:'identity',elapsedMS:37,cause:''},
    {operation:'settings_wait',elapsedMS:0,cause:''},
  ]);
  assert.deepEqual(collector.at(-1),{operation:'rendezvous',elapsedMS:37,cause:''});
  assert.equal(reader.length,5);
});
test('untrusted strings, clock faults and excess events cannot leak or affect capture',()=>{
  let now=0;const output=[];const t=createStartupTiming(v=>output.push(v),()=>now);
  t.step('PRIVATE_TOKEN');assert.equal(output.length,0);
  t.step('identity');now=-1;t.finish();assert.equal(output.length,2);
  const manyOutput=[],many=createStartupTiming(v=>manyOutput.push(v),()=>0);
  for(let i=0;i<100;i++)many.step('cleanup');
  assert.equal(manyOutput.length,32);
  const errors=[];createStartupTiming(v=>errors.push(v),()=>0).failure(new Error('PRIVATE_TOKEN'));
  assert.equal(JSON.stringify(errors).includes('PRIVATE'),false);
  assert.doesNotThrow(()=>createStartupTiming(()=>{throw Error('sink failed')}).step('identity'));
});

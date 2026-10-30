import test from 'node:test';
import assert from 'node:assert/strict';
import {createNativeOperationScope} from './native-source-kernel.mjs';

function fixture() {
  let clock=100,first;
  const fail=(stage,reason)=>first??=Object.assign(Error('bounded_failure'),{stage,reason});
  const scope=createNativeOperationScope({now:()=>clock,admitted:100,deadlineMS:600,fail,
    rejected:(_stage,error)=>first??=error});
  return {scope,set:time=>{clock=time;}};
}
test('operation budget shrinks across calls and rejects exact original deadline',async()=>{
  const {scope,set}=fixture();
  assert.equal(await scope.run('seed',remaining=>remaining),500);
  set(599);assert.equal(await scope.run('capture',remaining=>remaining),1);
  set(600);let called=false;
  await assert.rejects(scope.run('capture',()=>{called=true;}),error=>error.reason==='deadline');
  assert.equal(called,false);
});
test('cancellation before invocation prevents the scheduled operation from starting',async()=>{
  const {scope}=fixture();let called=false;
  const pending=scope.run('capture',()=>{called=true;});scope.cancel();
  await assert.rejects(pending,error=>error.reason==='canceled');
  assert.equal(called,false);
});
test('clock regression cannot restore budget and late completion cannot publish',async()=>{
  const {scope,set}=fixture();let finish,started;
  const ready=new Promise(resolve=>{started=resolve;});
  const pending=scope.run('capture',()=>{started();return new Promise(resolve=>{finish=resolve;});});
  await ready;set(99);finish('late');
  await assert.rejects(pending,error=>error.reason==='clock');
});

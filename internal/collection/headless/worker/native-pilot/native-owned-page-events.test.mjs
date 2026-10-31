import test from 'node:test';
import assert from 'node:assert/strict';
import {createNativeOwnedPage} from './native-source-kernel.mjs';
import {createTemporaryPage} from '../chrome.mjs';

async function fixture(kind,failClose=false){
 const listeners=new Set(),callbacks=[],unsubscribed=[];
 let closed=false,closeCalls=0;
 const subscribe=listener=>{
  assert.ok(listeners.size<32);listeners.add(listener);callbacks.push(listener);
  return()=>{listeners.delete(listener);unsubscribed.push(listener);};
 };
 const close=async()=>{closeCalls++;closed=true;if(failClose)throw Error('synthetic close failure');};
 const send=async method=>{
  if(method==='Target.createTarget')return {targetId:'owned'};
  if(method==='Target.attachToTarget')return {sessionId:'owned-session'};
  if(method==='Target.closeTarget'){await close();return {success:true};}
  if(method==='Target.getTargets')return {targetInfos:closed?[]:[{targetId:'owned'}]};
  return {};
 };
 const page=kind==='native'?await createNativeOwnedPage({call:send,subscribe,reserveTarget(){},reserveSession(){},releaseTarget(){},releaseSession(){}},1000):
  await createTemporaryPage({send,subscribe,browserSessionId:'browser',closeTargetAndWait:close});
 return {page,listeners,callbacks,unsubscribed,get closeCalls(){return closeCalls;}};
}

for(const kind of ['native','collector']){
 test(kind+' page subscriptions filter foreign sessions, detach once, and suppress late callbacks',async()=>{
  const f=await fixture(kind),seen=[];
  const off=f.page.on(event=>seen.push(event.method));
  const callback=f.callbacks[0];
  callback({sessionId:'foreign',method:'foreign'});callback({sessionId:'owned-session',method:'owned'});
  assert.deepEqual(seen,['owned']);off();off();
  callback({sessionId:'owned-session',method:'late'});
  assert.equal(f.listeners.size,0);assert.equal(f.unsubscribed.length,1);assert.deepEqual(seen,['owned']);
  const retained=f.page.on(event=>seen.push(event.method));
  const late=f.callbacks[1];
  await Promise.all([f.page.close(),f.page.close()]);retained();
  late({sessionId:'owned-session',method:'after-close'});
  assert.equal(f.listeners.size,0);assert.equal(f.unsubscribed.length,2);assert.equal(f.closeCalls,1);
  assert.deepEqual(seen,['owned']);assert.throws(()=>f.page.on(()=>{}));
 });
 test(kind+' page close retires all subscriptions even when target cleanup fails',async()=>{
  const f=await fixture(kind,true);
  for(let i=0;i<32;i++)f.page.on(()=>assert.fail('closed page emitted'));
  await assert.rejects(f.page.close());await assert.rejects(f.page.close());
  assert.equal(f.listeners.size,0);assert.equal(f.unsubscribed.length,32);assert.equal(f.closeCalls,1);
  for(const callback of f.callbacks)callback({sessionId:'owned-session',method:'late'});
 });
}

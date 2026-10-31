import test from 'node:test';
import assert from 'node:assert/strict';
import {createTemporaryPage} from '../chrome.mjs';
import {createChromeCommandSender,settleChromeCommandReply} from '../chrome-command-queue.mjs';
import {createOwnedInstagramBootstrapScope} from '../native-pilot/main-bootstrap-corroboration-scope.mjs';
import {createIGTrace} from '../native-pilot/instagram-diagnostic-trace.mjs';

const tick=()=>new Promise(resolve=>setImmediate(resolve));
async function waitFor(predicate){for(let i=0;i<100;i++){if(predicate())return;await tick();}assert.fail('synthetic collector did not settle');}
function fixture(t,retainLateResources=true){
  const pending=new Map(),requests=[],held=[],listeners=new Set(),closed=[];
  let hold;
  const answer=(q,result)=>settleChromeCommandReply(pending,{id:q.id,result});
  const send=createChromeCommandSender({pending,retainLateResources,isClosed:()=>false,write:(wire,callback)=>{
    const q=JSON.parse(wire.slice(0,-1));requests.push(q);callback();
    if(q.method===hold){held.push(q);return;}
    answer(q,q.method==='Target.createTarget'?{targetId:'collector-target'}:q.method==='Target.attachToTarget'?{sessionId:'collector-session'}:{});
  }});
  t.after(()=>{for(const p of pending.values())clearTimeout(p.timer);pending.clear();});
  const page=()=>createTemporaryPage({send,browserSessionId:'browser',timeoutMs:15,retainLateResources,
    subscribe:listener=>{listeners.add(listener);return()=>listeners.delete(listener);},closeTargetAndWait:async id=>{closed.push(id);}});
  return {pending,requests,held,listeners,closed,send,answer,page,set hold(method){hold=method;}};
}

test('ordinary collector timeouts keep their existing forget-late-reply behavior',async t=>{
  const f=fixture(t,false);f.hold='Fetch.takeResponseBodyAsStream';let late=0;
  await assert.rejects(f.send('Fetch.takeResponseBodyAsStream',{requestId:'owned'},'session',10,()=>late++),{code:'capture_timeout'});
  assert.equal(f.pending.size,0);f.answer(f.held[0],{stream:'late'});await tick();assert.equal(late,0);
});

for(const method of ['Target.createTarget','Target.attachToTarget'])test('IG collector '+method+' timeout retains late cleanup for its exact target',async t=>{
  const f=fixture(t);f.hold=method;
  await assert.rejects(f.page(),{code:'temporary_target_cleanup_failed'});assert.equal(f.pending.size,1);
  f.answer(f.held[0],method==='Target.createTarget'?{targetId:'collector-target'}:{sessionId:'collector-session'});await tick();await tick();
  assert.deepEqual(f.closed,['collector-target']);assert.equal(f.pending.size,0);
  assert.equal(f.requests.filter(q=>q.method==='Target.createTarget').length,1);
});

test('IG collector page forwards the real command queue late stream disposer after scope disposal',async t=>{
  const f=fixture(t),page=await f.page(),scope=createOwnedInstagramBootstrapScope(page,createIGTrace());
  await scope.call('Fetch.enable',{patterns:[{urlPattern:'https://www.instagram.com/',resourceType:'Document',requestStage:'Response'}]},'owned',1000);
  for(const listener of f.listeners)listener({sessionId:'collector-session',method:'Fetch.requestPaused',params:{requestId:'collector-request'}});
  f.hold='Fetch.takeResponseBodyAsStream';
  await assert.rejects(scope.call('Fetch.takeResponseBodyAsStream',{requestId:'collector-request'},'owned',10),{code:'capture_timeout'});
  assert.equal(await scope.retire(1000),false);scope.dispose();
  f.answer(f.held[0],{stream:'collector-late-stream'});await waitFor(()=>f.requests.some(q=>q.method==='IO.close'));
  const closes=f.requests.filter(q=>q.method==='IO.close');assert.equal(closes.length,1);
  assert.deepEqual(closes[0].params,{handle:'collector-late-stream'});assert.equal(closes[0].sessionId,'collector-session');
  await page.close();assert.deepEqual(f.closed,['collector-target']);
});

test('late callbacks cannot re-resolve a timed out collector command',async t=>{
  const f=fixture(t);f.hold='Fetch.takeResponseBodyAsStream';let published=false,disposed=false;
  const command=f.send('Fetch.takeResponseBodyAsStream',{requestId:'owned'},'collector-session',10,async()=>{disposed=true;});
  void command.then(()=>{published=true;},()=>{});await assert.rejects(command);
  f.answer(f.held[0],{stream:'late'});await tick();assert.equal(disposed,true);assert.equal(published,false);
});

test('collector page retirement keeps unknown stream cleanup uncertain after a late reply',async t=>{
  const f=fixture(t),page=await f.page(),scope=createOwnedInstagramBootstrapScope(page,createIGTrace());
  await scope.call('Fetch.enable',{patterns:[{urlPattern:'https://www.instagram.com/',resourceType:'Document',requestStage:'Response'}]},'owned',1000);
  for(const listener of f.listeners)listener({sessionId:'collector-session',method:'Fetch.requestPaused',params:{requestId:'collector-request'}});
  f.hold='Fetch.takeResponseBodyAsStream';
  await assert.rejects(scope.call('Fetch.takeResponseBodyAsStream',{requestId:'collector-request'},'owned',10));
  scope.dispose();await assert.rejects(page.close(),{code:'temporary_target_cleanup_failed'});
  await assert.rejects(page.send('Runtime.evaluate',{expression:'PRIVATE'},10));
  f.answer(f.held[0],{stream:'retired-page-stream'});await waitFor(()=>f.requests.some(q=>q.method==='IO.close'));
  assert.deepEqual(f.requests.find(q=>q.method==='IO.close').params,{handle:'retired-page-stream'});
  await assert.rejects(page.close(),{code:'temporary_target_cleanup_failed'});assert.deepEqual(f.closed,['collector-target']);
});

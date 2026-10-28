import test from 'node:test';
import assert from 'node:assert/strict';
import {applicationPostOperation,createApplicationReaderPost} from './x-application-reader-post.mjs';
const request={requestId:'broker_'+'a'.repeat(32),source:'x',url:'https://x.com/example/status/123'};
const marker='http://127.0.0.1:12345/split-reader-intent?id=split_'+'b'.repeat(32);
const prepare={id:1,op:'manual_prepare_post',owner:1,request,marker};
function fixture({processScopedOwner=false}={}){
  const calls=[];let clock=1000,postURL=marker,window=2,document='doc',pendingInventory,pendingWrite,blankFrames=0;
  const command=async(method,params={},session,beforeDispatch=()=>{})=>{
    if(pendingWrite?.method===method){const wait=pendingWrite.wait;pendingWrite=null;await wait;}
    beforeDispatch();
    calls.push({method,params,session});
    if(method==='Target.getTargets'){if(pendingInventory){const wait=pendingInventory;pendingInventory=null;await wait;}return{targetInfos:[{targetId:'bootstrap',type:'page',url:processScopedOwner?'https://example.com/user-work':'https://x.com/home'},...calls.some(call=>call.method==='Target.createTarget')?[{targetId:'post',type:'page',url:postURL}]:[]]};}
    if(method==='Browser.getWindowForTarget')return{windowId:params.targetId==='bootstrap'?1:window};
    if(method==='Target.createTarget'){assert.deepEqual(params,{url:marker,newWindow:true,background:true});return{targetId:'post'};}
    if(method==='Target.attachToTarget')return{sessionId:'post-session'};
    if(method==='Page.getFrameTree')return{frameTree:{frame:{id:'post-frame',loaderId:document,url:blankFrames-->0?'about:blank':postURL}}};
    if(method==='Page.navigate'){postURL=params.url;port.observe({method:'Page.frameNavigated',sessionId:'post-session',params:{frame:{id:'post-frame',url:postURL,loaderId:'post-document'}}});return{frameId:'post-frame'};}
    if(['Page.enable','Target.detachFromTarget'].includes(method))return{};
    throw new Error('UNEXPECTED_RAW_COMMAND');
  };
  const port=createApplicationReaderPost({owner:1,bootstrapTarget:processScopedOwner?undefined:'bootstrap',allowMainOrigin:processScopedOwner,processScopedOwner,command,assertCurrent:()=>{},now:()=>clock,token:()=>'c'.repeat(48)});
  return{port,calls,get clock(){return clock;},set clock(value){clock=value;},set url(value){postURL=value;},set window(value){window=value;},set document(value){document=value;},set inventory(value){pendingInventory=value;},set write(value){pendingWrite=value;},set blank(value){blankFrames=value;}};
}

test('borrowed process owner opens an idle reader without Home and preserves existing user windows',async()=>{
  const f=fixture({processScopedOwner:true}),ready=await f.port.execute(prepare);
  assert.equal(ready.windowId,2);
  await f.port.execute({id:2,op:'manual_complete_post',owner:1,handle:ready.handle,requestId:request.requestId});
  assert(!f.calls.some(call=>call.method==='Target.closeTarget'||call.params?.url==='https://x.com/home'));
  await f.port.close();
  const collision=fixture({processScopedOwner:true});collision.window=1;
  await assert.rejects(collision.port.execute(prepare));
  assert(!collision.calls.some(call=>call.method==='Page.navigate'));await collision.port.close();
  assert.throws(()=>createApplicationReaderPost({owner:1,processScopedOwner:true,command:async()=>{},assertCurrent:()=>{}}));
});
test('post operations reject open shapes, raw commands, noncanonical posts and changed local marker policy',()=>{
  applicationPostOperation(prepare);
  for(const value of [{...prepare,script:'arbitrary'},{...prepare,marker:marker+'&extra=1'},{...prepare,marker:marker.replace('12345','11122')},{...prepare,request:{...request,url:request.url+'?token=SECRET'}},{...prepare,request:{...request,url:'https://x.com/home'}},{id:2,op:'Page.navigate',owner:1},{...prepare,request:{...request,source:'facebook'}}])assert.throws(()=>applicationPostOperation(value));
});
test('fresh marker window binds exact request and navigates once without touching bootstrap or closing windows',async()=>{
  const f=fixture(),ready=await f.port.execute(prepare);
  assert.deepEqual(ready,{handle:'c'.repeat(48),targetId:'post',windowId:2,expiresMS:31000});
  assert.deepEqual(await f.port.execute({id:2,op:'manual_complete_post',owner:1,handle:ready.handle,requestId:request.requestId}),{navigated:true});
  await assert.rejects(f.port.execute({id:3,op:'manual_complete_post',owner:1,handle:ready.handle,requestId:request.requestId}));
  assert.equal(f.calls.filter(call=>call.method==='Page.navigate').length,1);
  assert(f.calls.filter(call=>call.method==='Page.navigate').every(call=>call.session==='post-session'&&call.params.url===request.url));
  assert(!f.calls.some(call=>call.method==='Target.closeTarget'||call.method==='Runtime.evaluate'));
  await f.port.close();
});
test('moved, changed, expired and lifecycle-lost marker cannot be overwritten',async()=>{
  for(const fault of ['window','url','document','deadline','destroyed']){
    const f=fixture(),ready=await f.port.execute(prepare);
    if(fault==='window')f.window=3;if(fault==='url')f.url='https://x.com/other';if(fault==='document')f.document='changed';if(fault==='deadline')f.clock=31000;
    if(fault==='destroyed')f.port.observe({method:'Target.targetDestroyed',params:{targetId:'post'}});
    await assert.rejects(f.port.execute({id:2,op:'manual_complete_post',owner:1,handle:ready.handle,requestId:request.requestId}));
    assert(!f.calls.some(call=>call.method==='Page.navigate'||call.method==='Target.closeTarget'));await f.port.close();
  }
});
test('cancel during pending preparation prevents late window creation and cannot replay request',async()=>{
  const f=fixture();let release;f.inventory=new Promise(accept=>{release=accept;});
  const pending=f.port.execute(prepare);await new Promise(accept=>setImmediate(accept));
  assert.deepEqual(await f.port.execute({id:2,op:'manual_cancel_post',owner:1,handle:null,requestId:request.requestId}),{retired:true});
  release();await assert.rejects(pending);await assert.rejects(f.port.execute({...prepare,id:3}));
  assert(!f.calls.some(call=>call.method==='Target.createTarget'||call.method==='Target.closeTarget'));await f.port.close();
});
test('wrong owner/request and a changed pinned marker origin reject without post navigation',async()=>{
  const f=fixture(),ready=await f.port.execute(prepare);
  await assert.rejects(f.port.execute({id:2,op:'manual_complete_post',owner:2,handle:ready.handle,requestId:request.requestId}));
  await assert.rejects(f.port.execute({id:3,op:'manual_complete_post',owner:1,handle:ready.handle,requestId:'broker_'+'d'.repeat(32)}));
  await assert.rejects(f.port.execute({...prepare,id:4,request:{...request,requestId:'broker_'+'e'.repeat(32)},marker:marker.replace('12345','12346')}));
  assert(!f.calls.some(call=>call.method==='Page.navigate'));await f.port.close();
});
test('cancellation and user navigation during asynchronous scope checks block the final command dispatch',async()=>{
  const f=fixture();let release;f.write={method:'Target.createTarget',wait:new Promise(accept=>{release=accept;})};
  const pending=f.port.execute(prepare);await new Promise(accept=>setImmediate(accept));
  await f.port.execute({id:2,op:'manual_cancel_post',owner:1,handle:null,requestId:request.requestId});
  release();await assert.rejects(pending);assert(!f.calls.some(call=>call.method==='Target.createTarget'));await f.port.close();
  const g=fixture(),ready=await g.port.execute(prepare);let dispatch;g.write={method:'Page.navigate',wait:new Promise(accept=>{dispatch=accept;})};
  const navigation=g.port.execute({id:2,op:'manual_complete_post',owner:1,handle:ready.handle,requestId:request.requestId});await new Promise(accept=>setImmediate(accept));
  g.port.observe({method:'Page.frameNavigated',sessionId:'post-session',params:{frame:{id:'post-frame',url:'https://x.com/user-selected',loaderId:'user-document'}}});
  dispatch();await assert.rejects(navigation);assert(!g.calls.some(call=>call.method==='Page.navigate'||call.method==='Target.closeTarget'));await g.port.close();
});
test('initial marker hydration polls only the untouched blank and never creates or navigates twice',async()=>{
  const f=fixture();f.blank=1;const ready=await f.port.execute(prepare);assert.equal(ready.targetId,'post');
  assert.equal(f.calls.filter(call=>call.method==='Target.createTarget').length,1);assert(!f.calls.some(call=>call.method==='Page.navigate'));await f.port.close();
});

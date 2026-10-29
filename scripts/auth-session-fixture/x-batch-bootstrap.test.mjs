import test from 'node:test';
import assert from 'node:assert/strict';
import {dirname,resolve} from 'node:path';
import {fileURLToPath} from 'node:url';
import {createBatchBootstrap} from './x-batch-bootstrap.mjs';
import {createApplicationDailyOwner} from './x-native-metadata-probe-driver.mjs';

test('borrowed owner initialization and idle close create no Home target',async()=>{
  const calls=[];
  const owner=await createApplicationDailyOwner({chromePath:fileURLToPath(import.meta.url),readerProfile:dirname(fileURLToPath(import.meta.url)),loadAssets:async()=>({}),
    borrowedReader:{pid:process.pid,profileDirectory:'Default',transport:{alive:()=>true,subscribe:()=>()=>{},call:async(...args)=>{calls.push(args);throw new Error('unexpected browser command');}}}});
  const initialized=await owner.initialize();
  assert.equal(initialized.readerPID,process.pid);assert(initialized.owner>0);
  await owner.check();await owner.close();assert.deepEqual(calls,[]);
});

test('batch bootstrap creates fresh exact targets and preserves user targets',async()=>{
  const targets=new Set(['user-post']),reserved=new Set(),calls=[];let next=0;
  const scope=createBatchBootstrap({reserveTarget:id=>reserved.add(id),releaseTarget:id=>reserved.delete(id),call:async(method,params)=>{
    calls.push({method,params});
    if(method==='Target.createTarget'){const targetId=`batch-${++next}`;targets.add(targetId);return{targetId};}
    if(method==='Target.getTargets')return{targetInfos:[...targets].map(targetId=>({targetId}))};
    if(method==='Target.closeTarget'){assert.notEqual(params.targetId,'user-post');targets.delete(params.targetId);return{success:true};}
    throw new Error('unexpected');
  }});
  assert.equal(calls.length,0);
  for(let i=1;i<=3;i++){assert.equal(await scope.begin(),`batch-${i}`);await assert.rejects(scope.begin());await scope.close();assert.deepEqual([...targets],['user-post']);assert.equal(reserved.size,0);}
});

test('unverified bootstrap cleanup blocks another batch and retains reservation',async()=>{
  let closes=0,absent=false,created=false,time=0;const reserved=new Set();
  const scope=createBatchBootstrap({now:()=>time,sleep:async ms=>{time+=ms;},reserveTarget:id=>reserved.add(id),releaseTarget:id=>reserved.delete(id),call:async(method)=>{
    if(method==='Target.createTarget'){created=true;return{targetId:'owned'};}
    if(method==='Target.getTargets')return{targetInfos:!created||absent?[]:[{targetId:'owned'}]};
    if(method==='Target.closeTarget'){closes++;return{success:true};}
  }});
  await scope.begin();await assert.rejects(scope.close());await assert.rejects(scope.begin());assert(reserved.has('owned'));
  absent=true;await scope.close();assert.equal(reserved.size,0);assert.equal(closes,1);
});

test('a reused target reply never grants ownership of a user tab',async()=>{
  const calls=[],reserved=[];
  const scope=createBatchBootstrap({reserveTarget:id=>reserved.push(id),call:async(method)=>{
    calls.push(method);
    if(method==='Target.getTargets')return{targetInfos:[{targetId:'user-post'}]};
    if(method==='Target.createTarget')return{targetId:'user-post'};
    throw new Error('user tab must not be closed');
  }});
  await assert.rejects(scope.begin());
  await assert.rejects(scope.close());
  assert.deepEqual(reserved,[]);
  assert(!calls.includes('Target.closeTarget'));
});

test('actual borrowed begin failure retires its exact bootstrap on owner close without launching Chrome',async()=>{
  const {createApplicationDailyOwner:create}=await import('./x-native-metadata-probe-driver.mjs?failed-batch-test');
  const calls=[],targets=new Set(['user-reader']);
  const transport={alive:()=>true,subscribe:()=>()=>{},call:async(method,params)=>{
    calls.push({method,params});
    if(method==='Target.createTarget'){targets.add('batch-home');return{targetId:'batch-home'};}
    if(method==='Target.attachToTarget')throw new Error('synthetic attachment failure');
    if(method==='Target.getTargets')return{targetInfos:[...targets].map(targetId=>({targetId}))};
    if(method==='Target.closeTarget'){assert.equal(params.targetId,'batch-home');targets.delete(params.targetId);return{success:true};}
    throw new Error('unexpected command');
  }};
  const profile=dirname(fileURLToPath(import.meta.url));
  const owner=await create({chromePath:fileURLToPath(import.meta.url),readerProfile:profile,loadAssets:async()=>({}),
    borrowedReader:{pid:process.pid,profileDirectory:'Default',transport}});
  await owner.initialize();assert.equal(calls.length,0);
  await assert.rejects(owner.begin({generation:1,collectorProfile:resolve(profile,'../../internal')}));
  await assert.rejects(owner.begin({generation:2,collectorProfile:resolve(profile,'../../internal')}));
  await owner.close();assert.deepEqual([...targets],['user-reader']);
  assert.equal(calls.filter(call=>call.method==='Target.createTarget').length,1);
  assert(!calls.some(call=>call.method==='Browser.close'));
});

test('actual daily begin/end composition creates fresh reader and collector sessions and preserves user targets',async()=>{
 const {createApplicationDailyOwner:create}=await import('./x-native-metadata-probe-driver.mjs?composed-batches');
 const calls=[],targets=new Set(['user-reader']);let serial=0,collectorCloses=0,failDispose=false,retiring=null,retirementReads=0;
 const transport={alive:()=>true,subscribe:()=>()=>{},call:async(method,params)=>{
   calls.push(method);
   if(method==='Target.getTargets'){
     if(retiring&&++retirementReads>=2){targets.delete(retiring);retiring=null;}
     return{targetInfos:[...targets].map(targetId=>({targetId}))};
   }
   if(method==='Target.createTarget'){const targetId=`home-${++serial}`;targets.add(targetId);return{targetId};}
   if(method==='Target.attachToTarget')return{sessionId:`session-${params.targetId}`};
   if(method==='Page.enable'||method==='Target.detachFromTarget')return{};
   if(method==='Runtime.evaluate')return{result:{type:'boolean',value:true}};
   if(method==='Browser.getWindowForTarget')return{windowId:1};
   if(method==='Target.closeTarget'){assert.notEqual(params.targetId,'user-reader');retiring=params.targetId;retirementReads=0;return{success:true};}
   throw Error('unexpected');
 }};
 const collectorFactory=()=>({launch:async()=>({pid:process.pid+1}),close:async()=>{collectorCloses++;},send:async(method,params,session,project)=>{
   if(failDispose&&method==='Target.disposeBrowserContext')throw Error('PRIVATE');
   const raw=method==='Target.createBrowserContext'?{browserContextId:'context'}:method==='Target.createTarget'?{targetId:'collector'}:method==='Target.attachToTarget'?{sessionId:'collector-session'}:method==='Target.getBrowserContexts'?{browserContextIds:[]}:{};
   return project(raw);
 }});
 const profile=dirname(fileURLToPath(import.meta.url));
 const owner=await create({chromePath:fileURLToPath(import.meta.url),readerProfile:profile,loadAssets:async()=>({}),collectorFactory,borrowedReader:{pid:process.pid,profileDirectory:'Default',transport}});
 await owner.initialize();const owners=[];
 for(let generation=1;generation<=2;generation++){
   const batch=await owner.begin({generation,collectorProfile:resolve(profile,'../../internal')});owners.push(batch.owner);
   const result=await owner.end({generation});assert.equal(result.closed,true);assert.equal(result.cleanup.collector,'verified');assert.equal(result.cleanup.attachments,'verified');assert.equal(result.cleanup.bootstrap,'verified');assert.deepEqual([...targets],['user-reader']);
 }
 assert.notEqual(owners[0],owners[1]);assert.equal(collectorCloses,2);
 await owner.begin({generation:3,collectorProfile:resolve(profile,'../../internal')});failDispose=true;
 const failed=await owner.end({generation:3});assert.equal(failed.closed,false);assert.equal(failed.cleanup.collector,'uncertain');assert.equal(failed.cleanup.bootstrap,'verified');assert.equal(failed.cleanup.attachments,'verified');assert.deepEqual([...targets],['user-reader']);assert.equal(collectorCloses,3);
 await assert.rejects(owner.close());assert(!calls.includes('Browser.close'));
});

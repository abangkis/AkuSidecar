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
  let closes=0,absent=false,created=false;const reserved=new Set();
  const scope=createBatchBootstrap({reserveTarget:id=>reserved.add(id),releaseTarget:id=>reserved.delete(id),call:async(method)=>{
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

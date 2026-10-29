import test from 'node:test';
import assert from 'node:assert/strict';
import {createBatchBootstrap,batchBootstrapDiagnostic} from './x-batch-bootstrap.mjs';
import {cleanupDailyBatch} from './x-daily-batch-cleanup.mjs';

function fixture(overrides={}){
  const state={time:0,created:false,present:true,closeCount:0,releaseCount:0,disappearAt:100,sleeps:[],budgets:[]};
  const reserved=new Set();
  const scope=createBatchBootstrap({timeoutMS:200,pollMS:50,now:()=>state.time,
    sleep:async ms=>{state.sleeps.push(ms);state.time+=ms;if(state.time>=state.disappearAt)state.present=false;},
    reserveTarget:id=>reserved.add(id),releaseTarget:id=>{assert.equal(id,'owned');assert.equal(state.present,false);state.releaseCount++;reserved.delete(id);},
    call:async(method,params,budget)=>{
      if(budget!==undefined){assert(Number.isInteger(budget)&&budget>0&&budget<=200);state.budgets.push(budget);}
      if(method==='Target.createTarget'){state.created=true;return{targetId:'owned'};}
      if(method==='Target.getTargets'){
        if(state.created&&state.inventory)return state.inventory(state.closeCount>0?'after':'before');
        return{targetInfos:[{targetId:'user-reader'},...(state.created&&state.present?[{targetId:'owned'}]:[])]};
      }
      if(method==='Target.closeTarget'){
        assert.deepEqual(params,{targetId:'owned'});state.closeCount++;
        if(state.close)return state.close();
        return{success:true};
      }
      throw Error('unexpected command');
    },...overrides});
  return{state,scope,reserved};
}

const diagnostic=(stage,condition)=>error=>{
  assert.deepEqual(batchBootstrapDiagnostic(error),{stage,condition});
  assert(!error.message.includes('PRIVATE'));
  return true;
};

test('delayed disappearance succeeds within one deadline and never closes the user reader',async()=>{
  const {state,scope,reserved}=fixture();await scope.begin();
  const closing=scope.close();assert.equal(scope.close(),closing);await assert.rejects(scope.begin());
  await closing;assert.equal(state.time,100);assert.equal(state.closeCount,1);assert.equal(state.releaseCount,1);assert.equal(reserved.size,0);
  assert.deepEqual(state.sleeps,[50,50]);assert(state.budgets.some(ms=>ms<200));
});

test('absence timeout retains ownership and forbids another batch or another close request',async()=>{
  const {state,scope,reserved}=fixture();state.disappearAt=Infinity;await scope.begin();
  await assert.rejects(scope.close(),diagnostic('absence','timeout'));
  assert.equal(state.time,200);assert.equal(state.closeCount,1);assert.equal(state.releaseCount,0);assert(reserved.has('owned'));await assert.rejects(scope.begin());
  state.present=false;await scope.close();assert.equal(state.closeCount,1);assert.equal(state.releaseCount,1);
});

test('already absent owned target is released without issuing close',async()=>{
  const {state,scope}=fixture();await scope.begin();state.present=false;await scope.close();assert.equal(state.closeCount,0);assert.equal(state.releaseCount,1);
});

for(const stage of ['inventory_before','inventory_after'])for(const condition of ['rpc','invalid_reply']){
  test(`${stage} ${condition} has a separate safe diagnostic and retains reservation`,async()=>{
    const {state,scope,reserved}=fixture();await scope.begin();
    state.inventory=where=>{
      if(where===(stage==='inventory_before'?'before':'after')){
        if(condition==='rpc')throw Error('PRIVATE target cookie token');
        return{targetInfos:[{targetId:'duplicate'},{targetId:'duplicate'}]};
      }
      return{targetInfos:[{targetId:'owned'},{targetId:'user-reader'}]};
    };
    await assert.rejects(scope.close(),diagnostic(stage,condition));assert.equal(state.releaseCount,0);assert(reserved.has('owned'));
  });
}

for(const condition of ['rpc','invalid_reply']){
  test(`close ${condition} is separate from absence timeout`,async()=>{
    const {state,scope,reserved}=fixture();await scope.begin();state.close=()=>{if(condition==='rpc')throw Error('PRIVATE');return{success:false};};
    await assert.rejects(scope.close(),diagnostic('close',condition));assert.equal(state.closeCount,1);assert(reserved.has('owned'));
  });
}

test('a slow RPC consumes the original deadline instead of restarting the polling budget',async()=>{
  const {state,scope}=fixture();await scope.begin();state.disappearAt=Infinity;
  state.close=()=>{state.time+=175;return{success:true};};
  await assert.rejects(scope.close(),diagnostic('absence','timeout'));assert.equal(state.time,200);assert.deepEqual(state.sleeps,[25]);
});

test('hung inventory is bounded and its late reply cannot release the target',async()=>{
  const {state,scope,reserved}=fixture({timeoutMS:25,pollMS:5});await scope.begin();
  let complete;state.inventory=()=>new Promise(resolve=>{complete=resolve;});
  await assert.rejects(scope.close(),diagnostic('inventory_before','timeout'));
  complete({targetInfos:[]});await new Promise(resolve=>setImmediate(resolve));assert.equal(state.releaseCount,0);assert(reserved.has('owned'));assert.equal(state.closeCount,0);
});

test('a nonfinite or regressed clock never supplies cleanup proof',async()=>{
  for(const clock of [NaN,-1]){
    const {state,scope,reserved}=fixture();await scope.begin();state.close=()=>{state.time=clock;return{success:true};};
    await assert.rejects(scope.close(),diagnostic('close','clock_invalid'));assert(reserved.has('owned'));assert.equal(state.releaseCount,0);
  }
});

test('hung close is bounded and a late acknowledgment cannot release ownership',async()=>{
  const {state,scope,reserved}=fixture({timeoutMS:25,pollMS:5});await scope.begin();
  let acknowledge;state.close=()=>new Promise(resolve=>{acknowledge=resolve;});
  await assert.rejects(scope.close(),diagnostic('close','timeout'));assert.equal(state.closeCount,1);
  acknowledge({success:true});await new Promise(resolve=>setImmediate(resolve));assert.equal(state.releaseCount,0);assert(reserved.has('owned'));await assert.rejects(scope.begin());
});

test('reservation release failure cannot become verified cleanup',async()=>{
  const {state,scope,reserved}=fixture({releaseTarget:()=>{throw Error('PRIVATE');}});await scope.begin();state.present=false;
  await assert.rejects(scope.close(),diagnostic('release','rpc'));assert(reserved.has('owned'));await assert.rejects(scope.begin());
});

test('bootstrap diagnostic survives the batch receipt separately from independent cleanup outcomes',async()=>{
  const {state,scope}=fixture();state.disappearAt=Infinity;await scope.begin();
  const receipt=await cleanupDailyBatch({collector:async()=>{},attachments:async()=>{},bootstrap:()=>scope.close()});
  assert.equal(receipt.closed,false);assert.equal(receipt.cleanup.collector,'verified');assert.equal(receipt.cleanup.attachments,'verified');assert.equal(receipt.cleanup.bootstrap,'uncertain');
  assert.deepEqual(receipt.bootstrapDiagnostic,{stage:'absence',condition:'timeout'});assert(Object.isFrozen(receipt.bootstrapDiagnostic));
  const unknown=await cleanupDailyBatch({bootstrap:async()=>{throw Object.assign(Error('PRIVATE'),{stage:'PRIVATE',condition:'PRIVATE'});}});
  assert.deepEqual(unknown.bootstrapDiagnostic,{stage:'unknown',condition:'unavailable'});assert(!JSON.stringify(unknown).includes('PRIVATE'));
});

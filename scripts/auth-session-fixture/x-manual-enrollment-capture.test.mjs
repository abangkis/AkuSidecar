import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {ManualEnrollmentCapture} from './x-manual-enrollment-capture.mjs';
import {projectCDPEvent,manualStartupStateReady} from './x-native-metadata-probe-driver.mjs';
import {createRetainedManualSetup,manualSeedDiagnostic} from './x-manual-retained-setup.mjs';
import {MANUAL_TIMING} from './x-manual-timing-policy.mjs';
const ID='1908123456789012345';
function fixture(fault){
  const scope={pid:1,profileId:'offline-profile',targetId:'offline-target',sessionId:'offline-main'};
  const observer='offline-observer',frame='offline-frame';
  const startup={...scope,earlyAttempted:false,qualificationAttempted:false,manualSeeded:false,gate:{observationStarted:false,loginVisible:true},shortFeasibility:{closed:false,contextId:'collector-context',sessionId:'collector-session'}};
  let actor,index=0,phase='instrumentation',normal=null,instrumentation=true,doc='offline-old',evaluations=0,reloads=0,closed=0,clock=0,alive=true;
  const operations=[];
  const context=vm.createContext({location:{origin:'https://x.com',pathname:'/home'}});
  const scripts=[`globalThis.__INITIAL_STATE__={session:{user_id:'${ID}'},entities:{users:{entities:{'${ID}':{id_str:'${ID}'}}}}}`, 'void 0', 'delete globalThis.__INITIAL_STATE__'];
  const emit=(method,params={},sessionId=scope.sessionId)=>actor.observe({method,params,...(sessionId===undefined?{}:{sessionId})});
  const pause=(ordinary=false)=>{
    phase=ordinary?'normal':'instrumentation';
    emit('Debugger.scriptParsed',{scriptId:`offline-script-${index}`,executionContextId:2,executionContextAuxData:{isDefault:true,frameId:frame}});
    if(ordinary&&fault==='foreign-session'){emit('Debugger.paused',{reason:'other',hitBreakpoints:[normal],callFrames:[{callFrameId:'offline-call',location:{scriptId:`offline-script-${index}`,lineNumber:0,columnNumber:0}}]},'foreign-session');actor.invalidate();return;}
    emit('Debugger.paused',{reason:ordinary?'other':'instrumentation',hitBreakpoints:ordinary?[fault==='wrong-hit'?'foreign-breakpoint':normal]:[],callFrames:[{callFrameId:'offline-call',location:{scriptId:`offline-script-${index}`,lineNumber:0,columnNumber:0}}]});
  };
  const execute=()=>{
    if(index<scripts.length)vm.runInContext(scripts[index++],context);
    if(index<scripts.length&&instrumentation)pause();
    else while(index<scripts.length)vm.runInContext(scripts[index++],context);
  };
  const send=async(method,params,sessionId)=>{
    assert.equal(actor.command(method,params,sessionId),true,`fixed command rejected: ${method}`);
    operations.push({method,sessionId});
    if(method==='Target.attachToTarget'){
      if(fault==='attach')throw new Error('PRIVATE_ATTACH');
      return actor.project(method,{sessionId:observer});
    }
    if(method==='Runtime.enable'&&sessionId===observer&&fault==='observer-enable')throw new Error('PRIVATE_ENABLE');
    if(method==='Page.getFrameTree')return actor.project(method,{frameTree:{frame:{id:frame,loaderId:doc,url:'https://x.com/home'}}});
    if(method==='Debugger.setInstrumentationBreakpoint')return actor.project(method,{breakpointId:'offline-instrumentation'});
    if(method==='Page.reload'){
      reloads++;doc='offline-new';
      for(const sid of [scope.sessionId,observer]){
        emit('Page.frameNavigated',{frame:{id:frame,loaderId:doc,url:'https://x.com/home'}},sid);
        if(fault==='observer-clear-order'&&sid===observer)emit('Runtime.executionContextsCleared',{},sid);
        emit('Runtime.executionContextCreated',{context:{id:2,origin:'https://x.com',auxData:{isDefault:true,frameId:frame}}},sid);
      }
      if(fault==='target-loss')emit('Target.targetDestroyed',{targetId:scope.targetId},undefined);
      if(fault==='foreign-context')emit('Runtime.executionContextCreated',{context:{id:3,origin:'https://x.com',auxData:{isDefault:true,frameId:frame}}},observer);
      pause();
    }
    if(method==='Debugger.setBreakpoint'){
      normal=`offline-normal-${index}`;
      return actor.project(method,{breakpointId:normal,actualLocation:{...params.location}});
    }
    if(method==='Debugger.removeBreakpoint'){
      if(params.breakpointId==='offline-instrumentation')instrumentation=false;
      else {if(fault==='normal-cleanup')throw new Error('PRIVATE_REMOVE');normal=null;}
    }
    if(method==='Debugger.resume'){
      emit('Debugger.resumed');
      if(normal&&phase==='instrumentation')pause(true);else execute();
    }
    if(method==='Debugger.evaluateOnCallFrame'){
      evaluations++;
      assert.equal(phase,'normal');assert.equal(params.throwOnSideEffect,true);assert.equal(params.timeout,MANUAL_TIMING.bootstrapEvaluationMS);
      assert.equal(actor.command('Runtime.evaluate',{expression:params.expression},sessionId),false);
      assert.equal(actor.command(method,{...params,throwOnSideEffect:false},sessionId),false);
      assert.equal(actor.command(method,{...params,callFrameId:'foreign-call'},sessionId),false);
      if(fault==='context-during-read')emit('Runtime.executionContextDestroyed',{executionContextId:2},observer);
      const value=JSON.parse(JSON.stringify(vm.runInContext(params.expression,context)));
      return actor.project(method,fault==='evaluation'?{exceptionDetails:{text:'PRIVATE_BODY'}}:{result:{type:'object',value}});
    }
    if(method==='Runtime.disable'&&sessionId===scope.sessionId&&fault==='capture-cleanup')throw new Error('PRIVATE_DISABLE');
    if(method==='Target.detachFromTarget'){
      closed++;
      if(fault==='detach')throw new Error('PRIVATE_DETACH');
      if(fault==='cleanup-context-loss')emit('Runtime.executionContextDestroyed',{executionContextId:2},observer);
    }
    return actor.project(method,{});
  };
  actor=new ManualEnrollmentCapture({scope,send,projectEvent:projectCDPEvent,now:()=>clock,snapshot:async()=>{
    if(fault==='deadline')clock=MANUAL_TIMING.attemptAuthorizationMaxMS;
    return{...scope,alive};
  }});
  return {actor,scope,startup,observer,frame,emit,operations,context,counts:()=>({evaluations,reloads,closed}),loseOwner:()=>{alive=false;}};
}
function seedFixture(fault,event){
  const f=fixture(fault);let reads=0,writes=0;
  const run=createRetainedManualSetup({enabled:true,transferAllowed:true,now:()=>0,
    ownership:async()=>{if(reads)await f.actor.check();return{readerPID:1,readerProfile:'r',collectorPID:2,collectorProfile:'c'};},
    readEnrollment:()=>f.actor.run(),
    readAllowedCookies:async()=>{
      await f.actor.check();reads++;await Promise.resolve();
      if(event==='frame-detach')f.emit('Page.frameDetached',{frameId:f.frame},f.observer);
      if(event==='context-loss')f.emit('Runtime.executionContextDestroyed',{executionContextId:2},f.observer);
      if(event==='target-crash')f.emit('Target.targetCrashed',{targetId:f.scope.targetId},undefined);
      if(event==='owner-loss')f.loseOwner();
      await f.actor.check();return[{},{}];
    },beforeWrite:()=>f.actor.check(),
    writeAllowedCookies:async()=>{f.actor.assertCurrent();writes++;return true;},
    cleanupEnrollment:()=>f.actor.close(),
  });return{...f,run,transfer:()=>({reads,writes})};
}

test('dedicated early enrollment captures at verified normal hits before bootstrap disappears, without consuming startup flags',async()=>{
  const f=seedFixture();
  assert.equal(manualStartupStateReady(f.startup),false);
  const result=await f.run();
  assert.equal(result.accountId,ID);assert.equal(Object.keys(result).length,7);
  assert.deepEqual(f.transfer(),{reads:1,writes:1});assert.equal(f.counts().reloads,1);assert.ok(f.counts().evaluations>=2);assert.equal(f.counts().closed,1);
  assert.equal(vm.runInContext('typeof globalThis.__INITIAL_STATE__',f.context),'undefined');
  assert.equal(f.startup.earlyAttempted,false);assert.equal(f.startup.qualificationAttempted,false);assert.equal(f.startup.gate.observationStarted,false);
  f.startup.manualSeeded=true;assert.equal(manualStartupStateReady(f.startup),true);
  assert.equal(f.actor.command('Page.reload',{ignoreCache:false},f.scope.sessionId),false);
  await assert.rejects(f.run(),error=>manualSeedDiagnostic(error).reason==='replay');
  assert.equal(JSON.stringify(f.actor),'{}');
});
test('observer old-context clear ordering during the one reload does not rebind an already pinned context',async()=>{
  const f=seedFixture('observer-clear-order');await f.run();assert.deepEqual(f.transfer(),{reads:1,writes:1});
});
for(const fault of ['attach','observer-enable','wrong-hit','foreign-session','target-loss','foreign-context','context-during-read','evaluation','normal-cleanup','capture-cleanup','deadline']){
  test(`pre-seed enrollment ${fault} fails closed with no cookie operation`,async()=>{
    const f=seedFixture(fault);
    await assert.rejects(f.run(),error=>{const safe=manualSeedDiagnostic(error);assert.ok(safe);assert.equal(safe.write,'not_attempted');assert.equal(error.message.includes('PRIVATE'),false);return true;});
    assert.deepEqual(f.transfer(),{reads:0,writes:0});assert.ok(f.counts().reloads<=1);
    if(['foreign-session','target-loss','foreign-context'].includes(fault))assert.equal(f.counts().evaluations,0);
    await assert.rejects(f.run());
  });
}
for(const event of ['frame-detach','context-loss','target-crash','owner-loss']){
  test(`observed ${event} during awaited cookie read prevents setCookies`,async()=>{
    const f=seedFixture(undefined,event);
    await assert.rejects(f.run(),error=>{assert.equal(manualSeedDiagnostic(error).write,'not_attempted');return true;});
    assert.deepEqual(f.transfer(),{reads:1,writes:0});
  });
}
for(const fault of ['detach','cleanup-context-loss']){
  test(`observer ${fault} after seed acknowledgement is sticky and cannot publish success`,async()=>{
    const f=seedFixture(fault);
    await assert.rejects(f.run(),error=>{assert.deepEqual(manualSeedDiagnostic(error),{phase:'enrollment_cleanup',reason:'unavailable',write:'acknowledged'});return true;});
    assert.deepEqual(f.transfer(),{reads:1,writes:1});await assert.rejects(f.actor.close());
  });
}
test('foreign and late events cannot create a candidate or broaden exact enrollment commands',async()=>{
  const f=fixture();
  assert.equal(f.actor.observe({method:'Runtime.executionContextCreated',sessionId:'foreign',params:{context:{id:2}}}),false);
  assert.equal(f.actor.command('Runtime.evaluate',{expression:'arbitrary'},f.scope.sessionId),false);
  const id=await f.actor.run();assert.equal(id,ID);
  f.emit('Target.targetDestroyed',{targetId:'foreign-target'},undefined);assert.equal(await f.actor.check(),true);
  assert.equal(f.actor.command('Target.attachToTarget',{targetId:'foreign',flatten:true},undefined),false);
  assert.equal(f.actor.command('Debugger.evaluateOnCallFrame',{callFrameId:'late',expression:'arbitrary'},f.scope.sessionId),false);
  await f.actor.close();
  assert.equal(f.actor.observe({method:'Debugger.paused',sessionId:f.scope.sessionId,params:{}}),false);
  await assert.rejects(f.actor.check());
});

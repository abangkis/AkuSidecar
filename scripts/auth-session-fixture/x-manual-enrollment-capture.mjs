// Dedicated one-use retained-QA enrollment. The private candidate is never a
// receipt, and this state does not consume or reset controller startup gates.
import {performance} from 'node:perf_hooks';
import {BootstrapHandoffCapture} from './x-bootstrap-breakpoint-handoff.mjs';
import {QA_BOUND_BOOTSTRAP_EXPRESSION,projectBoundBootstrap} from './x-qa-bound-observation.mjs';
import {manualSeedFailure,manualSeedDiagnostic} from './x-manual-retained-setup.mjs';
const ref=v=>typeof v==='string'&&v.length>0&&Buffer.byteLength(v)<=512&&!/[\u0000-\u001f\u007f]/u.test(v);
const exact=(v,keys)=>v&&typeof v==='object'&&!Array.isArray(v)&&Object.keys(v).length===keys.length&&Object.keys(v).every(k=>keys.includes(k));
const home=v=>v==='https://x.com/home';
const scopeKeys=['pid','profileId','targetId','sessionId'];
export class ManualEnrollmentCapture {
  #scope;#send;#snapshot;#projectEvent;#now;#started;#last;
  #used=false;#state='idle';#failure=null;#candidate=null;#capture;
  #frame=null;#document=null;#mainContext=null;#observerContext=null;
  #observer=null;#attachAttempted=false;#observerRuntime=false;#closing=false;#closePromise=null;#closeFailed=false;
  #reload=false;#issuingReload=false;#navigations=new Set();#instrumentation=null;
  constructor({scope,send,snapshot,projectEvent,now=()=>performance.now()}={}){
    if(!exact(scope,scopeKeys)||!Number.isSafeInteger(scope.pid)||scope.pid<1||!scopeKeys.slice(1).every(k=>ref(scope[k]))||![send,snapshot,projectEvent,now].every(v=>typeof v==='function'))throw manualSeedFailure('frame_before','rejected');
    this.#scope=Object.freeze({...scope});this.#send=send;this.#snapshot=snapshot;this.#projectEvent=projectEvent;this.#now=now;
    this.#capture=new BootstrapHandoffCapture({boundIdentity:true,
      beforeIdentityRead:async(context)=>{await this.#owner();this.#pinned(context);},
      send:async(method,params,purpose)=>{
        if(method==='Page.reload'){
          if(this.#reload)throw manualSeedFailure('enrollment_evaluate','replay');
          this.#reload=true;this.#issuingReload=true;this.#document=null;this.#mainContext=null;this.#observerContext=null;
        }
        try{
          const result=await this.#send(method,params,this.#scope.sessionId,purpose);
          if(method==='Page.getFrameTree'){
            if(this.#frame!==null&&this.#frame!==result.frameId)this.invalidate();
            this.#frame=result.frameId;return {frameId:result.frameId};
          }
          return result;
        }finally{if(method==='Page.reload')this.#issuingReload=false;}
      },
    });
  }
  toJSON(){return{};}
  toString(){return 'private-manual-enrollment';}
  #tick(){
    const t=this.#now();
    if(!Number.isFinite(this.#started)||!Number.isFinite(this.#last)||!Number.isFinite(t)||t<this.#last){this.#failure??=manualSeedFailure('enrollment_evaluate','clock_regressed');throw this.#failure;}
    this.#last=t;
    if(t-this.#started>=60000){this.#failure??=manualSeedFailure('enrollment_evaluate','deadline');throw this.#failure;}
    if(this.#failure)throw this.#failure;
  }
  async #owner(){
    this.#tick();let actual;
    try{actual=await this.#snapshot();}catch{this.#failure??=manualSeedFailure('owner_before','unavailable');throw this.#failure;}
    if(!actual||actual.alive!==true||scopeKeys.some(k=>actual[k]!==this.#scope[k])){this.#failure??=manualSeedFailure('owner_before','ownership_changed');throw this.#failure;}
    this.#tick();
  }
  #pinned(context=this.#mainContext){
    if(!this.#document||!Number.isSafeInteger(context)||context<1||context!==this.#mainContext||context!==this.#observerContext){this.invalidate();throw this.#failure;}
    this.#tick();
  }
  invalidate(){
    this.#failure??=manualSeedFailure('frame_after','document_changed');this.#candidate=null;
    this.#capture.observe({type:'enrollment_invalidated'});
  }
  command(method,params,sessionId){
    if(!this.#used||this.#state==='closed')return false;
    if(method==='Target.attachToTarget')return !this.#observer&&this.#attachAttempted&&sessionId===undefined&&JSON.stringify(params)===JSON.stringify({targetId:this.#scope.targetId,flatten:true});
    if(method==='Target.detachFromTarget')return this.#closing&&!!this.#observer&&sessionId===undefined&&exact(params,['sessionId'])&&params.sessionId===this.#observer;
    if(sessionId===this.#observer&&this.#observer!==null)return ['Page.enable','Runtime.enable','Runtime.disable','Page.getFrameTree'].includes(method)&&exact(params,[]);
    if(sessionId!==this.#scope.sessionId)return false;
    if(method==='Page.getFrameTree')return exact(params,[]);
    if(method==='Page.reload')return this.#state==='capturing'&&this.#issuingReload&&JSON.stringify(params)==='{"ignoreCache":false}';
    if(['Runtime.enable','Runtime.disable','Debugger.enable','Debugger.disable','Debugger.resume'].includes(method))return this.#state==='capturing'&&exact(params,[]);
    if(method==='Debugger.setInstrumentationBreakpoint')return this.#state==='capturing'&&JSON.stringify(params)==='{"instrumentation":"beforeScriptExecution"}';
    if(method==='Debugger.setBreakpoint')return exact(params,['location'])&&this.#capture.handoff.canInstall(params.location);
    if(method==='Debugger.removeBreakpoint')return exact(params,['breakpointId'])&&(params.breakpointId===this.#instrumentation||this.#capture.handoff.breakpoints.has(params.breakpointId));
    if(method==='Debugger.evaluateOnCallFrame')return this.#state==='capturing'&&this.#capture.canReadCallFrame(params.callFrameId)&&JSON.stringify(params)===JSON.stringify({callFrameId:params.callFrameId,expression:QA_BOUND_BOOTSTRAP_EXPRESSION,returnByValue:true,throwOnSideEffect:true,timeout:1500,silent:true,includeCommandLineAPI:false,generatePreview:false});
    return false;
  }
  project(method,result){
    if(!result||typeof result!=='object'||Array.isArray(result))throw manualSeedFailure('enrollment_evaluate','rejected');
    if(method==='Target.attachToTarget'){
      if(!ref(result.sessionId)||result.sessionId===this.#scope.sessionId||this.#observer!==null)throw manualSeedFailure('frame_before','rejected');
      this.#observer=result.sessionId;return{sessionId:this.#observer};
    }
    if(method==='Page.getFrameTree'){
      const f=result.frameTree?.frame;
      if(!f||f.parentId||!ref(f.id)||!ref(f.loaderId)||!home(f.url))throw manualSeedFailure('frame_before','wrong_page');
      return{frameId:f.id,loaderId:f.loaderId};
    }
    if(method==='Debugger.setInstrumentationBreakpoint'){
      if(!ref(result.breakpointId))throw manualSeedFailure('enrollment_evaluate','rejected');
      this.#instrumentation=result.breakpointId;return{breakpointId:result.breakpointId};
    }
    if(method==='Debugger.setBreakpoint'){
      const location=result.actualLocation,requested=this.#capture.handoff.installing;
      if(!requested||!exact(location,['scriptId','lineNumber','columnNumber'])||location.scriptId!==requested.scriptId||!['lineNumber','columnNumber'].every(k=>Number.isSafeInteger(location[k])&&location[k]>=0&&location[k]<=1000000))throw manualSeedFailure('enrollment_evaluate','rejected');
      this.#capture.handoff.created(result.breakpointId);return{breakpointId:result.breakpointId,actualLocation:{...location}};
    }
    if(method==='Debugger.evaluateOnCallFrame'){
      this.#pinned();
      if(Object.hasOwn(result,'exceptionDetails')||result.result?.type!=='object')throw manualSeedFailure('enrollment_project','rejected');
      const projection=projectBoundBootstrap(result.result.value,id=>{this.#candidate=id;});
      return projection;
    }
    return{acknowledged:true};
  }
  observe(raw){
    if(!this.#used||this.#state==='closed')return false;
    const p=raw?.params??{};
    if(['Target.targetDestroyed','Target.targetCrashed'].includes(raw?.method)&&p.targetId===this.#scope.targetId||raw?.method==='Target.detachedFromTarget'&&(p.sessionId===this.#scope.sessionId||p.sessionId===this.#observer&&!this.#closing)){this.invalidate();return false;}
    const session=raw?.sessionId;
    if(session!==this.#scope.sessionId&&session!==this.#observer)return false;
    if(raw.method==='Page.frameDetached'&&p.frameId===this.#frame){this.invalidate();return false;}
    if(['Inspector.detached','Inspector.targetCrashed'].includes(raw.method)){this.invalidate();return false;}
    if(this.#reload){
      if(raw.method==='Page.frameNavigated'&&!p.frame?.parentId){
        if(p.frame?.id!==this.#frame||!home(p.frame?.url)||!ref(p.frame?.loaderId)||this.#navigations.has(session)||this.#document!==null&&p.frame.loaderId!==this.#document){this.invalidate();return false;}
        this.#navigations.add(session);this.#document=p.frame.loaderId;
      }
      const localContext=session===this.#scope.sessionId?this.#mainContext:this.#observerContext;
      if(raw.method==='Runtime.executionContextsCleared'||raw.method==='Runtime.executionContextDestroyed'&&p.executionContextId===localContext){
        if(localContext!==null){this.invalidate();return false;}
      }
      if(raw.method==='Runtime.executionContextCreated'&&p.context?.auxData?.isDefault&&p.context.auxData.frameId===this.#frame){
        const id=p.context.id;
        if(!this.#document||!Number.isSafeInteger(id)||id<1||p.context.origin!=='https://x.com'){this.invalidate();return false;}
        if(session===this.#scope.sessionId){if(this.#mainContext!==null&&this.#mainContext!==id){this.invalidate();return false;}this.#mainContext=id;}
        else{if(this.#observerContext!==null&&this.#observerContext!==id){this.invalidate();return false;}this.#observerContext=id;}
      }
    }
    if(session===this.#scope.sessionId&&this.#state==='capturing'){
      let event;try{event=this.#projectEvent(raw);}catch{this.invalidate();return false;}
      if(event?.type==='early')this.#capture.observe(event.event,p);
    }
    return !this.#failure;
  }
  async run(){
    if(this.#used)throw manualSeedFailure('preconditions','replay');this.#used=true;this.#state='capturing';
    try{
      this.#started=this.#last=this.#now();
      await this.#owner();
      this.#attachAttempted=true;
      await this.#send('Target.attachToTarget',{targetId:this.#scope.targetId,flatten:true},undefined);
      await this.#send('Page.enable',{},this.#observer);
      this.#observerRuntime=true;await this.#send('Runtime.enable',{},this.#observer);
      const result=await this.#capture.run();
      if(result.cleanupVerified!==true||this.#capture.diagnostics().handoff.normalCleanupVerified!==true)throw manualSeedFailure('enrollment_cleanup','rejected');
      if(result.stopReason!=='captured'||this.#candidate===null){
        const reason=result.stopReason==='deadline'?'deadline':['bootstrap_missing','bootstrap_invalid','wrong_page','inspection_failed'].includes(result.snapshot?.diagnostic)?result.snapshot.diagnostic:'candidate_absent';
        throw this.#failure??manualSeedFailure('enrollment_project',reason);
      }
      this.#state='handoff';await this.check();return this.#candidate;
    }catch(error){this.#candidate=null;throw this.#failure??(manualSeedDiagnostic(error)?error:manualSeedFailure('enrollment_evaluate','unavailable'));}
  }
  async check(){
    if(this.#state!=='handoff'||this.#candidate===null)throw this.#failure??manualSeedFailure('frame_after','rejected');
    await this.#owner();this.#pinned();
    const frame=await this.#send('Page.getFrameTree',{},this.#observer);
    if(frame.frameId!==this.#frame||frame.loaderId!==this.#document)this.invalidate();
    await this.#owner();this.#pinned();return true;
  }
  assertCurrent(){
    if(this.#state!=='handoff'||this.#candidate===null)throw this.#failure??manualSeedFailure('frame_after','rejected');
    this.#pinned();return true;
  }
  close(){
    if(this.#closePromise)return this.#closePromise;
    this.#closePromise=(async()=>{
      const alreadyFailed=this.#failure!==null;
      this.#closing=true;
      if(this.#attachAttempted&&!this.#observer)this.#closeFailed=true;
      if(this.#observerRuntime){try{await this.#send('Runtime.disable',{},this.#observer);}catch{this.#closeFailed=true;}}
      if(this.#observer){try{await this.#send('Target.detachFromTarget',{sessionId:this.#observer},undefined);}catch{this.#closeFailed=true;}}
      if(!alreadyFailed&&this.#failure!==null)this.#closeFailed=true;
      this.#candidate=null;this.#state='closed';
      if(this.#closeFailed)throw manualSeedFailure('enrollment_cleanup','unavailable');
      return true;
    })();return this.#closePromise;
  }
}

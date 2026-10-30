import {performance} from 'node:perf_hooks';
import {instagramSelfBootstrapExpression} from './instagram-self-bootstrap-probe.mjs';
import {bootstrapFailure} from './instagram-bootstrap-failure.mjs';
import {INSTAGRAM_BOOTSTRAP_BUDGET} from './instagram-bootstrap-budget.mjs';
const rejected=()=>new Error('bootstrap_scope_rejected');
export function createMainBootstrapDiagnosticScope(raw,{budgetMS=30000}={}) {
  const until=performance.now()+Math.max(0,budgetMS-3000);
  let session,context,mainFrame,mainLoader,invalidated=false,unsubscribe,closing,attempted=false,pending=0;
  let cleanupStage='none',cleanupCondition='none';
  const detach=async id=>{await raw.call('Target.detachFromTarget',{sessionId:id},'',1000);raw.releaseSession(id);};
  const close=()=>closing??=(async()=>{
    unsubscribe?.();
    if(session)try{await detach(session);}catch{cleanupStage='detach';cleanupCondition='request_failed';throw rejected();}
    if(pending){cleanupStage='attachment';cleanupCondition='attachment_pending';throw rejected();}
  })();
  const call=async(stage,method,params,session,ms,late)=>{
    try{const remaining=Math.floor(until-performance.now());if(remaining<1)throw rejected();return await raw.call(method,params,session,Math.min(ms,remaining),late);}catch{throw bootstrapFailure(stage,'request_failed');}
  };
  return Object.freeze({close,cleanupDetail:()=>({cleanupStage,cleanupCondition}),async attach(){
    if(attempted)throw rejected();attempted=true;
    const targets=await call('target_selection','Target.getTargets',{},'',1000);
    if(!Array.isArray(targets?.targetInfos)||targets.targetInfos.length>4096||targets.targetInfos.some(t=>!t||typeof t.targetId!=='string'||!t.targetId)||new Set(targets.targetInfos.map(t=>t.targetId)).size!==targets.targetInfos.length)throw bootstrapFailure('target_selection','inventory');
    const candidates=targets.targetInfos.filter(t=>t.type==='page'&&t.url==='https://www.instagram.com/'&&!raw.privateTarget(t.targetId));
    if(candidates.length!==1)throw bootstrapFailure('target_selection',candidates.length===0?'missing_home':'ambiguous_home');
    pending++;
    const reply=await call('attachment','Target.attachToTarget',{targetId:candidates[0].targetId,flatten:true},'',1000,async late=>{
      if(typeof late?.sessionId==='string'&&!raw.privateSession(late.sessionId)){
        raw.reserveSession(late.sessionId);pending--;try{await detach(late.sessionId);}catch{}
      }
    });
    pending--;
    if(typeof reply?.sessionId!=='string'||!reply.sessionId||raw.privateSession(reply.sessionId))throw bootstrapFailure('attachment','ownership');
    session=reply.sessionId;raw.reserveSession(session);
    // Pin the main frame before Runtime.enable replays all existing contexts.
    const tree=await call('setup','Page.getFrameTree',{},session,1000),root=tree?.frameTree?.frame;
    if(!root||typeof root.id!=='string'||!root.id||typeof root.loaderId!=='string'||!root.loaderId||root.url!=='https://www.instagram.com/'||root.parentId!==undefined)throw bootstrapFailure('setup','document_changed');
    mainFrame=root.id;mainLoader=root.loaderId;
    unsubscribe=raw.subscribe(event=>{
      if(event.sessionId!==session)return;
      if(event.method==='Runtime.executionContextCreated'){
        const c=event.params?.context;
        if(c?.auxData?.isDefault===true&&c.auxData.frameId===mainFrame&&typeof c.uniqueId==='string'&&c.uniqueId){
          if(context&&context.uniqueId!==c.uniqueId)invalidated=true;
          context={id:c.id,uniqueId:c.uniqueId,frameId:c.auxData.frameId};
        }
      }else if(event.method==='Runtime.executionContextsCleared')invalidated=true;
      else if(event.method==='Runtime.executionContextDestroyed'&&context&&(event.params?.executionContextUniqueId===context.uniqueId||Number.isInteger(context.id)&&event.params?.executionContextId===context.id))invalidated=true;
      else if(event.method==='Page.frameNavigated'&&event.params?.frame?.id===mainFrame&&(event.params.frame.loaderId!==mainLoader||event.params.frame.url!=='https://www.instagram.com/'))invalidated=true;
      else if(event.method==='Page.frameDetached'&&event.params?.frameId===mainFrame)invalidated=true;
    });
    for(const method of ['Page.enable','Runtime.enable'])await call('setup',method,{},session,1000);
    return Object.freeze({
      async send(method,params){
        if(method!=='Page.getFrameTree'||Object.keys(params).length||closing)throw bootstrapFailure('setup','invalid_operation');
        if(invalidated)throw bootstrapFailure('setup','document_changed');
        const reply=await call('setup',method,{},session,1000),frame=reply?.frameTree?.frame;
        if(!frame||frame.id!==mainFrame||frame.loaderId!==mainLoader||frame.url!=='https://www.instagram.com/'||frame.parentId!==undefined)throw bootstrapFailure('setup','document_changed');
        if(!context)throw bootstrapFailure('setup','context_missing');return reply;
      },
      async evaluate(expression){
        if(expression!==instagramSelfBootstrapExpression()||closing)throw bootstrapFailure('evaluation','invalid_operation');
        if(invalidated)throw bootstrapFailure('evaluation','document_changed');
        if(!context)throw bootstrapFailure('evaluation','context_missing');
        const reply=await call('evaluation','Runtime.evaluate',{expression,uniqueContextId:context.uniqueId,returnByValue:true,awaitPromise:true,userGesture:false},session,INSTAGRAM_BOOTSTRAP_BUDGET.evaluationTimeoutMS);
        if(invalidated)throw bootstrapFailure('evaluation','document_changed');
        if(reply.exceptionDetails)throw bootstrapFailure('evaluation','exception');return reply.result?.value;
      },
    });
  }});
}

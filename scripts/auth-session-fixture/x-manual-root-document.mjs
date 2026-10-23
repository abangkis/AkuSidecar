// Root structure only. Cache/source facts never become cookie attempts.
const object = v => v !== null && typeof v === 'object' && !Array.isArray(v);
const invalid = Symbol();
const data = (v, k) => {
  const d = object(v) ? Object.getOwnPropertyDescriptor(v, k) : undefined;
  return d ? Object.hasOwn(d, 'value') ? d.value : invalid : undefined;
};
const ref = v => typeof v === 'string' && v.length > 0 && v.length <= 512;

// Closed projections never invoke accessors or retain browser-owned values.
const cacheProjection = response => Object.fromEntries([
  ['guardDiskCache','fromDiskCache'],['guardServiceWorker','fromServiceWorker'],['guardPrefetchCache','fromPrefetchCache']
].map(([key,field])=>{const value=data(response,field);return [key,value===undefined?'absent':value===true?'true':value===false?'false':'malformed'];}));
const firstCondition = checks => checks.find(([failed])=>failed)?.[1];
// Diagnostic only. Acceptance uses private request-local frame correlation.
function requestFrameProjection(params, root) {
  const d = Object.getOwnPropertyDescriptor(params, 'frameId');
  if (!d) return 'absent';
  if (!Object.hasOwn(d, 'value') || !ref(d.value)) return 'malformed';
  return d.value === root ? 'matching' : 'different';
}

export class RootDocumentValidator {
 #frame;#session;#armed=false;#baseline;#closed=false;#oldLoader;#root=null;#commit=null;#failure=null;#callback;#now;
 constructor(frame,session,{beforeSeed=false,onFailure=()=>{},now=()=>performance.now()}={}){this.#frame=frame;this.#session=session;this.#baseline=!beforeSeed;this.#callback=onFailure;this.#now=now;}
 beginPostSeed(){this.#baseline=true;}
 epoch(){return !this.#baseline?'before_seed':this.#commit!==null?'document_committed':this.#armed?'reload_armed':'post_seed';}
 get armed(){return this.#armed;} get committed(){return this.#commit!==null;}
 armReload(oldLoader){const condition=firstCondition([[this.#closed,'root_arm_closed'],[this.#armed,'root_arm_repeated'],[!this.#baseline,'root_arm_before_seed'],[!ref(oldLoader),'root_arm_loader'],[!ref(this.#session),'root_arm_session']]);if(condition){this.#rootFailure('guard_request_unbound','arm',condition);throw Error('manual_cookie_guard_invalid');}this.#armed=true;this.#oldLoader=oldLoader;}
 #rootFailure(reason,trigger,condition,response=null,frame=this.#root?.frameStatus??'unknown'){if(this.#failure)return;const trace=Object.freeze({guardTrigger:trigger,guardProvenance:'root_scope',guardCondition:condition,guardEpoch:this.epoch(),guardDiskCache:'unknown',guardServiceWorker:'unknown',guardPrefetchCache:'unknown',...(response===null?this.#root?.cache:cacheProjection(response)),guardRequestFrame:frame,guardHeaderReason:'unknown'});this.#failure={reason,trace};this.#callback(reason,trace,'scope_lost');}
 observe(raw,first){if(this.#closed||data(raw,'sessionId')!==this.#session)return;const method=data(raw,'method'),p=data(raw,'params');if(!object(p))return;const id=data(p,'requestId');
    if(method==='Page.frameNavigated'&&this.#armed&&data(data(p,'frame'),'id')===this.#frame){
      const frame=data(p,'frame'),loader=data(frame,'loaderId');
      const condition=firstCondition([[this.#commit!==null,'root_commit_duplicate'],[!ref(loader),'root_commit_loader_invalid'],[loader===this.#oldLoader,'root_commit_old_loader'],[data(frame,'url')!=='https://x.com/home','root_commit_url'],[data(frame,'parentId')!==undefined,'root_commit_parent']]);
      if(condition)this.#rootFailure('guard_response_correlation','document',condition);
      else this.#commit=loader;
      if(this.#root&&this.#root.loaderId!==loader)this.#rootFailure('guard_response_correlation','document','root_commit_loader_mismatch');
    }

if(method==='Network.requestWillBeSent'){const redirect=data(p,'redirectResponse')!==undefined,frameStatus=requestFrameProjection(p,this.#frame);
      const pinned=this.#armed&&data(p,'type')==='Document'&&data(p,'frameId')===this.#frame;
      if(pinned){
        const request=data(p,'request'),loader=data(p,'loaderId');
        const condition=firstCondition([[!!this.#root,'root_request_duplicate'],[!first,redirect?'root_request_redirect':'root_request_history'],[data(request,'method')!=='GET','root_request_method'],[data(request,'url')!=='https://x.com/home','root_request_url'],[!ref(loader),'root_request_loader_invalid'],[loader===this.#oldLoader,'root_request_old_loader'],[!!this.#commit&&this.#commit!==loader,'root_request_commit_mismatch']]);
        if(condition)this.#rootFailure('guard_request_unbound','request',condition,null,frameStatus);
        else this.#root={id,loaderId:loader,frameStatus,response:false,extra:null,needsExtra:null};
      }else if(this.#root&&this.#root.id===id)this.#rootFailure('guard_request_unbound','request','root_request_reused');

}
    const root=this.#root;
    if(root&&id===root.id){
      if(method==='Network.responseReceived'){
        const response=data(p,'response'),extra=data(p,'hasExtraInfo');
        // Retain only closed cache projections for subsequent root failures.
        // The sticky first trace is never enriched from a later response.
        root.cache??=cacheProjection(response);
        if(root.observedAt===undefined){try{const t=this.#now();root.observedAt=Number.isFinite(t)&&t>=0?t:'unknown';}catch{root.observedAt='unknown';}}
        const frame=requestFrameProjection(p,this.#frame);
        const condition=firstCondition([[root.response,'root_response_duplicate'],[data(p,'frameId')!==this.#frame,`root_response_frame_${frame}`],[data(p,'loaderId')!==root.loaderId,'root_response_loader'],[data(response,'url')!=='https://x.com/home','root_response_url'],[data(response,'status')!==200,'root_response_status'],[typeof extra!=='boolean','root_response_extra_flag']]);
        if(condition)this.#rootFailure('guard_response_correlation','response',condition,response);
        root.response=true;root.needsExtra=extra;
      }
      if(method==='Network.responseReceivedExtraInfo'){
        const condition=firstCondition([[root.extra!==null,'root_extra_duplicate'],[data(p,'statusCode')!==200,'root_extra_status']]);
        if(condition)this.#rootFailure('guard_response_correlation','extra',condition);
        root.extra=data(p,'statusCode');
      }
      if(method==='Network.loadingFailed')this.#rootFailure('guard_response_correlation','response','root_loading_failed');
      if(method==='Network.requestServedFromCache')root.cacheEvent=true;
      if(root.response&&root.extra!==null&&root.needsExtra!==true)this.#rootFailure('guard_response_correlation','extra','root_extra_unexpected');
    }

 }
 evidence(){const r=this.#root;return Object.freeze({armed:this.#armed,documentId:r?.loaderId??null,structurallyValid:!this.#failure,responseObserved:!!r?.response,cacheEvent:r?.cacheEvent===true,complete:!this.#armed||!!r&&r.response&&this.#commit===r.loaderId&&!(r.needsExtra===true&&r.extra===null),observedAt:r?.observedAt??'unknown',cache:r?.cache?Object.freeze({...r.cache}):null,frameStatus:r?.frameStatus??'unknown',epoch:this.epoch()});}
 finish(){if(this.#closed)throw Error('manual_cookie_guard_replayed');this.#closed=true;const root=this.#root;if(this.#armed&&!root)this.#rootFailure('guard_completion_missing','finish','root_missing');if(root){const condition=firstCondition([[!root.response,'root_response_missing'],[this.#commit!==root.loaderId,'root_commit_missing'],[root.needsExtra===true&&root.extra===null,'root_extra_missing']]);if(condition)this.#rootFailure('guard_completion_missing','finish',condition);}return this.evidence();}
}

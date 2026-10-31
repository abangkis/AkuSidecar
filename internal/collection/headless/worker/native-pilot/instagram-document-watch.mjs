// Private, value-free checks shared by acquisition and the final verifier.
export function instagramDocumentEventCondition(event,binding){
 const p=event.params??{};
 if(event.method==='Runtime.executionContextsCleared')return 'document_context_cleared';
 if(event.method==='Runtime.executionContextDestroyed'&&(p.executionContextUniqueId===binding.uniqueContextId||p.executionContextId===binding.contextId))return 'document_context_destroyed';
 if(event.method==='Runtime.executionContextCreated'&&p.context?.auxData?.isDefault===true&&p.context.auxData.frameId===binding.id&&p.context.uniqueId!==binding.uniqueContextId)return 'document_context_replaced';
 if(event.method==='Page.frameNavigated'&&(p.frame?.id??p.frameId)===binding.id)return 'document_frame_navigated';
 if(event.method==='Page.frameDetached'&&p.frameId===binding.id)return 'document_frame_detached';
 if(event.method==='Page.navigatedWithinDocument'&&p.frameId===binding.id)return 'document_within_navigation';
 return null;
}

export function instagramDocumentFrameCondition(frame,binding){
 if(!frame||typeof frame.id!=='string'||!frame.id||frame.parentId!==undefined)return 'document_frame_invalid';
 if(frame.id!==binding.id)return 'document_frame_mismatch';
 if(frame.loaderId!==binding.loaderId)return 'document_loader_mismatch';
 if(frame.url!==binding.url)return 'document_url_mismatch';
 return null;
}

export function createInstagramDocumentWatch(page,onInvalidation=()=>{}){
 let off,condition=null,bound=false,disposed=false;
 return Object.freeze({
  bind(binding){
   if(bound||disposed)throw new Error('instagram_document_binding_unavailable');
   bound=true;const pinned=Object.freeze({...binding});
   // Installed synchronously while acquisition's observer is still attached.
   off=page.on(event=>{
    if(condition)return;
    // An unreadable event makes document observation uncertain; it must not
    // escape into the shared transport or authorize a proof.
    try{condition=instagramDocumentEventCondition(event,pinned);}catch{condition='unavailable';}
    if(condition)try{onInvalidation(condition);}catch{}
   });
  },
  condition:()=>condition,
  dispose(){disposed=true;off?.();off=undefined;},
 });
}

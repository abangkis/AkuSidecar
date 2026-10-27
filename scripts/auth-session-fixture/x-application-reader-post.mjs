// Fixed explicit-post recipe. No bootstrap navigation, evaluation, raw CDP or
// automatic window close is exposed by this private application port.
import {randomBytes} from 'node:crypto';
export const POST_PREPARATION_MS=30000;
const fail=()=>new Error('application_post_unavailable');
const ref=value=>typeof value==='string'&&value.length>0&&value.length<=512&&!/[\u0000-\u001f\u007f]/u.test(value);
function exact(value,keys){
  if(!value||![Object.prototype,null].includes(Object.getPrototypeOf(value)))throw fail();
  const d=Object.getOwnPropertyDescriptors(value),names=Reflect.ownKeys(d);
  if(names.length!==keys.length||names.some(k=>!keys.includes(k)||!Object.hasOwn(d[k],'value')||!d[k].enumerable))throw fail();
}
export function applicationPostOperation(message,{allowMainOrigin=false}={}){
  const op=message&&Object.getOwnPropertyDescriptor(message,'op')?.value;
  if(!['manual_prepare_post','manual_complete_post','manual_cancel_post'].includes(op))throw fail();
  exact(message,op==='manual_prepare_post'?['id','op','owner','request','marker']:['id','op','owner','handle','requestId']);
  if(!Number.isSafeInteger(message.id)||message.id<1||!Number.isSafeInteger(message.owner)||message.owner<1)throw fail();
  if(op==='manual_prepare_post'){
    exact(message.request,['requestId','source','url']);
    if(!/^broker_[a-f0-9]{32}$/u.test(message.request.requestId)||message.request.source!=='x'||typeof message.request.url!=='string'||!/^https:\/\/x\.com\/[A-Za-z0-9_]{1,15}\/status\/[0-9]{1,20}$/u.test(message.request.url))throw fail();
    if(typeof message.marker!=='string')throw fail();
    let marker;try{marker=new URL(message.marker);}catch{throw fail();}
    const action=marker.searchParams.get('id');
    const fixed=/^http:\/\/127\.0\.0\.1:([1-9][0-9]{0,4})\/split-reader-intent\?id=split_[a-f0-9]{32}$/u.exec(message.marker);
    if(!fixed||Number(fixed[1])>65535||!allowMainOrigin&&fixed[1]==='11122'||marker.protocol!=='http:'||marker.hostname!=='127.0.0.1'||marker.username||marker.password||marker.hash||marker.pathname!=='/split-reader-intent'||!/^split_[a-f0-9]{32}$/u.test(action??''))throw fail();
  }else{
    if(!(message.op==='manual_cancel_post'&&message.handle===null)&&!/^[a-f0-9]{48}$/u.test(message.handle??''))throw fail();
    if(!/^broker_[a-f0-9]{32}$/u.test(message.requestId??''))throw fail();
  }
  return message;
}
export function createApplicationReaderPost({owner,bootstrapTarget,command,assertCurrent,now=()=>Date.now(),token=()=>randomBytes(24).toString('hex'),allowMainOrigin=false}={}){
  if(!Number.isSafeInteger(owner)||owner<1||!ref(bootstrapTarget)||![command,assertCurrent,now,token].every(v=>typeof v==='function'))throw fail();
  const requests=new Set(),held=new Map(),byRequest=new Map();let origin;
  const inventory=async()=>{
    const reply=await command('Target.getTargets',{});
    if(!Array.isArray(reply?.targetInfos)||reply.targetInfos.length>128)throw fail();
    const ids=new Set();for(const row of reply.targetInfos){if(!ref(row?.targetId)||ids.has(row.targetId))throw fail();ids.add(row.targetId);}
    const rows=reply.targetInfos.filter(row=>row?.type==='page');
    if(!rows.some(row=>row.targetId===bootstrapTarget&&row.url==='https://x.com/home'))throw fail();
    return rows;
  };
  const windowOf=async target=>{const reply=await command('Browser.getWindowForTarget',{targetId:target});if(!Number.isSafeInteger(reply?.windowId)||reply.windowId<1)throw fail();return reply.windowId;};
  const check=async state=>{
    assertCurrent();if(state.invalid||now()>=state.expires||!ref(state.session))throw fail();
    const rows=await inventory();
    if(!rows.some(row=>row.targetId===state.target&&row.url===state.marker)||await windowOf(state.target)!==state.window)throw fail();
    const frame=(await command('Page.getFrameTree',{},state.session))?.frameTree?.frame;
    if(frame?.id!==state.frame||frame?.loaderId!==state.document||frame?.url!==state.marker)throw fail();
    assertCurrent();if(state.invalid||now()>=state.expires)throw fail();
  };
  const detach=async state=>{
    if(!state.session)return;const session=state.session;state.session=null;
    await command('Target.detachFromTarget',{sessionId:session});
  };
  return Object.freeze({
    observe(raw){
      for(const state of held.values()){
        if(raw.method==='Target.targetDestroyed'&&raw.params?.targetId===state.target||raw.method==='Target.detachedFromTarget'&&raw.params?.sessionId===state.session)state.invalid=true;
        if(raw.sessionId===state.session){
          if(['Inspector.detached','Inspector.targetCrashed'].includes(raw.method)||raw.method==='Page.frameDetached'&&raw.params?.frameId===state.frame)state.invalid=true;
          if(state.prepared&&raw.method==='Page.frameNavigated'&&!raw.params?.frame?.parentId){
            const frame=raw.params?.frame;
            if(!state.navigationDispatched||state.navigationObserved||frame?.id!==state.frame||frame?.url!==state.url)state.invalid=true;
            else state.navigationObserved=true;
          }
        }
      }
    },
    async execute(raw){
      const message=applicationPostOperation(raw,{allowMainOrigin});if(message.owner!==owner)throw fail();
      if(message.op==='manual_prepare_post'){
        if(requests.has(message.request.requestId)||requests.size>=32)throw fail();requests.add(message.request.requestId);
        assertCurrent();const selectedOrigin=new URL(message.marker).origin;
        if(origin&&origin!==selectedOrigin)throw fail();origin=selectedOrigin;
        const handle=token();if(!/^[a-f0-9]{48}$/u.test(handle)||held.has(handle))throw fail();
        const state={requestId:message.request.requestId,url:message.request.url,marker:message.marker,expires:now()+POST_PREPARATION_MS,invalid:false,prepared:false};held.set(handle,state);byRequest.set(state.requestId,state);
        const live=()=>{assertCurrent();if(state.invalid||state.used||now()>=state.expires)throw fail();};
        state.timer=setTimeout(()=>{state.invalid=true;void detach(state).catch(()=>{});},POST_PREPARATION_MS);state.timer.unref?.();
        try{
          live();await inventory();live();const bootstrapWindow=await windowOf(bootstrapTarget);live();
          const created=await command('Target.createTarget',{url:state.marker,newWindow:true,background:true},undefined,live);
          if(!ref(created?.targetId)||created.targetId===bootstrapTarget)throw fail();state.target=created.targetId;
          live();state.window=await windowOf(state.target);live();if(state.window===bootstrapWindow)throw fail();
          state.session=(await command('Target.attachToTarget',{targetId:state.target,flatten:true},undefined,live))?.sessionId;
          live();if(!ref(state.session))throw fail();
          await command('Page.enable',{},state.session,live);live();
          let frame;
          for(;;){
            frame=(await command('Page.getFrameTree',{},state.session))?.frameTree?.frame;live();
            if(frame?.url===state.marker)break;
            // Target creation may precede its one fixed marker navigation.
            // Poll only that initial blank; never repair another user URL.
            if(frame?.url!=='about:blank')throw fail();
            await new Promise(accept=>setTimeout(accept,25));live();
          }
          if(!ref(frame?.id)||!ref(frame?.loaderId)||frame.url!==state.marker)throw fail();
          state.frame=frame.id;state.document=frame.loaderId;state.prepared=true;await check(state);
          return{handle,targetId:state.target,windowId:state.window,expiresMS:state.expires};
        }catch{state.invalid=true;clearTimeout(state.timer);await detach(state).catch(()=>{});throw fail();}
      }
      const state=message.op==='manual_cancel_post'&&message.handle===null?byRequest.get(message.requestId):held.get(message.handle);
      if(!state||state.requestId!==message.requestId||state.used)throw fail();state.used=true;
      clearTimeout(state.timer);
      if(message.op==='manual_cancel_post'){state.invalid=true;await detach(state);return{retired:true};}
      try{
        await check(state);
        // Recheck the exact marker immediately before the sole navigation.
        await check(state);
        const reply=await command('Page.navigate',{url:state.url},state.session,()=>{
          assertCurrent();if(state.invalid||now()>=state.expires)throw fail();state.navigationDispatched=true;
        });
        if(Object.hasOwn(reply??{},'errorText')||reply?.frameId!==state.frame)throw fail();
        assertCurrent();if(state.invalid||now()>=state.expires)throw fail();return{navigated:true};
      }catch{state.invalid=true;throw fail();}
      finally{try{await detach(state);}catch{state.invalid=true;throw fail();}}
    },
    async close(){for(const state of held.values()){state.invalid=true;clearTimeout(state.timer);await detach(state).catch(()=>{});}},
  });
}

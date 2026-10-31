import {performance} from 'node:perf_hooks';
import {createIGTrace,igFailureDetail} from './instagram-diagnostic-trace.mjs';
import {readInstagramBootstrapBodyStream} from './instagram-bootstrap-body-stream.mjs';
import {INSTAGRAM_CORROBORATION_POLICY as policy,INSTAGRAM_CORROBORATION_HOME as home} from './instagram-bootstrap-corroboration-policy.mjs';
import {createOwnedInstagramBootstrapScope} from './main-bootstrap-corroboration-scope.mjs';
export const INSTAGRAM_CORROBORATION_SCHEMA='aku.instagram-bootstrap-corroboration.v1';
export const INSTAGRAM_CORROBORATION_REASONS=Object.freeze(['corroborated','invalid_request','scope_unavailable','setup_failed','navigation_failed','wire_unmatched','http_rejected','delivery_unproven','body_bounds','body_read','body_encoding','body_deadline','body_invalid','context_unavailable','document_changed','stale_delivery','evaluation_failed','projection_invalid','page_scope','login_visible','challenge','bootstrap_rejected','id_mismatch','cookie_unsupported','cookie_missing','cookie_ambiguous','cookie_invalid','cookie_mismatch','cookie_failed','cleanup_failed']);
const booleans=['bodyBound','documentBound','serverFresh','idMatched','cookieMatched','scanComplete'];
const pageReasons=['corroborated','bootstrap_rejected','id_mismatch','cookie_unsupported','cookie_missing','cookie_ambiguous','cookie_invalid','cookie_mismatch','cookie_failed','page_scope','login_visible','challenge','body_encoding','body_invalid'];
const keys=['schema','reason',...booleans,'qualified','cleanup'];
const record=(v,fields)=>v&&[Object.prototype,null].includes(Object.getPrototypeOf(v))&&Reflect.ownKeys(v).length===fields.length&&fields.every(k=>Object.hasOwn(Object.getOwnPropertyDescriptor(v,k)??{},'value'));
const reject=()=>new Error('instagram_corroboration_rejected');
const reference=v=>typeof v==='string'&&v.length>0&&v.length<=512;
const pause=()=>new Promise(resolve=>setTimeout(resolve,10));
export function validateInstagramBootstrapCorroboration(value){
 if(!record(value,keys)||value.schema!==INSTAGRAM_CORROBORATION_SCHEMA||!INSTAGRAM_CORROBORATION_REASONS.includes(value.reason)||
  booleans.some(k=>typeof value[k]!=='boolean')||value.qualified!==false||!['not_started','verified','uncertain'].includes(value.cleanup)||
  value.reason==='corroborated'&&(!booleans.every(k=>value[k])||value.cleanup!=='verified')||Buffer.byteLength(JSON.stringify(value))>1024)throw reject();
 return Object.freeze(Object.fromEntries(keys.map(k=>[k,value[k]])));
}

// A separate diagnostic lane only: no native proof, Store call or transfer.
export async function runInstagramBootstrapCorroboration(transport,request,options={}){
 return (await acquireInstagramBootstrap(transport,request,options)).diagnostic;
}

export async function acquireOwnedInstagramIdentity(page,nonce,options={}){
 return acquireInstagramBootstrap({createBootstrapCorroborationScope:trace=>createOwnedInstagramBootstrapScope(page,trace)},{nonce},options);
}

async function acquireInstagramBootstrap(transport,request,{trace=createIGTrace(),budgetMS=policy.totalMS,now=()=>Math.floor(performance.now())}={}){
 let accountId='',observedAtMS,documentBinding;
 const result={schema:INSTAGRAM_CORROBORATION_SCHEMA,reason:'setup_failed',...Object.fromEntries(booleans.map(k=>[k,false])),qualified:false,cleanup:'not_started'};
 if(!record(request,['nonce'])||typeof request.nonce!=='string'||!/^[a-f0-9]{48}$/.test(request.nonce)){result.reason='invalid_request';return {diagnostic:validateInstagramBootstrapCorroboration(result)};}
 let scope,target,session,off,body,attempted=false,phase='setup_failed',firstCause;
 let rootFrame,root,paused,extra,response,finished=false,cached=false,invalidated=false,pinned,context,streamCleanup=true;
 let requestCount=0,pauseCount=0,extraCount=0,responseCount=0,navigationSettled=false,navigation;
 const until=now()+Math.max(0,Math.min(policy.acquisitionMS,budgetMS-policy.cleanupMS-3000));
 const active=stage=>trace.note(stage);active('scope');
 const mark=(reason,condition=reason,stage=trace.snapshot().stage)=>{firstCause??=reason;trace.fail(stage,condition);};
 const fail=(reason,condition=reason,stage=trace.snapshot().stage)=>{mark(reason,condition,stage);throw reject();};
 const remaining=()=>{const ms=until-now();if(ms<1)fail(phase);return Math.min(ms,1000);};
 const send=(method,params={},ms=remaining())=>scope.call(method,params,session??'',ms);
 const frame=async()=>{const f=(await send('Page.getFrameTree'))?.frameTree?.frame;if(!f||!reference(f.id)||f.parentId!==undefined)fail('document_changed');return f;};
 const wait=async predicate=>{while(!predicate()){remaining();if(firstCause)throw reject();await pause();}};
 try{
  if(typeof transport?.createBootstrapCorroborationScope!=='function')fail('scope_unavailable');
  scope=transport.createBootstrapCorroborationScope(trace);trace.done('scope');
  off=scope.on(event=>{
   if(!session||event.sessionId!==session)return;
   const p=event.params??{};
   try{
    if(event.method==='Network.requestWillBeSent'&&p.frameId===rootFrame&&p.type==='Document'){
     requestCount++;
     const condition=requestCount!==1?'root_request_duplicate':p.request?.url!==home?'root_url':p.request?.method!=='GET'?'root_method':p.redirectResponse?'root_redirect':!reference(p.requestId)?'root_request_id':!reference(p.loaderId)?'root_loader_id':null;
     if(condition){mark('wire_unmatched',condition,'root_delivery');return;}
     root={requestId:p.requestId,loaderId:p.loaderId};
    }else if(event.method==='Fetch.requestPaused'){
     pauseCount++;if(pauseCount!==1){mark('wire_unmatched','pause_duplicate','root_delivery');return;}
     paused={requestId:p.requestId,networkId:p.networkId,frameId:p.frameId,resourceType:p.resourceType,request:{url:p.request?.url,method:p.request?.method},responseStatusCode:p.responseStatusCode,responseErrorReason:p.responseErrorReason,responseHeaders:p.responseHeaders};
    }else if(event.method==='Network.responseReceivedExtraInfo'){
     // Only status and the trusted event time are retained; never headers or cookies.
     if(root&&p.requestId===root.requestId){extraCount++;if(extraCount!==1)mark('delivery_unproven','extra_duplicate','root_delivery');if(p.statusCode!==200)mark('delivery_unproven','extra_status','root_delivery');extra={status:p.statusCode,receivedAtMS:now()};}
    }else if(event.method==='Network.responseReceived'&&root&&p.requestId===root.requestId){
     responseCount++;response={frameId:p.frameId,loaderId:p.loaderId,type:p.type,url:p.response?.url,status:p.response?.status,
      cache:['fromDiskCache','fromPrefetchCache'].some(k=>p.response?.[k]!==undefined&&p.response[k]!==false),cacheKind:p.response?.fromDiskCache!==undefined&&p.response.fromDiskCache!==false?'disk_cache':'prefetch_cache',worker:p.response?.fromServiceWorker!==undefined&&p.response.fromServiceWorker!==false};
     const condition=responseCount!==1?'response_duplicate':response.frameId!==rootFrame?'response_frame':response.loaderId!==root.loaderId?'response_loader':response.type!=='Document'?'response_resource':response.url!==home?'response_url':response.status!==200?'response_status':null;
     if(condition)mark('wire_unmatched',condition,'root_delivery');if(response.cache)mark('delivery_unproven',response.cacheKind,'root_delivery');if(response.worker)mark('delivery_unproven','service_worker','root_delivery');
    }else if(event.method==='Network.requestServedFromCache'&&root&&p.requestId===root.requestId){cached=true;mark('delivery_unproven','cache_observed','root_delivery');}
    else if(event.method==='Network.loadingFinished'&&root&&p.requestId===root.requestId)finished=true;
    else if(event.method==='Network.loadingFailed'&&root&&p.requestId===root.requestId)mark('navigation_failed','navigation_failed','navigation');
    else if(event.method==='Runtime.executionContextCreated'){
     const c=p.context;if(c?.auxData?.isDefault===true&&c.auxData.frameId===rootFrame&&c.origin==='https://www.instagram.com'&&reference(c.uniqueId)){
      if(pinned&&c.uniqueId!==pinned.uniqueId)invalidated=true;context={uniqueId:c.uniqueId,id:c.id};
     }
    }else if(event.method==='Runtime.executionContextsCleared'){if(pinned)invalidated=true;context=undefined;}
    else if(event.method==='Runtime.executionContextDestroyed'&&context&&(p.executionContextId===context.id||p.executionContextUniqueId===context.uniqueId)){if(pinned)invalidated=true;context=undefined;}
    else if(event.method==='Page.frameNavigated'&&p.frame?.id===rootFrame){if(root&&(p.frame.loaderId!==root.loaderId||p.frame.url!==home)||pinned)invalidated=true;}
    else if((event.method==='Page.frameDetached'||event.method==='Page.navigatedWithinDocument')&&p.frameId===rootFrame)invalidated=true;
   }catch{mark('wire_unmatched','invalid_reply','root_delivery');}
  });
  trace.cleanup('pending');attempted=true;
  if(scope.ownedSession){session=scope.ownedSession;}else{
  active('target_create');target=(await scope.call('Target.createTarget',{url:'about:blank',background:true,forTab:false},'',remaining()))?.targetId;
  if(!reference(target))fail('setup_failed');trace.done('target_create');
  active('target_attach');session=(await scope.call('Target.attachToTarget',{targetId:target,flatten:true},'',remaining()))?.sessionId;
  if(!reference(session))fail('setup_failed');trace.done('target_attach');
  }
  active('setup');for(const method of ['Page.enable','Runtime.enable','Network.enable'])await send(method);
  rootFrame=(await frame()).id;
  await send('Network.setCacheDisabled',{cacheDisabled:true});await send('Network.setBypassServiceWorker',{bypass:true});
  await send('Fetch.enable',{patterns:[{urlPattern:home,resourceType:'Document',requestStage:'Response'}]});trace.done('setup');
  phase='navigation_failed';active('navigation');
  // Navigation can await the paused response. Observe/stream/replay while the
  // navigation promise is pending, without opening a second request path.
  const navigationTask=send('Page.navigate',{url:home},Math.min(15000,until-now())).then(v=>{navigation=v;navigationSettled=true;},()=>{navigationSettled=true;mark('navigation_failed','navigation_failed','navigation');});
  await wait(()=>paused&&root);
  const pauseCondition=paused.networkId!==root.requestId?'pause_network_id':paused.frameId!==rootFrame?'pause_frame':paused.resourceType!=='Document'?'pause_resource':paused.request?.url!==home?'pause_url':paused.request?.method!=='GET'?'pause_method':null;
  if(pauseCondition)fail('wire_unmatched',pauseCondition,'root_delivery');
  if(paused.responseStatusCode!==200)fail('http_rejected','http_rejected','root_delivery');if(paused.responseErrorReason)fail('http_rejected','response_error','root_delivery');
  const headers=paused.responseHeaders;
  if(!Array.isArray(headers)||headers.length>policy.maxHeaders||Buffer.byteLength(JSON.stringify(headers))>policy.maxHeadersBytes)fail('body_bounds');
  const types=headers.filter(h=>typeof h.name==='string'&&h.name.toLowerCase()==='content-type');
  if(types.length!==1||!/^text\/html(?:\s*;\s*charset\s*=\s*["']?utf-8["']?)?\s*$/i.test(types[0].value))fail('body_encoding');
  phase='delivery_unproven';active('root_delivery');await wait(()=>extra);
  if(extraCount!==1)fail('delivery_unproven','extra_duplicate','root_delivery');if(extra.status!==200)fail('delivery_unproven','extra_status','root_delivery');if(cached)fail('delivery_unproven','cache_observed','root_delivery');trace.done('root_delivery');
  phase='body_read';active('body_read');
  const handle=(await send('Fetch.takeResponseBodyAsStream',{requestId:paused.requestId}))?.stream;
  if(!reference(handle))fail('body_read');
  const read=await readInstagramBootstrapBodyStream(scope,session,handle,{deadlineAt:until,now,trace});
  streamCleanup=read.cleanup==='verified';body=read.body;
  if(read.reason!=='none')fail(read.reason);
  if(!streamCleanup){trace.cleanup('uncertain','stream_close_failed');phase='cleanup_failed';throw reject();}if(!body)fail('body_read');result.bodyBound=true;trace.done('body_read');
  if(now()-extra.receivedAtMS>policy.freshMS)fail('stale_delivery');
  active('replay');await scope.replay(body,remaining());trace.done('replay');
  phase='navigation_failed';active('navigation');await wait(()=>navigationSettled);await navigationTask;
  if(!navigation||navigation.errorText)fail('navigation_failed','navigation_error');if(navigation.frameId!==rootFrame)fail('navigation_failed','navigation_frame');if(navigation.loaderId!==root.loaderId)fail('navigation_failed','navigation_loader');
  phase='context_unavailable';active('document');await wait(()=>context&&response&&finished);
  const responseCondition=requestCount!==1?'root_request_duplicate':pauseCount!==1?'pause_duplicate':responseCount!==1?'response_duplicate':extraCount!==1?'extra_duplicate':response.frameId!==rootFrame?'response_frame':response.loaderId!==root.loaderId?'response_loader':response.type!=='Document'?'response_resource':response.url!==home?'response_url':response.status!==200?'response_status':null;
  if(responseCondition)fail('wire_unmatched',responseCondition,'root_delivery');
  if(cached)fail('delivery_unproven','cache_observed','root_delivery');if(response.cache)fail('delivery_unproven',response.cacheKind,'root_delivery');if(response.worker)fail('delivery_unproven','service_worker','root_delivery');
  for(;;){
   if(invalidated)fail('document_changed');
   const f=await frame();if(f.id!==rootFrame||f.loaderId!==root.loaderId||f.url!==home)fail('document_changed');
   const c=context;if(!c)fail('context_unavailable');
   const ready=await scope.ready(c.uniqueId,remaining());
   if(ready?.result?.value===true&&!ready.exceptionDetails){pinned={uniqueId:c.uniqueId};break;}await pause();remaining();
  }
  const fresh=()=>{const age=now()-extra.receivedAtMS;return age>=0&&age<=policy.freshMS;};
  if(!fresh())fail('stale_delivery');
  phase='evaluation_failed';active('parsing');
  const reply=await scope.inspect(body,request.nonce,pinned.uniqueId,Math.min(policy.evaluationMS,until-now()));
  if(reply?.exceptionDetails)fail('evaluation_failed');
  const value=reply?.result?.value;
  const privateSuccess=scope.ownedSession&&value?.reason==='corroborated';
  if(!record(value,['nonce','reason','scanComplete','idMatched','cookieMatched',...(privateSuccess?['accountId']:[])])||value.nonce!==request.nonce||!pageReasons.includes(value.reason)||['scanComplete','idMatched','cookieMatched'].some(k=>typeof value[k]!=='boolean')||
   privateSuccess&&(typeof value.accountId!=='string'||!/^[1-9][0-9]{0,29}$/.test(value.accountId))||
   value.reason==='corroborated'&&(!value.scanComplete||!value.idMatched||!value.cookieMatched)||Buffer.byteLength(JSON.stringify(value))>1024)fail('projection_invalid');
  for(const k of ['scanComplete','idMatched','cookieMatched'])result[k]=value[k];
  if(value.scanComplete)trace.done('parsing');if(value.idMatched)trace.done('id_comparison');if(value.cookieMatched)trace.done('cookie_comparison');
  if(value.reason!=='corroborated'){const stage=value.reason.startsWith('cookie_')?'cookie_comparison':value.reason==='id_mismatch'?'id_comparison':'parsing';trace.fail(stage,value.reason);}
  active('post_check');const after=await frame();
  if(firstCause)throw reject();
  if(requestCount!==1)fail('wire_unmatched','root_request_duplicate','root_delivery');if(pauseCount!==1)fail('wire_unmatched','pause_duplicate','root_delivery');if(responseCount!==1)fail('wire_unmatched','response_duplicate','root_delivery');if(extraCount!==1)fail('wire_unmatched','extra_duplicate','root_delivery');
  if(cached)fail('delivery_unproven','cache_observed','root_delivery');if(response.cache)fail('delivery_unproven',response.cacheKind,'root_delivery');if(response.worker)fail('delivery_unproven','service_worker','root_delivery');
  result.documentBound=!invalidated&&after.id===rootFrame&&after.loaderId===root.loaderId&&after.url===home&&context?.uniqueId===pinned.uniqueId;
  if(!result.documentBound)fail('document_changed');
  result.serverFresh=fresh();if(!result.serverFresh)fail('stale_delivery');
  result.reason=value.reason;
  if(privateSuccess){accountId=value.accountId;observedAtMS=extra.receivedAtMS;documentBinding={id:rootFrame,loaderId:root.loaderId,url:home,uniqueContextId:pinned.uniqueId,contextId:context.id};value.accountId='';}
 }catch(error){const detail=igFailureDetail(error);result.reason=firstCause??phase;if(result.reason!=='cleanup_failed')trace.fail(detail?.stage??trace.snapshot().stage,detail?.condition??result.reason);}
 finally{
  active('cleanup');body?.fill(0);body=undefined;paused=undefined;
  if(attempted)result.cleanup='uncertain';
  if(scope?.ownedSession){try{if(await scope.retire(Math.max(1,Math.min(1000,policy.cleanupMS))))result.cleanup=streamCleanup?'verified':'uncertain';}catch{}}
  if(target){
   try{
    const cleanupUntil=now()+policy.cleanupMS;
    try{await scope.call('Target.closeTarget',{targetId:target},'',Math.min(1000,policy.cleanupMS));}catch{}
    while(now()<cleanupUntil){
     const inventory=await scope.call('Target.getTargets',{},'',Math.max(1,Math.min(1000,cleanupUntil-now())));
     if(!Array.isArray(inventory?.targetInfos))throw reject();
     if(!inventory.targetInfos.some(t=>t?.targetId===target)){
      if(session)scope.releaseSession(session);scope.releaseTarget(target);
      if(scope.cleanupVerified())result.cleanup='verified';break;
     }await pause();
    }
   }catch{}
  }
  if(result.cleanup==='uncertain'&&result.reason==='corroborated')result.reason='cleanup_failed';
  try{off?.();}catch{result.cleanup='uncertain';trace.cleanup('uncertain','dispose_failed');}try{scope?.dispose();}catch{result.cleanup='uncertain';trace.cleanup('uncertain','dispose_failed');}
  trace.cleanup(result.cleanup,result.cleanup==='uncertain'?'cleanup_failed':'none');
 }
 if(result.cleanup==='uncertain'&&result.reason==='corroborated')result.reason='cleanup_failed';
 const diagnostic=validateInstagramBootstrapCorroboration(result);
 if(diagnostic.reason!=='corroborated'||diagnostic.cleanup!=='verified'){accountId='';observedAtMS=undefined;}
 return {diagnostic,accountId,observedAtMS,documentBinding};
}

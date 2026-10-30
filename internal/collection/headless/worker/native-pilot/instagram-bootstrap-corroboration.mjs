import {readInstagramBootstrapBodyStream} from './instagram-bootstrap-body-stream.mjs';
import {INSTAGRAM_CORROBORATION_POLICY as policy,INSTAGRAM_CORROBORATION_HOME as home} from './instagram-bootstrap-corroboration-policy.mjs';
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
export async function runInstagramBootstrapCorroboration(transport,request){
 const result={schema:INSTAGRAM_CORROBORATION_SCHEMA,reason:'setup_failed',...Object.fromEntries(booleans.map(k=>[k,false])),qualified:false,cleanup:'not_started'};
 if(!record(request,['nonce'])||typeof request.nonce!=='string'||!/^[a-f0-9]{48}$/.test(request.nonce)){result.reason='invalid_request';return validateInstagramBootstrapCorroboration(result);}
 let scope,target,session,off,body,attempted=false,phase='setup_failed',firstCause;
 let rootFrame,root,paused,extra,response,finished=false,cached=false,invalidated=false,pinned,context,streamCleanup=true;
 let requestCount=0,pauseCount=0,extraCount=0,responseCount=0,navigationSettled=false,navigation;
 const until=Date.now()+policy.acquisitionMS;
 const fail=reason=>{firstCause??=reason;throw reject();};
 const remaining=()=>{const ms=until-Date.now();if(ms<1)fail(phase);return Math.min(ms,1000);};
 const send=(method,params={},ms=remaining())=>scope.call(method,params,session??'',ms);
 const frame=async()=>{const f=(await send('Page.getFrameTree'))?.frameTree?.frame;if(!f||!reference(f.id)||f.parentId!==undefined)fail('document_changed');return f;};
 const wait=async predicate=>{while(!predicate()){remaining();if(firstCause)throw reject();await pause();}};
 try{
  if(typeof transport?.createBootstrapCorroborationScope!=='function')fail('scope_unavailable');
  scope=transport.createBootstrapCorroborationScope();
  off=scope.on(event=>{
   if(!session||event.sessionId!==session)return;
   const p=event.params??{};
   try{
    if(event.method==='Network.requestWillBeSent'&&p.frameId===rootFrame&&p.type==='Document'){
     requestCount++;
     if(requestCount!==1||p.request?.url!==home||p.request?.method!=='GET'||p.redirectResponse||!reference(p.requestId)||!reference(p.loaderId)){firstCause??='wire_unmatched';return;}
     root={requestId:p.requestId,loaderId:p.loaderId};
    }else if(event.method==='Fetch.requestPaused'){
     pauseCount++;if(pauseCount!==1){firstCause??='wire_unmatched';return;}
     paused={requestId:p.requestId,networkId:p.networkId,frameId:p.frameId,resourceType:p.resourceType,request:{url:p.request?.url,method:p.request?.method},responseStatusCode:p.responseStatusCode,responseErrorReason:p.responseErrorReason,responseHeaders:p.responseHeaders};
    }else if(event.method==='Network.responseReceivedExtraInfo'){
     // Only status and the trusted event time are retained; never headers or cookies.
     if(root&&p.requestId===root.requestId){extraCount++;extra={status:p.statusCode,receivedAtMS:Date.now()};}
    }else if(event.method==='Network.responseReceived'&&root&&p.requestId===root.requestId){
     responseCount++;response={frameId:p.frameId,loaderId:p.loaderId,type:p.type,url:p.response?.url,status:p.response?.status,
      cache:['fromDiskCache','fromPrefetchCache'].some(k=>p.response?.[k]!==undefined&&p.response[k]!==false),worker:p.response?.fromServiceWorker!==undefined&&p.response.fromServiceWorker!==false};
    }else if(event.method==='Network.requestServedFromCache'&&root&&p.requestId===root.requestId)cached=true;
    else if(event.method==='Network.loadingFinished'&&root&&p.requestId===root.requestId)finished=true;
    else if(event.method==='Network.loadingFailed'&&root&&p.requestId===root.requestId)firstCause??='navigation_failed';
    else if(event.method==='Runtime.executionContextCreated'){
     const c=p.context;if(c?.auxData?.isDefault===true&&c.auxData.frameId===rootFrame&&c.origin==='https://www.instagram.com'&&reference(c.uniqueId)){
      if(pinned&&c.uniqueId!==pinned.uniqueId)invalidated=true;context={uniqueId:c.uniqueId,id:c.id};
     }
    }else if(event.method==='Runtime.executionContextsCleared'){if(pinned)invalidated=true;context=undefined;}
    else if(event.method==='Runtime.executionContextDestroyed'&&context&&(p.executionContextId===context.id||p.executionContextUniqueId===context.uniqueId)){if(pinned)invalidated=true;context=undefined;}
    else if(event.method==='Page.frameNavigated'&&p.frame?.id===rootFrame){if(root&&(p.frame.loaderId!==root.loaderId||p.frame.url!==home)||pinned)invalidated=true;}
    else if((event.method==='Page.frameDetached'||event.method==='Page.navigatedWithinDocument')&&p.frameId===rootFrame)invalidated=true;
   }catch{firstCause??='wire_unmatched';}
  });
  attempted=true;
  target=(await scope.call('Target.createTarget',{url:'about:blank',background:true,forTab:false},'',remaining()))?.targetId;
  if(!reference(target))fail('setup_failed');
  session=(await scope.call('Target.attachToTarget',{targetId:target,flatten:true},'',remaining()))?.sessionId;
  if(!reference(session))fail('setup_failed');
  for(const method of ['Page.enable','Runtime.enable','Network.enable'])await send(method);
  rootFrame=(await frame()).id;
  await send('Network.setCacheDisabled',{cacheDisabled:true});await send('Network.setBypassServiceWorker',{bypass:true});
  await send('Fetch.enable',{patterns:[{urlPattern:home,resourceType:'Document',requestStage:'Response'}]});
  phase='navigation_failed';
  // Navigation can await the paused response. Observe/stream/replay while the
  // navigation promise is pending, without opening a second request path.
  const navigationTask=send('Page.navigate',{url:home},Math.min(15000,until-Date.now())).then(v=>{navigation=v;navigationSettled=true;},()=>{navigationSettled=true;firstCause??='navigation_failed';});
  await wait(()=>paused&&root);
  if(paused.networkId!==root.requestId||paused.frameId!==rootFrame||paused.resourceType!=='Document'||paused.request?.url!==home||paused.request?.method!=='GET')fail('wire_unmatched');
  if(paused.responseStatusCode!==200||paused.responseErrorReason)fail('http_rejected');
  const headers=paused.responseHeaders;
  if(!Array.isArray(headers)||headers.length>policy.maxHeaders||Buffer.byteLength(JSON.stringify(headers))>policy.maxHeadersBytes)fail('body_bounds');
  const types=headers.filter(h=>typeof h.name==='string'&&h.name.toLowerCase()==='content-type');
  if(types.length!==1||!/^text\/html(?:\s*;\s*charset\s*=\s*["']?utf-8["']?)?\s*$/i.test(types[0].value))fail('body_encoding');
  phase='delivery_unproven';await wait(()=>extra);
  if(extraCount!==1||extra.status!==200||cached)fail('delivery_unproven');
  phase='body_read';
  const handle=(await send('Fetch.takeResponseBodyAsStream',{requestId:paused.requestId}))?.stream;
  if(!reference(handle))fail('body_read');
  const read=await readInstagramBootstrapBodyStream(scope,session,handle,{deadlineAt:until});
  streamCleanup=read.cleanup==='verified';body=read.body;
  if(read.reason!=='none')fail(read.reason);
  if(!body||!streamCleanup)fail('body_read');result.bodyBound=true;
  if(Date.now()-extra.receivedAtMS>policy.freshMS)fail('stale_delivery');
  await scope.replay(body,remaining());
  phase='navigation_failed';await wait(()=>navigationSettled);await navigationTask;
  if(!navigation||navigation.errorText||navigation.frameId!==rootFrame||navigation.loaderId!==root.loaderId)fail('navigation_failed');
  phase='context_unavailable';await wait(()=>context&&response&&finished);
  if(requestCount!==1||pauseCount!==1||responseCount!==1||extraCount!==1||response.frameId!==rootFrame||response.loaderId!==root.loaderId||response.type!=='Document'||response.url!==home||response.status!==200)fail('wire_unmatched');
  if(cached||response.cache||response.worker)fail('delivery_unproven');
  for(;;){
   if(invalidated)fail('document_changed');
   const f=await frame();if(f.id!==rootFrame||f.loaderId!==root.loaderId||f.url!==home)fail('document_changed');
   const c=context;if(!c)fail('context_unavailable');
   const ready=await scope.ready(c.uniqueId,remaining());
   if(ready?.result?.value===true&&!ready.exceptionDetails){pinned={uniqueId:c.uniqueId};break;}await pause();remaining();
  }
  const fresh=()=>{const age=Date.now()-extra.receivedAtMS;return age>=0&&age<=policy.freshMS;};
  if(!fresh())fail('stale_delivery');
  phase='evaluation_failed';
  const reply=await scope.inspect(body,request.nonce,pinned.uniqueId,Math.min(policy.evaluationMS,until-Date.now()));
  if(reply?.exceptionDetails)fail('evaluation_failed');
  const value=reply?.result?.value;
  if(!record(value,['nonce','reason','scanComplete','idMatched','cookieMatched'])||value.nonce!==request.nonce||!pageReasons.includes(value.reason)||['scanComplete','idMatched','cookieMatched'].some(k=>typeof value[k]!=='boolean')||
   value.reason==='corroborated'&&(!value.scanComplete||!value.idMatched||!value.cookieMatched)||Buffer.byteLength(JSON.stringify(value))>1024)fail('projection_invalid');
  for(const k of ['scanComplete','idMatched','cookieMatched'])result[k]=value[k];
  const after=await frame();
  if(firstCause)throw reject();
  if(requestCount!==1||pauseCount!==1||responseCount!==1||extraCount!==1)fail('wire_unmatched');
  if(cached||response.cache||response.worker)fail('delivery_unproven');
  result.documentBound=!invalidated&&after.id===rootFrame&&after.loaderId===root.loaderId&&after.url===home&&context?.uniqueId===pinned.uniqueId;
  if(!result.documentBound)fail('document_changed');
  result.serverFresh=fresh();if(!result.serverFresh)fail('stale_delivery');
  result.reason=value.reason;
 }catch{result.reason=firstCause??phase;}
 finally{
  body?.fill(0);body=undefined;paused=undefined;
  if(attempted)result.cleanup='uncertain';
  if(target){
   try{
    const cleanupUntil=Date.now()+policy.cleanupMS;
    try{await scope.call('Target.closeTarget',{targetId:target},'',Math.min(1000,policy.cleanupMS));}catch{}
    while(Date.now()<cleanupUntil){
     const inventory=await scope.call('Target.getTargets',{},'',Math.max(1,Math.min(1000,cleanupUntil-Date.now())));
     if(!Array.isArray(inventory?.targetInfos))throw reject();
     if(!inventory.targetInfos.some(t=>t?.targetId===target)){
      if(session)scope.releaseSession(session);scope.releaseTarget(target);
      if(scope.cleanupVerified())result.cleanup='verified';break;
     }await pause();
    }
   }catch{}
  }
  if(result.cleanup==='uncertain'&&result.reason==='corroborated')result.reason='cleanup_failed';
  try{off?.();}catch{}try{scope?.dispose();}catch{}
 }
 return validateInstagramBootstrapCorroboration(result);
}

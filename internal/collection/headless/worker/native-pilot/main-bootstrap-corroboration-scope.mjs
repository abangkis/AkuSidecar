import {igFailure} from './instagram-diagnostic-trace.mjs';
import {createMainDiagnosticScope} from './main-browser-diagnostic-scope.mjs';
import {instagramBootstrapCorroborationExpression} from './instagram-bootstrap-corroboration-expression.mjs';
import {INSTAGRAM_CORROBORATION_POLICY as policy,INSTAGRAM_CORROBORATION_HOME as home,INSTAGRAM_CORROBORATION_READY as ready} from './instagram-bootstrap-corroboration-policy.mjs';

const ref=x=>typeof x==='string'&&x.length>0&&x.length<=512&&!/[\u0000-\u001f\u007f]/.test(x);
const exact=(x,keys)=>x&&typeof x==='object'&&!Array.isArray(x)&&Object.keys(x).length===keys.length&&keys.every(k=>Object.hasOwn(x,k));

// Separate private capability. Ordinary/passive scopes never gain Fetch, IO,
// arbitrary evaluation, cookie access or a body-returning public operation.
export function createMainBootstrapCorroborationScope(raw,trace){
 const reject=(condition='invalid_parameters')=>igFailure(trace?.snapshot().stage??'scope',condition);
 const base=createMainDiagnosticScope(raw),listeners=new Set(),streams=new Map();
 let session,paused,disposed=false,pendingStreams=0,taken=false,replayed=false,fetchEnabled=false,navigated=false;
 const own=id=>{if(disposed)throw reject('scope_disposed');if(id!==session)throw reject('ownership');base.reserveSession(id);};
 const unsubscribe=raw.subscribe(event=>{
  if(disposed||!session||event.sessionId!==session)return;
  if(event.method==='Fetch.requestPaused'){
   if(!paused&&ref(event.params?.requestId))paused={requestId:event.params.requestId,responseStatusCode:event.params.responseStatusCode,responseHeaders:event.params.responseHeaders};
  }
  if(['Fetch.requestPaused','Network.responseReceivedExtraInfo','Network.requestServedFromCache'].includes(event.method)){
   for(const listener of listeners)try{listener(event);}catch{}
  }
 });
 const timeout=ms=>{if(!Number.isInteger(ms)||ms<1||ms>15000)throw reject();return ms;};
 const evaluate=async(expression,context,ms)=>{
  own(session);if(!ref(context)||!replayed||Buffer.byteLength(expression)>7*1024*1024)throw reject();
  return raw.call('Runtime.evaluate',{expression,uniqueContextId:context,returnByValue:true,awaitPromise:true,userGesture:false},session,timeout(ms));
 };
 return Object.freeze({
  alive:base.alive,reserveTarget:base.reserveTarget,reserveSession:base.reserveSession,
  releaseTarget:base.releaseTarget,releaseSession:base.releaseSession,
  cleanupVerified(){return base.cleanupVerified()&&pendingStreams===0&&streams.size===0;},
  on(listener){if(disposed||typeof listener!=='function'||listeners.size>=2)throw reject();listeners.add(listener);const off=base.on(listener);return()=>{listeners.delete(listener);off();};},
  async call(method,params={},id='',ms=1000){
   timeout(ms);if(disposed)throw reject('scope_disposed');
   if(method==='Runtime.evaluate')throw reject('invalid_operation');
   if(method==='Page.navigate'){if(navigated||!fetchEnabled)throw reject('method_order');navigated=true;}
   if(method==='Target.attachToTarget'){
    const reply=await base.call(method,params,id,ms);session=reply.sessionId;return reply;
   }
   if(['Fetch.enable','Fetch.takeResponseBodyAsStream','IO.read','IO.close'].includes(method)){
    own(id);
    if(method==='Fetch.enable'){
     if(fetchEnabled||!exact(params,['patterns'])||!Array.isArray(params.patterns)||params.patterns.length!==1||
       !exact(params.patterns[0],['urlPattern','resourceType','requestStage'])||params.patterns[0].urlPattern!==home||params.patterns[0].resourceType!=='Document'||params.patterns[0].requestStage!=='Response')throw reject();
     fetchEnabled=true;return raw.call(method,params,id,ms);
    }
    if(method==='Fetch.takeResponseBodyAsStream'){
     if(!fetchEnabled||taken||!paused||!exact(params,['requestId'])||params.requestId!==paused.requestId)throw reject();
     taken=true;pendingStreams++;
     const late=async reply=>{if(ref(reply?.stream)){try{await raw.call('IO.close',{handle:reply.stream},id,1000);pendingStreams--;}catch{}}};
     const reply=await raw.call(method,params,id,ms,late);
     if(!ref(reply?.stream))throw reject('invalid_reply');streams.set(reply.stream,id);pendingStreams--;return reply;
    }
    if(!ref(params?.handle)||streams.get(params.handle)!==id)throw reject('ownership');
    if(method==='IO.read'){
     if(!exact(params,['handle','size'])||!Number.isInteger(params.size)||params.size<1||params.size>policy.chunkBytes)throw reject();
    }else if(!exact(params,['handle']))throw reject();
    const reply=await raw.call(method,params,id,ms);if(method==='IO.close')streams.delete(params.handle);return reply;
   }
   return base.call(method,params,id,ms);
  },
  async replay(body,ms=1000){
   own(session);if(!taken||replayed||streams.size||!Buffer.isBuffer(body)||body.length>policy.maxBodyBytes||paused?.responseStatusCode!==200)throw reject();
   const headers=paused.responseHeaders;
   if(!Array.isArray(headers)||headers.length>policy.maxHeaders||Buffer.byteLength(JSON.stringify(headers))>policy.maxHeadersBytes)throw reject();
   if(headers.some(h=>!exact(h,['name','value'])||typeof h.name!=='string'||! /^[!#$%&'*+.^_`|~0-9A-Za-z-]+$/.test(h.name)||typeof h.value!=='string'||/[\r\n\0]/.test(h.value)))throw reject();
   // Fetch stream bytes are decoded; preserve origin policy/cookie headers but
   // replace transport encodings/length before fulfilling the owned document.
   const responseHeaders=headers.filter(h=>!['content-length','content-encoding','transfer-encoding'].includes(h.name.toLowerCase()));
   responseHeaders.push({name:'Content-Length',value:String(body.length)});
   await raw.call('Fetch.fulfillRequest',{requestId:paused.requestId,responseCode:200,responseHeaders,body:body.toString('base64')},session,timeout(ms));
   replayed=true;
  },
  ready:(context,ms)=>evaluate(ready,context,ms),
  inspect:(body,nonce,context,ms)=>evaluate(instagramBootstrapCorroborationExpression(body,nonce),context,ms),
  dispose(){if(disposed)return;disposed=true;unsubscribe();listeners.clear();paused=undefined;streams.clear();base.dispose();},
 });
}

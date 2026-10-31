import assert from 'node:assert/strict';
import {runInNewContext} from 'node:vm';
import {createMainBootstrapCorroborationScope} from './main-bootstrap-corroboration-scope.mjs';
import {instagramBootstrapCorroborationExpression} from './instagram-bootstrap-corroboration-expression.mjs';
import {INSTAGRAM_CORROBORATION_POLICY as policy} from './instagram-bootstrap-corroboration-policy.mjs';
const nonce='a'.repeat(48),home='https://www.instagram.com/';
export const tuple=id=>JSON.stringify(['PolarisViewer',[],{id},1]);
const htmlFor=id=>`<!doctype html><html><head><script type="application/json">${tuple(id)}</script></head><body></body></html>`;

// Synthetic DOM fixture only; the production path uses the browser's inert
// HTML template parser. The strict JSON scanner/expression run unmodified.
export function realm(options={}){
 const calls=[],bodyID=options.bodyID??'123',domID=options.domID??'123';
 const doc=scripts=>({querySelector:selector=>options.login&&selector.includes('password')?{}:options.challenge&&selector.includes('captcha')?{}:null,
  querySelectorAll:()=>scripts.map(textContent=>({textContent}))});
 const actual=doc(options.domScripts??[tuple(domID)]);
 actual.readyState='complete';
 actual.createElement=name=>{assert.equal(name,'template');return {content:doc(options.bodyScripts??[tuple(bodyID)]),set innerHTML(value){if(value)assert.equal(value,htmlFor(bodyID));}};};
 Object.defineProperty(actual,'cookie',{get(){throw Error('whole cookie jar forbidden');}});
 const sandbox={document:actual,location:{origin:'https://www.instagram.com',pathname:'/'},performance,TextEncoder,TextDecoder,Uint8Array,atob,
  cookieStore:options.unsupported?undefined:{async getAll(value){calls.push(value);options.onCookie?.();if(options.cookieFailure)throw Error('private cookie failure');return options.cookies??[{name:'ds_user_id',value:options.cookieID??'123'}];}}};
 return {sandbox,calls,body:Buffer.from(htmlFor(bodyID))};
}
export async function expression(options={}){const f=realm(options);return {value:JSON.parse(JSON.stringify(await runInNewContext(instagramBootstrapCorroborationExpression(f.body,nonce),f.sandbox))),calls:f.calls};}

export function fixture(options={}){
 const listeners=new Set(),targets=new Set(),sessions=new Set(),calls=[],f=realm(options);
 let committed=false,closed=false,offset=0,resolveNavigation,disposed=false;
 const emit=(method,params)=>{for(const listener of listeners)listener({sessionId:'owned-session',method,params});};
 const response=()=>emit('Network.responseReceived',{requestId:'root-request',frameId:'root-frame',loaderId:'root-loader',type:'Document',response:{url:home,status:200,...options.responseFlags}});
 const extra=()=>emit('Network.responseReceivedExtraInfo',{requestId:'root-request',statusCode:options.extraStatus??200,headers:{'set-cookie':'PRIVATE_UNREAD'}});
 const paused=()=>emit('Fetch.requestPaused',{requestId:'fetch-request',networkId:options.wrongRequest?'foreign':'root-request',frameId:'root-frame',resourceType:'Document',request:{url:home,method:'GET'},responseStatusCode:options.status??200,
  responseHeaders:[{name:'Content-Type',value:'text/html; charset=utf-8'},{name:'Content-Length',value:'1'},{name:'Content-Encoding',value:'gzip'}]});
 const raw={alive:()=>true,privateTarget:id=>targets.has(id),privateSession:id=>sessions.has(id),reserveTarget:id=>targets.add(id),reserveSession:id=>sessions.add(id),releaseTarget:id=>targets.delete(id),releaseSession:id=>sessions.delete(id),
  subscribe(listener){listeners.add(listener);return()=>listeners.delete(listener);},
  async call(method,params,session,timeoutMS){
   calls.push({method,params,session,timeoutMS});
   if(method==='Target.createTarget')return {targetId:'owned-target'};
   if(method==='Target.attachToTarget')return {sessionId:'owned-session'};
   if(method==='Page.getFrameTree')return {frameTree:{frame:{id:'root-frame',loaderId:committed?'root-loader':'initial-loader',url:committed?home:'about:blank'}}};
   if(method==='Page.navigate'){
    offset=0;
    queueMicrotask(()=>{
     emit('Network.requestWillBeSent',{requestId:'root-request',frameId:'root-frame',loaderId:'root-loader',type:'Document',request:{url:home,method:'GET'}});
     if(options.order==='response-first')response();
     if(options.order==='pause-first'){paused();extra();}else{extra();paused();}
     if(options.cached)emit('Network.requestServedFromCache',{requestId:'root-request'});
    });
    return new Promise(resolve=>{resolveNavigation=resolve;});
   }
   if(method==='Fetch.takeResponseBodyAsStream')return {stream:'owned-stream'};
   if(method==='IO.read'){
    options.onRead?.();
    if(options.readFailure)throw Error('PRIVATE_BODY_ERROR');
    const source=options.oversize?Buffer.alloc(policy.maxBodyBytes+1,65):f.body;
    const bytes=source.subarray(offset,offset+params.size);offset+=bytes.length;
    return {data:bytes.toString('base64'),base64Encoded:true,eof:offset===source.length};
   }
   if(method==='IO.close'){if(options.closeStreamFails)throw Error('PRIVATE_CLOSE_ERROR');return {};}
   if(method==='Fetch.fulfillRequest'){
    assert.deepEqual(Buffer.from(params.body,'base64'),f.body);
    assert.ok(!params.responseHeaders.some(h=>['content-encoding','transfer-encoding'].includes(h.name.toLowerCase())));
    assert.equal(params.responseHeaders.find(h=>h.name.toLowerCase()==='content-length').value,String(f.body.length));
    committed=true;
    emit('Page.frameNavigated',{frame:{id:'root-frame',loaderId:'root-loader',url:home}});
    emit('Runtime.executionContextCreated',{context:{id:7,uniqueId:'root-context',origin:'https://www.instagram.com',auxData:{isDefault:true,frameId:'root-frame'}}});
    if(options.order!=='response-first')response();
    emit('Network.loadingFinished',{requestId:'root-request'});
    resolveNavigation({frameId:'root-frame',loaderId:'root-loader'});return {};
   }
   if(method==='Runtime.evaluate'){
    let value=await runInNewContext(params.expression,f.sandbox);
    if(options.forgedReply&&params.expression.includes('cookieStore'))value={nonce,reason:'corroborated',scanComplete:false,idMatched:false,cookieMatched:false};
    if(options.changeContext&&params.expression.includes('cookieStore'))emit('Runtime.executionContextsCleared',{});
    return {result:{value:JSON.parse(JSON.stringify(value))}};
   }
   if(method==='Target.closeTarget'){options.onCloseTarget?.();closed=true;resolveNavigation?.({errorText:'closed'});return {success:true};}
   if(method==='Target.getTargets'){
    if(options.cleanupFails)throw Error('PRIVATE_INVENTORY_ERROR');
    return {targetInfos:closed?[{targetId:'user-owned-reader'}]:[{targetId:'owned-target'},{targetId:'user-owned-reader'}]};
   }
   if(['Page.enable','Runtime.enable','Network.enable','Network.setCacheDisabled','Network.setBypassServiceWorker','Fetch.enable','Fetch.disable'].includes(method))return {};
   throw Error('forbidden operation '+method);
  }};
 return {calls,raw,emit,realm:f,listeners,transport:{createBootstrapCorroborationScope:()=>createMainBootstrapCorroborationScope(raw)}};
}


// A sealed diagnostic owns its target and session. It cannot acquire another
// owner's resources or use its request-local timeout policy on ordinary calls.
const failure=()=>new Error('diagnostic_scope_rejected');
const record=value=>value!==null&&typeof value==='object'&&!Array.isArray(value);
const exact=(value,keys)=>record(value)&&Object.keys(value).length===keys.length&&keys.every(key=>Object.hasOwn(value,key));
const ref=value=>typeof value==='string'&&value.length>0&&value.length<=512&&!/[\u0000-\u001f\u007f]/u.test(value);
const events=new Set(['Runtime.executionContextCreated','Runtime.executionContextDestroyed','Runtime.executionContextsCleared',
  'Page.frameNavigated','Page.navigatedWithinDocument','Page.frameDetached','Network.requestWillBeSent','Network.responseReceived','Network.loadingFailed','Network.loadingFinished']);

export function createMainDiagnosticScope(raw){
  return createOwnedScope(raw,false);
}

// Only the sealed IG owner receives this capability. Public/X callers retain
// the ordinary transport timeout policy and cannot select a local deadline.
export function createMainInstagramOwnedScope(raw){
  return createOwnedScope(raw,true);
}

function createOwnedScope(raw,instagram){
  const targets=new Set(),sessions=new Map(),absent=new Set(),subscriptions=new Set();
  const streams=new Map(),paused=new Map();
  let disposed=false,creating=false,creationAttempted=false,allocationsPending=0;
  function adoptTarget(value){
    if(!record(value)||!ref(value.targetId)||raw.privateTarget(value.targetId))throw failure();
    targets.add(value.targetId);raw.reserveTarget(value.targetId);return value;
  }
  function adoptSession(value,target){
    if(!record(value)||!ref(value.sessionId)||raw.privateSession(value.sessionId))throw failure();
    sessions.set(value.sessionId,target);raw.reserveSession(value.sessionId);return value;
  }
  function inventory(value){
    absent.clear();
    if(!record(value)||!Array.isArray(value.targetInfos)||value.targetInfos.length>4096)throw failure();
    const ids=new Set();
    for(const item of value.targetInfos){if(!record(item)||!ref(item.targetId)||ids.has(item.targetId))throw failure();ids.add(item.targetId);}
    for(const id of targets)if(!ids.has(id))absent.add(id);else absent.delete(id);
    return value;
  }
  function releaseTarget(id){
    if(!targets.has(id)||!absent.has(id))throw failure();
    raw.releaseTarget(id);targets.delete(id);absent.delete(id);
    for(const [handle,session]of streams)if(sessions.get(session)===id)streams.delete(handle);
  }
  function releaseSession(id){
    if(!sessions.has(id)||!absent.has(sessions.get(id))&&targets.has(sessions.get(id)))throw failure();
    for(const [handle,session]of streams)if(session===id)streams.delete(handle);
    raw.releaseSession(id);sessions.delete(id);
  }
  async function retireLateTarget(value){
    // A create reply may arrive after its local deadline. Adopt only that
    // returned resource and retire it; never infer ownership from an inventory.
    let id;
    try{
      adoptTarget(value);id=value.targetId;
      allocationsPending--;
      await raw.call('Target.closeTarget',{targetId:id},'',1000);
      inventory(await raw.call('Target.getTargets',{},'',1000));
      if(absent.has(id)){
        for(const [session,target]of sessions)if(target===id)releaseSession(session);
        releaseTarget(id);
      }
    }catch{/* Unverified ownership stays pinned until browser retirement. */}
  }
  async function lateSession(value,target){
    try{
      adoptSession(value,target);
      allocationsPending--;
      if(!targets.has(target)||absent.has(target))releaseSession(value.sessionId);
    }catch{/* No late reply can acquire a peer's resource. */}
  }
  async function lateStream(value,session,late){
    try{
      if(!record(value)||!ref(value.stream)||streams.has(value.stream))throw failure();
      streams.set(value.stream,session);allocationsPending--;
      // The caller may have already canceled/disposed its bootstrap scope.
      // Retire only this late returned handle; never publish its bytes.
      if(typeof late==='function')await late(value);
      else{await raw.call('IO.close',{handle:value.stream},session,1000);streams.delete(value.stream);}
    }catch{/* Unverified stream ownership remains pinned. */}
  }
  return Object.freeze({
    alive:()=>!disposed&&raw.alive(),
    cleanupVerified:()=>allocationsPending===0&&targets.size===0&&sessions.size===0&&streams.size===0,
    reserveTarget(id){if(disposed||!targets.has(id))throw failure();},
    reserveSession(id){if(disposed||!sessions.has(id))throw failure();},
    releaseTarget,releaseSession,
    on(listener){
      if(disposed||typeof listener!=='function'||subscriptions.size>=(instagram?32:4))throw failure();
      const unsubscribe=raw.subscribe(event=>{
        if(disposed||!sessions.has(event.sessionId)||!instagram&&!events.has(event.method))return;
        if(instagram&&event.method==='Fetch.requestPaused'&&ref(event.params?.requestId))paused.set(event.sessionId,event.params.requestId);
        // A diagnostic observer cannot tear down the shared CDP reader.
        if(instagram)listener(event);else try{listener(event);}catch{}
      });
      subscriptions.add(unsubscribe);
      return()=>{subscriptions.delete(unsubscribe);unsubscribe();};
    },
    async call(method,params={},session='',timeoutMS=10000,late){
      // A late stream disposer may outlive its page observer. It retains only
      // the exact IO.close obligation, never evaluation or proof authority.
      if(disposed&&!(instagram&&method==='IO.close'&&streams.get(params.handle)===session)||!raw.alive()||!Number.isInteger(timeoutMS)||timeoutMS<1||timeoutMS>(instagram?60000:15000))throw failure();
      if(session===''){
        if(method==='Target.createTarget'){
          if(creating||creationAttempted||targets.size||!exact(params,['url','background','forTab'])||params.url!=='about:blank'||params.background!==true||params.forTab!==false)throw failure();
          creating=true;creationAttempted=true;
          allocationsPending++;
          try{const result=adoptTarget(await raw.call(method,params,'',timeoutMS,retireLateTarget));allocationsPending--;return result;}
          finally{creating=false;}
        }
        if(method==='Target.attachToTarget'){
          if(!exact(params,['targetId','flatten'])||!targets.has(params.targetId)||params.flatten!==true)throw failure();
          allocationsPending++;
          const result=adoptSession(await raw.call(method,params,'',timeoutMS,value=>lateSession(value,params.targetId)),params.targetId);
          allocationsPending--;return result;
        }
        if(method==='Target.closeTarget'){
          if(!exact(params,['targetId'])||!targets.has(params.targetId))throw failure();
          return raw.call(method,params,'',timeoutMS);
        }
        if(method==='Target.getTargets'&&exact(params,[]))return inventory(await raw.call(method,params,'',timeoutMS));
        throw failure();
      }
      if(!sessions.has(session)||!targets.has(sessions.get(session)))throw failure();
      const empty=['Page.enable','Runtime.enable','Network.enable','Page.getFrameTree'];
      if(empty.includes(method)){if(!exact(params,[]))throw failure();}
      else if(method==='Page.navigate'){
        if(!exact(params,['url'])||params.url!=='https://www.instagram.com/')throw failure();
      }else if(method==='Network.setCacheDisabled'){
        if(!exact(params,['cacheDisabled'])||params.cacheDisabled!==true)throw failure();
      }else if(method==='Network.setBypassServiceWorker'){
        if(!exact(params,['bypass'])||params.bypass!==true)throw failure();
      }else if(method==='Runtime.evaluate'){
        const keys=['expression','returnByValue','awaitPromise','userGesture'];
        if(!instagram||Object.hasOwn(params,'uniqueContextId'))keys.push('uniqueContextId');
        if(!exact(params,keys)||typeof params.expression!=='string'||params.expression.length>(instagram?7*1024*1024:32768)||params.returnByValue!==true||params.awaitPromise!==true||
          params.userGesture!==false||keys.includes('uniqueContextId')&&!ref(params.uniqueContextId))throw failure();
      }else if(instagram&&method==='Network.getCookies'){
        if(!exact(params,['urls'])||!Array.isArray(params.urls)||params.urls.length!==1||params.urls[0]!=='https://www.instagram.com/')throw failure();
      }else if(instagram&&method==='Fetch.enable'){
        if(!exact(params,['patterns'])||!Array.isArray(params.patterns)||params.patterns.length!==1||
          !exact(params.patterns[0],['urlPattern','resourceType','requestStage'])||params.patterns[0].urlPattern!=='https://www.instagram.com/'||params.patterns[0].resourceType!=='Document'||params.patterns[0].requestStage!=='Response')throw failure();
      }else if(instagram&&method==='Fetch.disable'){
        if(!exact(params,[]))throw failure();
      }else if(instagram&&method==='Fetch.takeResponseBodyAsStream'){
        if(!exact(params,['requestId'])||params.requestId!==paused.get(session))throw failure();
        allocationsPending++;
        const value=await raw.call(method,params,session,timeoutMS,value=>lateStream(value,session,late));
        if(!record(value)||!ref(value.stream)||streams.has(value.stream))throw failure();
        streams.set(value.stream,session);allocationsPending--;return value;
      }else if(instagram&&['IO.read','IO.close'].includes(method)){
        if(streams.get(params.handle)!==session||!exact(params,method==='IO.read'?['handle','size']:['handle'])||
          method==='IO.read'&&(!Number.isInteger(params.size)||params.size<1||params.size>65536))throw failure();
        const value=await raw.call(method,params,session,timeoutMS,late);if(method==='IO.close')streams.delete(params.handle);return value;
      }else if(instagram&&method==='Fetch.fulfillRequest'){
        if(!exact(params,['requestId','responseCode','responseHeaders','body'])||params.requestId!==paused.get(session)||params.responseCode!==200||
          !Array.isArray(params.responseHeaders)||typeof params.body!=='string'||params.body.length>6*1024*1024)throw failure();
      }else throw failure();
      return raw.call(method,params,session,timeoutMS,late);
    },
    dispose(){if(disposed)return;disposed=true;for(const unsubscribe of subscriptions)unsubscribe();subscriptions.clear();},
  });
}

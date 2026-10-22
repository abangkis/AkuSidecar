// Accountless diagnostics fixture. No process/profile/cookie/network capability.
import {performance} from 'node:perf_hooks';
import {createManualV2BoundObservation} from './x-qa-bound-observation.mjs';
import {inspectBootstrapCurrentSelf} from './x-bootstrap-current-self.mjs';
import {createManualStartupOwner} from './x-manual-startup-owner.mjs';
import {createXManualOwnerPort} from './x-native-manual-owner-port.mjs';
import {offlineStartupOwner} from './x-manual-startup-offline-fixture.mjs';
import {startupFailureLatch,startupFailure,requireBoundStartup} from './x-manual-startup-diagnostic.mjs';
const settingsURL='https://api.x.com/1.1/account/settings.json';
const projection=inspectBootstrapCurrentSelf({location:{origin:'https://x.com',pathname:'/home'},__INITIAL_STATE__:{session:{user_id:'1908123456789012345'},entities:{users:{entities:{'1908123456789012345':{id_str:'1908123456789012345'}}}}}});

export function offlineFirstCausePair({faultRole='reader',fault='cookie'}={}){
  if(!['reader','collector'].includes(faultRole)||!['cookie','ambiguous','context-first','cookie-context','settings-order','extra-missing','extra-status','early-deadline','early-rejected','cleanup','guard-priority','guard-budget','guard-unbound','guard-response','guard-metadata','guard-completion','context-priority','generic-priority','budget-candidate','candidate-budget'].includes(fault))throw new Error('offline_diagnostic_invalid');
  const diagnostics=startupFailureLatch(),states={},reports={};
  const fail=(role,kind='cookie')=>{
    const s=states[role];if(!s)throw new Error('offline_diagnostic_unready');
    if(kind==='budget-candidate'){fail(role,'guard-budget');fail(role,'cookie');return;}
    if(kind==='candidate-budget'){fail(role,'cookie');fail(role,'guard-budget');return;}
    if(kind==='generic-priority'){s.binder.earlyOutcome('evaluation_failed',true);fail(role,'guard-priority');return;}
    if(kind==='context-first')s.emit('Runtime.executionContextDestroyed',{executionContextId:Number(s.pins.contextId)});
    if(kind==='context-priority')s.emit('Runtime.executionContextDestroyed',{executionContextId:Number(s.pins.contextId)});
    if(kind==='guard-budget'){
      s.emit('Network.requestWillBeSent',{requestId:'root',frameId:s.pins.frameId,loaderId:s.pins.documentId,type:'Document',request:{method:'GET',url:'https://x.com/home'}});
      for(let i=0;i<128;i++)s.emit('Network.requestWillBeSent',{requestId:'foreign-'+i,frameId:s.pins.frameId,loaderId:s.pins.documentId,type:'Image',request:{method:'GET',url:'https://foreign.invalid/asset'}});
    }
    if(['guard-priority','guard-unbound','guard-response','guard-metadata','guard-completion','context-priority'].includes(kind)){
      if(kind!=='guard-unbound')s.emit('Network.requestWillBeSent',{requestId:'asset',frameId:s.pins.frameId,loaderId:s.pins.documentId,type:'Image',request:{method:'GET',url:'https://x.com/asset'}});
      const header=kind==='guard-metadata'?['lang=en']:kind==='guard-priority'||kind==='context-priority'?'lang=en; Path=/; Priority=High':'lang=en; Path=/';
      s.emit('Network.responseReceived',{requestId:'asset',frameId:kind==='guard-response'?'foreign-frame':s.pins.frameId,loaderId:s.pins.documentId,hasExtraInfo:kind==='guard-completion',response:{url:'https://x.com/asset',status:200,headers:{'Set-Cookie':header}}});
    }
    if(['cookie','ambiguous','context-first','cookie-context','cleanup'].includes(kind)){
      s.emit('Network.requestWillBeSent',{requestId:'asset',frameId:s.pins.frameId,loaderId:s.pins.documentId,request:{method:'GET',url:'https://x.com/asset'}});
      s.emit('Network.responseReceived',{requestId:'asset',frameId:s.pins.frameId,loaderId:s.pins.documentId,hasExtraInfo:false,response:{url:'https://x.com/asset',status:200,headers:{'Set-Cookie':kind==='ambiguous'?'unknown_private=x; Path=/':'ct0=PRIVATE; Path=/'}}});
    }
    if(kind==='cookie-context')s.emit('Runtime.executionContextDestroyed',{executionContextId:Number(s.pins.contextId)});
    if(kind==='settings-order')s.settings(false,false);
  };
  const actors=Object.fromEntries(['reader','collector'].map(role=>[role,createManualStartupOwner({role,diagnostics,run:async sink=>{
    const pins=offlineStartupOwner(role),scope={owner:Number(pins.ownerId),profileId:pins.profileId,sessionId:'offline-session-'+role,targetId:pins.targetId};
    let binder;
    binder=createManualV2BoundObservation(scope,pins.frameId,async id=>{
      if(!binder.capture.consumeRead(id))throw new Error('offline_body_unavailable');
      return {schema:'aku.x.qa-direct-body.v1',category:'settings_match',settingsMatch:true};
    },()=>performance.now(),undefined,{deferCookieMutation:false,onPrivateResult:()=>{},onPrivateStartup:sink.onPrivateStartup},
    (reason,trace)=>diagnostics.record(startupFailure(role,'bound_finish',reason,false,undefined,trace),role,'bound_finish',reason));
    const emit=(method,params)=>binder.observe({method,params,sessionId:scope.sessionId},scope);
    const settings=(extra=false,withExtra=true)=>{
      emit('Network.requestWillBeSent',{requestId:'settings',frameId:pins.frameId,loaderId:pins.documentId,request:{method:'GET',url:settingsURL}});
      emit('Network.responseReceived',{requestId:'settings',frameId:pins.frameId,loaderId:pins.documentId,hasExtraInfo:extra,response:{url:settingsURL,status:200,headers:{},mimeType:'application/json',fromDiskCache:false,fromServiceWorker:false,fromPrefetchCache:false}});
      if(extra&&withExtra)emit('Network.responseReceivedExtraInfo',{requestId:'settings',statusCode:201,headers:{}});
      emit('Network.loadingFinished',{requestId:'settings',encodedDataLength:32});
    };
    states[role]={binder,pins,emit,settings};
    emit('Page.frameNavigated',{frame:{id:pins.frameId,loaderId:pins.documentId,url:'https://x.com/home'}});
    emit('Runtime.executionContextCreated',{context:{id:Number(pins.contextId),origin:'https://x.com',auxData:{isDefault:true,frameId:pins.frameId}}});
    await sink.beforeIdentityRead(binder.startupPins(Number(pins.contextId)));
    const selected=role===faultRole;
    if(selected)fail(role,fault);
    if(!selected||['extra-missing','extra-status'].includes(fault)){
      binder.acceptIdentity('1908123456789012345',projection);
      settings(selected, fault!=='extra-missing');
    }
    binder.earlyOutcome(selected?fault==='early-deadline'?'deadline':fault==='early-rejected'?'captured':'evaluation_failed':'captured',selected);
    const report=reports[role]=await binder.finish(!(selected&&fault==='cleanup'));
    requireBoundStartup(report,role,diagnostics);
  }})]));
  const port=createXManualOwnerPort({enabled:true,owner:1,diagnostics,createOwners:()=>actors,snapshot:async role=>({...offlineStartupOwner(role),sessionId:'offline-session-'+role,headless:role==='collector',windowCount:role==='reader'?1:0,bootstrapCount:role==='reader'?1:0,alive:true,lifecycleReady:true}),retire:async()=>{}});
  return Object.freeze({port,fail,diagnostics,reports});
}

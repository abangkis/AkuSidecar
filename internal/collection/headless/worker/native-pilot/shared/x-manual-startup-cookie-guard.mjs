// Explicit orchestration of independent header, root-structure and source-policy
// consumers. The legacy export below is a compatibility name, not a cookie veto.
import {HeaderSafetyGuard} from './x-manual-header-safety.mjs';
import {RootDocumentValidator} from './x-manual-root-document.mjs';
import {rootSourceVerdict} from './x-manual-root-source-policy.mjs';
const invalid=Symbol();const data=(v,k)=>{const d=v&&typeof v==='object'?Object.getOwnPropertyDescriptor(v,k):undefined;return d?Object.hasOwn(d,'value')?d.value:invalid:undefined;};
class OwnedRequestRegistry {
 #ids=new Map();#session;constructor(session){this.#session=session;}
 reserve(raw){const p=data(raw,'params'),id=data(p,'requestId'),method=data(raw,'method');if(data(raw,'sessionId')!==this.#session||typeof method!=='string'||!method.startsWith('Network.')||typeof id!=='string'||!id||id.length>512)return{first:false,known:false};const known=this.#ids.has(id),request=method==='Network.requestWillBeSent';if(known){if(request)this.#ids.set(id,false);return{first:false,known:true};}if(this.#ids.size>=16384)return{first:false,known:false,exhausted:true};const first=request&&data(p,'redirectResponse')===undefined;this.#ids.set(id,first);return{first,known:false};}
 canSelect(id){return this.#ids.get(id)===true;} clear(){this.#ids.clear();}
}
export class ManualStartupObservation {
 #registry;#headers;#root;#frame;#session;#generation=null;#facts=null;#first=null;#callback;#closed=false;
 constructor(frame,session,options={}){
  this.#frame=frame;this.#session=session;this.#registry=new OwnedRequestRegistry(session);this.#callback=options.onRejection??(()=>{});
  this.#headers=new HeaderSafetyGuard(frame,session,{...options,onRejection:(...args)=>this.#note('header',...args)});
  this.#root=new RootDocumentValidator(frame,session,{...options,onFailure:(...args)=>this.#note('root_structure',...args)});
 }
 #note(domain,reason,trace,failureReason){if(this.#first)return;this.#first=Object.freeze({domain,reason,trace,failureReason});try{this.#callback(reason,trace,failureReason);}catch{}}
 #policy(){const facts=this.#root.evidence(),verdict=rootSourceVerdict(facts);if(verdict.status==='excluded'&&verdict.condition!=='root_structure')this.#note('root_policy','guard_response_correlation',Object.freeze({guardTrigger:'response',guardProvenance:'root_scope',guardCondition:verdict.condition,guardEpoch:facts.epoch,guardDiskCache:'unknown',guardServiceWorker:'unknown',guardPrefetchCache:'unknown',...facts.cache,guardRequestFrame:facts.frameStatus,guardHeaderReason:'unknown'}),'scope_lost');}
 beginPostSeed(){this.#headers.beginPostSeed();this.#root.beginPostSeed();}
 claim(frame,session){this.#headers.claim(frame,session);return this;}
 armReload(loader){this.#root.armReload(loader);this.#headers.setEpoch(this.#root.armed,this.#root.committed);}
 canSelectRequest(id){return !this.#closed&&this.#registry.canSelect(id);}
 bindFreshnessGeneration(generation,frame,session){if(this.#generation!==null||typeof generation!=='symbol'||frame!==this.#frame||session!==this.#session)throw Error('manual_cookie_guard_invalid');this.#generation=generation;}
 freshnessEvidence(frame,session,generation){return frame===this.#frame&&session===this.#session&&generation===this.#generation?this.#facts:null;}
 rejection(){return this.#first?.reason??'unknown';}
 failureReason(){return this.#first?.failureReason??'observed_cookie_write';}
 failureDomain(){return this.#first?.domain??'unknown';}
 trace(){return this.#first?.trace??this.#headers.trace();}
 category(){return this.#headers.category();}
 observe(raw){if(this.#closed)return true;this.#headers.setEpoch(this.#root.armed,this.#root.committed);const reservation=this.#registry.reserve(raw);this.#headers.observe(raw,reservation);if(reservation.exhausted)this.#note('coverage','guard_request_budget',Object.freeze({...this.#headers.trace(),guardTrigger:'request',guardEpoch:this.#root.epoch()}),'observed_cookie_write');this.#root.observe(raw,reservation.first);this.#policy();return this.#first!==null;}
 finish(){if(this.#closed)throw Error('manual_cookie_guard_replayed');this.#closed=true;const report=this.#headers.finish();this.#facts=this.#root.finish();this.#policy();this.#registry.clear();return report;}
}
export {ManualStartupObservation as ManualStartupCookieGuard};

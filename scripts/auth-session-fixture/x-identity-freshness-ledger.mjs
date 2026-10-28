import {rootSourceVerdict,ROOT_SOURCE_POLICY} from './x-manual-root-source-policy.mjs';
// Private, owner-local authority. No IO, timers, serialized identity, or recovery.
import {ManualStartupObservation} from './x-manual-startup-cookie-guard.mjs';
import {ManualObservationDeadline} from './x-manual-observation-deadline.mjs';

export const IDENTITY_FRESHNESS_POLICY=Object.freeze({version:'manual-v2.691875d',identitySettingsMS:5000,observationMS:10000,jointMS:2000,pilotMS:60000});
const invalid=()=>new Error('identity_freshness_invalid');
const object=v=>v!==null&&typeof v==='object'&&!Array.isArray(v);
const own=(v,k)=>object(v)?Object.getOwnPropertyDescriptor(v,k)?.value:undefined;
const ref=v=>typeof v==='string'&&v.length>0&&v.length<=4096;
const account=v=>typeof v==='string'&&/^[1-9][0-9]{0,19}$/.test(v);
const finite=v=>typeof v==='number'&&Number.isFinite(v);
const publications=new WeakMap();
function record(v,keys){
  if(!object(v)||Reflect.ownKeys(v).length!==keys.length)throw invalid();
  const out={};for(const k of keys){const d=Object.getOwnPropertyDescriptor(v,k);if(!d||!Object.hasOwn(d,'value'))throw invalid();out[k]=d.value;}return out;
}
// A publication is an in-memory capability. Copying its public private-sink
// shape does not copy authority. Only the matching production owner consumes it.
export function consumeFreshnessPublication(value,role,pins){
  const receipt=publications.get(value);if(!receipt)return false;
  publications.delete(value);
  return receipt.role===role&&Object.keys(pins).every(k=>own(value,k)===own(pins,k))&&receipt.valid();
}

export class IdentityFreshnessLedger {
  #binding; #generation=Symbol(); #revision=0; #start; #last; #failure=null; #deadline;
  #document=null; #identity=null; #identityAt=null; #settingsAt=null; #settingsRequestId=null; #login=false;
  #root=null; #control=Object.freeze({state:'unknown',requestedAt:'unknown',acknowledgedAt:'unknown'}); #published=false; #sampleAt=null; #rootReceiptAt=null;
  constructor(binding,now,guard,deadline){
    const b=record(binding,['owner','profileId','sessionId','targetId','frameId','role']);
    if(!Number.isSafeInteger(b.owner)||b.owner<=0||!['profileId','sessionId','targetId','frameId'].every(k=>ref(b[k]))||!['reader','collector','unknown'].includes(b.role)||!finite(now)||now<0)throw invalid();
    if(!(guard instanceof ManualStartupObservation))throw invalid();
    if(deadline!==undefined&&!(deadline instanceof ManualObservationDeadline))throw invalid();
    this.#deadline=deadline?.expiresAt??now+IDENTITY_FRESHNESS_POLICY.observationMS;
    guard.bindFreshnessGeneration(this.#generation,b.frameId,b.sessionId);
    this.#binding=Object.freeze(b);this.#start=this.#last=now;
  }
  get revision(){return this.#revision;}
  // Generation capability never leaves the process and cannot be recreated.
  get generation(){return this.#generation;}
  #begin(revision,generation,now){
    if(revision!==this.#revision||generation!==this.#generation)throw invalid();
    if(!finite(now)||now<this.#last){this.#failure??='clock_regressed';this.#revision++;throw invalid();}
  }
  #advance(now){this.#last=now;this.#revision++;}
  #reject(reason){this.#failure??=reason;this.#revision++;throw invalid();}
  invalidate(revision,generation,reason,now){
    this.#begin(revision,generation,now);this.#failure??=['scope_lost','stale_observation','observed_cookie_write','cleanup_unverified'].includes(reason)?reason:'scope_lost';this.#advance(now);
  }
  bindDocument(revision,generation,document,now){
    const d=record(document,['documentId','contextId']);
    this.#begin(revision,generation,now);
    if(!ref(d.documentId)||!Number.isSafeInteger(d.contextId)||d.contextId<=0)this.#reject('binding_invalid');
    if(this.#document&&(this.#document.documentId!==d.documentId||this.#document.contextId!==d.contextId))this.#reject('binding_changed');
    this.#document??=Object.freeze(d);this.#advance(now);
  }
  observeCacheControl(revision,generation,input,now){
    const receipt=typeof input==='string'?{state:input,requestedAt:'unknown',acknowledgedAt:'unknown'}:record(input,['state','requestedAt','acknowledgedAt']);
    this.#begin(revision,generation,now);
    const {state,requestedAt,acknowledgedAt}=receipt;
    if(!['requested','acknowledged','failed','unavailable'].includes(state)||this.#control.state==='failed'||this.#control.state==='acknowledged'&&state!=='acknowledged'||
      ![requestedAt,acknowledgedAt].every(t=>t==='unknown'||finite(t)&&t>=0&&t<=now)||finite(requestedAt)&&finite(acknowledgedAt)&&acknowledgedAt<requestedAt)this.#reject('control_order');
    // An imported acknowledgment is an owner-provided fact, never root proof.
    this.#control=Object.freeze(receipt);this.#advance(now);
  }
  observeIdentity(revision,generation,id,now){
    this.#begin(revision,generation,now);
    if(!this.#document||!account(id))this.#reject('identity_order');
    if(this.#identity!==null&&this.#identity!==id)this.#reject('identity_changed');
    this.#identity=id;this.#identityAt??=now;this.#advance(now);
  }
  observeRoot(revision,generation,guard,now){
    this.#begin(revision,generation,now);
    if(!(guard instanceof ManualStartupObservation))this.#reject('root_source');
    const r=guard.freshnessEvidence(this.#binding.frameId,this.#binding.sessionId,this.#generation);
    if(!r||r.armed&&r.documentId!==this.#document?.documentId)this.#reject('root_binding');
    this.#root=r;this.#rootReceiptAt=now;this.#advance(now);
  }
  // One atomic evidence update, after the existing settings source validator.
  // An invalid update cannot leave a partially ready identity/settings pair.
  update(revision,generation,evidence,now){
    const e=record(evidence,['identityAt','settingsAt','settingsRequestId','login']);
    this.#begin(revision,generation,now);
    if(e.identityAt!==this.#identityAt||!finite(e.settingsAt)||typeof e.login!=='boolean'||e.settingsAt>now||e.settingsAt<this.#start||!ref(e.settingsRequestId))this.#reject('evidence_invalid');
    this.#settingsAt=e.settingsAt;this.#settingsRequestId=e.settingsRequestId;this.#login=e.login;this.#sampleAt=now;this.#advance(now);
    return this.read(now);
  }
  read(now){
    const clock=finite(now)&&now>=this.#last;
    const sampleAge=clock&&this.#sampleAt!==null?now-this.#sampleAt:'unknown';
    const identityCaptureAge=clock&&this.#identityAt!==null?now-this.#identityAt:'unknown';
    const fresh=clock&&this.#identityAt!==null&&this.#settingsAt!==null&&Math.abs(this.#settingsAt-this.#identityAt)<=IDENTITY_FRESHNESS_POLICY.identitySettingsMS&&now-Math.min(this.#identityAt,this.#settingsAt)<=IDENTITY_FRESHNESS_POLICY.identitySettingsMS;
    const rootDecision=this.#root===null?null:rootSourceVerdict(this.#root);
    const rootAccepted=rootDecision?.status==='accepted_under_policy';
    const rootPolicyVerdict=this.#root===null?'unknown':rootAccepted?'accepted_under_policy':rootDecision.status==='pending'?'pending':'rejected';
    const state=this.#failure||!clock?'invalidated':now>=this.#deadline?'expired':!this.#document||!this.#identity||this.#settingsAt===null||this.#root===null?'unknown':!rootAccepted||!this.#login?'excluded':!fresh?'stale':'ready';
    const missingProof=Object.freeze([!this.#document&&'document',!this.#identity&&'identity',this.#settingsAt===null&&'settings',this.#root===null&&'root_verdict',this.#root&&!rootAccepted&&'root_policy',this.#settingsAt!==null&&!this.#login&&'settings_proof',this.#settingsAt!==null&&!fresh&&'age','server_freshness'].filter(Boolean));
    return Object.freeze({policyVersion:IDENTITY_FRESHNESS_POLICY.version,rootPolicyVersion:ROOT_SOURCE_POLICY.version,revision:this.#revision,state,invalidationReason:this.#failure??'unknown',bindingState:!this.#document?'provisional':this.#root?.armed&&rootAccepted?'root_bound':'document_bound',rootPolicyVerdict,readyUnderPolicy:state==='ready',sourceFreshnessProven:'unknown',sampleAge,identityCaptureAge,withinIdentitySettingsWindow:fresh,cacheControl:this.#control,rootCache:this.#root?.cache??null,rootObservedAt:this.#root?.observedAt??'unknown',rootReceiptAt:this.#rootReceiptAt??'unknown',missingProof});
  }
  readRootPolicyVerdict(now){const s=this.read(now);return Object.freeze({policyVersion:s.rootPolicyVersion,verdict:s.rootPolicyVerdict,sourceFreshnessProven:s.sourceFreshnessProven});}
  publish(value,now,readNow){
    if(this.#published||!this.read(now).readyUnderPolicy||typeof readNow!=='function')throw invalid();
    const d=this.#document,b=this.#binding;
    if(own(value,'accountId')!==this.#identity||own(value,'identityAt')!==this.#identityAt||own(value,'settingsAt')!==this.#settingsAt||own(value,'settingsRequestId')!==this.#settingsRequestId||own(value,'ownerId')!==String(b.owner)||own(value,'profileId')!==b.profileId||own(value,'targetId')!==b.targetId||own(value,'frameId')!==b.frameId||own(value,'documentId')!==d.documentId||own(value,'contextId')!==String(d.contextId)||!Object.isFrozen(value))throw invalid();
    this.#published=true;
    publications.set(value,{role:b.role,valid:()=>this.read(readNow()).readyUnderPolicy});return value;
  }
}

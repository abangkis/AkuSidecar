import test from 'node:test';
import assert from 'node:assert/strict';
import {IdentityFreshnessLedger,IDENTITY_FRESHNESS_POLICY,consumeFreshnessPublication} from './x-identity-freshness-ledger.mjs';
import {ManualStartupCookieGuard} from './x-manual-startup-cookie-guard.mjs';
import {createManualV2BoundObservation} from './x-qa-bound-observation.mjs';
import {inspectBootstrapCurrentSelf} from './x-bootstrap-current-self.mjs';
import {createManualStartupOwner} from './x-manual-startup-owner.mjs';
import {QA_AB_URL} from './x-qa-direct-ab.mjs';
const binding={owner:1,profileId:'private-profile',sessionId:'private-session',targetId:'private-target',frameId:'frame',role:'reader'};
const pins={ownerId:'1',profileId:binding.profileId,targetId:binding.targetId,frameId:'frame',documentId:'new',contextId:'7'};
function fixture(role='reader'){
 const guard=new ManualStartupCookieGuard('frame',binding.sessionId),ledger=new IdentityFreshnessLedger({...binding,role},0,guard);
 const call=(method,...args)=>ledger[method](ledger.revision,ledger.generation,...args);
 const ready=()=>{call('bindDocument',{documentId:'new',contextId:7},1);call('observeIdentity','12345',2);guard.finish();call('observeRoot',guard,3);call('update',{identityAt:2,settingsAt:3,settingsRequestId:'settings',login:true},3);};
 const publication=()=>ledger.publish(Object.freeze({...pins,accountId:'12345',identityAt:2,settingsAt:3,settingsRequestId:'settings',qualification:'api_v1.cookie-auth-rejection.2026-10-20'}),3,()=>3);
 return{guard,ledger,call,ready,publication};
}
test('immutable generation, root authority and atomic evidence are required before ready',()=>{
 const s=fixture();assert.equal(s.ledger.read(0).state,'unknown');s.call('observeCacheControl','acknowledged',0);assert.equal(s.ledger.read(0).rootPolicyVerdict,'unknown');
 s.call('bindDocument',{documentId:'new',contextId:7},1);s.call('observeIdentity','12345',2);assert.equal(s.ledger.read(2).bindingState,'document_bound');
 s.guard.finish();s.call('observeRoot',s.guard,3);assert.equal(s.ledger.read(3).bindingState,'document_bound');assert.equal(s.ledger.read(3).state,'unknown');
 const before=s.ledger.read(3);let reads=0;const bad=Object.defineProperty({settingsAt:3,settingsRequestId:'settings',login:true},'identityAt',{get(){reads++;return 2;}});
 assert.throws(()=>s.call('update',bad,4));assert.equal(reads,0);assert.deepEqual(s.ledger.read(3),before);
 s.call('update',{identityAt:2,settingsAt:3,settingsRequestId:'settings',login:true},3);assert.equal(s.ledger.read(3).readyUnderPolicy,true);assert.equal(s.ledger.read(3).sourceFreshnessProven,'unknown');assert.ok(Object.isFrozen(s.ledger.read(3)));
});
test('stale revisions, foreign generations, guard reuse and bindings cannot create authority',()=>{
 const a=fixture(),b=fixture();const r=a.ledger.revision;
 a.call('bindDocument',{documentId:'new',contextId:7},1);
 assert.throws(()=>a.ledger.observeIdentity(r,a.ledger.generation,'12345',2));assert.throws(()=>a.ledger.observeIdentity(a.ledger.revision,b.ledger.generation,'12345',2));assert.equal(a.ledger.read(2).state,'unknown');
 assert.throws(()=>new IdentityFreshnessLedger({...binding,role:'collector'},1,a.guard));
 b.guard.finish();assert.throws(()=>a.call('observeRoot',b.guard,2));assert.equal(a.ledger.read(3).state,'invalidated');
 const c=fixture();c.call('bindDocument',{documentId:'new',contextId:7},1);assert.throws(()=>c.call('bindDocument',{documentId:'other',contextId:7},2));assert.equal(c.ledger.read(3).state,'invalidated');
});
test('reads never freshen evidence; repeated identity retains first anchor; expiry and mutation clock reversal stay closed',()=>{
 const s=fixture();s.ready();const revision=s.ledger.revision;
 assert.equal(s.ledger.read(5002).readyUnderPolicy,true);assert.equal(s.ledger.read(5003).state,'stale');assert.equal(s.ledger.read(10001).state,'expired');assert.equal(s.ledger.revision,revision);
 s.call('observeIdentity','12345',5003);assert.equal(s.ledger.read(5003).identityCaptureAge,5001);assert.equal(s.ledger.read(5003).state,'stale');
 assert.throws(()=>s.call('observeCacheControl','requested',5002));assert.equal(s.ledger.read(5004).state,'invalidated');
 s.call('observeCacheControl','acknowledged',5005);assert.equal(s.ledger.read(5005).state,'invalidated');
 assert.deepEqual([IDENTITY_FRESHNESS_POLICY.identitySettingsMS,IDENTITY_FRESHNESS_POLICY.jointMS,IDENTITY_FRESHNESS_POLICY.pilotMS],[5000,2000,60000]);
});
test('out-of-order identity and inconsistent atomic updates never produce partial ready state',()=>{
 const a=fixture();assert.throws(()=>a.call('observeIdentity','12345',1));assert.equal(a.ledger.read(2).state,'invalidated');
 const b=fixture();b.ready();assert.throws(()=>b.call('update',{identityAt:99,settingsAt:4,settingsRequestId:'settings',login:true},4));assert.equal(b.ledger.read(4).state,'invalidated');
 const c=fixture();c.ready();assert.throws(()=>c.call('observeIdentity','67890',4));assert.equal(c.ledger.read(5).state,'invalidated');
});
test('private publications cannot be copied, replayed, exchanged between roles or imported to another generation',()=>{
 const a=fixture();a.ready();const p=a.publication();assert.equal(consumeFreshnessPublication({...p},'reader',pins),false);assert.equal(consumeFreshnessPublication(p,'reader',pins),true);assert.equal(consumeFreshnessPublication(p,'reader',pins),false);
 const b=fixture();b.ready();assert.equal(consumeFreshnessPublication(b.publication(),'collector',pins),false);
 const c=fixture();c.ready();const value=c.publication();c.call('invalidate','scope_lost',4);assert.equal(consumeFreshnessPublication(value,'reader',pins),false);
 assert.equal(JSON.stringify(c.ledger.read(4)).includes('12345'),false);assert.equal(JSON.stringify(c.ledger.read(4)).includes('private'),false);
});
export async function binderCase(factory=createManualV2BoundObservation,{rootCache={},settingsCache={fromDiskCache:false,fromServiceWorker:false,fromPrefetchCache:false},extraFirst=false,lateExtra=false,delay=0}={}){
 let time=0,b,publication=null;const scope={owner:1,profileId:binding.profileId,sessionId:binding.sessionId,targetId:binding.targetId};
 b=factory(scope,'frame',async id=>{assert.equal(b.capture.consumeRead(id),true);time+=delay;return{schema:'aku.x.qa-direct-body.v1',category:'settings_match',settingsMatch:true};},()=>time,undefined,{deferCookieMutation:false,onPrivateResult:()=>{},onPrivateStartup:v=>{publication=v;}});
 const emit=(method,params)=>b.observe({method,params,sessionId:scope.sessionId},scope);
 b.armManualReload('old');emit('Network.requestWillBeSent',{requestId:'root',frameId:'frame',loaderId:'new',type:'Document',request:{method:'GET',url:'https://x.com/home'}});
 const extra=()=>emit('Network.responseReceivedExtraInfo',{requestId:'root',statusCode:200,headers:{}});
 if(extraFirst)extra();emit('Network.responseReceived',{requestId:'root',frameId:'frame',loaderId:'new',hasExtraInfo:true,response:{url:'https://x.com/home',status:200,headers:{},...rootCache}});if(!extraFirst&&!lateExtra)extra();
 emit('Page.frameNavigated',{frame:{id:'frame',loaderId:'new',url:'https://x.com/home'}});emit('Runtime.executionContextCreated',{context:{id:7,origin:'https://x.com',auxData:{isDefault:true,frameId:'frame'}}});if(lateExtra)extra();
 const page={location:{origin:'https://x.com',pathname:'/home'},__INITIAL_STATE__:{session:{user_id:'12345'},entities:{users:{entities:{'12345':{id_str:'12345'}}}}}};
 time=1;b.acceptIdentity('12345',inspectBootstrapCurrentSelf(page));
 emit('Network.requestWillBeSent',{requestId:'settings',frameId:'frame',loaderId:'new',request:{method:'GET',url:QA_AB_URL}});
 emit('Network.responseReceived',{requestId:'settings',frameId:'frame',loaderId:'new',hasExtraInfo:false,response:{url:QA_AB_URL,status:200,headers:{},mimeType:'application/json',...settingsCache}});time=2;emit('Network.loadingFinished',{requestId:'settings',encodedDataLength:32});
 const report=await b.finish(true);return{report,published:publication!==null};
}
export const binderCases=[{}, {extraFirst:true},{lateExtra:true},{rootCache:{fromDiskCache:undefined}},{rootCache:{fromDiskCache:false,fromServiceWorker:false,fromPrefetchCache:false}},{rootCache:{fromDiskCache:true}},{rootCache:{fromServiceWorker:true}},{rootCache:{fromPrefetchCache:true}},{settingsCache:{}},{settingsCache:{fromDiskCache:false,fromServiceWorker:false}},{settingsCache:{fromDiskCache:false,fromServiceWorker:true,fromPrefetchCache:false}},{delay:5001}];
test('actual manual binder preserves root ordering/cache policy, strict settings and post-read age',async()=>{
 for(let i=0;i<binderCases.length;i++){const r=await binderCase(undefined,binderCases[i]);assert.equal(r.report.diagnostic==='bound',i<5,`case ${i}`);assert.equal(r.published,i<5);}
});
test('production owner gate rejects a plain sample even with matching private pins',async()=>{
 const s=fixture();s.ready();const sample=Object.freeze({...pins,accountId:'12345',identityAt:2,settingsAt:3,settingsRequestId:'settings',qualification:'api_v1.cookie-auth-rejection.2026-10-20'});
 const owner=createManualStartupOwner({role:'reader',requireFreshness:true,now:()=>3,wallNow:()=>100,run:async sink=>{await sink.beforeIdentityRead(pins);sink.onPrivateStartup(sample);}});
 const p=await owner.prepare();await assert.rejects(owner.acquire(p.handle,{challenge:'b'.repeat(48),role:'reader',...p.pins}));await owner.abort(p.handle);
});

test('production owner consumes genuine ledger publications for both roles without new wire fields',async()=>{
 for(const role of ['reader','collector']){
  let now=0;const s=fixture(role);
  const owner=createManualStartupOwner({role,requireFreshness:true,now:()=>now,wallNow:()=>100,run:async sink=>{await sink.beforeIdentityRead(pins);s.ready();now=3;sink.onPrivateStartup(s.publication());}});
  const p=await owner.prepare();const bytes=await owner.acquire(p.handle,{challenge:'b'.repeat(48),role,...p.pins});
  const result=JSON.parse(bytes.toString());bytes.fill(0);assert.equal(result.role,role);assert.equal(Object.hasOwn(result,'generation'),false);assert.equal(Object.hasOwn(result,'policyVersion'),false);await owner.abort(p.handle);
 }
});
test('cache-control receipts retain actual times without refreshing the evidence snapshot',()=>{
 const s=fixture();s.ready();s.call('observeCacheControl',{state:'acknowledged',requestedAt:0,acknowledgedAt:1},4);
 const view=s.ledger.read(5);assert.equal(view.sampleAge,2);assert.equal(view.identityCaptureAge,3);assert.deepEqual(view.cacheControl,{state:'acknowledged',requestedAt:0,acknowledgedAt:1});assert.ok(view.missingProof.includes('server_freshness'));assert.equal(s.ledger.readRootPolicyVerdict(5).sourceFreshnessProven,'unknown');assert.equal(s.ledger.readRootPolicyVerdict(5).policyVersion,'manual-root-source.v1');
 const r=fixture();r.call('observeCacheControl','unavailable',0);assert.equal(r.ledger.read(0).cacheControl.state,'unavailable');
 assert.throws(()=>new IdentityFreshnessLedger(binding,-1,new ManualStartupCookieGuard('frame',binding.sessionId)));
});
test('armed root must correlate its complete document before exposing RootBound',()=>{
 const s=fixture();const emit=(method,params)=>s.guard.observe({method,params,sessionId:binding.sessionId});s.guard.armReload('old');
 emit('Network.requestWillBeSent',{requestId:'root',frameId:'frame',loaderId:'new',type:'Document',request:{method:'GET',url:'https://x.com/home'}});
 emit('Network.responseReceived',{requestId:'root',frameId:'frame',loaderId:'new',hasExtraInfo:false,response:{url:'https://x.com/home',status:200,headers:{}}});emit('Page.frameNavigated',{frame:{id:'frame',loaderId:'new',url:'https://x.com/home'}});
 s.call('bindDocument',{documentId:'new',contextId:7},1);assert.equal(s.ledger.read(1).bindingState,'document_bound');s.guard.finish();s.call('observeRoot',s.guard,2);assert.equal(s.ledger.read(2).bindingState,'root_bound');assert.equal(s.ledger.read(2).rootCache.guardDiskCache,'absent');assert.equal(s.ledger.read(2).sourceFreshnessProven,'unknown');
});

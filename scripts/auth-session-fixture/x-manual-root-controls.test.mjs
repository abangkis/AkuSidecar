import test from 'node:test';
import assert from 'node:assert/strict';
import {registerFreshStartupSession,startupRootControls,allowsStartupControl,mustRetireStartupOwner} from './x-manual-root-controls.mjs';
import {ROOT_SOURCE_POLICY,rootSourceVerdict} from './x-manual-root-source-policy.mjs';
import {RootDocumentValidator} from './x-manual-root-document.mjs';
import {ManualStartupObservation} from './x-manual-startup-cookie-guard.mjs';
import {startupDiagnostic} from './x-manual-startup-diagnostic.mjs';

test('single policy excludes source classes without making them cookie attempts',()=>{
 for(const value of [undefined,false,true,0,'false']){
  const g=new ManualStartupObservation('f','s'),root=new RootDocumentValidator('f','s');g.armReload('old');root.armReload('old');
  for(const [method,params]of [
   ['Network.requestWillBeSent',{requestId:'r',frameId:'f',loaderId:'new',type:'Document',request:{method:'GET',url:'https://x.com/home'}}],
   ['Network.responseReceived',{requestId:'r',frameId:'f',loaderId:'new',hasExtraInfo:false,response:{url:'https://x.com/home',status:200,headers:{},fromServiceWorker:value}}],
   ['Page.frameNavigated',{frame:{id:'f',loaderId:'new',url:'https://x.com/home'}}],
  ]){const raw={method,params,sessionId:'s'};g.observe(raw);root.observe(raw,true);}
  const facts=root.finish(),report=g.finish(),accepted=value===undefined||value===false;
  assert.equal(facts.structurallyValid,true);assert.equal(rootSourceVerdict(facts).status,accepted?'accepted_under_policy':'excluded');
  assert.equal(g.failureDomain(),accepted?'unknown':'root_policy');assert.equal(report.ambiguousAttempts,0);assert.equal(report.candidateAttempts,0);
 }
 assert.equal(ROOT_SOURCE_POLICY.version,'manual-root-source.v1');assert.ok(Object.isFrozen(ROOT_SOURCE_POLICY.controls));
});

test('owned controls require exact session, one attempt, both restore acknowledgments and no effects on read',async()=>{
 for(const fault of ['none','apply-cache','apply-worker','restore-worker','restore-cache','drift']){
  const owner={sessionId:'s',targetId:'t',owner:1};registerFreshStartupSession(owner,'s','reader');const calls=[];
  assert.throws(()=>startupRootControls(owner,()=>{},'collector'));const controls=startupRootControls(owner,async(method,params,session)=>{
   assert.equal(allowsStartupControl(owner,method,params,session),true);assert.equal(allowsStartupControl(owner,method,params,'foreign'),false);
   calls.push([method,params]);const stage=(params.cacheDisabled??params.bypass)?'apply':'restore',kind=method.endsWith('ServiceWorker')?'worker':'cache';
   if(fault===stage+'-'+kind)return {acknowledged:false};return {acknowledged:true};
  },'reader');
  assert.equal(controls.readyToRelease(),false);assert.equal(calls.length,0);
  if(fault==='drift')owner.sessionId='other';
  let error;try{await controls.apply();await controls.restore();}catch(e){error=e;}
  assert.equal(controls.readyToRelease(),fault==='none');assert.equal(mustRetireStartupOwner(owner),fault!=='none');
  if(error)assert.equal(startupDiagnostic(error).trace.firstCause,'root_control');
  else assert.deepEqual(calls,[['Network.setCacheDisabled',{cacheDisabled:true}],['Network.setBypassServiceWorker',{bypass:true}],['Network.setBypassServiceWorker',{bypass:false}],['Network.setCacheDisabled',{cacheDisabled:false}]]);
  assert.equal(allowsStartupControl(owner,'Network.setBypassServiceWorker',{bypass:true},'s'),false);
  registerFreshStartupSession(owner,'s','reader');assert.throws(()=>startupRootControls(owner,()=>{},'reader'));await assert.rejects(controls.apply());
 }
 assert.throws(()=>startupRootControls({},()=>{},'reader'));
});

test('Page lifecycle metadata never becomes a header inspection failure',()=>{const g=new ManualStartupObservation('f','s');g.armReload('old');for(const method of ['Page.frameNavigated','Network.loadingFinished','Network.loadingFailed','Network.requestServedFromCache'])g.observe({method,sessionId:'s',params:null});const report=g.finish();assert.equal(report.ambiguousAttempts,0);assert.equal(g.failureDomain(),'root_structure');assert.equal(g.trace().guardCondition,'root_missing');});

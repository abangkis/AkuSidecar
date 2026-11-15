import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFile} from 'node:fs/promises';
import {toObservation, projectSourceReadiness} from '../observation.mjs';
import {emptyCaptureDiagnostics, NATIVE_CAPTURE_CAPABILITIES} from '../capture.mjs';

const bridge = new URL('../../../../../../AkuBridge/', import.meta.url);
const [registry, adapter, primitives, windowed, headless] = await Promise.all([
  readFile(new URL('source-adapter-runtime.js', bridge), 'utf8'),
  readFile(new URL('adapters/instagram-adapter.js', bridge), 'utf8'),
  readFile(new URL('capture-primitives.js', bridge), 'utf8'),
  readFile(new URL('content-script.js', bridge), 'utf8'),
  readFile(new URL('../vendor/adapter-extract.js', import.meta.url), 'utf8'),
]);
const nativeCapabilitiesBefore=JSON.stringify(NATIVE_CAPTURE_CAPABILITIES);

function fixture(spec, effects) {
  const forbid = () => { effects.mutations++; throw Error('readiness must remain passive'); };
  const node = (tagName, text = '') => ({tagName, innerText:text, textContent:text, childNodes:[], style:{},
    getAttribute:()=>null, querySelectorAll:()=>[], querySelector:()=>null, closest:()=>null,
    getBoundingClientRect:()=>({width:600,height:300,left:0,right:600,top:10,bottom:310}),
    click:forbid, scrollIntoView:forbid, scrollTo:forbid});
  const body = node('BODY', spec.empty ? 'You are all caught up' : spec.mismatch ? 'Unrecognized feed markup' : '');
  const main = node('MAIN'); main.parentElement=body;
  const roots = [];
  if (spec.post) {
    const article=node('ARTICLE','fixture_author\nSame Instagram fixture caption.');
    const profile=node('A','fixture_author'); profile.href='https://www.instagram.com/fixture_author/';
    const permalink=node('A'); permalink.href='https://www.instagram.com/p/Fixture123/';
    const caption=node('SPAN','Same Instagram fixture caption.');
    const anchors=spec.native ? [permalink,profile] : [profile];
    article.querySelectorAll=s=>s==='a[href]' ? anchors : s==='a[href^="/"]' ? [profile]
      : s==='span[dir="auto"]' ? [caption] : [];
    article.contains=n=>n===article || anchors.includes(n) || n===caption;
    article.parentElement=main;
    for(const child of [...anchors,caption])child.parentElement=article;
    if(spec.offscreen)article.getBoundingClientRect=()=>({width:600,height:300,left:0,right:600,top:1200,bottom:1500});
    roots.push(article);
  }
  main.querySelectorAll=s=>s==='a[href]' ? roots.flatMap(r=>r.querySelectorAll(s)) : [];
  return {document:{title:'Offline Instagram readiness fixture', visibilityState:'visible',
    readyState:spec.ready ?? 'complete', body, scrollingElement:{scrollHeight:1400},
    querySelector:s=>s==='main' ? spec.feed===false ? null : main
      : s.includes('input[name=') ? spec.login ? node('INPUT') : null
      : s==='[aria-busy="true"], .artdeco-loader, [data-test-id*="loading" i]' ? spec.loading ? node('DIV') : null : null,
    querySelectorAll:s=>s==='main article' || s==='main article, [role="dialog"] article' ? roots : []}, forbid};
}

function runtime(spec, mode) {
  const effects={mutations:0,network:0,recoveries:0};
  const {document,forbid}=fixture(spec,effects);
  const context=vm.createContext({URL,Element:Object,setTimeout,clearTimeout,document,
    location:new URL('https://www.instagram.com/'),innerHeight:900,innerWidth:1280,scrollX:0,scrollY:0,
    getComputedStyle:node=>({display:'block',visibility:'visible',opacity:'1',...node.style}),
    scrollTo:forbid,scrollBy:forbid,fetch:()=>{effects.network++;throw Error('offline fixture');},
    chrome:{runtime:{onMessage:{addListener(){}}}},
    AkuBoundedCapturePolicy:{},AkuCaptureQualityPolicy:{},AkuSourceFreshnessRuntime:{},
    AkuMediaAcquisitionEngine:{},AkuMediaPostProcessor:{createDeferredInbox:()=>({})}});
  context.window=context;
  for(const source of [primitives,registry,adapter])vm.runInContext(source,context);
  const facts=[];
  const actual=context.AkuSourceAdapters;
  const actualAdapter=actual.get('instagram');
  // Preserve all real adapter methods, observing only the normalized assessment
  // context and trapping recovery if the read-only composition ever invokes it.
  const trappedAdapter={...actualAdapter,recoverReadiness:()=>{effects.recoveries++;throw Error('recovery forbidden');}};
  context.AkuSourceAdapters={...actual,get:source=>source==='instagram' ? trappedAdapter : actual.get(source),
    assessReadiness:(source,value)=>{facts.push(JSON.parse(JSON.stringify(value)));return actual.assessReadiness(source,value);}};
  vm.runInContext(mode==='windowed' ? windowed : headless,context);
  return {context,facts,effects};
}

const cases=[
  {name:'structural post without permalink',post:true,state:'feed_hydrating',diagnosis:'post_permalink_unhydrated',action:'reset_feed_top'},
  {name:'offscreen native post',post:true,native:true,offscreen:true,state:'feed_not_visible',diagnosis:'post_outside_capture_viewport',action:'align_first_candidate'},
  {name:'loading interactive document',loading:true,ready:'interactive',state:'loading',diagnosis:'navigation_incomplete'},
  {name:'loading complete document',loading:true,state:'loading',diagnosis:'navigation_incomplete'},
  {name:'loading structural post',loading:true,post:true,state:'loading',diagnosis:'navigation_incomplete'},
  {name:'settled visible native post',post:true,native:true,state:'feed_ready',diagnosis:'readiness_observed',posts:1},
  {name:'selector mismatch without visual proof',mismatch:true,state:'selector_mismatch',diagnosis:'feed_state_unverified'},
  {name:'empty feed remains unverified',empty:true,state:'selector_mismatch',diagnosis:'feed_state_unverified'},
  {name:'incomplete page shell',feed:false,ready:'interactive',state:'loading',diagnosis:'navigation_incomplete'},
  {name:'complete page shell',feed:false,state:'page_shell',diagnosis:'readiness_observed'},
  {name:'login without feed',feed:false,login:true,state:'login_required',diagnosis:'login_required'},
  {name:'login with feed shell',login:true,state:'login_required',diagnosis:'login_required'},
  {name:'login with visible post and loader',login:true,post:true,native:true,loading:true,state:'login_required',diagnosis:'login_required',posts:1},
];

for(const spec of cases)test(`real Instagram readiness parity: ${spec.name}`,async()=>{
  const w=runtime(spec,'windowed'),h=runtime(spec,'headless');
  let response;
  w.context.__akuBrowserSourceBridgeMessageHandler({type:'AKU_BROWSER_PROBE_SOURCE_READY',source:'instagram'},null,r=>response=r);
  const snapshot=await h.context.XHeadlessPoC.collect();
  assert.equal(response.ok,true);
  assert.deepEqual(w.facts,h.facts,'both real paths must supply equivalent adapter facts');
  assert.equal(h.facts.length,1);
  const result=JSON.parse(JSON.stringify(snapshot.sourceReadinessAssessment));
  assert.deepEqual(result,JSON.parse(JSON.stringify({state:response.readiness.state,diagnosis:response.readiness.diagnosis,recovery:response.readiness.recoveryHint})));
  assert.equal(result.state,spec.state);
  assert.equal(result.diagnosis,spec.diagnosis);
  assert.equal(result.recovery?.inPageAction,spec.action);
  assert.equal(snapshot.sourceReadiness,spec.state);
  assert.equal(snapshot.posts.length,spec.posts ?? 0);
  assert.equal(snapshot.rejectionReasons.adapter_error,undefined);
  const diagnostic=emptyCaptureDiagnostics([snapshot]);
  assert.deepEqual(diagnostic.samples[0].sourceReadinessAssessment,result);
  const observation=toObservation({source:'instagram',requestedUrl:'https://www.instagram.com/',snapshots:[snapshot],capturedAt:'2026-11-15T00:00:00Z'});
  assert.deepEqual(observation.coverage.sourceReadinessAssessment,result);
  const {sourceReadinessAssessment:ignored,...withoutAssessment}=snapshot;
  const baseline=toObservation({source:'instagram',requestedUrl:'https://www.instagram.com/',snapshots:[withoutAssessment],capturedAt:'2026-11-15T00:00:00Z'});
  assert.deepEqual({...observation.coverage,sourceReadinessAssessment:undefined},
    {...baseline.coverage,sourceReadinessAssessment:undefined},'assessment must not change existing coverage or proof');
  assert.equal(observation.coverage.authenticatedUiObserved,false,'readiness is not account proof');
  assert.equal(JSON.stringify(NATIVE_CAPTURE_CAPABILITIES),nativeCapabilitiesBefore);
  for(const execution of [w,h])assert.deepEqual(execution.effects,{mutations:0,network:0,recoveries:0});
});

test('Instagram assessment preserves availability and requires explicit visual evidence for settled mismatch',()=>{
  const {context}=runtime({},'headless');
  const assess=facts=>JSON.parse(JSON.stringify(context.AkuSourceAdapters.assessReadiness('instagram',{
    state:'selector_mismatch',feedRootPresent:true,documentReadyState:'complete',selectorCandidateCount:0,
    visibleSelectorCandidateCount:0,structuralCandidateCount:0,visualHydrationReady:true,...facts})));
  assert.deepEqual(assess({state:'source_unavailable'}),{state:'source_unavailable',diagnosis:'source_unavailable',recovery:null});
  assert.equal(assess({visualHydrationRequired:false}).diagnosis,'feed_state_unverified');
  assert.deepEqual(assess({visualHydrationRequired:true}),{state:'selector_mismatch',diagnosis:'dom_contract_mismatch',recovery:null});
});

test('readiness diagnostic projection excludes unknown fields, mismatched state and invalid recovery without invoking getters',()=>{
  const valid={source:'instagram',sourceReadiness:'feed_hydrating',sourceReadinessAssessment:{state:'feed_hydrating',diagnosis:'post_permalink_unhydrated',
    recovery:{action:'recreate_managed_surface',inPageAction:'reset_feed_top',reason:'post_permalink_unhydrated',maxAttempts:1}}};
  assert.deepEqual(projectSourceReadiness(valid),{sourceReadinessState:'feed_hydrating',sourceReadinessAssessment:valid.sourceReadinessAssessment});
  for(const assessment of [
    {...valid.sourceReadinessAssessment,state:'feed_ready'},
    {...valid.sourceReadinessAssessment,diagnosis:'PRIVATE PAGE TEXT'},
    {...valid.sourceReadinessAssessment,recovery:{...valid.sourceReadinessAssessment.recovery,maxAttempts:2}},
    {...valid.sourceReadinessAssessment,recovery:{...valid.sourceReadinessAssessment.recovery,inPageAction:'align_first_candidate'}},
  ])assert.deepEqual(projectSourceReadiness({...valid,sourceReadinessAssessment:assessment}),{sourceReadinessState:'feed_hydrating'});
  const privateHint={...valid.sourceReadinessAssessment.recovery,privateText:'must not escape'};
  assert.equal(JSON.stringify(projectSourceReadiness({...valid,sourceReadinessAssessment:{...valid.sourceReadinessAssessment,recovery:privateHint}})).includes('privateText'),false);
  for(const source of ['x','linkedin',undefined])assert.deepEqual(projectSourceReadiness({...valid,source}),{sourceReadinessState:'feed_hydrating'});
  const getter={...valid};
  Object.defineProperty(getter,'sourceReadinessAssessment',{get(){throw Error('getter invoked');}});
  assert.deepEqual(projectSourceReadiness(getter),{sourceReadinessState:'feed_hydrating'});
});

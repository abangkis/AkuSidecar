import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {recoverHeadlessFreshness} from '../headless-freshness.mjs';
import {sourceFreshnessContractFor} from '../source-freshness-contract.mjs';

const feedUrl='https://www.linkedin.com/feed/?trk=home';
const assets=await Promise.all(['capture-primitives.js','source-adapter-runtime.js','adapters/linkedin-adapter.js'].map(async name=>({
  relative:`AkuBridge/${name}`,
  content:await readFile(new URL(`../../../../../../AkuBridge/${name}`,import.meta.url),'utf8'),
})));
const contract=sourceFreshnessContractFor(assets,'linkedin');
const oldId='linkedin:activity:7412345678901234567';
const newId='linkedin:ugcpost:7412345678901234568';
const shareId='linkedin:share:7412345678901234569';

function post(id,text,extra={}){
  const [, ,kind,number]=/^(linkedin:(activity|ugcpost|share)):(\d+)$/.exec(id)||[];
  const urnKind=kind==='ugcpost'?'ugcPost':kind;
  return {id,permalink:`https://www.linkedin.com/feed/update/urn:li:${urnKind}:${number}/`,text,...extra};
}

function snapshot(posts,url=feedUrl){return {url,posts};}

function makePage({url=feedUrl,pending=true}={}){
  const state={url,probeCalls:0,activationCalls:0,evaluations:[]};
  const page={async evaluate(expression){
    state.evaluations.push(expression);
    if(expression==='location.href')return state.url;
    if(expression.includes('runtime.probe("linkedin")')){
      state.probeCalls++;
      return {pendingContentDetected:pending,pendingContentLabel:pending?'Show new posts':null,strategy:'pending_button'};
    }
    if(expression.includes('runtime.activatePending("linkedin", ')){
      state.activationCalls++;
      return {available:true,result:{activated:true,label:'Show new posts'}};
    }
    return undefined;
  }};
  return {page,state};
}

function options(overrides={}){
  return {source:'linkedin',pageUrl:feedUrl,acquisitionRound:1,explicitPageUrl:false,
    pendingContentPolicy:'reveal_if_present',sourceFreshnessPolicy:'wake_and_reveal',sameTabMutationAllowed:true,...overrides};
}

function invoke({page,initial=snapshot([post(oldId,'Original post')]),fresh,collectSnapshot,opts=options(),
  backend='headless_worker',captureMs=1600}={}){
  return recoverHeadlessFreshness({page,snapshot:initial,options:opts,deadlineAt:Date.now()+captureMs,
    collectSnapshot:collectSnapshot|| (async()=>fresh),backend,source:'linkedin',contract});
}

test('LinkedIn headless reveal accepts a new primary post and retains its snapshot',async()=>{
  assert.equal(contract.enabled,true);
  const {page,state}=makePage();
  const fresh=snapshot([post(oldId,'Original post'),post(newId,'New post'),post(shareId,'Another primary')]);
  const result=await invoke({page,fresh});
  assert.equal(result.freshness.status,'verified');
  assert.equal(result.freshness.addedPrimaryCount,2);
  assert.equal(result.freshness.changedPrimaryCount,0);
  assert.equal(result.freshness.verifiedPrimaryCount,2);
  assert.equal(result.snapshot,fresh);
  assert.equal(state.probeCalls,1);
  assert.equal(state.activationCalls,1);
});

test('LinkedIn headless reveal accepts a same-identity content change',async()=>{
  const {page}=makePage();
  const initial=snapshot([post(oldId,'A short post\nwith stable words')]);
  const fresh=snapshot([post(oldId,'A short post with\nstable words and new text')]);
  const result=await invoke({page,initial,fresh});
  assert.equal(result.freshness.status,'verified');
  assert.equal(result.freshness.addedPrimaryCount,0);
  assert.equal(result.freshness.changedPrimaryCount,1);
  assert.equal(result.snapshot,fresh);
});

test('LinkedIn route and mismatched primary identities cannot verify freshness',async t=>{
  await t.test('non-feed requested route is rejected before page evaluation',async()=>{
    const {page,state}=makePage();
    const result=await invoke({page,opts:options({pageUrl:'https://www.linkedin.com/in/person/'})});
    assert.equal(result.freshness.status,'not_verified');
    assert.equal(result.freshness.limitation,'source_contract_unsupported');
    assert.equal(state.evaluations.length,0);
  });
  await t.test('post captured after navigation away from feed is rejected',async()=>{
    const {page}=makePage();
    await assert.rejects(invoke({page,fresh:snapshot([post(newId,'New')],'https://www.linkedin.com/login')}),
      {code:'source_route_changed'});
  });
  await t.test('ID and permalink disagreement cannot count as a new or changed post',async()=>{
    const {page}=makePage();
    const initial=snapshot([post(oldId,'Original post')]);
    const bad=post(oldId,'Changed text');
    bad.permalink=`https://www.linkedin.com/feed/update/urn:li:share:7412345678901234567/`;
    const result=await invoke({page,initial,fresh:snapshot([bad])});
    assert.equal(result.freshness.status,'reveal_unverified');
    assert.equal(result.freshness.limitation,'no_primary_content_change');
    assert.equal(result.freshness.verifiedPrimaryCount,0);
    assert.equal(result.snapshot,initial);
  });
  await t.test('conflicting duplicate primary identity is discarded as ambiguous',async()=>{
    const {page}=makePage();
    const initial=snapshot([post(oldId,'Original post')]);
    const fresh=snapshot([post(oldId,'Conflicting copy one'),post(oldId,'Conflicting copy two')]);
    const result=await invoke({page,initial,fresh});
    assert.equal(result.freshness.status,'reveal_unverified');
    assert.equal(result.freshness.limitation,'no_primary_content_change');
    assert.equal(result.freshness.verifiedPrimaryCount,0);
    assert.equal(result.snapshot,initial);
  });
});

test('LinkedIn unchanged, reordered and engagement-only snapshots do not verify',async t=>{
  const initial=snapshot([
    post(oldId,'First primary',{author:'A',engagement:{likes:1,comments:2}}),
    post(newId,'Second primary',{author:'B',engagement:{likes:3,comments:4}}),
  ]);
  const cases=[
    ['unchanged',snapshot(initial.posts.map(value=>({...value})))],
    ['reordered and engagement-only',snapshot([
      post(newId,'Second primary',{author:'B renamed',engagement:{likes:31,comments:40}}),
      post(oldId,'First primary',{author:'A renamed',engagement:{likes:11,comments:20}}),
    ])],
  ];
  for(const [name,fresh] of cases){
    await t.test(name,async()=>{
      const {page}=makePage();
      const result=await invoke({page,initial,fresh});
      assert.equal(result.freshness.status,'reveal_unverified');
      assert.equal(result.freshness.limitation,'no_primary_content_change');
      assert.equal(result.freshness.verifiedPrimaryCount,0);
      assert.equal(result.snapshot,initial);
    });
  }
});

test('LinkedIn round two, explicit target and borrowed backend never reveal pending controls',async t=>{
  const cases=[
    ['round two',options({acquisitionRound:2,pendingContentPolicy:'detect_only',sourceFreshnessPolicy:'preserve_frontier',sameTabMutationAllowed:false}),'headless_worker','preserved'],
    ['explicit target',options({explicitPageUrl:true}),'headless_worker','not_applicable'],
    ['borrowed backend',options(),'browser_quiet_hidden','not_applicable'],
  ];
  for(const [name,opts,backend,status] of cases){
    await t.test(name,async()=>{
      const {page,state}=makePage();
      const result=await invoke({page,fresh:snapshot([post(oldId,'Original'),post(newId,'New')]),opts,backend});
      assert.equal(result.freshness.status,status);
      assert.equal(result.freshness.activationCount,0);
      assert.equal(result.freshness.activationAttempts,0);
      assert.equal(state.probeCalls,0);
      assert.equal(state.activationCalls,0);
    });
  }
});

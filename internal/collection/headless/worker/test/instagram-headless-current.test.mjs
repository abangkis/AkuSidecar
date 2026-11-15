import test from 'node:test';
import assert from 'node:assert/strict';
import {fileURLToPath} from 'node:url';
import {sourceAssets} from '../worker.mjs';
import {sourceFreshnessContractFor} from '../source-freshness-contract.mjs';
import {recoverHeadlessFreshness} from '../headless-freshness.mjs';

const assets=await sourceAssets(fileURLToPath(new URL('../../../../../../AkuBridge/',import.meta.url)),'instagram');
const actual=sourceFreshnessContractFor(assets,'instagram');
const home='https://www.instagram.com/';
const post=code=>({id:'instagram:p:'+code,permalink:home+'p/'+code+'/',text:'Owned primary '+code,author:'Fixture',media:[]});

function fixture(spec={}){
  const state={url:home,evaluations:0,collections:0,mutations:0};
  const snapshot={source:'instagram',url:home,documentReady:true,posts:[post('Fixture123')],...spec.snapshot};
  const page={async evaluate(expression){state.evaluations++;
    if(expression==='location.href')return state.url;
    if(expression.includes('.probe('))return {source:'instagram',pendingContentDetected:spec.pending===true};
    state.mutations++;throw Error('unexpected mutation or account probe');
  }};
  const collectSnapshot=async()=>{state.collections++;
    if(spec.routeChanged)state.url=home+'accounts/login/';
    return {...snapshot,...spec.fresh,posts:spec.added?[post('New123'),...snapshot.posts]:spec.fresh?.posts??snapshot.posts};
  };
  const request={source:'instagram',backend:'headless_worker',snapshot,page,collectSnapshot,
    contract:{...actual,settledObservationMs:500},deadlineAt:Date.now()+8000,
    options:{pageUrl:home,acquisitionRound:1,sourceFreshnessPolicy:'wake_and_reveal',pendingContentPolicy:'reveal_if_present',sameTabMutationAllowed:true}};
  return {state,snapshot,request};
}

test('actual Instagram contract permits bounded read-only current observation, never reveal',()=>{
  assert.equal(actual.enabled,true);assert.equal(actual.mode,'settled_current');assert.equal(actual.settledObservationMs,3500);
  for(const url of [home,home+'?hl=en#feed'])assert.equal(actual.matchesFeedURL(url),true,url);
  for(const url of [home+'p/Fixture123/',home+'accounts/login/',home+'./',home+'%2e/',home+'../',
    'http://www.instagram.com/','https://instagram.com/','https://www.instagram.com:443/','https://www.instagram.com:444/',
    'https://user@www.instagram.com/','https://www.instagram.com.evil.test/',home+'\\@evil.test/',42])assert.equal(actual.matchesFeedURL(url),false,String(url));
  for(const kind of ['p','reel','tv'])assert.equal(actual.primaryIdentity({id:`instagram:${kind}:Code_123`,permalink:home+kind+'/Code_123/'}).id,`instagram:${kind}:Code_123`);
  for(const value of [{id:'instagram:p:Other',permalink:post('Fixture123').permalink},{id:123,permalink:post('Fixture123').permalink},
    {id:'',permalink:post('Fixture123').permalink,quotedPost:{id:post('Fixture123').id}},
    {...post('Fixture123'),permalink:'https://evil.test/p/Fixture123/'},
    {...post('Fixture123'),permalink:home+'p/%46ixture123/'},
    {...post('Fixture123'),permalink:home+'p/x/../Fixture123/'},
    {...post('Fixture123'),permalink:post('Fixture123').permalink+'?tracking=1'}])assert.equal(actual.primaryIdentity(value),null);
});

test('actual 3500ms adapter settling observes owned primary content with zero mutation',async()=>{
  const f=fixture();f.request.contract=actual;
  const result=await recoverHeadlessFreshness(f.request);
  assert.equal(result.freshness.status,'checked_no_pending');assert.equal(result.freshness.verification,'adapter_wake_contract');
  assert.equal(result.freshness.readOnly,true);assert.equal(result.freshness.activationCount,0);assert.equal(result.freshness.feedMutation,false);
  assert.equal(result.freshness.verifiedPrimaryCount,1);assert.equal(f.state.mutations,0);assert.ok(f.state.collections>=2);
});

test('naturally changed owned primary evidence uses the same read-only source contract',async()=>{
  const f=fixture({added:true}),result=await recoverHeadlessFreshness(f.request);
  assert.equal(result.freshness.status,'verified');assert.equal(result.freshness.addedPrimaryCount,1);assert.equal(f.state.mutations,0);
});

test('pending Instagram content remains unsupported in both modes rather than clicking or claiming freshness',async()=>{
  const f=fixture({pending:true});await assert.rejects(recoverHeadlessFreshness(f.request),{code:'freshness_unavailable'});assert.equal(f.state.mutations,0);
});

test('invalid, empty, changed-identity and incomplete samples cannot establish current content',async()=>{
  for(const spec of [{snapshot:{posts:[]}},{snapshot:{posts:[{...post('Fixture123'),id:'instagram:p:Wrong'}]}},
    {snapshot:{documentReady:false}},{snapshot:{url:home+'p/Fixture123/'}},
    {fresh:{posts:[{...post('Fixture123'),id:'instagram:p:Wrong'}]}},{fresh:{documentReady:false}}]){
    const f=fixture(spec),result=await recoverHeadlessFreshness(f.request);
    assert.equal(result.freshness.status,'not_verified');assert.equal(result.snapshot,f.snapshot);assert.equal(f.state.mutations,0);
  }
});

test('route, login, challenge and availability changes keep their blocking errors',async()=>{
  for(const [spec,code]of [[{routeChanged:true},'source_route_changed'],[{snapshot:{loginRequired:true,posts:[]}},'login_required'],[{fresh:{loginRequired:true}},'login_required'],
    [{fresh:{challengeDetected:true}},'challenge_required'],[{fresh:{sourceUnavailable:true}},'source_unavailable']]){
    const f=fixture(spec);await assert.rejects(recoverHeadlessFreshness(f.request),{code});assert.equal(f.state.mutations,0);
  }
});

test('short budgets, continuation, explicit target and borrowed pages do not run a settling phase',async()=>{
  for(const kind of ['budget','round','target','backend','preserve_frontier','preserve_target']){
    const f=fixture();
    if(kind==='budget')f.request.deadlineAt=Date.now()+100;
    if(kind==='round')f.request.options.acquisitionRound=2;
    if(kind==='target')f.request.options.explicitPageUrl=true;
    if(kind==='backend')f.request.backend='browser_quiet_hidden';
    if(kind.startsWith('preserve_'))f.request.options.sourceFreshnessPolicy=kind;
    const result=await recoverHeadlessFreshness(f.request);
    assert.notEqual(result.freshness.status,'checked_no_pending');assert.equal(f.state.evaluations,0);assert.equal(f.state.collections,0);assert.equal(f.state.mutations,0);
  }
});

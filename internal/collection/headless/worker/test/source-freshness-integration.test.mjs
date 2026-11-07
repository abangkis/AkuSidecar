import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFile} from 'node:fs/promises';
import {createHash} from 'node:crypto';
import {capture} from '../capture.mjs';
import {sourceAssets} from '../worker.mjs';
import {fileURLToPath} from 'node:url';
import {sourceFreshnessContractFor} from '../source-freshness-contract.mjs';
const bridge=fileURLToPath(new URL('../../../../../../AkuBridge/',import.meta.url));
const runtime=await readFile(new URL('../../../../../../AkuBridge/source-freshness-runtime.js',import.meta.url),'utf8');
const adapterAssets=await Promise.all(['capture-primitives.js','source-adapter-runtime.js',...['x','linkedin','instagram','facebook'].map(source=>`adapters/${source}-adapter.js`)].map(async name=>({
  relative:'AkuBridge/'+name,execute:false,content:await readFile(new URL('../../../../../../AkuBridge/'+name,import.meta.url),'utf8'),sha256:'a'.repeat(64)})));
const linkedInAssets=(await sourceAssets(bridge,'linkedin')).filter(asset=>[
  'capture-primitives.js','source-adapter-runtime.js','source-feed-routes.js','adapters/linkedin-adapter.js','source-freshness-runtime.js',
].some(path=>asset.relative===`AkuBridge/${path}`));
const linkedInFeed='https://www.linkedin.com/feed/';
const linkedInAlias='https://www.linkedin.com/feed/foryou/';
const oldID='2106867448746021278',newID='2106867448746021279';
function fixture({pending=true,routeChange=false,source='x',pendingLabel='Show 2 posts',landedURL,activationURL,postsAt}={}){
  const contract=sourceFreshnessContractFor(adapterAssets,source);
  const state={clicks:0,navigations:0,scrolls:0,scrollY:0,freshnessEvaluations:0,posts:[oldID],url:'about:blank'};
  const sourcePost=id=>({
    x:{id,permalink:'https://x.com/example/status/'+id},
    linkedin:{id:'linkedin:activity:'+id,permalink:'https://www.linkedin.com/feed/update/urn:li:activity:'+id+'/'},
    instagram:{id:'instagram:p:'+id,permalink:'https://www.instagram.com/p/'+id+'/'},
    facebook:{id:'facebook:post:'+id,permalink:'https://www.facebook.com/example/posts/'+id},
  })[source];
  class Element{
    innerText=pendingLabel;textContent=pendingLabel;isConnected=true;
    getBoundingClientRect(){return {width:120,height:24,bottom:24,top:0,right:120,left:0};}
    click(){state.clicks++;state.posts=[newID,oldID];this.isConnected=false;if(routeChange)state.url='https://x.com/login';}
  }
  const button=new Element();
  const candidate={contains:()=>false,get innerText(){return state.posts.join('|');},getBoundingClientRect:()=>({width:200,height:100,bottom:100,top:0,right:200,left:0})};
  const location={get href(){return state.url;},get hostname(){return new URL(state.url).hostname;}};
  const context=vm.createContext({Element,URL,Date,setTimeout,clearTimeout,location,innerHeight:900,
    window:{location,innerHeight:900,innerWidth:1280,get scrollY(){return state.scrollY;},
      scrollTo({top}){state.scrolls++;state.scrollY=top;},scrollBy(_x,y){state.scrolls++;state.scrollY+=y;}},
    document:{visibilityState:'visible',querySelector:()=>null,
      querySelectorAll:selector=>selector==='button,[role="button"]'&&pending&&button.isConnected?[button]:[]},
    getComputedStyle:()=>({display:'block',visibility:'visible',opacity:'1'}),
    AkuSourceAdapters:{get:()=>({matchesPage:()=>state.url.startsWith(source==='linkedin'?'https://www.linkedin.com/':'https://x.com/'),freshness:{version:`${source}-freshness-v1`,headless:contract,revealSupported:true,rejectInsideFeedCandidate:true,pendingContentPattern:source==='linkedin'?/^(?:new posts?|show new posts?)$/i:/^show \d+ posts$/i},discoverCandidates:()=>({candidates:[]})})},
    XHeadlessPoC:{collect:()=>({url:state.url,posts:(postsAt?.(state)??state.posts).map(id=>({...sourcePost(id),author:'Example',text:'Native post '+id,limitations:[],media:[]})),scroll:{y:state.scrollY,viewportHeight:900,height:2000},documentReady:true})}});
  const page={async navigate(url){state.requestedUrl=url;state.url=landedURL??url;state.navigations++;return {};},async evaluate(expression){
    if(/runtime\.(?:probe|activatePending)/.test(expression))state.freshnessEvaluations++;
    if(activationURL&&expression.includes('runtime.activatePending('))state.url=activationURL;
    return vm.runInContext(expression,context);
  },async send(){return {};}};
  const asset={relative:'AkuBridge/source-freshness-runtime.js',execute:true,content:runtime,sha256:createHash('sha256').update(runtime).digest('hex')};
  const assets={[source]:source==='linkedin'?linkedInAssets:[...adapterAssets,asset]};
  return {state,browser:{backend:'headless_worker',forSource:async()=>page},assets};
}
const payload={scrolls:0,sourceHydrationTimeoutMs:1000,captureTimeoutMs:6000};
test('capture carries a verified revealed primary and preserves its frontier in round two',async()=>{
  const f=fixture();const first=await capture(f.browser,f.assets,'x',payload);
  assert.equal(f.state.clicks,1);
  assert.ok(first.snapshots[0].blocks.some(b=>b.platformId==='x:status:'+newID));
  assert.equal(first.coverage.freshness.workerStatus,'verified');
  const next=await capture(f.browser,f.assets,'x',{...payload,acquisitionRound:2,
    pendingContentPolicy:'detect_only',sourceFreshnessPolicy:'preserve_frontier',sameTabMutationAllowed:false,
    continuation:{startScrollY:first.coverage.frontier.scrollY,anchorKeys:first.coverage.frontier.anchorKeys,settleMs:0}});
  assert.ok(next.snapshots[0].blocks.some(b=>b.platformId==='x:status:'+newID));
  assert.equal(next.coverage.freshness.workerStatus,'preserved');
  assert.equal(next.coverage.freshness.activationCount,0);
  assert.equal(f.state.clicks,1);assert.equal(f.state.navigations,1);
});
test('capture never reveals on an explicit native permalink or Quiet backend',async()=>{
  for(const target of [true,false]){
    const f=fixture();if(!target)f.browser.backend='browser_quiet_hidden';
    await capture(f.browser,f.assets,'x',{...payload,...(target?{pageUrl:'https://x.com/example/status/'+oldID}:{})});
    assert.equal(f.state.clicks,0);
  }
});
test('capture rejects a route-changing reveal rather than admitting original feed evidence',async()=>{
  const f=fixture({routeChange:true});
  await assert.rejects(capture(f.browser,f.assets,'x',payload),error=>/route/.test(error.code||error.message));
  assert.equal(f.state.clicks,1);
});
test('Facebook and Instagram retain disabled contracts while preserving read-only capture',async()=>{
  for(const source of ['instagram','facebook']){
    const f=fixture({source});
    const observation=await capture(f.browser,f.assets,source,payload);
    assert.equal(observation.coverage.freshness.workerStatus,'not_verified',source);
    assert.equal(observation.coverage.freshness.limitation,'source_contract_unsupported',source);
    assert.ok(observation.snapshots[0].blocks.length>0,source);
    assert.equal(f.state.clicks,0,source);assert.equal(f.state.freshnessEvaluations,0,source);
  }
});
test('LinkedIn clicks Show new posts on For You and preserves that route in round two',async()=>{
  const f=fixture({source:'linkedin',pendingLabel:'Show new posts',landedURL:linkedInAlias});
  const first=await capture(f.browser,f.assets,'linkedin',payload);
  assert.equal(f.state.requestedUrl,linkedInFeed);
  assert.equal(f.state.clicks,1);
  assert.ok(first.snapshots[0].blocks.some(b=>b.platformId==='linkedin:activity:'+newID));
  assert.equal(first.coverage.freshness.workerStatus,'verified');
  const next=await capture(f.browser,f.assets,'linkedin',{...payload,acquisitionRound:2,
    pendingContentPolicy:'detect_only',sourceFreshnessPolicy:'preserve_frontier',sameTabMutationAllowed:false,
    continuation:{startScrollY:first.coverage.frontier.scrollY,anchorKeys:first.coverage.frontier.anchorKeys,settleMs:0}});
  assert.ok(next.snapshots[0].blocks.some(b=>b.platformId==='linkedin:activity:'+newID));
  assert.equal(next.coverage.freshness.workerStatus,'preserved');
  assert.equal(next.coverage.freshness.activationCount,0);
  assert.equal(f.state.clicks,1);
  assert.equal(f.state.navigations,1);
});
test('LinkedIn refuses a different route before clicking even when the route is also approved',async()=>{
  const f=fixture({source:'linkedin',pendingLabel:'Show new posts',landedURL:linkedInAlias,activationURL:linkedInFeed});
  await assert.rejects(capture(f.browser,f.assets,'linkedin',payload),{code:'source_route_changed'});
  assert.equal(f.state.clicks,0);
});
test('LinkedIn round two rejects a different landed route before restoring scroll',async()=>{
  const f=fixture({source:'linkedin',pendingLabel:'Show new posts',landedURL:linkedInAlias});
  const first=await capture(f.browser,f.assets,'linkedin',payload);
  f.state.url=linkedInFeed;
  const before=f.state.scrolls;
  await assert.rejects(capture(f.browser,f.assets,'linkedin',{...payload,acquisitionRound:2,
    continuation:{startScrollY:first.coverage.frontier.scrollY,anchorKeys:first.coverage.frontier.anchorKeys,settleMs:0}}),
    {code:'source_frontier_unavailable'});
  assert.equal(f.state.scrolls,before);
  assert.equal(f.state.clicks,1);
  assert.equal(f.state.navigations,1);
});
test('LinkedIn retains the preceding same-feed checkpoint for a scrolled round two',async()=>{
  const f=fixture({source:'linkedin',pending:false,landedURL:linkedInAlias,
    postsAt:state=>state.scrollY>0||state.hideAnchor?[newID]:[oldID]});
  const first=await capture(f.browser,f.assets,'linkedin',{...payload,scrolls:1,scrollSettleMs:100});
  const {frontier}=first.coverage;
  assert.equal(frontier.scrollY,675);
  assert.deepEqual(frontier.anchorKeys,['linkedin:activity:'+newID]);
  assert.deepEqual(frontier.continuationCheckpoint,{scrollY:0,anchorKeys:['linkedin:activity:'+oldID]});
  const second={...payload,acquisitionRound:2,
    continuation:{startScrollY:0,anchorKeys:frontier.continuationCheckpoint.anchorKeys,settleMs:0}};
  await assert.rejects(capture(f.browser,f.assets,'linkedin',{...second,
    continuation:{startScrollY:675,anchorKeys:frontier.anchorKeys,settleMs:0}}),{code:'unsupported_continuation'});
  f.state.hideAnchor=true;
  await assert.rejects(capture(f.browser,f.assets,'linkedin',second),{code:'source_frontier_unavailable'});
  f.state.hideAnchor=false;
  const resumed=await capture(f.browser,f.assets,'linkedin',second);
  assert.equal(resumed.snapshots[0].scrollY,0);
  assert.equal(resumed.coverage.freshness.workerStatus,'preserved');
  assert.equal(f.state.clicks,0);
  assert.equal(f.state.navigations,1);
});
test('all sources stage shared freshness after their adapter with generic controller provenance',async()=>{
  for(const source of ['x','linkedin','instagram','facebook']){
    const assets=await sourceAssets(bridge,source);
    const shared=assets.find(a=>a.relative==='AkuBridge/source-freshness-runtime.js');
    assert.equal(shared.execute,true);assert.equal(shared.sha256,createHash('sha256').update(runtime).digest('hex'));
    assert.ok(assets.indexOf(shared)>assets.findIndex(a=>a.relative===`AkuBridge/adapters/${source}-adapter.js`));
    assert.ok(assets.some(a=>a.relative==='worker/headless-freshness.mjs'&&!a.execute));
    assert.ok(assets.some(a=>a.relative==='worker/source-freshness-contract.mjs'&&!a.execute));
  }
});

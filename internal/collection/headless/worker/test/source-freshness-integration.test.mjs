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
const oldID='2106867448746021278',newID='2106867448746021279';
function fixture({pending=true,routeChange=false,source='x'}={}){
  const contract=sourceFreshnessContractFor(adapterAssets,source);
  const state={clicks:0,navigations:0,freshnessEvaluations:0,posts:[oldID],url:'about:blank'};
  const sourcePost=id=>({
    x:{id,permalink:'https://x.com/example/status/'+id},
    linkedin:{id:'linkedin:activity:'+id,permalink:'https://www.linkedin.com/feed/update/urn:li:activity:'+id+'/'},
    instagram:{id:'instagram:p:'+id,permalink:'https://www.instagram.com/p/'+id+'/'},
    facebook:{id:'facebook:post:'+id,permalink:'https://www.facebook.com/example/posts/'+id},
  })[source];
  class Element{
    innerText='Show 2 posts';textContent='Show 2 posts';isConnected=true;
    getBoundingClientRect(){return {width:120,height:24,bottom:24,top:0,right:120,left:0};}
    click(){state.clicks++;state.posts=[newID,oldID];this.isConnected=false;if(routeChange)state.url='https://x.com/login';}
  }
  const button=new Element();
  const candidate={contains:()=>false,get innerText(){return state.posts.join('|');},getBoundingClientRect:()=>({width:200,height:100,bottom:100,top:0,right:200,left:0})};
  const location={get href(){return state.url;}};
  const context=vm.createContext({Element,URL,Date,setTimeout,clearTimeout,location,window:{innerHeight:900,innerWidth:1280,scrollY:0,scrollTo(){}},
    document:{visibilityState:'visible',querySelectorAll:()=>pending&&button.isConnected?[button]:[]},
    getComputedStyle:()=>({display:'block',visibility:'visible',opacity:'1'}),
    AkuSourceAdapters:{get:()=>({matchesPage:()=>state.url.startsWith('https://x.com/'),freshness:{version:'x-freshness-v1',headless:contract,revealSupported:true,rejectInsideFeedCandidate:true,pendingContentPattern:/^show \d+ posts$/i},discoverCandidates:()=>({candidates:[]})})},
    XHeadlessPoC:{collect:()=>({url:state.url,posts:state.posts.map(id=>({...sourcePost(id),author:'Example',text:'Native post '+id,limitations:[],media:[]})),scroll:{y:0,viewportHeight:900,height:2000},documentReady:true})}});
  const page={async navigate(url){state.url=url;state.navigations++;return {};},async evaluate(expression){if(/runtime\.(?:probe|activatePending)/.test(expression))state.freshnessEvaluations++;return vm.runInContext(expression,context);},async send(){return {};}};
  const asset={relative:'AkuBridge/source-freshness-runtime.js',execute:true,content:runtime,sha256:createHash('sha256').update(runtime).digest('hex')};
  const assets={[source]:[...adapterAssets,asset]};
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
test('disabled source contracts preserve capture without probing or activating pending controls',async()=>{
  for(const source of ['linkedin','instagram','facebook']){
    const f=fixture({source});
    const observation=await capture(f.browser,f.assets,source,payload);
    assert.equal(observation.coverage.freshness.workerStatus,'not_verified',source);
    assert.equal(observation.coverage.freshness.limitation,'source_contract_unsupported',source);
    assert.ok(observation.snapshots[0].blocks.length>0,source);
    assert.equal(f.state.clicks,0,source);assert.equal(f.state.freshnessEvaluations,0,source);
  }
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

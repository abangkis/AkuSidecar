import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFile} from 'node:fs/promises';
import {fileURLToPath} from 'node:url';
import {createHash} from 'node:crypto';
import {sourceAssets} from '../worker.mjs';
import {instagramCandidateQualityPolicyFor} from '../capture-primitives.mjs';
import {sourceFreshnessContractFor} from '../source-freshness-contract.mjs';
import {recoverHeadlessFreshness} from '../headless-freshness.mjs';
import {recoverSourceFreshness} from '../../../../../../AkuBridge/source-freshness-recovery.js';
import {captureIsolated} from '../capture.mjs';
import {toObservation} from '../observation.mjs';

const bridge=new URL('../../../../../../AkuBridge/',import.meta.url);
const assets=await sourceAssets(fileURLToPath(bridge),'instagram');
const contract=instagramCandidateQualityPolicyFor(assets);
const contentScript=await readFile(new URL('content-script.js',bridge),'utf8');
const plain=value=>JSON.parse(JSON.stringify(value));
const fields=['profile','contentFamily','evidenceModalities','identitySource','candidateKey','verdict','score','attempt','issues'];

// A deterministic DOM fixture, not a browser/account fixture. Both consumers use
// the real adapter and Windowed's actual quality call on the same captured data.
function fixture(spec={}) {
  const effects={clicks:0,scrolls:0,network:0,wakes:0,navigations:0};
  class Element {
    constructor(tagName,text='',attrs={}) {Object.assign(this,{tagName,innerText:text,textContent:text,attrs,childNodes:[],isConnected:true,style:{},parentElement:null},attrs);}
    getAttribute(name){return this.attrs[name]??null;}
    hasAttribute(name){return this.attrs[name]!==undefined;}
    getBoundingClientRect(){return {width:600,height:300,left:0,right:600,top:10,bottom:310};}
    querySelectorAll(){return [];}
    querySelector(s){return this.querySelectorAll(s)[0]??null;}
    contains(node){for(let n=node;n;n=n.parentElement)if(n===this)return true;return false;}
    closest(){return null;}
    click(){effects.clicks++;throw Error('fixture mutation forbidden');}
    scrollIntoView(){effects.scrolls++;throw Error('fixture scrolling forbidden');}
  }
  const body=new Element('BODY'),main=new Element('MAIN'),article=new Element('ARTICLE');
  main.parentElement=body;article.parentElement=main;
  const link=new Element('A','',{href:'https://www.instagram.com/p/Fixture123/'});
  const profile=new Element('A','fixture_author',{href:'https://www.instagram.com/fixture_author/'});
  const box=new Element('DIV'),caption=new Element('SPAN',spec.text??'An authored Instagram caption with stable native identity.');
  const more=new Element('BUTTON','more');
  for(const n of [link,profile,box])n.parentElement=article;
  caption.parentElement=box;more.parentElement=box;
  article.innerText=article.textContent='fixture_author\n'+caption.innerText;
  box.innerText=box.textContent=caption.innerText;
  const media=spec.video ? [new Element('VIDEO','',{poster:'https://s.cdninstagram.com/poster.jpg',currentSrc:spec.playback?'https://s.cdninstagram.com/owned.mp4':'',src:'',readyState:0,paused:true,currentTime:0,duration:NaN})]
    : spec.image ? [new Element('IMG','',{src:spec.missing?'':'https://s.cdninstagram.com/owned.jpg',currentSrc:spec.missing?'':'https://s.cdninstagram.com/owned.jpg',alt:'Owned photo',complete:!spec.missing,naturalWidth:spec.missing?0:800,naturalHeight:spec.missing?0:600})] : [];
  for(const n of media)n.parentElement=article;
  box.querySelectorAll=s=>s==='span[dir="auto"]'?[caption]:s==='[role="button"], button'&&spec.collapsed?[more]:[];
  article.querySelectorAll=s=>{
    if(s==='a[href]')return [link,profile];
    if(s==='a[href^="/"]'||s==='a[href^="/"] img[alt*="profile picture" i], a[href^="/"]')return [profile];
    if(s==='span[dir="auto"]')return caption.innerText?[caption]:[];
    if(s==='img'||s==='img:not([alt*="profile picture" i])')return media.filter(n=>n.tagName==='IMG');
    if(s==='video')return media.filter(n=>n.tagName==='VIDEO');
    if(s==='video, img:not([alt*="profile picture" i])'||s==='img, video')return media;
    return [];
  };
  main.querySelectorAll=s=>s==='a[href]'?[link,profile]:[];
  const pending=new Element('BUTTON','New posts');
  const document={title:'Offline B2 fixture',readyState:'complete',visibilityState:spec.background?'hidden':'visible',body,
    scrollingElement:{scrollHeight:1400},querySelector:s=>s==='main'?main:null,
    querySelectorAll:s=>s==='main article'?[article]:s==='button,[role="button"]'&&spec.pending?[pending]:[]};
  const context=vm.createContext({URL,Element,Date,setTimeout,clearTimeout,document,location:new URL('https://www.instagram.com/'),
    innerHeight:900,innerWidth:1280,scrollY:0,getComputedStyle:()=>({display:'block',visibility:'visible',opacity:'1'}),
    fetch:()=>{effects.network++;throw Error('offline fixture');}});
  context.window=context;
  for(const asset of assets.filter(a=>a.execute))vm.runInContext(asset.content,context,{filename:asset.relative});
  context.AkuHeadlessCapturePolicy={allowContentExpansion:false};
  context.sourceAdapters=context.AkuSourceAdapters;
  let windowedFacts;
  context.qualityPolicy={...context.AkuCaptureQualityPolicy,evaluateCandidate(value){windowedFacts=plain(context.AkuCaptureQualityPolicy.normalizeFacts(value.facts));return context.AkuCaptureQualityPolicy.evaluateCandidate(value);}};
  context.compactText=value=>String(value??'').replace(/\s+/g,' ').trim();
  context.uniqueElements=values=>[...new Set(values)];
  for(const name of ['evaluateBlockQuality','selectorDetectedOutside','hasPotentialMediaRoot']){
    const code=contentScript.match(new RegExp('^  function '+name+'\\([\\s\\S]*?^  }','m'))?.[0];
    assert.ok(code,name);vm.runInContext(code,context);
  }
  return {context,article,effects,get windowedFacts(){return windowedFacts;},
    project:snapshot=>toObservation({source:'instagram',requestedUrl:'https://www.instagram.com/',snapshots:[snapshot],capturedAt:'2026-11-15T00:00:00Z',instagramCandidateQualityPolicy:contract})};
}

for(const spec of [
  {name:'caption',verdict:'complete'},
  {name:'owned image',image:true,verdict:'complete'},
  {name:'detected image without URL',image:true,missing:true,verdict:'usable_degraded'},
  {name:'video poster without playback',video:true,verdict:'complete',limitation:'video_stream_not_resolved'},
  {name:'owned video URL without playback verification',video:true,playback:true,verdict:'complete'},
  {name:'collapsed caption with no mutation authority',collapsed:true,verdict:'complete',limitation:'text_may_be_collapsed'},
  {name:'media-only post',image:true,text:'',verdict:'complete'},
])test(`Instagram quality parity at equal terminal budget: ${spec.name}`,async()=>{
  const f=fixture(spec),snapshot=await f.context.XHeadlessPoC.collect();
  assert.equal(snapshot.posts.length,1,JSON.stringify(snapshot.rejectionReasons));
  const post=snapshot.posts[0],block=f.project(snapshot).snapshots[0].blocks[0];
  const common={...block,media:plain(block.media)};
  const windowed=f.context.evaluateBlockQuality(f.article,'instagram',common,null,0,0,null);
  const {schema,...facts}=plain(post.qualityFacts);
  assert.equal(schema,'aku.instagram.candidate-quality-facts.v1');
  assert.deepEqual(facts,f.windowedFacts,'facts from the actual DOM observer must match Windowed');
  const report=block.captureQuality.assessment;
  for(const key of fields)assert.deepEqual(report[key],plain(windowed[key]),key);
  assert.equal(report.verdict,spec.verdict);assert.equal(report.retriesRemaining,0);
  assert.equal(block.captureQuality.status,'unverified');
  assert.equal(Object.hasOwn(block.captureQuality,'verdict'),false);
  assert.equal(Object.hasOwn(block.captureQuality,'qualityFacts'),false);
  if(spec.limitation)assert.ok(block.captureQuality.limitations.includes(spec.limitation));
  if(spec.collapsed)assert.equal(report.textCompleteness,'partial');
  if(spec.video)assert.notEqual(block.mediaRecovery.status,'resolved','profile completeness cannot establish playback proof');
  assert.deepEqual(f.effects,{clicks:0,scrolls:0,network:0,wakes:0,navigations:0});
});

test('Instagram terminal policy reassesses recovered media and never accepts page-supplied verdict or foreign schemas',()=>{
  const f=fixture();
  const candidate={platformId:'instagram:p:Fixture123',permalink:'https://www.instagram.com/p/Fixture123/',author:'fixture',text:'caption',media:[],presentation:{}};
  const facts={schema:'aku.instagram.candidate-quality-facts.v1',contentRootDetected:true,mediaRootDetected:true,verdict:'complete',retriesRemaining:99};
  assert.equal(contract.assess(candidate,facts,'partial').verdict,'usable_degraded');
  const recovered=contract.assess({...candidate,media:[{kind:'image',url:'https://s.cdninstagram.com/owned.jpg'}]},facts,'recovery_verified');
  assert.equal(recovered.verdict,'complete');assert.equal(recovered.retriesRemaining,0);assert.equal(recovered.textCompleteness,'recovery_verified');
  assert.equal(contract.assess(candidate,{...facts,schema:'aku.x.candidate-quality-facts.v1'},'partial'),null);
  const unknown={schema:'aku.instagram.candidate-quality-facts.v1',mediaRootDetected:'true'};
  Object.defineProperty(unknown,'authorRootDetected',{get(){throw Error('getter invoked');}});
  assert.equal(contract.assess(candidate,unknown,'unknown').verdict,'complete');
  assert.throws(()=>instagramCandidateQualityPolicyFor(assets.filter(a=>a.relative!=='AkuBridge/capture-quality-policy.js')),/Instagram candidate quality asset is required/);
  assert.equal(instagramCandidateQualityPolicyFor(assets),contract);
  assert.equal(f.context.AkuSourceAdapters.get('instagram').mediaAcquisition.isComplete,undefined,'no new acquisition semantics');
  const withoutFacts=toObservation({source:'instagram',requestedUrl:'https://www.instagram.com/',
    snapshots:[{posts:[{id:candidate.platformId,...candidate}]}],capturedAt:'2026-11-15T00:00:00Z',instagramCandidateQualityPolicy:contract});
  assert.equal(withoutFacts.snapshots[0].blocks[0].captureQuality.status,'unverified');
  assert.equal(withoutFacts.snapshots[0].blocks[0].captureQuality.assessment,undefined,'structured/legacy observations cannot invent DOM facts');
});

test('real capture composition retains passive quality and refuses an insufficient settling budget',async()=>{
  const f=fixture();
  const page={async navigate(){f.effects.navigations++;return {};},async send(){return {};},async evaluate(expression){return vm.runInContext(expression,f.context);}};
  const observation=await captureIsolated({backend:'headless_worker',forSource:async()=>page}, {instagram:assets},'instagram',{
    scrolls:0,sourceHydrationTimeoutMs:1000,captureTimeoutMs:5000,sameTabMutationAllowed:false,pendingContentPolicy:'detect_only'});
  const block=observation.snapshots[0].blocks[0];
  assert.equal(block.captureQuality.status,'unverified');assert.equal(block.captureQuality.assessment.verdict,'complete');
  assert.equal(observation.coverage.freshness.workerStatus,'unavailable');
  assert.equal(observation.coverage.freshness.limitation,'settlement_budget_unavailable');
  assert.equal(f.effects.clicks,0);assert.equal(f.effects.network,0);assert.equal(f.effects.navigations,1);
  const policy=assets.find(a=>a.relative==='AkuBridge/capture-quality-policy.js');
  assert.equal(policy.sha256,createHash('sha256').update(policy.content).digest('hex'));
});

for(const spec of [{name:'active feed'},{name:'background settled feed',background:true},{name:'pending control',pending:true},{name:'follow-up',round:2}])
test(`Instagram freshness capability reconciliation: ${spec.name}`,async()=>{
  const f=fixture(spec);let time=0;
  const probe=()=>f.context.AkuSourceFreshnessRuntime.probe('instagram');
  const request={source:'instagram',acquisitionRound:spec.round??1,backgroundAtDispatch:spec.background===true,
    pendingContentPolicy:'reveal_if_present',sameTabMutationAllowed:true,probe,
    activate:async()=>{f.effects.wakes++;f.context.document.visibilityState='visible';return true;},
    reveal:()=>f.context.AkuSourceFreshnessRuntime.reveal('instagram'),now:()=>time,delay:async ms=>{time+=ms;}};
  if(spec.pending)await assert.rejects(recoverSourceFreshness(request),{code:'freshness_unavailable'});
  else {
    const result=await recoverSourceFreshness(request);
    assert.equal(result.outcome,spec.round===2?'follow_up_preserved':spec.background?'adapter_wake_settled':'active_feed_ready');
    assert.equal(result.feedMutation,false);
  }
  let evaluations=0;
  const snapshot=await f.context.XHeadlessPoC.collect();
  const headless=()=>recoverHeadlessFreshness({source:'instagram',backend:'headless_worker',snapshot,
    options:{pageUrl:'https://www.instagram.com/',acquisitionRound:spec.round??1,pendingContentPolicy:'reveal_if_present',sourceFreshnessPolicy:'wake_and_reveal',sameTabMutationAllowed:true},
    contract:sourceFreshnessContractFor(assets,'instagram'),deadlineAt:Date.now()+10000,
    collectSnapshot:()=>f.context.XHeadlessPoC.collect(),
    page:{evaluate:async expression=>{evaluations++;return vm.runInContext(expression,f.context);}}});
  if(spec.pending)await assert.rejects(headless(),{code:'freshness_unavailable'});
  else {
    const result=await headless();
    assert.equal(result.freshness.workerStatus,spec.round===2?'preserved':'checked_no_pending');
    if(spec.round===2){assert.equal(result.snapshot,snapshot);assert.equal(evaluations,0);}
    else {
      assert.equal(result.freshness.verification,'adapter_wake_contract');
      assert.equal(result.freshness.readOnly,true);assert.equal(result.freshness.verifiedPrimaryCount,1);
      assert.equal(result.freshness.activationCount,0);assert.equal(result.freshness.feedMutation,false);
    }
  }
  assert.equal(f.effects.clicks,0);assert.equal(f.effects.network,0);
  assert.equal(f.effects.wakes,spec.background?1:0);
});

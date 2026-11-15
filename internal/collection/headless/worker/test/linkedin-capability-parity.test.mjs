import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFile} from 'node:fs/promises';
import {fileURLToPath} from 'node:url';
import {sourceAssets} from '../worker.mjs';
import {linkedinCandidateQualityPolicyFor} from '../capture-primitives.mjs';
import {toObservation} from '../observation.mjs';
import {resolveAdditionalSourceMedia} from '../additional-source-media.mjs';
import {captureIsolated} from '../capture.mjs';

const bridge=new URL('../../../../../../AkuBridge/',import.meta.url);
const assets=await sourceAssets(fileURLToPath(bridge),'linkedin');
const policy=linkedinCandidateQualityPolicyFor(assets);
const content=await readFile(new URL('content-script.js',bridge),'utf8');
const persisted=JSON.parse(await readFile(new URL('test/fixtures/linkedin-7525912124464857088-cover-only.json',bridge),'utf8'));
const plain=x=>JSON.parse(JSON.stringify(x));
const id=persisted.block.platformId,permalink=persisted.block.permalink;
const poster=persisted.block.media[0].url;
const playback='https://dms.licdn.com/playlist/vid/v2/synthetic-7525912124464857088/mp4-720p-30fp/0/video';

// Persisted normalized post fields, reconstructed DOM and synthetic player
// state. Both consumers run the actual adapter and shared policy on this DOM.
function fixture(mode='placeholder') {
  const effects={clicks:0,scrolls:0,network:0};
  class Element {
    constructor(tagName='DIV',text='',attrs={}) {
      Object.assign(this,{tagName,innerText:text,textContent:text,attrs,nodeType:1,
        childNodes:text?[{nodeType:3,nodeValue:text}]:[],parentElement:null,isConnected:true,style:{},
        id:attrs.id??'',href:attrs.href??'',src:attrs.src??'',currentSrc:attrs.src??'',alt:'',
        complete:true,naturalWidth:470,naturalHeight:264});
    }
    getAttribute(n){return this.attrs[n]??null;}
    hasAttribute(n){return n in this.attrs;}
    getBoundingClientRect(){return {left:0,right:600,top:20,bottom:320,width:600,height:300};}
    querySelectorAll(){return [];}
    querySelector(s){return this.querySelectorAll(s)[0]??null;}
    closest(){return null;}
    matches(){return false;}
    contains(n){for(;n;n=n.parentElement)if(n===this)return true;return false;}
    click(){effects.clicks++;throw Error('fixture mutation forbidden');}
    scrollIntoView(){effects.scrolls++;throw Error('fixture scroll forbidden');}
  }
  const body=new Element('BODY'),main=new Element('MAIN');main.parentElement=body;
  const post=new Element('ARTICLE',persisted.block.author+'\n'+persisted.block.text,
    {'data-urn':id.replace('linkedin:ugcpost:','urn:li:ugcPost:')});post.parentElement=main;
  post.matches=s=>s.includes('feed-full-update')||s.includes('feed-shared-update-v2');
  const text=new Element('DIV',persisted.block.text);
  const author=new Element('BUTTON','',{'aria-label':'Open control menu for post by '+persisted.block.author});
  const link=new Element('A','',{href:permalink});
  const player=new Element('DIV','',{id:'synthetic-player-7525912124464857088','data-vjs-player':'','data-view-name':'video'});
  for(const n of [text,author,link,player])n.parentElement=post;
  player.matches=()=>true;
  player.closest=s=>s.includes('feed-full-update')||s.includes('feed-shared-update-v2')||s.includes('[role="listitem"]')?post:null;
  const image=new Element('IMG','',{src:poster});image.parentElement=player;
  image.closest=s=>s.includes('data-vjs-player')?player:null;
  const video=new Element('VIDEO');video.parentElement=player;
  Object.assign(video,{poster,src:mode==='mp4'?playback:'',currentSrc:mode==='mp4'?playback:'',videoWidth:1280,videoHeight:720,
    readyState:0,paused:true,currentTime:0,duration:NaN});
  const images=mode==='cover'?[image]:[],videos=mode==='mp4'?[video]:[];
  player.querySelectorAll=s=>s==='img'?images:s==='video'?videos:[];
  const all=s=>s==='button[aria-label]'||s.startsWith('button[aria-label^=')?[author]
    :s==='a[href]'||s==='[data-urn], [data-id], a[href]'?[link]
    :s==='[data-vjs-player]'||s==='[data-vjs-player][id]'?[player]
    :s==='img'?images:s==='video'?videos
    :s==="video, [data-view-name*='video' i], [aria-label*='video' i]"?[player]:[];
  post.querySelectorAll=all;
  post.querySelector=s=>s==='[data-testid="expandable-text-box"]'?text:s==='video'?videos[0]??null:all(s)[0]??null;
  const document={body,title:'Offline LinkedIn C1 fixture',readyState:'complete',visibilityState:'visible',scrollingElement:{scrollHeight:1400},
    querySelector:s=>s==='main'||s.includes('data-testid="mainFeed"')?main:null,
    querySelectorAll:s=>s==='[data-vjs-player]'?[player]:s.includes('feed-full-update')||s==='.feed-shared-update-v2'?[post]:[]};
  const context=vm.createContext({URL,Date,Element,document,setTimeout,clearTimeout,
    location:new URL('https://www.linkedin.com/feed/'),innerHeight:900,innerWidth:1280,scrollY:0,
    getComputedStyle:()=>({display:'block',visibility:'visible',opacity:'1'}),fetch(){effects.network++;throw Error('offline fixture');}});
  context.window=context;
  for(const asset of assets.filter(a=>a.execute))vm.runInContext(asset.content,context,{filename:asset.relative});
  context.AkuHeadlessCapturePolicy={allowContentExpansion:false};
  context.sourceAdapters=context.AkuSourceAdapters;
  let windowedFacts;
  context.qualityPolicy={...context.AkuCaptureQualityPolicy,evaluateCandidate(value){windowedFacts=plain(context.AkuCaptureQualityPolicy.normalizeFacts(value.facts));return context.AkuCaptureQualityPolicy.evaluateCandidate(value);}};
  for(const name of ['evaluateBlockQuality','selectorDetectedOutside','hasPotentialMediaRoot']){
    const code=content.match(new RegExp('^  function '+name+'\\([\\s\\S]*?^  }','m'))?.[0];assert.ok(code,name);vm.runInContext(code,context);
  }
  return {context,post,player,document,effects,get windowedFacts(){return windowedFacts;}};
}
function project(snapshot){return toObservation({source:'linkedin',requestedUrl:permalink,snapshots:[snapshot],capturedAt:'2026-11-15T00:00:00Z',linkedinCandidateQualityPolicy:policy}).snapshots[0].blocks[0];}

for(const [mode,verdict]of [['placeholder','usable_degraded'],['cover','usable_degraded'],['mp4','complete']])
test(`LinkedIn equal-fixture terminal assessment and truthful recovery: ${mode}`,async()=>{
  const f=fixture(mode),snapshot=await f.context.XHeadlessPoC.collect();
  assert.equal(snapshot.posts.length,1,JSON.stringify(snapshot.rejectionReasons));
  const post=snapshot.posts[0],block=project(snapshot);
  assert.deepEqual(plain(post.mediaExpected),['video']);assert.equal(block.text,persisted.block.text);
  const windowed=f.context.evaluateBlockQuality(f.post,'linkedin',block,null,0,0,null);
  const {schema,...facts}=plain(post.qualityFacts);assert.equal(schema,'aku.linkedin.candidate-quality-facts.v1');
  assert.deepEqual(facts,f.windowedFacts);
  for(const key of ['profile','contentFamily','evidenceModalities','identitySource','candidateKey','verdict','score','attempt','issues'])
    assert.deepEqual(block.captureQuality.assessment[key],plain(windowed[key]),key);
  assert.equal(block.captureQuality.assessment.verdict,verdict);assert.equal(block.captureQuality.assessment.retriesRemaining,0);
  assert.equal(block.captureQuality.status,'unverified');assert.equal(block.captureQuality.verdict,undefined);
  assert.equal(block.mediaRecovery.outcome,mode==='mp4'?'owned_playback_url_observed':'unresolved');
  assert.equal(block.mediaRecovery.limitation,mode==='mp4'?'video_playback_unverified':'video_stream_not_resolved');
  assert.notEqual(block.mediaRecovery.status,'resolved');
  if(mode==='mp4')assert.equal(block.captureQuality.limitations.includes('video_stream_not_resolved'),false);
  assert.deepEqual(f.effects,{clicks:0,scrolls:0,network:0});
});

test('LinkedIn recovery reassesses final owned media through the same policy; retry and scope guards stay bounded',async()=>{
  for(const scenario of ['hydrated','unresolved','foreign','player','deadline']){
    const f=fixture(),snapshot=await f.context.XHeadlessPoC.collect();let calls=0,waits=0;
    if(scenario==='foreign')f.player.closest=()=>({getAttribute:n=>n==='data-urn'?'urn:li:ugcPost:7525912124464857999':null,querySelectorAll:()=>[]});
    if(scenario==='player')f.player.id='foreign-player';
    const recovered=await resolveAdditionalSourceMedia({source:'linkedin',posts:snapshot.posts,resolver:assets.structuredMediaResolver,
      deadlineAt:Date.now()+(scenario==='deadline'?80:5000),page:{evaluate:async expression=>{
        if(expression.includes('setTimeout')){waits++;return null;}
        calls++;return vm.runInNewContext(expression,{URL,document:f.document,vjsForDebug:{getPlayer:()=>scenario!=='unresolved' && calls>1?{source:playback,poster}:null}});
      }}});
    const block=project({...snapshot,posts:recovered.posts});
    const windowed=f.context.evaluateBlockQuality(f.post,'linkedin',block,null,0,0,null);
    assert.equal(block.captureQuality.assessment.verdict,windowed.verdict);
    assert.equal(block.captureQuality.assessment.verdict,scenario==='hydrated'?'complete':'usable_degraded');
    assert.equal(block.mediaRecovery.outcome,scenario==='hydrated'?'owned_playback_url_observed':'unresolved');
    assert.equal(block.media.some(m=>m.playbackUrl),scenario==='hydrated');
    assert.equal(calls,scenario==='deadline'?1:2);assert.equal(waits,scenario==='deadline'?0:1);
    assert.deepEqual(f.effects,{clicks:0,scrolls:0,network:0});
  }
});

test('legacy facts, foreign schemas, supplied verdicts and unsafe DOM URLs cannot invent LinkedIn proof',async()=>{
  const f=fixture('mp4'),snapshot=await f.context.XHeadlessPoC.collect(),post=snapshot.posts[0];
  for(const change of [{qualityFacts:undefined},{qualityFacts:{schema:'aku.instagram.candidate-quality-facts.v1',verdict:'complete'}}]){
    const block=project({...snapshot,posts:[{...post,...change}]});assert.equal(block.captureQuality.assessment,undefined);
    assert.equal(block.mediaRecovery.outcome,'unresolved');
  }
  for(const change of [{id:'linkedin:ugcpost:wrong'}, {permalink:'https://evil.test/'},
    {media:post.media.map(m=>({...m,playbackUrl:'https://evil.test/video.mp4'}))},
    {mediaEvidence:{...post.mediaEvidence,videoStreamStatus:'unknown'}},
    {mediaEvidence:{...post.mediaEvidence,expectedWithoutUrl:['video']}}])assert.equal(policy.hasObservedPlayback({...post,...change}),false);
  const raw={...plain(post.qualityFacts),verdict:'complete',retriesRemaining:99};
  const empty={...project(snapshot),media:[],contentKind:'post'};
  assert.equal(policy.assess(empty,raw,'partial',['video']).verdict,'usable_degraded');
  assert.equal(policy.assess(empty,raw,'partial',['video']).retriesRemaining,0);
  assert.throws(()=>linkedinCandidateQualityPolicyFor(assets.filter(a=>a.relative!=='AkuBridge/capture-quality-policy.js')),/LinkedIn candidate quality asset is required/);
});

test('actual LinkedIn capture composition loads the shared assessment without new mutation authority',async()=>{
  const f=fixture('mp4');
  const page={async navigate(){return {};},async send(){return {};},async evaluate(expression){
    return vm.runInContext(expression,f.context);
  }};
  const observation=await captureIsolated({backend:'headless_worker',forSource:async()=>page},{linkedin:assets},'linkedin',
    {scrolls:0,sourceHydrationTimeoutMs:1000,captureTimeoutMs:5000,sameTabMutationAllowed:false,pendingContentPolicy:'detect_only'});
  const block=observation.snapshots[0].blocks[0];assert.equal(block.captureQuality.assessment.verdict,'complete');
  assert.equal(block.mediaRecovery.outcome,'owned_playback_url_observed');assert.equal(block.captureQuality.status,'unverified');
  assert.deepEqual(f.effects,{clicks:0,scrolls:0,network:0});
});

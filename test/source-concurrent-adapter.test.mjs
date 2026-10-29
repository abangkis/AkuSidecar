import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {fileURLToPath} from 'node:url';
import {createSourceConcurrentAdapter,sourceConcurrentDiagnostic} from '../internal/collection/headless/worker/source-concurrent-adapter.mjs';
import {sourceAssets} from '../internal/collection/headless/worker/worker.mjs';
import {createInstagramVideoRecovery} from '../internal/collection/headless/worker/additional-source-media.mjs';
import {resolveInstagramStructuredMediaInMainWorld} from '../../AkuBridge/instagram-main-world-media-resolver.js';
import {resolveInstagramStructuredFeedInMainWorld} from '../../AkuBridge/instagram-main-world-feed-resolver.js';

const fixtureURL='https://www.instagram.com/p/Fixture/';
function setup(options={}) {
  const time=Date.now(),binding={readerProfileId:'reader-profile',collectorProfileId:'collector-profile',accountId:'fixture-account',generation:1,attempt:'fixture-attempt',deadlineMS:time+10000};
  const state={time,proofs:[],pages:[],readerClosed:0,after:false};
  const owner={source:'instagram',profileId:binding.collectorProfileId,generation:1,attempt:binding.attempt};
  const page=()=>({binding:{...owner},url:'about:blank',closed:0,
    async navigate(url){this.url=url;return{};},
    async evaluate(expression){
      if(expression==='location.href')return this.url;
      if(expression==='({url:location.href,ready:document.readyState})')return{url:this.url,ready:'complete'};
      if(expression.includes('prepareEvidenceTargets'))return[];
      if(expression.includes('XHeadlessPoC.collect()'))return JSON.stringify({posts:[{id:'instagram:p:Fixture',permalink:fixtureURL,author:'fixture.author',text:'Synthetic IG post',media:[]}],documentReady:true,authenticatedUiObserved:true,scroll:{y:0,height:1000,viewportHeight:900}});
      return undefined;
    },
    async send(){return{};},
    async close(){this.closed++;if(this.closeError)throw Error('PRIVATE_TOKEN');return{...owner,closed:true};},
  });
  const feed=page();
  const collector={binding:owner,forSource:async source=>{assert.equal(source,'instagram');return feed;},createTemporaryPage:async()=>{const p=page();state.pages.push(p);return p;}};
  const observePair=async phase=>{
    state.proofs.push(phase);
    const result={domain:'synthetic-fixture',source:'instagram',generation:1,attempt:binding.attempt,
      reader:{profileId:binding.readerProfileId,accountId:binding.accountId,revision:'fixture-revision',authenticated:true,observedAt:state.time},
      collector:{profileId:binding.collectorProfileId,accountId:binding.accountId,revision:'fixture-revision',authenticated:true,observedAt:state.time}};
    options.mutateProof?.(result,phase);return result;
  };
  const adapter=createSourceConcurrentAdapter({source:'instagram',binding,collector,observePair,
    loadAssets:source=>sourceAssets(fileURLToPath(new URL('../../AkuBridge/',import.meta.url)),source),now:()=>state.time,...options});
  return{adapter,state,collector,feed,binding,owner,page};
}
const observation={source:'instagram',snapshots:[],coverage:{}};
const rejects=(stage,reason)=>error=>{
  const diagnostic=sourceConcurrentDiagnostic(error);assert.equal(diagnostic.stage,stage);assert.equal(diagnostic.reason,reason);assert(!JSON.stringify(diagnostic).includes('PRIVATE'));return true;
};

test('IG adapter reuses actual asset loader and capture/parser pipeline without a reader capability',async()=>{
  const h=setup();const result=await h.adapter.capture({pageUrl:fixtureURL,scrolls:0,sourceHydrationTimeoutMs:1000,captureTimeoutMs:5000});
  assert.equal(result.source,'instagram');assert.equal(result.snapshots[0].blocks[0].text,'Synthetic IG post');assert.equal(h.feed.url,fixtureURL);
  assert.deepEqual(h.state.proofs,['before','after']);assert.equal(h.state.readerClosed,0);assert.equal(h.adapter.status().cleanup,'verified');assert.equal(h.adapter.capabilities.nativeQualified,false);
  await assert.rejects(h.adapter.capture({}),rejects('capture','replay'));
});

test('IG media recovery uses an owned temporary collector page and verifies its close receipt',async()=>{
  const resolver={available:true,functionSource:resolveInstagramStructuredMediaInMainWorld.toString(),runtimeRevision:'instagram-main-world-media-resolver-v2'};
  const feedResolver={available:true,functionSource:resolveInstagramStructuredFeedInMainWorld.toString(),runtimeRevision:'instagram-main-world-feed-resolver-v1'};
  let h;
  h=setup({runCapture:async browser=>{
    const recovery=createInstagramVideoRecovery({browser,resolver,feedResolver,deadlineAt:Date.now()+5000,
      inspectSnapshot:async()=>({posts:[],documentReady:true})});
    const result=await recovery.recoverSnapshot({posts:[{id:'instagram:p:Fixture',permalink:fixtureURL,author:'fixture.author',text:'',contentKind:'video',media:[{kind:'video_poster',url:'https://s.cdninstagram.com/poster.jpg'}],mediaExpected:['video']}]});
    await recovery.close();
    h.state.recoveryResult=result;
    return observation;
  }});
  h.collector.createTemporaryPage=async()=>{
    const p=h.page();const original=p.evaluate;
    p.evaluate=async function(expression){
      if(expression.includes('function resolveInstagramStructured')){
        const native={code:'Fixture',user:{username:'fixture.author'},caption:{text:'Synthetic video'},media_type:2,
          image_versions2:{candidates:[{url:'https://s.cdninstagram.com/poster.jpg'}]},video_versions:[{url:'https://s.cdninstagram.com/playback.mp4'}]};
        return vm.runInNewContext(expression,{document:{querySelectorAll:()=>[{textContent:JSON.stringify({require:[{data:[native]}]})}]},URL,TextEncoder});
      }
      return original.call(this,expression);
    };
    h.state.pages.push(p);return p;
  };
  await h.adapter.capture({});assert.equal(h.state.recoveryResult.posts[0].mediaEvidence.nativeVideoFallback.outcome,'owned_playback_url_observed');assert.equal(h.state.pages.length,1);assert.equal(h.state.pages[0].closed,1);assert.equal(h.state.readerClosed,0);
});

for(const name of ['source','profile','account','generation','attempt','revision','stale','logged_out','native_domain']){
  test(`IG rejects ${name} proof before capture`,async()=>{
    let captures=0;
    const h=setup({runCapture:async()=>{captures++;return observation;},mutateProof:proof=>{
      if(name==='source')proof.source='x';
      if(name==='profile')proof.collector.profileId='reader-profile';
      if(name==='account')proof.collector.accountId='different';
      if(name==='generation')proof.generation++;
      if(name==='attempt')proof.attempt='different';
      if(name==='revision')proof.collector.revision='different';
      if(name==='stale')proof.reader.observedAt-=6001;
      if(name==='logged_out')proof.collector.authenticated=false;
      if(name==='native_domain')proof.domain='real-instagram';
    }});
    await assert.rejects(h.adapter.capture({}),error=>{assert.equal(sourceConcurrentDiagnostic(error).stage,'proof');return true;});assert.equal(captures,0);
  });
}

test('changed post-capture evidence prevents publishing an otherwise valid observation',async()=>{
  const h=setup({runCapture:async()=>observation,mutateProof:(proof,phase)=>{if(phase==='after')proof.reader.accountId='other';}});
  await assert.rejects(h.adapter.capture({}),rejects('proof','identity'));
});

test('same-account session revision changes and unsafe exception getters cannot bypass cleanup',async()=>{
  const h=setup({runCapture:async()=>observation,mutateProof:(proof,phase)=>{if(phase==='after')proof.reader.revision=proof.collector.revision='rotated';}});
  await assert.rejects(h.adapter.capture({}),rejects('proof','revision_changed'));
  let second;
  second=setup({runCapture:async browser=>{await browser.createTemporaryPage();const error=Error('PRIVATE');Object.defineProperty(error,'code',{get(){throw Error('PRIVATE_GETTER');}});throw error;}});
  await assert.rejects(second.adapter.capture({}),rejects('capture','worker'));assert.equal(second.state.pages[0].closed,1);
});

test('source and navigation boundaries reject cross-source capture without affecting the reader',async()=>{
  const h=setup({runCapture:async browser=>{await assert.rejects(browser.forSource('x'),rejects('capture','source'));const p=await browser.forSource('instagram');assert.throws(()=>p.navigate('https://x.com/home'),rejects('target','url'));assert.throws(()=>p.send('Browser.close'),rejects('worker','method'));return observation;}});
  await h.adapter.capture({});assert.equal(h.state.readerClosed,0);
});

test('cleanup failure is separate and cannot replace the first capture failure',async()=>{
  let h;
  h=setup({runCapture:async browser=>{const p=await browser.createTemporaryPage();h.state.pages[0].closeError=true;void p;throw Error('PRIVATE_COOKIE');}});
  await assert.rejects(h.adapter.capture({}),error=>{const d=sourceConcurrentDiagnostic(error);assert.equal(d.stage,'capture');assert.equal(d.reason,'worker');assert.equal(d.cleanup,'uncertain');assert(!error.message.includes('PRIVATE'));return true;});
  assert.equal(h.state.pages[0].closed,1);assert.equal(h.state.readerClosed,0);
});

test('a reader-scoped media page never grants authority to close the reader',async()=>{
  let h;
  h=setup({runCapture:async browser=>{await browser.createTemporaryPage();return observation;}});
  h.collector.createTemporaryPage=async()=>({...h.page(),binding:{...h.owner,profileId:h.binding.readerProfileId},close:async()=>{h.state.readerClosed++;}});
  await assert.rejects(h.adapter.capture({}),rejects('target','scope'));assert.equal(h.state.readerClosed,0);assert.equal(h.adapter.status().cleanup,'uncertain');
});

test('deadline and cancellation block capture without renewing the original batch limit',async()=>{
  let h;h=setup({runCapture:async()=>{h.state.time=h.binding.deadlineMS;return observation;}});
  await assert.rejects(h.adapter.capture({}),rejects('capture','deadline'));
  const canceled=setup({runCapture:async()=>observation});const abort=new AbortController();abort.abort();
  await assert.rejects(canceled.adapter.capture({},{signal:abort.signal}),rejects('capture','canceled'));
});

test('late media creation after cancellation keeps its cleanup obligation without publishing evidence',async()=>{
  let h,created,resolvePage;
  const started=new Promise(resolve=>{created=resolve;});
  h=setup({cleanupMS:25,runCapture:async browser=>{await browser.createTemporaryPage();return observation;}});
  h.collector.createTemporaryPage=()=>{created();return new Promise(resolve=>{resolvePage=resolve;});};
  const abort=new AbortController();const pending=h.adapter.capture({},{signal:abort.signal});
  await started;abort.abort();
  await assert.rejects(pending,error=>{const d=sourceConcurrentDiagnostic(error);assert.equal(d.reason,'canceled');assert.equal(d.cleanup,'uncertain');return true;});
  const late=h.page();resolvePage(late);await new Promise(resolve=>setImmediate(resolve));
  assert.equal(late.closed,1);assert.equal(h.state.readerClosed,0);assert.equal(h.adapter.status().cleanup,'uncertain');
});

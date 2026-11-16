// Experimental harness: unmodified production extractor in an isolated CDP world.
// This compares extraction on the same headless DOM, not installed/headed runtime parity.
import { readFile, writeFile, mkdir, realpath } from 'node:fs/promises';
import { resolve, relative, isAbsolute } from 'node:path';
import { createHash, randomUUID } from 'node:crypto';
import { root, workspace, launchChrome } from './chrome.mjs';
import { loadScripts, inject, collect } from './capture.mjs';
import { resolveFacebookStructuredMediaInMainWorld } from '../../../AkuBridge/facebook-main-world-media-resolver.js';

const delay = ms => new Promise(r => setTimeout(r, ms));
const productionFiles = ['bounded-capture-policy.js', 'capture-quality-policy.js',
  'source-adapter-runtime.js', 'media-post-processor.js', 'adapters/facebook-adapter.js',
  'source-freshness-runtime.js', 'media-acquisition-engine.js', 'content-script.js'];
export async function productionScripts() {
  return Promise.all(productionFiles.map(async name => {
    const file = `AkuBridge/${name}`, content = await readFile(resolve(workspace, file), 'utf8');
    return { file, content, sha256: createHash('sha256').update(content).digest('hex') };
  }));
}
export async function productionContext(browser, scripts = null) {
  const { frameTree } = await browser.send('Page.getFrameTree');
  const { executionContextId } = await browser.send('Page.createIsolatedWorld', {
    frameId: frameTree.frame.id, worldName: 'aku-facebook-production-comparison', grantUniveralAccess: false,
  });
  async function evaluate(expression) {
    const result = await browser.send('Runtime.evaluate', {
      expression, contextId: executionContextId, awaitPromise: true, returnByValue: true,
    });
    if (result.exceptionDetails) throw new Error(result.exceptionDetails.exception?.description || result.exceptionDetails.text);
    return result.result.value;
  }
  // Only the listener and bounded extension timer are emulated. No service worker,
  // storage, Sidecar observations, feed changes or extension reload are dispatched.
  await evaluate(`globalThis.chrome = { runtime: {
    onMessage: { addListener(fn) { globalThis.__comparisonListener=fn; }, removeListener() {} },
    async sendMessage(message) {
      if(message.type!=='AKU_BROWSER_CAPTURE_DELAY') throw new Error('Unsupported comparison runtime message');
      await new Promise(resolve=>setTimeout(resolve,Math.min(2000,Math.max(0,message.milliseconds||0))));
      return {ok:true};
    }
  }};`);
  for (const script of scripts || await productionScripts()) await evaluate(script.content);
  return { evaluate, executionContextId };
}
export async function collectProduction(browser, context) {
  // Same self-contained function used by production MAIN-world resolution.
  const structured = await browser.evaluate(`(${resolveFacebookStructuredMediaInMainWorld.toString()})({
    maxCandidates:16,maxScripts:32,maxScriptBytes:256000,maxTotalBytes:2000000,maxTraversalNodes:20000,maxDepth:40
  })`);
  const payload = { source:'facebook',scrolls:0,maxBlocksPerSnapshot:20,maxBlockCharacters:4000,
    captureTimeoutMs:15000,qualityRetryBudget:1,qualityRetrySettleMs:300,
    pendingContentPolicy:'detect_only',sameTabMutationAllowed:false,
    tabAcquisition:{captureVisibilityMode:'headless_comparison'},
    structuredMediaRequest:{requestId:randomUUID(),waitMs:400} };
  const result = JSON.parse(await context.evaluate(`new Promise((resolve,reject)=>{
    globalThis.__comparisonListener({type:'AKU_BROWSER_COLLECT_VISIBLE',payload:${JSON.stringify(payload)}},{},
      response=>response.ok?resolve(JSON.stringify(response.observation)):reject(new Error(response.message)));
    globalThis.__comparisonListener({type:'AKU_BROWSER_STRUCTURED_MEDIA_READY',requestId:${JSON.stringify(payload.structuredMediaRequest.requestId)},
      evidence:${JSON.stringify(structured)}},{},()=>{});
  })`));
  return result;
}
const normalizedText = text => String(text||'').replace(/\s+/g,' ').trim();
const idOf = post => post?.id || post?.platformId || null;
const mediaKey = media => `${/video/.test(media.kind)?'video':'image'}:${media.posterUrl||media.url}`;
export function assessBinding(posts, targetId, baseline = null) {
  const candidates=posts.filter(post=>idOf(post)===targetId);
  const distinct=new Set(candidates.map(post=>`${normalizedText(post.author)}\n${normalizedText(post.text)}`));
  const baselineText=normalizedText(baseline?.text);
  const verified=baseline?candidates.filter(post=>normalizedText(post.author)===normalizedText(baseline.author)
    &&baselineText&&normalizedText(post.text)&&(baselineText.startsWith(normalizedText(post.text))
      ||normalizedText(post.text).startsWith(baselineText))):distinct.size===1?candidates:[];
  const signatures=new Set(verified.map(post=>`${normalizedText(post.author)}\n${normalizedText(post.text)}`));
  return {identityMatches:candidates.length,distinctAuthorTextBindings:distinct.size,ambiguous:distinct.size>1,
    verifiedAgainstBaseline:baseline?verified.length:null,
    post:signatures.size===1?verified[0]:null};
}
export function comparePosts(poc, production, targetId, baseline = null) {
  const pBinding=assessBinding(poc.posts,targetId,baseline);
  const qBinding=assessBinding(production.snapshots.flatMap(snapshot=>snapshot.blocks),targetId,baseline);
  const {post:p,...pocBinding}=pBinding,{post:q,...productionBinding}=qBinding;
  const binding={pocBinding,productionBinding,targetMatchedPoc:Boolean(p),targetMatchedProduction:Boolean(q)};
  if(!p || !q) return {...binding,comparable:false,reason:'identity_without_verified_author_text_binding'};
  const pKeys=new Set(p.media.map(mediaKey)),qKeys=new Set(q.media.map(mediaKey));
  const textP=normalizedText(p.text),textQ=normalizedText(q.text);
  return {...binding,comparable:true,
    authorEqual:p.author===q.author,permalinkEqual:p.permalink===q.permalink,
    normalizedTextEqual:textP===textQ,pocTextCharacters:p.text.length,productionTextCharacters:q.text.length,
    productionTextIsPocPrefix:textP.startsWith(textQ),
    pocTextCollapsed:p.textCollapsed,productionExpansion:q.presentation?.contentExpansion,
    pocMedia:p.media.length,productionMedia:q.media.length,
    exactMediaIntersection:[...pKeys].filter(key=>qKeys.has(key)).length,
    exactMediaSetsEqual:pKeys.size===qKeys.size&&[...pKeys].every(key=>qKeys.has(key)),
    pocPlaybackUrls:p.media.filter(media=>media.playbackUrl).length,
    productionPlaybackUrls:q.media.filter(media=>media.playbackUrl).length,
    pocTimestampAvailable:Boolean(p.publishedAt),productionTimestampAvailable:Boolean(q.publishedAt),
    pocTimestampSource:p.timestampSource,productionTimestampSource:q.presentation?.timestampSource,
    pocTimestampEstimated:p.timestampEstimated,productionTimestampEstimated:q.presentation?.timestampEstimated,
    pocRelationship:p.relationshipType,productionRelationship:q.relationshipType,
    productionQuality:q.captureQuality?.verdict||null};
}
function nativeId(url) {
  const u = new URL(url), value=u.pathname.match(/\/(?:posts|videos|reel)\/(pfbid[A-Za-z0-9]+|\d+)(?:\/|$)/)?.[1]
    || u.searchParams.get('story_fbid');
  return value?`facebook:post:${value}`:null;
}
export async function loadComparisonTargets(file) {
  const path=await realpath(resolve(file)),runs=await realpath(resolve(root,'runs'));
  const child=relative(runs,path);
  if(!child||child.startsWith('..')||isAbsolute(child))throw new Error('Comparison baseline must be an existing report inside this package runs.');
  const previous=JSON.parse(await readFile(path,'utf8'));
  if(previous.schema!=='aku.facebook-extractor-comparison.v1')throw new Error('Unsupported comparison baseline.');
  const targets=previous.cases.map(({category,id,url,baselineCreatedAt,baseline,status})=>({category,id,url,baselineCreatedAt,baseline,status}));
  if(!targets.length||targets.length>4)throw new Error('Expected one to four bounded baseline cases.');
  for(const target of targets.filter(t=>t.url)){
    const u=new URL(target.url);
    if(!['facebook.com','www.facebook.com'].includes(u.hostname)||u.protocol!=='https:'||u.username||u.password||u.port
      ||nativeId(target.url)!==target.id||target.baseline?.platformId!==target.id
      ||typeof target.baseline.author!=='string'||typeof target.baseline.text!=='string')throw new Error('Invalid native comparison baseline target.');
  }
  return targets;
}
export async function selectTargets() {
  const rows=[],seen=new Set();
  for(let offset=0;offset<1000;offset+=50){
    const response=await fetch(`http://127.0.0.1:11122/api/timeline?limit=50&offset=${offset}`,{signal:AbortSignal.timeout(10000)});
    if(!response.ok)throw new Error('Timeline target selection failed: HTTP '+response.status);
    const page=await response.json();let fresh=0;
    for(const item of page.items||[])if(!seen.has(item.id)){seen.add(item.id);fresh++;if(item.source==='facebook')rows.push(item);}
    if(!fresh)break;
  }
  const candidates=rows.filter(item=>{
    const e=item.evidence;
    try{const u=new URL(e.permalink);return ['facebook.com','www.facebook.com'].includes(u.hostname)
      &&u.protocol==='https:'&&!u.username&&!u.password&&!u.port&&e.platformId&&nativeId(e.permalink)===e.platformId;}catch{return false;}
  }).sort((a,b)=>b.createdAt.localeCompare(a.createdAt));
  const selected=[],used=new Set();
  for(const [category,predicate] of [
    ['long_text',e=>e.text.length>=700],['multi_image',e=>e.media.filter(m=>m.kind==='image').length>=2],
    ['video_candidate',e=>e.contentKind==='video'||e.media.some(m=>/video/.test(m.kind))],
    ['shared_post',e=>e.relationshipType==='repost'],
  ]){
    const item=candidates.find(item=>!used.has(item.evidence.platformId)&&predicate(item.evidence));
    if(item){const e=item.evidence;used.add(e.platformId);selected.push({category,id:e.platformId,url:e.permalink,
      baselineCreatedAt:item.createdAt,baseline:e});}
    else selected.push({category,status:'no_saved_native_target'});
  }
  return selected;
}
export async function captureComparison(options, registration, targets) {
  const started=Date.now(),report={schema:'aku.facebook-extractor-comparison.v1',source:'facebook-comparison',
    startedAt:new Date().toISOString(),status:'starting',headless:true,posts:[],samples:[],cases:[],
    scope:'production_extractor_and_poc_on_same_headless_dom',
    limitations:['not_installed_extension_runtime','not_headed_vs_headless_parity','sequential_dom_may_hydrate',
      'no_sidecar_ingestion','no_full_feed_completeness_claim','no_zero_os_blinking_claim']};
  const dir=resolve(root,'runs',new Date().toISOString().replace(/[:.]/g,'-')+'-'+process.pid+'-facebook-comparison');
  await mkdir(dir,{recursive:true});let browser,interrupted=false;
  const interrupt=()=>{interrupted=true;void browser?.close().catch(()=>{});};
  process.once('SIGINT',interrupt);process.once('SIGTERM',interrupt);
  try{
    if(!Array.isArray(targets)||!targets.some(target=>target.url))throw new Error('Select comparison targets before stopping Sidecar.');
    const production=await productionScripts(),pocScripts=await loadScripts('facebook');
    const resolverFile='AkuBridge/facebook-main-world-media-resolver.js';
    const resolver=await readFile(resolve(workspace,resolverFile),'utf8');
    report.sources=[...production,...pocScripts].map(({file,sha256})=>({file,sha256}));
    report.sources.push({file:resolverFile,sha256:createHash('sha256').update(resolver).digest('hex')});
    browser=await launchChrome({existingCapture:registration,timeoutMs:60000});
    report.browser={...browser.version,binarySha256:browser.binarySha256};
    await browser.send('Emulation.setDeviceMetricsOverride',{width:1280,height:900,deviceScaleFactor:1,mobile:false});
    for(const target of targets){
      if(interrupted)throw new Error('Comparison interrupted');
      if(!target.url){report.cases.push(target);continue;}
      console.log(JSON.stringify({comparisonStage:'case_started',category:target.category}));
      const result={...target,order:['production_before','poc_with_hover','production_after'],startedAt:new Date().toISOString()};
      try{
        const navigation=await browser.send('Page.navigate',{url:target.url});
        if(navigation.errorText)throw new Error('Target navigation failed: '+navigation.errorText);
        const deadline=Date.now()+options.waitMs;
        while(Date.now()<deadline){
          if(interrupted)throw new Error('Comparison interrupted');
          await delay(750);
          try{if(await browser.evaluate(`document.readyState==='complete'&&Boolean(document.querySelector('main [role="article"],div[aria-posinset],input[name="email"],input[name="pass"]'))`))break;}
          catch(error){if(!/context.*destroyed|Cannot find context/i.test(error.message))throw error;}
        }
        await delay(1500);
        await inject(browser,pocScripts);
        const context=await productionContext(browser,production);
        result.productionBefore=await collectProduction(browser,context);
        result.poc=await collect(browser);
        result.productionAfter=await collectProduction(browser,context);
        result.comparisonBefore=comparePosts(result.poc,result.productionBefore,target.id,target.baseline);
        result.comparisonAfter=comparePosts(result.poc,result.productionAfter,target.id,target.baseline);
        result.authenticated=result.poc.authenticatedUiObserved&&!result.poc.loginRequired&&!result.poc.challengeDetected;
        report.posts.push(...result.poc.posts);const {posts,...sample}=result.poc;report.samples.push(sample);
        if(!result.authenticated){result.status='authentication_unverified';report.cases.push(result);break;}
        result.status=result.comparisonBefore.comparable?'paired':result.comparisonAfter.comparable?'paired_after_poc_hydration':'target_binding_unverified';
      }catch(error){result.status='error';result.error=error.message;}
      result.completedAt=new Date().toISOString();report.cases.push(result);
      console.log(JSON.stringify({comparisonStage:'case_completed',category:target.category,status:result.status,
        comparisonBefore:result.comparisonBefore,comparisonAfter:result.comparisonAfter}));
    }
    report.status=report.cases.some(c=>c.comparisonBefore?.comparable||c.comparisonAfter?.comparable)?'captured_partial':'empty_unverified';
  }catch(error){report.status='error';report.error=error.message;}
  finally{
    process.removeListener('SIGINT',interrupt);process.removeListener('SIGTERM',interrupt);
    if(browser){report.documentResponses=browser.documentResponses();report.mediaResponses=browser.mediaResponses();
      try{await browser.close();}catch(error){report.shutdownError=error.message;}}
    report.durationMs=Date.now()-started;const output=resolve(dir,'capture.json');
    await writeFile(output,JSON.stringify(report,null,2)+'\n');
    console.log(JSON.stringify({status:report.status,posts:report.posts.length,durationMs:report.durationMs,output}));
  }
  return report;
}

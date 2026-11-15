import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile,writeFile,mkdtemp,mkdir} from 'node:fs/promises';
import {spawnSync} from 'node:child_process';
import {fileURLToPath,pathToFileURL} from 'node:url';
import {join} from 'node:path';
import {sourceAssets} from '../worker.mjs';
import {captureIsolated,NATIVE_CAPTURE_CAPABILITIES} from '../capture.mjs';

const workspace=fileURLToPath(new URL('../../../../../../',import.meta.url));
const bridge=join(workspace,'AkuBridge');
const assets=await sourceAssets(bridge,'x');
const contentScript=await readFile(join(bridge,'content-script.js'),'utf8');
const expandSource=contentScript.match(/^  async function expandSourceContent\([\s\S]*?^  }/m)?.[0];
const cleanSource=contentScript.match(/^  function cleanExpandedText\([\s\S]*?^  }/m)?.[0];
const chrome=process.env.AKU_OFFLINE_CHROME;
const permalink='https://x.com/fixture/status/1234500000000000022';
const quotePermalink='https://x.com/quote/status/1234500000000000023';
const wrongPermalink='https://x.com/other/status/1234500000000000024';
const excerpt='The owned caption starts here.';
const full=excerpt+'\n\n'+('Remaining authored paragraph 🌱. '.repeat(12)).trim()+'\nThe author ends with Show more';

// The same reconstructed post_wrapper DOM is read by the actual registered X
// adapter in a real offline DOM. No selector/identity/reader is stubbed here.
async function browserFixture(spec) {
  const location=new URL('https://x.com/home');
  const window={location};
  for(const code of spec.assets)new Function('window','location',code)(window,location);
  const adapter=globalThis.AkuSourceAdapters.get('x');
  const primitives=globalThis.AkuCapturePrimitives;
  const compactText=value=>String(value?.innerText??value??'').replace(/\s+/g,' ').trim();
  const expand=new Function('sourceAdapters','capturePrimitives','compactText','structuredText','delay','window',
    spec.cleanSource+'\n'+spec.expandSource+'\nreturn expandSourceContent;')(
      globalThis.AkuSourceAdapters,primitives,compactText,primitives.structuredText,
      ms=>new Promise(resolve=>setTimeout(resolve,ms)),window);
  const main=document.querySelector('main');
  const wrap=(href,text,control=true,quote='')=>`<div role="link" data-href="${new URL(href).pathname}"><article class="group/post">
    <div class="x-middot-group"><a href="https://x.com/${new URL(href).pathname.split('/')[1]}">Fixture Author</a></div>
    <div dir="auto" class="whitespace-pre-wrap"><span>${text}</span>${control?'<button type="button">Show more</button>':''}</div>${quote}</article></div>`;
  const cases=[
    ['feed','https://x.com/home'],['detail',spec.permalink],['wrong_detail',spec.wrongPermalink],
    ['quote_detail',spec.quotePermalink],['stalled',spec.permalink],['changed_route',spec.permalink],
    ['changed_owner',spec.permalink],['missing_identity',spec.permalink],
    ['authored_label',spec.permalink],['quote_control_only',spec.permalink],
  ];
  const rows=[];
  for(const [name,route]of cases) {
    location.href=route;
    const text=name==='authored_label'?'The author writes Show more':spec.excerpt;
    const quote=wrap(spec.quotePermalink,'Quoted text stays separate.',true);
    main.innerHTML=wrap(spec.permalink,text,!['authored_label','quote_control_only'].includes(name),quote);
    const article=main.querySelector('article');
    if(name==='missing_identity')article.parentElement.removeAttribute('data-href');
    const primaryRoot=adapter.resolveContentRoot(article);
    const button=primaryRoot?.querySelector('button');
    let primaryClicks=0,quoteClicks=0;
    for(const candidate of main.querySelectorAll('button'))candidate.addEventListener('click',()=>{
      if(candidate!==button){quoteClicks++;return;}
      primaryClicks++;
      if(name==='stalled')return;
      primaryRoot.textContent=spec.full;
      if(name==='changed_route')location.href=spec.wrongPermalink;
      if(name==='changed_owner'){
        article.parentElement.setAttribute('data-href',new URL(spec.wrongPermalink).pathname);
        article.querySelector('.x-middot-group a').href='https://x.com/other';
      }
    });
    globalThis.AkuHeadlessCapturePolicy={allowContentExpansion:spec.mode!=='native',deadlineAt:Date.now()+2000};
    let expansion=null,snapshot=null;
    if(spec.mode==='windowed')expansion=await expand(article,'x',Date.now()+2000);
    else snapshot=await globalThis.XHeadlessPoC.collect();
    const authored=adapter.extractText(article,{compactText,structuredText:primitives.structuredText});
    rows.push({name,primaryClicks,quoteClicks,text:authored,expansionState:expansion?.state,
      textCompleteness:expansion?primitives.textCompleteness({expansionState:expansion.state}):snapshot.posts[0]?.textCompleteness,
      textStatus:snapshot?.posts[0]?.textStatus,snapshot});
  }
  return rows;
}

test('real X wrapped DOM shares authored reading, owner guards and native detect-only behavior',
  {skip:!chrome&&'Set AKU_OFFLINE_CHROME to run the isolated real-DOM fixture.'},async()=>{
  assert.ok(expandSource&&cleanSource);
  const root=join(workspace,'AkuSidecar','build','a3-shared-text-20261115');
  await mkdir(root,{recursive:true});
  const scratch=await mkdtemp(join(root,'real-dom-'));
  const results={};
  for(const mode of ['windowed','headless','native']) {
    const spec={mode,permalink,quotePermalink,wrongPermalink,excerpt,full,expandSource,cleanSource,
      assets:assets.filter(asset=>asset.execute).map(asset=>asset.content)};
    const script=`(${browserFixture.toString()})(${JSON.stringify(spec)}).then(rows=>{
      document.getElementById('result').textContent=JSON.stringify({rows});
    }).catch(error=>{document.getElementById('result').textContent=JSON.stringify({error:String(error.stack||error)});});`;
    const fixture=join(scratch,mode+'.html');
    await writeFile(fixture,'<!doctype html><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src \'none\'; script-src \'unsafe-inline\' \'unsafe-eval\'; style-src \'unsafe-inline\'">'+
      '<base href="https://x.com/home"><style>article{min-height:32px}</style><main></main><pre id="result"></pre><script>'+script.replaceAll('</script','<\\/script')+'</script>');
    const run=spawnSync(chrome,['--headless','--dump-dom','--virtual-time-budget=15000','--disable-background-networking',
      '--disable-component-update','--no-first-run','--no-default-browser-check','--host-resolver-rules=MAP * ~NOTFOUND',
      '--window-size=1200,1000','--user-data-dir='+join(scratch,mode+'-profile'),pathToFileURL(fixture).href],
      {encoding:'utf8',timeout:30000,maxBuffer:8*1024*1024,windowsHide:true});
    assert.equal(run.status,0,String(run.error??run.stderr).slice(-1200));
    const encoded=run.stdout.match(/<pre id="result">([\s\S]*?)<\/pre>/)?.[1];
    assert.ok(encoded,run.stdout.slice(-1500));
    const result=JSON.parse(encoded.replaceAll('&lt;','<').replaceAll('&gt;','>').replaceAll('&amp;','&'));
    assert.ok(!result.error,result.error);
    results[mode]=Object.fromEntries(result.rows.map(row=>[row.name,row]));
  }
  await writeFile(join(scratch,'results.json'),JSON.stringify(results,null,2));
  for(const mode of Object.keys(results)) {
    for(const row of Object.values(results[mode]))assert.equal(row.quoteClicks,0,mode+' '+row.name);
    assert.equal(results[mode].authored_label.text,'The author writes Show more');
    assert.equal(results[mode].authored_label.textCompleteness,'no_collapse_observed');
    assert.equal(results[mode].quote_control_only.text,excerpt);
    assert.equal(results[mode].quote_control_only.textCompleteness,'no_collapse_observed');
    assert.equal(results[mode].missing_identity.primaryClicks,0);
    for(const name of ['wrong_detail','quote_detail'])assert.equal(results[mode][name].primaryClicks,0,mode+' '+name);
  }
  for(const mode of ['headless','native']){
    assert.equal(results[mode].feed.snapshot.posts[0].quotedPost.text,'Quoted text stays separate.');
    assert.equal(results[mode].feed.snapshot.posts[0].quotedPost.textCompleteness,'partial');
  }
  for(const mode of ['windowed','headless']) {
    assert.equal(results[mode].detail.text,full,mode);
    assert.equal(results[mode].detail.textCompleteness,'expanded',mode);
    assert.equal(results[mode].detail.primaryClicks,1,mode);
    assert.equal(results[mode].stalled.text,excerpt,mode);
    assert.equal(results[mode].stalled.textCompleteness,'partial',mode);
    assert.equal(results[mode].changed_route.textCompleteness,'partial',mode);
    assert.equal(results[mode].changed_owner.textCompleteness,'partial',mode);
  }
  assert.equal(results.windowed.feed.text,full);
  assert.equal(results.headless.feed.primaryClicks,0);
  assert.equal(results.headless.feed.textStatus,'requires_permalink_capture');
  for(const name of ['feed','detail','wrong_detail','quote_detail','stalled','changed_route','changed_owner']) {
    assert.equal(results.native[name].primaryClicks,0,name);
    assert.equal(results.native[name].textCompleteness,'partial',name);
  }
  // Compose actual orchestration with the real-DOM native snapshot. The page
  // transport is synthetic; this does not exercise daily owner RPC or live X.
  let pages=0;
  const page={navigate:async()=>({}),send:async()=>({}),evaluate:async expression=>
    expression.includes('XHeadlessPoC.collect()')?JSON.stringify(results.native.feed.snapshot):
      expression==='location.href'?'https://x.com/home':undefined};
  const observation=await captureIsolated({backend:'headless_worker',captureCapabilities:NATIVE_CAPTURE_CAPABILITIES.x,
    forSource:async()=>page,createTemporaryPage:async()=>{pages++;throw Error('native detail page attempted');}},
    {x:assets},'x',{scrolls:0,captureTimeoutMs:3000,sourceHydrationTimeoutMs:1000,
      sameTabMutationAllowed:true,pendingContentPolicy:'reveal_if_present',sourceFreshnessPolicy:'wake_and_reveal'});
  assert.equal(pages,0);
  assert.equal(observation.snapshots[0].blocks[0].text,excerpt);
  assert.equal(observation.snapshots[0].blocks[0].captureQuality.textCompleteness,'partial');
  assert.ok(observation.snapshots[0].blocks[0].captureQuality.limitations.includes('permalink_capture_unavailable'));
});

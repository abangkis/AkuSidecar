// Accountless full embedded-app acceptance; launched under the Go-owned tree.
import assert from "node:assert/strict";
import { writeFile } from "node:fs/promises";
import { join } from "node:path";
import { launchChrome } from "../internal/collection/headless/worker/chrome.mjs";
const [chromePath, root, artifacts, origin] = process.argv.slice(2);
assert.ok(chromePath && root && artifacts && /^http:\/\/127\.0\.0\.1:\d+$/.test(origin));
let browser;
const results=[];
try {
 browser=await launchChrome({chromePath,profilePath:join(root,'profile')});const page=await browser.createTemporaryPage();const errors=[];
 page.on(event=>{if(event.method==='Runtime.exceptionThrown')errors.push(event.params.exceptionDetails?.exception?.description||event.params.exceptionDetails?.text||'exception');});
 async function until(expression){for(let i=0;i<180;i++){if(await page.evaluate(expression))return;await new Promise(r=>setTimeout(r,50));}const diagnostic=await page.evaluate(`({notice:document.querySelector('#provider-notice')?.textContent,dialogs:[...document.querySelectorAll('dialog[open]')].map(x=>x.id),buttons:[...document.querySelectorAll('.feedback-actions button')].map(x=>({text:x.textContent,disabled:x.disabled,pressed:x.getAttribute('aria-pressed')}))})`);await shot('failure');throw Error('UI state timeout: '+expression+' exceptions='+JSON.stringify(errors)+' diagnostic='+JSON.stringify(diagnostic));}
 async function settleLayout(){await page.evaluate(`new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(()=>resolve(true))))`);}
 async function click(selector){await settleLayout();await page.evaluate(`document.querySelector(${JSON.stringify(selector)}).scrollIntoView({block:'center',behavior:'instant'})`);await settleLayout();const p=await page.evaluate(`(()=>{const r=document.querySelector(${JSON.stringify(selector)}).getBoundingClientRect();return{x:r.x+r.width/2,y:r.y+r.height/2}})()`);await page.send('Input.dispatchMouseEvent',{type:'mousePressed',button:'left',clickCount:1,...p});await page.send('Input.dispatchMouseEvent',{type:'mouseReleased',button:'left',clickCount:1,...p});}
 async function shot(name){await writeFile(join(artifacts,name+'.png'),Buffer.from((await page.send('Page.captureScreenshot',{format:'png'})).data,'base64'));}
 for(const[label,width,height]of[['desktop',1440,1000],['mobile',390,844]]){
  await page.send('Emulation.setDeviceMetricsOverride',{width,height,deviceScaleFactor:1,mobile:label==='mobile'});await page.navigate(origin);await until(`document.querySelector('#runtime-version')?.textContent.includes('go') && document.querySelector('#result-items .presentable-item')`);
  await until(`!document.querySelector('#timeline-panel')?.classList.contains('hidden')`);
  const more='.feedback-actions > button:first-child',less='.feedback-actions > button:last-child';await click(less);await until(`document.querySelector('${less}')?.getAttribute('aria-pressed')==='true'`);await click(more);await until(`document.querySelector('${more}')?.getAttribute('aria-pressed')==='true'`);
  await page.evaluate(`document.querySelector('${more}').focus()`);assert.equal(await page.evaluate(`document.activeElement===document.querySelector('${more}')`),true);
  await shot(label+'-timeline');
  await page.send('Page.handleJavaScriptDialog',{accept:true}).catch(()=>{});
  let dialogSeen=false;const unsubscribe=page.on(event=>{if(event.method==='Page.javascriptDialogOpening'){dialogSeen=true;void page.send('Page.handleJavaScriptDialog',{accept:true});}});
  const alreadySaved=await page.evaluate(`document.querySelector('.memory-read-later-button').disabled && document.querySelector('.memory-read-later-button').textContent==='Saved'`);
  if(!alreadySaved){await click('.memory-read-later-button');await until(`document.querySelector('.memory-read-later-button')?.textContent==='Saved'`);assert.equal(dialogSeen,true);}else assert.equal(dialogSeen,false);
  if(typeof unsubscribe==='function')unsubscribe();
  await click('#settings-view-button');await until(`!document.querySelector('#settings-panel').classList.contains('hidden')`);
  await page.evaluate(`(()=>{const field=document.querySelector('#timeline-capacity');field.value=field.value==='12'?'13':'12';field.dispatchEvent(new Event('input',{bubbles:true}));field.dispatchEvent(new Event('change',{bubbles:true}));})()`);
  await until(`!document.querySelector('#save-runtime-settings').disabled`);await shot(label+'-settings-dirty');
  await click('#save-runtime-settings');await until(`document.querySelector('#runtime-settings-status').textContent.startsWith('Saved') && document.querySelector('#save-runtime-settings').disabled`);
  await click('#session-view-button');await until(`document.querySelector('#result-items .presentable-item')`);assert.equal(await page.evaluate(`document.querySelector('.memory-read-later-button').textContent`),'Saved');
  await click('#inbox-view-button');await until(`!document.querySelector('#inbox-panel').classList.contains('hidden')`);
  await click('#library-view-button');await until(`!document.querySelector('#library-panel').classList.contains('hidden') && document.querySelector('#library-results .library-card-shell')`);
  await shot(label+'-library');
  await click('#topics-view-button');await until(`!document.querySelector('#topics-panel').classList.contains('hidden')`);
  await click('#session-view-button');await until(`document.querySelector('#result-items .presentable-item')`);
  await page.evaluate(`document.querySelector('.timeline-content-context-anchor').scrollIntoView({block:'center',behavior:'instant'})`);await settleLayout();
  if(label==='desktop'){
   await until(`document.querySelector('.timeline-content-context-tab').classList.contains('is-visible')`);
   await click('.timeline-content-context-tab');await until(`document.querySelector('#timeline-content-context-drawer').getAttribute('aria-hidden')==='false' && document.querySelector('#timeline-content-context-body').textContent.includes('No related local context found.')`);
   await shot(label+'-context');await click('#timeline-content-context-close');await until(`document.querySelector('#timeline-content-context-drawer').getAttribute('aria-hidden')==='true'`);
  }else assert.equal(await page.evaluate(`getComputedStyle(document.querySelector('.timeline-content-context-tab')).pointerEvents`),'none');
  assert.equal(await page.evaluate('document.documentElement.scrollWidth <= innerWidth'),true);results.push({viewport:label,width,height,bootstrap:true,feedback:true,readLater:true,settingsDirtySave:true,navigation:true,librarySavedRead:true,topicsNavigation:true,context:label==='desktop'?'open-close':'gutter-trigger-hidden',noOverflow:true});
 }
 assert.deepEqual(errors,[]);await page.close();await writeFile(join(artifacts,'result.json'),JSON.stringify({status:'passed',scope:'actual embedded app, real disposable Store/Engine/HTTP, offline provider, no scheduler/capture/profile',results},null,2));console.log(JSON.stringify({status:'passed',results}));
}finally{if(browser)await browser.close();}

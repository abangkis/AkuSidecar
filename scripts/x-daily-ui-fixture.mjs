// Accountless UI acceptance: loopback fixtures, fresh profile, Go-owned process tree.
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { readFile, writeFile } from 'node:fs/promises';
import { resolve, join } from 'node:path';
import { launchChrome } from '../internal/collection/headless/worker/chrome.mjs';

const [chromePath, root, artifacts] = process.argv.slice(2);
assert.ok(chromePath && root && artifacts);
const web = resolve(import.meta.dirname, '../internal/httpapi/web');
const sourceHTML = await readFile(join(web, 'index.html'), 'utf8');
const panel = sourceHTML.match(/<section id="x-daily-controls"[\s\S]*?<\/section>/)?.[0];
assert.ok(panel);
const notices = ['timeline','inbox'].map(surface=>sourceHTML.match(new RegExp('<section id="x-daily-'+surface+'-notice"[\\s\\S]*?<\\/section>'))?.[0]);
assert(notices.every(Boolean));
const fixture = `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="stylesheet" href="/styles.css"></head><body style="padding:20px"><main style="max-width:760px;margin:auto"><h1>Settings</h1><form id="runtime-settings-form"><div class="settings-card">${panel}</div></form>${notices.join('')}</main><script type="module">
import {createXDailyControls} from '/x-daily-controls.js';
const $=s=>document.querySelector(s);
const disabled={selectedMode:'concurrent',authorizationPolicy:'process',maxBatchMs:60000,derivationAuthorized:false,state:'disabled',reason:'process-consent-required'};
window.qa={ctx:{epoch:'synthetic-epoch',development:true},calls:[],disabled};
qa.visits=0;
qa.controls=createXDailyControls({panel:$('#x-daily-controls'),status:$('#x-daily-status'),detail:$('#x-daily-detail'),consent:$('#x-daily-consent'),startButton:$('#x-daily-start'),stopButton:$('#x-daily-stop'),feedback:$('#x-daily-feedback'),readContext:()=>qa.ctx,readActiveSources:()=>['x'],onReviewSettings:()=>qa.visits++,notices:['timeline','inbox'].map(surface=>({panel:$('#x-daily-'+surface+'-notice'),title:$('#x-daily-'+surface+'-notice-title'),detail:$('#x-daily-'+surface+'-notice-detail'),button:$('#x-daily-'+surface+'-notice-settings')})),api:async(path,options)=>{qa.calls.push({path,options});return {xCapture:{...disabled,cleanup:'verified',...(path.endsWith('/start')?{state:'ready',reason:'',derivationAuthorized:true}:{state:'stopped',reason:'opted-out'})}}}});
qa.controls.reset(disabled);window.qaReady=true;
</script></body></html>`;
const allowed = new Set(['/styles.css', '/x-daily-controls.js', '/settings-render.js']);
const server = createServer(async(req,res)=>{
  try {
    if(req.url==='/'){res.setHeader('Content-Type','text/html');return res.end(fixture);}
    if(!allowed.has(req.url)){res.writeHead(404);return res.end();}
    res.setHeader('Content-Type',req.url.endsWith('.css')?'text/css':'text/javascript');
    res.end(await readFile(join(web,req.url.slice(1))));
  }catch{res.writeHead(500);res.end();}
});
await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
let browser;
try {
  browser=await launchChrome({chromePath,profilePath:join(root,'profile')});
  const page=await browser.createTemporaryPage();
  await page.navigate(`http://127.0.0.1:${server.address().port}/`);
  async function until(expression){for(let i=0;i<80;i++){if(await page.evaluate(expression))return;await new Promise(r=>setTimeout(r,50));}throw Error('Synthetic UI state timeout');}
  async function click(selector){const point=await page.evaluate(`(()=>{const e=document.querySelector(${JSON.stringify(selector)});e.scrollIntoView({block:'center',behavior:'instant'});const r=e.getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2}})()`);await page.send('Input.dispatchMouseEvent',{type:'mousePressed',button:'left',clickCount:1,...point});await page.send('Input.dispatchMouseEvent',{type:'mouseReleased',button:'left',clickCount:1,...point});}
  async function screenshot(name){await writeFile(join(artifacts,name+'.png'),Buffer.from((await page.send('Page.captureScreenshot',{format:'png'})).data,'base64'));}
  for(const [label,width,height] of [['desktop',1100,900],['mobile',390,844]]) {
    await page.send('Emulation.setDeviceMetricsOverride',{width,height,deviceScaleFactor:1,mobile:false});
    await until('window.qaReady===true');
    await page.evaluate(`qa.ctx={epoch:'${label}-epoch',development:true};qa.controls.reset(qa.disabled);window.scrollTo(0,0)`);
    assert.equal(await page.evaluate(`getComputedStyle(document.querySelector('#x-daily-controls')).display==='none'`),false,'configured development panel must actually be visible');
    assert.equal(await page.evaluate(`document.querySelector('#x-daily-start').disabled`),true);
    assert.deepEqual(await page.evaluate(`Array.from(new FormData(document.querySelector('form')).keys())`),[],'consent must not be a saved form setting');
    await screenshot('consent-'+label);
    await click('#x-daily-consent');
    assert.equal(await page.evaluate(`document.querySelector('#x-daily-start').disabled`),false);
    const before=await page.evaluate('qa.calls.length');
    await click('#x-daily-start');
    await until(`document.querySelector('#x-daily-status').textContent==='Ready'`);
    assert.equal(await page.evaluate('qa.calls.length'),before+1);
    const request=await page.evaluate('qa.calls.at(-1)');
    assert.equal(request.options.instanceEpoch,label+'-epoch');
    assert.deepEqual(request.options.body,{acknowledgeLimitations:true,allowProcessSessionDerivation:true});
    assert.equal(await page.evaluate(`document.querySelector('#x-daily-consent').checked`),false);
    assert.equal(await page.evaluate(`getComputedStyle(document.querySelector('#x-daily-timeline-notice')).display`),'none');
    await screenshot('ready-'+label);
    await click('#x-daily-stop');
    await until(`document.querySelector('#x-daily-status').textContent.includes('restart required')`);
    assert.equal(await page.evaluate(`document.querySelector('#x-daily-start').disabled&&document.querySelector('#x-daily-stop').disabled`),true);
    assert.ok(await page.evaluate('document.documentElement.scrollWidth<=innerWidth'),'no horizontal overflow');
    await page.evaluate('window.scrollTo(0,0)');
    await screenshot('stopped-'+label);
    await page.evaluate(`qa.controls.reset({...qa.disabled,state:'stopped',reason:'batch-failed',cleanup:'uncertain'});document.querySelector('#x-daily-timeline-notice').scrollIntoView({block:'start',behavior:'instant'})`);
    assert.equal(await page.evaluate(`['timeline','inbox'].every(s=>getComputedStyle(document.querySelector('#x-daily-'+s+'-notice')).display!=='none')`),true);
    assert.ok(await page.evaluate('document.documentElement.scrollWidth<=innerWidth'),'warning must fit the viewport');
    await screenshot('warning-'+label);
    const requests=await page.evaluate('qa.calls.length');
    const visits=await page.evaluate('qa.visits');
    await click('#x-daily-timeline-notice-settings');
    assert.equal(await page.evaluate('qa.visits'),visits+1);
    assert.equal(await page.evaluate('qa.calls.length'),requests,'review action must not start, stop or retry X');
  }
  await page.evaluate(`qa.ctx={epoch:'release',development:false};qa.controls.reset(qa.disabled)`);
  assert.equal(await page.evaluate(`getComputedStyle(document.querySelector('#x-daily-controls')).display`),'none');
  await page.close();
  console.log(JSON.stringify({fixture:'desktop/mobile consent, Start/Stop, warning surfaces, read-only recovery, epoch and release hiding passed',realSessionRequests:0}));
}finally{if(browser)await browser.close();await new Promise(resolve=>server.close(resolve));}

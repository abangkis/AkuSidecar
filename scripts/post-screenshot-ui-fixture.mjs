// Offline visual/interaction acceptance only. Started inside the Go-owned Job.
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { readFile, writeFile } from 'node:fs/promises';
import { resolve, join } from 'node:path';
import { launchChrome } from '../internal/collection/headless/worker/chrome.mjs';

const [chromePath, root, artifacts] = process.argv.slice(2);
assert.ok(chromePath && root && artifacts);
const web = resolve(import.meta.dirname, '../internal/httpapi/web');
let requests = 0;
const source = { id:'synthetic-post', evidence:{text:'Synthetic content'}, source:'x', original:'https://x.com/fixture/status/123' };
const fixture = `<!doctype html><html><head><meta charset="utf-8"><link rel="stylesheet" href="/styles.css"></head><body style="padding:24px"><main style="max-width:760px;margin:auto"><h1 style="font-size:24px">Post screenshot QA</h1><article class="presentable-item"><div class="item-presentation-toolbar"><span class="item-signal-slot">X · AI signal · Neutral</span><span class="presentation-label">Source layout</span><div id="actions" class="item-presentation-actions"><button class="presentation-toggle">Switch to brief</button></div></div><div class="source-layout-content"><p>Captured synthetic post. Opening the menu must leave this card and its state unchanged.</p><button id="expansion">Show more</button><span id="playing">Video state retained</span></div></article><div style="height:1800px"></div></main><script type="module">import {createPostScreenshotUI} from '/post-screenshot-ui.js'; const ui=createPostScreenshotUI({getOriginalUrl:entry=>entry.original,request:async(path,options)=>{const response=await fetch(path,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(options.body),signal:options.signal});return response.json();}}); document.querySelector('#actions').append(ui.createMenu(${JSON.stringify(source)}));window.qaReady=true;</script></body></html>`;
const server = createServer(async(req,res)=>{
  try {
    if(req.url==='/') {res.setHeader('Content-Type','text/html');return res.end(fixture);}
    if(req.url==='/api/post-screenshots') {
      requests++;
      const png=await readFile(join(artifacts,'whole-post-1.png'));
      res.setHeader('Content-Type','application/json');return res.end(JSON.stringify({parts:[{data:png.toString('base64'),width:800,height:2000}],warnings:[],filename:'synthetic-post'}));
    }
    if(!['/post-screenshot-ui.js','/styles.css'].includes(req.url)) {res.writeHead(404);return res.end();}
    res.setHeader('Content-Type',req.url.endsWith('.css')?'text/css':'text/javascript');res.end(await readFile(join(web,req.url.slice(1))));
  }catch {res.writeHead(500);res.end();}
});
await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
let browser;
try {
  browser=await launchChrome({chromePath,profilePath:join(root,'ui-profile')});
  const page=await browser.createTemporaryPage();
  await page.navigate(`http://127.0.0.1:${server.address().port}/`);
  async function until(expression){for(let i=0;i<80;i++){if(await page.evaluate(expression))return;await new Promise(r=>setTimeout(r,50));}throw Error('UI fixture state timeout: '+expression);}
  async function click(selector){const rect=await page.evaluate(`(()=>{const r=document.querySelector(${JSON.stringify(selector)}).getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2}})()`);await page.send('Input.dispatchMouseEvent',{type:'mousePressed',button:'left',clickCount:1,...rect});await page.send('Input.dispatchMouseEvent',{type:'mouseReleased',button:'left',clickCount:1,...rect});}
  for(const [label,width,height] of [['desktop',1100,850],['mobile',390,844]]) {
    await page.send('Emulation.setDeviceMetricsOverride',{width,height,deviceScaleFactor:1,mobile:false});
    await until('window.qaReady===true');
    await page.evaluate(`Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:async(value)=>{window.qaCopiedURL=value;}}})`);
    await click('.post-screenshot-menu-button');
    assert.equal(await page.evaluate(`document.querySelector('.post-screenshot-menu-button').getAttribute('aria-expanded')`),'true');
    if(label==='mobile') assert.ok(await page.evaluate(`document.querySelector('.item-presentation-toolbar').getBoundingClientRect().height<65`),'compact mobile toolbar');
    await writeFile(join(artifacts,`menu-${label}.png`),Buffer.from((await page.send('Page.captureScreenshot',{format:'png'})).data,'base64'));
    await page.send('Input.dispatchKeyEvent',{type:'keyDown',key:'Escape',code:'Escape',windowsVirtualKeyCode:27});
    assert.equal(await page.evaluate(`document.querySelector('.post-screenshot-menu-button').getAttribute('aria-expanded')`),'false');
    const requestsBeforeCopy=requests;
    await click('.post-screenshot-menu-button');
    await click('.post-original-url-copy');
    await until(`document.querySelector('.post-original-url-status').textContent==='Original URL copied.'`);
    assert.equal(await page.evaluate('window.qaCopiedURL'),source.original);
    assert.equal(await page.evaluate(`getComputedStyle(document.querySelector('.presentable-item')).overflow`),'visible','menu feedback must not be clipped by a short post');
    assert.equal(requests,requestsBeforeCopy,'copy URL must not dispatch a screenshot request');
    assert.equal(await page.evaluate(`Boolean(document.querySelector('.post-screenshot-dialog[open]'))`),false);
    await writeFile(join(artifacts,`copy-url-${label}.png`),Buffer.from((await page.send('Page.captureScreenshot',{format:'png'})).data,'base64'));
    await page.send('Input.dispatchKeyEvent',{type:'keyDown',key:'Escape',code:'Escape',windowsVirtualKeyCode:27});
    await click('.post-screenshot-menu-button');await click('.post-screenshot-menu-item');
    await until(`document.querySelector('.post-screenshot-image')?.src?.startsWith('data:image/png')`);
    assert.equal(await page.evaluate(`getComputedStyle(document.querySelector('.post-screenshot-part-navigation')).display`),'none');
    assert.equal(await page.evaluate(`document.querySelector('#expansion').textContent`),'Show more');
    const dimensions=await page.evaluate(`(()=>{const i=document.querySelector('.post-screenshot-image');return {w:i.clientWidth,h:i.clientHeight,scroll:window.scrollY,body:document.documentElement.scrollWidth,viewport:innerWidth}})()`);
    assert.ok(dimensions.w>240 && dimensions.h>dimensions.w,'long screenshot must remain readable');
    assert.ok(dimensions.body<=dimensions.viewport,'no horizontal page overflow');
    await writeFile(join(artifacts,`preview-${label}.png`),Buffer.from((await page.send('Page.captureScreenshot',{format:'png'})).data,'base64'));
    await click('.post-screenshot-close');
    assert.equal(await page.evaluate(`Boolean(document.querySelector('.post-screenshot-dialog[open]'))`),false);
    assert.equal(await page.evaluate('window.scrollY'),dimensions.scroll);
  }
  assert.equal(requests,2);
  await page.close();
  process.stdout.write(JSON.stringify({parts:[],warnings:[],fixture:'desktop/mobile menu, Escape, preview, close, state preservation passed'}));
}finally{if(browser)await browser.close();await new Promise(resolve=>server.close(resolve));}

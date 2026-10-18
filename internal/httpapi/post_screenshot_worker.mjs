// Private one-shot compositor. No URL navigation, credentials, or extension.
import { spawn } from 'node:child_process';
import { join } from 'node:path';
import { buildPostScreenshotHTML } from './render.mjs';

let input = '';
for await (const chunk of process.stdin) {
  input += chunk;
  if (input.length > 64 * 1024 * 1024) throw new Error('Export input limit exceeded.');
}
const request = JSON.parse(input);
const document = buildPostScreenshotHTML(request.entry, { css: request.css, theme: request.theme, media: request.media });
const chrome = spawn(request.chrome, [
  `--user-data-dir=${join(request.root, 'profile')}`, '--remote-debugging-pipe', '--headless=new',
  '--no-first-run', '--no-default-browser-check', '--disable-extensions', '--disable-background-networking',
  '--disable-component-update', '--disable-sync', '--disable-default-apps', '--disable-background-mode',
  '--host-resolver-rules=MAP * ~NOTFOUND', '--proxy-server=direct://', '--no-proxy-server',
  '--disable-gpu', '--hide-scrollbars', 'about:blank',
], { windowsHide: true, stdio: ['ignore', 'ignore', 'ignore', 'pipe', 'pipe'] });
let sequence = 0, buffer = '', failure;
const pending = new Map();
const exited = new Promise(resolve => chrome.once('exit', resolve));
function fail(error) {
  failure = error;
  for (const call of pending.values()) { clearTimeout(call.timer); call.reject(error); }
  pending.clear();
}
chrome.on('error', fail);
chrome.on('exit', () => fail(new Error('Export browser exited.')));
chrome.stdio[3].on('error', fail);
chrome.stdio[4].setEncoding('utf8');
chrome.stdio[4].on('data', chunk => {
  buffer += chunk;
  if (buffer.length > 48 * 1024 * 1024) return fail(new Error('Export CDP frame limit exceeded.'));
  let end;
  while ((end = buffer.indexOf('\0')) >= 0) {
    const frame = buffer.slice(0, end); buffer = buffer.slice(end + 1);
    if (!frame) continue;
    let message; try { message = JSON.parse(frame); } catch { return fail(new Error('Invalid CDP frame.')); }
    const call = pending.get(message.id); if (!call) continue;
    pending.delete(message.id); clearTimeout(call.timer);
    message.error ? call.reject(new Error('Export CDP command failed.')) : call.resolve(message.result || {});
  }
});
function send(method, params = {}, sessionId) {
  if (failure) return Promise.reject(failure);
  const id = ++sequence;
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => { pending.delete(id); reject(new Error('Export operation timed out.')); }, 15000);
    pending.set(id, {resolve,reject,timer});
    chrome.stdio[3].write(JSON.stringify({id,method,params,...(sessionId ? {sessionId} : {})}) + '\0');
  });
}
let output;
try {
  const {targetId} = await send('Target.createTarget', {url:'about:blank'});
  const {sessionId} = await send('Target.attachToTarget', {targetId,flatten:true});
  const page = (method,params) => send(method,params,sessionId);
  await page('Page.enable');
  // Only embedded data images and trusted inline style can be used. Also turn
  // off target networking before assigning the generated document.
  await page('Network.enable');
  await page('Network.setBlockedURLs',{urls:['http://*','https://*','file://*','ftp://*','ws://*','wss://*']});
  await page('Emulation.setDeviceMetricsOverride',{width:800,height:900,deviceScaleFactor:1,mobile:false});
  const {frameTree} = await page('Page.getFrameTree');
  await page('Page.setDocumentContent',{frameId:frameTree.frame.id,html:document.html});
  const ready = await page('Runtime.evaluate',{expression:`(async()=>{await document.fonts.ready;await Promise.all([...document.images].map(i=>i.decode().catch(()=>{})));for(const i of document.images){if(i.naturalWidth*i.naturalHeight>32000000)throw Error('Media pixel limit exceeded');if(!i.naturalWidth){const p=document.createElement('p');p.textContent='Media preview unavailable';p.className='screenshot-media-unavailable';i.replaceWith(p);}}return Math.ceil(Math.max(document.documentElement.scrollHeight,document.body.scrollHeight));})()`,awaitPromise:true,returnByValue:true});
  if (ready.exceptionDetails) throw new Error('Export document could not be prepared.');
  const height = ready.result?.value;
  if (!Number.isInteger(height) || height<1 || height>262144) throw new Error('Export height exceeds resource limit; no content was saved.');
  const parts=[]; let encoded=0;
  for(let y=0;y<height;y+=8192) {
    const partHeight=Math.min(8192,height-y);
    const {data}=await page('Page.captureScreenshot',{format:'png',captureBeyondViewport:true,fromSurface:true,clip:{x:0,y,width:800,height:partHeight,scale:1}});
    encoded+=data.length;
    if(encoded>40*1024*1024) throw new Error('Export output exceeds resource limit; no content was saved.');
    parts.push({data,width:800,height:partHeight});
  }
  output={parts,warnings:[...new Set([...(request.warnings||[]),...(document.warnings||[])])]};
} finally {
  try { await send('Browser.close'); } catch {}
  await Promise.race([exited,new Promise(resolve=>setTimeout(resolve,1500))]);
  if (chrome.exitCode===null) chrome.kill();
}
if (output) process.stdout.write(JSON.stringify(output));

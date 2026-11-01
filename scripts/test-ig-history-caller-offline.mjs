// Local accountless Chromium fixture. Never connects to the active runtime or
// uses its browser profile, cookies, permissions, settings or source sessions.
import assert from 'node:assert/strict';
import {createServer} from 'node:http';
import {resolve} from 'node:path';
import {randomUUID} from 'node:crypto';
import {fileURLToPath} from 'node:url';
import {mkdir,writeFile} from 'node:fs/promises';
import {launchChrome} from '../internal/collection/headless/worker/chrome.mjs';
import {createIGHistoryDiagnostic,IG_HISTORY_SOURCE_URL} from '../internal/collection/headless/worker/native-pilot/instagram-history-diagnostic.mjs';
import {createInstagramDocumentWatch} from '../internal/collection/headless/worker/native-pilot/instagram-document-watch.mjs';
const root=resolve(fileURLToPath(new URL('../',import.meta.url)));
const evidence=resolve(root,'runtime/qa/ig-history-caller');await mkdir(evidence,{recursive:true});
const profile=resolve(evidence,'offline-profile-'+randomUUID());
const callerScript='https://static.cdninstagram.com/rsrc.php/v4/test/r/caller-fixture.js';
const server=createServer((_request,response)=>{response.writeHead(200,{'content-type':'text/html','content-security-policy':"default-src 'none'; script-src 'unsafe-inline'"});response.end('<!doctype html><title>Local accountless navigation fixture</title>');});
let browser,page,history,watch;const observed=[],invalidations=[],contexts=new Map();
const previousModulePath=process.env.PSModulePath;delete process.env.PSModulePath;
try{
 await new Promise(r=>server.listen(0,'127.0.0.1',r));
 browser=await launchChrome({chromePath:resolve(root,'runtime/chromium/bin/chrome.exe'),profilePath:profile});
 page=await browser.createTemporaryPage();
 page.on(event=>{if(event.method==='Runtime.executionContextCreated'&&event.params?.context?.auxData?.isDefault)contexts.set(event.params.context.auxData.frameId,event.params.context);});
 history=createIGHistoryDiagnostic(page,value=>observed.push(value));assert.equal(await history.install(1000),true);
 await page.navigate('http://127.0.0.1:'+server.address().port+'/');
 const frame=(await page.send('Page.getFrameTree')).frameTree.frame;
 for(let i=0;!contexts.has(frame.id)&&i<20;i++)await new Promise(r=>setTimeout(r,10));
 const context=contexts.get(frame.id);assert.ok(context);
 const binding={id:frame.id,contextId:context.id,uniqueContextId:context.uniqueId,loaderId:frame.loaderId,url:frame.url};
 history.bind(binding);watch=createInstagramDocumentWatch(page,condition=>invalidations.push(condition));watch.bind(binding);
 const native=await page.send('Runtime.evaluate',{expression:'globalThis.originalPush=history.pushState;globalThis.originalReplace=history.replaceState;true',returnByValue:true});assert.equal(native.result.value,true);
 for(const method of ['replaceState','pushState']){
  const result=await page.send('Runtime.evaluate',{expression:`history.${method}({privateMarker:'PRIVATE_ACCOUNT'},'',location.href);\n//# sourceURL=${callerScript}`,returnByValue:true});assert.equal(result.exceptionDetails,undefined);
 }
 assert.deepEqual(observed.map(v=>v.method),['replaceState','pushState']);
 assert.ok(observed.every(v=>v.callerScript===callerScript&&v.line===1&&v.column>=1));
 assert.deepEqual(invalidations,['document_within_navigation']);assert.equal(watch.condition(),'document_within_navigation');
 const unchanged=await page.send('Runtime.evaluate',{expression:'history.pushState===originalPush&&history.replaceState===originalReplace',returnByValue:true});assert.equal(unchanged.result.value,true);
 const result={schema:'aku.ig.history-caller.offline-test.v1',passed:true,accountless:true,fixtureOrigin:'loopback',syntheticCallerSourceURL:callerScript,observerSourceURL:IG_HISTORY_SOURCE_URL,observed,invalidations,nativeMethodsUnchanged:true,chromium:browser.version.product,productionRuntimeAccessed:false};
 assert.ok(!JSON.stringify(result).includes('PRIVATE_ACCOUNT'));
 await writeFile(resolve(evidence,'offline-chromium-result.json'),JSON.stringify(result,null,2)+'\n');console.log(JSON.stringify(result));
}finally{
 watch?.dispose();await history?.dispose();await page?.close();await browser?.close();
 await new Promise(r=>server.close(r));
 if(previousModulePath===undefined)delete process.env.PSModulePath;else process.env.PSModulePath=previousModulePath;
}

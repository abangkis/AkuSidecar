import { readdir, readFile, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import assert from 'node:assert/strict';
import { root } from './chrome.mjs';
const runs=resolve(root,'runs'),targets=new Map();
for (const folder of (await readdir(runs,{withFileTypes:true})).filter(f=>f.isDirectory()).sort((a,b)=>a.name.localeCompare(b.name))) {
  const report=JSON.parse(await readFile(resolve(runs,folder.name,'capture.json'),'utf8'));
  if(report.source!=='facebook')continue;
  for(const post of report.posts)if(!post.publishedAt&&!targets.has(post.id))targets.set(post.id,{post,baseline:resolve(runs,folder.name,'capture.json')});
}
assert.ok(targets.size);
const results=[];
for(const {post:before,baseline} of [...targets.values()].slice(0,2)) {
  const {stdout}=await promisify(execFile)(process.execPath,[resolve(root,'shared-profile-test.mjs'),'--source','facebook','--url',before.permalink,
    '--max-posts','5','--max-scrolls','0','--wait-ms','30000'],{cwd:root,windowsHide:true,timeout:180000,maxBuffer:1048576});
  const logs=stdout.trim().split(/\r?\n/).filter(line=>line.startsWith('{')).map(line=>JSON.parse(line));
  const capture=logs.find(log=>log.output).output;
  const report=JSON.parse(await readFile(capture,'utf8'));
  const after=report.posts.find(post=>post.id===before.id);
  const lifecycle=logs.find(log=>log.receipt);
  const receipt=JSON.parse(await readFile(lifecycle.receipt,'utf8'));
  assert.ok(receipt.restored&&!receipt.profileOwnersAfterCapture.length);
  assert.equal(receipt.afterRestore.health,'healthy');assert.equal(receipt.bridgeAfterRestore.compatible,true);
  results.push({baseline,capture,receipt:lifecycle.receipt,targetMatched:Boolean(after),
    timestampRecovered:Boolean(after?.publishedAt),timestampSource:after?.timestampSource||null,
    timestampEstimated:after?.timestampEstimated??null,hover:after?.hoverEvidence||null,
    authenticatedEverySample:report.samples.every(sample=>sample.authenticatedUiObserved&&!sample.loginRequired&&!sample.challengeDetected),restored:true});
  console.log(JSON.stringify(results.at(-1)));
}
await writeFile(resolve(runs,'facebook-recovery-validation.json'),JSON.stringify({testedAt:new Date().toISOString(),results},null,2)+'\n');
console.log(JSON.stringify({tested:results.length,targetsMatched:results.filter(result=>result.targetMatched).length,timestampsRecovered:results.filter(result=>result.timestampRecovered).length}));

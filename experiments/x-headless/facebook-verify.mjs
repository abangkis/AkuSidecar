import assert from 'node:assert/strict';
import { readdir, readFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { root } from './chrome.mjs';
const runs = resolve(root,'runs');
const entries = await readdir(runs,{withFileTypes:true});
const receipts = await Promise.all(entries.filter(e=>e.isFile()&&e.name.startsWith('shared-profile-'))
  .map(async e=>JSON.parse(await readFile(resolve(runs,e.name),'utf8'))));
const results = [];
const uniqueIds = new Set();
for (const folder of entries.filter(e=>e.isDirectory()).sort((a,b)=>b.name.localeCompare(a.name))) {
  const report = JSON.parse(await readFile(resolve(runs,folder.name,'capture.json'),'utf8'));
  if (report.source !== 'facebook') continue;
  assert.equal(report.status,'captured_partial');
  assert.equal(report.headless,true);
  assert.equal(report.profileScope,'existing_registered_capture');
  assert.ok(report.samples.every(s=>s.authenticatedUiObserved&&!s.loginRequired&&!s.challengeDetected&&!s.sourceUnavailable));
  assert.equal(new Set(report.posts.map(p=>p.id)).size,report.posts.length);
  for (const post of report.posts) {
    uniqueIds.add(post.id);
    assert.match(post.id,/^facebook:post:/);
    assert.ok(post.author&&(post.text||post.media.length));
    assert.ok(['facebook.com','www.facebook.com'].includes(new URL(post.permalink).hostname));
    if (post.publishedAt) assert.ok(Number.isFinite(Date.parse(post.publishedAt)));
    for (const media of post.media) assert.ok(['fbcdn.net','fbsbx.com'].some(host=>new URL(media.url).hostname===host||new URL(media.url).hostname.endsWith('.'+host)));
  }
  const receipt = receipts.find(r=>Date.parse(r.startedAt)<=Date.parse(report.startedAt)
    &&Date.parse(r.completedAt)>=Date.parse(report.startedAt)+report.durationMs);
  assert.ok(receipt?.restored&&receipt.profileOwnersAfterCapture.length===0);
  assert.equal(receipt.afterRestore.health,'healthy');
  assert.equal(receipt.bridgeAfterRestore.compatible,true);
  const media = report.posts.flatMap(p=>p.media);
  results.push({capture:resolve(runs,folder.name,'capture.json'),posts:report.posts.length,durationMs:report.durationMs,
    samplingRounds:report.samples.length,maxScrollY:Math.max(...report.samples.map(s=>s.scroll.y)),stopReason:report.stopReason,
    expandedTexts:report.posts.filter(p=>p.textStatus==='expanded').length,collapsedTexts:report.posts.filter(p=>p.textCollapsed).length,
    timestampsKnown:report.posts.filter(p=>p.publishedAt).length,images:media.filter(m=>m.kind==='image').length,
    decodedImages:media.filter(m=>m.kind==='image'&&m.loaded&&m.width>0).length,videoPosters:media.filter(m=>m.kind==='video_poster').length,
    mediaHttp200:media.filter(m=>report.mediaResponses.some(r=>r.url===m.url&&r.status===200)).length,
    missingIdentityObservations:report.samples.reduce((n,s)=>n+(s.rejectionReasons.missing_identity||0),0),authenticatedEverySample:true,restored:true});
  if(results.length>=2)break;
}
assert.ok(results.length>0,'No Facebook capture evidence available.');
console.log(JSON.stringify({status:'passed',uniquePosts:uniqueIds.size,results},null,2));

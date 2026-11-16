import assert from 'node:assert/strict';
import { readFile, readdir } from 'node:fs/promises';
import { resolve } from 'node:path';
import { root } from './chrome.mjs';
const runs=resolve(root,'runs');
const suite=JSON.parse(await readFile(resolve(runs,'facebook-recovery-validation.json'),'utf8'));
assert.equal(suite.results.length,2);
for(const result of suite.results) {
  const before=JSON.parse(await readFile(result.baseline,'utf8'));
  const after=JSON.parse(await readFile(result.capture,'utf8'));
  const post=after.posts.find(post=>post.permalink===after.target);
  const old=before.posts.find(old=>old.id===post?.id);
  assert.ok(old&&!old.publishedAt&&post.publishedAt);
  assert.equal(post.timestampSource,'relative_text_estimate');assert.equal(post.timestampEstimated,true);
  assert.ok(Number.isFinite(Date.parse(post.publishedAt))&&Date.parse(post.publishedAt)<=Date.parse(post.capturedAt)+60000);
  assert.ok(after.samples.every(sample=>sample.authenticatedUiObserved&&!sample.loginRequired&&!sample.challengeDetected));
  const receipt=JSON.parse(await readFile(result.receipt,'utf8'));
  assert.ok(receipt.restored&&receipt.profileOwnersAfterCapture.length===0);
  assert.equal(receipt.afterRestore.health,'healthy');assert.equal(receipt.bridgeAfterRestore.compatible,true);
}
let hoverRecoveredPosts=0;
for(const folder of (await readdir(runs,{withFileTypes:true})).filter(f=>f.isDirectory())) {
  const report=JSON.parse(await readFile(resolve(runs,folder.name,'capture.json'),'utf8'));
  if(report.source!=='facebook')continue;
  hoverRecoveredPosts+=report.posts.filter(post=>post.hoverEvidence?.permalinkRecoveredByHover).length;
}
assert.ok(hoverRecoveredPosts>=2);
console.log(JSON.stringify({status:'passed',sameIdentityTimestampEstimatesRecovered:2,observedHoverPermalinkRecoveries:hoverRecoveredPosts,
  restoredReceiptsVerified:2,exactLiveTimestampClaim:false}));

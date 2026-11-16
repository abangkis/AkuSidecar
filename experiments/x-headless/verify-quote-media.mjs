import assert from 'node:assert/strict';
import { readdir, readFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { root } from './chrome.mjs';
const runs = resolve(root, 'runs');
const validation = JSON.parse(await readFile(resolve(runs, 'quote-media-validation.json'), 'utf8'));
const receipts = await Promise.all((await readdir(runs)).filter(name => /^shared-profile-.*\.json$/.test(name))
  .map(async name => JSON.parse(await readFile(resolve(runs, name), 'utf8'))));
let images = 0, posters = 0, unknownQuoteIdentities = 0;
for (const result of validation.results) {
  const report = JSON.parse(await readFile(result.capture, 'utf8'));
  const post = report.posts.find(p => p.permalink === report.target);
  assert.ok(post?.quotedPost);
  const quote = post.quotedPost;
  assert.ok(report.samples.every(s => s.authenticatedUiObserved && !s.loginRequired && !s.challengeDetected));
  for (const media of quote.media) {
    assert.ok(['pbs.twimg.com', 'video.twimg.com'].includes(new URL(media.url).hostname));
    assert.ok(report.mediaResponses.some(response => response.url === media.url && response.status === 200));
    if (media.kind === 'image' && media.loadedMeaning === 'image_decoded' && media.loaded && media.width > 0) images++;
    if (media.kind === 'video_poster') posters++;
  }
  const receipt = receipts.find(r => Date.parse(r.startedAt) <= Date.parse(report.startedAt)
    && Date.parse(r.completedAt) >= Date.parse(report.startedAt) + report.durationMs);
  assert.ok(receipt?.restored);
  assert.equal(receipt.profileOwnersAfterCapture.length, 0);
  assert.equal(receipt.afterRestore.health, 'healthy');
  assert.equal(receipt.bridgeAfterRestore.compatible, true);
  if (!quote.permalink) unknownQuoteIdentities++;
}
assert.ok(images > 0, 'Need a decoded live quoted image.');
assert.ok(posters > 0, 'Need an observed live quoted video poster.');
console.log(JSON.stringify({ status: 'passed', cases: validation.results.length, decodedQuotedImages: images,
  quotedVideoPosters: posters, unknownQuoteIdentities, restoredReceiptsVerified: validation.results.length }));

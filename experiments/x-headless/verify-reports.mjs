import assert from 'node:assert/strict';
import { readFile, readdir } from 'node:fs/promises';
import { resolve } from 'node:path';
import { root } from './chrome.mjs';
import { summarize } from './audit.mjs';

const entries = await readdir(resolve(root, 'runs'), { withFileTypes: true });
const folders = entries.filter(e => e.isDirectory()).map(e => e.name).sort().reverse();
const receipts = await Promise.all(entries.filter(e => e.isFile() && e.name.startsWith('shared-profile-'))
  .map(async e => JSON.parse(await readFile(resolve(root, 'runs', e.name), 'utf8'))));
const reports = [];
for (const folder of folders) {
  const report = JSON.parse(await readFile(resolve(root, 'runs', folder, 'capture.json'), 'utf8'));
  if ((report.source || 'x') !== 'x' || report.status !== 'captured_partial' || !report.samples?.[0]?.scroll) continue;
  const s = summarize(report);
  assert.equal(report.profileScope, 'existing_registered_capture');
  assert.equal(report.headless, true);
  assert.match(report.browser.userAgent, /HeadlessChrome/);
  assert.equal(s.authenticatedEverySample, true);
  assert.equal(s.challengeObserved, false);
  assert.equal(s.posts, s.uniqueIds);
  assert.equal(s.textPosts, s.posts);
  assert.equal(s.timestampPosts, s.posts);
  assert.equal(s.futureTimestamps, 0);
  assert.ok(report.documentResponses.some(r => r.status === 200));
  for (const p of report.posts) {
    assert.ok(p.author);
    assert.equal(p.permalink, `https://x.com/${new URL(p.permalink).pathname.split('/')[1]}/status/${p.id}`);
    for (const m of p.media) assert.ok(['pbs.twimg.com', 'video.twimg.com'].includes(new URL(m.url).hostname));
  }
  const receipt = receipts.find(r => Date.parse(r.startedAt) <= Date.parse(report.startedAt)
    && Date.parse(r.completedAt) >= Date.parse(report.startedAt));
  assert.ok(receipt, 'matching lifecycle receipt');
  assert.equal(receipt.restored, true);
  assert.equal(receipt.afterStop.ownedPids.length, 0);
  assert.equal(receipt.profileOwnersAfterStop.length, 0);
  assert.equal(receipt.profileOwnersAfterCapture.length, 0);
  assert.equal(receipt.afterRestore.health, 'healthy');
  assert.equal(receipt.bridgeAfterRestore.compatible, true);
  reports.push(report);
  if (reports.length === 4) break;
}
assert.equal(reports.length, 4, 'three feed batches and targeted video test');
const feeds = reports.filter(r => new URL(r.target).pathname === '/home');
assert.equal(feeds.length, 3);
for (const r of feeds) {
  assert.equal(r.samples.length, 13);
  assert.equal(r.stopReason, 'scroll_limit');
  assert.ok(r.samples.at(-1).scroll.y > r.samples[0].scroll.y);
  assert.ok(r.samples.some(s => s.round > 0 && s.newPosts > 0));
}
const video = reports.find(r => new URL(r.target).pathname !== '/home');
assert.equal(summarize(video).targetPrimaryVideoPlaying, true);
const sets = feeds.map(r => new Set(r.posts.map(p => p.id)));
const union = new Set(feeds.flatMap(r => r.posts.map(p => p.id)));
const pairwiseOverlap = sets.map((s, i) => sets.slice(i + 1).map(t => [...s].filter(id => t.has(id)).length)).flat();
console.log(JSON.stringify({ status: 'passed', feedBatches: feeds.length, feedCounts: feeds.map(r => r.posts.length),
  uniquePostsAcrossFeedBatches: union.size, pairwiseOverlap, targetedVideoPlaying: true,
  restoredRuns: reports.length, fullFeedOrMediaParityProven: false }));

import assert from 'node:assert/strict';
import { readdir, readFile, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { root } from './chrome.mjs';

const targets = new Map();
for (const folder of (await readdir(resolve(root, 'runs'), { withFileTypes: true })).filter(f => f.isDirectory()).sort((a,b) => b.name.localeCompare(a.name))) {
  const report = JSON.parse(await readFile(resolve(root, 'runs', folder.name, 'capture.json'), 'utf8'));
  if (report.profileScope !== 'existing_registered_capture') continue;
  for (const post of report.posts ?? []) if (post.quotedPostObserved && !targets.has(post.id)) targets.set(post.id, post);
}
assert.ok(targets.size, 'No previously observed quoted posts available.');
const results = [];
for (const before of [...targets.values()].slice(0, 3)) {
  const { stdout } = await promisify(execFile)(process.execPath, [resolve(root, 'shared-profile-test.mjs'), '--url', before.permalink,
    '--max-posts', '5', '--max-scrolls', '0', '--wait-ms', '30000'], { cwd: root, windowsHide: true, timeout: 180000, maxBuffer: 1048576 });
  const logs = stdout.trim().split(/\r?\n/).filter(line => line.startsWith('{')).map(line => JSON.parse(line));
  const captureLog = logs.find(log => log.output);
  const report = JSON.parse(await readFile(captureLog.output, 'utf8'));
  const post = report.posts.find(p => p.id === before.id);
  assert.ok(post?.quotedPost, 'Quoted root must remain observed on the same primary identity.');
  // Some X quote cards expose text/media without a DOM permalink. Preserve unknown.
  if (post.quotedPost.permalink && before.parentPermalink) assert.equal(post.quotedPost.permalink, before.parentPermalink);
  assert.ok(report.samples.every(s => s.authenticatedUiObserved && !s.loginRequired && !s.challengeDetected));
  assert.ok(logs.some(log => log.restored === true));
  for (const media of post.quotedPost.media) assert.ok(['pbs.twimg.com', 'video.twimg.com'].includes(new URL(media.url).hostname));
  const quote = post.quotedPost;
  const decoded = quote.media.filter(m => m.loadedMeaning === 'image_decoded' && m.loaded);
  results.push({ capture: captureLog.output, quoteMedia: quote.media.length, images: quote.media.filter(m => m.kind === 'image').length,
    videoPosters: quote.media.filter(m => m.kind === 'video_poster').length, decodedImagesAndPosters: decoded.length,
    mediaEvidence: quote.mediaEvidence, mediaExpected: quote.mediaExpected,
    successfulNetworkMedia: quote.media.filter(m => report.mediaResponses.some(r => r.url === m.url && r.status === 200)).length,
    authenticated: true, quoteIdentityMatches: quote.permalink && before.parentPermalink ? true : null, restored: true });
  console.log(JSON.stringify(results.at(-1)));
}
const output = resolve(root, 'runs', 'quote-media-validation.json');
await writeFile(output, JSON.stringify({ testedAt: new Date().toISOString(), results }, null, 2) + '\n');
console.log(JSON.stringify({ output, tested: results.length, withQuoteMedia: results.filter(r => r.quoteMedia).length,
  media: results.reduce((sum,r) => sum + r.quoteMedia, 0) }));

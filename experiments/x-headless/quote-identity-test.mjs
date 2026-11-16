import assert from 'node:assert/strict';
import { readFile, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { root } from './chrome.mjs';
import { matchesQuote } from './quote-navigation.mjs';
const baseline = JSON.parse(await readFile(resolve(root, 'runs', 'quote-media-validation.json'), 'utf8'));
const results = [];
const statusId = url => url?.match(/\/status\/(\d+)/)?.[1] || null;
for (const sample of baseline.results.slice(0, process.argv.includes('--one') ? 1 : baseline.results.length)) {
  const previous = JSON.parse(await readFile(sample.capture, 'utf8'));
  const before = previous.posts.find(p => p.permalink === previous.target);
  const { stdout } = await promisify(execFile)(process.execPath, [resolve(root, 'shared-profile-test.mjs'), '--url', before.permalink,
    '--max-posts', '5', '--max-scrolls', '0', '--wait-ms', '30000'], { cwd: root, windowsHide: true, timeout: 180000, maxBuffer: 1048576 });
  const logs = stdout.trim().split(/\r?\n/).filter(line => line.startsWith('{')).map(line => JSON.parse(line));
  const captureLog = logs.find(log => log.output);
  const report = JSON.parse(await readFile(captureLog.output, 'utf8'));
  const post = report.posts.find(p => p.id === before.id);
  assert.ok(post?.quotedPost);
  const quote = post.quotedPost;
  const beforeId = statusId(before.quotedPost.permalink), afterId = statusId(quote.permalink);
  if (beforeId && afterId) assert.equal(afterId, beforeId);
  assert.ok(report.samples.every(s => s.authenticatedUiObserved && !s.loginRequired && !s.challengeDetected));
  const lifecycle = logs.find(log => log.receipt);
  const receipt = JSON.parse(await readFile(lifecycle.receipt, 'utf8'));
  assert.ok(receipt.restored && !receipt.profileOwnersAfterCapture.length);
  assert.equal(receipt.afterRestore.health, 'healthy');
  assert.equal(receipt.bridgeAfterRestore.compatible, true);
  if (quote.identityProvenance === 'main_structured_quote_relation') {
    assert.equal(quote.identityEvidence.explicitQuoteIds.length, 1);
    assert.equal(quote.identityEvidence.explicitQuoteIds[0], afterId);
    assert.ok(quote.identityEvidence.matchedPrimaryNodes > 0 && !quote.identityEvidence.bounded);
    assert.notEqual(afterId, post.id);
  }
  if (quote.identityProvenance === 'observed_quote_navigation') {
    assert.ok(report.quoteIdentityProbe.restored && report.quoteIdentityProbe.targetIdentityAndContentMatched);
    assert.equal(report.quoteIdentityProbe.destinationEvidence.id, afterId);
    assert.ok(matchesQuote(quote, report.quoteIdentityProbe.destinationEvidence));
  }
  assert.equal(post.parentPermalink, quote.permalink);
  results.push({ baseline: sample.capture, capture: captureLog.output, receipt: lifecycle.receipt,
    beforeIdentified: Boolean(beforeId), afterIdentified: Boolean(afterId), recovered: !beforeId && Boolean(afterId),
    identityStatus: quote.identityStatus, provenance: quote.identityProvenance, evidence: quote.identityEvidence,
    quotedMedia: quote.media.length, authenticated: true, restored: true });
  console.log(JSON.stringify({ ...results.at(-1), evidence: { ...quote.identityEvidence, explicitQuoteIds: quote.identityEvidence.explicitQuoteIds.length } }));
}
const output = resolve(root, 'runs', 'quote-identity-validation.json');
await writeFile(output, JSON.stringify({ testedAt: new Date().toISOString(), results }, null, 2) + '\n');
console.log(JSON.stringify({ output, tested: results.length, previouslyUnknown: results.filter(r => !r.beforeIdentified).length,
  recovered: results.filter(r => r.recovered).length, unresolved: results.filter(r => !r.afterIdentified).length }));

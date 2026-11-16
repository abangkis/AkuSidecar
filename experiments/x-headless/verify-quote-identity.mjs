import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { root } from './chrome.mjs';
import { matchesQuote } from './quote-navigation.mjs';
const validation = JSON.parse(await readFile(resolve(root, 'runs', 'quote-identity-validation.json'), 'utf8'));
assert.equal(validation.results.length, 3);
assert.equal(validation.results.filter(result => result.recovered).length, 2);
for (const result of validation.results) {
  const report = JSON.parse(await readFile(result.capture, 'utf8'));
  const previous = JSON.parse(await readFile(result.baseline, 'utf8'));
  const primary = report.posts.find(post => post.permalink === report.target);
  const before = previous.posts.find(post => post.permalink === previous.target);
  assert.equal(primary.id, before.id);
  const quote = primary.quotedPost;
  assert.equal(quote.identityStatus, 'identified');
  assert.equal(primary.parentPermalink, quote.permalink);
  if (result.recovered) {
    const probe = report.quoteIdentityProbe;
    assert.ok(probe.restored && probe.targetIdentityAndContentMatched);
    assert.equal(probe.destinationEvidence.permalink, quote.permalink);
    assert.notEqual(probe.destinationEvidence.id, primary.id);
    assert.ok(matchesQuote(quote, probe.destinationEvidence));
    assert.equal(quote.identityProvenance, 'observed_quote_navigation');
  } else assert.equal(quote.permalink, before.quotedPost.permalink);
  const receipt = JSON.parse(await readFile(result.receipt, 'utf8'));
  assert.ok(receipt.restored && !receipt.profileOwnersAfterCapture.length);
  assert.equal(receipt.afterRestore.health, 'healthy');
  assert.equal(receipt.bridgeAfterRestore.compatible, true);
}
console.log(JSON.stringify({ status: 'passed', recoveredUnknownQuotes: 2, knownControlPreserved: 1,
  navigationEvidenceAndRestorationVerified: 3 }));

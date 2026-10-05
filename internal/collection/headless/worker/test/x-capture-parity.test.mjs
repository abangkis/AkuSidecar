import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import { readFile } from 'node:fs/promises';
import { capturePrimitivesFor } from '../capture-primitives.mjs';

const shared = await readFile(new URL('../../../../../../AkuBridge/capture-primitives.js', import.meta.url), 'utf8');
const extractor = await readFile(new URL('../vendor/x-extract.js', import.meta.url), 'utf8');
const quoteResolver = await readFile(new URL('../vendor/x-quote-identity.js', import.meta.url), 'utf8');

function fixture({ detail = true, replaceRoute = false } = {}) {
  const permalink = 'https://x.com/owner/status/2106867448746021278';
  const location = { href: detail ? permalink : 'https://x.com/home', hostname: 'x.com', pathname: detail ? '/owner/status/2106867448746021278' : '/home' };
  const textNode = text => ({ nodeType: 1, tagName: 'DIV', childNodes: [{ nodeType: 3, nodeValue: text }], innerText: text });
  let root = textNode('Short excerpt'), more = true, clicks = 0;
  const button = { innerText: '… Show more', textContent: '… Show more', click() {
    clicks++; root = textNode('Full captured text. '.repeat(25)); more = false;
    if (replaceRoute) location.href = 'https://x.com/other/status/2106867448746021279';
  } };
  const time = { dateTime: '2026-10-05T00:00:00Z', closest: () => ({ href: permalink }) };
  const container = { querySelector: selector => selector === 'text' ? root : null,
    querySelectorAll: selector => selector === 'text' ? [root] : selector === 'time' ? [time] : selector === 'more' && more ? [button] : [],
    getBoundingClientRect: () => ({ top: 0, bottom: 200, height: 200 }) };
  const adapter = { version: 'fixture', discoverCandidates: () => ({ candidates: [container] }), findQuotedRoot: () => null,
    findAuthor: () => 'Owner', findAvatar: () => null, contentRootSelector: 'text',
    extractText: (_, helpers) => helpers.structuredText(root), extractSemantics: () => ({}), extractQuotedPost: () => null,
    contentExpansion: { buttonSelector: 'more', attempts: 2, intervalMs: 1, restorable: false },
    imageSelector: 'img', mediaHosts: [], mediaAcquisition: { detectExpectedKinds: () => [] }, loginRequired: () => false };
  const context = vm.createContext({ URL, setTimeout, location, AkuSourceAdapters: { get: () => adapter },
    innerHeight: 900, innerWidth: 1200, scrollY: 0,
    document: { querySelector: () => null, body: { innerText: '' }, readyState: 'complete', visibilityState: 'visible' } });
  vm.runInContext(shared + '\n' + quoteResolver + '\n' + extractor, context);
  return { context, clicks: () => clicks };
}

test('headless uses shared expansion with replacement DOM and preserves feed click policy', async () => {
  const detail = fixture();
  const expanded = (await detail.context.XHeadlessPoC.collect()).posts[0];
  assert.equal(detail.clicks(), 1);
  assert.equal(expanded.textStatus, 'expanded');
  assert.equal(expanded.textCompleteness, 'expanded');
  assert.ok(expanded.text.length > 400);
  assert.equal(expanded.identityComparison.agrees, true);
  const feed = fixture({ detail: false });
  const partial = (await feed.context.XHeadlessPoC.collect()).posts[0];
  assert.equal(feed.clicks(), 0);
  assert.equal(partial.textStatus, 'requires_permalink_capture');
  assert.equal(partial.textCompleteness, 'partial');
});

test('headless route change remains unresolved instead of declaring text recovery verified', async () => {
  const fixtureValue = fixture({ replaceRoute: true });
  const post = (await fixtureValue.context.XHeadlessPoC.collect()).posts[0];
  assert.equal(post.textStatus, 'navigation_changed');
  assert.notEqual(post.textCompleteness, 'recovery_verified');
});

test('Node recovery consumes the identical shared asset and fails closed when missing', () => {
  const assets = [{ relative: 'AkuBridge/capture-primitives.js', content: shared }];
  const primitives = capturePrimitivesFor(assets);
  assert.equal(primitives, capturePrimitivesFor(assets));
  assert.equal(primitives.evaluateTextReplacement({ originalText: 'short', recoveredText: 'longer text', identityMatches: false, resolved: true }), false);
  assert.equal(primitives.canonicalizeXPermalink('https://user@x.com/a/status/12345'), null);
  assert.throws(() => capturePrimitivesFor([]), /asset is required/);
});

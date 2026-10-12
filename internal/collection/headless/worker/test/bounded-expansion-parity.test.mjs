import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import { fileURLToPath } from 'node:url';
import { sourceAssets } from '../worker.mjs';

const bridgePath = fileURLToPath(new URL('../../../../../../AkuBridge/', import.meta.url));
const longText = `Expanded caption with source-owned detail. ${'Additional words remain visible. '.repeat(5)}`.trim();

async function fixture(source, { allow = true, policyPresent = true, deadlineAt = Date.now() + 3000, mutateIdentity = false,
  commentControl = false, navigate = false, replaceControl = false } = {}) {
  const assets = await sourceAssets(bridgePath, source);
  let expanded = false, clicks = 0, replacementClicks = 0, currentButton = null, replacementButton = null;
  const initialUrl = source === 'instagram'
    ? 'https://www.instagram.com/p/FixturePost1/'
    : 'https://www.linkedin.com/feed/update/urn:li:activity:1234567890/';
  let currentUrl = initialUrl;
  const pageLocation = { href: source === 'instagram' ? 'https://www.instagram.com/' : 'https://www.linkedin.com/feed/',
    hostname: source === 'instagram' ? 'www.instagram.com' : 'www.linkedin.com' };
  const comment = { contains: node => node?.commentControl === true };
  const contentRoot = {
    nodeType: 1, tagName: 'DIV', parentElement: null,
    get innerText() { return expanded ? longText : 'Short caption'; },
    get textContent() { return this.innerText; },
    get childNodes() { return [{ nodeType: 3, get nodeValue() { return expanded ? longText : 'Short caption'; } }]; },
    querySelectorAll(selector) {
      if (selector.includes('button') || selector.includes('[role="button"]')
          || selector.includes('expandable-text-button')) return commentControl ? [commentButton, currentButton] : [currentButton];
      if (selector === 'span[dir="auto"]') return [caption];
      if (selector === 'a[href]') return [];
      return [];
    },
    querySelector: () => null,
    contains: node => node === this || node === button || node === commentButton || node === caption,
    getBoundingClientRect: () => ({ width: 500, height: 100, left: 0, right: 500, top: 20, bottom: 120 }),
  };
  const caption = {
    tagName: 'SPAN', parentElement: contentRoot,
    get innerText() { return expanded ? longText : 'Short caption'; },
    get textContent() { return this.innerText; },
    closest: () => null,
    querySelectorAll: () => [],
  };
  const button = {
    tagName: 'BUTTON', parentElement: contentRoot,
    get innerText() { return expanded ? 'See less' : '… more'; },
    get textContent() { return this.innerText; },
    getAttribute: name => name === 'data-testid' && source === 'linkedin' ? 'expandable-text-button' : null,
    closest: () => null,
    click() {
      clicks++;
      expanded = !expanded;
      if (mutateIdentity && expanded) {
        currentUrl = source === 'instagram'
          ? 'https://www.instagram.com/p/ForeignPost2/'
          : 'https://www.linkedin.com/feed/update/urn:li:activity:1234567891/';
      }
      if (replaceControl && expanded) currentButton = replacementButton;
      if (navigate && expanded) {
        currentUrl = 'https://www.linkedin.com/feed/update/urn:li:activity:1234567891/';
        pageLocation.href = currentUrl;
      }
    },
  };
  const commentButton = {
    tagName: 'BUTTON', parentElement: contentRoot, commentControl: true,
    innerText: 'See more', textContent: 'See more',
    getAttribute: () => null,
    closest: selector => selector.includes('comments-comment-item') ? comment : null,
    click() { throw new Error('A comment control must never be clicked.'); },
  };
  replacementButton = {
    tagName: 'BUTTON', parentElement: contentRoot,
    get innerText() { return expanded ? 'See less' : '… more'; },
    get textContent() { return this.innerText; },
    getAttribute: name => name === 'data-testid' && source === 'linkedin' ? 'expandable-text-button' : null,
    closest: () => null,
    click() { replacementClicks++; expanded = !expanded; },
  };
  currentButton = button;
  contentRoot.contains = node => node === contentRoot || node === currentButton || node === commentButton || node === caption;

  const profile = { href: 'https://www.instagram.com/owner/', innerText: 'Owner', textContent: 'Owner',
    getAttribute: name => name === 'href' ? '/owner/' : null, querySelector: () => null };
  const permalink = { get href() { return currentUrl; }, innerText: '', textContent: '',
    getAttribute: name => name === 'href' ? currentUrl : null };
  const time = { dateTime: '2026-10-12T00:00:00Z', getAttribute: () => '2026-10-12T00:00:00Z',
    closest: () => permalink };
  const candidate = {
    tagName: 'ARTICLE', parentElement: null, style: {},
    get innerText() { return expanded ? longText : 'Short caption'; },
    get textContent() { return this.innerText; },
    getAttribute: name => name === 'data-urn' && source === 'linkedin' ? 'urn:li:activity:1234567890' : null,
    hasAttribute: () => false,
    getBoundingClientRect: () => ({ width: 600, height: 300, left: 0, right: 600, top: 10, bottom: 310 }),
    getClientRects: () => [{}],
    contains: node => node === candidate || node === contentRoot || node === currentButton || node === button || node === commentButton
      || node === caption || node === profile || node === permalink || node === time,
    querySelector(selector) {
      if (selector === '[data-testid="expandable-text-box"]') return source === 'linkedin' ? contentRoot : null;
      if (selector === 'time') return time;
      if (selector === 'a[href^="/"]') return source === 'instagram' ? profile : null;
      return null;
    },
    querySelectorAll(selector) {
      if (selector === 'span[dir="auto"]') return source === 'instagram' ? [caption] : [];
      if (selector === 'a[href^="/"]') return source === 'instagram' ? [profile] : [];
      if (selector === 'a[href]') return [permalink, ...(source === 'instagram' ? [profile] : [])];
      if (selector === '[data-urn], [data-id], a[href]') return [permalink];
      if (selector === 'time[datetime]' || selector === 'time') return [time];
      if (selector.includes('expandable-text-box')) return source === 'linkedin' ? [contentRoot] : [];
      if (selector === 'img' || selector === 'video' || selector.includes('update-components-image')
          || selector.includes('data-view-name*=') || selector.includes('data-test-document-container')
          || selector.startsWith('button[aria-label^=' ) || selector.includes('data-vjs-player')) return [];
      return [];
    },
  };
  const actualAdapter = assets.find(asset => asset.relative === `AkuBridge/adapters/${source}-adapter.js`);
  const capturePolicy = { deadlineAt };
  if (policyPresent) capturePolicy.allowContentExpansion = allow;
  const context = vm.createContext({
    URL, setTimeout, clearTimeout,
    location: pageLocation,
    getComputedStyle: () => ({ display: 'block', visibility: 'visible', opacity: '1' }),
    innerHeight: 900, innerWidth: 1280, scrollY: 0,
    AkuHeadlessCapturePolicy: capturePolicy,
    document: { visibilityState: 'visible', title: 'Fixture', readyState: 'complete', scrollingElement: { scrollHeight: 1400 },
      body: { innerText: '' }, querySelector: () => null, querySelectorAll: () => [] },
  });
  for (const asset of assets.filter(item => item.execute)) vm.runInContext(asset.content, context, { filename: asset.relative });
  const real = context.AkuSourceAdapters.get(source);
  const adapter = {
    ...real,
    matchesPage: () => true,
    discoverCandidates: () => ({ candidates: [candidate], strategy: 'fixture_native_permalink' }),
    findPermalinkDetails: source === 'instagram' ? () => ({ url: currentUrl, source: 'native_post_anchor' }) : real.findPermalinkDetails,
    findDomPermalink: source === 'linkedin' ? () => currentUrl : real.findDomPermalink,
    findAuthor: () => 'Owner',
    findAvatar: () => null,
    extractSemantics: () => ({ contentKind: 'post', relationshipType: 'original', engagement: {} }),
    extractPresentation: () => ({}),
    extractAttachments: () => [],
    mediaAcquisition: { detectExpectedKinds: () => [], extractCandidates: () => [] },
    loginRequired: () => false,
  };
  context.AkuSourceAdapters = { get: () => adapter };
  // Re-execute the collection adapter after the real shared parser and source policy.
  const extractor = assets.find(asset => asset.relative === 'worker/vendor/adapter-extract.js');
  vm.runInContext(extractor.content, context, { filename: extractor.relative });
  return { collect: () => context.XHeadlessPoC.collect(), clicks: () => clicks, replacementClicks: () => replacementClicks, expanded: () => expanded,
    currentUrl: () => currentUrl, initialUrl, adapter };
}

test('Instagram and LinkedIn capture expanded text through the shared bounded adapter policy', async () => {
  for (const source of ['instagram', 'linkedin']) {
    const f = await fixture(source);
    const snapshot = await f.collect();
    assert.equal(snapshot.posts.length, 1, `${source}: ${JSON.stringify(snapshot)}`);
    assert.equal(snapshot.posts[0].text, longText, source);
    assert.equal(snapshot.posts[0].textStatus, 'expanded', source);
    assert.equal(snapshot.posts[0].textCollapsed, false, source);
    assert.equal(f.clicks(), source === 'instagram' ? 1 : 2, source);
    assert.equal(f.expanded(), source === 'instagram', source);
  }
});

test('expansion skips detect-only, expired, comment, foreign identity, and changed-route cases', async () => {
  const detectOnly = await fixture('linkedin', { allow: false });
  const detectOnlySnapshot = await detectOnly.collect();
  assert.ok(detectOnlySnapshot.posts[0], JSON.stringify(detectOnlySnapshot));
  assert.equal(detectOnlySnapshot.posts[0].textStatus, 'expansion_skipped_detect_only');
  assert.equal(detectOnly.clicks(), 0);

  const missingPolicy = await fixture('instagram', { policyPresent: false });
  const missingPolicySnapshot = await missingPolicy.collect();
  assert.equal(missingPolicySnapshot.posts[0].textStatus, 'expansion_skipped_detect_only');
  assert.equal(missingPolicy.clicks(), 0);

  const expired = await fixture('instagram', { deadlineAt: Date.now() - 1 });
  assert.equal((await expired.collect()).posts[0].textStatus, 'deadline_expired');
  assert.equal(expired.clicks(), 0);

  const comment = await fixture('linkedin', { commentControl: true });
  assert.equal(comment.clicks(), 0);
  assert.equal((await comment.collect()).posts[0].text, 'Short caption');

  const foreign = await fixture('linkedin', { mutateIdentity: true });
  const foreignSnapshot = await foreign.collect();
  assert.equal(foreignSnapshot.posts.length, 0);
  assert.equal(foreignSnapshot.rejectionReasons.identity_mismatch, 1);
  assert.equal(foreign.clicks(), 2, 'expanded text is restored after the identity recheck fails');
  assert.equal(foreign.expanded(), false);

  const replacedForeignControl = await fixture('linkedin', { mutateIdentity: true, replaceControl: true });
  const replacedForeignSnapshot = await replacedForeignControl.collect();
  assert.equal(replacedForeignSnapshot.posts.length, 0);
  assert.equal(replacedForeignSnapshot.rejectionReasons.identity_mismatch, 1);
  assert.equal(replacedForeignControl.clicks(), 1);
  assert.equal(replacedForeignControl.replacementClicks(), 0, 'never click a replacement control on the foreign owner');

  const route = await fixture('linkedin', { navigate: true });
  const routeSnapshot = await route.collect();
  assert.equal(routeSnapshot.posts.length, 0);
  assert.equal(routeSnapshot.rejectionReasons.identity_mismatch, 1);
  assert.equal(route.clicks(), 1, 'a changed route cannot trigger restoration on the destination post');
});

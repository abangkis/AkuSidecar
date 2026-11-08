import assert from 'node:assert/strict';
import test from 'node:test';
import {createTemporaryPage} from '../chrome.mjs';
import {validateInit} from '../worker.mjs';
import {windowedIdentityForSource} from '../windowed-identity.mjs';

const metadata = {
  platform: 'Windows', platformVersion: '19.0.0', architecture: 'x86', model: '', mobile: false,
  brands: [{brand: 'Chromium', version: '157'}],
  fullVersionList: [{brand: 'Chromium', version: '157.0.1.2'}],
  bitness: '64', wow64: false, formFactors: ['Desktop'],
};
const windowed = {product: 'Chrome/157.0.1.2', userAgent: 'Mozilla/5.0 Chrome/157.0.0.0', userAgentMetadata: metadata};
const init = {chrome: 'C:/chrome.exe', profile: 'C:/profile', bridgePath: 'C:/bridge'};

test('accepts Windowed identity only for the selected sources and matching Chrome version', () => {
  const options = validateInit({...init, windowedIdentity: windowed, windowedIdentitySources: ['x', 'linkedin']});
  const forSource = (source, version) => windowedIdentityForSource(source, version, options.windowedIdentitySources, options.windowedIdentity);
  assert.deepEqual(forSource('x', windowed.product), {userAgent: windowed.userAgent, userAgentMetadata: metadata});
  assert.deepEqual(forSource('linkedin', 'HeadlessChrome/157.0.1.2'), forSource('x', windowed.product));
  assert.equal(forSource('instagram', 'Chrome/999.0.0.0'), undefined);
  assert.equal(forSource('facebook', windowed.product), undefined);
  assert.throws(() => forSource('x', 'Chrome/157.0.1.3'), {code: 'windowed_identity_unavailable'});
  assert.throws(() => validateInit({...init, windowedIdentitySources: ['x', 'x']}), {code: 'invalid_init'});
});

test('missing or unsafe identity cannot silently use the default UA for an enabled source', () => {
  for (const value of [null, {...windowed, userAgent: 'HeadlessChrome/157.0.0.0'},
    {...windowed, userAgentMetadata: {...metadata, brands: [{brand: 'HeadlessChrome', version: '157'}]}}]) {
    const options = validateInit({...init, windowedIdentity: value, windowedIdentitySources: ['x']});
    assert.throws(() => windowedIdentityForSource('x', windowed.product, options.windowedIdentitySources,
      options.windowedIdentity), {code: 'windowed_identity_unavailable'});
    assert.equal(windowedIdentityForSource('instagram', windowed.product, options.windowedIdentitySources,
      options.windowedIdentity), undefined);
  }
});

test('applies the override to the new target before navigation and retires a rejected target', async () => {
  const override = {userAgent: windowed.userAgent, userAgentMetadata: metadata};
  const calls = [];
  let rejected = false;
  const options = {
    browserSessionId: 'browser', userAgentOverride: override,
    async send(method, params, session) {
      calls.push({method, params, session});
      if (method === 'Target.createTarget') return {targetId: 'fixture'};
      if (method === 'Target.attachToTarget') return {sessionId: 'page'};
      if (rejected && method === 'Emulation.setUserAgentOverride') throw new Error('fixture rejection');
      return {};
    },
    async closeTargetAndWait(targetId) { calls.push({method: 'close', targetId}); },
  };
  const page = await createTemporaryPage(options);
  await page.navigate('http://127.0.0.1/fixture');
  const apply = calls.findIndex(({method}) => method === 'Emulation.setUserAgentOverride');
  assert(apply >= 0 && apply < calls.findIndex(({method}) => method === 'Page.navigate'));
  assert.deepEqual(calls[apply], {method: 'Emulation.setUserAgentOverride', params: override, session: 'page'});
  await page.close();

  calls.length = 0;
  rejected = true;
  await assert.rejects(createTemporaryPage(options), {code: 'windowed_identity_override_failed'});
  assert.deepEqual(calls.at(-1), {method: 'close', targetId: 'fixture'});
});

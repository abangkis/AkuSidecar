import test from 'node:test';
import assert from 'node:assert/strict';
import { resolve } from 'node:path';
import { localPopupChromeArgs, localPopupChildHTML, localPopupHTML } from './x-local-popup-control.mjs';

test('local popup Chrome args replace only Home with a validated loopback origin', () => {
  const profile = resolve('runtime/qa/auth-session-sidecar/popup-profile');
  assert.deepEqual(localPopupChromeArgs(profile, 'http://127.0.0.1:43127'), [
    `--user-data-dir=${profile}`, '--profile-directory=Default', '--no-first-run',
    '--no-default-browser-check', '--new-window', 'http://127.0.0.1:43127/'
  ]);
  for (const origin of [
    'https://127.0.0.1:43127/', 'http://localhost:43127/', 'http://127.0.0.1/',
    'http://127.0.0.1:43127/path', 'http://127.0.0.1:43127/?q=x',
    'http://user@127.0.0.1:43127/', 'http://127.0.0.1:43127/#fragment',
    'http://127.0.0.1:43127/'
  ]) assert.throws(() => localPopupChromeArgs(profile, origin), /invalid_origin/u);
  assert.throws(() => localPopupChromeArgs('relative-profile', 'http://127.0.0.1:43127'), /invalid_profile/u);
});

test('static root page scopes one trusted click and popup page reports only its load', () => {
  const root = localPopupHTML();
  const popup = localPopupChildHTML();
  assert.equal((root.match(/<button\b/gu) ?? []).length, 1);
  assert.match(root, /<button[^>]*autofocus/u);
  assert.match(root, /place-content: center; justify-items: center; width: 100%; min-height: 100vh/u);
  assert.match(root, /Wait for the test instructions\. No account or login is required\. The test will close this window automatically\./u);
  assert.match(root, /fetch\('\/ready', \{ method: 'POST', headers: qaHeaders, body: '' \}\)/u);
  assert.match(root, /const trusted = event\.isTrusted === true;[\s\S]*?try \{ active = navigator\.userActivation\.isActive === true; \} catch \{ active = false; \}[\s\S]*?window\.open\('\/popup', '_blank', 'popup,width=520,height=360'\)/u);
  assert.match(root, /const result = \{ trusted, active, returned: popup !== null \};/u);
  assert.match(root, /fetch\('\/click'[\s\S]*?JSON\.stringify\(result\)/u);
  assert.match(root, /X-Aku-Local-Popup/u);
  assert.equal((root.match(/__AKU_LOCAL_POPUP_NONCE__/gu) ?? []).length, 1);
  assert.match(popup, /fetch\('\/popup-loaded'/u);
  assert.match(popup, /body: ''/u);
  assert.match(popup, /X-Aku-Local-Popup/u);
  assert.equal((popup.match(/__AKU_LOCAL_POPUP_NONCE__/gu) ?? []).length, 1);
  for (const html of [root, popup]) {
    assert.doesNotMatch(html, /https?:\/\//u);
    assert.doesNotMatch(html, /<(?:script|link|img|iframe|audio|video)\b[^>]*(?:src|href)\s*=|url\s*\(/iu);
    assert.doesNotMatch(html, /<input\b|<form\b|x\.com|google\.com|setTimeout\s*\(|setInterval\s*\(|\.click\s*\(/iu);
    assert.doesNotMatch(html, /remote-debugging|disable-(?:web-security|blink)|automation/iu);
  }
});

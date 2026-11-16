import assert from 'node:assert/strict';
import { readdir, readFile, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { root } from './chrome.mjs';

const reports = [];
for (const folder of (await readdir(resolve(root, 'runs'), { withFileTypes: true })).filter(f => f.isDirectory()).sort((a,b) => b.name.localeCompare(a.name))) {
  const report = JSON.parse(await readFile(resolve(root, 'runs', folder.name, 'capture.json'), 'utf8'));
  if (report.target === 'https://x.com/home' && report.profileScope === 'existing_registered_capture') reports.push(report);
}
const prior = reports.flatMap(r => r.posts).filter(p => p.textCollapsed);
if (process.argv.includes('--verify-only')) {
  const saved = JSON.parse(await readFile(resolve(root, 'runs', 'text-validation.json'), 'utf8'));
  for (const result of saved.results) {
    const report = JSON.parse(await readFile(result.capture, 'utf8'));
    const after = report.posts.find(p => p.permalink === report.target);
    const before = prior.find(p => p.id === after?.id);
    assert.ok(before && after);
    const compact = text => text.replace(/\s+/g, ' ').trim();
    assert.ok(compact(after.text).startsWith(compact(before.text).slice(0, 80)));
    assert.ok(after.text.length > before.text.length && !after.textCollapsed);
  }
  console.log(JSON.stringify({ status: 'passed', savedIdentityAndOpeningChecks: saved.results.length }));
  process.exit(0);
}
const quote = prior.find(p => p.quotedPostObserved);
const targets = [quote, ...prior.filter(p => !p.quotedPostObserved).slice(0, 2)].filter(Boolean);
assert.equal(targets.length, 3, 'Need three previously collapsed posts including one quote.');
const results = [];
for (const before of targets) {
  const { stdout } = await promisify(execFile)(process.execPath, [resolve(root, 'shared-profile-test.mjs'), '--url', before.permalink,
    '--max-posts', '5', '--max-scrolls', '0', '--wait-ms', '30000'], { cwd: root, windowsHide: true, timeout: 180000, maxBuffer: 1048576 });
  const logs = stdout.trim().split(/\r?\n/).filter(line => line.startsWith('{')).map(line => JSON.parse(line));
  const captureLog = logs.find(log => log.output);
  const afterReport = JSON.parse(await readFile(captureLog.output, 'utf8'));
  const after = afterReport.posts.find(p => p.id === before.id);
  assert.ok(after, 'Previously observed identity must be captured on its permalink.');
  const compact = text => text.replace(/\s+/g, ' ').trim();
  assert.ok(compact(after.text).startsWith(compact(before.text).slice(0, 80)), 'Expanded text must retain the observed opening.');
  assert.ok(afterReport.samples.every(s => s.authenticatedUiObserved && !s.loginRequired && !s.challengeDetected));
  assert.ok(logs.some(log => log.restored === true), 'Normal Sidecar must be restored.');
  results.push({ capture: captureLog.output, beforeLength: before.text.length, afterLength: after.text.length,
    textGrew: after.text.length > before.text.length, textCollapsed: after.textCollapsed, textStatus: after.textStatus,
    quoteObserved: before.quotedPostObserved, quotedBodyExported: Boolean(after.quotedPost?.text),
    quotedTextCollapsed: after.quotedPost?.textCollapsed ?? null, authenticated: true, restored: true });
  console.log(JSON.stringify(results.at(-1)));
}
const output = resolve(root, 'runs', 'text-validation.json');
await writeFile(output, JSON.stringify({ testedAt: new Date().toISOString(), results }, null, 2) + '\n');
console.log(JSON.stringify({ output, tested: results.length, longer: results.filter(r => r.textGrew).length,
  stillCollapsed: results.filter(r => r.textCollapsed).length, quotesExported: results.filter(r => r.quotedBodyExported).length }));

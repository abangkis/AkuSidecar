// Select an already observed X video post from private local PoC results.
// No invented URL or account data is printed; lifecycle handling stays in wrapper.
import { readdir, readFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { spawn } from 'node:child_process';
import { root } from './chrome.mjs';

const folders = (await readdir(resolve(root, 'runs'), { withFileTypes: true }))
  .filter(f => f.isDirectory()).map(f => f.name).sort().reverse();
let target;
for (const folder of folders) {
  const report = JSON.parse(await readFile(resolve(root, 'runs', folder, 'capture.json'), 'utf8'));
  if (report.profileScope !== 'existing_registered_capture' || (report.source || 'x') !== 'x') continue;
  target = report.posts?.find(p => p.media?.some(m => m.kind === 'video_poster'));
  if (target) break;
}
if (!target) throw new Error('No previously observed video post available for targeted test.');
const child = spawn(process.execPath, [resolve(root, 'shared-profile-test.mjs'), '--url', target.permalink,
  '--max-posts', '2', '--max-scrolls', '0', '--wait-ms', '30000'], { cwd: root, windowsHide: true, stdio: 'inherit' });
child.on('error', error => { console.error(error.message); process.exitCode = 1; });
child.on('exit', code => { process.exitCode = code ?? 1; });

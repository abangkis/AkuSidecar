// Launch-only control. No browser connection, page reads or credential access.
import { spawn } from 'node:child_process';
import { lstat, mkdir, realpath, writeFile } from 'node:fs/promises';
import { dirname, isAbsolute, join, relative, resolve, sep } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { parseStrictJSON } from './x-evidence-ingress.mjs';

export function standardChromeArgs(profile) {
  if (typeof profile !== 'string' || !isAbsolute(profile)) throw new Error('invalid_profile');
  return [`--user-data-dir=${profile}`, '--profile-directory=Default', '--no-first-run',
    '--no-default-browser-check', '--new-window', 'https://x.com/home'];
}

export function decodeControl(bytes) {
  if (!(bytes instanceof Uint8Array) || bytes.length < 1 || bytes.length > 4096) throw new Error('invalid_control');
  try {
    const value = parseStrictJSON(new TextDecoder('utf-8', { fatal: true }).decode(bytes),
      { maxJSONDepth: 4, maxJSONTokens: 64 });
    if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error();
    return value;
  } catch { throw new Error('invalid_control'); }
}

async function main() {
  const [executable, root, origin, ...extra] = process.argv.slice(2);
  if (extra.length || !isAbsolute(executable ?? '') || !isAbsolute(root ?? '')) throw new Error();
  const control = new URL(origin);
  if (control.protocol !== 'http:' || control.hostname !== '127.0.0.1' || !control.port ||
      control.origin !== origin || control.username || control.password) throw new Error();
  const project = await realpath(resolve(dirname(fileURLToPath(import.meta.url)), '..', '..'));
  const parent = await realpath(join(project, 'runtime', 'qa', 'auth-session-sidecar'));
  const actualRoot = await realpath(root);
  const rel = relative(parent, actualRoot);
  const stat = await lstat(root);
  if (!stat.isDirectory() || stat.isSymbolicLink() || actualRoot.toLowerCase() !== resolve(root).toLowerCase() ||
      !rel || isAbsolute(rel) || rel === '..' || rel.startsWith(`..${sep}`) ||
      !/^run-x-standard-chrome-[a-f0-9]+$/u.test(actualRoot.split(/[\\/]/u).at(-1))) throw new Error();
  let bootstrapped = false;
  let launched = false;
  let pending = Buffer.alloc(0);
  let lines = 0;
  let chain = Promise.resolve();
  let child;
  let exited = false;
  const emit = value => process.stdout.write(`${JSON.stringify(value)}\n`);
  const handle = async bytes => {
    const message = decodeControl(bytes);
    if (!bootstrapped) {
      if (Object.keys(message).sort().join(',') !== 'op,withholdReady' ||
          message.op !== 'fixture_bootstrap' || message.withholdReady !== false) throw new Error();
      bootstrapped = true;
      emit({ ready: true });
      return;
    }
    const id = message.id;
    if (!Number.isSafeInteger(id) || id < 1) throw new Error();
    if (launched || Object.keys(message).sort().join(',') !== 'id,nativeReaderStartup,op,role' ||
        message.op !== 'launch' || message.role !== 'collector' || message.nativeReaderStartup !== false) {
      emit({ id, ok: false, code: 'control_operation_invalid' });
      return;
    }
    launched = true;
    const profile = join(actualRoot, 'shared');
    await mkdir(profile, { recursive: false, mode: 0o700 });
    await mkdir(join(profile, 'Default'), { mode: 0o700 });
    await writeFile(join(profile, 'Default', 'Preferences'),
      JSON.stringify({ profile: { name: 'AkuBrowser X QA Control' } }), { flag: 'wx', mode: 0o600 });
    child = spawn(executable, standardChromeArgs(profile), { windowsHide: false, stdio: 'ignore' });
    await new Promise((accept, reject) => { child.once('spawn', accept); child.once('error', reject); });
    child.on('error', () => { exited = true; });
    child.once('exit', () => { exited = true; });
    emit({ id, ok: true, result: { owner: 1, pid: child.pid, windowCount: 0,
      bootstrapCount: 0, nativeReaderStartup: false } });
  };
  process.stdin.on('data', chunk => {
    if (chunk.length > 8192 || pending.length + chunk.length > 8192) { process.stdin.destroy(); return; }
    pending = Buffer.concat([pending, chunk]);
    let index;
    while ((index = pending.indexOf(10)) >= 0) {
      if (++lines > 8 || index < 1 || index > 4096) { process.stdin.destroy(); return; }
      const line = Buffer.from(pending.subarray(0, index));
      pending = Buffer.from(pending.subarray(index + 1));
      chain = chain.then(() => handle(line)).catch(() => { emit({ ready: false }); process.stdin.destroy(); });
    }
    if (pending.length > 4096) process.stdin.destroy();
  });
  // The enclosing Go Job owns all processes. Do not abandon a live child on EOF.
  process.stdin.on('end', () => { if (!child || exited) process.exit(0); });
  process.stdin.on('error', () => { if (!child || exited) process.exit(2); });
}

if (process.argv[1] && pathToFileURL(resolve(process.argv[1])).href === import.meta.url) {
  void main().catch(() => { process.exitCode = 2; });
}

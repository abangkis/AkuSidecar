// Local popup QA control. No browser connection, page reads, or credential access.
import { spawn } from 'node:child_process';
import { lstat, mkdir, realpath, writeFile } from 'node:fs/promises';
import { dirname, isAbsolute, join, relative, resolve, sep } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { decodeControl, standardChromeArgs } from './x-standard-chrome-control.mjs';

const NONCE_PLACEHOLDER = '__AKU_LOCAL_POPUP_NONCE__';

function parseLocalOrigin(origin) {
  if (typeof origin !== 'string' || origin.length > 128) throw new Error('invalid_origin');
  let parsed;
  try { parsed = new URL(origin); } catch { throw new Error('invalid_origin'); }
  if (parsed.protocol !== 'http:' || parsed.hostname !== '127.0.0.1' || !parsed.port ||
      parsed.username || parsed.password || parsed.pathname !== '/' || parsed.search || parsed.hash ||
      parsed.origin !== origin) throw new Error('invalid_origin');
  return origin;
}

export function localPopupChromeArgs(profile, origin) {
  parseLocalOrigin(origin);
  const args = standardChromeArgs(profile);
  args[args.length - 1] = `${origin}/`;
  return args;
}

export function localPopupHTML() {
  return `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>AkuBrowser Local Popup QA</title>
  <style>
    :root { color-scheme: light; font-family: system-ui, sans-serif; }
    html, body { width: 100%; height: 100%; margin: 0; }
    body { display: grid; }
    button { display: grid; place-content: center; justify-items: center; width: 100%; min-height: 100vh; border: 0; padding: 2rem; background: #f3f6fa; color: #14243a; cursor: pointer; text-align: center; }
    button:focus-visible { outline: 5px solid #2463eb; outline-offset: -8px; }
    strong { display: block; font-size: clamp(2rem, 7vw, 4rem); line-height: 1.1; }
    span { display: block; max-width: 42rem; margin: 1.25rem auto 0; font-size: clamp(1.1rem, 2.5vw, 1.5rem); line-height: 1.5; }
  </style>
</head>
<body>
  <button id="open-popup" type="button" autofocus>
    <strong>Open local popup</strong>
    <span>Wait for the test instructions. No account or login is required. The test will close this window automatically.</span>
  </button>
  <script>
    const qaHeaders = { 'X-Aku-Local-Popup': '${NONCE_PLACEHOLDER}' };
    document.addEventListener('DOMContentLoaded', () => {
      void fetch('/ready', { method: 'POST', headers: qaHeaders, body: '' }).catch(() => {});
    }, { once: true });
    const button = document.getElementById('open-popup');
    button.addEventListener('click', event => {
      const trusted = event.isTrusted === true;
      let active = false;
      try { active = navigator.userActivation.isActive === true; } catch { active = false; }
      let popup = null;
      try { popup = window.open('/popup', '_blank', 'popup,width=520,height=360'); } catch { popup = null; }
      const result = { trusted, active, returned: popup !== null };
      button.disabled = true;
      void fetch('/click', {
        method: 'POST',
        headers: { ...qaHeaders, 'Content-Type': 'application/json' },
        body: JSON.stringify(result)
      }).catch(() => {});
    }, { once: true });
  </script>
</body>
</html>`;
}

export function localPopupChildHTML() {
  return `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>AkuBrowser Local Popup QA Result</title>
  <style>
    :root { color-scheme: light; font-family: system-ui, sans-serif; }
    html, body { width: 100%; height: 100%; margin: 0; }
    body { display: grid; place-items: center; background: #f3f6fa; color: #14243a; text-align: center; }
    main { padding: 2rem; }
    h1 { margin: 0; font-size: clamp(2rem, 7vw, 3rem); }
    p { margin: 1rem 0 0; font-size: 1.1rem; line-height: 1.5; }
  </style>
</head>
<body>
  <main><h1>Local popup opened</h1><p>This accountless QA page made no external requests.</p></main>
  <script>
    document.addEventListener('DOMContentLoaded', () => {
      void fetch('/popup-loaded', {
        method: 'POST',
        headers: { 'X-Aku-Local-Popup': '${NONCE_PLACEHOLDER}' },
        body: ''
      }).catch(() => {});
    }, { once: true });
  </script>
</body>
</html>`;
}

async function main() {
  const [executable, root, origin, ...extra] = process.argv.slice(2);
  if (extra.length || !isAbsolute(executable ?? '') || !isAbsolute(root ?? '')) throw new Error();
  const controlOrigin = parseLocalOrigin(origin);
  const project = await realpath(resolve(dirname(fileURLToPath(import.meta.url)), '..', '..'));
  const parent = await realpath(join(project, 'runtime', 'qa', 'auth-session-sidecar'));
  const actualRoot = await realpath(root);
  const rel = relative(parent, actualRoot);
  const stat = await lstat(root);
  if (!stat.isDirectory() || stat.isSymbolicLink() || actualRoot.toLowerCase() !== resolve(root).toLowerCase() ||
      !rel || isAbsolute(rel) || rel === '..' || rel.startsWith(`..${sep}`) ||
      !/^run-x-local-popup-[a-f0-9]{8,64}$/u.test(actualRoot.split(/[\\/]/u).at(-1))) throw new Error();
  let bootstrapped = false;
  let documentsProvided = false;
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
    if (message.op === 'local_popup_documents') {
      if (launched || documentsProvided || Object.keys(message).sort().join(',') !== 'id,op') {
        emit({ id, ok: false, code: 'control_operation_invalid' });
        return;
      }
      documentsProvided = true;
      emit({ id, ok: true, result: { root: localPopupHTML(), popup: localPopupChildHTML() } });
      return;
    }
    if (!documentsProvided || launched || Object.keys(message).sort().join(',') !== 'id,nativeReaderStartup,op,role' ||
        message.op !== 'launch' || message.role !== 'collector' || message.nativeReaderStartup !== false) {
      emit({ id, ok: false, code: 'control_operation_invalid' });
      return;
    }
    launched = true;
    const profile = join(actualRoot, 'shared');
    await mkdir(profile, { recursive: false, mode: 0o700 });
    await mkdir(join(profile, 'Default'), { mode: 0o700 });
    await writeFile(join(profile, 'Default', 'Preferences'),
      JSON.stringify({ profile: { name: 'AkuBrowser Local Popup QA' } }), { flag: 'wx', mode: 0o600 });
    child = spawn(executable, localPopupChromeArgs(profile, controlOrigin), { windowsHide: false, stdio: 'ignore' });
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
      if (++lines > 3 || index < 1 || index > 4096) { process.stdin.destroy(); return; }
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

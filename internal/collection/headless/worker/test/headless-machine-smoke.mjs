import { createServer } from 'node:http';
import { access, mkdir, realpath } from 'node:fs/promises';
import { basename, dirname, isAbsolute, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { launchChrome } from '../chrome.mjs';

const projectRoot = resolve(dirname(fileURLToPath(import.meta.url)), '../../../../../');
const buildRoot = resolve(projectRoot, 'build');
const cookieName = 'aku_headless_machine_smoke';
const cookieValue = 'shared';
const cookie = `${cookieName}=${cookieValue}`;
const observedCookies = [];
let stage = 'arguments';

function fail(message) {
  throw new Error(message);
}

function parseArguments(args) {
  const options = {};
  for (let index = 0; index < args.length; index++) {
    const key = args[index];
    if (!['--chrome', '--profile'].includes(key) || options[key]) fail('Expected one --chrome and one --profile argument.');
    const value = args[++index];
    if (!value || value.startsWith('--')) fail(`Missing value for ${key}.`);
    options[key] = value;
  }
  const chromePath = options['--chrome'];
  const profilePath = options['--profile'];
  if (!chromePath || !profilePath || !isAbsolute(chromePath) || !isAbsolute(profilePath)) {
    fail('Both --chrome and --profile must be absolute paths.');
  }
  return { chromePath: resolve(chromePath), profilePath: resolve(profilePath) };
}

async function validateFreshProfile(profilePath) {
  await mkdir(buildRoot, { recursive: true });
  const actualBuildRoot = await realpath(buildRoot);
  if (dirname(profilePath) !== actualBuildRoot || !basename(profilePath).startsWith('headless-machine-smoke-')) {
    fail('Profile must be a new direct child of AkuSidecar/build named headless-machine-smoke-*.');
  }
  try { await access(profilePath); } catch (error) {
    if (error.code === 'ENOENT') return;
    fail('Could not verify that the profile path is new.');
  }
  fail('Profile path already exists; use a new unique path under AkuSidecar/build.');
}

function createFixtureServer() {
  return createServer((request, response) => {
    const path = new URL(request.url, 'http://127.0.0.1').pathname;
    if (path === '/seed') {
      response.setHeader('Set-Cookie', `${cookie}; Path=/; HttpOnly; SameSite=Lax; Max-Age=3600`);
      response.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
      response.end('<!doctype html><title>fixture seed</title>');
      return;
    }
    if (path === '/observe') observedCookies.push(request.headers.cookie || '');
    response.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
    response.end('<!doctype html><title>fixture page</title>');
  });
}

async function waitForPage(page, pathname, search) {
  const deadline = Date.now() + 5000;
  while (Date.now() < deadline) {
    try {
      if (await page.evaluate(`location.pathname === ${JSON.stringify(pathname)} && location.search === ${JSON.stringify(search)} && document.readyState === 'complete'`)) return;
    } catch {}
    await new Promise(resolveWait => setTimeout(resolveWait, 50));
  }
  fail('Loopback page did not finish loading in time.');
}

async function navigate(page, url) {
  const result = await page.navigate(url);
  if (result.errorText) fail('Chrome rejected a loopback fixture navigation.');
  const target = new URL(url);
  await waitForPage(page, target.pathname, target.search);
}

function assertCookieAt(index) {
  const value = observedCookies[index] || '';
  if (!value.split(';').map(part => part.trim()).includes(cookie)) fail('A source page did not receive the shared loopback cookie.');
}

async function closeServer(server) {
  if (!server?.listening) return;
  await new Promise((resolveClose, rejectClose) => server.close(error => error ? rejectClose(error) : resolveClose()));
}

async function run() {
  const { chromePath, profilePath } = parseArguments(process.argv.slice(2));
  await validateFreshProfile(profilePath);
  const server = createFixtureServer();
  let firstRun;
  let secondRun;
  try {
    await new Promise((resolveListen, rejectListen) => {
      server.once('error', rejectListen);
      server.listen(0, '127.0.0.1', resolveListen);
    });
    const baseUrl = `http://127.0.0.1:${server.address().port}`;
    stage = 'first_launch';
    firstRun = await launchChrome({ chromePath, profilePath });
    if (observedCookies.length !== 0) fail('The fixture received a request before a source page navigated.');
    stage = 'hidden_source_contexts';
    const x = await firstRun.forSource('x');
    const facebook = await firstRun.forSource('facebook');
    if (x === facebook) fail('Source pages must use distinct CDP targets.');
    stage = 'cookie_seed';
    await navigate(x, `${baseUrl}/seed`);
    await navigate(x, `${baseUrl}/observe?source=x`);
    await navigate(facebook, `${baseUrl}/observe?source=facebook`);
    if (observedCookies.length !== 2) fail('Both source pages must reach the local fixture.');
    assertCookieAt(0);
    assertCookieAt(1);
    stage = 'first_close';
    await firstRun.close();
    firstRun = null;

    stage = 'second_launch';
    secondRun = await launchChrome({ chromePath, profilePath });
    stage = 'cookie_restart';
    const reopenedSource = await secondRun.forSource('x');
    await navigate(reopenedSource, `${baseUrl}/observe?source=restart`);
    if (observedCookies.length !== 3) fail('The reopened source page must reach the local fixture.');
    assertCookieAt(2);
    await secondRun.close();
    secondRun = null;
  } finally {
    await Promise.allSettled([firstRun?.close(), secondRun?.close()]);
    await closeServer(server);
  }
}

run().then(() => {
  process.stdout.write('headless-machine-smoke passed: hidden source targets share and retain a loopback cookie.\n');
}).catch(error => {
  const diagnostic = String(error.message).split(' Chrome diagnostic:')[0]
    .replace(/https?:\/\/\S+/g, '[url]').replace(/[A-Za-z]:[\\/][^\r\n]*/g, '[path]');
  process.stderr.write(`headless-machine-smoke failed at ${stage}: ${diagnostic.slice(0, 300)}\n`);
  process.exitCode = 1;
});

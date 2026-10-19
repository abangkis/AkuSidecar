// Private fixture RPC: no native URL, profile, cookie or arbitrary CDP inputs.
import { lstat, realpath, readdir } from 'node:fs/promises';
import { isAbsolute, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createXSyntheticChromeAdapter, concurrencyFixtureOrigin, concurrencyFixtureRoot } from './x-concurrency-chrome-adapter.mjs';

const denied = () => new Error('synthetic_concurrency_operation_denied');
function exact(value, names) {
  return value && typeof value === 'object' && !Array.isArray(value) &&
    Object.keys(value).length === names.length && names.every(name => Object.hasOwn(value, name));
}

export function createConcurrencyFixtureRPC(adapter) {
  let bootstrapped = false, prepared = 'new', used = false, closed = false;
  return async message => {
    if (!bootstrapped) {
      if (!exact(message, ['op', 'withholdReady']) || message.op !== 'fixture_bootstrap' || message.withholdReady !== false) throw denied();
      bootstrapped = true;
      return { ready: true };
    }
    if (!Number.isSafeInteger(message?.id) || message.id < 1 || closed) throw denied();
    if (message.op === 'concurrency_prepare' && exact(message, ['id', 'op']) && prepared === 'new') {
      prepared = 'starting';
      try {
        const result = await adapter.prepare();
        prepared = 'ready';
        return { id: message.id, ok: true, result };
      } catch { prepared = 'failed'; throw denied(); }
    }
    if (message.op === 'concurrency_run' && exact(message, ['id', 'op', 'scenario']) && prepared === 'ready' && !used &&
        ['overlap', 'mutation', 'coverage_loss'].includes(message.scenario)) {
      used = true;
      try { return { id: message.id, ok: true, result: await adapter.run(message.scenario) }; }
      catch { throw denied(); }
    }
    if (message.op === 'concurrency_close' && exact(message, ['id', 'op'])) {
      closed = true;
      try { return { id: message.id, ok: true, result: await adapter.close() }; }
      catch { throw denied(); }
    }
    throw denied();
  };
}

async function main() {
  const args = process.argv.slice(2);
  if (args.length !== 3 || !isAbsolute(args[0] ?? '')) throw denied();
  const [executable, root, origin] = args;
  concurrencyFixtureOrigin(origin); concurrencyFixtureRoot(root);
  const stat = await lstat(root);
  if (!stat.isDirectory() || stat.isSymbolicLink() || (await realpath(root)).toLowerCase() !== resolve(root).toLowerCase() ||
      (await readdir(root)).length !== 0) throw denied();
  const adapter = createXSyntheticChromeAdapter({ executable, root, origin });
  const handle = createConcurrencyFixtureRPC(adapter);
  let buffered = Buffer.alloc(0), count = 0, chain = Promise.resolve(), ending = false;
  const emit = value => process.stdout.write(JSON.stringify(value) + '\n');
  const stop = () => {
    if (ending) return;
    ending = true;
    void chain.finally(() => adapter.close()).catch(() => {}).finally(() => { process.exitCode = 1; });
  };
  process.stdin.on('data', chunk => {
    if (ending) return;
    if (chunk.length > 4096 || buffered.length + chunk.length > 4096) { stop(); process.stdin.destroy(); return; }
    buffered = Buffer.concat([buffered, chunk]);
    for (let index; (index = buffered.indexOf(10)) >= 0;) {
      const line = buffered.subarray(0, index).toString('utf8');
      buffered = buffered.subarray(index + 1);
      if (++count > 6) { stop(); process.stdin.destroy(); return; }
      chain = chain.then(async () => {
        let message;
        try { message = JSON.parse(line); emit(await handle(message)); }
        catch {
          if (!Number.isSafeInteger(message?.id) || message.id < 1) { stop(); process.stdin.destroy(); return; }
          emit({ id: message.id, ok: false, code: adapter.diagnostic() });
        }
      });
    }
  });
  process.stdin.on('end', stop);
  process.stdin.on('error', stop);
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main().catch(() => { process.stderr.write('synthetic_concurrency_driver_failed\n'); process.exitCode = 1; });
}

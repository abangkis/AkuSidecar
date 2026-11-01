import assert from 'node:assert/strict';
import {existsSync, lstatSync, readFileSync, statSync} from 'node:fs';
import {mkdir, mkdtemp, rm, writeFile} from 'node:fs/promises';
import test from 'node:test';
import {dirname, join, resolve} from 'node:path';
import {fileURLToPath} from 'node:url';
import {resolveSourceBuildCachePaths} from './source-cache.mjs';

const repository = resolve(dirname(fileURLToPath(import.meta.url)), '..');
function nearestSharedTemp(root) {
  let cursor = root;
  while (true) {
    const candidate = join(cursor, 'SharedTemp');
    try {
      if (statSync(candidate).isDirectory()) return resolve(candidate);
    } catch {}
    const parent = dirname(cursor);
    if (parent === cursor) return join(root, 'SharedTemp');
    cursor = parent;
  }
}
function configuredOrFallbackRoot(root) {
  const configPath = join(repository, '.cache', 'source-build-cache.json');
  if (existsSync(configPath)) {
    const config = JSON.parse(readFileSync(configPath, 'utf8'));
    if (config.schemaVersion === 1 && typeof config.root === 'string') return resolve(config.root);
  }
  return join(nearestSharedTemp(repository), 'build-cache');
}

test('defaults use the configured cache root or nearest SharedTemp without creating caches', () => {
  const root = configuredOrFallbackRoot(repository);
  assert.deepEqual(resolveSourceBuildCachePaths(repository, {}), {
    GoBuild: join(root, 'go-build'),
    GoModules: join(root, 'go-mod'),
    Npm: join(root, 'npm'),
  });
});

test('nonempty incoming cache variables take precedence', () => {
  const environment = Object.freeze({
    GOCACHE: 'D:\\cache\\go-build',
    GOMODCACHE: 'D:\\cache\\go-mod',
    npm_config_cache: 'D:\\cache\\npm',
  });
  assert.deepEqual(resolveSourceBuildCachePaths(repository, environment), {
    GoBuild: environment.GOCACHE,
    GoModules: environment.GOMODCACHE,
    Npm: environment.npm_config_cache,
  });
});

test('repo-local source cache config is read and validated without creating the target', async t => {
  const sharedRoot = nearestSharedTemp(repository);
  const fixtureRoot = await mkdtemp(join(sharedRoot, 'source-cache-config-test-'));
  const resolvedFixture = resolve(fixtureRoot);
  assert.equal(dirname(resolvedFixture), sharedRoot);
  t.after(async () => {
    assert.equal(resolve(fixtureRoot), resolvedFixture);
    assert.equal(dirname(resolvedFixture), sharedRoot);
    assert.equal(lstatSync(resolvedFixture).isSymbolicLink(), false);
    await rm(resolvedFixture, {recursive: true, force: false});
  });

  const fakeRepository = join(fixtureRoot, 'repository');
  const configDirectory = join(fakeRepository, '.cache');
  const cacheRoot = join(fixtureRoot, 'configured-cache-root');
  await mkdir(configDirectory, {recursive: true});
  const configPath = join(configDirectory, 'source-build-cache.json');
  await writeFile(configPath, JSON.stringify({schemaVersion: 1, root: cacheRoot}));
  assert.deepEqual(resolveSourceBuildCachePaths(fakeRepository, {}), {
    GoBuild: join(cacheRoot, 'go-build'),
    GoModules: join(cacheRoot, 'go-mod'),
    Npm: join(cacheRoot, 'npm'),
  });
  assert.equal(existsSync(cacheRoot), false);

  await writeFile(configPath, JSON.stringify({schemaVersion: 2, root: cacheRoot}));
  assert.throws(() => resolveSourceBuildCachePaths(fakeRepository, {}), /schemaVersion must be 1/);
  await writeFile(configPath, JSON.stringify({schemaVersion: 1, root: 'C:relative'}));
  assert.throws(() => resolveSourceBuildCachePaths(fakeRepository, {}), /nonempty absolute path/);
});

test('falls back to Repository/SharedTemp when no ancestor SharedTemp exists', {
  skip: existsSync(join(resolve(repository).slice(0, 3), 'SharedTemp')),
}, () => {
  const driveRoot = resolve(repository).slice(0, 3);
  const fakeRepository = join(driveRoot, '__source_cache_fallback_fixture__', 'repo');
  assert.deepEqual(resolveSourceBuildCachePaths(fakeRepository, {}), {
    GoBuild: join(fakeRepository, 'SharedTemp', 'build-cache', 'go-build'),
    GoModules: join(fakeRepository, 'SharedTemp', 'build-cache', 'go-mod'),
    Npm: join(fakeRepository, 'SharedTemp', 'build-cache', 'npm'),
  });
});
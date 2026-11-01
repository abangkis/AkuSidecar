import { existsSync, readFileSync, statSync } from 'node:fs';
import { dirname, isAbsolute, join, resolve } from 'node:path';

function isDirectory(path) {
  try {
    return statSync(path).isDirectory();
  } catch {
    return false;
  }
}

function resolveCacheRoot(repository) {
  const repositoryPath = resolve(repository);
  const configPath = join(repositoryPath, '.cache', 'source-build-cache.json');
  if (existsSync(configPath)) {
    let configuration;
    try {
      configuration = JSON.parse(readFileSync(configPath, 'utf8'));
    } catch {
      throw new Error('Source cache configuration is invalid JSON: ' + configPath);
    }
    if (configuration?.schemaVersion !== 1) {
      throw new Error('Source cache configuration schemaVersion must be 1: ' + configPath);
    }
    if (typeof configuration.root !== 'string' || configuration.root.trim() === '' || !isAbsolute(configuration.root)) {
      throw new Error('Source cache configuration root must be a nonempty absolute path: ' + configPath);
    }
    return resolve(configuration.root);
  }

  let cursor = repositoryPath;
  while (true) {
    const candidate = join(cursor, 'SharedTemp');
    if (existsSync(candidate) && isDirectory(candidate)) return resolve(candidate, 'build-cache');
    const parent = dirname(cursor);
    if (parent === cursor) break;
    cursor = parent;
  }
  return join(repositoryPath, 'SharedTemp', 'build-cache');
}

export function resolveSourceBuildCachePaths(repository, environment = process.env) {
  const cacheRoot = resolveCacheRoot(repository);
  const firstNonEmpty = (...values) => values.find(value => typeof value === 'string' && value.trim() !== '');
  return {
    GoBuild: firstNonEmpty(environment.GOCACHE) ?? join(cacheRoot, 'go-build'),
    GoModules: firstNonEmpty(environment.GOMODCACHE) ?? join(cacheRoot, 'go-mod'),
    Npm: firstNonEmpty(environment.npm_config_cache, environment.NPM_CONFIG_CACHE) ?? join(cacheRoot, 'npm'),
  };
}
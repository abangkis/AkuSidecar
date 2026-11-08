const knownSources = new Set(['x', 'facebook', 'instagram', 'linkedin']);
const chromeVersion = product => typeof product === 'string'
  ? /^(?:Chrome|HeadlessChrome)\/(\d+(?:\.\d+){3})$/.exec(product)?.[1] : undefined;
const safeText = (value, max = 64) => typeof value === 'string' &&
  value.length <= max && !/[\x00-\x1f\x7f]/.test(value);

export function validateIdentitySources(sources = []) {
  if (!Array.isArray(sources) || sources.length > knownSources.size ||
      sources.some(source => !knownSources.has(source)) || new Set(sources).size !== sources.length) {
    throw Object.assign(new Error('Invalid Windowed identity source policy.'), { code: 'invalid_init' });
  }
  return sources;
}

export function parseWindowedIdentity(value) {
  const version = chromeVersion(value?.product);
  const userAgent = value?.userAgent;
  const data = value?.userAgentMetadata;
  if (!version || !safeText(userAgent, 1024) || !userAgent.trim() || /\bHeadlessChrome\//i.test(userAgent) ||
      !data || typeof data.mobile !== 'boolean') return undefined;

  const userAgentMetadata = {mobile: data.mobile};
  for (const key of ['platform', 'platformVersion', 'architecture', 'model']) {
    if (!safeText(data[key])) return undefined;
    userAgentMetadata[key] = data[key];
  }
  for (const key of ['brands', 'fullVersionList']) {
    const entries = data[key];
    if (!Array.isArray(entries) || entries.length < 1 || entries.length > 8 ||
        entries.some(entry => !safeText(entry?.brand) || !entry.brand.trim() ||
          /[\r\n]/.test(entry.brand) || typeof entry.version !== 'string' || !/^\d{1,6}(?:\.\d{1,6}){0,3}$/.test(entry.version))) return undefined;
    if (entries.some(entry => /^HeadlessChrome$/i.test(entry.brand))) return undefined;
    userAgentMetadata[key] = entries.map(({brand, version}) => ({brand, version}));
  }
  if (safeText(data.bitness)) userAgentMetadata.bitness = data.bitness;
  if (typeof data.wow64 === 'boolean') userAgentMetadata.wow64 = data.wow64;
  if (Array.isArray(data.formFactors) && data.formFactors.length <= 8 &&
      data.formFactors.every(value => safeText(value))) userAgentMetadata.formFactors = data.formFactors;
  return {version, override: {userAgent, userAgentMetadata}};
}

export function windowedIdentityForSource(source, product, sources, identity) {
  if (!sources.includes(source)) return undefined;
  if (!identity || identity.version !== chromeVersion(product)) {
    throw Object.assign(new Error('A matching Windowed Chrome identity is unavailable for this source.'), {
      code: 'windowed_identity_unavailable',
    });
  }
  return identity.override;
}

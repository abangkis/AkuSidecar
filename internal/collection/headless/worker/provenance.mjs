export function sourceProvenance(assets) {
  return {
    schema: 'aku.headless-source-provenance.v1',
    algorithm: 'sha256',
    sources: assets.map(({ relative, sha256 }) => ({ path: relative, sha256 })),
  };
}

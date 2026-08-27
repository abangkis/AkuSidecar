export function providerRequiresSecureCredential(provider) {
  return Boolean(provider?.credentialName) && provider.configurationStatus !== "ready";
}

export function providerCanActivate(provider) {
  return Boolean(provider) && provider.configured !== false && !providerRequiresSecureCredential(provider);
}

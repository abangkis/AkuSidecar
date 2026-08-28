export function providerRequiresSecureCredential(provider) {
  return Boolean(provider?.credentialName) && provider.configurationStatus !== "ready";
}

export function providerCanActivate(provider) {
  if (!provider || provider.configured === false || providerRequiresSecureCredential(provider)) return false;
  if (provider.availabilityRequired) {
    return provider.availabilityChecked === true && provider.available === true;
  }
  return true;
}

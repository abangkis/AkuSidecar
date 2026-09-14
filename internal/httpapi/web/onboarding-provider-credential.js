export function providerRequiresSecureCredential(provider) {
  return Boolean(provider?.credentialName) && provider.configurationStatus !== "ready";
}

export function providerHasSecureCredential(provider) {
  return Boolean(provider?.credentialName) && provider.configured === true && !providerRequiresSecureCredential(provider);
}

export function providerReadinessFeedback(provider) {
  if (provider?.credentialName && !providerHasSecureCredential(provider)) {
    return { label: "Key required", className: "is-unavailable" };
  }
  if (provider?.availabilityRequired && provider.availabilityChecked) {
    return {
      label: provider.available ? "Ready" : "Unavailable",
      className: provider.available ? "is-ready" : "is-unavailable",
    };
  }
  if (providerHasSecureCredential(provider)) return { label: "Key saved", className: "is-ready" };
  return { label: "Not checked", className: "" };
}

export function providerCanActivate(provider) {
  if (!provider || provider.configured === false) return false;
  if (provider.credentialName && !providerHasSecureCredential(provider)) return false;
  if (provider.availabilityRequired) {
    return provider.availabilityChecked === true && provider.available === true;
  }
  return true;
}

export function applyReasoningRuntimeResponse(bootstrap, response = {}) {
  if (!bootstrap || !response) return bootstrap;

  if (response.settings) bootstrap.settings = response.settings;
  if (response.reasoningProviders) bootstrap.reasoningProviders = response.reasoningProviders;
  if (response.reasoningRuntime) bootstrap.reasoningRuntime = response.reasoningRuntime;
  if (response.reasoningProcesses) bootstrap.reasoningProcesses = response.reasoningProcesses;
  if (response.mediaProvenanceRuntime) bootstrap.mediaProvenanceRuntime = response.mediaProvenanceRuntime;

  const activeProvider = response.provider || response.settings?.reasoningProvider;
  if (activeProvider) bootstrap.provider = activeProvider;
  return bootstrap;
}

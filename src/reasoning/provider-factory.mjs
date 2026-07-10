export async function createReasoningProvider(config) {
  switch (config.provider) {
    case "codex-sdk": {
      const { CodexSdkReasoningProvider } = await import("./codex-sdk-provider.mjs");
      return new CodexSdkReasoningProvider(config);
    }
    case "deterministic": {
      const { DeterministicReasoningProvider } = await import(
        "./deterministic-provider.mjs"
      );
      return new DeterministicReasoningProvider();
    }
    default:
      throw new Error(`Unknown reasoning provider: ${config.provider}`);
  }
}

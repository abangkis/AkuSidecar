// Fixed entry point for the separately approved two-phase QA workflow.
// The phase selection is code-defined and cannot be changed through env or CLI.
import { runNativeMetadataProbe } from './x-native-metadata-probe-driver.mjs';

void runNativeMetadataProbe('two_phase').catch(() => {
  process.exitCode = 2;
  process.stdin.destroy();
});

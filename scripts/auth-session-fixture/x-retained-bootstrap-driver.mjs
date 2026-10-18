// Fixed bootstrap-only diagnostic entrypoint; requires the retained QA lease.
import { runNativeMetadataProbe } from './x-native-metadata-probe-driver.mjs';

void runNativeMetadataProbe('retained_bootstrap').catch(() => {
  process.exitCode = 2;
  process.stdin.destroy();
});

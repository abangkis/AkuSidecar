// Fixed opt-in diagnostic entry point; normal metadata modes cannot read bodies.
import { runNativeMetadataProbe } from './x-native-metadata-probe-driver.mjs';

void runNativeMetadataProbe('two_phase_account_settings').catch(() => {
  process.exitCode = 2;
  process.stdin.destroy();
});

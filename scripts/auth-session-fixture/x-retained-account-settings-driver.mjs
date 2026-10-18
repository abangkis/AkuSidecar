import { runNativeMetadataProbe } from './x-native-metadata-probe-driver.mjs';

void runNativeMetadataProbe('retained_account_settings').catch(() => {
  process.exitCode = 2;
  process.stdin.destroy();
});

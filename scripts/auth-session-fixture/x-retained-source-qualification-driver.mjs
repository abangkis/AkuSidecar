// Dedicated retained-QA entrypoint. No transfer command or caller-supplied scripts.
import { runNativeMetadataProbe } from './x-native-metadata-probe-driver.mjs';
void runNativeMetadataProbe('retained_source_qualification').catch(() => {
  process.stderr.write('qa_source_driver_failed\n');
  process.exitCode = 2;
});

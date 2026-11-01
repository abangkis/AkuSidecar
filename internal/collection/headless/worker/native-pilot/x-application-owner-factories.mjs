// A single runtime preserves admission across manual/daily creation and close.
import {createNativeOwnerRuntime} from './x-native-owner-runtime.mjs';
export {manualSeedRPCCode} from './x-native-owner-runtime.mjs';
const runtime=createNativeOwnerRuntime();
export const {createApplicationManualOwner,createApplicationDailyOwner}=runtime;

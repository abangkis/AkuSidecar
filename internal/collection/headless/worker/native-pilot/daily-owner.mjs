import {serveDailyOwnerPipe} from './daily-owner-protocol.mjs';
import {loadPackagedAssets} from './packaged-assets.mjs';
import {createApplicationDailyOwner} from '../../../../../scripts/auth-session-fixture/x-native-metadata-probe-driver.mjs';
export function runDailyOwner(){
  return serveDailyOwnerPipe({input:process.stdin,output:process.stdout,
    createOwner:(scope,emitProgress)=>createApplicationDailyOwner({...scope,loadAssets:loadPackagedAssets,emitProgress}),
    onClosed:()=>{process.stdin.destroy();process.stdout.end();process.exitCode=0;}});
}

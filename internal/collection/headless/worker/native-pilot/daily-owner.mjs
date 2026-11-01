import {serveDailyOwnerPipe} from './daily-owner-protocol.mjs';
import {loadPackagedAssets} from './packaged-assets.mjs';
import {createApplicationDailyOwner} from './x-application-owner-factories.mjs';
export function runDailyOwner(){
  return serveDailyOwnerPipe({input:process.stdin,output:process.stdout,
    createOwner:(scope,emitProgress)=>createApplicationDailyOwner({...scope,loadAssets:loadPackagedAssets,emitProgress}),
    onClosed:()=>{process.stdin.destroy();process.stdout.end();process.exitCode=0;}});
}

import {serveOwnerPipe} from './owner-protocol.mjs';
import {loadPackagedAssets} from './packaged-assets.mjs';
import {createApplicationManualOwner} from '../../../../../scripts/auth-session-fixture/x-native-metadata-probe-driver.mjs';

// This entry is launched only by the owned Go factory with sanitized env and
// pinned executable bytes. Handshake establishes transport, never qualification.
export function runApplicationOwner(){
  return serveOwnerPipe({input:process.stdin,output:process.stdout,
    createOwner:(scope,emitProgress)=>createApplicationManualOwner({...scope,loadAssets:loadPackagedAssets,emitProgress}),
    onClosed:()=>{process.stdin.destroy();process.stdout.end();process.exitCode=0;},
  });
}

import {verifyPackage} from './x-development-pilot-package.mjs';
if(process.argv.length!==3)throw new Error('usage: node scripts/verify-x-development-pilot.mjs <activation-package.json>');
process.stdout.write(JSON.stringify(await verifyPackage(process.argv[2]))+'\n');

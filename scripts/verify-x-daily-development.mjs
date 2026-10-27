import {verifyDailyPackage} from './x-daily-development-package.mjs';
if(process.argv.length!==3)throw new Error('usage: node scripts/verify-x-daily-development.mjs <daily-package.json>');
process.stdout.write(JSON.stringify(await verifyDailyPackage(process.argv[2]))+'\n');

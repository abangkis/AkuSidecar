// Source-only capability staging. No browser, profile or service is accessed.
import {buildMainBrowserTransportPackage,buildNativeDailyOwnerPackage} from './build-x-native-owner.mjs';
import {join,resolve} from 'node:path';
const [root,...extra]=process.argv.slice(2);
if(!root||extra.length)throw new Error('usage: node scripts/stage-dev-x-assets.mjs <project-local-stage-root>');
const directory=resolve(root);
const main=await buildMainBrowserTransportPackage(join(directory,'main-browser-transport'));
const daily=await buildNativeDailyOwnerPackage(join(directory,'x-daily-owner'));
process.stdout.write(JSON.stringify({main,daily})+'\n');

import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {execFileSync} from 'node:child_process';
import {createHash} from 'node:crypto';

// Read-only inventory of tracked source. Build artifacts, profiles, databases,
// dependencies and untracked experiments are intentionally never traversed.
const workspace=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'../..');
const repositories=['AkuSidecar','AkuBridge','AkuBrowser'];
const groups=new Map(),reports=[];
for(const name of repositories){
 const root=path.join(workspace,name);
 const files=execFileSync('git',['ls-files','-z'],{cwd:root,encoding:'utf8'}).split('\0').filter(Boolean);
 const sizes=[];
 for(const file of files){
  if(!/\.(?:go|mjs|js|html|css|ps1)$/u.test(file)||/^(?:vendor|runtime|build|dist|node_modules)\//u.test(file))continue;
  const absolute=path.join(root,file);if(!fs.existsSync(absolute))continue;
  const bytes=fs.readFileSync(absolute),lines=bytes.toString('utf8').split('\n').length;
  sizes.push({file,lines,bytes:bytes.length});
  if(bytes.length<100)continue;
  const hash=createHash('sha256').update(bytes).digest('hex');
  const group=groups.get(hash)??[];group.push(`${name}/${file}`);groups.set(hash,group);
 }
 reports.push({repository:name,sourceFiles:sizes.length,largest:sizes.sort((a,b)=>b.lines-a.lines).slice(0,10)});
}
const app=fs.readFileSync(path.join(workspace,'AkuSidecar/internal/httpapi/web/app.js'),'utf8');
const unreferencedPageFunctions=[...app.matchAll(/^function ([A-Za-z_$][\w$]*)\(/gmu)]
 .map(match=>match[1]).filter(name=>[...app.matchAll(new RegExp(`\\b${name}\\b`,'gu'))].length===1);
process.stdout.write(JSON.stringify({repositories:reports,identicalFiles:[...groups.values()].filter(group=>group.length>1),unreferencedPageFunctions},null,2)+'\n');

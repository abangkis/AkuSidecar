import test from 'node:test';
import assert from 'node:assert/strict';
import net from 'node:net';
import {spawn} from 'node:child_process';
import {createHash,randomBytes} from 'node:crypto';
import {mkdir,mkdtemp,readFile,copyFile,rm,realpath} from 'node:fs/promises';
import {dirname,join,resolve} from 'node:path';
import {fileURLToPath} from 'node:url';
import {buildNativeOwnerPackage,buildNativeDailyOwnerPackage,buildMainBrowserTransportPackage} from './build-x-native-owner.mjs';
import {OWNER_SCHEMA} from '../internal/collection/headless/worker/native-pilot/owner-protocol.mjs';
import {DAILY_OWNER_SCHEMA} from '../internal/collection/headless/worker/native-pilot/daily-owner-protocol.mjs';
import {MAIN_TRANSPORT_SCHEMA} from '../internal/collection/headless/worker/native-pilot/main-browser-transport-protocol.mjs';

const project=resolve(dirname(fileURLToPath(import.meta.url)),'..');
const catalog='internal/collection/headless/worker/navigation-catalog.json';
const hash=bytes=>createHash('sha256').update(bytes).digest('hex');

async function until(check,child,stderr,label){
  const end=Date.now()+5000;
  while(!check()){
    assert.equal(child.exitCode,null,`${label}: child exited before readiness: ${stderr()}`);
    assert.ok(Date.now()<end,`${label}: readiness timeout: ${stderr()}`);
    await new Promise(accept=>setTimeout(accept,10));
  }
}

for(const [kind,build,schema]of [
  ['manual',buildNativeOwnerPackage,OWNER_SCHEMA],
  ['daily',buildNativeDailyOwnerPackage,DAILY_OWNER_SCHEMA],
  ['main',buildMainBrowserTransportPackage,MAIN_TRANSPORT_SCHEMA],
]){
  test(`${kind} sealed package imports its hashed JSON catalog and bootstraps after relocation without browser work`,{timeout:20000},async t=>{
    const parent=join(project,'build');await mkdir(parent,{recursive:true});
    const root=await mkdtemp(join(parent,'native-json-bootstrap-'));
    t.after(async()=>{assert.equal(dirname(root),parent);await rm(root,{recursive:true,force:true});});
    const first=await build(join(root,'first')),second=await build(join(root,'second'));
    assert.equal(first.sha256,second.sha256,'the embedded package remains reproducible');
    const manifest=JSON.parse(await readFile(first.manifest,'utf8'));
    assert.equal(manifest.modules.find(row=>row.path===catalog)?.sha256,
      hash(await readFile(join(project,catalog))),'manifest pins the original JSON bytes');
    assert.equal(hash(await readFile(first.driver)),first.sha256);
    const moved=join(root,'relocated.cjs');await copyFile(first.driver,moved);
    const reader=join(root,'reader'),collector=join(root,'collector');
    await mkdir(reader);await mkdir(collector);
    const nonce=randomBytes(24).toString('hex');
    let control,server;
    let received='',stderr='',cdpOutput='';
    const argumentsForChild=[moved];
    if(kind==='main'){
      const pipe=process.platform==='win32'?`\\\\.\\pipe\\AkuBrowser.main-transport.${nonce}`:join(root,'control.sock');
      // The production main entry accepts its fixed Windows pipe namespace.
      // Non-Windows hosts exercise manual/daily JSON imports in the other cases.
      if(process.platform!=='win32'){t.skip('main transport entry requires Windows named pipes');return;}
      server=net.createServer(socket=>{control=socket;socket.setEncoding('utf8');socket.on('data',chunk=>{received+=chunk;});});
      server.on('error',()=>{});
      await new Promise((accept,reject)=>{server.once('error',reject);server.listen(pipe,accept);});
      argumentsForChild.push(pipe,nonce);
    }
    const child=spawn(await realpath(process.execPath),argumentsForChild,
      {cwd:root,stdio:['pipe','pipe','pipe'],env:{SystemRoot:process.env.SystemRoot??''}});
    const closed=new Promise(accept=>child.once('close',accept));
    child.stdin.on('error',()=>{});
    child.stderr.on('data',chunk=>{stderr+=chunk;});
    child.stdout.on('data',chunk=>{if(kind==='main')cdpOutput+=chunk;else received+=chunk;});
    try{
      if(kind==='main'){
        await until(()=>control,child,()=>stderr,'control pipe connection');
        control.write(JSON.stringify({op:'main_transport_bootstrap',schema,nonce,transport:'inherited-stdio'})+'\n');
      }else{
        child.stdin.write(JSON.stringify({op:kind==='manual'?'native_owner_bootstrap':'daily_owner_bootstrap',
          schema,nonce,chromePath:await realpath(process.execPath),readerProfile:reader,
          ...(kind==='manual'?{collectorProfile:collector}:{})})+'\n');
      }
      await until(()=>received.includes('\n'),child,()=>stderr,'bootstrap reply');
      assert.deepEqual(JSON.parse(received.trim()),{ready:true,schema,nonce});
      assert.equal(cdpOutput,'','bootstrap cannot send browser/CDP commands');
      // No initialize/seed/source/capture request is sent, so only synthetic
      // directories are inspected and no browser process or session is opened.
      if(kind==='main')control.end();else child.stdin.end();
      const exitCode=await closed;
      assert.equal(exitCode,0,stderr);
      assert.equal(cdpOutput,'');
      assert.equal(received.trim().split('\n').length,1,'only the bootstrap receipt is emitted');
      t.diagnostic(`${kind} verified driver SHA-256: ${first.sha256}`);
    }finally{
      control?.destroy();
      if(child.exitCode===null){child.kill();await closed;}
      if(server)await new Promise(accept=>server.close(accept));
    }
  });
}

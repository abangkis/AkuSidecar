import test from 'node:test';
import assert from 'node:assert/strict';
import {PassThrough} from 'node:stream';
import {spawn} from 'node:child_process';
import {mkdir,mkdtemp,readFile,copyFile,rm,realpath} from 'node:fs/promises';
import {resolve,dirname,join,relative,sep} from 'node:path';
import {fileURLToPath} from 'node:url';
import {createHash} from 'node:crypto';
import {serveOwnerPipe,ownerBootstrap,ownerOperation,ownerErrorReply,OWNER_SCHEMA} from '../internal/collection/headless/worker/native-pilot/owner-protocol.mjs';
import {manualCaptureFailure} from './auth-session-fixture/x-native-manual-capture.mjs';
import {buildNativeOwnerPackage} from './build-x-native-owner.mjs';

const project=resolve(dirname(fileURLToPath(import.meta.url)),'..');
const boot={op:'native_owner_bootstrap',schema:OWNER_SCHEMA,nonce:'a'.repeat(48),chromePath:process.execPath,readerProfile:'reader',collectorProfile:'collector'};
const turn=()=>new Promise(accept=>setImmediate(accept));
const pins={ownerId:'owner',profileId:'profile',contextId:'context',targetId:'target',frameId:'frame',documentId:'document'};
function harness(overrides={},options={}){
  const input=new PassThrough(),output=new PassThrough(),records=[],calls=[];let closed=0;
  let pending='';output.on('data',chunk=>{pending+=chunk.toString();let end;while((end=pending.indexOf('\n'))>=0){records.push(JSON.parse(pending.slice(0,end)));pending=pending.slice(end+1);}});
  const owner={initialize:async()=>{calls.push('initialize');return{owner:1,readerPID:101,collectorPID:102};},seed:async()=>{calls.push('seed');return{seeded:true};},execute:async message=>{calls.push(message.op);return{closed:true};},close:async()=>{closed++;},...overrides};
  const port=serveOwnerPipe({input,output,createOwner:async()=>owner,...options});
  const send=async value=>{input.write(typeof value==='string'?value:JSON.stringify(value)+'\n');await turn();};
  return{send,records,calls,input,port,owner,get closed(){return closed;}};
}
async function ready(h){await h.send(boot);assert.deepEqual(h.records.shift(),{ready:true,schema:OWNER_SCHEMA,nonce:boot.nonce});}

if(process.argv.includes('--pipe-fixture')){
  serveOwnerPipe({input:process.stdin,output:process.stdout,
    createOwner:async()=>({initialize:async()=>({owner:1,readerPID:101,collectorPID:102}),seed:async()=>({seeded:true}),execute:async message=>message.op==='manual_prepare_post'?{handle:'c'.repeat(48),targetId:'synthetic-post',windowId:2,expiresMS:Date.now()+30000}:{closed:true},close:async()=>{}}),
    onClosed:()=>{process.stdin.destroy();process.stdout.end();},
  });
}else{
  test('closed bootstrap and operation shapes reject QA/CDP/script/URL authority',()=>{
    ownerBootstrap(boot);
    for(const value of [{...boot,transferAllowed:true},{...boot,nonce:'bad'},{...boot,schema:'fixture_bootstrap'}])assert.throws(()=>ownerBootstrap(value));
    ownerOperation({id:1,op:'native_owner_initialize'});
    ownerOperation({id:2,op:'native_owner_seed',owner:1});
    ownerOperation({id:3,op:'manual_open_reader',owner:1});
    for(const op of ['launch','release','qualify_source','qa_profile_scope','Page.reload','Runtime.evaluate','manual_seed_bootstrap'])assert.throws(()=>ownerOperation({id:4,op,owner:1}));
    assert.throws(()=>ownerOperation({id:5,op:'manual_open_reader',owner:1,url:'https://x.com/other'}));
    assert.throws(()=>ownerOperation({id:6,op:'manual_acquire',owner:1,role:'reader',handle:'h',request:{challenge:'challenge',role:'reader',...pins},startupBudgetMS:0}));
  });
  test('initialize and explicit seed are one use; manual work cannot precede seed',async()=>{
    const h=harness();await ready(h);
    await h.send({id:1,op:'manual_inventory',owner:1,role:'reader'});assert.equal(h.records.at(-1).ok,false);assert.deepEqual(h.calls,[]);
    await h.send({id:2,op:'native_owner_seed',owner:1});assert.equal(h.records.at(-1).ok,false);
    await h.send({id:3,op:'native_owner_initialize'});assert.deepEqual(h.records.at(-1),{id:3,ok:true,result:{owner:1,readerPID:101,collectorPID:102}});
    await h.send({id:4,op:'native_owner_initialize'});assert.equal(h.records.at(-1).ok,false);
    await h.send({id:5,op:'native_owner_seed',owner:1});assert.equal(h.records.at(-1).ok,true);
    await h.send({id:6,op:'native_owner_seed',owner:1});assert.equal(h.records.at(-1).ok,false);
    await h.send({id:7,op:'manual_inventory',owner:1,role:'reader'});assert.equal(h.records.at(-1).ok,true);
    assert.deepEqual(h.calls,['initialize','seed','manual_inventory']);h.port.close();await turn();assert.equal(h.closed,1);
  });
  test('failed seed is not retried and raw exceptions never enter diagnostics',async()=>{
    let attempts=0;const h=harness({seed:async()=>{attempts++;throw new Error('cookie auth_token=SECRET path=C:/private');}});await ready(h);
    await h.send({id:1,op:'native_owner_initialize'});
    await h.send({id:2,op:'native_owner_seed',owner:1});
    await h.send({id:3,op:'native_owner_seed',owner:1});
    assert.equal(attempts,1);assert.equal(h.records[1].code,'manual_seed_preconditions_unavailable_unknown');assert.doesNotMatch(JSON.stringify(h.records),/SECRET|private|auth_token/u);
    h.port.close();
  });
  test('framing fails closed on duplicate IDs, duplicate keys, extra fields, invalid UTF8 and line limits',async()=>{
    for(const malformed of ['{"id":1,"id":2,"op":"native_owner_initialize"}\n',JSON.stringify({id:1,op:'native_owner_initialize',script:'arbitrary'})+'\n',Buffer.from([0xff,10]),'x'.repeat(65537)]){
      const h=harness();await ready(h);h.input.write(malformed);await turn();assert.equal(h.closed,1);assert.deepEqual(h.calls,[]);
    }
    const h=harness();await ready(h);await h.send({id:1,op:'native_owner_initialize'});await h.send({id:1,op:'native_owner_initialize'});assert.equal(h.closed,1);
  });
  test('initialization deadline retires the pipe and rejects late work',async()=>{
    const h=harness({initialize:()=>new Promise(()=>{})},{initializeBudgetMS:5});await ready(h);
    await h.send({id:1,op:'native_owner_initialize'});await new Promise(accept=>setTimeout(accept,20));
    assert.equal(h.closed,1);await h.send({id:2,op:'native_owner_seed',owner:1});assert.deepEqual(h.calls,[]);
  });
  test('manual requests remain concurrent for rendezvous, cancel and branded capture diagnostics',async()=>{
    let release,wire;const h=harness({execute:message=>{
      if(message.op==='manual_prepare')return new Promise(accept=>{release=accept;});
      wire=Buffer.from('{"closed":true}');return wire;
    }});await ready(h);await h.send({id:1,op:'native_owner_initialize'});await h.send({id:2,op:'native_owner_seed',owner:1});
    await h.send({id:3,op:'manual_prepare',owner:1,role:'reader'});
    await h.send({id:4,op:'manual_capture_cancel',owner:1,role:'collector',handle:'handle'});
    assert.equal(h.records.at(-1).id,4);assert(wire.every(value=>value===0));
    release({handle:'handle',pins});await turn();assert.equal(h.records.at(-1).id,3);h.port.close();
  });
  test('fixed reader open request and closed capture diagnostic retain their exact wire shape',async()=>{
    const h=harness({execute:async message=>{assert.equal(message.op,'manual_open_reader');return{opened:true,windowCount:2,bootstrapCount:1};}});
    await ready(h);await h.send({id:1,op:'native_owner_initialize'});await h.send({id:2,op:'native_owner_seed',owner:1});await h.send({id:3,op:'manual_open_reader',owner:1});
    assert.deepEqual(h.records.at(-1),{id:3,ok:true,result:{opened:true,windowCount:2,bootstrapCount:1}});h.port.close();
    const error=manualCaptureFailure(new Error('SECRET'),'worker','unavailable');
    assert.deepEqual(ownerErrorReply({id:4,op:'manual_capture'},error),{id:4,ok:false,code:'manual_capture_rejected',diagnostic:{schema:'aku.capture-diagnostic.v1',stage:'worker',reason:'unavailable',cleanup:'unknown'}});
  });
  test('actual Node pipes honor handshake and exact request shapes without browser access',{timeout:10000},async()=>{
    const child=spawn(process.execPath,[fileURLToPath(import.meta.url),'--pipe-fixture'],{stdio:['pipe','pipe','pipe'],env:{SystemRoot:process.env.SystemRoot??''}});
    const received=[];let raw='';child.stdout.on('data',chunk=>{raw+=chunk;let end;while((end=raw.indexOf('\n'))>=0){received.push(JSON.parse(raw.slice(0,end)));raw=raw.slice(end+1);}});
    const exited=new Promise(accept=>child.once('exit',accept));
    const until=async count=>{for(let i=0;i<100&&received.length<count;i++)await new Promise(accept=>setTimeout(accept,20));assert.equal(received.length,count);};
    try{
      child.stdin.write(JSON.stringify(boot)+'\n');await until(1);assert.equal(received[0].nonce,boot.nonce);
      child.stdin.write('{"id":1,"op":"native_owner_initialize"}\n');await until(2);assert.equal(received[1].ok,true);
      child.stdin.write('{"id":2,"op":"native_owner_seed","owner":1}\n');await until(3);assert.equal(received[2].ok,true);
      child.stdin.write(JSON.stringify({id:3,op:'manual_prepare_post',owner:1,request:{requestId:'broker_'+'a'.repeat(32),source:'x',url:'https://x.com/example/status/123'},marker:'http://127.0.0.1:12345/split-reader-intent?id=split_'+'b'.repeat(32)})+'\n');
      await until(4);assert.equal(received[3].result.targetId,'synthetic-post');
      child.stdin.write('{"id":4,"op":"Runtime.evaluate","expression":"SECRET"}\n');await exited;assert.equal(received.length,4);
    }finally{if(child.exitCode===null)child.kill();}
  });
  test('sealed package is reproducible, self contained, bootstrap only with private seed implementation',{timeout:20000},async()=>{
    const tempRoot=join(project,'.tmp');await mkdir(tempRoot,{recursive:true});const scratch=await mkdtemp(join(tempRoot,'native-owner-offline-'));
    assert(scratch.startsWith(tempRoot+sep));
    try{
      const first=await buildNativeOwnerPackage(join(scratch,'first')),second=await buildNativeOwnerPackage(join(scratch,'second'));
      assert.equal(first.sha256,second.sha256);assert.equal(first.capabilities.sessionSeed,true);assert(first.moduleCount>40);assert(first.assetCount>20);
      const bytes=await readFile(first.driver);assert.equal(createHash('sha256').update(bytes).digest('hex'),first.sha256);
      const moved=join(scratch,'moved.cjs');await copyFile(first.driver,moved);
      const reader=join(scratch,'reader'),collector=join(scratch,'collector');await mkdir(reader);await mkdir(collector);
      const child=spawn(process.execPath,[moved],{stdio:['pipe','pipe','pipe'],env:{SystemRoot:process.env.SystemRoot??''}});
      let text='',stderr='';child.stdout.on('data',chunk=>{text+=chunk;});child.stderr.on('data',chunk=>{stderr+=chunk;});
      const exited=new Promise(accept=>child.once('exit',accept));
      try{
        child.stdin.write(JSON.stringify({...boot,chromePath:await realpath(process.execPath),readerProfile:reader,collectorProfile:collector})+'\n');
        for(let i=0;i<150&&!text.includes('\n')&&child.exitCode===null;i++)await new Promise(accept=>setTimeout(accept,20));
        assert(text.trim(),stderr||'packaged bootstrap produced no reply');
        assert.deepEqual(JSON.parse(text.trim()),{ready:true,schema:OWNER_SCHEMA,nonce:boot.nonce},stderr);
        // No initialization is sent: neither Chrome nor a retained profile is
        // accessed. A seed request before initialize is rejected at framing.
        child.stdin.write('{"id":1,"op":"native_owner_seed","owner":1}\n');
        for(let i=0;i<100&&text.trim().split('\n').length<2;i++)await new Promise(accept=>setTimeout(accept,20));
        assert.equal(JSON.parse(text.trim().split('\n')[1]).ok,false);
        child.stdin.end();await exited;
      }finally{if(child.exitCode===null)child.kill();}
    }finally{
      const rel=relative(tempRoot,scratch);assert(rel&&!rel.startsWith('..')&&!resolve(scratch).includes('..'));
      await rm(scratch,{recursive:true,force:true});
    }
  });
}

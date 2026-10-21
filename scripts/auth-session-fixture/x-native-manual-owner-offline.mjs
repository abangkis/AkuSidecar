// Accountless private-stdio companion for the exact X owner-port protocol.
// It has no browser, profile, network, cookie or native-authorization capability.
import {resolve} from 'node:path';
import {pathToFileURL} from 'node:url';
import {createXManualOwnerPort} from './x-native-manual-owner-port.mjs';
import {offlineStartupOwner,runOfflineStartupRecipe} from './x-manual-startup-offline-fixture.mjs';
import {createManualStartupOwner} from './x-manual-startup-owner.mjs';
import {parseStrictJSON} from './x-evidence-ingress.mjs';

export function offlineXManualOwnerPort({afterRelease,changeSnapshot=()=>{},onRetire=()=>{}}={}){
  const alive={reader:true,collector:true};
  return createXManualOwnerPort({enabled:true,owner:1,
    createOwners:()=>Object.fromEntries(['reader','collector'].map(role=>[role,createManualStartupOwner({
      run:sink=>runOfflineStartupRecipe({role},sink,{beforeIdentityRead:async(pins,ports)=>{
        await sink.beforeIdentityRead(pins);await afterRelease?.(role,ports);
      }}),
    })])),
    snapshot:async(role)=>{
      const pins=offlineStartupOwner(role);
      const value={...pins,sessionId:'offline-session-'+role,headless:role==='collector',
        windowCount:role==='reader'?1:0,bootstrapCount:role==='reader'?1:0,alive:alive[role],lifecycleReady:true};
      changeSnapshot(role,value);return value;
    },
    retire:async(role,preserve)=>{if(!preserve)alive[role]=false;await onRetire(role,preserve);},
  });
}

async function run(){
  if(process.env.AKU_X_NATIVE_MANUAL_OWNER_OFFLINE!=='1')throw new Error('offline_owner_disabled');
  const port=offlineXManualOwnerPort();
  let pending=Buffer.alloc(0),lines=0;const tasks=new Set();
  const execute=async message=>{
    let result;
    try{
      result=await port.execute(message);
      const value=Buffer.isBuffer(result)?parseStrictJSON(result.toString('utf8'),{maxJSONDepth:4,maxJSONTokens:96}):result;
      process.stdout.write(JSON.stringify({id:message.id,ok:true,result:value})+'\n');
    }catch{process.stdout.write(JSON.stringify({id:message.id,ok:false,code:'manual_owner_unavailable'})+'\n');}
    finally{if(Buffer.isBuffer(result))result.fill(0);}
  };
  try{
    for await(const chunk of process.stdin){
      if(pending.length+chunk.length>32768)throw new Error('offline_owner_closed');
      const previous=pending;pending=Buffer.concat([previous,chunk]);previous.fill(0);
      for(let end;(end=pending.indexOf(10))>=0;){
        const line=Buffer.from(pending.subarray(0,end)),previous=pending;
        pending=Buffer.from(previous.subarray(end+1));previous.fill(0);
        try{
          if(line.length>16384||++lines>128||tasks.size>=8)throw new Error('offline_owner_closed');
          const message=parseStrictJSON(line.toString('utf8'),{maxJSONDepth:4,maxJSONTokens:96});
          const task=execute(message);tasks.add(task);void task.finally(()=>tasks.delete(task));
        }finally{line.fill(0);}
      }
    }
    if(pending.length)throw new Error('offline_owner_closed');
  }finally{
    pending.fill(0);
    await Promise.allSettled(['reader','collector'].map((role,index)=>port.execute({id:1000+index,op:'manual_abort',owner:1,role,handle:null,preserveReader:role==='reader'})));
    await Promise.allSettled(tasks);
  }
}
if(process.argv[1]&&pathToFileURL(resolve(process.argv[1])).href===import.meta.url){
  void run().catch(()=>{process.exitCode=2;});
}

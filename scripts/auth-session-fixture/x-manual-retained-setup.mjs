// One-shot private QA setup, not account/login authorization. Cookie projection
// and write validation stay at the existing bounded CDP dispatch boundaries.
import {performance} from 'node:perf_hooks';
const fail=()=>new Error('xnp_manual_setup_unavailable');
const keys=['readerPID','readerProfile','collectorPID','collectorProfile'];
function checkedOwnership(value){
  if(!value||Object.keys(value).sort().join(',')!==[...keys].sort().join(',')||
    !['readerPID','collectorPID'].every(k=>Number.isSafeInteger(value[k])&&value[k]>0)||value.readerPID===value.collectorPID||
    !['readerProfile','collectorProfile'].every(k=>typeof value[k]==='string'&&value[k].length>0&&Buffer.byteLength(value[k])<=512)||value.readerProfile===value.collectorProfile)throw fail();
  return Object.freeze({...value});
}
export function createRetainedManualSetup({enabled=false,transferAllowed=false,ownership,readEnrollment,readAllowedCookies,writeAllowedCookies,now=()=>performance.now()}={}){
  if(enabled!==true||transferAllowed!==true||![ownership,readEnrollment,readAllowedCookies,writeAllowedCookies,now].every(v=>typeof v==='function'))throw fail();
  let used=false;
  return async()=>{
    if(used)throw fail();used=true;
    const start=now();let last=start,selected=null;
    const tick=()=>{const t=now();if(!Number.isFinite(start)||!Number.isFinite(t)||t<last||t-start>=60000)throw fail();last=t;};
    try{
      tick();const actual=checkedOwnership(await ownership());tick();
      // This candidate enrolls the intended account only. Fresh startup evidence
      // from BOTH roles must subsequently match it before controller admission.
      const accountId=await readEnrollment();tick();
      if(typeof accountId!=='string'||!/^[1-9][0-9]{0,19}$/.test(accountId))throw fail();
      selected=await readAllowedCookies();tick();
      if(!Array.isArray(selected)||selected.length!==2)throw fail();
      if(await writeAllowedCookies(selected)!==true)throw fail();tick();
      const after=checkedOwnership(await ownership());tick();
      if(keys.some(k=>actual[k]!==after[k]))throw fail();
      return Object.freeze({accountId,...actual,seeded:true,homeReady:true});
    }catch{throw fail();}
    finally{selected=null;}
  };
}

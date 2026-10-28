const fail=()=>new Error('batch_bootstrap_cleanup_uncertain');
const valid=id=>typeof id==='string'&&id.length>0&&id.length<=512;

// Holds only the target created for this batch; never adopts a user target.
export function createBatchBootstrap({call,reserveTarget,releaseTarget}){
  let target=null,pending=false;
  const inventory=async()=>{
    const result=await call('Target.getTargets',{});
    if(!Array.isArray(result?.targetInfos)||result.targetInfos.length>128||result.targetInfos.some(row=>!valid(row?.targetId))||
      new Set(result.targetInfos.map(row=>row.targetId)).size!==result.targetInfos.length)throw fail();
    return result.targetInfos;
  };
  return Object.freeze({
    async begin(){
      if(target||pending)throw fail();
      pending=true;
      let before;
      try{before=new Set((await inventory()).map(row=>row.targetId));}
      catch(error){pending=false;throw error;}
      const created=await call('Target.createTarget',{url:'https://x.com/home',background:true});
      if(!valid(created?.targetId)||before.has(created.targetId))throw fail();
      target=created.targetId;reserveTarget?.(target);pending=false;
      return target;
    },
    async close(){
      if(pending)throw fail();
      if(!target)return;
      if((await inventory()).some(row=>row.targetId===target)){
        const result=await call('Target.closeTarget',{targetId:target});
        if(result?.success!==true)throw fail();
      }
      if((await inventory()).some(row=>row.targetId===target))throw fail();
      releaseTarget?.(target);target=null;
    },
  });
}

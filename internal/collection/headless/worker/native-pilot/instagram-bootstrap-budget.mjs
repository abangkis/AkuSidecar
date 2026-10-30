// Trusted package policy, never supplied by the page or diagnostic request.
const MiB=1024*1024;
export const INSTAGRAM_BOOTSTRAP_BUDGET=Object.freeze({
 version:1,deadlineMS:1000,evaluationTimeoutMS:2000,frameReadMS:1000,depth:64,
 scripts:Object.freeze({initial:256,max:512}),
 scanBytes:Object.freeze({initial:8*MiB,max:16*MiB}),
 scriptBytes:Object.freeze({initial:4*MiB,max:8*MiB}),
 candidateBytes:Object.freeze({initial:2*MiB,max:4*MiB}),
 parseBytes:Object.freeze({initial:4*MiB,max:8*MiB}),
 containers:Object.freeze({initial:64000,max:128000}),
 entries:Object.freeze({initial:256000,max:512000}),
});
export function validateInstagramBootstrapBudget(value){
 const hard=INSTAGRAM_BOOTSTRAP_BUDGET;
 if(!value||Object.keys(value).length!==Object.keys(hard).length)throw Error('bootstrap_budget_rejected');
 for(const key of ['version','deadlineMS','evaluationTimeoutMS','frameReadMS','depth']){
  if(!Number.isSafeInteger(value[key])||value[key]<1||value[key]>hard[key])throw Error('bootstrap_budget_rejected');
 }
 if(value.version!==1||value.deadlineMS>=value.evaluationTimeoutMS)throw Error('bootstrap_budget_rejected');
 const copy={};for(const key of Object.keys(hard)){
  if(typeof hard[key]==='number'){copy[key]=value[key];continue;}
  const pair=value[key];if(!pair||Object.keys(pair).length!==2||!Number.isSafeInteger(pair.initial)||!Number.isSafeInteger(pair.max)||pair.initial<1||pair.initial>pair.max||pair.max>hard[key].max)throw Error('bootstrap_budget_rejected');
  copy[key]=Object.freeze({initial:pair.initial,max:pair.max});
 }
 return Object.freeze(copy);
}

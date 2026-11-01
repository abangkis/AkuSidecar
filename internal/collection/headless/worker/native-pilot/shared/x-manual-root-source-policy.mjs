// One policy for observed source eligibility and the owned startup recipe.
// Service-worker involvement is a source class, not proof of stale content.
export const ROOT_SOURCE_POLICY=Object.freeze({version:'manual-root-source.v1',
  controls:Object.freeze({cacheDisabled:true,bypassServiceWorker:true}),
  allowedCacheClasses:Object.freeze(['absent','false'])});
export function rootSourceVerdict(facts){
  if(!facts||!facts.structurallyValid)return Object.freeze({status:'excluded',condition:'root_structure'});
  if(facts.cacheEvent)return Object.freeze({status:'excluded',condition:'root_served_from_cache'});
  if(facts.responseObserved){
    for(const [field,condition]of [['guardDiskCache','root_response_disk_cache'],['guardServiceWorker','root_response_service_worker'],['guardPrefetchCache','root_response_prefetch_cache']]){
      if(!ROOT_SOURCE_POLICY.allowedCacheClasses.includes(facts.cache?.[field]))return Object.freeze({status:'excluded',condition});
    }
  }
  return Object.freeze({status:facts.complete?'accepted_under_policy':'pending',condition:'unknown'});
}

// Structural telemetry only; none of these fields supplies account or capture authority.
const own=(v,k)=>v&&typeof v==='object'?Object.getOwnPropertyDescriptor(v,k)?.value:undefined;
const count=v=>Number.isSafeInteger(v)&&v>=0&&v<=1000000?v:null;
const flag=v=>typeof v==='boolean'?v:null;
const rejectionKeys=['missing_identity','ambiguous_identity','missing_author','empty_evidence'];
export function projectCaptureWorkerDiagnostic(error,{hydrationMS,elapsedMS}) {
  const code=own(error,'code'),diagnostics=own(error,'diagnostics');
  if(!['empty_unverified','invalid_observation'].includes(code)||!diagnostics)return undefined;
  const sampleCount=count(own(diagnostics,'sampleCount')),raw=own(diagnostics,'samples');
  if(sampleCount===null||!Array.isArray(raw))return undefined;
  const samples=[];
  for(let i=Math.max(0,raw.length-2);i<raw.length;i++){
    const sample=own(raw,String(i));
    if(!sample||typeof sample!=='object')continue;
    const rejections=own(sample,'rejectionReasons');
    samples.push(Object.freeze({candidateCount:count(own(sample,'candidateCount')),rejected:count(own(sample,'rejected')),
      documentReady:flag(own(sample,'documentReady')),authenticatedUiObserved:flag(own(sample,'authenticatedUiObserved')),
      loginRequired:flag(own(sample,'loginRequired')),challengeDetected:flag(own(sample,'challengeDetected')),
      rejections:Object.freeze(Object.fromEntries(rejectionKeys.map(k=>[k,count(own(rejections,k))])))}));
  }
  return Object.freeze({phase:code==='invalid_observation'?'observation_invalid':sampleCount===0?'snapshot_unavailable':'no_posts',
    hydrationMS:count(hydrationMS),elapsedMS:count(elapsedMS),sampleCount,samples:Object.freeze(samples)});
}

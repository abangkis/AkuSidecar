import {batchBootstrapDiagnostic} from './x-batch-bootstrap.mjs';

// Closed, value-free outcomes. Independent resources are always attempted.
export async function cleanupDailyBatch({readerControls='not_needed',collectorControls='not_needed',collector,enrollment,attachments,bootstrap}) {
  const outcome=value=>['verified','uncertain','not_needed'].includes(value)?value:'uncertain';
  const cleanup={readerControls:outcome(readerControls),collectorControls:outcome(collectorControls)};
  let bootstrapDiagnostic;
  for(const [stage,release]of Object.entries({collector,enrollment,attachments,bootstrap})) {
    cleanup[stage]='not_needed';
    if(release)try{await release();cleanup[stage]='verified';}catch(error){cleanup[stage]='uncertain';if(stage==='bootstrap')bootstrapDiagnostic=batchBootstrapDiagnostic(error);}
  }
  return Object.freeze({closed:!Object.values(cleanup).includes('uncertain'),cleanup:Object.freeze(cleanup),...(bootstrapDiagnostic?{bootstrapDiagnostic}:{})});
}

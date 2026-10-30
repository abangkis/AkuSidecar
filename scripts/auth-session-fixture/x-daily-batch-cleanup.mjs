import {batchBootstrapDiagnostic} from './x-batch-bootstrap.mjs';
import {settleNativeCleanup} from '../../internal/collection/headless/worker/native-pilot/native-source-kernel.mjs';

// Closed, value-free outcomes. Independent resources are always attempted.
export async function cleanupDailyBatch({readerControls='not_needed',collectorControls='not_needed',collector,enrollment,attachments,bootstrap}) {
  const outcome=value=>['verified','uncertain','not_needed'].includes(value)?value:'uncertain';
  const cleanup={readerControls:outcome(readerControls),collectorControls:outcome(collectorControls)};
  let bootstrapDiagnostic;
  const results=await settleNativeCleanup(Object.entries({collector,enrollment,attachments,bootstrap}),
    (stage,error)=>{if(stage==='bootstrap')bootstrapDiagnostic=batchBootstrapDiagnostic(error);});
  for(const {stage,outcome}of results)cleanup[stage]=outcome;
  return Object.freeze({closed:!Object.values(cleanup).includes('uncertain'),cleanup:Object.freeze(cleanup),...(bootstrapDiagnostic?{bootstrapDiagnostic}:{})});
}

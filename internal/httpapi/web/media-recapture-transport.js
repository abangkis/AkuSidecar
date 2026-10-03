export function mediaRecaptureTransport(job) {
  const stamp=job?.payload?.captureCollector;
  if(stamp!==undefined){
    if(stamp?.version!==1||!['bridge','headless','browser_quiet_hidden'].includes(stamp.backend))throw new Error('Unsupported media recapture collector.');
    return stamp.backend==='bridge'?'bridge':'sidecar';
  }
  return job?.payload?.captureRuntime?.driver==='headless'?'sidecar':'bridge';
}

export async function waitForMediaRecapture(id, read, {now=Date.now,sleep=ms=>new Promise(r=>setTimeout(r,ms)),timeoutMs=70000}={}) {
  const deadline=now()+timeoutMs;
  while(now()<deadline){
    const {recapture}=await read(`/api/media-recaptures/${encodeURIComponent(id)}`,{
      signal:AbortSignal.timeout(Math.max(1,Math.min(5000,deadline-now()))),
    });
    if(recapture?.id!==id)throw new Error('Media recapture response did not match the request.');
    if(recapture.status==='completed')return recapture;
    if(recapture.status==='failed')throw new Error(recapture.error?.message||'Media recapture failed.');
    if(!['queued','claimed'].includes(recapture.status))throw new Error('Unknown media recapture status.');
    await sleep(Math.min(500,Math.max(0,deadline-now())));
  }
  throw new Error('Media recapture is still running. Check the timeline again shortly.');
}

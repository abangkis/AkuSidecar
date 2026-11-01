// Real CDP observation is asynchronous. These bounded waits keep synthetic
// startup work from completing entirely within a cross-process clock boundary.
async function awaitWallTransitions(transitions,wallNow=()=>Date.now(),wait=()=>new Promise(resolve=>setTimeout(resolve,1))){
  let last=wallNow();
  for(let i=0;i<32;i++){
    await wait();
    const current=wallNow();
    if(current<last)throw new Error('offline_clock_invalid');
    if(current>last&&--transitions===0)return;
    last=current;
  }
  throw new Error('offline_clock_stalled');
}

export function awaitOfflineWallTick(wallNow,wait){
  return awaitWallTransitions(1,wallNow,wait);
}

// Two transitions include a complete intervening wall interval; the first edge
// alone can be the tail of the current tick. This only schedules fixture work.
// It does not synchronize clock epochs or change any observation timestamp.
export function awaitOfflineWallInterval(wallNow,wait){
  return awaitWallTransitions(2,wallNow,wait);
}

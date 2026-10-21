// One protected collection may wait while serialized control/probe operations
// progress. Chrome/CDP implementations and ownership checks remain in the caller.
export function createConcurrentDriverDispatch(execute) {
  let controls = Promise.resolve(), collecting = false, queued = 0;
  return message => new Promise((resolve, reject) => {
    if (queued >= 64) { reject(new Error('fixture_concurrent_control_busy')); return; }
    queued++;
    controls = controls.then(async () => {
      queued--;
      if (message.op === 'collect') {
        if (collecting) { reject(new Error('fixture_concurrent_collect_busy')); return; }
        collecting = true;
        Promise.resolve().then(() => execute(message)).then(
          result => { collecting = false; resolve(result); },
          error => { collecting = false; reject(error); });
      } else {
        try { resolve(await execute(message)); } catch (error) { reject(error); }
      }
    });
  });
}

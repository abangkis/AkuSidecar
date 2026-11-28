import { runWorker } from '../../headless/worker/worker.mjs';

// Run the real worker protocol and diagnostic emitter without a live browser.
await runWorker({
  sourceAssetsImpl: async () => [],
  launchChromeImpl: async () => ({ close: async () => {} }),
  captureImpl: async (_browser, _assets, source, payload) => {
    if (!payload.burst) throw Object.assign(new Error('private fixture detail'), { code: 'login_required' });
    const event = JSON.stringify({operation:'headless_worker', phase:'fatal', level:'error',
      sequence:2, reason:'worker_error', message:'private fixture detail'});
    const noise = 'private text\n' + 'z'.repeat(8192) + event + '\n' + (event + '\n').repeat(2000);
    await new Promise((resolve, reject) => process.stderr.write(noise, err => err ? reject(err) : resolve()));
    return { source };
  },
});

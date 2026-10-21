// Keep one actual browser fetch open beyond the shared transport's per-CDP
// deadline. Each evaluation is short and returns primitive bounded state only;
// no shared Chrome timeout or authentication behavior is changed.
const start = `(() => {
  if (globalThis.__akuConcurrentCollect && !globalThis.__akuConcurrentCollect.finished) throw Error('fixture_concurrent_collect_busy');
  const state = {finished:false, status:0, item:false};
  globalThis.__akuConcurrentCollect = state;
  fetch('/collect', {method:'POST', cache:'no-store'}).then(async response => {
    const body = await response.json(); state.status = response.status === 200 ? 200 : 0;
    state.item = body?.postId === 'fixture-post-1'; state.finished = true;
  }).catch(() => { state.finished = true; });
  return true;
})()`;
const poll = `(() => { const state=globalThis.__akuConcurrentCollect; return state ? {finished:state.finished===true,status:state.status===200?200:0,item:state.item===true} : null; })()`;
const clear = `delete globalThis.__akuConcurrentCollect`;

export async function collectProtectedFixture(page, { now = () => performance.now(), pause = () => new Promise(resolve => setTimeout(resolve, 25)) } = {}) {
  let started = false;
  try {
    if (await page.evaluate(start) !== true) throw Error('fixture_concurrent_collection_invalid');
    started = true;
    const deadline = now() + 30000;
    for (let attempt = 0; attempt < 1200 && now() < deadline; attempt++) {
      const value = await page.evaluate(poll);
      if (!value || typeof value.finished !== 'boolean' || ![0, 200].includes(value.status) || typeof value.item !== 'boolean') throw Error('fixture_concurrent_collection_invalid');
      if (value.finished) return { status: value.status, item: value.item };
      await pause();
    }
    throw Error('fixture_concurrent_collection_timeout');
  } finally {
    if (started) await page.evaluate(clear).catch(() => {});
  }
}

// Fixed loopback fixture DOM capture. No caller URL, script, or source is accepted.
import { toObservation } from '../../internal/collection/headless/worker/observation.mjs';

export function fixtureDOMSnapshot() {
  const articles = [...document.querySelectorAll('article[data-fixture-post]')];
  if (articles.length < 1 || articles.length > 20) throw new Error('fixture_feed_shape');
  const posts = articles.map(article => {
    const id = article.getAttribute('data-fixture-post');
    const permalink = article.querySelector('a[data-permalink]')?.getAttribute('href');
    const text = article.querySelector('[data-text]')?.textContent;
    const author = article.querySelector('[data-author]')?.textContent;
    if (!/^\d{1,20}$/.test(id || '') || typeof text !== 'string' || !text.trim() || text.length > 8192 ||
        typeof author !== 'string' || author.length > 256 || typeof permalink !== 'string') throw new Error('fixture_post_shape');
    return { id, permalink, text, author };
  });
  return { posts, title:document.title, candidateCount:posts.length, documentReady:document.readyState==='complete',
    adapterVersion:'loopback-dom-v1', discoveryStrategy:'fixture_article_dom', scroll:{y:0,viewportHeight:innerHeight} };
}

export async function captureManualFixtureDOM(browser, origin, startupPins, signal) {
  const parsed = new URL(origin);
  if (parsed.origin !== origin || parsed.protocol !== 'http:' || parsed.hostname !== '127.0.0.1' || !parsed.port ||
      browser.role !== 'collector' || !startupPins?.targetId) throw new Error('fixture_capture_scope');
  const feed = await browser.page(origin);
  if (feed.targetId === startupPins.targetId) throw new Error('fixture_capture_target');
  const close=()=>browser.send('Target.closeTarget',{targetId:feed.targetId});
  const abort=()=>{void close().catch(()=>{});};
  signal?.addEventListener('abort',abort,{once:true});
  const timer=setTimeout(abort,15000);
  try {
    if(signal?.aborted)throw new Error('fixture_capture_canceled');
    await feed.navigate('/feed');
    if (await feed.evaluate('location.href') !== origin+'/feed') throw new Error('fixture_capture_origin');
    const snapshot = await feed.evaluate(`(${fixtureDOMSnapshot.toString()})()`);
    const capturedAt = new Date().toISOString();
    const observation = toObservation({source:'x',requestedUrl:origin+'/feed',snapshots:[snapshot],capturedAt,
      provenance:{kind:'loopback_synthetic_dom',nativeQualified:false},stopReason:'fixture_single_snapshot',performedScrolls:0});
    if (Buffer.byteLength(JSON.stringify(observation)) > 128*1024) throw new Error('fixture_capture_size');
    if(signal?.aborted)throw new Error('fixture_capture_canceled');
    return { startup:startupPins,captureTarget:feed.targetId,observation };
  } finally {clearTimeout(timer);signal?.removeEventListener('abort',abort);await close().catch(()=>{});}
}

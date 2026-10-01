const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
const statusId = url => { try { const u = new URL(url); return u.protocol === 'https:' && u.hostname === 'x.com'
  ? u.pathname.match(/^\/[^/]+\/status\/(\d+)(?:\/.*)?$/)?.[1] || null : null; } catch { return null; } };
const compact = value => String(value || '').replace(/\s+/g, ' ').trim();
export function matchesQuote(quote, target) {
  const text = compact(quote.text);
  if (text.length >= 20) return compact(target.text).startsWith(text.slice(0, 80));
  return quote.media?.some(media => target.media?.some(other => media.url === other.url)) === true;
}
// Only native detail targets are probed; feed capture never navigates away for this fallback.
export async function probeQuoteNavigation(browser, snapshot, options, readSnapshot) {
  const deadlineAt = options.deadlineAt;
  const remaining = () => {
    if (!deadlineAt) return 10000;
    const value = deadlineAt - Date.now();
    if (value <= 0) throw Object.assign(new Error('Capture deadline expired.'), { code: 'capture_timeout' });
    return value;
  };
  const evaluate = expression => browser.evaluate(expression, remaining());
  const send = (method, params) => browser.send(method, params, remaining());
  const pause = async ms => {
    const left = remaining();
    await delay(Math.min(ms, left));
  };
  const primaryId = statusId(options.url);
  const post = snapshot.posts.find(post => post.id === primaryId);
  if (!primaryId || !post?.quotedPost || post.quotedPost.permalink
    || post.quotedPost.identityStatus === 'conflicting_identity') return null;
  const probe = { primaryId, status: 'unresolved', restored: false };
  let recovery = null;
  try {
    const clicked = await evaluate(`globalThis.XHeadlessPoC.clickQuote(${JSON.stringify(primaryId)})`);
    if (!clicked) { probe.reason = 'no_scoped_quote_control'; return { probe, recovery }; }
    const deadline = Date.now() + 10000;
    while (Date.now() < deadline) {
      await pause(250);
      const destination = await evaluate('location.href');
      const quoteId = statusId(destination);
      if (!quoteId || quoteId === primaryId) continue;
      const destinationSnapshot = await readSnapshot();
      const target = destinationSnapshot.posts.find(post => post.id === quoteId);
      if (!target || !matchesQuote(post.quotedPost, target)) continue;
      recovery = { primaryId, quoteId, permalink: target.permalink, quotedText: post.quotedPost.text,
        identityStatus: 'identified', identityProvenance: 'observed_quote_navigation' };
      probe.status = 'identified';
      probe.targetIdentityAndContentMatched = true;
      probe.destinationEvidence = { id: target.id, permalink: target.permalink, text: target.text.slice(0, 4000), media: target.media };
      break;
    }
    if (!recovery) probe.reason = 'no_matching_quote_destination';
  } catch (error) { probe.reason = 'probe_error'; probe.error = error.message; }
  finally {
    const navigation = await send('Page.navigate', { url: options.url });
    if (navigation.errorText) throw new Error(`Quote probe return navigation failed: ${navigation.errorText}`);
    const deadline = Date.now() + 10000;
    while (Date.now() < deadline) {
      await pause(250);
      try {
        const restored = await readSnapshot();
        if (restored.posts.some(post => post.id === primaryId && post.quotedPost)) { probe.restored = true; break; }
      } catch (error) { if (!/context.*destroyed|Cannot find context/i.test(error.message)) throw error; }
    }
    if (!probe.restored) throw new Error('Quote probe could not verify return to the original primary post.');
  }
  return { probe, recovery };
}
export function applyQuoteRecovery(snapshot, recovery) {
  if (!recovery) return snapshot;
  const post = snapshot.posts.find(post => post.id === recovery.primaryId);
  if (!post?.quotedPost || compact(post.quotedPost.text).slice(0, 80) !== compact(recovery.quotedText).slice(0, 80)) return snapshot;
  const quote = post.quotedPost;
  if (quote.identityStatus === 'conflicting_identity') return snapshot;
  if (quote.permalink && statusId(quote.permalink) !== recovery.quoteId) {
    quote.permalink = null; post.parentPermalink = null; quote.identityStatus = 'conflicting_identity';
    quote.identityProvenance = null;
    if (!post.limitations.includes('quoted_identity_unresolved')) post.limitations.push('quoted_identity_unresolved');
    return snapshot;
  }
  Object.assign(quote, { permalink: recovery.permalink, identityStatus: recovery.identityStatus,
    identityProvenance: recovery.identityProvenance, navigationEvidence: { targetIdentityAndContentMatched: true, returnedToPrimary: true } });
  post.parentPermalink = recovery.permalink;
  post.limitations = post.limitations.filter(value => value !== 'quoted_identity_unresolved');
  return snapshot;
}

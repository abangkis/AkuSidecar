import { capturePrimitivesFor } from './capture-primitives.mjs';

const MAX_TARGETS = 3;
// Reserve half of each target's bounded budget for one transient retry.
// The shared recovery deadline never extends the enclosing capture deadline.
const MAX_PER_TARGET_MS = 8000;
const MAX_TOTAL_MS = 18000;
const X_COLLECT = '(async () => JSON.stringify(await globalThis.XHeadlessPoC.collect()))()';
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));

export function createXTextRecovery({ browser, assets, deadlineAt, perTargetMs = MAX_PER_TARGET_MS, totalMs = MAX_TOTAL_MS, maxTargets = MAX_TARGETS }) {
  const targetLimit = Math.max(0, Math.min(MAX_TARGETS, Number.isSafeInteger(maxTargets) ? maxTargets : MAX_TARGETS));
  const targetBudget = Math.max(1, Math.min(MAX_PER_TARGET_MS, Number.isFinite(perTargetMs) ? Math.trunc(perTargetMs) : MAX_PER_TARGET_MS));
  const totalBudget = Math.max(1, Math.min(MAX_TOTAL_MS, Number.isFinite(totalMs) ? Math.trunc(totalMs) : MAX_TOTAL_MS));
  const outcomes = new Map();
  let primitives;
  const helpers = () => primitives ??= capturePrimitivesFor(assets);
  const routeFor = value => xRoute(value, helpers());
  const captureDeadline = Number.isFinite(deadlineAt) ? deadlineAt : Date.now() + totalBudget;
  let attemptedTargets = 0;
  let totalDeadline = null;
  let page = null;
  let pagePromise = null;
  let closed = false;
  const unsupported = browser?.backend === 'browser_quiet_hidden' || typeof browser?.createTemporaryPage !== 'function';

  async function getPage(at) {
    if (page) return page;
    if (!pagePromise) {
      pagePromise = Promise.resolve().then(() => browser.createTemporaryPage(remaining(at)))
        .then(async created => {
          if (!created || typeof created.navigate !== 'function' || typeof created.evaluate !== 'function' || typeof created.close !== 'function') {
            throw new Error('temporary page API is incomplete');
          }
          if (closed) {
            try { await created.close(); }
            catch { throw Object.assign(new Error('temporary text recovery page cleanup failed.'), { code: 'temporary_target_cleanup_failed' }); }
            throw new Error('text recovery was closed');
          }
          page = created;
          return created;
        }).catch(error => {
          pagePromise = null;
          throw error;
        });
    }
    return within(pagePromise, at);
  }

  async function recoverTarget(target, post) {
    if (unsupported) return { kind: 'failed', limitation: 'permalink_capture_unavailable' };
    if (targetLimit === 0 || attemptedTargets >= targetLimit) return { kind: 'failed', limitation: 'permalink_capture_limit_reached' };
    attemptedTargets++;
    const now = Date.now();
    totalDeadline ??= Math.min(captureDeadline, now + totalBudget);
    const targetDeadline = Math.min(totalDeadline, now + targetBudget);
    let outcome;
    const attempts = [];
    for (let attempt = 0; attempt < 2; attempt++) {
      const started = Date.now();
      const at = attempt === 0 ? Math.min(targetDeadline, started + Math.ceil(targetBudget / 2)) : targetDeadline;
      outcome = await recoverAttempt(target, post, at);
      attempts.push({ stage: outcome.stage, outcome: outcome.kind, limitation: outcome.limitation || null,
        durationMs: Math.max(0, Date.now() - started) });
      if (outcome.kind !== 'failed' || !['permalink_capture_timeout', 'permalink_capture_failed'].includes(outcome.limitation)
          || Date.now() >= targetDeadline) break;
    }
    return { ...outcome, diagnostics: { attempts, elapsedMs: Math.max(0, Date.now() - now) } };
  }

  async function recoverAttempt(target, post, at) {
    let stage = 'deadline';
    const failed = limitation => ({ kind: 'failed', limitation, stage });
    const now = Date.now();
    if (at <= now) return failed('permalink_capture_timeout');
    if (!target.route || !target.identityId || target.identityId !== target.route.statusId) {
      return failed('permalink_identity_mismatch');
    }

    try {
      stage = 'target_setup';
      const targetPage = await getPage(at);
      stage = 'navigation';
      const navigation = await within(targetPage.navigate(target.route.url, remaining(at)), at);
      if (navigation?.errorText) return failed('permalink_capture_failed');

      stage = 'document_ready';
      let ready = false;
      while (Date.now() < at) {
        const state = await within(targetPage.evaluate('({url:location.href,ready:document.readyState})', remaining(at)), at);
        const actual = routeFor(state?.url);
        if (actual) {
          if (actual.url !== target.route.url || actual.statusId !== target.statusId) {
            return failed('permalink_identity_mismatch');
          }
          if (state.ready === 'complete') { ready = true; break; }
        } else if (typeof state?.url === 'string' && state.url.startsWith('https://x.com/') && state.url !== 'https://x.com/') {
          return failed('permalink_identity_mismatch');
        }
        await delay(Math.min(100, Math.max(1, at - Date.now())));
      }
      if (!ready) return failed('permalink_capture_timeout');

      stage = 'asset_injection';
      await within(targetPage.evaluate(`globalThis.AkuHeadlessCapturePolicy={allowContentExpansion:true,deadlineAt:${at}}`, remaining(at)), at);
      for (const asset of Array.isArray(assets) ? assets : []) {
        if (asset?.execute) await within(targetPage.evaluate(asset.content, remaining(at)), at);
      }
      let lastLimitation = 'permalink_capture_failed';
      while (Date.now() < at) {
        stage = 'text_collection';
        let snapshot;
        try {
          const encoded = await within(targetPage.evaluate(X_COLLECT, remaining(at)), at);
          if (typeof encoded !== 'string') throw new Error('X extractor returned no snapshot.');
          snapshot = JSON.parse(encoded);
        } catch (error) {
          if (/context.*destroyed|cannot find context/i.test(String(error?.message || error))) {
            lastLimitation = 'permalink_capture_failed';
            await delay(Math.min(80, Math.max(1, at - Date.now())));
            continue;
          }
          throw error;
        }
        if (snapshot.loginRequired === true || snapshot.challengeDetected === true || snapshot.sourceUnavailable === true) {
          return failed('permalink_capture_unavailable');
        }
        const finalUrl = routeFor(snapshot?.url);
        const actualUrl = routeFor(await within(targetPage.evaluate('location.href', remaining(at)), at));
        if ((finalUrl && finalUrl.url !== target.route.url) || (actualUrl && actualUrl.url !== target.route.url)) {
          return failed('permalink_identity_mismatch');
        }
        if (!finalUrl || !actualUrl) {
          lastLimitation = 'permalink_capture_failed';
          await delay(Math.min(80, Math.max(1, at - Date.now())));
          continue;
        }
        const matches = Array.isArray(snapshot.posts) ? snapshot.posts.filter(candidate =>
          postStatusId(candidate?.id) === target.statusId && routeFor(candidate?.permalink)?.url === target.route.url) : [];
        if (matches.length > 1) return failed('permalink_identity_mismatch');
        if (matches.length !== 1 || typeof matches[0].text !== 'string') {
          lastLimitation = 'permalink_capture_failed';
          await delay(Math.min(80, Math.max(1, at - Date.now())));
          continue;
        }
        if (matches[0].textCollapsed === true
            || !['expanded', 'visible_text_no_collapse_control'].includes(matches[0].textStatus)) {
          lastLimitation = 'permalink_text_unresolved';
          await delay(Math.min(80, Math.max(1, at - Date.now())));
          continue;
        }
        const text = matches[0].text;
        if (!helpers().evaluateTextReplacement({ originalText: post.text || '', recoveredText: text,
          identityMatches: true, resolved: true })) {
          lastLimitation = 'permalink_text_unresolved';
          await delay(Math.min(80, Math.max(1, at - Date.now())));
          continue;
        }
        if (codePointLength(text) >= 4000) {
          return { kind: 'truncated', text: [...text].slice(0, 4000).join(''), stage };
        }
        return { kind: 'verified', text, stage };
      }
      return failed(lastLimitation === 'permalink_capture_failed' ? 'permalink_capture_timeout' : lastLimitation);
    } catch (error) {
      if (error?.code === 'temporary_target_cleanup_failed') throw error;
      return failed(error?.code === 'capture_timeout' || Date.now() >= at
        ? 'permalink_capture_timeout' : 'permalink_capture_failed');
    }
  }

  async function recoverSnapshot(snapshot) {
    if (!Array.isArray(snapshot?.posts)) return snapshot;
    const posts = [];
    for (const post of snapshot.posts) {
      const needsRecovery = post?.textStatus === 'requires_permalink_capture' || post?.textCollapsed === true;
      if (!needsRecovery && outcomes.size === 0) { posts.push(post); continue; }
      const target = targetFor(post, routeFor);
      if (!target || (!needsRecovery && !outcomes.has(target.key))) {
        posts.push(post);
        continue;
      }
      let outcome = outcomes.get(target.key);
      if (!outcome && needsRecovery) {
        outcome = await recoverTarget(target, post);
        outcomes.set(target.key, outcome);
      }
      posts.push(outcome && !preservesCurrentText(post, outcome) ? applyOutcome(post, outcome, helpers()) : post);
    }
    return { ...snapshot, posts };
  }

  async function close() {
    if (closed) return;
    closed = true;
    if (page) {
      const targetPage = page;
      page = null;
      try { await targetPage.close(); }
      catch { throw Object.assign(new Error('temporary text recovery page cleanup failed.'), { code: 'temporary_target_cleanup_failed' }); }
      return;
    }
    if (pagePromise) {
      try {
        const targetPage = await pagePromise;
        try { await targetPage.close(); }
        catch { throw Object.assign(new Error('temporary text recovery page cleanup failed.'), { code: 'temporary_target_cleanup_failed' }); }
      }
      catch (error) {
        if (error?.code === 'temporary_target_cleanup_failed') throw error;
      }
    }
  }

  return { recoverSnapshot, close };
}

function targetFor(post, routeFor) {
  if (!post || typeof post !== 'object') return null;
  const route = routeFor(post.permalink);
  const identityId = postStatusId(post.id);
  const statusId = route?.statusId || identityId;
  if (!statusId) return null;
  const key = `${statusId}\0${route?.url || String(post.permalink || '')}\0${identityId || ''}`;
  return { key, route, statusId, identityId };
}

function xRoute(value, primitives) {
  const url = primitives.canonicalizeXPermalink(value);
  if (!url) return null;
  const statusId = new URL(url).pathname.match(/^\/[^/]+\/status\/(\d+)$/)?.[1] || null;
  return statusId ? { url, statusId } : null;
}

function postStatusId(value) {
  if (typeof value !== 'string') return null;
  if (/^\d+$/.test(value)) return value;
  return /^x:status:(\d+)$/.exec(value)?.[1] || null;
}

function codePointLength(value) {
  return Array.from(String(value)).length;
}

function applyOutcome(post, outcome, primitives) {
  const limitations = Array.isArray(post.limitations) ? post.limitations.filter(value => typeof value === 'string') : [];
  if (outcome.kind === 'verified') {
    return {
      ...post,
      textRecovery: outcome.diagnostics,
      textCompleteness: primitives.textCompleteness({ recoveryVerified: true }),
      text: outcome.text,
      textStatus: 'permalink_text_verified',
      textCollapsed: false,
      limitations: unique(limitations.filter(value => !['text_may_be_collapsed', 'text_may_be_truncated', 'text_truncated'].includes(value))),
    };
  }
  const textLimitations = outcome.kind === 'truncated'
    ? ['text_may_be_truncated', 'text_truncated'] : [outcome.limitation];
  return {
    ...post,
    textRecovery: outcome.diagnostics,
    textCompleteness: primitives.textCompleteness({ truncated: outcome.kind === 'truncated', collapsed: true }),
    ...(outcome.kind === 'truncated' ? { text: outcome.text } : {}),
    textStatus: 'requires_permalink_capture',
    limitations: unique([...limitations, ...textLimitations]),
  };
}

function preservesCurrentText(post, outcome) {
  const currentIsComplete = post?.textCollapsed !== true
    && ['expanded', 'visible_text_no_collapse_control', 'permalink_text_verified'].includes(post?.textStatus);
  if (currentIsComplete) return true;
  if (['verified', 'truncated'].includes(outcome.kind)
      && codePointLength(post?.text || '') > codePointLength(outcome.text || '')) return true;
  return false;
}

function unique(values) {
  return [...new Set(values)];
}

function remaining(deadline) {
  const value = Math.trunc(deadline - Date.now());
  if (value < 1) throw Object.assign(new Error('permalink text recovery timed out'), { code: 'capture_timeout' });
  return value;
}

function within(promise, deadline) {
  const ms = remaining(deadline);
  let timer;
  return Promise.race([
    Promise.resolve(promise),
    new Promise((_, reject) => { timer = setTimeout(() => reject(Object.assign(new Error('permalink text recovery timed out'), { code: 'capture_timeout' })), ms); }),
  ]).finally(() => clearTimeout(timer));
}

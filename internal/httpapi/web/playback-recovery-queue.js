// Failed playback is user initiated. Keep its recovery intent while capture is
// busy, without starting parallel jobs or retaining entire Timeline payloads.
export function createPlaybackRecoveryQueue({ isReady, resolveEntry, recover, maxEntries = 64 }) {
  const pending = new Map();
  const attempted = new Map();
  let draining = false;

  function enqueue(request) {
    if (!request?.id || !request.source || !request.playbackUrl) return false;
    if (attempted.get(request.id) === request.playbackUrl || pending.get(request.id)?.playbackUrl === request.playbackUrl) return false;
    if (!pending.has(request.id) && pending.size >= maxEntries) return false;
    pending.set(request.id, { id: request.id, source: request.source, playbackUrl: request.playbackUrl });
    return true;
  }

  async function drain() {
    if (draining) return;
    draining = true;
    try {
      while (pending.size && isReady()) {
        const [id, request] = pending.entries().next().value;
        const entry = resolveEntry(request);
        pending.delete(id);
        // A refresh may already have replaced the failed URL or removed the post.
        if (!entry) continue;
        if (!attempted.has(id) && attempted.size >= maxEntries) attempted.delete(attempted.keys().next().value);
        attempted.set(id, request.playbackUrl);
        await recover(entry);
      }
    } finally {
      draining = false;
    }
  }

  return { enqueue, drain };
}

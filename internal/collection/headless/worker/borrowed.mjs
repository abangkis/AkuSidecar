// A Quiet worker owns Node only. Chrome target/session IDs and the root CDP
// connection remain in Go; the worker asks for source-scoped page operations.
const METHODS = new Set(['Page.navigate', 'Runtime.evaluate', 'Input.dispatchMouseEvent']);

export function createBorrowedChrome(rpc, version) {
  let closed = false;
  const pages = new Map();
  return {
    backend: 'browser_quiet_hidden',
    version,
    async forSource(source) {
      if (closed) throw new Error('Quiet worker is closed.');
      if (!['x', 'facebook'].includes(source)) throw new Error('Unsupported Quiet source.');
      if (pages.has(source)) return pages.get(source);
      const send = (method, params = {}, timeoutMs = 15000) => {
        if (closed || !METHODS.has(method)) return Promise.reject(new Error('Quiet operation is unavailable.'));
        return rpc(source, method, params, timeoutMs);
      };
      const page = {
        send,
        navigate: (url, timeoutMs) => send('Page.navigate', {url}, timeoutMs),
        async evaluate(expression, timeoutMs) {
          const result = await send('Runtime.evaluate', {expression, returnByValue: true, awaitPromise: true}, timeoutMs);
          if (result.exceptionDetails) throw new Error('Quiet page evaluation failed.');
          return result.result?.value;
        },
      };
      pages.set(source, page);
      return page;
    },
    async close() { closed = true; pages.clear(); }, // Never Browser.close or pipe EOF.
  };
}

export function createBorrowedRPC(write) {
  let sequence = 0;
  let closed = false;
  const pending = new Map();
  return {
    send(source, method, params, timeoutMs) {
      if (closed || pending.size >= 32) return Promise.reject(new Error('Quiet RPC unavailable.'));
      const rpcId = ++sequence;
      const bounded = Math.max(1, Math.min(15000, Number.isFinite(timeoutMs) ? timeoutMs : 15000));
      return new Promise((resolve, reject) => {
        const timer = setTimeout(() => {
          pending.delete(rpcId);
          reject(Object.assign(new Error('Quiet CDP operation timed out.'), {code: 'capture_timeout'}));
        }, bounded);
        pending.set(rpcId, {resolve, reject, timer});
        void write({type: 'cdp', rpcId, source, method, params, timeoutMs: bounded}).catch(() => {
          const item = pending.get(rpcId);
          if (!item) return;
          pending.delete(rpcId); clearTimeout(item.timer);
          item.reject(new Error('Quiet RPC write failed.'));
        });
      });
    },
    receive(message) {
      if (message?.type !== 'cdp_result') return false;
      const item = pending.get(message.rpcId);
      if (!item) return true; // Late replies never resolve a new operation.
      pending.delete(message.rpcId); clearTimeout(item.timer);
      if (message.ok === true) item.resolve(message.result ?? {});
      else item.reject(new Error('Quiet CDP operation failed.'));
      return true;
    },
    close() {
      closed = true;
      for (const item of pending.values()) { clearTimeout(item.timer); item.reject(new Error('Quiet RPC closed.')); }
      pending.clear();
    },
  };
}

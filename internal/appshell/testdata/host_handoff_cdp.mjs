// Opt-in smoke helper: one command against an isolated loopback Chrome only.
const [endpoint, method, encoded] = process.argv.slice(2);
const url = new URL(endpoint);
if (url.protocol !== 'ws:' || url.hostname !== '127.0.0.1' ||
    !new Set(['Target.getTargets', 'Target.createTarget', 'Target.closeTarget',
      'Browser.getWindowForTarget', 'Browser.getVersion']).has(method)) {
  throw new Error('Only isolated loopback smoke commands are permitted.');
}
const params = JSON.parse(encoded);
if (method === 'Target.createTarget' &&
    (params.background !== true || params.windowState !== 'minimized')) {
  throw new Error('Smoke windows must start minimized in the background.');
}
const ws = new WebSocket(endpoint);
const timer = setTimeout(() => {
  console.error('Isolated CDP smoke command timed out.');
  process.exitCode = 1;
  ws.close();
}, 10000);
ws.addEventListener('open', () => ws.send(JSON.stringify({id: 1, method, params})));
ws.addEventListener('message', ({data}) => {
  const message = JSON.parse(data);
  if (message.id !== 1) return;
  clearTimeout(timer);
  if (message.error) {
    console.error(JSON.stringify(message.error));
    process.exitCode = 1;
  } else {
    console.log(JSON.stringify(message.result));
  }
  ws.close();
});
ws.addEventListener('error', (event) => {
  clearTimeout(timer);
  console.error('Isolated CDP smoke connection failed:', event.error?.message ?? event.message ?? 'unknown');
  process.exitCode = 1;
});

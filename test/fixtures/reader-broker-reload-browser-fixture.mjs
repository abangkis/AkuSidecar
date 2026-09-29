// Loopback-only integration fixture. No real broker, native host, or user data.
// Open /#aku-startup=<64 hex characters>. A new document must be requested once;
// the real watchdog strips the fragment and the real relay performs recovery.
import { createServer } from "node:http";
import { readFile } from "node:fs/promises";

let documents = 0;
const assets = new Map([
  ["/split-ui-bridge.js", "../../internal/httpapi/split_ui_bridge.js"],
  ["/startup-watchdog.js", "../../internal/httpapi/web/startup-watchdog.js"],
  ["/native-post-diagnostics.js", "../../internal/httpapi/web/native-post-diagnostics.js"],
]);
const server = createServer(async (request, response) => {
  const path = new URL(request.url, "http://127.0.0.1:12779").pathname;
  response.setHeader("Cache-Control", "no-store");
  if (path === "/") {
    documents++;
    response.setHeader("Content-Type", "text/html; charset=utf-8");
    response.end(`<!doctype html><html><head><script src="/split-ui-bridge.js"></script></head><body>
      <h1>Reader broker reload regression</h1><p>Document request: ${documents}</p>
      <p id="result">Waiting for bounded recovery...</p>
      <section id="startup-recovery" hidden><h2 id="startup-recovery-heading"></h2><p id="startup-recovery-detail"></p></section>
      <script src="/startup-watchdog.js"></script><script src="/native-post-diagnostics.js"></script>
      <script>
        const documentIdentity = performance.timeOrigin;
        const previous = sessionStorage.getItem('fixtureDocumentIdentity');
        sessionStorage.setItem('fixtureDocumentIdentity', String(documentIdentity));
        setTimeout(() => {
          const events = window.akuNativePostDiagnostics.read();
          const attempts = events.filter(e => e.phase === 'broker_reload_attempt').length;
          const changed = previous !== null && previous !== String(documentIdentity);
          document.getElementById('result').textContent = JSON.stringify({
            documentChanged: changed, documentRequests: ${documents}, attempts,
            status: window.akuReaderBrokerStatus, fragmentStripped: location.hash === ''
          });
        }, 6500);
      </script></body></html>`);
    return;
  }
  const asset = assets.get(path);
  if (!asset) { response.writeHead(404); response.end(); return; }
  try {
    response.setHeader("Content-Type", "text/javascript; charset=utf-8");
    response.end(await readFile(new URL(asset, import.meta.url)));
  } catch { response.writeHead(500); response.end(); }
});
server.listen(12779, "127.0.0.1", () => console.log("Reader reload fixture: http://127.0.0.1:12779/"));

// Local-only visual fixture: serve the real first-run HTML while simulating a
// missing application module. Never binds outside loopback or serves user data.
import { createServer } from "node:http";
import { readFile } from "node:fs/promises";

const assets = new Map([
  ["/", { file: new URL("../../internal/httpapi/web/index.html", import.meta.url), type: "text/html; charset=utf-8" }],
  ["/styles.css", { file: new URL("../../internal/httpapi/web/styles.css", import.meta.url), type: "text/css; charset=utf-8" }],
  ["/startup-watchdog.js", { file: new URL("../../internal/httpapi/web/startup-watchdog.js", import.meta.url), type: "text/javascript; charset=utf-8" }],
]);

const server = createServer(async (request, response) => {
  const pathname = new URL(request.url, "http://127.0.0.1:12778").pathname;
  response.setHeader("Cache-Control", "no-store");
  response.setHeader("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'");
  if (pathname === "/app.js") {
    response.writeHead(404, { "Content-Type": "text/plain; charset=utf-8" });
    response.end("Simulated missing application module");
    return;
  }
  const asset = assets.get(pathname);
  if (!asset) {
    response.writeHead(404);
    response.end();
    return;
  }
  try {
    const body = await readFile(asset.file);
    response.writeHead(200, { "Content-Type": asset.type });
    response.end(body);
  } catch {
    response.writeHead(500);
    response.end();
  }
});

server.listen(12778, "127.0.0.1", () => {
  process.stdout.write("startup-watchdog-fixture ready on http://127.0.0.1:12778/\n");
});

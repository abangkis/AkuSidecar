import { readFileSync } from "node:fs";

// Load the classic diagnostics scripts in the same order as the embedded UI.
const web = new URL("../../internal/httpapi/web/", import.meta.url);
const html = readFileSync(new URL("index.html", web), "utf8");
const files = [...html.matchAll(/<script src="\/([^"?]+)(?:\?[^"]*)?"><\/script>/g)]
  .map(match => match[1])
  .filter(name => ["diagnostic-state-core.js", "diagnostic-recorder.js", "web-diagnostics.js", "native-post-diagnostics.js"].includes(name));
export const webDiagnosticsScripts = files.map(name => readFileSync(new URL(name, web), "utf8")).join("\n");

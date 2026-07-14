const port = process.env.AKU_BROWSER_PORT || "47821";
const response = await fetch(`http://127.0.0.1:${port}/api/preferences/benchmark`);
if (!response.ok) throw new Error(`AkuSidecar benchmark endpoint returned HTTP ${response.status}`);
const payload = await response.json();
console.log(JSON.stringify(payload.benchmark, null, 2));

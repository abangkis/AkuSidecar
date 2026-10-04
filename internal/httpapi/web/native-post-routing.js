// The current runtime chooses the route, not the saved collection preference.
// A headless preference can temporarily have a Browser owner for a reader or FB.
export function createNativePostRouter({ readRuntime, openHeadless, openForeground }) {
  return async function open(request, { signal } = {}) {
    signal?.throwIfAborted();
    const runtime = await readRuntime();
    signal?.throwIfAborted();
    // pending also means a ready Browser is borrowed by an existing reader.
    if (runtime?.state && runtime.state !== "ready") {
      throw new Error("Chrome is changing collection mode. Wait until it is ready and retry.");
    }
    if (runtime?.effective === "headless" || (runtime?.effective === "browser" && runtime?.nativeReaderOnly)) return openHeadless(request, runtime);
    if (runtime?.effective === "browser") return openForeground(request, runtime);
    throw new Error("The current Chrome mode could not be confirmed.");
  };
}

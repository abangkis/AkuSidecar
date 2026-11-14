// Owns fetch/stage/apply sequencing without accessing the DOM. Snapshot equality,
// detection status, reading guards, and presentation updates remain caller ports.
export function createTimelineRefreshCoordinator({
  fetchSnapshot,
  isUnchanged,
  applySnapshot,
  hasPendingDetection,
  onSnapshot = () => {},
  onStage = () => {},
  canApply = () => true,
  waitForIdle = () => Promise.resolve(),
  getContextKey = () => "",
  onError = () => {},
  schedule = setTimeout,
  cancel = clearTimeout,
  pollIntervalMs = 5000,
}) {
  // Factories can be composed before their host/state ports are initialized.
  let context;
  let contextInitialized = false;
  let epoch = 0;
  let generation = 0;
  let stage = null;
  let pending = false;
  let timer = null;
  let flight = null;
  let application = null;

  function cancelPoll() {
    if (timer !== null) cancel(timer);
    timer = null;
  }

  function clearStage() {
    stage = null;
    onStage(null);
  }

  function invalidate() {
    epoch++;
    pending = false;
    cancelPoll();
    clearStage();
  }

  function syncContext() {
    const nextContext = getContextKey();
    if (!contextInitialized) {
      context = nextContext;
      contextInitialized = true;
    } else if (nextContext !== context) {
      context = nextContext;
      invalidate();
    }
    return context;
  }

  function isCurrent(token) {
    syncContext();
    return token.epoch === epoch && token.context === context;
  }

  function armPoll() {
    if (!pending || timer !== null) return;
    timer = schedule(() => {
      timer = null;
      return request();
    }, pollIntervalMs);
  }

  function setPending(value) {
    syncContext();
    pending = Boolean(value);
    if (pending) armPoll();
    else cancelPoll();
  }

  async function fetchOne() {
    syncContext();
    cancelPoll();
    // Generation is local request identity, not a server revision or ordering claim.
    const token = { epoch, context, generation: ++generation };
    try {
      const snapshot = await fetchSnapshot();
      if (!isCurrent(token)) return;
      // This port may synchronize non-destructive status, never timeline cards.
      onSnapshot(snapshot);
      if (!isCurrent(token)) return;
      const unchanged = isUnchanged(snapshot);
      const nextPending = hasPendingDetection(snapshot);
      if (!isCurrent(token)) return;
      setPending(nextPending);
      if (unchanged) clearStage();
      else {
        stage = { snapshot, epoch: token.epoch, context: token.context, generation: token.generation };
        onStage(snapshot);
      }
    } catch (error) {
      if (isCurrent(token)) onError(error);
    } finally {
      // A transient fetch failure preserves the last known detection work.
      if (isCurrent(token)) armPoll();
    }
  }

  function request() {
    syncContext();
    if (flight) {
      flight.replay = true;
      return flight.promise;
    }
    const nextFlight = { replay: false, promise: null };
    flight = nextFlight;
    nextFlight.promise = (async () => {
      try {
        do {
          nextFlight.replay = false;
          await fetchOne();
        } while (nextFlight.replay);
      } finally {
        if (flight === nextFlight) flight = null;
      }
    })();
    return nextFlight.promise;
  }

  function apply() {
    syncContext();
    if (application) return application;
    if (!stage) return Promise.resolve(false);
    const token = { epoch, context };
    // Set the shared promise before invoking caller ports, including idle waits.
    application = Promise.resolve().then(async () => {
      try {
        await waitForIdle();
        if (!isCurrent(token) || !stage || !canApply()) return false;
        if (!isCurrent(token) || !stage) return false;
        // A newer accepted stage from the same context replaces the one clicked.
        const selected = stage;
        await applySnapshot(selected.snapshot);
        if (!isCurrent(token)) return false;
        if (stage === selected) clearStage();
        return true;
      } catch (error) {
        // The presentation port owns invalidating any partially rendered output.
        // Keep the stage available so an explicit retry can rebuild it.
        if (isCurrent(token)) onError(error);
        return false;
      } finally {
        application = null;
      }
    });
    return application;
  }

  return {
    request,
    apply,
    invalidate,
    setPending,
    get staged() { return stage?.snapshot ?? null; },
    get applying() { return application !== null; },
  };
}

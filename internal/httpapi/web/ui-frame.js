// Coalesce independent scroll/layout requests into one callback per frame.
// Tasks scheduled while flushing are deferred to the following frame.
export function createFrameTaskQueue({ requestFrame, run }) {
  let pending = false;
  let tasks = new Set();
  return {
    schedule(task) {
      tasks.add(task);
      if (pending) return;
      pending = true;
      requestFrame(() => {
        const current = tasks;
        tasks = new Set();
        pending = false;
        run(current);
      });
    },
  };
}

export function setInlineStyle(element, name, value) {
  if (element.style.getPropertyValue(name) === value) return;
  if (value === "") element.style.removeProperty(name);
  else element.style.setProperty(name, value);
}

export function setAttributeValue(element, name, value) {
  if (element.getAttribute(name) !== value) element.setAttribute(name, value);
}
